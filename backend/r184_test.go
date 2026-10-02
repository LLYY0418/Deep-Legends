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

type r184Fixture struct {
	a        *app
	client   *LCUClient
	location settingsLocation
	calls    atomic.Int32
	status   atomic.Int32
	phase    string
	ctx      context.Context
	cancel   context.CancelFunc
}

const r184Persisted = `{"account":"private-account","files":[{"name":"Game.cfg","sections":[{"name":"General","settings":[{"name":"CameraLockMode","value":"1"},{"name":"SnapCameraOnRespawn","value":"0"},{"name":"MouseSpeed","value":"private-mouse"},{"name":"CameraPath","value":"/Users/SecretAlice/private-path"},{"name":"CameraAccount","value":"private-account"},{"name":"CameraProfile","value":"SecretAlice"}]}]},{"name":"Input.ini","sections":[{"name":"GameEvents","settings":[{"name":"evtCameraLockToggle","value":"[PRIVATE_BINDING]"},{"name":"evtChat","value":"[PRIVATE_CHAT]"}]}]}]}`

func newR184Fixture(t *testing.T) *r184Fixture {
	t.Helper()
	root := filepath.Join(t.TempDir(), "SecretAlice", "League")
	install := filepath.Join(root, "LeagueClient")
	f := &r184Fixture{a: r175App(t), phase: "Lobby"}
	f.ctx, f.cancel = context.WithCancel(context.Background())
	t.Cleanup(f.cancel)
	for _, dir := range []string{filepath.Join(install, "Config"), filepath.Join(root, "Game", "Config"), filepath.Join(install, "Game", "Config")} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	f.location = settingsLocation{installRoot: install, allowedRoot: root, configRoot: filepath.Join(root, "Game", "Config"), file: filepath.Join(root, "Game", "Config", "PersistedSettings.json")}
	for _, file := range []string{f.location.file, filepath.Join(install, "Config", "PersistedSettings.json")} {
		if err := os.WriteFile(file, []byte(r184Persisted), 0600); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(file, 0600) })
	}
	if err := os.WriteFile(filepath.Join(f.location.configRoot, "game.cfg"), []byte("[General]\nCameraLockMode=1\nSnapCameraOnRespawn=0\nAccount=private-account\nMouseSpeed=private-mouse\n[HUD]\nLockFoo=locked\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.location.configRoot, "input.ini"), []byte("[GameEvents]\nevtCameraLockToggle=[PRIVATE_BINDING]\nevtCameraSnap=[PRIVATE_SNAP]\nevtChat=[PRIVATE_CHAT]\nLockFoo=PRIVATE_LOCK\n"), 0600); err != nil {
		t.Fatal(err)
	}
	f.status.Store(200)
	f.client = &LCUClient{baseURL: "http://lcu.local", token: "test", region: "TENCENT", rsoPlatform: "HN1", platformProbe: true}
	f.client.http = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodGet {
			t.Errorf("watcher wrote LCU settings: %s %s", r.Method, r.URL.Path)
		}
		var value any
		status := 200
		switch r.URL.Path {
		case "/data-store/v1/install-dir":
			value = install
		case "/lol-game-settings/v1/game-settings":
			f.calls.Add(1)
			status = int(f.status.Load())
			value = map[string]any{"General": map[string]any{"CameraLockMode": 1, "SnapCameraOnRespawn": true, "MouseSpeed": "private-mouse"}, "input.ini": map[string]any{"evtCameraLockToggle": "[PRIVATE_BINDING]"}, "AccountId": "private-account"}
		case "/lol-gameflow/v1/gameflow-phase":
			value = f.phase
		case "/riotclient/ux-state":
			value = "Running"
		default:
			status = 404
			value = map[string]any{}
		}
		data, _ := json.Marshal(value)
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(string(data))), Header: make(http.Header)}, nil
	})}
	f.a.gameSettingsWatch.wait = func(ctx context.Context, _ time.Duration) bool { <-ctx.Done(); return false }
	f.a.beginGameSettingsWatch(f.ctx, f.client)
	r184WaitStage(t, f, "app_start", 1)
	return f
}
func r184WaitStage(t *testing.T, f *r184Fixture, stage string, count int) []map[string]any {
	t.Helper()
	until := time.Now().Add(3 * time.Second)
	for time.Now().Before(until) {
		rows := r175Events(t, f.a, "game_settings_watch")
		out := []map[string]any{}
		for _, row := range rows {
			if row["stage"] == stage {
				out = append(out, row)
			}
		}
		if len(out) >= count {
			return out
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("missing stage %s count %d", stage, count)
	return nil
}
func r184Snapshot(t *testing.T, row map[string]any, kind string) map[string]any {
	t.Helper()
	for _, file := range row["files"].([]any) {
		v := file.(map[string]any)
		if v["path_kind"] == kind {
			return v
		}
	}
	t.Fatalf("missing %s", kind)
	return nil
}
func (f *r184Fixture) sample(stage string) {
	s := &f.a.gameSettingsWatch
	s.mu.Lock()
	generation := s.generation
	s.mu.Unlock()
	f.a.recordGameSettingsWatch(f.ctx, f.client, gameSettingsWatchJob{stage: stage, generation: generation})
}
func TestR184CandidatePrivacyAndSettingsFilter(t *testing.T) {
	f := newR184Fixture(t)
	rows := r184WaitStage(t, f, "app_start", 1)
	target := r184Snapshot(t, rows[0], "../Game/Config/PersistedSettings.json")
	other := r184Snapshot(t, rows[0], "Config/PersistedSettings.json")
	if target["exists"] != true || target["is_located_target"] != true || other["exists"] != true || other["is_located_target"] != false {
		t.Fatal(target, other)
	}
	located := 0
	for _, file := range rows[0]["files"].([]any) {
		if file.(map[string]any)["is_located_target"] == true {
			located++
		}
	}
	if located != 1 || len(target["content_hash8"].(string)) != 8 || target["mtime_utc"] == "" {
		t.Fatal(rows)
	}
	values := target["camera_values"].(map[string]any)
	if values["Game.cfg.General.CameraLockMode"] != "1" || values["Game.cfg.General.SnapCameraOnRespawn"] != "0" || values["Input.ini.GameEvents.evtCameraLockToggle"] != "present" || values["Game.cfg.General.CameraPath"] != "[redacted]" || values["Game.cfg.General.CameraProfile"] != "[redacted]" {
		t.Fatal(values)
	}
	input := r184Snapshot(t, rows[0], "../Game/Config/input.ini")["camera_values"].(map[string]any)
	if len(input) != 2 || input["GameEvents.evtCameraSnap"] != "present" {
		t.Fatal(input)
	}
	lcu := rows[0]["lcu_settings"].(map[string]any)
	if lcu["General.CameraLockMode"] != "1" || lcu["General.SnapCameraOnRespawn"] != "true" || lcu["input.ini.evtCameraLockToggle"] != "present" {
		t.Fatal(lcu)
	}
	encoded, _ := json.Marshal(rows)
	for _, secret := range []string{f.location.installRoot, "SecretAlice", "private-account", "private-mouse", "PRIVATE_BINDING", "PRIVATE_CHAT", "PRIVATE_SNAP", "PRIVATE_LOCK"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatal("diagnostic leaked", secret)
		}
	}
	after, _ := os.ReadFile(f.location.file)
	if string(after) != r184Persisted {
		t.Fatal("watcher changed file")
	}
	f.status.Store(404)
	f.sample("champselect")
	latest := r184WaitStage(t, f, "champselect", 1)[0]
	if latest["lcu_settings"] != "unavailable" || latest["lcu_http_status"] != float64(404) {
		t.Fatal(latest)
	}
}
func TestR184ReadOnlyLockActionAndRigDiagnostics(t *testing.T) {
	f := newR184Fixture(t)
	f.a.mu.Lock()
	f.a.connected = true
	f.a.lcu = f.client
	f.a.mu.Unlock()
	// The first operation is attributable even if no prior file snapshot exists.
	f.a.gameSettingsWatch.mu.Lock()
	f.a.gameSettingsWatch.prev = make(map[string]gameSettingsFileSnapshot)
	f.a.gameSettingsWatch.mu.Unlock()
	for _, locked := range []bool{true, false} {
		recorder := httptest.NewRecorder()
		f.a.handleSettingsLock(recorder, httptest.NewRequest(http.MethodPost, "/api/rig/settings-lock", strings.NewReader(fmt.Sprintf(`{"locked":%t}`, locked))))
		if recorder.Code != 200 {
			t.Fatal(recorder.Code, recorder.Body.String())
		}
		count := 1
		if !locked {
			count = 2
		}
		row := r184WaitStage(t, f, "lock_action", count)[count-1]
		snapshot := r184Snapshot(t, row, "../Game/Config/PersistedSettings.json")
		initial := r184Snapshot(t, r184WaitStage(t, f, "app_start", 1)[0], "../Game/Config/PersistedSettings.json")
		if snapshot["read_only"] != locked || snapshot["changed_since_prev"] != false || snapshot["content_hash8"] != initial["content_hash8"] {
			t.Fatal(snapshot)
		}
	}
	changes := r175Events(t, f.a, "game_settings_changed")
	if len(changes) != 2 {
		t.Fatal(changes)
	}
	for _, event := range changes {
		if event["writer_guess"] != "app_lock_action" || len(event["camera_changed_keys"].(map[string]any)) != 0 {
			t.Fatal(event)
		}
	}
	maintenance := r175Events(t, f.a, "rig_maintenance")
	if len(maintenance) != 2 || maintenance[0]["settings_locked"] != true || maintenance[1]["settings_locked"] != false || maintenance[1]["path_kind"] != "../Game/Config/PersistedSettings.json" {
		t.Fatal(maintenance)
	}
	rec := httptest.NewRecorder()
	f.a.handleRigStatus(rec, httptest.NewRequest(http.MethodGet, "/api/rig/status", nil))
	status := r175Events(t, f.a, "rig_status_read")
	if len(status) != 1 || status[0]["settings_locked"] != false || status[0]["path_kind"] != "../Game/Config/PersistedSettings.json" {
		t.Fatal(status)
	}
}
func TestR184ChangeAttributionAndWriteBlocked(t *testing.T) {
	for _, before := range []string{"game_end", "champselect"} {
		t.Run(before, func(t *testing.T) {
			f := newR184Fixture(t)
			f.sample(before)
			changed := strings.Replace(r184Persisted, `"value":"1"`, `"value":"0"`, 1)
			if err := os.WriteFile(f.location.file, []byte(changed), 0600); err != nil {
				t.Fatal(err)
			}
			next, guess := "lobby_after_10s", "game"
			if before == "champselect" {
				next, guess = "game_start", "client_before_launch"
			}
			f.sample(next)
			events := r175Events(t, f.a, "game_settings_changed")
			if len(events) != 1 || events[0]["writer_guess"] != guess || events[0]["between"] != before+" → "+next {
				t.Fatal(events)
			}
			delta := events[0]["camera_changed_keys"].(map[string]any)["Game.cfg.General.CameraLockMode"].(map[string]any)
			if delta["before"] != "1" || delta["after"] != "0" {
				t.Fatal(delta)
			}
			snapshot := r184Snapshot(t, r184WaitStage(t, f, next, 1)[0], "../Game/Config/PersistedSettings.json")
			if snapshot["changed_since_prev"] != true {
				t.Fatal(snapshot)
			}
		})
	}
	t.Run("read-only-no-change", func(t *testing.T) {
		f := newR184Fixture(t)
		if err := os.Chmod(f.location.file, 0444); err != nil {
			t.Fatal(err)
		}
		f.sample("game_end")
		f.sample("lobby_after_10s")
		f.sample("lobby_after_60s")
		events := r175Events(t, f.a, "game_settings_write_blocked_suspected")
		if len(events) != 1 || events[0]["path_kind"] != "../Game/Config/PersistedSettings.json" {
			t.Fatal(events)
		}
	})
	t.Run("changed-and-restored-not-blocked", func(t *testing.T) {
		f := newR184Fixture(t)
		f.sample("game_end")
		changed := strings.Replace(r184Persisted, `"value":"1"`, `"value":"0"`, 1)
		_ = os.WriteFile(f.location.file, []byte(changed), 0600)
		f.sample("lobby_after_10s")
		_ = os.WriteFile(f.location.file, []byte(r184Persisted), 0600)
		_ = os.Chmod(f.location.file, 0444)
		f.sample("lobby_after_60s")
		if len(r175Events(t, f.a, "game_settings_write_blocked_suspected")) != 0 {
			t.Fatal("rewrite wrongly blamed on read-only")
		}
	})
}
func TestR184StageDedupAndConnectionPrime(t *testing.T) {
	f := newR184Fixture(t)
	f.a.beginGameSettingsWatch(f.ctx, f.client)
	f.phase = "ChampSelect"
	f.a.primeGameplayState(f.ctx, f.client)
	r184WaitStage(t, f, "champselect", 1)
	for i := 0; i < 10; i++ {
		f.a.observeGameplayPhase(f.ctx, f.client, "ChampSelect")
	}
	f.a.observeGameplayPhase(f.ctx, f.client, "GameStart")
	for i := 0; i < 10; i++ {
		f.a.observeGameplayPhase(f.ctx, f.client, "InProgress")
	}
	r184WaitStage(t, f, "game_start", 1)
	for _, phase := range []string{"WaitingForStats", "PreEndOfGame", "EndOfGame", "Lobby"} {
		f.a.observeGameplayPhase(f.ctx, f.client, phase)
	}
	r184WaitStage(t, f, "game_end", 1)
	// Serialize a sentinel after all duplicate notifications, without extra phase reads.
	f.sample("lock_action")
	rows := r175Events(t, f.a, "game_settings_watch")
	counts := map[string]int{}
	for _, row := range rows {
		counts[row["stage"].(string)]++
	}
	for _, stage := range []string{"app_start", "champselect", "game_start", "game_end"} {
		if counts[stage] != 1 {
			t.Fatal(counts)
		}
	}
	if f.calls.Load() != 5 {
		t.Fatal("duplicate stage settings reads", f.calls.Load())
	}
	f.a.observeGameSettingsPhase(f.client, "ChampSelect")
	r184WaitStage(t, f, "champselect", 2)
}
func TestR184DelayedStageLifecycle(t *testing.T) {
	f := newR184Fixture(t)
	type gate struct {
		delay   time.Duration
		release chan struct{}
	}
	gates := make(chan gate, 3)
	f.a.gameSettingsWatch.mu.Lock()
	f.a.gameSettingsWatch.wait = func(ctx context.Context, d time.Duration) bool {
		g := gate{d, make(chan struct{})}
		gates <- g
		select {
		case <-ctx.Done():
			return false
		case <-g.release:
			return true
		}
	}
	f.a.gameSettingsWatch.mu.Unlock()
	f.a.observeGameSettingsPhase(f.client, "InProgress")
	inGame := <-gates
	if inGame.delay != 60*time.Second {
		t.Fatal(inGame.delay)
	}
	close(inGame.release)
	r184WaitStage(t, f, "in_game_60s", 1)
	f.a.observeGameSettingsPhase(f.client, "WaitingForStats")
	r184WaitStage(t, f, "game_end", 1)
	first, second := <-gates, <-gates
	if first.delay > second.delay {
		first, second = second, first
	}
	if first.delay != 10*time.Second || second.delay != 60*time.Second {
		t.Fatal(first.delay, second.delay)
	}
	f.a.observeGameSettingsPhase(f.client, "Lobby")
	close(first.release)
	r184WaitStage(t, f, "lobby_after_10s", 1)
	close(second.release)
	r184WaitStage(t, f, "lobby_after_60s", 1)
	// New game invalidates old delayed jobs, while immediate snapshots retain order.
	f.a.observeGameSettingsPhase(f.client, "InProgress")
	stale := <-gates
	f.a.observeGameSettingsPhase(f.client, "ChampSelect")
	close(stale.release)
	r184WaitStage(t, f, "champselect", 1)
	f.sample("lock_action")
	if len(r184WaitStage(t, f, "in_game_60s", 1)) != 1 {
		t.Fatal("old in-game timer survived next game")
	}
}
func TestR184BoundedAndSymlinkReads(t *testing.T) {
	f := newR184Fixture(t)
	outside := filepath.Join(t.TempDir(), "secret.json")
	_ = os.WriteFile(outside, []byte(`{"CameraSecret":"private-outside"}`), 0600)
	candidate := filepath.Join(f.location.installRoot, "Game", "Config", "PersistedSettings.json")
	if _, err := os.Lstat(candidate); !os.IsNotExist(err) {
		t.Fatal("symlink fixture target already exists", err)
	}
	if err := os.Symlink(outside, candidate); err != nil {
		t.Skip(err)
	}
	f.sample("champselect")
	row := r184WaitStage(t, f, "champselect", 1)[0]
	if got := r184Snapshot(t, row, "Game/Config/PersistedSettings.json"); got["result"] != "unsafe-or-unreadable" || got["content_hash8"] != "" {
		t.Fatal(got)
	}
	_ = os.Remove(candidate)
	_ = os.WriteFile(candidate, []byte(strings.Repeat("x", 256<<10+1)), 0600)
	f.sample("game_start")
	if got := r184Snapshot(t, r184WaitStage(t, f, "game_start", 1)[0], "Game/Config/PersistedSettings.json"); got["result"] != "too-large" || got["content_hash8"] != "" {
		t.Fatal(got)
	}
	encoded, _ := json.Marshal(r175Events(t, f.a, "game_settings_watch"))
	if strings.Contains(string(encoded), "private-outside") {
		t.Fatal("unsafe file read")
	}
}

func TestR184WatchOwnershipAndOutsideInstallation(t *testing.T) {
	f := newR184Fixture(t)
	s := &f.a.gameSettingsWatch
	s.mu.Lock()
	connectedCtx := s.ctx
	s.mu.Unlock()
	// A fallback lock observation cannot displace an active connection watcher.
	f.a.startGameSettingsWatch(context.Background(), f.client, true)
	s.mu.Lock()
	same := s.ctx == connectedCtx && !s.standalone
	s.mu.Unlock()
	if !same {
		t.Fatal("standalone fallback displaced connected lifetime")
	}
	f.cancel()
	f.a.startGameSettingsWatch(context.Background(), f.client, true)
	r184WaitStage(t, f, "app_start", 2)
	s.mu.Lock()
	fallbackCtx := s.ctx
	s.mu.Unlock()
	newCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	f.a.beginGameSettingsWatch(newCtx, f.client)
	r184WaitStage(t, f, "app_start", 3)
	if fallbackCtx.Err() == nil {
		t.Fatal("connected watcher did not cancel standalone lifetime")
	}
	cancel()
	s.mu.Lock()
	activeCtx := s.ctx
	s.mu.Unlock()
	if activeCtx.Err() == nil {
		t.Fatal("connection cancellation not propagated")
	}
	// Non-Tencent install roots do not authorize reads of sibling Game files.
	location := f.location
	location.allowedRoot = location.installRoot
	location.configRoot = filepath.Join(location.installRoot, "Config")
	location.file = filepath.Join(location.configRoot, "PersistedSettings.json")
	for _, file := range readGameSettingsWatchFiles(location, "app_start") {
		if file.PathKind == "../Game/Config/PersistedSettings.json" && (file.Result != "unsafe-or-unreadable" || file.Hash != "") {
			t.Fatal(file)
		}
	}
}
