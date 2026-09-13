package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// R75: instrumentation is worthless if it is silently dropped. This project has
// already shipped telemetry that never reached the log because a whitelist
// rejected it, so assert the recorded event end to end rather than just the
// HTTP status.
func TestDesktopStartupPhasesReachTheDiagnosticLog(t *testing.T) {
	store := newStartupTestStore(t)
	a := &app{storage: store}

	body := `{"processToJs":4200,"jsToReady":310,"readyToSplash":18,"splashPaint":260,"splashWindowShown":4800,"spawnToReady":420,"readyToWindow":900,"total":5850}`
	recorder := httptest.NewRecorder()
	a.handleDesktopStartup(recorder, httptest.NewRequest(http.MethodPost, "/api/diagnostics/startup", strings.NewReader(body)))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNoContent)
	}

	entries := readDiagnosticEvents(t, store, "desktop_startup_phases_ms")
	if len(entries) != 1 {
		t.Fatalf("desktop_startup_phases_ms entries = %d, want 1", len(entries))
	}
	phases, ok := entries[0]["phases_ms"].(map[string]any)
	if !ok {
		t.Fatalf("phases_ms missing or wrong type: %#v", entries[0])
	}
	for key, want := range map[string]float64{
		"process_to_js": 4200, "js_to_ready": 310, "ready_to_splash": 18,
		"splash_paint": 260, "spawn_to_ready": 420, "ready_to_window": 900, "total": 5850,
		"splash_window_shown": 4800,
	} {
		got, ok := phases[key].(float64)
		if !ok {
			t.Fatalf("phase %q missing: %#v", key, phases)
		}
		if got != want {
			t.Fatalf("phase %q = %v, want %v", key, got, want)
		}
	}
}

func TestDesktopStartupRejectsMalformedPayloads(t *testing.T) {
	cases := map[string]string{
		"unknown field": `{"processToJs":1,"somethingElse":2}`,
		"trailing json": `{"processToJs":1}{"processToJs":2}`,
		"not an object": `[1,2,3]`,
		"wrong type":    `{"processToJs":"slow"}`,
		"empty body":    ``,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			store := newStartupTestStore(t)
			a := &app{storage: store}
			recorder := httptest.NewRecorder()
			a.handleDesktopStartup(recorder, httptest.NewRequest(http.MethodPost, "/api/diagnostics/startup", strings.NewReader(body)))
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
			}
			if entries := readDiagnosticEvents(t, store, "desktop_startup_phases_ms"); len(entries) != 0 {
				t.Fatalf("rejected payload still recorded %d entries", len(entries))
			}
		})
	}
}

// A stuck clock or a machine suspended mid-launch must not write absurd values
// into the log; clamping keeps the field bounded without dropping the record.
func TestDesktopStartupClampsOutOfRangePhases(t *testing.T) {
	store := newStartupTestStore(t)
	a := &app{storage: store}
	body := `{"processToJs":-5,"splashWindowShown":99999999,"total":99999999}`
	recorder := httptest.NewRecorder()
	a.handleDesktopStartup(recorder, httptest.NewRequest(http.MethodPost, "/api/diagnostics/startup", strings.NewReader(body)))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNoContent)
	}
	entries := readDiagnosticEvents(t, store, "desktop_startup_phases_ms")
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}
	phases := entries[0]["phases_ms"].(map[string]any)
	if phases["process_to_js"].(float64) != 0 {
		t.Fatalf("negative phase not clamped to 0: %v", phases["process_to_js"])
	}
	if phases["total"].(float64) != desktopStartupPhaseLimitMs {
		t.Fatalf("oversized phase not clamped: %v", phases["total"])
	}
	if phases["splash_window_shown"].(float64) != desktopStartupPhaseLimitMs {
		t.Fatalf("oversized splash show time not clamped: %v", phases["splash_window_shown"])
	}
}

func readDiagnosticEvents(t *testing.T, store *localStore, event string) []map[string]any {
	t.Helper()
	raw, err := store.readDiagnosticLog()
	if err != nil {
		t.Fatalf("read diagnostics: %v", err)
	}
	var matched []map[string]any
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if name, _ := entry["event"].(string); name == event {
			matched = append(matched, entry)
		}
	}
	return matched
}

// appendDiagnostic writes into <root>/logs and fails silently if the directory
// is missing, so a store without it would make every assertion below vacuous.
func newStartupTestStore(t *testing.T) *localStore {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "logs"), 0o700); err != nil {
		t.Fatalf("prepare logs directory: %v", err)
	}
	return trackTestStore(t, &localStore{root: root})
}
