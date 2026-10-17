package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestR284ReconnectDelayMigration(t *testing.T) {
	if got := defaultWatchSettings().Rules.AutoReconnect.DelayMS; got != 15000 {
		t.Fatalf("reconnect default=%d, want 15000ms settlement window", got)
	}
	for _, delay := range []int{3000, 9000, 10000, 15000} {
		settings := defaultWatchSettings()
		settings.Rules.AutoReconnect.DelayMS = delay
		if got := normalizeWatchSettings(settings).Rules.AutoReconnect.DelayMS; got != 15000 {
			t.Fatalf("saved delay %d migrated to %d, want 15000", delay, got)
		}
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, convenienceSettingsFile), []byte(`{"rules":{"autoReconnect":{"enabled":true,"delayMs":3000}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	settings := loadWatchSettings(trackTestStore(t, &localStore{root: root}))
	if !settings.Rules.AutoReconnect.Enabled || settings.Rules.AutoReconnect.DelayMS != 15000 {
		t.Fatalf("disk migration=%+v", settings.Rules.AutoReconnect)
	}
}

type r284ReconnectFixture struct {
	runner    *watchRunner
	client    *LCUClient
	now       atomic.Int64
	game      atomic.Int64
	phase     atomic.Value
	signal    atomic.Value
	posts     atomic.Int32
	waiting   chan time.Duration
	release   chan struct{}
	sampled   chan struct{}
	decisions chan map[string]any
}

func r284Reconnect(t *testing.T, signal watchReconnectSignal) *r284ReconnectFixture {
	t.Helper()
	f := &r284ReconnectFixture{waiting: make(chan time.Duration, 8), release: make(chan struct{}, 8), sampled: make(chan struct{}, 8), decisions: make(chan map[string]any, 8)}
	f.game.Store(284)
	f.phase.Store("Reconnect")
	f.signal.Store(signal)
	f.client = &LCUClient{baseURL: "https://fixture", token: "fixture", http: &http.Client{Transport: gameplayRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/lol-gameflow/v1/session":
			return response2351(map[string]any{"gameData": map[string]any{"gameId": f.game.Load()}}), nil
		case "/lol-gameflow/v1/gameflow-phase":
			return response2351(f.phase.Load().(string)), nil
		case "/lol-gameflow/v1/reconnect":
			if req.Method == http.MethodPost {
				f.posts.Add(1)
			}
			return response2351(nil), nil
		}
		return response2351(nil), nil
	})}}
	r := newWatchRunner(nil, nil)
	f.runner = r
	r.now = func() time.Time { return time.UnixMilli(1700000000000 + f.now.Load()) }
	r.wait = func(ctx context.Context, d time.Duration) error {
		f.waiting <- d
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-f.release:
			return nil
		}
	}
	r.reconnectSignals = func(context.Context, *LCUClient) watchReconnectSignal {
		select {
		case f.sampled <- struct{}{}:
		default:
		}
		return f.signal.Load().(watchReconnectSignal)
	}
	r.observe = func(e map[string]any) {
		if e["event"] == "watch_reconnect_decision" {
			f.decisions <- e
		}
	}
	settings := r.currentWatch()
	settings.Rules.AutoReconnect.Enabled = true
	settings.Rules.AutoReconnect.DelayMS = 3000
	r.apply(settings)
	t.Cleanup(func() { r.handlePhase(f.client, "Lobby") })
	return f
}
func (f *r284ReconnectFixture) start(t *testing.T) {
	t.Helper()
	f.phase.Store("Reconnect")
	f.runner.handlePhase(f.client, "Reconnect")
	select {
	case delay := <-f.waiting:
		if delay < 15*time.Second {
			t.Fatalf("reconnect timer=%s, settlement window bypassed", delay)
		}
	case <-time.After(time.Second):
		t.Fatal("reconnect timer missing")
	}
	select {
	case <-f.sampled:
	case <-time.After(time.Second):
		t.Fatal("initial signals missing")
	}
}
func (f *r284ReconnectFixture) decision(t *testing.T, want string) map[string]any {
	t.Helper()
	select {
	case d := <-f.decisions:
		if d["outcome"] != want {
			t.Fatalf("decision=%v want %s", d, want)
		}
		raw, _ := json.Marshal(d)
		for _, secret := range []string{"puuid", "fixture", "gameId", "path"} {
			if strings.Contains(string(raw), secret) {
				t.Fatalf("private reconnect diagnostic=%s", raw)
			}
		}
		return d
	case <-time.After(2 * time.Second):
		t.Fatalf("missing decision %s", want)
	}
	return nil
}
func TestR284ReconnectSettleAndRepeatedNotifications(t *testing.T) {
	f := r284Reconnect(t, watchReconnectSignal{ExitSignal: "unknown"})
	f.start(t)
	f.now.Store(3000)
	f.runner.handlePhase(f.client, "Reconnect")
	select {
	case <-f.waiting:
		t.Fatal("repeated phase restarted timer")
	default:
	}
	f.phase.Store("WaitingForStats")
	f.runner.handlePhase(f.client, "WaitingForStats")
	d := f.decision(t, "canceled-settle")
	if f.posts.Load() != 0 || d["reconnect_ms"] != int64(3000) {
		t.Fatalf("settle request/clock mismatch: %v posts=%d", d, f.posts.Load())
	}
}
func TestR284ReconnectCrashAtFifteenSeconds(t *testing.T) {
	code := int64(1)
	f := r284Reconnect(t, watchReconnectSignal{ExitSignal: "crash", ExitCode: &code, ClientLogParsed: true, CrashHandlerSeen: true})
	f.start(t)
	f.now.Store(3000)
	if f.posts.Load() != 0 {
		t.Fatal("reconnect fired during settlement window")
	}
	f.now.Store(15000)
	f.release <- struct{}{}
	d := f.decision(t, "fired")
	if f.posts.Load() != 1 || d["reconnect_ms"] != int64(15000) || d["exit_signal"] != "crash" || d["effective_delay_ms"] != 15000 {
		t.Fatalf("crash decision=%v posts=%d", d, f.posts.Load())
	}
	f.now.Store(20000)
	if f.posts.Load() != 1 {
		t.Fatal("more than one automatic attempt")
	}
}
func TestR284ReconnectOncePerGameAndManualUnaffected(t *testing.T) {
	f := r284Reconnect(t, watchReconnectSignal{ExitSignal: "unknown"})
	f.start(t)
	f.now.Store(15000)
	f.release <- struct{}{}
	f.decision(t, "fired")
	f.phase.Store("InProgress")
	f.runner.handlePhase(f.client, "InProgress")
	f.start(t)
	f.decision(t, "canceled-once-per-game")
	if f.posts.Load() != 1 {
		t.Fatal("second automatic reconnect in same game")
	}
	if err := f.client.RequestJSON(context.Background(), http.MethodPost, "/lol-gameflow/v1/reconnect", nil, nil); err != nil {
		t.Fatal(err)
	}
	if f.posts.Load() != 2 {
		t.Fatal("manual client reconnect was intercepted")
	}
}
func TestR284ReconnectOnceSurvivesClientRebinding(t *testing.T) {
	f := r284Reconnect(t, watchReconnectSignal{ExitSignal: "unknown"})
	f.client.region, f.client.rsoPlatform = "TENCENT", "HN1"
	f.start(t)
	f.now.Store(15000)
	f.release <- struct{}{}
	f.decision(t, "fired")
	f.runner.handlePhase(f.client, "InProgress")
	f.client = &LCUClient{baseURL: f.client.baseURL, token: "new-session", http: f.client.http}
	f.start(t)
	f.now.Store(30000)
	f.release <- struct{}{}
	f.decision(t, "canceled-once-per-game")
	if f.posts.Load() != 1 {
		t.Fatal("client reconnection reset the same game's automatic attempt")
	}
	if reconnectGameRecorded(map[watchReconnectKey]bool{{scope: "TENCENT|HN1", gameID: 284}: true}, watchReconnectKey{scope: "TENCENT|HN2", gameID: 284}) {
		t.Fatal("different known servers were conflated")
	}
}
func TestR284ReconnectNormalAndGameEnd(t *testing.T) {
	for _, tc := range []struct {
		name    string
		signal  watchReconnectSignal
		outcome string
	}{
		{"normal", watchReconnectSignal{ExitSignal: "normal", ClientLogParsed: true}, "canceled-voluntary"},
		{"game-end", watchReconnectSignal{ExitSignal: "crash", GameEndSeen: true}, "canceled-game-end"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := r284Reconnect(t, tc.signal)
			f.start(t)
			f.decision(t, tc.outcome)
			if f.posts.Load() != 0 {
				t.Fatal("voluntary/end reconnect")
			}
		})
	}
}
func TestR284ReconnectPreflightKeepsGameAndPhase(t *testing.T) {
	for _, kind := range []string{"game-changed", "settled-without-event", "late-normal"} {
		t.Run(kind, func(t *testing.T) {
			f := r284Reconnect(t, watchReconnectSignal{ExitSignal: "unknown"})
			f.start(t)
			want := "canceled-phase-changed"
			switch kind {
			case "game-changed":
				f.game.Store(285)
			case "settled-without-event":
				f.phase.Store("EndOfGame")
				want = "canceled-settle"
			case "late-normal":
				f.signal.Store(watchReconnectSignal{ExitSignal: "normal", ClientLogParsed: true})
				want = "canceled-voluntary"
			}
			f.now.Store(15000)
			f.release <- struct{}{}
			f.decision(t, want)
			if f.posts.Load() != 0 {
				t.Fatal("preflight allowed stale/voluntary game")
			}
		})
	}
}
func TestR284ReconnectExitLogParser(t *testing.T) {
	for _, tc := range []struct {
		text, signal string
		code         *int64
	}{
		{"gameflow: Game exited with code: 0\n", "normal", r284ExitCode(0)},
		{"gameflow: Game process exitCode = -1\n", "crash", r284ExitCode(-1)},
		{"gameflow: League of Legends exited with code 0xC0000005\n", "crash", r284ExitCode(3221225477)},
		{"gameflow: game user closed\n", "normal", nil},
		{"unrelated client exit code: 0\n", "unknown", nil},
		{"gameflow: unexpected format\n", "unknown", nil},
	} {
		signal := parseReconnectExitSignal(tc.text)
		if signal.ExitSignal != tc.signal || !reflect.DeepEqual(signal.ExitCode, tc.code) || signal.ClientLogParsed != (tc.signal != "unknown") {
			t.Fatalf("parse %q=%+v", tc.text, signal)
		}
	}
}
func r284ExitCode(code int64) *int64 { return &code }
func TestR284ReconnectLogTailIgnoresPreviousExit(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "2026_LeagueClient.log")
	if err := os.WriteFile(file, []byte("gameflow: game exited with code: 0\n"), 0600); err != nil {
		t.Fatal(err)
	}
	tail := reconnectLogTail{roots: []string{root}}
	if signal := tail.read(); signal.ClientLogParsed {
		t.Fatal("old game exit reused")
	}
	appendFile, err := os.OpenFile(file, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, err = appendFile.WriteString("gameflow: game exited with code: 1\n")
	appendFile.Close()
	if err != nil {
		t.Fatal(err)
	}
	if signal := tail.read(); signal.ExitSignal != "crash" || signal.ExitCode == nil || *signal.ExitCode != 1 {
		t.Fatalf("new exit not read: %+v", signal)
	}
	if signal := tail.read(); signal.ClientLogParsed {
		t.Fatal("old line consumed twice")
	}
}

func TestR284ReconnectCannotForgetSettlementInSameGame(t *testing.T) {
	f := r284Reconnect(t, watchReconnectSignal{ExitSignal: "unknown"})
	f.start(t)
	f.now.Store(3000)
	f.phase.Store("WaitingForStats")
	f.runner.handlePhase(f.client, "WaitingForStats")
	f.decision(t, "canceled-settle")
	f.start(t)
	f.now.Store(18000)
	f.release <- struct{}{}
	f.decision(t, "canceled-settle")
	if f.posts.Load() != 0 {
		t.Fatal("stale Reconnect erased the same game's settlement")
	}
}

func TestR284ReconnectEventMetadataIncrementalAndPrivate(t *testing.T) {
	client := &http.Client{Transport: gameplayRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != "GET" || req.URL.Scheme != "https" || req.URL.Host != "127.0.0.1:2999" || req.URL.Path != "/liveclientdata/eventdata" || req.URL.Query().Get("eventID") != "4" || req.Header.Get("Authorization") != "" {
			t.Errorf("unsafe event request: method=%s host=%s path=%s query=%s", req.Method, req.URL.Host, req.URL.Path, req.URL.RawQuery)
		}
		return response2351(map[string]any{"Events": []map[string]any{{"EventID": 3, "EventName": "ChampionKill", "EventTime": 100, "KillerName": "private-name"}, {"EventID": 4, "EventName": "GameEnd", "EventTime": 150, "Result": "Win", "PlayerName": "private-name"}, {"EventID": 5, "EventName": "GameClosed", "EventTime": 151}}, "Players": []string{"private-name"}}), nil
	})}
	name, at, next, ended := readReconnectEvents(context.Background(), 4, client)
	if name != "GameClosed" || at != 151 || next != 6 || !ended {
		t.Fatalf("incremental metadata=%s %v %d %v", name, at, next, ended)
	}
	signal := mergeReconnectSignal(watchReconnectSignal{}, watchReconnectSignal{LastEventName: name, LastEventTime: at, GameEndSeen: ended})
	raw, _ := json.Marshal(signal)
	if strings.Contains(string(raw), "private-name") || strings.Contains(string(raw), "Players") {
		t.Fatalf("player data retained: %s", raw)
	}
}
func TestR284ReconnectCrashDetectionUsesNamesOnly(t *testing.T) {
	raw, err := os.ReadFile("process_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	start := strings.Index(source, "func nativeLeagueCrashHandlerSeen()")
	end := strings.Index(source[start:], "\n}") + start
	if start < 0 || end < start {
		t.Fatal("crash snapshot helper missing")
	}
	body := source[start:end]
	if !strings.Contains(body, "nativeProcessSnapshot(false,") || strings.Contains(body, "OpenProcess") || strings.Contains(body, "windowsProcessCommandLine") || strings.Contains(body, "League of Legends.exe") {
		t.Fatal("crash detection must only enumerate handler names without process handles")
	}
}

func TestR284ReconnectRotatedLogDoesNotReplayPreviousGame(t *testing.T) {
	root := t.TempDir()
	oldFile := filepath.Join(root, "1_LeagueClient.log")
	nextFile := filepath.Join(root, "2_LeagueClient.log")
	if err := os.WriteFile(oldFile, []byte("gameflow: game exited with code: 0\n"), 0600); err != nil {
		t.Fatal(err)
	}
	at := time.Unix(1700000000, 0)
	if err := os.Chtimes(oldFile, at, at); err != nil {
		t.Fatal(err)
	}
	tail := reconnectLogTail{roots: []string{root}}
	tail.read()
	if err := os.WriteFile(nextFile, []byte("gameflow: game exited with code: 0\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(nextFile, at.Add(time.Second), at.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if signal := tail.read(); signal.ClientLogParsed {
		t.Fatal("rotation replayed a prior exit as the current game")
	}
	file, err := os.OpenFile(nextFile, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, err = file.WriteString("gameflow: game exited with code: 1\n")
	file.Close()
	if err != nil {
		t.Fatal(err)
	}
	if signal := tail.read(); signal.ExitSignal != "crash" {
		t.Fatalf("new rotated log append lost: %+v", signal)
	}
}
func TestR284ReconnectSignalsCannotCrossUnknownGame(t *testing.T) {
	client := &LCUClient{baseURL: "https://fixture", http: &http.Client{Transport: gameplayRoundTripFunc(func(*http.Request) (*http.Response, error) { return response2351(map[string]any{}), nil })}}
	r := newWatchRunner(nil, nil)
	r.reconnectMonitor = &watchReconnectMonitor{client: client, gameID: 284, signal: watchReconnectSignal{ExitSignal: "normal", GameEndSeen: true}}
	r.reconnectAttempt = &watchReconnectAttempt{client: client, gameID: 285}
	signal := r.readReconnectSignal(context.Background(), client)
	if signal.ExitSignal == "normal" || signal.GameEndSeen {
		t.Fatal("an unknown session read reused the previous game's exit signal")
	}
}

func TestR284ReconnectCrashHandlerMustAppearNearExit(t *testing.T) {
	monitor := &watchReconnectMonitor{gameID: 284}
	at := time.Unix(1700000000, 0)
	monitor.observeCrashHandler(true, at)
	if signal := monitor.snapshotForGame(284, at); signal.ExitSignal == "crash" || signal.CrashHandlerSeen {
		t.Fatal("pre-existing handler attributed to current exit")
	}
	monitor.observeCrashHandler(false, at.Add(time.Second))
	monitor.observeCrashHandler(true, at.Add(2*time.Second))
	if signal := monitor.snapshotForGame(284, at.Add(15*time.Second)); signal.ExitSignal != "crash" || !signal.CrashHandlerSeen {
		t.Fatal("new handler was not observed")
	}
	if signal := monitor.snapshotForGame(284, at.Add(33*time.Second)); signal.ExitSignal == "crash" {
		t.Fatal("old handler leaked into a later reconnect window")
	}
	if signal := monitor.snapshotForGame(285, at.Add(15*time.Second)); signal.CrashHandlerSeen {
		t.Fatal("handler crossed games")
	}
}
