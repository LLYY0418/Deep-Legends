package main

import (
	"context"
	"strings"
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

// Playerlist is current-game evidence. Restore missing players by exact Riot ID,
// using the existing scoped identity resolver. Failed identity lookups preserve
// only playerlist fields, never an invented PUUID, champion or squad assignment.
func (a *app) recoverArenaPlayerList(ctx context.Context, client *LCUClient, current Summoner, response *gameplayLiveResponse, snapshot liveClientSnapshot, names map[int64]string) {
	if len(snapshot.RosterPlayers) <= len(response.Players) || len(snapshot.RosterPlayers) > 18 {
		return
	}
	for _, entry := range snapshot.RosterPlayers {
		if entry.GameName == "" || entry.TagLine == "" {
			continue
		}
		present := false
		for _, p := range response.Players {
			if strings.EqualFold(p.GameName, entry.GameName) && strings.EqualFold(p.TagLine, entry.TagLine) {
				present = true
				break
			}
		}
		if present {
			continue
		}
		ref := normalizeGameplayReference(gameplayReference{GameName: entry.GameName, TagLine: entry.TagLine, DisplayName: entry.GameName, Region: clientRiotPlatform(client), ServerID: clientTencentServerID(client)})
		a.arenaAlliesMu.RLock()
		allies := append([]lcuLivePlayer(nil), a.arenaAllyPlayers...)
		if a.arenaAllyGameID != 0 && a.arenaAllyGameID != response.GameID {
			allies = nil
		}
		a.arenaAlliesMu.RUnlock()
		for _, ally := range allies {
			known := normalizeGameplayReference(gameplayReference{PlayerRef: ally.PUUID, SummonerID: ally.SummonerID, GameName: ally.GameName, TagLine: ally.TagLine, DisplayName: ally.SummonerName, ServerID: clientTencentServerID(client)})
			if known.GameName == "" || known.TagLine == "" {
				if summoner, capability := loadGameplaySummoner(client, known); capability.State == capabilityAvailable {
					known.GameName, known.TagLine = summoner.GameName, summoner.TagLine
				}
			}
			if strings.EqualFold(known.GameName, entry.GameName) && strings.EqualFold(known.TagLine, entry.TagLine) {
				ref = known
				break
			}
		}
		if ref.PlayerRef == "" && ref.ServerID != "" {
			if resolved, err := a.resolveTencentRiotID(ctx, client, entry.GameName, entry.TagLine, ref.ServerID); err == nil {
				ref = resolved
			}
		}
		if ref.PlayerRef != "" {
			for _, p := range response.Players {
				if p.reference.PlayerRef == ref.PlayerRef {
					present = true
					break
				}
			}
		}
		if present {
			continue
		}
		championID := int64(0)
		for id, name := range names {
			if strings.EqualFold(strings.TrimSpace(name), strings.TrimSpace(entry.ChampionName)) && entry.ChampionName != "" {
				if championID != 0 {
					championID = 0
					break
				}
				championID = id
			}
		}
		self := ref.PlayerRef != "" && ref.PlayerRef == current.PUUID
		response.Players = append(response.Players, gameplayLivePlayer{gameplayPlayer: gameplayPlayer{PlayerRef: a.registerGameplayReferenceDetails(ref), GameName: entry.GameName, TagLine: entry.TagLine, DisplayName: entry.GameName, IsCurrent: self, Region: ref.Region, reference: ref}, TeamID: 100, IsAlly: self, ChampionID: championID, ChampionName: championName(names, championID), ChampionLocked: championID > 0, Position: "other", HistoryState: "unavailable"})
		response.MergeAppended++
	}
	deduplicateLiveResponse(response)
}
