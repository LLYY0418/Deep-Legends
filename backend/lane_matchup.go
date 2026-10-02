package main

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type gameplayLaneMatchup struct {
	ChampionID      int     `json:"championId"`
	EnemyChampionID int     `json:"enemyChampionId"`
	WinRate         float64 `json:"winRate"`
	Games           int     `json:"games"`
}

func (a *app) handleGameplayLaneMatchup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	query := r.URL.Query()
	own, ownErr := strconv.Atoi(strings.TrimSpace(query.Get("champion")))
	enemy, enemyErr := strconv.Atoi(strings.TrimSpace(query.Get("enemy")))
	position, positionErr := normalizeOPGGPosition(query.Get("position"))
	tier, tierErr := gameplayRecommendationTier(opggModeSpecs["ranked"], query.Get("tier"))
	if ownErr != nil || enemyErr != nil || own <= 0 || enemy <= 0 || own > 1000000 || enemy > 1000000 || positionErr != nil || position == "" || tierErr != nil {
		http.Error(w, "对位参数无效", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	// Exactly the recommendation detail cache key/loader, including data version.
	// A cold concurrent recommendation and matchup share the existing cache flight.
	rows, err := a.championDataProvider().loadStructuredCounterRows(ctx, "ranked", strconv.Itoa(own), position, tier)
	if err != nil {
		http.Error(w, "对位数据暂不可用", http.StatusBadGateway)
		return
	}
	for _, row := range rows {
		if row.ChampionID == enemy {
			respondJSON(w, gameplayLaneMatchup{own, enemy, row.WinRate, row.Games})
			return
		}
	}
	respondJSON(w, map[string]any{})
}
