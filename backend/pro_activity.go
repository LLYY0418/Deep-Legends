package main

import (
	"context"
	"time"
)

// The ID lookup is unfiltered: normal games and ARAM also indicate activity.
// Errors are not cached; no records is a successful, explicitly unknown result.
func (p *riotProvider) lastMatchStart(ctx context.Context, puuid string) (time.Time, bool, error) {
	return p.lastMatchStartCached(ctx, puuid, "proseed-lastmatch:v1:", 7*24*time.Hour)
}

func (p *riotProvider) lastMatchStartCached(ctx context.Context, puuid, prefix string, ttl time.Duration) (time.Time, bool, error) {
	var value struct {
		At    time.Time `json:"at"`
		Known bool      `json:"known"`
	}
	err := p.cachedPublicIdentity(ctx, prefix+puuid, ttl, &value, func(ctx context.Context) error {
		ids, err := p.matchIDsFiltered(ctx, puuid, 0, 1, 0, "")
		if err != nil {
			return err
		}
		if len(ids) == 0 {
			return nil
		}
		match, _, err := p.matchByIDWithCache(ctx, ids[0])
		if err != nil {
			return err
		}
		// Older cached payloads can lack this field. Never substitute creation time.
		if match.Info.GameStartTimestamp > 0 {
			value.At, value.Known = time.UnixMilli(match.Info.GameStartTimestamp).UTC(), true
		}
		return nil
	})
	return value.At, value.Known, err
}
func setProLastMatch(row *opggProAccount, at time.Time, known bool) {
	row.LastMatchAt, row.LastMatchAtKnown = "", known
	if known {
		row.LastMatchAt = at.UTC().Format(time.RFC3339Nano)
	}
}

// Only the background publisher performs HTTP; renderer and badge reads stay
// pure snapshot projections. Seed activity was already fetched in its budget.
func (a *app) enrichProActivity(ctx context.Context, teams []opggProTeam, previous []opggProTeam) {
	ctx, cost := newProRefreshCost(withRiotBackground(ctx))
	defer func() { a.recordProRefreshCost("pro_activity_cost", cost) }()
	if a.riot == nil || !riotKeyConfigured() {
		return
	}
	old := map[string]opggProAccount{}
	for _, team := range previous {
		for _, member := range team.Members {
			for _, row := range member.Summoners {
				if row.PUUID != "" && row.LastMatchAtKnown {
					old[row.PUUID] = row
				}
			}
		}
	}
	type activity struct {
		at    time.Time
		known bool
		err   error
	}
	seen := map[string]activity{}
	for ti := range teams {
		team := &teams[ti]
		for mi := range team.Members {
			member := &team.Members[mi]
			if _, ok := proReviewedMemberBadge(*team, *member); !ok {
				continue
			}
			for ai := range member.Summoners {
				row := &member.Summoners[ai]
				if row.PUUID == "" || row.Source == "seed" {
					continue
				}
				if _, valid := normalizeProAccount(*row); !valid {
					continue
				}
				value, ok := seen[row.PUUID]
				if !ok {
					cost.total++
					step, cancel := context.WithTimeout(ctx, proSeedPerAccountBudget)
					value.at, value.known, value.err = a.riot.lastMatchStart(withRiotSingleWaitLimit(step, 100*time.Millisecond), row.PUUID)
					cancel()
					seen[row.PUUID] = value
					if value.err == nil {
						cost.refreshed++
					}
				}
				if value.err == nil {
					setProLastMatch(row, value.at, value.known)
				} else if fallback, ok := old[row.PUUID]; ok {
					row.LastMatchAt, row.LastMatchAtKnown = fallback.LastMatchAt, fallback.LastMatchAtKnown
				}
			}
		}
	}
}

func proSeedTotalAccounts() int {
	count := 0
	for _, seed := range proSeedAccounts {
		count += len(seed.Accounts)
	}
	return count
}

func proActivityAccountCount(teams []opggProTeam) int {
	ids := map[string]bool{}
	for _, team := range teams {
		for _, member := range team.Members {
			if _, ok := proReviewedMemberBadge(team, member); !ok {
				continue
			}
			for _, row := range member.Summoners {
				if row.PUUID != "" && row.Source != "seed" {
					ids[row.PUUID] = true
				}
			}
		}
	}
	return len(ids)
}
