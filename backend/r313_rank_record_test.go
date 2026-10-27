package main

import (
	"encoding/json"
	"testing"
)

func r313Record(t *testing.T, a *app, s seasonRankRecordSnapshot, official int, wantWins, wantLoss, wantRate int, estimated, unknown bool) gameplayRank {
	t.Helper()
	out, _ := a.applySeasonRankWinRateFallback([]gameplayRank{{QueueType: "RANKED_SOLO_5x5", Wins: official, WinRate: -1}}, EndpointCapability{}, s)
	r := out[0]
	if r.Wins != wantWins || r.Losses != wantLoss || r.WinRate != wantRate || r.RecordPending {
		t.Fatalf("official=%d got %+v, want %d/%d/%d", official, r, wantWins, wantLoss, wantRate)
	}
	b, _ := json.Marshal(r)
	var fields map[string]any
	json.Unmarshal(b, &fields)
	if (fields["recordEstimated"] == true) != estimated || (fields["recordLossUnknown"] == true) != unknown {
		t.Fatalf("flags got %s want estimated=%v unknown=%v", b, estimated, unknown)
	}
	return r
}
func TestR313OfficialWinsEstimateAndStableRefresh(t *testing.T) {
	for _, branch := range []string{"normal", "stream-first"} {
		t.Run(branch, func(t *testing.T) {
			a := &app{}
			s := seasonRankRecordSnapshot{player: "HN1|truncated", branch: branch, progress: seasonStatsProgress{Season: "S26", RankedComplete: true, Scanned: 51}, byQueue: map[int64]gameplayAggregate{420: {Games: 51, Wins: 20, Losses: 31, WinRate: 39}}}
			for i := 0; i < 10; i++ {
				r313Record(t, a, s, 49, 49, 76, 39, true, false)
			}
			r313Record(t, a, s, 50, 50, 76, 40, true, false)
			for i := 0; i < 10; i++ {
				r313Record(t, a, s, 50, 50, 76, 40, true, false)
			}
			r313Record(t, a, s, 49, 50, 76, 40, true, false) // stale official response cannot reset the increment baseline.
			r313Record(t, a, s, 50, 50, 76, 40, true, false)
			s.byQueue[420] = gameplayAggregate{Games: 52, Wins: 21, Losses: 31, WinRate: 40}
			s.progress.Scanned++
			r313Record(t, a, s, 50, 50, 74, 40, true, false)
			s.progress.Paused = true
			s.byQueue = nil
			r313Record(t, a, s, 51, 51, 74, 41, true, false)
			s.progress.Season = "S27"
			out, _ := a.applySeasonRankWinRateFallback([]gameplayRank{{QueueType: "RANKED_SOLO_5x5", Wins: 51, WinRate: -1}}, EndpointCapability{}, s)
			if out[0].RecordSource != "none" {
				t.Fatal(out)
			}
		})
	}
}
func TestR313EstimateSamplesAndCompletePlayers(t *testing.T) {
	for _, c := range []struct {
		official, w, l, rate, outW, outL, outRate int
		estimate, unknown                         bool
	}{{49, 20, 31, 39, 49, 76, 39, true, false}, {154, 85, 98, 46, 154, 178, 46, true, false}, {178, 177, 180, 50, 177, 180, 50, false, false}, {11, 11, 15, 42, 11, 15, 42, false, false}, {0, 20, 31, 39, 20, 31, 39, false, false}, {49, 3, 2, 60, 49, 0, -1, true, true}, {49, 0, 12, 0, 49, 0, -1, true, true}, {49, 4, 6, 40, 49, 74, 40, true, false}} {
		s := seasonRankRecordSnapshot{player: "HN1|sample", progress: seasonStatsProgress{Season: "S26", RankedComplete: true}, byQueue: map[int64]gameplayAggregate{420: {Games: c.w + c.l, Wins: c.w, Losses: c.l, WinRate: c.rate}}}
		r313Record(t, &app{}, s, c.official, c.outW, c.outL, c.outRate, c.estimate, c.unknown)
	}
}
