package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestR206StartupPrefetchIdleAndGameBoundary(t *testing.T) {
	client := &LCUClient{}
	for _, phase := range []string{"None", "Lobby", "ChampSelect", "GameStart", "InProgress", "Reconnect", "WaitingForStats", "PreEndOfGame", "EndOfGame", ""} {
		a := &app{lcu: client, connected: true, identityReady: true, refreshRequests: make(chan struct{}, 2)}
		a.gameplayFlow.client, a.gameplayFlow.phase = client, phase
		now := time.Now()
		a.maybeStartupPrefetch(client, now)
		if a.maybeStartupPrefetch(client, now.Add(9*time.Second)) {
			t.Fatal("prefetch before ten seconds")
		}
		got := a.maybeStartupPrefetch(client, now.Add(10*time.Second))
		if got != (phase == "None" || phase == "Lobby") {
			t.Fatal("wrong phase", phase, got)
		}
		if got && (a.collectionRefreshPendingSource != "startup_prefetch" || len(a.refreshRequests) != 1) {
			t.Fatal("missing startup source")
		}
		if a.maybeStartupPrefetch(client, now.Add(time.Minute)) {
			t.Fatal("prefetch repeated")
		}
	}
}

func TestR206CollectionEventsLogEveryEventAndThrottle(t *testing.T) {
	a := &app{storage: newFlowDiagnosticStore(t), refreshRequests: make(chan struct{}, 10)}
	now := time.Now()
	for i := 0; i < 5; i++ {
		if a.markCollectionDirtyAt(now.Add(time.Duration(i)*2*time.Second), "loot") {
			a.queueCollectionRefresh("event")
			<-a.refreshRequests
			a.collectionRefreshPending = false
		}
	}
	data, err := a.storage.readDiagnosticLogForExport()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(data), `"event":"collection_dirty_marked"`) != 5 || strings.Count(string(data), `"event":"collection_refresh_request"`) != 1 {
		t.Fatal("event diagnostics/throttle", string(data))
	}
	a.collectionRefreshFinishedAt = now.Add(30 * time.Second)
	if a.markCollectionDirtyAt(now.Add(32*time.Second), "champions") {
		t.Fatal("post-refresh suppression missing")
	}
}

func TestR206CollectionRetryDoesNotReadIdentity(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; http.NotFound(w, r) }))
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "fixture", http: server.Client()}
	a := &app{lcu: client, connected: true, refreshRequests: make(chan struct{}, 1)}
	w := httptest.NewRecorder()
	a.handleRefresh(w, httptest.NewRequest("POST", "/api/refresh?source=collection_retry", nil))
	if w.Code != 202 || calls != 0 || a.collectionRefreshPendingSource != "collection_retry" {
		t.Fatal("retry read identity/full scope", w.Code, calls)
	}
}

func TestR206LocalImage404NeverFetchesRemote(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "fixture", http: server.Client()}
	provider := newChampionProvider()
	remoteCalls := 0
	provider.client = &http.Client{Transport: r196RoundTrip(func(r *http.Request) (*http.Response, error) {
		remoteCalls++
		return updateResponse(200, []byte("\x89PNG\r\n\x1a\nfixture")), nil
	})}
	a := &app{lcu: client, connected: true, champions: provider}
	path := "/lol-game-data/assets/v1/profile-icons/123456.jpg"
	w := httptest.NewRecorder()
	a.handleImage(w, httptest.NewRequest("GET", "/api/image?path="+url.QueryEscape(path), nil))
	if w.Code != 404 || w.Header().Get("X-Image-Fallback") != "communitydragon" || remoteCalls != 0 {
		t.Fatal("local lane leaked remote", w.Code, w.Header(), remoteCalls)
	}
	w = httptest.NewRecorder()
	a.handleImage(w, httptest.NewRequest("GET", "/api/image?path="+url.QueryEscape(path)+"&source=communitydragon", nil))
	if w.Code != 200 || remoteCalls != 1 {
		t.Fatal("explicit remote candidate", w.Code, remoteCalls)
	}
	a.flushImageProxyCosts()
}

func TestR206TranslationsTimeoutUsesPreviousCache(t *testing.T) {
	t.Parallel()
	provider := newChampionProvider()
	provider.lootTranslations = map[string]lootMetadata{"CHEST_FIXTURE": {Name: "缓存名称"}}
	provider.client = &http.Client{Transport: r196RoundTrip(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "trans.json") {
			<-r.Context().Done()
			return nil, r.Context().Err()
		}
		return updateResponse(404, []byte(`{}`)), nil
	})}
	started := time.Now()
	metadata := loadLootMetadata(context.Background(), nil, provider, nil)
	if time.Since(started) > 3*time.Second || metadata["CHEST_FIXTURE"].Name != "缓存名称" {
		t.Fatal("translations blocked or lost cache", time.Since(started), metadata)
	}
}

func TestR206FullCollectionRefreshTranslationTimeoutUnderThreeSeconds(t *testing.T) {
	t.Parallel()
	type catalogEntry struct {
		ID         int64  `json:"id"`
		Name       string `json:"name"`
		ChampionID int64  `json:"championId"`
		Owned      bool   `json:"owned"`
	}
	catalog := make([]catalogEntry, 0, 1000)
	champions := make([]map[string]any, 0, 100)
	const targetID int64 = 1001
	targetName := "测试奖池皮肤"
	for championID := int64(1); championID <= 100; championID++ {
		champions = append(champions, map[string]any{"id": championID, "owned": true})
		for offset := int64(0); offset < 10; offset++ {
			id := championID*1000 + offset
			name := fmt.Sprintf("测试皮肤 %d-%d", championID, offset)
			if id == targetID {
				name = targetName
			}
			catalog = append(catalog, catalogEntry{ID: id, Name: name, ChampionID: championID, Owned: id == targetID})
		}
	}
	catalogJSON, err := json.Marshal(catalog)
	if err != nil {
		t.Fatal(err)
	}
	championsJSON, err := json.Marshal(champions)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/lol-summoner/v1/current-summoner":
			_, _ = w.Write([]byte(`{"summonerId":1,"puuid":"test-puuid","gameName":"测试玩家"}`))
		case r.URL.Path == "/lol-game-data/assets/v1/skins.json":
			_, _ = w.Write(catalogJSON)
		case strings.HasSuffix(r.URL.Path, "/skins-minimal"):
			_, _ = w.Write(catalogJSON)
		case strings.HasSuffix(r.URL.Path, "/champions-minimal"):
			_, _ = w.Write(championsJSON)
		case strings.HasPrefix(r.URL.Path, "/lol-champion-mastery/v1/"):
			_, _ = w.Write([]byte(`[]`))
		case r.URL.Path == "/lol-summoner/v1/current-summoner/summoner-profile":
			_, _ = w.Write([]byte(`{}`))
		case r.URL.Path == "/lol-loot/v1/player-loot-map":
			_, _ = w.Write([]byte(`{}`))
		case r.URL.Path == "/lol-inventory/v1/wallet/lol_blessing_token":
			_, _ = w.Write([]byte(`0`))
		case r.URL.Path == "/lol-rewards/v1/grants":
			_, _ = w.Write([]byte(`[]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	pool := PoolManifest{ID: "test", Names: []string{targetName}, Entries: []PoolEntry{{ID: targetID, Name: targetName}}, Hash: "test-hash"}
	provider := newChampionProvider()
	provider.lootTranslations = map[string]lootMetadata{"CHEST_FIXTURE": {Name: "缓存名称"}}
	provider.client = &http.Client{Transport: r196RoundTrip(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "trans.json") {
			<-r.Context().Done()
			return nil, r.Context().Err()
		}
		return updateResponse(404, []byte(`{}`)), nil
	})}
	started := time.Now()
	a := &app{
		champions:         provider,
		collectionDirty:   true,
		collectionDirtyAt: time.Now().Add(-time.Second),
		poolID:            pool.ID,
		pools:             map[string]PoolManifest{pool.ID: pool},
		eventSubscribers:  make(map[chan string]struct{}),
	}
	if alive := a.refreshWithClient(client); !alive {
		t.Fatal("successful fixture was treated as a dead client")
	}
	if elapsed := time.Since(started); elapsed > 3*time.Second {
		t.Fatalf("full refresh blocked %s", elapsed)
	}
	if provider.lootTranslations["CHEST_FIXTURE"].Name != "缓存名称" {
		t.Fatal("previous translation cache lost")
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	if !a.snapshotReady || a.collectionDirty || !a.collectionDirtyAt.IsZero() {
		t.Fatalf("successful refresh state: ready=%v dirty=%v dirtyAt=%v error=%q", a.snapshotReady, a.collectionDirty, a.collectionDirtyAt, a.lastError)
	}
}

func TestR206DisconnectResetsCollectionReadLifecycle(t *testing.T) {
	a := &app{collectionRefreshStartedAt: time.Now().Add(-time.Minute), collectionRefreshFinishedAt: time.Now(), collectionRefreshPendingSource: "event", collectionEventAt: map[string]time.Time{"loot": time.Now()}, collectionDirty: true}
	a.clearSnapshotLocked("")
	if !a.collectionRefreshStartedAt.IsZero() || !a.collectionRefreshFinishedAt.IsZero() || a.collectionRefreshPendingSource != "" || len(a.collectionEventAt) != 0 || a.collectionDirty {
		t.Fatal("stale collection read state survived disconnect")
	}
}
