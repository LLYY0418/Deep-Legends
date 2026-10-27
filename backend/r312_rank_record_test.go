package main

import "testing"

func TestR312TruncatedRankRecordSurvivesElevenLoads(t *testing.T) {
	for _, branch := range []string{"normal", "stream-first"} {
		t.Run(branch, func(t *testing.T) {
			a := &app{}
			snapshot := seasonRankRecordSnapshot{player: "HN1|truncated", branch: branch, progress: seasonStatsProgress{Season: "S26", RankedComplete: true, Scanned: 78}, byQueue: map[int64]gameplayAggregate{420: {Games: 78, Wins: 40, Losses: 38, WinRate: 51}}}
			load := func(official, wins, losses int, source string) {
				t.Helper()
				ranks, _ := a.applySeasonRankWinRateFallback([]gameplayRank{{QueueType: "RANKED_SOLO_5x5", Wins: official, WinRate: -1}}, EndpointCapability{}, snapshot)
				rank := ranks[0]
				if rank.Wins != wins || rank.Losses != losses || rank.WinRate < 0 || rank.RecordPending || rank.RecordSource != source {
					t.Fatalf("official=%d: %+v, want %d/%d source=%s without pending", official, rank, wins, losses, source)
				}
			}
			for i := 0; i < 11; i++ {
				load(49, 49, 47, "season") // Same persisted snapshot: head refresh has no new game.
			}
			load(50, 50, 47, "season-cache")
			snapshot.byQueue[420] = gameplayAggregate{Games: 79, Wins: 41, Losses: 38, WinRate: 52}
			snapshot.progress.Scanned++
			load(50, 50, 46, "season")
			snapshot.progress.RankedComplete = false
			snapshot.progress.Paused = true
			snapshot.byQueue = nil
			load(51, 51, 46, "season-cache")
		})
	}
}

func TestR312CachedRecordRejectsOlderSnapshotAndKeepsOfficialWinsOnReentry(t *testing.T) {
	a := &app{}
	snapshot := seasonRankRecordSnapshot{player: "HN1|reentry", progress: seasonStatsProgress{Season: "S26", RankedComplete: true, Scanned: 79}, byQueue: map[int64]gameplayAggregate{420: {Games: 79, Wins: 41, Losses: 38, WinRate: 52}}}
	ranks := []gameplayRank{{QueueType: "RANKED_SOLO_5x5", Wins: 50, WinRate: -1}}
	filled, _ := a.applySeasonRankWinRateFallback(ranks, EndpointCapability{}, snapshot)
	if filled[0].OfficialWins != 50 {
		t.Fatal(filled)
	}
	snapshot.byQueue[420] = gameplayAggregate{Games: 78, Wins: 40, Losses: 38, WinRate: 51}
	older, _ := a.applySeasonRankWinRateFallback(ranks, EndpointCapability{}, snapshot)
	if older[0].Wins != 50 || older[0].RecordSource != "season-cache" {
		t.Fatal(older)
	}
	snapshot.progress.Paused = true
	snapshot.byQueue = nil
	reentered, _ := a.applySeasonRankWinRateFallback(filled, EndpointCapability{}, snapshot)
	if reentered[0].Wins != 50 || reentered[0].Losses != 46 || reentered[0].OfficialWins != 50 || reentered[0].RecordPending || reentered[0].RecordSource != "season-cache" {
		t.Fatal(reentered)
	}
}
