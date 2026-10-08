package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
)

func TestR151YourGGAugmentsKeepOfficialGradesScoresAndCatalogRarity(t *testing.T) {
	// The field names and nesting mirror the live YOUR.GG aggregate response.
	var payload yourGGArenaAggregateResponse
	fixture := `{"response":{"augments":[{"augmentId":901,"tier":"OP","score":96.5,"winRate":0.6,"averagePlacement":2.8,"firstPlacementRate":0.3,"matches":400},{"augmentId":902,"tier":"S","score":88.2,"winRate":0.55,"averagePlacement":3.1,"firstPlacementRate":0.2,"matches":300},{"augmentId":903,"tier":"A","score":77.4,"winRate":0.52,"averagePlacement":3.4,"firstPlacementRate":0.1,"matches":200},{"augmentId":999,"tier":"B","score":70.1,"winRate":0.5,"averagePlacement":3.6,"firstPlacementRate":0.08,"matches":100}],"coreItems":[],"prismaticItems":[]},"statusCode":200,"success":true}`
	if err := json.Unmarshal([]byte(fixture), &payload); err != nil {
		t.Fatal(err)
	}
	catalog := []gameplayAugment{
		{ID: 901, Name: "银色测试", Rarity: "kSilver", IconPath: "/lol-game-data/assets/ASSETS/Augments/silver.png"},
		{ID: 902, Name: "黄金测试", Rarity: "kGold", IconPath: "/lol-game-data/assets/ASSETS/Augments/gold.png"},
		{ID: 903, Name: "棱彩测试", Rarity: "kPrismatic", IconPath: "/lol-game-data/assets/ASSETS/Augments/prismatic.png"},
	}
	groups, _, err := mapYourGGArenaAggregateAugmentsWithStats(payload.Response.Augments, catalog)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 4 || groups[0].Rarity != 1 || groups[1].Rarity != 4 || groups[2].Rarity != 8 || groups[3].Rarity != -1 {
		t.Fatalf("official augment quality groups = %+v", groups)
	}
	for i, want := range []struct {
		grade  string
		score  float64
		rarity string
	}{
		{"OP", 96.5, "silver"}, {"S", 88.2, "gold"}, {"A", 77.4, "prismatic"}, {"B", 70.1, "unknown"},
	} {
		row := groups[i].Rows[0]
		if row.Tier != want.grade || row.Grade != want.grade || row.Score != want.score || row.Rarity != want.rarity || row.Games == 0 || row.PickRate != 0 {
			t.Fatalf("official augment row %d = %+v", i, row)
		}
		if i < 3 && (row.Assets[0].Source != "communitydragon" || !strings.HasPrefix(row.Assets[0].Path, "/latest/game/")) {
			t.Fatalf("catalog asset %d = %+v", i, row.Assets[0])
		}
	}
	if got := groups[0].Rows[0].WinRate; got != 60 {
		t.Fatalf("fractional win rate = %v", got)
	}
	if got := groups[0].Rows[0].FirstPlaceRate; got != 30 {
		t.Fatalf("fractional first place rate = %v", got)
	}
	unknown := groups[3].Rows[0].Assets[0]
	if unknown.Name != "海克斯 999" || unknown.Source != "" || unknown.Path != "" {
		t.Fatalf("missing catalog ID fallback = %+v", unknown)
	}
}

func TestR151PartialYourGGAugmentsFallBackAsAWhole(t *testing.T) {
	var payload yourGGArenaAggregateResponse
	if err := json.Unmarshal([]byte(`{"response":{"augments":[{"augmentId":901,"tier":"S","score":80,"matches":100}]}}`), &payload); err != nil {
		t.Fatal(err)
	}
	catalog := []gameplayAugment{{ID: 901, Rarity: "kSilver"}}
	if _, _, err := mapYourGGArenaAggregateAugmentsWithStats(payload.Response.Augments, catalog); err == nil {
		t.Fatal("one quality must not replace the whole OP.GG augment list")
	}
	for _, malformed := range []string{
		`{"augmentId":901,"tier":"S","matches":100}`,
		`{"augmentId":901,"tier":"E","score":80,"matches":100}`,
	} {
		var row yourGGArenaAggregateAugment
		if err := json.Unmarshal([]byte(malformed), &row); err != nil {
			t.Fatal(err)
		}
		if _, _, err := mapYourGGArenaAggregateAugmentsWithStats([]yourGGArenaAggregateAugment{row}, catalog); err == nil {
			t.Fatalf("incomplete upstream row was accepted: %s", malformed)
		}
	}
}

func TestR151StructuredArenaUsesYourGGAugmentsAndFallsBackToOPGG(t *testing.T) {
	for _, test := range []struct {
		name     string
		upstream string
		official bool
	}{
		{"official", `{"success":true,"statusCode":200,"response":{"version":"16.19","coreItems":[],"prismaticItems":[],"augments":[{"augmentId":901,"tier":"OP","score":96.5,"winRate":0.6,"averagePlacement":2.8,"firstPlacementRate":0.3,"matches":400},{"augmentId":902,"tier":"S","score":88.2,"winRate":0.55,"averagePlacement":3.1,"firstPlacementRate":0.2,"matches":300},{"augmentId":903,"tier":"A","score":77.4,"winRate":0.52,"averagePlacement":3.4,"firstPlacementRate":0.1,"matches":200}]}}`, true},
		{"aggregate failure", "", false},
		{"missing augments", `{"success":true,"statusCode":200,"response":{"version":"16.19","coreItems":[],"prismaticItems":[]}}`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			provider := newChampionProvider()
			provider.cache = newChampionDataCache(nil)
			provider.patch = "16.19.1"
			// These synthetic IDs are deliberately outside the bundled catalog.
			// Seed the cached supplement so the source-selection test still
			// exercises YOUR.GG mapping without a network catalog dependency.
			provider.remoteAugments = []gameplayAugment{
				{ID: 901, Name: "银色测试", Rarity: "kSilver"},
				{ID: 902, Name: "黄金测试", Rarity: "kGold"},
				{ID: 903, Name: "棱彩测试", Rarity: "kPrismatic"},
				{ID: 904, Name: "回退测试", Rarity: "kSilver"},
			}
			var eventsMu sync.Mutex
			events := make([]map[string]any, 0)
			provider.diag = func(event map[string]any) {
				eventsMu.Lock()
				events = append(events, event)
				eventsMu.Unlock()
			}
			provider.static["item/3153.png"] = championAssetDescription{Name: "测试装备"}
			provider.client = &http.Client{Transport: championRoundTripFunc(func(request *http.Request) (*http.Response, error) {
				key := request.URL.Host + request.URL.Path
				var body string
				status := http.StatusOK
				switch key {
				case opggChampionHost + "/api/global/champions/arena/versions":
					body = `{"data":["16.19"]}`
				case opggChampionHost + "/api/global/champions/arena/67":
					body = `{"data":{"summary":{"id":67,"average_stats":{"play":1000,"win":550}},"core_items":[{"ids":[3153],"play":100,"win":60}],"augment_group":[{"rarity":1,"augments":[{"id":904,"play":100,"win":55,"pick_rate":0.1}]}]},"meta":{"version":"16.19"}}`
				case yourGGArenaHost + "/kr/api/arena/champions/67":
					if test.upstream == "" {
						status = http.StatusServiceUnavailable
						body = `{"error":"unavailable"}`
					} else {
						body = test.upstream
					}
				case communityDragonHost + "/latest/plugins/rcp-be-lol-game-data/global/zh_cn/v1/cherry-augments.json":
					body = `[{"id":901,"nameTRA":"银色测试","rarity":"kSilver","iconPath":"/lol-game-data/assets/ASSETS/Augments/silver.png"},{"id":902,"nameTRA":"黄金测试","rarity":"kGold","iconPath":"/lol-game-data/assets/ASSETS/Augments/gold.png"},{"id":903,"nameTRA":"棱彩测试","rarity":"kPrismatic","iconPath":"/lol-game-data/assets/ASSETS/Augments/prismatic.png"},{"id":904,"nameTRA":"回退测试","rarity":"kSilver"}]`
				case communityDragonHost + "/latest/cdragon/arena/zh_cn.json":
					body = `{"augments":[]}`
				case communityDragonHost + "/latest/plugins/rcp-be-lol-game-data/global/zh_cn/v1/items.json":
					body = `[{"id":3153,"name":"测试装备"}]`
				default:
					return nil, fmt.Errorf("unexpected R151 request: %s", key)
				}
				return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
			})}
			response, err := provider.loadStructuredDetail(context.Background(), "arena", "67", "", "")
			if err != nil {
				t.Fatal(err)
			}
			rows := response.ArenaAugments
			if test.official {
				if !strings.Contains(response.Source, "YOUR.GG aggregate") {
					t.Fatalf("official augment source = %q", response.Source)
				}
				if len(rows) != 3 || len(response.ArenaAugmentGroups) != 3 {
					t.Fatalf("YOUR.GG augment coverage = %+v", rows)
				}
				for index, grade := range []string{"OP", "S", "A"} {
					if rows[index].Grade != grade || rows[index].Tier != grade {
						t.Fatalf("official grade %d = %+v, want %s", index, rows[index], grade)
					}
				}
				for _, row := range rows {
					if row.Score <= 0 || row.Grade == "" || row.Assets[0].ID == 904 {
						t.Fatalf("OP.GG or local score leaked into YOUR.GG path: %+v", row)
					}
				}
			} else {
				if len(rows) != 1 || rows[0].Assets[0].ID != 904 || rows[0].Score != 0 || rows[0].Tier != "" || rows[0].Grade == "" {
					t.Fatalf("OP.GG fallback augment = %+v", rows)
				}
			}
			eventsMu.Lock()
			eventsSnapshot := append([]map[string]any(nil), events...)
			eventsMu.Unlock()
			foundSource := false
			for _, event := range eventsSnapshot {
				if event["event"] != "arena_augment_source" {
					continue
				}
				foundSource = true
				wantSource := "op.gg"
				if test.official {
					wantSource = "your.gg"
				}
				if event["source"] != wantSource {
					t.Fatalf("augment diagnostic = %+v, want %s", event, wantSource)
				}
				for _, forbidden := range []string{"cookie", "token", "requestBody", "score", "winRate"} {
					if _, ok := event[forbidden]; ok {
						t.Fatalf("sensitive augment diagnostic field %q: %+v", forbidden, event)
					}
				}
			}
			if !foundSource {
				t.Fatalf("missing arena_augment_source diagnostic: %+v", eventsSnapshot)
			}
		})
	}
}
