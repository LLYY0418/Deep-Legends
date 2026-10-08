package main

// Same match-local result feeds card color, aggregates and streaks. Actual
// subteams and the known queue size provide independent lower bounds. Partial
// participant lists must not shrink a known arena, or truncate larger arenas.
func arenaPlacementResult(m gameplayMatch, placement int) string {
	if placement <= 0 {
		return "unknown"
	}
	teams := map[int64]bool{}
	for _, p := range m.Participants {
		if p.SubteamID > 0 {
			teams[p.SubteamID] = true
		}
	}
	count := len(teams)
	if m.QueueID == 1750 {
		count = max(count, 6)
	} else if isArenaQueue(m.QueueID, "") {
		count = max(count, 8)
	}
	if count < 2 {
		return "unknown"
	}
	if placement > (count+1)/2 {
		return "loss"
	}
	return "win"
}
func applyArenaResults(m *gameplayMatch) {
	if m.Result == "remake" || !isArenaQueue(m.QueueID, m.GameMode) && m.ModeGroup != "arena" {
		return
	}
	for i := range m.Participants {
		p := &m.Participants[i]
		result := arenaPlacementResult(*m, p.Placement)
		p.Win = result == "win"
		if p.ParticipantID == m.SubjectParticipantID {
			m.Result = result
		}
	}
}
