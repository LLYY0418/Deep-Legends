package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func r231SeasonFixture(t *testing.T, n int, boundary bool) (*app, *LCUClient, string, *sync.Map) {
	t.Helper()
	ref := strings.Repeat("s", 48)
	counts := &sync.Map{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start, _ := strconv.Atoi(r.URL.Query().Get("startIndex"))
		tags := r.URL.Query()["tag"]
		stream := strings.Join(tags, ",")
		key := stream + ":" + strconv.Itoa(start)
		v, _ := counts.LoadOrStore(key, &atomic.Int32{})
		v.(*atomic.Int32).Add(1)
		games := []any{}
		if len(tags) > 0 && tags[0] == "q_420" {
			for i := start; i < start+50 && i < n+1; i++ {
				if i == n && !boundary {
					break
				}
				created := time.Now().UnixMilli()
				if i == n {
					created = seasonStartS26.Add(-time.Second).UnixMilli()
				}
				games = append(games, map[string]any{"json": map[string]any{"gameId": i + 1, "queueId": 440, "gameCreation": created, "gameDuration": 1800, "participants": []any{map[string]any{"participantId": 1, "teamId": 100, "puuid": ref, "championId": 103, "win": true, "kills": 2, "deaths": 1}}}})
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"games": games})
	}))
	t.Cleanup(server.Close)
	client := &LCUClient{region: "TENCENT", rsoPlatform: "HN1"}
	a := r175App(t)
	a.sgp = newSGPProvider()
	a.sgp.http = server.Client()
	a.sgp.serverBases["HN1"] = server.URL
	a.sgp.token = "test"
	a.sgp.tokenAt = time.Now()
	a.sgp.tokenClient = client
	return a, client, ref, counts
}
func r231Scan(ref string) *seasonScanState {
	return &seasonScanState{cache: seasonStatsCache{SchemaVersion: seasonStatsCacheSchemaVersion, Source: seasonStatsSource, Season: "S26"}, stats: map[int64]*gameplaySeasonChampionStat{}, queueStats: map[int64]gameplayAggregate{}, seen: map[int64]bool{}, seasonStartMillis: seasonStartS26.UnixMilli(), playerRef: ref}
}
func TestR231TaggedSeasonBeyondMixedWindowAndCap(t *testing.T) {
	for _, tc := range []struct {
		n        int
		boundary bool
		stop     string
	}{{313, true, "season_start"}, {1000, false, "upstream_cap_1000"}} {
		t.Run(tc.stop, func(t *testing.T) {
			a, client, ref, counts := r231SeasonFixture(t, tc.n, tc.boundary)
			scan := r231Scan(ref)
			a.seasonScanPagesWithHistoryCache(t.Context(), client, "HN1", ref, scan, 30, false)
			if !scan.cache.Complete || scan.queueStats[440].Games != tc.n || scan.cache.Streams["ranked"].StopReason != tc.stop || scan.cache.CappedByUpstream != !tc.boundary {
				t.Fatal(scan.cache, scan.queueStats)
			}
			counts.Range(func(key, value any) bool {
				if strings.HasPrefix(key.(string), ":") || value.(*atomic.Int32).Load() != 1 {
					t.Errorf("unexpected duplicate/unfiltered page %s: %d", key, value.(*atomic.Int32).Load())
				}
				return true
			})
		})
	}
}
func TestR231HeadAlwaysZeroAndAddsNewGames(t *testing.T) {
	a, client, ref, counts := r231SeasonFixture(t, 150, false)
	scan := r231Scan(ref)
	for i := 3; i <= 50; i++ {
		scan.seen[int64(i)] = true
		scan.cache.GameIDs = append(scan.cache.GameIDs, int64(i))
	}
	scan.cache.Streams = map[string]seasonStatsStream{"ranked": {Complete: true, PendingIndex: 700}, "mayhem": {Complete: true, PendingIndex: 800}}
	scan.cache.Complete = true
	scan.headOnly = true
	for i := 0; i < 3; i++ {
		a.seasonScanPagesWithHistoryCache(t.Context(), client, "HN1", ref, scan, 2, false)
	}
	if scan.scanned != 2 || scan.cache.ResumeIndex != 0 || scan.cache.PendingIndex != 0 {
		t.Fatal(scan.scanned, scan.cache)
	}
	counts.Range(func(key, value any) bool {
		if !strings.HasSuffix(key.(string), ":0") {
			t.Errorf("head jumped: %s", key)
		}
		return true
	})
}
func TestR231ConcurrentWritesMergeGamesAndAggregates(t *testing.T) {
	a := r175App(t)
	ref := strings.Repeat("x", 48)
	hash := a.storage.accountHash(Summoner{PUUID: ref})
	scans := []*seasonScanState{r231Scan(ref), r231Scan(ref)}
	for i, scan := range scans {
		scan.cache.AccountHash = hash
		info := &riotMatchInfo{GameID: int64(i + 1), QueueID: 440, GameCreation: time.Now().UnixMilli(), GameDuration: 1800, Participants: []riotParticipant{{PUUID: ref, ChampionID: 103, Win: true}}}
		scan.cache.GameIDs = []int64{info.GameID}
		scan.newInfos = map[int64]*riotMatchInfo{info.GameID: info}
		seasonStatsAccumulate(scan.stats, scan.queueStats, info, ref, scan.seasonStartMillis)
	}
	var wg sync.WaitGroup
	for _, scan := range scans {
		wg.Add(1)
		go func(scan *seasonScanState) { defer wg.Done(); a.finishSeasonScan(scan, nil, hash) }(scan)
	}
	wg.Wait()
	cache, err := a.storage.loadSeasonStats(seasonStatsSource, hash, "S26")
	if err != nil || len(cache.GameIDs) != 2 || seasonStatsCount(cache.Stats) != 2 || cache.QueueStats[440].Games != 2 {
		t.Fatal(cache, err)
	}
}
func TestR231BackgroundSuppressesHead(t *testing.T) {
	a, client, ref, counts := r231SeasonFixture(t, 50, false)
	season, _ := currentRankedSeason(time.Now())
	hash := a.storage.accountHash(Summoner{PUUID: ref})
	key := sourceScopedKey(seasonStatsSource, hash+"|"+season)
	a.seasonBackfills = map[string]struct{}{key: {}}
	a.startSeasonStatsRefresh(client, gameplayReference{ServerID: "HN1", PlayerRef: ref}, Summoner{PUUID: ref}, ref, nil, true)
	counts.Range(func(key, value any) bool { t.Error("head queried during backfill", key); return true })
	a.seasonBackfillMu.Lock()
	delete(a.seasonBackfills, key)
	a.seasonBackfillMu.Unlock()
}
func TestR231SeasonSummaryIsNetworkFree(t *testing.T) {
	a, client, ref, _ := r231SeasonFixture(t, 20, false)
	scan := r231Scan(ref)
	scan.cache.AccountHash = a.storage.accountHash(Summoner{PUUID: ref})
	a.seasonScanPagesWithHistoryCache(t.Context(), client, "HN1", ref, scan, 3, false)
	a.finishSeasonScan(scan, nil, scan.cache.AccountHash)
	public := a.registerGameplayReferenceDetails(gameplayReference{ServerID: "HN1", PlayerRef: ref})
	var network atomic.Int32
	a.sgp.http.Transport = sgpRoundTripFunc(func(*http.Request) (*http.Response, error) {
		network.Add(1)
		return nil, errors.New("network forbidden")
	})
	w := httptest.NewRecorder()
	a.handleGameplaySeasonSummary(w, httptest.NewRequest("GET", "/api/gameplay/season-summary?playerRef="+public, nil))
	if w.Code != 200 || network.Load() != 0 || !strings.Contains(w.Body.String(), `"scanned":20`) {
		t.Fatal(w.Code, w.Body.String(), network.Load())
	}
}
func TestR231RelayForceBackoffAndSlowBody(t *testing.T) {
	r206RelayFixture(t)
	state := &riotRelayState{}
	var calls atomic.Int32
	client := &http.Client{Transport: r196RoundTrip(func(r *http.Request) (*http.Response, error) {
		if calls.Add(1) == 1 {
			return nil, errors.New("temporary network failure")
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(&r231SlowReader{})}, nil
	})}
	if _, err := state.ensure(t.Context(), client, nil); !errors.Is(err, errRiotRelayUnavailable) {
		t.Fatal(err)
	}
	began := time.Now()
	if _, err := state.ensureForce(t.Context(), client, nil, true); err != nil || time.Since(began) > time.Second || calls.Load() != 2 {
		t.Fatal(err, time.Since(began), calls.Load())
	}
	state.failed("https://relay.example")
	if state.unavailable() {
		t.Fatal("single request failure locked relay")
	}
	state.failed("https://relay.example")
	if !state.unavailable() {
		t.Fatal("two failures did not back off")
	}
	for i, want := range []int{15, 30, 60, 120, 120} {
		if got := int(relayBackoff(i + 1).Seconds()); got != want {
			t.Fatal(got, want)
		}
	}
}

type r231SlowReader struct{}

func (*r231SlowReader) Read([]byte) (int, error) { time.Sleep(10 * time.Second); return 0, io.EOF }
func TestR231ClientShutdownSignalAndRestore(t *testing.T) {
	a := r175App(t)
	client := &LCUClient{}
	a.lcu = client
	a.connected = true
	a.identityReady = true
	now := time.Now()
	a.observeClientShutdownEvent(client, LCUEvent{URI: "/riotclient/pre-shutdown/begin", Data: json.RawMessage(`true`)}, now)
	defer func() {
		a.mu.Lock()
		if a.shutdownTimer != nil {
			a.shutdownTimer.Stop()
		}
		a.mu.Unlock()
	}()
	if a.connectionState != "client-exiting" || !a.clientShutdownPending(client) {
		t.Fatal(a.connectionState)
	}
	if a.restoreClientShutdown(client, now.Add(14*time.Second)) {
		t.Fatal("early restore")
	}
	if !a.restoreClientShutdown(client, now.Add(15*time.Second)) || a.connectionState != "connected" {
		t.Fatal("did not restore")
	}
}
func TestR231RatingTemporaryAndNotRecordedTTL(t *testing.T) {
	for _, permanent := range []bool{false, true} {
		t.Run(fmt.Sprint(permanent), func(t *testing.T) {
			var calls int
			now := time.Now()
			client := newAramkitTestClient(aramkitRoundTripFunc(func(*http.Request) (*http.Response, error) {
				calls++
				if permanent {
					return aramkitJSONResponse(200, `{"code":400,"message":"not found"}`), nil
				}
				return nil, errors.New("timeout")
			}))
			client.now = func() time.Time { return now }
			client.sleep = func(context.Context, time.Duration) error { return nil }
			client.lookup(t.Context(), "玩家", "1")
			now = now.Add(31 * time.Second)
			client.lookup(t.Context(), "玩家", "1")
			want := 2
			if permanent {
				want = 1
			}
			if calls != want {
				t.Fatal(calls, want)
			}
		})
	}
}

func TestR231CurrentForeignHistoryDoesNotWaitForRiot(t *testing.T) {
	t.Setenv("RIOT_API_KEY", "RGAPI-r231-fixture")
	block := make(chan struct{})
	champs := newChampionProvider()
	champs.client = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		select {
		case <-block:
		case <-r.Context().Done():
		}
		return nil, errors.New("Riot unavailable")
	})}
	client := newLCUClient(1, "fixture")
	client.region, client.rsoPlatform = "JP", "JP1"
	client.http = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		game := `{"gameId":231,"queueId":420,"gameDuration":1800,"gameType":"MATCHED_GAME","participants":[{"participantId":1,"teamId":100,"championId":1,"stats":{"win":true}}],"participantIdentities":[{"participantId":1,"player":{"puuid":"r231-local-player"}}]}`
		if strings.Contains(r.URL.Path, "/games/") {
			return proHTTPBody([]byte(game)), nil
		}
		return proHTTPBody([]byte(`{"games":{"games":[` + game + `]}}`)), nil
	})}
	a := &app{riot: newRiotProvider(champs), lcu: client, summoner: Summoner{PUUID: "r231-local-player", GameName: "Fixture", TagLine: "JP1"}}
	defer func() {
		close(block)
		deadline := time.Now().Add(2 * time.Second)
		for {
			a.mu.RLock()
			pending := len(a.currentRiotDetailFlights)
			a.mu.RUnlock()
			if pending == 0 {
				return
			}
			if time.Now().After(deadline) {
				t.Error("background fixture did not stop")
				return
			}
			time.Sleep(time.Millisecond)
		}
	}()
	began := time.Now()
	matches, caps, _ := a.loadDetailedMatches(t.Context(), client, gameplayReferenceFromSummoner(a.summoner), a.summoner.PUUID, true, 0, 10, "all", nil, nil)
	if time.Since(began) > time.Second || len(matches) != 1 || matches[0].GameID != 231 || caps[0].State != capabilityAvailable {
		t.Fatal(time.Since(began), matches, caps)
	}
	a.connected = true
	began = time.Now()
	overview := a.loadGameplayOverview(t.Context(), client, a.summoner, gameplayReferenceFromSummoner(a.summoner), 0, 10, "all", false)
	if time.Since(began) > time.Second || len(overview.Matches) != 1 || overview.Matches[0].GameID != 231 {
		t.Fatal("full overview blocked", time.Since(began), overview.Matches)
	}
}

func TestR231QueueLabelsReturnBeforeSlowLCU(t *testing.T) {
	client := newLCUClient(1, "fixture")
	done := make(chan struct{})
	client.http = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		<-r.Context().Done()
		close(done)
		return nil, r.Context().Err()
	})}
	began := time.Now()
	labels := loadQueueLabelsContext(t.Context(), client)
	if time.Since(began) > time.Second || labels[420] == "" {
		t.Fatal(time.Since(began), labels)
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("queue refresh exceeded 1.5s budget")
	}
}

func TestR231OPGGZeroIsExplicitAndRegionCopy(t *testing.T) {
	ref := gameplayReference{Region: "jp1", GameName: "Fixture", TagLine: "JP1"}
	profile, _ := json.Marshal(map[string]any{"region": "jp", "data": map[string]any{"gameName": ref.GameName, "tagline": ref.TagLine, "puuid": "r231-opgg-player"}})
	for _, missing := range []bool{false, true} {
		data := map[string]any{"game_type": "SOLORANKED", "season_id": 33, "my_champion_stats": []any{}, "play": 0, "win": 0, "lose": 0}
		if missing {
			delete(data, "play")
		}
		stats, _ := json.Marshal(map[string]any{"recommendationRequestContext": map[string]any{"puuid": "r231-opgg-player"}, "gameType": "SOLORANKED", "year": time.Now().Year(), "data": data})
		push, _ := json.Marshal([]any{1, "1:" + string(profile) + "\n2:" + string(stats) + "\n"})
		result, err := parseOPGGChampionTable([]byte("<script>self.__next_f.push("+string(push)+")</script>"), ref, "SOLORANKED", nil)
		if missing && err == nil || !missing && (err != nil || !result.TableSupported || result.Overall.Games != 0 || len(result.TableRows) != 0) {
			t.Fatal(missing, result, err)
		}
	}
	body := riotHTTPErrorBody(context.DeadlineExceeded, "jp1")
	raw, _ := json.Marshal(body)
	if !strings.Contains(string(raw), "日服") || strings.Contains(string(raw), "韩服") {
		t.Fatal(string(raw))
	}
}

func TestR231ShutdownChatSignalsMustBeRecent(t *testing.T) {
	a := r175App(t)
	client := &LCUClient{}
	a.lcu, a.connected = client, true
	now := time.Now()
	a.observeClientShutdownEvent(client, LCUEvent{URI: "/lol-chat/v1/session", Data: json.RawMessage(`{"state":"offline"}`)}, now)
	a.observeClientShutdownEvent(client, LCUEvent{URI: "/lol-chat/v1/friends", Data: json.RawMessage(`[]`)}, now.Add(11*time.Second))
	if a.clientShutdownPending(client) {
		t.Fatal("stale offline state triggered shutdown")
	}
	a.observeClientShutdownEvent(client, LCUEvent{URI: "/lol-chat/v1/session", Data: json.RawMessage(`{"state":"offline"}`)}, now.Add(12*time.Second))
	if !a.clientShutdownPending(client) {
		t.Fatal("recent combined signal was ignored")
	}
	a.mu.Lock()
	a.shutdownTimer.Stop()
	a.mu.Unlock()
}

func TestR231ForceJoinsProbeDuringBackoff(t *testing.T) {
	r206RelayFixture(t)
	flight := make(chan struct{})
	state := &riotRelayState{config: strings.Join(configuredRiotRelays(), "\n"), nextTry: time.Now().Add(time.Minute), flight: flight}
	done := make(chan error, 1)
	go func() {
		_, err := state.ensureForce(t.Context(), &http.Client{}, nil, true)
		done <- err
	}()
	select {
	case err := <-done:
		t.Fatal("force skipped existing probe", err)
	case <-time.After(20 * time.Millisecond):
	}
	state.mu.Lock()
	state.active = configuredRiotRelays()[0]
	state.flight = nil
	close(flight)
	state.mu.Unlock()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestR231ShutdownTraceRedactsDynamicPaths(t *testing.T) {
	for _, uri := range []string{"/lol-summoner/v1/player-private", "/foo/v1/player-private", "/lol-chat/v1/friends/player-private/messages?name=secret"} {
		if got := shutdownDiagnosticURI(uri); strings.Contains(got, "private") || strings.Contains(got, "secret") {
			t.Fatal(got)
		}
	}
}

func TestR231RankedAndMayhemStreamsStayIndependent(t *testing.T) {
	a, client, ref, _ := r231SeasonFixture(t, 313, true)
	original := a.sgp.http.Transport
	if original == nil {
		original = http.DefaultTransport
	}
	a.sgp.http.Transport = sgpRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Query().Get("tag") != "q_2300" {
			return original.RoundTrip(r)
		}
		if got := strings.Join(r.URL.Query()["tag"], ","); got != "q_2300,q_2400,q_3270" || r.URL.Query().Get("tagsQueryType") != "OR" {
			t.Error("wrong mayhem filter", r.URL.RawQuery)
		}
		games := []any{}
		for i, queue := range []int{2300, 2400, 3270} {
			games = append(games, map[string]any{"json": map[string]any{"gameId": 2000 + i, "queueId": queue, "gameCreation": time.Now().UnixMilli(), "gameDuration": 1800, "participants": []any{map[string]any{"participantId": 1, "puuid": ref, "teamId": 100, "championId": 105, "win": true}}}})
		}
		games = append(games, map[string]any{"json": map[string]any{"gameId": 2003, "gameCreation": seasonStartS26.Add(-time.Second).UnixMilli()}})
		body, _ := json.Marshal(map[string]any{"games": games})
		return proHTTPBody(body), nil
	})
	scan := r231Scan(ref)
	a.seasonScanPagesWithHistoryCache(t.Context(), client, "HN1", ref, scan, 30, false)
	if !scan.cache.Complete || len(scan.seen) != 316 || scan.queueStats[440].Games != 313 {
		t.Fatal(scan.cache, scan.queueStats)
	}
	for _, queue := range []int64{2300, 2400, 3270} {
		if scan.queueStats[queue].Games != 1 {
			t.Fatal(queue, scan.queueStats)
		}
	}
}

func TestR231ProbeCountsReadErrorResponseBytes(t *testing.T) {
	r206RelayFixture(t)
	state := &riotRelayState{}
	body := `{"ok":false}`
	client := &http.Client{Transport: r196RoundTrip(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 503, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	events := make(chan map[string]any, 1)
	state.ensure(t.Context(), client, func(row map[string]any) {
		if row["event"] == "riot_relay_probe" {
			events <- row
		}
	})
	if row := <-events; row["bytes"] != len(body) {
		t.Fatal(row)
	}
}
