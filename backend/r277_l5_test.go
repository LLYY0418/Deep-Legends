package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A synthetic one-hour pressure schedule, not the missing 10-12 real-device log.
// All five new event families use their production producers/HTTP/store paths.
func TestR277L5AllAddedEventsStoredHourlyBudget(t *testing.T) {
	oldFingerprint := buildFingerprint
	buildFingerprint = strings.Repeat("f", 12)
	defer func() { buildFingerprint = oldFingerprint }()
	a, client, _ := r258ViewFixture()
	defer client.Close()
	a.storage = newFlowDiagnosticStore(t)
	post := func(body map[string]any) {
		t.Helper()
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		w := httptest.NewRecorder()
		a.handleClientDiagnostic(w, httptest.NewRequest("POST", "/api/diagnostics/client", bytes.NewReader(raw)))
		if w.Code != 204 {
			t.Fatalf("event rejected: %s: %d %s", raw, w.Code, w.Body.String())
		}
	}
	// Six close/reopen cycles, sixty polls per state, plus an old-generation rejection.
	for cycle := 0; cycle < 6; cycle++ {
		for _, state := range []string{"ready", "exiting", "no-client"} {
			a.mu.Lock()
			a.lcu = client
			a.connected = true
			a.shutdownClient = nil
			if state == "exiting" {
				a.shutdownClient = client
			}
			if state == "no-client" {
				a.connected = false
				a.lcu = nil
			}
			a.mu.Unlock()
			view := a.currentClientView()
			if view.State != state {
				t.Fatalf("fixture state %s: %+v", state, view)
			}
			for poll := 0; poll < 60; poll++ {
				a.currentClientView()
				for _, action := range []string{"received", "applied"} {
					reason := "new-generation"
					if action == "received" {
						reason = "frame"
					}
					post(map[string]any{"event": "client_view_client", "action": action, "reason": reason, "state": state, "generation": view.Generation})
				}
			}
			post(map[string]any{"event": "client_view_client", "action": "rejected", "reason": "generation-rollback", "state": state, "generation": view.Generation - 1})
		}
	}
	// Sixty distinct zoom configurations/hour, each reported twice.
	for i := 0; i < 60; i++ {
		body := map[string]any{"event": "ui_zoom_state", "reason": "applied", "mode": "fixed", "multiplier": 1.25, "windowWidth": 1280 + i, "windowHeight": 900, "screenWidth": 1920, "screenHeight": 1080, "devicePixelRatio": 2}
		post(body)
		post(body)
	}
	// Six stuck sessions/hour: the cached watchdog must emit once per session.
	for i := 0; i < 6; i++ {
		a.mu.Lock()
		a.shutdownClient = nil
		a.lcu = client
		a.connected = true
		a.eventStream = true
		a.selfSGPReady = false
		a.selfReadinessPending = true
		a.clientViewStuckSince = time.Time{}
		a.clientViewRecoveryAttempted = false
		a.mu.Unlock()
		now := time.Unix(int64(100+i*600), 0)
		a.recoverStuckClientView(client, now)
		for repeat := 0; repeat < 60; repeat++ {
			a.recoverStuckClientView(client, now.Add(10*time.Second))
		}
	}
	// Thirty exhausted two-minute windows/hour, two foreground calls per window.
	p, now, _, _ := r277OverflowFixture(t, "user")
	p.champions.diag = a.recordDiagnostic
	scope := riotRequestRateScope(p.clusterHost(), "/lol/match/v5/matches/by-puuid/r277/ids")
	for window := 0; window < 30; window++ {
		p.observeRiotRate(scope, http.Header{"X-App-Rate-Limit": {"20:1,100:120"}, "X-App-Rate-Limit-Count": {"0:1,100:120"}}, 200)
		for call := 0; call < 2; call++ {
			if _, err := p.matchIDs(context.Background(), "r277", window*20+call*10, 10); err != nil {
				t.Fatal(err)
			}
		}
		*now = now.Add(121 * time.Second)
	}
	data, err := a.storage.readDiagnosticLog()
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{"client_view_published": true, "client_view_client": true, "client_view_stuck": true, "ui_zoom_state": true, "riot_route_switch": true}
	counts, sizes := map[string]int{}, map[string]int{}
	total := 0
	for _, line := range bytes.Split(data, []byte{'\n'}) {
		if len(line) == 0 {
			continue
		}
		var row map[string]any
		if err := json.Unmarshal(line, &row); err != nil {
			t.Fatal(err)
		}
		name, _ := row["event"].(string)
		if !names[name] {
			continue
		}
		if row["build_fingerprint"] != strings.Repeat("f", 12) || row["run_id"] == nil || row["log_seq"] == nil || row["time"] == nil {
			t.Fatalf("missing stored metadata: %v", row)
		}
		counts[name]++
		sizes[name] += len(line) + 1
		total += len(line) + 1
	}
	expected := map[string]int{"client_view_published": 18, "client_view_client": 54, "client_view_stuck": 6, "ui_zoom_state": 60, "riot_route_switch": 30}
	for name, count := range expected {
		if counts[name] != count {
			t.Errorf("%s count=%d want=%d (missing or noisy event)", name, counts[name], count)
		}
	}
	if total > 50000 {
		t.Errorf("added diagnostics %d bytes/hour > 50KB/hour", total)
	}
	report := map[string]any{"kind": "synthetic-pressure-production-storage", "real_device_replay": false, "duration_hours": 1, "added_bytes_per_hour": total, "limit_bytes_per_hour": 50000, "counts": counts, "bytes_by_event": sizes, "schedule": "6 reopen cycles x 3 states x 60 polls; generation rollback each state; 60 zoom configurations repeated twice; 6 stuck sessions x 60 watchdog checks; 30 overflow windows x 2 calls; release 12-character fingerprint and storage metadata"}
	raw, _ := json.MarshalIndent(report, "", "  ")
	t.Log(string(raw))
	if out := os.Getenv("R277_L5_OUTPUT"); out != "" {
		if err := os.MkdirAll(out, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(out, "synthetic-result.json"), append(raw, '\n'), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(out, "synthetic-stored.jsonl"), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestR277ClientDiagnosticsRejectInvalidActionsAndZoom(t *testing.T) {
	for _, body := range []string{
		`{"event":"client_view_client","action":"unexpected","reason":"new-generation","state":"ready","generation":3}`,
		`{"event":"client_view_client","action":"applied","reason":"sensitive-arbitrary-string","state":"ready","generation":3}`,
		`{"event":"ui_zoom_state","reason":"applied","mode":"fixed","multiplier":1.25,"windowWidth":0,"windowHeight":900,"screenWidth":1920,"screenHeight":1080,"devicePixelRatio":2}`,
	} {
		a := &app{storage: newFlowDiagnosticStore(t)}
		w := httptest.NewRecorder()
		a.handleClientDiagnostic(w, httptest.NewRequest("POST", "/api/diagnostics/client", strings.NewReader(body)))
		if w.Code != 400 {
			t.Fatalf("invalid diagnostic accepted: %s -> %d", body, w.Code)
		}
		data, err := a.storage.readDiagnosticLog()
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(data, []byte(`"event":"client_view_client"`)) || bytes.Contains(data, []byte(`"event":"ui_zoom_state"`)) || bytes.Contains(data, []byte("sensitive-arbitrary-string")) {
			t.Fatalf("invalid diagnostic payload stored: %s", data)
		}
	}
}
