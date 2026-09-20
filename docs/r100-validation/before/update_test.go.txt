package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type updateRoundTrip func(*http.Request) (*http.Response, error)

func (f updateRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func updateTestManifest(data []byte) updateManifest {
	digest := sha256.Sum256(data)
	name := "Deep-Legends-Setup-0.12.0-a1b2c3d4e5f6.exe"
	return updateManifest{Schema: 1, Version: "0.12.0", Fingerprint: "a1b2c3d4e5f6", PublishedAt: time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC), MinSupported: "0.9.0", Notes: "### 新增\n- 更新", Asset: updateAsset{Name: name, Size: int64(len(data)), SHA256: hex.EncodeToString(digest[:]), URL: "https://github.com/" + updateRepo + "/releases/download/v0.12.0/" + name}}
}
func updateTestManager(t *testing.T, data []byte) *updateManager {
	t.Helper()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "updates"), 0700); err != nil {
		t.Fatal(err)
	}
	u := newUpdateManager("0.11.2", trackTestStore(t, &localStore{root: root}), nil)
	t.Cleanup(u.Close)
	u.status.Portable = false
	u.installDir = filepath.Join(root, "installed")
	u.freeBytes = func(string) (int64, error) { return 1 << 40, nil }
	manifest := updateTestManifest(data)
	u.manifest = &manifest
	u.status.State = "available"
	u.status.Latest = manifest.Version
	u.status.SizeBytes = manifest.Asset.Size
	return u
}
func updateResponse(code int, data []byte) *http.Response {
	return &http.Response{StatusCode: code, Body: io.NopCloser(bytes.NewReader(data)), Header: make(http.Header)}
}
func waitUpdateCheck(t *testing.T, u *updateManager) {
	t.Helper()
	u.mu.Lock()
	done := u.checkDone
	u.mu.Unlock()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("check stuck")
	}
}
func waitUpdateDownload(t *testing.T, u *updateManager) {
	t.Helper()
	u.mu.Lock()
	done := u.downloadDone
	u.mu.Unlock()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("download stuck")
	}
}
func TestUpdateVersions(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want int
	}{
		{"0.12.0", "0.12.0", 0}, {"v0.12.0", "0.12.0", 0}, {"v0.12.0", "0.11.9", 1}, {"0.12.0-beta.1", "0.12.0", -1}, {"0.12.0", "0.12.0-beta.1", 1}, {"0.12.0-beta.10", "0.12.0-beta.2", 1}, {"0.12.0-1", "0.12.0-beta", -1}, {"0.11.9", "0.12.0", -1}, {"1.0.0", "0.99.99", 1}, {"0.12.0-beta", "0.12.0-beta.1", -1},
	} {
		if got := compareVersions(tc.a, tc.b); got != tc.want {
			t.Errorf("%s %s = %d want %d", tc.a, tc.b, got, tc.want)
		}
	}
	for _, bad := range []string{"dev", "1.2", "01.2.3", "1.2.3/evil"} {
		if _, ok := parseUpdateVersion(bad); ok {
			t.Errorf("accepted %s", bad)
		}
	}
}
func TestUpdateDevDisabled(t *testing.T) {
	u := newUpdateManager("dev", trackTestStore(t, &localStore{root: t.TempDir()}), nil)
	defer u.Close()
	var calls atomic.Int32
	u.client = &http.Client{Transport: updateRoundTrip(func(*http.Request) (*http.Response, error) { calls.Add(1); return nil, errors.New("unexpected") })}
	u.Start()
	if u.Status().Supported || u.Check(true) || calls.Load() != 0 {
		t.Fatal("dev updater enabled")
	}
}
func TestUpdateMirrorFallbackSingleFlightAndLastError(t *testing.T) {
	u := updateTestManager(t, []byte("setup"))
	var calls []string
	var mu sync.Mutex
	gate := make(chan struct{})
	u.mirrors = []string{"", "https://mirror.test/"}
	u.client = &http.Client{Transport: updateRoundTrip(func(r *http.Request) (*http.Response, error) {
		mu.Lock()
		calls = append(calls, r.URL.String())
		mu.Unlock()
		<-gate
		if r.URL.Host == "github.com" {
			return nil, errors.New("first failed")
		}
		data, _ := json.Marshal(u.manifest)
		return updateResponse(200, data), nil
	})}
	if !u.Check(true) {
		t.Fatal("not started")
	}
	for i := 0; i < 9; i++ {
		if u.Check(true) {
			t.Fatal("duplicate check")
		}
	}
	close(gate)
	waitUpdateCheck(t, u)
	if len(calls) != 2 || calls[1] != "https://mirror.test/"+updateManifestPath || u.Status().State != "available" {
		t.Fatalf("fallback: %v %#v", calls, u.Status())
	}
	if u.Check(true) {
		t.Fatal("burst was not coalesced")
	}
	u.now = func() time.Time { return time.Now().Add(10 * time.Second) }
	u.client = &http.Client{Transport: updateRoundTrip(func(r *http.Request) (*http.Response, error) { return updateResponse(503, nil), nil })}
	u.manifest = nil
	u.Check(true)
	waitUpdateCheck(t, u)
	if status := u.Status(); status.State != "idle" || !strings.Contains(status.Error, "https://mirror.test/"+updateManifestPath+"：HTTP 503") {
		t.Fatalf("last error lost: %#v", status)
	}
}
func TestUpdateCacheTTLAndNoDowngrade(t *testing.T) {
	for _, latest := range []string{"0.12.0", "0.11.2", "0.10.0"} {
		t.Run(latest, func(t *testing.T) {
			u := updateTestManager(t, []byte("setup"))
			now := time.Now()
			u.now = func() time.Time { return now }
			m := *u.manifest
			m.Version = latest
			m.Asset.Name = "Deep-Legends-Setup-" + latest + "-" + m.Fingerprint + ".exe"
			m.Asset.URL = "https://github.com/" + updateRepo + "/releases/download/v" + latest + "/" + m.Asset.Name
			u.cache = updateCache{CheckedAt: now.Add(-time.Hour), Current: "0.11.2", Manifest: m}
			var calls atomic.Int32
			u.client = &http.Client{Transport: updateRoundTrip(func(*http.Request) (*http.Response, error) {
				calls.Add(1)
				data, _ := json.Marshal(m)
				return updateResponse(200, data), nil
			})}
			u.Check(false)
			waitUpdateCheck(t, u)
			want := "idle"
			if latest == "0.12.0" {
				want = "available"
			}
			if calls.Load() != 0 || u.Status().State != want {
				t.Fatalf("fresh cache/state %d %#v", calls.Load(), u.Status())
			}
			now = now.Add(6 * time.Hour)
			u.Check(false)
			waitUpdateCheck(t, u)
			if calls.Load() != 1 {
				t.Fatal("stale cache was used")
			}
			u.Check(true)
			waitUpdateCheck(t, u)
			if calls.Load() != 2 {
				t.Fatal("manual check did not bypass cache")
			}
			restored := newUpdateManager("0.11.2", u.store, nil)
			defer restored.Close()
			if restored.cache.Manifest.Version != latest {
				t.Fatal("cache not persisted")
			}
		})
	}
}

// Exercise real in-flight work, with no manual-check debounce or fresh cache.
// A background timer must not reset state or clean files under that work.
func TestUpdateBusyOperationsRejectOverlappingWork(t *testing.T) {
	for _, state := range []string{"checking", "downloading", "verifying", "applying"} {
		t.Run(state, func(t *testing.T) {
			data := []byte("complete installer")
			u := updateTestManager(t, data)
			u.mirrors = []string{""}
			manifestJSON, _ := json.Marshal(u.manifest)
			entered, release := make(chan struct{}, 1), make(chan struct{})
			var releaseOnce sync.Once
			unblock := func() { releaseOnce.Do(func() { close(release) }) }
			defer unblock()
			block := func() {
				select {
				case entered <- struct{}{}:
				default:
				}
				select {
				case <-release:
				case <-u.ctx.Done():
				}
			}
			var requests atomic.Int32
			u.client = &http.Client{Transport: updateRoundTrip(func(r *http.Request) (*http.Response, error) {
				requests.Add(1)
				if state == "checking" || state == "downloading" {
					block()
				}
				if r.URL.String() == updateManifestPath {
					return updateResponse(200, manifestJSON), nil
				}
				return updateResponse(200, data), nil
			})}
			u.notify = func(kind string, value any) {
				if status, ok := value.(updateStatus); state == "verifying" && ok && status.State == state {
					block()
				}
			}
			var applied chan error
			switch state {
			case "checking":
				if !u.Check(false) {
					t.Fatal("background check did not start")
				}
			default:
				if err := u.Download(); err != nil {
					t.Fatal(err)
				}
				if state == "applying" {
					waitUpdateDownload(t, u)
					u.launch = func(string, string) error { block(); return nil }
					applied = make(chan error, 1)
					go func() { applied <- u.Apply() }()
				}
			}
			select {
			case <-entered:
			case <-time.After(3 * time.Second):
				t.Fatal("operation did not reach barrier")
			}
			before := u.Status()
			if before.State != state {
				t.Fatalf("expected active %s, got %s", state, before.State)
			}
			for i := 0; i < 10; i++ {
				if u.Check(false) {
					t.Fatal("background check overlapped active " + state)
				}
			}
			if u.Check(true) {
				t.Fatal("manual check overlapped active " + state)
			}
			if err := u.Download(); err == nil {
				t.Fatal("download overlapped active " + state)
			}
			if err := u.SetSettings(updateSettings{Mirrors: []string{"https://changed.test/"}}); err == nil {
				t.Fatal("mirrors changed during " + state)
			}
			if u.Status() != before || len(u.Settings().Mirrors) != 1 || u.Settings().Mirrors[0] != "" {
				t.Fatal("rejected work changed active state or mirrors")
			}
			unblock()
			if state == "checking" {
				waitUpdateCheck(t, u)
			} else if applied != nil {
				select {
				case err := <-applied:
					if err != nil {
						t.Fatal(err)
					}
				case <-time.After(3 * time.Second):
					t.Fatal("apply stuck")
				}
			} else {
				waitUpdateDownload(t, u)
			}
			if requests.Load() != 1 {
				t.Fatalf("overlap issued extra requests: %d", requests.Load())
			}
			want := "ready"
			if state == "checking" {
				want = "available"
			}
			if state == "applying" {
				want = "applying"
			}
			if u.Status().State != want {
				t.Fatalf("original operation did not finish: %#v", u.Status())
			}
		})
	}
}
func TestUpdateManifestAndSettingsValidation(t *testing.T) {
	m := updateTestManifest([]byte("setup"))
	if err := validateUpdateManifest(m); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*updateManifest){func(m *updateManifest) { m.Schema = 2 }, func(m *updateManifest) { m.Asset.Name = "../evil.exe" }, func(m *updateManifest) { m.Asset.Size = -1 }, func(m *updateManifest) { m.Asset.SHA256 = "bad" }, func(m *updateManifest) { m.Asset.URL = "http://evil.test/a" }, func(m *updateManifest) { m.MinSupported = "wrong" }} {
		bad := m
		mutate(&bad)
		if validateUpdateManifest(bad) == nil {
			t.Fatalf("bad manifest accepted: %#v", bad)
		}
	}
	u := updateTestManager(t, []byte("setup"))
	if err := u.SetSettings(updateSettings{Mirrors: []string{"https://mirror.example/", ""}}); err != nil {
		t.Fatal(err)
	}
	restored := newUpdateManager("0.11.2", u.store, nil)
	defer restored.Close()
	if restored.Settings().Mirrors[0] != "https://mirror.example/" {
		t.Fatal("settings not restored")
	}
	for _, prefix := range []string{"http://bad/", "https://bad", "https://user:pw@bad/", "https://bad/?query"} {
		if validUpdateMirrors([]string{prefix}) == nil {
			t.Fatal("invalid prefix accepted")
		}
	}
}

func TestUpdateVersionNamedAssetAndLegacyCompatibility(t *testing.T) {
	for _, name := range []string{"Deep-Legends-Setup-0.12.0.exe", "Deep-Legends-Setup-0.12.0-a1b2c3d4e5f6.exe"} {
		m := updateTestManifest([]byte("setup"))
		m.Asset.Name = name
		m.Asset.URL = "https://github.com/" + updateRepo + "/releases/download/v0.12.0/" + name
		if err := validateUpdateManifest(m); err != nil {
			t.Fatalf("valid release rejected: %s: %v", name, err)
		}
		m.Asset.URL += "-unexpected"
		if validateUpdateManifest(m) == nil {
			t.Fatal("URL/name mismatch accepted")
		}
	}
}
func TestUpdateDownloadHashRetryAndCleanup(t *testing.T) {
	data := []byte("real installer content")
	for _, corrupt := range []bool{false, true} {
		t.Run(fmt.Sprint(corrupt), func(t *testing.T) {
			u := updateTestManager(t, data)
			u.mirrors = []string{""}
			var requests int
			u.client = &http.Client{Transport: updateRoundTrip(func(r *http.Request) (*http.Response, error) {
				requests++
				body := data
				if corrupt {
					body = bytes.Repeat([]byte("x"), len(data))
				}
				return updateResponse(200, body), nil
			})}
			if err := u.Download(); err != nil {
				t.Fatal(err)
			}
			waitUpdateDownload(t, u)
			final := filepath.Join(u.directory, u.manifest.Asset.Name)
			if _, err := os.Stat(final + ".part"); !os.IsNotExist(err) {
				t.Fatal("part retained")
			}
			if corrupt {
				if requests != 2 || u.Status().State != "failed" || u.Status().Error != errUpdateChecksum.Error() {
					t.Fatalf("bad retry %#v count=%d", u.Status(), requests)
				}
				if _, err := os.Stat(final); !os.IsNotExist(err) {
					t.Fatal("corrupt final retained")
				}
			} else {
				if requests != 1 || u.Status().State != "ready" {
					t.Fatalf("download %#v", u.Status())
				}
				if err := verifyUpdateFile(context.Background(), final, u.manifest.Asset); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
	if verifyUpdateDigest("a", "b") == nil || verifyUpdateDigest("abc", "ABC") != nil {
		t.Fatal("checksum comparison broken")
	}
}
func TestUpdateResumeReprobesEachMirrorAndHashesPrefix(t *testing.T) {
	data := []byte("0123456789installer")
	u := updateTestManager(t, data)
	part := filepath.Join(u.directory, u.manifest.Asset.Name+".part")
	os.WriteFile(part, data[:5], 0600)
	var requests []string
	u.mirrors = []string{"", "https://second.test/"}
	u.client = &http.Client{Transport: updateRoundTrip(func(r *http.Request) (*http.Response, error) {
		requests = append(requests, r.Method+" "+r.URL.Host+" "+r.Header.Get("Range"))
		if r.Method == "HEAD" {
			resp := updateResponse(200, nil)
			if r.URL.Host == "github.com" {
				resp.Header.Set("Accept-Ranges", "bytes")
			}
			return resp, nil
		}
		if r.URL.Host == "github.com" {
			return updateResponse(503, nil), nil
		}
		return updateResponse(200, data), nil
	})}
	if err := u.Download(); err != nil {
		t.Fatal(err)
	}
	waitUpdateDownload(t, u)
	want := "HEAD github.com |GET github.com bytes=5-|HEAD second.test |GET second.test "
	if strings.Join(requests, "|") != want || u.Status().State != "ready" {
		t.Fatalf("mirror resume: %v %#v", requests, u.Status())
	}
	// A server advertising ranges must actually return the correct 206 range.
	os.Remove(strings.TrimSuffix(part, ".part"))
	os.WriteFile(part, data[:5], 0600)
	u.status.State = "available"
	u.mirrors = []string{""}
	u.client = &http.Client{Transport: updateRoundTrip(func(r *http.Request) (*http.Response, error) {
		if r.Method == "HEAD" {
			resp := updateResponse(200, nil)
			resp.Header.Set("Accept-Ranges", "bytes")
			return resp, nil
		}
		if r.Header.Get("Range") != "bytes=5-" {
			t.Error("missing Range")
		}
		resp := updateResponse(206, data[5:])
		resp.Header.Set("Content-Range", fmt.Sprintf("bytes 5-%d/%d", len(data)-1, len(data)))
		return resp, nil
	})}
	u.Download()
	waitUpdateDownload(t, u)
	if u.Status().State != "ready" {
		t.Fatalf("prefix not hashed: %#v", u.Status())
	}
}
func TestUpdateCancelAndSpaceMargin(t *testing.T) {
	data := bytes.Repeat([]byte("x"), 100)
	u := updateTestManager(t, data)
	var calls atomic.Int32
	u.freeBytes = func(string) (int64, error) { return 110, nil }
	u.client = &http.Client{Transport: updateRoundTrip(func(*http.Request) (*http.Response, error) { calls.Add(1); return updateResponse(200, data), nil })}
	if u.Download() == nil || calls.Load() != 0 {
		t.Fatal("space margin ignored")
	}
	u.freeBytes = func(string) (int64, error) { return 120, nil }
	started := make(chan struct{})
	u.mirrors = []string{""}
	u.client = &http.Client{Transport: updateRoundTrip(func(r *http.Request) (*http.Response, error) {
		close(started)
		<-r.Context().Done()
		return nil, r.Context().Err()
	})}
	part := filepath.Join(u.directory, u.manifest.Asset.Name+".part")
	if err := u.Download(); err != nil {
		t.Fatal(err)
	}
	<-started
	if err := u.Cancel(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(part); !os.IsNotExist(err) {
		t.Fatal("cancel did not delete part")
	}
	if s := u.Status(); s.State != "available" || s.Progress.ReceivedBytes != 0 {
		t.Fatalf("cancel state %#v", s)
	}
}
func TestUpdateFiveSecondSpeedWindow(t *testing.T) {
	start := time.Unix(0, 0)
	w := updateSpeedWindow{}
	w.speed(start, 0)
	for second := 1; second <= 10; second++ {
		w.speed(start.Add(time.Duration(second)*time.Second), int64(second*100))
	}
	w.speed(start.Add(11*time.Second), 11000)
	got := w.speed(start.Add(12*time.Second), 21000)
	if got != 4060 {
		t.Fatalf("five second speed=%v want4060; lifetime average=1750", got)
	}
}
func TestUpdateApplyRehashPortableMinimumAndCommand(t *testing.T) {
	data := []byte("setup")
	u := updateTestManager(t, data)
	file := filepath.Join(u.directory, u.manifest.Asset.Name)
	os.WriteFile(file, data, 0600)
	u.status.State = "ready"
	var launched int
	u.launch = func(setup, dest string) error {
		launched++
		if setup != file || dest != u.installDir {
			t.Error("wrong handoff")
		}
		return nil
	}
	u.status.Portable = true
	if u.Apply() == nil || launched != 0 {
		t.Fatal("portable applied")
	}
	a := &app{updates: u}
	w := httptest.NewRecorder()
	a.handleUpdateAction(w, httptest.NewRequest("POST", "/api/update/apply", nil))
	if w.Code != 400 {
		t.Fatal("portable HTTP must be400")
	}
	u.status.Portable = false
	u.status.ManualOnly = true
	if u.Apply() == nil {
		t.Fatal("minimum bypass")
	}
	u.status.ManualOnly = false
	os.WriteFile(file, []byte("wrong"), 0600)
	if u.Apply() == nil || launched != 0 {
		t.Fatal("tamper not detected")
	}
	os.WriteFile(file, data, 0600)
	u.status.State = "ready"
	if err := u.Apply(); err != nil || launched != 1 || u.Status().State != "applying" {
		t.Fatalf("apply %v count%d", err, launched)
	}
	command := updateCommandLine(`C:\更新 包\setup.exe`, `C:\Program Files\Deep Legends`)
	if command != `"C:\更新 包\setup.exe" --update --dest "C:\Program Files\Deep Legends"` {
		t.Fatal(command)
	}
	root := filepath.Join("/", "install", "Deep Legends")
	exe := filepath.Join(root, "resources", "app.asar.unpacked", "backend", "loot-service.exe")
	if updateRootForExecutable(exe) != root {
		t.Fatal("backend directory mistaken for install directory")
	}
	for _, tc := range []struct {
		name, location  string
		uninstall, want bool
	}{{"Deep Legends", root, true, true}, {"Deep Legends", root, false, false}, {"Other", root, true, false}, {"Deep Legends", root + "-portable", true, false}} {
		if updateInstallationMatches(root, tc.name, tc.location, tc.uninstall) != tc.want {
			t.Fatal(tc)
		}
	}
}
func TestUpdateProgressSSEAndCleanup(t *testing.T) {
	var out bytes.Buffer
	if err := writeLiveEvent(&out, "update:progress\n{\"receivedBytes\":123}"); err != nil {
		t.Fatal(err)
	}
	if out.String() != "event: update:progress\ndata: {\"receivedBytes\":123}\n\n" {
		t.Fatal(out.String())
	}
	out.Reset()
	writeLiveEvent(&out, "gameflow:InProgress")
	if out.String() != "data: gameflow:InProgress\n\n" {
		t.Fatal("existing SSE changed")
	}
	u := updateTestManager(t, []byte("setup"))
	for _, name := range []string{u.manifest.Asset.Name + ".part", "old.exe", "old.part"} {
		os.WriteFile(filepath.Join(u.directory, name), []byte("old"), 0600)
	}
	if err := u.cleanDownloads(u.manifest.Asset.Name); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(u.directory)
	if len(entries) != 1 {
		t.Fatal("old versions remain")
	}
	if err := u.cleanDownloads(""); err != nil {
		t.Fatal(err)
	}
	entries, _ = os.ReadDir(u.directory)
	if len(entries) != 0 {
		t.Fatal("upgrade did not clean all files")
	}
}

func TestUpdateAfterUpgradeCleanupAndMinimum(t *testing.T) {
	u := updateTestManager(t, []byte("setup"))
	m := *u.manifest
	m.MinSupported = "0.12.0"
	u.cache = updateCache{CheckedAt: time.Now(), Current: "0.10.0", Manifest: m}
	os.WriteFile(filepath.Join(u.directory, "old.exe"), []byte("old"), 0600)
	os.WriteFile(filepath.Join(u.directory, m.Asset.Name+".part"), []byte("old"), 0600)
	u.Check(false)
	waitUpdateCheck(t, u)
	entries, _ := os.ReadDir(u.directory)
	if len(entries) != 0 || !u.Status().ManualOnly || u.cache.Current != "0.11.2" {
		t.Fatalf("cleanup/minimum: %d %#v", len(entries), u.Status())
	}
	if u.Download() == nil {
		t.Fatal("minimum allowed download")
	}
}

type updateSlowBody struct {
	ctx        context.Context
	left       int
	delay      time.Duration
	beforeRead func()
}

func (b *updateSlowBody) Read(p []byte) (int, error) {
	if b.left == 0 {
		return 0, io.EOF
	}
	if b.beforeRead != nil {
		b.beforeRead()
	}
	select {
	case <-time.After(b.delay):
	case <-b.ctx.Done():
		return 0, b.ctx.Err()
	}
	n := min(len(p), b.left)
	for i := 0; i < n; i++ {
		p[i] = 'x'
	}
	b.left -= n
	return n, nil
}
func (b *updateSlowBody) Close() error { return nil }
func TestUpdateProgressAndStalledSourceFallback(t *testing.T) {
	data := bytes.Repeat([]byte("x"), 64*1024)
	u := updateTestManager(t, data)
	u.mirrors = []string{"", "https://fallback.test/"}
	var progress atomic.Int32
	ticks := make(chan time.Time, 1)
	progressSeen := make(chan struct{}, 1)
	u.progressTicks = func() (<-chan time.Time, func()) { return ticks, func() {} }
	var expireHeader func()
	u.startSourceTimer = func(d time.Duration, expire func()) func() {
		if d != 8*time.Second {
			t.Errorf("source timeout=%v", d)
		}
		expireHeader = expire
		return func() {}
	}
	u.notify = func(kind string, value any) {
		if kind == "update:progress" {
			progress.Add(1)
			select {
			case progressSeen <- struct{}{}:
			default:
			}
		}
	}
	var requests atomic.Int32
	u.client = &http.Client{Transport: updateRoundTrip(func(r *http.Request) (*http.Response, error) {
		requests.Add(1)
		if r.URL.Host == "github.com" {
			expireHeader() // Manually fire the unchanged eight-second header deadline.
			<-r.Context().Done()
			return nil, r.Context().Err()
		}
		response := updateResponse(200, nil)
		response.Body = &updateSlowBody{ctx: r.Context(), left: len(data), beforeRead: func() {
			ticks <- time.Now()
			select {
			case <-progressSeen:
			case <-time.After(time.Second):
				t.Error("progress timer not observed")
			}
		}}
		return response, nil
	})}
	if err := u.Download(); err != nil {
		t.Fatal(err)
	}
	u.mu.Lock()
	done := u.downloadDone
	u.mu.Unlock()
	select {
	case <-done:
	case <-time.After(12 * time.Second):
		t.Fatal("stalled source never fell back")
	}
	if requests.Load() != 2 || progress.Load() < 2 || u.Status().State != "ready" {
		t.Fatalf("requests=%d progress=%d status=%#v", requests.Load(), progress.Load(), u.Status())
	}
}

type updateSnapshotRecorder struct {
	*httptest.ResponseRecorder
	cancel context.CancelFunc
}

func (w updateSnapshotRecorder) Flush() { w.ResponseRecorder.Flush(); w.cancel() }
func TestUpdateHTTPStatusAuthAndInitialEventSnapshot(t *testing.T) {
	u := updateTestManager(t, []byte("setup"))
	a := &app{updates: u, token: "local-session", eventSubscribers: make(map[chan string]struct{})}
	mux := http.NewServeMux()
	a.registerUpdateRoutes(mux)
	for _, route := range []struct{ method, path string }{{"GET", "status"}, {"POST", "check"}, {"POST", "download"}, {"POST", "cancel"}, {"POST", "apply"}, {"POST", "settings"}} {
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, httptest.NewRequest(route.method, "/api/update/"+route.path, nil))
		if recorder.Code != 401 {
			t.Fatalf("unprotected %s", route.path)
		}
	}
	request := httptest.NewRequest("GET", "/api/update/status", nil)
	request.Header.Set("X-Local-Token", "local-session")
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, request)
	if recorder.Code != 200 || !strings.Contains(recorder.Body.String(), `"latest":"0.12.0"`) {
		t.Fatal(recorder.Body.String())
	}
	status := httptest.NewRecorder()
	a.handleStatus(status, httptest.NewRequest("GET", "/api/status", nil))
	var body statusResponse
	if json.Unmarshal(status.Body.Bytes(), &body) != nil || body.Update.Latest != "0.12.0" {
		t.Fatal("main status omitted updater")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stream := updateSnapshotRecorder{httptest.NewRecorder(), cancel}
	a.handleEvents(stream, httptest.NewRequest("GET", "/api/events", nil).WithContext(ctx))
	if !strings.Contains(stream.Body.String(), "event: update:status\n") || !strings.Contains(stream.Body.String(), `"state":"available"`) {
		t.Fatal("initial subscribe lost snapshot: " + stream.Body.String())
	}
	if len(a.eventSubscribers) != 0 {
		t.Fatal("closed SSE subscriber leaked")
	}
}
