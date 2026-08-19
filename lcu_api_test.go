package main

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
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
			_, _ = w.Write([]byte(`[{"info":{"id":"pending","status":"PENDING_SELECTION","dateCreated":"2026-08-07T00:00:00Z"},"rewardGroup":{"localizations":{"title":"待选奖励"},"rewards":[{"id":"r1","itemId":"1","itemType":"SKIN","quantity":1,"localizations":{"title":"皮肤奖励"}}]}},{"info":{"id":"done","status":"CLAIMED"},"rewardGroup":{}}]`))
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
	if len(rewards) != 1 || rewards[0].ID != "pending" || rewardCapability.Count != 1 {
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
	if scope := lcuEventRefreshScope(LCUEvent{URI: "/lol-inventory/v2/inventory/CHAMPION_SKIN"}); scope != "full" {
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
	}
	items = enrichLootItems(items, []Skin{{ID: 143002, Name: "K/DA ALL OUT 萨勒芬妮 独立音乐人", ChampionID: 143, ChampionName: "萨勒芬妮", TilePath: "/lol-game-data/assets/skin.png", Owned: true}, {ID: 45000, Name: "维迦", ChampionID: 45, ChampionName: "维迦"}})
	want := []string{"K/DA ALL OUT 萨勒芬妮 独立音乐人", "蓝色精粹", "橙色精粹", "战利品宝箱钥匙", "钥匙碎片", "维迦", "未识别材料", "战利品宝箱"}
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
