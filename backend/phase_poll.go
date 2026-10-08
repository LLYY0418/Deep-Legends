package main

import "time"

// The session owns these timers. Phase notifications wake its select loop;
// ticker fields are never written by event/probe goroutines.
type champSelectPolling struct {
	ticker   *time.Ticker
	ticks    <-chan time.Time
	interval time.Duration
}

func (p *champSelectPolling) stop() {
	if p.ticker != nil {
		p.ticker.Stop()
	}
	p.ticker = nil
	p.ticks = nil
}
func (p *champSelectPolling) sync(a *app, client *LCUClient) bool {
	a.gameplayFlow.mu.Lock()
	active := a.gameplayFlow.client == client && a.gameplayFlow.phase == "ChampSelect"
	a.gameplayFlow.mu.Unlock()
	a.mu.RLock()
	active = active && a.lcu == client && a.connected && a.eventStream && a.shutdownClient != client
	a.mu.RUnlock()
	if !active {
		p.stop()
		return false
	}
	if p.ticker == nil {
		interval := p.interval
		if interval <= 0 {
			interval = time.Second
		}
		p.ticker = time.NewTicker(interval)
		p.ticks = p.ticker.C
	}
	return true
}
func (a *app) subscribeGameplayPhase(client *LCUClient) (chan struct{}, func()) {
	changes := make(chan struct{}, 1)
	f := &a.gameplayFlow
	f.mu.Lock()
	f.pollClient, f.pollChanges = client, changes
	f.mu.Unlock()
	return changes, func() {
		f.mu.Lock()
		if f.pollChanges == changes {
			f.pollClient = nil
			f.pollChanges = nil
		}
		f.mu.Unlock()
	}
}
func (a *app) signalGameplayPhase(client *LCUClient) {
	f := &a.gameplayFlow
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.pollClient == client {
		select {
		case f.pollChanges <- struct{}{}:
		default:
		}
	}
}
