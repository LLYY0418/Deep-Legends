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

func TestLCUEventRefreshScopes(t *testing.T) {
	tests := []struct {
		uri  string
		want string
	}{
		{uri: "/lol-champions/v1/inventories/123", want: "collection"},
		{uri: "/lol-champion-mastery/v1/player/123", want: "collection"},
		{uri: "/lol-inventory/v2/inventory/CHAMPION_SKIN", want: "collection"},
		{uri: "/lol-summoner/v1/current-summoner", want: "full"},
		{uri: "/lol-loot/v1/player-loot-map/item", want: "account"},
		{uri: "/lol-rewards/v1/grants/1", want: "account"},
	}
	for _, test := range tests {
		t.Run(test.uri, func(t *testing.T) {
			if got := lcuEventRefreshScope(LCUEvent{URI: test.uri}); got != test.want {
				t.Fatalf("lcuEventRefreshScope(%q) = %q, want %q", test.uri, got, test.want)
			}
		})
	}
}

func TestCollectionDebounceOnlyMarksDirty(t *testing.T) {
	updates := make(chan string, 1)
	a := &app{eventSubscribers: map[chan string]struct{}{updates: {}}}
	accountRefreshes := 0
	fullRefreshes := 0
	ok := a.handleDebouncedRefreshScope(
		nil,
		"collection",
		func(*LCUClient) { accountRefreshes++ },
		func(*LCUClient) bool { fullRefreshes++; return true },
	)
	if !ok {
		t.Fatal("collection scope unexpectedly failed")
	}
	if accountRefreshes != 0 || fullRefreshes != 0 {
		t.Fatalf("collection scope refreshed account=%d full=%d, want both 0", accountRefreshes, fullRefreshes)
	}
	a.mu.RLock()
	dirty, dirtyAt := a.collectionDirty, a.collectionDirtyAt
	a.mu.RUnlock()
	if !dirty || dirtyAt.IsZero() {
		t.Fatalf("collection dirty state = %v at %v", dirty, dirtyAt)
	}
	select {
	case event := <-updates:
		if event != "collection-dirty" {
			t.Fatalf("event = %q, want collection-dirty", event)
		}
	default:
		t.Fatal("collection-dirty event was not broadcast")
	}
}

func TestDebouncedAccountAndCollectionScopesPreserveBothActions(t *testing.T) {
	for _, scopes := range [][]string{{"account", "collection"}, {"collection", "account"}} {
		merged := ""
		for _, scope := range scopes {
			merged = mergeDebouncedRefreshScope(merged, scope)
		}
		if merged != "account+collection" {
			t.Fatalf("merge %v = %q, want account+collection", scopes, merged)
		}
		a := &app{eventSubscribers: make(map[chan string]struct{})}
		accountRefreshes := 0
		fullRefreshes := 0
		if !a.handleDebouncedRefreshScope(
			nil,
			merged,
			func(*LCUClient) { accountRefreshes++ },
			func(*LCUClient) bool { fullRefreshes++; return true },
		) {
			t.Fatalf("merged scope %v unexpectedly failed", scopes)
		}
		if !a.collectionDirty || accountRefreshes != 1 || fullRefreshes != 0 {
			t.Fatalf("merged scope %v: dirty=%v account=%d full=%d", scopes, a.collectionDirty, accountRefreshes, fullRefreshes)
		}
	}
	if got := mergeDebouncedRefreshScope("account+collection", "full"); got != "full" {
		t.Fatalf("full scope must dominate a merged pending scope, got %q", got)
	}
}

func TestSuccessfulSnapshotCommitClearsOnlyExistingCollectionDirtyState(t *testing.T) {
	refreshStartedAt := time.Now()
	a := &app{collectionDirty: true, collectionDirtyAt: refreshStartedAt.Add(-time.Second)}
	a.mu.Lock()
	a.snapshotReady = true
	a.clearCollectionDirtyThroughLocked(refreshStartedAt)
	a.mu.Unlock()
	if a.collectionDirty || !a.collectionDirtyAt.IsZero() {
		t.Fatalf("successful snapshot commit left dirty state: dirty=%v at=%v", a.collectionDirty, a.collectionDirtyAt)
	}

	newEventAt := refreshStartedAt.Add(time.Second)
	a.collectionDirty = true
	a.collectionDirtyAt = newEventAt
	a.mu.Lock()
	a.clearCollectionDirtyThroughLocked(refreshStartedAt)
	a.mu.Unlock()
	if !a.collectionDirty || !a.collectionDirtyAt.Equal(newEventAt) {
		t.Fatalf("refresh cleared a newer collection event: dirty=%v at=%v", a.collectionDirty, a.collectionDirtyAt)
	}
}

func TestRefreshWithClientSuccessClearsCollectionDirty(t *testing.T) {
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
	a := &app{
		collectionDirty:   true,
		collectionDirtyAt: time.Now().Add(-time.Second),
		poolID:            pool.ID,
		pools:             map[string]PoolManifest{pool.ID: pool},
		eventSubscribers:  make(map[chan string]struct{}),
	}
	if alive := a.refreshWithClient(client); !alive {
		t.Fatal("successful fixture was treated as a dead client")
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	if !a.snapshotReady || a.collectionDirty || !a.collectionDirtyAt.IsZero() {
		t.Fatalf("successful refresh state: ready=%v dirty=%v dirtyAt=%v error=%q", a.snapshotReady, a.collectionDirty, a.collectionDirtyAt, a.lastError)
	}
}

func TestStatusReportsCollectionDirty(t *testing.T) {
	a := &app{connected: true, collectionDirty: true}
	recorder := httptest.NewRecorder()
	a.handleStatus(recorder, httptest.NewRequest(http.MethodGet, "/api/status", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status code = %d", recorder.Code)
	}
	var response statusResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if !response.CollectionDirty {
		t.Fatalf("status omitted collectionDirty=true: %s", recorder.Body.String())
	}
}
