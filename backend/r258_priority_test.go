package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestR258TokenHotSingleFlightAndEventWins(t *testing.T) {
	var calls atomic.Int32
	entered, release := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			close(entered)
		}
		<-release
		w.Write([]byte(`{"accessToken":"old-read"}`))
	}))
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "fixture", http: server.Client()}
	p := newSGPProvider()
	results := make(chan string, 12)
	var group sync.WaitGroup
	for i := 0; i < 12; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			token, err := p.entitlementsTokenContext(t.Context(), client, false)
			if err != nil {
				t.Error(err)
			}
			results <- token
		}()
	}
	<-entered
	p.observeTokenEvent(client, LCUEvent{URI: "/entitlements/v1/token", EventType: "Update", Data: json.RawMessage(`{"accessToken":"event-token"}`)})
	close(release)
	group.Wait()
	close(results)
	for token := range results {
		if token != "event-token" {
			t.Fatal("stale token read replaced event", token)
		}
	}
	p.mu.Lock()
	p.tokenAt = time.Now().Add(-time.Hour)
	p.mu.Unlock()
	if token, err := p.entitlementsTokenContext(t.Context(), client, false); err != nil || token != "event-token" || calls.Load() != 1 {
		t.Fatal(token, err, calls.Load())
	}
	p.observeTokenEvent(client, LCUEvent{URI: "/entitlements/v1/token", EventType: "Delete"})
	if token, err := p.entitlementsTokenContext(t.Context(), client, false); err != nil || token != "old-read" || calls.Load() != 2 {
		t.Fatal(token, err, calls.Load())
	}
}

func TestR258OverviewTokenWaitIsBounded(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "fixture", http: server.Client()}
	p := newSGPProvider()
	ctx := context.WithValue(t.Context(), overviewLoadCostContextKey{}, &overviewLoadCost{})
	start := time.Now()
	if _, err := p.entitlementsTokenContext(ctx, client, false); err == nil || time.Since(start) > 2*time.Second {
		t.Fatal("unbounded token wait", err, time.Since(start))
	}
}

func TestR258ConnectionPriorityAndIdleObjective(t *testing.T) {
	client := &LCUClient{}
	a := &app{lcu: client, connected: true}
	a.gameplayFlow.client, a.gameplayFlow.phase = client, "None"
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	g := a.beginConnectionPriority(ctx, client)
	p1, p2 := make(chan struct{}), make(chan struct{})
	a.scheduleConnectionWork(ctx, client, "fixture-p1", false, func() { close(p1) })
	a.scheduleConnectionWork(ctx, client, "fixture-p2", true, func() { close(p2) })
	select {
	case <-p1:
		t.Fatal("P1 before card")
	case <-p2:
		t.Fatal("objective before thirty seconds")
	case <-time.After(30 * time.Millisecond):
	}
	a.observeConnectionFirstCard()
	select {
	case <-p1:
	case <-time.After(time.Second):
		t.Fatal("first card did not release P1")
	}
	select {
	case <-p2:
		t.Fatal("first card incorrectly released objective")
	default:
	}
	cancel()
	ctx2, cancel2 := context.WithCancel(t.Context())
	defer cancel2()
	g = a.beginConnectionPriority(ctx2, client)
	g.started = time.Now().Add(-31 * time.Second)
	a.gameplayFlow.client, a.gameplayFlow.phase = client, "ChampSelect"
	idle := make(chan struct{})
	a.scheduleConnectionWork(ctx2, client, "fixture-idle", true, func() { close(idle) })
	select {
	case <-idle:
		t.Fatal("objective during champ select")
	case <-time.After(30 * time.Millisecond):
	}
	a.gameplayFlow.mu.Lock()
	a.gameplayFlow.phase = "None"
	a.gameplayFlow.mu.Unlock()
	select {
	case <-idle:
	case <-time.After(1500 * time.Millisecond):
		t.Fatal("idle objective not released")
	}
}

func TestR258ConnectedSessionDoesNotProbeObjectiveBeforeCard(t *testing.T) {
	var probes atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/lol-missions/") || strings.HasPrefix(r.URL.Path, "/lol-notifications/") || strings.HasPrefix(r.URL.Path, "/swagger/") {
			probes.Add(1)
		}
		w.Write([]byte(`{"summonerId":1}`))
	}))
	defer server.Close()
	a := r175App(t)
	client := &LCUClient{baseURL: server.URL, token: "fixture", http: server.Client(), region: "TENCENT", rsoPlatform: "HN1", platformProbe: true}
	a.lcu, a.connected = client, true
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { defer close(done); _ = a.runConnectedSession(ctx, client) }()
	time.Sleep(100 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("session did not cancel")
	}
	if probes.Load() != 0 {
		t.Fatal("objective/swagger occupied the cold connection", probes.Load())
	}
}

func TestR258SwaggerOnlyOnExport(t *testing.T) {
	var swagger atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/swagger/") {
			swagger.Add(1)
		}
		w.Write([]byte(`{}`))
	}))
	defer server.Close()
	a := r175App(t)
	client := &LCUClient{baseURL: server.URL, token: "fixture", http: server.Client()}
	a.collectObjectiveDiagnostics(t.Context(), client, "connected")
	if swagger.Load() != 0 {
		t.Fatal("swagger on connected path")
	}
	a.collectObjectiveDiagnostics(t.Context(), client, "export")
	if swagger.Load() != 2 {
		t.Fatal("swagger export missing", swagger.Load())
	}
}

func r258HeadFixture(t *testing.T, ids []int64) (*app, *LCUClient, *[]int) {
	t.Helper()
	a, _, _ := newGameplayOverviewSGPFixture(t, false)
	client := a.lcu
	counts := []int{}
	a.sgp.http = &http.Client{Transport: sgpRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		start, _ := strconv.Atoi(r.URL.Query().Get("startIndex"))
		count, _ := strconv.Atoi(r.URL.Query().Get("count"))
		counts = append(counts, count)
		rows := []any{}
		for i := start; i < min(len(ids), start+count); i++ {
			rows = append(rows, map[string]any{"json": map[string]any{"gameId": ids[i], "queueId": 420, "gameDuration": 1800, "participants": []any{map[string]any{"participantId": 1, "puuid": a.summoner.PUUID, "teamId": 100, "championId": 61}}}})
		}
		return r178JSON(map[string]any{"games": rows}, 200), nil
	})}
	return a, client, &counts
}

func TestR258HeadHitAndObservedDelta(t *testing.T) {
	for _, newer := range []bool{false, true} {
		t.Run(strconv.FormatBool(newer), func(t *testing.T) {
			ids := []int64{500, 400, 300}
			if newer {
				ids = append([]int64{999999, 900000}, ids...)
			}
			a, client, counts := r258HeadFixture(t, ids)
			old := clientHistorySnapshot{Matches: []gameplayMatch{{GameID: 500}, {GameID: 400}, {GameID: 300}}, Capabilities: []EndpointCapability{{Name: "match-history", State: capabilityAvailable}}, Pagination: gameplayPagination{Count: 3, Filter: "all"}}
			ctx := context.WithValue(t.Context(), overviewLoadCostContextKey{}, &overviewLoadCost{})
			got := a.refreshCurrentHistorySnapshot(ctx, client, gameplayReference{ServerID: "HN1"}, a.summoner.PUUID, 3, old, nil, nil)
			if !newer {
				if len(*counts) != 1 || (*counts)[0] != 1 || got.Matches[0].GameID != 500 {
					t.Fatal(*counts, got.Matches)
				}
			} else {
				if len(*counts) != 4 || (*counts)[3] != 2 || got.Matches[0].GameID != 999999 || got.Matches[2].GameID != 500 {
					t.Fatal("delta must equal observed two rows, not game ID subtraction", *counts, got.Matches)
				}
			}
		})
	}
}

func TestR258ExpiredHistoryUsesOneHeadProbe(t *testing.T) {
	a, client, counts := r258HeadFixture(t, []int64{500, 400, 300})
	a.sgp.cacheHistoryPage("HN1", a.summoner.PUUID, 0, 3, nil, sgpHistoryPage{games: []*riotMatchInfo{{GameID: 500}, {GameID: 400}, {GameID: 300}}, consumed: 3, more: true})
	key := sgpHistoryPageCacheKey("HN1", a.summoner.PUUID, 0, 3, nil)
	a.sgp.mu.Lock()
	entry := a.sgp.historyCache[key]
	entry.at = time.Now().Add(-10 * time.Minute)
	a.sgp.historyCache[key] = entry
	a.sgp.mu.Unlock()
	games, _, _, err := a.sgp.matchHistoryOn(t.Context(), client, "HN1", a.summoner.PUUID, 0, 3, true)
	if err != nil || len(games) != 3 || len(*counts) != 1 || (*counts)[0] != 1 {
		t.Fatal(err, len(games), *counts)
	}
}

func TestR258HeadMarkerIncludesGamesAbsentFromVisibleList(t *testing.T) {
	a, client, counts := r258HeadFixture(t, []int64{999999, 500})
	old := clientHistorySnapshot{Matches: []gameplayMatch{{GameID: 500}}, HeadGameID: 999999, Capabilities: []EndpointCapability{{Name: "match-history", State: capabilityAvailable}}}
	ctx := context.WithValue(t.Context(), overviewLoadCostContextKey{}, &overviewLoadCost{})
	got := a.refreshCurrentHistorySnapshot(ctx, client, gameplayReference{ServerID: "HN1"}, a.summoner.PUUID, 20, old, nil, nil)
	if len(*counts) != 1 || got.HeadGameID != 999999 || got.Matches[0].GameID != 500 {
		t.Fatal("non-displayable game made unchanged head cold", *counts, got)
	}
}

func TestR258TencentSnapshotImmediateAndAccountScoped(t *testing.T) {
	a, client, _ := r258HeadFixture(t, []int64{500})
	a.storage = r175App(t).storage
	t.Cleanup(func() {
		a.mu.Lock()
		a.connected = false
		a.mu.Unlock()
		var done []chan struct{}
		clientHistoryFlights.Range(func(key, value any) bool {
			if k, ok := key.(struct {
				a      *app
				client *LCUClient
				puuid  string
				count  int
			}); ok && k.a == a {
				done = append(done, value.(*clientHistoryFlight).done)
			}
			return true
		})
		for _, ch := range done {
			select {
			case <-ch:
			case <-time.After(2 * time.Second):
				t.Error("history background did not finish before TempDir cleanup")
			}
		}
	})
	a.connected = true
	ref := gameplayReference{ServerID: "HN1", PlayerRef: a.summoner.PUUID}
	key := a.clientHistorySnapshotKey("", ref.ServerID, ref.PlayerRef)
	if key == a.clientHistorySnapshotKey("", "HN10", ref.PlayerRef) || key == a.clientHistorySnapshotKey("", "HN1", r161Ref(9)) {
		t.Fatal("snapshot scope collision")
	}
	raw, _ := json.Marshal(clientHistorySnapshot{Matches: []gameplayMatch{{GameID: 500}}, Capabilities: []EndpointCapability{{Name: "match-history", State: capabilityAvailable}}})
	if err := writeLocalStoreFile(a.storage, key, raw); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	g := a.beginConnectionPriority(ctx, client)
	_ = g
	start := time.Now()
	matches, _, _, _ := a.loadCurrentHistoryFast(ctx, client, ref, ref.PlayerRef, 20, nil, nil)
	if len(matches) != 1 || matches[0].GameID != 500 || time.Since(start) > 100*time.Millisecond {
		t.Fatal("Tencent snapshot missed", matches, time.Since(start))
	}
}

func TestR258VersionedCatalogSurvivesReconnectAndRestart(t *testing.T) {
	var reads atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reads.Add(1)
		w.Write([]byte(`[{"id":1,"name":"fixture"}]`))
	}))
	defer server.Close()
	a := r175App(t)
	makeClient := func(version string) *LCUClient {
		return &LCUClient{baseURL: server.URL, token: "fixture", http: server.Client(), region: "TENCENT", rsoPlatform: "HN1", gameVersion: version}
	}
	for _, appInstance := range []*app{a, a, {storage: a.storage}} {
		for _, path := range []string{"/lol-game-data/assets/v1/items.json", "/lol-game-data/assets/v1/perkstyles.json", "/lol-game-data/assets/v1/perks.json", "/lol-game-data/assets/v1/summoner-spells.json"} {
			if _, err := appInstance.clientCatalogBytes(t.Context(), makeClient("26.19.1"), path); err != nil {
				t.Fatal(err)
			}
		}
	}
	if reads.Load() != 4 {
		t.Fatal("reconnect reread directories", reads.Load())
	}
	if _, err := a.clientCatalogBytes(t.Context(), makeClient("26.20.1"), "/lol-game-data/assets/v1/items.json"); err != nil || reads.Load() != 5 {
		t.Fatal("version did not invalidate", err, reads.Load())
	}
}

func TestR258TencentOverviewFirstCardUsesSnapshotOrOnePage(t *testing.T) {
	for _, cached := range []bool{true, false} {
		t.Run(strconv.FormatBool(cached), func(t *testing.T) {
			a, _, requests := newGameplayOverviewSGPFixture(t, true)
			a.storage = r175App(t).storage
			waitGameplaySeasonJobsBeforeCleanup(t, a)
			client := a.lcu
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			a.beginConnectionPriority(ctx, client)
			if cached {
				raw, _ := json.Marshal(clientHistorySnapshot{Matches: []gameplayMatch{{GameID: 420, QueueID: 420}}, Capabilities: []EndpointCapability{{Name: "match-history", State: capabilityAvailable}}, Pagination: gameplayPagination{Count: 1, Filter: "all"}})
				if err := writeLocalStoreFile(a.storage, a.clientHistorySnapshotKey("", "HN1", a.summoner.PUUID), raw); err != nil {
					t.Fatal(err)
				}
			}
			cards := 0
			cost := &overviewLoadCost{}
			ctx = context.WithValue(ctx, overviewLoadCostContextKey{}, cost)
			ctx = context.WithValue(ctx, localOverviewProgressKey{}, func(partial gameplayOverview) {
				cards++
				if len(partial.Matches) != 1 {
					t.Error("first card missing", partial)
				}
				if requests.Load() > 1 {
					t.Error("ranked or season pages ran before first card", requests.Load())
				}
				if cached {
					a.coldLaunch.mu.Lock()
					source := a.coldLaunch.overviewSource
					a.coldLaunch.mu.Unlock()
					if source != "snapshot" {
						t.Error("Tencent bypassed snapshot", source)
					}
				}
				a.observeConnectionFirstCard()
			})
			got := a.loadGameplayOverview(ctx, client, a.summoner, gameplayReference{ServerID: "HN1"}, 0, 20, "all", false)
			if cards != 1 || len(got.Matches) != 1 {
				t.Fatal(cards, len(got.Matches))
			}
		})
	}
}

func TestR258FallbackDoesNotPoisonVersionedPerks(t *testing.T) {
	a := r175App(t)
	key := "lcu-versioned|26.19.1|TENCENT|HN1"
	fallback := func() (gameplayPerkCatalogResponse, error) {
		return gameplayPerkCatalogResponse{source: "ddragon", Perks: []gameplayPerk{{ID: 1}}}, nil
	}
	if value, err := a.cachedGameplayPerkCatalog(t.Context(), key, fallback); err != nil || value.source != "ddragon" {
		t.Fatal(value, err)
	}
	var calls int
	value, err := a.cachedGameplayPerkCatalog(t.Context(), key, func() (gameplayPerkCatalogResponse, error) {
		calls++
		return gameplayPerkCatalogResponse{source: "lcu", Perks: []gameplayPerk{{ID: 2}}}, nil
	})
	if err != nil || calls != 1 || value.Perks[0].ID != 2 {
		t.Fatal("fallback poisoned client cache", value, err, calls)
	}
}
