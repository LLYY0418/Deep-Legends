package main

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"
)

// Background work never joins the foreground FIFO: a blocked decorator must
// not own its head while an interactive search is ready to use reserved quota.
type riotBackgroundKey struct{}

func withRiotBackground(ctx context.Context) context.Context {
	return context.WithValue(ctx, riotBackgroundKey{}, true)
}
func isRiotBackground(ctx context.Context) bool {
	value, _ := ctx.Value(riotBackgroundKey{}).(bool)
	return value
}
func (p *riotProvider) admitRiotBackground(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	p.limitMu.Lock()
	defer p.limitMu.Unlock()
	now := time.Now()
	if p.limitNow != nil {
		now = p.limitNow()
	}
	if delay, scoped := p.scopedRiotDelay(ctx, now, len(p.limitQueue) == 0, true); scoped {
		if len(p.limitQueue) > 0 || delay > 0 {
			if cost := proRefreshCostFromContext(ctx); cost != nil {
				cost.skipped.Add(1)
			}
			return fmt.Errorf("%w: background yields to foreground quota", errThrottled)
		}
		return nil
	}
	p.shortWindow = pruneTimestamps(p.shortWindow, now.Add(-time.Second))
	p.longWindow = pruneTimestamps(p.longWindow, now.Add(-2*time.Minute))
	// 30 is shared instantaneous occupancy, NOT a separate background token bucket.
	if len(p.limitQueue) > 0 || len(p.shortWindow) >= 15 || len(p.longWindow) >= 30 {
		if cost := proRefreshCostFromContext(ctx); cost != nil {
			cost.skipped.Add(1)
		}
		return fmt.Errorf("%w: background yields to foreground quota", errThrottled)
	}
	p.shortWindow = append(p.shortWindow, now)
	p.longWindow = append(p.longWindow, now)
	return nil
}

type proRefreshCostKey struct{}
type proRefreshCost struct {
	total, refreshed  int
	requests, skipped atomic.Int64
}

func newProRefreshCost(ctx context.Context) (context.Context, *proRefreshCost) {
	cost := new(proRefreshCost)
	return context.WithValue(ctx, proRefreshCostKey{}, cost), cost
}
func proRefreshCostFromContext(ctx context.Context) *proRefreshCost {
	cost, _ := ctx.Value(proRefreshCostKey{}).(*proRefreshCost)
	return cost
}
func (a *app) recordProRefreshCost(event string, cost *proRefreshCost) {
	a.recordDiagnostic(map[string]any{"event": event, "accounts_total": cost.total, "accounts_refreshed": cost.refreshed, "riot_requests": cost.requests.Load(), "skipped_by_budget": cost.skipped.Load()})
}

// Use exact normalized Riot IDs only. No nickname/token/fuzzy identity matching.
func proDirectoryAccounts(groups ...[]opggProTeam) map[string]opggProAccount {
	result := map[string]opggProAccount{}
	for _, teams := range groups {
		for _, team := range teams {
			for _, member := range team.Members {
				for _, row := range member.Summoners {
					at, err := time.Parse(time.RFC3339Nano, proDirectoryRevisionAt(row))
					if row.Source == "seed" || row.PUUID == "" || err != nil || at.IsZero() {
						continue
					}
					if _, valid := normalizeProAccount(row); !valid {
						continue
					}
					result[proLadderAccountKey(row.GameName, row.TagLine)] = row
				}
			}
		}
	}
	return result
}
func (a *app) rememberProSeedAnchor(ctx context.Context, key, puuid string) {
	if a.riot == nil || puuid == "" {
		return
	}
	anchor := struct {
		PUUID string `json:"puuid"`
	}{PUUID: puuid}
	// The loader only returns the directory's identity; renewing an anchor costs no HTTP.
	_ = a.riot.cachedPublicIdentity(ctx, key, 30*24*time.Hour, &anchor, func(context.Context) error { return nil })
}
func (a *app) rememberProDirectoryAnchors(ctx context.Context, seeds []opggProTeam) {
	for _, team := range seeds {
		for _, member := range team.Members {
			for _, row := range member.Summoners {
				if proDirectoryRevisionAt(row) != "" {
					a.rememberProSeedAnchor(ctx, row.SeedKey, row.PUUID)
				}
			}
		}
	}
}

type proSeedSelectionKey struct{}

func proSeedSelected(ctx context.Context, key string) bool {
	selected, ok := ctx.Value(proSeedSelectionKey{}).(string)
	return !ok || selected == key
}

const proSeedRefreshInterval = time.Minute

// One worker per app, independent of directory refresh flights. It always waits
// a full minute before its first account, even if the directory loads instantly.
func (a *app) startProSeedRefresh() {
	if a.riot == nil || !riotKeyConfigured() || a.proRefreshContext == nil {
		return
	}
	c := &a.proPlayers
	c.mu.Lock()
	if c.refreshStarted {
		c.mu.Unlock()
		return
	}
	c.refreshStarted = true
	c.mu.Unlock()
	go a.runProSeedRefresh(a.proRefreshContext)
}
func (a *app) runProSeedRefresh(ctx context.Context) {
	wait, now := a.proPlayers.refreshWait, a.proPlayers.refreshNow
	if wait == nil {
		wait = waitRiotDelay
	}
	if now == nil {
		now = time.Now
	}
	for {
		if wait(ctx, proSeedRefreshInterval) != nil {
			return
		}
		if ctx.Err() != nil {
			return
		}
		a.refreshNextProSeed(ctx, now())
	}
}
func (a *app) refreshNextProSeed(ctx context.Context, now time.Time) {
	c := &a.proPlayers
	c.mu.Lock()
	// A directory flight publishes immutable snapshots. Do not race its final publish.
	if c.updating || c.flight != nil {
		c.mu.Unlock()
		return
	}
	previous := cloneProTeams(c.teams)
	known := proDirectoryAccounts(previous)
	if c.refreshAttempted == nil {
		c.refreshAttempted = map[string]time.Time{}
	}
	var selected string
	var seed proSeedAccount
	var oldest time.Time
	for _, candidate := range proSeedAccounts {
		for index, ref := range candidate.Accounts {
			// Public profile batches own accounts they have checked successfully.
			// Keep the low-quota Riot worker only as a fallback for failed profiles.
			profileKnown := false
			for _, team := range previous {
				for _, member := range team.Members {
					for _, row := range member.Summoners {
						if proLadderAccountKey(row.GameName, row.TagLine) == proLadderAccountKey(ref.GameName, ref.TagLine) && !row.CheckFailed {
							at, err := time.Parse(time.RFC3339Nano, row.CheckedAt)
							profileKnown = err == nil && now.Sub(at) >= 0 && now.Sub(at) < proProfileTTL(row)
						}
					}
				}
			}
			if profileKnown {
				continue
			}
			if _, found := known[proLadderAccountKey(ref.GameName, ref.TagLine)]; found {
				continue
			}
			key := proSeedAnchor(candidate, index)
			at := c.refreshAttempted[key]
			if at.IsZero() {
				for _, team := range previous {
					for _, member := range team.Members {
						for _, row := range member.Summoners {
							if row.SeedKey == key {
								at, _ = time.Parse(time.RFC3339Nano, row.UpdatedAt)
							}
						}
					}
				}
			}
			if selected == "" || at.Before(oldest) {
				selected, seed, oldest = key, candidate, at
			}
		}
	}
	if selected == "" {
		c.mu.Unlock()
		return
	}
	c.refreshAttempted[selected] = now
	c.mu.Unlock()
	step, cancel := context.WithTimeout(withRiotBackground(ctx), proSeedPerAccountBudget)
	defer cancel()
	rows := a.loadProSeedAccounts(context.WithValue(step, proSeedSelectionKey{}, selected), previous, []proSeedAccount{seed}, previous)
	var refreshed opggProAccount
	for _, team := range rows {
		for _, member := range team.Members {
			for _, row := range member.Summoners {
				if row.SeedKey == selected {
					refreshed = row
				}
			}
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.updating || c.flight != nil {
		return
	}
	snapshot := cloneProTeams(c.teams)
	for ti := range snapshot {
		for mi := range snapshot[ti].Members {
			for ai := range snapshot[ti].Members[mi].Summoners {
				row := &snapshot[ti].Members[mi].Summoners[ai]
				if row.SeedKey == selected && refreshed.GameName != "" {
					*row = refreshed
				}
			}
		}
	}
	c.teams = snapshot
	a.persistProSnapshotLocked()
}
