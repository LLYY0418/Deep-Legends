package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLPHistoryLoadDiagnostics(t *testing.T) {
	for _, test := range []struct {
		name, data, status, reason string
		asDirectory                bool
	}{
		{name: "missing", status: "read_failed", reason: "not_found"},
		{name: "read failure", asDirectory: true, status: "read_failed", reason: "io_error"},
		{name: "malformed", data: `{"private":"sensitive-player",`, status: "reset", reason: "invalid_json"},
		{name: "schema", data: `{"schemaVersion":99,"games":{}}`, status: "reset", reason: "invalid_history"},
		{name: "legacy identity", data: `{"schemaVersion":1,"baselines":{"sensitive-player":{}}}`, status: "reset", reason: "invalid_history"},
		{name: "oversized", data: strings.Repeat(" ", (1<<20)+1), status: "reset", reason: "file_too_large"},
		{name: "empty valid history", data: `{"schemaVersion":2,"games":{}}`, status: "loaded"},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := trackTestStore(t, &localStore{root: t.TempDir()})
			path := filepath.Join(store.root, lpHistoryFile)
			if test.asDirectory {
				if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
			} else if test.data != "" {
				if err := os.WriteFile(path, []byte(test.data), 0600); err != nil {
					t.Fatal(err)
				}
			}
			var events []map[string]any
			tracker := newLPTrackerWithDiagnostics(store, func(event map[string]any) {
				if !store.mu.TryLock() {
					t.Fatal("load callback holds store lock")
				}
				store.mu.Unlock()
				events = append(events, event)
			})
			event, ok := lpObservation(events, "lp_history_load")
			if !ok || event["status"] != test.status || (test.reason != "" && event["reason"] != test.reason) || event["record_count"] != 0 {
				t.Fatalf("load result = %#v", event)
			}
			if test.status == "reset" {
				if event["reset_result"] != "saved" {
					t.Fatal(event)
				}
				reloaded := newLPTracker(store)
				if len(reloaded.history.Games) != 0 || len(tracker.history.Games) != 0 {
					t.Fatal("invalid records retained")
				}
			}
			raw, _ := json.Marshal(events)
			if bytes.Contains(raw, []byte(store.root)) || bytes.Contains(raw, []byte("sensitive-player")) {
				t.Fatal("load diagnostics expose raw path or file contents")
			}
		})
	}
	if lpHistoryIOReason(os.ErrPermission) != "permission_denied" {
		t.Fatal("permission classification lost")
	}
}

func TestLPHistorySaveDiagnosticsAndRestart(t *testing.T) {
	ref, queue := strings.Repeat("s", 48), "RANKED_SOLO_5x5"
	baseline := lpSnapshot{Tier: "GOLD", Division: "II", LeaguePoints: 50, Wins: 10, Losses: 10}
	tracker, hash := seededLPTracker(t, ref, queue, baseline)
	events := captureLPObservations(tracker)
	clock := time.Unix(1820000000, 0)
	tracker.now = func() time.Time { return clock }
	observe := func() { tracker.observe(ref, []gameplayRank{r182Rank(baseline, queue)}) }
	for i := 0; i < 5; i++ {
		observe()
	}
	if lpObservationCount(*events, "lp_history_save") != 1 {
		t.Fatal("unchanged saves not bounded", *events)
	}
	clock = clock.Add(time.Minute)
	observe()
	if lpObservationCount(*events, "lp_history_save") != 2 {
		t.Fatal("periodic save evidence missing")
	}
	// Block the atomic rename without relying on OS/user permission behavior.
	path := filepath.Join(tracker.store.root, lpHistoryFile)
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	observe()
	failed := (*events)[len(*events)-1]
	if failed["event"] != "lp_history_save" || failed["status"] != "write_failed" {
		t.Fatal("save failure lost", failed)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	tracker.mu.Lock()
	tracker.history.Games["90003"] = lpGameRecord{AccountHash: hash, QueueType: queue, Delta: 24, RecordedAt: 1}
	tracker.mu.Unlock()
	observe()
	saved := (*events)[len(*events)-1]
	if saved["status"] != "saved" || saved["record_count"] != 1 {
		t.Fatal("recovery/count missing", saved)
	}
	var load map[string]any
	restarted := newLPTrackerWithDiagnostics(tracker.store, func(event map[string]any) { load = event })
	if load["status"] != "loaded" || load["record_count"] != 1 || load["baseline_count"] != 1 || restarted.history.Games["90003"].Delta != 24 {
		t.Fatal("restart evidence missing", load)
	}
	assertLPObservationPrivacy(t, *events, ref, hash, queue, tracker.store.root)
}

func TestLPHistorySaveCallbacksAndExport(t *testing.T) {
	tracker, ref, queue, baseline, client := r182Start(t)
	a := r175App(t)
	var saves []map[string]any
	tracker.observeEvent = func(event map[string]any) {
		if !tracker.mu.TryLock() {
			t.Fatal("diagnostic callback holds tracker lock")
		}
		tracker.mu.Unlock()
		if !tracker.store.mu.TryLock() {
			t.Fatal("diagnostic callback holds store lock")
		}
		tracker.store.mu.Unlock()
		a.recordDiagnostic(event)
		if event["event"] == "lp_history_save" {
			saves = append(saves, event)
		}
	}
	tracker.observe(ref, []gameplayRank{r182Rank(baseline, queue)})
	// A fresh game-start request and capture both produce persisted outcomes.
	tracker.mu.Lock()
	tracker.startSeen = map[string]bool{}
	delete(tracker.history.GameStarts[tracker.accountHash(ref)], queue)
	tracker.mu.Unlock()
	tracker.takeGameStart(client, ref, func() ([]gameplayRank, EndpointCapability) {
		return []gameplayRank{r182Rank(baseline, queue)}, EndpointCapability{State: capabilityAvailable, Path: "lcu:ranked"}
	})
	after := baseline
	after.Wins++
	after.LeaguePoints += 24
	r182Capture(tracker, client, ref, queue, []lpSnapshot{after}, nil)
	if len(saves) != 3 || saves[0]["stage"] != "observe" || saves[1]["stage"] != "game_start" || saves[2]["stage"] != "capture" || saves[2]["record_count"] != 1 {
		t.Fatal("save stages missing", saves)
	}
	if len(r175Events(t, a, "lp_history_save")) != 3 {
		t.Fatal("save events missing from diagnostic export")
	}
	newLPTrackerWithDiagnostics(tracker.store, a.recordDiagnostic)
	loads := r175Events(t, a, "lp_history_load")
	if len(loads) != 1 || loads[0]["record_count"] != float64(1) {
		t.Fatal("startup load event missing from diagnostic export", loads)
	}
}
