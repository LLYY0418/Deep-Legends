package main

import (
	"encoding/json"
	"errors"
	"math"
	"time"
)

func parseYourGGArenaRankings(data []byte, at time.Time) (championRankingResponse, error) {
	var payload struct {
		Success    bool `json:"success"`
		StatusCode int  `json:"statusCode"`
		Response   struct {
			Version      string `json:"version"`
			TotalMatches int    `json:"totalMatches"`
			Champions    []struct {
				ChampionID         int     `json:"championId"`
				Matches            int     `json:"matches"`
				Tier               string  `json:"tier"`
				Score              float64 `json:"score"`
				WinRate            float64 `json:"winRate"`
				BanRate            float64 `json:"banRate"`
				AveragePlacement   float64 `json:"averagePlacement"`
				FirstPlacementRate float64 `json:"firstPlacementRate"`
			} `json:"champions"`
		} `json:"response"`
	}
	invalid := errors.New("YOUR.GG Arena champion response changed")
	if json.Unmarshal(data, &payload) != nil || !payload.Success || payload.StatusCode != 200 || payload.Response.Version == "" || payload.Response.TotalMatches <= 0 || len(payload.Response.Champions) < 100 {
		return championRankingResponse{}, invalid
	}
	result := championRankingResponse{Mode: "arena", Region: "KR", Patch: payload.Response.Version, Source: "YOUR.GG", FetchedAt: at, EntertainmentSample: true, Rows: []championRankingRow{}}
	// Six upstream grades map monotonically to the shared six-level badge.
	tiers := map[string]int{"S": 0, "A": 1, "B": 2, "C": 3, "D": 4, "F": 5}
	seen := map[int]bool{}
	lastScore := math.Inf(1)
	for i, row := range payload.Response.Champions {
		tier, ok := tiers[row.Tier]
		if !ok || row.ChampionID <= 0 || seen[row.ChampionID] || row.Matches <= 0 || row.Score > lastScore || row.WinRate < 0 || row.WinRate > 1 || row.BanRate < 0 || row.BanRate > 1 || row.FirstPlacementRate < 0 || row.FirstPlacementRate > 1 || row.AveragePlacement < 1 || row.AveragePlacement > 8 {
			return championRankingResponse{}, invalid
		}
		seen[row.ChampionID], lastScore = true, row.Score
		// Preserve YOUR.GG's actual order. Do not rank by win rate or placement.
		result.Rows = append(result.Rows, championRankingRow{ChampionID: row.ChampionID, Rank: i + 1, Tier: tier, Grade: row.Tier, Play: row.Matches, WinRate: row.WinRate * 100, BanRate: row.BanRate * 100, AveragePlacement: row.AveragePlacement, FirstPlaceRate: row.FirstPlacementRate * 100})
	}
	return result, nil
}
