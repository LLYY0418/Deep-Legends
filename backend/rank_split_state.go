package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const rankSplitStateFile = "rank-split-windows.json"

type rankSplitPersistence struct {
	loaded  bool
	windows map[string]sgpRankedSeason
	retry   map[string]time.Time
}

// Only id 23 has a verified SGP-to-display mapping. OPGG's S2024 S3 strings
// do not establish SGP ids 21/22. Unknown ids must remain "当前赛段" until such
// evidence exists, even when the hypothetical three-split rule matches a date.
func rankSplitLabel(id int, start int64) (label, idResult, dateResult string) {
	date := time.UnixMilli(start).In(time.FixedZone("CST", 8*3600))
	split := 1
	if date.Month() >= 4 {
		split = 2
	}
	if date.Month() >= 7 {
		split = 3
	}
	names := []string{"", "第一", "第二", "第三"}
	dateResult = fmt.Sprintf("%d 赛季%s赛段", date.Year(), names[split])
	if id >= 0 {
		year := 2026 + (id-21)/3
		part := (id-21)%3 + 1
		if part >= 1 && part <= 3 {
			idResult = fmt.Sprintf("%d 赛季%s赛段", year, names[part])
		}
	}
	if id == 23 && start == rankSplitStart && idResult == dateResult {
		return idResult, idResult, dateResult
	}
	return "", idResult, dateResult
}
func (a *app) loadRankSplitStateLocked() {
	if a.rankSplitState.loaded {
		return
	}
	a.rankSplitState.loaded = true
	a.rankSplitState.windows = map[string]sgpRankedSeason{}
	a.rankSplitState.retry = map[string]time.Time{}
	if a.storage == nil {
		return
	}
	raw, err := readLocalStoreFile(a.storage, rankSplitStateFile)
	if err != nil || len(raw) > 65536 {
		return
	}
	var windows map[string]sgpRankedSeason
	if json.Unmarshal(raw, &windows) != nil {
		return
	}
	for queue, v := range windows {
		parts := strings.SplitN(queue, "|", 2)
		if len(parts) != 2 {
			continue
		}
		if q, _ := rankRecordQueue(parts[1]); q > 0 && v.CurrentSeasonStart > 0 && v.CurrentSeasonID > 0 {
			a.rankSplitState.windows[queue] = v
		}
	}
}
func (a *app) rankSplitSnapshot(player string) (map[string]sgpRankedSeason, bool) {
	a.rankRecordMu.Lock()
	defer a.rankRecordMu.Unlock()
	a.loadRankSplitStateLocked()
	out := map[string]sgpRankedSeason{}
	for _, q := range []string{"RANKED_SOLO_5x5", "RANKED_FLEX_SR"} {
		if v := a.rankSplitState.windows[rankSplitQueueKey(player, q)]; v.CurrentSeasonStart > 0 {
			out[q] = v
		}
	}
	windows := a.rankSplitWindows[player]
	persisted := len(windows) == 0
	for q, v := range windows {
		out[q] = v
	}
	return out, persisted
}
func (a *app) rememberRankSplits(player string, seasons map[string]sgpRankedSeason) {
	a.rankRecordMu.Lock()
	a.loadRankSplitStateLocked()
	events := []map[string]any{}
	changed := false
	accepted := map[string]sgpRankedSeason{}
	for q, v := range a.rankSplitWindows[player] {
		accepted[q] = v
	}
	for q, v := range seasons {
		if queue, _ := rankRecordQueue(q); queue == 0 || v.CurrentSeasonStart <= 0 {
			continue
		}
		old := a.rankSplitState.windows[rankSplitQueueKey(player, q)]
		if v.CurrentSeasonStart < old.CurrentSeasonStart {
			v = old
		}
		accepted[q] = v
		if old.CurrentSeasonStart > 0 && (old.CurrentSeasonID != v.CurrentSeasonID || old.CurrentSeasonStart != v.CurrentSeasonStart) {
			events = append(events, map[string]any{"event": "rank_split_changed", "queue": q, "from_id": old.CurrentSeasonID, "to_id": v.CurrentSeasonID, "from_start": time.UnixMilli(old.CurrentSeasonStart).UTC().Format(time.RFC3339), "to_start": time.UnixMilli(v.CurrentSeasonStart).UTC().Format(time.RFC3339)})
		}
		if old != v {
			changed = true
		}
		a.rankSplitState.windows[rankSplitQueueKey(player, q)] = v
	}
	if a.rankSplitWindows == nil {
		a.rankSplitWindows = map[string]map[string]sgpRankedSeason{}
	}
	if _, ok := a.rankSplitWindows[player]; !ok && len(a.rankSplitWindows) >= 64 {
		for key := range a.rankSplitWindows {
			delete(a.rankSplitWindows, key)
			break
		}
	}
	a.rankSplitWindows[player] = accepted
	if changed && a.storage != nil {
		raw, _ := json.Marshal(a.rankSplitState.windows)
		if err := writeLocalStoreFile(a.storage, rankSplitStateFile, raw); err != nil {
			events = append(events, map[string]any{"event": "rank_split_persist_failed", "reason": "local-write"})
		}
	}
	a.rankRecordMu.Unlock()
	for _, event := range events {
		if event["event"] == "rank_split_changed" {
			// In-flight reads retain their old start in their key, so they cannot
			// repopulate the new window. Remove every cached old player result.
			rawPlayer := player
			if parts := strings.SplitN(player, "|", 2); len(parts) == 2 {
				rawPlayer = parts[1]
			}
			cache := a.ensureRankScores()
			cache.mu.Lock()
			for key, element := range cache.entries {
				if strings.HasSuffix(key, "|"+rawPlayer) {
					cache.removeElement(element)
				}
			}
			cache.mu.Unlock()
		}
		a.recordDiagnostic(event)
	}
}
func (a *app) rankSplitScope(player string, now time.Time) string {
	windows, _ := a.rankSplitSnapshot(player)
	solo, _ := resolveRankSplit(windows["RANKED_SOLO_5x5"], now)
	flex, _ := resolveRankSplit(windows["RANKED_FLEX_SR"], now)
	return fmt.Sprintf("split=%d,%d", solo, flex)
}

// Called before the rank cache is consulted. rankedStatsOn itself has no body
// cache, so this read really crosses the endpoint; failed/old responses retry.
func (a *app) refreshExpiredRankSplit(ctx context.Context, client *LCUClient, server, player string, self bool, privacy string, now time.Time) {
	if a.sgp == nil || client == nil || !isTencentClient(client) {
		return
	}
	key := strings.ToUpper(server) + "|" + player
	windows, _ := a.rankSplitSnapshot(key)
	expired := len(windows) == 0 && now.UnixMilli() >= rankSplitEnd
	for _, v := range windows {
		_, source := resolveRankSplit(v, now)
		if source == "sgp-next" || source == "sgp-end" {
			expired = true
		}
	}
	if !expired {
		return
	}
	a.rankRecordMu.Lock()
	if now.Before(a.rankSplitState.retry[key]) {
		a.rankRecordMu.Unlock()
		return
	}
	if len(a.rankSplitState.retry) >= 64 {
		for old := range a.rankSplitState.retry {
			delete(a.rankSplitState.retry, old)
			break
		}
	}
	a.rankSplitState.retry[key] = now.Add(5 * time.Second)
	a.rankRecordMu.Unlock()
	_, _, _, _ = a.loadSGPRanks(ctx, client, server, player, self, privacy)
}

func rankSplitQueueKey(player, queue string) string {
	server := "_"
	if parts := strings.SplitN(player, "|", 2); len(parts) == 2 {
		server = strings.ToUpper(strings.TrimSpace(parts[0]))
	}
	return server + "|" + queue
}
