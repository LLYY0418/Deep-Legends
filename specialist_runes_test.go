package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func specialistTestProvider(transport http.RoundTripper) *riotProvider {
	champions := newChampionProvider()
	champions.clientMu.Lock()
	champions.client = &http.Client{Transport: transport}
	champions.clientMu.Unlock()
	return newRiotProvider(champions)
}

func specialistTestResponse(request *http.Request, status int, body string) (*http.Response, error) {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    request,
	}, nil
}

func specialistLeaderboardBody(players ...championTopPlayer) string {
	rows := make([]map[string]any, 0, len(players))
	for index, player := range players {
		rows = append(rows, map[string]any{
			"rank":     index + 1,
			"summoner": map[string]any{"game_name": player.Name, "tagline": player.Tagline},
			"league_stats": map[string]any{
				"tier_info": map[string]any{"tier": player.Tier, "lp": player.LP},
				"win_ratio": player.WinRate,
			},
			"most_champion_stat": map[string]any{"play": player.Games},
		})
	}
	payload, _ := json.Marshal(map[string]any{"data": rows})
	return `self.__next_f.push([1,` + strconv.Quote(string(payload)) + `])`
}

func specialistMatchBody(matchID, puuid string, championID int64, runeState string, win bool) string {
	primarySelections := []map[string]any{{"perk": 8005}, {"perk": 8009}, {"perk": 9104}, {"perk": 8014}}
	secondarySelections := []map[string]any{{"perk": 8143}, {"perk": 8135}}
	styles := []map[string]any{
		{"description": "primaryStyle", "style": 8000, "selections": primarySelections},
		{"description": "subStyle", "style": 8100, "selections": secondarySelections},
	}
	statPerks := map[string]any{"offense": 5008, "flex": 5008, "defense": 5001}
	switch runeState {
	case "missing-substyle":
		styles = styles[:1]
	case "too-few-perks":
		styles[0]["selections"] = primarySelections[:2]
		styles[1]["selections"] = secondarySelections[:1]
		statPerks = map[string]any{}
	}
	payload, _ := json.Marshal(map[string]any{
		"metadata": map[string]any{"matchId": matchID},
		"info": map[string]any{
			"gameId": 123, "gameCreation": int64(1_720_000_000_000),
			"participants": []map[string]any{{
				"puuid": puuid, "championId": championID, "win": win,
				"perks": map[string]any{"styles": styles, "statPerks": statPerks},
			}},
		},
	})
	return string(payload)
}

func TestSpecialistRunesUsesThirdMatchingGame(t *testing.T) {
	t.Setenv("RIOT_API_KEY", "RGAPI-test")
	player := championTopPlayer{Name: "Expert", Tagline: "KR1", Tier: "master", LP: "211", Games: "1,451", WinRate: 51}
	matchIDs := []string{"KR_1", "KR_2", "KR_3", "KR_4", "KR_5", "KR_6", "KR_7", "KR_8", "KR_9", "KR_10"}
	var riotRequests atomic.Int64
	var detailRequests atomic.Int64
	provider := specialistTestProvider(gameplayRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch {
		case request.URL.Host == opggPageHost:
			return specialistTestResponse(request, http.StatusOK, specialistLeaderboardBody(player))
		case strings.Contains(request.URL.Path, "/accounts/by-riot-id/"):
			riotRequests.Add(1)
			return specialistTestResponse(request, http.StatusOK, `{"puuid":"expert-puuid","gameName":"Expert","tagLine":"KR1"}`)
		case strings.HasSuffix(request.URL.Path, "/ids"):
			riotRequests.Add(1)
			body, _ := json.Marshal(matchIDs)
			return specialistTestResponse(request, http.StatusOK, string(body))
		case strings.Contains(request.URL.Path, "/lol/match/v5/matches/"):
			riotRequests.Add(1)
			detail := detailRequests.Add(1)
			championID := int64(1)
			if detail == 3 {
				championID = 64
			}
			matchID := strings.TrimPrefix(request.URL.Path, "/lol/match/v5/matches/")
			return specialistTestResponse(request, http.StatusOK, specialistMatchBody(matchID, "expert-puuid", championID, "complete", true))
		default:
			return specialistTestResponse(request, http.StatusNotFound, `{}`)
		}
	}))
	runes := provider.specialistRunes(context.Background(), 64, "leesin", "李青")
	if len(runes) != 1 {
		t.Fatalf("specialist runes = %#v", runes)
	}
	rune := runes[0]
	if rune.PlayerName != "Expert" || rune.TagLine != "KR1" || rune.Tier != "master" || rune.ChampionGames != 1451 || rune.PlayedAt != 1_720_000_000_000 || rune.Result != "win" || rune.Region != "kr" {
		t.Fatalf("specialist attribution = %#v", rune)
	}
	if rune.PrimaryStyleID != 8000 || rune.SubStyleID != 8100 || len(rune.SelectedPerkIDs) != 9 || detailRequests.Load() != 3 || riotRequests.Load() != 5 {
		t.Fatalf("rune=%#v detailRequests=%d riotRequests=%d", rune, detailRequests.Load(), riotRequests.Load())
	}
}

func TestSpecialistRunesDoesNotUseAnotherChampion(t *testing.T) {
	t.Setenv("RIOT_API_KEY", "RGAPI-test")
	player := championTopPlayer{Name: "Expert", Tagline: "KR1", Games: "200"}
	matchIDs := []string{"KR_1", "KR_2", "KR_3", "KR_4", "KR_5", "KR_6", "KR_7", "KR_8", "KR_9", "KR_10"}
	var details atomic.Int64
	provider := specialistTestProvider(gameplayRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch {
		case request.URL.Host == opggPageHost:
			return specialistTestResponse(request, http.StatusOK, specialistLeaderboardBody(player))
		case strings.Contains(request.URL.Path, "/accounts/by-riot-id/"):
			return specialistTestResponse(request, http.StatusOK, `{"puuid":"expert-puuid"}`)
		case strings.HasSuffix(request.URL.Path, "/ids"):
			body, _ := json.Marshal(matchIDs)
			return specialistTestResponse(request, http.StatusOK, string(body))
		case strings.Contains(request.URL.Path, "/lol/match/v5/matches/"):
			details.Add(1)
			matchID := strings.TrimPrefix(request.URL.Path, "/lol/match/v5/matches/")
			return specialistTestResponse(request, http.StatusOK, specialistMatchBody(matchID, "expert-puuid", 1, "complete", false))
		default:
			return specialistTestResponse(request, http.StatusNotFound, `{}`)
		}
	}))
	if runes := provider.specialistRunes(context.Background(), 64, "leesin", "李青"); len(runes) != 0 {
		t.Fatalf("another champion's runes were returned: %#v", runes)
	}
	if details.Load() != specialistRuneMatchScanMax {
		t.Fatalf("match detail requests = %d, want %d", details.Load(), specialistRuneMatchScanMax)
	}
}

func TestSpecialistRunesRejectsIncompleteRunes(t *testing.T) {
	t.Setenv("RIOT_API_KEY", "RGAPI-test")
	for _, runeState := range []string{"missing-substyle", "too-few-perks"} {
		t.Run(runeState, func(t *testing.T) {
			player := championTopPlayer{Name: "Expert", Tagline: "KR1", Games: "200"}
			provider := specialistTestProvider(gameplayRoundTripFunc(func(request *http.Request) (*http.Response, error) {
				switch {
				case request.URL.Host == opggPageHost:
					return specialistTestResponse(request, http.StatusOK, specialistLeaderboardBody(player))
				case strings.Contains(request.URL.Path, "/accounts/by-riot-id/"):
					return specialistTestResponse(request, http.StatusOK, `{"puuid":"expert-puuid"}`)
				case strings.HasSuffix(request.URL.Path, "/ids"):
					return specialistTestResponse(request, http.StatusOK, `["KR_1"]`)
				case strings.Contains(request.URL.Path, "/lol/match/v5/matches/"):
					return specialistTestResponse(request, http.StatusOK, specialistMatchBody("KR_1", "expert-puuid", 64, runeState, true))
				default:
					return specialistTestResponse(request, http.StatusNotFound, `{}`)
				}
			}))
			if runes := provider.specialistRunes(context.Background(), 64, "leesin", "李青"); len(runes) != 0 {
				t.Fatalf("incomplete runes were returned: %#v", runes)
			}
		})
	}
}

func TestSpecialistRunesCacheAvoidsUpstreamRequests(t *testing.T) {
	t.Setenv("RIOT_API_KEY", "RGAPI-test")
	player := championTopPlayer{Name: "Expert", Tagline: "KR1", Games: "200"}
	var requests atomic.Int64
	provider := specialistTestProvider(gameplayRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests.Add(1)
		switch {
		case request.URL.Host == opggPageHost:
			return specialistTestResponse(request, http.StatusOK, specialistLeaderboardBody(player))
		case strings.Contains(request.URL.Path, "/accounts/by-riot-id/"):
			return specialistTestResponse(request, http.StatusOK, `{"puuid":"expert-puuid"}`)
		case strings.HasSuffix(request.URL.Path, "/ids"):
			return specialistTestResponse(request, http.StatusOK, `["KR_1"]`)
		default:
			return specialistTestResponse(request, http.StatusOK, specialistMatchBody("KR_1", "expert-puuid", 64, "complete", true))
		}
	}))
	first := provider.specialistRunes(context.Background(), 64, "leesin", "李青")
	requestCount := requests.Load()
	second := provider.specialistRunes(context.Background(), 64, "leesin", "李青")
	if len(first) != 1 || len(second) != 1 || requests.Load() != requestCount {
		t.Fatalf("cache miss: first=%d second=%d requests=%d->%d", len(first), len(second), requestCount, requests.Load())
	}
}

func TestSpecialistRunesConcurrentCallsShareOneFlight(t *testing.T) {
	t.Setenv("RIOT_API_KEY", "RGAPI-test")
	player := championTopPlayer{Name: "Expert", Tagline: "KR1", Games: "200"}
	var requests atomic.Int64
	accountStarted := make(chan struct{})
	releaseAccount := make(chan struct{})
	var startedOnce sync.Once
	provider := specialistTestProvider(gameplayRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests.Add(1)
		switch {
		case request.URL.Host == opggPageHost:
			return specialistTestResponse(request, http.StatusOK, specialistLeaderboardBody(player))
		case strings.Contains(request.URL.Path, "/accounts/by-riot-id/"):
			startedOnce.Do(func() { close(accountStarted) })
			<-releaseAccount
			return specialistTestResponse(request, http.StatusOK, `{"puuid":"expert-puuid"}`)
		case strings.HasSuffix(request.URL.Path, "/ids"):
			return specialistTestResponse(request, http.StatusOK, `["KR_1"]`)
		default:
			return specialistTestResponse(request, http.StatusOK, specialistMatchBody("KR_1", "expert-puuid", 64, "complete", true))
		}
	}))
	results := make(chan []gameplayRecommendationRune, 2)
	go func() { results <- provider.specialistRunes(context.Background(), 64, "leesin", "李青") }()
	<-accountStarted
	go func() { results <- provider.specialistRunes(context.Background(), 64, "leesin", "李青") }()
	deadline := time.Now().Add(time.Second)
	for {
		provider.specialistMu.Lock()
		flight := provider.specialistFlights[64]
		waiters := 0
		if flight != nil {
			waiters = flight.waiters
		}
		provider.specialistMu.Unlock()
		if waiters == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("second call did not join the specialist singleflight")
		}
		time.Sleep(time.Millisecond)
	}
	close(releaseAccount)
	first, second := <-results, <-results
	if len(first) != 1 || len(second) != 1 || requests.Load() != 4 {
		t.Fatalf("singleflight results=%d/%d requests=%d", len(first), len(second), requests.Load())
	}
}

func TestSpecialistRunesWithoutRiotKeyReturnsEmptyWithoutRequests(t *testing.T) {
	t.Setenv("RIOT_API_KEY", "")
	var requests atomic.Int64
	provider := specialistTestProvider(gameplayRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests.Add(1)
		return specialistTestResponse(request, http.StatusInternalServerError, `{}`)
	}))
	if runes := provider.specialistRunes(context.Background(), 64, "leesin", "李青"); len(runes) != 0 || requests.Load() != 0 {
		t.Fatalf("runes=%#v requests=%d", runes, requests.Load())
	}
}
