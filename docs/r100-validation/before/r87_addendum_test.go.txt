package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestR87P7EveryEndPhaseResetsMatchmakingExhaustion(t *testing.T) {
	for _, phase := range []string{"WaitingForStats", "PreEndOfGame", "EndOfGame"} {
		t.Run(phase, func(t *testing.T) {
			r := newWatchRunner(nil, nil)
			r.autoMatchStarted, r.autoMatchExhausted = true, true
			r.handlePhase(&LCUClient{}, phase)
			if r.autoMatchStarted || r.autoMatchExhausted {
				t.Fatal("end phase did not reset matchmaking state")
			}
		})
	}
}

func TestR87P7FourStateGuardExplainsEachSkip(t *testing.T) {
	for _, reason := range []string{"custom", "started", "inflight", "exhausted"} {
		t.Run(reason, func(t *testing.T) {
			r := newWatchRunner(nil, nil)
			var events []map[string]any
			r.observe = func(e map[string]any) { events = append(events, e) }
			switch reason {
			case "custom":
				r.customSession = true
			case "started":
				r.autoMatchStarted = true
			case "inflight":
				r.autoMatchInFlight = true
			case "exhausted":
				r.autoMatchExhausted = true
			}
			if r.scheduleAutoMatchmaking(&LCUClient{}, 0) {
				t.Fatal("guard scheduled IO")
			}
			if len(events) != 1 || events[0]["result"] != "skipped_state" || events[0]["reason"] != reason {
				t.Fatalf("events=%v", events)
			}
		})
	}
}

func TestR87P7CanceledOldMatchmakingCannotPoisonNextGame(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	done := make(chan struct{}, 1)
	c := &LCUClient{baseURL: "https://127.0.0.1:2999", token: "fixture", http: &http.Client{Transport: gameplayRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method == http.MethodPost {
			close(entered)
			<-release
			return response2351(nil), nil
		}
		return response2351(map[string]any{"localMember": map[string]any{"isLeader": true}, "gameConfig": map[string]any{"queueId": 420}}), nil
	})}}
	r := newWatchRunner(nil, nil)
	r.observe = func(e map[string]any) {
		if e["result"] == "canceled" {
			select {
			case done <- struct{}{}:
			default:
			}
		}
	}
	t.Cleanup(func() { r.cancelPending("auto-matchmaking") })
	r.scheduleAutoMatchmaking(c, 0)
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("search not started")
	}
	r.handlePhase(c, "WaitingForStats")
	if !r.scheduleAutoMatchmaking(c, 60000) {
		t.Fatal("reset did not permit next game")
	}
	close(release)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("old action not canceled")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.autoMatchStarted || r.autoMatchExhausted || !r.autoMatchInFlight {
		t.Fatal("old action overwrote next game's state")
	}
}

func TestR87P7CanceledJobsDoNotBlockReenabledMatchmaking(t *testing.T) {
	for _, mode := range []string{"rule-toggle", "master-toggle", "disabled-phase", "custom-session", "leave-lobby"} {
		t.Run(mode, func(t *testing.T) {
			r := newWatchRunner(nil, nil)
			settings := r.currentWatch()
			settings.MasterEnabled, settings.Rules.AutoMatchmaking.Enabled = true, true
			r.apply(settings)
			client := &LCUClient{}
			if !r.scheduleAutoMatchmaking(client, 60000) {
				t.Fatal("initial job not scheduled")
			}
			t.Cleanup(func() { r.cancelPending("auto-matchmaking") })
			switch mode {
			case "rule-toggle":
				settings.Rules.AutoMatchmaking.Enabled = false
				r.apply(settings)
			case "master-toggle":
				settings.MasterEnabled = false
				r.apply(settings)
			case "disabled-phase":
				r.mu.Lock()
				r.settings.MasterEnabled = false
				r.mu.Unlock()
				r.handlePhase(client, "Lobby")
			case "custom-session":
				r.setCustomSession(true)
				r.setCustomSession(false)
			case "leave-lobby":
				r.handlePhase(client, "None")
			}
			settings.MasterEnabled, settings.Rules.AutoMatchmaking.Enabled = true, true
			r.apply(settings)
			if !r.scheduleAutoMatchmaking(client, 60000) {
				t.Fatal("detached job left inflight set after re-enabling")
			}
		})
	}
}

func TestR87P7AndP9ClientDiagnosticsReachExport(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(fmt.Sprint(enabled), func(t *testing.T) {
			store := newFlowDiagnosticStore(t)
			a := &app{storage: store}
			w := httptest.NewRecorder()
			a.handleClientDiagnostic(w, httptest.NewRequest("POST", "/api/diagnostics/client", strings.NewReader(fmt.Sprintf(`{"event":"watch_settings_client","reason":"rendered","autoMatchmakingEnabled":%t}`, enabled))))
			data, _ := store.readDiagnosticLog()
			var e map[string]any
			if w.Code != 204 || json.Unmarshal(data, &e) != nil || e["auto_matchmaking_enabled"] != enabled {
				t.Fatalf("diagnostic=%s", data)
			}
		})
	}
	for _, reason := range []string{"open", "rerender-while-open", "close"} {
		store := newFlowDiagnosticStore(t)
		a := &app{storage: store}
		w := httptest.NewRecorder()
		a.handleClientDiagnostic(w, httptest.NewRequest("POST", "/api/diagnostics/client", strings.NewReader(fmt.Sprintf(`{"event":"champselect_dialog_client","reason":%q,"revision":6}`, reason))))
		data, _ := store.readDiagnosticLog()
		var e map[string]any
		if w.Code != 204 || json.Unmarshal(data, &e) != nil || e["reason"] != reason || e["revision"] != float64(6) || e["time"] == nil {
			t.Fatalf("dialog diagnostic=%s", data)
		}
	}
}
