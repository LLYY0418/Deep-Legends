package main

import (
	"encoding/json"
	"sort"
	"strings"
)

func matchInHistoryFilter(match gameplayMatch, filter string) bool {
	if isCustomGameplayMatch(match) {
		return false
	}
	group := queueModeGroupForLabel(match.QueueID, match.GameMode, match.MapID, match.QueueLabel)
	switch filter {
	case "more:hextech-classic":
		return group == "hextech-classic"
	case "more:hextech-qualifier":
		return group == "hextech-qualifier"
	case "more:special":
		return group == "other"
	}
	return matchHistoryFilterFor(filter).accepts(&riotMatchInfo{QueueID: match.QueueID, GameMode: match.GameMode, MapID: match.MapID})
}
func mergeFallbackHistory(filter string, known, incoming []gameplayMatch) []gameplayMatch {
	byID := map[int64]gameplayMatch{}
	for _, rows := range [][]gameplayMatch{known, incoming} {
		for _, match := range rows {
			if matchInHistoryFilter(match, filter) {
				byID[match.GameID] = match
			}
		}
	}
	rows := make([]gameplayMatch, 0, len(byID))
	for _, match := range byID {
		rows = append(rows, match)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].CreatedAt != rows[j].CreatedAt {
			return rows[i].CreatedAt > rows[j].CreatedAt
		}
		return rows[i].GameID > rows[j].GameID
	})
	return rows
}

// Read only retained data, scoped to the same client platform/server/account.
// Season scans use the same retained SGP page store, including older pages.
func (a *app) retainedFilterHistory(client *LCUClient, reference gameplayReference, ref, filter string, begIndex, count int, names, labels map[int64]string) ([]gameplayMatch, int, bool) {
	rows := []gameplayMatch{}
	cachedRows := []gameplayMatch{}
	key := a.clientHistorySnapshotKey(clientRiotPlatform(client), reference.ServerID, ref)
	if key != "" {
		if raw, err := readLocalStoreFile(a.storage, key); err == nil {
			var snapshot clientHistorySnapshot
			if json.Unmarshal(raw, &snapshot) == nil {
				rows = append(rows, snapshot.Matches...)
			}
		}
	}
	if a.sgp != nil {
		prefix := sourceScopedKey(dataSourceSGP, strings.ToUpper(reference.ServerID)+"|"+ref+"|")
		a.sgp.mu.Lock()
		infos := map[int64]*riotMatchInfo{}
		for key, entry := range a.sgp.historyCache {
			if strings.HasPrefix(key, prefix) {
				if page, ok := a.sgp.materializeHistoryPageLocked(reference.ServerID, ref, entry); ok {
					for _, info := range page.games {
						if info != nil {
							infos[info.GameID] = info
						}
					}
				}
			}
		}
		a.sgp.mu.Unlock()
		for _, info := range infos {
			cachedRows = append(cachedRows, convertRiotMatchInfo(info, ref, names, labels, "", reference.ServerID))
		}
	}
	// Fallback cursors count unfiltered source rows. Page the combined retained
	// history once, then apply the mode filter; never prepend the snapshot head.
	rows = mergeFallbackHistory("all", rows, cachedRows)
	start := min(max(0, begIndex), len(rows))
	end := min(start+max(1, count), len(rows))
	return mergeFallbackHistory(filter, rows[start:end], nil), end - start, end < len(rows)
}

func missingHistoryMatches(filter string, known, shown []gameplayMatch) int {
	ids := map[int64]bool{}
	for _, row := range shown {
		ids[row.GameID] = true
	}
	missing := 0
	for _, row := range mergeFallbackHistory(filter, known, nil) {
		if !ids[row.GameID] {
			missing++
		}
	}
	return missing
}
