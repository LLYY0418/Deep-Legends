package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"

	xhtml "golang.org/x/net/html"
)

func TestDDragonItemCatalogParsesGoldTotal(t *testing.T) {
	var catalog ddragonAssetList
	if err := json.Unmarshal([]byte(`{"data":{"3153":{"name":"破败王者之刃","gold":{"total":3200},"image":{"full":"3153.png"}}}}`), &catalog); err != nil {
		t.Fatal(err)
	}
	if item := catalog.Data["3153"]; item.Gold.Total != 3200 {
		t.Fatalf("ddragon item gold total = %#v", item.Gold)
	}
}

func TestValidateChampionAssetPath(t *testing.T) {
	tests := []struct {
		source string
		path   string
		host   string
		ok     bool
	}{
		{"opgg", "/meta/images/lol/16.15.1/champion/Vayne.png", opggAssetHost, true},
		{"opgg", "/meta/images/lol/latest/aram-augment/DoubleTap_large.png", opggAssetHost, true},
		{"ddragon", "/cdn/16.15.1/img/item/3153.png", dataDragonHost, true},
		{"ddragon", "/cdn/img/champion/splash/Vayne_0.jpg", dataDragonHost, true},
		{"communitydragon", "/latest/plugins/rcp-fe-lol-collections/global/default/images/item-element/crest-and-banner-mastery-10.png", communityDragonHost, true},
		{"communitydragon", "/latest/plugins/rcp-fe-lol-collections/global/default/images/other.png", communityDragonHost, false},
		{"gtimg", "/images/lol/act/img/champion/Vayne.png", prestigeArtworkHost, true},
		{"gtimg", "/images/lol/act/img/skin/big67000.jpg", prestigeArtworkHost, true},
		{"opgg", "/meta/images/lol/../../secret.png", "", false},
		{"ddragon", "/cdn/16.15.1/data/zh_CN/champion.json", "", false},
		{"opgg", "https://example.com/image.png", "", false},
		{"other", "/meta/images/lol/16.15.1/champion/Vayne.png", "", false},
		{"gtimg", "/images/lol/act/img/skin/../secret.png", "", false},
		{"gtimg", "/cdn/img/champion/splash/Vayne_0.jpg", prestigeArtworkHost, false},
	}
	for _, test := range tests {
		host, ok := validateChampionAssetPath(test.source, test.path)
		if ok != test.ok || host != test.host {
			t.Fatalf("validateChampionAssetPath(%q, %q) = %q, %v; want %q, %v", test.source, test.path, host, ok, test.host, test.ok)
		}
	}
}

func TestRuneShardSlotsUsePositionalMatching(t *testing.T) {
	chosen := []championAsset{}
	rows := runeShardSlots([]int{5008, 5008, 5001}, &chosen)
	if len(rows) != 3 {
		t.Fatalf("shard rows = %d, want 3", len(rows))
	}
	for rowIndex, row := range rows {
		active := 0
		for _, asset := range row {
			if asset.Active {
				active++
			}
		}
		if active != 1 {
			t.Fatalf("row %d has %d active shards, want exactly 1", rowIndex, active)
		}
	}
	if len(chosen) != 3 {
		t.Fatalf("chosen shards = %d, want 3", len(chosen))
	}
}

func TestLoadTopPlayersParsesLeaderboardFlight(t *testing.T) {
	payload := `{"data":[{"rank":1,"summoner":{"puuid":"x","game_name":"목숨뿐","tagline":"아초록스","profile_image_url":"https://opgg-static.akamaized.net/meta/images/profile_icons/profileIcon1594.jpg","level":"2,185"},"league_stats":{"tier_info":{"tier":"master","abbreviation":"M","lp":"211"},"win":747,"lose":704,"win_ratio":51},"most_champion_stat":{"play":"1,451","cs":173.5}},{"rank":2,"summoner":{"game_name":"Stuntman","tagline":"아트록스","profile_image_url":"https://example.com/evil.jpg"},"league_stats":{"tier_info":{"tier":"diamond 1","lp":"75"},"win_ratio":54},"most_champion_stat":{"play":"777"}}]}`
	flight := `self.__next_f.push([1,` + strconv.Quote(payload) + `])`
	decoded := decodeNextFlight([]byte(flight))
	var rows []opggLeaderboardRow
	if !extractBestArray(decoded, `"data":`, func(items []json.RawMessage) int {
		count := 0
		for _, item := range items {
			if strings.Contains(string(item), `"game_name"`) && strings.Contains(string(item), `"most_champion_stat"`) {
				count++
			}
		}
		return count
	}, &rows) {
		t.Fatal("leaderboard array was not extracted")
	}
	if len(rows) != 2 || rows[0].Summoner.GameName != "목숨뿐" || rows[0].LeagueStats.TierInfo.Tier != "master" || rows[0].MostChampionStat.Play != "1,451" || rows[1].LeagueStats.WinRatio != 54 {
		t.Fatalf("unexpected leaderboard rows: %+v", rows)
	}
}

func TestLoadTopPlayersUsesLeaderboardTableAndCapsFive(t *testing.T) {
	players := []struct {
		name, tag, tier, lp, games, winRate string
	}{
		{"Player One", "KR1", "大师", "211", "1,675", "51%"},
		{"Player Two", "KR2", "大师", "226", "1,380", "55%"},
		{"Player Three", "KR3", "大师", "0", "1,104", "53%"},
		{"Player Four", "kr2", "大师", "94", "858", "52%"},
		{"Player Five", "4208", "钻石 2", "20", "841", "51%"},
		{"Player Six", "KR6", "大师", "811", "768", "50%"},
	}
	var table strings.Builder
	table.WriteString(`<table><caption>Champion Table</caption><tbody>`)
	for index, player := range players {
		fmt.Fprintf(&table, `<tr><td>%d</td><td><a href="/zh-cn/lol/summoners/kr/player-%s"><img src="https://opgg-static.akamaized.net/meta/images/profile_icons/profileIcon%d.jpg?image=test"><span>%s</span><span>#<!-- -->%s</span></a></td><td><div>%s</div></td><td>%s</td><td>1.00:1</td><td>%s</td><td><span>10胜</span><span>9败</span><span>%s</span></td></tr>`, index+1, player.tag, index+1, player.name, player.tag, player.tier, player.lp, player.games, player.winRate)
	}
	table.WriteString(`</tbody></table>`)

	provider := newChampionProvider()
	provider.client = &http.Client{Transport: championRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path != "/zh-cn/lol/leaderboards/champions/nasus" || request.URL.Query().Get("region") != "kr" {
			return nil, fmt.Errorf("unexpected leaderboard request: %s", request.URL.String())
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(table.String())), Header: make(http.Header)}, nil
	})}

	got := provider.loadTopPlayers(context.Background(), "nasus")
	if len(got) != 5 {
		t.Fatalf("top players = %d, want 5: %#v", len(got), got)
	}
	if got[3].Name != "Player Four" || got[3].Games != "858" || got[3].WinRate != 52 || got[3].Tier != "master" {
		t.Fatalf("fourth player was not parsed from the table: %#v", got[3])
	}
	if got[4].Name != "Player Five" || got[4].Tagline != "4208" || got[4].Tier != "diamond 2" || got[4].LP != "20" || got[4].Games != "841" || got[4].WinRate != 51 {
		t.Fatalf("fifth player was not parsed from the table: %#v", got[4])
	}
	if got[4].IconSource != "opgg" || got[4].IconPath != "/meta/images/profile_icons/profileIcon5.jpg" {
		t.Fatalf("fifth player icon was not normalized: %#v", got[4])
	}
}

func TestValidateChampionAssetPathAllowsOPGGProfileIcons(t *testing.T) {
	host, ok := validateChampionAssetPath("opgg", "/meta/images/profile_icons/profileIcon1594.jpg")
	if !ok || host != opggAssetHost {
		t.Fatalf("profile icon path rejected: %q %v", host, ok)
	}
}

func TestCommunityDragonAugmentAssetPathsStayScoped(t *testing.T) {
	requestPath := "/latest/game/assets/maps/cherry/augments/icons/test.png"
	host, ok := validateChampionAssetPath("communitydragon", requestPath)
	if !ok || host != communityDragonHost {
		t.Fatalf("CommunityDragon augment path rejected: %q %v", host, ok)
	}
	provider := newChampionProvider()
	lcuPath, ok := provider.championAssetLCUPath("communitydragon", requestPath)
	if !ok || lcuPath != "/lol-game-data/assets/ASSETS/maps/cherry/augments/icons/test.png" {
		t.Fatalf("CommunityDragon LCU path = %q %v", lcuPath, ok)
	}
	if _, ok := validateChampionAssetPath("communitydragon", "/latest/other/test.png"); ok {
		t.Fatal("CommunityDragon proxy accepted an unrelated path")
	}
}

func TestAugmentMetadataMergeUsesRealIDsAndPreservesLCUDescriptions(t *testing.T) {
	lcu := []gameplayAugment{
		{ID: 1323, Name: "残忍", Description: "客户端真实说明", Rarity: "kGold"},
		{ID: 1400, Name: "本地条目", Description: ""},
	}
	communityDragon := []gameplayAugment{
		{ID: 1323, Name: "错误的补充名称", Description: "不应覆盖客户端说明", Rarity: "kSilver", IconPath: "/lol-game-data/assets/ASSETS/UX/Cherry/Augments/Icons/test.png"},
		{ID: 1400, Name: "远端名称", Description: "远端补充说明", Rarity: "kPrismatic"},
		{ID: 2323, Name: "新增条目", Description: "新增说明"},
	}

	merged := mergeGameplayAugmentMetadata(lcu, communityDragon)
	byID := gameplayAugmentIndexAll(merged)
	if got := byID[1323].Description; got != "客户端真实说明" {
		t.Fatalf("LCU description was overwritten: %q", got)
	}
	if got := byID[1323].IconPath; got == "" {
		t.Fatal("CommunityDragon icon was not merged into the real ID")
	}
	if got := byID[1400].Description; got != "远端补充说明" {
		t.Fatalf("empty LCU description was not supplemented: %q", got)
	}
	if _, ok := byID[323]; ok {
		t.Fatal("augment metadata must not use id-1000 mapping")
	}
	if _, ok := byID[2323]; !ok {
		t.Fatal("new CommunityDragon entry was not retained")
	}
}

func TestAugmentOfflineGuidanceOnlyFillsMissingMayhemDescriptions(t *testing.T) {
	if got := augmentDescriptionWithOfflineGuidance(1323, ""); got != augmentOfflineDescription {
		t.Fatalf("offline mayhem guidance = %q, want %q", got, augmentOfflineDescription)
	}
	if got := augmentDescriptionWithOfflineGuidance(1323, "已有说明"); got != "已有说明" {
		t.Fatalf("existing description was replaced: %q", got)
	}
	if got := augmentDescriptionWithOfflineGuidance(323, ""); got != "" {
		t.Fatalf("non-mayhem empty description got guidance: %q", got)
	}
}

func TestChampionAssetFallbackMapsGtimgOntoDataDragon(t *testing.T) {
	provider := newChampionProvider()
	provider.patch = "16.15.1"
	provider.championIDs["vayne"] = 67
	provider.championMeta[67] = championMetadata{ID: 67, Key: "Vayne"}

	host, path, ok := provider.championAssetFallback("gtimg", "/images/lol/act/img/champion/Vayne.png")
	if !ok || host != dataDragonHost || path != "/cdn/16.15.1/img/champion/Vayne.png" {
		t.Fatalf("icon fallback = %q %q %v", host, path, ok)
	}
	host, path, ok = provider.championAssetFallback("gtimg", "/images/lol/act/img/skin/big67012.jpg")
	if !ok || host != dataDragonHost || path != "/cdn/img/champion/splash/Vayne_12.jpg" {
		t.Fatalf("splash fallback = %q %q %v", host, path, ok)
	}
	if _, _, ok := provider.championAssetFallback("gtimg", "/images/lol/act/img/skin/big99012.jpg"); ok {
		t.Fatal("fallback accepted an unknown champion ID")
	}
	if _, _, ok := provider.championAssetFallback("ddragon", "/cdn/16.15.1/img/item/3153.png"); ok {
		t.Fatal("fallback accepted a non-gtimg source")
	}
}

func TestChampionAssetLCUPathUsesGameDataRoutes(t *testing.T) {
	provider := newChampionProvider()
	provider.championIDs["vayne"] = 67

	path, ok := provider.championAssetLCUPath("gtimg", "/images/lol/act/img/champion/Vayne.png")
	if !ok || path != "/lol-game-data/assets/v1/champion-icons/67.png" {
		t.Fatalf("icon LCU path = %q %v", path, ok)
	}
	path, ok = provider.championAssetLCUPath("gtimg", "/images/lol/act/img/skin/big67012.jpg")
	if !ok || path != "/lol-game-data/assets/v1/champion-splashes/67/67012.jpg" {
		t.Fatalf("splash LCU path = %q %v", path, ok)
	}
	if _, ok := provider.championAssetLCUPath("gtimg", "/images/lol/act/img/champion/Unknown.png"); ok {
		t.Fatal("LCU path accepted an unknown champion key")
	}
	if _, ok := provider.championAssetLCUPath("opgg", "/meta/images/lol/16.15.1/champion/Vayne.png"); ok {
		t.Fatal("LCU path accepted a non-gtimg source")
	}
}

func TestLoadRankedFiltersUnknownPositionsBeforeChoosingDefaults(t *testing.T) {
	positionStats := func(play int, winRate, pickRate, banRate float64, tier, rank int) map[string]any {
		return map[string]any{
			"play": play, "win_rate": winRate, "pick_rate": pickRate, "ban_rate": banRate, "kda": 3.4,
			"tier_data": map[string]any{"tier": tier, "rank": rank},
		}
	}
	data := []map[string]any{
		{
			"id": 1,
			"positions": []map[string]any{
				{"name": "ALIEN", "stats": positionStats(999, 0.99, 0.99, 0.99, 1, 1)},
				{"name": "mid", "stats": positionStats(100, 0.51, 0.12, 0.03, 2, 7)},
			},
		},
		{
			"id":        2,
			"positions": []map[string]any{{"name": "ALIEN", "stats": positionStats(999, 0.99, 0.99, 0.99, 1, 1)}},
		},
	}
	for id := 3; id <= 51; id++ {
		data = append(data, map[string]any{
			"id": id,
			"positions": []map[string]any{{
				"name": "top", "stats": positionStats(200+id, 0.50, 0.10, 0.02, 3, 100+id),
			}},
		})
	}
	fixture, err := json.Marshal(map[string]any{"data": data})
	if err != nil {
		t.Fatal(err)
	}

	provider := newChampionProvider()
	provider.cache = newChampionDataCache(nil)
	provider.client = &http.Client{Transport: championRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Host != opggChampionHost || request.URL.Path != "/api/KR/champions/ranked" || request.URL.Query().Get("tier") != "emerald_plus" {
			return nil, fmt.Errorf("unexpected ranked request: %s", request.URL.String())
		}
		return &http.Response{StatusCode: http.StatusOK, Status: http.StatusText(http.StatusOK), Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(fixture))), ContentLength: int64(len(fixture)), Request: request}, nil
	})}

	all, err := provider.loadRanked(context.Background(), "emerald_plus", "all")
	if err != nil {
		t.Fatal(err)
	}
	if len(all.Rows) != 50 {
		t.Fatalf("ranked rows = %d, want 50 after dropping the unknown-only row", len(all.Rows))
	}
	var defaulted *championRankingRow
	for index := range all.Rows {
		if all.Rows[index].ChampionID == 2 {
			t.Fatalf("unknown-only position row leaked: %#v", all.Rows[index])
		}
		if all.Rows[index].ChampionID == 1 {
			defaulted = &all.Rows[index]
		}
	}
	if defaulted == nil {
		t.Fatal("row with a valid second position was dropped")
	}
	if defaulted.Position != "mid" || len(defaulted.Positions) != 1 || defaulted.Positions[0] != "mid" || defaulted.Play != 100 || defaulted.Rank != 7 || defaulted.Tier != 2 || defaulted.WinRate != 51 || defaulted.PickRate != 12 || defaulted.BanRate != 3 {
		t.Fatalf("default position did not use the first valid candidate: %#v", defaulted)
	}

	top, err := provider.loadRanked(context.Background(), "emerald_plus", "TOP")
	if err != nil {
		t.Fatal(err)
	}
	if len(top.Rows) != 49 || top.Position != "top" {
		t.Fatalf("top filter = position %q rows %d", top.Position, len(top.Rows))
	}
	for _, row := range top.Rows {
		if row.Position != "top" || len(row.Positions) != 1 || row.Positions[0] != "top" {
			t.Fatalf("top filter leaked a non-canonical position: %#v", row)
		}
	}
	if _, err := provider.loadRanked(context.Background(), "emerald_plus", "ALIEN"); err == nil {
		t.Fatal("unknown ranked position was accepted")
	}
}

func TestDecodeNextFlightAndExtractBestArray(t *testing.T) {
	fragment := `0:["$",{"small":{"data":[{"id":1}]},"champions":[{"champion_id":67,"key":"vayne"},{"champion_id":22,"key":"ashe"}]}}]`
	quoted, err := json.Marshal(fragment)
	if err != nil {
		t.Fatal(err)
	}
	page := `<script>self.__next_f.push([1,` + string(quoted) + `])</script>`
	decoded := decodeNextFlight([]byte(page))
	if decoded != fragment {
		t.Fatalf("decoded flight mismatch: %q", decoded)
	}
	var champions []aramChampionRaw
	ok := extractBestArray(decoded, `"champions":`, func(candidate []json.RawMessage) int {
		return len(candidate)
	}, &champions)
	if !ok || len(champions) != 2 || champions[0].ChampionID != 67 {
		t.Fatalf("unexpected extracted champions: %#v", champions)
	}
}

func TestParseArenaTeamCompositionsAndStats(t *testing.T) {
	decoded := `{"average_stats":{"win_rate":48.77,"pick_rate":13.85,"ban_rate":42.49,"first_place":16.07,"avg_place":3.56},"teamData":[{"champion_ids":[44,350,11],"champion_id":11,"champions":[{"id":44,"key":"taric","name":"瓦洛兰之盾","image_url":"https://opgg-static.akamaized.net/meta/images/lol/16.15.1/champion/Taric.png"},{"id":350,"key":"yuumi","name":"魔法猫咪","image_url":"https://opgg-static.akamaized.net/meta/images/lol/16.15.1/champion/Yuumi.png"},{"id":11,"key":"masteryi","name":"无极剑圣","image_url":"https://opgg-static.akamaized.net/meta/images/lol/16.15.1/champion/MasterYi.png"}],"combination_size":3,"play":"342","win_rate":68.13,"first_place_rate":29.53,"average_place":2.85,"pick_rate":0.53}]}`
	teams := parseArenaTeamCompositions(decoded, `"teamData":`, 11, 3)
	if len(teams) != 1 || len(teams[0].Champions) != 3 || teams[0].Champions[0].Key != "masteryi" || teams[0].Champions[1].Key != "taric" || teams[0].Champions[2].Key != "yuumi" {
		t.Fatalf("arena team champions were not parsed: %#v", teams)
	}
	if teams[0].Games != 342 || teams[0].AveragePlacement != 2.85 || teams[0].FirstPlaceRate != 29.53 || teams[0].WinRate != 68.13 {
		t.Fatalf("arena team metrics were not parsed: %#v", teams[0])
	}
	stats := parseArenaStats(decoded)
	if stats.AveragePlacement != 3.56 || stats.FirstPlaceRate != 16.07 || stats.PickRate != 13.85 || stats.WinRate != 48.77 || stats.BanRate != 42.49 {
		t.Fatalf("arena summary metrics were not parsed: %#v", stats)
	}
}

func TestParseArenaAugmentsKeepsMetricsAndTooltip(t *testing.T) {
	decoded := `{"data":{"id":52,"name":"闪电打击","image_url":"https://opgg-static.akamaized.net/meta/images/lol/latest/augment/lightningstrikes_large.png","pick_rate":18.86,"win_rate":54.09,"play":"29,587","desc":"获得<attention>总攻击速度</attention>。"}}`
	rows := parseArenaAugments(decoded)
	if len(rows) != 1 || len(rows[0].Assets) != 1 {
		t.Fatalf("arena augment row was not parsed: %#v", rows)
	}
	if rows[0].Games != 29587 || rows[0].PickRate != 18.86 || rows[0].WinRate != 54.09 || rows[0].Assets[0].Description != "获得总攻击速度。" {
		t.Fatalf("arena augment metrics changed: %#v", rows[0])
	}
}

func TestBalancedJSONArrayHandlesQuotedBrackets(t *testing.T) {
	source := `[{"name":"[not a boundary]","description":"escaped \"]\""},{"id":2}] trailing`
	data, end, ok := balancedJSONArray(source, 0)
	if !ok || end <= 0 {
		t.Fatal("expected a balanced array")
	}
	var rows []map[string]any
	if err := json.Unmarshal(data, &rows); err != nil || len(rows) != 2 {
		t.Fatalf("balanced array was invalid: %s (%v)", string(data), err)
	}
}

func TestNormalizeArenaBuildSeparatesBootsAndRemovesPlaceholder(t *testing.T) {
	build := championBuildSections{CoreItems: []championMetricRow{
		{Assets: []championAsset{{ID: 223020, Name: "法师之靴", Path: "/item/223020.png"}}, PickRate: 43.46},
		{Assets: []championAsset{{ID: 226653, Name: "兰德里的苦楚", Path: "/item/226653.png"}, {ID: 220007, Name: "棱彩装备", Path: "/item/220007.png"}, {ID: 224633, Name: "裂隙制造者", Path: "/item/224633.png"}}, PickRate: 4.97},
	}}
	normalized := normalizeArenaBuild(build)
	if len(normalized.Boots) != 1 || normalized.Boots[0].Assets[0].Name != "法师之靴" {
		t.Fatalf("arena boots were not separated: %#v", normalized.Boots)
	}
	if len(normalized.CoreItems) != 1 || len(normalized.CoreItems[0].Assets) != 2 {
		t.Fatalf("arena core route changed: %#v", normalized.CoreItems)
	}
	for _, asset := range normalized.CoreItems[0].Assets {
		if asset.ID == 220007 || strings.Contains(asset.Name, "棱彩装备") {
			t.Fatalf("arena placeholder leaked into a route: %#v", normalized.CoreItems)
		}
	}
}

func TestChampionSearchTermsAndAliases(t *testing.T) {
	terms := championPinyinTerms("暗夜猎手")
	joined := strings.Join(terms, "|")
	if !strings.Contains(joined, "anyelieshou") || !strings.Contains(joined, "ayls") {
		t.Fatalf("unexpected pinyin terms: %#v", terms)
	}
	if championOPGGSlug("MonkeyKing") != "wukong" || championOPGGSlug("Vayne") != "vayne" {
		t.Fatal("OP.GG champion slug normalization failed")
	}
	for champion, alias := range map[string]string{"vayne": "uzi", "ryze": "faker", "aatrox": "theshy", "drmundo": "bin"} {
		found := false
		for _, value := range championAliases[champion] {
			found = found || value == alias
		}
		if !found {
			t.Fatalf("%s alias did not resolve to %s", alias, champion)
		}
	}
}

func TestParseChampionAbilityDescriptionsIncludesConcreteValues(t *testing.T) {
	data := []byte(`{"data":{"Vayne":{"partype":"法力","spells":[
		{"id":"VayneTumble","name":"闪避突袭","description":"向前翻滚。","costType":"{{ abilityresourcename }}","cost":[30,30,30,30,30],"cooldown":[6,5,4,3,2],"range":[300,300,300,300,300]},
		{"id":"VayneSilveredBolts","name":"圣银弩箭","description":"第三次攻击造成真实伤害。","costType":"无消耗","cost":[0,0,0,0,0],"cooldown":[0,0,0,0,0],"range":[0,0,0,0,0]},
		{"id":"VayneCondemn","name":"恶魔审判","description":"击退目标。","costType":"法力","cost":[90,90,90,90,90],"cooldown":[20,18,16,14,12],"range":[550,550,550,550,550]},
		{"id":"VayneInquisition","name":"终极时刻","description":"强化自身。","costType":"法力","cost":[80,80,80],"cooldown":[100,85,70],"range":[0,0,0]}
	]}}}`)
	abilities, err := parseChampionAbilityDescriptions(data, "Vayne")
	if err != nil {
		t.Fatal(err)
	}
	if abilities["Q"].Name != "闪避突袭" || abilities["Q"].CostType != "法力" || abilities["Q"].Costs[0] != 30 || abilities["Q"].Cooldowns[4] != 2 || abilities["Q"].Ranges[0] != 300 {
		t.Fatalf("Q ability details were incomplete: %#v", abilities["Q"])
	}
	rows := []championMetricRow{{SkillPriority: []string{"Q", "E", "W"}, Assets: []championAsset{{Kind: "spell"}, {Kind: "spell"}, {Kind: "spell"}}}}
	decorateChampionSkills(rows, abilities)
	if rows[0].Assets[1].Name != "恶魔审判" || rows[0].Assets[1].Cooldowns[4] != 12 || rows[0].Assets[2].CostType != "无消耗" {
		t.Fatalf("skill priority was not enriched by slot: %#v", rows[0].Assets)
	}
}

func TestCleanMarkupAndRarity(t *testing.T) {
	value := cleanMarkup(`获得<crit>暴击</crit>。<br /><br />造成<trueDamage>真实伤害</trueDamage>。`)
	if value != "获得暴击。\n造成真实伤害。" {
		t.Fatalf("unexpected cleaned markup: %q", value)
	}
	if augmentRarity(1) != "silver" || augmentRarity(4) != "gold" || augmentRarity(8) != "prismatic" {
		t.Fatal("augment rarity mapping changed")
	}
	if augmentRarityOrder("prismatic") >= augmentRarityOrder("gold") || augmentRarityOrder("gold") >= augmentRarityOrder("silver") {
		t.Fatal("augment rarity sorting must keep prismatic, gold, silver order")
	}
}

func TestParseChampionCounters(t *testing.T) {
	document, err := xhtml.Parse(strings.NewReader(`<!doctype html><main><section>
<div><div>劣势对抗</div></div><div><ul><li><a href="/zh-cn/lol/champions/vayne/counters?target_champion=yunara"><img alt="芸阿娜" src="https://opgg-static.akamaized.net/meta/images/lol/16.15.1/champion/Yunara.png"><strong>41.27%</strong><span>126 场</span></a></li></ul></div>
<div><div>优势对抗</div></div><div><ul><li><a href="/zh-cn/lol/champions/vayne/counters?target_champion=kaisa"><img alt="卡莎" src="https://opgg-static.akamaized.net/meta/images/lol/16.15.1/champion/Kaisa.png"><strong>58.73%</strong><span>1,206 场</span></a></li></ul></div>
</section></main>`))
	if err != nil {
		t.Fatal(err)
	}
	counters := parseChampionCounters(document)
	if len(counters.WeakAgainst) != 1 || counters.WeakAgainst[0].Key != "yunara" || counters.WeakAgainst[0].WinRate != 41.27 || counters.WeakAgainst[0].Games != 126 {
		t.Fatalf("weak counters were not parsed: %#v", counters.WeakAgainst)
	}
	if len(counters.StrongAgainst) != 1 || counters.StrongAgainst[0].Key != "kaisa" || counters.StrongAgainst[0].Games != 1206 {
		t.Fatalf("strong counters were not parsed: %#v", counters.StrongAgainst)
	}
}

func TestParseChampionCountersAcceptsNonDivSectionHeadings(t *testing.T) {
	document, err := xhtml.Parse(strings.NewReader(`<main><section>
<h3>劣势对抗</h3><div><ul>
<li><a href="/counters?target_champion=yunara"><img alt="芸阿娜" src="https://opgg-static.akamaized.net/meta/images/lol/16.15.1/champion/Yunara.png"><strong>41.27%</strong><span>126 场</span></a></li>
<li><a href="/counters?target_champion=kaisa"><img alt="卡莎" src="https://opgg-static.akamaized.net/meta/images/lol/16.15.1/champion/Kaisa.png"><strong>43.10%</strong><span>90 场</span></a></li>
</ul></div>
<span>优势对抗</span><div><ul>
<li><a href="/counters?target_champion=vayne"><img alt="薇恩" src="https://opgg-static.akamaized.net/meta/images/lol/16.15.1/champion/Vayne.png"><strong>58.73%</strong><span>1,206 场</span></a></li>
<li><a href="/counters?target_champion=ezreal"><img alt="伊泽瑞尔" src="https://opgg-static.akamaized.net/meta/images/lol/16.15.1/champion/Ezreal.png"><strong>56.20%</strong><span>800 场</span></a></li>
</ul></div>
</section></main>`))
	if err != nil {
		t.Fatal(err)
	}
	counters := parseChampionCounters(document)
	if len(counters.WeakAgainst) != 2 || len(counters.StrongAgainst) != 2 {
		t.Fatalf("non-div headings were not parsed: %#v", counters)
	}
	if counters.WeakAgainst[0].Key != "yunara" || counters.StrongAgainst[0].Key != "vayne" {
		t.Fatalf("counter grouping changed: %#v", counters)
	}
}

func TestParseChampionRunesKeepsFullTreeAndShards(t *testing.T) {
	decoded := `{"rune_pages":[{"play":1200,"pick_rate":0.64,"win_rate":0.5275,"builds":[{"primary_perk_style":{"id":8100,"name":"主宰","image_url":"https://opgg-static.akamaized.net/meta/images/lol/16.15.1/perkStyle/8100.png"},"perk_sub_style":{"id":8300,"name":"启迪","image_url":"https://opgg-static.akamaized.net/meta/images/lol/16.15.1/perkStyle/8300.png"},"main_runes":[[{"id":8112,"name":"电刑","image_url":"https://opgg-static.akamaized.net/meta/images/lol/16.15.1/perk/8112.png","isActive":true},{"id":8124,"name":"掠食者","image_url":"https://opgg-static.akamaized.net/meta/images/lol/16.15.1/perk/8124.png"}]],"sub_runes":[[{"id":8345,"name":"饼干配送","image_url":"https://opgg-static.akamaized.net/meta/images/lol/16.15.1/perk/8345.png","isActive":true}]],"shards":[[{"id":5005,"name":"攻击速度","image_url":"https://opgg-static.akamaized.net/meta/images/lol/16.15.1/perkShard/5005.png","isActive":true}]]}]}]}`
	pages := parseChampionRunes(decoded)
	if len(pages) != 1 || len(pages[0].PrimarySlots) != 1 || len(pages[0].PrimarySlots[0]) != 2 || len(pages[0].SubSlots) != 1 || len(pages[0].ShardSlots) != 1 {
		t.Fatalf("full rune tree was not preserved: %#v", pages)
	}
	if len(pages[0].Selected) != 3 || !pages[0].PrimarySlots[0][0].Active || pages[0].PickRate != 64 || pages[0].WinRate != 52.75 {
		t.Fatalf("selected rune state or metrics changed: %#v", pages[0])
	}
	shard := pages[0].ShardSlots[0][0]
	if shard.Source != "ddragon" || shard.Path != "/cdn/img/perk-images/StatMods/StatModsAttackSpeedIcon.png" || !strings.Contains(shard.Description, "10%攻击速度") {
		t.Fatalf("rune shard did not use the crisp Data Dragon asset: %#v", shard)
	}
}

func TestDecorateChampionAssetAddsTooltipMetadata(t *testing.T) {
	asset := championAsset{ID: 3153, Kind: "item", Name: "旧名称", Path: "/meta/images/lol/16.15.1/item/3153.png"}
	decorateChampionAsset(&asset, map[string]championAssetDescription{
		"item/3153.png": {Name: "破败王者之刃", Description: "攻击会造成额外伤害。"},
	})
	if asset.Name != "破败王者之刃" || asset.Description == "" {
		t.Fatalf("item tooltip metadata was not applied: %#v", asset)
	}
}

func TestFilterChampionItemComponentsKeepsFinalAndUnknownItems(t *testing.T) {
	rows := []championMetricRow{{Assets: []championAsset{
		{ID: 3070, Kind: "item", Name: "女神之泪", Path: "/cdn/16.16/img/item/3070.png"},
		{ID: 6655, Kind: "item", Name: "卢登的伙伴", Path: "/cdn/16.16/img/item/6655.png"},
		{ID: 999999, Kind: "item", Name: "未知上游装备", Path: "/cdn/16.16/img/item/999999.png"},
	}}}
	descriptions := map[string]championAssetDescription{
		"item/3070.png": {Name: "女神之泪", BuildsInto: true},
		"item/6655.png": {Name: "卢登的伙伴"},
	}
	filtered := filterChampionItemComponents(rows, descriptions)
	if len(filtered) != 1 || len(filtered[0].Assets) != 2 {
		t.Fatalf("filtered routes = %#v", filtered)
	}
	if filtered[0].Assets[0].ID != 6655 || filtered[0].Assets[1].ID != 999999 {
		t.Fatalf("final or unknown item was removed: %#v", filtered[0].Assets)
	}
}

func TestFillMissingChampionCountersPreservesAvailableSide(t *testing.T) {
	current := championCounterSections{
		StrongAgainst: []championCounterRow{{ChampionID: 24, Name: "贾克斯"}},
	}
	fallback := championCounterSections{
		WeakAgainst: []championCounterRow{{ChampionID: 122, Name: "德莱厄斯"}},
		StrongAgainst: []championCounterRow{
			{ChampionID: 24, Name: "贾克斯"},
			{ChampionID: 92, Name: "锐雯"},
		},
	}
	if !fillMissingChampionCounters(&current, fallback) {
		t.Fatal("missing counter side was not filled")
	}
	if len(current.WeakAgainst) != 1 || current.WeakAgainst[0].ChampionID != 122 {
		t.Fatalf("weak counters were not filled: %#v", current.WeakAgainst)
	}
	if len(current.StrongAgainst) != 2 || current.StrongAgainst[0].ChampionID != 24 || current.StrongAgainst[1].ChampionID != 92 {
		t.Fatalf("available strong counters were not preserved and extended: %#v", current.StrongAgainst)
	}
}

func TestLoadStructuredCountersUsesOnlyOPGGDetailPayload(t *testing.T) {
	provider := newChampionProvider()
	provider.championIDs["jax"] = 24
	provider.championMeta[24] = championMetadata{ID: 24, Slug: "jax", Key: "Jax"}
	for _, id := range []int{92, 122, 164, 266, 891} {
		provider.championMeta[id] = championMetadata{ID: id, Key: strconv.Itoa(id), NameZH: strconv.Itoa(id)}
	}
	provider.client = &http.Client{Transport: championRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Host != opggChampionHost || request.URL.Path != "/api/KR/champions/ranked/24/TOP" || request.URL.Query().Get("tier") != championCounterFallbackTier {
			return nil, fmt.Errorf("unexpected counter fallback request: %s", request.URL.String())
		}
		body := `{"data":{"summary":{"id":24},"counters":[{"champion_id":92,"play":100,"win":40},{"champion_id":122,"play":100,"win":45},{"champion_id":164,"play":100,"win":50},{"champion_id":266,"play":100,"win":55},{"champion_id":891,"play":100,"win":60}]},"meta":{"version":"16.16"}}`
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
	counters, err := provider.loadStructuredCounters(context.Background(), "ranked", "jax", "top", championCounterFallbackTier)
	if err != nil {
		t.Fatal(err)
	}
	if len(counters.WeakAgainst) != 3 || len(counters.StrongAgainst) != 2 {
		t.Fatalf("counter fallback = %#v", counters)
	}
	seen := make(map[int]bool)
	for _, row := range counters.WeakAgainst {
		seen[row.ChampionID] = true
	}
	for _, row := range counters.StrongAgainst {
		if seen[row.ChampionID] {
			t.Fatalf("counter appeared on both sides: %#v", counters)
		}
	}
}

func TestStructuredCountersDropsSubjectAndDuplicateRows(t *testing.T) {
	provider := newChampionProvider()
	provider.championMeta[24] = championMetadata{ID: 24, Key: "jax", NameZH: "贾克斯"}
	for _, id := range []int{92, 122, 164, 266, 891} {
		provider.championMeta[id] = championMetadata{ID: id, Key: strconv.Itoa(id), NameZH: strconv.Itoa(id)}
	}
	counters := provider.structuredCountersForChampion([]opggCounter{
		{ChampionID: 24, Play: 999, Win: 500}, // OP.GG occasionally echoes the subject.
		{ChampionID: 92, Play: 100, Win: 40},
		{ChampionID: 122, Play: 100, Win: 45},
		{ChampionID: 164, Play: 100, Win: 55},
		{ChampionID: 266, Play: 100, Win: 60},
		{ChampionID: 266, Play: 90, Win: 50}, // Duplicate must not appear twice.
	}, 24)
	seen := make(map[int]bool)
	for _, rows := range [][]championCounterRow{counters.WeakAgainst, counters.StrongAgainst} {
		for _, row := range rows {
			if row.ChampionID == 24 || seen[row.ChampionID] {
				t.Fatalf("subject or duplicate counter survived: %#v", counters)
			}
			seen[row.ChampionID] = true
		}
	}
}

func TestLoadStructuredCountersUsesTopLevelCounters(t *testing.T) {
	provider := newChampionProvider()
	provider.championIDs["jax"] = 24
	provider.championMeta[24] = championMetadata{ID: 24, Slug: "jax", Key: "Jax", NameZH: "贾克斯"}
	for id := 100; id < 130; id++ {
		provider.championMeta[id] = championMetadata{ID: id, Key: strconv.Itoa(id), NameZH: strconv.Itoa(id)}
	}
	provider.client = &http.Client{Transport: championRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		rows := make([]string, 0, 30)
		for index := 0; index < 30; index++ {
			rows = append(rows, fmt.Sprintf(`{"champion_id":%d,"play":1000,"win":%d}`, 100+index, 30+index))
		}
		body := fmt.Sprintf(`{"data":{"summary":{"id":24,"positions":[{"name":"TOP","counters":[{"champion_id":100,"play":100,"win":40},{"champion_id":101,"play":100,"win":45},{"champion_id":102,"play":100,"win":55}]}]},"counters":[%s]},"meta":{"version":"16.16"}}`, strings.Join(rows, ","))
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
	counters, err := provider.loadStructuredCounters(context.Background(), "ranked", "jax", "top", championCounterFallbackTier)
	if err != nil {
		t.Fatal(err)
	}
	if len(counters.WeakAgainst) != 5 || len(counters.StrongAgainst) != 5 {
		t.Fatalf("top-level counters were not retained for strong/weak split: %#v", counters)
	}
	for _, row := range append(counters.WeakAgainst, counters.StrongAgainst...) {
		if row.ChampionID < 100 || row.ChampionID >= 130 {
			t.Fatalf("summary counter leaked into result: %#v", counters)
		}
	}
}

func TestLoadStructuredDetailUsesTopLevelCounters(t *testing.T) {
	provider := newChampionProvider()
	provider.championIDs["jax"] = 24
	provider.championMeta[24] = championMetadata{ID: 24, Slug: "jax", Key: "Jax", NameZH: "贾克斯"}
	for id := 100; id < 130; id++ {
		provider.championMeta[id] = championMetadata{ID: id, Key: strconv.Itoa(id), NameZH: strconv.Itoa(id)}
	}
	provider.client = &http.Client{Transport: championRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Host == opggPageHost {
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`no item depths`)), Header: make(http.Header)}, nil
		}
		rows := make([]string, 0, 30)
		for index := 0; index < 30; index++ {
			rows = append(rows, fmt.Sprintf(`{"champion_id":%d,"play":1000,"win":%d}`, 100+index, 30+index))
		}
		cores := make([]string, 0, 6)
		for index := 0; index < 6; index++ {
			cores = append(cores, fmt.Sprintf(`{"ids":[%d,%d,%d],"play":%d,"win":20}`, 3000+index*3, 3001+index*3, 3002+index*3, 49-index))
		}
		body := fmt.Sprintf(`{"data":{"summary":{"id":24,"average_stats":{},"positions":[{"name":"TOP","counters":[{"champion_id":100,"play":100,"win":40},{"champion_id":101,"play":100,"win":45},{"champion_id":102,"play":100,"win":55}]}]},"core_items":[%s],"counters":[%s]},"meta":{"version":"16.16"}}`, strings.Join(cores, ","), strings.Join(rows, ","))
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
	detail, err := provider.loadStructuredDetail(context.Background(), "ranked", "jax", "top", championCounterFallbackTier)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Counters.WeakAgainst) != 5 || len(detail.Counters.StrongAgainst) != 5 {
		t.Fatalf("detail used summary counters: %#v", detail.Counters)
	}
	if len(detail.Build.CoreItems) != 6 {
		t.Fatalf("ranked detail sample-gated OP.GG core recommendations: %#v", detail.Build.CoreItems)
	}
}

func TestParseOPGGItemDepthsExpandsDelayedFifthItemGames(t *testing.T) {
	payload := []byte(`1:["$","tr",null,{"children":["depth_4_item_0",{"metaType":"item","metaId":6333},{"children":["61.19","%"]},{"children":"572 场"}]}]
2:["$","tr",null,{"children":["depth_5_item_0",{"metaId":3026,"metaType":"item"},{"children":["57.14","%"]},"$L7e"]}]
7e:["$","span",null,{"children":"49 场"}]`)
	depths := parseOPGGDepthRows(payload)
	if len(depths[4]) != 1 || depths[4][0].Assets[0].ID != 6333 || depths[4][0].WinRate != 61.19 || depths[4][0].Games != 572 {
		t.Fatalf("fourth items = %#v", depths[4])
	}
	if len(depths[5]) != 1 || depths[5][0].Assets[0].ID != 3026 || depths[5][0].WinRate != 57.14 || depths[5][0].Games != 49 {
		t.Fatalf("fifth items = %#v", depths[5])
	}
}

func TestParseOPGGItemDepthsDeduplicatesExpandedRowsBeforeApplyingLimit(t *testing.T) {
	// Current OP.GG RSC contains a long expanded mobile row followed by the
	// compact desktop row with the same logical key. The duplicate item_0 must
	// not consume one of the five recommendation slots.
	duplicate := `"depth_5_item_0",{"metaType":"item","metaId":3089,"children":[54.35,"%"],"sample":"20 场","filler":"` + strings.Repeat("x", 5000) + `"}`
	rows := []string{
		duplicate,
		`"depth_5_item_0",{"metaType":"item","metaId":3089,"children":[54.35,"%"],"sample":"46 场"}`,
		`"depth_5_item_1",{"metaType":"item","metaId":3157,"children":[48.28,"%"],"sample":"29 场"}`,
		`"depth_5_item_2",{"metaType":"item","metaId":3135,"children":[52,"%"],"sample":"25 场"}`,
		`"depth_5_item_3",{"metaType":"item","metaId":3110,"children":[58.33,"%"],"sample":"12 场"}`,
		`"depth_5_item_4",{"metaType":"item","metaId":3165,"children":[75,"%"],"sample":"4 场"}`,
	}
	depths := parseOPGGDepthRows([]byte(strings.Join(rows, "\n")))
	if len(depths[5]) != 5 {
		t.Fatalf("fifth-item rows = %#v", depths[5])
	}
	if depths[5][0].Assets[0].ID != 3089 || depths[5][0].Games != 46 || depths[5][4].Assets[0].ID != 3165 {
		t.Fatalf("expanded duplicate displaced an OP.GG recommendation: %#v", depths[5])
	}
}

func TestRankedCoreRecommendationsKeepCompleteOPGGOrderBelowLocalSampleGate(t *testing.T) {
	provider := newChampionProvider()
	values := make([]opggMetric, 0, championCoreRecommendationLimit+1)
	for index := 0; index < championCoreRecommendationLimit+1; index++ {
		values = append(values, opggMetric{IDs: []int{3000 + index}, Play: 49 - index, Win: 20})
	}
	rows := provider.structuredRecommendationMetrics(values, "item", "core", championCoreRecommendationLimit)
	if len(rows) != championCoreRecommendationLimit {
		t.Fatalf("ranked core recommendations were sample-gated: %#v", rows)
	}
	for index, row := range rows {
		if len(row.Assets) != 1 || row.Assets[0].ID != 3000+index {
			t.Fatalf("OP.GG order changed at %d: %#v", index, rows)
		}
	}
}

func TestParseOPGGItemDepthsAcceptsCurrentNumericRSCPercentages(t *testing.T) {
	payload := []byte(`1:["$","tr",null,{"children":["depth_4_item_0",{"metaType":"item","metaId":6333},{"children":[54.05,"%"]},{"children":"1,112 场"}]}]
2:["$","tr",null,{"children":["depth_5_item_0",{"metaType":"item","metaId":3026},{"children":[63.19,"%"]},{"children":"49 场"}]}]`)
	depths := parseOPGGDepthRows(payload)
	if len(depths[4]) != 1 || depths[4][0].WinRate != 54.05 || depths[4][0].Games != 1112 {
		t.Fatalf("current fourth item shape was not parsed: %#v", depths[4])
	}
	if len(depths[5]) != 1 || depths[5][0].WinRate != 63.19 || depths[5][0].Games != 49 {
		t.Fatalf("current fifth item shape was not parsed: %#v", depths[5])
	}
}

func TestParseOPGGItemDepthsKeepsZeroWinRateWithRealGames(t *testing.T) {
	payload := []byte(`1:["$","tr",null,{"children":["depth_5_item_0",{"metaType":"item","metaId":3110},{"children":[0,"%"]},{"children":"1 场"}]}]
2:["$","tr",null,{"children":["depth_6_item_0",{"metaType":"item","metaId":3089},{"children":[0,"%"]},{"children":"1场"}]}]`)
	depths := parseOPGGDepthRows(payload)
	for _, depth := range []int{5, 6} {
		if len(depths[depth]) != 1 || depths[depth][0].WinRate != 0 || depths[depth][0].Games != 1 {
			t.Fatalf("zero-win depth %d row was dropped: %#v", depth, depths[depth])
		}
	}
}

func TestParseOPGGItemDepthsIncludesSixthItem(t *testing.T) {
	payload := []byte(`1:["$","tr",null,{"children":["depth_6_item_0",{"metaType":"item","metaId":3089},{"children":[55.5,"%"]},{"children":"123 场"}]}]`)
	depths := parseOPGGDepthRows(payload)
	if len(depths[6]) != 1 || depths[6][0].Assets[0].ID != 3089 || depths[6][0].Games != 123 {
		t.Fatalf("sixth items = %#v", depths[6])
	}
}

func TestLoadOPGGDepthRowsAllowsMissingSixthItemDepth(t *testing.T) {
	payload := `1:["$","tr",null,{"children":["depth_4_item_0",{"metaType":"item","metaId":6333},{"children":[54.05,"%"]},{"children":"1,112 场"}]}]
2:["$","tr",null,{"children":["depth_5_item_0",{"metaType":"item","metaId":3026},{"children":[63.19,"%"]},{"children":"49 场"}]}]`
	provider := newChampionProvider()
	provider.cache = nil
	provider.client = &http.Client{Transport: championRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Host != opggPageHost || request.Header.Get("Accept") != "text/x-component" || request.Header.Get("RSC") != "1" {
			t.Fatalf("unexpected OP.GG depth request: %s headers=%v", request.URL, request.Header)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(payload)), Header: make(http.Header)}, nil
	})}

	depths, _, err := provider.loadOPGGDepthRows(context.Background(), "lulu", "support", "emerald_plus", "16.16.1")
	if err != nil {
		t.Fatal(err)
	}
	if len(depths[4]) != 1 || len(depths[5]) != 1 || len(depths[6]) != 0 {
		t.Fatalf("missing sixth depth invalidated usable fourth/fifth rows: %#v", depths)
	}
}

func TestLoadOPGGDepthRowsRejectsMissingFifthItemDepth(t *testing.T) {
	payload := `1:["$","tr",null,{"children":["depth_4_item_0",{"metaType":"item","metaId":6333},{"children":[54.05,"%"]},{"children":"1,112 场"}]}]
2:["$","tr",null,{"children":["depth_6_item_0",{"metaType":"item","metaId":3089},{"children":[55.5,"%"]},{"children":"123 场"}]}]`
	provider := newChampionProvider()
	provider.cache = nil
	provider.client = &http.Client{Transport: championRoundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(payload)), Header: make(http.Header)}, nil
	})}

	if _, _, err := provider.loadOPGGDepthRows(context.Background(), "lulu", "support", "emerald_plus", "16.16.1"); err == nil || !strings.Contains(err.Error(), "item-depth response changed") {
		t.Fatalf("missing fifth depth was accepted: %v", err)
	}
}

func TestParseOPGGItemDepthsKeepsThreeRowsForFiveChampionFixtures(t *testing.T) {
	for _, champion := range []string{"jax", "leesin", "malphite", "jayce", "sylas"} {
		var rows []string
		for _, depth := range []int{4, 5, 6} {
			for index := 0; index < 3; index++ {
				rows = append(rows, fmt.Sprintf(`%d:["$","tr",null,{"children":["depth_%d_item_%d",{"metaType":"item","metaId":%d},{"children":[55.0,"%%"]},{"children":"%d 场"}]}]`, len(rows)+1, depth, index, depth*100+index, 200-index))
			}
		}
		depths := parseOPGGDepthRows([]byte(strings.Join(rows, "\n")))
		for _, depth := range []int{4, 5, 6} {
			if len(depths[depth]) < 3 {
				t.Fatalf("%s depth %d rows = %#v", champion, depth, depths[depth])
			}
		}
	}
}

func TestOPGGDepthRowsAreNotDeduplicatedAgainstCoreItems(t *testing.T) {
	// 第四/五/六件一律原样展示 OP.GG 给的行，不再剔除核心装里出现过的装备。
	// 去重看着「更干净」，但会把上游真实的推荐删掉——用户反复反馈的就是这个。
	source, err := os.ReadFile("champions_structured.go")
	if err != nil {
		t.Fatalf("read champions_structured.go: %v", err)
	}
	if strings.Contains(string(source), "filterOPGGDepthCoreItems") {
		t.Fatal("depth rows must keep the OP.GG rows verbatim; the core-item dedup must stay deleted")
	}
}
