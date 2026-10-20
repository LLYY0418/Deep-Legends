package main

import (
	"context"
	"errors"
	"strings"
	"time"
)

func gameplaySummonerHasName(player Summoner) bool {
	return strings.TrimSpace(player.GameName) != "" || strings.TrimSpace(player.DisplayName) != ""
}

// The overview and live cards must use the same Tencent profile endpoint and
// server scope. A queryable hidden identity does not require previous encounters.
func (a *app) loadSGPGameplaySummoner(ctx context.Context, client *LCUClient, reference gameplayReference) (Summoner, EndpointCapability) {
	capability := EndpointCapability{Name: "summoner", Path: "sgp: /summoner-ledge/v1/regions/{server}/summoners/puuids"}
	var loaded sgpSummoner
	var err error
	if a.sgp == nil {
		err = errors.New("SGP 召唤师数据源不可用")
	} else if !validPlayerReference(reference.PlayerRef) || reference.PlayerRef == emptyLCUPlayerPUUID || reference.ServerID == "" {
		err = errors.New("召唤师身份或区服不可用")
	} else {
		loaded, err = a.sgp.summonerByPUUIDOn(ctx, client, reference.ServerID, reference.PlayerRef)
		if err == nil && loaded.PUUID != "" && loaded.PUUID != reference.PlayerRef {
			err = errors.New("召唤师身份与请求不一致")
		}
	}
	if err != nil {
		switch {
		case isCancellation(err):
			capability.State = capabilityCanceled
		case a.sgp == nil:
			capability.State = capabilityUnsupported
			capability.Detail = "当前未启用跨服召唤师数据源"
		default:
			capability.State = capabilityFailed
			capability.Detail = "所选服务器暂时无法读取召唤师资料"
		}
		return Summoner{}, capability
	}
	capability.State, capability.Count = capabilityAvailable, 1
	return Summoner{PUUID: reference.PlayerRef, DisplayName: loaded.Name, ProfileIconID: loaded.ProfileIconID, SummonerLevel: loaded.Level, Privacy: loaded.Privacy}, capability
}

// LCU remains the fast path. If it cannot supply a name, query the same profile
// source used when clicking the card to open an overview, within a shared budget.
func (a *app) loadLiveGameplaySummoner(parent context.Context, client *LCUClient, reference gameplayReference) (Summoner, EndpointCapability) {
	ctx, cancel := context.WithTimeout(parent, 4*time.Second)
	defer cancel()
	lcuCtx, cancelLCU := context.WithTimeout(ctx, 1500*time.Millisecond)
	loaded, capability := loadGameplaySummonerContext(lcuCtx, client, reference)
	cancelLCU()
	if capability.State == capabilityAvailable && gameplaySummonerHasName(loaded) {
		return loaded, capability
	}
	if ctx.Err() != nil || !isTencentClient(client) || reference.ServerID == "" || !validPlayerReference(reference.PlayerRef) || reference.PlayerRef == emptyLCUPlayerPUUID {
		return loaded, capability
	}
	if fallback, fallbackCapability := a.loadSGPGameplaySummoner(ctx, client, reference); fallbackCapability.State == capabilityAvailable && gameplaySummonerHasName(fallback) {
		// Do not inherit a placeholder name from the failed/nameless LCU response.
		loaded.GameName, loaded.TagLine, loaded.DisplayName = "", "", ""
		return mergeSummonerIdentity(fallback, loaded), fallbackCapability
	}
	return loaded, capability
}

// Hidden source names may be champion aliases. Do not put them into the public
// reference cache: a later verified profile with only DisplayName would otherwise
// inherit that earlier alias as its GameName when references are merged.
func liveGameplayReference(client *LCUClient, raw lcuLivePlayer) gameplayReference {
	ref := normalizeGameplayReference(gameplayReference{ClientIdentity: true, PlayerRef: visibleLivePlayerReference(raw), AlternatePlayerRef: raw.ObfuscatedPUUID, SummonerID: raw.SummonerID, AlternateSummonerID: raw.ObfuscatedSummonerID, GameName: raw.GameName, TagLine: raw.TagLine, DisplayName: raw.SummonerName, ProfileIconID: raw.ProfileIconID, Region: clientRiotPlatform(client), ServerID: clientTencentServerID(client)})
	if strings.EqualFold(strings.TrimSpace(raw.NameVisibilityType), "HIDDEN") {
		ref.GameName, ref.TagLine, ref.DisplayName = "", "", ""
	}
	ref.TagVisibilityKnown = true
	ref.TagBlocked = strings.EqualFold(strings.TrimSpace(raw.NameVisibilityType), "HIDDEN") || (ref.GameName == "" && ref.DisplayName == "")
	return ref
}
