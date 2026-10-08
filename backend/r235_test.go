package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestR235MigrationKeepsHistoricalGames(t *testing.T) {
	store := trackTestStore(t, &localStore{root: t.TempDir()})
	for version := 1; version <= seasonStatsCacheSchemaVersion; version++ {
		old := seasonStatsCache{SchemaVersion: version, Source: seasonStatsSource, Season: "S26", AccountHash: "self", GameIDs: []int64{11, 12}, Stats: []gameplaySeasonChampionStat{{ChampionID: 103, Games: 2, Wins: 1, TotalKills: 16}}, QueueStats: map[int64]gameplayAggregate{440: {Games: 2, Wins: 1}}, Complete: true}
		raw, _ := json.Marshal(old)
		if err := writeLocalStoreFile(store, seasonStatsFileKey(seasonStatsSource, "self", "S26"), raw); err != nil {
			t.Fatal(err)
		}
		got, err := store.loadSeasonStats(seasonStatsSource, "self", "S26")
		if err != nil || len(got.GameIDs) != 2 || len(got.Stats) != 1 || got.Stats[0].TotalKills != 16 || got.QueueStats[440].Games != 2 {
			t.Fatalf("schema %d lost history: %#v %v", version, got, err)
		}
	}
}
func TestR235UpstreamWindowAndSupplementDedup(t *testing.T) {
	a, client, ref, _ := r231SeasonFixture(t, 256, false)
	scan := r231Scan(ref)
	a.seasonScanPagesWithHistoryCache(t.Context(), client, "HN1", ref, scan, 10, false)
	if c := scan.cache.Streams["ranked"]; c.StopReason != "upstream_window" || !c.CappedByUpstream || !scan.cache.CappedByUpstream {
		t.Fatal(c)
	}
	info := riotMatchInfo{GameID: 99001, QueueID: 440, GameCreation: seasonStartS26.Add(24 * time.Hour).UnixMilli(), GameDuration: 1800, Participants: []riotParticipant{{PUUID: ref, ChampionID: 103, Kills: 9, Deaths: 2, Assists: 7, Win: true}}}
	raw, _ := json.Marshal(map[string]any{"games": []riotMatchInfo{info, info}})
	infos := decodeSeasonSupplement(raw, ref)
	for _, game := range infos {
		if scan.seen[game.GameID] {
			continue
		}
		scan.seen[game.GameID] = true
		scan.cache.GameIDs = append(scan.cache.GameIDs, game.GameID)
		seasonStatsAccumulate(scan.stats, scan.queueStats, game, ref, scan.seasonStartMillis)
	}
	if scan.queueStats[440].Games != 257 || scan.stats[103].Games != 257 {
		t.Fatal(scan.queueStats, scan.stats[103])
	}
}
func TestR235HeadStreamsConcurrentAndFirstPagePush(t *testing.T) {
	a, client, ref, _ := r231SeasonFixture(t, 100, false)
	base := a.sgp.http.Transport
	var mu sync.Mutex
	starts := map[string]time.Time{}
	a.sgp.http = &http.Client{Transport: r196RoundTrip(func(r *http.Request) (*http.Response, error) {
		stream := r.URL.Query().Get("tag")
		offset := r.URL.Query().Get("startIndex")
		mu.Lock()
		if offset == "0" {
			starts[stream] = time.Now()
		}
		mu.Unlock()
		if offset == "0" {
			time.Sleep(100 * time.Millisecond)
		} else {
			time.Sleep(400 * time.Millisecond)
		}
		return base.RoundTrip(r)
	})}
	scan := r231Scan(ref)
	scan.headOnly = true
	pushed := 0
	started := time.Now()
	scan.onPage = func(scan *seasonScanState) {
		pushed++
		if pushed == 1 && time.Since(started) > 300*time.Millisecond {
			t.Fatal("first page waited for second page")
		}
	}
	a.seasonScanPagesWithHistoryCache(t.Context(), client, "HN1", ref, scan, 2, false)
	mu.Lock()
	defer mu.Unlock()
	if len(starts) != 2 {
		t.Fatal(starts)
	}
	var first time.Time
	for _, at := range starts {
		if first.IsZero() {
			first = at
		}
		delta := at.Sub(first)
		if delta < 0 {
			delta = -delta
		}
		if delta >= 50*time.Millisecond {
			t.Fatalf("streams serialized: %v", delta)
		}
	}
	if pushed < 2 || scan.scanned != 100 {
		t.Fatal(pushed, scan.scanned)
	}
}
func TestR235RelayProbeIndependentBudgets(t *testing.T) {
	state := &riotRelayState{config: "test"}
	flight := make(chan struct{})
	state.flight = flight
	transport := r196RoundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/health" {
			time.Sleep(3 * time.Second)
			return r206RelayResponse(404, []byte(`{}`)), nil
		}
		select {
		case <-time.After(6 * time.Second):
			return r206RelayResponse(200, []byte(`{}`)), nil
		case <-r.Context().Done():
			return nil, r.Context().Err()
		}
	})
	state.probe([]string{"https://fixture.test"}, "test", flight, &http.Client{Transport: transport}, nil)
	if state.active == "" || !state.nextTry.IsZero() {
		t.Fatal(state.active, state.nextTry)
	}
}
func TestR235RelayThreeErrorsAndRecentSuccess(t *testing.T) {
	stamp := time.Now()
	s := &riotRelayState{active: "https://fixture.test", now: func() time.Time { return stamp }}
	s.succeeded(s.active)
	for i := 0; i < 2; i++ {
		s.failed("https://fixture.test")
		if s.active == "" {
			t.Fatal("early breaker")
		}
	}
	s.failed("https://fixture.test")
	if s.active != "" || s.nextTry.Sub(stamp) != 15*time.Second {
		t.Fatal(s.active, s.nextTry)
	}
}
func TestR235RatingHeaderAndBodyBudgetsAndFailureRetry(t *testing.T) {
	var calls atomic.Int32
	client := newAramkitTestClient(aramkitRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if calls.Add(1) == 1 {
			return nil, io.ErrUnexpectedEOF
		}
		select {
		case <-time.After(8 * time.Second):
		case <-r.Context().Done():
			return nil, r.Context().Err()
		}
		reader, writer := io.Pipe()
		go func() {
			time.Sleep(15 * time.Second)
			_, _ = io.WriteString(writer, `{"code":200,"data":{"rating":2350}}`)
			writer.Close()
		}()
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: reader}, nil
	}))
	client.timeout = 10 * time.Second
	client.bodyTimeout = 25 * time.Second
	if client.lookup(t.Context(), "Self", "tag").Available {
		t.Fatal("expected failure")
	}
	var event map[string]any
	client.observe = func(e map[string]any) { event = e }
	if got := client.lookup(t.Context(), "Self", "tag"); !got.Available || got.Rating != 2350 {
		t.Fatal(got)
	}
	if calls.Load() != 2 || event["ttfb_ms"].(int64) < 7900 || event["bytes"].(int) <= 0 {
		t.Fatal(calls.Load(), event)
	}
}
func TestR235ArenaResultBoundariesAndOptionalFields(t *testing.T) {
	for _, tc := range []struct {
		queue            int64
		count, placement int
		want             string
	}{{1750, 6, 3, "win"}, {1750, 6, 4, "loss"}, {1700, 8, 4, "win"}, {1700, 8, 5, "loss"}, {1750, 0, 4, "loss"}, {0, 0, 4, "unknown"}} {
		m := gameplayMatch{QueueID: tc.queue}
		for i := 0; i < tc.count; i++ {
			m.Participants = append(m.Participants, gameplayParticipant{SubteamID: int64(i + 1)})
		}
		if got := arenaPlacementResult(m, tc.placement); got != tc.want {
			t.Fatal(tc, got)
		}
	}
	var raw riotParticipant
	if err := json.Unmarshal([]byte(`{"totalHeal":"1300","damageDealtToTurrets":900,"challenges":{"soloKills":3,"knockEnemyIntoTeamAndKill":6,"killsNearEnemyTurret":4,"killsUnderOwnTurret":2,"maxCsAdvantageOnLaneOpponent":45}}`), &raw); err != nil {
		t.Fatal(err)
	}
	m := convertRiotMatchInfo(&riotMatchInfo{Participants: []riotParticipant{raw}}, "", nil, nil, "", "")
	p := m.Participants[0]
	if p.SoloKills == nil || *p.SoloKills != 3 || p.TotalHeal == nil || *p.TotalHeal != 1300 || p.MaxCsAdvantageOnLaneOpponent == nil {
		t.Fatal(p)
	}
	empty := normalizeGameplayMatch(lcuGame{Participants: []lcuParticipant{{ParticipantID: 1}}}, gameplayReference{}, nil, nil).Participants[0]
	if empty.SoloKills != nil || empty.TotalHeal != nil {
		t.Fatal("missing LCU values became zero")
	}
}
func TestR235CurrentHistorySnapshotImmediateAndSSE(t *testing.T) {
	a := r175App(t)
	ref := strings.Repeat("s", 48)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(6 * time.Second)
		io.WriteString(w, `{"games":{"gameCount":0,"games":[]}}`)
	}))
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "fixture", http: server.Client(), region: "JP", rsoPlatform: "JP1", platformProbe: true}
	a.lcu = client
	a.summoner = Summoner{PUUID: ref}
	events := make(chan string, 8)
	a.eventSubscribers = map[chan string]struct{}{events: {}}
	key := a.clientHistorySnapshotKey("jp1", "", ref)
	raw, _ := json.Marshal(clientHistorySnapshot{Matches: []gameplayMatch{{GameID: 111, Result: "win"}}, Capabilities: []EndpointCapability{{Name: "match-history", State: capabilityAvailable}}})
	if err := writeLocalStoreFile(a.storage, key, raw); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	matches, _, _, _ := a.loadCurrentHistoryFast(t.Context(), client, gameplayReference{PlayerRef: ref, Region: "jp1", ClientIdentity: true}, ref, 20, nil, nil)
	if time.Since(started) > 300*time.Millisecond || len(matches) != 1 || matches[0].GameID != 111 {
		t.Fatal(matches, time.Since(started))
	}
	deadline := time.After(8 * time.Second)
	flightKey := struct {
		a      *app
		client *LCUClient
		puuid  string
		count  int
	}{a, client, ref, 20}
	f, ok := clientHistoryFlights.Load(flightKey)
	if !ok {
		t.Fatal("history did not start")
	}
	select {
	case <-f.(*clientHistoryFlight).done:
		select {
		case event := <-events:
			var payload map[string]any
			if json.Unmarshal([]byte(event), &payload) != nil || payload["type"] != "overview-matches" || payload["historyGeneration"].(float64) <= 0 {
				t.Fatal(event)
			}
		default:
			t.Fatalf("refresh completed without SSE; result=%#v diagnostics=%#v", f.(*clientHistoryFlight).result, r175Events(t, a, "panic_recovered"))
		}
	case <-deadline:
		t.Fatal("history did not finish")
	}
}

func TestR235HistoryPublicReferencesDoNotMutateFlight(t *testing.T) {
	original := []gameplayMatch{{Participants: []gameplayParticipant{{PlayerRef: "private", reference: gameplayReference{PlayerRef: "private"}}}}}
	public := cloneClientHistoryMatches(original)
	public[0].Participants[0].PlayerRef = "public"
	if original[0].Participants[0].PlayerRef != "private" {
		t.Fatal("shared participants")
	}
}

func TestR235RelayTTFBWindowAndPathCategories(t *testing.T) {
	now := time.Now()
	state := &riotRelayState{now: func() time.Time { return now }}
	for i, ms := range []int{100, 300, 200, 400} {
		state.recordTTFB(time.Duration(ms) * time.Millisecond)
		failure := ""
		if i == 0 {
			failure = "network"
		}
		if i == 1 {
			failure = "auth"
		}
		state.recordRequest(failure, 200, "match", nil)
	}
	now = now.Add(10 * time.Minute)
	var summary map[string]any
	state.flushSummary(func(event map[string]any) { summary = event })
	if summary["ttfb_median_ms"] != float64(250) || summary["ttfb_p90_ms"] != float64(400) {
		t.Fatal(summary)
	}
	failures := summary["failures"].(map[string]any)
	if failures["network"].(map[string]int)["match"] != 1 || failures["auth"].(map[string]int)["match"] != 1 {
		t.Fatal(summary)
	}
	t.Logf("synthetic 10-minute summary: %#v", summary)
}

func TestR235HeadPersistsEachPageWithoutLosingAccumulator(t *testing.T) {
	a, client, ref, _ := r231SeasonFixture(t, 100, false)
	scan := r231Scan(ref)
	scan.headOnly = true
	scan.onPage = func(scan *seasonScanState) { a.finishSeasonScan(scan, nil, scan.cache.AccountHash) }
	a.seasonScanPagesWithHistoryCache(t.Context(), client, "HN1", ref, scan, 2, false)
	a.finishSeasonScan(scan, nil, scan.cache.AccountHash)
	if len(scan.cache.GameIDs) != 100 || seasonStatsCount(scan.cache.Stats) != 100 {
		t.Fatal(len(scan.cache.GameIDs), scan.cache.Stats)
	}
}

func TestR235FastHistoryReturnsIndependentParticipantsForSharedFlight(t *testing.T) {
	a := r175App(t)
	ref := strings.Repeat("v", 48)
	client := &LCUClient{region: "JP", rsoPlatform: "JP1"}
	key := struct {
		a      *app
		client *LCUClient
		puuid  string
		count  int
	}{a, client, ref, 20}
	flight := &clientHistoryFlight{done: make(chan struct{}), result: clientHistorySnapshot{Matches: []gameplayMatch{{GameID: 1, Participants: []gameplayParticipant{{PlayerRef: ref}}}}}}
	close(flight.done)
	clientHistoryFlights.Store(key, flight)
	defer clientHistoryFlights.Delete(key)
	first, _, _, _ := a.loadCurrentHistoryFast(t.Context(), client, gameplayReference{PlayerRef: ref}, ref, 20, nil, nil)
	first[0].Participants[0].PlayerRef = "public"
	second, _, _, _ := a.loadCurrentHistoryFast(t.Context(), client, gameplayReference{PlayerRef: ref}, ref, 20, nil, nil)
	if second[0].Participants[0].PlayerRef != ref {
		t.Fatal("shared flight result polluted")
	}
}

func TestR235SelfOverviewBypassesRelayAndColdHistory(t *testing.T) {
	for _, cached := range []bool{true, false} {
		t.Run(fmt.Sprint(cached), func(t *testing.T) {
			r206RelayFixture(t)
			a := r175App(t)
			ref := strings.Repeat("j", 48)
			champs := newChampionProvider()
			var relayCalls atomic.Int32
			champs.client = &http.Client{Transport: r196RoundTrip(func(r *http.Request) (*http.Response, error) {
				if r.URL.Host == "relay.example" {
					relayCalls.Add(1)
				}
				return r206RelayResponse(404, []byte(`{}`)), nil
			})}
			a.champions = champs
			a.riot = newRiotProvider(champs)
			a.lpTracker = newLPTracker(a.storage)
			var historyCalls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.Contains(r.URL.Path, "current-summoner/matches"):
					historyCalls.Add(1)
					time.Sleep(6 * time.Second)
					io.WriteString(w, `{"games":{"gameCount":0,"games":[]}}`)
				case strings.Contains(r.URL.Path, "lol-ranked"):
					io.WriteString(w, `{"queues":[]}`)
				default:
					io.WriteString(w, `[]`)
				}
			}))
			defer server.Close()
			client := &LCUClient{baseURL: server.URL, token: "fixture", http: server.Client(), region: "JP", rsoPlatform: "JP1", platformProbe: true}
			a.lcu = client
			a.connected = true
			a.summoner = Summoner{PUUID: ref, GameName: "Fixture", TagLine: "JP"}
			events := make(chan string, 8)
			a.eventSubscribers = map[chan string]struct{}{events: {}}
			if cached {
				raw, _ := json.Marshal(clientHistorySnapshot{Matches: []gameplayMatch{{GameID: 111, QueueID: 420, Result: "win", CreatedAt: time.Now().UnixMilli()}}, Capabilities: []EndpointCapability{{Name: "match-history", State: capabilityAvailable}}})
				if err := writeLocalStoreFile(a.storage, a.clientHistorySnapshotKey("jp1", "", ref), raw); err != nil {
					t.Fatal(err)
				}
			}
			started := time.Now()
			result := a.loadGameplayOverview(t.Context(), client, a.summoner, gameplayReference{PlayerRef: ref, Region: "jp1", ClientIdentity: true}, 0, 20, "all", false)
			elapsed := time.Since(started)
			if cached && (elapsed > 600*time.Millisecond || len(result.Matches) != 1) {
				t.Fatal(elapsed, len(result.Matches))
			}
			if !cached && elapsed > 3300*time.Millisecond {
				t.Fatal("headers waited for cold history", elapsed)
			}
			if relayCalls.Load() != 0 {
				t.Fatal("self overview waited for relay", relayCalls.Load())
			}
			timeout := time.After(8 * time.Second)
			for {
				select {
				case event := <-events:
					if strings.Contains(event, `"type":"overview-matches"`) {
						if historyCalls.Load() != 1 {
							t.Fatal("duplicate cold history", historyCalls.Load())
						}
						t.Logf("cached=%v overview=%v; history=6s; relay=%d", cached, elapsed, relayCalls.Load())
						return
					}
				case <-timeout:
					t.Fatal("missing history SSE")
				}
			}
		})
	}
}
