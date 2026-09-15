package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func Test1945FacadeApplyUsesDisplayedCatalogAndCanonicalChampionIdentity(t *testing.T) {
	// Cold career path: Collection has not been opened. Include the actual
	// HoL quest tiers and chromas, plus enough unrelated rows for catalog guards.
	data, err := os.ReadFile("testdata/diagnostics-1945/hall-of-legends-skins.json")
	if err != nil {
		t.Fatal(err)
	}
	var catalog map[string]any
	if err := json.Unmarshal(data, &catalog); err != nil {
		t.Fatal(err)
	}
	for champion := 1; champion <= 110; champion++ {
		if champion == 67 || champion == 103 {
			continue
		}
		for skin := 0; skin < 10; skin++ {
			id := champion*1000 + skin
			name := fmt.Sprintf("Champion %d", champion)
			if skin == 1 {
				name = "至臻 测试 (2022)"
			}
			if skin == 2 {
				name = "司马懿 仲达"
			}
			catalog[fmt.Sprint(id)] = map[string]any{"id": id, "name": name}
		}
	}
	body, _ := json.Marshal(catalog)
	reads, writes := 0, []int64{}
	client := &LCUClient{baseURL: "https://fixture", token: "fixture-token", http: &http.Client{Transport: championRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method == http.MethodGet && r.URL.Path == "/lol-game-data/assets/v1/skins.json" {
			reads++
			return proHTTPBody(body), nil
		}
		if r.Method == http.MethodPost && r.URL.Path == "/lol-summoner/v1/current-summoner/summoner-profile" {
			var request struct {
				Key   string
				Value int64
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Fatal(err)
			}
			if request.Key != "backgroundSkinId" {
				t.Fatal(request)
			}
			writes = append(writes, request.Value)
			return proHTTPBody([]byte(`{}`)), nil
		}
		return nil, fmt.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
	})}}
	a := &app{}
	skins, _, err := a.loadFacadeSkins(context.Background(), client)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[int64]Skin{}
	for _, skin := range skins {
		byID[skin.ID] = skin
		if skin.ChampionName == "(2022)" || skin.ChampionName == "仲达" || skin.ChampionName == "" {
			t.Fatalf("invalid champion identity: %+v", skin)
		}
	}
	for _, skin := range skins {
		if skin.ChampionName != byID[skin.ChampionID*1000].Name {
			t.Fatalf("skin name contaminated champion identity: %+v", skin)
		}
	}
	for _, id := range []int64{103086, 145071} {
		if !byID[id].IsVariant {
			t.Fatalf("quest variant missing: %d", id)
		}
		if err := a.applyFacadeAction(context.Background(), client, Summoner{}, facadeApplyRequest{Action: "background", SkinID: id}); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []int64{67065, 67066, 999999} {
		if err := a.applyFacadeAction(context.Background(), client, Summoner{}, facadeApplyRequest{Action: "background", SkinID: id}); !errors.Is(err, errFacadeInvalid) {
			t.Fatalf("unknown/chroma accepted %d: %v", id, err)
		}
	}
	if reads != 1 || len(a.allSkins) != 0 || !reflect.DeepEqual(writes, []int64{103086, 145071}) {
		t.Fatalf("reads=%d writes=%v", reads, writes)
	}
}

func Test1945YourGGArenaRankingsMatchRealPage(t *testing.T) {
	data, err := os.ReadFile("testdata/diagnostics-1945/yourgg-arena-rankings.json")
	if err != nil {
		t.Fatal(err)
	}
	p := newChampionProvider()
	p.client = &http.Client{Transport: championRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != yourGGArenaHost || r.URL.Path != "/kr/api/arena/champions" {
			t.Fatalf("wrong ranking source: %s", r.URL)
		}
		return proHTTPBody(data), nil
	})}
	got, err := p.loadArenaRankings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.Source != "YOUR.GG" || got.Region != "KR" || got.Patch != "16.17" || len(got.Rows) != 173 {
		t.Fatalf("wrong source/sample: %+v", got)
	}
	for i, id := range []int{3, 799, 50, 8, 14, 526, 103, 102} {
		if got.Rows[i].ChampionID != id || got.Rows[i].Rank != i+1 || got.Rows[i].Tier != 0 {
			t.Fatal(got.Rows[i])
		}
	}
	if got.Rows[0].WinRate < 54.51 || got.Rows[0].WinRate > 54.53 {
		t.Fatal(got.Rows[0])
	}
	for _, bad := range [][]byte{bytes.Replace(data, []byte(`"success":true`), []byte(`"success":false`), 1)} {
		if _, err := parseYourGGArenaRankings(bad, time.Now()); err == nil {
			t.Fatal("invalid response accepted")
		}
	}
}

func Test1945ProShardMultiplicityAndAmbiguity(t *testing.T) {
	for _, tc := range []struct {
		ids, want []int64
		complete  bool
	}{
		{[]int64{5008, 5011}, []int64{5008, 5008, 5011}, true},
		{[]int64{5008, 5001}, []int64{5008, 0, 5001}, false},
		{[]int64{5008, 5001, 5011}, []int64{5008, 5001, 5011}, true},
		{[]int64{5008, 5001, 5001}, []int64{5008, 5001, 5001}, true},
		{[]int64{5008, 5011, 5011}, []int64{0, 0, 0}, false},
		{[]int64{5001, 5011}, []int64{0, 0, 0}, false},
		{[]int64{5008, 5008}, []int64{0, 0, 0}, false},
	} {
		got, ok := proRuneShards(append([]int64{8437, 8401, 8444, 8451, 8347, 8345}, tc.ids...))
		if ok != tc.complete || !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("%v => %v %v", tc.ids, got, ok)
		}
	}
}

func Test1945DropBearArtworkAcrossClientAndWebCatalogs(t *testing.T) {
	for _, file := range []string{"drop_bear_small.png", "drop_bear_large.png"} {
		path, fallback := normalizeAugmentIconPaths("/lol-game-data/assets/ASSETS/UX/Cherry/Augments/Icons/" + file)
		if !strings.HasSuffix(path, "/drop_bear.png") || !strings.HasSuffix(fallback, "/drop_bear_small.png") {
			t.Fatalf("wrong artwork %s / %s", path, fallback)
		}
	}
}

// The user's working Akari file is the byte-level serialization contract,
// including field order and UTF-8. This is NOT a game renderer acceptance test.
func Test1945RecommendedSerializationMatchesUserAkariFileByteForByte(t *testing.T) {
	data, err := os.ReadFile("testdata/diagnostics-1945/akari-user-verified.json")
	if err != nil {
		t.Fatal(err)
	}
	var set recommendedItemSet
	if err := json.Unmarshal(data, &set); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(set)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, encoded) {
		t.Fatal("serialized game file diverged from the known-working Akari format")
	}
	// Repeated consumables and source order must survive the actual file encoder.
	if len(set.Blocks) != 9 || set.Blocks[0].Items[1].ID != "2003" || set.Blocks[0].Items[2].ID != "2003" {
		t.Fatal("reference block content changed")
	}
}

func Test1945ProSupplementRetriesNetworkNotIdentityAndPublishesIndependently(t *testing.T) {
	fixture := func(player, realName string) []byte {
		return []byte(`<h1>` + player + `</h1><table><tr><td>Name</td><td>` + realName + `</td></tr></table><div><h4>Accounts</h4><table><tr><td>[KR] example#KR1</td><td>Master 200LP</td></tr></table></div>`)
	}
	var mu sync.Mutex
	calls := map[string]int{}
	var events []map[string]any
	p := newChampionProvider()
	p.diag = func(event map[string]any) { mu.Lock(); defer mu.Unlock(); events = append(events, event) }
	p.client = &http.Client{Transport: championRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		name := strings.TrimPrefix(r.URL.Path, "/player/")
		mu.Lock()
		calls[name]++
		attempt := calls[name]
		mu.Unlock()
		if name == "TheShy" && attempt == 1 {
			return nil, context.DeadlineExceeded
		}
		_, player, _ := proSupplementPlayer(name)
		if name == "Assum" {
			return proHTTPBody(fixture("Unrelated", player.Names[0])), nil
		}
		return proHTTPBody(fixture(name, player.Names[0])), nil
	})}
	var snapshots [][]opggProTeam
	got := loadProSupplements(context.Background(), p, func(teams []opggProTeam) { snapshots = append(snapshots, teams) })
	if calls["TheShy"] != 2 || calls["Assum"] != 1 || len(snapshots) != 3 || len(got[0].Members[0].Summoners) != 1 || !got[1].Members[0].Incomplete {
		t.Fatalf("calls=%v snapshots=%d got=%+v", calls, len(snapshots), got)
	}
	if len(snapshots[0][0].Members[0].Summoners) != 0 {
		t.Fatal("late completion mutated published snapshot")
	}
	for _, event := range events {
		b, _ := json.Marshal(event)
		if strings.Contains(string(b), "example") || strings.Contains(string(b), "TheShy") {
			t.Fatal("account data leaked in diagnostics")
		}
	}
}

// Opt-in live checks exercise the same production transports/parsers, not
// fixture PUUIDs or a copied JSON response. Never sends local account data.
func Test1945LivePublicSources(t *testing.T) {
	if os.Getenv("DEEP_LEGENDS_1945_LIVE") != "1" {
		t.Skip("opt-in network test")
	}
	p := newChampionProvider()
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	rank, err := p.loadArenaRankings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("YOUR.GG: patch=%s champions=%d first=%d,%d,%d", rank.Patch, len(rank.Rows), rank.Rows[0].ChampionID, rank.Rows[1].ChampionID, rank.Rows[2].ChampionID)
	body, err := fetchProSupplement(ctx, p, "TheShy")
	if err != nil {
		t.Fatal(err)
	}
	_, player, _ := proSupplementPlayer("TheShy")
	member, err := parseProSupplement(body, player)
	if err != nil {
		t.Fatal(err)
	}
	if len(member.Summoners) == 0 || member.Incomplete {
		t.Fatal("TheShy accounts missing/incomplete")
	}
	t.Logf("TheShy: verified KR accounts=%d", len(member.Summoners))
}
