package main

import (
	"encoding/json"
	"testing"
)

func r313Record(t *testing.T, a *app, s seasonRankRecordSnapshot, official, w, l, rate int, estimated, unknown bool) gameplayRank {
	t.Helper()
	out, _ := a.applySeasonRankWinRateFallback([]gameplayRank{{QueueType: "RANKED_SOLO_5x5", Wins: official, WinRate: -1}}, EndpointCapability{}, s)
	r := out[0]
	if r.Wins != w || r.Losses != l || r.WinRate != rate || r.RecordPending {
		t.Fatalf("got %+v want %d/%d/%d", r, w, l, rate)
	}
	raw, _ := json.Marshal(r)
	var fields map[string]any
	json.Unmarshal(raw, &fields)
	for _, key := range []string{"recordEstimated", "recordLossUnknown", "recordSampleWins", "recordSampleLosses", "recordSampleGames"} {
		if _, ok := fields[key]; ok {
			t.Fatalf("obsolete field %s in %s", key, raw)
		}
	}
	return r
}
func TestR313OfficialWinsEstimateAndStableRefresh(t *testing.T) {
	for _, branch := range []string{"normal", "stream-first"} {
		t.Run(branch, func(t *testing.T) {
			a := &app{}
			s := r309RecordSnapshot("HN1|stable")
			s.branch = branch
			for i := 0; i < 11; i++ {
				r313Record(t, a, s, 49+i%2, 162, 168, 49, false, false)
			}
			s.ranked = append(s.ranked, seasonRankedMatch{GameID: 999, CreatedAt: rankSplitStart + 1000, QueueID: 420, Win: true})
			r313Record(t, a, s, 50, 163, 168, 49, false, false)
			s.progress.Paused = true
			out, _ := a.applySeasonRankWinRateFallback(r309UnknownRanks(), EndpointCapability{}, s)
			if !out[0].RecordPending || out[0].WinRate != -1 || out[0].Wins != 0 {
				t.Fatal(out)
			}
		})
	}
}
func TestR313EstimateSamplesAndCompletePlayers(t *testing.T) {
	for _, c := range []struct{ official, w, l, rate int }{{49, 0, 0, -1}, {0, 20, 31, 39}, {154, 35, 45, 44}} {
		s := seasonRankRecordSnapshot{player: "HN1|sample", progress: seasonStatsProgress{RankedComplete: true}, ranked: r315Matches(c.w, c.l, 420)}
		r313Record(t, &app{}, s, c.official, c.w, c.l, c.rate, false, false)
	}
}
