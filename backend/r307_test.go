package main

import (
	"context"
	"encoding/json"
	"net/http"
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

func TestR307ProgressionThreeGroupsIdentityDeltaAndEventPrivacy(t *testing.T) {
	a := r175App(t)
	after := false
	calls := map[string]int{}
	c := &LCUClient{baseURL: "https://fixture", token: "fixture"}
	c.http = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls[r.URL.Path]++
		if strings.HasSuffix(r.URL.Path, "configuration") {
			if after {
				return r206RelayResponse(200, []byte(`{"groups":[{"groupId":"gamma"},{"groupId":"beta"},{"groupId":"alpha"}]}`)), nil
			}
			return r206RelayResponse(200, []byte(`{"groups":[{"groupId":"alpha"},{"groupId":"beta"},{"groupId":"gamma"}]}`)), nil
		}
		value := 92742
		if after && strings.Contains(r.URL.Path, "/beta") {
			value = 92842
		}
		raw, _ := json.Marshal(map[string]any{"counters": []map[string]any{{"counterValue": value}}, "milestones": []map[string]any{{"triggerValue": 100}}, "gameName": "SECRET"})
		return r206RelayResponse(200, raw), nil
	})}
	ops := []lcuHelpOperation{{Name: "progression group configuration", Method: "GET", Path: "/lol-progression/v1/groups/configuration"}, {Name: "progression group instance-data", Method: "GET", Path: "/lol-progression/v1/groups/{id}/instance-data"}, {Name: "cherry inventory", Method: "GET", Path: "/lol-inventory/v1/cherry-inventory"}}
	before := a.readArenaProgressionSnapshot(context.Background(), c, ops, Summoner{})
	after = true
	next := a.readArenaProgressionSnapshot(context.Background(), c, ops, Summoner{}, before)
	groups := arenaProgressionGroupDiff(before, next)
	if len(groups) != 1 || groups[0]["group_id"] != "beta" {
		t.Fatalf("groups=%v", groups)
	}
	changes := groups[0]["changes"].(map[string]arenaProgressionDelta)
	if len(changes) != 1 || changes["counters[0].counterValue"].Delta != 100 {
		t.Fatal(changes)
	}
	for _, id := range []string{"alpha", "beta", "gamma"} {
		if calls["/lol-progression/v1/groups/"+id+"/instance-data"] != 2 || calls["/lol-progression/v1/groups/"+id] != 2 {
			t.Fatal(calls)
		}
	}
	var events []map[string]any
	c.setDiagnosticObserver(func(e map[string]any) { events = append(events, e) })
	for _, uri := range []string{"/lol-progression/v1/groups/beta/instance-data", "/account-private/progression/v1/notifications/beta/12345"} {
		c.observeArenaProgressionEvent(LCUEvent{URI: uri, Data: json.RawMessage(`{"counterValue":92842,"milestone":{"triggerValue":100},"message":"SECRET","playerId":12345}`)})
	}
	if len(events) != 2 {
		t.Fatal(events)
	}
	for _, e := range events {
		raw := stringMustR307(e)
		if e["group_id"] != "beta" || strings.Contains(raw, "SECRET") || strings.Contains(raw, "account-private") || strings.Contains(raw, "12345") || strings.Contains(raw, "playerId") {
			t.Fatal(raw)
		}
		if !strings.Contains(raw, "message") {
			t.Fatal("payload key missing", raw)
		}
	}
}

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
