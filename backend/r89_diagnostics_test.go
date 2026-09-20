package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestR89DiagnosticArenaParserDirectStoragePath(t *testing.T) {
	store := newFlowDiagnosticStore(t)
	data, err := os.ReadFile("testdata/r88/yourgg-arena-rankings.json")
	if err != nil {
		t.Fatal(err)
	}
	data = bytes.Replace(data, []byte(`"tier":"OP"`), []byte(`"tier":"XYZ"`), 1)
	// Match the early provider callback in main: deliberately bypass app's
	// diagnostic wrapper, so moving the stamp back there fails this regression.
	provider := newChampionProvider()
	provider.diag = func(event map[string]any) {
		if err := store.appendDiagnostic(event); err != nil {
			t.Fatal(err)
		}
	}
	ranking, err := parseYourGGArenaRankings(data, time.Now(), provider.diag)
	if err != nil || len(ranking.Rows) != 172 {
		t.Fatalf("parser fixture: rows=%d err=%v", len(ranking.Rows), err)
	}
	events := readDiagnosticEvents(t, store, "arena_rankings_parse_failed")
	if len(events) != 1 || events[0]["build_fingerprint"] != buildFingerprint || events[0]["field"] != "tier" {
		t.Fatalf("unmarked parser failure: %#v", events)
	}
}

func TestR89DiagnosticStorageOverridesStaleProducerMarkerWithoutMutation(t *testing.T) {
	store := newFlowDiagnosticStore(t)
	input := map[string]any{"event": "direct-storage-probe", "build_fingerprint": "old-build"}
	if err := store.appendDiagnostic(input); err != nil {
		t.Fatal(err)
	}
	if input["build_fingerprint"] != "old-build" || len(input) != 2 {
		t.Fatal("caller payload mutated", input)
	}
	events := readDiagnosticEvents(t, store, "direct-storage-probe")
	if len(events) != 1 || events[0]["build_fingerprint"] != buildFingerprint || events[0]["log_seq"] != float64(1) {
		t.Fatal(events)
	}
}

func TestR89DiagnosticStandardEventsRetainFingerprint(t *testing.T) {
	store := newFlowDiagnosticStore(t)
	a := &app{storage: store}
	for _, name := range []string{"live_load_cost", "champselect_trace"} {
		a.recordDiagnostic(map[string]any{"event": name, "stage": "evaluation", "duration_ms": 12})
		events := readDiagnosticEvents(t, store, name)
		if len(events) != 1 || events[0]["build_fingerprint"] != buildFingerprint {
			t.Fatalf("%s: %#v", name, events)
		}
	}
}

func TestR89DiagnosticStorageRotationMarkerAlsoHasFingerprint(t *testing.T) {
	store := newFlowDiagnosticStore(t)
	if err := os.WriteFile(filepath.Join(store.root, "logs", "diagnostics.jsonl"), bytes.Repeat([]byte(" "), 2*1024*1024+1), 0600); err != nil {
		t.Fatal(err)
	}
	if err := store.appendDiagnostic(map[string]any{"event": "rotation-trigger"}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"log_rotated", "rotation-trigger"} {
		events := readDiagnosticEvents(t, store, name)
		if len(events) != 1 || events[0]["build_fingerprint"] != buildFingerprint {
			t.Fatalf("%s: %#v", name, events)
		}
	}
}
