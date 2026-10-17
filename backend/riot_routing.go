package main

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type riotRequestPathKey struct{}
type riotRequestSourceKey struct{}
type riotAuthProbeKey struct{}
type riotRouteKey struct{}
type riotPinnedCredentialKey struct{}
type riotPinnedCredential struct{ key, source string }
type riotFallbackAttemptKey struct{}

// Preserve deadline identity for cancellation/handoff while keeping scheduler
// timeouts distinct from an HTTP attempt that actually reached the network.
type riotInternalWaitTimeout struct{ cause error }

func (e *riotInternalWaitTimeout) Error() string { return "Riot internal scheduling wait timed out" }
func (e *riotInternalWaitTimeout) Unwrap() error { return e.cause }
func riotInternalWaitError(err error) error {
	var internal *riotInternalWaitTimeout
	if errors.Is(err, context.DeadlineExceeded) && !errors.As(err, &internal) {
		return &riotInternalWaitTimeout{cause: err}
	}
	return err
}

var errRiotQuotaOverflow = errors.New("Riot local application quota overflow")

// A route decision, not a reservation. It never consumes local tokens and never
// installs a sticky relay cooldown, so the first recovered request goes direct.
func (p *riotProvider) tryRiotQuotaOverflow(ctx context.Context) bool {
	_, source := riotRequestCredential(ctx)
	if ctx.Err() != nil || isRiotBackground(ctx) || source != "user" && source != "embedded" {
		return false
	}
	scope, ok := ctx.Value(riotRateScopeKey{}).(riotRateScope)
	if !ok {
		return false
	}
	p.limitMu.Lock()
	now := p.riotRateNow()
	var until time.Time
	if host := p.rateHosts[scope.host]; host != nil && !now.Before(host.app.cooldown) {
		for period, window := range host.app.windows {
			if period >= time.Minute && window.used >= window.limit && now.Before(window.reset) && window.reset.After(until) {
				until = window.reset
			}
		}
	} else if host == nil && len(p.longWindow) >= 90 {
		// Conservative pre-header budget is also a local application window.
		reset := p.longWindow[0].Add(2 * time.Minute)
		if now.Before(reset) {
			until = reset
		}
	}
	p.limitMu.Unlock()
	if until.IsZero() || !riotRelayRecentlyHealthy() {
		return false
	}
	p.limitMu.Lock()
	if p.quotaOverflowWindows == nil {
		p.quotaOverflowWindows = make(map[string]time.Time)
	}
	for host, reset := range p.quotaOverflowWindows {
		if !now.Before(reset) {
			delete(p.quotaOverflowWindows, host)
		}
	}
	emit := !p.quotaOverflowWindows[scope.host].Equal(until)
	p.quotaOverflowWindows[scope.host] = until
	p.limitMu.Unlock()
	if emit {
		// The relay uses its own server-side Secret. No direct token is forwarded.
		p.routeRecord(map[string]any{"event": "riot_route_switch", "from": "direct", "to": "relay", "reason": "quota-overflow", "source": source, "same_credential": false})
	}
	return true
}

func riotRelayRecentlyHealthy() bool {
	s := riotRelays
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.nowLocked()
	if s.active == "" || s.lastSuccess.IsZero() || now.Sub(s.lastSuccess) >= 10*time.Minute || s.requestErrorLocked() != nil {
		return false
	}
	origins := configuredRiotRelays()
	return s.config == strings.Join(origins, "\n") && len(origins) > 0
}

type riotForegroundState struct {
	sync.Mutex
	active           int
	resumeAt         time.Time
	now              func() time.Time
	visibilityKnown  bool
	directoryVisible bool
}

func (s *riotForegroundState) nowLocked() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}

func beginRiotForeground(p *riotProvider) func() {
	if p.foreground == nil {
		return func() {}
	}
	riotForeground := p.foreground
	riotForeground.Lock()
	riotForeground.active++
	riotForeground.Unlock()
	return func() {
		riotForeground.Lock()
		riotForeground.active--
		if riotForeground.active == 0 {
			riotForeground.resumeAt = riotForeground.nowLocked().Add(5 * time.Second)
		}
		riotForeground.Unlock()
	}
}
func (p *riotProvider) waitForRiotForeground(ctx context.Context) error {
	defer startRiotTimingProbe(ctx, "foreground_wait")()
	if p.foreground == nil {
		return nil
	}
	riotForeground := p.foreground
	proOwned := ctx.Value(proBackgroundGateKey{}) != nil
	bounded := isRiotBackground(ctx) && !proOwned
	riotForeground.Lock()
	yieldUntil := riotForeground.nowLocked().Add(3 * time.Second)
	riotForeground.Unlock()
	for {
		if err := ctx.Err(); err != nil {
			return riotInternalWaitError(err)
		}
		riotForeground.Lock()
		active, until := riotForeground.active, riotForeground.resumeAt
		now := riotForeground.nowLocked()
		hidden := proOwned && riotForeground.visibilityKnown && !riotForeground.directoryVisible
		riotForeground.Unlock()
		// Overview verification is low priority, not directory-owned. After a
		// bounded yield it continues through the stricter background limiter.
		if bounded && !hidden && !now.Before(yieldUntil) {
			return nil
		}
		if !hidden && active == 0 && !now.Before(until) {
			return nil
		}
		delay := 100 * time.Millisecond
		if !hidden && active == 0 {
			delay = until.Sub(now)
		}
		if bounded {
			delay = min(delay, yieldUntil.Sub(now))
		}
		wait := waitRiotDelay
		if controlled, ok := ctx.Value(riotForegroundWaitKey{}).(func(context.Context, time.Duration) error); ok {
			wait = controlled
		}
		if err := wait(ctx, delay); err != nil {
			return riotInternalWaitError(err)
		}
	}
}
func riotRequestCredential(ctx context.Context) (string, string) {
	if route, _ := ctx.Value(riotRouteKey{}).(string); route == "relay" {
		return "", "relay"
	}
	if credential, ok := ctx.Value(riotPinnedCredentialKey{}).(riotPinnedCredential); ok {
		return credential.key, credential.source
	}
	return riotUserKeys.effective()
}
func riotCancelReason(ctx context.Context) string {
	if cause := context.Cause(ctx); cause != nil && cause.Error() == "superseded" {
		return "superseded"
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return "timeout"
	}
	if ctx.Err() != nil {
		return "tab_closed"
	}
	return ""
}
func riotRequestEvent(record func(map[string]any), ctx context.Context, route, category, entry string, status int, ttfb, total, queued time.Duration, bytes int, attempt int) {
	if record == nil {
		return
	}
	// Platform health and spectator lookups share the summoner control bucket.
	if category == "other" || category == "spectator" {
		category = "summoner"
	}
	proxy, stage := "none", ""
	if trace, ok := ctx.Value(networkTraceKey{}).(*networkTrace); ok {
		proxy, stage = trace.proxy, trace.stage
	}
	shape, _ := ctx.Value(riotRequestPathKey{}).(string)
	source, _ := ctx.Value(riotRequestSourceKey{}).(string)
	if source == "" {
		source = "overview"
	}
	record(map[string]any{"path_shape": shape, "source": source, "proxy": proxy, "error_stage": stage, "event": "riot_request", "route": route, "category": category, "entry": entry, "status": status, "ttfb_ms": ttfb.Milliseconds(), "total_ms": total.Milliseconds(), "bytes": bytes, "queued_ms": queued.Milliseconds(), "attempt": attempt, "cancelled": ctx.Err() != nil, "cancel_reason": riotCancelReason(ctx), "foreground": !isRiotBackground(ctx)})
}
func (p *riotProvider) routeRecord(event map[string]any) {
	if p.champions != nil && p.champions.diag != nil {
		p.champions.diag(event)
	}
}
func (p *riotProvider) relayFallback(ctx context.Context, host, path string, query url.Values, out any, max int64) error {
	p.platformMu.Lock()
	if p.relayProvider == nil {
		p.relayProvider = newRiotProviderWithCaches(p.champions, p.matchDisk, p.identityDisk)
		p.relayProvider.platform = p.platform
		p.relayProvider.foreground = p.foreground
		p.relayProvider.puuidState = p.puuidState
	}
	relay := p.relayProvider
	p.platformMu.Unlock()
	ctx = context.WithValue(ctx, riotRouteKey{}, "relay")
	ctx = context.WithValue(ctx, riotFallbackAttemptKey{}, true)
	return relay.getScopedPUUID(ctx, host, path, query, out, max)
}

func (p *riotProvider) getLimited(ctx context.Context, host, path string, query url.Values, out any, max int64) error {
	if route, _ := ctx.Value(riotRouteKey{}).(string); route != "" {
		if route == "relay" {
			// Explicit identity retries keep their cache owner but must use the
			// relay's admission state, not the exhausted direct credential bucket.
			return p.relayFallback(ctx, host, path, query, out, max)
		}
		return p.getScopedPUUID(ctx, host, path, query, out, max)
	}
	_, source := riotUserKeys.effective()
	if source != "embedded" {
		return p.getScopedPUUID(ctx, host, path, query, out, max)
	}
	if isRiotBackground(ctx) {
		// Background jobs retain local low-priority admission; only interactive
		// work may overflow an exhausted application window to the relay.
		return p.getScopedPUUID(ctx, host, path, query, out, max)
	}
	s := riotUserKeys
	s.mu.Lock()
	// The old embedded 80% hint is telemetry, not a sticky route. The actual
	// local application limiter decides overflow for both credential sources,
	// and permits direct I/O immediately when its controlled window recovers.
	quotaHint := !s.quotaWindowAt.IsZero() && s.routeUntil.Equal(s.quotaWindowAt.Add(120*time.Second))
	cooldown := time.Now().Before(s.routeUntil) && !quotaHint
	probe := s.networkFailures >= 3 && !cooldown
	s.mu.Unlock()
	if cooldown {
		return p.relayFallback(ctx, host, path, query, out, max)
	}
	if probe {
		s.mu.Lock()
		if s.probing {
			s.mu.Unlock()
			return p.relayFallback(ctx, host, path, query, out, max)
		}
		s.probing = true
		s.mu.Unlock()
		probeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		var status any
		err := p.getLimitedRoute(probeCtx, p.platformHost(), "/lol/status/v4/platform-data", nil, &status, max)
		cancel()
		s.mu.Lock()
		s.probing = false
		if err == nil {
			s.networkFailures = 0
			s.routeUntil = time.Time{}
		} else {
			s.routeUntil = time.Now().Add(5 * time.Minute)
		}
		s.mu.Unlock()
		if err != nil {
			return p.relayFallback(ctx, host, path, query, out, max)
		}
		p.routeRecord(map[string]any{"event": "riot_route_switch", "from": "relay", "to": "direct", "reason": "probe_recovered"})
	}
	err := p.getScopedPUUID(ctx, host, path, query, out, max)
	// Local quota is not an upstream rejection. Admission already checked
	// relay health; do not let the legacy embedded 429 fallback bypass it.
	if errors.Is(err, errThrottled) {
		return err
	}
	var status *riotStatusError
	if errors.As(err, &status) {
		if status.puuidMismatch {
			return p.relayFallback(ctx, host, path, query, out, max)
		}
		if status.status == 401 || status.status == 403 {
			s.mu.Lock()
			rejected := s.embeddedRejected
			s.mu.Unlock()
			if !rejected {
				return err
			}
			return p.relayFallback(ctx, host, path, query, out, max)
		}
		if status.status == 429 {
			s.mu.Lock()
			s.routeUntil = time.Now().Add(time.Duration(status.retryAfter) * time.Second)
			s.mu.Unlock()
			p.routeRecord(map[string]any{"event": "riot_quota_guard", "priority": riotPriority(ctx), "action": "relay", "window_s": status.retryAfter})
			return p.relayFallback(ctx, host, path, query, out, max)
		}
		return err
	}
	if err == nil {
		s.mu.Lock()
		s.networkFailures = 0
		s.mu.Unlock()
		return nil
	}
	if ctx.Err() != nil {
		return err
	}
	var internal *riotInternalWaitTimeout
	if errors.As(err, &internal) {
		return err
	}
	// Only network/read failures switch routes; successful error responses retain their meaning.
	if !strings.Contains(err.Error(), "无法连接 Riot") && !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		return err
	}
	s.mu.Lock()
	s.networkFailures++
	if s.networkFailures >= 3 {
		s.routeUntil = time.Now().Add(5 * time.Minute)
	}
	s.mu.Unlock()
	p.routeRecord(map[string]any{"event": "riot_route_switch", "from": "direct", "to": "relay", "reason": "network"})
	return p.relayFallback(ctx, host, path, query, out, max)
}
func (p *riotProvider) observeEmbeddedQuota(header http.Header, status int, contexts ...context.Context) {
	ctx := context.Background()
	if len(contexts) > 0 {
		ctx = contexts[0]
	}
	_, source := riotUserKeys.effective()
	if source != "embedded" {
		return
	}
	limit := 100
	for _, pair := range strings.Split(header.Get("X-App-Rate-Limit"), ",") {
		parts := strings.Split(strings.TrimSpace(pair), ":")
		if len(parts) == 2 && parts[1] == "120" {
			if n, _ := strconv.Atoi(parts[0]); n > 0 {
				limit = n
			}
		}
	}
	for _, pair := range strings.Split(header.Get("X-App-Rate-Limit-Count"), ",") {
		parts := strings.Split(strings.TrimSpace(pair), ":")
		if len(parts) != 2 || parts[1] != "120" {
			continue
		}
		count, _ := strconv.Atoi(parts[0])
		riotUserKeys.mu.Lock()
		if riotUserKeys.quotaWindowAt.IsZero() || time.Since(riotUserKeys.quotaWindowAt) >= 120*time.Second {
			riotUserKeys.quotaWindowAt = time.Now()
		}
		until := riotUserKeys.quotaWindowAt.Add(120 * time.Second)
		wait := time.Until(until)
		relay, direct := riotRouteMedianCosts()
		nearQuota := count*5 >= limit*4
		if nearQuota {
			riotUserKeys.routeUntil = until
		}
		riotUserKeys.mu.Unlock()
		if count*5 >= limit*4 {
			p.routeRecord(map[string]any{"event": "riot_quota_guard", "priority": riotPriority(ctx), "count": count, "limit": limit, "window_s": 120, "action": "local-admission", "wait_ms": wait.Milliseconds(), "relay_median_ms": relay.Milliseconds(), "direct_median_ms": direct.Milliseconds()})
		}
	}

}

func riotAttemptBudget(path string) time.Duration {
	if strings.HasSuffix(path, "/ids") {
		return 8 * time.Second
	}
	return 15 * time.Second
}

func riotRouteMedianCosts() (time.Duration, time.Duration) {
	riotRelays.mu.Lock()
	defer riotRelays.mu.Unlock()
	median := func(label string, fallback time.Duration) time.Duration {
		entry := riotRelays.entrySummaries[label]
		if entry == nil || len(entry.TTFBMillis) < 3 {
			return fallback
		}
		values := append([]float64(nil), entry.TTFBMillis[max(0, len(entry.TTFBMillis)-20):]...)
		sort.Float64s(values)
		return time.Duration(values[len(values)/2] * float64(time.Millisecond))
	}
	return median("A", 8*time.Second), median("direct", 500*time.Millisecond)
}

func riotPriority(ctx context.Context) string {
	if isRiotBackground(ctx) {
		return "background"
	}
	return "foreground"
}
