package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
)

const r69EmptyTitleSummary = `{"title":{"backgroundImagePath":"","challengeTitleData":null,"contentId":"","iconPath":"","isPermanentTitle":false,"itemId":0,"name":"","purchaseDate":"","titleAcquisitionName":"","titleAcquisitionType":"","titleRequirementDescription":""},"selectedChallengesString":"1,2,3"}`

func r69EmptyTitleObject() map[string]any {
	return map[string]any{
		"backgroundImagePath": "", "challengeTitleData": nil, "contentId": "", "iconPath": "",
		"isPermanentTitle": false, "itemId": float64(0), "name": "", "purchaseDate": "",
		"titleAcquisitionName": "", "titleAcquisitionType": "", "titleRequirementDescription": "",
	}
}

func TestR69FacadeTitleRestorePlanUsesValuesNotObjectKeys(t *testing.T) {
	if count := len(r69EmptyTitleObject()); count != 11 {
		t.Fatalf("empty title fixture has %d keys, want 11", count)
	}
	tests := []struct {
		name           string
		mutate         func(map[string]any)
		chat           map[string]any
		wantHasTitle   bool
		wantCandidates []facadeTitleRestoreCandidate
	}{
		{name: "eleven empty keys", wantHasTitle: false},
		{name: "itemId", mutate: func(title map[string]any) { title["itemId"] = float64(12345) }, wantHasTitle: true, wantCandidates: []facadeTitleRestoreCandidate{{Value: "12345", Diagnostic: "item-id-string"}}},
		{name: "contentId", mutate: func(title map[string]any) { title["contentId"] = "CONTENT_ID" }, wantHasTitle: true, wantCandidates: []facadeTitleRestoreCandidate{{Value: "CONTENT_ID", Diagnostic: "content-id"}}},
		{name: "chat playerTitleSelected", chat: map[string]any{"playerTitleSelected": "PLAYER_TITLE"}, wantHasTitle: true, wantCandidates: []facadeTitleRestoreCandidate{{Value: "PLAYER_TITLE", Diagnostic: "player-title-selected"}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			title := r69EmptyTitleObject()
			if test.mutate != nil {
				test.mutate(title)
			}
			summary := map[string]any{"title": title}
			candidates, hasTitle := facadeTitleRestorePlan(summary, test.chat)
			if hasTitle != test.wantHasTitle || len(candidates) != len(test.wantCandidates) || (len(candidates) > 0 && !reflect.DeepEqual(candidates, test.wantCandidates)) {
				t.Fatalf("plan = (%#v, %v), want (%#v, %v)", candidates, hasTitle, test.wantCandidates, test.wantHasTitle)
			}
			if test.chat == nil && facadeSummaryHasTitle(summary) != test.wantHasTitle {
				t.Fatalf("facadeSummaryHasTitle = %v, want %v", facadeSummaryHasTitle(summary), test.wantHasTitle)
			}
		})
	}
	if got := projectFacadeTitle(r69EmptyTitleObject()); got != nil {
		t.Fatalf("projectFacadeTitle(empty object) = %#v, want nil", got)
	}
}

func TestR69NameOnlyTitleReturnsNoCandidateWithoutPosting(t *testing.T) {
	postCount := 0
	server := newR66PreferenceServer(t,
		`{"title":{"name":"当前头衔","itemId":0,"contentId":""},"selectedChallengesString":"1,2,3"}`,
		`{"lol":{"bannerIdSelected":"5","playerTitleSelected":""}}`,
		func(w http.ResponseWriter, _ map[string]json.RawMessage) {
			postCount++
			w.WriteHeader(http.StatusNoContent)
		},
	)
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	result, err := (&app{}).applyFacadeActionResultDetails(context.Background(), client, Summoner{}, facadeApplyRequest{Action: "clear-challenges"})
	if err == nil || !strings.Contains(err.Error(), "缺少可回填的标识") {
		t.Fatalf("error = %v, want explicit no-candidate error", err)
	}
	var refused *facadeTitleRestoreRefusedError
	if errors.As(err, &refused) {
		t.Fatalf("no-candidate error was misclassified as refused: %v", err)
	}
	var noCandidate *facadeTitleRestoreNoCandidateError
	if !errors.As(err, &noCandidate) {
		t.Fatalf("error type = %T, want facadeTitleRestoreNoCandidateError", err)
	}
	if result.TitleRestore != "no-candidate" || !result.TitleHasTitle || result.TitleCandidateCount != 0 || len(result.TitleAttemptStatus) != 0 {
		t.Fatalf("result = %#v, want no-candidate with zero attempts", result)
	}
	if postCount != 0 {
		t.Fatalf("POST count = %d, want zero", postCount)
	}
}

func TestR69FacadeApplyDiagnosticShapesReachJSONL(t *testing.T) {
	tests := []struct {
		name               string
		summary            string
		postStatus         int
		wantHTTP           int
		wantResult         string
		wantRestore        string
		wantHasTitle       bool
		wantCandidateCount int
		wantAttemptStatus  []any
		wantPosts          int
	}{
		{name: "no title writes normally", summary: r69EmptyTitleSummary, postStatus: http.StatusNoContent, wantHTTP: http.StatusOK, wantResult: "ok", wantRestore: "not-set", wantAttemptStatus: []any{}, wantPosts: 1},
		{name: "title without identifier stops before post", summary: `{"title":{"name":"当前头衔","itemId":0,"contentId":""},"selectedChallengesString":"1,2,3"}`, postStatus: http.StatusNoContent, wantHTTP: http.StatusBadGateway, wantResult: "failed", wantRestore: "no-candidate", wantHasTitle: true, wantAttemptStatus: []any{}},
		{name: "real candidate refused after attempt", summary: `{"title":{"name":"当前头衔","itemId":77,"contentId":""},"selectedChallengesString":"1,2,3"}`, postStatus: http.StatusBadRequest, wantHTTP: http.StatusBadGateway, wantResult: "failed", wantRestore: "refused", wantHasTitle: true, wantCandidateCount: 1, wantAttemptStatus: []any{float64(http.StatusBadRequest)}, wantPosts: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			a, postCount := newR69FacadeDiagnosticFixture(t, test.summary, test.postStatus)
			recorder := httptest.NewRecorder()
			a.handleFacadeApply(recorder, httptest.NewRequest(http.MethodPost, "/api/facade/apply", strings.NewReader(`{"action":"clear-challenges"}`)))
			if recorder.Code != test.wantHTTP {
				t.Fatalf("response = %d %q, want %d", recorder.Code, recorder.Body.String(), test.wantHTTP)
			}
			if *postCount != test.wantPosts {
				t.Fatalf("POST count = %d, want %d", *postCount, test.wantPosts)
			}
			event, raw := r69ReadDiagnosticEvent(t, a.storage, "facade_apply")
			t.Logf("facade_apply_json=%s", raw)
			if event["result"] != test.wantResult || event["title_restore"] != test.wantRestore || event["title_has_title"] != test.wantHasTitle {
				t.Fatalf("facade_apply = %#v", event)
			}
			if event["title_attempt_count"] != float64(len(test.wantAttemptStatus)) || event["title_candidate_count"] != float64(test.wantCandidateCount) {
				t.Fatalf("facade_apply counts = %#v", event)
			}
			if got, ok := event["title_attempt_status"].([]any); !ok || !reflect.DeepEqual(got, test.wantAttemptStatus) {
				t.Fatalf("title_attempt_status = %#v, want %#v", event["title_attempt_status"], test.wantAttemptStatus)
			}
			for _, forbidden := range []string{"当前头衔", `"itemId"`, `"contentId"`, `"title"`} {
				if strings.Contains(raw, forbidden) {
					t.Fatalf("facade_apply leaked %q: %s", forbidden, raw)
				}
			}
		})
	}
}

func TestR69FacadeLoadCostWritesDurationsAndTrigger(t *testing.T) {
	a, _, closeServer := newR60FacadeFixture(t, r69EmptyTitleSummary, func(w http.ResponseWriter) { _, _ = w.Write([]byte(`{}`)) })
	defer closeServer()
	root := t.TempDir()
	if err := os.MkdirAll(root+"/logs", 0o700); err != nil {
		t.Fatal(err)
	}
	a.storage = &localStore{root: root}
	a.loadFacadeStateTriggered(context.Background(), "sse")
	event, _ := r69ReadDiagnosticEvent(t, a.storage, "facade_load_cost")
	if event["trigger"] != "sse" {
		t.Fatalf("trigger = %#v, want sse", event["trigger"])
	}
	for _, field := range []string{"chat_ms", "challenges_ms", "catalog_ms", "total_ms"} {
		value, ok := event[field].(float64)
		if !ok || value < 0 {
			t.Fatalf("%s = %#v, want non-negative duration", field, event[field])
		}
	}
	for input, want := range map[string]string{"manual": "manual", "SSE": "sse", "poll": "poll", "": "poll", "private": "poll"} {
		if got := facadeLoadTrigger(input); got != want {
			t.Fatalf("facadeLoadTrigger(%q) = %q, want %q", input, got, want)
		}
	}
}

func newR69FacadeDiagnosticFixture(t *testing.T, summary string, postStatus int) (*app, *int) {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(root+"/logs", 0o700); err != nil {
		t.Fatal(err)
	}
	postCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/lol-summoner/v1/current-summoner/summoner-profile":
			_, _ = w.Write([]byte(`{"backgroundSkinId":1}`))
		case r.Method == http.MethodGet && r.URL.Path == "/lol-chat/v1/me":
			_, _ = w.Write([]byte(`{"availability":"chat","statusMessage":"","lol":{"bannerIdSelected":"5","playerTitleSelected":""}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/lol-regalia/v2/current-summoner/regalia":
			_, _ = w.Write([]byte(`{}`))
		case r.Method == http.MethodGet && r.URL.Path == "/lol-challenges/v1/summary-player-data/local-player":
			_, _ = w.Write([]byte(summary))
		case r.Method == http.MethodGet && r.URL.Path == "/lol-challenges/v1/challenges/local-player":
			_, _ = w.Write([]byte(`{}`))
		case r.Method == http.MethodPost && r.URL.Path == "/lol-challenges/v1/update-player-preferences/":
			postCount++
			if postStatus >= http.StatusBadRequest {
				http.Error(w, `{"message":"rejected"}`, postStatus)
				return
			}
			w.WriteHeader(postStatus)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	a := &app{connected: true, lcu: client, summoner: Summoner{SummonerID: 1}, storage: &localStore{root: root}}
	return a, &postCount
}

func r69ReadDiagnosticEvent(t *testing.T, store *localStore, eventName string) (map[string]any, string) {
	t.Helper()
	data, err := store.readDiagnosticLog()
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var event map[string]any
		if json.Unmarshal([]byte(line), &event) == nil && event["event"] == eventName {
			return event, line
		}
	}
	t.Fatalf("diagnostic event %q missing: %s", eventName, data)
	return nil, ""
}
