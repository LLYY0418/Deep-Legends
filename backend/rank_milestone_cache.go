package main

import (
	"context"
	"fmt"
	"sync"
)

type milestoneCache struct {
	mu      sync.Mutex
	entries map[string]*gameplayRankMilestones
	wins    map[string]map[string]int
	totals  map[string]map[string]int
}

func (a *app) rankMilestoneKey(client *LCUClient, playerRef, server string) string {
	return fmt.Sprintf("%p:%s:%s", client, server, playerRef)
}
func (a *app) rememberRankMilestones(client *LCUClient, playerRef, server string, value *gameplayRankMilestones) *gameplayRankMilestones {
	key := a.rankMilestoneKey(client, playerRef, server)
	a.milestoneCache.mu.Lock()
	defer a.milestoneCache.mu.Unlock()
	if a.milestoneCache.entries == nil {
		a.milestoneCache.entries = map[string]*gameplayRankMilestones{}
	}
	if value != nil {
		if len(a.milestoneCache.entries) >= 256 {
			for k := range a.milestoneCache.entries {
				delete(a.milestoneCache.entries, k)
				break
			}
		}
		a.milestoneCache.entries[key] = value
	}
	return a.milestoneCache.entries[key]
}
func (a *app) loadRanksWithFallback(ctx context.Context, client *LCUClient, playerRef string, isCurrent bool, serverID, privacy string, tierScope ...bool) ([]gameplayRank, *gameplayRankMilestones, EndpointCapability) {
	if serverID == "" {
		serverID = clientTencentServerID(client)
	}
	ranks, milestones, capability := a.loadRanksWithFallbackRaw(ctx, client, playerRef, isCurrent, serverID, privacy, tierScope...)
	a.rememberOfficialRankWins(client, playerRef, serverID, ranks)
	if !isRiotRegion(serverID) {
		milestones = a.rememberRankMilestones(client, playerRef, serverID, milestones)
	}
	return ranks, milestones, capability
}
