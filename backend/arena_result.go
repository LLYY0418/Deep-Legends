package main

// Same match-local result feeds card color, aggregates and streaks. Actual
// subteams take precedence over queue fallback; no placement means unknown.
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
	if count < 2 {
		if m.QueueID == 1750 {
			count = 6
		} else if isArenaQueue(m.QueueID, "") {
			count = 8
		} else {
			return "unknown"
		}
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
