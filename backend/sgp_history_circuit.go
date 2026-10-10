package main

import (
	"context"
	"errors"
	"net/url"
	"time"
)

const historyServerUnavailableMessage = "战绩服务器暂时不可用，稍后自动重试"

var errSGPHistoryCooling = errors.New(historyServerUnavailableMessage)

type sgpHistoryCircuit struct {
	failures      int
	window, until time.Time
	probing       bool
	probeTimer    *time.Timer
	nextProbe     time.Time
}

func sgpHistoryRoute(route string) bool { return route == "SUMMARY" || route == "DETAILS" }

func historyServerError(err error) bool {
	var sgpErr *sgpHTTPError
	var lcuErr *LCUHTTPError
	return errors.Is(err, errSGPHistoryCooling) || errors.As(err, &sgpErr) && sgpErr.StatusCode >= 500 || errors.As(err, &lcuErr) && lcuErr.StatusCode >= 500
}
func (p *sgpProvider) historyCircuitTime() time.Time {
	if p.historyClock != nil {
		return p.historyClock()
	}
	return time.Now()
}
func (p *sgpProvider) acquireHistoryCircuit(server, route string) (bool, error) {
	if !sgpHistoryRoute(route) {
		return false, nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.historyCircuits == nil {
		p.historyCircuits = map[string]*sgpHistoryCircuit{}
	}
	c := p.historyCircuits[server]
	if c == nil {
		c = &sgpHistoryCircuit{}
		p.historyCircuits[server] = c
	}
	if c.until.IsZero() {
		return false, nil
	}
	if (p.historyCircuitTime().Before(c.until) && (route != "SUMMARY" || p.historyCircuitTime().Before(c.nextProbe))) || c.probing {
		return false, errSGPHistoryCooling
	}
	c.probing = true
	return true, nil
}
func (p *sgpProvider) historyCircuitCooling(server, route string, probe bool) bool {
	if !sgpHistoryRoute(route) || probe {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	c := p.historyCircuits[server]
	return c != nil && !c.until.IsZero()
}
func (p *sgpProvider) observeHistoryCircuit(server, route string, status int, probe bool) {
	if !sgpHistoryRoute(route) {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	c := p.historyCircuits[server]
	if c == nil {
		return
	}
	now := p.historyCircuitTime()
	if status >= 500 {
		if now.Sub(c.window) > 30*time.Second {
			c.failures = 0
			c.window = now
		}
		if c.window.IsZero() {
			c.window = now
		}
		c.failures++
		if probe || c.failures >= 3 {
			c.until = now.Add(60 * time.Second)
			c.nextProbe = now.Add(15 * time.Second)
			c.probing = false
		}
	} else if probe || c.until.IsZero() {
		if c.probeTimer != nil {
			c.probeTimer.Stop()
		}
		*c = sgpHistoryCircuit{}
	}
}
func (p *sgpProvider) releaseHistoryProbe(server string, probe bool) {
	if !probe {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if c := p.historyCircuits[server]; c != nil && c.probing {
		c.probing = false
		c.until = p.historyCircuitTime().Add(60 * time.Second)
		c.nextProbe = p.historyCircuitTime().Add(15 * time.Second)
	}
}

// Status codes are evidence; a single failed source is not a dual-source outage.
func historyFailureStatus(err error) int {
	var s *sgpHTTPError
	var l *LCUHTTPError
	if errors.As(err, &s) {
		return s.StatusCode
	}
	if errors.As(err, &l) {
		return l.StatusCode
	}
	return 0
}
func historyOfficialOutage(attempts []DataSourceAttempt, cooling int) bool {
	if cooling > 0 {
		return true
	}
	sgp, lcu := false, false
	for _, a := range attempts {
		if a.StatusCode >= 500 {
			sgp = sgp || a.Source == dataSourceSGP
			lcu = lcu || a.Source == dataSourceLCU
		}
	}
	return sgp && lcu
}
func (p *sgpProvider) historyRetryAfter(server string) int {
	if p == nil {
		return 0
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if c := p.historyCircuits[server]; c != nil && !c.until.IsZero() {
		next := c.nextProbe
		if next.IsZero() {
			next = c.until
		}
		return max(0, int(next.Sub(p.historyCircuitTime()).Seconds()+0.999))
	}
	return 0
}

// Cooldown probes use the same official source with a one-match history page.
func (p *sgpProvider) scheduleHistoryProbe(ctx context.Context, client *LCUClient, kind sgpTokenKind, server, route, path, endpoint string) {
	if route != "SUMMARY" || p.historyClock != nil {
		return
	}
	p.mu.Lock()
	c := p.historyCircuits[server]
	if c == nil || c.until.IsZero() || c.probeTimer != nil {
		p.mu.Unlock()
		return
	}
	c.probeTimer = time.AfterFunc(15*time.Second, func() {
		p.mu.Lock()
		c.probeTimer = nil
		p.mu.Unlock()
		if _, active := client.credentials(); !active {
			return
		}
		probeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 8*time.Second)
		defer cancel()
		target, err := url.Parse(endpoint)
		if err != nil {
			return
		}
		q := target.Query()
		q.Set("startIndex", "0")
		q.Set("count", "1")
		target.RawQuery = q.Encode()
		var data map[string]any
		_ = p.getJSONWithToken(probeCtx, client, kind, server, route, path, target.String(), &data)
	})
	p.mu.Unlock()
}
