package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Riot quotas are regional. Method limits additionally belong to an endpoint
// family, never to an individual PUUID or match ID.
type riotRateScopeKey struct{}
type riotRateScope struct{ host, method string }
type riotRateWindow struct {
	limit, used int
	reset       time.Time
}
type riotRateBucket struct {
	windows  map[time.Duration]*riotRateWindow
	cooldown time.Time
}
type riotHostRate struct {
	app            riotRateBucket
	methods        map[string]*riotRateBucket
	appFromHeaders bool
}

func riotRequestRateScope(host, requestPath string) riotRateScope {
	method := riotDiagnosticPath(requestPath)
	if strings.HasPrefix(requestPath, "/lol/match/v5/matches/") && !strings.HasPrefix(requestPath, "/lol/match/v5/matches/by-puuid/") {
		method = "/lol/match/v5/matches/[id]"
		if strings.HasSuffix(requestPath, "/timeline") {
			method += "/timeline"
		}
	}
	return riotRateScope{host, method}
}

func parseRiotRatePairs(raw string) map[time.Duration]int {
	pairs := make(map[time.Duration]int)
	for _, part := range strings.Split(raw, ",") {
		values := strings.Split(strings.TrimSpace(part), ":")
		if len(values) != 2 {
			continue
		}
		value, e1 := strconv.Atoi(values[0])
		seconds, e2 := strconv.Atoi(values[1])
		if e1 == nil && e2 == nil && value >= 0 && seconds > 0 && seconds <= 86400 {
			pairs[time.Duration(seconds)*time.Second] = value
		}
	}
	return pairs
}

func (p *riotProvider) riotRateNow() time.Time {
	if p.limitNow != nil {
		return p.limitNow()
	}
	return time.Now()
}

// Call under limitMu. Unknown headers retain the conservative legacy limiter.
func (p *riotProvider) scopedRiotDelay(ctx context.Context, now time.Time, consume, background bool) (time.Duration, bool) {
	scope, ok := ctx.Value(riotRateScopeKey{}).(riotRateScope)
	host := p.rateHosts[scope.host]
	if !ok || host == nil {
		return 0, false
	}
	buckets := []*riotRateBucket{&host.app}
	if method := host.methods[scope.method]; method != nil {
		buckets = append(buckets, method)
	}
	var delay time.Duration
	for _, bucket := range buckets {
		delay = max(delay, bucket.cooldown.Sub(now))
		for period, window := range bucket.windows {
			if !now.Before(window.reset) {
				window.used = 0
				window.reset = now.Add(period)
			}
			limit := window.limit
			if background && period >= time.Minute {
				limit = max(1, limit/3)
			}
			if window.used >= limit {
				delay = max(delay, window.reset.Sub(now))
			}
		}
	}
	if delay <= 0 && consume {
		for _, bucket := range buckets {
			for _, window := range bucket.windows {
				window.used++
			}
		}
	}
	return max(0, delay), true
}

func (p *riotProvider) beginRiotRateRequest(scope riotRateScope) {
	p.limitMu.Lock()
	defer p.limitMu.Unlock()
	if p.rateFlights == nil {
		p.rateFlights = make(map[riotRateScope]int)
	}
	p.rateFlights[scope]++
}

// Incorporate server-observed usage without lowering reservations made by
// concurrent requests. A server window's phase isn't provided; waiting a whole
// period from first observation is conservative and avoids guessing its reset.
func (p *riotProvider) observeRiotRate(scope riotRateScope, header http.Header, status int) {
	p.limitMu.Lock()
	var diagnostic map[string]any
	defer func() {
		p.limitMu.Unlock()
		if diagnostic != nil && p.champions != nil && p.champions.diag != nil {
			p.champions.diag(diagnostic)
		}
	}()
	if p.rateFlights != nil {
		p.rateFlights[scope] = max(0, p.rateFlights[scope]-1)
	}
	now := p.riotRateNow()
	limits := parseRiotRatePairs(header.Get("X-App-Rate-Limit"))
	for period, limit := range limits {
		if limit <= 0 {
			delete(limits, period)
		}
	}
	appFromHeaders := len(limits) > 0
	host := p.rateHosts[scope.host]
	before := riotRatePolicy(host, scope.method)
	if host == nil && (len(limits) > 0 || status == http.StatusTooManyRequests) {
		if p.rateHosts == nil {
			p.rateHosts = make(map[string]*riotHostRate)
		}
		host = &riotHostRate{methods: make(map[string]*riotRateBucket)}
		p.rateHosts[scope.host] = host
		// Header-less 429s still need a bounded regional admission budget.
		if len(limits) == 0 {
			limits = map[time.Duration]int{time.Second: 15, 2 * time.Minute: 90}
		}
	}
	if host == nil {
		return
	}
	if appFromHeaders {
		host.appFromHeaders = true
	}
	inFlight := 0
	for pending, count := range p.rateFlights {
		if pending.host == scope.host {
			inFlight += count
		}
	}
	update := func(bucket *riotRateBucket, limits, counts map[time.Duration]int, pending int) {
		if bucket.windows == nil {
			bucket.windows = make(map[time.Duration]*riotRateWindow)
		}
		for period, limit := range limits {
			if limit <= 0 {
				continue
			}
			window := bucket.windows[period]
			if window == nil || !now.Before(window.reset) {
				window = &riotRateWindow{reset: now.Add(period)}
				bucket.windows[period] = window
			}
			window.limit = limit
			window.used = max(window.used, counts[period]+pending)
		}
	}
	update(&host.app, limits, parseRiotRatePairs(header.Get("X-App-Rate-Limit-Count")), inFlight)
	methodLimits := parseRiotRatePairs(header.Get("X-Method-Rate-Limit"))
	if len(methodLimits) > 0 && host.methods[scope.method] == nil {
		host.methods[scope.method] = &riotRateBucket{}
	}
	if method := host.methods[scope.method]; method != nil {
		update(method, methodLimits, parseRiotRatePairs(header.Get("X-Method-Rate-Limit-Count")), p.rateFlights[scope])
	}
	if after := riotRatePolicy(host, scope.method); after != before && (appFromHeaders || len(methodLimits) > 0) {
		diagnostic = map[string]any{"event": "riot_rate_policy", "host": scope.host, "path": scope.method, "policy": after}
	}
	if status == http.StatusTooManyRequests {
		seconds, err := strconv.Atoi(header.Get("Retry-After"))
		if err != nil || seconds <= 0 || seconds > 86400 {
			seconds = 3
		}
		bucket := &host.app
		if header.Get("X-Rate-Limit-Type") == "method" {
			if host.methods[scope.method] == nil {
				host.methods[scope.method] = &riotRateBucket{}
			}
			bucket = host.methods[scope.method]
		}
		until := now.Add(time.Duration(seconds) * time.Second)
		if until.After(bucket.cooldown) {
			bucket.cooldown = until
		}
	}
}

// Limits only, no account identifiers, credentials, or request bodies.
func riotRatePolicy(host *riotHostRate, method string) string {
	if host == nil {
		return ""
	}
	policy := map[string]map[string]int{}
	for name, bucket := range map[string]*riotRateBucket{"app": &host.app, "method": host.methods[method]} {
		if bucket == nil {
			continue
		}
		limits := map[string]int{}
		for period, window := range bucket.windows {
			limits[strconv.Itoa(int(period.Seconds()))] = window.limit
		}
		policy[name] = limits
	}
	raw, _ := json.Marshal(policy)
	return string(raw)
}

func (p *riotProvider) riotQuotaDiagnostic(ctx context.Context, seconds int) map[string]any {
	p.limitMu.Lock()
	defer p.limitMu.Unlock()
	scope, _ := ctx.Value(riotRateScopeKey{}).(riotRateScope)
	event := map[string]any{"event": "riot_local_rate_limited", "retry_after_seconds": seconds, "host": scope.host, "path": scope.method, "upstream_429": false}
	host := p.rateHosts[scope.host]
	if host == nil {
		event["source"] = "fallback"
		return event
	}
	event["source"] = "fallback"
	if host.appFromHeaders {
		event["source"] = "response-headers"
	}
	windows := []map[string]any{}
	for name, bucket := range map[string]*riotRateBucket{"app": &host.app, "method": host.methods[scope.method]} {
		if bucket == nil {
			continue
		}
		for period, window := range bucket.windows {
			windows = append(windows, map[string]any{"scope": name, "period_seconds": int(period.Seconds()), "limit": window.limit, "reserved": window.used})
		}
	}
	event["windows"] = windows
	return event
}
