package main

import "context"

type localOverviewCardsProgressKey struct{}

func (a *app) publishOverviewCards(ctx context.Context, player Summoner, reference gameplayReference, isCurrent bool, ranks []gameplayRank, masteries []gameplayMastery, capability EndpointCapability) {
	progress, ok := ctx.Value(localOverviewCardsProgressKey{}).(func(gameplayOverview))
	if !ok || ctx.Err() != nil {
		return
	}
	partial := gameplayOverview{
		Player:  gameplayPlayer{PlayerRef: player.PUUID, DisplayName: gameplayDisplayName(player), GameName: player.GameName, TagLine: player.TagLine, ProfileIconID: player.ProfileIconID, SummonerLevel: player.SummonerLevel, Region: reference.Region, ServerID: reference.ServerID, ServerName: tencentServerName(reference.ServerID), IsCurrent: isCurrent, reference: reference},
		Matches: []gameplayMatch{}, Ranks: append([]gameplayRank(nil), ranks...), Masteries: append([]gameplayMastery(nil), masteries...), Capabilities: []EndpointCapability{capability}, ProfilePending: capability.Name != "summoner" || capability.State != capabilityAvailable,
	}
	a.publicizeOverviewReferences(&partial)
	progress(partial)
}

func (flight *overviewQueryFlight) publishOverviewCards(partial gameplayOverview) {
	flight.progressMu.Lock()
	if previous := flight.cardProgress; previous != nil {
		if partial.ProfilePending && !previous.ProfilePending && previous.Player.SummonerLevel > 0 {
			partial.Player.ProfileIconID = previous.Player.ProfileIconID
			partial.Player.SummonerLevel = previous.Player.SummonerLevel
			partial.ProfilePending = false
		}
		if len(partial.Matches) == 0 {
			partial.Matches = previous.Matches
		}
		if len(partial.Ranks) == 0 {
			partial.Ranks = previous.Ranks
		}
		if len(partial.Masteries) == 0 {
			partial.Masteries = previous.Masteries
		}
		ready := map[string]bool{}
		for _, capability := range partial.Capabilities {
			ready[capability.Name] = true
		}
		for _, capability := range previous.Capabilities {
			if !ready[capability.Name] {
				partial.Capabilities = append(partial.Capabilities, capability)
			}
		}
	}
	flight.cardProgress = &partial
	listeners := make([]func(gameplayOverview), 0, len(flight.cardListeners))
	for _, listener := range flight.cardListeners {
		listeners = append(listeners, listener)
	}
	flight.progressMu.Unlock()
	for _, listener := range listeners {
		listener(partial)
	}
}
