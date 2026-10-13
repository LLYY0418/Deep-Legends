package main

import (
	"context"
	"fmt"
	"github.com/gorilla/websocket"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

func TestR277ClearSnapshotCleansShutdownAndReopen(t *testing.T) {
	for _, delay := range []time.Duration{2 * time.Second, 20 * time.Second, 70 * time.Second} {
		t.Run(fmt.Sprint(delay), func(t *testing.T) {
			a, old, _ := r258ViewFixture()
			now := time.Unix(100, 0)
			a.observeClientShutdownEvent(old, LCUEvent{URI: "/riotclient/pre-shutdown/begin"}, now)
			a.mu.Lock()
			a.clearSnapshotLocked("old port unavailable")
			a.mu.Unlock()
			if a.shutdownClient != nil {
				t.Fatal("clearSnapshot retained shutdown pointer")
			}
			a.disconnectClient(old, "old session ended")
			if a.shutdownClient != nil || a.shutdownTimer != nil || a.shutdownEvents != nil || !a.clientLastEventAt.IsZero() || !a.shutdownChatOfflineAt.IsZero() || !a.shutdownFriendsEmptyAt.IsZero() {
				t.Fatal("shutdown state survived snapshot clear/disconnect")
			}
			next := newLCUClient(1, "fixture")
			defer next.Close()
			next.region = "TENCENT"
			next.rsoPlatform = "HN10"
			a.mu.Lock()
			a.lcu = next
			a.connected = true
			a.identityReady = true
			a.summoner = Summoner{SummonerID: 1, PUUID: "new"}
			a.summonerIdentityAt = now.Add(delay)
			a.selfReadinessClient = next
			a.selfReadinessAccount = "new"
			a.selfSGPReady = true
			a.mu.Unlock()
			if got := a.currentClientView(); got.State != "ready" {
				t.Fatalf("new client not ready within controlled 3s: %+v", got)
			}
		})
	}
}
func TestR277DisconnectClearsDetachedShutdown(t *testing.T) {
	a, old, _ := r258ViewFixture()
	a.shutdownClient = old
	a.lcu = nil
	a.disconnectClient(old, "already cleared")
	if a.shutdownClient != nil {
		t.Fatal("detached shutdown pointer retained")
	}
}
func TestR277ManualRefreshReconnectsStuckView(t *testing.T) {
	a, old, _ := r258ViewFixture()
	defer old.Close()
	a.refreshRequests = make(chan struct{}, 1)
	a.selfSGPReady = false
	a.shutdownClient = old
	a.requestRefresh()
	if a.lcu != nil || a.shutdownClient != nil || a.connected {
		t.Fatal("manual reread did not fully disconnect stuck view")
	}
	if len(a.refreshRequests) != 1 {
		t.Fatal("manual reread did not wake discovery")
	}
}
func TestR277InventorySuppressedDuringShutdown(t *testing.T) {
	a, old, _ := r258ViewFixture()
	defer old.Close()
	a.refreshRequests = make(chan struct{}, 1)
	a.observeClientShutdownEvent(old, LCUEvent{URI: "/riotclient/pre-shutdown/begin"}, time.Now())
	defer func() {
		if a.shutdownTimer != nil {
			a.shutdownTimer.Stop()
		}
	}()
	if a.markCollectionDirty("inventory") {
		t.Fatal("inventory accepted while exiting")
	}
	if a.collectionDirty || a.collectionEventTimer != nil || len(a.refreshRequests) != 0 {
		t.Fatal("inventory refresh queued while exiting")
	}
}
func TestR277ShutdownCancelsOldCollectionProbe(t *testing.T) {
	a, old, _ := r258ViewFixture()
	defer old.Close()
	entered, done, emergency := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var scans atomic.Int32
	a.collectionProbe = func(ctx context.Context, _ *LCUClient, _ int64) (bool, int) {
		close(entered)
		select {
		case <-ctx.Done():
		case <-emergency:
		}
		return false, 1
	}
	a.collectionRefresh = func(*LCUClient) bool { scans.Add(1); return true }
	go func() { a.refreshCollectionWithClient(old); close(done) }()
	<-entered
	a.observeClientShutdownEvent(old, LCUEvent{URI: "/riotclient/pre-shutdown/begin"}, time.Now())
	select {
	case <-done:
	case <-time.After(time.Second):
		close(emergency)
		<-done
		t.Error("old probe not cancelled within 1s of shutdown")
	}
	if a.shutdownTimer != nil {
		a.shutdownTimer.Stop()
	}
	if scans.Load() != 0 {
		t.Fatal("cancelled probe ran collection scan")
	}
}

func TestR277SessionProcessesShutdownWhileCollectionBlocked(t *testing.T) {
	entered, probeDone, sendShutdown := make(chan struct{}), make(chan struct{}), make(chan struct{})
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !websocket.IsWebSocketUpgrade(r) {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"summonerId":1,"puuid":"fixture"}`)
			return
		}
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		if _, _, err = c.ReadMessage(); err != nil {
			return
		}
		select {
		case <-sendShutdown:
		case <-r.Context().Done():
			return
		}
		c.WriteJSON([]any{8, "OnJsonApiEvent", map[string]any{"uri": "/riotclient/pre-shutdown/begin", "eventType": "Update", "data": true}})
		c.WriteJSON([]any{8, "OnJsonApiEvent", map[string]any{"uri": "/lol-inventory/v1/wallet", "eventType": "Update", "data": map[string]any{}}})
	}))
	defer server.Close()
	u, _ := url.Parse(server.URL)
	port, _ := strconv.Atoi(u.Port())
	client := newLCUClient(port, "fixture")
	defer client.Close()
	a := &app{lcu: client, connected: true, identityReady: true, summoner: Summoner{SummonerID: 1, PUUID: "fixture"}, refreshRequests: make(chan struct{}, 1)}
	a.collectionProbe = func(ctx context.Context, _ *LCUClient, _ int64) (bool, int) {
		close(entered)
		<-ctx.Done()
		close(probeDone)
		return false, 1
	}
	var scans atomic.Int32
	a.collectionRefresh = func(*LCUClient) bool { scans.Add(1); return true }
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sessionDone := make(chan error, 1)
	go func() { sessionDone <- a.runConnectedSession(ctx, client) }()
	a.queueCollectionRefresh("user_refresh")
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("probe not entered")
	}
	close(sendShutdown)
	select {
	case <-sessionDone:
	case <-time.After(time.Second):
		t.Fatal("blocked collection prevented shutdown handling")
	}
	select {
	case <-probeDone:
	case <-time.After(time.Second):
		t.Fatal("old probe survived session")
	}
	a.mu.RLock()
	retained := a.lcu != nil || a.shutdownClient != nil || a.collectionDirty
	a.mu.RUnlock()
	if retained || scans.Load() != 0 {
		t.Fatal("shutdown retained old state or scanned old client")
	}
}
func TestR277StuckWatchdogRetriesThenReconnectsOnce(t *testing.T) {
	a, old, _ := r258ViewFixture()
	defer old.Close()
	a.selfSGPReady = false
	a.selfReadinessPending = true
	start := time.Unix(100, 0)
	a.recoverStuckClientView(old, start)
	a.recoverStuckClientView(old, start.Add(9*time.Second))
	if a.clientViewRecoveryAttempted {
		t.Fatal("watchdog fired early")
	}
	a.recoverStuckClientView(old, start.Add(10*time.Second))
	if !a.clientViewRecoveryAttempted || a.lcu == nil {
		t.Fatal("watchdog did not retry readiness first")
	}
	a.recoverStuckClientView(old, start.Add(20*time.Second))
	if a.lcu != nil || a.shutdownClient != nil {
		t.Fatal("watchdog failed to reconnect persistent stuck view")
	}
}
func TestR277ClientViewPublishedStateDedup(t *testing.T) {
	a, old, _ := r258ViewFixture()
	defer old.Close()
	a.storage = newFlowDiagnosticStore(t)
	a.currentClientView()
	a.currentClientView()
	a.summoner.DisplayName = "Changed header"
	a.currentClientView()
	events := r175Events(t, a, "client_view_published")
	if len(events) != 1 || events[0]["state"] != "ready" {
		t.Fatalf("non-state publication not deduped: %v", events)
	}
}
