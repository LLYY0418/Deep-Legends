package main

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// R263 P2.1：斗魂等级与名望。
//
// 斗魂取消排位后改为名望（Fame）等级制：1–12 级，满 12 级后名望继续累计。
// 累计门槛来自 League Wiki「Arena」页（2026-10 核对）：
// https://wiki.leagueoflegends.com/en-us/Arena
// 1–2 木、3–5 青铜、6–8 白银、9–11 黄金、12 角斗士。
//
// 客户端排位接口仍保留 CHERRY 队列条目，带 ratedTier / ratedRating
// （LolRankedRankedQueueStats，2026-09 LCU swagger）。仓库里没有真实样本，
// 所以只在 ratedTier 与按名望换算出的档位一致时才展示；不一致或缺字段时
// 不显示，并记 arena_fame_source_shape 诊断供发版后真机取证。
var arenaFameLevelThresholds = []int{100, 1600, 3600, 6100, 9100, 12600, 16600, 21600, 26600, 31600, 37600, 47600}

type gameplayArenaFame struct {
	Level int    `json:"level"`
	Fame  int    `json:"fame"`
	Tier  string `json:"tier"`
}

func arenaLevelForFame(fame int) int {
	level := 0
	for index, threshold := range arenaFameLevelThresholds {
		if fame >= threshold {
			level = index + 1
		}
	}
	return level
}

func arenaTierForLevel(level int) string {
	switch {
	case level <= 0:
		return ""
	case level <= 2:
		return "WOOD"
	case level <= 5:
		return "BRONZE"
	case level <= 8:
		return "SILVER"
	case level <= 11:
		return "GOLD"
	default:
		return "GLADIATOR"
	}
}

// arenaFameFromRated validates ratedTier against the level implied by
// ratedRating. Returns nil when the pair cannot be trusted as Fame.
func arenaFameFromRated(tier string, rating float64) (*gameplayArenaFame, string) {
	tier = strings.ToUpper(strings.TrimSpace(tier))
	if math.IsNaN(rating) || math.IsInf(rating, 0) || rating <= 0 {
		return nil, "no_rating"
	}
	fame := int(math.Round(rating))
	level := arenaLevelForFame(fame)
	if level == 0 {
		return nil, "below_level_one"
	}
	expected := arenaTierForLevel(level)
	if tier == "" || tier == "NONE" || tier == "UNRANKED" {
		return nil, "no_tier"
	}
	if tier != expected {
		return nil, "tier_mismatch"
	}
	return &gameplayArenaFame{Level: level, Fame: fame, Tier: tier}, ""
}

type arenaFameCacheEntry struct {
	value *gameplayArenaFame
	at    time.Time
}

type arenaFameCache struct {
	entries map[string]arenaFameCacheEntry
	logged  map[string]bool
}

const arenaFameCacheTTL = 10 * time.Minute

// loadLiveArenaFame reads the CHERRY ranked entry for one live player. In Arena
// the live page no longer shows Summoner's Rift ranks, so this replaces that
// lookup rather than adding a request.
func (a *app) loadLiveArenaFame(ctx context.Context, client *LCUClient, reference gameplayReference, playerRef string, isCurrent bool, privacy string) *gameplayArenaFame {
	if a == nil || !validPlayerReference(playerRef) {
		return nil
	}
	serverID := strings.ToUpper(strings.TrimSpace(reference.ServerID))
	if serverID == "" {
		serverID = strings.ToUpper(clientTencentServerID(client))
	}
	key := serverID + "|" + playerRef
	state := a.r263()
	state.mu.Lock()
	if entry, ok := state.fame.entries[key]; ok && time.Since(entry.at) < arenaFameCacheTTL {
		state.mu.Unlock()
		return entry.value
	}
	state.mu.Unlock()

	var tier string
	var rating float64
	var raw json.RawMessage
	source := ""
	found := false
	if a.sgp != nil && clientRiotPlatform(client) == "" && serverID != "" && !isRemoteTencentServer(client, serverID) {
		if stats, err := a.sgp.rankedStatsOn(ctx, client, serverID, playerRef, isCurrent, privacy); err == nil {
			source = dataSourceSGP
			for _, queue := range stats.Queues {
				if queue.QueueType == "CHERRY" {
					tier, rating, found = queue.RatedTier, queue.RatedRating, true
					raw = stats.cherryRaw
					break
				}
			}
		}
	}
	if source == "" && client != nil {
		path := "/lol-ranked/v1/ranked-stats/" + url.PathEscape(playerRef)
		if isCurrent {
			path = "/lol-ranked/v1/current-ranked-stats"
		}
		var payload json.RawMessage
		if err := client.RequestJSON(ctx, http.MethodGet, path, nil, &payload); err == nil {
			source = dataSourceLCU
			for _, entry := range rankedQueueEntries(payload) {
				var queue struct {
					QueueType   string  `json:"queueType"`
					RatedTier   string  `json:"ratedTier"`
					RatedRating float64 `json:"ratedRating"`
				}
				if json.Unmarshal(entry, &queue) == nil && queue.QueueType == "CHERRY" {
					tier, rating, found, raw = queue.RatedTier, queue.RatedRating, true, entry
					break
				}
			}
		}
	}
	if source == "" {
		return nil // transient failure: do not cache
	}
	value, reason := arenaFameFromRated(tier, rating)
	if !found {
		reason = "no_cherry_entry"
	}
	a.recordArenaFameShape(source, isCurrent, found, raw, reason)
	state.mu.Lock()
	if state.fame.entries == nil {
		state.fame.entries = map[string]arenaFameCacheEntry{}
	}
	if len(state.fame.entries) >= 256 {
		for oldKey := range state.fame.entries {
			delete(state.fame.entries, oldKey)
			break
		}
	}
	state.fame.entries[key] = arenaFameCacheEntry{value: value, at: time.Now()}
	state.mu.Unlock()
	return value
}

// recordArenaFameShape records, once per source and self/other, the CHERRY
// entry key names plus a fixed set of scalar values. No identity is recorded.
func (a *app) recordArenaFameShape(source string, isSelf, found bool, raw json.RawMessage, reason string) {
	logKey := source + "|" + map[bool]string{true: "self", false: "other"}[isSelf]
	state := a.r263()
	state.mu.Lock()
	if state.fame.logged == nil {
		state.fame.logged = map[string]bool{}
	}
	if state.fame.logged[logKey] {
		state.mu.Unlock()
		return
	}
	state.fame.logged[logKey] = true
	state.mu.Unlock()
	event := map[string]any{"event": "arena_fame_source_shape", "source": source, "is_self": isSelf, "found": found, "reason": reason}
	var object map[string]json.RawMessage
	if len(raw) > 0 && json.Unmarshal(raw, &object) == nil {
		keys := make([]string, 0, len(object))
		for key := range object {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		event["keys"] = keys
		values := map[string]any{}
		for _, key := range []string{"ratedTier", "ratedRating", "tier", "division", "rank", "leaguePoints", "wins", "losses"} {
			value, ok := object[key]
			if !ok {
				continue
			}
			var scalar any
			if json.Unmarshal(value, &scalar) == nil {
				switch scalar.(type) {
				case nil, bool, float64, string:
					values[key] = scalar
				}
			}
		}
		event["values"] = values
	}
	a.recordDiagnostic(event)
}
