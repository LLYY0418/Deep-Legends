package main

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	xhtml "golang.org/x/net/html"
)

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

func TestValidateChampionAssetPathAllowsOPGGProfileIcons(t *testing.T) {
	host, ok := validateChampionAssetPath("opgg", "/meta/images/profile_icons/profileIcon1594.jpg")
	if !ok || host != opggAssetHost {
		t.Fatalf("profile icon path rejected: %q %v", host, ok)
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
	decoded := `{"average_stats":{"win_rate":48.77,"pick_rate":13.85,"ban_rate":42.49,"first_place":16.07,"avg_place":3.56},"teamData":[{"champion_ids":[11,44,350],"champion_id":0,"champions":[{"id":11,"key":"masteryi","name":"无极剑圣","image_url":"https://opgg-static.akamaized.net/meta/images/lol/16.15.1/champion/MasterYi.png"},{"id":44,"key":"taric","name":"瓦洛兰之盾","image_url":"https://opgg-static.akamaized.net/meta/images/lol/16.15.1/champion/Taric.png"},{"id":350,"key":"yuumi","name":"魔法猫咪","image_url":"https://opgg-static.akamaized.net/meta/images/lol/16.15.1/champion/Yuumi.png"}],"combination_size":3,"play":"342","win_rate":68.13,"first_place_rate":29.53,"average_place":2.85,"pick_rate":0.53}]}`
	teams := parseArenaTeamCompositions(decoded, `"teamData":`, 3)
	if len(teams) != 1 || len(teams[0].Champions) != 3 || teams[0].Champions[1].Key != "taric" {
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

func TestParseChampionBuild(t *testing.T) {
	document, err := xhtml.Parse(strings.NewReader(`<!doctype html><table>
<caption>SummonerSpells Table</caption><thead><tr><th>召唤师技能推荐</th></tr></thead><tbody><tr>
<td><img alt="闪现" src="https://opgg-static.akamaized.net/meta/images/lol/16.15.1/spell/SummonerFlash.png"><img alt="屏障" src="https://opgg-static.akamaized.net/meta/images/lol/16.15.1/spell/SummonerBarrier.png"></td>
<td><strong>69.31</strong><span>4,282 场</span></td><td><strong>49.58%</strong></td></tr></tbody></table>
<table><caption>Items Table</caption><thead><tr><th>出门装</th></tr></thead><tbody><tr>
<td><img alt="多兰之刃" src="https://opgg-static.akamaized.net/meta/images/lol/16.15.1/item/1055.png"></td><td>72.86% 4,491 场</td><td>48.19%</td></tr></tbody></table>
<table><caption>Depth 4 Items Table</caption><thead><tr><th>第四件装备</th></tr></thead><tbody><tr>
<td><img alt="无尽之刃" src="https://opgg-static.akamaized.net/meta/images/lol/16.15.1/item/3031.png"></td><td>62.08% 683 场</td></tr></tbody></table>
<table><caption>Prismatic Items Table</caption><thead><tr><th>棱彩装备</th></tr></thead><tbody><tr>
<td><img alt="残忍" src="https://opgg-static.akamaized.net/meta/images/lol/16.15.1/item/447104.png"></td><td>13.72% 17,675 场</td><td>58.06%</td></tr></tbody></table>`))
	if err != nil {
		t.Fatal(err)
	}
	build := parseChampionBuild(document)
	if len(build.SummonerSpells) != 1 || len(build.SummonerSpells[0].Assets) != 2 {
		t.Fatalf("summoner spells were not parsed: %#v", build.SummonerSpells)
	}
	if build.SummonerSpells[0].PickRate != 69.31 || build.SummonerSpells[0].Games != 4282 || build.SummonerSpells[0].WinRate != 49.58 {
		t.Fatalf("summoner metrics mismatch: %#v", build.SummonerSpells[0])
	}
	if len(build.StarterItems) != 1 || build.StarterItems[0].Assets[0].Kind != "item" {
		t.Fatalf("starter items were not parsed: %#v", build.StarterItems)
	}
	if len(build.FourthItems) != 1 || build.FourthItems[0].WinRate != 62.08 || build.FourthItems[0].PickRate != 0 {
		t.Fatalf("late item metrics were not normalized: %#v", build.FourthItems)
	}
	if len(build.PrismItems) != 1 || build.PrismItems[0].Assets[0].Name != "残忍" || build.PrismItems[0].WinRate != 58.06 {
		t.Fatalf("prismatic items were not parsed: %#v", build.PrismItems)
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
<div><div>对线劣势的英雄</div></div><div><ul><li><a href="/zh-cn/lol/champions/vayne/counters?target_champion=yunara"><img alt="芸阿娜" src="https://opgg-static.akamaized.net/meta/images/lol/16.15.1/champion/Yunara.png"><strong>41.27%</strong><span>126 场</span></a></li></ul></div>
<div><div>强烈对抗</div></div><div><ul><li><a href="/zh-cn/lol/champions/vayne/counters?target_champion=kaisa"><img alt="卡莎" src="https://opgg-static.akamaized.net/meta/images/lol/16.15.1/champion/Kaisa.png"><strong>58.73%</strong><span>1,206 场</span></a></li></ul></div>
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
