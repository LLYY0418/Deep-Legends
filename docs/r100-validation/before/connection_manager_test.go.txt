package main

import (
	"context"
	"errors"
	"github.com/gorilla/websocket"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

func TestR86ConnectionBackoffResetsAfterSuccessfulDiscovery(t *testing.T) {
	a := &app{refreshRequests: make(chan struct{}, 1)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var delays []time.Duration
	attempt := 0
	a.runConnectionManagerWith(ctx, connectionLoopOps{
		discover: func() (*LCUClient, LCUDiscoveryStatus, error) {
			attempt++
			if attempt == 4 {
				return newLCUClient(1, "fixture"), LCUDiscoveryStatus{Result: "connected"}, nil
			}
			return nil, LCUDiscoveryStatus{Result: "process-not-found"}, errLCUNotFound
		},
		identity: func(c *LCUClient) bool { a.mu.Lock(); a.lcu = c; a.connected = true; a.mu.Unlock(); return true },
		session:  func(context.Context, *LCUClient) error { return errors.New("fixture disconnected") },
		wait: func(_ context.Context, d time.Duration) bool {
			delays = append(delays, d)
			if len(delays) == 4 {
				cancel()
				return false
			}
			return true
		},
	})
	want := []time.Duration{3 * time.Second, 6 * time.Second, 8 * time.Second, 3 * time.Second}
	if len(delays) != len(want) {
		t.Fatal(delays)
	}
	for i := range want {
		if delays[i] != want[i] {
			t.Fatalf("delays=%v", delays)
		}
	}
}

func TestR86ManualDisconnectPausesDiscoveryUntilRefresh(t *testing.T) {
	a := &app{manualDisconnected: true, refreshRequests: make(chan struct{}, 1)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	called := make(chan struct{}, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		a.runConnectionManagerWith(ctx, connectionLoopOps{
			discover: func() (*LCUClient, LCUDiscoveryStatus, error) {
				called <- struct{}{}
				cancel()
				return nil, LCUDiscoveryStatus{}, errLCUNotFound
			},
			wait: func(context.Context, time.Duration) bool { return false },
		})
	}()
	select {
	case <-called:
		t.Fatal("discovered while paused")
	case <-time.After(30 * time.Millisecond):
	}
	a.requestRefresh()
	select {
	case <-called:
	case <-time.After(time.Second):
		t.Fatal("refresh did not resume discovery")
	}
	<-done
	a.refreshRequests <- struct{}{}
	start := time.Now()
	if !a.waitForDiscovery(context.Background(), time.Hour) || time.Since(start) > 100*time.Millisecond {
		t.Fatal("refresh did not wake long backoff")
	}
}

func TestR86WebsocketDropKeepsLiveLCUConnection(t *testing.T) {
	var attempts atomic.Int32
	reconnected := make(chan struct{}, 1)
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !websocket.IsWebSocketUpgrade(r) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"summonerId": 1, "puuid": "fixture"}`))
			return
		}
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		if _, _, err := c.ReadMessage(); err != nil {
			return
		}
		if attempts.Add(1) == 1 {
			return
		}
		reconnected <- struct{}{}
		for {
			if _, _, err := c.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer server.Close()
	u, _ := url.Parse(server.URL)
	port, _ := strconv.Atoi(u.Port())
	client := newLCUClient(port, "fixture")
	defer client.Close()
	a := &app{lcu: client, connected: true, refreshRequests: make(chan struct{}, 1)}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- a.runConnectedSession(ctx, client) }()
	defer func() {
		cancel()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Error("session failed to stop")
		}
	}()
	select {
	case <-reconnected:
	case err := <-done:
		done <- err
		t.Fatalf("session stopped after WS-only drop: %v", err)
	case <-time.After(8 * time.Second):
		t.Fatal("event stream not retried")
	}
	a.mu.RLock()
	connected, same := a.connected, a.lcu == client
	a.mu.RUnlock()
	if !connected || !same {
		t.Fatal("WS-only loss disconnected healthy LCU")
	}
}
