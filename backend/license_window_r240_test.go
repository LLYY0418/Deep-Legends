//go:build license

package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestR240WindowDiagnosticReachesActualExportAndRejectsIdentity(t *testing.T) {
	store := newStartupTestStore(t)
	a := &app{storage: store}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Local-Token") != startupStageTestToken {
			http.Error(w, "unauthorized", 401)
			return
		}
		if r.URL.Path == "/api/diagnostics/client" {
			a.handleClientDiagnostic(w, r)
		} else {
			a.handleDiagnosticLog(w, r)
		}
	}))
	defer server.Close()
	value := licenseWindowDiagnostic{FromState: "ACTIVE", ToState: "REVOKED", WasMaximized: true, RequestedContent: licenseWindowBounds{X: 10, Y: 20, Width: 860, Height: 580}, ActualContent: licenseWindowBounds{X: 10, Y: 20, Width: 1600, Height: 900}, DisplayScale: 1.25, ElapsedMS: 120, Retried: false}
	body, _ := json.Marshal(map[string]any{"event": "license_window_state", "reason": "transition", "licenseWindow": value})
	req, _ := http.NewRequest("POST", server.URL+"/api/diagnostics/client", strings.NewReader(string(body)))
	req.Header.Set("X-Local-Token", startupStageTestToken)
	response, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 204 {
		t.Fatal(response.StatusCode)
	}
	get, _ := http.NewRequest("GET", server.URL+"/api/diagnostics/log", nil)
	get.Header.Set("X-Local-Token", startupStageTestToken)
	response, err = server.Client().Do(get)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(response.Body)
	if response.StatusCode != 200 || !strings.Contains(string(data), `"event":"license_window_state"`) || !strings.Contains(string(data), `"was_maximized":true`) || !strings.Contains(string(data), `"width":1600`) {
		t.Fatalf("export lost native state: %d %s", response.StatusCode, data)
	}
	for _, bad := range []string{strings.Replace(string(body), `"licenseWindow":{`, `"licenseWindow":{"code":"forbidden-test-identity",`, 1), strings.Replace(string(body), `"ACTIVE"`, `"arbitrary-identity"`, 1)} {
		recorder := httptest.NewRecorder()
		a.handleClientDiagnostic(recorder, httptest.NewRequest("POST", "/api/diagnostics/client", strings.NewReader(bad)))
		if recorder.Code != 400 {
			t.Fatalf("unsafe body admitted: %d", recorder.Code)
		}
	}
	events := readDiagnosticEvents(t, store, "license_window_state")
	if len(events) != 1 {
		t.Fatalf("rejected body persisted: %#v", events)
	}
}

func TestR240CachedActiveStartupHasExactlyOneRenewInFirstFiveSeconds(t *testing.T) {
	// Each issuer/manager generates its own fresh in-memory signing keys. No
	// existing private-key file or shared protocol vector is read.
	issuer := newLicenseTestIssuer(t)
	store := &licenseTestStore{}
	initial := issuer.Manager(store)
	testActivate(t, initial, false)
	restored := issuer.Manager(store)
	if !restored.Allowed() {
		t.Fatal("fresh cached lease not ACTIVE")
	}
	restored.options.Elapsed = time.Now
	original := restored.options.Client.Transport
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	var renews atomic.Int64
	restored.options.Client = &http.Client{Transport: updateRoundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/v1/renew" {
			if renews.Add(1) == 1 {
				started <- struct{}{}
				select {
				case <-release:
				case <-r.Context().Done():
					return nil, r.Context().Err()
				}
			}
		}
		return original.RoundTrip(r)
	})}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); restored.Run(ctx) }()
	defer func() { cancel(); <-done }()
	start := time.Now()
	select {
	case <-started:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("first startup renew was postponed")
	}
	// Hold the first request over a 1s Run tick. The old implementation queued a
	// second request while nextRenew still had its initial zero value.
	time.Sleep(1200 * time.Millisecond)
	close(release)
	time.Sleep(time.Until(start.Add(5 * time.Second)))
	if n := renews.Load(); n != 1 {
		t.Fatalf("cached ACTIVE startup sent %d renews in first 5s, want 1", n)
	}
}

func TestR240ReplacementAndRevocationMessages(t *testing.T) {
	if got := licenseMessage("REPLACED"); got != "注册码已在其他设备使用或已被重置" {
		t.Fatal("REPLACED message", got)
	}
	if got := licenseMessage("REVOKED"); got != "注册码已停用" {
		t.Fatal("REVOKED message", got)
	}
}
