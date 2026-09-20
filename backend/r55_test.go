package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestR55ClaimSelectionShapes(t *testing.T) {
	tests := []struct {
		name        string
		items       []RewardItem
		minimum     int
		maximum     int
		wantRequest bool
		wantIDs     []string
	}{
		{name: "one of one is completed automatically", items: []RewardItem{{ID: "one"}}, minimum: 1, maximum: 1, wantRequest: true, wantIDs: []string{"one"}},
		{name: "one of three remains a user choice", items: []RewardItem{{ID: "one"}, {ID: "two"}, {ID: "three"}}, minimum: 1, maximum: 1},
		{name: "three of three are completed automatically", items: []RewardItem{{ID: "one"}, {ID: "two"}, {ID: "three"}}, minimum: 3, maximum: 3, wantRequest: true, wantIDs: []string{"one", "two", "three"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requests := make(chan []string, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != "/lol-rewards/v1/grants/grant/select" {
					http.NotFound(w, r)
					return
				}
				var body struct {
					Selections []string `json:"selections"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				requests <- body.Selections
				w.WriteHeader(http.StatusNoContent)
			}))
			t.Cleanup(server.Close)
			client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
			entry := claimEntry{Source: "grant", ID: "grant", RewardGroupID: "group", Items: test.items, MinSelections: test.minimum, MaxSelections: test.maximum}
			err := executeClaimEntry(context.Background(), client, entry, nil)
			if test.wantRequest {
				if err != nil {
					t.Fatalf("executeClaimEntry() error = %v", err)
				}
				select {
				case got := <-requests:
					if strings.Join(got, ",") != strings.Join(test.wantIDs, ",") {
						t.Fatalf("selections = %#v, want %#v", got, test.wantIDs)
					}
				case <-time.After(time.Second):
					t.Fatal("claim request was not issued")
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), "需要选择 1 到 1 项奖励") {
				t.Fatalf("ambiguous claim error = %v", err)
			}
			select {
			case got := <-requests:
				t.Fatalf("ambiguous claim issued request with %#v", got)
			default:
			}
		})
	}
}

func TestR55ClaimScanOnlyFlagsAmbiguousChoices(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/lol-rewards/v1/grants":
			_, _ = io.WriteString(w, `[
              {"info":{"id":"one","status":"PENDING"},"rewardGroup":{"id":"g-one","selectionStrategyConfig":{"minSelectionsAllowed":1,"maxSelectionsAllowed":1},"rewards":[{"id":"one-a","quantity":1}]}},
              {"info":{"id":"choice","status":"PENDING"},"rewardGroup":{"id":"g-choice","selectionStrategyConfig":{"minSelectionsAllowed":1,"maxSelectionsAllowed":1},"rewards":[{"id":"choice-a","quantity":1},{"id":"choice-b","quantity":1},{"id":"choice-c","quantity":1}]}},
              {"info":{"id":"all","status":"PENDING"},"rewardGroup":{"id":"g-all","selectionStrategyConfig":{"minSelectionsAllowed":3,"maxSelectionsAllowed":3},"rewards":[{"id":"all-a","quantity":1},{"id":"all-b","quantity":1},{"id":"all-c","quantity":1}]}}
            ]`)
		case "/lol-missions/v1/missions", "/lol-event-hub/v1/events":
			_, _ = io.WriteString(w, `[]`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	scan := scanClaims(context.Background(), client)
	choices := map[string]bool{}
	for _, entry := range scan.Items {
		choices[entry.ID] = entry.NeedsChoice
	}
	if choices["one"] || !choices["choice"] || choices["all"] {
		t.Fatalf("choice decisions = %#v", choices)
	}
}

func TestR55ReadyCheckSignalsTriggerOneAccept(t *testing.T) {
	tests := []struct {
		name    string
		trigger func(*watchRunner, *LCUClient)
	}{
		{name: "ready-check event", trigger: func(r *watchRunner, c *LCUClient) {
			r.handleEvent(c, LCUEvent{URI: "/lol-matchmaking/v1/ready-check", Data: json.RawMessage(`{"state":"InProgress"}`)}, Summoner{})
		}},
		{name: "phase event", trigger: func(r *watchRunner, c *LCUClient) { r.handlePhase(c, "ReadyCheck") }},
		{name: "both signals are deduplicated", trigger: func(r *watchRunner, c *LCUClient) {
			r.handlePhase(c, "ReadyCheck")
			time.Sleep(25 * time.Millisecond)
			r.handleEvent(c, LCUEvent{URI: "/lol-matchmaking/v1/ready-check", Data: json.RawMessage(`{"state":"Pending"}`)}, Summoner{})
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost && r.URL.Path == "/lol-matchmaking/v1/ready-check/accept" {
					requests.Add(1)
					w.WriteHeader(http.StatusNoContent)
					return
				}
				http.NotFound(w, r)
			}))
			t.Cleanup(server.Close)
			client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
			runner := newWatchRunner(nil, nil)
			settings := defaultWatchSettings()
			settings.Rules.AutoAccept.Enabled = true
			settings.Rules.AutoAccept.DelayMS = 0
			runner.apply(settings)
			test.trigger(runner, client)
			deadline := time.Now().Add(time.Second)
			for requests.Load() == 0 && time.Now().Before(deadline) {
				time.Sleep(5 * time.Millisecond)
			}
			time.Sleep(40 * time.Millisecond)
			if got := requests.Load(); got != 1 {
				t.Fatalf("accept requests = %d, want 1", got)
			}
		})
	}
}

func TestR66CustomReadyCheckNeverAutoAccepts(t *testing.T) {
	triggers := []struct {
		name string
		run  func(*watchRunner, *LCUClient)
	}{
		{"phase", func(r *watchRunner, c *LCUClient) { r.handlePhase(c, "ReadyCheck") }},
		{"event", func(r *watchRunner, c *LCUClient) {
			r.handleEvent(c, LCUEvent{URI: "/lol-matchmaking/v1/ready-check", Data: json.RawMessage(`{"state":"InProgress"}`)}, Summoner{})
		}},
	}
	for _, trigger := range triggers {
		t.Run(trigger.name, func(t *testing.T) {
			var accepts atomic.Int32
			diagnostics := make(chan map[string]any, 4)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				switch {
				case request.Method == http.MethodGet && request.URL.Path == "/lol-gameflow/v1/session":
					_, _ = io.WriteString(w, `{"phase":"ReadyCheck","gameData":{"queue":{"type":"CUSTOM_GAME"}}}`)
				case request.Method == http.MethodPost && request.URL.Path == "/lol-matchmaking/v1/ready-check/accept":
					accepts.Add(1)
					w.WriteHeader(http.StatusNoContent)
				default:
					http.NotFound(w, request)
				}
			}))
			t.Cleanup(server.Close)
			client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
			runner := newWatchRunner(nil, nil)
			runner.observe = func(event map[string]any) {
				// This assertion consumes action results, not the independent focus trace.
				if event["event"] == "watch_action" {
					diagnostics <- event
				}
			}
			settings := defaultWatchSettings()
			settings.Rules.AutoAccept.Enabled = true
			settings.Rules.AutoAccept.DelayMS = 0
			runner.apply(settings)
			trigger.run(runner, client)
			deadline := time.After(time.Second)
			for {
				select {
				case diagnostic := <-diagnostics:
					if diagnostic["result"] == "skipped_custom" {
						goto skipped
					}
				case <-deadline:
					t.Fatal("missing custom ReadyCheck diagnostic")
				}
			}
		skipped:
			time.Sleep(25 * time.Millisecond)
			if got := accepts.Load(); got != 0 {
				t.Fatalf("custom ReadyCheck accept requests = %d, want 0", got)
			}
		})
	}
}

func TestR66AcceptTransitionRaceDoesNotToast(t *testing.T) {
	events := make(chan string, 8)
	diagnostics := make(chan map[string]any, 8)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/lol-gameflow/v1/session":
			_, _ = io.WriteString(w, `{"phase":"ReadyCheck","gameData":{"queue":{"id":420,"gameMode":"CLASSIC","mapId":11,"type":"MATCHED_GAME"}}}`)
		case request.Method == http.MethodPost && request.URL.Path == "/lol-matchmaking/v1/ready-check/accept":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = io.WriteString(w, `{"errorCode":"ReadyCheckExpired","message":"window closed"}`)
		case request.Method == http.MethodGet && request.URL.Path == "/lol-gameflow/v1/gameflow-phase":
			_, _ = io.WriteString(w, `"ChampSelect"`)
		default:
			http.NotFound(w, request)
		}
	}))
	t.Cleanup(server.Close)
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	runner := newWatchRunner(nil, func(event string) { events <- event })
	runner.observe = func(event map[string]any) { diagnostics <- event }
	settings := defaultWatchSettings()
	settings.Rules.AutoAccept.Enabled = true
	settings.Rules.AutoAccept.DelayMS = 0
	runner.apply(settings)
	runner.handlePhase(client, "ReadyCheck")

	deadline := time.After(time.Second)
	var observedDiagnostics []map[string]any
	for {
		select {
		case diagnostic := <-diagnostics:
			observedDiagnostics = append(observedDiagnostics, diagnostic)
			if diagnostic["result"] == "failed_stale" {
				if diagnostic["status_code"] != 500 {
					t.Fatalf("stale diagnostic = %#v", diagnostic)
				}
				goto observed
			}
		case <-deadline:
			t.Fatalf("missing stale ReadyCheck diagnostic; got %#v", observedDiagnostics)
		}
	}

observed:
	time.Sleep(25 * time.Millisecond)
	for len(events) > 0 {
		if event := <-events; strings.HasPrefix(event, "watch:failed:accept:") {
			t.Fatalf("benign transition emitted an error toast event: %q", event)
		}
	}
}

func TestR55PrimeWatchStateReadsPhaseAndLobby(t *testing.T) {
	var phaseReads atomic.Int32
	var lobbyReads atomic.Int32
	writes := make(chan string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/lol-gameflow/v1/gameflow-phase":
			phaseReads.Add(1)
			_, _ = io.WriteString(w, `"ReadyCheck"`)
		case r.Method == http.MethodGet && r.URL.Path == "/lol-lobby/v2/lobby":
			lobbyReads.Add(1)
			_, _ = io.WriteString(w, `{"localMember":{"isLeader":true},"members":[{"summonerId":1}],"gameConfig":{"isCustom":false,"queueId":420}}`)
		case r.Method == http.MethodPost:
			writes <- r.URL.Path
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	runner := newWatchRunner(nil, nil)
	settings := defaultWatchSettings()
	settings.Rules.AutoAccept.Enabled = true
	settings.Rules.AutoAccept.DelayMS = 0
	settings.Rules.AutoMatchmaking.Enabled = true
	settings.Rules.AutoMatchmaking.DelayMS = 0
	settings.Rules.AutoMatchmaking.MinPartySize = 1
	runner.apply(settings)
	a := &app{watch: runner, summoner: Summoner{SummonerID: 1}}
	a.primeWatchState(client)

	seen := map[string]bool{}
	for len(seen) < 2 {
		select {
		case path := <-writes:
			seen[path] = true
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for primed actions; seen=%#v", seen)
		}
	}
	// Auto-matchmaking performs a fresh lobby preflight immediately before its
	// write, so priming now has the initial snapshot plus one safety read.
	if phaseReads.Load() != 1 || lobbyReads.Load() != 2 || !seen["/lol-matchmaking/v1/ready-check/accept"] || !seen["/lol-lobby/v2/lobby/matchmaking/search"] {
		t.Fatalf("phase reads=%d lobby reads=%d writes=%#v", phaseReads.Load(), lobbyReads.Load(), seen)
	}
	source, err := os.ReadFile("connection_manager.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(source), "go a.primeWatchState(client)") {
		t.Fatal("event-stream ready callback no longer primes watch state")
	}
}

func TestR55WatchFailureIncludesReasonAndRedactedDiagnostic(t *testing.T) {
	events := make(chan string, 4)
	diagnostics := make(chan map[string]any, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"errorCode":"ReadyCheckExpired","message":"确认窗口已过期"}`)
	}))
	t.Cleanup(server.Close)
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	runner := newWatchRunner(nil, func(event string) { events <- event })
	runner.observe = func(event map[string]any) {
		// Window observations are separate from this action-only assertion.
		if event["event"] == "watch_action" {
			diagnostics <- event
		}
	}
	settings := defaultWatchSettings()
	settings.Rules.AutoAccept.Enabled = true
	settings.Rules.AutoAccept.DelayMS = 0
	runner.apply(settings)
	runner.handlePhase(client, "ReadyCheck")

	var failedEvent string
	deadline := time.After(time.Second)
	for failedEvent == "" {
		select {
		case event := <-events:
			if strings.HasPrefix(event, "watch:failed:accept:") {
				failedEvent = event
			}
		case <-deadline:
			t.Fatal("missing failed watch event")
		}
	}
	if !strings.Contains(failedEvent, ":400:ReadyCheckExpired:确认窗口已过期") {
		t.Fatalf("failed event = %q", failedEvent)
	}
	var failedDiagnostic map[string]any
	deadline = time.After(time.Second)
	for failedDiagnostic == nil {
		select {
		case diagnostic := <-diagnostics:
			if diagnostic["result"] == "failed" {
				failedDiagnostic = diagnostic
			}
		case <-deadline:
			t.Fatal("missing failed watch diagnostic")
		}
	}
	if len(failedDiagnostic) != 5 || failedDiagnostic["event"] != "watch_action" || failedDiagnostic["action"] != "accept" || failedDiagnostic["status_code"] != 400 || failedDiagnostic["error_code"] != "ReadyCheckExpired" {
		t.Fatalf("diagnostic = %#v", failedDiagnostic)
	}
	if _, leaked := failedDiagnostic["message"]; leaked {
		t.Fatalf("diagnostic persisted player-bearing message: %#v", failedDiagnostic)
	}
}

func TestR55EventRetryBackoffBounds(t *testing.T) {
	tests := []struct {
		current time.Duration
		want    time.Duration
	}{
		{0, 5 * time.Second},
		{5 * time.Second, 10 * time.Second},
		{30 * time.Second, time.Minute},
		{45 * time.Second, time.Minute},
		{time.Minute, time.Minute},
	}
	for _, test := range tests {
		if got := nextEventRetryDelay(test.current); got != test.want {
			t.Errorf("nextEventRetryDelay(%s) = %s, want %s", test.current, got, test.want)
		}
	}
}
