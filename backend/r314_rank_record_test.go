package main

import "testing"

// R315 replaces the superseded even-fill policy with observed split results.
func TestR314MissingSeasonLossesUseEvenFill(t *testing.T) {
	for _, c := range []struct{ official, w, l, rate int }{{49, 11, 18, 38}, {154, 35, 45, 44}, {49, 3, 2, 60}, {49, 0, 12, 0}, {49, 0, 0, -1}} {
		snapshot := seasonRankRecordSnapshot{progress: seasonStatsProgress{RankedComplete: true}}
		for i := 0; i < c.w+c.l; i++ {
			snapshot.ranked = append(snapshot.ranked, seasonRankedMatch{CreatedAt: rankSplitStart, QueueID: 420, Win: i < c.w})
		}
		out, _ := (&app{}).applySeasonRankWinRateFallback([]gameplayRank{{QueueType: "RANKED_SOLO_5x5", Wins: c.official, WinRate: -1}}, EndpointCapability{}, snapshot)
		if r := out[0]; r.Wins != c.w || r.Losses != c.l || r.WinRate != c.rate || r.RecordPending {
			t.Fatalf("%+v: %+v", c, r)
		}
	}
}
