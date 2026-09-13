package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
)

var r66PreferenceActions = []struct {
	name       string
	action     string
	challenges []int64
	banner     string
}{
	{name: "clear-challenges", action: "clear-challenges", challenges: []int64{}, banner: "5"},
	{name: "previous-banner", action: "previous-banner", challenges: []int64{1, 2, 3}, banner: "2"},
}

func TestR66TitleRestoreCandidatesAreTriedInOrder(t *testing.T) {
	tests := []struct {
		name           string
		accept         string
		wantRestore    string
		wantStatuses   []int
		wantBodyCount  int
		rejectStatuses []int
	}{
		{name: "string itemId", accept: "77", wantRestore: "item-id-string", wantBodyCount: 1},
		{name: "contentId", accept: "PRIVATE_CONTENT_ID", wantRestore: "content-id", wantStatuses: []int{http.StatusBadRequest}, wantBodyCount: 2, rejectStatuses: []int{http.StatusBadRequest}},
		{name: "playerTitleSelected", accept: "PRIVATE_PLAYER_TITLE", wantRestore: "player-title-selected", wantStatuses: []int{http.StatusBadRequest, http.StatusUnprocessableEntity}, wantBodyCount: 3, rejectStatuses: []int{http.StatusBadRequest, http.StatusUnprocessableEntity}},
	}
	for _, action := range r66PreferenceActions {
		for _, test := range tests {
			t.Run(action.name+"/"+test.name, func(t *testing.T) {
				bodies := make([]map[string]json.RawMessage, 0, 3)
				server := newR66PreferenceServer(t,
					`{"title":{"itemId":77,"contentId":"PRIVATE_CONTENT_ID","name":"当前头衔"},"selectedChallengesString":"1,2,3"}`,
					`{"lol":{"bannerIdSelected":"5","playerTitleSelected":"PRIVATE_PLAYER_TITLE"}}`,
					func(w http.ResponseWriter, body map[string]json.RawMessage) {
						bodies = append(bodies, body)
						var title any
						if err := json.Unmarshal(body["title"], &title); err != nil {
							t.Errorf("title = %s: %v", body["title"], err)
						}
						if value, ok := title.(string); ok && value == test.accept {
							w.WriteHeader(http.StatusNoContent)
							return
						}
						status := http.StatusBadRequest
						if index := len(bodies) - 1; index < len(test.rejectStatuses) {
							status = test.rejectStatuses[index]
						}
						http.Error(w, `{"message":"title candidate rejected"}`, status)
					},
				)
				client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
				result, err := (&app{}).applyFacadeActionResultDetails(context.Background(), client, Summoner{}, facadeApplyRequest{Action: action.action})
				if err != nil {
					t.Fatal(err)
				}
				if result.TitleRestore != test.wantRestore || !reflect.DeepEqual(result.TitleAttemptStatus, test.wantStatuses) {
					t.Fatalf("result = %#v, want restore %q statuses %v", result, test.wantRestore, test.wantStatuses)
				}
				if len(bodies) != test.wantBodyCount {
					t.Fatalf("POST count = %d, want %d", len(bodies), test.wantBodyCount)
				}
				for index, body := range bodies {
					assertR64PreferenceBody(t, body, action.challenges, action.banner)
					var title string
					if err := json.Unmarshal(body["title"], &title); err != nil || title == "" {
						t.Fatalf("POST %d title = %s, want non-empty string", index+1, body["title"])
					}
				}
				var finalTitle string
				if err := json.Unmarshal(bodies[len(bodies)-1]["title"], &finalTitle); err != nil || finalTitle != test.accept {
					t.Fatalf("final title = %q (%v), want %q", finalTitle, err, test.accept)
				}
			})
		}
	}
}

func TestR66TitleRestoreRefusalNeverPostsWithoutTitle(t *testing.T) {
	for _, action := range r66PreferenceActions {
		t.Run(action.name, func(t *testing.T) {
			bodies := make([]map[string]json.RawMessage, 0, 3)
			statuses := []int{http.StatusBadRequest, http.StatusUnprocessableEntity, http.StatusConflict}
			server := newR66PreferenceServer(t,
				`{"title":{"itemId":77,"contentId":"PRIVATE_CONTENT_ID","name":"当前头衔"},"selectedChallengesString":"1,2,3"}`,
				`{"lol":{"bannerIdSelected":"5","playerTitleSelected":"PRIVATE_PLAYER_TITLE"}}`,
				func(w http.ResponseWriter, body map[string]json.RawMessage) {
					bodies = append(bodies, body)
					http.Error(w, `{"message":"all title candidates rejected"}`, statuses[len(bodies)-1])
				},
			)
			client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
			result, err := (&app{}).applyFacadeActionResultDetails(context.Background(), client, Summoner{}, facadeApplyRequest{Action: action.action})
			if err == nil || !strings.Contains(err.Error(), "无法在保留头衔的前提下完成操作") {
				t.Fatalf("error = %v, want explicit preservation refusal", err)
			}
			if result.TitleRestore != "refused" || !result.TitleHasTitle || result.TitleCandidateCount != len(statuses) || !reflect.DeepEqual(result.TitleAttemptStatus, statuses) {
				t.Fatalf("result = %#v, want refused with %v", result, statuses)
			}
			if len(bodies) != 3 {
				t.Fatalf("POST count = %d, want exactly three title candidates", len(bodies))
			}
			for index, body := range bodies {
				assertR64PreferenceBody(t, body, action.challenges, action.banner)
				if _, present := body["title"]; !present {
					t.Fatalf("POST %d omitted title after a title was detected: %s", index+1, body)
				}
			}
		})
	}
}

func TestR66NoCurrentTitleMayPostPreferencesWithoutTitle(t *testing.T) {
	for _, action := range r66PreferenceActions {
		t.Run(action.name, func(t *testing.T) {
			var bodies []map[string]json.RawMessage
			server := newR66PreferenceServer(t,
				`{"title":null,"selectedChallengesString":"1,2,3"}`,
				`{"lol":{"bannerIdSelected":"5","playerTitleSelected":""}}`,
				func(w http.ResponseWriter, body map[string]json.RawMessage) {
					bodies = append(bodies, body)
					if _, present := body["title"]; present {
						http.Error(w, `{"message":"unexpected title"}`, http.StatusBadRequest)
						return
					}
					w.WriteHeader(http.StatusNoContent)
				},
			)
			client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
			result, err := (&app{}).applyFacadeActionResultDetails(context.Background(), client, Summoner{}, facadeApplyRequest{Action: action.action})
			if err != nil {
				t.Fatal(err)
			}
			if result.TitleRestore != "not-set" || result.TitleHasTitle || result.TitleCandidateCount != 0 || len(result.TitleAttemptStatus) != 0 {
				t.Fatalf("result = %#v, want harmless not-set", result)
			}
			if len(bodies) != 1 {
				t.Fatalf("POST count = %d, want 1", len(bodies))
			}
			assertR64PreferenceBody(t, bodies[0], action.challenges, action.banner)
		})
	}
}

func TestR66FacadeApplyLogsIntermediateStatusesWithoutCandidateValues(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(root+"/logs", 0o700); err != nil {
		t.Fatal(err)
	}
	bodies := make([]map[string]json.RawMessage, 0, 3)
	server := newR66PreferenceServer(t,
		`{"title":{"itemId":987654321,"contentId":"PRIVATE_CONTENT_ID","name":"当前头衔"},"selectedChallengesString":"1,2,3"}`,
		`{"availability":"chat","statusMessage":"","lol":{"bannerIdSelected":"5","playerTitleSelected":"PRIVATE_PLAYER_TITLE"}}`,
		func(w http.ResponseWriter, body map[string]json.RawMessage) {
			bodies = append(bodies, body)
			if len(bodies) == 1 {
				http.Error(w, `{"message":"first rejected"}`, http.StatusBadRequest)
				return
			}
			if len(bodies) == 2 {
				http.Error(w, `{"message":"second rejected"}`, http.StatusUnprocessableEntity)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		},
	)
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	a := &app{connected: true, lcu: client, summoner: Summoner{SummonerID: 1}, storage: trackTestStore(t, &localStore{root: root})}
	recorder := httptest.NewRecorder()
	a.handleFacadeApply(recorder, httptest.NewRequest(http.MethodPost, "/api/facade/apply", strings.NewReader(`{"action":"clear-challenges"}`)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("response = %d %q", recorder.Code, recorder.Body.String())
	}
	data, err := a.storage.readDiagnosticLog()
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{`"event":"facade_apply"`, `"action":"clear-challenges"`, `"result":"ok"`, `"title_restore":"player-title-selected"`, `"title_attempt_status":[400,422]`} {
		if !strings.Contains(text, want) {
			t.Fatalf("facade diagnostic missing %s: %s", want, text)
		}
	}
	for _, forbidden := range []string{"987654321", "PRIVATE_CONTENT_ID", "PRIVATE_PLAYER_TITLE", `"challengeIds"`} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("facade diagnostic leaked title candidate %q: %s", forbidden, text)
		}
	}
}

func TestR66LCURequestDiagnosticsIncludePreferencePOSTWithoutBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/lol-challenges/v1/update-player-preferences/" {
			http.NotFound(w, r)
			return
		}
		http.Error(w, `{"message":"PRIVATE_RESPONSE_BODY"}`, http.StatusUnprocessableEntity)
	}))
	t.Cleanup(server.Close)
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	var events []map[string]any
	client.setDiagnosticObserver(func(event map[string]any) { events = append(events, event) })
	err := client.RequestJSON(context.Background(), http.MethodPost, "/lol-challenges/v1/update-player-preferences/", map[string]any{"title": "PRIVATE_REQUEST_TITLE"}, nil)
	if !facadeHTTP4xx(err) {
		t.Fatalf("POST error = %v, want recorded 4xx", err)
	}
	client.flushRequestDiagnostics()
	if len(events) != 1 {
		t.Fatalf("lcu_request events = %#v", events)
	}
	event := events[0]
	statuses, _ := event["http_status"].(map[string]int)
	if event["event"] != "lcu_request" || event["method"] != http.MethodPost || event["path"] != "/lol-challenges/v1/update-player-preferences/" || event["count"] != 1 || statuses["422"] != 1 {
		t.Fatalf("lcu_request POST diagnostic = %#v", event)
	}
	payload, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"PRIVATE_REQUEST_TITLE", "PRIVATE_RESPONSE_BODY", `"body"`, `"title"`} {
		if strings.Contains(string(payload), forbidden) {
			t.Fatalf("lcu_request diagnostic leaked %q: %s", forbidden, payload)
		}
	}
}

func newR66PreferenceServer(t *testing.T, summary, chat string, post func(http.ResponseWriter, map[string]json.RawMessage)) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/lol-challenges/v1/summary-player-data/local-player":
			_, _ = w.Write([]byte(summary))
		case r.Method == http.MethodGet && r.URL.Path == "/lol-chat/v1/me":
			_, _ = w.Write([]byte(chat))
		case r.Method == http.MethodPost && r.URL.Path == "/lol-challenges/v1/update-player-preferences/":
			var body map[string]json.RawMessage
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			post(w, body)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server
}
