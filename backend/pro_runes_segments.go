package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"
)

const proOPGGSegmentTTL = 30 * time.Minute
const proOPGGSegmentCacheKey = "opgg-segments-v1"

type proRuneScope struct {
	proRuneLeague
	Serie string
	Last  time.Time
}

type proRunePeriod struct {
	Scopes        []proRuneScope
	After, Before time.Time
}

func (period proRunePeriod) key() string {
	// The last map advances during a tournament. Only changes to eligibility
	// (league/serie or the separating international boundary) change the key.
	h := sha256.New()
	fmt.Fprintf(h, "%s:%s", period.After.UTC().Format(time.RFC3339Nano), period.Before.UTC().Format(time.RFC3339Nano))
	for _, scope := range period.Scopes {
		fmt.Fprintf(h, ":%s=%s", scope.ID, scope.Serie)
	}
	return fmt.Sprintf("%x", h.Sum(nil)[:12])
}

func (period proRunePeriod) buildKey(championID int) string {
	return "opgg-builds-" + strconv.Itoa(championID) + "-" + period.key()
}

func proLatestInternational(scopes []proRuneScope) (latest proRuneScope) {
	for _, scope := range scopes {
		if !scope.Domestic && scope.Last.After(latest.Last) {
			latest = scope
		}
	}
	return latest
}

func proCurrentPeriod(scopes []proRuneScope) (period proRunePeriod) {
	international := proLatestInternational(scopes)
	for _, scope := range scopes {
		if scope.Domestic && scope.Last.After(international.Last) {
			period.Scopes = append(period.Scopes, scope)
		}
	}
	if len(period.Scopes) > 0 {
		period.After = international.Last
	} else if international.Serie != "" {
		period.Scopes = []proRuneScope{international}
	}
	return period
}

type proOPGGSegmentState struct {
	Heads         []proRuneScope
	At, Attempted time.Time
}

func (p *proRuneProvider) opggSegmentState(ctx context.Context, now time.Time) (state proOPGGSegmentState, err error) {
	err = p.gameResource(ctx, proOPGGSegmentCacheKey, func() error {
		p.mu.Lock()
		state = p.opggSegments
		p.mu.Unlock()
		if state.Attempted.IsZero() && p.readDisk(proOPGGSegmentCacheKey, &state) {
			if state.At.IsZero() || state.At.After(now) {
				state = proOPGGSegmentState{}
			} else {
				state.Attempted = state.At
			}
		}
		if state.Attempted.IsZero() || now.Sub(state.Attempted) >= proOPGGSegmentTTL {
			heads, loadErr := p.opggHeads(ctx, now)
			if ctx.Err() != nil {
				return ctx.Err()
			}
			state.Attempted = now
			if loadErr == nil {
				state.Heads, state.At = heads, now
				p.writeDisk(proOPGGSegmentCacheKey, state)
			}
		}
		p.mu.Lock()
		p.opggSegments = state
		p.mu.Unlock()
		if state.At.IsZero() {
			return errors.New("OP.GG tournament metadata unavailable")
		}
		return nil
	})
	return state, err
}

func opggScope(league proRuneLeague, row proOPGGBuild, now time.Time) (proRuneScope, error) {
	serie, at := row.Game.Match.Tournament.Serie.ID, row.Game.BeginAt
	if !proNumericID(serie) || !proNumericID(row.Game.ID) || at.IsZero() || at.After(now) {
		return proRuneScope{}, errors.New("invalid OP.GG tournament metadata")
	}
	return proRuneScope{league, serie, at}, nil
}

func (p *proRuneProvider) opggHeads(ctx context.Context, now time.Time) ([]proRuneScope, error) {
	heads := make([]proRuneScope, len(proOPGGLeagues))
	var mu sync.Mutex
	var failed bool
	indices := make([]int, len(heads))
	for i := range indices {
		indices[i] = i
	}
	proParallelLimit(ctx, indices, 3, func(i int) {
		league := proOPGGLeagues[i]
		rows, err := p.fetchOPGGBuilds(ctx, 0, league.ID, 0, 1)
		if err == nil && len(rows) == 1 {
			heads[i], err = opggScope(league, rows[0], now)
		}
		if err != nil {
			mu.Lock()
			failed = true
			mu.Unlock()
		}
	})
	if failed || ctx.Err() != nil {
		return nil, errors.New("OP.GG tournament metadata unavailable")
	}
	return heads, nil
}

// Historical reads are only needed for a champion with no rows in the active
// international segment. Visit the global, date-ordered feed without a champion.
func (p *proRuneProvider) walkOPGGSeries(ctx context.Context, league proRuneLeague, now time.Time, visit func(proRuneScope, string) bool) error {
	previous := now
	for page := 0; page < proOPGGMaxPages; page++ {
		rows, err := p.fetchOPGGBuilds(ctx, 0, league.ID, page, proOPGGPageSize)
		if err != nil {
			return err
		}
		stopped := false
		for _, row := range rows {
			scope, err := opggScope(league, row, previous)
			if err != nil {
				return err
			}
			previous = scope.Last
			if !stopped {
				stopped = !visit(scope, row.Game.ID)
			}
		}
		if stopped || len(rows) < proOPGGPageSize {
			return nil
		}
	}
	return errors.New("OP.GG tournament history pagination limit")
}

func (p *proRuneProvider) previousOPGGPeriod(ctx context.Context, state proOPGGSegmentState, current proRunePeriod, now time.Time) (period proRunePeriod, err error) {
	international := proLatestInternational(state.Heads)
	if international.Serie == "" {
		return period, nil
	}
	if len(current.Scopes) != 0 && current.Scopes[0].Domestic {
		return proRunePeriod{Scopes: []proRuneScope{international}}, nil
	}

	err = p.gameResource(ctx, "opgg-previous-"+current.key(), func() error {
		p.mu.Lock()
		at, cached := p.opggPreviousAt, p.opggPrevious
		p.mu.Unlock()
		if at.Equal(state.At) {
			period = cached
			return nil
		}
		first := international.Last
		var earlier proRuneScope
		err := p.walkOPGGSeries(ctx, international.proRuneLeague, now, func(scope proRuneScope, _ string) bool {
			if scope.Serie == international.Serie {
				first = scope.Last
				return true
			}
			earlier = scope
			return false
		})
		if err != nil {
			return err
		}
		prior := make([]proRuneScope, len(state.Heads))
		var historyErr error
		var mu sync.Mutex
		indices := make([]int, len(prior))
		for i := range indices {
			indices[i] = i
		}
		proParallelLimit(ctx, indices, 3, func(i int) {
			head := state.Heads[i]
			switch {
			case head.ID == international.ID:
				prior[i] = earlier
			case head.Serie == "" || head.Last.Before(first):
				prior[i] = head
			default:
				err := p.walkOPGGSeries(ctx, head.proRuneLeague, now, func(scope proRuneScope, _ string) bool {
					if scope.Last.Before(first) {
						prior[i] = scope
						return false
					}
					return true
				})
				if err != nil {
					mu.Lock()
					historyErr = err
					mu.Unlock()
				}
			}
		})
		if historyErr != nil {
			return historyErr
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		period.After = proLatestInternational(prior).Last
		period.Before = first
		for _, scope := range prior {
			if scope.Domestic && scope.Last.After(period.After) {
				period.Scopes = append(period.Scopes, scope)
			}
		}
		p.mu.Lock()
		p.opggPreviousAt, p.opggPrevious = state.At, period
		p.mu.Unlock()
		return nil
	})
	return period, err
}
