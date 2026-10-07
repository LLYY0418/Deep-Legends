package main

import (
	"net/http"
	"sync"
	"time"
)

type clientLaunchTimingState struct {
	mu                                             sync.Mutex
	id                                             string
	started, process, connected, overview, startup time.Time
}

func (a *app) startClientLaunchTiming(id string, now time.Time) {
	s := &a.clientLaunchTiming
	s.mu.Lock()
	s.id = id
	s.started = now
	s.process = time.Time{}
	s.connected = time.Time{}
	s.overview = time.Time{}
	s.startup = now
	s.mu.Unlock()
}
func (a *app) clientDiscoveryInterval(normal time.Duration, report LCUDiscoveryStatus, now time.Time) time.Duration {
	if report.ProbeErrorKind == "refused" {
		return 500 * time.Millisecond
	}
	return time.Second
}
func (a *app) observeClientLaunchDiscovery(report LCUDiscoveryStatus, now time.Time) {
	s := &a.clientLaunchTiming
	s.mu.Lock()
	if report.Result == "connected" {
		s.startup = time.Time{}
	}
	if s.started.IsZero() {
		s.mu.Unlock()
		return
	}
	changed := false
	if report.ProcessCount > 0 && s.process.IsZero() {
		s.process = now
		changed = true
	}
	if report.Result == "connected" && s.connected.IsZero() {
		s.connected = now
		changed = true
	}
	event := s.eventLocked()
	s.mu.Unlock()
	if changed {
		a.recordDiagnostic(event)
	}
}
func (s *clientLaunchTimingState) eventLocked() map[string]any {
	elapsed := func(t time.Time) int64 {
		if t.IsZero() {
			return -1
		}
		return max(0, t.Sub(s.started).Milliseconds())
	}
	_, id := safeClientLaunchEvent(s.id)
	return map[string]any{"event": "client_launch_to_connected", "client_id": id, "ms_to_process": elapsed(s.process), "ms_to_connected": elapsed(s.connected), "ms_to_overview": elapsed(s.overview)}
}
func (a *app) handleClientLaunchOverviewReady(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	a.mu.RLock()
	connected := a.connected
	a.mu.RUnlock()
	if connected {
		a.observeColdLaunchMilestone("overview_first_card_ms", time.Now())
	}
	s := &a.clientLaunchTiming
	s.mu.Lock()
	var event map[string]any
	if connected && !s.started.IsZero() && !s.connected.IsZero() && s.overview.IsZero() {
		s.overview = time.Now()
		event = s.eventLocked()
	}
	s.mu.Unlock()
	if event != nil {
		a.recordDiagnostic(event)
	}
	respondJSON(w, map[string]bool{"ok": true})
}

// -1 denotes a milestone not yet observed, never an inferred duration.
func allowClientLaunchTimingDiagnostic(input map[string]any) map[string]any {
	out := map[string]any{"event": "client_launch_to_connected"}
	if id, ok := input["client_id"].(string); ok {
		_, out["client_id"] = safeClientLaunchEvent(id)
	}
	for _, k := range []string{"ms_to_process", "ms_to_connected", "ms_to_overview"} {
		switch v := input[k].(type) {
		case int64:
			if v >= -1 {
				out[k] = v
			}
		case int:
			if v >= -1 {
				out[k] = v
			}
		}
	}
	return out
}
