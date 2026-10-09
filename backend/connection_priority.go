package main

import (
	"context"
	"sync"
	"time"
)

// The gate belongs to a connection, so delayed work cannot leak into its successor.
type connectionPriority struct {
	client  *LCUClient
	ctx     context.Context
	started time.Time
	card    chan struct{}
	once    sync.Once
}

func (a *app) beginConnectionPriority(ctx context.Context, client *LCUClient) *connectionPriority {
	g := &connectionPriority{client: client, ctx: ctx, started: time.Now(), card: make(chan struct{})}
	a.mu.Lock()
	a.connectionPriority = g
	a.mu.Unlock()
	return g
}

func (a *app) observeConnectionFirstCard() {
	a.mu.RLock()
	g := a.connectionPriority
	valid := g != nil && a.clientSessionConnectedLocked() && a.lcu == g.client && a.shutdownClient != g.client
	a.mu.RUnlock()
	if valid {
		g.once.Do(func() { close(g.card) })
	}
}

func (g *connectionPriority) wait(ctx context.Context, objective bool) bool {
	delay := 3 * time.Second
	card := g.card
	if objective {
		delay, card = 30*time.Second, nil
	}
	timer := time.NewTimer(max(time.Duration(0), time.Until(g.started.Add(delay))))
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-g.ctx.Done():
		return false
	case <-card:
	case <-timer.C:
	}
	return ctx.Err() == nil && g.ctx.Err() == nil
}

func (a *app) waitConnectionPriority(ctx context.Context, client *LCUClient) bool {
	a.mu.RLock()
	g := a.connectionPriority
	a.mu.RUnlock()
	if g == nil || g.client != client {
		return ctx.Err() == nil
	}
	return g.wait(ctx, false)
}

func (a *app) scheduleConnectionWork(ctx context.Context, client *LCUClient, name string, objective bool, work func()) {
	a.goSafe(name, func() {
		a.mu.RLock()
		g := a.connectionPriority
		a.mu.RUnlock()
		if g == nil || g.client != client || !g.wait(ctx, objective) {
			return
		}
		if objective {
			for {
				a.gameplayFlow.mu.Lock()
				phase, owner := a.gameplayFlow.phase, a.gameplayFlow.client
				a.gameplayFlow.mu.Unlock()
				// Unknown phase is not evidence of idle.
				if owner == client && phase != "" && phase != "ChampSelect" && phase != "GameStart" && phase != "InProgress" && phase != "Reconnect" {
					break
				}
				timer := time.NewTimer(time.Second)
				select {
				case <-ctx.Done():
					timer.Stop()
					return
				case <-g.ctx.Done():
					timer.Stop()
					return
				case <-timer.C:
				}
			}
		}
		a.mu.RLock()
		valid := a.lcu == client && a.clientSessionConnectedLocked() && a.shutdownClient != client
		a.mu.RUnlock()
		if valid && ctx.Err() == nil && g.ctx.Err() == nil {
			work()
		}
	})
}
