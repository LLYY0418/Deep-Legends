package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestWatchSettingsMigrateLegacyAndDefaultNewRulesOff(t *testing.T) {
	root := t.TempDir()
	store := &localStore{root: root}
	if err := os.WriteFile(filepath.Join(root, convenienceSettingsFile), []byte(`{"autoAccept":true,"autoPlayAgain":false,"autoReconnect":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	settings := loadWatchSettings(store)
	if settings.SchemaVersion != watchSettingsVersion || !settings.MasterEnabled {
		t.Fatalf("migration metadata = %#v", settings)
	}
	if !settings.Rules.AutoAccept.Enabled || settings.Rules.AutoPlayAgain.Enabled || !settings.Rules.AutoReconnect.Enabled {
		t.Fatalf("legacy fields were not migrated: %#v", settings.Rules)
	}
	for name, enabled := range map[string]bool{
		"autoHonor": settings.Rules.AutoHonor.Enabled, "skipCelebration": settings.Rules.SkipCelebration.Enabled,
		"positionBroadcast": settings.Rules.PositionBroadcast.Enabled, "promoteLeader": settings.Rules.PromoteLeader.Enabled,
		"invitations": settings.Rules.Invitations.Enabled, "autoMatchmaking": settings.Rules.AutoMatchmaking.Enabled,
	} {
		if enabled {
			t.Errorf("new rule %s unexpectedly enabled after migration", name)
		}
	}
}

func TestWatchSettingsNormalizeEnumsAndSelectionBounds(t *testing.T) {
	settings := defaultWatchSettings()
	settings.Rules.AutoHonor.Strategy = "enemy"
	settings.Rules.PositionBroadcast.Visibility = "all"
	settings.Rules.Invitations.Policies = map[string]string{"420": "accept", "440": "random"}
	settings.Rules.AutoMatchmaking.MinPartySize = 99
	normalized := normalizeWatchSettings(settings)
	if normalized.Rules.AutoHonor.Strategy != "prefer-party" || normalized.Rules.PositionBroadcast.Visibility != "self" {
		t.Fatalf("enum fallback = %#v", normalized.Rules)
	}
	if normalized.Rules.AutoMatchmaking.MinPartySize != 5 || normalized.Rules.Invitations.Policies["420"] != "accept" {
		t.Fatalf("bounds/policies = %#v", normalized.Rules)
	}
	if _, ok := normalized.Rules.Invitations.Policies["440"]; ok {
		t.Fatal("invalid invitation policy survived normalization")
	}
}

func TestWatchApplyCancelsPendingActionsWhenDisabled(t *testing.T) {
	runner := newWatchRunner(nil, nil)
	settings := defaultWatchSettings()
	settings.Rules.AutoPlayAgain.Enabled = true
	runner.apply(settings)
	ctx, cancel := context.WithCancel(context.Background())
	runner.mu.Lock()
	runner.pending["play-again"] = cancel
	runner.mu.Unlock()
	settings.MasterEnabled = false
	runner.apply(settings)
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("pending play-again was not canceled")
	}
	runner.mu.Lock()
	_, stillPending := runner.pending["play-again"]
	runner.mu.Unlock()
	if stillPending {
		t.Fatal("canceled action remained in pending map")
	}
}

func TestWatchReadFailuresBroadcastAndDoNotWrite(t *testing.T) {
	var writes int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			atomic.AddInt32(&writes, 1)
		}
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()
	events := make(chan string, 8)
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	runner := newWatchRunner(nil, func(event string) { events <- event })
	rules := defaultWatchSettings().Rules
	rules.PromoteLeader.Enabled = true
	rules.AutoMatchmaking.Enabled = true
	runner.handleLobby(client, Summoner{SummonerID: 1}, rules)
	seen := map[string]bool{<-events: true, <-events: true}
	if !seen["watch:failed:promote-leader"] || !seen["watch:failed:auto-matchmaking"] {
		t.Fatalf("lobby failure events = %#v", seen)
	}
	runner.handleHonor(client, json.RawMessage(`{"eligiblePlayers":[{"puuid":"ally","isAlly":true}]}`), Summoner{PUUID: "self"}, watchHonorRule{Enabled: true, Strategy: "any-teammate"})
	if event := <-events; event != "watch:priority:honoring" {
		t.Fatalf("honor priority event = %q", event)
	}
	if event := <-events; event != "watch:failed:auto-honor" {
		t.Fatalf("honor failure event = %q", event)
	}
	if atomic.LoadInt32(&writes) != 0 {
		t.Fatalf("read failure still issued %d writes", writes)
	}
}

func TestSettingsLockUsesTencentConfigAndRejectsSymlink(t *testing.T) {
	base := t.TempDir()
	installRoot := filepath.Join(base, "LeagueClient")
	settingsFile := filepath.Join(base, "Game", "Config", "PersistedSettings.json")
	if err := os.MkdirAll(filepath.Dir(settingsFile), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settingsFile, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/data-store/v1/install-dir":
			_ = json.NewEncoder(w).Encode(installRoot)
		case "/riotclient/ux-state":
			_ = json.NewEncoder(w).Encode("Running")
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client(), region: "TENCENT", platformProbe: true}
	a := &app{connected: true, lcu: client}

	recorder := httptest.NewRecorder()
	a.handleSettingsLock(recorder, httptest.NewRequest(http.MethodPost, "/api/rig/settings-lock", strings.NewReader(`{"locked":true}`)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("lock status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	info, err := os.Stat(settingsFile)
	if err != nil || info.Mode().Perm()&0o222 != 0 {
		t.Fatalf("settings mode = %v, err = %v", info.Mode().Perm(), err)
	}

	if err := os.Remove(settingsFile); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "outside.json")
	if err := os.WriteFile(target, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, settingsFile); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	recorder = httptest.NewRecorder()
	a.handleSettingsLock(recorder, httptest.NewRequest(http.MethodPost, "/api/rig/settings-lock", strings.NewReader(`{"locked":false}`)))
	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("symlink status = %d, want 422", recorder.Code)
	}
}

func TestClientMaintenanceUsesLeadingSlashAndRejectsUnknownAction(t *testing.T) {
	paths := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths <- r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	a := &app{connected: true, lcu: client}
	recorder := httptest.NewRecorder()
	a.handleClientMaintenance(recorder, httptest.NewRequest(http.MethodPost, "/api/rig/maintenance", strings.NewReader(`{"action":"restart-ux"}`)))
	if recorder.Code != http.StatusOK || <-paths != "/riotclient/kill-and-restart-ux" {
		t.Fatalf("restart response = %d %q", recorder.Code, recorder.Body.String())
	}
	recorder = httptest.NewRecorder()
	a.handleClientMaintenance(recorder, httptest.NewRequest(http.MethodPost, "/api/rig/maintenance", strings.NewReader(`{"action":"unknown"}`)))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("unknown action status = %d", recorder.Code)
	}
}

func TestFacadeRankAndLoginResetValidation(t *testing.T) {
	payload, err := facadeRankPayload("RANKED_SOLO_5X5", "MASTER", "IV")
	if err != nil || payload["rankedLeagueDivision"] != nil {
		t.Fatalf("master payload = %#v, err = %v", payload, err)
	}
	if _, err := facadeRankPayload("RANKED_SOLO_5X5", "DIAMOND", "V"); err == nil {
		t.Fatal("invalid division was accepted")
	}
	store := &localStore{root: t.TempDir()}
	runner := newWatchRunner(store, nil)
	a := &app{storage: store, watch: runner}
	reset := facadeLoginResetSettings{
		StatusMessageEnabled: true, StatusMessage: "重设签名", RankEnabled: true,
		Rank: map[string]any{"rankedLeagueQueue": "RANKED_SOLO_5X5", "rankedLeagueTier": "DIAMOND", "rankedLeagueDivision": "II"},
	}
	if err := a.applyFacadeAction(context.Background(), nil, Summoner{}, facadeApplyRequest{Action: "login-reset", LoginReset: &reset}); err != nil {
		t.Fatal(err)
	}
	got := runner.currentWatch().Facade
	if !got.StatusMessageEnabled || !got.RankEnabled || got.StatusMessage != "重设签名" {
		t.Fatalf("saved reset = %#v", got)
	}
}

func TestFacadeBackgroundWritesOnlySkinID(t *testing.T) {
	bodies := make(chan map[string]any, 1)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		bodies <- body
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	a := &app{allSkins: []Skin{{ID: 99001}}}
	if err := a.applyFacadeAction(context.Background(), client, Summoner{}, facadeApplyRequest{Action: "background", SkinID: 99001}); err != nil {
		t.Fatal(err)
	}
	body := <-bodies
	if body["key"] != "backgroundSkinId" || body["value"] != float64(99001) {
		t.Fatalf("background request = %#v", body)
	}
	if calls.Load() != 1 {
		t.Fatalf("background request count = %d, want 1", calls.Load())
	}
}

func TestClaimExecuteContinuesAfterLCUFailureAndParsesBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/lol-rewards/v1/grants":
			_, _ = w.Write([]byte(`[{"info":{"id":"fail","status":"PENDING"},"rewardGroup":{"id":"group-fail","localizations":{"title":"失败奖励"},"rewards":[{"id":"reward-fail","itemId":"1","quantity":1}]}},{"info":{"id":"ok","status":"PENDING"},"rewardGroup":{"id":"group-ok","localizations":{"title":"成功奖励"},"rewards":[{"id":"reward-ok","itemId":"2","quantity":1}]}}]`))
		case r.Method == http.MethodGet && (r.URL.Path == "/lol-missions/v1/missions" || r.URL.Path == "/lol-event-hub/v1/events"):
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodPost && r.URL.Path == "/lol-rewards/v1/grants/fail/select":
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"errorCode":"RewardGrantAlreadyFulfilled","message":"already handled"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/lol-rewards/v1/grants/ok/select":
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	a := &app{connected: true, lcu: client}
	recorder := httptest.NewRecorder()
	a.handleClaimExecute(recorder, httptest.NewRequest(http.MethodPost, "/api/claim/execute", strings.NewReader(`{"items":[{"key":"grant:fail"},{"key":"grant:ok"}]}`)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var response claimExecuteResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Succeeded != 1 || response.Failed != 1 || len(response.Results) != 2 {
		t.Fatalf("response = %#v", response)
	}
	failed := response.Results[0]
	if failed.StatusCode != 400 || failed.ErrorCode != "RewardGrantAlreadyFulfilled" || failed.Message != "already handled" || !strings.Contains(failed.Consequence, "其它条目") {
		t.Fatalf("parsed error = %#v", failed)
	}
	if !response.Results[1].OK {
		t.Fatalf("second item did not continue: %#v", response.Results[1])
	}
}

func TestEventClaimDetailConcurrencyIsLimitedToFour(t *testing.T) {
	var current int32
	var maximum int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/lol-event-hub/v1/events" {
			_, _ = w.Write([]byte(`[{"id":"e1","unclaimedRewardCount":1},{"id":"e2","unclaimedRewardCount":1},{"id":"e3","unclaimedRewardCount":1},{"id":"e4","unclaimedRewardCount":1},{"id":"e5","unclaimedRewardCount":1},{"id":"e6","unclaimedRewardCount":1},{"id":"e7","unclaimedRewardCount":1},{"id":"e8","unclaimedRewardCount":1}]`))
			return
		}
		active := atomic.AddInt32(&current, 1)
		for {
			seen := atomic.LoadInt32(&maximum)
			if active <= seen || atomic.CompareAndSwapInt32(&maximum, seen, active) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		atomic.AddInt32(&current, -1)
		_, _ = w.Write([]byte(`[]`))
	}))
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	entries, source := scanEventClaims(context.Background(), client)
	if len(entries) != 8 || source.State != "available" {
		t.Fatalf("entries=%d source=%#v", len(entries), source)
	}
	if maximum > 4 {
		t.Fatalf("event detail concurrency = %d, want <= 4", maximum)
	}
}

func TestEventClaimDetailsAllFailedAreNotReportedAvailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/lol-event-hub/v1/events" {
			_, _ = w.Write([]byte(`[{"id":"event-one","unclaimedRewardCount":1}]`))
			return
		}
		http.Error(w, `{"errorCode":"Unavailable","message":"detail failed"}`, http.StatusBadGateway)
	}))
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	entries, source := scanEventClaims(context.Background(), client)
	if len(entries) != 0 || source.State == "available" {
		t.Fatalf("entries=%#v source=%#v", entries, source)
	}
}

func TestClaimCenterHasNoPersistenceOrDiagnosticWritePath(t *testing.T) {
	data, err := os.ReadFile("claim_center.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	for _, forbidden := range []string{"recordDiagnostic", "appendDiagnostic", "atomicWriteFile", "os.WriteFile", "writeLocalStoreFile"} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("claim center contains forbidden persistence path %q", forbidden)
		}
	}
}
