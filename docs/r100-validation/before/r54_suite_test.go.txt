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

func TestR54RetainedSnapshotExposesIdentityButNotAccount(t *testing.T) {
	client := &LCUClient{}
	a := &app{
		connected: true,
		lcu:       client,
		summoner:  Summoner{SummonerID: 42, GameName: "测试玩家", PUUID: "puuid"},
		account:   AccountData{Profile: SummonerProfile{BackgroundSkinID: 164001, BackgroundSkinName: "背景"}},
	}
	a.retainClientAfterSnapshotErrorLocked(client, Snapshot{}, "库存暂不可用")
	if !a.identityReady || a.snapshotReady || a.account.Profile.BackgroundSkinID != 164001 {
		t.Fatalf("retain state lost identity/profile: identityReady=%v snapshotReady=%v profile=%+v", a.identityReady, a.snapshotReady, a.account.Profile)
	}
	status := httptest.NewRecorder()
	a.handleStatus(status, httptest.NewRequest(http.MethodGet, "/api/status", nil))
	var payload statusResponse
	if err := json.Unmarshal(status.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if !payload.IdentityReady || payload.SnapshotReady || payload.Summoner.GameName != "测试玩家" {
		t.Fatalf("unexpected status payload: %+v", payload)
	}
	account := httptest.NewRecorder()
	a.handleAccount(account, httptest.NewRequest(http.MethodGet, "/api/account", nil))
	if account.Code != http.StatusConflict {
		t.Fatalf("account status = %d, want %d", account.Code, http.StatusConflict)
	}
}

func TestR54CollectionProbeRetriesUntilReady(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/lol-champions/v1/inventories/42/skins-minimal" {
			http.NotFound(w, r)
			return
		}
		if calls.Add(1) < 4 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	ready, attempts := waitForCollectionEndpoint(t.Context(), client, 42)
	if !ready || attempts != 4 || calls.Load() != 4 {
		t.Fatalf("probe ready=%v attempts=%d calls=%d", ready, attempts, calls.Load())
	}
}

func TestR54CollectionRefreshRunsProbeBeforeSnapshot(t *testing.T) {
	var probeCalls atomic.Int32
	var refreshCalls atomic.Int32
	a := &app{
		summoner: Summoner{SummonerID: 42},
		collectionProbe: func(context.Context, *LCUClient, int64) (bool, int) {
			probeCalls.Add(1)
			return true, 1
		},
		collectionRefresh: func(*LCUClient) bool {
			refreshCalls.Add(1)
			return true
		},
	}
	if !a.refreshCollectionWithClient(&LCUClient{}) || probeCalls.Load() != 1 || refreshCalls.Load() != 1 {
		t.Fatalf("probe=%d refresh=%d", probeCalls.Load(), refreshCalls.Load())
	}
}

func TestR54AcquisitionLoadersRunInParallel(t *testing.T) {
	started := time.Now()
	runParallelLoaders(func() { time.Sleep(300 * time.Millisecond) }, func() { time.Sleep(300 * time.Millisecond) })
	if elapsed := time.Since(started); elapsed >= 500*time.Millisecond {
		t.Fatalf("acquisition loaders took %s; expected parallel execution", elapsed)
	}
}

func TestR54IdentitySucceedsWhenProfileRequestFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/lol-summoner/v1/current-summoner":
			_ = json.NewEncoder(w).Encode(map[string]any{"summonerId": 42, "puuid": "puuid", "gameName": "新名字"})
		case "/lol-summoner/v1/current-summoner/summoner-profile":
			w.WriteHeader(http.StatusServiceUnavailable)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	a := identityTestApp(client, Summoner{SummonerID: 42, PUUID: "puuid", GameName: "旧名字"})
	if _, err := a.refreshSummonerIdentity(client); err != nil {
		t.Fatalf("profile failure should not fail identity: %v", err)
	}
	if !a.identityReady || a.summoner.GameName != "新名字" {
		t.Fatalf("identity not applied after profile failure: ready=%v summoner=%+v", a.identityReady, a.summoner)
	}
}

func TestR54IdentityRefreshUpdatesProfileWhenAvailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/lol-summoner/v1/current-summoner":
			_ = json.NewEncoder(w).Encode(map[string]any{"summonerId": 42, "puuid": "puuid", "gameName": "玩家"})
		case "/lol-summoner/v1/current-summoner/summoner-profile":
			_ = json.NewEncoder(w).Encode(SummonerProfile{BackgroundSkinID: 164001, BackgroundSkinName: "新背景"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	a := identityTestApp(client, Summoner{SummonerID: 42, PUUID: "puuid", GameName: "旧名字"})
	if _, err := a.refreshSummonerIdentity(client); err != nil {
		t.Fatalf("identity refresh failed: %v", err)
	}
	if got := a.account.Profile.BackgroundSkinID; got != 164001 {
		t.Fatalf("profile background id = %d, want 164001", got)
	}
}

func TestR54IdentityRefreshSkipsCollectionEndpoints(t *testing.T) {
	var collectionCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/lol-summoner/v1/current-summoner":
			_ = json.NewEncoder(w).Encode(map[string]any{"summonerId": 42, "puuid": "puuid", "gameName": "玩家"})
		case "/lol-summoner/v1/current-summoner/summoner-profile":
			_ = json.NewEncoder(w).Encode(SummonerProfile{BackgroundSkinID: 164001})
		case "/lol-champion-mastery/v1/puuid/champion-mastery":
			_ = json.NewEncoder(w).Encode([]any{})
		default:
			if r.URL.Path == "/lol-champions/v1/inventories/42/skins-minimal" || r.URL.Path == "/lol-champions/v1/inventories/42/champions" || r.URL.Path == "/lol-loot/v1/player-loot-map" {
				collectionCalls.Add(1)
			}
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	a := identityTestApp(client, Summoner{})
	if !a.refreshIdentityWithClient(client) || !a.identityReady {
		t.Fatal("identity refresh did not become ready")
	}
	if got := collectionCalls.Load(); got != 0 {
		t.Fatalf("identity refresh made %d collection requests", got)
	}
}

func TestR54InstallationScanUsesCacheUnlessForced(t *testing.T) {
	var scans atomic.Int32
	a := &app{clientInstallations: func() []clientInstallation {
		scans.Add(1)
		return []clientInstallation{{ID: "tcls", Available: true}}
	}}
	a.detectedClientInstallationsWithScanCached(false)
	a.detectedClientInstallationsWithScanCached(false)
	if scans.Load() != 1 {
		t.Fatalf("cached scan count = %d, want 1", scans.Load())
	}
	a.detectedClientInstallationsWithScanCached(true)
	if scans.Load() != 2 {
		t.Fatalf("forced scan count = %d, want 2", scans.Load())
	}
}

func TestR54NoisyDiagnosticsIncludeDiscoveryAndInstallations(t *testing.T) {
	if !isNoisyDiagnosticEvent("client_installations_scan") || !isNoisyDiagnosticEvent("lcu_discovery") {
		t.Fatal("R54 noisy diagnostic events are not deduplicated")
	}
}
