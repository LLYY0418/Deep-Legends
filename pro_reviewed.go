package main

import "sort"

// The reviewed list owns page membership and spelling. Cached directories only
// decorate exact Riot IDs; an old owner, seed index or PUUID cannot replace it.
// Apply at the response boundary so cold starts, restored snapshots and every
// asynchronous enrichment publication obey the same 33-player/53-account rule.
func (a *app) buildReviewedProPlayers(source []opggProTeam) proPlayersResponse {
	result := a.buildProPlayers(nil, proRoster)
	metadata := map[string]opggProAccount{}
	for _, team := range source {
		for _, member := range team.Members {
			for _, row := range member.Summoners {
				if _, valid := normalizeProAccount(row); !valid {
					continue
				}
				key := proLadderAccountKey(row.GameName, row.TagLine)
				old, exists := metadata[key]
				if !exists || row.CheckedAt > old.CheckedAt || row.CheckedAt == old.CheckedAt && (row.UpdatedAt > old.UpdatedAt || row.UpdatedAt == old.UpdatedAt && row.LastMatchAt > old.LastMatchAt) {
					metadata[key] = row
				}
			}
		}
	}
	result.MissingCount = 0
	result.Partial = proSupplementsIncomplete(source)
	for ti := range result.Teams {
		for pi := range result.Teams[ti].Players {
			player := &result.Teams[ti].Players[pi]
			for _, seed := range proSeedAccounts {
				if seed.TeamCode != result.Teams[ti].Code || seed.Player != player.Name {
					continue
				}
				for _, ref := range seed.Accounts {
					row := metadata[proLadderAccountKey(ref.GameName, ref.TagLine)]
					row.GameName, row.TagLine, row.Source = ref.GameName, ref.TagLine, "seed"
					account, _ := normalizeProAccount(row)
					account.Reviewed, account.Confidence = true, ""
					player.Accounts = append(player.Accounts, account)
					result.AccountCount++
					if account.Dormant {
						result.DormantCount++
					}
					if account.RankStatus == "ranked" && !account.LadderRankKnown {
						result.LadderRankPartial = true
					}
				}
			}
			player.Status = "available"
			// Preserve reviewed order for unknown/equal activity, never infer a
			// primary account from LP. Known activity still sorts newest first.
			sort.SliceStable(player.Accounts, func(i, j int) bool {
				x, y := player.Accounts[i], player.Accounts[j]
				if x.LastMatchAtKnown != y.LastMatchAtKnown {
					return x.LastMatchAtKnown
				}
				return x.LastMatchAtKnown && x.LastMatchAt > y.LastMatchAt
			})
		}
	}
	return result
}
