package main

import (
	"context"
	"time"
)

// Caller holds a.mu. All LCU clear/replacement paths share this invariant.
func (a *app) clearClientShutdownLocked() {
	if a.shutdownTimer != nil {
		a.shutdownTimer.Stop()
	}
	a.shutdownClient = nil
	a.shutdownTimer = nil
	a.shutdownEvents = nil
	a.shutdownAt = time.Time{}
	a.shutdownChatOfflineAt = time.Time{}
	a.shutdownFriendsEmptyAt = time.Time{}
	a.clientLastEventAt = time.Time{}
	if a.collectionEventTimer != nil {
		a.collectionEventTimer.Stop()
		a.collectionEventTimer = nil
	}
	a.collectionEventGeneration++
	a.clientViewStuckSince = time.Time{}
	a.clientViewRecoveryAttempted = false
}
func (a *app) cancelCollectionProbeLocked(client *LCUClient) {
	if a.collectionProbeCancel != nil && (client == nil || a.collectionProbeClient == client) {
		a.collectionProbeCancel()
		a.collectionProbeCancel = nil
		a.collectionProbeClient = nil
	}
}
func (a *app) cancelClientSessionLocked() {
	a.cancelCollectionProbeLocked(nil)
	if a.clientSessionCancel != nil {
		a.clientSessionCancel()
	}
	a.clientSessionCancel = nil
	a.clientSessionCancelClient = nil
}
func (a *app) replaceClientLocked(client *LCUClient) {
	if a.lcu != client || a.shutdownClient != nil && a.shutdownClient != client {
		a.clearClientShutdownLocked()
		a.cancelClientSessionLocked()
	}
	a.lcu = client
}
func (a *app) clientViewReasonsLocked() map[string]any {
	return map[string]any{"connected": a.connected, "identity_ready": a.identityReady, "shutdown_pending": a.shutdownClient != nil, "shutdown_matches_lcu": a.shutdownClient != nil && a.shutdownClient == a.lcu, "readiness_pending": a.selfReadinessPending, "sgp_ready": a.selfSGPReady, "event_stream": a.eventStream}
}

// A cached-state watchdog: no I/O on the select loop, once per stalled session.
func (a *app) recoverStuckClientView(client *LCUClient, now time.Time) {
	view := a.currentClientView()
	a.mu.Lock()
	if a.lcu != client || !a.identityReady || !a.eventStream || view.State == "ready" || a.shutdownClient != nil {
		a.clientViewStuckSince = time.Time{}
		a.mu.Unlock()
		return
	}
	if a.clientViewStuckSince.IsZero() {
		a.clientViewStuckSince = now
		a.mu.Unlock()
		return
	}
	elapsed := now.Sub(a.clientViewStuckSince)
	if elapsed < 10*time.Second {
		a.mu.Unlock()
		return
	}
	if a.clientViewRecoveryAttempted {
		a.mu.Unlock()
		if elapsed >= 20*time.Second {
			a.markDisconnected("客户端连接状态正在恢复")
		}
		return
	}
	a.clientViewRecoveryAttempted = true
	event := a.clientViewReasonsLocked()
	event["event"] = "client_view_stuck"
	a.selfReadinessClient = nil
	a.selfReadinessAccount = ""
	a.selfSGPReady = false
	a.mu.Unlock()
	a.recordDiagnostic(event)
	a.refreshSelfReadinessAsync(client)
}

// Legacy API loaders also honor the lifetime of the LCU session, without
// replacing their caller's deadline or cancellation cause.
func (c *LCUClient) sessionRequestContext(parent context.Context) (context.Context, func()) {
	c.mu.RLock()
	session := c.sessionContext
	c.mu.RUnlock()
	if session == nil {
		return parent, func() {}
	}
	ctx, cancel := context.WithCancel(parent)
	stop := context.AfterFunc(session, cancel)
	if session.Err() != nil {
		cancel()
	}
	return ctx, func() { stop(); cancel() }
}
