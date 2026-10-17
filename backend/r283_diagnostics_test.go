package main

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestR283PendingRefreshIsBusy(t *testing.T) {
	a := &app{collectionRefreshPending: true}
	w := httptest.NewRecorder()
	a.handleStatus(w, httptest.NewRequest("GET", "/api/status", nil))
	var got statusResponse
	if json.Unmarshal(w.Body.Bytes(), &got) != nil || !got.Syncing {
		t.Fatal("queued refresh was reported idle", w.Body.String())
	}
}
func TestR283DiagnosticsAreAccepted(t *testing.T) {
	a := r175App(t)
	for _, body := range []string{
		`{"event":"lp_chip_render","reason":"render","source":"overview-matches","lost":0,"images_recreated":0,"ranked_matches":5,"with_delta":5,"chips":5}`,
		`{"event":"manual_refresh_client","reason":"complete","overview_requests":1,"canceled_requests":0,"busy_ms":2500,"label_changes":2}`,
		`{"event":"match_filter_view","reason":"render","source":"client-fallback","filter":"solo","count":3,"latest_game_id":5,"latest_ended_at":8,"all_list_count":3,"missing_count":0}`,
		`{"event":"overview_subpage_open","reason":"open","page":"champion-table","from":"dialog","ok":true}`,
	} {
		w := httptest.NewRecorder()
		a.handleClientDiagnostic(w, httptest.NewRequest("POST", "/api/diagnostics/client", strings.NewReader(body)))
		if w.Code != 204 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	for _, name := range []string{"lp_chip_render", "manual_refresh_client", "match_filter_view", "overview_subpage_open"} {
		if len(readDiagnosticEvents(t, a.storage, name)) != 1 {
			t.Fatal("diagnostic missing", name)
		}
	}
}

func TestR283SubpageDiagnosticRejectsUnknownActions(t *testing.T) {
	a := r175App(t)
	for _, body := range []string{
		`{"event":"overview_subpage_open","reason":"open","page":"private-player-name","from":"dialog","ok":true}`,
		`{"event":"overview_subpage_open","reason":"open","page":"masteries","from":"private-tab-name","ok":true}`,
	} {
		w := httptest.NewRecorder()
		a.handleClientDiagnostic(w, httptest.NewRequest("POST", "/api/diagnostics/client", strings.NewReader(body)))
		if w.Code != 400 {
			t.Fatal("unbounded action accepted", w.Code)
		}
	}
	if len(readDiagnosticEvents(t, a.storage, "overview_subpage_open")) != 0 {
		t.Fatal("invalid action was recorded")
	}
}
