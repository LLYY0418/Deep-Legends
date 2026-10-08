package main

import (
	"fmt"
	"testing"
	"time"
)

func TestRankScoreCacheUsesBoundedLRUForLargeInsertions(t *testing.T) {
	cache := newRankScoreCache()
	now := time.Now()
	for index := 0; index < 5000; index++ {
		cache.mu.Lock()
		cache.putLocked(fmt.Sprintf("player-%d", index), rankScoreEntry{score: index, known: true, at: now})
		cache.mu.Unlock()
	}
	if got := len(cache.entries); got != rankScoreCacheMax {
		t.Fatalf("cache size = %d, want %d", got, rankScoreCacheMax)
	}
	if got := cache.recent.Len(); got != rankScoreCacheMax {
		t.Fatalf("LRU size = %d, want %d", got, rankScoreCacheMax)
	}
	if want := uint64(5000 - rankScoreCacheMax); cache.evictionSteps != want {
		t.Fatalf("eviction work = %d, want one O(1) unlink per eviction (%d)", cache.evictionSteps, want)
	}
	if _, ok := cache.get("player-0"); ok {
		t.Fatal("oldest entry survived bounded eviction")
	}
	if entry, ok := cache.get("player-4999"); !ok || entry.score != 4999 {
		t.Fatalf("newest entry = %#v ok=%v", entry, ok)
	}
}

func TestRankScoreCacheGetKeepsHotEntry(t *testing.T) {
	cache := newRankScoreCache()
	now := time.Now()
	for index := 0; index < rankScoreCacheMax; index++ {
		cache.mu.Lock()
		cache.putLocked(fmt.Sprintf("player-%d", index), rankScoreEntry{score: index, known: true, at: now})
		cache.mu.Unlock()
	}
	if _, ok := cache.get("player-0"); !ok {
		t.Fatal("hot entry missing before eviction")
	}
	cache.mu.Lock()
	cache.putLocked("new-player", rankScoreEntry{score: 9999, known: true, at: now})
	cache.mu.Unlock()
	if _, ok := cache.get("player-0"); !ok {
		t.Fatal("recently accessed entry was evicted")
	}
	if _, ok := cache.get("player-1"); ok {
		t.Fatal("least recently used entry survived eviction")
	}
}

func TestRankScoreCacheRemovesExpiredEntry(t *testing.T) {
	cache := newRankScoreCache()
	cache.mu.Lock()
	cache.putLocked("expired", rankScoreEntry{score: 1, known: true, at: time.Now().Add(-rankScoreCacheTTL - time.Second)})
	cache.mu.Unlock()
	if _, ok := cache.get("expired"); ok {
		t.Fatal("expired entry returned as a cache hit")
	}
	if len(cache.entries) != 0 || cache.recent.Len() != 0 {
		t.Fatalf("expired entry was not removed: entries=%d recent=%d", len(cache.entries), cache.recent.Len())
	}
}
