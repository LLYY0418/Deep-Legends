package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"
)

// Only one due probe is admitted, using spare existing slots. A busy detail
// queue does not create another queue or goroutines waiting for a probe slot.
func (m *riotRelayEntryManager) maybeProbe(ctx context.Context, p *riotProvider) {
	if m.config.Mode != "auto" || riotRelays.requestErrorFor(p.clusterHost(), "/health", p.region()) != nil {
		return
	}
	m.mu.Lock()
	var e riotRelayEntry
	for _, candidate := range m.config.Entries {
		s := m.runtime[candidate.Label]
		if !s.until.IsZero() && !time.Now().Before(s.until) && !s.probing {
			e = candidate
			break
		}
	}
	if e.Label == "" {
		m.mu.Unlock()
		return
	}
	select {
	case m.slots <- struct{}{}:
	default:
		m.mu.Unlock()
		return
	}
	shared, _ := ctx.Value(riotDetailSlotsKey{}).(chan struct{})
	if shared != nil {
		select {
		case shared <- struct{}{}:
		default:
			<-m.slots
			m.mu.Unlock()
			return
		}
	}
	m.runtime[e.Label].probing = true
	m.mu.Unlock()
	go func() {
		defer recoverPanic("relay.entryProbe")
		defer func() {
			<-m.slots
			if shared != nil {
				<-shared
			}
		}()
		probe, cancel := context.WithTimeout(ctx, 8*time.Second)
		defer cancel()
		started := time.Now()
		failure, status := "", 0
		var err error
		for _, route := range []string{"/health", "/r/kr/lol/status/v4/platform-data"} {
			err = p.wait(probe)
			if err != nil {
				break
			}
			if err = riotRelays.requestErrorFor(p.clusterHost(), route, p.region()); err != nil {
				break
			}
			req, _ := http.NewRequestWithContext(probe, http.MethodGet, e.origin()+route, nil)
			response, requestErr := m.client(p.champions.httpClient(), e).Do(req)
			err = requestErr
			if err != nil {
				failure = "network"
			} else {
				status = response.StatusCode
				response.Body.Close()
				failure = ""
				if status < 200 || status >= 300 {
					failure = "http"
				}
			}
			riotRelays.recordRequest(failure, status, "other", p.champions.diag, e.Label)
			if err == nil && route == "/health" && status == 404 {
				continue
			}
			break
		}
		m.mu.Lock()
		s := m.runtime[e.Label]
		s.probing = false
		if err == nil && failure == "" {
			s.until = time.Time{}
			s.backoffs, s.consecutive = 0, 0
		} else {
			s.backoffs++
			s.until = time.Now().Add(time.Duration(30<<min(2, s.backoffs-1)) * time.Second)
		}
		m.mu.Unlock()
		result := "ok"
		if err != nil || failure != "" {
			result = "failed"
		}
		var timeout net.Error
		stage := ""
		if errors.As(err, &timeout) && timeout.Timeout() {
			stage = "timeout"
		}
		m.emit(map[string]any{"event": "riot_relay_probe", "relay_entry": e.Label, "result": result, "failure_stage": stage, "duration_ms": time.Since(started).Milliseconds(), "http_status": status})
	}()
}
