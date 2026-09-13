package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestR56ReadyCheckAcceptIsOncePerWindowAndResetsForNextRound(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/lol-matchmaking/v1/ready-check/accept" {
			requests.Add(1)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(server.Close)
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	runner := newWatchRunner(nil, nil)
	settings := defaultWatchSettings()
	settings.Rules.AutoAccept.Enabled = true
	settings.Rules.AutoAccept.DelayMS = 0
	runner.apply(settings)

	for index := 0; index < 10; index++ {
		runner.handleEvent(client, LCUEvent{URI: "/lol-matchmaking/v1/ready-check", Data: []byte(`{"state":"InProgress"}`)}, Summoner{})
		if index < 9 {
			time.Sleep(500 * time.Millisecond)
		}
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("accept requests during one ready-check = %d, want 1", got)
	}

	runner.handlePhase(client, "ChampSelect")
	runner.handleEvent(client, LCUEvent{URI: "/lol-matchmaking/v1/ready-check", Data: []byte(`{"state":"InProgress"}`)}, Summoner{})
	deadline := time.Now().Add(time.Second)
	for requests.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if got := requests.Load(); got != 2 {
		t.Fatalf("next ready-check accept requests = %d, want 2", got)
	}
}

func TestR56EventStreamDropReasonUsesFiniteCategories(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		wanted string
	}{
		{name: "read limit", err: errors.New("websocket: read limit exceeded"), wanted: "read-limit"},
		{name: "normal close", err: &websocket.CloseError{Code: websocket.CloseNormalClosure}, wanted: "close-normal"},
		{name: "abnormal close", err: &websocket.CloseError{Code: websocket.CloseAbnormalClosure}, wanted: "close-abnormal"},
		{name: "eof", err: io.EOF, wanted: "eof"},
		{name: "timeout", err: context.DeadlineExceeded, wanted: "timeout"},
		{name: "other", err: errors.New("private endpoint 127.0.0.1:1234 failed"), wanted: "other"},
	}
	allowed := map[string]bool{"read-limit": true, "close-normal": true, "close-abnormal": true, "timeout": true, "eof": true, "other": true}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := eventStreamDropReason(test.err); got != test.wanted || !allowed[got] {
				t.Fatalf("eventStreamDropReason() = %q, want %q", got, test.wanted)
			}
			if got := eventStreamDropReason(test.err); strings.Contains(got, "127.0.0.1") || strings.Contains(got, "1234") {
				t.Fatalf("drop reason leaked endpoint: %q", got)
			}
		})
	}
}

func TestR56FacadeIdentityShapeIsRecordedOnceWithoutValues(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(root+"/logs", 0o700); err != nil {
		t.Fatal(err)
	}
	a := &app{storage: &localStore{root: root}}
	client := &LCUClient{}
	chat := map[string]any{"availability": "chat", "statusMessage": "SECRET_NAME", "lol": map[string]any{"rankedLeagueTier": "DIAMOND", "title": "SECRET_TITLE", "playerTitleSelected": float64(321), "gameStatus": "inGame"}}
	regalia := map[string]any{"preferredCrestType": "SECRET_CREST", "selectedPrestigeCrest": 22}
	summary := []byte(`{"title":{"label":"SECRET_TITLE"},"topChallenges":[{"challengeId":123,"name":"SECRET_MEDAL"}],"selectedChallengesString":"987654321,876543210,765432109","categoryProgress":[{"name":"SECRET_CATEGORY"}]}`)
	catalog := []byte(`{"987654321":{"name":"SECRET_CATALOG_NAME","localizedName":"SECRET_LOCALIZED_NAME"}}`)
	if !a.claimFacadeIdentityShape(client) {
		t.Fatal("first facade shape claim was rejected")
	}
	a.recordFacadeIdentityShape(chat, regalia, nil, nil, summary, nil, catalog, nil)
	if a.claimFacadeIdentityShape(client) {
		t.Fatal("same connection produced a second facade shape claim")
	}
	data, err := os.ReadFile(root + "/logs/diagnostics.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(data), `"event":"facade_identity_shape"`) != 1 {
		t.Fatalf("facade shape diagnostic count = %d, data=%s", strings.Count(string(data), `"event":"facade_identity_shape"`), data)
	}
	for _, secret := range []string{"SECRET_NAME", "SECRET_TITLE", "SECRET_MEDAL", "SECRET_CREST", "SECRET_CATEGORY", "SECRET_CATALOG_NAME", "SECRET_LOCALIZED_NAME", "987654321", "876543210", "765432109"} {
		if strings.Contains(string(data), secret) {
			t.Fatalf("facade shape diagnostic leaked %q: %s", secret, data)
		}
	}
	if strings.Contains(string(data), `"player_title_selected"`) {
		t.Fatalf("facade shape diagnostic leaked a raw title id: %s", data)
	}
	for _, safeField := range []string{`"availability":"chat"`, `"game_status":"inGame"`, `"challenge_summary_title_type":"object"`, `"challenge_summary_top_challenges_length":1`, `"challenge_summary_selected_type":"string"`, `"challenge_summary_selected_segments":3`, `"challenge_summary_category_progress_length":1`, `"challenge_catalog_status":200`, `"challenge_catalog_result":"ok"`, `"challenge_catalog_type":"object"`, `"challenge_catalog_key_count":1`} {
		if !strings.Contains(string(data), safeField) {
			t.Fatalf("facade shape diagnostic missing %s: %s", safeField, data)
		}
	}
	if !strings.Contains(string(data), `"challenge_summary_title_keys":["label"]`) || !strings.Contains(string(data), `"challenge_summary_top_challenges_item_keys":["challengeId","name"]`) {
		t.Fatalf("facade shape diagnostic missing safe fields: %s", data)
	}
	if !strings.Contains(string(data), `"challenge_catalog_item_keys":["localizedName","name"]`) {
		t.Fatalf("facade catalog diagnostic missing safe item keys: %s", data)
	}
}

func TestR56FacadeSummaryLoadsForStateAndShapeDiagnosticIsOncePerConnection(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(root+"/logs", 0o700); err != nil {
		t.Fatal(err)
	}
	var summaryReads, catalogReads, titlesReads atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/lol-summoner/v1/current-summoner/summoner-profile":
			_, _ = w.Write([]byte(`{"backgroundSkinId":1}`))
		case "/lol-chat/v1/me":
			_, _ = w.Write([]byte(`{"availability":"chat","statusMessage":"PRIVATE_VALUE","lol":{"rankedLeagueTier":"DIAMOND"}}`))
		case "/lol-regalia/v2/current-summoner/regalia":
			_, _ = w.Write([]byte(`{"preferredCrestType":"PRIVATE_VALUE"}`))
		case "/lol-challenges/v1/summary-player-data/local-player":
			summaryReads.Add(1)
			_, _ = w.Write([]byte(`{"title":"峡谷先锋","topChallenges":[],"selectedChallengesString":"1,2,3","categoryProgress":[]}`))
		case "/lol-challenges/v1/challenges/local-player":
			catalogReads.Add(1)
			http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		case "/lol-challenges/v1/titles":
			titlesReads.Add(1)
			http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	a := &app{connected: true, lcu: client, storage: &localStore{root: root}, summoner: Summoner{SummonerID: 1}}
	first := a.loadFacadeState(context.Background())
	second := a.loadFacadeState(context.Background())
	if !first.Connected || first.Reason != "" || !second.Connected || second.Reason != "" {
		t.Fatalf("catalog 404 blocked facade state: first=%#v second=%#v", first, second)
	}
	if summaryReads.Load() != 2 || catalogReads.Load() != 1 || titlesReads.Load() != 0 {
		t.Fatalf("facade reads = summary %d, catalog %d, retired titles %d; want 2, 1 and 0", summaryReads.Load(), catalogReads.Load(), titlesReads.Load())
	}
	if first.ChallengeSummary["title"] != "峡谷先锋" || second.ChallengeSummary["title"] != "峡谷先锋" {
		t.Fatalf("challenge summary was not returned to the renderer: first=%#v second=%#v", first.ChallengeSummary, second.ChallengeSummary)
	}
	data, err := os.ReadFile(root + "/logs/diagnostics.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(data), `"event":"facade_identity_shape"`) != 1 || !strings.Contains(string(data), `"challenge_summary_result":"ok"`) || !strings.Contains(string(data), `"challenge_catalog_status":404`) || !strings.Contains(string(data), `"challenge_catalog_result":"not-found"`) || strings.Contains(string(data), `"challenge_titles_`) {
		t.Fatalf("candidate shape diagnostic = %s", data)
	}
}
