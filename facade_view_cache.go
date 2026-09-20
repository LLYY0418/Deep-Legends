package main

import (
	"context"
	"sync"
	"time"
)

// Account-bound, memory-only snapshots. A renderer navigation does not cancel
// the shared read or cache an empty catalogue for the next page visit.
type facadeViewCache struct {
	mu      sync.Mutex
	client  *LCUClient
	account int64
	value   facadeState
	at      time.Time
	flight  chan struct{}
}

func (a *app) invalidateFacadeView() {
	a.facadeView.mu.Lock()
	a.facadeView.at = time.Time{}
	// Detach pre-mutation flights too: their late completion must not restore
	// a stale snapshot after a successful write.
	a.facadeView.flight = nil
	a.facadeView.mu.Unlock()
}

func (a *app) cachedFacadeState(ctx context.Context, force bool, trigger string) (facadeState, error) {
	client, summoner, err := a.gameplayClient()
	if err != nil {
		return facadeState{Reason: "未连接英雄联盟客户端"}, nil
	}
	c := &a.facadeView
	for {
		c.mu.Lock()
		if c.client != client || c.account != summoner.SummonerID {
			c.client, c.at, c.value, c.flight = client, time.Time{}, facadeState{}, nil
			c.account = summoner.SummonerID
		}
		ttl := 30 * time.Second
		if c.value.SkinsUnavailable || !c.value.ChallengesReady {
			ttl = 2 * time.Second
		}
		if !force && !c.at.IsZero() && time.Since(c.at) < ttl {
			value := c.value
			c.mu.Unlock()
			return value, nil
		}
		done := c.flight
		if done == nil {
			done = make(chan struct{})
			c.flight = done
			go func() {
				readCtx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
				defer cancel()
				value := a.loadFacadeStateTriggered(readCtx, trigger)
				c.mu.Lock()
				if c.client == client && c.account == summoner.SummonerID && c.flight == done {
					c.value, c.at, c.flight = value, time.Now(), nil
				}
				close(done)
				c.mu.Unlock()
			}()
		}
		c.mu.Unlock()
		select {
		case <-ctx.Done():
			return facadeState{}, ctx.Err()
		case <-done:
			force = false
		}
		// Never deliver an old account after a client switch.
		current, account, err := a.gameplayClient()
		if err != nil || current != client || account.SummonerID != summoner.SummonerID {
			return facadeState{Reason: "客户端连接已变化"}, nil
		}
	}
}
