package main

import (
	"context"
	"time"
)

const collectionEventRefreshDelay = 2500 * time.Millisecond

func (a *app) scheduleCollectionEventRefresh(delay time.Duration) {
	a.mu.Lock()
	if !a.collectionDirty || !a.connected || a.lcu == nil || a.shutdownClient != nil || a.refreshRequests == nil {
		a.mu.Unlock()
		return
	}
	if a.collectionEventTimer != nil {
		a.collectionEventTimer.Stop()
	}
	a.collectionEventGeneration++
	generation, epoch := a.collectionEventGeneration, a.clientSessionEpoch
	a.collectionEventTimer = time.AfterFunc(delay, func() {
		a.mu.Lock()
		valid := generation == a.collectionEventGeneration && epoch == a.clientSessionEpoch && a.collectionDirty && a.connected && a.shutdownClient == nil
		a.mu.Unlock()
		if !valid {
			return
		}
		a.gameplayFlow.mu.Lock()
		phase := a.gameplayFlow.phase
		a.gameplayFlow.mu.Unlock()
		if phase == "GameStart" || phase == "InProgress" || phase == "Reconnect" {
			return
		} // dirty survives until the end-phase handoff
		a.queueCollectionRefresh("inventory_event")
	})
	a.mu.Unlock()
}

func (a *app) prefetchSelfMayhemRating(ctxClient *LCUClient) {
	if a.mayhemRatings == nil || !isTencentClient(ctxClient) {
		return
	}
	a.mu.RLock()
	self := a.summoner
	a.mu.RUnlock()
	if self.GameName == "" || self.TagLine == "" {
		return
	}
	a.goSafe("mayhem.startup-prefetch", func() {
		a.mayhemRatings.lookup(context.WithValue(context.Background(), aramkitRatingPrefetchKey{}, true), self.GameName, self.TagLine)
	})
}
