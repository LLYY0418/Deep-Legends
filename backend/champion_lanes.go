package main

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type championLaneShare struct {
	Position string  `json:"position"`
	Rate     float64 `json:"rate"`
}

func (a *app) handleGameplayChampionLanes(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Query().Get("champions"), ",")
	if len(parts) == 0 || len(parts) > 5 {
		http.Error(w, "英雄列表无效", http.StatusBadRequest)
		return
	}
	ids := make([]int, 0, len(parts))
	seen := map[int]bool{}
	for _, part := range parts {
		id, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || id <= 0 || id > 10000 || seen[id] {
			http.Error(w, "英雄列表无效", http.StatusBadRequest)
			return
		}
		seen[id] = true
		ids = append(ids, id)
	}
	tier := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("tier")))
	if tier == "" {
		tier = "all"
	}
	switch tier {
	case "all", "iron", "bronze", "silver", "gold", "gold_plus", "platinum", "platinum_plus", "emerald", "emerald_plus", "diamond", "diamond_plus", "master", "master_plus", "grandmaster", "challenger":
	default:
		http.Error(w, "段位无效", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()
	result := map[string][]championLaneShare{}
	provider := a.championDataProvider()
	for _, id := range ids {
		lanes, err := provider.loadChampionLaneShares(ctx, id, tier)
		if err != nil {
			lanes = []championLaneShare{}
		}
		result[strconv.Itoa(id)] = lanes
	}
	respondJSON(w, result)
}

func (p *championProvider) loadChampionLaneShares(ctx context.Context, id int, tier string) ([]championLaneShare, error) {
	// Reuse the role-share providers and their immutable request/cache keys used
	// by recommendation chips. Never substitute a broader tier for an exact tier.
	if _, supported := qq101TierID(tier); supported && p.featureGates.enabled(featureGateQQ101) {
		positions, _, err := p.loadQQ101Positions(ctx, id, tier)
		if err == nil {
			result := []championLaneShare{}
			for _, row := range positions {
				result = append(result, championLaneShare{row.Position, row.RoleRate / 100})
			}
			return result, nil
		}
	}
	// QQ101 does not advertise individual lower tiers. The existing OPGG detail
	// request supports them and contains all lane shares in summary.positions.
	spec, requestPath, query, err := opggDetailRequest("ranked", id, "mid", tier)
	if err != nil {
		return nil, err
	}
	query, version, err := p.applyOPGGRequestVersion(ctx, spec, query)
	if err != nil {
		return nil, err
	}
	key := opggDetailCacheKey("ranked", spec.Region, id, spec.requestPosition("mid"), tier, version)
	raw, _, err := p.fetchWithMetadataCacheKey(ctx, opggChampionHost, requestPath, query, championJSONMax, "application/json", key)
	if err != nil {
		return nil, err
	}
	var payload opggStructuredDetail
	if json.Unmarshal(raw, &payload) != nil || payload.Data.Summary.ID != id {
		return nil, errors.New("invalid champion lane summary")
	}
	result := []championLaneShare{}
	for _, row := range payload.Data.Summary.Positions {
		position := normalizeQQ101Position(row.Name)
		if position == "" {
			continue
		}
		// This is displayRoleRate's existing explicit-share / game-count fallback.
		rate := row.Stats.RoleRate
		if rate == 0 && row.Stats.Play > 0 && payload.Data.Summary.AverageStats.Play > 0 {
			rate = float64(row.Stats.Play) / float64(payload.Data.Summary.AverageStats.Play)
		}
		if math.IsNaN(rate) || math.IsInf(rate, 0) || rate <= 0 || rate > 1 {
			continue
		}
		result = append(result, championLaneShare{position, rate})
	}
	return result, nil
}
