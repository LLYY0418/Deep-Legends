package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
)

func TestR58ClearTitleRetriesWithNullAfterEmptyString(t *testing.T) {
	var calls atomic.Int32
	bodies := make(chan map[string]json.RawMessage, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/lol-challenges/v1/summary-player-data/local-player" {
			_, _ = w.Write([]byte(`{"title":{"itemId":77},"selectedChallengesString":"1,2,3"}`))
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == "/lol-chat/v1/me" {
			_, _ = w.Write([]byte(`{"lol":{"bannerIdSelected":"5"}}`))
			return
		}
		if r.Method != http.MethodPost || r.URL.Path != "/lol-challenges/v1/update-player-preferences/" {
			http.NotFound(w, r)
			return
		}
		var body map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		bodies <- body
		if calls.Add(1) == 1 {
			http.Error(w, `{"message":"empty string rejected"}`, http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	if err := (&app{}).applyFacadeAction(context.Background(), client, Summoner{}, facadeApplyRequest{Action: "clear-title"}); err != nil {
		t.Fatal(err)
	}
	first, second := <-bodies, <-bodies
	if string(first["title"]) != `""` || string(second["title"]) != "null" {
		t.Fatalf("clear-title bodies = first %s, second %s", first["title"], second["title"])
	}
	for index, body := range []map[string]json.RawMessage{first, second} {
		if string(body["challengeIds"]) != `[1,2,3]` || string(body["bannerAccent"]) != `"5"` {
			t.Fatalf("clear-title body %d did not preserve challenges/banner: %s", index+1, body)
		}
	}
}

func TestR58ClearTitleFailureDiagnosticIncludesOnlyHTTPStatus(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(root+"/logs", 0o700); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/lol-challenges/v1/summary-player-data/local-player" {
			_, _ = w.Write([]byte(`{"title":{"itemId":987654321},"selectedChallengesString":"1,2,3"}`))
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == "/lol-chat/v1/me" {
			_, _ = w.Write([]byte(`{"lol":{"bannerIdSelected":"5"}}`))
			return
		}
		http.Error(w, `{"message":"PRIVATE_UPSTREAM_BODY"}`, http.StatusUnprocessableEntity)
	}))
	t.Cleanup(server.Close)
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	a := &app{connected: true, lcu: client, storage: &localStore{root: root}}
	recorder := httptest.NewRecorder()
	a.handleFacadeApply(recorder, httptest.NewRequest(http.MethodPost, "/api/facade/apply", strings.NewReader(`{"action":"clear-title"}`)))
	if recorder.Code != http.StatusBadGateway {
		t.Fatalf("clear-title response = %d %q", recorder.Code, recorder.Body.String())
	}
	data, err := os.ReadFile(root + "/logs/diagnostics.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{`"event":"facade_apply"`, `"action":"clear-title"`, `"result":"failed"`, `"status_code":422`, `"title_restore":"not-set"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("clear-title diagnostic missing %s: %s", want, text)
		}
	}
	if strings.Contains(text, "PRIVATE_UPSTREAM_BODY") || strings.Contains(text, "/lol-challenges/") || strings.Contains(text, "987654321") || strings.Contains(text, `"challengeIds"`) {
		t.Fatalf("clear-title diagnostic leaked upstream data: %s", text)
	}
}
