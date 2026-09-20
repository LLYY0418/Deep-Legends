package main

import "context"

type currentGameRefreshKey struct{}

func currentGameManualRefresh(ctx context.Context) bool {
	value, _ := ctx.Value(currentGameRefreshKey{}).(bool)
	return value
}

func currentGameSummary(game *currentGame) map[string]any {
	fields := map[string]any{"has_result": game != nil}
	if game == nil {
		return fields
	}
	counts := []int{}
	for _, team := range game.Teams {
		counts = append(counts, len(team.Players))
	}
	fields["state"], fields["source"], fields["roster_counts"] = game.Status, game.Source, counts
	fields["has_start_time"] = game.StartedAt != nil
	outcome := "no-roster"
	if game.Status == "active" {
		outcome = "partial-roster"
		if len(counts) == 2 && counts[0] == 5 && counts[1] == 5 {
			outcome = "complete-roster"
		}
	}
	fields["roster_outcome"] = outcome
	return fields
}
