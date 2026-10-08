package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestR241HeadFreshnessSmallRequestNewGamesAndRecentGate(t *testing.T) {
	ref := strings.Repeat("h", 48)
	var mu sync.Mutex
	ids := []int64{24102, 24101}
	requests := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		start, _ := strconv.Atoi(q.Get("startIndex"))
		count, _ := strconv.Atoi(q.Get("count"))
		tags := q["tag"]
		mu.Lock()
		defer mu.Unlock()
		requests = append(requests, strings.Join(tags, ",")+":"+strconv.Itoa(start)+":"+strconv.Itoa(count))
		games := []any{}
		if len(tags) == 0 || tags[0] == "q_420" {
			for i := start; i < len(ids) && i < start+count; i++ {
				games = append(games, map[string]any{"json": map[string]any{"gameId": ids[i], "queueId": 420, "mapId": 11, "gameCreation": time.Now().UnixMilli(), "gameDuration": 1800, "padding": strings.Repeat("x", 1000), "participants": []any{map[string]any{"participantId": 1, "puuid": ref, "teamId": 100, "championId": 13, "win": true}}}})
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"games": games})
	}))
	defer server.Close()
	a := r175App(t)
	client := &LCUClient{region: "TENCENT", rsoPlatform: "HN1"}
	a.sgp = newSGPProvider()
	a.sgp.http = server.Client()
	a.sgp.serverBases["HN1"] = server.URL
	a.sgp.token = "fixture"
	a.sgp.tokenAt = time.Now()
	a.sgp.tokenClient = client
	player := Summoner{PUUID: ref}
	reference := gameplayReference{ServerID: "HN1", PlayerRef: ref}
	run := func() (seasonStatsProgress, bool, string, int, int) {
		cost := &overviewLoadCost{}
		ctx := context.WithValue(t.Context(), overviewLoadCostContextKey{}, cost)
		p, s, r, n := a.refreshSeasonHead(ctx, client, reference, player, ref, nil)
		_, bytes, _, _ := cost.snapshot()
		return p, s, r, n, bytes
	}
	p, skipped, _, added, before := run()
	if !p.Complete || skipped || added != 2 {
		t.Fatal(p, skipped, added)
	}
	mu.Lock()
	requests = nil
	mu.Unlock()
	p, skipped, reason, added, after := run()
	if !p.Complete || !skipped || reason != "no_new_game" || added != 0 {
		t.Fatal(p, skipped, reason, added)
	}
	mu.Lock()
	if len(requests) != 1 || requests[0] != ":0:1" {
		t.Fatal(requests)
	}
	requests = nil
	ids = append([]int64{24103}, ids...)
	mu.Unlock()
	p, skipped, _, added, newBytes := run()
	if !p.Complete || skipped || added != 1 || p.Scanned != 3 {
		t.Fatal(p, skipped, added)
	}
	mu.Lock()
	if strings.Join(requests, ";") != ":0:1;q_420,q_440:0:1;q_2300,q_2400,q_3270:0:1;q_420,q_440:1:1" && strings.Join(requests, ";") != ":0:1;q_2300,q_2400,q_3270:0:1;q_420,q_440:0:1;q_420,q_440:1:1" {
		t.Fatal(requests)
	}
	requests = nil
	mu.Unlock()
	// Repeated automatic opens remain request-free for 60s; manual refresh is
	// covered by TestR252ManualSeasonHeadBypassesRecentGateAndStillProbes.
	a.startSeasonStatsRefresh(client, reference, player, ref, nil, false)
	a.startSeasonStatsRefresh(client, reference, player, ref, nil, false)
	mu.Lock()
	if len(requests) != 0 {
		t.Fatal(requests)
	}
	mu.Unlock()
	events := r175Events(t, a, "season_stats_head_refresh")
	if len(events) != 2 || events[0]["skip_reason"] != "recent" || events[0]["sgp_bytes"] != float64(0) || events[0]["new_games"] != float64(0) {
		t.Fatal(events)
	}
	t.Logf("head fixture bytes cold=%d no_new=%d new_one=%d recent=0", before, after, newBytes)
	// Background writes merge their games without discarding a newer head marker.
	season, _ := currentRankedSeason(time.Now())
	hash := a.storage.accountHash(player)
	cache, err := a.storage.loadSeasonStats(seasonStatsSource, hash, season)
	if err != nil {
		t.Fatal(err)
	}
	old := cache
	old.HeadCheckedAt = time.Time{}
	old.HeadGameID = 0
	var group sync.WaitGroup
	group.Add(2)
	go func() {
		defer group.Done()
		if _, err := a.storage.saveSeasonStatsReported(old); err != nil {
			t.Error(err)
		}
	}()
	go func() {
		defer group.Done()
		if err := a.storage.saveSeasonHeadMarker(seasonStatsSource, hash, season, 24103, time.Now()); err != nil {
			t.Error(err)
		}
	}()
	group.Wait()
	cache, err = a.storage.loadSeasonStats(seasonStatsSource, hash, season)
	if err != nil || cache.HeadGameID != 24103 || len(cache.GameIDs) != 3 {
		t.Fatal(cache, err)
	}
}

func TestR241CollectionRetriesSamplesPersistentNegativeTTLAndCatalogInvalidation(t *testing.T) {
	a := r175App(t)
	client := &LCUClient{}
	pending := AccountData{Loot: []LootItem{{Blank: true, DataPending: true, Kind: "类型未知", Count: 74, catalogRevision: "catalog-a", rawFieldKeys: []string{"count", "disenchantValue"}}}}
	a.lcu = client
	a.connected = true
	a.account = cloneAccountData(pending)
	a.refreshRequests = make(chan struct{}, 1)
	for i := 0; i < 3; i++ {
		a.scheduleCollectionDataRetry(client, pending)
		if a.collectionDataRetry == nil {
			t.Fatal("retry missing", i)
		}
		a.collectionDataRetry.Reset(time.Millisecond)
		select {
		case <-a.refreshRequests:
		case <-time.After(time.Second):
			t.Fatal("retry timeout")
		}
		a.mu.Lock()
		a.collectionRefreshPending = false
		a.mu.Unlock()
	}
	a.scheduleCollectionDataRetry(client, pending)
	events := r175Events(t, a, "collection_data_retry_exhausted")
	if len(events) != 1 {
		t.Fatal(events)
	}
	event := events[0]
	kinds := event["blank_kinds"].(map[string]any)
	samples := event["blank_samples"].([]any)
	if event["attempts"] != float64(3) || event["last_error_kind"] != "empty_identity" || kinds["类型未知"] != float64(1) || len(samples) != 1 || samples[0].(map[string]any)["id"] != float64(0) {
		t.Fatal(event)
	}
	t.Logf("diagnostic fixture: %v", event)
	// Simulate a process restart with only the existing on-disk store.
	b := &app{storage: &localStore{root: a.storage.root}, lcu: client, connected: true, account: cloneAccountData(pending)}
	b.scheduleCollectionDataRetry(client, pending)
	if b.collectionDataRetry != nil || b.collectionDataRetryCount != 0 || b.account.Loot[0].DataPending || b.account.Loot[0].Count != 74 {
		t.Fatal(b.account, b.collectionDataRetryCount)
	}
	if _, n := b.storage.suppressCachedCollectionBlanks(pending, time.Now().Add(time.Hour+time.Second)); n != 0 {
		t.Fatal("expired record suppressed")
	}
	changed := cloneAccountData(pending)
	changed.Loot[0].catalogRevision = "catalog-b"
	if _, n := b.storage.suppressCachedCollectionBlanks(changed, time.Now()); n != 0 {
		t.Fatal("new catalog suppressed")
	}
	data, err := os.ReadFile(filepath.Join(a.storage.root, collectionRetryCacheFile))
	if err != nil {
		t.Fatal(err)
	}
	var entries map[string]time.Time
	if json.Unmarshal(data, &entries) != nil || len(entries) != 1 {
		t.Fatal(string(data))
	}
	for key := range entries {
		if len(key) != 64 {
			t.Fatal("stored un-hashed identity", key)
		}
	}
	if collectionCatalogRevision([]Skin{{ID: 1, Name: "public", Owned: true}}, nil) != collectionCatalogRevision([]Skin{{ID: 1, Name: "public", Owned: false}}, nil) {
		t.Fatal("ownership included in catalog version")
	}
	if collectionCatalogRevision(nil, map[string]lootMetadata{"one": {Name: "old"}}) == collectionCatalogRevision(nil, map[string]lootMetadata{"one": {Name: "new"}}) {
		t.Fatal("catalog change not detected")
	}
}

func TestR241CanceledHeadDoesNotAdvanceFreshnessMarker(t *testing.T) {
	a, client, ref, id, _ := r214HistoryFixture(t)
	player := Summoner{PUUID: ref}
	reference := gameplayReference{ServerID: "HN1", PlayerRef: ref}
	a.loadSeasonChampionStatsWithHistoryCache(t.Context(), client, reference, player, ref, nil, false)
	id.Store(24104)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	original := a.sgp.http.Transport
	a.sgp.http.Transport = gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if len(r.URL.Query()["tag"]) > 0 {
			cancel()
			return nil, ctx.Err()
		}
		return original.RoundTrip(r)
	})
	progress, skipped, _, _ := a.refreshSeasonHead(ctx, client, reference, player, ref, nil)
	if skipped || progress.Message != "赛季统计暂时中断，已保留当前进度" {
		t.Fatal(progress, skipped)
	}
	season, _ := currentRankedSeason(time.Now())
	cache, err := a.storage.loadSeasonStats(seasonStatsSource, a.storage.accountHash(player), season)
	if err != nil || !cache.HeadCheckedAt.IsZero() || cache.HeadGameID != 0 || len(cache.GameIDs) != 1 {
		t.Fatal(cache, err)
	}
}
