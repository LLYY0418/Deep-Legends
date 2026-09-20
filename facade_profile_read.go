package main

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

// A client can return an empty local customization while its summoner profile
// still has the applied background. Ask for the same logged-in account's
// profile before treating zero as the client's default background.
func (a *app) loadFacadeProfile(ctx context.Context, client *LCUClient, current Summoner) (SummonerProfile, EndpointCapability) {
	profile, capability := (SummonerAPI{client: client, ctx: ctx}).Profile()
	if profile.BackgroundSkinID > 0 || current.PUUID == "" {
		return profile, capability
	}
	localAvailable := capability.State == capabilityAvailable
	lookup, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	var observed SummonerProfile
	path := "/lol-summoner/v1/summoner-profile?puuid=" + url.QueryEscape(current.PUUID)
	err := client.RequestJSON(lookup, http.MethodGet, path, nil, &observed)
	if err == nil && observed.BackgroundSkinID > 0 {
		profile = observed
		capability.State = capabilityAvailable
	}
	a.recordDiagnostic(map[string]any{"event": "facade_background_read", "local_available": localAvailable, "account_profile_available": err == nil, "background_skin_id": profile.BackgroundSkinID})
	return profile, capability
}
