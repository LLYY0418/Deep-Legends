package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func waitForWatchDiagnostic(t *testing.T, diagnostics <-chan map[string]any, action, result string) map[string]any {
	t.Helper()
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	for {
		select {
		case event := <-diagnostics:
			if event["action"] == action && event["result"] == result {
				return event
			}
		case <-timer.C:
			t.Fatalf("timed out waiting for %s/%s diagnostic", action, result)
		}
	}
}

func TestR68EventClaimWithoutParsedItemsIsOmitted(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/lol-event-hub/v1/events":
			_, _ = io.WriteString(w, `[{"id":"event-68","eventInfo":{"unclaimedRewardCount":1}}]`)
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/lol-event-hub/v1/events/event-68/reward-track/"):
			_, _ = io.WriteString(w, `[]`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	entries, source := scanEventClaims(context.Background(), client)
	if len(entries) != 0 {
		t.Fatalf("empty event details created phantom claims: %#v", entries)
	}
	if source.Count != 0 || source.State != "available" {
		t.Fatalf("empty event source = %#v, want available with zero claims", source)
	}
}

func TestR68EventRewardStateDiagnosticContainsOnlyDistribution(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/lol-event-hub/v1/events" {
			_, _ = io.WriteString(w, `[{"id":"event-68","unclaimedRewardCount":1}]`)
			return
		}
		_, _ = io.WriteString(w, `[{"state":"unselected","playerName":"private-name","puuid":"private-puuid","reward":{"id":"one","title":"奖品","quantity":1}},{"state":"claimed"}]`)
	}))
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	var diagnostic map[string]any
	_, _ = scanEventClaims(context.Background(), client, func(event map[string]any) { diagnostic = event })
	raw, _ := json.Marshal(diagnostic)
	text := string(raw)
	if !strings.Contains(text, `"unselected":2`) || !strings.Contains(text, `"claimed":2`) {
		t.Fatalf("state distribution = %s", text)
	}
	for _, secret := range []string{"private-name", "private-puuid", "奖品"} {
		if strings.Contains(text, secret) {
			t.Fatalf("state diagnostic leaked %q: %s", secret, text)
		}
	}
}

func TestR68HonorContinuationFiresCanceledPlayAgainExactlyOnce(t *testing.T) {
	playAgain := make(chan struct{}, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/lol-honor/v1/ballot":
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && r.URL.Path == "/lol-lobby/v2/play-again":
			playAgain <- struct{}{}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	diagnostics := make(chan map[string]any, 16)
	runner := newWatchRunner(nil, nil)
	runner.observe = func(event map[string]any) { diagnostics <- event }
	settings := defaultWatchSettings()
	settings.Rules.AutoPlayAgain.Enabled = true
	runner.apply(settings)
	if !runner.schedule(client, "play-again", 5_000, http.MethodPost, "/lol-lobby/v2/play-again", nil) {
		t.Fatal("initial play-again was not armed")
	}
	runner.handleHonor(client, json.RawMessage(`{"eligiblePlayers":[]}`), Summoner{}, watchHonorRule{Enabled: true, Strategy: "abstain"})
	waitForWatchDiagnostic(t, diagnostics, "play-again", "canceled")
	select {
	case <-playAgain:
	case <-time.After(2 * time.Second):
		t.Fatal("deferred play-again was swallowed by markRun")
	}
	select {
	case <-playAgain:
		t.Fatal("play-again fired more than once")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestR68CanceledWatchActionIsRecorded(t *testing.T) {
	diagnostics := make(chan map[string]any, 4)
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "logs"), 0o700); err != nil {
		t.Fatal(err)
	}
	a := &app{storage: trackTestStore(t, &localStore{root: root})}
	runner := newWatchRunner(nil, nil)
	runner.observe = func(event map[string]any) {
		a.recordDiagnostic(event)
		diagnostics <- event
	}
	runner.scheduleMarked(&LCUClient{}, "reconnect", 5_000, http.MethodPost, "/lol-gameflow/v1/reconnect", nil)
	runner.cancelPending("reconnect")
	waitForWatchDiagnostic(t, diagnostics, "reconnect", "canceled")
	logged, err := a.storage.readDiagnosticLog()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, line := range strings.Split(strings.TrimSpace(string(logged)), "\n") {
		var event map[string]any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}
		found = found || event["action"] == "reconnect" && event["event"] == "watch_action" && event["result"] == "canceled"
	}
	if !found {
		t.Fatalf("canceled watch action did not reach disk: %s", logged)
	}
	t.Logf("canceled diagnostic persisted: %s", strings.TrimSpace(string(logged)))
}

func TestR68CanceledOlderWatchJobCannotLoseNewPendingJob(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	diagnostics := make(chan map[string]any, 8)
	runner := newWatchRunner(nil, nil)
	runner.observe = func(event map[string]any) { diagnostics <- event }
	runner.scheduleMarked(client, "same-action", 5_000, http.MethodPost, "/probe", nil)
	runner.scheduleMarked(client, "same-action", 400, http.MethodPost, "/probe", nil)
	waitForWatchDiagnostic(t, diagnostics, "same-action", "canceled")
	time.Sleep(30 * time.Millisecond)
	runner.cancelPending("same-action")
	waitForWatchDiagnostic(t, diagnostics, "same-action", "canceled")
	time.Sleep(450 * time.Millisecond)
	if requests.Load() != 0 {
		t.Fatalf("new pending job escaped cancellation: %d requests", requests.Load())
	}
}

func TestR68AutoMatchmakingPreflightSkipsLostLeaderAndCustom(t *testing.T) {
	for _, test := range []struct {
		name       string
		secondBody string
		wantResult string
	}{
		{name: "lost leader", secondBody: `{"localMember":{"isLeader":false},"members":[{}],"gameConfig":{"isCustom":false}}`, wantResult: "skipped_not_leader"},
		{name: "custom lobby", secondBody: `{"localMember":{"isLeader":true},"members":[{}],"gameConfig":{"isCustom":true,"queueId":420}}`, wantResult: "skipped_custom"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var lobbyReads atomic.Int32
			var searches atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet && r.URL.Path == "/lol-lobby/v2/lobby" {
					if lobbyReads.Add(1) == 1 {
						_, _ = io.WriteString(w, `{"localMember":{"isLeader":true},"members":[{}],"gameConfig":{"isCustom":false,"queueId":420}}`)
					} else {
						_, _ = io.WriteString(w, test.secondBody)
					}
					return
				}
				if r.Method == http.MethodPost && r.URL.Path == "/lol-lobby/v2/lobby/matchmaking/search" {
					searches.Add(1)
					w.WriteHeader(http.StatusNoContent)
					return
				}
				http.NotFound(w, r)
			}))
			defer server.Close()
			client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
			diagnostics := make(chan map[string]any, 8)
			runner := newWatchRunner(nil, nil)
			runner.observe = func(event map[string]any) { diagnostics <- event }
			rules := defaultWatchSettings().Rules
			rules.AutoMatchmaking = watchMatchmakingRule{Enabled: true, DelayMS: 10, MinPartySize: 1}
			runner.handleLobby(client, Summoner{}, rules)
			waitForWatchDiagnostic(t, diagnostics, "auto-matchmaking", test.wantResult)
			time.Sleep(30 * time.Millisecond)
			if searches.Load() != 0 {
				t.Fatalf("preflight still issued %d searches", searches.Load())
			}
		})
	}
}

func TestR68AutoMatchmakingRetriesTwoBadRequestsThenFires(t *testing.T) {
	var searches atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/lol-lobby/v2/lobby":
			_, _ = io.WriteString(w, `{"localMember":{"isLeader":true},"members":[{}],"gameConfig":{"isCustom":false,"queueId":420}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/lol-lobby/v2/lobby/matchmaking/search":
			if searches.Add(1) <= 2 {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = io.WriteString(w, `{"errorCode":"LobbyNotReady","message":"settling"}`)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	diagnostics := make(chan map[string]any, 32)
	runner := newWatchRunner(nil, nil)
	runner.autoMatchRetryDelays = []time.Duration{5 * time.Millisecond, 5 * time.Millisecond, 5 * time.Millisecond}
	runner.observe = func(event map[string]any) { diagnostics <- event }
	rules := defaultWatchSettings().Rules
	rules.AutoMatchmaking = watchMatchmakingRule{Enabled: true, MinPartySize: 1}
	runner.handleLobby(&LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}, Summoner{}, rules)
	retry := waitForWatchDiagnostic(t, diagnostics, "auto-matchmaking", "retrying")
	if retry["attempt"] != 1 {
		t.Fatalf("first retry diagnostic = %#v", retry)
	}
	runner.mu.Lock()
	startedAfter400 := runner.autoMatchStarted
	runner.mu.Unlock()
	if startedAfter400 {
		t.Fatal("autoMatchStarted latched after HTTP 400")
	}
	fired := waitForWatchDiagnostic(t, diagnostics, "auto-matchmaking", "fired")
	if searches.Load() != 3 || fired["attempts"] != 3 {
		t.Fatalf("searches=%d fired=%#v, want third attempt success", searches.Load(), fired)
	}
}

func TestR68AutoMatchmakingGivesUpAndRequiresExplicitRetry(t *testing.T) {
	var searches atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/lol-lobby/v2/lobby":
			_, _ = io.WriteString(w, `{"localMember":{"isLeader":true},"members":[{}],"gameConfig":{"isCustom":false,"queueId":420}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/lol-lobby/v2/lobby/matchmaking/search":
			if searches.Add(1) <= 3 {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = io.WriteString(w, `{"errorCode":"LobbyNotReady"}`)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	diagnostics := make(chan map[string]any, 48)
	runner := newWatchRunner(nil, nil)
	runner.autoMatchRetryDelays = []time.Duration{5 * time.Millisecond, 5 * time.Millisecond, 5 * time.Millisecond}
	runner.observe = func(event map[string]any) { diagnostics <- event }
	rules := defaultWatchSettings().Rules
	rules.AutoMatchmaking = watchMatchmakingRule{Enabled: true, MinPartySize: 1}
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	runner.handleLobby(client, Summoner{}, rules)
	gaveUp := waitForWatchDiagnostic(t, diagnostics, "auto-matchmaking", "gave_up")
	if searches.Load() != 3 || gaveUp["attempts"] != 3 {
		t.Fatalf("searches=%d gave_up=%#v", searches.Load(), gaveUp)
	}
	runner.mu.Lock()
	started := runner.autoMatchStarted
	runner.mu.Unlock()
	if started {
		t.Fatal("autoMatchStarted latched after retry exhaustion")
	}
	// The terminal diagnostic is emitted just before the worker clears its
	// in-flight bit; wait for cleanup, then prove repeated lobby events stop.
	deadline := time.Now().Add(time.Second)
	for {
		runner.mu.Lock()
		inFlight := runner.autoMatchInFlight
		runner.mu.Unlock()
		if !inFlight {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("auto-matchmaking worker did not clear in-flight state")
		}
		time.Sleep(time.Millisecond)
	}
	runner.handleLobby(client, Summoner{}, rules)
	time.Sleep(30 * time.Millisecond)
	if searches.Load() != 3 {
		t.Fatalf("unchanged lobby restarted exhausted retries: %d", searches.Load())
	}
	settings := runner.currentWatch()
	settings.Rules.AutoMatchmaking.Enabled = true
	runner.apply(settings)
	runner.handleLobby(client, Summoner{}, rules)
	waitForWatchDiagnostic(t, diagnostics, "auto-matchmaking", "fired")
	if searches.Load() != 4 {
		t.Fatalf("same lobby did not retry after gave_up: searches=%d", searches.Load())
	}
}

func TestR68AutoMatchmakingBackoffAndQueueReadiness(t *testing.T) {
	runner := newWatchRunner(nil, nil)
	for attempt, want := range []time.Duration{2 * time.Second, 4 * time.Second, 8 * time.Second} {
		if got := runner.autoMatchBackoff(attempt + 1); got != want {
			t.Fatalf("backoff %d = %s, want %s", attempt+1, got, want)
		}
	}
	var searches atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/lol-lobby/v2/lobby" {
			_, _ = io.WriteString(w, `{"localMember":{"isLeader":true},"members":[{}],"gameConfig":{"isCustom":false,"queueId":0}}`)
			return
		}
		if r.Method == http.MethodPost && r.URL.Path == "/lol-lobby/v2/lobby/matchmaking/search" {
			searches.Add(1)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	diagnostics := make(chan map[string]any, 24)
	runner.autoMatchRetryDelays = []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond}
	runner.observe = func(event map[string]any) { diagnostics <- event }
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	if !runner.scheduleAutoMatchmaking(client, 0) {
		t.Fatal("not-ready lobby did not arm its bounded retry job")
	}
	waitForWatchDiagnostic(t, diagnostics, "auto-matchmaking", "gave_up")
	if searches.Load() != 0 {
		t.Fatalf("queueId=0 issued %d matchmaking requests", searches.Load())
	}
}

func TestR68LobbyShapeDiagnosticRecordsOnlyStructuralKeys(t *testing.T) {
	var diagnostic map[string]any
	runner := newWatchRunner(nil, nil)
	runner.observe = func(event map[string]any) { diagnostic = event }
	runner.recordLobbyShape(map[string]any{
		"localMember": map[string]any{"isLeader": true, "summonerName": "PrivateName"},
		"members":     []any{map[string]any{"puuid": "private-puuid"}},
		"gameConfig":  map[string]any{"queueId": float64(420), "isCustom": false},
	})
	raw, _ := json.Marshal(diagnostic)
	text := string(raw)
	for _, expected := range []string{`"event":"lcu_lobby_shape"`, `"top_level_keys"`, `"local_member_keys"`, `"game_config_keys"`, `"summonerName"`, `"queueId"`} {
		if !strings.Contains(text, expected) {
			t.Fatalf("lobby shape missing %s: %s", expected, text)
		}
	}
	for _, secret := range []string{"PrivateName", "private-puuid"} {
		if strings.Contains(text, secret) {
			t.Fatalf("lobby shape leaked %q: %s", secret, text)
		}
	}
}

func TestR68WatchActionLifecycleHasNoArmedOrphans(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/lol-lobby/v2/lobby":
			_, _ = io.WriteString(w, `{"localMember":{"isLeader":false},"gameConfig":{"queueId":420}}`)
		case r.URL.Path == "/failed":
			http.Error(w, "failed", http.StatusInternalServerError)
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer server.Close()
	diagnostics := make(chan map[string]any, 32)
	runner := newWatchRunner(nil, nil)
	runner.observe = func(event map[string]any) { diagnostics <- event }
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	runner.scheduleMarked(client, "success", 0, http.MethodPost, "/ok", nil)
	runner.scheduleMarked(client, "failure", 0, http.MethodPost, "/failed", nil)
	runner.scheduleMarked(client, "cancellation", 5_000, http.MethodPost, "/never", nil)
	runner.cancelPending("cancellation")
	runner.scheduleMarked(client, "auto-matchmaking", 0, http.MethodPost, "/lol-lobby/v2/lobby/matchmaking/search", nil)
	armed, terminal := 0, 0
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	for terminal < 4 {
		select {
		case event := <-diagnostics:
			if event["event"] != "watch_action" {
				continue
			}
			result := fmt.Sprint(event["result"])
			if result == "armed" {
				armed++
			}
			if result == "fired" || strings.HasPrefix(result, "failed") || result == "canceled" || strings.HasPrefix(result, "skipped_") {
				terminal++
			}
		case <-deadline.C:
			t.Fatalf("lifecycle timed out: armed=%d terminal=%d", armed, terminal)
		}
	}
	if armed != terminal || armed != 4 {
		t.Fatalf("orphaned watch actions: armed=%d terminal=%d", armed, terminal)
	}
}

func TestR68PositionBroadcastModesTeamLookupAndRetryGate(t *testing.T) {
	for _, test := range []struct {
		name         string
		queueID      int
		gameflowMode string
		bench        bool
		team         int
		wantResult   string
		wantMessage  bool
	}{
		{name: "aram", queueID: 450, bench: true, team: 1, wantResult: "fired", wantMessage: true},
		{name: "aram mayhem", queueID: 0, gameflowMode: gameModeARAMMayhem, bench: true, team: 2, wantResult: "fired", wantMessage: true},
		{name: "classic", queueID: 420, gameflowMode: "CLASSIC", bench: true, team: 1, wantResult: "skipped_mode"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var messages atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/lol-champ-select/v1/session":
					fmt.Fprintf(w, `{"queueId":%d,"benchEnabled":%t,"localPlayerCellId":7,"myTeam":[{"cellId":7,"team":%d}]}`, test.queueID, test.bench, test.team)
				case "/lol-gameflow/v1/session":
					fmt.Fprintf(w, `{"gameData":{"queue":{"id":%d,"gameMode":%q,"mapId":12}}}`, test.queueID, test.gameflowMode)
				case "/lol-chat/v1/me":
					_, _ = io.WriteString(w, `{"id":"me"}`)
				case "/lol-chat/v1/conversations":
					_, _ = io.WriteString(w, `[{"id":"champ-select","type":"championSelect"}]`)
				case "/lol-chat/v1/conversations/champ-select/messages":
					messages.Add(1)
					w.WriteHeader(http.StatusNoContent)
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
			diagnostics := make(chan map[string]any, 8)
			runner := newWatchRunner(nil, nil)
			var diagnosticStore *localStore
			if test.name == "aram" {
				root := t.TempDir()
				if err := os.MkdirAll(filepath.Join(root, "logs"), 0o700); err != nil {
					t.Fatal(err)
				}
				diagnosticStore = trackTestStore(t, &localStore{root: root})
			}
			runner.observe = func(event map[string]any) {
				if diagnosticStore != nil {
					(&app{storage: diagnosticStore}).recordDiagnostic(event)
				}
				diagnostics <- event
			}
			result := runner.broadcastPosition(client, Summoner{SummonerID: 68}, watchBroadcastRule{Visibility: "self"})
			if result != test.wantResult || (messages.Load() > 0) != test.wantMessage {
				t.Fatalf("broadcast result=%q messages=%d", result, messages.Load())
			}
			waitForWatchDiagnostic(t, diagnostics, "position-broadcast", test.wantResult)
			if diagnosticStore != nil {
				logged, err := diagnosticStore.readDiagnosticLog()
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(string(logged), `"action":"position-broadcast"`) || !strings.Contains(string(logged), `"result":"fired"`) {
					t.Fatalf("position broadcast diagnostic did not reach disk: %s", logged)
				}
				t.Logf("position broadcast diagnostic persisted: %s", strings.TrimSpace(string(logged)))
			}
		})
	}

	var sessionReads atomic.Int32
	message := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/lol-champ-select/v1/session":
			bench := sessionReads.Add(1) > 1
			fmt.Fprintf(w, `{"queueId":450,"benchEnabled":%t,"localPlayerCellId":7,"myTeam":[{"cellId":7,"team":1}]}`, bench)
		case "/lol-chat/v1/me":
			_, _ = io.WriteString(w, `{"id":"me"}`)
		case "/lol-chat/v1/conversations":
			_, _ = io.WriteString(w, `[{"id":"champ-select","type":"championSelect"}]`)
		case "/lol-chat/v1/conversations/champ-select/messages":
			message <- struct{}{}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	diagnostics := make(chan map[string]any, 12)
	runner := newWatchRunner(nil, nil)
	runner.observe = func(event map[string]any) { diagnostics <- event }
	settings := defaultWatchSettings()
	settings.Rules.PositionBroadcast.Enabled = true
	runner.apply(settings)
	runner.handleChampSelect(client, Summoner{})
	waitForWatchDiagnostic(t, diagnostics, "position-broadcast", "skipped_no_bench")
	runner.handleChampSelect(client, Summoner{})
	select {
	case <-message:
	case <-time.After(2 * time.Second):
		t.Fatal("second ready champ-select frame was blocked by first-frame gate")
	}
}

func TestR68LivePositionSnapshotUsesAnonymousKeyAndGameBoundary(t *testing.T) {
	a := &app{}
	secret := strings.Repeat("p", 48)
	players := []gameplayLivePlayer{{gameplayPlayer: gameplayPlayer{PlayerRef: "player_0123456789abcdef", reference: gameplayReference{PlayerRef: secret}}, Position: "bottom"}}
	a.rememberLivePositionSnapshot(68, players)
	current := []gameplayLivePlayer{{gameplayPlayer: gameplayPlayer{PlayerRef: "player_0123456789abcdef"}, Position: "top"}}
	a.applyLivePositionSnapshot(68, current)
	if current[0].Position != "bottom" {
		t.Fatalf("snapshot position = %q", current[0].Position)
	}
	for key := range a.livePositionByRef {
		if key == secret || !strings.HasPrefix(key, "player_") {
			t.Fatalf("snapshot key is not anonymous: %q", key)
		}
	}
	next := []gameplayLivePlayer{{gameplayPlayer: gameplayPlayer{PlayerRef: "player_0123456789abcdef"}, Position: "top"}}
	a.applyLivePositionSnapshot(69, next)
	if next[0].Position != "top" || len(a.livePositionByRef) != 0 {
		t.Fatalf("new game reused stale snapshot: %#v / %#v", next, a.livePositionByRef)
	}
}

func TestR68LiveHandlerCarriesChampSelectPositionIntoSameGame(t *testing.T) {
	phase := "ChampSelect"
	gameID := int64(68)
	puuid := strings.Repeat("q", 48)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/lol-gameflow/v1/gameflow-phase":
			_ = json.NewEncoder(w).Encode(phase)
		case "/lol-gameflow/v1/session":
			_ = json.NewEncoder(w).Encode(map[string]any{"gameData": map[string]any{"gameId": gameID, "queue": map[string]any{"id": 420, "mapId": 11, "gameMode": "CLASSIC"}, "teamOne": []map[string]any{{"puuid": puuid, "selectedPosition": "TOP"}}, "teamTwo": []any{}}, "map": map[string]any{"id": 11, "gameMode": "CLASSIC"}})
		case "/lol-champ-select/v1/session":
			_ = json.NewEncoder(w).Encode(map[string]any{"gameId": gameID, "queueId": 420, "localPlayerCellId": 7, "myTeam": []map[string]any{{"cellId": 7, "puuid": puuid, "assignedPosition": "BOTTOM"}}, "theirTeam": []any{}})
		case "/lol-lobby/v2/lobby":
			_, _ = io.WriteString(w, `{"gameConfig":{"queueId":420,"mapId":11,"gameMode":"CLASSIC"}}`)
		case "/lol-champ-select/v1/current-champion":
			_, _ = io.WriteString(w, `0`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	a := &app{token: "session-secret", connected: true, lcu: client, summoner: Summoner{PUUID: puuid}}
	load := func() gameplayLiveResponse {
		recorder := httptest.NewRecorder()
		a.handleGameplayLive(recorder, httptest.NewRequest(http.MethodGet, "/api/gameplay/live", nil))
		var response gameplayLiveResponse
		if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		return response
	}
	champ := load()
	if len(champ.Players) != 1 || champ.Players[0].Position != "bottom" {
		t.Fatalf("champ-select position = %#v", champ.Players)
	}
	phase = "InProgress"
	inGame := load()
	if len(inGame.Players) != 1 || inGame.Players[0].Position != "bottom" {
		t.Fatalf("same-game position did not use snapshot: %#v", inGame.Players)
	}
	gameID = 69
	newGame := load()
	if len(newGame.Players) != 1 || newGame.Players[0].Position != "top" {
		t.Fatalf("new game reused snapshot: %#v", newGame.Players)
	}
}

func TestR68ChampSelectQueueIDSurvivesLobbyFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/lol-gameflow/v1/gameflow-phase":
			_, _ = io.WriteString(w, `"ChampSelect"`)
		case "/lol-gameflow/v1/session":
			_, _ = io.WriteString(w, `{}`)
		case "/lol-champ-select/v1/session":
			_, _ = io.WriteString(w, `{"gameId":68,"queueId":420,"myTeam":[],"theirTeam":[]}`)
		case "/lol-lobby/v2/lobby":
			http.Error(w, "failed", http.StatusInternalServerError)
		case "/lol-champ-select/v1/current-champion":
			_, _ = io.WriteString(w, `0`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	a := &app{connected: true, lcu: client}
	recorder := httptest.NewRecorder()
	a.handleGameplayLive(recorder, httptest.NewRequest(http.MethodGet, "/api/gameplay/live", nil))
	var response gameplayLiveResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.QueueID != 420 {
		t.Fatalf("queue id = %d, want champ-select 420", response.QueueID)
	}
	found := false
	for _, capability := range response.Capabilities {
		if capability.Name == "lobby" && capability.State == capabilityFailed {
			found = true
		}
	}
	if !found {
		t.Fatalf("lobby failure capability missing: %#v", response.Capabilities)
	}
}

func TestR68LivePositionShapeDistributionAndPrivacy(t *testing.T) {
	secretPUUID := strings.Repeat("u", 48)
	secretName := "Private Player"
	secretSummonerID := "987654321"
	cell := int64(7)
	var session lcuGameflowSession
	session.GameData.TeamOne = []lcuLivePlayer{{PUUID: secretPUUID, SelectedPosition: "TOP", SelectedRole: "", Spell1ID: 11}, {SelectedPosition: "JUNGLE", SelectedRole: "DUO"}}
	session.GameData.TeamTwo = []lcuLivePlayer{{SelectedPosition: "", SelectedRole: ""}}
	champ := lcuChampSelectSession{LocalPlayerCellID: &cell, MyTeam: []lcuChampSelectPlayer{{CellID: &cell, PUUID: secretPUUID, GameName: secretName, SummonerID: 987654321, AssignedPosition: "BOTTOM"}}, PositionSwaps: []map[string]any{{"id": 1, "state": "PENDING"}}}
	response := gameplayLiveResponse{Phase: "ChampSelect", QueueID: 420, GameMode: "CLASSIC", GameID: 68, Players: []gameplayLivePlayer{{Position: "bottom", Spell1ID: 11}, {Position: "jungle"}, {Position: ""}}}
	diagnostic := livePositionShapeDiagnostic(response, session, champ, Summoner{PUUID: secretPUUID}, livePositionSnapshotStats{})
	raw, _ := json.Marshal(diagnostic)
	text := string(raw)
	for _, expected := range []string{`"TOP":1`, `"JUNGLE":1`, `"BOTTOM":1`, `"bottom":1`, `"jungle":1`, `"smite_count":1`, `"self_selected_position":"TOP"`, `"self_assigned_position":"BOTTOM"`} {
		if !strings.Contains(text, expected) {
			t.Fatalf("missing %s in %s", expected, text)
		}
	}
	for _, secret := range []string{secretPUUID, secretName, secretSummonerID} {
		if strings.Contains(text, secret) {
			t.Fatalf("position diagnostic leaked %q: %s", secret, text)
		}
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "logs"), 0o700); err != nil {
		t.Fatal(err)
	}
	a := &app{storage: trackTestStore(t, &localStore{root: root})}
	a.recordDiagnostic(diagnostic)
	logged, err := a.storage.readDiagnosticLog()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(logged), `"event":"live_position_shape"`) {
		t.Fatalf("live_position_shape did not reach disk: %s", logged)
	}
	t.Logf("live position diagnostic persisted: %s", strings.TrimSpace(string(logged)))
}

func TestR68WatchFailureDiagnosticIncludesSafeErrorCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"errorCode":"NotLeader","message":"player private-name cannot search"}`)
	}))
	defer server.Close()
	diagnostics := make(chan map[string]any, 8)
	runner := newWatchRunner(nil, nil)
	runner.observe = func(event map[string]any) { diagnostics <- event }
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	runner.scheduleMarked(client, "probe", 0, http.MethodPost, "/probe", nil)
	event := waitForWatchDiagnostic(t, diagnostics, "probe", "failed")
	if event["error_code"] != "NotLeader" {
		t.Fatalf("error code diagnostic = %#v", event)
	}
	if _, leaked := event["message"]; leaked {
		t.Fatalf("diagnostic persisted message: %#v", event)
	}
}

func TestR68SharedARAMModeSet(t *testing.T) {
	for _, mode := range []string{gameModeARAM, gameModeKIWI, gameModeARAMMayhem, gameModeARAMMayhemClassic} {
		if !isARAMFamilyGameMode(mode) {
			t.Fatalf("ARAM-family mode %q was rejected", mode)
		}
	}
	if isARAMFamilyGameMode("CLASSIC") {
		t.Fatal("CLASSIC was treated as ARAM-family")
	}
}
