package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestR86OverviewDeadlineCancelsQueuesAndMastery(t *testing.T) {
	for _, endpoint := range []string{"/lol-game-queues/v1/queues", "champion-mastery"} {
		t.Run(endpoint, func(t *testing.T) {
			a, ref, _ := newGameplayOverviewSGPFixture(t, true)
			original := a.lcu.http.Transport
			var hung atomic.Int32
			a.lcu.http = &http.Client{Timeout: time.Second, Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
				if strings.Contains(r.URL.Path, endpoint) {
					hung.Add(1)
					<-r.Context().Done()
					return nil, r.Context().Err()
				}
				return original.RoundTrip(r)
			})}
			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer cancel()
			started := time.Now()
			a.loadGameplayOverview(ctx, a.lcu, a.summoner, a.gameplayRefDetails[ref], 0, 20, "all", false)
			if time.Since(started) > 200*time.Millisecond || hung.Load() == 0 {
				t.Fatalf("elapsed=%v hang calls=%d", time.Since(started), hung.Load())
			}
		})
	}
}

func r86PagingProvider(t *testing.T, lastPage int) (*app, *atomic.Int32) {
	t.Helper()
	a, _, _ := newGameplayOverviewSGPFixture(t, true)
	count := &atomic.Int32{}
	a.sgp.http = &http.Client{Transport: sgpRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		count.Add(1)
		start, _ := strconv.Atoi(r.URL.Query().Get("startIndex"))
		size := 50
		if lastPage >= 0 && start >= lastPage {
			size = 49
		}
		games := make([]map[string]any, 0, size)
		for i := 0; i < size; i++ {
			games = append(games, map[string]any{"json": map[string]any{"gameId": start + i + 1, "queueId": 420, "gameCreation": time.Now().UnixMilli(), "gameDuration": 1800, "padding": strings.Repeat("x", 2048), "participants": []map[string]any{{"puuid": "subject", "participantId": 1, "championId": 103, "win": true}}}})
		}
		data, _ := json.Marshal(map[string]any{"games": games})
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(data)), Request: r}, nil
	})}
	return a, count
}

func TestR86SGPThreeHundredPhysicalPagesStayWithinBudget(t *testing.T) {
	a, calls := r86PagingProvider(t, -1)
	for page := 0; page < 300; page++ {
		games, consumed, _, err := a.sgp.matchHistoryOn(context.Background(), a.lcu, "HN1", "subject", page*50, 50, true)
		if err != nil || len(games) != 50 || consumed != 50 {
			t.Fatalf("page %d: games=%d consumed=%d err=%v", page, len(games), consumed, err)
		}
		a.sgp.mu.Lock()
		total := 0
		for _, entry := range a.sgp.historyCache {
			total += entry.bytes
		}
		valid := total == a.sgp.historyBytes && total <= sgpCacheMaxBytes && len(a.sgp.historyCache) <= sgpCacheMax
		a.sgp.mu.Unlock()
		if !valid {
			t.Fatalf("page %d cache budget/accounting violated", page)
		}
	}
	if calls.Load() != 300 {
		t.Fatalf("calls=%d", calls.Load())
	}
}

func TestR86TenPageSeasonBackfillDoesNotPopulateHistoryCache(t *testing.T) {
	a, calls := r86PagingProvider(t, 500)
	a.storage = trackTestStore(t, &localStore{root: t.TempDir()})
	season, start := currentRankedSeason(time.Now())
	cache := seasonStatsCache{SchemaVersion: seasonStatsCacheSchemaVersion, Source: seasonStatsSource, Season: season, AccountHash: "r86-fixture", ResumeIndex: 50}
	if err := a.storage.saveSeasonStats(cache); err != nil {
		t.Fatal(err)
	}
	a.sgp.cacheHistoryPage("HN1", "subject", 0, 50, nil, sgpHistoryCacheEntry{bytes: 1})
	a.startSeasonBackfill(a.lcu, gameplayReference{PlayerRef: "subject", ServerID: "HN1"}, Summoner{}, "subject", nil, "HN1", cache.AccountHash, season, start)
	deadline := time.Now().Add(3 * time.Second)
	for {
		a.seasonBackfillMu.Lock()
		running := len(a.seasonBackfills)
		a.seasonBackfillMu.Unlock()
		if running == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("backfill never completed")
		}
		time.Sleep(time.Millisecond)
	}
	if calls.Load() != 10 {
		t.Fatalf("backfill requests=%d want10", calls.Load())
	}
	result, err := a.storage.loadSeasonStats(seasonStatsSource, cache.AccountHash, season)
	if err != nil || !result.Complete || len(result.GameIDs) != 499 {
		t.Fatalf("result complete=%v ids=%d err=%v", result.Complete, len(result.GameIDs), err)
	}
	a.sgp.mu.Lock()
	defer a.sgp.mu.Unlock()
	if len(a.sgp.historyCache) != 1 {
		t.Fatalf("background polluted page cache: %d", len(a.sgp.historyCache))
	}
	for key := range a.sgp.historyCache {
		if key != sgpHistoryPageCacheKey("HN1", "subject", 0, 50, nil) {
			t.Fatal(fmt.Sprintf("unexpected background key %s", key))
		}
	}
}
