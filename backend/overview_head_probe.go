package main

import (
	"context"
	"encoding/json"
	"sync"
	"time"
)

// The API exposes no total count. Locate an actual saved ID, never subtract IDs
// to invent a game count. Boundary probes are background-only and bounded at 20.
func (a *app) refreshCurrentHistorySnapshot(ctx context.Context, client *LCUClient, ref gameplayReference, puuid string, count int, old clientHistorySnapshot, names, labels map[int64]string) clientHistorySnapshot {
	cost := overviewCostFromContext(ctx)
	_, before, _, _ := cost.snapshot()
	newGames, hit, verified := 0, false, false
	defer func() {
		_, after, _, _ := cost.snapshot()
		a.recordDiagnostic(map[string]any{"event": "overview_head_probe", "hit": hit, "new_games": newGames, "bytes": after - before, "boundary_verified": verified})
	}()
	seen := map[int64]bool{}
	if old.HeadGameID > 0 {
		seen[old.HeadGameID] = true
	}
	for _, match := range old.Matches {
		seen[match.GameID] = true
	}
	latest := int64(0)
	for offset := 0; offset < 20; offset++ {
		infos, consumed, _, err := a.sgp.matchHistoryFilteredOn(ctx, client, ref.ServerID, puuid, offset, 1, nil, false)
		if err != nil || len(infos) != consumed || consumed == 0 && len(old.Matches) > 0 {
			return old
		}
		if offset == 0 && len(infos) > 0 {
			latest = infos[0].GameID
		}
		if consumed == 0 || infos[0] != nil && seen[infos[0].GameID] {
			verified = true
			hit = offset == 0
			break
		}
		newGames++
		if offset == 0 && !a.waitConnectionPriority(ctx, client) {
			return old
		}
	}
	if hit {
		return old
	}
	// A missing boundary after twenty rows means replace the visible page; it
	// does not claim the account gained exactly twenty games.
	if newGames == 0 {
		return old
	}
	if !a.waitConnectionPriority(ctx, client) {
		return old
	}
	fresh := context.WithValue(ctx, overviewFreshHistoryKey{}, true)
	matches, caps, pagination := a.loadDetailedMatches(fresh, client, ref, puuid, true, 0, min(count, newGames), "all", names, labels)
	valid := false
	for _, cap := range caps {
		if cap.Name == "match-history" && cap.State == capabilityAvailable {
			valid = true
		}
	}
	if !valid {
		return old
	}
	merged := append(cloneClientHistoryMatches(matches), cloneClientHistoryMatches(old.Matches)...)
	ids := map[int64]bool{}
	result := merged[:0]
	for _, match := range merged {
		if !ids[match.GameID] {
			ids[match.GameID] = true
			result = append(result, match)
		}
	}
	clipped := len(result) > count
	if clipped {
		result = result[:count]
	}
	if verified {
		pagination = old.Pagination
		pagination.Count = len(result)
		pagination.HasMore = old.Pagination.HasMore || clipped
	}
	return clientHistorySnapshot{Matches: result, Capabilities: caps, Pagination: pagination, HeadGameID: latest}
}

func (a *app) broadcastOverviewRankedSupplement(account string, matches []gameplayMatch, queues map[string]gameplayRankedQueueStats, puuid string, head int64) {
	if len(matches) == 0 {
		return
	}
	summary := recentRankedSummary(matches, puuid, nil)
	event, err := json.Marshal(map[string]any{"type": "overview-incremental", "account": account, "recentRanked": summary, "rankedQueues": queues, "headGameId": head})
	if err == nil {
		a.broadcastEvent(string(event))
	}
}

func (a *app) observeOverviewSource(source string) {
	a.coldLaunch.mu.Lock()
	if a.coldLaunch.overviewSource == "" {
		a.coldLaunch.overviewSource = source
	}
	a.coldLaunch.mu.Unlock()
}

func (a *app) overviewSupplementContext(client *LCUClient) context.Context {
	a.mu.RLock()
	g := a.connectionPriority
	a.mu.RUnlock()
	if g != nil && g.client == client {
		return g.ctx
	}
	return a.licenseBusinessContext()
}

var overviewRankedSupplementFlights sync.Map

func (a *app) startOverviewRankedSupplement(client *LCUClient, ref gameplayReference, puuid string, matches []gameplayMatch, names, labels map[int64]string) {
	if a.sgp == nil || ref.ServerID == "" || !isTencentClient(client) {
		return
	}
	if len(recentRankedMatchesForQueue(matches, 420, 20)) >= 20 && len(recentRankedMatchesForQueue(matches, 440, 20)) >= 20 {
		return
	}
	key := clientHistoryAccount{a, client, puuid}
	if _, loaded := overviewRankedSupplementFlights.LoadOrStore(key, struct{}{}); loaded {
		return
	}
	a.goSafe("overview-ranked-supplement", func() {
		defer overviewRankedSupplementFlights.Delete(key)
		parent := a.overviewSupplementContext(client)
		if !a.waitConnectionPriority(parent, client) {
			return
		}
		ctx, cancel := context.WithTimeout(parent, 45*time.Second)
		defer cancel()
		cost := &overviewLoadCost{}
		ctx = context.WithValue(ctx, overviewLoadCostContextKey{}, cost)
		samples := a.loadRecentRankedSamples(ctx, client, ref, puuid, "all", gameplayPagination{}, matches, names, labels)
		a.mu.RLock()
		valid := a.lcu == client && a.connected && a.summoner.PUUID == puuid && a.shutdownClient != client
		a.mu.RUnlock()
		if !valid || ctx.Err() != nil {
			return
		}
		ranked := append(append([]gameplayMatch(nil), samples.ByQueue[420]...), samples.ByQueue[440]...)
		queues := buildGameplayRankedQueues(gameplayRankedQueueTabs(samples.ByQueue, nil), puuid, nil, ref.Region)
		public := a.registerGameplayReferenceDetails(ref)
		head := int64(0)
		if len(matches) > 0 {
			head = matches[0].GameID
		}
		a.broadcastOverviewRankedSupplement(public, ranked, queues, puuid, head)
	})
}
