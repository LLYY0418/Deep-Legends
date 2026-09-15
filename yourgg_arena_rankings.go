package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

func parseYourGGArenaRankings(data []byte, at time.Time, observers ...func(map[string]any)) (championRankingResponse, error) {
	var payload struct {
		Success    bool `json:"success"`
		StatusCode int  `json:"statusCode"`
		Response   struct {
			Version      string            `json:"version"`
			TotalMatches int               `json:"totalMatches"`
			Champions    []json.RawMessage `json:"champions"`
		} `json:"response"`
	}
	type arenaRankingInput struct {
		ChampionID         int     `json:"championId"`
		Matches            int     `json:"matches"`
		Tier               string  `json:"tier"`
		Score              float64 `json:"score"`
		WinRate            float64 `json:"winRate"`
		BanRate            float64 `json:"banRate"`
		AveragePlacement   float64 `json:"averagePlacement"`
		FirstPlacementRate float64 `json:"firstPlacementRate"`
	}
	invalid := errors.New("YOUR.GG Arena champion response changed")
	var firstField, firstValue string
	failures := map[string]int{}
	report := func(field string, value any) {
		failures[field]++
		if firstField == "" {
			firstField = field
			firstValue = truncateClientDiagnosticText(fmt.Sprint(value))
		}
	}
	skipped, accepted := 0, 0
	defer func() {
		if firstField == "" {
			return
		}
		for _, observe := range observers {
			if observe != nil {
				observe(map[string]any{"event": "arena_rankings_parse_failed", "field": firstField, "value": firstValue, "field_counts": failures, "skipped_rows": skipped, "accepted_rows": accepted})
			}
		}
	}()
	if err := json.Unmarshal(data, &payload); err != nil {
		report("envelope", "invalid-json-or-schema")
		return championRankingResponse{}, invalid
	}
	if !payload.Success || payload.StatusCode != 200 || payload.Response.Version == "" || payload.Response.TotalMatches <= 0 || len(payload.Response.Champions) == 0 {
		report("envelope", "missing-or-invalid-metadata")
		return championRankingResponse{}, invalid
	}
	result := championRankingResponse{Mode: "arena", Region: "KR", Patch: payload.Response.Version, Source: "YOUR.GG", FetchedAt: at, EntertainmentSample: true, Rows: []championRankingRow{}}
	// Source Grade distinguishes OP's gold badge from S's letter badge.
	tiers := map[string]int{"OP": 0, "S": 0, "A": 1, "B": 2, "C": 3, "D": 4, "F": 5}
	seen := map[int]bool{}
	lastScore := 101.0
	for i, raw := range payload.Response.Champions {
		var row arenaRankingInput
		field := ""
		var value any
		if json.Unmarshal(raw, &row) != nil {
			report("row", "invalid-schema")
			skipped++
			continue
		}
		tier, ok := tiers[row.Tier]
		switch {
		case !ok:
			field, value = "tier", row.Tier
		case row.ChampionID <= 0 || seen[row.ChampionID]:
			field, value = "championId", row.ChampionID
		case row.Matches <= 0:
			field, value = "matches", row.Matches
		case row.Score < 0 || row.Score > 100:
			field, value = "score", row.Score
		case row.WinRate < 0 || row.WinRate > 1:
			field, value = "winRate", row.WinRate
		case row.BanRate < 0 || row.BanRate > 1:
			field, value = "banRate", row.BanRate
		case row.FirstPlacementRate < 0 || row.FirstPlacementRate > 1:
			field, value = "firstPlacementRate", row.FirstPlacementRate
		case row.AveragePlacement < 1 || row.AveragePlacement > 8:
			field, value = "averagePlacement", row.AveragePlacement
		}
		if field != "" {
			report(field, value)
			skipped++
			continue
		}
		// An ordering change is observable but must not discard valid champions.
		if row.Score > lastScore {
			report("score_order", row.Score)
		}
		seen[row.ChampionID], lastScore = true, row.Score
		result.Rows = append(result.Rows, championRankingRow{ChampionID: row.ChampionID, Rank: i + 1, Tier: tier, Grade: row.Tier, Play: row.Matches, WinRate: row.WinRate * 100, BanRate: row.BanRate * 100, AveragePlacement: row.AveragePlacement, FirstPlaceRate: row.FirstPlacementRate * 100})
	}
	accepted = len(result.Rows)
	if accepted == 0 {
		return championRankingResponse{}, invalid
	}
	return result, nil
}
