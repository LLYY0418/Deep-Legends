package main

import (
	"context"
	"sort"
	"strconv"
	"strings"
)

func riotOverviewFilter(filters []string) string {
	if len(filters) == 0 {
		return "all"
	}
	return normalizeGameplayMatchFilter(filters[0])
}

func riotOverviewPagination(start, loaded, requested int, filter string) gameplayPagination {
	return gameplayPagination{BegIndex: start, Count: loaded, HasMore: loaded == requested,
		Filter: filter, ServerFiltered: filter != "all" && riotDirectMatchFilter(filter),
		FilterFallback: filter != "all" && !riotDirectMatchFilter(filter)}
}

// Ask Match v5 for IDs in the requested queues, never download unrelated
// matches to discover whether a mode has any history. Each queue prefix is
// cached by matchIDsFiltered; only the final page's details are downloaded.
// KR match IDs share a monotonically increasing platform game ID, allowing
// multiple queue streams to merge before fetching any match payloads.
func (p *riotProvider) matchIDsForOverview(ctx context.Context, puuid string, start, count int, filter string) ([]string, error) {
	filter = normalizeGameplayMatchFilter(filter)
	if filter == "all" || !riotDirectMatchFilter(filter) {
		return p.matchIDs(ctx, puuid, start, count)
	}
	var queues []int64
	for _, definition := range supportedQueueDefinitions {
		if definition.Filter == filter || (filter == "ranked" && (definition.ID == 420 || definition.ID == 440)) {
			queues = append(queues, definition.ID)
		}
	}
	// Unknown queue families retain the bounded client-side fallback.
	if len(queues) == 0 {
		return p.matchIDs(ctx, puuid, start, count)
	}
	if len(queues) == 1 {
		return p.matchIDsFiltered(ctx, puuid, start, count, queues[0], "")
	}
	ids := make(map[string]struct{})
	for _, queue := range queues {
		for offset := 0; offset < start+count; offset += 100 {
			page, err := p.matchIDsFiltered(ctx, puuid, offset, 100, queue, "")
			if err != nil {
				return nil, err
			}
			for _, id := range page {
				ids[id] = struct{}{}
			}
			if len(page) < 100 {
				break
			}
		}
	}
	merged := make([]string, 0, len(ids))
	for id := range ids {
		merged = append(merged, id)
	}
	sort.Slice(merged, func(i, j int) bool { return riotGameID(merged[i]) > riotGameID(merged[j]) })
	if start >= len(merged) {
		return []string{}, nil
	}
	end := start + count
	if end > len(merged) {
		end = len(merged)
	}
	return merged[start:end], nil
}

func riotGameID(id string) int64 {
	_, number, _ := strings.Cut(id, "_")
	value, _ := strconv.ParseInt(number, 10, 64)
	return value
}

func riotDirectMatchFilter(filter string) bool {
	return filter != "more:hextech-classic" && filter != "more:hextech-qualifier" && filter != "more:special"
}
