package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOptionalReadOnlyAPIsAndCapabilityDegradation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.User != nil || strings.Contains(r.RequestURI, "test-secret") {
			t.Fatal("credential must never appear in the request URL")
		}
		wantAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("riot:test-secret"))
		if r.Header.Get("Authorization") != wantAuth {
			t.Fatalf("authorization header = %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/lol-summoner/v1/current-summoner/summoner-profile":
			_, _ = w.Write([]byte(`{"backgroundSkinId":103000}`))
		case "/lol-loot/v1/player-loot-map":
			_, _ = w.Write([]byte(`{"skin_1":{"localizedName":"测试皮肤碎片","lootName":"CHAMPION_SKIN","type":"SKIN","count":2},"currency":{"localizedName":"蓝色精粹","type":"CURRENCY","count":5}}`))
		case "/lol-rewards/v1/grants":
			_, _ = w.Write([]byte(`[{"info":{"id":"pending","status":"PENDING_SELECTION","dateCreated":"2026-08-07T00:00:00Z"},"rewardGroup":{"localizations":{"title":"待选奖励","description":"Placeholder Description DO NOT TRANSLATE"},"rewards":[{"id":"r1","itemId":"1","itemType":"SKIN","quantity":1,"localizations":{"title":"皮肤奖励"}}]}},{"info":{"id":"done","status":"CLAIMED"},"rewardGroup":{}}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "test-secret", http: server.Client()}
	profile, profileCapability := NewSummonerAPI(client).Profile()
	if profile.BackgroundSkinID != 103000 || profileCapability.State != capabilityAvailable {
		t.Fatalf("profile=%#v capability=%#v", profile, profileCapability)
	}
	loot, lootCapability := NewLootAPI(client).PlayerLoot()
	if len(loot) != 2 || !loot[0].IsSkinRelated || lootCapability.Count != 2 {
		t.Fatalf("loot=%#v capability=%#v", loot, lootCapability)
	}
	rewards, rewardCapability := NewRewardsAPI(client).PendingGrants()
	if len(rewards) != 1 || rewards[0].ID != "pending" || rewards[0].Description != "" || rewardCapability.Count != 1 {
		t.Fatalf("rewards=%#v capability=%#v", rewards, rewardCapability)
	}
}

func TestOptionalCapability404DoesNotBecomeHardFailure(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	items, capability := NewLootAPI(client).PlayerLoot()
	if items != nil || capability.State != capabilityUnsupported {
		t.Fatalf("items=%#v capability=%#v", items, capability)
	}
}

func TestLCUClientCloseErasesCredential(t *testing.T) {
	client := newLCUClient(12345, "do-not-persist")
	client.Close()
	if token, ok := client.credentials(); ok || token != "" {
		t.Fatalf("credential remained after close: ok=%v token=%q", ok, token)
	}
	if _, err := client.GetBytes("/test"); err == nil || !strings.Contains(err.Error(), "credentials") {
		t.Fatalf("closed client request error = %v", err)
	}
}

func TestRelevantLCUEventClassification(t *testing.T) {
	for _, uri := range []string{"/lol-inventory/v2/inventory/CHAMPION_SKIN", "/lol-champion-mastery/v1/player/champion-mastery", "/lol-loot/v1/player-loot-map/item", "/lol-rewards/v1/grants/1", "/lol-summoner/v1/current-summoner"} {
		if !shouldRefreshForLCUEvent(LCUEvent{URI: uri}) {
			t.Fatalf("expected relevant event: %s", uri)
		}
	}
	if shouldRefreshForLCUEvent(LCUEvent{URI: "/lol-chat/v1/friends"}) {
		t.Fatal("unrelated chat event must not trigger inventory refresh")
	}
	if scope := lcuEventRefreshScope(LCUEvent{URI: "/lol-loot/v1/player-loot-map/item"}); scope != "account" {
		t.Fatalf("loot event scope = %q", scope)
	}
	if scope := lcuEventRefreshScope(LCUEvent{URI: "/lol-inventory/v2/inventory/CHAMPION_SKIN"}); scope != "collection" {
		t.Fatalf("inventory event scope = %q", scope)
	}
}

func TestAccountEndpointDoesNotExposeStableIdentifiersOrToken(t *testing.T) {
	a := &app{
		connected:     true,
		snapshotReady: true,
		summoner:      Summoner{SummonerID: 7, AccountID: 8, PUUID: "private-puuid", GameName: "玩家", TagLine: "CN1"},
		account:       AccountData{Loot: []LootItem{{LootID: "skin", Count: 1}}, Capabilities: []EndpointCapability{{Name: "player-loot", State: capabilityAvailable}}},
		lcu:           &LCUClient{token: "private-token"},
	}
	recorder := httptest.NewRecorder()
	a.handleAccount(recorder, httptest.NewRequest(http.MethodGet, "/api/account", nil))
	body := recorder.Body.String()
	for _, secret := range []string{"private-puuid", "private-token", "summonerId", "accountId", "puuid"} {
		if strings.Contains(body, secret) {
			t.Fatalf("account endpoint leaked %q: %s", secret, body)
		}
	}
}

func TestChampionMasteryDecoderSupportsBothLCUShapes(t *testing.T) {
	for _, fixture := range []string{
		`[{"championId":103,"championLevel":7,"championPoints":123456}]`,
		`{"masteries":[{"championId":103,"championLevel":7,"championPoints":123456}]}`,
	} {
		masteries, err := decodeChampionMasteries([]byte(fixture))
		if err != nil || len(masteries) != 1 || masteries[0].ChampionID != 103 || masteries[0].ChampionPoints != 123456 {
			t.Fatalf("masteries=%#v err=%v", masteries, err)
		}
	}
}

func TestSkinAcquisitionDatesRequireOwnedPurchaseDate(t *testing.T) {
	fixture := []any{
		map[string]any{"id": float64(103001), "ownership": map[string]any{"owned": true, "purchaseDate": float64(1709164800000)}},
		map[string]any{"id": float64(103004), "ownership": map[string]any{"owned": true, "purchaseDate": "2024-02-29T08:30:00+08:00"}},
		map[string]any{"id": float64(103005), "owned": true, "purchased": float64(1709337600000)},
		map[string]any{"id": float64(103006), "ownership": map[string]any{"owned": true, "rental": map[string]any{"purchaseDate": float64(1709424000000)}}},
		map[string]any{"id": float64(103002), "ownership": map[string]any{"owned": false, "purchaseDate": float64(1709164800000)}},
		map[string]any{"id": float64(103003), "owned": true, "purchaseDate": float64(1)},
	}
	dates := extractSkinAcquisitionDates(fixture, time.Date(2026, 8, 7, 0, 0, 0, 0, time.UTC))
	if dates[103001] != "2024-02-29T00:00:00Z" || dates[103004] != "2024-02-29T00:30:00Z" || dates[103005] != "2024-03-02T00:00:00Z" || dates[103006] != "2024-03-03T00:00:00Z" || len(dates) != 4 {
		t.Fatalf("dates=%#v", dates)
	}
}

func TestSkinAcquisitionDatesUseVerifiedOwnershipAcrossTencentShapes(t *testing.T) {
	fixture := map[string]any{"payload": []any{
		map[string]any{"itemId": float64(103001), "acquiredDate": float64(1709164800000000)},
		map[string]any{"skinId": float64(103002), "ownership": map[string]any{"purchaseDateMillis": "2024-03-01T08:00:00+08:00"}},
		map[string]any{"skinId": float64(103003), "purchaseDate": float64(1709251200000)},
	}}
	dates := extractSkinAcquisitionDatesForOwned(fixture, time.Date(2026, 8, 7, 0, 0, 0, 0, time.UTC), map[int64]bool{103001: true, 103002: true})
	if dates[103001] != "2024-02-29T00:00:00Z" || dates[103002] != "2024-03-01T00:00:00Z" || len(dates) != 2 {
		t.Fatalf("dates=%#v", dates)
	}
}

func TestSkinAcquisitionDatesReadNestedTencentItemKey(t *testing.T) {
	fixture := map[string]any{"inventoryItems": []any{
		map[string]any{"itemKey": map[string]any{"itemId": float64(103001), "inventoryType": "CHAMPION_SKIN"}, "purchaseDate": "2025-05-29T08:00:00+08:00", "quantity": float64(1)},
	}}
	dates := extractSkinAcquisitionDatesForOwned(fixture, time.Date(2026, 8, 7, 0, 0, 0, 0, time.UTC), map[int64]bool{103001: true})
	if dates[103001] != "2025-05-29T00:00:00Z" || len(dates) != 1 {
		t.Fatalf("dates=%#v", dates)
	}
}

func TestSkinLootClassificationUsesClientTypeInsteadOfDisplayName(t *testing.T) {
	if !isSkinLoot(LootItem{Type: "CHAMPION_SKIN", LootID: "loot-1"}) {
		t.Fatal("CHAMPION_SKIN must be skin related")
	}
	if isSkinLoot(LootItem{Type: "CHEST", LootName: "Skin Celebration Chest", LootID: "loot-2"}) {
		t.Fatal("a display name containing skin must not change the inventory type")
	}
}

func TestEnrichLootItemsUsesChineseNamesAndCatalogSkinNames(t *testing.T) {
	items := []LootItem{
		{LootID: "CHAMPION_SKIN_RENTAL_143002", Type: "SKIN_RENTAL", Count: 1, IsSkinRelated: true, DisenchantValue: 1580, UpgradeEssenceValue: 6950},
		{LootID: "CURRENCY_champion", Count: 83531},
		{LootID: "CURRENCY_cosmetic", Count: 10929},
		{LootID: "MATERIAL_key", Count: 9},
		{LootID: "MATERIAL_key_fragment", Count: 1},
		{LootID: "CHAMPION_45", Type: "CHAMPION", Count: 1},
		{LootID: "loot-box", LocalizedName: "未命名战利品", Count: 64},
		{LootID: "CHEST_champion_mastery", Type: "CHEST", Count: 54},
		{LootID: "CHEST_promotion", Type: "CHEST", Count: 1},
	}
	items = enrichLootItems(items, []Skin{{ID: 143002, Name: "K/DA ALL OUT 萨勒芬妮 独立音乐人", ChampionID: 143, ChampionName: "萨勒芬妮", TilePath: "/lol-game-data/assets/skin.png", Owned: true}, {ID: 45000, Name: "维迦", ChampionID: 45, ChampionName: "维迦"}})
	want := []string{"K/DA ALL OUT 萨勒芬妮 独立音乐人", "蓝色精粹", "橙色精粹", "战利品宝箱钥匙", "钥匙碎片", "维迦", "loot-box", "战利品宝箱", "紫色宝箱"}
	for index, expected := range want {
		if items[index].DisplayName != expected {
			t.Fatalf("item %d name=%q want=%q", index, items[index].DisplayName, expected)
		}
	}
	if items[0].Category != "皮肤" || items[0].SkinID != 143002 {
		t.Fatalf("skin enrichment=%#v", items[0])
	}
	if !items[0].SkinOwnedKnown || !items[0].SkinOwned {
		t.Fatalf("skin ownership=%#v", items[0])
	}
	if items[0].Kind != "皮肤碎片" || items[0].TilePath == "" {
		t.Fatalf("skin kind/assets=%#v", items[0])
	}
	if items[0].DisenchantValue != 1580 || items[0].UpgradeEssenceValue != 6950 {
		t.Fatalf("skin essence values=%#v", items[0])
	}
	if items[1].Category != "材料" || items[3].Category != "材料" {
		t.Fatalf("resource categories=%#v %#v", items[1], items[3])
	}
	if items[5].Category != "英雄" {
		t.Fatalf("champion enrichment=%#v", items[5])
	}
	if items[8].Asset != "/fe/lol-loot/assets/loot_item_icons/chest_promotion.png" || items[8].Category != "宝箱" {
		t.Fatalf("promotion chest enrichment=%#v", items[8])
	}
}

func TestEnrichLootItemsRetainsTheLCUPendingShell(t *testing.T) {
	items := enrichLootItems([]LootItem{
		{Count: 30, rawKeyEmpty: true},
		{LootID: "CHEST_224", LocalizedName: "未命名战利品", Count: 1},
		{LootName: "MATERIAL_REAL", Count: 1},
	}, nil)
	if len(items) != 3 || !items[0].DataPending || items[0].DisplayName != "客户端数据暂未同步，可稍后重试" {
		t.Fatalf("empty-shell filtering dropped non-empty loot: %#v", items)
	}
	if items[1].DisplayName != "CHEST_224" || items[2].DisplayName != "MATERIAL_REAL" {
		t.Fatalf("real loot identifiers were not preserved: %#v", items)
	}
}

func TestPromotionChestUsesRequestedChineseName(t *testing.T) {
	if got := lootChineseNames["CHEST_PROMOTION"]; got != "紫色宝箱" {
		t.Fatalf("CHEST_PROMOTION name = %q, want 紫色宝箱", got)
	}
}

func TestGenericAndPromotionChestsStayDistinctAndUseCurrentClientArtwork(t *testing.T) {
	metadata := map[string]lootMetadata{
		"CHEST_GENERIC": {Name: "海克斯科技宝箱", Image: "/lol-game-data/assets/ASSETS/Loot/chest_generic.png"},
	}
	items := enrichLootItemsWithMetadata([]LootItem{
		{LootID: "CHEST_generic", Type: "CHEST", Count: 1},
		{LootID: "CHEST_promotion", Type: "CHEST", Count: 1},
	}, nil, metadata, nil)
	if len(items) != 2 || items[0].LootID == items[1].LootID || items[0].Count != 1 || items[1].Count != 1 {
		t.Fatalf("distinct chest entries were merged: %#v", items)
	}
	if items[0].DisplayName != "海克斯科技宝箱" || items[1].DisplayName != "紫色宝箱" {
		t.Fatalf("chest names = %q / %q", items[0].DisplayName, items[1].DisplayName)
	}
	wantIcon := "/fe/lol-loot/assets/loot_item_icons/chest_promotion.png"
	if items[0].Asset != wantIcon || items[1].Asset != wantIcon {
		t.Fatalf("current client chest artwork = %q / %q, want %q", items[0].Asset, items[1].Asset, wantIcon)
	}
}

func TestCommunityDragonLootMetadataNamesMasterworkChestAndKeepsPromotionFallback(t *testing.T) {
	provider := newChampionProvider()
	provider.clientMu.Lock()
	provider.client = &http.Client{Transport: championRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Host != communityDragonHost || request.URL.Path != "/latest/plugins/rcp-be-lol-game-data/global/zh_cn/v1/loot.json" {
			t.Fatalf("unexpected loot catalog request: %s", request.URL.String())
		}
		body := `{"LootItems":[{"id":"CHEST_224","name":"杰作宝箱","description":"开启后获得战利品","image":"/lol-game-data/assets/v1/loot/chest_224.png"}]}`
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
	})}
	provider.clientMu.Unlock()
	metadata, err := provider.loadCommunityDragonLootMetadata(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	items := enrichLootItemsWithMetadata([]LootItem{
		{LootID: "CHEST_224", LocalizedName: "未命名战利品", Count: 1},
		{LootID: "CHEST_PROMOTION", Count: 1},
	}, nil, metadata, nil)
	if items[0].DisplayName != "杰作宝箱" || items[0].Asset == "" || items[0].LocalizedDescription == "" {
		t.Fatalf("CommunityDragon metadata was not applied: %#v", items[0])
	}
	if items[1].DisplayName != "紫色宝箱" || items[1].Asset != lootClientIcons["CHEST_PROMOTION"] {
		t.Fatalf("hard-coded promotion fallback regressed: %#v", items[1])
	}
}

func TestLootDiagnosticsAllPersistWithReviewedFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/lol-loot/v1/player-loot-map" {
			http.NotFound(w, r)
			return
		}
		_, _ = io.WriteString(w, `{
			"CHEST_224":{"localizedName":"未命名战利品","type":"CHEST","count":1},
			"CHEST_SKIN_EVENT":{"displayCategories":"SKIN","type":"CHEST","count":2},
			"":{"lootId":"","type":"","count":30},
			"MATERIAL_EMPTY":{"type":"MATERIAL","count":0}
		}`)
	}))
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	store := trackTestStore(t, &localStore{root: t.TempDir()})
	if err := os.MkdirAll(filepath.Join(store.root, "logs"), 0o700); err != nil {
		t.Fatal(err)
	}
	a := &app{storage: store}
	items, capability := NewObservedLootAPI(client, a.recordDiagnostic).PlayerLoot()
	if capability.State != capabilityAvailable || len(items) != 3 {
		t.Fatalf("loot fixture = items:%#v capability:%#v", items, capability)
	}
	items = enrichLootItemsWithMetadata(items, nil, nil, a.recordDiagnostic)
	if len(items) != 3 || !items[1].DataPending || items[0].LootID == "" || items[2].LootID == "" {
		t.Fatalf("loot enrichment must label the pending shell and preserve real loot: %#v", items)
	}
	data, err := store.readDiagnosticLog()
	if err != nil {
		t.Fatal(err)
	}
	diagnosticLog := string(data)
	if strings.Contains(diagnosticLog, "CHEST_224") {
		t.Fatalf("diagnostics leaked full loot ID CHEST_224: %s", diagnosticLog)
	}
	events := make(map[string]map[string]any)
	var emptyKeyFallback map[string]any
	for _, line := range strings.Split(strings.TrimSpace(diagnosticLog), "\n") {
		var event map[string]any
		if json.Unmarshal([]byte(line), &event) == nil {
			if name, _ := event["event"].(string); name != "" {
				events[name] = event
				if name == "loot_name_fallback" && event["raw_key_empty"] == true {
					emptyKeyFallback = event
				}
			}
		}
	}
	shape := events["loot_map_shape"]
	if shape["raw_entries"] != float64(4) || shape["kept"] != float64(3) || shape["dropped_zero_count"] != float64(1) || shape["type_counts"] == nil || shape["id_prefix_counts"] == nil || shape["known_chest_counts"] == nil {
		t.Fatalf("loot_map_shape = %#v", shape)
	}
	knownChests, _ := shape["known_chest_counts"].(map[string]any)
	if knownChests["masterwork"] != float64(1) || knownChests["other"] != float64(1) {
		t.Fatalf("loot_map_shape known_chest_counts = %#v", shape)
	}
	unnamed, _ := shape["unnamed_type_counts"].(map[string]any)
	if unnamed[""] != float64(1) {
		t.Fatalf("loot_map_shape unnamed_type_counts = %#v", shape)
	}
	fallback := emptyKeyFallback
	if fallback["loot_id_prefix"] != "OTHER" || fallback["raw_key_empty"] != true || fallback["type_empty"] != true || fallback["display_categories"] != "" {
		t.Fatalf("loot_name_fallback = %#v", fallback)
	}
	if _, ok := fallback["loot_id"]; ok {
		t.Fatalf("loot_name_fallback retained a full loot_id field: %#v", fallback)
	}
	if _, ok := fallback["count"]; ok {
		t.Fatalf("loot_name_fallback retained an unnecessary item count: %#v", fallback)
	}
	category := events["loot_category_assigned"]
	if category["category"] != "宝箱" || category["id_prefix"] != "CHEST_" || category["type"] != "CHEST" {
		t.Fatalf("loot_category_assigned = %#v", category)
	}
}

func TestLootEmptyShellFilteringStillReportsItsShape(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/lol-loot/v1/player-loot-map" {
			http.NotFound(w, r)
			return
		}
		_, _ = io.WriteString(w, `{"":{"lootId":"","type":"","count":30}}`)
	}))
	defer server.Close()
	store := trackTestStore(t, &localStore{root: t.TempDir()})
	if err := os.MkdirAll(filepath.Join(store.root, "logs"), 0o700); err != nil {
		t.Fatal(err)
	}
	a := &app{storage: store}
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	items, _ := NewObservedLootAPI(client, a.recordDiagnostic).PlayerLoot()
	if items = enrichLootItemsWithMetadata(items, nil, nil, a.recordDiagnostic); len(items) != 1 || !items[0].DataPending {
		t.Fatalf("empty-shell loot survived filtering: %#v", items)
	}
	data, err := store.readDiagnosticLog()
	if err != nil {
		t.Fatal(err)
	}
	var shape map[string]any
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var event map[string]any
		if json.Unmarshal([]byte(line), &event) == nil && event["event"] == "loot_map_shape" {
			shape = event
		}
	}
	if shape["raw_entries"] != float64(1) || shape["kept"] != float64(1) {
		t.Fatalf("filtered empty-shell shape = %#v; log=%s", shape, data)
	}
}

func TestLootIDPrefixCoversKnownLCUTypes(t *testing.T) {
	fixtures := map[string]string{
		"CHEST_224": "CHEST_", "MATERIAL_KEY": "MATERIAL_", "CURRENCY_CHAMPION": "CURRENCY_",
		"CHAMPION_266": "CHAMPION_", "SKIN_SHARD_266001": "SKIN_", "STATSTONE_1": "STATSTONE_",
		"EMOTE_1": "EMOTE_", "WARD_1": "WARD_", "COMPANION_1": "COMPANION_", "TFT_ITEM_1": "TFT_",
		"UNKNOWN_1": "OTHER",
	}
	for input, want := range fixtures {
		if got := lootIDPrefix(input); got != want {
			t.Fatalf("lootIDPrefix(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestLCURequestDiagnosticsCaptureRealTLSAndConnectionReuse(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"queues":[]}`)
	}))
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	events := make([]map[string]any, 0, 1)
	client.setDiagnosticObserver(func(event map[string]any) { events = append(events, event) })
	for range 2 {
		var payload map[string]any
		if err := client.GetJSON("/lol-ranked/v1/ranked-stats/secret-player-reference", &payload); err != nil {
			t.Fatal(err)
		}
	}
	client.flushRequestDiagnostics()
	if len(events) != 1 {
		t.Fatalf("LCU request aggregates = %#v", events)
	}
	event := events[0]
	tlsSummary, _ := event["tls_ms"].(map[string]int64)
	if event["event"] != "lcu_request" || event["method"] != http.MethodGet || event["path"] != "/lol-ranked/v1/ranked-stats/{puuid}" || event["count"] != 2 {
		t.Fatalf("LCU request diagnostic identity = %#v", event)
	}
	if event["conn_reused"] != true || tlsSummary["max"] <= 0 || event["conn_wait_ms"] == nil || event["ttfb_ms"] == nil || event["http_status"] == nil {
		t.Fatalf("LCU httptrace fields are not real: %#v", event)
	}
}

func TestRewardTitleFiltersClientPlaceholders(t *testing.T) {
	for _, fixture := range []string{
		"Placeholder Name for Reward Group dO nOt TrAnSlAtE",
		"PLACEHOLDER_NAME_FOR_REWARD_GROUP",
		"REWARD GROUP PLACEHOLDER",
	} {
		if got := rewardTitle(fixture); got != "待领取奖励" {
			t.Fatalf("rewardTitle(%q) = %q", fixture, got)
		}
		if got := rewardDescription(fixture); got != "" {
			t.Fatalf("rewardDescription(%q) = %q", fixture, got)
		}
	}
	for _, fixture := range []string{"待选奖励", "K/DA ALL OUT 奖励", "2026 SEASON REWARD"} {
		if got := rewardTitle(fixture); got != fixture {
			t.Fatalf("valid reward title %q was replaced with %q", fixture, got)
		}
	}
	for _, fixture := range []string{"完成一场对局后领取。", "Includes one skin shard", "2026 SEASON REWARD"} {
		if got := rewardDescription(fixture); got != fixture {
			t.Fatalf("valid reward description %q was replaced with %q", fixture, got)
		}
	}
}

func TestPendingGrantPlaceholderDoesNotInventPassSource(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/lol-rewards/v1/grants" {
			http.NotFound(w, r)
			return
		}
		_, _ = io.WriteString(w, `[{"info":{"id":"pass-a","status":"PENDING_SELECTION"},"rewardGroup":{"id":"group-a","localizations":{"title":"PLACEHOLDER_NAME_FOR_REWARD_GROUP"},"rewards":[{"id":"orange","itemId":"CURRENCY_orange","quantity":25}]}},{"info":{"id":"pass-b","status":"PENDING_SELECTION"},"rewardGroup":{"id":"group-b","localizations":{"title":"Placeholder Name for Reward Group DO NOT TRANSLATE"},"rewards":[{"id":"blue","itemId":"CURRENCY_blue","quantity":750}]}}]`)
	}))
	defer server.Close()

	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	grants, capability := NewRewardsAPI(client).PendingGrants()
	if capability.State != capabilityAvailable || len(grants) != 2 {
		t.Fatalf("pending pass grants = %#v capability=%#v", grants, capability)
	}
	for _, grant := range grants {
		if grant.Title != "待领取奖励" || grant.DisplayGroup != "" {
			t.Fatalf("placeholder grant invented a pass source: %#v", grant)
		}
	}
}

func TestOptionalCapabilityCancellationIsNotFailure(t *testing.T) {
	capability := optionalCapabilityError(EndpointCapability{Name: "pending-rewards"}, context.Canceled)
	if capability.State != capabilityCanceled || capability.Detail != "" {
		t.Fatalf("canceled optional capability = %#v", capability)
	}
}

func TestSanctumSparkWalletSupportsTencentWalletShapes(t *testing.T) {
	for _, fixture := range []string{`28`, `"28"`, `{"lol_blessing_token":28}`, `{"lol_blessing_token":"28"}`, `{"balance":28}`, `{"amount":{"value":28}}`} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/lol-inventory/v1/wallet/lol_blessing_token" {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(fixture))
		}))
		client := &LCUClient{baseURL: server.URL, token: "test-secret", http: server.Client()}
		balance, capability := NewLootAPI(client).SanctumSparks()
		server.Close()
		if balance != 28 || capability.State != capabilityAvailable || capability.Count != 28 {
			t.Fatalf("fixture=%s balance=%d capability=%#v", fixture, balance, capability)
		}
	}
}

func TestOwnedChampionIDsDoNotConfuseNestedSkins(t *testing.T) {
	fixture := []any{
		map[string]any{"id": float64(103), "ownership": map[string]any{"owned": true}, "skins": []any{map[string]any{"id": float64(103001), "championId": float64(103), "ownership": map[string]any{"owned": true}}}},
		map[string]any{"id": float64(84), "ownership": map[string]any{"owned": false}},
	}
	ids := extractOwnedChampionIDs(fixture)
	if !ids[103] || len(ids) != 1 {
		t.Fatalf("champion IDs=%#v", ids)
	}
}

func TestRPPriceRequiresExplicitRPCurrency(t *testing.T) {
	price, ok := extractRPPrice(map[string]any{"prices": []any{map[string]any{"currency": "RP", "basePrice": float64(1350)}}})
	if !ok || price != 1350 {
		t.Fatalf("price=%d ok=%v", price, ok)
	}
	if price, ok := extractRPPrice(map[string]any{"price": float64(1350)}); ok || price != 0 {
		t.Fatalf("unqualified price must not be trusted: price=%d ok=%v", price, ok)
	}
	if price, ok := extractRPPrice(map[string]any{"rpCost": float64(1820), "rpPrice": float64(1350)}); ok || price != 0 {
		t.Fatalf("conflicting RP fields must fail closed: price=%d ok=%v", price, ok)
	}
}

func TestSkinBorderRequiresMatchingSkinAugment(t *testing.T) {
	fixture := map[string]any{"skins": []any{
		map[string]any{"id": float64(103001), "skinAugments": map[string]any{"borders": map[string]any{"layer0": map[string]any{"contentId": "Border-A", "borderPath": "/asset/border.png"}}}},
		map[string]any{"id": float64(103002)},
	}}
	hasBorder, ids := skinBorderContentIDs(fixture, 103001)
	if !hasBorder || !ids["border-a"] || len(ids) != 1 {
		t.Fatalf("hasBorder=%v ids=%#v", hasBorder, ids)
	}
	hasBorder, ids = skinBorderContentIDs(fixture, 103002)
	if hasBorder || len(ids) != 0 {
		t.Fatalf("skin without border reported one: hasBorder=%v ids=%#v", hasBorder, ids)
	}
}
