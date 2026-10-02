package main

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestR188LaneMatchupPlacementDiagnosticAllowlist(t *testing.T) {
	a := r175App(t)
	for _, placement := range []string{"tab-row", "", "PRIVATE-LOCATION"} {
		w := httptest.NewRecorder()
		body := fmt.Sprintf(`{"event":"lane_matchup_card","reason":"render","mode":"a+b","shown":true,"ownLocked":false,"enemyChampionId":103,"tier":"emerald_plus","placement":%q}`, placement)
		a.handleClientDiagnostic(w, httptest.NewRequest("POST", "/api/diagnostics/client", strings.NewReader(body)))
		if w.Code != 204 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	events := r175Events(t, a, "lane_matchup_card")
	if len(events) != 3 || events[0]["placement"] != "tab-row" {
		t.Fatal(events)
	}
	for i, event := range events {
		if event["mode"] != "a+b" || event["shown"] != true || event["own_locked"] != false || event["enemy_champion_id"] != float64(103) || event["tier"] != "emerald_plus" {
			t.Fatal("existing fields changed", event)
		}
		if i > 0 {
			if _, ok := event["placement"]; ok {
				t.Fatal("unknown placement escaped allowlist", event)
			}
		}
	}
}
