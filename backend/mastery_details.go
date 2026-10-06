package main

import (
	"context"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

type masteryDetailsItem struct {
	ChampionMastery
	ChampionName string `json:"championName"`
}
type masteryDetailsResponse struct {
	Available      bool                 `json:"available"`
	Detail         string               `json:"detail,omitempty"`
	Items          []masteryDetailsItem `json:"items"`
	TotalScore     int64                `json:"totalScore"`
	TotalPoints    int64                `json:"totalPoints"`
	TotalChampions int                  `json:"totalChampions"`
}

func masteryDetails(rows []ChampionMastery, names map[int64]string) masteryDetailsResponse {
	result := masteryDetailsResponse{Available: true, Items: make([]masteryDetailsItem, 0, len(rows))}
	for id := range names {
		if id > 0 {
			result.TotalChampions++
		}
	}
	for _, row := range rows {
		if row.ChampionID <= 0 {
			continue
		}
		result.Items = append(result.Items, masteryDetailsItem{row, championName(names, row.ChampionID)})
		result.TotalScore += row.ChampionLevel
		result.TotalPoints += row.ChampionPoints
	}
	sort.Slice(result.Items, func(i, j int) bool {
		if result.Items[i].ChampionPoints != result.Items[j].ChampionPoints {
			return result.Items[i].ChampionPoints > result.Items[j].ChampionPoints
		}
		return result.Items[i].ChampionID < result.Items[j].ChampionID
	})
	return result
}

func (p *riotProvider) fullMasteries(ctx context.Context, puuid string) ([]ChampionMastery, error) {
	var rows []ChampionMastery
	err := p.cachedPublicIdentity(ctx, "mastery-all:"+puuid, 30*time.Minute, &rows, func(ctx context.Context) error {
		return p.get(ctx, p.platformHost(), "/lol/champion-mastery/v4/champion-masteries/by-puuid/"+url.PathEscape(puuid), nil, &rows)
	})
	return rows, err
}

func (a *app) handleGameplayMasteries(w http.ResponseWriter, r *http.Request) {
	ref := strings.TrimSpace(r.URL.Query().Get("playerRef"))
	var reference gameplayReference
	if ref != "" {
		var ok bool
		reference, ok = a.resolveGameplayReferenceDetails(ref)
		if !ok {
			http.Error(w, "玩家引用已失效", http.StatusNotFound)
			return
		}
	}
	a.mu.RLock()
	localClient, localPUUID, localConnected := a.lcu, a.summoner.PUUID, a.connected
	a.mu.RUnlock()
	localAccount := localConnected && reference.PlayerRef == localPUUID && reference.Region == clientRiotPlatform(localClient)
	if isRiotRegion(reference.Region) && !localAccount {
		if a.riot == nil {
			http.Error(w, "Riot 接口不可用", http.StatusConflict)
			return
		}
		p := a.riot.forPlatform(reference.Region)
		rows, err := p.fullMasteries(r.Context(), reference.PlayerRef)
		if err != nil {
			writeRiotHTTPError(w, err)
			return
		}
		result := masteryDetails(rows, a.displayChampionNames(r.Context(), "masteries"))
		var official int64
		err = p.cachedPublicIdentity(r.Context(), "mastery-score:"+reference.PlayerRef, 30*time.Minute, &official, func(ctx context.Context) error {
			return p.get(ctx, p.platformHost(), "/lol/champion-mastery/v4/scores/by-puuid/"+url.PathEscape(reference.PlayerRef), nil, &official)
		})
		if err == nil {
			if result.TotalScore != official {
				a.recordDiagnostic(map[string]any{"event": "mastery_score_mismatch", "calculated": result.TotalScore, "official": official})
			}
			result.TotalScore = official
		} else {
			a.recordDiagnostic(map[string]any{"event": "mastery_score_failed", "reason": safeDiagnosticReason(err)})
		}
		respondJSON(w, result)
		return
	}
	client, current, err := a.gameplayClient()
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	if isRemoteTencentServer(client, reference.ServerID) {
		respondJSON(w, masteryDetailsResponse{Available: false, Detail: "所选服务器不支持熟练度查询"})
		return
	}
	if reference.PlayerRef == "" {
		reference.PlayerRef = current.PUUID
	}
	source, capability := NewChampionMasteryAPI(client).AllContext(r.Context(), reference.PlayerRef)
	if capability.State != capabilityAvailable {
		respondJSON(w, masteryDetailsResponse{Available: false, Detail: "熟练度读取失败"})
		return
	}
	rows := make([]ChampionMastery, 0, len(source))
	for _, row := range source {
		rows = append(rows, row)
	}
	respondJSON(w, masteryDetails(rows, a.displayChampionNames(r.Context(), "masteries")))
}
