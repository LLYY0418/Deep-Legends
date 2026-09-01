package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestMatchOPGGAverageTierUsesClosestValidGame(t *testing.T) {
	createdAt := int64(1_800_000_000_000)
	games := []opggGameTier{
		{createdAt: createdAt + 90_000, duration: 1_805, tier: matchTiersResponse{Tier: "GOLD", Division: "I"}},
		{createdAt: createdAt + 10_000, duration: 1_815, tier: matchTiersResponse{Tier: "EMERALD", Division: "II", LP: 44}},
	}
	got := matchOPGGAverageTier(createdAt, 1_800, games)
	if got == nil || got.Tier != "EMERALD" || got.Division != "II" || got.LP != 44 {
		t.Fatalf("closest tier = %#v", got)
	}
	got.Tier = "MUTATED"
	if games[1].tier.Tier != "EMERALD" {
		t.Fatal("returned value aliases the OP.GG cache entry")
	}
	if got := matchOPGGAverageTier(createdAt, 1_800, []opggGameTier{{
		createdAt: createdAt + opggGamesTimeSlack,
		duration:  1_800 + opggGamesSpanSlack,
		tier:      matchTiersResponse{Tier: "SILVER"},
	}}); got == nil || got.Tier != "SILVER" {
		t.Fatalf("inclusive boundary did not match: %#v", got)
	}
	for name, got := range map[string]*matchTiersResponse{
		"missing timestamp": matchOPGGAverageTier(0, 1_800, games),
		"missing duration":  matchOPGGAverageTier(createdAt, 0, games),
		"time outside": matchOPGGAverageTier(createdAt, 1_800, []opggGameTier{{
			createdAt: createdAt + opggGamesTimeSlack + 1, duration: 1_800, tier: matchTiersResponse{Tier: "GOLD"},
		}}),
		"duration outside": matchOPGGAverageTier(createdAt, 1_800, []opggGameTier{{
			createdAt: createdAt, duration: 1_800 + opggGamesSpanSlack + 1, tier: matchTiersResponse{Tier: "GOLD"},
		}}),
	} {
		if got != nil {
			t.Fatalf("%s unexpectedly matched: %#v", name, got)
		}
	}
}

func TestParseOPGGHistoricalRanksKeeps2023AndNewerRankedSeasons(t *testing.T) {
	payload := []byte(`<script>self.__next_f.push([1,"{\"data\":[{\"season\":\"S2025 \",\"rank_entries\":{\"high_rank_info\":{\"value\":\"CHALLENGER\"},\"rank_info\":{\"tier\":\"master\",\"value\":\"MASTER\",\"division\":1,\"lp\":\"285\"}}},{\"season\":\"S2024 S3\",\"rank_entries\":{\"rank_info\":{\"tier\":\"diamond 1\",\"value\":\"DIAMOND\",\"division\":1,\"lp\":\"1,024\"}}},{\"season\":\"S2024 S2\",\"rank_entries\":{\"rank_info\":{\"tier\":\"\",\"value\":\"Unranked\",\"division\":\"$undefined\",\"lp\":null}}},{\"season\":\"S2023 S1\",\"rank_entries\":{\"rank_info\":{\"tier\":\"gold 2\",\"value\":\"GOLD\",\"division\":2,\"lp\":\"70\"}}},{\"season\":\"S2022 \",\"rank_entries\":{\"rank_info\":{\"tier\":\"diamond 4\",\"value\":\"DIAMOND\",\"division\":4,\"lp\":\"20\"}}}]}" ])</script>`)
	ranks := parseOPGGHistoricalRanks(payload)
	if len(ranks) != 3 {
		t.Fatalf("historical ranks = %#v", ranks)
	}
	if ranks[0].Season != "S2025" || ranks[0].Tier != "MASTER" || ranks[0].LeaguePoints == nil || *ranks[0].LeaguePoints != 285 {
		t.Fatalf("first rank = %#v", ranks[0])
	}
	if ranks[1].Season != "S2024 S3" || ranks[1].Tier != "DIAMOND" || ranks[1].Division != "I" || ranks[1].LeaguePoints == nil || *ranks[1].LeaguePoints != 1024 {
		t.Fatalf("second rank = %#v", ranks[1])
	}
	if ranks[2].Season != "S2023 S1" || ranks[2].Division != "II" || ranks[2].LeaguePoints == nil || *ranks[2].LeaguePoints != 70 {
		t.Fatalf("third rank = %#v", ranks[2])
	}
}

func TestParseOPGGHistoricalRanksPreservesMissingLP(t *testing.T) {
	ranks := parseOPGGHistoricalRanks([]byte(`{"season":"S2024 S1","rank_entries":{"rank_info":{"tier":"gold","value":"GOLD","division":3,"lp":null}}}`))
	if len(ranks) != 1 || ranks[0].LeaguePoints != nil {
		t.Fatalf("historical rank with missing LP = %#v", ranks)
	}
}

func TestOPGGHistoricalRanksSkipsPrivatePlayer(t *testing.T) {
	var calls atomic.Int32
	champions := newChampionProvider()
	champions.clientMu.Lock()
	champions.client = &http.Client{Transport: gameplayRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		calls.Add(1)
		return testHTTPResponse(request, http.StatusOK, "unexpected request"), nil
	})}
	champions.clientMu.Unlock()
	a := &app{champions: champions, opgg: newOPGGInsights()}
	if ranks := a.opggHistoricalRanks(context.Background(), "Private", "KR1", strings.Repeat("p", 48), "PRIVATE"); len(ranks) != 0 {
		t.Fatalf("private historical ranks = %#v", ranks)
	}
	if calls.Load() != 0 {
		t.Fatalf("private player triggered %d OP.GG request(s)", calls.Load())
	}
}

func TestOPGGHistoricalRanksStartDoesNotBlockCaller(t *testing.T) {
	playerRef := strings.Repeat("a", 48)
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	released := false
	defer func() {
		if !released {
			close(release)
		}
	}()
	champions := newChampionProvider()
	champions.clientMu.Lock()
	champions.client = &http.Client{Transport: sgpRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		select {
		case started <- struct{}{}:
		default:
		}
		<-release
		return &http.Response{StatusCode: http.StatusServiceUnavailable, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("unavailable")), Request: request}, nil
	})}
	champions.clientMu.Unlock()
	a := &app{
		champions: champions, opgg: newOPGGInsights(), token: "session-secret",
		gameplayRefs: make(map[string]string), gameplayRefDetails: make(map[string]gameplayReference),
	}
	returned := make(chan struct{})
	go func() {
		a.startOPGGHistoricalRanks(gameplayReference{PlayerRef: playerRef, Region: riotRegionKR}, "Player", "KR1", playerRef, "PUBLIC")
		close(returned)
	}()
	select {
	case <-returned:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("OPGG historical ranks blocked the overview caller")
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("OPGG historical rank background request did not start")
	}
	close(release)
	released = true
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		a.opgg.mu.Lock()
		running := len(a.opgg.historyStarts)
		a.opgg.mu.Unlock()
		if running == 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("OPGG historical rank background request did not finish")
}

func TestGameplayMatchTiersKeepsCNContract(t *testing.T) {
	playerPUUID := strings.Repeat("p", 48)
	currentPUUID := strings.Repeat("c", 48)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/lol-ranked/v1/ranked-stats/"+playerPUUID {
			http.Error(w, "unexpected endpoint", http.StatusNotFound)
			return
		}
		_, _ = io.WriteString(w, `{"queues":[{"queueType":"RANKED_SOLO_5x5","tier":"GOLD","division":"II","leaguePoints":55,"wins":12,"losses":8}]}`)
	}))
	defer server.Close()

	a := &app{
		token: "session-secret", connected: true,
		lcu:      &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()},
		summoner: currentSummoner(currentPUUID), rankScores: newRankScoreCache(),
		gameplayRefs: make(map[string]string), gameplayRefDetails: make(map[string]gameplayReference),
	}
	publicRef := a.registerGameplayReferenceDetails(gameplayReference{PlayerRef: playerPUUID})
	body, _ := json.Marshal(matchTiersRequest{PlayerRefs: []string{publicRef}})
	request := httptest.NewRequest(http.MethodPost, "/api/gameplay/match-tiers", strings.NewReader(string(body)))
	recorder := httptest.NewRecorder()
	a.handleGameplayMatchTiers(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	var response matchTiersResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Samples != 1 || response.Score != 1455 || response.Tier != "GOLD" || response.Division != "II" {
		t.Fatalf("response = %#v", response)
	}
	if player := response.Players[publicRef]; player.Score != 1455 || player.Tier != "GOLD" || player.Division != "II" {
		t.Fatalf("per-player response = %#v", response.Players)
	}
}

func TestPlayerRankScoreCoalescesConcurrentLookup(t *testing.T) {
	playerPUUID := strings.Repeat("s", 48)
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			close(started)
		}
		<-release
		_, _ = io.WriteString(w, `{"queues":[{"queueType":"RANKED_SOLO_5x5","tier":"GOLD","division":"II","leaguePoints":55,"wins":12,"losses":8}]}`)
	}))
	defer server.Close()
	a := &app{rankScores: newRankScoreCache()}
	client := &LCUClient{baseURL: server.URL, token: "test", http: server.Client(), platformProbe: true}
	result := make(chan rankScoreEntry, 1)
	go func() { result <- a.playerRankScore(context.Background(), client, playerPUUID, false, "", "") }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("first ranked lookup did not start")
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if entry := a.playerRankScore(cancelled, client, playerPUUID, false, "", ""); entry.known {
		t.Fatalf("cancelled follower returned a rank: %#v", entry)
	}
	close(release)
	if entry := <-result; !entry.known || entry.score != 1455 {
		t.Fatalf("leader result = %#v", entry)
	}
	if calls.Load() != 1 {
		t.Fatalf("ranked lookups = %d, want 1", calls.Load())
	}
}

func TestPlayerRankScoreAllConcurrentFollowersReceiveLeaderResult(t *testing.T) {
	playerPUUID := strings.Repeat("q", 48)
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			close(started)
		}
		<-release
		_, _ = io.WriteString(w, `{"queues":[{"queueType":"RANKED_SOLO_5x5","tier":"GOLD","division":"II","leaguePoints":55,"wins":12,"losses":8}]}`)
	}))
	defer server.Close()
	a := &app{rankScores: newRankScoreCache()}
	client := &LCUClient{baseURL: server.URL, token: "test", http: server.Client(), platformProbe: true}
	const count = 8
	results := make(chan rankScoreEntry, count)
	for index := 0; index < count; index++ {
		go func() { results <- a.playerRankScore(context.Background(), client, playerPUUID, false, "", "") }()
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("concurrent ranked lookup did not start")
	}
	close(release)
	for index := 0; index < count; index++ {
		entry := <-results
		if !entry.known || entry.score != 1455 {
			t.Fatalf("concurrent result %d = %#v", index, entry)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("ranked lookups = %d, want 1", calls.Load())
	}
}

func TestGameplayMatchTiersPropagatesReferencePrivacy(t *testing.T) {
	playerPUUID := strings.Repeat("v", 48)
	currentPUUID := strings.Repeat("c", 48)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/leagues-ledge/v2/rankedStats/puuid/") {
			http.Error(w, "unexpected endpoint", http.StatusNotFound)
			return
		}
		_, _ = io.WriteString(w, `{"queues":[{"queueType":"RANKED_SOLO_5x5","tier":"GOLD","rank":"II","leaguePoints":55,"wins":12,"losses":8}]}`)
	}))
	defer server.Close()

	client := &LCUClient{http: server.Client(), region: "TENCENT", rsoPlatform: "HN1", platformProbe: true}
	provider := newSGPProvider()
	provider.http = server.Client()
	provider.serverBases["HN1"] = server.URL
	provider.sessionToken = "test-session"
	provider.sessionAt = time.Now()
	provider.sessionOwner = client
	observed := captureSGPObservations(provider)
	a := &app{
		token: "session-secret", connected: true, lcu: client, sgp: provider,
		summoner: currentSummoner(currentPUUID), rankScores: newRankScoreCache(),
		gameplayRefs: make(map[string]string), gameplayRefDetails: make(map[string]gameplayReference),
	}
	publicRef := a.registerGameplayReferenceDetails(gameplayReference{PlayerRef: playerPUUID, ServerID: "HN1", Privacy: "PRIVATE"})
	body, _ := json.Marshal(matchTiersRequest{PlayerRefs: []string{publicRef}})
	recorder := httptest.NewRecorder()
	a.handleGameplayMatchTiers(recorder, httptest.NewRequest(http.MethodPost, "/api/gameplay/match-tiers", strings.NewReader(string(body))))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	var shape map[string]any
	for _, event := range *observed {
		if event["event"] == "sgp_ranked_stats_shape" {
			shape = event
			break
		}
	}
	if shape == nil || shape["privacy"] != "PRIVATE" {
		t.Fatalf("ranked shape privacy = %#v", shape)
	}
}

func TestGameplayMatchTiersBatchesKRWithoutLCU(t *testing.T) {
	created := time.Date(2026, time.August, 18, 12, 0, 0, 0, time.UTC)
	puuid := strings.Repeat("k", 48)
	var calls atomic.Int32
	champions := newChampionProvider()
	champions.clientMu.Lock()
	champions.client = &http.Client{Transport: gameplayRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		calls.Add(1)
		if request.URL.Host != "op.gg" || !strings.Contains(request.URL.Path, "/Trusted-KR1") {
			t.Fatalf("untrusted OP.GG route: %s", request.URL.String())
		}
		if request.Header.Get("Next-Action") != opggGamesAction {
			t.Fatalf("Next-Action = %q", request.Header.Get("Next-Action"))
		}
		data, _ := io.ReadAll(request.Body)
		if !strings.Contains(string(data), puuid) || strings.Contains(string(data), "Attacker") {
			t.Fatalf("unexpected request body: %s", data)
		}
		payload := fmt.Sprintf("0:{\"data\":[{\"created_at\":%q,\"game_length\":1800,\"average_tier\":{\"tier\":\"emerald\",\"division\":2,\"lp\":37}}]}\n", created.Format(time.RFC3339))
		return testHTTPResponse(request, http.StatusOK, payload), nil
	})}
	champions.clientMu.Unlock()

	a := &app{
		token: "session-secret", champions: champions, opgg: newOPGGInsights(),
		gameplayRefs: make(map[string]string), gameplayRefDetails: make(map[string]gameplayReference),
	}
	publicRef := a.registerGameplayReferenceDetails(gameplayReference{
		PlayerRef: puuid, GameName: "Trusted", TagLine: "KR1", Region: riotRegionKR,
	})
	body, _ := json.Marshal(matchTiersRequest{
		Region: riotRegionKR, PlayerRef: publicRef, GameName: "Attacker", TagLine: "BAD",
		Matches: []matchTierMatchRequest{
			{GameID: 101, CreatedAt: created.UnixMilli(), Duration: 1_800},
			{GameID: 102, CreatedAt: created.Add(-time.Hour).UnixMilli(), Duration: 1_800},
		},
	})
	request := httptest.NewRequest(http.MethodPost, "/api/gameplay/match-tiers", strings.NewReader(string(body)))
	recorder := httptest.NewRecorder()
	a.handleGameplayMatchTiers(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	var response map[string]*matchTiersResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if got := response["101"]; got == nil || got.Tier != "EMERALD" || got.Division != "II" || got.LP != 37 {
		t.Fatalf("matched tier = %#v", got)
	}
	if got, exists := response["102"]; !exists || got != nil {
		t.Fatalf("unmatched tier = %#v, exists=%v", got, exists)
	}
	if calls.Load() != 1 {
		t.Fatalf("OP.GG calls = %d, want 1", calls.Load())
	}
	if strings.Contains(recorder.Body.String(), puuid) {
		t.Fatal("response leaked the stable PUUID")
	}
}

func TestGameplayMatchTiersKRFailureAndCancellationDegrade(t *testing.T) {
	created := time.Date(2026, time.August, 18, 12, 0, 0, 0, time.UTC)
	puuid := strings.Repeat("z", 48)
	started := make(chan struct{})
	champions := newChampionProvider()
	champions.clientMu.Lock()
	champions.client = &http.Client{Transport: gameplayRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		close(started)
		<-request.Context().Done()
		return nil, request.Context().Err()
	})}
	champions.clientMu.Unlock()
	a := &app{
		token: "session-secret", champions: champions, opgg: newOPGGInsights(),
		gameplayRefs: make(map[string]string), gameplayRefDetails: make(map[string]gameplayReference),
	}
	publicRef := a.registerGameplayReferenceDetails(gameplayReference{PlayerRef: puuid, GameName: "Cancel", TagLine: "KR1", Region: riotRegionKR})
	body, _ := json.Marshal(matchTiersRequest{Region: riotRegionKR, PlayerRef: publicRef, Matches: []matchTierMatchRequest{{GameID: 201, CreatedAt: created.UnixMilli(), Duration: 1_800}}})
	ctx, cancel := context.WithCancel(context.Background())
	request := httptest.NewRequest(http.MethodPost, "/api/gameplay/match-tiers", strings.NewReader(string(body))).WithContext(ctx)
	recorder := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		a.handleGameplayMatchTiers(recorder, request)
		close(done)
	}()
	select {
	case <-started:
		cancel()
	case <-time.After(time.Second):
		t.Fatal("OP.GG request did not start")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("cancelled OP.GG request blocked the handler")
	}
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	var response map[string]*matchTiersResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if got, exists := response["201"]; !exists || got != nil {
		t.Fatalf("cancelled result = %#v, exists=%v", got, exists)
	}
}

func TestGameplayMatchTiersKRHTTPFailureAndExpiredReference(t *testing.T) {
	created := time.Date(2026, time.August, 18, 12, 0, 0, 0, time.UTC)
	puuid := strings.Repeat("f", 48)
	var calls atomic.Int32
	champions := newChampionProvider()
	champions.clientMu.Lock()
	champions.client = &http.Client{Transport: gameplayRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		calls.Add(1)
		return testHTTPResponse(request, http.StatusInternalServerError, "upstream failed"), nil
	})}
	champions.clientMu.Unlock()
	a := &app{
		token: "session-secret", champions: champions, opgg: newOPGGInsights(),
		gameplayRefs: make(map[string]string), gameplayRefDetails: make(map[string]gameplayReference),
	}
	publicRef := a.registerGameplayReferenceDetails(gameplayReference{PlayerRef: puuid, GameName: "Failure", TagLine: "KR1", Region: riotRegionKR})
	makeBody := func(playerRef string) string {
		body, _ := json.Marshal(matchTiersRequest{Region: riotRegionKR, PlayerRef: playerRef, Matches: []matchTierMatchRequest{{GameID: 211, CreatedAt: created.UnixMilli(), Duration: 1_800}}})
		return string(body)
	}
	recorder := httptest.NewRecorder()
	a.handleGameplayMatchTiers(recorder, httptest.NewRequest(http.MethodPost, "/api/gameplay/match-tiers", strings.NewReader(makeBody(publicRef))))
	if recorder.Code != http.StatusOK {
		t.Fatalf("upstream failure status = %d: %s", recorder.Code, recorder.Body.String())
	}
	var response map[string]*matchTiersResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if got, exists := response["211"]; !exists || got != nil {
		t.Fatalf("failed result = %#v, exists=%v", got, exists)
	}

	recorder = httptest.NewRecorder()
	a.handleGameplayMatchTiers(recorder, httptest.NewRequest(http.MethodPost, "/api/gameplay/match-tiers", strings.NewReader(makeBody("player_expired_123"))))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expired reference status = %d, want 404", recorder.Code)
	}
	if calls.Load() != 1 {
		t.Fatalf("expired reference reached OP.GG; calls = %d", calls.Load())
	}
}

func TestOPGGGameTiersCacheExpandsForOlderPages(t *testing.T) {
	base := time.Date(2026, time.August, 18, 12, 0, 0, 0, time.UTC)
	puuid := strings.Repeat("e", 48)
	var calls atomic.Int32
	champions := newChampionProvider()
	champions.clientMu.Lock()
	champions.client = &http.Client{Transport: gameplayRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		calls.Add(1)
		var payload []opggGamesRequest
		data, _ := io.ReadAll(request.Body)
		if err := json.Unmarshal(data, &payload); err != nil || len(payload) != 1 {
			t.Fatalf("request payload = %s, err=%v", data, err)
		}
		page := 0
		if payload[0].EndedAt != "" {
			page = 1
		}
		rows := make([]map[string]any, opggGamesPageSize)
		for index := range rows {
			offset := page*opggGamesPageSize + index
			rows[index] = map[string]any{
				"created_at":   base.Add(-time.Duration(offset) * time.Minute).Format(time.RFC3339),
				"game_length":  1_800,
				"average_tier": map[string]any{"tier": "gold", "division": 1, "lp": 0},
			}
		}
		encoded, _ := json.Marshal(map[string]any{"data": rows})
		return testHTTPResponse(request, http.StatusOK, "0:"+string(encoded)+"\n"), nil
	})}
	champions.clientMu.Unlock()
	a := &app{champions: champions, opgg: newOPGGInsights()}

	first := a.opggGameTiers(context.Background(), "Cache", "KR1", puuid, base.Add(-10*time.Minute).UnixMilli())
	if len(first) != opggGamesPageSize || calls.Load() != 1 {
		t.Fatalf("first load: games=%d calls=%d", len(first), calls.Load())
	}
	older := a.opggGameTiers(context.Background(), "Cache", "KR1", puuid, base.Add(-30*time.Minute).UnixMilli())
	if len(older) != 2*opggGamesPageSize || calls.Load() != 3 {
		t.Fatalf("older load: games=%d calls=%d", len(older), calls.Load())
	}
	again := a.opggGameTiers(context.Background(), "Cache", "KR1", puuid, base.Add(-30*time.Minute).UnixMilli())
	if len(again) != len(older) || calls.Load() != 3 {
		t.Fatalf("cache hit: games=%d calls=%d", len(again), calls.Load())
	}
}

func TestOPGGGameTiersCoalescesConcurrentRequests(t *testing.T) {
	base := time.Date(2026, time.August, 18, 12, 0, 0, 0, time.UTC)
	puuid := strings.Repeat("q", 48)
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	champions := newChampionProvider()
	champions.clientMu.Lock()
	champions.client = &http.Client{Transport: gameplayRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		<-release
		payload := fmt.Sprintf("0:{\"data\":[{\"created_at\":%q,\"game_length\":1800,\"average_tier\":{\"tier\":\"gold\",\"division\":1}}]}\n", base.Format(time.RFC3339))
		return testHTTPResponse(request, http.StatusOK, payload), nil
	})}
	champions.clientMu.Unlock()
	a := &app{champions: champions, opgg: newOPGGInsights()}
	results := make(chan []opggGameTier, 2)
	go func() { results <- a.opggGameTiers(context.Background(), "Flight", "KR1", puuid, base.UnixMilli()) }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("first request did not start")
	}
	go func() { results <- a.opggGameTiers(context.Background(), "Flight", "KR1", puuid, base.UnixMilli()) }()
	time.Sleep(20 * time.Millisecond)
	close(release)
	for range 2 {
		select {
		case result := <-results:
			if len(result) != 1 {
				t.Fatalf("games = %d, want 1", len(result))
			}
		case <-time.After(time.Second):
			t.Fatal("coalesced request did not finish")
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("OP.GG calls = %d, want 1", calls.Load())
	}
}

func TestLoadRiotOverviewLoadsOPGGHistoryWithoutMatchTierRequests(t *testing.T) {
	t.Setenv("RIOT_API_KEY", "RGAPI-test")
	puuid := strings.Repeat("r", 48)
	created := time.Date(2026, time.August, 18, 12, 0, 0, 0, time.UTC)
	var match riotMatch
	match.Metadata.MatchID = "KR_301"
	match.Info.GameID = 301
	match.Info.GameCreation = created.UnixMilli()
	match.Info.GameDuration = 1_800
	match.Info.QueueID = 420
	match.Info.GameMode = "CLASSIC"
	match.Info.MapID = 11
	match.Info.Participants = []riotParticipant{{ParticipantID: 1, TeamID: 100, PUUID: puuid, RiotIDGameName: "Fast", RiotIDTagline: "KR1", ChampionID: 1, ChampLevel: 18, Kills: 5, Deaths: 2, Assists: 8, Win: true}}
	match.Info.Teams = []riotTeam{{TeamID: 100, Win: true}}
	matchJSON, _ := json.Marshal(match)
	var opggCalls atomic.Int32
	champions := newChampionProvider()
	champions.mu.Lock()
	champions.championMeta[1] = championMetadata{ID: 1, NameZH: "黑暗之女"}
	champions.mu.Unlock()
	events := make([]map[string]any, 0, 1)
	champions.diag = func(event map[string]any) { events = append(events, event) }
	champions.clientMu.Lock()
	champions.client = &http.Client{Transport: gameplayRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Host == "op.gg" {
			opggCalls.Add(1)
			if request.Method != http.MethodGet {
				t.Fatalf("overview sent unexpected OP.GG %s request", request.Method)
			}
			return testHTTPResponse(request, http.StatusOK, `{"season":"S2025 S3","rank_entries":{"rank_info":{"tier":"diamond","value":"DIAMOND","division":1,"lp":"70"}}`), nil
		}
		switch {
		case strings.Contains(request.URL.Path, "/lol/summoner/v4/summoners/by-puuid/"):
			return testHTTPResponse(request, http.StatusOK, fmt.Sprintf(`{"puuid":%q,"profileIconId":7,"summonerLevel":99}`, puuid)), nil
		case strings.Contains(request.URL.Path, "/lol/match/v5/matches/by-puuid/"):
			return testHTTPResponse(request, http.StatusOK, `["KR_301"]`), nil
		case request.URL.Path == "/lol/match/v5/matches/KR_301":
			return testHTTPResponse(request, http.StatusOK, string(matchJSON)), nil
		case strings.Contains(request.URL.Path, "/lol/league/v4/entries/by-puuid/"):
			return testHTTPResponse(request, http.StatusOK, `[]`), nil
		case strings.Contains(request.URL.Path, "/lol/champion-mastery/v4/champion-masteries/by-puuid/"):
			return testHTTPResponse(request, http.StatusOK, `[]`), nil
		default:
			return testHTTPResponse(request, http.StatusNotFound, `not found`), nil
		}
	})}
	champions.clientMu.Unlock()
	a := &app{
		token: "session-secret", champions: champions, riot: newRiotProvider(champions), opgg: newOPGGInsights(),
		gameplayRefs: make(map[string]string), gameplayRefDetails: make(map[string]gameplayReference),
	}
	overview, err := a.loadRiotOverview(context.Background(), gameplayReference{PlayerRef: puuid, GameName: "Fast", TagLine: "KR1", Region: riotRegionKR}, 0, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(overview.Matches) != 1 || overview.Matches[0].GameID != 301 {
		t.Fatalf("matches = %#v", overview.Matches)
	}
	if opggCalls.Load() != 1 {
		t.Fatalf("overview requested OP.GG %d time(s), want one history request", opggCalls.Load())
	}
	if len(overview.HistoricalRanks) != 1 || overview.HistoricalRanks[0].Season != "S2025 S3" || overview.HistoricalRanks[0].LeaguePoints == nil || *overview.HistoricalRanks[0].LeaguePoints != 70 {
		t.Fatalf("historical ranks = %#v", overview.HistoricalRanks)
	}
	var cost map[string]any
	for _, event := range events {
		if event["event"] == "riot_overview_cost" {
			cost = event
			break
		}
	}
	if cost == nil || cost["matches_requested"] != 1 || cost["matches_loaded"] != 1 || cost["rate_limited_count"] != 0 {
		t.Fatalf("riot overview cost = %#v", cost)
	}
}

func TestRiotRateLimitRecordsDiagnosticAndRequestScopedCount(t *testing.T) {
	t.Setenv("RIOT_API_KEY", "RGAPI-test")
	champions := newChampionProvider()
	events := make([]map[string]any, 0, 1)
	champions.diag = func(event map[string]any) { events = append(events, event) }
	champions.clientMu.Lock()
	champions.client = &http.Client{Transport: gameplayRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		response := testHTTPResponse(request, http.StatusTooManyRequests, "rate limited")
		response.Header.Set("Retry-After", "3")
		return response, nil
	})}
	champions.clientMu.Unlock()
	provider := newRiotProvider(champions)
	tracker := &riotOverviewCostTracker{}
	ctx, cancel := context.WithTimeout(context.WithValue(context.Background(), riotOverviewCostTrackerKey{}, tracker), 20*time.Millisecond)
	defer cancel()
	var payload map[string]any
	err := provider.getLimited(ctx, "kr.api.riotgames.com", "/lol/match/v5/matches/KR_123", nil, &payload, riotResponseMax)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("rate limit error = %v", err)
	}
	if tracker.rateLimitCount() != 1 {
		t.Fatalf("request rate-limit count = %d", tracker.rateLimitCount())
	}
	if len(events) != 1 || events[0]["event"] != "riot_rate_limited" || events[0]["host"] != "kr.api.riotgames.com" || events[0]["path"] != "/lol/match/v5/matches/KR_123" || events[0]["retry_after_ms"] != int64(3000) || events[0]["attempt"] != 1 {
		t.Fatalf("rate-limit diagnostic = %#v", events)
	}
}

func currentSummoner(puuid string) Summoner {
	return Summoner{PUUID: puuid}
}

func testHTTPResponse(request *http.Request, status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    request,
	}
}
