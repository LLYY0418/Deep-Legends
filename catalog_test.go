package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEmbeddedPoolHas554UniqueEntries(t *testing.T) {
	data, err := embedded.ReadFile("data/reroll_pool_14_5.txt")
	if err != nil {
		t.Fatal(err)
	}
	names := parsePoolNames(string(data))
	if len(names) != 554 {
		t.Fatalf("pool entries = %d, want 554", len(names))
	}
}

func TestEmbeddedPoolHas554UniqueStableIDs(t *testing.T) {
	data, err := embedded.ReadFile("data/reroll_pool_14_5.json")
	if err != nil {
		t.Fatal(err)
	}
	var source struct {
		Entries []PoolEntry `json:"entries"`
	}
	if err := json.Unmarshal(data, &source); err != nil {
		t.Fatal(err)
	}
	seen := map[int64]bool{}
	textData, err := embedded.ReadFile("data/reroll_pool_14_5.txt")
	if err != nil {
		t.Fatal(err)
	}
	confirmedNames := parsePoolNames(string(textData))
	for _, entry := range source.Entries {
		if entry.ID <= 0 || entry.Name == "" || seen[entry.ID] {
			t.Fatalf("invalid or duplicate stable entry: %#v", entry)
		}
		seen[entry.ID] = true
	}
	if len(source.Entries) != 554 || len(seen) != 554 {
		t.Fatalf("stable entries=%d unique IDs=%d, want 554/554", len(source.Entries), len(seen))
	}
	for index, entry := range source.Entries {
		if entry.Name != confirmedNames[index] {
			t.Fatalf("stable entry %d name=%q, confirmed text=%q", index+1, entry.Name, confirmedNames[index])
		}
	}
}

func TestEmbeddedPoolMapsOneToOneWithoutOmissions(t *testing.T) {
	data, err := embedded.ReadFile("data/reroll_pool_14_5.txt")
	if err != nil {
		t.Fatal(err)
	}
	names := parsePoolNames(string(data))
	skins := make([]Skin, 0, len(names))
	for index, name := range names {
		championID := int64(index + 1)
		skins = append(skins, Skin{ID: championID*1000 + 1, Name: name, ChampionID: championID})
	}
	matched, issues := matchPool(names, skins)
	if len(issues) != 0 || len(matched) != 554 {
		t.Fatalf("matched=%d issues=%#v", len(matched), issues)
	}
}

func TestNormalizeName(t *testing.T) {
	got := normalizeName("  腥红之月·鬼武姬 阿卡丽！ ")
	want := "腥红之月鬼武姬阿卡丽"
	if got != want {
		t.Fatalf("normalizeName() = %q, want %q", got, want)
	}
}

func TestTencentRegionRarityHierarchyAndMythicSubgroups(t *testing.T) {
	cases := []struct {
		object  map[string]any
		tier    string
		subtier string
	}{
		{map[string]any{"regionRarityId": float64(10)}, "圣堂", ""},
		{map[string]any{"regionRarityId": float64(11)}, "卓越", ""},
		{map[string]any{"regionRarityId": float64(8), "emblems": []any{map[string]any{"name": "Prestige"}}}, "神话", "至臻"},
		{map[string]any{"regionRarityId": float64(8), "emblems": []any{map[string]any{"name": "Hextech Limited"}}}, "神话", "海克斯系列"},
		{map[string]any{"regionRarityId": float64(8), "emblems": []any{map[string]any{"name": "Mythic"}}}, "神话", "神话幻想"},
		{map[string]any{"regionRarityId": float64(8), "name": "MVP T1 厄运小姐"}, "神话", "总决赛FMVP系列"},
		{map[string]any{"regionRarityId": float64(8), "name": "殿堂传奇 薇恩"}, "神话", "殿堂系列"},
		{map[string]any{"regionRarityId": float64(7)}, "限定", ""},
		{map[string]any{"regionRarityId": float64(1)}, "典藏", ""},
	}
	for _, item := range cases {
		name := firstString(item.object, "name")
		if name == "" {
			name = "测试皮肤"
		}
		skin := Skin{Name: name, RegionRarityID: firstInt(item.object, "regionRarityId")}
		tier, subtier := classifySkinRarity(item.object, skin)
		if tier != item.tier || subtier != item.subtier {
			t.Fatalf("classify %#v = %q/%q, want %q/%q", item.object, tier, subtier, item.tier, item.subtier)
		}
	}
}

func TestParsePoolNamesDeduplicates(t *testing.T) {
	got := parsePoolNames("# source\n海克斯科技 安妮\n海克斯科技·安妮\n\n青年 瑞兹\n")
	if len(got) != 2 {
		t.Fatalf("parsePoolNames() len = %d, want 2: %#v", len(got), got)
	}
}

func TestExtractOwnedIDs(t *testing.T) {
	fixture := []any{
		map[string]any{"id": float64(1001), "owned": true},
		map[string]any{"id": float64(1002), "owned": false},
		map[string]any{"itemId": "1003", "inventoryType": "CHAMPION_SKIN"},
		map[string]any{"skinId": float64(1004), "ownership": map[string]any{"owned": true}},
	}
	got := extractOwnedIDs(fixture, false)
	for _, id := range []int64{1001, 1004} {
		if !got[id] {
			t.Errorf("expected skin %d to be owned", id)
		}
	}
	for _, id := range []int64{1002, 1003} {
		if got[id] {
			t.Errorf("skin %d must not be owned without explicit evidence", id)
		}
	}

	presence := extractOwnedIDs(fixture, true)
	if !presence[1003] {
		t.Error("skin 1003 should be owned for an inventory endpoint where presence is authoritative")
	}

	available := extractOwnedIDs(map[string]any{"id": float64(1005), "status": "AVAILABLE"}, true)
	if available[1005] {
		t.Error("AVAILABLE must not be interpreted as owned")
	}
}

func TestPresenceInventoryRequiresPermanentChampionSkin(t *testing.T) {
	fixture := []any{
		map[string]any{"itemId": float64(1001), "inventoryType": "CHAMPION_SKIN", "quantity": float64(1)},
		map[string]any{"itemId": float64(1002), "inventoryType": "CHAMPION_SKIN", "freeToPlay": true},
		map[string]any{"itemId": float64(1003), "inventoryType": "CHAMPION_SKIN", "quantity": float64(0)},
		map[string]any{"itemId": float64(1004), "inventoryType": "CHAMPION_SKIN_RENTAL"},
		map[string]any{"itemId": float64(1005)},
		map[string]any{"id": float64(1006), "inventoryType": "CHAMPION_SKIN"},
	}
	got := extractOwnedIDs(fixture, true)
	if !got[1001] || !got[1005] || len(got) != 2 {
		t.Fatalf("scoped presence IDs = %#v, want 1001 and type-implicit item 1005", got)
	}
}

func TestPresenceInventoryReadsTencentItemKeyContainers(t *testing.T) {
	fixture := map[string]any{"inventoryItems": []any{
		map[string]any{"itemKey": map[string]any{"itemId": float64(1001), "inventoryType": "CHAMPION_SKIN"}, "quantity": float64(1)},
		map[string]any{"itemKey": map[string]any{"itemId": float64(1002), "inventoryType": "CHAMPION_SKIN"}, "quantity": float64(0)},
		map[string]any{"itemKey": map[string]any{"itemId": float64(1003), "inventoryType": "CHROMA"}, "quantity": float64(1)},
	}}
	got := extractOwnedIDs(fixture, true)
	if len(got) != 1 || !got[1001] {
		t.Fatalf("itemKey inventory IDs = %#v, want only 1001", got)
	}
}

func TestQuestSkinVariantKeepsIndependentDynamicArtwork(t *testing.T) {
	parent := Skin{ID: 103085, Name: "殿堂传奇 阿狸", ChampionID: 103, ChampionName: "阿狸", RarityTier: "卓越"}
	fixture := map[string]any{"questSkinInfo": map[string]any{"tiers": []any{
		map[string]any{"id": float64(103085), "name": "殿堂传奇 阿狸", "stage": float64(1)},
		map[string]any{"id": float64(103086), "name": "联盟不朽 阿狸", "stage": float64(2), "uncenteredSplashPath": "/lol-game-data/assets/ASSETS/Characters/Ahri/Skins/Skin86/Images/ahri.jpg", "splashVideoPath": "/lol-game-data/assets/ASSETS/Characters/Ahri/Skins/Skin86/AnimatedSplash/Ahri_Skin86_centered.webm"},
	}}}
	variants := questSkinVariants(fixture, parent)
	if len(variants) != 1 || variants[0].ID != 103086 || variants[0].Name != "联盟不朽 阿狸" || variants[0].ParentSkinID != 103085 || !variants[0].IsVariant || !strings.HasSuffix(variants[0].SplashVideoPath, ".webm") {
		t.Fatalf("variants=%#v", variants)
	}
}

func TestOwnedCollectionKeepsQuestVariantsOutOfOrdinarySkinCount(t *testing.T) {
	catalog := []Skin{
		{ID: 103000, ChampionID: 103, Name: "阿狸", Owned: true},
		{ID: 103085, ChampionID: 103, Name: "殿堂传奇 阿狸", Owned: true},
		{ID: 103086, ChampionID: 103, Name: "联盟不朽 阿狸", Owned: true, IsVariant: true, ParentSkinID: 103085},
	}
	owned := ownedCollectionSkins(catalog)
	if len(owned) != 1 || owned[0].ID != 103085 {
		t.Fatalf("ordinary owned count must exclude independently displayed quest variants: %#v", owned)
	}
}

func TestQuestVariantDoesNotInheritParentOwnership(t *testing.T) {
	skins := []Skin{
		{ID: 103000, ChampionID: 103, Name: "阿狸"},
		{ID: 103085, ChampionID: 103, Name: "殿堂传奇 阿狸"},
		{ID: 103086, ChampionID: 103, Name: "联盟不朽 阿狸", IsVariant: true, ParentSkinID: 103085},
		{ID: 145070, ChampionID: 145, Name: "殿堂传奇 卡莎"},
		{ID: 145071, ChampionID: 145, Name: "联盟不朽 卡莎", IsVariant: true, ParentSkinID: 145070},
	}
	applySkinOwnership(skins, map[int64]bool{103085: true, 145070: true}, map[int64]bool{103: true, 145: true})
	if !skins[0].Owned || !skins[1].Owned || skins[2].Owned || !skins[3].Owned || skins[4].Owned {
		t.Fatalf("independent quest variants must require their own inventory IDs: %#v", skins)
	}
}

func TestChromaPresenceExtractionRequiresOwnedPositiveInventory(t *testing.T) {
	fixture := map[string]any{"inventoryItems": []any{
		map[string]any{"itemKey": map[string]any{"itemId": float64(1001001), "inventoryType": "CHROMA"}, "quantity": float64(1)},
		map[string]any{"itemKey": map[string]any{"itemId": float64(1001002), "inventoryType": "CHROMA"}, "quantity": float64(0)},
		map[string]any{"chromaId": float64(1001003), "inventoryType": "CHROMA", "owned": false},
		map[string]any{"chromaId": float64(1001004), "inventoryType": "CHROMA", "owned": true},
	}}
	got := extractChromaPresenceIDs(fixture)
	if len(got) != 2 || !got[1001001] || !got[1001004] {
		t.Fatalf("chroma ownership evidence = %#v, want 1001001 and 1001004", got)
	}
}

func TestTencentClassicArtworkIsNotCountedAsChroma(t *testing.T) {
	chromas := make([]any, 0, 1000)
	for index := int64(0); index < 1000; index++ {
		chromas = append(chromas, map[string]any{
			"id":         1_001_001 + index,
			"name":       "测试炫彩",
			"chromaPath": "/lol-game-data/assets/v1/champion-chroma-images/1/test.png",
		})
	}
	chromas = append(chromas, map[string]any{
		"id": 10082, "name": "Spirit Blossom Kayle (Tanzanite)",
		"chromaPath": "/lol-game-data/assets/v1/champion-chroma-images/10/10082.png",
	})
	fixture := []any{
		map[string]any{"id": 1001, "name": "测试皮肤", "contentId": "parent", "chromas": chromas},
		map[string]any{
			"id": 60_001_301, "name": "经典 安妮", "skinClassification": "kChampion",
			"splashPath": "/lol-game-data/assets/project_jade/annie.jpg",
		},
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(fixture)
	})
	server := httptest.NewServer(handler)
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	got, err := loadChromaCatalog(client, []Skin{{ID: 1001, Name: "测试皮肤", ChampionID: 1, ChampionName: "安妮", SplashPath: "/lol-game-data/assets/test-splash.jpg"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1001 {
		t.Fatalf("chroma catalog size = %d, want 1001", len(got))
	}
	foundPrestige := false
	foundParentArtwork := false
	for _, chroma := range got {
		if chroma.ID == 60_001_301 {
			t.Fatalf("classic artwork was misclassified as chroma: %#v", chroma)
		}
		if chroma.ID == 10082 {
			foundPrestige = chroma.IsPrestige && chroma.Name == "灵魂莲华 凯尔 星回" && chroma.PrestigeImageID != ""
		} else if chroma.ID == 1_001_001 {
			foundParentArtwork = chroma.ParentSplashPath == "/lol-game-data/assets/test-splash.jpg"
		} else if chroma.IsPrestige {
			t.Fatalf("ordinary chroma was misclassified as prestige: %#v", chroma)
		}
	}
	if !foundPrestige {
		t.Fatal("official prestige metadata did not enrich the known Kayle chroma")
	}
	if !foundParentArtwork {
		t.Fatal("ordinary chroma did not retain its parent skin artwork fallback")
	}
}

func TestIndependentArtworkChromaRequiresBothParentEvidenceAndSplash(t *testing.T) {
	parent := Skin{ID: 64001, Name: "至高之拳 李青", ChampionID: 64, ChampionName: "李青", RarityTier: "传说"}
	skins := map[int64]Skin{parent.ID: parent}
	contents := map[string]Skin{"parent-content": parent}
	entry, ok := independentArtworkChroma(map[string]any{
		"id":                    float64(64_001_901),
		"name":                  "至高之拳 李青 臻彩",
		"skinClassification":    "kRecolor",
		"relatedPrimeContentId": "parent-content",
		"splashPath":            "/lol-game-data/assets/ASSETS/Characters/LeeSin/Skins/Chroma/lee_sin_chroma_splash.jpg",
		"chromaPath":            "/lol-game-data/assets/v1/champion-chroma-images/64/64001901.png",
	}, skins, contents)
	if !ok || !entry.IsPrestige || entry.ParentSkinID != parent.ID || entry.SplashPath == "" {
		t.Fatalf("independent artwork chroma = %#v/%v", entry, ok)
	}
	if _, ok := independentArtworkChroma(map[string]any{
		"id": float64(64_001_902), "name": "普通炫彩", "skinClassification": "kRecolor", "relatedPrimeContentId": "parent-content",
	}, skins, contents); ok {
		t.Fatal("a chroma without independent splash art must remain an ordinary nested chroma")
	}
	if _, ok := independentArtworkChroma(map[string]any{
		"id": float64(60_064_301), "name": "经典 李青", "skinClassification": "kChampion",
		"splashPath": "/lol-game-data/assets/project_jade/lee_sin.jpg",
	}, skins, contents); ok {
		t.Fatal("standalone project_jade artwork must not be classified as prestige")
	}
}

func TestOfficialPrestigeCatalogEnrichesKnownChineseChromas(t *testing.T) {
	catalog, err := loadPrestigeCatalog()
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog) < 400 {
		t.Fatalf("prestige catalog size = %d, want at least 400", len(catalog))
	}
	tests := []struct {
		id   int64
		name string
	}{
		{id: 10082, name: "灵魂莲华 凯尔 星回"},
		{id: 114095, name: "女帝 菲奥娜 纹章之刻印·强运"},
	}
	for _, test := range tests {
		chroma := enrichPrestigeChroma(Chroma{ID: test.id, Name: "客户端普通炫彩名"})
		if !chroma.IsPrestige || chroma.Name != test.name || chroma.PrestigeImageID == "" {
			t.Fatalf("prestige chroma %d = %#v", test.id, chroma)
		}
	}
	ordinary := enrichPrestigeChroma(Chroma{ID: 999999999, Name: "普通炫彩"})
	if ordinary.IsPrestige || ordinary.PrestigeImageID != "" || ordinary.Name != "普通炫彩" {
		t.Fatalf("unknown chroma was enriched: %#v", ordinary)
	}
}

func TestChampionSkinsContainerIsTraversed(t *testing.T) {
	fixture := []any{map[string]any{
		"id": float64(1), "owned": true,
		"championSkins": []any{
			map[string]any{"skinId": float64(1001), "owned": true},
			map[string]any{"skinId": float64(1002), "owned": false},
		},
	}}
	got := extractOwnedIDs(fixture, false)
	if !got[1001] || got[1002] {
		t.Fatalf("championSkins ownership was not parsed: %#v", got)
	}
}

func TestConflictingOwnershipEvidenceIsRejected(t *testing.T) {
	fixture := map[string]any{"id": float64(1001), "owned": true, "status": "AVAILABLE"}
	if got := extractOwnedIDs(fixture, false); len(got) != 0 {
		t.Fatalf("conflicting ownership must fail closed: %#v", got)
	}
}

func TestOwnedSourcesMustAgree(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "skins-minimal"):
			_, _ = w.Write([]byte(`[{"id":1001,"owned":true},{"id":2001,"owned":true}]`))
		case r.URL.Path == "/lol-inventory/v2/inventory/CHAMPION_SKIN":
			_, _ = w.Write([]byte(`[{"itemId":1001,"inventoryType":"CHAMPION_SKIN"},{"itemId":2001,"inventoryType":"CHAMPION_SKIN"}]`))
		default:
			http.NotFound(w, r)
		}
	})
	server := httptest.NewServer(handler)
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	catalog := []Skin{{ID: 1001, Name: "皮肤一", ChampionID: 1}, {ID: 2001, Name: "皮肤二", ChampionID: 2}}
	got, failures := loadOwnedSkinIDs(client, 42, catalog)
	if len(got) != 2 {
		t.Fatalf("owned IDs = %#v, failures=%#v", got, failures)
	}
}

func TestPartialOwnedSourceStopsCalculation(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "skins-minimal"):
			_, _ = w.Write([]byte(`[{"id":1001,"owned":true},{"id":2001,"owned":true}]`))
		case r.URL.Path == "/lol-inventory/v2/inventory/CHAMPION_SKIN":
			_, _ = w.Write([]byte(`[{"itemId":1001,"inventoryType":"CHAMPION_SKIN"}]`))
		default:
			http.NotFound(w, r)
		}
	})
	server := httptest.NewServer(handler)
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	catalog := []Skin{{ID: 1001, Name: "皮肤一", ChampionID: 1}, {ID: 2001, Name: "皮肤二", ChampionID: 2}}
	got, failures := loadOwnedSkinIDs(client, 42, catalog)
	if len(got) != 2 || len(failures) != 1 || !strings.Contains(failures[0], "presence-only audit differs") {
		t.Fatalf("explicit full-coverage source should survive a partial presence audit: got=%#v failures=%#v", got, failures)
	}
}

func TestAuthoritativeOwnedSourcesStillMustAgree(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "skins-minimal"):
			_, _ = w.Write([]byte(`[{"id":1001,"owned":true},{"id":2001,"owned":false}]`))
		case strings.HasSuffix(r.URL.Path, "/champions"):
			_, _ = w.Write([]byte(`[{"championSkins":[{"skinId":1001,"owned":false},{"skinId":2001,"owned":true}]}]`))
		default:
			http.NotFound(w, r)
		}
	})
	server := httptest.NewServer(handler)
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	catalog := []Skin{{ID: 1001, Name: "皮肤一", ChampionID: 1}, {ID: 2001, Name: "皮肤二", ChampionID: 2}}
	got, statuses, err := loadOwnedSkinInventory(client, 42, catalog)
	if err == nil || len(got) != 0 || countSourceState(statuses, "conflict") != 1 {
		t.Fatalf("authoritative disagreement must stop calculation: got=%#v statuses=%#v err=%v", got, statuses, err)
	}
}

func TestCatalogIntersectionIgnoresBaseChromaAndAttachmentIDs(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "skins-minimal"):
			_, _ = w.Write([]byte(`[
				{"id":1001,"owned":true},
				{"id":2001,"owned":false},
				{"id":1000,"owned":true},
				{"skinId":1001001,"owned":true}
			]`))
		default:
			http.NotFound(w, r)
		}
	})
	server := httptest.NewServer(handler)
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	catalog := []Skin{
		{ID: 1000, Name: "基础皮肤", ChampionID: 1},
		{ID: 1001, Name: "皮肤一", ChampionID: 1},
		{ID: 2001, Name: "皮肤二", ChampionID: 2},
	}
	got, statuses, err := loadOwnedSkinInventory(client, 42, catalog)
	if err != nil || len(got) != 1 || !got[1001] {
		t.Fatalf("catalog intersection should retain only active owned skins: got=%#v statuses=%#v err=%v", got, statuses, err)
	}
	if countSourceState(statuses, "warning") != 1 || statuses[0].BaseCount != 1 || statuses[0].UnknownCount != 1 || statuses[0].RawOwnedCount != 3 {
		t.Fatalf("out-of-catalog IDs must remain visible as a non-fatal audit: %#v", statuses)
	}
}

func TestOwnershipDiagnosticsExposeOnlyPublicInventoryIDs(t *testing.T) {
	fixture := []any{
		map[string]any{"id": float64(1001), "ownership": map[string]any{"owned": true}},
		map[string]any{"id": float64(1002), "ownership": map[string]any{"owned": false, "rental": map[string]any{"rented": true}}},
		map[string]any{"id": float64(1003), "ownership": map[string]any{"owned": false, "freeToPlayReward": true}},
	}
	evidence := extractOwnedEvidence(fixture, false)
	if !evidence.OwnedIDs[1001] || !evidence.RentalIDs[1002] || !evidence.FreeToPlayIDs[1003] {
		t.Fatalf("ownership classifications were not retained for diagnostics: %#v", evidence)
	}
	status := ownershipSourceStatus("/test", "success", map[int64]bool{1001: true}, evidence, map[int64]bool{}, map[int64]bool{}, map[int64]bool{9000001: true}, "", 3)
	if status.RentalCount != 1 || status.FreeToPlayCount != 1 || len(status.UnknownIDs) != 1 || status.UnknownIDs[0] != 9000001 || len(status.CatalogOwnedIDHash) != 64 {
		t.Fatalf("inventory diagnostic is incomplete: %#v", status)
	}
}

func TestCorroboratedExplicitSourceSurvivesSecondarySchemaDifference(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "skins-minimal"):
			_, _ = w.Write([]byte(`[{"id":1001,"owned":true},{"id":2001,"owned":false},{"id":1001001,"owned":true}]`))
		case strings.HasSuffix(r.URL.Path, "/champions"):
			_, _ = w.Write([]byte(`[{"championSkins":[{"skinId":1001,"owned":true},{"skinId":2001,"owned":true}]}]`))
		case r.URL.Path == "/lol-inventory/v2/inventory/CHAMPION_SKIN":
			_, _ = w.Write([]byte(`[{"itemId":9000001,"skin":{"skinId":1001}},{"skinId":1001,"inventoryType":"CHAMPION_SKIN"}]`))
		default:
			http.NotFound(w, r)
		}
	})
	server := httptest.NewServer(handler)
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	catalog := []Skin{{ID: 1001, Name: "皮肤一", ChampionID: 1}, {ID: 2001, Name: "皮肤二", ChampionID: 2}}
	got, statuses, err := loadOwnedSkinInventory(client, 42, catalog)
	if err != nil || len(got) != 1 || !got[1001] {
		t.Fatalf("corroborated primary evidence should win: got=%#v statuses=%#v err=%v", got, statuses, err)
	}
	if countSourceState(statuses, "warning") < 2 {
		t.Fatalf("schema differences must remain visible as warnings: %#v", statuses)
	}
}

func TestVerifiedInventorySupersetCompletesTencentOwnedCollection(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "skins-minimal"):
			_, _ = w.Write([]byte(`[{"id":1001,"owned":true},{"id":2001,"owned":false},{"id":3001,"owned":false}]`))
		case strings.HasSuffix(r.URL.Path, "/champions"):
			_, _ = w.Write([]byte(`[{"championSkins":[{"skinId":1001,"owned":true},{"skinId":2001,"owned":true},{"skinId":3001,"owned":false}]}]`))
		case r.URL.Path == "/lol-inventory/v2/inventory/CHAMPION_SKIN":
			_, _ = w.Write([]byte(`{"inventoryItems":[{"itemKey":{"itemId":1001,"inventoryType":"CHAMPION_SKIN"},"quantity":1},{"itemKey":{"itemId":2001,"inventoryType":"CHAMPION_SKIN"},"quantity":1},{"itemKey":{"itemId":3001,"inventoryType":"CHAMPION_SKIN"},"quantity":1}]}`))
		default:
			http.NotFound(w, r)
		}
	})
	server := httptest.NewServer(handler)
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	catalog := []Skin{{ID: 1001, Name: "皮肤一", ChampionID: 1}, {ID: 2001, Name: "皮肤二", ChampionID: 2}, {ID: 3001, Name: "皮肤三", ChampionID: 3}}
	got, statuses, err := loadOwnedSkinInventory(client, 42, catalog)
	if err != nil || len(got) != 3 || !got[1001] || !got[2001] || !got[3001] {
		t.Fatalf("verified inventory superset should complete the collection: got=%#v statuses=%#v err=%v", got, statuses, err)
	}
}

func TestTencentInventoryEvidencePatternFromDiagnosticLog(t *testing.T) {
	catalog := make([]Skin, 0, 1935)
	for index := 0; index < 1935; index++ {
		championID := int64(index/900 + 1)
		skinID := championID*1000 + int64(index%900+1)
		catalog = append(catalog, Skin{ID: skinID, Name: "测试皮肤", ChampionID: championID})
	}
	explicitPayload := func(ownedCount, unknownCount int) []byte {
		items := make([]map[string]any, 0, len(catalog)+unknownCount)
		for index, skin := range catalog {
			items = append(items, map[string]any{"id": skin.ID, "owned": index < ownedCount})
		}
		for index := 0; index < unknownCount; index++ {
			items = append(items, map[string]any{"skinId": int64(9_000_000 + index), "owned": true})
		}
		data, err := json.Marshal(items)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	presencePayload := func(ownedCount, unknownCount int) []byte {
		items := make([]map[string]any, 0, ownedCount+unknownCount)
		for index := 0; index < ownedCount; index++ {
			items = append(items, map[string]any{"itemId": catalog[index].ID, "inventoryType": "CHAMPION_SKIN"})
		}
		for index := 0; index < unknownCount; index++ {
			items = append(items, map[string]any{"itemId": int64(8_000_000 + index)})
		}
		data, err := json.Marshal(items)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	minimal := explicitPayload(1125, 452)
	champions := explicitPayload(1127, 659)
	inventory := presencePayload(1125, 1628)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "skins-minimal"):
			_, _ = w.Write(minimal)
		case strings.HasSuffix(r.URL.Path, "/champions"):
			_, _ = w.Write(champions)
		case r.URL.Path == "/lol-inventory/v2/inventory/CHAMPION_SKIN":
			_, _ = w.Write(inventory)
		default:
			http.NotFound(w, r)
		}
	})
	server := httptest.NewServer(handler)
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	got, statuses, err := loadOwnedSkinInventory(client, 42, catalog)
	if err != nil || len(got) != 1125 {
		t.Fatalf("Tencent evidence pattern should resolve to the corroborated 1125 catalog skins: got=%d statuses=%#v err=%v", len(got), statuses, err)
	}
	if len(statuses) != 4 || statuses[0].UnknownCount != 452 || statuses[1].UnknownCount != 659 || statuses[2].UnknownCount != 1628 {
		t.Fatalf("diagnostic unknown counts were not preserved: %#v", statuses)
	}
	if countSourceState(statuses, "warning") != 3 || countSourceState(statuses, "unsupported") != 1 {
		t.Fatalf("expected three non-fatal audits and one unsupported endpoint: %#v", statuses)
	}
	for _, status := range statuses {
		if strings.Contains(status.Path, "/42/") {
			t.Fatalf("diagnostic endpoint path leaked the summoner ID: %q", status.Path)
		}
	}
}

func TestMatchPoolFailsClosed(t *testing.T) {
	skins := []Skin{
		{ID: 1001, Name: "海克斯科技 安妮", ChampionID: 1},
		{ID: 1002, Name: "青年 瑞兹", ChampionID: 13},
	}
	matched, issues := matchPool([]string{"海克斯科技 安妮", "不存在的皮肤 阿狸"}, skins)
	if len(matched) != 1 {
		t.Fatalf("matched len = %d, want 1", len(matched))
	}
	if len(issues) != 1 || issues[0].Name != "不存在的皮肤 阿狸" {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestStablePoolMatchesByIDWhenClientNameChanges(t *testing.T) {
	pool, err := validatePoolManifest(PoolManifest{Name: "稳定奖池", Entries: []PoolEntry{{ID: 1001, Name: "公告旧名称 安妮"}}})
	if err != nil {
		t.Fatal(err)
	}
	matched, issues := matchPoolManifest(pool, []Skin{{ID: 1001, Name: "客户端新名称 安妮", ChampionID: 1}})
	if len(issues) != 0 || len(matched) != 1 || matched[0].PoolName != "公告旧名称 安妮" {
		t.Fatalf("stable ID mapping failed: matched=%#v issues=%#v", matched, issues)
	}
	_, issues = matchPoolManifest(pool, []Skin{{ID: 1002, Name: "其他皮肤 安妮", ChampionID: 1}})
	if len(issues) != 1 {
		t.Fatalf("missing stable ID must fail closed: %#v", issues)
	}
	chromaPool, err := validatePoolManifest(PoolManifest{Name: "炫彩误入", Entries: []PoolEntry{{ID: 1001001, Name: "主皮肤 朝云"}}})
	if err != nil {
		t.Fatal(err)
	}
	matched, issues = matchPoolManifest(chromaPool, []Skin{{ID: 1001001, Name: "主皮肤 朝云", ChampionID: 1001}})
	if len(matched) != 0 || len(issues) != 1 {
		t.Fatalf("chroma-like stable ID must not match an ordinary skin: matched=%#v issues=%#v", matched, issues)
	}
}

func TestPoolMembershipIsAnnotatedOnAllCatalogSkins(t *testing.T) {
	all := []Skin{{ID: 1001, Name: "奖池皮肤", ChampionID: 1}, {ID: 1002, Name: "普通皮肤", ChampionID: 1}}
	matched := []Skin{{ID: 1001, Name: "奖池皮肤", ChampionID: 1, PoolName: "公告名称"}}
	annotatePoolMembership(all, matched)
	if all[0].PoolName != "公告名称" || all[1].PoolName != "" {
		t.Fatalf("all=%#v", all)
	}
}

func TestMatchPoolDoesNotApplyUnverifiedCorrection(t *testing.T) {
	skins := []Skin{
		{ID: 1001, Name: "腥红岁月 努努和威朗普", ChampionID: 20},
		{ID: 1002, Name: "丧尸 努努和威朗普", ChampionID: 20},
	}
	matched, issues := matchPool([]string{"腥红岁 努努和威朗普"}, skins)
	if len(matched) != 0 || len(issues) != 1 {
		t.Fatalf("unverified correction must fail closed: matched=%#v issues=%#v", matched, issues)
	}
}

func TestMatchPoolRejectsCrossChampionCorrection(t *testing.T) {
	skins := []Skin{{ID: 133001, Name: "血羽凤凰 奎因", ChampionID: 133}}
	matched, issues := matchPool([]string{"血羽剑皇 亚索"}, skins)
	if len(matched) != 0 || len(issues) != 1 {
		t.Fatalf("cross-champion correction must fail closed: matched=%#v issues=%#v", matched, issues)
	}
}

func TestMatchPoolRejectsNormalizedNameCollision(t *testing.T) {
	skins := []Skin{
		{ID: 1001, Name: "测试·皮肤 安妮", ChampionID: 1},
		{ID: 1002, Name: "测试皮肤 安妮", ChampionID: 1},
	}
	matched, issues := matchPool([]string{"测试皮肤 安妮"}, skins)
	if len(matched) != 0 || len(issues) != 1 || len(issues[0].Candidates) != 2 {
		t.Fatalf("normalized collision must fail closed: matched=%#v issues=%#v", matched, issues)
	}
}

func TestCatalogObjectsIgnoreNestedChromas(t *testing.T) {
	root := map[string]any{
		"1001": map[string]any{
			"id": float64(1001), "name": "主皮肤",
			"chromas": []any{map[string]any{"id": float64(1001001), "name": "炫彩"}},
		},
	}
	objects := catalogObjects(root)
	if len(objects) != 1 || firstInt(objects[0], "id") != 1001 {
		t.Fatalf("catalog objects = %#v", objects)
	}
}

func TestChromaCatalogRecordsAreRejected(t *testing.T) {
	if !isChromaCatalogObject(map[string]any{"id": float64(1001001), "skinClassification": "kChroma", "parentSkinId": float64(1001)}) {
		t.Fatal("explicit chroma metadata must be recognized")
	}
	if isChromaCatalogObject(map[string]any{"id": float64(1001), "skinClassification": "kChampion", "chromaPath": "/has/chromas"}) {
		t.Fatal("a regular skin that owns chromas must not itself be classified as a chroma")
	}
	if validCatalogSkinIdentity(1001001, 1001) {
		t.Fatal("seven-digit chroma ID must not enter the ordinary skin catalog")
	}
}

func TestBaseSkinRequiresChampionIdentity(t *testing.T) {
	if isBaseSkin(Skin{ID: 2000, ChampionID: 1, Name: "合法非基础皮肤"}) {
		t.Error("ID ending in 000 alone must not mark a skin as base")
	}
	if !isBaseSkin(Skin{ID: 2000, ChampionID: 2, Name: "默认"}) {
		t.Error("championID*1000 must be recognized as the base skin")
	}
}

func TestLegacyChampionCatalogEntriesAreRejected(t *testing.T) {
	if validCatalogSkinIdentity(60013001, 60013) {
		t.Error("archived legacy champion skin IDs must not enter the active catalog")
	}
	if !validCatalogSkinIdentity(13001, 13) {
		t.Error("active champion skin ID should be accepted")
	}
}

func TestCatalogJSONNumbersRemainSupported(t *testing.T) {
	var value map[string]any
	decoder := json.NewDecoder(strings.NewReader(`{"id":1001}`))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		t.Fatal(err)
	}
	if firstInt(value, "id") != 1001 {
		t.Fatalf("firstInt failed for json.Number: %#v", value)
	}
}

func TestMatchPoolDoesNotAutoAcceptFuzzyOnly(t *testing.T) {
	skins := []Skin{{ID: 1001, Name: "海克斯科技 安妮", ChampionID: 1}}
	matched, issues := matchPool([]string{"海克斯技科 安妮"}, skins)
	if len(matched) != 0 || len(issues) != 1 {
		t.Fatalf("fuzzy-only match must fail closed: matched=%#v issues=%#v", matched, issues)
	}
	if len(issues[0].Candidates) != 1 || issues[0].Candidates[0] != "海克斯科技 安妮" {
		t.Fatalf("expected fuzzy suggestion, got %#v", issues[0])
	}
}

func TestLevenshtein(t *testing.T) {
	if got := levenshtein([]rune("kitten"), []rune("sitting")); got != 3 {
		t.Fatalf("levenshtein = %d, want 3", got)
	}
}
