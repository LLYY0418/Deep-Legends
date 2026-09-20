package main

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestOverviewQuerySnapshotKeyIncludesAllRequestDimensions(t *testing.T) {
	player := strings.Repeat("p", 48)
	localClient := &LCUClient{region: "TENCENT", rsoPlatform: "HN1", platformProbe: true}
	local := overviewQuerySnapshotKey(localClient, gameplayReference{ServerID: "HN1", Privacy: "PUBLIC"}, player, false, 0, 20, "all")
	remote := overviewQuerySnapshotKey(localClient, gameplayReference{ServerID: "HN10", Privacy: "PUBLIC"}, player, false, 0, 20, "all")
	nextPage := overviewQuerySnapshotKey(localClient, gameplayReference{ServerID: "HN1", Privacy: "PUBLIC"}, player, false, 20, 20, "all")
	private := overviewQuerySnapshotKey(localClient, gameplayReference{ServerID: "HN1", Privacy: "PRIVATE"}, player, false, 0, 20, "all")
	if local == remote || local == nextPage || local == private {
		t.Fatalf("overview query keys collapsed dimensions: local=%q remote=%q next=%q private=%q", local, remote, nextPage, private)
	}
	if !strings.HasPrefix(local, "sgp+lcu:") || !strings.HasPrefix(remote, "sgp:") {
		t.Fatalf("overview query keys omitted source: local=%q remote=%q", local, remote)
	}
}

func TestOverviewQuerySnapshotReusesIdenticalResponse(t *testing.T) {
	player := strings.Repeat("q", 48)
	client := &LCUClient{region: "TENCENT", rsoPlatform: "HN1", platformProbe: true}
	reference := gameplayReference{PlayerRef: player, ServerID: "HN1"}
	cache := newOverviewQueryCache()
	key := overviewQuerySnapshotKey(client, reference, player, true, 0, 20, "all")
	cache.putLocked(key, overviewQueryCacheEntry{at: time.Now(), response: gameplayOverview{Player: gameplayPlayer{DisplayName: "cached"}}})
	a := &app{overviewQueries: cache}
	response := a.loadGameplayOverviewDeduplicated(context.Background(), client, Summoner{PUUID: player}, reference, 0, 20, "all", false)
	if response.Player.DisplayName != "cached" {
		t.Fatalf("identical query did not reuse snapshot: %#v", response)
	}
}

func TestOverviewQueryCacheRemovesExpiredEntriesAndCapsCapacity(t *testing.T) {
	cache := newOverviewQueryCache()
	now := time.Now()
	cache.putLocked("expired", overviewQueryCacheEntry{at: now.Add(-overviewQuerySnapshotTTL - time.Millisecond)})
	cache.putLocked("fresh", overviewQueryCacheEntry{at: now})
	if _, ok := cache.entries["expired"]; ok {
		t.Fatal("expired overview entry survived the next cache write")
	}
	flight := &overviewQueryFlight{done: make(chan struct{})}
	cache.flights["in-flight"] = flight
	for index := 0; index < overviewQueryCacheMax+32; index++ {
		key := "player-" + strconv.Itoa(index)
		cache.putLocked(key, overviewQueryCacheEntry{at: now.Add(time.Duration(index) * time.Nanosecond)})
	}
	if len(cache.entries) != overviewQueryCacheMax || cache.recent.Len() != overviewQueryCacheMax {
		t.Fatalf("overview cache size = entries %d, order %d, want %d", len(cache.entries), cache.recent.Len(), overviewQueryCacheMax)
	}
	if _, ok := cache.entries["player-0"]; ok {
		t.Fatal("least-recent overview entry was not evicted")
	}
	if cache.flights["in-flight"] != flight {
		t.Fatal("entry eviction disturbed an active overview flight")
	}
}

func TestOverviewQueryCacheRefreshesExistingEntryWithoutGrowing(t *testing.T) {
	cache := newOverviewQueryCache()
	cache.putLocked("same", overviewQueryCacheEntry{at: time.Now(), response: gameplayOverview{Player: gameplayPlayer{DisplayName: "first"}}})
	cache.putLocked("same", overviewQueryCacheEntry{at: time.Now(), response: gameplayOverview{Player: gameplayPlayer{DisplayName: "second"}}})
	if len(cache.entries) != 1 || cache.recent.Len() != 1 {
		t.Fatalf("refreshing one overview key grew the cache: entries=%d order=%d", len(cache.entries), cache.recent.Len())
	}
	entry, ok := cache.getLocked("same", time.Now())
	if !ok || entry.response.Player.DisplayName != "second" {
		t.Fatalf("refreshed overview entry = %#v, ok=%v", entry, ok)
	}
}

func TestCanceledOverviewOwnerDoesNotPopulateSnapshot(t *testing.T) {
	cache := newOverviewQueryCache()
	flight := &overviewQueryFlight{done: make(chan struct{})}
	cache.flights["sgp+lcu:player"] = flight
	cache.complete("sgp+lcu:player", flight, gameplayOverview{Player: gameplayPlayer{DisplayName: "partial"}}, context.Canceled)
	if _, ok := cache.entries["sgp+lcu:player"]; ok {
		t.Fatal("canceled overview owner cached a partial response")
	}
	if !isCancellation(flight.err) {
		t.Fatalf("flight error = %v, want cancellation", flight.err)
	}
}

func TestSeasonQuerySnapshotKeyIncludesSourceAndQueueScope(t *testing.T) {
	key := seasonQuerySnapshotKey("hn1", strings.Repeat("s", 48), "S26")
	if !strings.HasPrefix(key, "sgp:") || !strings.Contains(key, "queues=420,440") {
		t.Fatalf("season query key = %q", key)
	}
}

func TestRiotOverviewQuerySnapshotKeyIncludesSourceAndPagination(t *testing.T) {
	reference := gameplayReference{GameName: "Player", TagLine: "KR1", Region: riotRegionKR}
	first := riotOverviewQuerySnapshotKey(reference, 0, 20)
	next := riotOverviewQuerySnapshotKey(reference, 20, 20)
	if !strings.HasPrefix(first, "riot:") || first == next {
		t.Fatalf("Riot overview query keys: first=%q next=%q", first, next)
	}
}
