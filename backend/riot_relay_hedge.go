package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"strings"
	"sync/atomic"
	"time"
)

type relayAttemptResult struct {
	entry         riotRelayEntry
	status        int
	header        http.Header
	body          []byte
	err           error
	failure       string
	ttfb, elapsed time.Duration
	started       bool
}

func (p *riotProvider) candidateAttempt(ctx context.Context, m *riotRelayEntryManager, e riotRelayEntry, host, path string, query url.Values, responseMax int64, headers *atomic.Bool, ticket *relayHedgeTicket, admitted chan<- struct{}, release func()) relayAttemptResult {
	defer release()
	if ticket != nil && admitted == nil {
		defer m.releaseHedgeReservation(ticket)
	}
	r := relayAttemptResult{entry: e}
	endpoint, err := riotRelayEndpoint(e.origin(), host, path)
	if err != nil {
		r.err = err
		return r
	}
	if q := query.Encode(); q != "" {
		endpoint += "?" + q
	}
	queuedAt := time.Now()
	if err = p.wait(ctx); err != nil {
		r.err = err
		return r
	}
	if err = riotRelays.requestErrorFor(host, path, p.region()); err != nil {
		r.err = err
		return r
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		r.err = err
		return r
	}
	request.Header.Set("X-Riot-Platform", p.region())
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "Deep-Legends/"+version)
	if err = ctx.Err(); err != nil {
		r.err = err
		return r
	}
	scope := riotRequestRateScope(host, path)
	p.beginRiotRateRequest(scope)
	started := time.Now()
	queuedDuration := time.Since(queuedAt)
	attempt, cancel := context.WithTimeout(ctx, riotAttemptBudget(path))
	defer func() {
		riotRequestEvent(p.champions.diag, attempt, "relay", riotRelayRequestCategory(path), e.Label, r.status, r.ttfb, time.Since(started), queuedDuration, len(r.body), 1)
		cancel()
	}()
	r.started = true
	if ticket != nil {
		if admitted != nil {
			m.addGET(ticket)
		} else if !m.commitHedge(ticket) {
			p.observeRiotRate(scope, nil, 0)
			r.started = false
			r.err = errThrottled
			riotRelays.recordHedgeSuppressed(e.Label)
			return r
		}
	}
	if admitted != nil {
		admitted <- struct{}{}
	}
	if host == p.clusterHost() && strings.HasPrefix(path, "/lol/match/v5/matches/"+strings.ToUpper(p.region())+"_") && !strings.HasSuffix(path, "/timeline") {
		tracker := riotOverviewCostTrackerFromContext(ctx)
		tracker.detailInFlight(1)
		defer tracker.detailInFlight(-1)
	}
	var ttfb atomic.Int64
	request = request.WithContext(httptrace.WithClientTrace(attempt, &httptrace.ClientTrace{GotFirstResponseByte: func() {
		elapsed := time.Since(started)
		ttfb.Store(int64(elapsed))
		headers.Store(true)
		riotRelays.recordTTFB(elapsed, e.Label)
	}}))
	response, err := m.client(p.champions.httpClient(), e).Do(request)
	if cost := proRefreshCostFromContext(ctx); cost != nil {
		cost.requests.Add(1)
	}
	if err != nil {
		p.observeRiotRate(scope, nil, 0)
		r.err = err
		if ctx.Err() != nil {
			r.failure = "canceled"
		} else {
			r.failure = "network"
		}
		r.elapsed = time.Since(started)
		riotRelays.recordBusinessRequest(r.failure, 0, riotRelayRequestCategory(path), p.champions.diag, e.Label, started, time.Duration(ttfb.Load()))
		return r
	}
	if response.StatusCode == 429 {
		p.observeRiotRate(scope, nil, 0)
	} else {
		p.observeRiotRate(scope, response.Header, response.StatusCode)
	}
	r.status, r.header = response.StatusCode, response.Header
	r.body, r.err = readLimited(response.Body, responseMax)
	response.Body.Close()
	r.ttfb = time.Duration(ttfb.Load())
	r.elapsed = time.Since(started)
	switch {
	case r.status == http.StatusServiceUnavailable && riotRelayQuotaResponse(r.header, r.body):
		r.failure = "quota_exhausted"
	case r.err != nil:
		if ctx.Err() != nil {
			r.failure = "canceled"
		} else {
			r.failure = "network"
		}
	case r.status == 200 && !json.Valid(r.body):
		r.failure = "invalid_json"
	case r.status == 404:
		r.failure = "not_found"
	case r.status == 401 || r.status == 403:
		r.failure = "auth"
	case r.status >= 500:
		r.failure = "upstream"
	case r.status >= 400 && r.status != 429:
		r.failure = "http"
	}
	riotRelays.recordBusinessRequest(r.failure, r.status, riotRelayRequestCategory(path), p.champions.diag, e.Label, started, r.ttfb)
	return r
}

func (p *riotProvider) candidateGET(ctx context.Context, m *riotRelayEntryManager, e riotRelayEntry, host, path string, query url.Values, responseMax int64) relayAttemptResult {
	ticket := &relayHedgeTicket{}
	select {
	case m.slots <- struct{}{}:
	case <-ctx.Done():
		return relayAttemptResult{err: ctx.Err()}
	}
	results := make(chan relayAttemptResult, 2)
	admitted := make(chan struct{}, 1)
	var primaryHeaders atomic.Bool
	mainCtx, cancelMain := context.WithCancel(ctx)
	defer cancelMain()
	go func() {
		r := relayAttemptResult{entry: e, err: errRiotRelayUnavailable}
		defer func() { results <- r }()
		defer recoverPanic("relay.primary")
		r = p.candidateAttempt(mainCtx, m, e, host, path, query, responseMax, &primaryHeaders, ticket, admitted, func() { <-m.slots })
	}()
	started := 1
	received := 0
	cancelOther := func() {}
	triggered := false
	durations := map[string]int64{}
	defer func() {
		cancelMain()
		cancelOther()
		for received < started {
			r := <-results
			received++
			durations[r.entry.Label] = r.elapsed.Milliseconds()
			m.observe(r.entry.Label, r.failure == "network", r.ttfb, r.failure == "canceled")
			if r.failure == "network" {
				m.networkFailure(r.entry.Label, r.err)
			}
		}
	}()
	timer := time.NewTimer(time.Hour)
	timer.Stop()
	defer timer.Stop()
	fire := func() bool {
		if fallback, _ := ctx.Value(riotFallbackAttemptKey{}).(bool); fallback {
			return false
		}
		if m.config.Mode != "auto" || len(m.config.Entries) < 2 || triggered || primaryHeaders.Load() || ctx.Err() != nil || riotRelays.unavailable() || riotRelays.requestErrorFor(host, path, p.region()) != nil {
			return false
		}
		select {
		case m.slots <- struct{}{}:
		default:
			return false
		}
		shared, _ := ctx.Value(riotDetailSlotsKey{}).(chan struct{})
		if shared != nil {
			select {
			case shared <- struct{}{}:
			default:
				<-m.slots
				return false
			}
		}
		other, ok := m.reserveHedge(e.Label, ticket)
		if !ok {
			<-m.slots
			if shared != nil {
				<-shared
			}
			if m.budgetBlocked(ticket) {
				riotRelays.recordHedgeSuppressed(e.Label)
			}
			return false
		}
		triggered = true
		started++
		secondary, cancel := context.WithCancel(ctx)
		cancelOther = cancel
		go func() {
			r := relayAttemptResult{entry: other, err: errRiotRelayUnavailable}
			defer func() { results <- r }()
			defer recoverPanic("relay.backup")
			var header atomic.Bool
			r = p.candidateAttempt(secondary, m, other, host, path, query, responseMax, &header, ticket, nil, func() {
				<-m.slots
				if shared != nil {
					<-shared
				}
			})
		}()
		return true
	}
	var failed relayAttemptResult
	for {
		select {
		case <-admitted:
			admitted = nil
			timer.Reset(m.threshold(e.Label))
		case <-ctx.Done():
			return relayAttemptResult{err: ctx.Err()}
		case <-timer.C:
			fire()
		case r := <-results:
			received++
			durations[r.entry.Label] = r.elapsed.Milliseconds()
			if r.started {
				m.observe(r.entry.Label, r.failure == "network", r.ttfb, r.failure == "canceled")
			}
			if r.failure == "network" {
				m.networkFailure(r.entry.Label, r.err)
			}
			valid := r.err == nil && (r.status >= 200 && r.status < 300 && r.failure == "" || r.status >= 400 && r.status < 500 || r.failure == "quota_exhausted")
			if valid || received == started && !fire() {
				cancelMain()
				cancelOther()
				for received < started {
					loser := <-results
					received++
					durations[loser.entry.Label] = loser.elapsed.Milliseconds()
					if loser.started {
						m.observe(loser.entry.Label, loser.failure == "network", loser.ttfb, loser.failure == "canceled")
					}
					if loser.failure == "network" {
						m.networkFailure(loser.entry.Label, loser.err)
					}
				}
				m.emit(map[string]any{"event": "relay_hedge", "triggered": triggered, "winner": r.entry.Label, "entry_durations_ms": durations})
				riotRelays.recordHedge(e.Label, r.entry.Label, triggered)
				return r
			}
			failed = r
			if received == started {
				return failed
			}
		}
	}
}

func (p *riotProvider) getCandidateRelay(ctx context.Context, m *riotRelayEntryManager, host, path string, query url.Values, out any, responseMax int64) error {
	networkAttempts := 0
	networkAttemptLimit, attemptLimit := 2, 3
	fallback, _ := ctx.Value(riotFallbackAttemptKey{}).(bool)
	if fallback {
		networkAttemptLimit, attemptLimit = 1, 1
	}
	for attempt := 0; attempt < attemptLimit; attempt++ {
		if err := riotRelays.requestErrorFor(host, path, p.region()); err != nil {
			return err
		}
		e := m.choose(p.champions.diag)
		if !fallback {
			m.maybeProbe(ctx, p)
		}
		m.mu.Lock()
		cold := time.Now().Before(m.runtime[e.Label].until) || m.runtime[e.Label].probing
		m.mu.Unlock()
		if cold {
			return errRiotRelayUnavailable
		}
		r := p.candidateGET(ctx, m, e, host, path, query, responseMax)
		if r.err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if r.failure != "network" {
				return r.err
			}
			networkAttempts++
			if networkAttempts < networkAttemptLimit {
				if err := waitRiotDelay(ctx, 500*time.Millisecond); err != nil {
					return err
				}
				continue
			}
			return fmt.Errorf("%w: %w", errRiotRelayUnavailable, r.err)
		}
		if r.failure == "quota_exhausted" {
			riotRelays.mu.Lock()
			now := time.Now().UTC()
			riotRelays.quotaUntil = time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.UTC)
			riotRelays.mu.Unlock()
			return errRiotRelayQuotaExhausted
		}
		switch r.status {
		case 200:
			if json.Unmarshal(r.body, out) != nil {
				return errRiotRelayUnavailable
			}
			return nil
		case 400:
			return &riotStatusError{status: 400, message: "Riot 接口返回 HTTP 400", puuidMismatch: strings.Contains(strings.ToLower(string(r.body)), "decrypt")}
		case 404:
			return errRiotNotFound
		case 401, 403:
			return errRiotRelayUnavailable
		case 429:
			kind, seconds := riotRelayCooldown(r.header)
			riotRelays.observeCooldown(host, path, kind, seconds, p.region())
			riotOverviewCostTrackerFromContext(ctx).recordRateLimit()
			err := &riotStatusError{status: 429, retryAfter: seconds, relayCooldown: kind, message: "查询额度恢复中，请稍后重试"}
			deadline, bounded := ctx.Deadline()
			if kind == "application" || kind == "ip" || attempt == attemptLimit-1 || seconds > 30 || bounded && time.Until(deadline) <= time.Duration(seconds)*time.Second {
				return err
			}
			if wait := waitRiotDelay(ctx, time.Duration(seconds)*time.Second); wait != nil {
				if errors.Is(wait, context.DeadlineExceeded) {
					return err
				}
				return wait
			}
		default:
			return errRiotRelayUnavailable
		}
	}
	return errRiotRelayUnavailable
}
