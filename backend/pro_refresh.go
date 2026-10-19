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
	// Reserve the final thirty percent of the shared window for interactive work.
	if len(p.limitQueue) > 0 || len(p.shortWindow) >= 15 || len(p.longWindow)*10 > 90*7 {
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
		PUUID           string `json:"puuid"`
		CredentialScope string `json:"credentialScope"`
	}{PUUID: puuid, CredentialScope: "opgg"}
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
