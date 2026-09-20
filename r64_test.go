package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"
)

func TestR64FacadeEventBurstIsThrottled(t *testing.T) {
	events := make(chan string, 32)
	a := &app{eventSubscribers: map[chan string]struct{}{events: {}}}
	t.Cleanup(a.clearFacadeEventThrottle)
	for index := 0; index < 20; index++ {
		if !a.handleFacadeLCUEvent(LCUEvent{URI: "/lol-chat/v1/me"}) {
			t.Fatal("facade event was not recognized")
		}
	}
	time.Sleep(facadeEventThrottleInterval + 150*time.Millisecond)
	count := 0
	for {
		select {
		case event := <-events:
			if event != "facade:changed" {
				t.Fatalf("broadcast = %q", event)
			}
			count++
		default:
			if count < 1 || count > 2 {
				t.Fatalf("20 facade events produced %d broadcasts, want 1..2", count)
			}
			return
		}
	}
}

func TestR64ClearChallengesPreservesTitle(t *testing.T) {
	var posted map[string]json.RawMessage
	server := newR64PreferenceServer(t, func(w http.ResponseWriter, r *http.Request, body map[string]json.RawMessage) {
		posted = body
		w.WriteHeader(http.StatusNoContent)
	})
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	restore, err := (&app{}).applyFacadeActionResult(context.Background(), client, Summoner{}, facadeApplyRequest{Action: "clear-challenges"})
	if err != nil {
		t.Fatal(err)
	}
	if restore != "item-id-string" {
		t.Fatalf("title_restore = %q, want item-id-string", restore)
	}
	assertR64PreferenceBody(t, posted, []int64{}, "5")
	if string(posted["title"]) != `"77"` {
		t.Fatalf("clear-challenges title = %s, want string itemId 77", posted["title"])
	}
}

func TestR64ClearChallengesRefusesToDropUnsupportedTitle(t *testing.T) {
	bodies := make(chan map[string]json.RawMessage, 1)
	server := newR64PreferenceServer(t, func(w http.ResponseWriter, r *http.Request, body map[string]json.RawMessage) {
		bodies <- body
		http.Error(w, `{"message":"title itemId rejected"}`, http.StatusUnprocessableEntity)
	})
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	restore, err := (&app{}).applyFacadeActionResult(context.Background(), client, Summoner{}, facadeApplyRequest{Action: "clear-challenges"})
	if err == nil {
		t.Fatal("clear-challenges silently dropped the current title")
	}
	if restore != "refused" {
		t.Fatalf("title_restore = %q, want refused", restore)
	}
	body := <-bodies
	assertR64PreferenceBody(t, body, []int64{}, "5")
	if string(body["title"]) != `"77"` {
		t.Fatalf("clear-challenges title = %s, want string itemId 77", body["title"])
	}
}

func TestR64TitleRestoreValuesAreStaticEnums(t *testing.T) {
	for value, want := range map[string]string{
		"item-id-string": "item-id-string", "content-id": "content-id", "player-title-selected": "player-title-selected",
		"refused": "refused", "no-candidate": "no-candidate", "not-set": "not-set", "item-id": "not-set", "omitted": "not-set", "987654321": "not-set",
	} {
		if got := facadeTitleRestoreDiagnostic(value); got != want {
			t.Fatalf("facadeTitleRestoreDiagnostic(%q) = %q, want %q", value, got, want)
		}
	}
}

func newR64PreferenceServer(t *testing.T, post func(http.ResponseWriter, *http.Request, map[string]json.RawMessage)) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/lol-challenges/v1/summary-player-data/local-player":
			_, _ = w.Write([]byte(`{"title":{"itemId":77},"selectedChallengesString":"1,2,3"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/lol-chat/v1/me":
			_, _ = w.Write([]byte(`{"lol":{"bannerIdSelected":"5"}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/lol-challenges/v1/update-player-preferences/":
			var body map[string]json.RawMessage
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			post(w, r, body)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func assertR64PreferenceBody(t *testing.T, body map[string]json.RawMessage, challenges []int64, banner string) {
	t.Helper()
	var gotChallenges []int64
	if err := json.Unmarshal(body["challengeIds"], &gotChallenges); err != nil {
		t.Fatalf("challengeIds = %s: %v", body["challengeIds"], err)
	}
	if !reflect.DeepEqual(gotChallenges, challenges) {
		t.Fatalf("challengeIds = %v, want %v", gotChallenges, challenges)
	}
	var gotBanner string
	if err := json.Unmarshal(body["bannerAccent"], &gotBanner); err != nil {
		t.Fatalf("bannerAccent = %s: %v", body["bannerAccent"], err)
	}
	if gotBanner != banner {
		t.Fatalf("bannerAccent = %q, want %q", gotBanner, banner)
	}
}
