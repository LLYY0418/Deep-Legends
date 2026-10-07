//go:build license

package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestR243ActivationRetriesFreshSignedRequest(t *testing.T) {
	for _, mode := range []string{"timeout", "connect", "dns", "5xx", "unsigned", "service"} {
		t.Run(mode, func(t *testing.T) {
			issuer := newLicenseTestIssuer(t)
			m := issuer.Manager(&licenseTestStore{})
			original := m.options.Client.Transport
			var requests []licenseRequest
			var events []map[string]any
			m.options.Observe = func(row map[string]any) { events = append(events, row) }
			m.options.Client = &http.Client{Transport: updateRoundTrip(func(r *http.Request) (*http.Response, error) {
				body, _ := io.ReadAll(r.Body)
				r.Body = io.NopCloser(bytes.NewReader(body))
				var request licenseRequest
				json.Unmarshal(body, &request)
				requests = append(requests, request)
				deadline, ok := r.Context().Deadline()
				if !ok || time.Until(deadline) > licenseTimeout {
					t.Fatal("attempt timeout not bounded")
				}
				if len(requests) == 1 {
					switch mode {
					case "timeout":
						<-r.Context().Done()
						return nil, r.Context().Err()
					case "connect":
						return nil, &net.OpError{Op: "dial", Err: errors.New("test secret must not be logged")}
					case "dns":
						return nil, &net.DNSError{Err: "test secret", Name: "private.test", IsNotFound: true}
					case "5xx", "unsigned":
						w := httptest.NewRecorder()
						w.Header().Set("CF-Ray", "abc123-SJC")
						w.WriteHeader(503)
						if mode == "5xx" {
							w.WriteString("upstream failed")
						} else {
							w.WriteString(`{"error":"SERVICE_UNAVAILABLE","secret":"must-not-be-exported"}`)
						}
						return w.Result(), nil
					case "service":
						pub, _ := rawURLDecode(request.DevicePub, 32)
						w := httptest.NewRecorder()
						w.WriteHeader(503)
						json.NewEncoder(w).Encode(testSignedEnvelope(issuer.key, "test-issuer", "DL-LICENSE-RESPONSE-V1", licensePayload{Version: 1, RequestID: request.RequestID, DeviceHash: licenseDeviceHash(pub), ServerTime: issuer.clock.Now().Unix(), Status: "ERROR", Error: "SERVICE_UNAVAILABLE"}))
						return w.Result(), nil
					}
				}
				return original.RoundTrip(r)
			})}
			start := time.Now()
			if err := m.Activate(context.Background(), strings.Repeat("0", 20)); err != nil {
				t.Fatal(err)
			}
			if time.Since(start) > licenseActivationTimeout || len(requests) != 2 || !m.Allowed() {
				t.Fatal("activation retry missing or over budget")
			}
			if requests[0].RequestID == requests[1].RequestID || requests[0].Counter == requests[1].Counter || requests[0].Signature == requests[1].Signature {
				t.Fatal("retry reused signed request")
			}
			var failed map[string]any
			for _, row := range events {
				if row["result"] == "failed" {
					failed = row
				}
			}
			want := mode
			if mode == "5xx" || mode == "service" {
				want = "http_status"
			}
			if mode == "unsigned" {
				want = "unsigned_error"
			}
			if failed["kind"] != "activate" || failed["failure"] != want {
				t.Fatalf("failure classification: %#v", failed)
			}
			encoded, _ := json.Marshal(events)
			if strings.Contains(string(encoded), "secret") || strings.Contains(string(encoded), strings.Repeat("0", 20)) {
				t.Fatal("sensitive diagnostic content")
			}
		})
	}
}
func TestR243BusinessErrorsAndInvalidProofDoNotRetry(t *testing.T) {
	for _, code := range []string{"INVALID_CODE", "REVOKED", "EXPIRED", "REPLACED", "RATE_LIMITED", "REPLAY", "UNSUPPORTED_VERSION", "bad_signature", "parse", "tls"} {
		t.Run(code, func(t *testing.T) {
			issuer := newLicenseTestIssuer(t)
			m := issuer.Manager(&licenseTestStore{})
			calls := 0
			m.options.Client = &http.Client{Transport: updateRoundTrip(func(r *http.Request) (*http.Response, error) {
				calls++
				if code == "tls" {
					return nil, tls.RecordHeaderError{Msg: "secret"}
				}
				w := httptest.NewRecorder()
				if code == "parse" {
					w.WriteString("{")
					return w.Result(), nil
				}
				var req licenseRequest
				json.NewDecoder(r.Body).Decode(&req)
				pub, _ := rawURLDecode(req.DevicePub, 32)
				env := testSignedEnvelope(issuer.key, "test-issuer", "DL-LICENSE-RESPONSE-V1", licensePayload{Version: 1, RequestID: req.RequestID, DeviceHash: licenseDeviceHash(pub), ServerTime: issuer.clock.Now().Unix(), Status: "ERROR", Error: code})
				if code == "bad_signature" {
					env.Signature = strings.Repeat("A", 86)
				}
				w.WriteHeader(403)
				json.NewEncoder(w).Encode(env)
				return w.Result(), nil
			})}
			if m.Activate(context.Background(), strings.Repeat("0", 20)) == nil || calls != 1 || m.Allowed() {
				t.Fatal("nontransient error retried or admitted")
			}
		})
	}
}
func TestR243QueuedActivationSharesTotalBudget(t *testing.T) {
	issuer := newLicenseTestIssuer(t)
	m := issuer.Manager(&licenseTestStore{})
	m.opMu.Lock()
	defer m.opMu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	start := time.Now()
	if m.Activate(ctx, strings.Repeat("0", 20)) == nil || time.Since(start) > time.Second || issuer.requests.Load() != 0 {
		t.Fatal("queued activation escaped total deadline")
	}
}
func TestR243DeadlineCancellationAndRenewBackoff(t *testing.T) {
	issuer := newLicenseTestIssuer(t)
	m := issuer.Manager(&licenseTestStore{})
	testActivate(t, m, false)
	calls := 0
	m.options.Client = &http.Client{Transport: updateRoundTrip(func(r *http.Request) (*http.Response, error) {
		calls++
		<-r.Context().Done()
		return nil, r.Context().Err()
	})}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	start := time.Now()
	if m.Activate(ctx, strings.Repeat("0", 20)) == nil || calls != 1 || time.Since(start) > time.Second {
		t.Fatal("parent deadline ignored")
	}
	m.options.Client = &http.Client{Transport: updateRoundTrip(func(r *http.Request) (*http.Response, error) { return nil, context.DeadlineExceeded })}
	m.failures = 0
	for _, want := range []time.Duration{15 * time.Second, 30 * time.Second, 60 * time.Second} {
		m.Renew(context.Background())
		if got := m.nextRenew.Sub(issuer.clock.Elapsed()); got != want {
			t.Fatalf("renew backoff %s != %s", got, want)
		}
	}
}
func TestR243FailureDiagnosticsReachExportAndSanitize(t *testing.T) {
	store := newStartupTestStore(t)
	a := &app{storage: store}
	issuer := newLicenseTestIssuer(t)
	m := issuer.Manager(&licenseTestStore{})
	testActivate(t, m, false)
	m.options.Observe = a.recordDiagnostic
	for _, body := range []string{`{"error":"SERVICE_UNAVAILABLE"}`, `{"error":"private-code-device-secret"}`, `{"error":42}`} {
		m.options.Client = &http.Client{Transport: updateRoundTrip(func(r *http.Request) (*http.Response, error) {
			w := httptest.NewRecorder()
			w.Header().Set("CF-Ray", "a123-SJC")
			w.WriteHeader(503)
			w.WriteString(body)
			return w.Result(), nil
		})}
		m.Renew(context.Background())
	}
	rr := httptest.NewRecorder()
	a.handleDiagnosticLog(rr, httptest.NewRequest("GET", "/api/diagnostics/log", nil))
	data := rr.Body.String()
	for _, marker := range []string{`"kind":"renew"`, `"failure":"unsigned_error"`, `"http_status":503`, `"cf_ray":"a123-SJC"`, `"elapsed_ms":`, `"server_error":"other"`, `"server_error":"SERVICE_UNAVAILABLE"`} {
		if !strings.Contains(data, marker) {
			t.Fatal("export missing", marker, data)
		}
	}
	if strings.Contains(data, "private-code") {
		t.Fatal("unsafe unsigned error exported")
	}
}
func TestR243InvalidWindowRecordsOnlyFieldAndType(t *testing.T) {
	store := newStartupTestStore(t)
	a := &app{storage: store}
	base := `{"event":"license_window_state","reason":"transition","licenseWindow":{"fromState":"NETWORK_LOCKED","toState":"ACTIVE","requestedContent":{"x":0,"y":0,"width":1200,"height":780},"actualContent":{"x":0,"y":0,"width":860,"height":580},"displayScale":1,"elapsedMs":3500}}`
	for _, bad := range []string{strings.Replace(base, `"width":1200`, `"width":null`, 1), strings.Replace(base, `"width":1200`, `"width":"private-secret"`, 1), strings.Replace(base, `"height":780`, `"height":0`, 1), strings.Replace(base, `"width":1200,`, ``, 1)} {
		rr := httptest.NewRecorder()
		a.handleClientDiagnostic(rr, httptest.NewRequest("POST", "/api/diagnostics/client", strings.NewReader(bad)))
		if rr.Code != 400 {
			t.Fatal(rr.Code)
		}
	}
	rows := readDiagnosticEvents(t, store, "license_window_state_invalid")
	if len(rows) != 4 {
		t.Fatal(rows)
	}
	rr := httptest.NewRecorder()
	a.handleDiagnosticLog(rr, httptest.NewRequest("GET", "/api/diagnostics/log", nil))
	data := rr.Body.String()
	for _, marker := range []string{"requestedContent.width", "requestedContent.height", `"value_type":"null"`, `"value_type":"string"`, `"value_type":"number"`, `"value_type":"missing"`} {
		if !strings.Contains(data, marker) {
			t.Fatal("missing field type", marker, data)
		}
	}
	if strings.Contains(data, "private-secret") {
		t.Fatal("raw value leaked")
	}
}

// A real cached signed lease and production manager back the native Electron
// fixture. Every key/code is generated or synthetic in this test; no user files.
func TestR243ExpiredCacheRecoveryElectronFixture(t *testing.T) {
	issuer := newLicenseTestIssuer(t)
	cache := &licenseTestStore{}
	old := issuer.Manager(cache)
	testActivate(t, old, false)
	issuer.clock.Advance(licenseLifetime + time.Second)
	m := issuer.Manager(cache)
	m.options.Elapsed = time.Now
	if m.Snapshot().State != "NETWORK_LOCKED" {
		t.Fatal("expired cache admitted")
	}
	original := m.options.Client.Transport
	var renews atomic.Int64
	immediate := os.Getenv("R243_FIXTURE_MODE") == "immediate"
	activation := os.Getenv("R243_FIXTURE_MODE") == "activation"
	var activates atomic.Int64
	var firstActivation licenseRequest
	var freshActivation atomic.Bool
	m.options.Client = &http.Client{Transport: updateRoundTrip(func(r *http.Request) (*http.Response, error) {
		if activation && r.URL.Path == "/v1/activate" {
			data, _ := io.ReadAll(r.Body)
			r.Body = io.NopCloser(bytes.NewReader(data))
			var req licenseRequest
			json.Unmarshal(data, &req)
			if activates.Add(1) == 1 {
				firstActivation = req
				<-r.Context().Done()
				return nil, r.Context().Err()
			}
			freshActivation.Store(req.RequestID != firstActivation.RequestID && req.Counter != firstActivation.Counter && req.Signature != firstActivation.Signature)
		}
		if r.URL.Path == "/v1/renew" && renews.Add(1) == 1 && !immediate {
			w := httptest.NewRecorder()
			w.WriteHeader(503)
			w.WriteString(`{"error":"SERVICE_UNAVAILABLE"}`)
			return w.Result(), nil
		}
		return original.RoundTrip(r)
	})}
	ready := os.Getenv("R243_FIXTURE_READY")
	if ready == "" {
		if m.Renew(context.Background()) == nil {
			t.Fatal("first renew should fail")
		}
		if m.Renew(context.Background()) != nil || !m.Allowed() {
			t.Fatal("second renew did not recover")
		}
		return
	}
	store := newStartupTestStore(t)
	a := &app{license: m, storage: store}
	m.options.Observe = a.recordDiagnostic
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go m.Run(ctx)
	finished := make(chan struct{}, 1)
	web := http.FileServer(http.Dir("web"))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/license/status":
			a.handleLicenseStatus(w, r)
		case "/api/license/activate":
			a.handleLicenseActivate(w, r)
		case "/api/diagnostics/client":
			a.handleClientDiagnostic(w, r)
		case "/api/diagnostics/log":
			a.handleDiagnosticLog(w, r)
		case "/fixture/done":
			w.WriteHeader(204)
			finished <- struct{}{}
		case "/fixture/evidence":
			respondJSON(w, map[string]any{"expired_cache": true, "renew_requests": renews.Load(), "activate_requests": activates.Load(), "activation_fresh_signed_request": freshActivation.Load(), "state": m.Snapshot().State})
		case "/api/events":
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, ": fixture\n\n")
		case "/api/status":
			respondJSON(w, map[string]any{"connected": false, "installations": []any{}})
		default:
			if strings.HasPrefix(r.URL.Path, "/api/") {
				respondJSON(w, map[string]any{})
			} else {
				web.ServeHTTP(w, r)
			}
		}
	}))
	defer server.Close()
	if err := os.WriteFile(filepath.Clean(ready), []byte(server.URL), 0600); err != nil {
		t.Fatal(err)
	}
	select {
	case <-finished:
	case <-time.After(60 * time.Second):
		t.Fatal("Electron fixture did not finish")
	}
	if !m.Allowed() {
		t.Fatal("fixture never recovered")
	}
}
