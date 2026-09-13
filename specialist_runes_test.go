package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
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
	return specialistMatchBodyWithEarlySurrender(matchID, puuid, championID, runeState, win, false)
}

func specialistMatchBodyWithEarlySurrender(matchID, puuid string, championID int64, runeState string, win, earlySurrender bool) string {
	return specialistMatchBodyAtPosition(matchID, puuid, championID, runeState, "MIDDLE", win, earlySurrender)
}

func specialistMatchBodyAtPosition(matchID, puuid string, championID int64, runeState, position string, win, earlySurrender bool) string {
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
			"gameId": 123, "gameCreation": int64(1_720_000_000_000), "gameDuration": 185, "queueId": 420, "gameMode": "CLASSIC", "gameType": "MATCHED_GAME",
			"participants": []map[string]any{
				{
					"puuid": puuid, "teamId": 100, "championId": championID, "win": win, "teamPosition": position,
					"gameEndedInEarlySurrender": earlySurrender,
					"perks":                     map[string]any{"styles": styles, "statPerks": statPerks},
				},
				{
					"puuid": "opponent-puuid", "teamId": 200, "championId": 99, "teamPosition": position,
					"riotIdGameName": "LaneRival", "riotIdTagline": "KR1",
				},
			},
		},
	})
	return string(payload)
}

func TestSpecialistRuneMarksEarlySurrenderAsRemake(t *testing.T) {
	var match riotMatch
	body := specialistMatchBodyWithEarlySurrender("KR_1", "expert-puuid", 64, "complete", true, true)
	if err := json.Unmarshal([]byte(body), &match); err != nil {
		t.Fatal(err)
	}
	participant, ok := specialistParticipant(&match, "expert-puuid")
	if !ok {
		t.Fatal("specialist participant missing")
	}
	rune, ok := specialistRuneFromParticipant(64, "李青", championTopPlayer{Name: "Expert", Tagline: "KR1"}, &match, participant, 0)
	if !ok || rune.Result != "remake" {
		t.Fatalf("specialist remake = %#v, ok=%v", rune, ok)
	}
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
			detailRequests.Add(1)
			championID := int64(1)
			matchID := strings.TrimPrefix(request.URL.Path, "/lol/match/v5/matches/")
			matchNumber, _ := strconv.Atoi(strings.TrimPrefix(matchID, "KR_"))
			if matchNumber >= 3 && matchNumber <= 5 {
				championID = 64
			}
			return specialistTestResponse(request, http.StatusOK, specialistMatchBody(matchID, "expert-puuid", championID, "complete", true))
		default:
			return specialistTestResponse(request, http.StatusNotFound, `{}`)
		}
	}))
	provider.opponentRankScore = func(_ context.Context, puuid string) rankScoreEntry {
		if puuid != "opponent-puuid" {
			return rankScoreEntry{}
		}
		return rankScoreEntry{known: true, source: dataSourceRiot, tier: "master", division: "I", winRate: 57, winRateKnown: true}
	}
	runes, outcome := provider.specialistRunes(context.Background(), 64, "leesin", "李青", "mid")
	if outcome != specialistOutcomeSuccess {
		t.Fatalf("specialist outcome = %q", outcome)
	}
	if len(runes) != 3 {
		t.Fatalf("specialist runes = %#v", runes)
	}
	rune := runes[0]
	if rune.PlayerName != "Expert" || rune.TagLine != "KR1" || rune.Tier != "master" || rune.ChampionGames != 1451 || rune.PlayedAt != 1_720_000_000_000 || rune.Result != "win" || rune.Region != "kr" {
		t.Fatalf("specialist attribution = %#v", rune)
	}
	if rune.Position != "mid" || rune.OpponentChampionID != 99 || rune.OpponentPlayerName != "LaneRival" || rune.OpponentTagLine != "KR1" {
		t.Fatalf("specialist matchup attribution = %#v", rune)
	}
	if rune.opponentPUUID != "opponent-puuid" || rune.OpponentTier != "master" || rune.OpponentDivision != "I" || rune.OpponentWinRate == nil || *rune.OpponentWinRate != 57 {
		t.Fatalf("specialist opponent rank = %#v", rune)
	}
	if rune.PrimaryStyleID != 8000 || rune.SubStyleID != 8100 || len(rune.SelectedPerkIDs) != 9 || detailRequests.Load() != specialistRuneMatchScanMax || riotRequests.Load() != 2+specialistRuneMatchScanMax {
		t.Fatalf("rune=%#v detailRequests=%d riotRequests=%d", rune, detailRequests.Load(), riotRequests.Load())
	}
}

func TestSpecialistRunesScansTopPlayersConcurrently(t *testing.T) {
	t.Setenv("RIOT_API_KEY", "RGAPI-test")
	players := []championTopPlayer{
		{Name: "First", Tagline: "KR1"},
		{Name: "Second", Tagline: "KR1"},
		{Name: "Third", Tagline: "KR1"},
	}
	puuids := map[string]string{"First": "first-puuid", "Second": "second-puuid", "Third": "third-puuid"}
	started := make(chan struct{}, len(players))
	release := make(chan struct{})
	provider := specialistTestProvider(gameplayRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch {
		case request.URL.Host == opggPageHost:
			return specialistTestResponse(request, http.StatusOK, specialistLeaderboardBody(players...))
		case strings.Contains(request.URL.Path, "/accounts/by-riot-id/"):
			name := strings.TrimPrefix(request.URL.Path, "/riot/account/v1/accounts/by-riot-id/")
			name = strings.SplitN(name, "/", 2)[0]
			name, _ = url.PathUnescape(name)
			return specialistTestResponse(request, http.StatusOK, fmt.Sprintf(`{"puuid":%q}`, puuids[name]))
		case strings.HasSuffix(request.URL.Path, "/ids"):
			for name, puuid := range puuids {
				if strings.Contains(request.URL.Path, "/by-puuid/"+puuid+"/") {
					return specialistTestResponse(request, http.StatusOK, fmt.Sprintf(`["%s-match"]`, strings.ToLower(name)))
				}
			}
			return specialistTestResponse(request, http.StatusNotFound, `{}`)
		case strings.Contains(request.URL.Path, "/lol/match/v5/matches/"):
			started <- struct{}{}
			<-release
			matchID := strings.TrimPrefix(request.URL.Path, "/lol/match/v5/matches/")
			puuid := strings.TrimSuffix(strings.TrimSuffix(matchID, "-match"), "") + "-puuid"
			if strings.HasPrefix(matchID, "first") {
				puuid = "first-puuid"
			} else if strings.HasPrefix(matchID, "second") {
				puuid = "second-puuid"
			} else {
				puuid = "third-puuid"
			}
			return specialistTestResponse(request, http.StatusOK, specialistMatchBody(matchID, puuid, 64, "complete", true))
		default:
			return specialistTestResponse(request, http.StatusNotFound, `{}`)
		}
	}))
	result := make(chan []gameplayRecommendationRune, 1)
	go func() {
		runes, _ := provider.specialistRunes(context.Background(), 64, "leesin", "李青", "mid")
		result <- runes
	}()
	for range players {
		select {
		case <-started:
		case <-time.After(time.Second):
			close(release)
			t.Fatal("top-player scans did not run concurrently")
		}
	}
	close(release)
	runes := <-result
	if len(runes) != len(players) {
		t.Fatalf("specialist runes = %d, want %d: %#v", len(runes), len(players), runes)
	}
}

func TestSpecialistRunesBoundsDetailConcurrencyAndPreservesMatchOrder(t *testing.T) {
	t.Setenv("RIOT_API_KEY", "RGAPI-test")
	player := championTopPlayer{Name: "Expert", Tagline: "KR1"}
	matchIDs := []string{"KR_1", "KR_2", "KR_3", "KR_4", "KR_5", "KR_6"}
	var active atomic.Int64
	var maximum atomic.Int64
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
			current := active.Add(1)
			for {
				prior := maximum.Load()
				if current <= prior || maximum.CompareAndSwap(prior, current) {
					break
				}
			}
			matchID := strings.TrimPrefix(request.URL.Path, "/lol/match/v5/matches/")
			matchNumber, _ := strconv.Atoi(strings.TrimPrefix(matchID, "KR_"))
			time.Sleep(time.Duration(7-matchNumber) * 4 * time.Millisecond)
			active.Add(-1)
			body := specialistMatchBody(matchID, "expert-puuid", 64, "complete", true)
			body = strings.Replace(body, `"gameCreation":1720000000000`, fmt.Sprintf(`"gameCreation":%d`, 1_720_000_000_000+int64(matchNumber)), 1)
			return specialistTestResponse(request, http.StatusOK, body)
		default:
			return specialistTestResponse(request, http.StatusNotFound, `{}`)
		}
	}))
	runes, outcome := provider.specialistRunes(context.Background(), 64, "leesin", "李青", "mid")
	if outcome != specialistOutcomeSuccess || maximum.Load() != specialistMatchConcurrency {
		t.Fatalf("outcome=%q detail concurrency=%d, want %d", outcome, maximum.Load(), specialistMatchConcurrency)
	}
	if len(runes) != 3 || runes[0].PlayedAt != 1_720_000_000_001 || runes[1].PlayedAt != 1_720_000_000_002 || runes[2].PlayedAt != 1_720_000_000_003 {
		t.Fatalf("detail completion order leaked into recommendations: %#v", runes)
	}
}

func TestSpecialistRiotStepsCarryIndependentDeadlines(t *testing.T) {
	t.Setenv("RIOT_API_KEY", "RGAPI-test")
	player := championTopPlayer{Name: "Expert", Tagline: "KR1"}
	checked := make(map[string]bool)
	var checkedMu sync.Mutex
	provider := specialistTestProvider(gameplayRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		step, maximum := "", time.Duration(0)
		switch {
		case request.URL.Host == opggPageHost:
			return specialistTestResponse(request, http.StatusOK, specialistLeaderboardBody(player))
		case strings.Contains(request.URL.Path, "/accounts/by-riot-id/"):
			step, maximum = "account", specialistAccountTimeout
		case strings.HasSuffix(request.URL.Path, "/ids"):
			step, maximum = "match_ids", specialistMatchIDsTimeout
		case strings.Contains(request.URL.Path, "/lol/match/v5/matches/"):
			step, maximum = "match_detail", specialistMatchDetailTimeout
		default:
			return specialistTestResponse(request, http.StatusNotFound, `{}`)
		}
		deadline, ok := request.Context().Deadline()
		if !ok || time.Until(deadline) <= 0 || time.Until(deadline) > maximum+250*time.Millisecond {
			t.Errorf("%s deadline = %v, ok=%v, want <= %v", step, time.Until(deadline), ok, maximum)
		}
		checkedMu.Lock()
		checked[step] = true
		checkedMu.Unlock()
		switch step {
		case "account":
			return specialistTestResponse(request, http.StatusOK, `{"puuid":"expert-puuid"}`)
		case "match_ids":
			return specialistTestResponse(request, http.StatusOK, `["KR_1"]`)
		default:
			return specialistTestResponse(request, http.StatusOK, specialistMatchBody("KR_1", "expert-puuid", 64, "complete", true))
		}
	}))
	if runes, outcome := provider.specialistRunes(context.Background(), 64, "leesin", "李青", "mid"); len(runes) != 1 || outcome != specialistOutcomeSuccess {
		t.Fatalf("deadline fixture result = %#v, outcome=%q", runes, outcome)
	}
	for _, step := range []string{"account", "match_ids", "match_detail"} {
		if !checked[step] {
			t.Fatalf("%s request was not observed", step)
		}
	}
}

func TestRiotSpecialistQueueLimitReturnsThrottled(t *testing.T) {
	provider := newRiotProvider(newChampionProvider())
	now := time.Now()
	provider.longWindow = make([]time.Time, 90)
	for index := range provider.longWindow {
		provider.longWindow[index] = now
	}
	started := time.Now()
	err := provider.wait(withRiotQueueLimit(context.Background(), 20*time.Millisecond))
	if !errors.Is(err, errThrottled) || time.Since(started) > 250*time.Millisecond {
		t.Fatalf("specialist queue wait = %v after %v", err, time.Since(started))
	}
}

func TestSpecialistSlotQueueReturnsThrottled(t *testing.T) {
	slots := make(chan struct{}, 1)
	slots <- struct{}{}
	started := time.Now()
	err := acquireSpecialistSlot(context.Background(), slots, 20*time.Millisecond)
	if !errors.Is(err, errThrottled) || time.Since(started) > 250*time.Millisecond {
		t.Fatalf("specialist slot wait = %v after %v", err, time.Since(started))
	}
}

func TestRiotAccountCacheStoresSuccessAndBacksOffFailures(t *testing.T) {
	t.Setenv("RIOT_API_KEY", "RGAPI-test")
	t.Run("success", func(t *testing.T) {
		var requests atomic.Int64
		provider := specialistTestProvider(gameplayRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			requests.Add(1)
			return specialistTestResponse(request, http.StatusOK, `{"puuid":"stable-puuid"}`)
		}))
		for range 2 {
			account, err := provider.accountByRiotID(context.Background(), "Expert", "KR1")
			if err != nil || account.PUUID != "stable-puuid" {
				t.Fatalf("cached account = %#v, err=%v", account, err)
			}
		}
		if requests.Load() != 1 {
			t.Fatalf("positive account cache requests = %d", requests.Load())
		}
	})
	t.Run("not-found", func(t *testing.T) {
		var requests atomic.Int64
		provider := specialistTestProvider(gameplayRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			requests.Add(1)
			return specialistTestResponse(request, http.StatusNotFound, `{}`)
		}))
		for range 2 {
			if _, err := provider.accountByRiotID(context.Background(), "Broken", "KR1"); err == nil {
				t.Fatal("negative account cache unexpectedly succeeded")
			}
		}
		if requests.Load() != 1 {
			t.Fatalf("negative account cache requests = %d", requests.Load())
		}
		key := strings.ToLower("Broken") + "\x1f" + strings.ToLower("KR1")
		provider.accountMu.Lock()
		entry := provider.accountCache[key]
		entry.expiresAt = time.Now().Add(-time.Second)
		provider.accountCache[key] = entry
		provider.accountMu.Unlock()
		if _, err := provider.accountByRiotID(context.Background(), "Broken", "KR1"); err == nil {
			t.Fatal("second negative account request unexpectedly succeeded")
		}
		provider.accountMu.Lock()
		entry = provider.accountCache[key]
		provider.accountMu.Unlock()
		if requests.Load() != 2 || entry.failures != 2 || time.Until(entry.expiresAt) < 55*time.Second {
			t.Fatalf("negative backoff = requests:%d entry:%#v", requests.Load(), entry)
		}
	})
}

func TestSpecialistOpponentRanksDeduplicateBoundConcurrencyAndIsolateFailures(t *testing.T) {
	provider := newRiotProvider(newChampionProvider())
	const uniqueOpponents = 8
	runes := make([]gameplayRecommendationRune, 0, uniqueOpponents*2)
	for index := 0; index < uniqueOpponents; index++ {
		puuid := fmt.Sprintf("opponent-puuid-%d", index)
		for duplicate := 0; duplicate < 2; duplicate++ {
			runes = append(runes, gameplayRecommendationRune{
				OpponentPlayerName: fmt.Sprintf("Rival %d", index),
				opponentPUUID:      puuid,
			})
		}
	}
	var active atomic.Int64
	var maximum atomic.Int64
	var callsMu sync.Mutex
	calls := make(map[string]int)
	provider.opponentRankScore = func(_ context.Context, puuid string) rankScoreEntry {
		callsMu.Lock()
		calls[puuid]++
		callsMu.Unlock()
		current := active.Add(1)
		for {
			prior := maximum.Load()
			if current <= prior || maximum.CompareAndSwap(prior, current) {
				break
			}
		}
		time.Sleep(15 * time.Millisecond)
		active.Add(-1)
		if puuid == "opponent-puuid-3" {
			return rankScoreEntry{}
		}
		return rankScoreEntry{known: true, tier: "DIAMOND", division: "II", winRate: 54, winRateKnown: true}
	}

	batch := newSpecialistOpponentRankBatch(context.Background(), provider.opponentRankScore)
	for _, rune := range runes {
		batch.add(rune.opponentPUUID)
	}
	batch.apply(runes)
	if len(calls) != uniqueOpponents {
		t.Fatalf("unique opponent rank calls = %d, want %d: %#v", len(calls), uniqueOpponents, calls)
	}
	for puuid, count := range calls {
		if count != 1 {
			t.Fatalf("opponent %s rank calls = %d, want 1", puuid, count)
		}
	}
	if maximum.Load() != matchTiersRankConcurrency {
		t.Fatalf("opponent rank concurrency = %d, want %d", maximum.Load(), matchTiersRankConcurrency)
	}
	for index := range runes {
		if runes[index].opponentPUUID == "opponent-puuid-3" {
			if runes[index].OpponentPlayerName != "Rival 3" || runes[index].OpponentTier != "" || runes[index].OpponentWinRate != nil {
				t.Fatalf("failed opponent lookup damaged rune %d: %#v", index, runes[index])
			}
			continue
		}
		if runes[index].OpponentTier != "DIAMOND" || runes[index].OpponentDivision != "II" || runes[index].OpponentWinRate == nil || *runes[index].OpponentWinRate != 54 {
			t.Fatalf("enriched opponent rune %d = %#v", index, runes[index])
		}
	}
	encoded, err := json.Marshal(runes[0])
	if err != nil || strings.Contains(string(encoded), "opponent-puuid-0") {
		t.Fatalf("internal opponent PUUID leaked: %s err=%v", encoded, err)
	}
}

func TestSpecialistOpponentRanksDeduplicateThreeRecommendationsAndPreserveFailure(t *testing.T) {
	provider := newRiotProvider(newChampionProvider())
	runes := []gameplayRecommendationRune{
		{Title: "first-rune", OpponentPlayerName: "Shared Rival", opponentPUUID: "shared-opponent"},
		{Title: "second-rune", OpponentPlayerName: "Shared Rival", opponentPUUID: "shared-opponent"},
		{Title: "third-rune", OpponentPlayerName: "Unranked Rival", opponentPUUID: "missing-opponent"},
	}
	var calls atomic.Int64
	provider.opponentRankScore = func(_ context.Context, puuid string) rankScoreEntry {
		calls.Add(1)
		if puuid == "missing-opponent" {
			return rankScoreEntry{}
		}
		return rankScoreEntry{known: true, tier: "gold", division: "III", winRate: 51, winRateKnown: true}
	}

	batch := newSpecialistOpponentRankBatch(context.Background(), provider.opponentRankScore)
	for _, rune := range runes {
		batch.add(rune.opponentPUUID)
	}
	batch.apply(runes)
	if calls.Load() != 2 {
		t.Fatalf("three recommendations made %d rank calls, want 2 unique PUUIDs", calls.Load())
	}
	for index := range runes[:2] {
		if runes[index].Title == "" || runes[index].OpponentPlayerName != "Shared Rival" || runes[index].OpponentTier != "gold" || runes[index].OpponentWinRate == nil || *runes[index].OpponentWinRate != 51 {
			t.Fatalf("shared opponent recommendation %d = %#v", index, runes[index])
		}
	}
	if runes[2].Title != "third-rune" || runes[2].OpponentPlayerName != "Unranked Rival" || runes[2].OpponentTier != "" || runes[2].OpponentWinRate != nil {
		t.Fatalf("failed rank lookup damaged recommendation: %#v", runes[2])
	}
}

func TestSpecialistOpponentRankSurvivesScanDeadline(t *testing.T) {
	t.Setenv("RIOT_API_KEY", "RGAPI-test")
	player := championTopPlayer{Name: "Expert", Tagline: "KR1"}
	lookupStarted := make(chan struct{})
	provider := specialistTestProvider(gameplayRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch {
		case request.URL.Host == opggPageHost:
			return specialistTestResponse(request, http.StatusOK, specialistLeaderboardBody(player))
		case strings.Contains(request.URL.Path, "/accounts/by-riot-id/"):
			return specialistTestResponse(request, http.StatusOK, `{"puuid":"first-puuid"}`)
		case strings.HasSuffix(request.URL.Path, "/ids"):
			return specialistTestResponse(request, http.StatusOK, `["KR_1","KR_2"]`)
		case strings.Contains(request.URL.Path, "/lol/match/v5/matches/KR_1"):
			return specialistTestResponse(request, http.StatusOK, specialistMatchBody("KR_1", "first-puuid", 64, "complete", true))
		case strings.Contains(request.URL.Path, "/lol/match/v5/matches/KR_2"):
			<-request.Context().Done()
			return nil, request.Context().Err()
		default:
			return specialistTestResponse(request, http.StatusNotFound, `{}`)
		}
	}))
	provider.opponentRankScore = func(ctx context.Context, _ string) rankScoreEntry {
		if ctx.Err() != nil {
			t.Errorf("opponent rank context already canceled: %v", ctx.Err())
		}
		if _, ok := ctx.Deadline(); !ok {
			t.Error("opponent rank context has no independent deadline")
		}
		close(lookupStarted)
		return rankScoreEntry{known: true, tier: "platinum", division: "IV", winRate: 52, winRateKnown: true}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	runes, _ := provider.specialistRunes(ctx, 64, "leesin", "李青", "mid")
	if ctx.Err() == nil {
		t.Fatal("scan context did not reach its deadline")
	}
	select {
	case <-lookupStarted:
	default:
		t.Fatal("opponent rank lookup did not run after scan deadline")
	}
	if len(runes) != 1 || runes[0].OpponentTier != "platinum" || runes[0].OpponentWinRate == nil || *runes[0].OpponentWinRate != 52 {
		t.Fatalf("rank lookup did not survive specialist scan deadline: %#v", runes)
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
	if runes, outcome := provider.specialistRunes(context.Background(), 64, "leesin", "李青", "mid"); len(runes) != 0 || outcome != specialistOutcomeNoPositionSample {
		t.Fatalf("another champion's runes were returned: %#v", runes)
	}
	if details.Load() != int64(len(matchIDs)) {
		t.Fatalf("match detail requests = %d, want %d", details.Load(), len(matchIDs))
	}
}

func TestSpecialistRunesStopsAtTenWithoutWrongPositionFallback(t *testing.T) {
	t.Setenv("RIOT_API_KEY", "RGAPI-test")
	player := championTopPlayer{Name: "Expert", Tagline: "KR1", Games: "200"}
	matchIDs := make([]string, 50)
	for index := range matchIDs {
		matchIDs[index] = fmt.Sprintf("KR_%d", index+1)
	}
	var details atomic.Int64
	provider := specialistTestProvider(gameplayRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch {
		case request.URL.Host == opggPageHost:
			return specialistTestResponse(request, http.StatusOK, specialistLeaderboardBody(player))
		case strings.Contains(request.URL.Path, "/accounts/by-riot-id/"):
			return specialistTestResponse(request, http.StatusOK, `{"puuid":"expert-puuid"}`)
		case strings.HasSuffix(request.URL.Path, "/ids"):
			if request.URL.Query().Get("count") != "30" {
				t.Fatalf("match ID count = %q, want 30", request.URL.Query().Get("count"))
			}
			body, _ := json.Marshal(matchIDs)
			return specialistTestResponse(request, http.StatusOK, string(body))
		case strings.Contains(request.URL.Path, "/lol/match/v5/matches/"):
			details.Add(1)
			matchID := strings.TrimPrefix(request.URL.Path, "/lol/match/v5/matches/")
			return specialistTestResponse(request, http.StatusOK, specialistMatchBodyAtPosition(matchID, "expert-puuid", 64, "complete", "MIDDLE", true, false))
		default:
			return specialistTestResponse(request, http.StatusNotFound, `{}`)
		}
	}))
	runes, outcome := provider.specialistRunes(context.Background(), 64, "leesin", "李青", "top")
	if len(runes) != 0 || outcome != specialistOutcomeNoPositionSample || details.Load() != specialistRuneMatchScanMax {
		t.Fatalf("bounded no-position sample = runes:%#v outcome:%q details:%d", runes, outcome, details.Load())
	}
}

func TestSpecialistRunesSkipsPlayerAfterThreeConsecutiveDetailFailures(t *testing.T) {
	t.Setenv("RIOT_API_KEY", "RGAPI-test")
	player := championTopPlayer{Name: "Expert", Tagline: "KR1"}
	matchIDs := []string{"KR_1", "KR_2", "KR_3", "KR_4", "KR_5"}
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
			return specialistTestResponse(request, http.StatusInternalServerError, `{}`)
		default:
			return specialistTestResponse(request, http.StatusNotFound, `{}`)
		}
	}))
	if runes, outcome := provider.specialistRunes(context.Background(), 64, "leesin", "李青", "mid"); len(runes) != 0 || outcome != specialistOutcomeError || details.Load() != specialistMatchFailureLimit {
		t.Fatalf("three-failure stop = runes:%#v outcome:%q details:%d", runes, outcome, details.Load())
	}
}

func TestSpecialistRunesTimeoutReturnsCompletedPartialResults(t *testing.T) {
	t.Setenv("RIOT_API_KEY", "RGAPI-test")
	if specialistRuneRequestBudget > 90 {
		t.Fatalf("specialist request budget = %d, exceeds Riot long-window limit", specialistRuneRequestBudget)
	}
	players := []championTopPlayer{{Name: "First", Tagline: "KR1"}, {Name: "Second", Tagline: "KR1"}}
	provider := specialistTestProvider(gameplayRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch {
		case request.URL.Host == opggPageHost:
			return specialistTestResponse(request, http.StatusOK, specialistLeaderboardBody(players...))
		case strings.Contains(request.URL.Path, "/accounts/by-riot-id/First/"):
			return specialistTestResponse(request, http.StatusOK, `{"puuid":"first-puuid"}`)
		case strings.Contains(request.URL.Path, "/accounts/by-riot-id/Second/"):
			<-request.Context().Done()
			return nil, request.Context().Err()
		case strings.Contains(request.URL.Path, "/by-puuid/first-puuid/ids"):
			return specialistTestResponse(request, http.StatusOK, `["KR_1"]`)
		case strings.Contains(request.URL.Path, "/lol/match/v5/matches/KR_1"):
			return specialistTestResponse(request, http.StatusOK, specialistMatchBodyAtPosition("KR_1", "first-puuid", 64, "complete", "MIDDLE", true, false))
		default:
			return specialistTestResponse(request, http.StatusNotFound, `{}`)
		}
	}))
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	started := time.Now()
	runes, outcome := provider.specialistRunes(ctx, 64, "leesin", "李青", "mid")
	if len(runes) != 1 || outcome != specialistOutcomeSuccess || runes[0].PlayerName != "First" || time.Since(started) > time.Second {
		t.Fatalf("timeout partial result = runes:%#v outcome:%q duration:%v", runes, outcome, time.Since(started))
	}
}

func TestSpecialistRunesTimeoutIsNotReportedAsNoPositionSample(t *testing.T) {
	t.Setenv("RIOT_API_KEY", "RGAPI-test")
	player := championTopPlayer{Name: "Expert", Tagline: "KR1"}
	provider := specialistTestProvider(gameplayRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Host == opggPageHost {
			return specialistTestResponse(request, http.StatusOK, specialistLeaderboardBody(player))
		}
		<-request.Context().Done()
		return nil, request.Context().Err()
	}))
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	runes, outcome := provider.specialistRunes(ctx, 64, "leesin", "李青", "mid")
	if len(runes) != 0 || outcome != specialistOutcomeTimeout {
		t.Fatalf("timeout result = %#v, outcome=%q", runes, outcome)
	}
}

func TestSpecialistRecentSummaryAvoidsRepeatMatchListAndDetails(t *testing.T) {
	t.Setenv("RIOT_API_KEY", "RGAPI-test")
	player := championTopPlayer{Name: "Expert", Tagline: "KR1"}
	var accounts atomic.Int64
	var matchLists atomic.Int64
	var details atomic.Int64
	provider := specialistTestProvider(gameplayRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch {
		case request.URL.Host == opggPageHost:
			return specialistTestResponse(request, http.StatusOK, specialistLeaderboardBody(player))
		case strings.Contains(request.URL.Path, "/accounts/by-riot-id/"):
			accounts.Add(1)
			return specialistTestResponse(request, http.StatusOK, `{"puuid":"expert-puuid"}`)
		case strings.HasSuffix(request.URL.Path, "/ids"):
			matchLists.Add(1)
			return specialistTestResponse(request, http.StatusOK, `["KR_1","KR_2"]`)
		case strings.Contains(request.URL.Path, "/lol/match/v5/matches/"):
			details.Add(1)
			matchID := strings.TrimPrefix(request.URL.Path, "/lol/match/v5/matches/")
			return specialistTestResponse(request, http.StatusOK, specialistMatchBodyAtPosition(matchID, "expert-puuid", 64, "complete", "MIDDLE", true, false))
		default:
			return specialistTestResponse(request, http.StatusNotFound, `{}`)
		}
	}))
	if runes, outcome := provider.specialistRunes(context.Background(), 64, "leesin", "李青", "mid"); len(runes) != 2 || outcome != specialistOutcomeSuccess {
		t.Fatalf("first summary scan = %#v, outcome=%q", runes, outcome)
	}
	if runes, outcome := provider.specialistRunes(context.Background(), 1, "annie", "安妮", "mid"); len(runes) != 0 || outcome != specialistOutcomeNoPositionSample {
		t.Fatalf("summary-filtered scan = %#v, outcome=%q", runes, outcome)
	}
	if accounts.Load() != 1 || matchLists.Load() != 1 || details.Load() != 2 {
		t.Fatalf("summary cache requests account=%d ids=%d details=%d", accounts.Load(), matchLists.Load(), details.Load())
	}
}

func TestSpecialistErrorKindClassifiesActionableFailures(t *testing.T) {
	tests := []struct {
		err  error
		want string
	}{
		{context.DeadlineExceeded, "timeout"},
		{errRiotNotFound, "404"},
		{errors.New("Riot 接口限流中（HTTP 429）"), "rate-limit"},
		{errors.New("Riot 接口返回的数据无法解析"), "parse"},
		{&riotStatusError{message: "Riot 接口返回 HTTP 503", status: http.StatusServiceUnavailable}, "5xx"},
		{errors.New("player has 550 LP"), "other"},
		{errors.New("Riot API Key 无效、过期或无权访问"), "forbidden"},
	}
	for _, test := range tests {
		if got := specialistErrorKind(test.err); got != test.want {
			t.Errorf("specialistErrorKind(%q) = %q, want %q", test.err, got, test.want)
		}
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
			if runes, outcome := provider.specialistRunes(context.Background(), 64, "leesin", "李青", "mid"); len(runes) != 0 || outcome != specialistOutcomeNoPositionSample {
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
	var rankRequests atomic.Int64
	provider.opponentRankScore = func(_ context.Context, _ string) rankScoreEntry {
		rankRequests.Add(1)
		return rankScoreEntry{known: true, tier: "emerald", division: "I", winRate: 55, winRateKnown: true}
	}
	first, _ := provider.specialistRunes(context.Background(), 64, "leesin", "李青", "mid")
	requestCount := requests.Load()
	if len(first) == 1 && first[0].OpponentWinRate != nil {
		*first[0].OpponentWinRate = 1
	}
	second, _ := provider.specialistRunes(context.Background(), 64, "leesin", "李青", "mid")
	if len(first) != 1 || len(second) != 1 || requests.Load() != requestCount || rankRequests.Load() != 1 {
		t.Fatalf("cache miss: first=%d second=%d requests=%d->%d rank=%d", len(first), len(second), requestCount, requests.Load(), rankRequests.Load())
	}
	if second[0].OpponentTier != "emerald" || second[0].OpponentWinRate == nil || *second[0].OpponentWinRate != 55 {
		t.Fatalf("cached opponent rank = %#v", second[0])
	}
}

func TestSpecialistRunesConfirmedEmptyResultsAreCached(t *testing.T) {
	t.Setenv("RIOT_API_KEY", "RGAPI-test")
	var requests atomic.Int64
	provider := specialistTestProvider(gameplayRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests.Add(1)
		return specialistTestResponse(request, http.StatusOK, specialistLeaderboardBody())
	}))

	for range 2 {
		if runes, outcome := provider.specialistRunes(context.Background(), 64, "leesin", "李青", "mid"); len(runes) != 0 || outcome != specialistOutcomeNoPositionSample {
			t.Fatalf("empty leaderboard returned runes: %#v", runes)
		}
	}
	if requests.Load() != 1 {
		t.Fatalf("confirmed empty specialist result was not cached: requests=%d, want 1", requests.Load())
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
	go func() {
		runes, _ := provider.specialistRunes(context.Background(), 64, "leesin", "李青", "mid")
		results <- runes
	}()
	<-accountStarted
	go func() {
		runes, _ := provider.specialistRunes(context.Background(), 64, "leesin", "李青", "mid")
		results <- runes
	}()
	deadline := time.Now().Add(time.Second)
	for {
		provider.specialistMu.Lock()
		flight := provider.specialistFlights[specialistRuneKey(64, "mid")]
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

func TestSpecialistRuneKeyIsCollisionFreeAndCanonical(t *testing.T) {
	seen := make(map[string]string, 6000)
	positions := []string{"", "top", "jungle", "mid", "middle", "adc", "bottom", "support", "utility"}
	for championID := int64(1); championID <= 1000; championID++ {
		for _, position := range positions {
			key := specialistRuneKey(championID, position)
			binding := strconv.FormatInt(championID, 10) + "|" + canonicalSpecialistPosition(position)
			if prior, ok := seen[key]; ok && prior != binding {
				t.Fatalf("cache key collision: %q binds %q and %q", key, prior, binding)
			}
			seen[key] = binding
		}
	}
	if specialistRuneKey(12, "middle") != specialistRuneKey(12, "mid") || specialistRuneKey(12, "bottom") != specialistRuneKey(12, "adc") || specialistRuneKey(12, "utility") != specialistRuneKey(12, "support") {
		t.Fatal("lane aliases were not canonicalized")
	}
}

func TestSpecialistOpponentRequiresMatchingPosition(t *testing.T) {
	match := &riotMatch{}
	match.Info.Participants = []riotParticipant{
		{TeamID: 100, ChampionID: 1, TeamPosition: "TOP"},
		{TeamID: 200, ChampionID: 2, TeamPosition: "JUNGLE"},
	}
	if opponent := specialistOpponent(match, match.Info.Participants[0], "mid"); opponent.ChampionID != 0 {
		t.Fatalf("unexpected cross-position fallback: %#v", opponent)
	}
}

func TestSpecialistRunesWithoutRiotKeyReturnsEmptyWithoutRequests(t *testing.T) {
	t.Setenv("RIOT_API_KEY", "")
	var requests atomic.Int64
	provider := specialistTestProvider(gameplayRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests.Add(1)
		return specialistTestResponse(request, http.StatusInternalServerError, `{}`)
	}))
	if runes, outcome := provider.specialistRunes(context.Background(), 64, "leesin", "李青"); len(runes) != 0 || outcome != specialistOutcomeError || requests.Load() != 0 {
		t.Fatalf("runes=%#v requests=%d", runes, requests.Load())
	}
}

func TestR68SpecialistHandlerRecordsResolvedADCPosition(t *testing.T) {
	t.Setenv("RIOT_API_KEY", "")
	provider := newChampionProvider()
	var events []map[string]any
	provider.diag = func(event map[string]any) { events = append(events, event) }
	a := &app{champions: provider}
	recorder := httptest.NewRecorder()
	a.handleGameplaySpecialistRunes(recorder, httptest.NewRequest(http.MethodGet, "/api/gameplay/specialist-runes?championId=222&position=adc", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("handler status = %d: %s", recorder.Code, recorder.Body.String())
	}
	if len(events) != 1 || events[0]["event"] != "specialist_runes_handler" || events[0]["champion_id"] != int64(222) || events[0]["position"] != "adc" {
		t.Fatalf("specialist handler diagnostic = %#v", events)
	}
}

func TestSpecialistRunesRecordsStartFailureAndDoneDiagnostics(t *testing.T) {
	t.Setenv("RIOT_API_KEY", "RGAPI-test")
	player := championTopPlayer{Name: "Expert", Tagline: "KR1"}
	provider := specialistTestProvider(gameplayRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Host == opggPageHost {
			return specialistTestResponse(request, http.StatusOK, specialistLeaderboardBody(player))
		}
		return specialistTestResponse(request, http.StatusForbidden, `{}`)
	}))
	var events []map[string]any
	provider.champions.diag = func(event map[string]any) { events = append(events, event) }
	if runes, _ := provider.loadSpecialistRunes(context.Background(), 64, "leesin", "李青", "jungle"); len(runes) != 0 {
		t.Fatalf("runes = %#v", runes)
	}
	var specialistEvents []map[string]any
	for _, event := range events {
		if strings.HasPrefix(fmt.Sprint(event["event"]), "specialist_runes_") {
			specialistEvents = append(specialistEvents, event)
		}
	}
	if len(specialistEvents) != 3 || specialistEvents[0]["event"] != "specialist_runes_start" || specialistEvents[1]["event"] != "specialist_runes_step_failed" || specialistEvents[2]["event"] != "specialist_runes_done" {
		t.Fatalf("diagnostics = %#v", events)
	}
	if specialistEvents[1]["step"] != "account" || specialistEvents[1]["errorKind"] != "forbidden" {
		t.Fatalf("failure diagnostic = %#v", specialistEvents[1])
	}
	if _, ok := specialistEvents[1]["budget_remaining"]; !ok {
		t.Fatalf("failure budget missing: %#v", specialistEvents[1])
	}
}
