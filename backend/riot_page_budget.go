package main

import (
	"context"
	"math"
	"time"
)

// Reserve a whole detail page without consuming or bypassing individual admissions.
func (p *riotProvider) reserveOverviewPage(ctx context.Context, count int) (func(), error) {
	p.limitMu.Lock()
	now := p.riotRateNow()
	need := count + 1
	delay := time.Duration(0)
	p.longWindow = pruneTimestamps(p.longWindow, now.Add(-2*time.Minute))
	if len(p.longWindow)+p.pageReserved+need > 90 {
		index := min(len(p.longWindow)-1, max(0, len(p.longWindow)+p.pageReserved+need-91))
		if index >= 0 {
			delay = p.longWindow[index].Add(2 * time.Minute).Sub(now)
		}
	}
	for host, rate := range p.rateHosts {
		if host != p.platformHost() && host != p.clusterHost() {
			continue
		}
		for period, window := range rate.app.windows {
			if period >= time.Minute && now.Before(window.reset) && window.limit-window.used-p.pageReserved < need {
				delay = max(delay, window.reset.Sub(now))
			}
		}
		if method := rate.methods["/lol/match/v5/matches/[id]"]; method != nil {
			for period, window := range method.windows {
				if period >= time.Minute && now.Before(window.reset) && window.limit-window.used-p.pageReserved < count {
					delay = max(delay, window.reset.Sub(now))
				}
			}
		}
	}
	if delay > 0 {
		p.limitMu.Unlock()
		return nil, &riotStatusError{status: 429, retryAfter: max(1, int(math.Ceil(delay.Seconds()))), message: "外服查询额度恢复中，恢复后自动继续"}
	}
	p.pageReserved += need
	p.limitMu.Unlock()
	return func() { p.limitMu.Lock(); p.pageReserved -= need; p.limitMu.Unlock() }, nil
}
