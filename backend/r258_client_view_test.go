package main

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func r258ViewFixture() (*app, *LCUClient, chan string) {
	client := &LCUClient{http: &http.Client{Transport: &http.Transport{}}, region: "TENCENT", rsoPlatform: "HN10", gameVersion: "26.19.1"}
	a := &app{lcu: client, connected: true, identityReady: true, eventStream: true, summoner: Summoner{SummonerID: 1, PUUID: strings.Repeat("v", 48), GameName: "Fixture", ProfileIconID: 7}, selfReadinessClient: client, selfReadinessAccount: strings.Repeat("v", 48), selfSGPReady: true, eventSubscribers: map[chan string]struct{}{}}
	updates := make(chan string, 32)
	a.eventSubscribers[updates] = struct{}{}
	return a, client, updates
}
func r258ReadView(t *testing.T, updates chan string) clientView {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		select {
		case event := <-updates:
			var view clientView
			if json.Unmarshal([]byte(event), &view) == nil && view.Type == "client-view" {
				return view
			}
		case <-deadline:
			t.Fatal("client-view not published")
			return clientView{}
		}
	}
}
func TestR258ClientViewSixEntrances(t *testing.T) {
	now := time.Now()
	for _, tc := range []struct {
		name, state string
		run         func(*app, *LCUClient)
	}{
		{"phase", "ready", func(a *app, c *LCUClient) { a.setConnectionPhase("probe-failed", false) }},
		{"identity", "ready", func(a *app, c *LCUClient) {
			c.discoverySummoner = &a.summoner
			a.connected = false
			if !a.refreshIdentityWithClient(c) {
				t.Fatal("identity failed")
			}
		}},
		{"shutdown", "exiting", func(a *app, c *LCUClient) {
			a.observeClientShutdownEvent(c, LCUEvent{URI: "/riotclient/pre-shutdown/begin"}, now)
		}},
		{"mark-disconnected", "no-client", func(a *app, c *LCUClient) { a.markDisconnected("fixture") }},
		{"disconnect", "no-client", func(a *app, c *LCUClient) { a.disconnectClient(c, "fixture") }},
		{"restore", "ready", func(a *app, c *LCUClient) {
			a.shutdownClient = c
			a.shutdownAt = now.Add(-16 * time.Second)
			a.clientLastEventAt = now
			if !a.restoreClientShutdown(c, now) {
				t.Fatal("restore failed")
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, client, updates := r258ViewFixture()
			tc.run(a, client)
			if a.shutdownTimer != nil {
				defer a.shutdownTimer.Stop()
			}
			view := r258ReadView(t, updates)
			if view.State != tc.state || view.Generation == 0 {
				t.Fatalf("view=%+v", view)
			}
			if view.State != "no-client" && (view.ServerID != "HN10" || !strings.HasPrefix(view.Summoner.PlayerRef, "player_")) {
				t.Fatalf("identity=%+v", view)
			}
			raw, _ := json.Marshal(view)
			if strings.Contains(string(raw), strings.Repeat("v", 48)) || strings.Contains(string(raw), "probe-failed") {
				t.Fatalf("internal data leaked: %s", raw)
			}
			after := a.currentClientView()
			if after.Generation != view.Generation {
				t.Fatalf("unchanged view incremented: %d -> %d", view.Generation, after.Generation)
			}
		})
	}
}

func TestR258ClientViewReadinessAndCacheOnly(t *testing.T) {
	a, client, updates := r258ViewFixture()
	var reads atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reads.Add(1); http.Error(w, "unexpected I/O", 500) }))
	defer server.Close()
	client.http = server.Client()
	client.baseURL = server.URL
	client.token = "fixture"
	a.selfSGPReady = false
	if view := a.currentClientView(); view.State != "no-client" {
		t.Fatalf("SGP waiting view=%+v", view)
	}
	a.selfSGPReady = true
	a.publishClientView()
	r258ReadView(t, updates)
	view := r258ReadView(t, updates)
	if view.State != "ready" || view.Generation != 2 {
		t.Fatalf("ready=%+v", view)
	}
	client.rsoPlatform = ""
	a.publishClientView()
	if r258ReadView(t, updates).State != "no-client" {
		t.Fatal("unknown Tencent server became ready")
	}
	if reads.Load() != 0 {
		t.Fatalf("view performed %d requests", reads.Load())
	}
}

func TestR258ClientViewInitialSSEAndGeneration(t *testing.T) {
	a, client, _ := r258ViewFixture()
	server := httptest.NewServer(http.HandlerFunc(a.handleEvents))
	defer server.Close()
	res, err := server.Client().Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	reader := bufio.NewReader(res.Body)
	next := func() clientView {
		t.Helper()
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				t.Fatal(err)
			}
			var view clientView
			if strings.HasPrefix(line, "data: ") && json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &view) == nil && view.Type == "client-view" {
				return view
			}
		}
	}
	initial := next()
	if initial.State != "ready" || initial.Generation != 1 {
		t.Fatalf("initial=%+v", initial)
	}
	a.observeClientShutdownEvent(client, LCUEvent{URI: "/riotclient/pre-shutdown/begin"}, time.Now())
	defer a.shutdownTimer.Stop()
	view := next()
	if view.Generation == initial.Generation {
		view = next()
	} // initial publication can also be queued
	if view.State != "exiting" || view.Generation <= initial.Generation {
		t.Fatalf("exit=%+v", view)
	}
	a.publishClientView()
	if got := a.currentClientView().Generation; got != view.Generation {
		t.Fatalf("duplicate exit advanced generation to %d", got)
	}
}
