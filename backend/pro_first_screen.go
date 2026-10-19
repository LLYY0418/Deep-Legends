package main

import (
	"context"
	"time"
)

// The first response is always local. Only a scoped refresh enriches this snapshot; unknown fields stay unknown.
func (a *app) proPlayersFirstScreen(force bool) ([]opggProTeam, time.Time, error) {
	c := &a.proPlayers
	a.restoreProSnapshot()
	c.mu.Lock()
	if len(c.teams) == 0 {
		c.teams = new(app).loadProSeeds(context.Background(), nil)
	}
	teams, at, err := cloneProTeams(c.teams), c.fetchedAt, c.err
	c.mu.Unlock()
	now := a.proProfileNow()
	for ti := range teams {
		for mi := range teams[ti].Members {
			for ai := range teams[ti].Members[mi].Summoners {
				row := &teams[ti].Members[mi].Summoners[ai]
				*row = expireProProfileFacts(*row, now)
			}
		}
	}
	return teams, at, err
}
