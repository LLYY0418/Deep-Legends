package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const r189Catalog = `[{"id":1468,"name":"演示表情（测试夹具）","description":"目录描述","inventoryIcon":"/lol-game-data/assets/ASSETS/Loadouts/SummonerEmotes/fixture.png"}]`
const r189Image = "/lol-game-data/assets/ASSETS/Loadouts/SummonerEmotes/fixture.png"

func TestR189ParseLootEmoteCatalog(t *testing.T) {
	entries, err := parseLootEmoteCatalog([]byte(`[{"id":1468,"name":" 演示表情 ","description":" 测试描述 ","inventoryIcon":"` + r189Image + `"},{"id":-1,"name":"invalid"},{"id":2,"name":" "},{"name":"missing id"},{"id":0,"name":"零号"},{"id":3,"name":"安全路径","inventoryIcon":"https://private.invalid/icon.png"}]`))
	if err != nil || len(entries) != 3 {
		t.Fatal(entries, err)
	}
	if got := entries["EMOTE_1468"]; got.Name != "演示表情" || got.Description != "测试描述" || got.Image != r189Image {
		t.Fatal(got)
	}
	if entries["EMOTE_3"].Image != "" || entries["EMOTE_0"].Name != "零号" {
		t.Fatal(entries)
	}
	for _, body := range []string{`[]`, `[{"id":-1,"name":"无效"}]`, `{}`, `broken`} {
		if _, err := parseLootEmoteCatalog([]byte(body)); err == nil {
			t.Fatal("unusable catalog accepted", body)
		}
	}
}

func TestR189RegisteredEmoteCatalogEnrichment(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("metadata wrote to client: %s", r.Method)
		}
		if r.URL.Path == "/lol-game-data/assets/v1/summoner-emotes.json" {
			fmt.Fprint(w, r189Catalog)
		} else {
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	var events []map[string]any
	observe := func(e map[string]any) { events = append(events, e) }
	metadata := loadLootMetadata(context.Background(), client, nil, observe)
	// Even a generic client mapping and earlier client artwork cannot replace
	// the emote directory image. This test must use the registered loader.
	old, existed := lootClientIcons["EMOTE_1468"]
	lootClientIcons["EMOTE_1468"] = "/fe/lol-loot/assets/generic.png"
	defer func() {
		if existed {
			lootClientIcons["EMOTE_1468"] = old
		} else {
			delete(lootClientIcons, "EMOTE_1468")
		}
	}()
	items := enrichLootItemsWithMetadata([]LootItem{{LootID: "EMOTE_1468", LootName: "EMOTE_1468", Type: "EMOTE", ItemStatus: "OWNED", Count: 1, Asset: "/fe/lol-loot/assets/generic.png", TilePath: "/fe/lol-loot/assets/generic-tile.png"}}, nil, metadata, observe)
	got := items[0]
	if got.DisplayName != "演示表情（测试夹具）" || got.LocalizedDescription != "目录描述" || got.Asset != r189Image || got.TilePath != "" || got.SplashPath != "" || got.DataPending || !got.OwnedKnown || !got.Owned {
		t.Fatal(got)
	}
	found := false
	for _, e := range events {
		if e["event"] == "loot_name_fallback" {
			t.Fatal("resolved emote reported as fallback", e)
		}
		if e["event"] == "loot_metadata_source" && e["catalog"] == "emotes" {
			found = e["source"] == "client" && e["entries"] == 1
		}
	}
	if !found {
		t.Fatal("registered emote source absent", events)
	}
}

func TestR189UnknownLootFallbackAndDiagnostic(t *testing.T) {
	for _, tc := range []struct{ id, kind, want string }{
		{"EMOTE_1468", "EMOTE", "表情 1468"}, {"EMOTE_FUTURE", "EMOTE", "表情"}, {"WARD_SKIN_RENTAL_123", "WARDSKIN_RENTAL", "守卫 123"},
		{"SUMMONER_ICON_456", "SUMMONERICON", "图标 456"}, {"CHEST_999999", "CHEST", "宝箱 999999"}, {"MATERIAL_FUTURE", "MATERIAL", "材料"},
	} {
		t.Run(tc.id, func(t *testing.T) {
			var events []map[string]any
			items := enrichLootItemsWithMetadata([]LootItem{{LootID: tc.id, LootName: tc.id, Type: tc.kind, ItemStatus: "NONE", Count: 567891, Asset: "/fe/lol-loot/assets/generic.png"}}, nil, nil, func(e map[string]any) { events = append(events, e) })
			got := items[0]
			if got.DisplayName != tc.want || got.DataPending || got.Blank || got.Count != 567891 {
				t.Fatal(got)
			}
			if tc.kind == "EMOTE" && (got.Asset != "" || got.TilePath != "" || got.SplashPath != "") {
				t.Fatal("unknown emote must use category placeholder", got)
			}
			var fallback map[string]any
			for _, e := range events {
				if e["event"] == "loot_name_fallback" {
					fallback = e
				}
			}
			if fallback == nil || fallback["item_status"] != "NONE" || fallback["catalog_hit"] != false {
				t.Fatal(events)
			}
			encoded, _ := json.Marshal(fallback)
			if strings.Contains(string(encoded), tc.id) || strings.Contains(string(encoded), "567891") {
				t.Fatal("diagnostics leaked inventory ID/quantity", string(encoded))
			}
		})
	}
	var event map[string]any
	enrichLootItemsWithMetadata([]LootItem{{LootID: "EMOTE_FUTURE", Type: "EMOTE"}}, nil, map[string]lootMetadata{"EMOTE_FUTURE": {}}, func(e map[string]any) { event = e })
	if event["catalog_hit"] != true || event["item_status"] != "" {
		t.Fatal("empty catalog entry hit must still be diagnosed", event)
	}
}

func TestR189OwnershipFromItemStatus(t *testing.T) {
	for _, kind := range []string{"EMOTE", "WARDSKIN", "SUMMONERICON"} {
		for _, status := range []string{"OWNED", "NONE", "", "UNKNOWN"} {
			item := enrichLootItemsWithMetadata([]LootItem{{LootID: kind + "_777777", Type: kind, ItemStatus: status, Owned: true, OwnedKnown: true}}, nil, nil, nil)[0]
			if item.OwnedKnown != (status == "OWNED" || status == "NONE") || item.Owned != (status == "OWNED") {
				t.Fatal(kind, status, item)
			}
			body, _ := json.Marshal(item)
			var decoded map[string]any
			json.Unmarshal(body, &decoded)
			if decoded["owned"] != item.Owned || item.OwnedKnown && decoded["ownedKnown"] != true {
				t.Fatal("ownership fields not serialized", decoded)
			}
		}
	}
	// ItemStatus does not reinterpret skin ownership from the real inventory.
	got := enrichLootItemsWithMetadata([]LootItem{{LootID: "SKIN_103001", IsSkinRelated: true, ItemStatus: "NONE"}}, []Skin{{ID: 103001, Name: "测试皮肤", Owned: true}}, nil, nil)[0]
	if !got.SkinOwnedKnown || !got.SkinOwned || got.OwnedKnown {
		t.Fatal(got)
	}
}

func TestR189EmotePublicFallbackAndBothUnavailable(t *testing.T) {
	for _, available := range []bool{true, false} {
		t.Run(fmt.Sprint(available), func(t *testing.T) {
			var publicReads atomic.Int32
			provider := newChampionProvider()
			provider.cache = newChampionDataCache(nil)
			provider.client = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
				if r.URL.Host == communityDragonHost && strings.HasSuffix(r.URL.Path, "/summoner-emotes.json") {
					publicReads.Add(1)
					if r.Method != "GET" || r.URL.RawQuery != "" || r.Header.Get("Authorization") != "" {
						t.Error("public fallback included private request data")
					}
					if available {
						var body any
						json.Unmarshal([]byte(r189Catalog), &body)
						return r178JSON(body, 200), nil
					}
				}
				return r178JSON(map[string]any{}, 404), nil
			})}
			client := &LCUClient{baseURL: "http://lcu.local", token: "test-token", http: &http.Client{Transport: gameplayRoundTripFunc(func(*http.Request) (*http.Response, error) { return r178JSON(map[string]any{}, 404), nil })}}
			for attempt := 0; attempt < 2; attempt++ {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				metadata := loadLootMetadata(ctx, client, provider, nil)
				cancel()
				item := enrichLootItemsWithMetadata([]LootItem{{LootID: "EMOTE_1468", Type: "EMOTE"}}, nil, metadata, nil)[0]
				want := "表情 1468"
				if available {
					want = "演示表情（测试夹具）"
				}
				if item.DisplayName != want || item.DataPending {
					t.Fatal(item)
				}
			}
			if publicReads.Load() < 1 || available && publicReads.Load() != 1 {
				t.Fatal("successful public catalog was not cached", publicReads.Load())
			}
		})
	}
}
