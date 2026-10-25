package main

import (
	"encoding/json"
	"testing"
)

func TestR311ArenaCoreCandidatesSurviveUntilGamesOrdering(t *testing.T) {
	rows := make([]championMetricRow, 24)
	for i := range rows {
		rows[i] = championMetricRow{Assets: []championAsset{{ID: i + 1}}, Grade: "S", Score: float64(100 - i), Games: i + 10}
	}
	rows[23].Games = 9999
	options := recommendationOptions(rows)
	arena := capGameplayCoreOptions(options, "arena")
	if len(arena) != 24 || arena[23].Stats.Games == nil || *arena[23].Stats.Games != 9999 {
		t.Fatalf("arena lost its highest-games candidate: %#v", arena)
	}
	if arena[23].Score != rows[23].Score || arena[23].Grade != "S" {
		t.Fatal("score/grade tie breakers not passed through")
	}
	payload, err := json.Marshal(arena[23])
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if err := json.Unmarshal(payload, &wire); err != nil {
		t.Fatal(err)
	}
	if wire["score"] != rows[23].Score {
		t.Fatalf("wire score missing: %s", payload)
	}
	for _, mode := range []string{"ranked", "hextech-aram", "aram"} {
		got := capGameplayCoreOptions(options, mode)
		if len(got) != championCoreRecommendationLimit || got[0].IDs[0] != 1 {
			t.Fatalf("%s cap/order changed", mode)
		}
	}
}

func TestR311ArenaAugmentsRemainCompleteInLiveBundle(t *testing.T) {
	rows := make([]championMetricRow, 24)
	for i := range rows {
		rows[i] = championMetricRow{Assets: []championAsset{{ID: i + 1}}, Rarity: "silver", Grade: "S", Games: i + 1}
	}
	detail := championDetailResponse{Mode: "arena", ArenaAugments: rows}
	bundle := gameplayRecommendationsFromResolvedDetail(32, "", detail, gameplayRecommendationModeResolution{InternalMode: "arena"})
	if len(bundle.Augments) != 24 || bundle.Augments[23].Games != 24 {
		t.Fatalf("augment candidates truncated: %#v", bundle.Augments)
	}
}
