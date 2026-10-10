package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSettleBlankLootEndsPendingStateWithoutMutatingTheSharedSlice(t *testing.T) {
	shared := AccountData{Loot: []LootItem{
		{LootID: "CURRENCY_CHAMPION", Count: 5},
		{Blank: true, DataPending: true, LootID: "MATERIAL_123", StoreItemID: 123, Kind: "类型未知", Count: 74},
	}}
	settled, blanks := settleBlankLoot(shared)
	if blanks != 1 || settled.Loot[1].DataPending || !settled.Loot[1].Blank || settled.Loot[1].Count != 74 || settled.Loot[0].LootID != "CURRENCY_CHAMPION" {
		t.Fatalf("settled account = %#v blanks=%d", settled.Loot, blanks)
	}
	if !shared.Loot[1].DataPending {
		t.Fatal("settleBlankLoot changed the caller's slice in place")
	}
	if _, blanks := settleBlankLoot(settled); blanks != 0 {
		t.Fatalf("settling twice reported %d records", blanks)
	}
}

func TestRetryExhaustionStopsPresentingTheBlankRecordAsNotSyncedYet(t *testing.T) {
	c := &LCUClient{baseURL: "https://127.0.0.1:2999", token: "fixture", http: &http.Client{}}
	pending := AccountData{Loot: []LootItem{{Blank: true, DataPending: true, LootID: "MATERIAL_123", StoreItemID: 123, Kind: "类型未知", Count: 74}}}
	a := &app{lcu: c, connected: true, account: cloneAccountData(pending), refreshRequests: make(chan struct{}, 1)}
	for attempt := 1; attempt <= 3; attempt++ {
		a.scheduleCollectionDataRetry(c, pending)
		if a.collectionDataRetry == nil {
			t.Fatalf("retry %d missing", attempt)
		}
		if !a.account.Loot[0].DataPending {
			t.Fatalf("record was settled before retry %d ran", attempt)
		}
		a.collectionDataRetry.Reset(time.Millisecond)
		select {
		case <-a.refreshRequests:
		case <-time.After(time.Second):
			t.Fatal("retry not requested")
		}
		a.mu.Lock()
		a.collectionRefreshPending = false
		a.mu.Unlock()
	}
	a.scheduleCollectionDataRetry(c, pending)
	if a.account.Loot[0].DataPending || !a.account.Loot[0].Blank || a.account.Loot[0].Count != 74 {
		t.Fatalf("exhausted retries left the record pending: %#v", a.account.Loot)
	}
	if !pending.Loot[0].DataPending {
		t.Fatal("the freshly loaded account was mutated in place")
	}
	if collectionDataPending(a.account) {
		t.Fatal("a settled record still asks for more retries")
	}
}

func TestRetryBudgetNotSpentYetDoesNotSettle(t *testing.T) {
	c := &LCUClient{baseURL: "https://127.0.0.1:2999", token: "fixture", http: &http.Client{}}
	pending := AccountData{Loot: []LootItem{{Blank: true, DataPending: true, LootID: "MATERIAL_123", StoreItemID: 123, Count: 3}}}
	a := &app{lcu: c, connected: true, account: cloneAccountData(pending), collectionDataRetryClient: c, collectionDataRetryCount: 3}
	// The third retry is still waiting on its timer, so the record may yet fill in.
	a.collectionDataRetry = time.AfterFunc(time.Hour, func() {})
	defer a.collectionDataRetry.Stop()
	a.scheduleCollectionDataRetry(c, pending)
	if !a.account.Loot[0].DataPending {
		t.Fatal("record was settled while the last retry was still scheduled")
	}
}

func TestPlayerLootKeepsTheBlankRecordLastAndLogsOnlyItsFieldNames(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{
			"":{"lootId":"","lootName":"","type":"","count":74,"asset":"","storeItemId":0,"tags":[],"rarity":"","disenchantValue":12},
			"CURRENCY_CHAMPION":{"lootId":"CURRENCY_CHAMPION","type":"CURRENCY","count":9},
			"MATERIAL_KEY":{"lootId":"MATERIAL_KEY","type":"MATERIAL","count":2}
		}`))
	}))
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	var events []map[string]any
	items, capability := NewObservedLootAPI(client, func(event map[string]any) { events = append(events, event) }).PlayerLoot()
	if capability.State != capabilityAvailable || len(items) != 3 {
		t.Fatalf("items=%#v capability=%#v", items, capability)
	}
	if items[2].LootID != "" || items[2].Count != 74 || items[0].LootID == "" || items[1].LootID == "" {
		t.Fatalf("blank record was not sorted last: %#v", items)
	}
	items = enrichLootItemsWithMetadata(items, nil, nil, func(event map[string]any) { events = append(events, event) })
	var fallback map[string]any
	for _, event := range events {
		if event["event"] == "loot_name_fallback" {
			fallback = event
		}
	}
	keys, _ := fallback["field_keys"].([]string)
	if strings.Join(keys, ",") != "count,disenchantValue" {
		t.Fatalf("field_keys must list only the filled fields, got %#v (event=%#v)", fallback["field_keys"], fallback)
	}
	if !items[2].Blank || !items[2].DataPending || items[2].Count != 74 {
		t.Fatalf("blank record lost its quantity or state: %#v", items[2])
	}
}

func TestBlankLootDropsImagePathsButKeepsDiagnosticAsset(t *testing.T) {
	const brokenIcon = "/fe/lol-loot/assets/loot_item_icons/.png"
	items := []LootItem{
		{Count: 74, Asset: brokenIcon, TilePath: "/fe/lol-loot/assets/loot_item_icons/tile.png", SplashPath: "/fe/lol-loot/assets/loot_item_icons/splash.png"},
		{LootID: "MATERIAL_UNMAPPED_R144", Count: 2, Asset: "/fe/lol-loot/assets/loot_item_icons/key.png"},
	}
	var fallback map[string]any
	items = enrichLootItemsWithMetadata(items, nil, nil, func(event map[string]any) {
		if event["event"] == "loot_name_fallback" && event["loot_id_prefix"] == "OTHER" {
			fallback = event
		}
	})
	if !items[0].Blank || items[0].Asset != "" || items[0].TilePath != "" || items[0].SplashPath != "" {
		t.Fatalf("blank loot still exposes image paths: %#v", items[0])
	}
	if items[1].Asset != "/fe/lol-loot/assets/loot_item_icons/key.png" {
		t.Fatalf("named loot lost its image path: %#v", items[1])
	}
	if fallback["asset"] != brokenIcon {
		t.Fatalf("diagnostic lost the original image path: %#v", fallback)
	}
}
