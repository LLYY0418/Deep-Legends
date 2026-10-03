package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type r196RoundTrip func(*http.Request) (*http.Response, error)

func (f r196RoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type r196Events struct {
	mu   sync.Mutex
	rows []map[string]any
}

func (e *r196Events) record(row map[string]any) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.rows = append(e.rows, row)
}
func (e *r196Events) named(name string) []map[string]any {
	e.mu.Lock()
	defer e.mu.Unlock()
	var out []map[string]any
	for _, row := range e.rows {
		if row["event"] == name {
			out = append(out, row)
		}
	}
	return out
}
func r196Response(data []byte) *http.Response {
	r := updateResponse(200, data)
	r.ContentLength = int64(len(data))
	return r
}

type r196PacedBody struct {
	ctx  context.Context
	data []byte
	rate int64
}

func (b *r196PacedBody) Read(p []byte) (int, error) {
	if len(b.data) == 0 {
		return 0, io.EOF
	}
	n := min(len(p), len(b.data), 1024)
	timer := time.NewTimer(time.Duration(float64(n) / float64(b.rate) * float64(time.Second)))
	defer timer.Stop()
	select {
	case <-b.ctx.Done():
		return 0, b.ctx.Err()
	case <-timer.C:
	}
	copy(p, b.data[:n])
	b.data = b.data[n:]
	return n, nil
}
func (b *r196PacedBody) Close() error { return nil }
func TestR196ProbeSelectsFastMirror(t *testing.T) {
	data := bytes.Repeat([]byte("installer"), 256*1024)
	u := updateTestManager(t, data)
	u.mirrors = []string{"", "https://fast.test/", "https://html.test/"}
	events := &r196Events{}
	u.diagnostic = events.record
	var starts atomic.Int32
	var selected string
	var mu sync.Mutex
	u.client = &http.Client{Transport: r196RoundTrip(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Range") == "bytes=0-1048575" {
			starts.Add(1)
			if _, ok := r.Context().Deadline(); !ok {
				t.Error("probe lacks deadline")
			}
			if r.URL.Host == "html.test" {
				return r196Response([]byte("<html>error</html>")), nil
			}
			response := r196Response(data)
			rate := int64(2 * 1024 * 1024)
			if r.URL.Host == "github.com" {
				rate = 5 * 1024
			}
			response.Body = &r196PacedBody{r.Context(), data, rate}
			return response, nil
		}
		mu.Lock()
		selected = r.URL.Host
		mu.Unlock()
		return r196Response(data), nil
	})}
	if err := u.Download(); err != nil {
		t.Fatal(err)
	}
	u.mu.Lock()
	done := u.downloadDone
	u.mu.Unlock()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("probe race stuck")
	}
	mu.Lock()
	got := selected
	mu.Unlock()
	if got != "fast.test" || u.Status().State != "ready" || starts.Load() != 3 {
		t.Fatalf("selection=%s status=%+v starts=%d", got, u.Status(), starts.Load())
	}
	probes := events.named("update_download_probe")
	if len(probes) != 3 {
		t.Fatalf("probes=%v", probes)
	}
	for _, row := range probes {
		if row["mirror_prefix"] == "https://html.test/" && row["ok"] != false {
			t.Fatal("HTML accepted")
		}
	}
	final := filepath.Join(u.directory, u.manifest.Asset.Name)
	if err := verifyUpdateFile(context.Background(), final, u.manifest.Asset); err != nil {
		t.Fatal(err)
	}
}

// Deterministic transfer clock: actual body reads and watcher ticks exercise the
// 15-second / trailing 10-second guard without sleeping through a large fixture.
type r196ClockBody struct {
	ctx     context.Context
	data    []byte
	clock   *atomic.Int64
	ticks   chan time.Time
	ack     chan struct{}
	reads   int
	allSlow bool
}

func (b *r196ClockBody) Read(p []byte) (int, error) {
	if len(b.data) == 0 {
		return 0, io.EOF
	}
	n := min(len(p), len(b.data))
	step := 125 * time.Millisecond
	if b.reads >= 160 {
		step = 640 * time.Millisecond
	}
	if b.allSlow {
		n = min(n, 32)
		step = time.Second
	}
	b.reads++
	now := time.Unix(0, b.clock.Add(int64(step)))
	select {
	case b.ticks <- now:
	case <-b.ctx.Done():
		return 0, b.ctx.Err()
	}
	select {
	case <-b.ack:
	case <-b.ctx.Done():
		return 0, b.ctx.Err()
	}
	if b.ctx.Err() != nil {
		return 0, b.ctx.Err()
	}
	copy(p, b.data[:n])
	b.data = b.data[n:]
	return n, nil
}
func (b *r196ClockBody) Close() error { return nil }
func TestR196SlowSourceSwitchResumesAndHashes(t *testing.T) {
	data := bytes.Repeat([]byte("abcdefgh"), 1024*1024)
	u := updateTestManager(t, data)
	u.mirrors = []string{"https://a.test/", "https://b.test/"}
	events := &r196Events{}
	u.diagnostic = events.record
	var clock atomic.Int64
	ticks := make(chan time.Time)
	ack := make(chan struct{}, 1)
	u.downloadClock = func() time.Time { return time.Unix(0, clock.Load()) }
	u.progressTicks = func() (<-chan time.Time, func()) { return ticks, func() {} }
	u.notify = func(kind string, _ any) {
		if kind == "update:progress" {
			select {
			case ack <- struct{}{}:
			default:
			}
		}
	}
	var offset int64
	var head bool
	u.client = &http.Client{Transport: r196RoundTrip(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Range") == "bytes=0-1048575" {
			delay := 20 * time.Millisecond
			if r.URL.Host == "b.test" {
				delay = 100 * time.Millisecond
			}
			time.Sleep(delay)
			response := r196Response(data[:updateProbeBytes])
			response.StatusCode = 206
			response.Header.Set("Content-Range", fmt.Sprintf("bytes 0-%d/%d", updateProbeBytes-1, len(data)))
			return response, nil
		}
		if r.URL.Host == "a.test" {
			response := r196Response(data)
			response.Body = &r196ClockBody{ctx: r.Context(), data: data, clock: &clock, ticks: ticks, ack: ack}
			return response, nil
		}
		if r.Method == "HEAD" {
			head = true
			response := r196Response(nil)
			response.Header.Set("Accept-Ranges", "bytes")
			return response, nil
		}
		if _, err := fmt.Sscanf(r.Header.Get("Range"), "bytes=%d-", &offset); err != nil {
			t.Fatal(err)
		}
		response := r196Response(data[offset:])
		response.StatusCode = 206
		response.Header.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", offset, len(data)-1, len(data)))
		return response, nil
	})}
	if err := u.Download(); err != nil {
		t.Fatal(err)
	}
	waitUpdateDownload(t, u)
	switches := events.named("update_download_source_switched")
	if len(switches) != 1 || switches[0]["reason"] != "slow" || !head || offset <= 0 || u.Status().State != "ready" {
		t.Fatalf("switches=%v head=%v offset=%d state=%s", switches, head, offset, u.Status().State)
	}
	if clock.Load() < int64(20*time.Second) {
		t.Fatal("did not exercise 20 seconds of healthy transfer before slowdown")
	}
	if err := verifyUpdateFile(context.Background(), filepath.Join(u.directory, u.manifest.Asset.Name), u.manifest.Asset); err != nil {
		t.Fatal(err)
	}
}
func TestR196AllSlowStayFastest(t *testing.T) {
	data := bytes.Repeat([]byte("x"), 1024)
	u := updateTestManager(t, data)
	u.mirrors = []string{"", "https://slow.test/"}
	events := &r196Events{}
	u.diagnostic = events.record
	var clock atomic.Int64
	ticks := make(chan time.Time)
	ack := make(chan struct{}, 1)
	u.downloadClock = func() time.Time { return time.Unix(0, clock.Load()) }
	u.progressTicks = func() (<-chan time.Time, func()) { return ticks, func() {} }
	u.notify = func(kind string, _ any) {
		if kind == "update:progress" {
			select {
			case ack <- struct{}{}:
			default:
			}
		}
	}
	var downloads int
	u.client = &http.Client{Transport: r196RoundTrip(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Range") == "bytes=0-1048575" {
			delay := 50 * time.Millisecond
			if r.URL.Host != "github.com" {
				delay = 100 * time.Millisecond
			}
			time.Sleep(delay)
			return r196Response(data), nil
		}
		downloads++
		if r.URL.Host != "github.com" {
			t.Error("left fastest slow source")
		}
		response := r196Response(data)
		response.Body = &r196ClockBody{ctx: r.Context(), data: data, clock: &clock, ticks: ticks, ack: ack, allSlow: true}
		return response, nil
	})}
	u.Download()
	waitUpdateDownload(t, u)
	if u.Status().State != "ready" || downloads != 1 || len(events.named("update_download_source_switched")) != 0 {
		t.Fatalf("state=%+v downloads=%d", u.Status(), downloads)
	}
}
func TestR196PreferredSourcePersistedAndCheckedFirst(t *testing.T) {
	data := []byte("setup")
	u := updateTestManager(t, data)
	u.mirrors = []string{"", "https://preferred.test/"}
	events := &r196Events{}
	u.diagnostic = events.record
	manifest := *u.manifest
	var checked []string
	u.client = &http.Client{Transport: r196RoundTrip(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "latest.json") {
			checked = append(checked, r.URL.Host)
			raw, _ := json.Marshal(manifest)
			return r196Response(raw), nil
		}
		if r.URL.Host == "github.com" {
			return updateResponse(503, nil), nil
		}
		return r196Response(data), nil
	})}
	u.Download()
	waitUpdateDownload(t, u)
	if u.Settings().PreferredSource != "https://preferred.test/" {
		t.Fatalf("preference=%+v", u.Settings())
	}
	restored := newUpdateManager("0.11.2", u.store, nil)
	defer restored.Close()
	if restored.Settings().PreferredSource != u.Settings().PreferredSource {
		t.Fatal("preference not persisted")
	}
	if !u.Check(true) {
		t.Fatal("check blocked")
	}
	waitUpdateCheck(t, u)
	if len(checked) != 1 || checked[0] != "preferred.test" {
		t.Fatalf("check order=%v", checked)
	}
	if got := updatePreferredSources(u.mirrors, u.Settings().PreferredSource); len(got) != 2 || got[1] != "" {
		t.Fatalf("direct not second: %v", got)
	}
	rows := events.named("update_check_succeeded")
	if len(rows) != 1 || rows[0]["mirror_prefix"] != "https://preferred.test/" {
		t.Fatalf("check diagnostics=%v", rows)
	}
}
func TestR196AllProbesFailRetainsFallback(t *testing.T) {
	data := []byte("setup")
	u := updateTestManager(t, data)
	u.mirrors = []string{"", "https://fallback.test/"}
	var requests []string
	var mu sync.Mutex
	u.client = &http.Client{Transport: r196RoundTrip(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Range") == "bytes=0-1048575" {
			return updateResponse(500, nil), nil
		}
		mu.Lock()
		requests = append(requests, r.URL.Host)
		mu.Unlock()
		if r.URL.Host == "github.com" {
			return updateResponse(503, nil), nil
		}
		return r196Response(data), nil
	})}
	u.Download()
	waitUpdateDownload(t, u)
	if strings.Join(requests, ",") != "github.com,fallback.test" || u.Status().State != "ready" {
		t.Fatalf("fallback=%v %+v", requests, u.Status())
	}
}
func TestR196CancelDeletesPartialAndRecords(t *testing.T) {
	data := []byte("setup")
	u := updateTestManager(t, data)
	u.mirrors = []string{""}
	events := &r196Events{}
	u.diagnostic = events.record
	started := make(chan struct{})
	part := filepath.Join(u.directory, u.manifest.Asset.Name+".part")
	os.WriteFile(part, data[:2], 0600)
	u.client = &http.Client{Transport: r196RoundTrip(func(r *http.Request) (*http.Response, error) {
		close(started)
		<-r.Context().Done()
		return nil, r.Context().Err()
	})}
	u.Download()
	<-started
	u.Cancel()
	if _, err := os.Stat(part); !os.IsNotExist(err) {
		t.Fatal("partial retained")
	}
	if rows := events.named("update_download_cancelled"); len(rows) != 1 || rows[0]["received_bytes"] != int64(2) {
		t.Fatalf("cancel=%v", rows)
	}
}
func TestR196DiagnosticPrivacyAndProxyPresence(t *testing.T) {
	data := []byte("setup")
	u := updateTestManager(t, data)
	u.mirrors = []string{"https://custom.test/private-token/"}
	events := &r196Events{}
	u.diagnostic = events.record
	t.Setenv("HTTP_PROXY", "http://secret-user:secret-password@proxy.test:8080")
	t.Setenv("HTTPS_PROXY", "https://private-proxy.test")
	u.client = &http.Client{Transport: r196RoundTrip(func(r *http.Request) (*http.Response, error) { return r196Response(data), nil })}
	u.Download()
	waitUpdateDownload(t, u)
	raw, _ := json.Marshal(events.rows)
	for _, private := range []string{u.directory, u.manifest.Asset.URL, "private-token", "secret-user", "secret-password", "private-proxy.test"} {
		if strings.Contains(string(raw), private) {
			t.Fatalf("diagnostic leaked %s", private)
		}
	}
	rows := events.named("update_proxy_env")
	if len(rows) != 1 || rows[0]["http_proxy_set"] != true || rows[0]["https_proxy_set"] != true {
		t.Fatalf("proxy flags=%v", rows)
	}
}

func TestR196RestartRestoresReadySnapshot(t *testing.T) {
	data := []byte("ready setup")
	u := updateTestManager(t, data)
	u.mirrors = []string{""}
	manifest := *u.manifest
	u.client = &http.Client{Transport: r196RoundTrip(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "latest.json") {
			raw, _ := json.Marshal(manifest)
			return r196Response(raw), nil
		}
		return r196Response(data), nil
	})}
	u.Download()
	waitUpdateDownload(t, u)
	u.Check(true)
	waitUpdateCheck(t, u)
	u.Close()
	readySnapshot := make(chan updateStatus, 1)
	restored := newUpdateManager("0.11.2", u.store, func(kind string, value any) {
		if kind == "update:status" {
			status := value.(updateStatus)
			if status.State == "ready" {
				readySnapshot <- status
			}
		}
	})
	defer restored.Close()
	restored.client = &http.Client{Transport: r196RoundTrip(func(*http.Request) (*http.Response, error) {
		t.Error("fresh manifest cache should avoid network")
		return nil, fmt.Errorf("unexpected network")
	})}
	restored.Start()
	select {
	case status := <-readySnapshot:
		waitUpdateCheck(t, restored)
		if status.Latest != manifest.Version || restored.Status().State != "ready" {
			t.Fatalf("ready restore=%+v", status)
		}
	case <-time.After(time.Second):
		t.Fatal("restart did not publish ready")
	}
}

func TestR196CustomSourceRetainsBuiltins(t *testing.T) {
	u := updateTestManager(t, []byte("setup"))
	u.sourceDefaults = nil
	u.mirrors = []string{"https://custom.test/"}
	u.preferredSource = "https://custom.test/"
	sources := u.sourcesLocked()
	if len(sources) != 5 || sources[0] != "https://custom.test/" || sources[1] != "" {
		t.Fatalf("route inventory=%v", sources)
	}
	for _, prefix := range updateMirrors {
		if !updateSourcePresent(sources, prefix) {
			t.Fatalf("missing built-in %q", prefix)
		}
	}
}
