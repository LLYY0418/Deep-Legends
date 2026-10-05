package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestR207HonorBallotOnlyAtEndgame(t *testing.T) {
	var writes atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { writes.Add(1); w.WriteHeader(http.StatusNoContent) }))
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "test", http: server.Client()}
	finished := make(chan string, 8)
	runner := newWatchRunner(nil, func(event string) { finished <- event })
	settings := defaultWatchSettings()
	settings.MasterEnabled = true
	settings.Rules.AutoHonor.Enabled = true
	settings.Rules.AutoHonor.Strategy = "abstain"
	runner.apply(settings)
	triggers := make(chan map[string]any, 10)
	runner.observe = func(event map[string]any) {
		if event["event"] == "endgame_trigger" && event["source"] == "honor-ballot" {
			triggers <- event
		}
	}
	event := LCUEvent{URI: "/lol-honor-v2/v1/ballot", Data: json.RawMessage(`{"eligiblePlayers":[]}`)}
	for _, phase := range []string{"ChampSelect", "GameStart", "InProgress", "WaitingForStats"} {
		runner.handlePhase(client, phase)
		runner.handleEvent(client, event, Summoner{})
	}
	time.Sleep(30 * time.Millisecond)
	if writes.Load() != 0 || len(triggers) != 0 {
		t.Fatalf("early ballot produced writes=%d triggers=%d", writes.Load(), len(triggers))
	}
	for _, phase := range []string{"PreEndOfGame", "EndOfGame"} {
		runner.handlePhase(client, phase)
		runner.handleEvent(client, event, Summoner{})
		select {
		case got := <-triggers:
			if got["phase"] != phase {
				t.Fatalf("phase = %v", got)
			}
		case <-time.After(time.Second):
			t.Fatal("endgame ballot did not trigger")
		}
		deadline := time.After(time.Second)
	waitFinished:
		for {
			select {
			case event := <-finished:
				if event == "watch:fired:auto-honor" {
					break waitFinished
				}
			case <-deadline:
				t.Fatal("honor did not complete")
			}
		}
		// fired is emitted before handleHonor's defer clears honorInProgress.
		// Wait for that cleanup before the next phase, within the same deadline.
		for {
			runner.mu.Lock()
			inProgress := runner.honorInProgress
			runner.mu.Unlock()
			if !inProgress {
				break
			}
			select {
			case <-deadline:
				t.Fatal("honor completion did not clear in-progress state")
			case <-time.After(time.Millisecond):
			}
		}
	}
}

func TestR207ClientDiagnosticsPersistCompleteBoundedFields(t *testing.T) {
	a := r175App(t)
	body := `{"event":"live_recommendation_render","reason":"empty","phase":"InProgress","gameId":9014523366,"championId":420,"targetKey":"420:other:KIWI:12:emerald_plus:none","exactHit":false,"fallbackHit":false,"cachedKeys":["a","b","c","d","e","f"],"hasPayloadField":false,"augmentRows":0,"hasBuild":false,"lastResetReason":"hard_refresh","msSinceReset":17}`
	w := httptest.NewRecorder()
	a.handleClientDiagnostic(w, httptest.NewRequest("POST", "/api/diagnostics/client", strings.NewReader(body)))
	if w.Code != http.StatusNoContent {
		t.Fatalf("render diagnostic: %d %s", w.Code, w.Body.String())
	}
	events := r175Events(t, a, "live_recommendation_render")
	if len(events) != 1 {
		t.Fatalf("events=%v", events)
	}
	for _, field := range []string{"phase", "game_id", "champion_id", "target_key", "exact_hit", "fallback_hit", "cached_keys", "has_payload_field", "augment_rows", "has_build", "last_reset_reason", "ms_since_reset"} {
		if _, ok := events[0][field]; !ok {
			t.Fatalf("missing %s", field)
		}
	}
	if len(events[0]["cached_keys"].([]any)) != 5 {
		t.Fatalf("unbounded keys: %v", events[0])
	}
	body = `{"event":"live_scope_reset","reason":"hard_refresh","source":"manual","previousGameId":9014523366,"gameId":9014523366,"previousPhase":"InProgress","phase":"InProgress","clearedRecommendations":2}`
	w = httptest.NewRecorder()
	a.handleClientDiagnostic(w, httptest.NewRequest("POST", "/api/diagnostics/client", strings.NewReader(body)))
	if w.Code != http.StatusNoContent {
		t.Fatalf("reset diagnostic: %d", w.Code)
	}
	events = r175Events(t, a, "live_scope_reset")
	if len(events) != 1 || events[0]["cleared_recommendations"] != float64(2) {
		t.Fatalf("reset=%v", events)
	}
}

func TestR207HonorStopsWhenPhaseChangesDuringRead(t *testing.T) {
	reading, resume := make(chan struct{}), make(chan struct{})
	var writes atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			close(reading)
			<-resume
			_, _ = w.Write([]byte(`[]`))
			return
		}
		writes.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "test", http: server.Client()}
	runner := newWatchRunner(nil, nil)
	settings := defaultWatchSettings()
	settings.MasterEnabled = true
	settings.Rules.AutoHonor.Enabled = true
	settings.Rules.AutoHonor.Strategy = "any-teammate"
	runner.apply(settings)
	runner.handlePhase(client, "EndOfGame")
	runner.handleEvent(client, LCUEvent{URI: "/lol-honor-v2/v1/ballot", Data: json.RawMessage(`{"eligiblePlayers":[{"puuid":"ally","isAlly":true}]}`)}, Summoner{})
	select {
	case <-reading:
	case <-time.After(time.Second):
		close(resume)
		t.Fatal("read did not start")
	}
	runner.handlePhase(client, "GameStart")
	close(resume)
	deadline := time.Now().Add(time.Second)
	for {
		runner.mu.Lock()
		busy := runner.honorInProgress
		runner.mu.Unlock()
		if !busy {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("honor did not stop")
		}
		time.Sleep(time.Millisecond)
	}
	if writes.Load() != 0 {
		t.Fatalf("late honor writes = %d", writes.Load())
	}
}
