package main

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestR307WindowResizeTraceDiagnostics(t *testing.T) {
	a := &app{storage: newFlowDiagnosticStore(t)}
	body := `{"event":"window_resize_trace","reason":"summary","resizeTrace":{"drag":123,"sample":60,"bounds":{"x":0,"y":0,"width":858,"height":900},"contentBounds":{"width":858,"height":860},"minimum":[858,660],"zoom":1.1,"innerWidth":858,"innerHeight":860,"nextFrameMs":12,"longtaskCount":1,"longtaskMaxMs":49,"ignored":"private-name"}}`
	w := httptest.NewRecorder()
	a.handleClientDiagnostic(w, httptest.NewRequest("POST", "/api/diagnostics/client", strings.NewReader(body)))
	if w.Code != 204 {
		t.Fatalf("status=%d %s", w.Code, w.Body.String())
	}
	events := r175Events(t, a, "window_resize_trace")
	if len(events) != 1 {
		t.Fatalf("events=%v", events)
	}
	if strings.Contains(stringMustR307(events), "private-name") {
		t.Fatal("unknown text leaked")
	}
	w = httptest.NewRecorder()
	a.handleClientDiagnostic(w, httptest.NewRequest("POST", "/api/diagnostics/client", strings.NewReader(strings.Replace(body, `"sample":60`, `"sample":61`, 1))))
	if w.Code != 400 {
		t.Fatalf("invalid sample accepted: %d", w.Code)
	}
}

func stringMustR307(v any) string { b, _ := json.Marshal(v); return string(b) }

func TestR307TagSavedRecordsGamePhaseAndChampionRestore(t *testing.T) {
	a := r175App(t)
	a.gameplayFlow.phase = "InProgress"
	ref := a.registerGameplayReferenceDetails(gameplayReference{PlayerRef: "11111111-1111-4111-8111-111111111111", GameName: "fixture", TagVisibilityKnown: true}, "live")
	w := httptest.NewRecorder()
	a.handlePlayerTags(w, httptest.NewRequest("POST", "/api/player-tags", strings.NewReader(`{"op":"update","playerRef":"`+ref+`","tags":["preset-1"]}`)))
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	rows := r175Events(t, a, "player_tags_saved")
	if len(rows) != 1 || rows[0]["phase"] != "InProgress" {
		t.Fatal(rows)
	}
	for _, source := range []string{"session", "open-champion", "user"} {
		w = httptest.NewRecorder()
		a.handleClientDiagnostic(w, httptest.NewRequest("POST", "/api/diagnostics/client", strings.NewReader(`{"event":"champion_detail_restore","reason":"open","championRestore":{"source":"`+source+`","mode":"ranked","championId":164}}`)))
		if w.Code != 204 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	if len(r175Events(t, a, "champion_detail_restore")) != 3 {
		t.Fatal("restore diagnostics")
	}
}
