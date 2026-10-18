package main

import (
	"context"
	"strings"
	"sync"
	"time"
)

// TeamOne is authoritative in CHERRY; TeamTwo can still hold the previous 5v5.
// Other modes retain both teams. First occurrence wins, so TeamOne also wins
// when an old TeamTwo entry repeats the same PUUID.
func gameflowLiveRoster(session lcuGameflowSession) ([]liveRosterEntry, int, int) {
	rows := make([]liveRosterEntry, 0, len(session.GameData.TeamOne)+len(session.GameData.TeamTwo))
	for _, p := range session.GameData.TeamOne {
		rows = append(rows, liveRosterEntry{p, 100})
	}
	mode := session.GameData.Queue.GameMode
	if mode == "" {
		mode = session.Map.GameMode
	}
	stale := 0
	if isArenaQueue(session.GameData.Queue.ID, mode) {
		stale = len(session.GameData.TeamTwo)
	} else {
		for _, p := range session.GameData.TeamTwo {
			rows = append(rows, liveRosterEntry{p, 200})
		}
	}
	rows, duplicates := deduplicateLiveRoster(rows)
	return rows, stale, duplicates
}
func deduplicateLiveRoster(rows []liveRosterEntry) ([]liveRosterEntry, int) {
	result := make([]liveRosterEntry, 0, len(rows))
	seen := map[string]bool{}
	dropped := 0
	for _, row := range rows {
		ref := strings.TrimSpace(visibleLivePlayerReference(row.player))
		if ref != "" {
			if seen[ref] {
				dropped++
				continue
			}
			seen[ref] = true
		}
		result = append(result, row)
	}
	return result, dropped
}
func deduplicateLiveResponse(response *gameplayLiveResponse) {
	seen := map[string]bool{}
	players := make([]gameplayLivePlayer, 0, len(response.Players))
	for _, p := range response.Players {
		ref := strings.TrimSpace(p.reference.PlayerRef)
		if ref == "" {
			ref = strings.TrimSpace(p.PlayerRef)
		}
		if ref != "" {
			if seen[ref] {
				response.DuplicatesDropped++
				continue
			}
			seen[ref] = true
		}
		players = append(players, p)
	}
	response.Players = players
}

// Restore queryable ChampSelect members before the normal profile/history fan-out.
// Arena allows duplicate champions: a champion alone never identifies a member.
func (a *app) restoreArenaIdentityRoster(response *gameplayLiveResponse, raw []liveRosterEntry) []liveRosterEntry {
	limit := liveGSMRosterSize(response.QueueID)
	if limit == 0 || !gameplayIdentityPhase(response.Phase) {
		return raw
	}
	a.arenaAlliesMu.RLock()
	allies := append([]lcuLivePlayer(nil), a.arenaAllyPlayers...)
	gameID := a.arenaAllyGameID
	a.arenaAlliesMu.RUnlock()
	if gameID != response.GameID || gameID <= 0 {
		return raw
	}
	for _, ally := range allies {
		ref := visibleLivePlayerReference(ally)
		if !validPlayerReference(ref) || ref == emptyLCUPlayerPUUID {
			continue
		}
		present := false
		for _, row := range raw {
			if visibleLivePlayerReference(row.player) == ref {
				present = true
				break
			}
		}
		if present || len(raw) >= limit {
			continue
		}
		ally.PUUID, ally.ObfuscatedPUUID = ref, ""
		raw = append(raw, liveRosterEntry{player: ally, team: 100})
		response.MergeAppended++
	}
	return raw
}

func arenaRiotIDKey(name, tag string) string {
	if strings.TrimSpace(name) == "" || strings.TrimSpace(tag) == "" {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(name) + "#" + strings.TrimSpace(tag))
}

// Recover identities before enrichment so restored players receive the same
// history, fame and progress updates as gameflow players. No roster-order or
// champion matching is used to assign an identity to an anonymous row.
func (a *app) recoverArenaPlayerList(ctx context.Context, client *LCUClient, response *gameplayLiveResponse, raw []liveRosterEntry, snapshot liveClientSnapshot, names map[int64]string) []liveRosterEntry {
	limit := liveGSMRosterSize(response.QueueID)
	if limit == 0 || !gameplayIdentityPhase(response.Phase) || !liveGSMRosterNeeded(raw, response.QueueID) {
		return raw
	}
	ctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	before, anonymous, attempts := len(raw), 0, 0
	reason := "partial"
	defer func() {
		a.recordDiagnostic(map[string]any{"event": "arena_identity_recovery", "game_id": response.GameID, "queue_id": response.QueueID, "phase": response.Phase, "playerlist_count": len(snapshot.RosterPlayers), "players_before": before, "players_after": len(raw), "anonymous_entries": anonymous, "alias_attempts": attempts, "reason": reason})
	}()
	if len(snapshot.RosterPlayers) == 0 || len(snapshot.RosterPlayers) > limit {
		reason = "playerlist-unavailable"
		return raw
	}
	// Gameflow frequently gives only PUUIDs. Resolve these once (shared cache)
	// before comparing full Riot IDs, otherwise every known player looks missing.
	raw = append([]liveRosterEntry(nil), raw...)
	var wait sync.WaitGroup
	sem := make(chan struct{}, 4)
	for i := range raw {
		if arenaRiotIDKey(raw[i].player.GameName, raw[i].player.TagLine) != "" {
			continue
		}
		wait.Add(1)
		go func(i int) {
			defer wait.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()
			p := &raw[i].player
			ref := gameplayReference{ClientIdentity: true, PlayerRef: visibleLivePlayerReference(*p), AlternatePlayerRef: p.ObfuscatedPUUID, SummonerID: p.SummonerID, AlternateSummonerID: p.ObfuscatedSummonerID, Region: clientRiotPlatform(client), ServerID: clientTencentServerID(client)}
			if s, cap := a.loadLiveGameplaySummoner(ctx, client, ref); cap.State == capabilityAvailable {
				p.GameName, p.TagLine = s.GameName, s.TagLine
				if validPlayerReference(s.PUUID) {
					p.PUUID, p.ObfuscatedPUUID = s.PUUID, ""
				}
			}
		}(i)
	}
	wait.Wait()
	byName := map[string]lcuLivePlayer{}
	conflicting := map[string]bool{}
	for _, row := range raw {
		key := arenaRiotIDKey(row.player.GameName, row.player.TagLine)
		if key == "" {
			continue
		}
		if previous, ok := byName[key]; ok && visibleLivePlayerReference(previous) != visibleLivePlayerReference(row.player) {
			conflicting[key] = true
		}
		byName[key] = row.player
	}
	candidates := []liveRosterEntry{}
	seen := map[string]bool{}
	seenRefs := map[string]bool{}
	matched := map[string]bool{}
	allResolved := true
	for _, entry := range snapshot.RosterPlayers {
		key := arenaRiotIDKey(entry.GameName, entry.TagLine)
		if key == "" || entry.IsBot {
			anonymous++
			allResolved = false
			continue
		}
		if seen[key] || conflicting[key] {
			reason = "identity-conflict"
			return raw
		}
		seen[key] = true
		p, found := byName[key]
		if found {
			matched[key] = true
		} else {
			p = lcuLivePlayer{GameName: entry.GameName, TagLine: entry.TagLine, SummonerName: entry.GameName}
		}
		if (!validPlayerReference(visibleLivePlayerReference(p)) || visibleLivePlayerReference(p) == emptyLCUPlayerPUUID) && ctx.Err() == nil && clientTencentServerID(client) != "" {
			attempts++
			if ref, err := a.resolveTencentRiotID(ctx, client, entry.GameName, entry.TagLine, clientTencentServerID(client)); err == nil {
				p.PUUID, p.ObfuscatedPUUID = ref.PlayerRef, ""
			}
		}
		if !validPlayerReference(visibleLivePlayerReference(p)) || visibleLivePlayerReference(p) == emptyLCUPlayerPUUID {
			allResolved = false
		}
		if p.ChampionID <= 0 {
			for id, name := range names {
				if entry.ChampionName != "" && strings.EqualFold(strings.TrimSpace(name), strings.TrimSpace(entry.ChampionName)) {
					if p.ChampionID != 0 {
						p.ChampionID = 0
						break
					}
					p.ChampionID = id
				}
			}
		}
		p.PUUID, p.ObfuscatedPUUID = visibleLivePlayerReference(p), ""
		if validPlayerReference(p.PUUID) && p.PUUID != emptyLCUPlayerPUUID {
			if seenRefs[p.PUUID] {
				reason = "identity-conflict"
				return raw
			}
			seenRefs[p.PUUID] = true
		}
		candidates = append(candidates, liveRosterEntry{player: p, team: 100})
	}
	if ctx.Err() != nil {
		reason = "cancelled"
		return raw
	}
	if allResolved && len(candidates) == limit {
		var appended int
		raw, appended, _, reason = mergeGSMArenaRoster(raw, candidates, limit)
		response.MergeAppended += appended
		return raw
	}
	// A partial source can add named rows only after every existing row has an
	// exact match; otherwise it could duplicate an unidentified existing player.
	if len(matched) != len(raw) {
		reason = "existing-identities-unmatched"
		return raw
	}
	for _, row := range candidates {
		key := arenaRiotIDKey(row.player.GameName, row.player.TagLine)
		if matched[key] {
			for i := range raw {
				if arenaRiotIDKey(raw[i].player.GameName, raw[i].player.TagLine) == key {
					raw[i].player = row.player
					break
				}
			}
			continue
		}
		if len(raw) >= limit {
			continue
		}
		duplicate := false
		for _, existing := range raw {
			if validPlayerReference(row.player.PUUID) && visibleLivePlayerReference(existing.player) == row.player.PUUID {
				duplicate = true
				break
			}
		}
		if duplicate {
			continue
		}
		raw = append(raw, row)
		response.MergeAppended++
	}
	reason = "merged-partial"
	return raw
}
