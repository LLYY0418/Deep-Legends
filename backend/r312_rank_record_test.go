package main

import "testing"

func TestR312TruncatedRankRecordSurvivesElevenLoads(t *testing.T) {
	for _, branch := range []string{"normal", "stream-first"} {
		t.Run(branch, func(t *testing.T) {
			a := &app{}
			snapshot := r309RecordSnapshot("HN1|truncated")
			snapshot.branch = branch
			snapshot.ranked = r315Matches(40, 38, 420)
			for i := 0; i < 11; i++ {
				out, _ := a.applySeasonRankWinRateFallback([]gameplayRank{{QueueType: "RANKED_SOLO_5x5", Wins: 49 + i%2, WinRate: -1}}, EndpointCapability{}, snapshot)
				if out[0].Wins != 40 || out[0].Losses != 38 || out[0].WinRate != 51 || out[0].RecordPending {
					t.Fatal(out)
				}
			}
			snapshot.progress.Paused = true
			out, _ := a.applySeasonRankWinRateFallback(r309UnknownRanks(), EndpointCapability{}, snapshot)
			if out[0].Wins != 0 || !out[0].RecordPending {
				t.Fatal(out)
			}
			snapshot.progress.Paused = false
			snapshot.ranked = append(snapshot.ranked, seasonRankedMatch{GameID: 999999, QueueID: 420, CreatedAt: rankSplitStart + 1000, Win: true})
			out, _ = a.applySeasonRankWinRateFallback(r309UnknownRanks(), EndpointCapability{}, snapshot)
			if out[0].Wins != 41 || out[0].Losses != 38 || out[0].WinRate != 52 || out[0].RecordSource != "season" {
				t.Fatal(out)
			}
		})
	}
}
func TestR312CachedRecordRejectsOlderSnapshotAndKeepsOfficialWinsOnReentry(t *testing.T) {
	a := &app{}
	s := r309RecordSnapshot("HN1|self")
	ranks := r309UnknownRanks()
	ranks[0].countryRank = true
	ranks[0].WinRate = 90
	ranks[0].Losses = 1
	out, _ := a.applySeasonRankWinRateFallback(ranks, EndpointCapability{}, s)
	if out[0].Wins != 162 || out[0].Losses != 168 {
		t.Fatal(out)
	}
	// Foreign verified records remain exactly upstream-provided.
	ranks[0].countryRank = false
	s.player = "EUW1|foreign"
	out, _ = a.applySeasonRankWinRateFallback(ranks, EndpointCapability{}, s)
	if out[0].Wins != 162 || out[0].Losses != 1 || out[0].WinRate != 90 || out[0].RecordSource != "upstream" {
		t.Fatal(out)
	}
}
