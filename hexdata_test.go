package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const hexdataTestBuild = "hexdata-2026-08-19-test"

func hexdataTestPage(body string) []byte {
	return []byte(`<html><head><meta name="hexdata-build-id" content="` + hexdataTestBuild + `"></head><body data-hexdata-build-id="` + hexdataTestBuild + `"><main><h1>Patch 16.16</h1><p>数据日期 2026-08-19</p><section data-primary-content>` + body + `</section></main></body></html>`)
}

func hexdataAugmentsTestPage() []byte {
	var rows strings.Builder
	rows.WriteString(`<table><tbody>`)
	for id := 1000; id < 1150; id++ {
		fmt.Fprintf(&rows, `<tr><td><a href="/augment/%d-augment%d">海克斯%d</a></td><td>globalHexScore 80.0 · 胜率 55.0%%</td></tr>`, id, id, id)
	}
	rows.WriteString(`</tbody></table>`)
	return hexdataTestPage(rows.String())
}

func hexdataRankingFixtures(t *testing.T) ([]byte, []byte) {
	t.Helper()
	answer := hexdataAnswerCards{BuildID: hexdataTestBuild, ReportPatch: "16.16", ReportDate: "2026-08-19"}
	answer.Methodology.RecommendationPolicy.Ranking = "sample_tier_then_wilson_lower_bound_v1"
	answer.Methodology.RecommendationPolicy.WilsonZ = 1.96
	answer.Methodology.SamplePolicy.Medium.Min = 250
	answer.Methodology.SamplePolicy.High.Min = 1000
	answer.HeroAliases = make([]struct {
		ID             string   `json:"id"`
		URL            string   `json:"url"`
		Name           string   `json:"name"`
		AlternateNames []string `json:"alternateNames"`
	}, 172)
	for index := range answer.HeroAliases {
		id := index + 1
		answer.HeroAliases[index].ID = fmt.Sprint(id)
		answer.HeroAliases[index].URL = fmt.Sprintf("/hero/%d-hero%d", id, id)
		answer.HeroAliases[index].Name = fmt.Sprintf("英雄%d", id)
	}
	answerData, err := json.Marshal(answer)
	if err != nil {
		t.Fatal(err)
	}
	var rows strings.Builder
	rows.WriteString(`<table><tbody>`)
	for id := 1; id <= 172; id++ {
		name := fmt.Sprintf("英雄%d", id)
		if id == 1 {
			name += "（昵称）"
		}
		fmt.Fprintf(&rows, `<tr><td><a href="/hero/%d-hero%d">%s</a></td><td>胜率 %.1f%% · 样本 1,000</td><td>查看详情</td></tr>`, id, id, name, 60-float64(id)/10)
	}
	rows.WriteString(`</tbody></table>`)
	return answerData, hexdataTestPage(rows.String())
}

func newHexdataBudgetProvider(t *testing.T, root string, transport http.RoundTripper) *championProvider {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, championDataCacheDirectory), 0o700); err != nil {
		t.Fatal(err)
	}
	store := &localStore{root: root}
	provider := newChampionProvider()
	provider.cache = newChampionDataCache(store)
	provider.hexdata = newHexdataClient(provider, store)
	provider.hexdata.minInterval = 0
	provider.hexdata.maximumJitter = 0
	provider.hexdata.retryDelay = 0
	provider.client = &http.Client{Transport: transport}
	return provider
}

func hexdataResponse(request *http.Request, data []byte) *http.Response {
	return &http.Response{
		StatusCode:    http.StatusOK,
		Header:        make(http.Header),
		Body:          io.NopCloser(bytes.NewReader(data)),
		ContentLength: int64(len(data)),
		Request:       request,
	}
}

func TestParseHexdataHeroesRequiresCompleteShapeAndCalculatesTiers(t *testing.T) {
	var rows strings.Builder
	rows.WriteString(`<table><tbody>`)
	for id := 1; id <= 172; id++ {
		fmt.Fprintf(&rows, `<tr><td><a href="/hero/%d-hero%d">英雄%d</a></td><td>胜率 %.1f%% · 样本 1,000</td><td>查看详情</td></tr>`, id, id, id, 60-float64(id)/10)
	}
	rows.WriteString(`</tbody></table>`)
	answer := hexdataAnswerCards{BuildID: hexdataTestBuild, ReportPatch: "16.16", ReportDate: "2026-08-19"}
	answer.Methodology.RecommendationPolicy.WilsonZ = 1.96
	answer.Methodology.SamplePolicy.Medium.Min = 250
	answer.Methodology.SamplePolicy.High.Min = 1000
	answer.HeroAliases = make([]struct {
		ID             string   `json:"id"`
		URL            string   `json:"url"`
		Name           string   `json:"name"`
		AlternateNames []string `json:"alternateNames"`
	}, 172)
	parsed, citation, err := parseHexdataHeroes(hexdataTestPage(rows.String()), answer)
	if err != nil || len(parsed) != 172 || citation.BuildID != hexdataTestBuild {
		t.Fatalf("heroes parse = %d %#v %v", len(parsed), citation, err)
	}
	if parsed[0].TierBand != 2 || parsed[0].Wilson <= parsed[len(parsed)-1].Wilson {
		t.Fatalf("local tier order not applied: %#v %#v", parsed[0], parsed[len(parsed)-1])
	}
	if parsed[0].Name != "英雄1" {
		t.Fatalf("champion nickname was not removed: %q", parsed[0].Name)
	}
	broken := strings.Replace(rows.String(), `<tr><td><a href="/hero/172-hero172">`, `<tr><td>`, 1)
	if _, _, err := parseHexdataHeroes(hexdataTestPage(broken), answer); err == nil {
		t.Fatal("expected incomplete heroes shape to fail")
	}
}

func TestParseHexdataHeroDetailFiltersLowSampleRecommendations(t *testing.T) {
	var body strings.Builder
	body.WriteString(`<p>胜率 57.8% · 样本 1,720,638 场，层级 T1</p><table><tbody>`)
	for id := 1; id <= 8; id++ {
		games := 1000
		if id == 2 {
			games = 1
		}
		fmt.Fprintf(&body, `<tr><td><a href="/augment/%d-augment%d">海克斯%d</a></td><td>%.1f</td><td>60.0%%</td><td>%d</td></tr>`, id, id, id, 90-float64(id), games)
	}
	body.WriteString(`</tbody></table><table><tbody>`)
	for id := 1; id <= 8; id++ {
		fmt.Fprintf(&body, `<tr><td>装备%d</td><td>%.1f</td><td>55.0%%</td><td>1,000</td></tr>`, id, 80-float64(id))
	}
	body.WriteString(`</tbody></table>`)
	detail, err := parseHexdataHeroDetail(hexdataTestPage(body.String()), "/hero/67-vayne")
	if err != nil || detail.Tier != 1 || len(detail.Augments) != 7 || len(detail.Items) != 8 {
		t.Fatalf("detail shape = %#v %v", detail, err)
	}
	for _, row := range detail.Augments {
		if row.Games < hexdataMinimumSample {
			t.Fatalf("low sample recommendation leaked: %#v", row)
		}
	}
	broken := strings.Replace(body.String(), `<tr><td>装备8`, `<tr><td>`, 1)
	if _, err := parseHexdataHeroDetail(hexdataTestPage(broken), "/hero/67-vayne"); err == nil {
		t.Fatal("expected 8+8 shape gate to fail")
	}
}

func TestParseHexdataHeroDetailAcceptsMetricSeparatorsAndExpandedShape(t *testing.T) {
	for _, separator := range []string{"·", "，", ","} {
		t.Run(separator, func(t *testing.T) {
			var body strings.Builder
			fmt.Fprintf(&body, `<p>胜率 57.8%%%s样本 1,720,638 场，层级 T1</p><table><tbody><tr><td>说明</td></tr></tbody></table><table><tbody>`, separator)
			for id := 1; id <= 9; id++ {
				href := fmt.Sprintf("/augment/%d-augment%d", id, id)
				if id%2 == 0 {
					href = "https://hexdata.com.cn" + href + "?source=hero#row"
				}
				fmt.Fprintf(&body, `<tr><td><a href="%s">海克斯%d</a></td><td>%.1f</td><td>60.0%%</td><td>1,000</td></tr>`, href, id, 90-float64(id))
			}
			body.WriteString(`</tbody></table><table><tbody>`)
			for id := 1; id <= 9; id++ {
				fmt.Fprintf(&body, `<tr><td>装备%d</td><td>%.1f</td><td>55.0%%</td><td>1,000</td></tr>`, id, 80-float64(id))
			}
			body.WriteString(`</tbody></table>`)
			detail, err := parseHexdataHeroDetail(hexdataTestPage(body.String()), "/hero/67-vayne")
			if err != nil || detail.WinRate != 57.8 || detail.Games != 1720638 || len(detail.Augments) != 9 || len(detail.Items) != 9 {
				t.Fatalf("separator %q detail = %#v err=%v", separator, detail, err)
			}
		})
	}
}

func TestParseHexdataAugmentsAcceptsMetricSeparatorsAndURLShapes(t *testing.T) {
	for _, separator := range []string{"·", "，", ","} {
		t.Run(separator, func(t *testing.T) {
			var body strings.Builder
			body.WriteString(`<table><tbody>`)
			for id := 1; id <= 150; id++ {
				href := fmt.Sprintf("/augment/%d-augment%d", id, id)
				if id%2 == 0 {
					href = "https://hexdata.com.cn" + href + "?source=global#row"
				}
				fmt.Fprintf(&body, `<tr><td><a href="%s">海克斯%d</a></td><td>globalHexScore %.1f %s 胜率 60.0%%</td></tr>`, href, id, 90-float64(id)/10, separator)
			}
			body.WriteString(`</tbody></table>`)
			rows, citation, err := parseHexdataAugments(hexdataTestPage(body.String()))
			if err != nil || len(rows) != 150 || rows[1].ID != 2 || citation.BuildID != hexdataTestBuild {
				t.Fatalf("separator %q augments=%d second=%#v citation=%#v err=%v", separator, len(rows), rows[1], citation, err)
			}
		})
	}
}

func TestHexdataHeroShapeReportsActualRecommendationAndItemCounts(t *testing.T) {
	var event map[string]any
	provider := &championProvider{diag: func(payload map[string]any) { event = payload }}
	provider.reportHexdataHeroShape(hexdataHeroDetail{
		Citation: championSourceCitation{BuildID: hexdataTestBuild},
		Augments: make([]championMetricRow, 7),
		Items:    make([]championMetricRow, 8),
	})
	if event["event"] != "hexdata_shape" || event["kind"] != "hero" || event["rows"] != 7 || event["fields"] != 8 || event["buildId"] != hexdataTestBuild {
		t.Fatalf("hero shape event = %#v", event)
	}
}

func TestParseHexdataAugmentDetailAndRarityRequireExactRows(t *testing.T) {
	var detail strings.Builder
	detail.WriteString(`<table><tbody>`)
	for id := 1; id <= 12; id++ {
		fmt.Fprintf(&detail, `<tr><td><a href="/hero/%d-hero%d">英雄%d</a></td><td>90.0</td><td>60.0%%</td><td>1,000</td></tr>`, id, id, id)
	}
	detail.WriteString(`</tbody></table>`)
	parsed, err := parseHexdataAugmentDetail(hexdataTestPage(detail.String()), "/augment/1-test")
	if err != nil || len(parsed.Rows) != 12 {
		t.Fatalf("augment detail = %#v %v", parsed, err)
	}
	if _, err := parseHexdataAugmentDetail(hexdataTestPage(strings.Replace(detail.String(), `<tr><td><a href="/hero/12-hero12">`, `<tr><td>`, 1)), "/augment/1-test"); err == nil {
		t.Fatal("expected missing augment hero to fail")
	}
	rarity := `<table><tbody><tr><td>第 1 阶段</td><td>白银 10.6% · 黄金 58.5% · 棱彩 30.9%</td><td>1,000</td></tr><tr><td>第 2 阶段</td><td>白银 28.8% · 黄金 43.8% · 棱彩 27.4%</td><td>1,000</td></tr><tr><td>第 3 阶段</td><td>白银 28.8% · 黄金 43.8% · 棱彩 27.4%</td><td>1,000</td></tr><tr><td>第 4 阶段</td><td>白银 28.8% · 黄金 43.7% · 棱彩 27.6%</td><td>1,000</td></tr></tbody></table>`
	stages, _, err := parseHexdataRarity(hexdataTestPage(rarity))
	if err != nil || len(stages) != 4 || stages[0].Gold != 58.5 {
		t.Fatalf("rarity = %#v %v", stages, err)
	}
	for _, separator := range []string{"，", ","} {
		variant := strings.ReplaceAll(rarity, " · ", " "+separator+" ")
		stages, _, err := parseHexdataRarity(hexdataTestPage(variant))
		if err != nil || len(stages) != 4 || stages[0].Gold != 58.5 {
			t.Fatalf("rarity separator %q = %#v %v", separator, stages, err)
		}
	}
}

func TestHexdataShapeFailureDistinguishesParseFromEmptyPayload(t *testing.T) {
	events := make([]map[string]any, 0, 2)
	provider := &championProvider{diag: func(event map[string]any) { events = append(events, event) }}
	client := newHexdataClient(provider, nil)
	client.recordShapeFailure("augments", 0, 0, hexdataTestBuild)
	client.recordLoadFailure("augments", errHexdataEmptyPayload)
	if len(events) < 2 {
		t.Fatalf("shape diagnostics = %#v", events)
	}
	parseEvent, emptyEvent := events[0], events[1]
	if parseEvent["failure_kind"] != "parse" || parseEvent["buildId"] != hexdataTestBuild {
		t.Fatalf("parse shape diagnostic = %#v", parseEvent)
	}
	if emptyEvent["failure_kind"] != "empty" {
		t.Fatalf("empty payload diagnostic = %#v", emptyEvent)
	}
}

func TestInspectHexdataPayloadReportsRawFieldsWhenRowsAreRejected(t *testing.T) {
	var body strings.Builder
	body.WriteString(`<table><tbody>`)
	for id := 1; id <= 150; id++ {
		fmt.Fprintf(&body, `<tr><td><a href="https://hexdata.com.cn/augment/%d-augment%d?source=shape">海克斯%d</a></td><td>globalHexScore 80.0</td><td>胜率 55.0%%</td></tr>`, id, id, id)
	}
	body.WriteString(`</tbody></table>`)
	var event map[string]any
	shape, err := inspectHexdataPayload("augments", "/augments", hexdataTestPage(body.String()), func(payload map[string]any) { event = payload })
	if err == nil {
		t.Fatal("expected split metric columns to reject every parsed row")
	}
	if shape.Rows != 0 || shape.Fields != 450 || shape.BuildID != hexdataTestBuild {
		t.Fatalf("shape = %#v, want rows=0 fields=450 build=%q", shape, hexdataTestBuild)
	}
	if event["event"] != "hexdata_table_shape" || event["kind"] != "augments" || event["table_count"] != 1 {
		t.Fatalf("table shape event = %#v", event)
	}
	if counts := event["row_cell_count_counts"].(map[string]int); counts["3"] != 150 || len(counts) != 1 {
		t.Fatalf("row cell counts = %#v", counts)
	}
	columns := event["columns"].([]map[string]any)
	if len(columns) != 3 || columns[0]["link_present_counts"].(map[string]int)["present"] != 150 || columns[0]["link_path_head_counts"].(map[string]int)["/augment"] != 150 || columns[0]["augment_path_match_counts"].(map[string]int)["matched"] != 150 {
		t.Fatalf("link column shape = %#v", columns)
	}
	if columns[1]["text_class_counts"].(map[string]int)["has_globalhexscore"] != 150 || columns[2]["text_class_counts"].(map[string]int)["has_胜率+has_percent_sign"] != 150 {
		t.Fatalf("metric column shape = %#v", columns)
	}
}

// The page's <meta name="description"> is a win-rate blurb ("…海克斯大乱斗胜率
// 53.2%，选取率 0.4%…"). Only the guide paragraph carries the augment's own
// gameplay text, and it is the sole source of Chinese copy for Mayhem augments.
func TestParseHexdataAugmentDetailUsesGuideParagraphNotSEOBlurb(t *testing.T) {
	var detail strings.Builder
	detail.WriteString(`<meta name="description" content="Patch 16.16 双刀流海克斯大乱斗胜率 52.7%，选取率 1.1%，样本 5,403,684 场。">`)
	detail.WriteString(`<table><tbody>`)
	for id := 1; id <= 12; id++ {
		href := fmt.Sprintf("/hero/%d-hero%d", id, id)
		if id%2 == 0 {
			href = "https://hexdata.com.cn" + href + "?source=detail#row"
		}
		fmt.Fprintf(&detail, `<tr><td><a href="%s">英雄%d</a></td><td>90.0</td><td>60.0%%</td><td>1,000</td></tr>`, href, id)
	}
	detail.WriteString(`</tbody></table>`)
	detail.WriteString(`<section class="seo-guide-section" data-seo-guide><h2>双刀流适合英雄和出装思路</h2>`)
	// The lead-in sentence deliberately ends in 考虑。 as well: the anchor has to
	// be the full phrase, otherwise the cut lands on the wrong sentence.
	detail.WriteString(`<p>双刀流值得考虑。双刀流更适合先在蛮族之王、虚空女皇这类高分高样本英雄上考虑。在攻击时，发射一个箭矢，它施加效能削减的攻击特效。获得10%总攻击速度。如果你的英雄机制能稳定触发这个海克斯，优先看上方适配英雄。</p></section>`)
	parsed, err := parseHexdataAugmentDetail(hexdataTestPage(detail.String()), "/augment/1225-dual-wield")
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Description != "在攻击时，发射一个箭矢，它施加效能削减的攻击特效。获得10%总攻击速度。" {
		t.Fatalf("augment description = %q", parsed.Description)
	}
	if strings.Contains(parsed.Description, "胜率") {
		t.Fatalf("SEO win-rate blurb leaked into the description: %q", parsed.Description)
	}
}

// Without both anchors the paragraph shape has changed; returning an empty
// description is correct, silently shipping the surrounding prose is not.
func TestParseHexdataAugmentDetailRejectsUnanchoredGuideProse(t *testing.T) {
	var detail strings.Builder
	detail.WriteString(`<table><tbody>`)
	for id := 1; id <= 12; id++ {
		fmt.Fprintf(&detail, `<tr><td><a href="/hero/%d-hero%d">英雄%d</a></td><td>90.0</td><td>60.0%%</td><td>1,000</td></tr>`, id, id, id)
	}
	detail.WriteString(`</tbody></table>`)
	detail.WriteString(`<section data-seo-guide><p>双刀流是一个很强的海克斯，值得优先考虑。</p></section>`)
	parsed, err := parseHexdataAugmentDetail(hexdataTestPage(detail.String()), "/augment/1225-dual-wield")
	if err != nil || parsed.Description != "" {
		t.Fatalf("unanchored prose was accepted: %q err=%v", parsed.Description, err)
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

func TestRSCSpellRowsIgnoreExpandedAugmentMetadata(t *testing.T) {
	source := `6d:["$","section",null,{"children":[{"metaId":2010,"metaType":"aram-augment"},{"metaType":"aram-augment","metaId":1225},["$","x","spell_0",{"metaId":4,"metaType":"spell"}],["$","x","spell_1",{"metaType":"spell","metaId":6}]]}]`
	rows := rscSpellRows(source)
	if len(rows) != 1 || len(rows[0]) != 2 || rows[0][0] != 4 || rows[0][1] != 6 {
		t.Fatalf("spell rows = %#v", rows)
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

func TestHexdataColdRankingBudgetAndDiskRestart(t *testing.T) {
	root := t.TempDir()
	answerData, heroesData := hexdataRankingFixtures(t)
	var requests atomic.Int32
	transport := championRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests.Add(1)
		switch request.URL.Path {
		case hexdataAnswerPath:
			return hexdataResponse(request, answerData), nil
		case "/heroes":
			return hexdataResponse(request, heroesData), nil
		default:
			return &http.Response{StatusCode: http.StatusNotFound, Header: make(http.Header), Body: http.NoBody, Request: request}, nil
		}
	})
	provider := newHexdataBudgetProvider(t, root, transport)
	ranking, err := provider.loadHexdataRankings(context.Background())
	if err != nil || len(ranking.Rows) != 172 {
		t.Fatalf("cold ranking rows=%d err=%v", len(ranking.Rows), err)
	}
	if got := requests.Load(); got != 2 {
		t.Fatalf("cold ranking Hexdata requests = %d, want 2", got)
	}

	restarted := newHexdataBudgetProvider(t, root, transport)
	ranking, err = restarted.loadHexdataRankings(context.Background())
	if err != nil || len(ranking.Rows) != 172 {
		t.Fatalf("disk ranking rows=%d err=%v", len(ranking.Rows), err)
	}
	if got := requests.Load(); got != 2 {
		t.Fatalf("restart made %d additional Hexdata requests", got-2)
	}
}

func TestHexdataSameHeroTenConcurrentLoadsUseOneRequestAndRestartUsesNone(t *testing.T) {
	root := t.TempDir()
	heroData := hexdataTestPage(`<p>singleflight hero fixture</p>`)
	var requests atomic.Int32
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	transport := championRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests.Add(1)
		select {
		case entered <- struct{}{}:
		default:
		}
		<-release
		return hexdataResponse(request, heroData), nil
	})
	provider := newHexdataBudgetProvider(t, root, transport)
	provider.hexdata.adoptCitation(championSourceCitation{Source: "Hexdata", Patch: "16.16", ReportDate: "2026-08-19", BuildID: hexdataTestBuild, CanonicalURL: "https://hexdata.com.cn/hero/67-vayne"})

	const concurrentLoads = 10
	start := make(chan struct{})
	errorsSeen := make(chan error, concurrentLoads)
	var workers sync.WaitGroup
	for range concurrentLoads {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			page, err := provider.hexdata.load(context.Background(), "hero", "67", "/hero/67-vayne", "text/html,application/xhtml+xml", false)
			if err == nil && len(page.Data) == 0 {
				err = fmt.Errorf("empty hero page")
			}
			errorsSeen <- err
		}()
	}
	close(start)
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("Hexdata request did not start")
	}
	close(release)
	workers.Wait()
	close(errorsSeen)
	for err := range errorsSeen {
		if err != nil {
			t.Fatal(err)
		}
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("ten same-hero loads made %d Hexdata requests, want 1", got)
	}

	restarted := newHexdataBudgetProvider(t, root, transport)
	page, err := restarted.hexdata.load(context.Background(), "hero", "67", "/hero/67-vayne", "text/html,application/xhtml+xml", false)
	if err != nil || len(page.Data) == 0 {
		t.Fatalf("restart disk load bytes=%d err=%v", len(page.Data), err)
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("restart made %d additional same-hero requests", got-1)
	}
}

func TestHexdataBuildSoftTTLRefreshesThroughUserRequestedPage(t *testing.T) {
	root := t.TempDir()
	oldPage := hexdataTestPage(`<p>old build</p>`)
	newPage := []byte(strings.ReplaceAll(string(hexdataTestPage(`<p>new build</p>`)), hexdataTestBuild, "hexdata-2026-08-21-next"))
	var requests atomic.Int32
	transport := championRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests.Add(1)
		if request.URL.Path != "/hero/67-vayne" {
			t.Fatalf("unexpected refresh path %q", request.URL.Path)
		}
		return hexdataResponse(request, newPage), nil
	})
	provider := newHexdataBudgetProvider(t, root, transport)
	checkedAt := time.Now().Add(-hexdataBuildSoftTTL - time.Hour)
	provider.hexdata.now = func() time.Time { return checkedAt }
	provider.hexdata.adoptCitation(championSourceCitation{Source: "Hexdata", Patch: "16.16", ReportDate: "2026-08-19", BuildID: hexdataTestBuild, CanonicalURL: "https://hexdata.com.cn/hero/67-vayne"})
	provider.hexdata.promote("hero", "67", hexdataTestBuild, oldPage, checkedAt)
	provider.hexdata.now = time.Now

	page, err := provider.hexdata.load(context.Background(), "hero", "67", "/hero/67-vayne", "text/html,application/xhtml+xml", false)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(page.Data, []byte("new build")) || bytes.Contains(page.Data, []byte("old build")) {
		t.Fatalf("soft TTL returned stale page: %s", page.Data)
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("soft TTL refresh requests = %d, want 1", got)
	}
}

func TestHexdataCircuitStatePersistsAcrossRestart(t *testing.T) {
	root := t.TempDir()
	statePath := filepath.Join(root, championDataCacheDirectory, "hexdata-state.json")
	if err := os.Mkdir(filepath.Dir(statePath), 0o700); err != nil {
		t.Fatal(err)
	}
	provider := newChampionProvider()
	client := newHexdataClient(provider, &localStore{root: root})
	now := time.Now().UTC()
	client.now = func() time.Time { return now }
	for range hexdataCircuitFailureLimit {
		client.recordFailure("heroes", 429, true)
	}
	restarted := newHexdataClient(provider, &localStore{root: root})
	restarted.now = func() time.Time { return now.Add(30 * time.Second) }
	circuit := restarted.circuitSnapshot("heroes")
	if !circuit.Until.Equal(now.Add(time.Minute)) {
		t.Fatalf("persisted circuit = %#v", circuit)
	}
	if _, err := restarted.load(context.Background(), "heroes", "all", "/heroes", "text/html,application/xhtml+xml", false); err == nil || !strings.Contains(err.Error(), "circuit") {
		t.Fatalf("open circuit did not block request: %v", err)
	}
	data, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if json.Unmarshal(data, &raw) != nil {
		t.Fatal("invalid persisted state")
	}
}

func TestHexdataRestartClearsLongLivedPersistedCircuit(t *testing.T) {
	root := t.TempDir()
	statePath := filepath.Join(root, championDataCacheDirectory, "hexdata-state.json")
	if err := os.MkdirAll(filepath.Dir(statePath), 0o700); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	state := hexdataState{
		Failures:       4,
		CircuitUntil:   now.Add(24 * time.Hour),
		CircuitProbeAt: now.Add(23 * time.Hour),
	}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statePath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	provider := newChampionProvider()
	client := newHexdataClient(provider, &localStore{root: root})
	client.now = func() time.Time { return now }
	client.loadState()
	got := client.snapshot()
	if !got.CircuitUntil.IsZero() || !got.CircuitProbeAt.IsZero() || got.Failures != 0 {
		t.Fatalf("long-lived persisted circuit was not cleared: %#v", got)
	}
}

func TestHexdataShapeCircuitBacksOffAndSuccessResetsIt(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, championDataCacheDirectory), 0o700); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	client := newHexdataClient(newChampionProvider(), &localStore{root: root})
	client.now = func() time.Time { return now }

	for failure := 1; failure < hexdataCircuitFailureLimit; failure++ {
		client.recordShapeFailure("heroes", 8, 40, hexdataTestBuild)
		circuit := client.circuitSnapshot("heroes")
		if !circuit.Until.IsZero() || !circuit.ProbeAt.IsZero() || circuit.Failures != failure {
			t.Fatalf("shape failure %d tripped early: %#v", failure, circuit)
		}
	}
	durations := []time.Duration{time.Minute, 5 * time.Minute, 15 * time.Minute, hexdataCircuitDuration}
	for index, duration := range durations {
		client.recordShapeFailure("heroes", 8, 40, hexdataTestBuild)
		circuit := client.circuitSnapshot("heroes")
		failure := index + hexdataCircuitFailureLimit
		if !circuit.Until.Equal(now.Add(duration)) || circuit.Failures != failure {
			t.Fatalf("shape failure %d state = %#v", failure, circuit)
		}
		probeDelay := min(duration, hexdataCircuitProbeInterval)
		if !circuit.ProbeAt.Equal(now.Add(probeDelay)) {
			t.Fatalf("shape failure %d probe = %v, want %v", failure, circuit.ProbeAt, now.Add(probeDelay))
		}
		if other := client.circuitSnapshot("augments"); other.Failures != 0 || !other.Until.IsZero() {
			t.Fatalf("heroes shape failure leaked into augments: %#v", other)
		}
	}
	client.recordShapeFailure("heroes", 8, 40, hexdataTestBuild)
	if circuit := client.circuitSnapshot("heroes"); circuit.Failures != hexdataCircuitFailureMax || !circuit.Until.Equal(now.Add(hexdataCircuitDuration)) {
		t.Fatalf("failure count was not capped: %#v", circuit)
	}

	now = now.Add(hexdataCircuitFailureDecay + time.Minute)
	client.recordShapeFailure("heroes", 8, 40, hexdataTestBuild)
	if circuit := client.circuitSnapshot("heroes"); circuit.Failures != 1 || !circuit.Until.IsZero() || !circuit.ProbeAt.IsZero() {
		t.Fatalf("failure window did not decay: %#v", circuit)
	}

	client.recordSuccess("heroes", "/heroes")
	if circuit := client.circuitSnapshot("heroes"); !circuit.Until.IsZero() || !circuit.ProbeAt.IsZero() || circuit.Failures != 0 {
		t.Fatalf("successful request did not reset circuit state: %#v", circuit)
	}
}

func TestHexdataCircuitStateWorksWithoutPersistentStore(t *testing.T) {
	client := newHexdataClient(newChampionProvider(), nil)
	client.recordShapeFailure("augments", 0, 0, hexdataTestBuild)
	circuit := client.circuitSnapshot("augments")
	if circuit.Failures != 1 || !circuit.Until.IsZero() {
		t.Fatalf("single shape failure should not trip the circuit: %#v", circuit)
	}
	client.recordShapeFailure("augments", 0, 0, hexdataTestBuild)
	client.recordShapeFailure("augments", 0, 0, hexdataTestBuild)
	circuit = client.circuitSnapshot("augments")
	remaining := circuit.Until.Sub(client.now())
	if circuit.Failures != 3 || remaining <= 0 || remaining > time.Minute {
		t.Fatalf("in-memory circuit was not initialized: %#v", circuit)
	}
	client.recordSuccess("augments", "/augments")
	for range hexdataCircuitFailureLimit {
		client.recordLoadFailure("augments", errHexdataEmptyPayload)
	}
	circuit = client.circuitSnapshot("augments")
	remaining = circuit.Until.Sub(client.now())
	if circuit.Failures != 3 || remaining <= 0 || remaining > 30*time.Second {
		t.Fatalf("empty-payload circuit did not use the short backoff: %#v", circuit)
	}
}

func TestHexdataHTTP429OpensPersistentCircuitBeforeNextRequest(t *testing.T) {
	root := t.TempDir()
	var requests atomic.Int32
	transport := championRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests.Add(1)
		return &http.Response{StatusCode: http.StatusTooManyRequests, Header: make(http.Header), Body: http.NoBody, Request: request}, nil
	})
	provider := newHexdataBudgetProvider(t, root, transport)
	now := time.Now().UTC()
	provider.hexdata.now = func() time.Time { return now }
	for attempt := 0; attempt < hexdataCircuitFailureLimit; attempt++ {
		if _, err := provider.hexdata.load(context.Background(), "heroes", "all", "/heroes", "text/html,application/xhtml+xml", false); err == nil {
			t.Fatal("expected HTTP 429 to fail")
		}
	}
	if got := requests.Load(); got != hexdataCircuitFailureLimit {
		t.Fatalf("HTTP 429 made %d requests, want %d without retries", got, hexdataCircuitFailureLimit)
	}
	if circuit := provider.hexdata.circuitSnapshot("heroes"); !circuit.Until.Equal(now.Add(time.Minute)) || circuit.Failures != hexdataCircuitFailureLimit {
		t.Fatalf("HTTP 429 circuit = %#v", circuit)
	}

	restarted := newHexdataBudgetProvider(t, root, transport)
	restarted.hexdata.now = func() time.Time { return now.Add(30 * time.Second) }
	if _, err := restarted.hexdata.load(context.Background(), "heroes", "all", "/heroes", "text/html,application/xhtml+xml", false); err == nil || !strings.Contains(err.Error(), "circuit") {
		t.Fatalf("persisted HTTP 429 circuit did not block request: %v", err)
	}
	if got := requests.Load(); got != 3 {
		t.Fatalf("open circuit made %d additional requests", got-3)
	}
	if _, err := restarted.hexdata.load(context.Background(), "augments", "all", "/augments", "text/html,application/xhtml+xml", false); err == nil || !strings.Contains(err.Error(), "HTTP 429") {
		t.Fatalf("independent augment request error = %v", err)
	}
	if got := requests.Load(); got != 4 {
		t.Fatalf("independent augment request count = %d, want 4", got)
	}

	restarted.hexdata.now = func() time.Time { return now.Add(time.Minute) }
	if _, err := restarted.hexdata.load(context.Background(), "heroes", "all", "/heroes", "text/html,application/xhtml+xml", false); err == nil || !strings.Contains(err.Error(), "HTTP 429") {
		t.Fatalf("half-open probe error = %v", err)
	}
	if got := requests.Load(); got != 5 {
		t.Fatalf("recovery request made %d requests, want one additional request", got)
	}
}

func TestHexdataEmptyPayloadKeepsLastSuccessfulCacheAndTripsOnlyAfterThreeFailures(t *testing.T) {
	root := t.TempDir()
	var requests atomic.Int32
	provider := newHexdataBudgetProvider(t, root, championRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests.Add(1)
		return hexdataResponse(request, nil), nil
	}))
	provider.hexdata.adoptCitation(championSourceCitation{Source: "Hexdata", Patch: "16.16", ReportDate: "2026-08-19", BuildID: hexdataTestBuild, CanonicalURL: "https://hexdata.com.cn/augments"})
	fetchedAt := time.Now().Add(-11 * 365 * 24 * time.Hour)
	valid := hexdataAugmentsTestPage()
	provider.hexdata.promote("augments", "all", hexdataTestBuild, valid, fetchedAt)

	for failure := 1; failure <= hexdataCircuitFailureLimit; failure++ {
		page, err := provider.hexdata.load(context.Background(), "augments", "all", "/augments", "text/html,application/xhtml+xml", false)
		if err != nil || page.Cache != championCacheStateStale || !bytes.Equal(page.Data, valid) {
			t.Fatalf("failure %d stale page = cache:%q bytes:%d err:%v", failure, page.Cache, len(page.Data), err)
		}
		circuit := provider.hexdata.circuitSnapshot("augments")
		if circuit.Failures != failure {
			t.Fatalf("failure %d circuit = %#v", failure, circuit)
		}
		if failure < hexdataCircuitFailureLimit && !circuit.Until.IsZero() {
			t.Fatalf("failure %d tripped early: %#v", failure, circuit)
		}
	}
	if circuit := provider.hexdata.circuitSnapshot("augments"); circuit.Until.IsZero() || circuit.Until.Sub(provider.hexdata.now()) > time.Minute {
		t.Fatalf("empty payload did not use short backoff: %#v", circuit)
	}
	if got := requests.Load(); got != 2*hexdataCircuitFailureLimit {
		t.Fatalf("empty payload request attempts = %d, want %d", got, 2*hexdataCircuitFailureLimit)
	}
	provider.hexdata.recordSuccess("augments", "/augments")
}

func TestHexdataCircuitProbesLazilyOnNextLoadAndClosesOnSuccess(t *testing.T) {
	var requests atomic.Int32
	provider := newChampionProvider()
	provider.client = &http.Client{Transport: championRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests.Add(1)
		return hexdataResponse(request, hexdataAugmentsTestPage()), nil
	})}
	client := newHexdataClient(provider, nil)
	provider.hexdata = client
	client.minInterval = 0
	client.maximumJitter = 0
	client.retryDelay = 0
	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	client.now = func() time.Time { return now }
	for range hexdataCircuitFailureLimit {
		client.recordShapeFailure("augments", 0, 0, "")
	}
	circuit := client.circuitSnapshot("augments")
	client.mu.Lock()
	circuit.Until = now.Add(5 * time.Minute)
	circuit.ProbeAt = now.Add(time.Minute)
	client.state.Circuits["augments"] = circuit
	client.mu.Unlock()
	time.Sleep(30 * time.Millisecond)
	if got := requests.Load(); got != 0 {
		t.Fatalf("open circuit made %d background probe requests", got)
	}

	now = circuit.ProbeAt
	if _, err := client.load(context.Background(), "augments", "all", "/augments", "text/html,application/xhtml+xml", false); err != nil {
		t.Fatalf("lazy half-open load failed: %v", err)
	}
	if circuit := client.circuitSnapshot("augments"); circuit.Failures != 0 || !circuit.Until.IsZero() {
		t.Fatalf("successful lazy probe did not close circuit: %#v", circuit)
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("lazy probe requests = %d, want exactly 1", got)
	}
}

func TestMayhemAugmentSlugsNegativeCachesListFailure(t *testing.T) {
	var requests atomic.Int32
	provider := newHexdataBudgetProvider(t, t.TempDir(), championRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests.Add(1)
		return &http.Response{StatusCode: http.StatusInternalServerError, Header: make(http.Header), Body: http.NoBody, Request: request}, nil
	}))

	if slugs := provider.mayhemAugmentSlugs(context.Background()); len(slugs) != 0 {
		t.Fatalf("failed augment list returned slugs: %#v", slugs)
	}
	first := requests.Load()
	if first == 0 {
		t.Fatal("failed augment list made no request")
	}
	if slugs := provider.mayhemAugmentSlugs(context.Background()); len(slugs) != 0 {
		t.Fatalf("negative-cached augment list returned slugs: %#v", slugs)
	}
	if got := requests.Load(); got != first {
		t.Fatalf("negative cache made another list request: %d -> %d", first, got)
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

func TestHexdataMethodologyValuesComeFromAnswerCards(t *testing.T) {
	answer := hexdataAnswerCards{}
	answer.Methodology.MeasurementTechnique = "上游公布的统计口径"
	answer.Methodology.RecommendationPolicy.WilsonZ = 2.58
	answer.Methodology.SamplePolicy.Medium.Min = 321
	answer.Methodology.SamplePolicy.High.Min = 1234
	if got := hexdataMeasurementTechnique(answer); got != "上游公布的统计口径" {
		t.Fatalf("measurement technique = %q", got)
	}
	rows := []hexdataHeroRow{{ID: 1, WinRate: 60, Games: 1200}, {ID: 2, WinRate: 59, Games: 500}}
	applyHexdataLocalTiers(rows, answer.Methodology)
	if rows[0].TierBand != 1 || rows[1].TierBand != 1 {
		t.Fatalf("sample bands ignored answer cards: %#v", rows)
	}
}

func TestHexdataRequestUsesHonestUserAgent(t *testing.T) {
	provider := newChampionProvider()
	var got string
	provider.client = &http.Client{Transport: championRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		got = request.Header.Get("User-Agent")
		return hexdataResponse(request, hexdataTestPage("<p>ok</p>")), nil
	})}
	provider.hexdata.minInterval = 0
	provider.hexdata.maximumJitter = 0
	if _, _, err := provider.hexdata.fetchOnce(context.Background(), "/heroes", "text/html", nil); err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"DeepLegends/", "github.com/LLYY0418/Deep-Legends", "用户触发查询"} {
		if !strings.Contains(got, required) {
			t.Fatalf("User-Agent %q missing %q", got, required)
		}
	}
	for _, forbidden := range []string{"GPTBot", "ClaudeBot", "CCBot"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("User-Agent %q contains bot token %q", got, forbidden)
		}
	}
}

func TestHexdataRequestJitterUsesRandomSource(t *testing.T) {
	provider := newChampionProvider()
	provider.client = &http.Client{Transport: championRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		return hexdataResponse(request, hexdataTestPage("<p>ok</p>")), nil
	})}
	now := time.Date(2026, time.August, 21, 12, 0, 0, 0, time.UTC)
	provider.hexdata.now = func() time.Time { return now }
	provider.hexdata.minInterval = 0
	provider.hexdata.maximumJitter = 800 * time.Millisecond
	provider.hexdata.jitter = func(maximum time.Duration) time.Duration {
		if maximum != 800*time.Millisecond {
			t.Fatalf("jitter maximum = %v", maximum)
		}
		return 317 * time.Millisecond
	}
	var slept time.Duration
	provider.hexdata.sleep = func(_ context.Context, duration time.Duration) error {
		slept = duration
		return nil
	}

	hexdataGlobalPaceMu.Lock()
	previousLastRequest := hexdataGlobalLastRequest
	hexdataGlobalLastRequest = now.Add(time.Hour)
	hexdataGlobalPaceMu.Unlock()
	t.Cleanup(func() {
		hexdataGlobalPaceMu.Lock()
		hexdataGlobalLastRequest = previousLastRequest
		hexdataGlobalPaceMu.Unlock()
	})

	if _, _, err := provider.hexdata.fetchOnce(context.Background(), "/heroes", "text/html", nil); err != nil {
		t.Fatal(err)
	}
	if slept != 317*time.Millisecond {
		t.Fatalf("jitter sleep = %v, want %v", slept, 317*time.Millisecond)
	}
}

// 海斗海克斯的说明只有 Hexdata 详情页有，所以推荐卡上的那几条要就地补齐，
// 而不是把用户推去图鉴。一次列表请求拿全 ID→slug，再按需取详情页。
func TestHydrateMayhemAugmentCopyFillsRecommendationRows(t *testing.T) {
	var list strings.Builder
	list.WriteString(`<table><tbody>`)
	// 斗魂 ID 322 也在榜上：它的说明来自 CommunityDragon，即使能查到 slug
	// 也不该走 Hexdata，否则就是在为已有数据的条目白发请求。
	fmt.Fprintf(&list, `<tr><td><a href="/augment/322-augmented-power">强化之能量</a></td><td>globalHexScore 80.0 · 胜率 55.0%%</td><td>查看详情</td></tr>`)
	for id := 1001; id <= 1160; id++ {
		fmt.Fprintf(&list, `<tr><td><a href="/augment/%d-augment-%d">海克斯%d</a></td><td>globalHexScore 80.0 · 胜率 55.0%%</td><td>查看详情</td></tr>`, id, id, id)
	}
	list.WriteString(`</tbody></table>`)

	augmentPage := func(id int) []byte {
		var body strings.Builder
		body.WriteString(`<table><tbody>`)
		for hero := 1; hero <= 12; hero++ {
			fmt.Fprintf(&body, `<tr><td><a href="/hero/%d-hero%d">英雄%d</a></td><td>90.0</td><td>60.0%%</td><td>1,000</td></tr>`, hero, hero, hero)
		}
		body.WriteString(`</tbody></table>`)
		fmt.Fprintf(&body, `<section data-seo-guide><p>海克斯%d更适合先在英雄1这类高分高样本英雄上考虑。这是 %d 的效果说明。如果你的英雄机制能稳定触发这个海克斯，优先看上方适配英雄。</p></section>`, id, id)
		return hexdataTestPage(body.String())
	}

	var mu sync.Mutex
	hits := map[string]int{}
	// 1099 每次都 500：失败不能被记进缓存，否则一次抖动就让这个海克斯整轮没说明。
	provider := newHexdataBudgetProvider(t, t.TempDir(), gameplayRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		mu.Lock()
		hits[request.URL.Path]++
		mu.Unlock()
		if request.URL.Path == "/augments" {
			return hexdataResponse(request, hexdataTestPage(list.String())), nil
		}
		match := hexdataAugmentPathPattern.FindStringSubmatch(request.URL.Path)
		if len(match) != 3 {
			t.Errorf("unexpected hexdata path: %s", request.URL.Path)
			return nil, errors.New("unexpected path")
		}
		id, _ := strconv.Atoi(match[1])
		if id == 1099 {
			return &http.Response{StatusCode: http.StatusInternalServerError, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("boom")), Request: request}, nil
		}
		return hexdataResponse(request, augmentPage(id)), nil
	}))
	count := func(path string) int {
		mu.Lock()
		defer mu.Unlock()
		return hits[path]
	}

	rows := []championMetricRow{
		{Assets: []championAsset{{ID: 1001, Name: "海克斯1001"}}},
		{Assets: []championAsset{{ID: 1002, Name: "海克斯1002"}}},
		{Assets: []championAsset{{ID: 322, Name: "强化之能量"}}},
		// 同一个 ID 已经带着 CommunityDragon 的说明，不能被上游覆盖。
		{Assets: []championAsset{{ID: 1002, Name: "海克斯1002", Description: "已有说明"}}},
		{Assets: []championAsset{{ID: 1099, Name: "海克斯1099"}}},
	}
	provider.hydrateMayhemAugmentCopy(context.Background(), rows)

	if rows[0].Assets[0].Description != "这是 1001 的效果说明。" || rows[1].Assets[0].Description != "这是 1002 的效果说明。" {
		t.Fatalf("mayhem copy = %q / %q", rows[0].Assets[0].Description, rows[1].Assets[0].Description)
	}
	if rows[2].Assets[0].Description != "" {
		t.Fatalf("arena augment must not be hydrated from hexdata: %q", rows[2].Assets[0].Description)
	}
	if rows[3].Assets[0].Description != "已有说明" {
		t.Fatalf("existing copy was overwritten: %q", rows[3].Assets[0].Description)
	}
	// 斗魂 ID 在榜上有 slug，但它的说明来自 CommunityDragon：一次都不该请求。
	if got := count("/augment/322-augmented-power"); got != 0 {
		t.Fatalf("arena augment page was fetched %d times", got)
	}
	firstRound := count("/augment/1001-augment-1001")
	failedFirst := count("/augment/1099-augment-1099")
	if firstRound == 0 || failedFirst == 0 {
		t.Fatalf("first render fetches = %d / %d", firstRound, failedFirst)
	}

	// 第二轮：命中的走缓存；失败过的必须重试；slug 表不再重新拉取。
	repeat := []championMetricRow{
		{Assets: []championAsset{{ID: 1001, Name: "海克斯1001"}}},
		{Assets: []championAsset{{ID: 1099, Name: "海克斯1099"}}},
		{Assets: []championAsset{{ID: 1003, Name: "海克斯1003"}}},
	}
	provider.hydrateMayhemAugmentCopy(context.Background(), repeat)
	if repeat[0].Assets[0].Description != "这是 1001 的效果说明。" {
		t.Fatalf("cached copy = %q", repeat[0].Assets[0].Description)
	}
	if repeat[2].Assets[0].Description != "这是 1003 的效果说明。" {
		t.Fatalf("newly requested copy = %q", repeat[2].Assets[0].Description)
	}
	if got := count("/augment/1001-augment-1001"); got != firstRound {
		t.Fatalf("resolved copy was re-fetched: %d -> %d", firstRound, got)
	}
	if got := count("/augment/1099-augment-1099"); got <= failedFirst {
		t.Fatalf("failed copy was memoized and never retried: %d -> %d", failedFirst, got)
	}
	if got := count("/augments"); got != 1 {
		t.Fatalf("augment list was fetched %d times, want 1", got)
	}
}

// 补齐必须挂在 decorateHexdataAugments 上，否则详情页拿到的还是空说明。
func TestDecorateHexdataAugmentsHydratesMayhemCopy(t *testing.T) {
	source, err := os.ReadFile("hexdata.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(source)
	start := strings.Index(body, "func (p *championProvider) decorateHexdataAugments(")
	if start < 0 {
		t.Fatal("decorateHexdataAugments not found")
	}
	end := strings.Index(body[start:], "\n}\n")
	if end < 0 {
		t.Fatal("decorateHexdataAugments body is not balanced")
	}
	decorate := body[start : start+end]
	if !strings.Contains(decorate, "p.hydrateMayhemAugmentCopy(ctx, rows)") {
		t.Fatal("decorateHexdataAugments no longer hydrates Mayhem copy")
	}
	// 补齐要排在离线兜底之前，否则真说明永远被占位文案挡住。
	if strings.Index(decorate, "p.hydrateMayhemAugmentCopy(ctx, rows)") > strings.LastIndex(decorate, "augmentDescriptionWithOfflineGuidance") {
		t.Fatal("offline guidance runs before the Hexdata copy is hydrated")
	}
}
