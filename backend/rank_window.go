package main

import "time"

// Only retain official values already read by the normal rank pipeline. This
// observation must never add a request or replace an official win count with a
// season aggregate.
func (a *app) rememberOfficialRankWins(client *LCUClient, playerRef, server string, ranks []gameplayRank) {
	wins := map[string]int{}
	for _, rank := range ranks {
		if !rank.seasonFallback && rank.Wins >= 0 {
			wins[rank.QueueType] = rank.Wins
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
			break
		}
	}
	a.milestoneCache.wins[key] = wins
}

func (a *app) recordRankWindow(client *LCUClient, server, playerRef string, scan *seasonScanState) {
	cursor := scan.cache.Streams["ranked"]
	if !cursor.Complete {
		return
	}
	oldest := ""
	if cursor.OldestCreatedAt > 0 {
		oldest = time.UnixMilli(cursor.OldestCreatedAt).UTC().Format("2006-01")
	}
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
		a.recordDiagnostic(map[string]any{"event": "rank_window_check", "queue": q.label, "official_wins": wins[q.key], "aggregated_wins": aggregate.Wins, "aggregated_losses": aggregate.Losses, "capped": cursor.CappedByUpstream, "oldest_month": oldest})
	}
}
