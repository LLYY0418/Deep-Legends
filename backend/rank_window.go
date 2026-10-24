package main

import "time"

// Only retain official values already read by the normal rank pipeline. This
// observation must never add a request or replace an official win count with a
// season aggregate.
func (a *app) rememberOfficialRankWins(client *LCUClient, playerRef, server string, ranks []gameplayRank) {
	wins := map[string]int{}
	totals := map[string]int{}
	for _, rank := range ranks {
		if !rank.seasonFallback && rank.Wins >= 0 {
			wins[rank.QueueType] = rank.Wins
			if rank.Losses >= 0 {
				totals[rank.QueueType] = rank.Wins + rank.Losses
			}
		}
	}
	if len(wins) == 0 {
		return
	}
	key := a.rankMilestoneKey(client, playerRef, server)
	a.milestoneCache.mu.Lock()
	defer a.milestoneCache.mu.Unlock()
	if a.milestoneCache.wins == nil {
		a.milestoneCache.wins = map[string]map[string]int{}
	}
	if len(a.milestoneCache.wins) >= 256 {
		for k := range a.milestoneCache.wins {
			delete(a.milestoneCache.wins, k)
			delete(a.milestoneCache.totals, k)
			break
		}
	}
	a.milestoneCache.wins[key] = wins
	if a.milestoneCache.totals == nil {
		a.milestoneCache.totals = map[string]map[string]int{}
	}
	a.milestoneCache.totals[key] = totals
}

func (a *app) recordRankWindow(client *LCUClient, server, playerRef string, scan *seasonScanState) {
	a.milestoneCache.mu.Lock()
	official := a.milestoneCache.wins[a.rankMilestoneKey(client, playerRef, server)]
	wins := map[string]int{"RANKED_SOLO_5x5": -1, "RANKED_FLEX_SR": -1}
	for q, w := range official {
		wins[q] = w
	}
	a.milestoneCache.mu.Unlock()
	for _, q := range []struct {
		id         int64
		key, label string
	}{{420, "RANKED_SOLO_5x5", "solo"}, {440, "RANKED_FLEX_SR", "flex"}} {
		aggregate := scan.queueStats[q.id]
		stream := "ranked"
		if q.id == 440 {
			stream = "flex"
		}
		cursor := scan.cache.Streams[stream]
		if !cursor.Complete {
			continue
		}
		oldest := ""
		if cursor.OldestCreatedAt > 0 {
			oldest = time.UnixMilli(cursor.OldestCreatedAt).UTC().Format("2006-01")
		}
		a.recordDiagnostic(map[string]any{"event": "rank_window_check", "queue": q.label, "official_wins": wins[q.key], "aggregated_wins": aggregate.Wins, "aggregated_losses": aggregate.Losses, "capped": seasonModeCapped(scan.cache, stream), "oldest_month": oldest})
	}
}

// Copy only counts already returned by the rank pipeline. No extra upstream read.
func (a *app) captureSeasonOfficialTotals(cache *seasonStatsCache, c *LCUClient, server, ref string) {
	a.milestoneCache.mu.Lock()
	defer a.milestoneCache.mu.Unlock()
	totals := a.milestoneCache.totals[a.rankMilestoneKey(c, ref, server)]
	if cache.OfficialTotals == nil {
		cache.OfficialTotals = map[int64]int{}
	}
	for _, q := range []struct {
		id  int64
		key string
	}{{420, "RANKED_SOLO_5x5"}, {440, "RANKED_FLEX_SR"}} {
		if total, ok := totals[q.key]; ok {
			cache.OfficialTotals[q.id] = max(cache.OfficialTotals[q.id], total)
		}
	}
}
