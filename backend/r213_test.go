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

func TestR213ProgressAndRebuildDiagnosticWhitelist(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		check      func(*testing.T, map[string]any)
	}{
		{"progress", `{"event":"live_progress_apply","reason":"aggregated","applied":2,"ignored_same_game":3,"ignored_stale":-7,"windowMs":60000,"requestId":123}`, func(t *testing.T, event map[string]any) {
			if event["applied"] != float64(2) || event["ignored_same_game"] != float64(3) || event["ignored_stale"] != float64(0) || event["window_ms"] != float64(60000) {
				t.Fatal(event)
			}
			if _, ok := event["request_id"]; ok {
				t.Fatal("progress diagnostic retained request identity", event)
			}
		}},
		{"rebuild", `{"event":"live_render_rebuild","reason":"aggregated","counts":{"full":1,"PRIVATE":99},"sources":{"progress":2,"PRIVATE":99},"fullReasons":{"tab-row":1,"banner":-2,"panel-count":2,"lane-slot":3,"other":2000000,"PRIVATE":99},"imagesRecreated":0}`, func(t *testing.T, event map[string]any) {
			reasons := event["full_reasons"].(map[string]any)
			if reasons["tab-row"] != float64(1) || reasons["banner"] != float64(0) || reasons["other"] != float64(1000000) || len(reasons) != 5 || event["sources"].(map[string]any)["progress"] != float64(2) {
				t.Fatal(event)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.MkdirAll(filepath.Join(root, "logs"), 0700); err != nil {
				t.Fatal(err)
			}
			a := &app{storage: trackTestStore(t, &localStore{root: root})}
			response := httptest.NewRecorder()
			a.handleClientDiagnostic(response, httptest.NewRequest(http.MethodPost, "/api/diagnostics/client", strings.NewReader(tc.body)))
			if response.Code != http.StatusNoContent {
				t.Fatal(response.Code, response.Body.String())
			}
			raw, err := a.storage.readDiagnosticLog()
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(raw), "PRIVATE") {
				t.Fatal("unapproved aggregate key retained", string(raw))
			}
			var event map[string]any
			if err := json.Unmarshal(raw, &event); err != nil {
				t.Fatal(err)
			}
			tc.check(t, event)
		})
	}
}
