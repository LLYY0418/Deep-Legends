package main

import (
	"context"
	"testing"
	"time"
)

func TestR252ArenaTeamCountIsMaximumOfObservedAndKnown(t *testing.T) {
	for _, row := range []struct {
		queue            int64
		teams, placement int
		want             string
	}{
		{1750, 3, 3, "win"}, {1750, 3, 4, "loss"},
		{1700, 3, 4, "win"}, {1700, 3, 5, "loss"},
		{1750, 10, 5, "win"}, {1750, 10, 6, "loss"},
		{0, 3, 2, "win"}, {0, 3, 3, "loss"},
		{0, 0, 1, "unknown"}, {1750, 3, 0, "unknown"},
	} {
		m := gameplayMatch{QueueID: row.queue}
		for i := 0; i < row.teams; i++ {
			m.Participants = append(m.Participants, gameplayParticipant{SubteamID: int64(i + 1)})
		}
		if got := arenaPlacementResult(m, row.placement); got != row.want {
			t.Errorf("queue=%d observed=%d placement=%d got=%s want=%s", row.queue, row.teams, row.placement, got, row.want)
		}
	}
}

func TestR252ManualSeasonHeadBypassesRecentGateAndStillProbes(t *testing.T) {
	a, client, ref, id, calls := r214HistoryFixture(t)
	waitGameplaySeasonJobsBeforeCleanup(t, a)
	player := Summoner{PUUID: ref}
	reference := gameplayReference{ServerID: "HN1", PlayerRef: ref}
	_, progress, _, _ := a.loadSeasonChampionStatsWithHistoryCache(context.Background(), client, reference, player, ref, nil, true)
	if !progress.Complete || progress.Scanned != 1 {
		t.Fatal(progress)
	}
	season, _ := currentRankedSeason(time.Now())
	if err := a.storage.saveSeasonHeadMarker(seasonStatsSource, a.storage.accountHash(player), season, 214, time.Now()); err != nil {
		t.Fatal(err)
	}
	a.seasonBackfillMu.Lock()
	a.cacheSeasonQuerySnapshotLocked(seasonQuerySnapshotKey("HN1", ref, season), time.Now())
	a.seasonBackfillMu.Unlock()
	before := calls.Load()
	a.startSeasonStatsRefresh(client, reference, player, ref, nil, false)
	if calls.Load() != before {
		t.Fatal("automatic refresh bypassed recent gate")
	}
	id.Store(215)
	a.startSeasonStatsRefresh(client, reference, player, ref, nil, true)
	// These waits observe background completion, not a product latency budget.
	const backgroundWait = 5 * time.Second
	waitFresh := func(count int) map[string]any {
		t.Helper()
		started := time.Now()
		deadline := started.Add(backgroundWait)
		for {
			var fresh []map[string]any
			for _, row := range r175Events(t, a, "season_stats_head_refresh") {
				if row["fresh"] == true && row["skip_reason"] != "recent" {
					fresh = append(fresh, row)
				}
			}
			if len(fresh) == count {
				t.Logf("manual head refresh %d observed after %s", count, time.Since(started))
				return fresh[count-1]
			}
			if time.Now().After(deadline) {
				t.Fatalf("manual head refresh remained throttled after %s (want %d events): %v", time.Since(started), count, r175Events(t, a, "season_stats_head_refresh"))
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	row := waitFresh(1)
	if row["new_games"] != float64(1) || row["sgp_history_cache_hits"] != float64(0) {
		t.Fatal(row)
	}
	started := time.Now()
	deadline := started.Add(backgroundWait)
	quiet := 0
	for {
		a.seasonBackfillMu.Lock()
		running := len(a.seasonBackfills)
		a.seasonBackfillMu.Unlock()
		if running == 0 {
			quiet++
			if quiet >= 3 {
				t.Logf("manual head background jobs settled after %s", time.Since(started))
				break
			}
		} else {
			quiet = 0
		}
		if time.Now().After(deadline) {
			t.Fatalf("manual refresh did not finish after %s (running=%d)", time.Since(started), running)
		}
		time.Sleep(10 * time.Millisecond)
	}
	before = calls.Load()
	a.startSeasonStatsRefresh(client, reference, player, ref, nil, true)
	row = waitFresh(2)
	if row["skip_reason"] != "no_new_game" || row["new_games"] != float64(0) || calls.Load() != before+1 {
		t.Fatal("manual unchanged head must only probe one newest game", row, calls.Load()-before)
	}
}
