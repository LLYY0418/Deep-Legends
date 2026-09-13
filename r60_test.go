package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestR60FacadeLCUEventsBroadcastOnlyForFacadePrefixes(t *testing.T) {
	tests := []struct {
		uri  string
		want bool
	}{
		{uri: "/lol-challenges/v1/summary-player-data/local-player", want: true},
		{uri: "/lol-challenges/v1/summary-player-data/local-player/title", want: true},
		{uri: "/LOL-CHAT/V1/ME", want: true},
		{uri: "/lol-regalia/v2/current-summoner/regalia/banner", want: true},
		{uri: "/lol-chat/v1/medley", want: false},
		{uri: "/lol-rewards/v1/grants/1", want: false},
	}
	for _, test := range tests {
		t.Run(test.uri, func(t *testing.T) {
			events := make(chan string, 1)
			a := &app{eventSubscribers: map[chan string]struct{}{events: {}}}
			if got := a.handleFacadeLCUEvent(LCUEvent{URI: test.uri}); got != test.want {
				t.Fatalf("handleFacadeLCUEvent(%q) = %v, want %v", test.uri, got, test.want)
			}
			select {
			case event := <-events:
				if !test.want {
					t.Fatalf("unrelated URI broadcast %q", event)
				}
				if event != "facade:changed" {
					t.Fatalf("broadcast = %q, want facade:changed", event)
				}
			case <-time.After(20 * time.Millisecond):
				if test.want {
					t.Fatal("facade URI did not broadcast facade:changed")
				}
			}
		})
	}
}

func TestR60FacadeSelectedChallengesUseCachedCompactCatalog(t *testing.T) {
	catalog := make(map[string]any, 399)
	for id := 1; id <= 399; id++ {
		catalog[fmt.Sprint(id)] = map[string]any{"name": fmt.Sprintf("目录勋章 %d", id), "iconPath": fmt.Sprintf("/challenge/%d.png", id), "description": "不应进入 facadeState"}
	}
	var catalogReads atomic.Int32
	a, client, closeServer := newR60FacadeFixture(t,
		`{"title":{"name":"新头衔"},"selectedChallengesString":"1,2,3","topChallenges":[{"id":91,"name":"不应优先"}]}`,
		func(w http.ResponseWriter) {
			catalogReads.Add(1)
			_ = json.NewEncoder(w).Encode(catalog)
		},
	)
	defer closeServer()

	first := a.loadFacadeState(context.Background())
	second := a.loadFacadeState(context.Background())
	for _, state := range []facadeState{first, second} {
		if got := facadeChallengeNames(state.Challenges); strings.Join(got, ",") != "目录勋章 1,目录勋章 2,目录勋章 3" {
			t.Fatalf("selected challenges = %v", got)
		}
		if !state.ChallengesReady {
			t.Fatal("non-empty challenge summary marked not ready")
		}
		payload, err := json.Marshal(state)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(payload), "目录勋章 399") || strings.Contains(string(payload), "不应进入 facadeState") {
			t.Fatalf("facadeState leaked the full challenge catalog: %s", payload)
		}
	}
	if catalogReads.Load() != 1 {
		t.Fatalf("catalog reads = %d, want one per LCU connection", catalogReads.Load())
	}
	a.clearFacadeChallengeCatalog(client)
	_ = a.loadFacadeState(context.Background())
	if catalogReads.Load() != 2 {
		t.Fatalf("catalog reads after invalidation = %d, want 2", catalogReads.Load())
	}
	a.markDisconnected("test disconnect")
	a.facadeChallengeCatalogMu.Lock()
	cachedClient := a.facadeChallengeCatalogClient
	cachedEntries := len(a.facadeChallengeCatalog)
	a.facadeChallengeCatalogMu.Unlock()
	if cachedClient != nil || cachedEntries != 0 {
		t.Fatalf("disconnect retained facade catalog: client=%p entries=%d", cachedClient, cachedEntries)
	}
}

func TestR60FacadeChallengesFallbackAndEmptyStates(t *testing.T) {
	t.Run("malformed selected uses topChallenges when catalog is 404", func(t *testing.T) {
		a, _, closeServer := newR60FacadeFixture(t,
			`{"title":null,"selectedChallengesString":"1,broken,3","topChallenges":[{"id":11,"name":"坚韧不拔"},{"challengeId":12,"name":"完美配合","iconPath":"/top/12.png"},{"id":13,"name":"收藏大师"}]}`,
			func(w http.ResponseWriter) { http.Error(w, `{"message":"not found"}`, http.StatusNotFound) },
		)
		defer closeServer()
		state := a.loadFacadeState(context.Background())
		if !state.Connected {
			t.Fatalf("catalog 404 blocked facade state: %#v", state)
		}
		if got := strings.Join(facadeChallengeNames(state.Challenges), ","); got != "坚韧不拔,完美配合,收藏大师" {
			t.Fatalf("topChallenges fallback = %q", got)
		}
	})

	t.Run("missing startup summary stays empty and requests one retry", func(t *testing.T) {
		a, _, closeServer := newR60FacadeFixture(t,
			`{"title":null,"selectedChallengesString":"","topChallenges":[]}`,
			func(w http.ResponseWriter) { _, _ = w.Write([]byte(`{}`)) },
		)
		defer closeServer()
		state := a.loadFacadeState(context.Background())
		if len(state.Challenges) != 0 {
			t.Fatalf("empty summary fabricated challenges: %#v", state.Challenges)
		}
		if state.ChallengesReady {
			t.Fatal("empty 200 challenge summary was cached as ready")
		}
	})
}

func newR60FacadeFixture(t *testing.T, summary string, catalogHandler func(http.ResponseWriter)) (*app, *LCUClient, func()) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/lol-summoner/v1/current-summoner/summoner-profile":
			_, _ = w.Write([]byte(`{"backgroundSkinId":1}`))
		case "/lol-chat/v1/me":
			_, _ = w.Write([]byte(`{"availability":"chat","statusMessage":"","lol":{}}`))
		case "/lol-regalia/v2/current-summoner/regalia":
			_, _ = w.Write([]byte(`{}`))
		case "/lol-challenges/v1/summary-player-data/local-player":
			_, _ = w.Write([]byte(summary))
		case "/lol-challenges/v1/challenges/local-player":
			catalogHandler(w)
		default:
			http.NotFound(w, r)
		}
	}))
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	a := &app{connected: true, lcu: client, summoner: Summoner{SummonerID: 1}}
	return a, client, server.Close
}

func facadeChallengeNames(challenges []facadeChallenge) []string {
	names := make([]string, 0, len(challenges))
	for _, challenge := range challenges {
		names = append(names, challenge.Name)
	}
	return names
}
