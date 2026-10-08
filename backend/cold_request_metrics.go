package main

import "time"

type coldSGPBytes struct {
	at    time.Time
	bytes int64
}

func (c *LCUClient) noteRequestStarted() {
	c.diagnosticMu.Lock()
	callback := c.requestStarted
	c.diagnosticMu.Unlock()
	if callback != nil {
		callback(time.Now())
	}
}

// The diagnostic window is independent of client-view presentation. It carries
// only a local epoch timestamp, allowing ResourceTiming and Go starts to share
// the process-detection zero without exposing account or request parameters.
func (a *app) coldRequestWindow() map[string]any {
	s := &a.coldLaunch
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.seen {
		return nil
	}
	return map[string]any{"type": "client-cold-window", "processAt": s.process.UnixMilli()}
}
func (a *app) bindColdRequestCounters(client *LCUClient) {
	if client == nil {
		return
	}
	client.diagnosticMu.Lock()
	client.requestStarted = func(at time.Time) { a.recordColdLCURequest(at) }
	client.diagnosticMu.Unlock()
}
func (a *app) recordColdLCURequest(at time.Time) {
	s := &a.coldLaunch
	s.mu.Lock()
	var event map[string]any
	if !s.seen || !at.Before(s.process) && at.Before(s.process.Add(3*time.Second)) {
		s.lcuStarts = append(s.lcuStarts, at)
		if s.seen && s.emitted {
			event = s.eventLocked()
		}
	}
	s.mu.Unlock()
	if event != nil {
		a.recordDiagnostic(event)
	}
}
func (a *app) recordColdSGPBytes(client *LCUClient, bytes int) {
	a.mu.RLock()
	current := a.lcu == client && a.connected && a.shutdownClient != client
	a.mu.RUnlock()
	if !current || bytes <= 0 {
		return
	}
	s := &a.coldLaunch
	s.mu.Lock()
	_, card := s.milestones["matches_card_ms"]
	if s.seen && !card {
		s.sgpFirstScreenBytes += int64(bytes)
		s.sgpSamples = append(s.sgpSamples, coldSGPBytes{at: time.Now(), bytes: int64(bytes)})
	}
	s.mu.Unlock()
}
func (a *app) recordBrowserColdRequests(processAt int64, queued, resources int, elapsed bool) bool {
	s := &a.coldLaunch
	s.mu.Lock()
	if !s.seen || processAt != s.process.UnixMilli() || resources < s.browserResources || queued < 0 || queued > resources {
		s.mu.Unlock()
		return false
	}
	s.browserMeasured = true
	s.browserQueued = max(s.browserQueued, queued)
	s.browserResources = resources
	s.browserWindowElapsed = s.browserWindowElapsed || elapsed
	var event map[string]any
	if s.emitted {
		event = s.eventLocked()
	}
	s.mu.Unlock()
	if event != nil {
		a.recordDiagnostic(event)
	}
	return true
}

// Renderer epoch avoids including diagnostic delivery latency in DOM milestones.
// Legacy diagnostic producers without an epoch retain their earlier behaviour.
func clientDOMEventTime(epoch int64, received time.Time) (time.Time, bool) {
	if epoch == 0 {
		return received, true
	}
	at := time.UnixMilli(epoch)
	return at, !at.After(received.Add(time.Second)) && !at.Before(received.Add(-5*time.Minute))
}
