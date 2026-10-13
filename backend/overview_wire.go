package main

// These wrappers are wire-only. Cached matches and the single-match endpoint
// retain score parts, so hovering details never has to refresh the whole page.
type overviewListScore struct {
	Value float64 `json:"value"`
	Rank  int     `json:"rank"`
	Total int     `json:"total"`
	Badge string  `json:"badge"`
}
type overviewParticipantFields gameplayParticipant

type overviewListParticipant struct {
	overviewParticipantFields
	Score *overviewListScore `json:"score,omitempty"`
}
type overviewListMatch struct {
	gameplayMatch
	Participants []overviewListParticipant `json:"participants"`
}
type overviewMatchReference struct {
	GameID   int64  `json:"gameId"`
	Revision string `json:"revision"`
}
type overviewListWire struct {
	gameplayOverview
	Matches []any `json:"matches"`
}

func overviewListMatches(matches []gameplayMatch) []any {
	result := make([]any, 0, len(matches))
	for _, match := range matches {
		players := make([]overviewListParticipant, len(match.Participants))
		for i, p := range match.Participants {
			players[i].overviewParticipantFields = overviewParticipantFields(p)
			if p.Score != nil {
				players[i].Score = &overviewListScore{p.Score.Value, p.Score.Rank, p.Score.Total, p.Score.Badge}
			}
		}
		result = append(result, overviewListMatch{match, players})
	}
	return result
}
func overviewWireFrame(value gameplayOverview, kind string, seen map[int64]string) overviewListWire {
	if kind != "complete" {
		value = compactOverviewProgress(value, seen)
		// Derived card samples change while details arrive; publish once when final.
		value.RankedQueues = nil
	}
	matches := overviewListMatches(value.Matches)
	if kind == "complete" {
		for i, match := range value.Matches {
			if revision, ok := seen[match.GameID]; ok && match.Revision != "" && revision == match.Revision {
				matches[i] = overviewMatchReference{match.GameID, match.Revision}
			}
		}
	}
	return overviewListWire{value, matches}
}

// Preserve missing-score-input semantics of the original participant encoder.
func (p overviewListParticipant) MarshalJSON() ([]byte, error) {
	type plain overviewListParticipant
	return marshalScorePresence(plain(p), p.scoreMissing, false)
}
