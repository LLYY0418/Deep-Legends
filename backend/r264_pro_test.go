package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestR264ProFirstScreenUsesLocalRosterDuringSevenSecondSeventyPercentFailures(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var requests atomic.Int32
	a := &app{champions: newChampionProvider(), proRefreshContext: ctx}
	a.champions.client = &http.Client{Transport: championRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		n := requests.Add(1)
		select {
		case <-time.After(7 * time.Second):
		case <-r.Context().Done():
			return nil, r.Context().Err()
		}
		if n%10 < 7 {
			return nil, errors.New("simulated 70 percent failures")
		}
		return r206RelayResponse(200, []byte(`{}`)), nil
	})}
	requestCtx, stop := context.WithTimeout(context.Background(), 1100*time.Millisecond)
	defer stop()
	started := time.Now()
	w := httptest.NewRecorder()
	a.handleProPlayers(w, httptest.NewRequest(http.MethodGet, "/api/pro-players", nil).WithContext(requestCtx))
	if time.Since(started) >= time.Second {
		t.Fatalf("local roster waited for relay: %s", time.Since(started))
	}
	var result proPlayersResponse
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || result.PlayerCount != 33 || result.AccountCount != 53 || len(result.Teams) != 6 || !result.Updating {
		t.Fatalf("local first screen: err=%v players=%d accounts=%d teams=%d updating=%v", err, result.PlayerCount, result.AccountCount, len(result.Teams), result.Updating)
	}
}

func TestR264ForeignCardsAndFirstMatchPrecedeSevenSecondSeventyPercentFailures(t *testing.T) {
	runR264ForeignPartial(t, 7)
}
func TestR264ForeignNineFailuresKeepOneVerifiedRankedMatch(t *testing.T) { runR264ForeignPartial(t, 9) }
func runR264ForeignPartial(t *testing.T, failedCount int) {
	t.Helper()
	t.Setenv("RIOT_API_KEY", "RGAPI-r264-synthetic-fixture")
	var detailRequests, failures atomic.Int32
	puuid := strings.Repeat("f", 48)
	champs := newChampionProvider()
	champs.client = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		route := r.URL.Path
		if strings.Contains(route, "/accounts/by-riot-id/") {
			return proHTTPBody([]byte(fmt.Sprintf(`{"puuid":%q,"gameName":"Fixture","tagLine":"KR1"}`, puuid))), nil
		}
		if strings.HasSuffix(route, "/ids") {
			return proHTTPBody([]byte(`["KR_2640","KR_2641","KR_2642","KR_2643","KR_2644","KR_2645","KR_2646","KR_2647","KR_2648","KR_2649"]`)), nil
		}
		if strings.Contains(route, "/matches/") {
			detailRequests.Add(1)
			id, _ := strconv.Atoi(strings.TrimPrefix(path.Base(route), "KR_"))
			if id != 2640 {
				select {
				case <-time.After(7 * time.Second):
				case <-r.Context().Done():
					return nil, r.Context().Err()
				}
			}
			if id >= 2641 && id <= 2640+failedCount {
				failures.Add(1)
				return nil, errors.New("synthetic slow relay failure")
			}
			body := fmt.Sprintf(`{"metadata":{"matchId":"KR_%d"},"info":{"gameId":%d,"queueId":420,"gameDuration":1800,"gameType":"MATCHED_GAME","participants":[{"participantId":1,"teamId":100,"puuid":"%s","championId":103,"win":true}]}}`, id, id, puuid)
			return proHTTPBody([]byte(body)), nil
		}
		if strings.Contains(route, "/league/") {
			return proHTTPBody([]byte(`[{"queueType":"RANKED_SOLO_5x5","tier":"GOLD","rank":"I","leaguePoints":20}]`)), nil
		}
		if strings.Contains(route, "champion-masteries") {
			return proHTTPBody([]byte(`[{"championId":103,"championPoints":1234}]`)), nil
		}
		if strings.Contains(route, "/summoner/") {
			return proHTTPBody([]byte(`{"puuid":"` + puuid + `","summonerLevel":30,"profileIconId":1}`)), nil
		}
		return proHTTPBody([]byte(`{}`)), nil
	})}
	a := &app{riot: newRiotProvider(champs), overviewQueries: newOverviewQueryCache()}
	ref := a.registerGameplayReferenceDetails(gameplayReference{PlayerRef: puuid, Region: "kr", GameName: "Fixture", TagLine: "KR1", Privacy: "PRIVATE"})
	server := httptest.NewServer(http.HandlerFunc(a.handleGameplayOverview))
	defer server.Close()
	request, _ := http.NewRequest(http.MethodPost, server.URL, strings.NewReader(`{"playerRef":"`+ref+`","region":"kr","count":10}`))
	request.Header.Set("Accept", "application/x-ndjson")
	request.Header.Set("X-Overview-Cards", "1")
	client := server.Client()
	client.Timeout = 20 * time.Second
	started := time.Now()
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 4096), 2<<20)
	sawRanks, sawMasteries, sawMatch := false, false, false
	var verifiedMatches []gameplayMatch
	for scanner.Scan() {
		var frame struct {
			Type     string           `json:"type"`
			Overview gameplayOverview `json:"overview"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &frame); err != nil {
			t.Fatal(err)
		}
		if len(frame.Overview.Ranks) > 0 {
			sawRanks = true
		}
		if len(frame.Overview.Masteries) > 0 {
			sawMasteries = true
		}
		if len(frame.Overview.Matches) > 0 {
			sawMatch = true
			verifiedMatches = frame.Overview.Matches
		}
		if sawRanks && sawMasteries && sawMatch {
			if time.Since(started) > 2*time.Second {
				t.Fatalf("known cards/match blocked by seven-second failures: %s", time.Since(started))
			}
			break
		}
	}
	if !sawRanks || !sawMasteries || !sawMatch {
		t.Fatalf("missing early data ranks=%v mastery=%v match=%v error=%v", sawRanks, sawMasteries, sawMatch, scanner.Err())
	}
	var complete gameplayOverview
	terminal := ""
	for scanner.Scan() {
		var frame struct {
			Type     string           `json:"type"`
			Overview gameplayOverview `json:"overview"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &frame); err != nil {
			t.Fatal(err)
		}
		if len(frame.Overview.Matches) > 0 {
			verifiedMatches = frame.Overview.Matches
		}
		if frame.Type == "complete" {
			complete = frame.Overview
			terminal = "complete"
		}
		if frame.Type == "error" {
			terminal = "error"
		}
	}
	if failedCount == 9 {
		if len(verifiedMatches) != 1 || terminal == "" {
			t.Fatalf("one verified preview and explicit terminal frame required: matches=%d terminal=%s", len(verifiedMatches), terminal)
		}
		if terminal == "complete" && complete.RecentRanked.Games != 1 {
			t.Fatalf("completed statistics must use the read match: games=%d", complete.RecentRanked.Games)
		}
	}
	if scanner.Err() != nil {
		t.Fatal(scanner.Err())
	}
	if detailRequests.Load() != 10 || failures.Load() != int32(failedCount) {
		t.Fatalf("partial scenario did not execute: requests=%d failures=%d expected=%d", detailRequests.Load(), failures.Load(), failedCount)
	}
}

func TestR264LateCardsSubscriberRetainsAlreadyReadFirstMatch(t *testing.T) {
	flight := &overviewQueryFlight{}
	flight.publishOverviewCards(gameplayOverview{Player: gameplayPlayer{ProfileIconID: 71, SummonerLevel: 30}, Matches: []gameplayMatch{{GameID: 264}}, Ranks: []gameplayRank{{Tier: "GOLD"}}})
	flight.publishOverviewCards(gameplayOverview{ProfilePending: true, Matches: []gameplayMatch{}, Masteries: []gameplayMastery{{ChampionID: 103}}})
	var got gameplayOverview
	cancel := flight.subscribeOverview(context.WithValue(context.Background(), localOverviewCardsProgressKey{}, func(value gameplayOverview) { got = value }))
	defer cancel()
	if got.ProfilePending || got.Player.ProfileIconID != 71 || got.Player.SummonerLevel != 30 || len(got.Matches) != 1 || got.Matches[0].GameID != 264 || len(got.Ranks) != 1 || len(got.Masteries) != 1 {
		t.Fatal("late subscriber lost known data", got)
	}
}

func TestR264ForeignProfilePrecedesSevenSecondHistoryIDs(t *testing.T) {
	t.Setenv("RIOT_API_KEY", "RGAPI-r264-synthetic-fixture")
	puuid := strings.Repeat("g", 48)
	champs := newChampionProvider()
	champs.client = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		route := r.URL.Path
		if strings.Contains(route, "/accounts/by-riot-id/") {
			return proHTTPBody([]byte(fmt.Sprintf(`{"puuid":%q,"gameName":"Fixture","tagLine":"KR1"}`, puuid))), nil
		}
		if strings.HasSuffix(route, "/ids") {
			select {
			case <-time.After(7 * time.Second):
				return proHTTPBody([]byte(`[]`)), nil
			case <-r.Context().Done():
				return nil, r.Context().Err()
			}
		}
		if strings.Contains(route, "/summoner/") {
			return proHTTPBody([]byte(`{"puuid":"` + puuid + `","summonerLevel":30,"profileIconId":71}`)), nil
		}
		if strings.Contains(route, "/league/") {
			return proHTTPBody([]byte(`[{"queueType":"RANKED_SOLO_5x5","tier":"GOLD","rank":"I"}]`)), nil
		}
		if strings.Contains(route, "champion-masteries") {
			return proHTTPBody([]byte(`[{"championId":103,"championPoints":1234}]`)), nil
		}
		return proHTTPBody([]byte(`{}`)), nil
	})}
	a := &app{riot: newRiotProvider(champs)}
	frames := make(chan gameplayOverview, 8)
	base, cancel := context.WithCancel(context.Background())
	ctx := context.WithValue(base, localOverviewCardsProgressKey{}, func(p gameplayOverview) { frames <- p })
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = a.loadRiotOverview(ctx, gameplayReference{PlayerRef: puuid, Region: "kr", GameName: "Fixture", TagLine: "KR1", Privacy: "PRIVATE"}, 0, 10)
	}()
	defer func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("cancelled fixture leaked")
		}
	}()
	ready := map[string]bool{}
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	for len(ready) < 3 {
		select {
		case p := <-frames:
			if len(p.Ranks) > 0 {
				ready["ranks"] = true
			}
			if len(p.Masteries) > 0 {
				ready["masteries"] = true
			}
			if !p.ProfilePending && p.Player.ProfileIconID == 71 && p.Player.SummonerLevel == 30 {
				ready["profile"] = true
			}
		case <-deadline.C:
			t.Fatalf("known profile/cards waited for slow history IDs: %v", ready)
		}
	}
}
