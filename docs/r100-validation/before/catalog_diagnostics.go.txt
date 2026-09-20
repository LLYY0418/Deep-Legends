package main

import (
	"errors"
	"time"
)

// Catalog attempts and the final HTTP result are intentionally separate: an
// unavailable LCU followed by a healthy public catalog is a successful fallback.
func (a *app) reportCatalogLoad(endpoint, source string, count int, started time.Time, err error) {
	outcome, kind := "success", "none"
	if err == nil && count == 0 {
		err = errors.New("empty response")
	}
	if err != nil {
		outcome, kind = "failure", championProviderErrorKind(err)
	}
	a.recordDiagnostic(map[string]any{
		"event": "catalog_load", "endpoint": endpoint, "source": source,
		"items": count, "duration_ms": time.Since(started).Milliseconds(),
		"outcome": outcome, "error_kind": kind,
	})
}

func riotMatchItemsDiagnostic(match gameplayMatch) map[string]any {
	present, zero, slots, nonzero := 0, 0, 0, 0
	participants := make([]map[string]int, 0, len(match.Participants))
	for index, participant := range match.Participants {
		n := 0
		for _, id := range participant.ItemIDs {
			if id > 0 {
				n++
			}
		}
		if n > 0 {
			present++
		} else {
			zero++
		}
		slots += len(participant.ItemIDs)
		nonzero += n
		participants = append(participants, map[string]int{"index": index, "length": len(participant.ItemIDs), "nonzero": n})
	}
	return map[string]any{"event": "riot_match_items", "participants": participants,
		"items_present_count": present, "items_zero_count": zero,
		"item_slots_count": slots, "item_nonzero_count": nonzero}
}
