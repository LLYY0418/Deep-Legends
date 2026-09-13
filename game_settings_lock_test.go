package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestSettingsLockRejectsUnverifiedResult(t *testing.T) {
	for _, tc := range []struct {
		name    string
		locked  bool
		missing bool
	}{
		{name: "lock-reverted", locked: true},
		{name: "unlock-reverted"},
		{name: "lock-file-missing", locked: true, missing: true},
		{name: "unlock-file-missing", missing: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := t.TempDir()
			settingsFile := filepath.Join(base, "Game", "Config", "PersistedSettings.json")
			if err := os.MkdirAll(filepath.Dir(settingsFile), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(settingsFile, []byte(`{}`), 0o600); err != nil {
				t.Fatal(err)
			}
			// Restore writability before TempDir cleanup, including on Windows.
			t.Cleanup(func() { _ = os.Chmod(settingsFile, 0o600) })
			if !tc.locked {
				if err := os.Chmod(settingsFile, 0o444); err != nil {
					t.Fatal(err)
				}
			}
			var reads atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/data-store/v1/install-dir":
					if reads.Add(1) == 2 {
						// Deterministically undo chmod before the handler reads back
						// the file. No sleeps, global hooks, or production test seams.
						mode := os.FileMode(0o444)
						if tc.locked || tc.missing {
							mode = 0o600
						}
						if err := os.Chmod(settingsFile, mode); err != nil {
							t.Errorf("revert permissions: %v", err)
						}
						if tc.missing {
							if err := os.Remove(settingsFile); err != nil {
								t.Errorf("remove settings: %v", err)
							}
						}
					}
					_ = json.NewEncoder(w).Encode(filepath.Join(base, "LeagueClient"))
				case "/riotclient/ux-state":
					_ = json.NewEncoder(w).Encode("Running")
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client(), region: "TENCENT", platformProbe: true}
			a := &app{connected: true, lcu: client}
			payload := `{"locked":false}`
			if tc.locked {
				payload = `{"locked":true}`
			}
			recorder := httptest.NewRecorder()
			a.handleSettingsLock(recorder, httptest.NewRequest(http.MethodPost, "/api/rig/settings-lock", strings.NewReader(payload)))
			if reads.Load() != 2 {
				t.Fatalf("install-dir reads = %d, want initial lookup and verification", reads.Load())
			}
			if recorder.Code != http.StatusServiceUnavailable || !strings.Contains(recorder.Body.String(), "未生效或无法确认") {
				t.Fatalf("unverified state response = %d %s, want 503 verification failure", recorder.Code, recorder.Body.String())
			}
		})
	}
}
