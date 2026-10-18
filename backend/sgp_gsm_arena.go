package main

func arenaLiveRosterAtCapacity(response gameplayLiveResponse) bool {
	limit := liveGSMRosterSize(response.QueueID)
	return limit > 0 && len(response.Players) >= limit
}

func arenaLiveIdentityPending(response gameplayLiveResponse) bool {
	if !isArenaQueue(response.QueueID, response.GameMode) || !gameplayIdentityPhase(response.Phase) {
		return false
	}
	expected := liveGSMRosterSize(response.QueueID)
	if expected == 0 {
		return false
	}
	if len(response.Players) < expected {
		return true
	}
	for _, player := range response.Players {
		if player.Hidden || player.IdentityUnresolved || player.GameName != "" && player.PlayerRef == "" && player.reference.PlayerRef == "" {
			return true
		}
	}
	return false
}

// CHERRY permits duplicate champions and exposes everyone on one side. Merge
// only by verified PUUID, never by champion, list position or teamParticipantId.
func mergeGSMArenaRoster(raw, supplemental []liveRosterEntry, limit int) ([]liveRosterEntry, int, int, string) {
	sources := make(map[string]liveRosterEntry, len(supplemental))
	for _, row := range supplemental {
		sources[row.player.PUUID] = row
	}
	complete := len(supplemental) == limit
	rows := make([]liveRosterEntry, 0, limit)
	indices := make(map[string]int, len(raw))
	anonymous := 0
	for _, row := range raw {
		ref := visibleLivePlayerReference(row.player)
		if !validPlayerReference(ref) || ref == emptyLCUPlayerPUUID {
			anonymous++
			continue
		}
		source, found := sources[ref]
		if complete && !found || found && row.player.ChampionID > 0 && source.player.ChampionID > 0 && row.player.ChampionID != source.player.ChampionID {
			return raw, 0, 0, "roster-conflict"
		}
		if _, exists := indices[ref]; exists {
			continue
		}
		row.team = 100
		indices[ref] = len(rows)
		rows = append(rows, row)
	}
	// A complete verified roster can replace anonymous slots wholesale, without
	// attributing their metadata to an arbitrary player with the same champion.
	if anonymous > 0 && !complete {
		return raw, 0, 0, "anonymous-roster-incomplete"
	}
	restored := 0
	for _, source := range supplemental {
		if index, found := indices[source.player.PUUID]; found {
			mergeGSMPlayerDetails(&rows[index].player, source.player)
			continue
		}
		if len(rows) >= limit {
			return raw, 0, 0, "roster-conflict"
		}
		source.team = 100
		indices[source.player.PUUID] = len(rows)
		rows = append(rows, source)
		restored++
	}
	return rows, max(0, len(rows)-len(raw)), restored, "merged"
}
