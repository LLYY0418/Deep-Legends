package main

import (
	"context"
	"encoding/json"
	"errors"
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

var lootNames0912 = map[string]string{
	"CHEST_128": "英雄魔法引擎", "MATERIAL_clashtickets": "冠军杯赛挑战券",
	"WARD_SKIN_RENTAL_12": "光辉守卫", "WARD_SKIN_RENTAL_23": "金球守卫",
	"WARD_SKIN_RENTAL_44": "光学增强器 守卫", "WARD_SKIN_RENTAL_63": "星之守护者 守卫",
	"SUMMONER_ICON_782": "心形爆破",
}

func lootNamingFixtureItems() map[string]LootItem {
	items := make(map[string]LootItem)
	for id := range lootNames0912 {
		kind := "CHEST"
		switch {
		case strings.HasPrefix(id, "MATERIAL_"):
			kind = "MATERIAL"
		case strings.HasPrefix(id, "WARD_"):
			kind = "WARDSKIN_RENTAL"
		case strings.HasPrefix(id, "SUMMONER_"):
			kind = "SUMMONERICON"
		}
		items[id] = LootItem{LootID: id, LootName: id, LocalizedName: "未命名战利品", Count: 1, Type: kind, StoreItemID: 999999}
	}
	return items
}

func lootNamingCatalogFixture(t *testing.T, name string) []byte {
	t.Helper()
	if name == "loot.json" {
		// Current loot.json omits all seven IDs, but is otherwise a valid catalog.
		return []byte(`{"LootItems":[{"id":"CHEST_224","name":"杰作宝箱"}]}`)
	}
	data, err := os.ReadFile(filepath.Join("testdata", "loot-names-0912", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func assertLootNamingFixture(t *testing.T, items []LootItem) {
	t.Helper()
	if len(items) != len(lootNames0912) {
		t.Fatalf("items=%d, want all seven real inventory entries", len(items))
	}
	seen := make(map[string]bool)
	for _, item := range items {
		want, ok := lootNames0912[item.LootID]
		if !ok || seen[item.LootID] || item.DisplayName != want || item.Count != 1 || item.Asset == "" {
			t.Fatalf("incorrect loot name/artwork/identity/quantity: %#v, want %q", item, want)
		}
		seen[item.LootID] = true
		if strings.HasPrefix(item.LootID, "WARD_") && (item.Category != "守卫" || item.SkinID != 0) {
			t.Fatalf("ward was mistaken for a champion skin: %#v", item)
		}
	}
}

func TestLootNamingAccountRefreshUsesClientCatalogs(t *testing.T) {
	fixtures := make(map[string][]byte)
	for _, catalog := range lootMetadataCatalogs {
		fixtures[catalog.clientPath] = lootNamingCatalogFixture(t, filepath.Base(catalog.clientPath))
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("metadata refresh mutated LCU: %s", r.Method)
		}
		if body, ok := fixtures[r.URL.Path]; ok {
			_, _ = w.Write(body)
		} else if r.URL.Path == "/lol-loot/v1/player-loot-map" {
			_ = json.NewEncoder(w).Encode(lootNamingFixtureItems())
		} else {
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	a := &app{connected: true, lcu: client}
	a.refreshAccountWithClient(client)
	assertLootNamingFixture(t, a.account.Loot)
	if a.syncing {
		t.Fatal("account refresh remained busy")
	}
}

func TestLootNamingPublicCatalogFallbackIsCachedAndPrivate(t *testing.T) {
	fixtures := make(map[string][]byte)
	for _, catalog := range lootMetadataCatalogs {
		fixtures[catalog.publicPath] = lootNamingCatalogFixture(t, filepath.Base(catalog.publicPath))
	}
	var requests atomic.Int32
	provider := newChampionProvider()
	provider.cache = newChampionDataCache(nil)
	provider.client = &http.Client{Transport: championRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		requests.Add(1)
		body, ok := fixtures[r.URL.Path]
		if !ok || r.Method != http.MethodGet || r.URL.Host != communityDragonHost || r.URL.RawQuery != "" || r.Header.Get("Authorization") != "" {
			t.Errorf("unexpected public metadata request: %s %s", r.Method, r.URL)
			return nil, errors.New("unexpected request")
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(body))), Request: r}, nil
	})}
	// An unsupported or malformed local catalog must not suppress public data.
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "ward-skins.json") {
			_, _ = w.Write([]byte(`[{"name":"missing id"}]`))
			return
		}
		http.NotFound(w, r)
	}))
	defer local.Close()
	client := &LCUClient{baseURL: local.URL, token: "private-test-token", http: local.Client()}
	for attempt := 0; attempt < 2; attempt++ {
		var events []map[string]any
		observe := func(event map[string]any) { events = append(events, event) }
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		metadata := loadLootMetadata(ctx, client, provider, observe)
		cancel()
		var items []LootItem
		for _, item := range lootNamingFixtureItems() {
			items = append(items, item)
		}
		assertLootNamingFixture(t, enrichLootItemsWithMetadata(items, nil, metadata, observe))
		for _, event := range events {
			if event["event"] == "loot_name_fallback" {
				t.Fatalf("resolved fixture still reported missing names: %#v", event)
			}
		}
		log, _ := json.Marshal(events)
		for id := range lootNames0912 {
			if strings.Contains(string(log), id) {
				t.Fatalf("diagnostics exposed full inventory ID %q", id)
			}
		}
		if strings.Contains(string(log), "private-test-token") {
			t.Fatal("diagnostics leaked credential")
		}
	}
	if got := requests.Load(); got != 4 {
		t.Fatalf("public catalogs fetched %d times, want four cached reads", got)
	}
}

func TestLootNamingSharedDeadlineAndOfflineFallback(t *testing.T) {
	var active, peak atomic.Int32
	provider := newChampionProvider()
	provider.client = &http.Client{Transport: championRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		current := active.Add(1)
		defer active.Add(-1)
		for seen := peak.Load(); current > seen && !peak.CompareAndSwap(seen, current); seen = peak.Load() {
		}
		<-r.Context().Done()
		return nil, r.Context().Err()
	})}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	started := time.Now()
	metadata := loadLootMetadata(ctx, nil, provider, nil)
	if time.Since(started) > time.Second || peak.Load() != 4 || len(metadata) != 0 {
		t.Fatalf("catalogs did not share a concurrent deadline: peak=%d, entries=%d", peak.Load(), len(metadata))
	}
	items := enrichLootItems([]LootItem{{LootID: "CHEST_128", Count: 1}, {LootID: "MATERIAL_clashtickets", Count: 1}}, nil)
	for _, item := range items {
		if item.DisplayName != lootNames0912[item.LootID] || item.Asset == "" {
			t.Fatalf("verified legacy fallback failed offline: %#v", item)
		}
	}
}

func TestLootNamingPreservesLocalNamesAndReportsUnresolvedRawIDs(t *testing.T) {
	shape := &lootShapeDiagnostics{unnamedTypeCounts: make(map[string]int)}
	var events []map[string]any
	items := enrichLootItemsWithMetadata([]LootItem{
		{LootID: "SUMMONER_ICON_782", LocalizedName: "客户端专属名称", Count: 1, Type: "SUMMONERICON"},
		// P2-2（R120 复测）：泄漏哨兵不能用纯数字——「99999」这类五位数字子串会被
		// 时间戳/计数偶然包含而误报。ZQ7XK 含 a–f 之外的字母，十六进制 run_id 与
		// 纯数字字段都拼不出它。前缀匹配（WARD_）与回退语义不受影响。
		{LootID: "WARD_SKIN_RENTAL_ZQ7XK", LootName: "WARD_SKIN_RENTAL_ZQ7XK", Count: 2, Type: "WARDSKIN_RENTAL", shapeDiagnostics: shape},
	}, nil, map[string]lootMetadata{"SUMMONER_ICON_782": {Name: "公开目录名称"}}, func(event map[string]any) { events = append(events, event) })
	if len(items) != 2 || items[0].DisplayName != "客户端专属名称" || items[1].DisplayName != items[1].LootID || items[1].Count != 2 {
		t.Fatalf("local names or unresolved inventory were overwritten: %#v", items)
	}
	if len(events) != 1 || events[0]["event"] != "loot_name_fallback" || events[0]["loot_id_prefix"] != "WARD_" || shape.unnamedTypeCounts["WARDSKIN_RENTAL"] != 1 {
		t.Fatalf("raw-ID fallback was absent from diagnostics: %#v / %#v", events, shape)
	}
	log, _ := json.Marshal(events)
	if strings.Contains(string(log), "ZQ7XK") {
		t.Fatal("fallback diagnostics leaked full ID")
	}
}
