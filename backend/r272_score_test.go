package main

import (
	"context"
	"testing"
)

func TestR272ScoreBatchRemakesDistinctFromMissing(t *testing.T) {
	a := r175App(t)
	ctx, b := newMatchScoreBatch(context.Background())
	a.recordMatchScores("sgp", gameplayMatch{GameID: 1, Result: "remake"}, ctx)
	a.recordMatchScores("sgp", gameplayMatch{GameID: 2, Result: "unknown"}, ctx)
	a.recordMatchScores("sgp", gameplayMatch{GameID: 3, Result: "win"}, ctx)
	a.recordMatchScores("sgp", gameplayMatch{GameID: 4, Result: "win", Participants: []gameplayParticipant{{Score: &matchScore{Badge: "MVP"}}}}, ctx)
	a.recordMatchScores("sgp", gameplayMatch{GameID: 1, Result: "remake"}, ctx)
	a.finishMatchScoreBatch(b)
	events := r175Events(t, a, "match_score_computed")
	if len(events) != 1 || events[0]["matches"] != float64(4) || events[0]["failed"] != float64(2) || events[0]["remake_skipped"] != float64(1) {
		t.Fatal("remake counted as missing or duplicated", events)
	}
	if len(r175Events(t, a, "match_score_failed")) != 2 {
		t.Fatal("remake emitted missing_score failure")
	}
}
