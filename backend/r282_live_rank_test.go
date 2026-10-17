package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestR282LiveRankSoloFirstAndFlexFallback(t *testing.T) {
	solo := &gameplayRank{QueueType: "RANKED_SOLO_5x5", Tier: "DIAMOND", Division: "II"}
	flex := &gameplayRank{QueueType: "RANKED_FLEX_SR", Tier: "MASTER", LeaguePoints: 132}
	if livePreferredRank(solo, flex) != solo || livePreferredRank(&gameplayRank{Tier: "UNRANKED"}, flex) != flex || livePreferredRank(nil, nil) != nil {
		t.Fatal("rank precedence lost")
	}
}
func TestR282LiveRankOneLookupPerGameBeyondNormalCacheTTL(t *testing.T) {
	var calls atomic.Int64
	c := &LCUClient{baseURL: "https://fixture", token: "test", platformProbe: true}
	c.http = &http.Client{Transport: r196RoundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/lol-ranked/v1/current-ranked-stats" {
			calls.Add(1)
			return r206RelayResponse(200, []byte(`{"queues":[{"queueType":"RANKED_SOLO_5x5","tier":"DIAMOND","division":"II"}]}`)), nil
		}
		return r206RelayResponse(404, []byte(`{}`)), nil
	})}
	a := &app{}
	ref := strings.Repeat("r", 48)
	for range 6 {
		rank := a.livePlayerRankScore(context.Background(), c, 777, ref, true, "", "")
		if len(rank.ranks) == 0 {
			t.Fatal("rank not loaded")
		}
		cache := a.ensureRankScores()
		cache.mu.Lock()
		for _, element := range cache.entries {
			item := element.Value.(rankScoreCacheItem)
			item.entry.at = time.Now().Add(-11 * time.Minute)
			element.Value = item
		}
		cache.mu.Unlock()
	}
	if calls.Load() != 1 {
		t.Fatalf("same-game ranked reads=%d", calls.Load())
	}
	a.invalidateRankPlayer(ref)
	a.livePlayerRankScore(context.Background(), c, 777, ref, true, "", "")
	if calls.Load() != 2 {
		t.Fatal("live-game cache ignored player invalidation")
	}
}

func TestR282ARAMAndMayhemUseExistingLiveRankPipeline(t *testing.T) {
	for _, queue := range []int{450, 2400} {
		t.Run(strconv.Itoa(queue), func(t *testing.T) {
			var calls atomic.Int64
			ref := strings.Repeat("r", 48)
			client := &LCUClient{baseURL: "https://fixture", token: "test", platformProbe: true}
			client.http = &http.Client{Transport: r196RoundTrip(func(r *http.Request) (*http.Response, error) {
				body := ""
				switch r.URL.Path {
				case "/lol-gameflow/v1/gameflow-phase":
					body = `"ChampSelect"`
				case "/lol-gameflow/v1/session":
					body = fmt.Sprintf(`{"gameData":{"gameId":777,"queue":{"id":%d,"mapId":12,"gameMode":"ARAM"},"teamOne":[],"teamTwo":[]},"map":{"id":12,"gameMode":"ARAM"}}`, queue)
				case "/lol-champ-select/v1/session":
					body = fmt.Sprintf(`{"localPlayerCellId":7,"myTeam":[{"cellId":7,"puuid":%q,"summonerId":123,"summonerName":"Player","championId":22}],"theirTeam":[],"actions":[]}`, ref)
				case "/lol-champ-select/v1/current-champion":
					body = `22`
				case "/lol-ranked/v1/current-ranked-stats":
					calls.Add(1)
					body = `{"queues":[{"queueType":"RANKED_SOLO_5x5","tier":"DIAMOND","division":"II"}]}`
				default:
					return r206RelayResponse(404, []byte(`{}`)), nil
				}
				return r206RelayResponse(200, []byte(body)), nil
			})}
			a := &app{connected: true, lcu: client, summoner: Summoner{SummonerID: 123, PUUID: ref}}
			for range 2 {
				rec := httptest.NewRecorder()
				a.handleGameplayLive(rec, httptest.NewRequest(http.MethodGet, "/api/gameplay/live", nil))
				var response gameplayLiveResponse
				if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
					t.Fatal(err)
				}
				if len(response.Players) != 1 || response.Players[0].Rank == nil || response.Players[0].Rank.Tier != "DIAMOND" {
					t.Fatalf("ARAM rank missing: %s", rec.Body.String())
				}
			}
			if calls.Load() != 1 {
				t.Fatalf("rank reads=%d", calls.Load())
			}
		})
	}
}

func TestR282LiveRankInvalidationRetriesCancelledLeader(t *testing.T) {
	var calls atomic.Int64
	entered := make(chan struct{})
	c := &LCUClient{baseURL: "https://fixture", token: "test", platformProbe: true}
	c.http = &http.Client{Transport: r196RoundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/lol-ranked/v1/current-ranked-stats" {
			return r206RelayResponse(404, []byte(`{}`)), nil
		}
		if calls.Add(1) == 1 {
			close(entered)
			<-r.Context().Done()
			return nil, r.Context().Err()
		}
		return r206RelayResponse(200, []byte(`{"queues":[{"queueType":"RANKED_SOLO_5x5","tier":"DIAMOND","division":"II"}]}`)), nil
	})}
	a := &app{}
	ref := strings.Repeat("r", 48)
	result := make(chan rankScoreEntry, 1)
	go func() { result <- a.livePlayerRankScore(context.Background(), c, 778, ref, true, "", "") }()
	<-entered
	a.invalidateRankPlayer(ref)
	if got := <-result; len(got.ranks) == 0 || calls.Load() != 2 {
		t.Fatalf("invalidated leader returned stale/empty: ranks=%d calls=%d", len(got.ranks), calls.Load())
	}
}
