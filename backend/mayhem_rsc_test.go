package main

import (
	"os"
	"strings"
	"testing"
)

func TestLocalAugmentGradesAndMeasurementAreExplicit(t *testing.T) {
	rows := []championMetricRow{
		{Score: 50}, {Score: 90}, {Score: 10}, {Score: 80}, {Score: 70},
		{Score: 60}, {Score: 40}, {Score: 30}, {Score: 20},
	}
	applyLocalAugmentGrades(rows)
	want := []string{"B", "S", "B", "A", "A", "B", "B", "B", "B"}
	for index := range rows {
		if rows[index].Grade != want[index] {
			t.Fatalf("grade %d = %q, want %q", index, rows[index].Grade, want[index])
		}
	}
	fallback := make([]championMetricRow, 9)
	applyLocalAugmentGrades(fallback)
	if fallback[0].Grade != "S" || fallback[1].Grade != "A" || fallback[3].Grade != "B" {
		t.Fatalf("missing-score order fallback = %#v", fallback)
	}
	technique := appendMeasurementTechnique("上游统计口径", "当前档位为本地综合评分分位计算，非官方等级；缺少统计指标时按推荐顺序回退")
	if technique != "上游统计口径；当前档位为本地综合评分分位计算，非官方等级；缺少统计指标时按推荐顺序回退" {
		t.Fatalf("measurement technique = %q", technique)
	}
}

func TestMergeMayhemAugmentRowsPreservesPrimaryAndFillsNine(t *testing.T) {
	row := func(id int, score float64) championMetricRow {
		return championMetricRow{Assets: []championAsset{{ID: id, Kind: "augment"}}, Score: score}
	}
	primary := []championMetricRow{row(1, 91), row(2, 82), row(3, 73), row(4, 64), row(5, 55)}
	fallback := []championMetricRow{row(2, 0), row(6, 0), row(7, 0), row(8, 0), row(9, 0), row(10, 0)}
	merged := mergeMayhemAugmentRows(primary, fallback, 9)
	if len(merged) != 9 || merged[0].Assets[0].ID != 1 || merged[1].Score != 82 || merged[5].Assets[0].ID != 6 || merged[8].Assets[0].ID != 9 {
		t.Fatalf("merged augments = %#v", merged)
	}
}

func TestNormalizeAugmentRarityIsShared(t *testing.T) {
	for _, test := range []struct {
		input any
		want  string
	}{{0, "silver"}, {1, "gold"}, {2, "prismatic"}, {4, "event"}, {"kSilver", "silver"}, {"kPrismatic", "prismatic"}, {"1", "unknown"}, {"4", "unknown"}, {"8", "unknown"}} {
		if got := normalizeAugmentRarity(test.input); got != test.want {
			t.Fatalf("normalizeAugmentRarity(%v) = %q, want %q", test.input, got, test.want)
		}
	}
	for _, test := range []struct {
		input int
		want  string
	}{{1, "silver"}, {4, "gold"}, {8, "prismatic"}} {
		if got := augmentRarity(test.input); got != test.want {
			t.Fatalf("legacy rarity %d = %q, want %q", test.input, got, test.want)
		}
	}
}

func TestParseMayhemRSCRealFixture(t *testing.T) {
	fixture := strings.TrimSpace(os.Getenv("MAYHEM_RSC_FIXTURE"))
	if fixture == "" {
		t.Skip("set MAYHEM_RSC_FIXTURE to an OP.GG aram-mayhem RSC payload")
	}
	data, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	result, err := parseMayhemRSC(data, "vayne")
	if err != nil {
		t.Fatal(err)
	}
	if result.Citation.Patch != "16.16.1" {
		t.Fatalf("RSC patch = %q", result.Citation.Patch)
	}
	if len(result.Build.StarterItems) == 0 || len(result.Build.StarterItems[0].Assets) < 2 {
		t.Fatalf("starter items = %#v", result.Build.StarterItems)
	}
	if len(result.Build.Boots) == 0 || len(result.Build.Boots[0].Assets) == 0 {
		t.Fatalf("boots = %#v", result.Build.Boots)
	}
	if len(result.Build.CoreItems) == 0 || len(result.Build.CoreItems[0].Assets) < 3 {
		t.Fatalf("core items = %#v", result.Build.CoreItems)
	}
	if len(result.Build.Skills) == 0 || len(result.Build.Skills[0].SkillPriority) != 3 || len(result.Build.Skills[0].SkillOrder) == 0 {
		t.Fatalf("skills = %#v", result.Build.Skills)
	}
	if len(result.Build.SummonerSpells) == 0 || len(result.Build.SummonerSpells[0].Assets) != 2 || result.Build.SummonerSpells[0].Assets[0].ID != 4 {
		t.Fatalf("summoner spells = %#v", result.Build.SummonerSpells)
	}
}

func TestParseMayhemRSCRejectsNonItemCoreAssets(t *testing.T) {
	source := `72:["$","tr","starter_items_0",{"metaType":"item","metaId":1038}]
73:["$","tr","boots_0",{"metaType":"item","metaId":3006}]
74:["$","tr","core_items_0",{"children":[{"metaType":"item","metaId":3032},{"metaId":4500},{"metaId":4,"metaType":"spell"},{"metaType":"rune","metaId":8345},{"metaType":"item","metaId":126697}]}]
75:["$","tr",null,{"children":[["$","x","spell_0",{"metaId":4,"metaType":"spell"}],["$","x","spell_1",{"metaId":6,"metaType":"spell"}]]}]
76:["$","div",null,{"children":[["$","x","skill_0",{"extraData":"Q"}],["$","x","skill_1",{"extraData":"W"}],["$","x","skill_2",{"extraData":"E"}],["$","span","0",{"children":"Q"}]]}]
90:["$","x",null,{"src":"https://opgg-static.akamaized.net/meta/images/lol/16.16.1/item/3032.png"}]
91:["$","x",null,{"metaId":2095,"metaType":"aram-augment"}]`
	result, err := parseMayhemRSC([]byte(source), "vayne")
	if err != nil {
		t.Fatal(err)
	}
	assets := result.Build.CoreItems[0].Assets
	if len(assets) != 2 || assets[0].ID != 3032 || assets[1].ID != 126697 || assets[1].Kind != "item" {
		t.Fatalf("filtered core assets = %#v", assets)
	}
	want := map[int]string{4: "kind_spell", 4500: "missing_meta_type", 8345: "kind_rune"}
	if len(result.RejectedCoreItems) != len(want) {
		t.Fatalf("core item rejections = %#v", result.RejectedCoreItems)
	}
	for _, rejected := range result.RejectedCoreItems {
		if want[rejected.ID] != rejected.Reason {
			t.Fatalf("unexpected rejection %#v", rejected)
		}
	}
}

func TestParseMayhemRSCResolvesDelayedRows(t *testing.T) {
	source := `72:["$","tr","starter_items_0",{"metaType":"item","metaId":1038,"children":"$L81"}]
73:["$","tr","boots_0",{"metaType":"item","metaId":3006}]
74:["$","tr","core_items_0",{"metaType":"item","metaId":3032,"children":"$L82"}]
75:["$","tr",null,{"children":[["$","x","spell_0",{"metaId":4,"metaType":"spell"}],["$","x","spell_1",{"metaId":6,"metaType":"spell"}]]}]
76:["$","div",null,{"children":[["$","x","skill_0",{"extraData":"Q"}],["$","x","skill_1",{"extraData":"W"}],["$","x","skill_2",{"extraData":"E"}],["$","span","0",{"children":"Q"}]]}]
81:["$","x",null,{"metaId":1042}]
82:["$","x",null,{"metaType":"item","metaId":3085}]
90:["$","x",null,{"src":"https://opgg-static.akamaized.net/meta/images/lol/16.16.1/item/3032.png"}]
91:["$","x",null,{"metaId":2095,"metaType":"aram-augment"}]`
	result, err := parseMayhemRSC([]byte(source), "vayne")
	if err != nil {
		t.Fatal(err)
	}
	if got := result.Build.StarterItems[0].Assets; len(got) != 2 || got[1].ID != 1042 {
		t.Fatalf("delayed starter row = %#v", got)
	}
	if got := result.Build.CoreItems[0].Assets; len(got) != 2 || got[1].ID != 3085 {
		t.Fatalf("delayed core row = %#v", got)
	}
	if len(result.Build.SummonerSpells) != 1 || len(result.Build.SummonerSpells[0].Assets) != 2 {
		t.Fatalf("spell pair = %#v", result.Build.SummonerSpells)
	}
}

func TestRSCSpellRowsIgnoreExpandedAugmentMetadata(t *testing.T) {
	source := `6d:["$","section",null,{"children":[{"metaId":2010,"metaType":"aram-augment"},{"metaType":"aram-augment","metaId":1225},["$","x","spell_0",{"metaId":4,"metaType":"spell"}],["$","x","spell_1",{"metaType":"spell","metaId":6}]]}]`
	rows := rscSpellRows(source)
	if len(rows) != 1 || len(rows[0]) != 2 || rows[0][0] != 4 || rows[0][1] != 6 {
		t.Fatalf("spell rows = %#v", rows)
	}
}

func TestReportRSCRejectedItemsEmitsActualIDAndReason(t *testing.T) {
	events := make([]map[string]any, 0, 2)
	provider := &championProvider{diag: func(event map[string]any) { events = append(events, event) }}
	provider.reportRSCRejectedItems([]rscRejectedItem{
		{ID: 4500, Reason: "missing_meta_type"},
		{ID: 8345, Reason: "kind_rune"},
	})
	if len(events) != 2 {
		t.Fatalf("diagnostic events = %#v", events)
	}
	if events[0]["event"] != "rsc_core_item_rejected" || events[0]["id"] != 4500 || events[0]["reason"] != "missing_meta_type" {
		t.Fatalf("first diagnostic event = %#v", events[0])
	}
	if events[1]["event"] != "rsc_core_item_rejected" || events[1]["id"] != 8345 || events[1]["reason"] != "kind_rune" {
		t.Fatalf("second diagnostic event = %#v", events[1])
	}
}
