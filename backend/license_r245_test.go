//go:build license

package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestR245SlowNetwork(t *testing.T) {
	for _, kind := range []string{"activate", "renew"} {
		for _, phase := range []string{"headers10s", "body12s"} {
			t.Run(kind+"_"+phase, func(t *testing.T) {
				t.Parallel()
				issuer := newLicenseTestIssuer(t)
				m := issuer.Manager(&licenseTestStore{})
				m.options.Elapsed = time.Now
				if kind == "renew" {
					testActivate(t, m, false)
				}
				store := newStartupTestStore(t)
				a := &app{storage: store}
				m.options.Observe = a.recordDiagnostic
				server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					recorder := httptest.NewRecorder()
					issuer.ServeHTTP(recorder, r)
					delay := 10 * time.Second
					if phase == "body12s" {
						delay = 12 * time.Second
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(recorder.Code)
						w.(http.Flusher).Flush()
					}
					select {
					case <-time.After(delay):
					case <-r.Context().Done():
						return
					}
					if phase == "headers10s" {
						w.WriteHeader(recorder.Code)
					}
					w.Write(recorder.Body.Bytes())
				}))
				defer server.Close()
				m.options.Client = server.Client()
				m.options.Origin = server.URL
				start := time.Now()
				var err error
				if kind == "activate" {
					err = m.Activate(context.Background(), strings.Repeat("0", 20))
				} else {
					err = m.Renew(context.Background())
				}
				if err != nil || !m.Allowed() {
					t.Fatalf("R245 slow %s %s must succeed: %v", kind, phase, err)
				}
				if time.Since(start) > 35*time.Second {
					t.Fatal("R245 activation budget exceeded")
				}
				rows := readDiagnosticEvents(t, store, "license_renew")
				row := rows[len(rows)-1]
				for _, key := range []string{"dns_ms", "connect_ms", "tls_ms", "ttfb_ms", "body_ms"} {
					if _, ok := row[key]; !ok {
						t.Fatalf("missing timing %s: %#v", key, row)
					}
				}
				if row["kind"] != kind || row["result"] != "ok" {
					t.Fatal(row)
				}
				if phase == "headers10s" && row["ttfb_ms"].(float64) < 9900 {
					t.Fatal("slow header timing lost", row)
				}
				if phase == "body12s" && (row["body_ms"].(float64) < 11900 || row["ttfb_ms"].(float64) > 1000) {
					t.Fatal("slow body not distinguished from TTFB", row)
				}
				if row["connect_ms"] == nil || row["tls_ms"] == nil {
					t.Fatal("real TLS trace missing", row)
				}
				rr := httptest.NewRecorder()
				a.handleDiagnosticLog(rr, httptest.NewRequest("GET", "/api/diagnostics/log", nil))
				if strings.Contains(rr.Body.String(), "127.0.0.1") || strings.Contains(rr.Body.String(), strings.Repeat("0", 20)) {
					t.Fatal("trace leaked IP or code")
				}
			})
		}
	}
}

func TestR245StartupPendingAndBackoffRemainFailClosed(t *testing.T) {
	issuer := newLicenseTestIssuer(t)
	cache := &licenseTestStore{}
	testActivate(t, issuer.Manager(cache), false)
	issuer.clock.Advance(licenseLifetime + time.Second)
	m := issuer.Manager(cache)
	if m.Allowed() || m.Snapshot().Message != "正在连接激活服务…" {
		t.Fatal("expired startup must connect without admission", m.Snapshot())
	}
	started, release := make(chan struct{}), make(chan struct{})
	m.options.Client = &http.Client{Transport: updateRoundTrip(func(r *http.Request) (*http.Response, error) {
		close(started)
		<-release
		return nil, context.DeadlineExceeded
	})}
	done := make(chan error, 1)
	go func() { done <- m.Renew(context.Background()) }()
	<-started
	if m.Snapshot().Message != "正在连接激活服务…" || m.Allowed() {
		t.Fatal("pending renew flashed error or admitted")
	}
	close(release)
	if <-done == nil {
		t.Fatal("expected failure")
	}
	if m.Snapshot().Message != "无法连接激活服务，请联网后重试" || m.Allowed() {
		t.Fatal("first failure did not show error")
	}
	if got := m.nextRenew.Sub(issuer.clock.Elapsed()); got != 3*time.Second {
		t.Fatal(got)
	}
	m.options.Client = &http.Client{Transport: updateRoundTrip(func(*http.Request) (*http.Response, error) { return nil, context.DeadlineExceeded })}
	for _, delay := range []time.Duration{6 * time.Second, 12 * time.Second, 15 * time.Second, 30 * time.Second, 60 * time.Second, 60 * time.Second} {
		m.Renew(context.Background())
		if got := m.nextRenew.Sub(issuer.clock.Elapsed()); got != delay {
			t.Fatalf("startup retry %s != %s", got, delay)
		}
		if m.Allowed() {
			t.Fatal("failed renew admitted expired lease")
		}
	}
	m.options.Client = issuer.client
	if m.Renew(context.Background()) != nil || !m.Allowed() {
		t.Fatal("successful signed renew did not restore")
	}
	issuer.offline.Store(true)
	m.Renew(context.Background())
	if got := m.nextRenew.Sub(issuer.clock.Elapsed()); got != 15*time.Second {
		t.Fatal("normal backoff not restored", got)
	}
}

func TestR245TracePartialFailuresAndReusedConnection(t *testing.T) {
	issuer := newLicenseTestIssuer(t)
	m := issuer.Manager(&licenseTestStore{})
	store := newStartupTestStore(t)
	a := &app{storage: store}
	m.options.Observe = a.recordDiagnostic
	m.options.Client = &http.Client{Transport: updateRoundTrip(func(r *http.Request) (*http.Response, error) {
		trace := httptrace.ContextClientTrace(r.Context())
		if trace == nil {
			t.Fatal("httptrace not attached")
		}
		trace.DNSStart(httptrace.DNSStartInfo{Host: "private-host"})
		time.Sleep(2 * time.Millisecond)
		trace.DNSDone(httptrace.DNSDoneInfo{})
		trace.ConnectStart("tcp", "private-ip")
		time.Sleep(2 * time.Millisecond)
		trace.ConnectDone("tcp", "private-ip", nil)
		trace.TLSHandshakeStart()
		time.Sleep(2 * time.Millisecond)
		trace.TLSHandshakeDone(tls.ConnectionState{}, nil)
		return nil, context.DeadlineExceeded
	})}
	m.Activate(context.Background(), strings.Repeat("0", 20))
	rows := readDiagnosticEvents(t, store, "license_renew")
	for _, row := range rows {
		for _, key := range []string{"dns_ms", "connect_ms", "tls_ms"} {
			if row[key].(float64) < 1 {
				t.Fatal(row)
			}
		}
		if row["ttfb_ms"] != nil || row["body_ms"] != nil {
			t.Fatal("unreached phase invented", row)
		}
	}
	b, _ := json.Marshal(rows)
	if strings.Contains(string(b), "private-") {
		t.Fatal("trace leaked metadata")
	}
	v := newLicenseNetworkTimings()
	row := map[string]any{}
	v.add(row)
	for _, key := range []string{"dns_ms", "connect_ms", "tls_ms", "ttfb_ms", "body_ms"} {
		if row[key] != nil {
			t.Fatal("unused phase invented")
		}
	}
	v.begin("connect_ms")
	v.begin("connect_ms")
	v.end("connect_ms")
	time.Sleep(3 * time.Millisecond)
	partial := map[string]any{}
	v.add(partial)
	if partial["connect_ms"].(int64) < 2 {
		t.Fatal("parallel unfinished dial was truncated", partial)
	}
	v.end("connect_ms")
	// Trace callbacks may race the diagnostic snapshot on multi-address dials.
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); v.begin("connect_ms"); v.end("connect_ms"); v.add(map[string]any{}) }()
	}
	wg.Wait()
}
func TestR245ActivationBudget(t *testing.T) {
	if licenseActivationTimeout != 35*time.Second {
		t.Fatal("activation total budget must be 35s")
	}
	issuer := newLicenseTestIssuer(t)
	m := issuer.Manager(&licenseTestStore{})
	m.options.Client = &http.Client{Transport: updateRoundTrip(func(r *http.Request) (*http.Response, error) {
		deadline, _ := r.Context().Deadline()
		if time.Until(deadline) > 15*time.Second {
			t.Fatal("attempt exceeds 15s")
		}
		return nil, context.Canceled
	})}
	m.Activate(context.Background(), strings.Repeat("0", 20))
}

func TestR245WindowInvalidValuesReasonsAndNativeExport(t *testing.T) {
	store := newStartupTestStore(t)
	a := &app{storage: store}
	base := `{"fromState":"REVOKED","toState":"ACTIVE","requestedContent":{"x":0,"y":0,"width":1200,"height":780},"actualContent":{"x":0,"y":0,"width":1200,"height":780},"displayScale":1.5,"native_error":null}`
	for _, entry := range []struct{ replacement, reason string }{{`"width":1200.5`, "non_integer"}, {`"width":0`, "non_positive"}, {`"width":1000001`, "out_of_range"}, {`"gone":null`, "missing"}, {`"width":"private-secret"`, "missing"}} {
		raw := strings.Replace(base, `"width":1200`, entry.replacement, 1)
		a.recordLicenseWindowDiagnostic(json.RawMessage(raw))
		rows := readDiagnosticEvents(t, store, "license_window_state_invalid")
		fields := rows[len(rows)-1]["invalid_fields"].([]any)
		found := false
		for _, item := range fields {
			row := item.(map[string]any)
			if row["field"] == "requestedContent.width" {
				found = true
				if row["reason"] != entry.reason {
					t.Fatal(row)
				}
				if row["value_type"] == "number" {
					if _, ok := row["value"]; !ok {
						t.Fatal("raw numeric value missing")
					}
				} else if _, ok := row["value"]; ok {
					t.Fatal("non-numeric content echoed")
				}
			}
		}
		if !found {
			t.Fatal(fields)
		}
	}
	raw := strings.Replace(base, `"native_error":null`, `"native_error":"TypeError"`, 1)
	if !a.recordLicenseWindowDiagnostic(json.RawMessage(raw)) {
		t.Fatal("valid native diagnostic rejected")
	}
	rows := readDiagnosticEvents(t, store, "license_window_state")
	if rows[0]["scale_factor"] != 1.5 || rows[0]["native_error"] != "TypeError" {
		t.Fatal(rows)
	}
	if path := os.Getenv("R245_NATIVE_REPORT"); path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var runs []struct {
			Diagnostics []struct {
				Value struct {
					LicenseWindow json.RawMessage `json:"licenseWindow"`
				} `json:"value"`
			} `json:"diagnostics"`
		}
		if json.Unmarshal(data, &runs) != nil {
			t.Fatal("invalid native report")
		}
		for _, run := range runs {
			for _, row := range run.Diagnostics {
				if len(row.Value.LicenseWindow) > 0 && !a.recordLicenseWindowDiagnostic(row.Value.LicenseWindow) {
					t.Fatalf("real Electron diagnostic rejected: %s", row.Value.LicenseWindow)
				}
			}
		}
	}
	rr := httptest.NewRecorder()
	a.handleDiagnosticLog(rr, httptest.NewRequest("GET", "/api/diagnostics/log", nil))
	if strings.Contains(rr.Body.String(), "private-secret") {
		t.Fatal("non-numeric secret leaked")
	}
}
