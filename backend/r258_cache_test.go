package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestR258OverviewInvalidationKeepsOtherAccount(t *testing.T) {
	a := &app{overviewQueries: newOverviewQueryCache()}
	c := a.overviewQueries
	self := sourceScopedKey(dataSourceSGP+"+"+dataSourceLCU, "HN1|self|true|0|20|all|")
	other := sourceScopedKey(dataSourceSGP+"+"+dataSourceLCU, "HN1|other|false|0|20|all|")
	old := &overviewQueryFlight{done: make(chan struct{})}
	peer := &overviewQueryFlight{done: make(chan struct{})}
	c.putLocked(self, overviewQueryCacheEntry{at: time.Now()})
	c.putLocked(other, overviewQueryCacheEntry{at: time.Now()})
	c.flights[self] = old
	c.flights[other] = peer
	a.clearOverviewQuerySnapshots("self")
	if c.entries[other] == nil || c.flights[other] != peer {
		t.Fatal("A progress/disconnect invalidated B's snapshot or flight")
	}
	if c.entries[self] != nil || c.flights[self] != nil {
		t.Fatal("self remained cached")
	}
	c.complete(self, old, gameplayOverview{}, nil)
	if c.entries[self] != nil {
		t.Fatal("late self flight repopulated invalidated entry")
	}
}

func TestR258ImagesSurviveSameVersionReconnectAndClearOnVersionChange(t *testing.T) {
	a, client, _ := r258ViewFixture()
	a.noteAssetClientVersion(client, "26.19.1")
	a.storeAssetLocked("fixture-image", []byte("original"))
	a.markDisconnected("fixture")
	var version = "26.19.1"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _ = json.NewEncoder(w).Encode(version) }))
	defer server.Close()
	next := &LCUClient{http: server.Client(), baseURL: server.URL, token: "fixture", region: "TENCENT", rsoPlatform: "HN10"}
	a.lcu = next
	a.connected = true
	a.warmClientCatalogVersion(context.Background(), next)
	var loads int
	read := func() []byte {
		t.Helper()
		data, err := a.loadAsset(context.Background(), "fixture-image", 100, 0, func(context.Context) ([]byte, error) { loads++; return []byte("updated"), nil })
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	if got := string(read()); got != "original" || loads != 0 {
		t.Fatalf("reconnect=%s downloads=%d", got, loads)
	}
	a.observeClientCatalogVersion(client, LCUEvent{URI: "/lol-patch/v1/game-version", Data: json.RawMessage(`"stale-old-session"`)})
	if string(read()) != "original" {
		t.Fatal("old session evicted current images")
	}
	version = "26.20.1"
	a.observeClientCatalogVersion(next, LCUEvent{URI: "/lol-patch/v1/game-version", Data: json.RawMessage(`"26.20.1"`)})
	if got := string(read()); got != "updated" || loads != 1 {
		t.Fatalf("version changed=%s downloads=%d", got, loads)
	}
}

func TestR258LCUOverviewAndDetailShareImmutableDownload(t *testing.T) {
	var details atomic.Int32
	complete := lcuGame{GameID: 77, QueueID: 420, Participants: make([]lcuParticipant, 10), ParticipantIdentities: make([]lcuParticipantIdentity, 10), Teams: []lcuTeam{{TeamID: 100}}}
	complete.Participants[0].Stats.Kills = 7
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/lol-match-history/v1/games/77" {
			details.Add(1)
			time.Sleep(15 * time.Millisecond)
			_ = json.NewEncoder(w).Encode(complete)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/matches") {
			var page lcuMatchHistory
			page.Games.Games = []lcuGame{{GameID: 77, QueueID: 420, Participants: make([]lcuParticipant, 1)}}
			_ = json.NewEncoder(w).Encode(page)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	client := &LCUClient{http: server.Client(), baseURL: server.URL, token: "fixture", region: "TENCENT", rsoPlatform: "HN10"}
	games, _, _ := loadGameplayHistoryContext(context.Background(), client, "self", true, 0, 1, true)
	if len(games) != 1 || len(games[0].Participants) != 10 {
		t.Fatalf("overview=%+v", games)
	}
	games[0].Participants[0].Stats.Kills = 99
	detail, err := client.cachedHistoryDetail(context.Background(), 77)
	if err != nil || detail.Participants[0].Stats.Kills != 7 {
		t.Fatalf("detail changed: %+v %v", detail, err)
	}
	games, _, _ = loadGameplayHistoryContext(context.Background(), client, "other", false, 0, 1, true)
	if details.Load() != 1 || games[0].Participants[0].Stats.Kills != 7 {
		t.Fatalf("downloads=%d", details.Load())
	}
}

func TestR258LCUConcurrentDetailReadsSingleFlight(t *testing.T) {
	var reads atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reads.Add(1)
		time.Sleep(20 * time.Millisecond)
		_ = json.NewEncoder(w).Encode(lcuGame{GameID: 88, QueueID: 420, Participants: make([]lcuParticipant, 10), ParticipantIdentities: make([]lcuParticipantIdentity, 10), Teams: []lcuTeam{{TeamID: 100}}})
	}))
	defer server.Close()
	client := &LCUClient{http: server.Client(), baseURL: server.URL, token: "fixture", region: "TENCENT", rsoPlatform: "HN10"}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			game, err := client.cachedHistoryDetail(context.Background(), 88)
			if err != nil || game.GameID != 88 {
				t.Errorf("detail=%+v err=%v", game, err)
			}
		}()
	}
	wg.Wait()
	if reads.Load() != 1 {
		t.Fatalf("parallel downloads=%d", reads.Load())
	}
}

func TestR258GameCacheCopiesOptionalFieldsAndScopesIDs(t *testing.T) {
	cache := newHistoryGameCache()
	n := int64(5)
	game := &riotMatchInfo{GameID: 11, QueueID: 420, Participants: make([]riotParticipant, 10), PerkStatsStale: true}
	game.Participants[0].scoreMissing = map[string]bool{"gold": true}
	game.Participants[0].TripleKills = &lenientInt{}
	game.Participants[0].TripleKills.UnmarshalJSON([]byte(`2`))
	game.Participants[0].Perks.Styles = make([]riotPerkSelections, 1)
	game.Participants[0].Perks.Styles[0].Style = n
	cache.putSGP("HN10", game)
	game.Participants[0].scoreMissing["gold"] = false
	game.Participants[0].Perks.Styles[0].Style = 999
	first, ok := cache.sgp("HN10", 11)
	if !ok || !first.PerkStatsStale || !first.Participants[0].scoreMissing["gold"] || first.Participants[0].Perks.Styles[0].Style != 5 || *historyIntValue(first.Participants[0].TripleKills) != 2 {
		t.Fatal("copy lost optional values/missingness")
	}
	first.Participants[0].scoreMissing["gold"] = false
	*first.Participants[0].TripleKills.value = 99
	second, _ := cache.sgp("HN10", 11)
	if !second.Participants[0].scoreMissing["gold"] || *historyIntValue(second.Participants[0].TripleKills) != 2 {
		t.Fatal("caller changed immutable cache")
	}
	if _, ok := cache.sgp("HN1", 11); ok {
		t.Fatal("same ID crossed servers")
	}
	cache.putLCU("TENCENT|HN10", lcuGame{GameID: 11})
	if len(cache.entries) != 2 {
		t.Fatal("source shapes collided")
	}
}

func TestR258PageIndexMissesAfterGameLRUEviction(t *testing.T) {
	p := newSGPProvider()
	p.cacheHistoryPage("HN10", "self", 0, 1, nil, sgpHistoryPage{games: []*riotMatchInfo{{GameID: 1}}, consumed: 1})
	p.mu.Lock()
	cache := p.historyGamesLocked()
	p.mu.Unlock()
	for id := int64(2); id <= historyGameCacheMax+1; id++ {
		cache.putSGP("HN10", &riotMatchInfo{GameID: id})
	}
	if len(cache.entries) != 512 {
		t.Fatalf("LRU size=%d", len(cache.entries))
	}
	if _, _, ok := p.cachedHistoryPage("HN10", "self", 0, 1, nil); ok {
		t.Fatal("index returned a missing game as a valid page")
	}
}

func TestR258PartialGameIndexCannotBorrowAnotherSubjectsSummary(t *testing.T) {
	p := newSGPProvider()
	first := &riotMatchInfo{GameID: 123, QueueID: 420, Participants: []riotParticipant{{PUUID: "self", Kills: 7}}}
	second := &riotMatchInfo{GameID: 123, QueueID: 420, Participants: []riotParticipant{{PUUID: "other", Kills: 9}}}
	p.cacheHistoryPage("hn10", "self", 0, 1, nil, sgpHistoryPage{games: []*riotMatchInfo{first}, consumed: 1})
	if page, ok := p.retainedHistoryPage("HN10", "self", 1); !ok || page.games[0].Participants[0].Kills != 7 {
		t.Fatal("server alias did not match")
	}
	p.cacheHistoryPage("HN10", "other", 0, 1, nil, sgpHistoryPage{games: []*riotMatchInfo{second}, consumed: 1})
	if _, _, ok := p.cachedHistoryPage("HN10", "self", 0, 1, nil); ok {
		t.Fatal("self page borrowed another subject's partial roster")
	}
	if page, _, ok := p.cachedHistoryPage("HN10", "other", 0, 1, nil); !ok || page.games[0].Participants[0].Kills != 9 {
		t.Fatal("actual partial subject was unavailable")
	}
}

func TestR258HistoryDetailPanicReleasesFlightAndAllowsRetry(t *testing.T) {
	var calls atomic.Int32
	client := &LCUClient{baseURL: "https://fixture.invalid", token: "fixture", region: "TENCENT", rsoPlatform: "HN10", http: &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if calls.Add(1) == 1 {
			panic("fixture")
		}
		return r178JSON(lcuGame{GameID: 9, QueueID: 420, Participants: make([]lcuParticipant, 10), ParticipantIdentities: make([]lcuParticipantIdentity, 10), Teams: []lcuTeam{{TeamID: 100}}}, 200), nil
	})}}
	if _, err := client.cachedHistoryDetail(context.Background(), 9); err == nil {
		t.Fatal("panic became false success")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if game, err := client.cachedHistoryDetail(ctx, 9); err != nil || game.GameID != 9 {
		t.Fatalf("flight stranded: %+v %v", game, err)
	}
}
