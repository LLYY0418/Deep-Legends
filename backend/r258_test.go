package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestR258ShutdownStatusAndRecentEventRestore(t *testing.T) {
	client := &LCUClient{}
	now := time.Now()
	a := &app{lcu: client, connected: true, eventStream: true, summoner: Summoner{SummonerID: 1}, discovery: LCUDiscoveryStatus{Result: "connected"}}
	a.observeClientShutdownEvent(client, LCUEvent{URI: "/riotclient/pre-shutdown/begin", Data: json.RawMessage(`true`)}, now)
	defer func() {
		if a.shutdownTimer != nil {
			a.shutdownTimer.Stop()
		}
	}()
	w := httptest.NewRecorder()
	a.handleStatus(w, httptest.NewRequest(http.MethodGet, "/api/status", nil))
	var status statusResponse
	if err := json.Unmarshal(w.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status.Connected || status.ClientDiscovery != "exiting" {
		t.Fatalf("unexpected shutdown view: %+v", status)
	}
	if a.restoreClientShutdown(client, now.Add(15*time.Second)) {
		t.Fatal("stale stream restored shutdown")
	}
	a.observeClientShutdownEvent(client, LCUEvent{URI: "/lol-gameflow/v1/gameflow-phase", Data: json.RawMessage(`"Lobby"`)}, now.Add(14*time.Second))
	a.eventStream = false
	if a.restoreClientShutdown(client, now.Add(15*time.Second)) {
		t.Fatal("closed stream restored shutdown")
	}
	a.eventStream = true
	if !a.restoreClientShutdown(client, now.Add(15*time.Second)) {
		t.Fatal("live stream could not restore false positive")
	}
}

func TestR258SignaledStreamCloseDisconnectsWithoutProbe(t *testing.T) {
	var reads atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reads.Add(1); <-r.Context().Done() }))
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, http: server.Client(), token: "fixture-token"}
	a := &app{lcu: client, connected: true, shutdownClient: client}
	started := time.Now()
	if err := a.probeAfterEventStreamClose(context.Background(), client); err == nil {
		t.Fatal("shutdown remained connected")
	}
	if reads.Load() != 0 || a.connected || time.Since(started) > 50*time.Millisecond {
		t.Fatalf("shutdown took %s, probes=%d, connected=%v", time.Since(started), reads.Load(), a.connected)
	}
}

func TestR258UnsignaledStreamCloseHasShortProbe(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, http: server.Client(), token: "fixture-token"}
	a := &app{lcu: client, connected: true}
	started := time.Now()
	if err := a.probeAfterEventStreamClose(context.Background(), client); err == nil {
		t.Fatal("hung client remained connected")
	}
	if a.connected || time.Since(started) > 2*time.Second {
		t.Fatalf("short probe took %s, connected=%v", time.Since(started), a.connected)
	}
}

func TestR258StatusDoesNotWaitForGatewayDiscovery(t *testing.T) {
	client := &LCUClient{region: "TENCENT", rsoPlatform: "HN1"}
	provider := newSGPProvider()
	provider.gatewayAccount = func(*LCUClient) string { return "fixture-account" }
	provider.gateway.mu.Lock()
	defer provider.gateway.mu.Unlock()
	a := &app{lcu: client, connected: true, identityReady: true, summoner: Summoner{SummonerID: 1}, sgp: provider, selfReadinessPending: true}
	done := make(chan struct{})
	go func() {
		w := httptest.NewRecorder()
		a.handleStatus(w, httptest.NewRequest(http.MethodGet, "/api/status", nil))
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("status waited for gateway discovery")
	}
}

func TestR258TimelineHeadersMatchesAndClose(t *testing.T) {
	a := r175App(t)
	now := time.Now()
	a.observeColdLaunchDiscovery(LCUDiscoveryStatus{ProcessCount: 1, ProcessAt: now, AttemptAt: now}, now)
	a.observeFirstMatchesCard("snapshot", now.Add(400*time.Millisecond))
	if events := r175Events(t, a, "client_cold_launch_timeline"); len(events) != 0 {
		t.Fatal("timeline emitted before header", events)
	}
	a.observeColdLaunchMilestone("self_tab_header_ms", now.Add(100*time.Millisecond))
	events := r175Events(t, a, "client_cold_launch_timeline")
	if len(events) != 1 || events[0]["self_tab_header_ms"] != float64(100) || events[0]["ui_first_change_ms"] != float64(100) || events[0]["matches_card_ms"] != float64(400) || events[0]["matches_card_source"] != "snapshot" || events[0]["overlay_shown"] != float64(0) {
		t.Fatal(events)
	}
	a.mu.Lock()
	a.beginClientCloseLocked(&LCUClient{}, now)
	a.mu.Unlock()
	a.recordClientTabRemoved(now.Add(200 * time.Millisecond))
	closes := r175Events(t, a, "client_close_timeline")
	if len(closes) != 1 || closes[0]["tab_removed_ms"] != float64(200) || closes[0]["overlay_shown"] != float64(0) {
		t.Fatal(closes)
	}
}

func TestR258StatusSelfReferenceMatchesOverviewScope(t *testing.T) {
	raw := "r258-self-fixture-player-000000001"
	client := &LCUClient{region: "TENCENT", rsoPlatform: "HN1"}
	a := &app{lcu: client, connected: true, summoner: Summoner{SummonerID: 1, PUUID: raw}}
	expected := a.registerGameplayReferenceDetails(gameplayReference{PlayerRef: raw, ServerID: "HN1"})
	w := httptest.NewRecorder()
	a.handleStatus(w, httptest.NewRequest(http.MethodGet, "/api/status", nil))
	var status statusResponse
	if err := json.Unmarshal(w.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status.Summoner.PlayerRef == "" || status.Summoner.PlayerRef != expected {
		t.Fatal("self reference differs from overview scope", status.Summoner.PlayerRef, expected)
	}
}
