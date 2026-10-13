package main

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"
)

func TestR277ManualRefreshHTTPReconnectsNonReadyView(t *testing.T) {
	for _, state := range []string{"exiting", "no-client"} {
		t.Run(state, func(t *testing.T) {
			a, old, _ := r258ViewFixture()
			defer old.Close()
			a.refreshRequests = make(chan struct{}, 1)
			session, cancel := context.WithCancel(context.Background())
			defer cancel()
			a.clientSessionCancel = cancel
			a.clientSessionCancelClient = old
			if state == "exiting" {
				a.observeClientShutdownEvent(old, LCUEvent{URI: "/riotclient/pre-shutdown/begin"}, time.Now())
			} else {
				a.selfSGPReady = false
			}
			defer func() { a.mu.Lock(); a.clearClientShutdownLocked(); a.cancelClientSessionLocked(); a.mu.Unlock() }()
			w := httptest.NewRecorder()
			a.handleRefresh(w, httptest.NewRequest("POST", "/api/refresh?source=user_refresh", nil))
			if w.Code != 202 {
				t.Fatalf("manual refresh status=%d", w.Code)
			}
			a.mu.RLock()
			retained := a.lcu != nil || a.shutdownClient != nil || a.connected
			a.mu.RUnlock()
			if retained || session.Err() == nil || len(a.refreshRequests) != 1 {
				t.Fatalf("HTTP reread failed full reconnect: state=%s retained=%v old_session=%v discovery_wakes=%d", state, retained, session.Err(), len(a.refreshRequests))
			}
		})
	}
}
