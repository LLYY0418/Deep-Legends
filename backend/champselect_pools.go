package main

// Ranked bans are shared; picks still use the assigned position. Preserve
// normalized legacy lane pools once, including candidates outside the new
// five-slot limit. Backups are not read by execution and cannot refill a pool
// that the user later clears. Existing shared/default candidates come first.
func migrateChampSelectSharedBan(side champSelectSideConfig, definition champSelectGroupDefinition) champSelectSideConfig {
	legacy := false
	for _, position := range definition.Positions {
		if position != "default" {
			if _, present := side.Champions[position]; present {
				legacy = true
			}
		}
	}
	// Sanitize backups on every load/save; at most six old pools of five IDs.
	backup := map[string][]int64{}
	for _, position := range definition.Positions {
		if ids := uniqueChampSelectBanIDs(side.LegacyLaneChampions[position], definition.BanLimit); len(ids) > 0 {
			backup[position] = ids
		}
	}
	if legacy {
		order := []string{"default"}
		for _, position := range definition.Positions {
			if position != "default" {
				order = append(order, position)
			}
		}
		candidates := []int64{}
		for _, position := range order {
			ids := uniqueChampSelectBanIDs(side.Champions[position], definition.BanLimit)
			if len(ids) > 0 {
				if _, exists := backup[position]; !exists {
					backup[position] = append([]int64(nil), ids...)
				}
				candidates = append(candidates, ids...)
			}
		}
		side.Champions = map[string][]int64{"default": uniqueChampSelectBanIDs(candidates, definition.BanLimit)}
	}
	if len(backup) == 0 {
		backup = nil
	}
	side.LegacyLaneChampions = backup
	return side
}

func uniqueChampSelectBanIDs(ids []int64, limit int) []int64 {
	result := []int64{}
	seen := map[int64]bool{}
	for _, id := range ids {
		if len(result) >= limit {
			break
		}
		if id > 0 && !seen[id] {
			result = append(result, id)
			seen[id] = true
		}
	}
	return result
}
