package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func r214HistoryFixture(t *testing.T) (*app, *LCUClient, string, *atomic.Int64, *atomic.Int64) {
	t.Helper()
	playerRef := strings.Repeat("r", 48)
	var gameID, calls atomic.Int64
	gameID.Store(214)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if tag := r.URL.Query().Get("tag"); tag != "" && tag != "q_420" && tag != "q_2300" {
			t.Errorf("unexpected tag %q", tag)
		}
		if r.URL.Query().Get("tag") == "q_2300" {
			_ = json.NewEncoder(w).Encode(map[string]any{"games": []any{}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"games": []map[string]any{{"json": map[string]any{
			"gameId": gameID.Load(), "queueId": 420, "gameMode": "CLASSIC", "mapId": 11, "gameCreation": time.Now().UnixMilli(), "gameDuration": 1800,
			"participants": []map[string]any{{"participantId": 1, "teamId": 100, "puuid": playerRef, "championId": 13, "win": true, "kills": 8, "deaths": 2, "assists": 6}},
		}}}})
	}))
	t.Cleanup(server.Close)
	client := &LCUClient{region: "TENCENT", rsoPlatform: "HN1"}
	provider := newSGPProvider()
	provider.http = server.Client()
	provider.serverBases["HN1"] = server.URL
	provider.token, provider.tokenAt, provider.tokenClient = "test-entitlements", time.Now(), client
	a := r175App(t)
	a.sgp = provider
	return a, client, playerRef, &gameID, &calls
}

func TestR214FreshRecentRankedBypassesBothCaches(t *testing.T) {
	a, client, ref, id, calls := r214HistoryFixture(t)
	cost := &overviewLoadCost{}
	ctx := context.WithValue(context.Background(), overviewLoadCostContextKey{}, cost)
	reference := gameplayReference{ServerID: "HN1"}
	get := func(ctx context.Context) []gameplayMatch {
		return a.loadRecentRankedSampleQueue(ctx, client, reference, ref, 420, nil, nil, nil)
	}
	if got := get(ctx); len(got) != 1 || got[0].GameID != 214 {
		t.Fatal(got)
	}
	id.Store(215)
	if got := get(ctx); got[0].GameID != 214 || calls.Load() != 1 {
		t.Fatal("ordinary read missed recent cache", got, calls.Load())
	}
	freshCost := &overviewLoadCost{}
	fresh := context.WithValue(context.WithValue(context.Background(), overviewFreshHistoryKey{}, true), overviewLoadCostContextKey{}, freshCost)
	if got := get(fresh); len(got) != 1 || got[0].GameID != 215 || calls.Load() != 2 {
		t.Fatal("fresh read reused stale history", got, calls.Load())
	}
	_, _, historyCalls, hits := freshCost.snapshot()
	if historyCalls != 1 || hits != 0 {
		t.Fatalf("fresh history calls=%d hits=%d", historyCalls, hits)
	}
	if got := get(ctx); got[0].GameID != 215 || calls.Load() != 2 {
		t.Fatal("fresh result not written back", got, calls.Load())
	}
	t.Logf("fresh recent ranked: upstream=1 history_calls=%d sgp_history_cache_hits=%d", historyCalls, hits)
}

func TestR214FreshSeasonHeadBypassesRecentGate(t *testing.T) {
	a, client, ref, id, calls := r214HistoryFixture(t)
	player := Summoner{PUUID: ref}
	reference := gameplayReference{ServerID: "HN1", PlayerRef: ref}
	_, progress, _, _ := a.loadSeasonChampionStatsWithHistoryCache(context.Background(), client, reference, player, ref, nil, true)
	if !progress.Complete || progress.Scanned != 1 || calls.Load() != 2 {
		t.Fatal(progress, calls.Load())
	}
	// Both persisted complete snapshot and the query dedup timestamp are fresh.
	season, _ := currentRankedSeason(time.Now())
	key := seasonQuerySnapshotKey("HN1", ref, season)
	a.seasonBackfillMu.Lock()
	a.cacheSeasonQuerySnapshotLocked(key, time.Now())
	a.seasonBackfillMu.Unlock()
	id.Store(215)
	a.startSeasonStatsRefresh(client, reference, player, ref, nil, false)
	if calls.Load() != 2 {
		t.Fatal("ordinary overview bypassed dedup", calls.Load())
	}
	a.startSeasonStatsRefresh(client, reference, player, ref, nil, true)
	observed, ready := r214WaitHeadProgress(func() r214HeadProgress {
		a.seasonBackfillMu.Lock()
		running := len(a.seasonBackfills)
		a.seasonBackfillMu.Unlock()
		return r214HeadProgress{r175Events(t, a, "season_stats_head_refresh"), calls.Load(), running}
	}, time.Now, func() { time.Sleep(time.Millisecond) })
	rows := observed.rows
	if len(rows) == 0 {
		t.Fatal("missing season_stats_head_refresh")
	}
	if !ready {
		t.Logf("head refresh stopped making progress: events=%d calls=%d running=%d", len(rows), observed.calls, observed.running)
	}
	row := rows[len(rows)-1]
	if len(rows) != 2 || row["fresh"] != true || row["use_history_cache"] != false || row["sgp_history_calls"] != float64(4) || row["sgp_history_cache_hits"] != float64(0) || calls.Load() != 6 {
		t.Fatal(rows, calls.Load())
	}
	// Wait for background cleanup before closing the store or inspecting state.
	deadline := time.Now().Add(time.Second)
	for {
		a.seasonBackfillMu.Lock()
		running := len(a.seasonBackfills)
		a.seasonBackfillMu.Unlock()
		if running == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("refresh did not finish")
		}
		time.Sleep(time.Millisecond)
	}
	cache, err := a.storage.loadSeasonStats(seasonStatsSource, a.storage.accountHash(player), season)
	if err != nil || len(cache.GameIDs) != 2 || seasonStatsCount(cache.Stats) != 2 {
		t.Fatal(cache, err)
	}
	a.startSeasonStatsRefresh(client, reference, player, ref, nil, false)
	if calls.Load() != 6 {
		t.Fatal("ordinary load stopped using recent snapshot")
	}
	t.Logf("fresh season head: upstream=1 sgp_history_cache_hits=%v scanned=%v", rows[0]["sgp_history_cache_hits"], rows[0]["scanned"])
}

// These windows bound test observation, not product latency. Calls and background
// state can advance before the final diagnostic is published. Keep the original
// assertions at the call site, including the exact number and content of events.
type r214HeadProgress struct {
	rows    []map[string]any
	calls   int64
	running int
}

func r214WaitHeadProgress(observe func() r214HeadProgress, now func() time.Time, pause func()) (r214HeadProgress, bool) {
	const idleWindow = 5 * time.Second
	const totalLimit = 30 * time.Second
	started := now()
	lastProgress := started
	previous := [3]int64{-1, -1, -1}
	for {
		snapshot := observe()
		if len(snapshot.rows) >= 2 {
			return snapshot, true
		}
		current := now()
		progress := [3]int64{int64(len(snapshot.rows)), snapshot.calls, int64(snapshot.running)}
		if progress != previous {
			previous, lastProgress = progress, current
		}
		if current.Sub(started) >= totalLimit || current.Sub(lastProgress) >= idleWindow {
			return snapshot, false
		}
		pause()
	}
}

func TestR214HeadProgressWaitBounds(t *testing.T) {
	for _, tc := range []struct {
		name                                string
		readyAt, progressEvery, wantSeconds int
		wantReady                           bool
	}{
		{"after-old-one-second-window", 2, 0, 2, true},
		{"progress-renews-idle-window", 8, 4, 8, true},
		{"missing-event-idle-fails", -1, 0, 5, false},
		{"continuous-progress-total-fails", -1, 1, 30, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var elapsed atomic.Int64
			clock := func() time.Time { return time.Unix(elapsed.Load(), 0) }
			observe := func() r214HeadProgress {
				rows := []map[string]any{{"fresh": false}}
				if tc.readyAt >= 0 && elapsed.Load() >= int64(tc.readyAt) {
					rows = append(rows, map[string]any{"fresh": true})
				}
				calls := int64(6)
				if tc.progressEvery > 0 {
					calls += elapsed.Load() / int64(tc.progressEvery)
				}
				return r214HeadProgress{rows, calls, 1}
			}
			_, ready := r214WaitHeadProgress(observe, clock, func() { elapsed.Add(1) })
			if ready != tc.wantReady || elapsed.Load() != int64(tc.wantSeconds) {
				t.Fatalf("ready=%v elapsed=%ds; want ready=%v elapsed=%ds", ready, elapsed.Load(), tc.wantReady, tc.wantSeconds)
			}
		})
	}
}
