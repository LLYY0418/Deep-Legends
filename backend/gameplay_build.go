package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	xhtml "golang.org/x/net/html"
)

// nil preserves absence through the v2 disk cache; explicit zero remains data.
type rawPerkStat struct {
	ID   int64
	Vars [3]*int64
}

func completePerkStats(entries []rawPerkStat) []gameplayPerkStat {
	out := make([]gameplayPerkStat, 0, 6)
	for _, entry := range entries {
		if entry.ID <= 0 {
			continue
		}
		if entry.Vars[0] == nil && entry.Vars[1] == nil && entry.Vars[2] == nil {
			return nil
		}
		stat := gameplayPerkStat{PerkID: entry.ID}
		for i, value := range entry.Vars {
			if value != nil {
				stat.Vars[i] = *value
			}
		}
		out = append(out, stat)
	}
	return out
}

func riotRawPerkStats(raw riotParticipant) []rawPerkStat {
	var entries []rawPerkStat
	for _, style := range raw.Perks.Styles {
		for _, selection := range style.Selections {
			if selection.Perk > 0 && len(entries) < 6 {
				entries = append(entries, rawPerkStat{selection.Perk, [3]*int64{selection.Var1, selection.Var2, selection.Var3}})
			}
		}
	}
	return entries
}

func riotParticipantPerkStats(raw riotParticipant, stale bool) []gameplayPerkStat {
	if stale {
		return nil
	}
	return completePerkStats(riotRawPerkStats(raw))
}

func lcuRawPerkStats(raw lcuParticipant) []rawPerkStat {
	s := raw.Stats
	return []rawPerkStat{
		{s.Perk0, [3]*int64{s.Perk0Var1, s.Perk0Var2, s.Perk0Var3}},
		{s.Perk1, [3]*int64{s.Perk1Var1, s.Perk1Var2, s.Perk1Var3}},
		{s.Perk2, [3]*int64{s.Perk2Var1, s.Perk2Var2, s.Perk2Var3}},
		{s.Perk3, [3]*int64{s.Perk3Var1, s.Perk3Var2, s.Perk3Var3}},
		{s.Perk4, [3]*int64{s.Perk4Var1, s.Perk4Var2, s.Perk4Var3}},
		{s.Perk5, [3]*int64{s.Perk5Var1, s.Perk5Var2, s.Perk5Var3}},
	}
}

func lcuPerkStats(raw lcuParticipant) []gameplayPerkStat {
	return completePerkStats(lcuRawPerkStats(raw))
}

// These caps live for the app session, including diagnostic log rotations.
func (a *app) allowPerkDiagnostic(key string) bool {
	a.perkDiagnosticMu.Lock()
	defer a.perkDiagnosticMu.Unlock()
	if a.perkDiagnosticCounts == nil {
		a.perkDiagnosticCounts = make(map[string]int)
	}
	if a.perkDiagnosticCounts[key] >= 3 {
		return false
	}
	if _, exists := a.perkDiagnosticCounts[key]; !exists && len(a.perkDiagnosticCounts) >= 256 {
		a.perkDiagnosticCounts = map[string]int{}
	}
	a.perkDiagnosticCounts[key]++
	return true
}

func (a *app) recordPerkPresence(source string, participants, withVars int) {
	if a.allowPerkDiagnostic("presence:" + source) {
		a.recordDiagnostic(map[string]any{"event": "perk_stats_presence", "source": source, "participants": participants, "with_vars": withVars, "without_vars": participants - withVars})
	}
}

func (a *app) recordPerkSamples(source string, entries []rawPerkStat, queueID, duration int64) {
	for _, entry := range entries {
		if entry.ID != 8008 && entry.ID != 8304 {
			continue
		}
		if a.allowPerkDiagnostic("sample:" + strconv.FormatInt(entry.ID, 10)) {
			a.recordDiagnostic(map[string]any{"event": "perk_effect_sample", "perk_id": entry.ID, "vars": entry.Vars, "source": source, "queue_id": queueID, "game_duration": duration})
		}
	}
}

func (a *app) recordRiotPerkDiagnostics(source string, infos []*riotMatchInfo, subjectPUUID string, subjectID int64) {
	participants, withVars := 0, 0
	for _, info := range infos {
		if info == nil {
			continue
		}
		duration := riotMatchDurationSeconds(info)
		for _, raw := range info.Participants {
			participants++
			if len(riotParticipantPerkStats(raw, info.PerkStatsStale)) > 0 {
				withVars++
			}
			if subjectID > 0 && raw.ParticipantID == subjectID || subjectID == 0 && subjectPUUID != "" && raw.PUUID == subjectPUUID {
				a.recordPerkSamples(source, riotRawPerkStats(raw), info.QueueID, duration)
			}
		}
	}
	a.recordPerkPresence(source, participants, withVars)
}

func (a *app) recordLCUPerkDiagnostics(games []lcuGame, subject gameplayReference) {
	participants, withVars := 0, 0
	for _, game := range games {
		match := normalizeGameplayMatch(game, subject, nil, nil)
		for _, raw := range game.Participants {
			participants++
			if len(lcuPerkStats(raw)) > 0 {
				withVars++
			}
			if match.SubjectParticipantID > 0 && raw.ParticipantID == match.SubjectParticipantID {
				a.recordPerkSamples("lcu", lcuRawPerkStats(raw), game.QueueID, game.GameDuration)
			}
		}
	}
	a.recordPerkPresence("lcu", participants, withVars)
}

// CommunityDragon supplies only templates here; names and artwork remain from
// the selected LCU/Data Dragon catalog. A failed supplemental read is harmless.
func mergePerkEffectTemplates(perks, templates []gameplayPerk) []gameplayPerk {
	byID := make(map[int64][]string, len(templates))
	for _, perk := range templates {
		byID[perk.ID] = perk.EndOfGameStatDescs
	}
	for i := range perks {
		if len(perks[i].EndOfGameStatDescs) == 0 {
			perks[i].EndOfGameStatDescs = byID[perks[i].ID]
		}
	}
	return perks
}

func (a *app) enrichPerkEffectTemplates(ctx context.Context, perks []gameplayPerk) []gameplayPerk {
	missing := false
	for _, perk := range perks {
		missing = missing || len(perk.EndOfGameStatDescs) == 0
	}
	if !missing {
		return perks
	}
	provider := a.championDataProvider()
	requestCtx, cancel := context.WithTimeout(ctx, communityDragonAugmentRequestBudget)
	defer cancel()
	data, err := provider.fetchCommunityDragonAugmentPart(requestCtx, ctx, "perks", "/latest/plugins/rcp-be-lol-game-data/global/zh_cn/v1/perks.json")
	var templates []gameplayPerk
	if err == nil {
		err = json.Unmarshal(data, &templates)
	}
	if err != nil {
		a.recordDiagnostic(map[string]any{"event": "perk_effect_templates_failed", "reason": safeDiagnosticReason(err)})
		return perks
	}
	return mergePerkEffectTemplates(perks, templates)
}

// Only old Riot disk entries need refreshing. This endpoint updates one game,
// never the overview page or a batch, and preserves anonymous player references.
func (a *app) handleGameplayMatch(w http.ResponseWriter, r *http.Request) {
	var request struct {
		GameID        int64  `json:"gameId"`
		ParticipantID int64  `json:"participantId"`
		Region        string `json:"region"`
		PlayerRef     string `json:"playerRef"`
		Refresh       string `json:"refresh"`
	}
	if err := decodeJSONRequest(r, &request, 4<<10); err != nil || request.GameID <= 0 || request.ParticipantID <= 0 || !isRiotRegion(request.Region) || request.Refresh != "perk-stats" {
		http.Error(w, "查询参数无效", http.StatusBadRequest)
		return
	}
	if a.riot == nil {
		http.Error(w, "Riot 接口不可用", http.StatusConflict)
		return
	}
	raw, _, err := a.riot.forPlatform(request.Region).matchByIDWithCacheMode(r.Context(), riotMatchID(request.Region, request.GameID), true)
	if err != nil {
		a.recordDiagnostic(map[string]any{"event": "perk_stats_refresh_failed", "reason": safeDiagnosticReason(err)})
		http.Error(w, "读取对局失败", http.StatusBadGateway)
		return
	}
	match := riotConvertMatch(raw, "", nil)
	for _, participant := range match.Participants {
		if participant.ParticipantID == request.ParticipantID {
			match.SubjectParticipantID = request.ParticipantID
			if participant.Win {
				match.Result = "win"
			} else {
				match.Result = "loss"
			}
			break
		}
	}
	if match.SubjectParticipantID == 0 {
		http.Error(w, "对局参与者无效", http.StatusBadRequest)
		return
	}
	applyMatchScores(&match)
	a.recordMatchScores("riot", match)
	a.recordRiotPerkDiagnostics("riot", []*riotMatchInfo{&raw.Info}, "", request.ParticipantID)
	a.publicizeMatchReferences(&match)
	a.recordDiagnostic(map[string]any{"event": "perk_stats_refreshed", "game_id": request.GameID})
	respondJSON(w, match)
}

type gameplayAugmentDescription struct {
	ID          int64  `json:"id"`
	Description string `json:"description"`
	Status      string `json:"status"`
}

func parseAugmentDescriptionIDs(value string) ([]int64, error) {
	parts := strings.Split(value, ",")
	if len(parts) > 6 {
		return nil, errors.New("too many ids")
	}
	ids := make([]int64, 0, len(parts))
	seen := map[int64]bool{}
	for _, part := range parts {
		id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
		if err != nil || id <= 0 || id > 10000 {
			return nil, errors.New("invalid id")
		}
		if !seen[id] {
			ids = append(ids, id)
			seen[id] = true
		}
	}
	return ids, nil
}

func augmentEffectDescription(value string) string {
	document, err := xhtml.Parse(strings.NewReader(value))
	if err != nil {
		return ""
	}
	text := strings.TrimSpace(hexdataNodeText(document))
	if text == "" || strings.Contains(text, augmentOfflineDescription) {
		return ""
	}
	return text
}

func loadGameplayAugmentDescriptions(ctx context.Context, ids []int64, loader func(context.Context, int64) (string, error)) []gameplayAugmentDescription {
	items := make([]gameplayAugmentDescription, len(ids))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 2)
	for i, id := range ids {
		wg.Add(1)
		go func(i int, id int64) {
			defer recoverPanic("gameplay_build.loadGameplayAugmentDescriptions.1")
			defer wg.Done()
			items[i] = gameplayAugmentDescription{ID: id, Status: "unavailable"}
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()
			requestCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
			defer cancel()
			description, err := loader(requestCtx, id)
			if err == nil {
				if text := augmentEffectDescription(description); text != "" {
					items[i].Description = text
					items[i].Status = "ok"
				}
			}
		}(i, id)
	}
	wg.Wait()
	return items
}

func (p *championProvider) gameplayAugmentDescription(ctx context.Context, id int64) (string, error) {
	if id < 1000 {
		catalog, err := p.loadCommunityDragonAugments(ctx)
		if err != nil {
			return "", err
		}
		for _, entry := range catalog {
			if entry.ID == id {
				return entry.Description, nil
			}
		}
		return "", nil
	}
	detail, err := p.loadHexdataAugmentDetail(ctx, int(id), "")
	return detail.effectDescription, err
}

func (a *app) handleGameplayAugmentDescriptions(w http.ResponseWriter, r *http.Request) {
	ids, err := parseAugmentDescriptionIDs(r.URL.Query().Get("ids"))
	if err != nil {
		http.Error(w, "海克斯编号无效", http.StatusBadRequest)
		return
	}
	started := time.Now()
	items := loadGameplayAugmentDescriptions(r.Context(), ids, a.championDataProvider().gameplayAugmentDescription)
	ok := 0
	for _, item := range items {
		if item.Status == "ok" {
			ok++
		}
	}
	a.recordDiagnostic(map[string]any{"event": "augment_descriptions_loaded", "requested": len(ids), "ok": ok, "unavailable": len(ids) - ok, "duration_ms": time.Since(started).Milliseconds()})
	respondJSON(w, struct {
		Items []gameplayAugmentDescription `json:"items"`
	}{items})
}
