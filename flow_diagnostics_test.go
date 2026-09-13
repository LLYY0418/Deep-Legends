package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func newFlowDiagnosticStore(t *testing.T) *localStore {
	t.Helper()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "logs"), 0700); err != nil {
		t.Fatal(err)
	}
	store := trackTestStore(t, &localStore{root: root})
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestFlowDiagnosticsPreflightReasonsAndNoWrites(t *testing.T) {
	f := newR78ChampSelectFixture(t, "practice", "pick", 0, []int64{5}, map[int64]champSelectGridSelectionStatus{5: {}})
	f.enable("practice", "pick", []int64{5})
	var events []map[string]any
	f.runner.observe = func(event map[string]any) { events = append(events, event) }
	d := champSelectDecision{Action: champSelectActionPick, ActionID: 42, ChampionID: 5, SessionAPI: champSelectAPI, LocalCellID: 9, QueueID: 3100}
	if f.runner.champSelectRequestStillCurrent(context.Background(), f.client, d) {
		t.Fatal("mismatch accepted")
	}
	if len(events) == 0 || events[len(events)-1]["reason"] != "session-identity-changed" {
		t.Fatalf("events=%v", events)
	}
	if f.patches.Load() != 0 {
		t.Fatal("diagnostics performed a write")
	}
}

func TestFlowDiagnosticsPostflightDistinguishesAckFromApplied(t *testing.T) {
	f := newR78ChampSelectFixture(t, "practice", "pick", 0, []int64{5}, map[int64]champSelectGridSelectionStatus{5: {}})
	f.runner.champSelect.submitted[42] = champSelectSubmitRecord{ChampionID: 5, Completed: true, At: time.Now().Add(-3 * time.Second), TraceID: "cs-write-fixture"}
	var reasons []string
	f.runner.observe = func(e map[string]any) {
		if e["stage"] == "postflight" {
			reasons = append(reasons, e["reason"].(string))
		}
	}
	f.runner.recordChampSelectPostflight(*f.session)
	f.session.Actions[0][0].ChampionID = 5
	f.session.Actions[0][0].Completed = true
	f.runner.recordChampSelectPostflight(*f.session)
	if strings.Join(reasons, ",") != "not-applied-after-2s,applied" {
		t.Fatal(reasons)
	}
	if f.patches.Load() != 0 || f.gridReads.Load() != 0 {
		t.Fatal("postflight issued extra API calls")
	}
}

func TestFlowDiagnosticsDedupHeartbeatAndExportCounts(t *testing.T) {
	f := newR78ChampSelectFixture(t, "practice", "pick", 0, nil, nil)
	var events []map[string]any
	f.runner.observe = func(e map[string]any) { events = append(events, e) }
	for i := 0; i < 50; i++ {
		f.runner.champDiagnostic("trigger", "disabled", champSelectDecision{}, nil)
	}
	if len(events) != 1 {
		t.Fatalf("poll spam: %d", len(events))
	}
	f.runner.recordFlowExportSnapshot()
	counts := events[1]["suppressed_since_last_emit"].(map[string]int)
	if counts["trigger:"] != 49 {
		t.Fatal(counts)
	}
	f.runner.mu.Lock()
	sample := f.runner.champDiagnosticSamples["trigger:"]
	sample.At = time.Now().Add(-31 * time.Second)
	f.runner.champDiagnosticSamples["trigger:"] = sample
	f.runner.mu.Unlock()
	f.runner.champDiagnostic("trigger", "disabled", champSelectDecision{}, nil)
	if events[2]["previous_repeats"] != 49 {
		t.Fatal(events[2])
	}
}

func TestFlowDiagnosticsRunAndSequenceAreOrderedUnderConcurrency(t *testing.T) {
	store := newFlowDiagnosticStore(t)
	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := store.appendDiagnostic(map[string]any{"event": "test"}); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	data, err := store.readDiagnosticLog()
	if err != nil {
		t.Fatal(err)
	}
	run := ""
	for i, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var event map[string]any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			run = event["run_id"].(string)
		}
		if run == "" || event["run_id"] != run || event["log_seq"] != float64(i+1) {
			t.Fatal(event)
		}
	}
}

func TestFlowDiagnosticsExportKeepsFiveOrderedGenerations(t *testing.T) {
	store := newFlowDiagnosticStore(t)
	for _, name := range []string{"diagnostics.4.jsonl", "diagnostics.3.jsonl", "diagnostics.2.jsonl", "diagnostics.1.jsonl", "diagnostics.jsonl"} {
		if err := os.WriteFile(filepath.Join(store.root, "logs", name), []byte(name+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	data, err := store.readDiagnosticLogForExport()
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "diagnostics.4.jsonl\ndiagnostics.3.jsonl\ndiagnostics.2.jsonl\ndiagnostics.1.jsonl\ndiagnostics.jsonl\n" {
		t.Fatal(string(data))
	}
}

func TestFlowDiagnosticsClientAllowlistDiscardsIdentityText(t *testing.T) {
	store := newFlowDiagnosticStore(t)
	a := &app{storage: store}
	r := httptest.NewRecorder()
	body := `{"event":"current_game_client","reason":"received","traceId":"cg-1234567890123-1","key":"private-player","position":"private-token","phase":"active","playersReceived":10,"gameId":999}`
	a.handleClientDiagnostic(r, httptest.NewRequest(http.MethodPost, "/api/diagnostics/client", strings.NewReader(body)))
	if r.Code != 204 {
		t.Fatal(r.Body.String())
	}
	data, err := store.readDiagnosticLog()
	if err != nil {
		t.Fatal(err)
	}
	var record map[string]any
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatal(err)
	}
	// Do not match "999" inside random run IDs or timestamps.
	if strings.Contains(string(data), "private") || record["game_id"] != nil || record["gameId"] != nil || record["trace_id"] != "cg-1234567890123-1" {
		t.Fatal(string(data))
	}
}

func TestFlowDiagnosticsCanceledEventsRemainExportable(t *testing.T) {
	store := newFlowDiagnosticStore(t)
	a := &app{storage: store}
	a.currentGameDiagnostic(context.WithValue(context.Background(), currentGameTraceKey{}, "cg-fixture"), "request", "canceled", nil)
	data, _ := store.readDiagnosticLog()
	if !strings.Contains(string(data), `"reason":"canceled"`) {
		t.Fatal(string(data))
	}
}

func TestFlowDiagnosticsRosterSummaryAndNestedAuthArePrivate(t *testing.T) {
	auth := sgpAuthDiagnostic([]byte(`{"error":{"message":"expired private-secret"},"secret":"never-log"}`), "secret-token")
	if auth["auth_error_class"] != "expired" {
		t.Fatal(auth)
	}
	data, _ := json.Marshal(auth)
	if strings.Contains(string(data), "private-secret") || strings.Contains(string(data), "never-log") || strings.Contains(string(data), "secret-token") {
		t.Fatal(string(data))
	}
}

func TestFlowDiagnosticsEarlyRequestFailuresKeepTrace(t *testing.T) {
	for _, tc := range []struct {
		name, body, reason string
		status             int
	}{
		{"invalid", `{`, "invalid-payload", 400},
		{"expired", `{"playerRef":"expired","traceId":"cg-1234567890123-1"}`, "reference-expired", 404},
		{"private", "", "privacy-blocked", 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := newFlowDiagnosticStore(t)
			a := &app{storage: store}
			body := tc.body
			if tc.name == "private" {
				ref := a.registerGameplayReferenceDetails(gameplayReference{PlayerRef: "current-game-fixture-private-000000001", Region: "kr", Privacy: "PRIVATE"})
				body = `{"playerRef":"` + ref + `","traceId":"cg-1234567890123-1"}`
			}
			w := httptest.NewRecorder()
			a.handleOverviewCurrentGame(w, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body)))
			trace := w.Header().Get("X-Diagnostic-Trace")
			if w.Code != tc.status || trace == "" || tc.name != "invalid" && trace != "cg-1234567890123-1" {
				t.Fatalf("status=%d trace=%q", w.Code, trace)
			}
			data, err := store.readDiagnosticLog()
			if err != nil {
				t.Fatal(err)
			}
			var event map[string]any
			if err := json.Unmarshal(data, &event); err != nil {
				t.Fatal(err)
			}
			if event["trace_id"] != trace || event["reason"] != tc.reason {
				t.Fatal(event)
			}
		})
	}
}

func TestFlowDiagnosticsRotationMarkerSequence(t *testing.T) {
	store := newFlowDiagnosticStore(t)
	path := filepath.Join(store.root, "logs", "diagnostics.jsonl")
	if err := os.WriteFile(path, []byte(strings.Repeat(" ", 2*1024*1024+1)), 0600); err != nil {
		t.Fatal(err)
	}
	if err := store.appendDiagnostic(map[string]any{"event": "trigger"}); err != nil {
		t.Fatal(err)
	}
	data, err := store.readDiagnosticLog()
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("lines=%d", len(lines))
	}
	for i, line := range lines {
		var event map[string]any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}
		if event["log_seq"] != float64(i+1) || event["run_id"] != store.diagnosticRunID {
			t.Fatal(event)
		}
	}
}

func TestFlowDiagnosticsRotationFailurePreservesEvidence(t *testing.T) {
	store := newFlowDiagnosticStore(t)
	path := filepath.Join(store.root, "logs", "diagnostics.jsonl")
	original := strings.Repeat(" ", 2*1024*1024+1)
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(store.root, "logs", "diagnostics.1.jsonl")
	if err := os.Mkdir(archive, 0700); err != nil {
		t.Fatal(err)
	}
	if err := store.appendDiagnostic(map[string]any{"event": "trigger"}); err == nil {
		t.Fatal("untrusted archive accepted")
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != original {
		t.Fatalf("original evidence changed: %v", err)
	}
	if info, err := os.Stat(archive); err != nil || !info.IsDir() {
		t.Fatal("untrusted path removed")
	}
}

func TestFlowDiagnosticsSamplingMemoryIsBounded(t *testing.T) {
	runner := newWatchRunner(nil, nil)
	runner.observe = func(map[string]any) {}
	for id := int64(0); id < 1000; id++ {
		runner.champDiagnostic("postflight", "applied", champSelectDecision{ActionID: id}, nil)
	}
	if len(runner.champDiagnosticSamples) != 256 {
		t.Fatal(len(runner.champDiagnosticSamples))
	}
}
