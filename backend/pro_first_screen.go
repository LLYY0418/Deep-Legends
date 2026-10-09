package main

import (
	"context"
	"time"
)

// The first response is always local. Public directory, relay and supplements
// enrich immutable snapshots in the background; unknown fields stay unknown.
func (a *app) proPlayersFirstScreen(force bool) ([]opggProTeam, time.Time, error) {
	c := &a.proPlayers
	c.mu.Lock()
	a.restoreProSnapshotLocked()
	if len(c.teams) == 0 {
		c.teams = new(app).loadProSeeds(context.Background(), nil)
	}
	teams, at, err := c.teams, c.fetchedAt, c.err
	stale := at.IsZero() || time.Since(at) >= proPlayersTTL
	refresh := !c.firstScreenFlight && !c.updating && (force || stale || proSupplementsIncomplete(teams)) && (force || c.attemptedAt.IsZero() || time.Since(c.attemptedAt) >= proPlayersRetry)
	if refresh {
		c.firstScreenFlight = true
	}
	c.mu.Unlock()
	if refresh {
		a.goSafe("pro-players-first-screen-refresh", func() {
			defer func() { c.mu.Lock(); c.firstScreenFlight = false; c.mu.Unlock() }()
			background := a.proBusinessContext()
			if background == nil {
				background = context.Background()
			}
			_, _, _ = a.loadProPlayers(background, force)
		})
	}
	return teams, at, err
}
