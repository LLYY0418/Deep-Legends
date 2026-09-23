package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// R116-A：Hexdata 数据源由 HTML 抓取替换为 JSON API 的验证夹具。
// testdata/r116/ 下四份文件是 2026-09-20 带 Referer 抓到的真实上游响应（裁剪版），
// 所有正样例都直接吃真实数据，避免「测试自造字段名、上线对不上」的老问题。

const r116aFixtureDir = "testdata/r116"

func r116aFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(r116aFixtureDir, name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// r116aMetaFixture 用真实 meta 响应，只把 buildId 换成测试 build：生产上
// answer-cards 与 meta 的 buildId 同源，测试里必须保持一致，否则 cache key
// 会在两个 build 之间来回跳，测出来的缓存行为是假的。
func r116aMetaFixture(t *testing.T, buildID string) []byte {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal(r116aFixture(t, "hexdata-meta.json"), &payload); err != nil {
		t.Fatal(err)
	}
	payload["buildId"] = buildID
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// r116aAnswerFixture 构造一份通过 validHexdataAnswer 的 answer-cards，
// 它是 measurementTechnique 的唯一来源（meta 里没有这个字段）。
func r116aAnswerFixture(t *testing.T, buildID string) []byte {
	t.Helper()
	answer := hexdataAnswerCards{BuildID: buildID, ReportPatch: "16.18", ReportDate: "2026-09-18"}
	answer.Methodology.MeasurementTechnique = "上游公布的统计口径"
	answer.Methodology.RecommendationPolicy.Ranking = "sample_tier_then_wilson_lower_bound_v1"
	answer.Methodology.RecommendationPolicy.WilsonZ = 1.96
	answer.Methodology.SamplePolicy.Medium.Min = 250
	answer.Methodology.SamplePolicy.High.Min = 1000
	answer.HeroAliases = make([]struct {
		ID             string   `json:"id"`
		URL            string   `json:"url"`
		Name           string   `json:"name"`
		AlternateNames []string `json:"alternateNames"`
	}, 173)
	for index := range answer.HeroAliases {
		answer.HeroAliases[index].ID = strconv.Itoa(index + 1)
		answer.HeroAliases[index].Name = "英雄" + strconv.Itoa(index+1)
	}
	data, err := json.Marshal(answer)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// r116aInsightsFixture 把裁剪版 sample 程序化扩容到指定规模：仓库里不放 373 KB
// 的原文件，条数护栏（heroes 150~200 / augments 190~230）靠扩容后的数据验证。
func r116aInsightsFixture(t *testing.T, heroCount, augmentCount int) []byte {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal(r116aFixture(t, "hexdata-hextech-insights-sample.json"), &payload); err != nil {
		t.Fatal(err)
	}
	expand := func(key string, want int) []any {
		source, ok := payload[key].([]any)
		if !ok || len(source) == 0 {
			t.Fatalf("fixture %s is missing rows", key)
		}
		template, ok := source[0].(map[string]any)
		if !ok {
			t.Fatalf("fixture %s row is not an object", key)
		}
		rows := make([]any, 0, want)
		for index := 0; index < want; index++ {
			row := make(map[string]any, len(template))
			for field, value := range template {
				row[field] = value
			}
			row["id"] = strconv.Itoa(index + 1)
			rows = append(rows, row)
		}
		return rows
	}
	payload["heroes"] = expand("heroes", heroCount)
	payload["augments"] = expand("augments", augmentCount)
	payload["heroCount"] = heroCount
	payload["augmentCount"] = augmentCount
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// r116aRecorder 记录 mock server 收到的请求，请求数判据全靠它。
type r116aRecorder struct {
	mu       sync.Mutex
	hexdata  []string
	foreign  []string
	referers []string
}

func (r *r116aRecorder) add(request *http.Request) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if request.URL.Host == hexdataHost {
		r.hexdata = append(r.hexdata, request.URL.Path)
		r.referers = append(r.referers, request.Header.Get("Referer"))
	} else {
		r.foreign = append(r.foreign, request.URL.Host+request.URL.Path)
	}
}

func (r *r116aRecorder) hexdataPaths() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.hexdata...)
}

func (r *r116aRecorder) count(path string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	total := 0
	for _, entry := range r.hexdata {
		if entry == path {
			total++
		}
	}
	return total
}

func (r *r116aRecorder) countPrefix(prefix string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	total := 0
	for _, entry := range r.hexdata {
		if strings.HasPrefix(entry, prefix) {
			total++
		}
	}
	return total
}

// r116aMock 是 hexdata JSON API 的 mock server：按路径发夹具，非 hexdata 域名
// （OP.GG RSC / Data Dragon / CommunityDragon）一律 404，逼出既有降级路径。
type r116aMock struct {
	bodies   map[string][]byte
	heroJSON []byte
	status   map[string]int
}

func (m *r116aMock) roundTrip(recorder *r116aRecorder) championRoundTripFunc {
	return func(request *http.Request) (*http.Response, error) {
		recorder.add(request)
		if request.URL.Host != hexdataHost {
			return &http.Response{StatusCode: http.StatusNotFound, Header: make(http.Header), Body: http.NoBody, Request: request}, nil
		}
		if status, ok := m.status[request.URL.Path]; ok {
			return &http.Response{StatusCode: status, Header: make(http.Header), Body: http.NoBody, Request: request}, nil
		}
		body, ok := m.bodies[request.URL.Path]
		if !ok && hexdataHeroJSONPathPattern.MatchString(request.URL.Path) {
			body, ok = m.heroJSON, len(m.heroJSON) > 0
		}
		if !ok {
			return nil, fmt.Errorf("unexpected hexdata path %s", request.URL.Path)
		}
		return hexdataResponse(request, body), nil
	}
}

// ---------------------------------------------------------------------------
// P0-1：Referer
// ---------------------------------------------------------------------------

// 上游对 /api/hexdata/* 校验 Referer：不带就返 403 data_not_public。
// 这条是「有 Referer 通」的正样例。
func TestHexdataRequestCarriesRefererAndUpstreamAccepts(t *testing.T) {
	recorder := &r116aRecorder{}
	mock := &r116aMock{bodies: map[string][]byte{hexdataMetaPath: r116aMetaFixture(t, hexdataTestBuild)}}
	provider := newHexdataBudgetProvider(t, t.TempDir(), championRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		// mock server 复刻上游行为：缺 Referer 直接 403。
		if request.Header.Get("Referer") != "https://"+hexdataHost+"/" {
			return &http.Response{StatusCode: http.StatusForbidden, Header: make(http.Header), Body: http.NoBody, Request: request}, nil
		}
		return mock.roundTrip(recorder)(request)
	}))
	if _, err := provider.loadHexdataMeta(context.Background()); err != nil {
		t.Fatalf("meta load with Referer failed: %v", err)
	}
	if got := recorder.count(hexdataMetaPath); got != 1 {
		t.Fatalf("meta requests = %d, want 1", got)
	}
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	for _, referer := range recorder.referers {
		if referer != "https://"+hexdataHost+"/" {
			t.Fatalf("outbound Referer = %q", referer)
		}
	}
}

// 「无 Referer 拒」的负样例：用一层 transport 把 Referer 摘掉，模拟「忘了加这个头」
// 的变异，确认上游 403 会让 load() 直接失败（而不是静默返回空数据）。
func TestHexdataLoadFailsWhenRefererIsMissing(t *testing.T) {
	recorder := &r116aRecorder{}
	mock := &r116aMock{bodies: map[string][]byte{hexdataMetaPath: r116aMetaFixture(t, hexdataTestBuild)}}
	upstream := championRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Header.Get("Referer") == "" {
			return &http.Response{StatusCode: http.StatusForbidden, Header: make(http.Header), Body: http.NoBody, Request: request}, nil
		}
		return mock.roundTrip(recorder)(request)
	})
	provider := newHexdataBudgetProvider(t, t.TempDir(), championRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		// 变异点：出站前删掉 Referer。
		request.Header.Del("Referer")
		return upstream.RoundTrip(request)
	}))
	if _, err := provider.loadHexdataMeta(context.Background()); err == nil {
		t.Fatal("missing Referer was accepted by upstream")
	} else if status := championUpstreamHTTPStatus(err); status != http.StatusForbidden {
		t.Fatalf("missing Referer error = %v (status %d), want HTTP 403", err, status)
	}
}

// ---------------------------------------------------------------------------
// P0-2：路径白名单
// ---------------------------------------------------------------------------

func TestHexdataAllowedPathCoversJSONAPIAndRejectsHeroList(t *testing.T) {
	client := newHexdataClient(newChampionProvider(), nil)
	for _, test := range []struct {
		path string
		want bool
	}{
		{"/api/hexdata/heroes/157", true},
		{"/api/hexdata/postmatch", true},
		{"/api/hexdata/hextech-insights", true},
		{"/api/hexdata/meta", true},
		// Anti-scope 第 1 条：无 ID 的列表接口上游已 403 拦截并明文警告，绝不放行。
		{"/api/hexdata/heroes", false},
		{"/api/hexdata/heroes/", false},
		{"/api/hexdata/heroes/abc", false},
		{"/api/hexdata/heroes/157/trios", false},
		{"/api/hexdata/heroes/-1", false},
		{"/api/hexdata/augments", false},
		{"/api/hexdata/augments/1058", false},
		// 既有链路保持不变。
		{hexdataAnswerPath, true},
		{"/heroes", true},
		{"/augments", true},
		{"/augment-rarity", true},
		{"/hero/157-yasuo", true},
		{"/augment/1058-mystic-punch", true},
	} {
		if got := client.allowedPath(test.path); got != test.want {
			t.Fatalf("allowedPath(%q) = %v, want %v", test.path, got, test.want)
		}
	}
}

// ---------------------------------------------------------------------------
// P0-4：inspectHexdataPayload 三个新 case 的真实结构校验
// ---------------------------------------------------------------------------

func TestInspectHexdataPayloadValidatesMetaSnapshot(t *testing.T) {
	shape, err := inspectHexdataPayload("meta", hexdataMetaPath, r116aFixture(t, "hexdata-meta.json"))
	if err != nil {
		t.Fatalf("real meta fixture rejected: %v", err)
	}
	if shape.BuildID != "hexdata-2026-09-18-167d464cf859" || shape.Rows != 173 || shape.Fields != 211 {
		t.Fatalf("meta shape = %#v", shape)
	}
	for _, test := range []struct {
		name    string
		payload string
	}{
		{"缺 buildId", `{"reportPatch":"16.18","reportDate":"2026-09-18","heroCount":173,"augmentCount":211}`},
		{"buildId 前缀不对", `{"buildId":"2026-09-18","reportPatch":"16.18","reportDate":"2026-09-18","heroCount":173,"augmentCount":211}`},
		{"缺 reportPatch", `{"buildId":"hexdata-x","reportDate":"2026-09-18","heroCount":173,"augmentCount":211}`},
		{"reportDate 不是 ISO 日期", `{"buildId":"hexdata-x","reportPatch":"16.18","reportDate":"09/18/2026","heroCount":173,"augmentCount":211}`},
		{"heroCount 掉出护栏", `{"buildId":"hexdata-x","reportPatch":"16.18","reportDate":"2026-09-18","heroCount":12,"augmentCount":211}`},
		{"augmentCount 掉出护栏", `{"buildId":"hexdata-x","reportPatch":"16.18","reportDate":"2026-09-18","heroCount":173,"augmentCount":12}`},
		{"不是 JSON", `<html><body>not json</body></html>`},
	} {
		if _, err := inspectHexdataPayload("meta", hexdataMetaPath, []byte(test.payload)); err == nil {
			t.Fatalf("%s was accepted", test.name)
		}
	}
}

// 对抗变异（P0 第 1 条）：删掉 augments 空值检查就应该让空 augments 通过。
// 这里同时断言「空 augments 报错」和「补一条就通过」，证明护栏不是同义反复。
func TestInspectHexdataPayloadRejectsHeroJSONWithoutAugments(t *testing.T) {
	items := `{"itemId":"3153","itemName":"破败王者之刃","winRate":0.6,"pickRate":0.18,"games":445607,"tier":1,"hexTier":"hang","hexLabel":"夯","hexScore":90.7}`
	trios := `{"trioKey":"1058:1077:1336","augmentIds":["1058","1077","1336"],"winRate":0.65,"pickRate":0.004,"games":10453}`
	augment := `{"augmentId":"2095","augmentName":"掷骰狂人","augmentDescription":"附近敌人在阵亡时有几率掉落【属性锻造器】。","rarity":"棱彩","tier":1,"score":28.07,"hexScore":95.8,"pairWinRate":0.679,"games":91113}`
	empty := []byte(`{"items":[` + items + `],"augments":[],"trios":[` + trios + `]}`)
	shape, err := inspectHexdataPayload("hero-json", "/api/hexdata/heroes/157", empty)
	if err == nil {
		t.Fatal("hero-json with empty augments was accepted")
	}
	if !strings.Contains(err.Error(), "augments=0") {
		t.Fatalf("error does not name the empty array: %v", err)
	}
	if shape.Rows != 1 {
		t.Fatalf("shape.Rows = %d, want len(items)=1", shape.Rows)
	}
	filled := []byte(`{"items":[` + items + `],"augments":[` + augment + `],"trios":[` + trios + `]}`)
	if _, err := inspectHexdataPayload("hero-json", "/api/hexdata/heroes/157", filled); err != nil {
		t.Fatalf("hero-json with one augment rejected: %v", err)
	}
	for _, test := range []struct {
		name    string
		payload string
		want    string
	}{
		{"items 为空", `{"items":[],"augments":[` + augment + `],"trios":[` + trios + `]}`, "items=0"},
		{"trios 为空", `{"items":[` + items + `],"augments":[` + augment + `],"trios":[]}`, "trios=0"},
		{"三个数组都缺", `{}`, "items=0"},
		{"不是 JSON", `{"items":`, "unexpected end of JSON input"},
	} {
		_, err := inspectHexdataPayload("hero-json", "/api/hexdata/heroes/157", []byte(test.payload))
		if err == nil {
			t.Fatalf("%s was accepted", test.name)
		}
		if !strings.Contains(err.Error(), test.want) {
			t.Fatalf("%s error = %v, want it to contain %q", test.name, err, test.want)
		}
	}
}

func TestInspectHexdataPayloadAcceptsRealHeroJSONFixture(t *testing.T) {
	shape, err := inspectHexdataPayload("hero-json", "/api/hexdata/heroes/157", r116aFixture(t, "hexdata-hero-157.json"))
	if err != nil {
		t.Fatalf("real hero-json fixture rejected: %v", err)
	}
	if shape.Rows != 20 || shape.Fields != 10 {
		t.Fatalf("hero-json shape = %#v, want rows=20 fields=10", shape)
	}
	// JSON 响应体不带 buildId，shape.BuildID 必须保持空值而不是编一个出来。
	if shape.BuildID != "" {
		t.Fatalf("hero-json shape invented a buildId: %q", shape.BuildID)
	}
}

func TestInspectHexdataPayloadValidatesPostmatch(t *testing.T) {
	fixture := r116aFixture(t, "hexdata-postmatch.json")
	shape, err := inspectHexdataPayload("postmatch", hexdataPostmatchPath, fixture)
	if err != nil {
		t.Fatalf("real postmatch fixture rejected: %v", err)
	}
	if shape.Rows != 173 {
		t.Fatalf("postmatch shape = %#v, want rows=173", shape)
	}
	// 条目数掉出护栏。
	if _, err := inspectHexdataPayload("postmatch", hexdataPostmatchPath, []byte(`{"157":{"kda":2.5,"avgDamage":41778}}`)); err == nil {
		t.Fatal("single-hero postmatch was accepted")
	}
	// 任一条缺 kda / avgDamage 都要被发现（逐条校验，不靠抽查）。
	without := func(field string) []byte {
		var rows map[string]map[string]json.RawMessage
		if err := json.Unmarshal(fixture, &rows); err != nil {
			t.Fatal(err)
		}
		delete(rows["157"], field)
		broken, err := json.Marshal(rows)
		if err != nil {
			t.Fatal(err)
		}
		return broken
	}
	if _, err := inspectHexdataPayload("postmatch", hexdataPostmatchPath, without("kda")); err == nil || !strings.Contains(err.Error(), "kda") {
		t.Fatalf("postmatch without kda error = %v", err)
	}
	if _, err := inspectHexdataPayload("postmatch", hexdataPostmatchPath, without("avgDamage")); err == nil || !strings.Contains(err.Error(), "avgDamage") {
		t.Fatalf("postmatch without avgDamage error = %v", err)
	}
	if _, err := inspectHexdataPayload("postmatch", hexdataPostmatchPath, []byte(`[]`)); err == nil {
		t.Fatal("array payload was accepted as a postmatch map")
	}
}

func TestInspectHexdataPayloadValidatesHextechInsights(t *testing.T) {
	if _, err := inspectHexdataPayload("hextech-insights", hexdataHextechInsightsPath, r116aInsightsFixture(t, 173, 211)); err != nil {
		t.Fatalf("expanded insights fixture rejected: %v", err)
	}
	// 裁剪版 sample 只有 8/8 条，必须被护栏挡住。
	if _, err := inspectHexdataPayload("hextech-insights", hexdataHextechInsightsPath, r116aFixture(t, "hexdata-hextech-insights-sample.json")); err == nil {
		t.Fatal("8-hero insights sample was accepted")
	}
	if _, err := inspectHexdataPayload("hextech-insights", hexdataHextechInsightsPath, r116aInsightsFixture(t, 173, 100)); err == nil {
		t.Fatal("100-augment insights was accepted")
	}
	if _, err := inspectHexdataPayload("hextech-insights", hexdataHextechInsightsPath, r116aInsightsFixture(t, 201, 211)); err == nil {
		t.Fatal("201-hero insights was accepted")
	}
	if _, err := inspectHexdataPayload("hextech-insights", hexdataHextechInsightsPath, []byte(`{"heroes":[]}`)); err == nil {
		t.Fatal("empty insights was accepted")
	}
}

// ---------------------------------------------------------------------------
// P0-3：citation 一律来自 meta 快照，且指向人类可读的 HTML 页面
// ---------------------------------------------------------------------------

func TestHexdataJSONCitationComesFromMetaSnapshotAndPointsAtHTMLPage(t *testing.T) {
	snapshot, err := parseHexdataMeta(r116aFixture(t, "hexdata-meta.json"))
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.BuildID != "hexdata-2026-09-18-167d464cf859" || snapshot.ReportPatch != "16.18" || snapshot.ReportDate != "2026-09-18" || snapshot.HeroCount != 173 || snapshot.AugmentCount != 211 {
		t.Fatalf("meta snapshot = %#v", snapshot)
	}
	for _, canonical := range []string{"/hero/157-yasuo", hexdataAggregateCanonical} {
		citation := hexdataJSONCitation(snapshot, canonical)
		if !hexdataCitationComplete(citation) {
			t.Fatalf("citation for %q is incomplete: %#v", canonical, citation)
		}
		if citation.CanonicalURL != "https://"+hexdataHost+canonical {
			t.Fatalf("canonical url = %q", citation.CanonicalURL)
		}
		if strings.Contains(citation.CanonicalURL, "/api/") {
			t.Fatalf("citation links to a JSON endpoint: %q", citation.CanonicalURL)
		}
		if citation.Source != "Hexdata" || citation.Patch != "16.18" || citation.ReportDate != "2026-09-18" {
			t.Fatalf("citation = %#v", citation)
		}
	}
	if strings.HasPrefix(hexdataAggregateCanonical, "/api/") {
		t.Fatalf("aggregate canonical must be an HTML page: %q", hexdataAggregateCanonical)
	}
}

// ---------------------------------------------------------------------------
// P1：parseHexdataHeroJSON
// ---------------------------------------------------------------------------

func TestParseHexdataHeroJSONMatchesRealUpstreamFixture(t *testing.T) {
	detail, err := parseHexdataHeroJSON(r116aFixture(t, "hexdata-hero-157.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Items) != 20 || len(detail.Augments) != 10 || len(detail.Trios) != 5 ||
		len(detail.WeakAgainst) != 7 || len(detail.StrongAgainst) != 7 || len(detail.TeammateSynergies) != 7 ||
		len(detail.TerminalItemTrios) != 10 || len(detail.SummonerSpellPairs) != 6 {
		t.Fatalf("detail counts = items %d augments %d trios %d weak %d strong %d synergy %d terminal %d spells %d",
			len(detail.Items), len(detail.Augments), len(detail.Trios), len(detail.WeakAgainst), len(detail.StrongAgainst),
			len(detail.TeammateSynergies), len(detail.TerminalItemTrios), len(detail.SummonerSpellPairs))
	}
	item := detail.Items[0]
	if item.ItemID != 123430 || item.ItemName != "毁坏仪式" || item.Games != 1406649 || item.Tier != 1 ||
		item.HexTier != "hang" || item.HexLabel != "夯" || item.HexScore != 90.7 || item.WithoutItemGames != 965920 {
		t.Fatalf("item[0] = %#v", item)
	}
	if math.Abs(item.WinRate-0.616255) > 1e-9 || math.Abs(item.CoreDelta-0.119732) > 1e-9 || math.Abs(item.AverageIndex-1.483893) > 1e-9 {
		t.Fatalf("item[0] metrics = %#v", item)
	}
	augment := detail.Augments[0]
	if augment.AugmentID != 2095 || augment.AugmentName != "掷骰狂人" || augment.Rarity != "棱彩" || augment.Tier != 1 ||
		augment.HexLabel != "夯" || augment.Games != 91113 || augment.AugmentDescription != "附近敌人在阵亡时有几率掉落【属性锻造器】。" {
		t.Fatalf("augment[0] = %#v", augment)
	}
	if len(augment.Stages) != 4 {
		t.Fatalf("augment[0] stages = %d, want 4", len(augment.Stages))
	}
	for index, stage := range augment.Stages {
		if stage.Stage != index+1 || stage.Games <= 0 {
			t.Fatalf("augment stage %d = %#v", index, stage)
		}
	}
	if detail.Augments[0].Stages[0].StageBaselineWinRate <= 0 {
		t.Fatalf("stage baseline win rate missing: %#v", detail.Augments[0].Stages[0])
	}
	// 上游把 ID 写成字符串，解析层必须转成 int。
	if detail.WeakAgainst[0].OpponentChampionID != 200 || detail.WeakAgainst[0].Evidence != "supported" || detail.WeakAgainst[0].QValue != 0 {
		t.Fatalf("weakAgainst[0] = %#v", detail.WeakAgainst[0])
	}
	if detail.StrongAgainst[0].OpponentChampionID != 29 {
		t.Fatalf("strongAgainst[0] = %#v", detail.StrongAgainst[0])
	}
	if detail.TeammateSynergies[0].TeammateChampionID != 4 || detail.TeammateSynergies[0].SynergyDelta <= 0 {
		t.Fatalf("teammateSynergies[0] = %#v", detail.TeammateSynergies[0])
	}
	if len(detail.Trios[0].AugmentIDs) != 3 || detail.Trios[0].AugmentIDs[0] != 1058 || len(detail.Trios[0].AugmentNames) != 3 {
		t.Fatalf("trios[0] = %#v", detail.Trios[0])
	}
	if len(detail.TerminalItemTrios[0].ItemIDs) != 3 || detail.TerminalItemTrios[0].ItemIDs[0] != 3153 {
		t.Fatalf("terminalItemTrios[0] = %#v", detail.TerminalItemTrios[0])
	}
	if len(detail.SummonerSpellPairs[0].SpellIDs) != 2 || detail.SummonerSpellPairs[0].SpellIDs[0] != 4 ||
		detail.SummonerSpellPairs[0].SpellIDs[1] != 32 || detail.SummonerSpellPairs[0].SpellKey != "4:32" {
		t.Fatalf("summonerSpellPairs[0] = %#v", detail.SummonerSpellPairs[0])
	}
	// items[] 没有 heroWinRate，英雄胜率取自 augments[]/trios[] 的官方字段。
	if math.Abs(detail.HeroWinRate-56.7509) > 1e-6 {
		t.Fatalf("hero win rate = %v, want 56.7509", detail.HeroWinRate)
	}
}

// 项目红线：ID 转换失败必须整条丢弃，绝不用 0 顶替（0 会指向另一个真实实体）。
func TestParseHexdataHeroJSONDropsRowsWithUnparseableIDs(t *testing.T) {
	payload := `{
		"items":[{"itemId":"3153","itemName":"破败王者之刃","games":445607},{"itemId":"NaN","itemName":"脏数据","games":1},{"itemId":"0","itemName":"零","games":1},{"itemId":"","itemName":"空","games":1},{"itemId":" 6333 ","itemName":"带空格","games":2}],
		"augments":[{"augmentId":"2095","augmentDescription":"说明","games":91113},{"augmentId":"不是数字","games":1}],
		"trios":[{"trioKey":"1058:1077:1336","augmentIds":["1058","1077","1336"],"games":10453},{"trioKey":"坏","augmentIds":["1058","oops","1336"],"games":9}],
		"weakAgainst":[{"opponentChampionId":"200","evidence":"supported","games":38696},{"opponentChampionId":"abc","games":1},{"opponentChampionId":"0","games":1}],
		"strongAgainst":[{"opponentChampionId":"29","games":102230}],
		"teammateSynergies":[{"teammateChampionId":"4","games":96625},{"teammateChampionId":null,"games":1}],
		"terminalItemTrios":[{"trioKey":"3153:6333:123430","itemIds":["3153","6333","123430"],"games":445607},{"trioKey":"坏","itemIds":["3153","","123430"],"games":9}],
		"summonerSpellPairs":[{"spellIds":["4","32"],"spellKey":"4:32","games":2153718},{"spellIds":["4","不是数字"],"games":1}]
	}`
	detail, stats, err := parseHexdataHeroJSONWithStats([]byte(payload))
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Items) != 2 || detail.Items[0].ItemID != 3153 || detail.Items[1].ItemID != 6333 {
		t.Fatalf("items = %#v", detail.Items)
	}
	if stats.DroppedItems != 3 {
		t.Fatalf("dropped items = %d, want 3", stats.DroppedItems)
	}
	if len(detail.Augments) != 1 || stats.DroppedAugments != 1 {
		t.Fatalf("augments = %#v dropped=%d", detail.Augments, stats.DroppedAugments)
	}
	if len(detail.Trios) != 1 || stats.DroppedTrios != 1 {
		t.Fatalf("trios = %#v dropped=%d", detail.Trios, stats.DroppedTrios)
	}
	if len(detail.WeakAgainst) != 1 || detail.WeakAgainst[0].OpponentChampionID != 200 || stats.DroppedMatchups != 2 {
		t.Fatalf("weakAgainst = %#v dropped=%d", detail.WeakAgainst, stats.DroppedMatchups)
	}
	if len(detail.StrongAgainst) != 1 || detail.StrongAgainst[0].OpponentChampionID != 29 {
		t.Fatalf("strongAgainst = %#v", detail.StrongAgainst)
	}
	if len(detail.TeammateSynergies) != 1 || detail.TeammateSynergies[0].TeammateChampionID != 4 || stats.DroppedSynergies != 1 {
		t.Fatalf("teammateSynergies = %#v dropped=%d", detail.TeammateSynergies, stats.DroppedSynergies)
	}
	if len(detail.TerminalItemTrios) != 1 || stats.DroppedTerminalItemTrios != 1 {
		t.Fatalf("terminalItemTrios = %#v dropped=%d", detail.TerminalItemTrios, stats.DroppedTerminalItemTrios)
	}
	if len(detail.SummonerSpellPairs) != 1 || stats.DroppedSpellPairs != 1 {
		t.Fatalf("summonerSpellPairs = %#v dropped=%d", detail.SummonerSpellPairs, stats.DroppedSpellPairs)
	}
	// 任何被保留下来的行都不允许出现 0 值 ID。
	for _, row := range detail.Items {
		if row.ItemID <= 0 {
			t.Fatalf("zero item id survived: %#v", row)
		}
	}
	if stats.dropped() != 10 {
		t.Fatalf("total dropped = %d, want 10", stats.dropped())
	}
	event := map[string]any{}
	stats.addToDiagnostic(event)
	if event["dropped_items"] != 3 || event["dropped_spell_pairs"] != 1 {
		t.Fatalf("diagnostic = %#v", event)
	}
}

// trios / terminalItemTrios 上游各 1000 条（meta.heroTrioLimitPerHero 是硬上限），
// 解析层按 games 降序裁剪到前 50 条。
func TestParseHexdataHeroJSONTrimsTriosToTopFiftyByGames(t *testing.T) {
	build := func(idPrefix string, count int) string {
		var rows []string
		for index := 1; index <= count; index++ {
			rows = append(rows, fmt.Sprintf(`{"trioKey":"%s:%d","augmentIds":["1058","1077","1336"],"itemIds":["3153","6333","123430"],"winRate":0.6,"games":%d}`, idPrefix, index, index))
		}
		return strings.Join(rows, ",")
	}
	payload := []byte(`{"items":[{"itemId":"3153","games":1000}],"augments":[{"augmentId":"2095","games":1000}],"trios":[` + build("trio", 1000) + `],"terminalItemTrios":[` + build("terminal", 1000) + `]}`)
	detail, stats, err := parseHexdataHeroJSONWithStats(payload)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Trios) != hexdataTrioKeepLimit || len(detail.TerminalItemTrios) != hexdataTrioKeepLimit {
		t.Fatalf("trios = %d terminal = %d, want %d each", len(detail.Trios), len(detail.TerminalItemTrios), hexdataTrioKeepLimit)
	}
	if stats.TrimmedTrios != 950 || stats.TrimmedTerminalItemTrios != 950 {
		t.Fatalf("trimmed = %d/%d, want 950/950", stats.TrimmedTrios, stats.TrimmedTerminalItemTrios)
	}
	if detail.Trios[0].Games != 1000 || detail.Trios[hexdataTrioKeepLimit-1].Games != 951 {
		t.Fatalf("trios were not sorted by games desc: first=%d last=%d", detail.Trios[0].Games, detail.Trios[hexdataTrioKeepLimit-1].Games)
	}
	for index := 1; index < len(detail.Trios); index++ {
		if detail.Trios[index-1].Games < detail.Trios[index].Games {
			t.Fatalf("trios out of order at %d", index)
		}
	}
	// 同一份 payload 解析两次必须裁出同一批（排序键有 TrioKey 兜底）。
	again, _, err := parseHexdataHeroJSONWithStats(payload)
	if err != nil {
		t.Fatal(err)
	}
	for index := range detail.Trios {
		if detail.Trios[index].TrioKey != again.Trios[index].TrioKey {
			t.Fatalf("trim is not deterministic at %d", index)
		}
	}
	// 少于 50 条时不裁。
	small, smallStats, err := parseHexdataHeroJSONWithStats([]byte(`{"items":[{"itemId":"3153","games":1}],"augments":[{"augmentId":"2095","games":1}],"trios":[` + build("trio", 5) + `]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(small.Trios) != 5 || smallStats.TrimmedTrios != 0 {
		t.Fatalf("small trios = %d trimmed=%d", len(small.Trios), smallStats.TrimmedTrios)
	}
}

// ---------------------------------------------------------------------------
// P1-3 + 判据 1/2/3：loadMayhemDetail 走 JSON API，per-augment 扇出消失
// ---------------------------------------------------------------------------

func r116aMayhemProvider(t *testing.T, recorder *r116aRecorder, heroJSON []byte) (*championProvider, string) {
	t.Helper()
	root := t.TempDir()
	mock := &r116aMock{
		bodies: map[string][]byte{
			hexdataMetaPath:            r116aMetaFixture(t, hexdataTestBuild),
			hexdataAnswerPath:          r116aAnswerFixture(t, hexdataTestBuild),
			hexdataPostmatchPath:       r116aFixture(t, "hexdata-postmatch.json"),
			hexdataHextechInsightsPath: r116aInsightsFixture(t, 173, 211),
		},
		heroJSON: heroJSON,
	}
	provider := newHexdataBudgetProvider(t, root, mock.roundTrip(recorder))
	provider.championMeta[157] = championMetadata{ID: 157, Key: "Yasuo", Slug: "yasuo", NameZH: "疾风剑豪"}
	return provider, root
}

func TestLoadMayhemDetailRequestsHeroJSONAndMetaOnly(t *testing.T) {
	recorder := &r116aRecorder{}
	provider, _ := r116aMayhemProvider(t, recorder, r116aFixture(t, "hexdata-hero-157.json"))
	response, err := provider.loadMayhemDetail(context.Background(), "157")
	if err != nil {
		t.Fatalf("loadMayhemDetail failed: %v", err)
	}
	paths := recorder.hexdataPaths()
	// 工单判据：数据请求是 meta + hero-json 两条，per-augment 描述扇出彻底消失。
	if got := recorder.countPrefix("/augment/"); got != 0 {
		t.Fatalf("augment HTML pages were still fetched %d times: %v", got, paths)
	}
	if got := recorder.countPrefix("/hero/"); got != 0 {
		t.Fatalf("hero HTML page was still fetched %d times: %v", got, paths)
	}
	if got := recorder.count(hexdataMetaPath); got != 1 {
		t.Fatalf("meta requests = %d, want 1 (%v)", got, paths)
	}
	if got := recorder.count("/api/hexdata/heroes/157"); got != 1 {
		t.Fatalf("hero-json requests = %d, want 1 (%v)", got, paths)
	}
	dataRequests := 0
	for _, path := range paths {
		if path == hexdataMetaPath || path == "/api/hexdata/heroes/157" {
			dataRequests++
		}
	}
	if dataRequests != 2 {
		t.Fatalf("hexdata data requests = %d, want 2 (%v)", dataRequests, paths)
	}
	// answer-cards 是既有的站点元数据请求（measurementTechnique 的唯一来源，
	// 12h 软 TTL、与榜单页共用），工单没有要求移除；缓存命中后详情页就是 2 条。
	if got := recorder.count(hexdataAnswerPath); got != 1 {
		t.Fatalf("answer-cards requests = %d, want 1 (%v)", got, paths)
	}
	if !strings.Contains(response.MeasurementTechnique, "上游公布的统计口径") {
		t.Fatalf("measurement technique = %q", response.MeasurementTechnique)
	}
	// citation 来自 meta 快照，CanonicalURL 指向 HTML 详情页而不是 JSON 端点。
	if response.Citation == nil || response.Citation.BuildID != hexdataTestBuild || response.Citation.Patch != "16.18" ||
		response.Citation.ReportDate != "2026-09-18" || response.Citation.CanonicalURL != "https://"+hexdataHost+"/hero/157-yasuo" {
		t.Fatalf("citation = %#v", response.Citation)
	}
	if math.Abs(response.Stats.WinRate-56.7509) > 1e-6 {
		t.Fatalf("hero win rate = %v, want 56.7509", response.Stats.WinRate)
	}
	// R116-B P0-5-2：英雄级梯度改由 hextech-insights.heroes[].tier 提供（官方 T1-T5）。
	// hero-json 顶层依然没有英雄级 tier，items[].tier / augments[].tier 都是行级档位，
	// 所以这里的 2 只可能来自 r116aInsightsFixture 扩容出的官方档位。
	// 「insights 不可用时不许用行级档位冒充」这条红线由
	// TestLoadMayhemDetailHeroTierStaysNilWithoutInsights 单独钉住。
	if response.Stats.Tier == nil || *response.Stats.Tier != 2 {
		t.Fatalf("hero tier = %v, want official tier 2 from hextech-insights", response.Stats.Tier)
	}
	// 判据 2：Description 非空比例 ≥ 95%。
	filled := 0
	for _, row := range response.RecommendedAugments {
		if len(row.Assets) > 0 && strings.TrimSpace(row.Assets[0].Description) != "" {
			filled++
		}
	}
	if len(response.RecommendedAugments) == 0 {
		t.Fatal("no recommended augments")
	}
	if ratio := float64(filled) / float64(len(response.RecommendedAugments)); ratio < 0.95 {
		t.Fatalf("description coverage = %d/%d (%.2f), want >= 0.95", filled, len(response.RecommendedAugments), ratio)
	}
	// 判据 3：装备资产 ID 直接等于 JSON 的 itemId。
	var fixture struct {
		Items []struct {
			ItemID   string `json:"itemId"`
			ItemName string `json:"itemName"`
			Tier     int    `json:"tier"`
			HexTier  string `json:"hexTier"`
			HexLabel string `json:"hexLabel"`
		} `json:"items"`
	}
	if err := json.Unmarshal(r116aFixture(t, "hexdata-hero-157.json"), &fixture); err != nil {
		t.Fatal(err)
	}
	if len(response.ItemRanking) != len(fixture.Items) {
		t.Fatalf("item ranking = %d rows, want %d", len(response.ItemRanking), len(fixture.Items))
	}
	for index, row := range response.ItemRanking {
		want, _ := strconv.Atoi(fixture.Items[index].ItemID)
		if len(row.Assets) != 1 || row.Assets[0].ID != want || row.Assets[0].Kind != "item" {
			t.Fatalf("item row %d = %#v, want id %d", index, row.Assets, want)
		}
		if row.Assets[0].Name != fixture.Items[index].ItemName {
			t.Fatalf("item row %d name = %q, want %q", index, row.Assets[0].Name, fixture.Items[index].ItemName)
		}
		// 官方档位字段原样搬运；上游对部分装备（如草鞋）不给 hexTier/hexLabel，
		// 解析层保持空值，不许自己编一个档位出来。
		if row.OfficialTier != fixture.Items[index].Tier || row.HexTier != fixture.Items[index].HexTier || row.HexLabel != fixture.Items[index].HexLabel {
			t.Fatalf("official tier fields were not preserved: %#v (fixture %#v)", row, fixture.Items[index])
		}
	}
	// 官方字段与稀有度映射。
	if response.RecommendedAugments[0].Assets[0].ID != 2095 || response.RecommendedAugments[0].Rarity != "prismatic" ||
		response.RecommendedAugments[0].HexLabel != "夯" || response.RecommendedAugments[0].HexTier != "hang" {
		t.Fatalf("augment row = %#v", response.RecommendedAugments[0])
	}
	if math.Abs(response.RecommendedAugments[0].WinRate-67.9036) > 1e-6 {
		t.Fatalf("augment win rate = %v, want 67.9036 (pairWinRate 换算成百分数)", response.RecommendedAugments[0].WinRate)
	}
	if response.RecommendedAugments[0].DeltaWinRate <= 0 || response.RecommendedAugments[0].WilsonLowerWinRate <= 0 {
		t.Fatalf("delta/wilson were not preserved: %#v", response.RecommendedAugments[0])
	}
	// 落盘走 hexdata- 前缀通道，key 用 meta 快照的 buildID。
	if entry, err := provider.cache.readDisk(hexdataTestBuild + "|hero-json|157"); err != nil || len(entry.Data) == 0 {
		t.Fatalf("hero-json was not promoted to disk: %v", err)
	}
	if entry, err := provider.cache.readDisk(hexdataTestBuild + "|meta|site"); err != nil || len(entry.Data) == 0 {
		t.Fatalf("meta was not promoted to disk: %v", err)
	}
}

// 判据 1 的字面版本（R116-B 之后的预算变更记账：2 → 4 条）。
// answer-cards 已在缓存里（榜单页先打开过）时，打开一个从未访问过的英雄详情页：
//
//	R116-A 交付时 = 2 条：/api/hexdata/meta + /api/hexdata/heroes/157
//	R116-B 之后   = 4 条：再加 /api/hexdata/hextech-insights（P0-5-2 官方英雄档位）
//	                       与 /api/hexdata/postmatch（P1-6 的 22 项表现指标）
//
// 两条新增都是「每个 buildID 只取一次」的目录型全量文件（373 KB / 89 KB），都会
// promote 落盘并长期缓存，所以只有该 buildID 下的第一次详情页付这个代价；此后同一
// 英雄再打开必须是 0 条。这正是评审第 4.2/4.3 节要求的形态（「/postmatch +
// /hextech-insights 两个全量文件，0.46 MB / 2 请求」），而不是对 10 个英雄各拉一次
// hero-json（最坏排队 11 秒，且 recordLoadFailure 对取消直接 return 会让熔断永远不触发）。
func TestLoadMayhemDetailColdHeroCostsFourHexdataRequestsWithWarmSiteMetadata(t *testing.T) {
	recorder := &r116aRecorder{}
	provider, _ := r116aMayhemProvider(t, recorder, r116aFixture(t, "hexdata-hero-157.json"))
	if _, _, err := provider.loadHexdataAnswer(context.Background()); err != nil {
		t.Fatal(err)
	}
	*recorder = r116aRecorder{}
	if _, err := provider.loadMayhemDetail(context.Background(), "157"); err != nil {
		t.Fatal(err)
	}
	paths := recorder.hexdataPaths()
	want := []string{hexdataMetaPath, "/api/hexdata/heroes/157", hexdataHextechInsightsPath, hexdataPostmatchPath}
	if len(paths) != len(want) {
		t.Fatalf("hexdata requests = %v, want %v", paths, want)
	}
	for index := range want {
		if paths[index] != want[index] {
			t.Fatalf("hexdata requests = %v, want %v", paths, want)
		}
	}
	// 四个响应都必须落盘，否则同一英雄每次打开都要重新付这 4 条请求。
	*recorder = r116aRecorder{}
	if _, err := provider.loadMayhemDetail(context.Background(), "157"); err != nil {
		t.Fatal(err)
	}
	if again := recorder.hexdataPaths(); len(again) != 0 {
		t.Fatalf("warm hero detail made %d additional hexdata requests %v, want 0", len(again), again)
	}
}

// R116-A 立下的红线在 R116-B 之后仍然成立：hextech-insights 不可用（熔断 / 降级 /
// 上游改字段）时，英雄级梯度必须留空，绝不允许拿 hero-json 的行级 items[].tier 或
// augments[].tier 冒充——那会把「这件装备在该英雄下的档位」渲染成「这个英雄的梯度」，
// 属于伪造数据。前端拿到 nil 时整块隐藏梯度徽章（评审 6.1「取不到就整块隐藏」）。
//
// 对抗变异：把 hexdata.go 里 officialTiers[id] 的查表换成 primary.Items[0].Tier，
// 或把 `if snapshotErr == nil` 的守卫去掉后在 insights 失败时回退行级档位，本测试必须 FAIL。
func TestLoadMayhemDetailHeroTierStaysNilWithoutInsights(t *testing.T) {
	recorder := &r116aRecorder{}
	mock := &r116aMock{
		bodies: map[string][]byte{
			hexdataMetaPath:      r116aMetaFixture(t, hexdataTestBuild),
			hexdataAnswerPath:    r116aAnswerFixture(t, hexdataTestBuild),
			hexdataPostmatchPath: r116aFixture(t, "hexdata-postmatch.json"),
		},
		status:   map[string]int{hexdataHextechInsightsPath: http.StatusInternalServerError},
		heroJSON: r116aFixture(t, "hexdata-hero-157.json"),
	}
	provider := newHexdataBudgetProvider(t, t.TempDir(), mock.roundTrip(recorder))
	provider.championMeta[157] = championMetadata{ID: 157, Key: "Yasuo", Slug: "yasuo", NameZH: "疾风剑豪"}
	response, err := provider.loadMayhemDetail(context.Background(), "157")
	if err != nil {
		t.Fatalf("insights being unavailable must degrade, not fail the whole detail page: %v", err)
	}
	if response.Stats.Tier != nil {
		t.Fatalf("hero tier was invented from row-level data without hextech-insights: %v", *response.Stats.Tier)
	}
	// 行级官方档位本身仍要透传（P0-5-1 用它替代本地分位算法），只是不许被提升成英雄级梯度。
	if len(response.ItemRanking) == 0 || response.ItemRanking[0].OfficialTier == 0 {
		t.Fatalf("row-level official tier was lost while insights was down: %#v", response.ItemRanking)
	}
	if got := recorder.count(hexdataHextechInsightsPath); got == 0 {
		t.Fatal("insights was never requested, so this test proves nothing about the fallback")
	}
}

// 判据 3 的对抗版本：中文名故意重复且含特殊字符时，资产 ID 仍等于 JSON 的 itemId，
// 不会因为名称反查走错装备（Data Dragon 目录在这里不可用，404）。
func TestLoadMayhemDetailItemIDsSurviveDuplicateAndSpecialCharacterNames(t *testing.T) {
	recorder := &r116aRecorder{}
	heroJSON := []byte(`{
		"items":[
			{"itemId":"3031","itemName":"无尽之刃（改）","winRate":0.6,"pickRate":0.2,"games":1000,"tier":1,"hexTier":"hang","hexLabel":"夯","hexScore":90},
			{"itemId":"3032","itemName":"无尽之刃（改）","winRate":0.55,"pickRate":0.1,"games":900,"tier":2,"hexTier":"top","hexLabel":"顶级","hexScore":80},
			{"itemId":"6675","itemName":"纳沃利迅刃 · 特殊/字符\\","winRate":0.5,"pickRate":0.05,"games":800,"tier":3,"hexTier":"mid","hexLabel":"中","hexScore":70}
		],
		"augments":[{"augmentId":"2095","augmentName":"掷骰狂人","augmentDescription":"说明","rarity":"棱彩","tier":1,"hexScore":95.8,"pairWinRate":0.679,"games":91113,"hexTier":"hang","hexLabel":"夯"}],
		"trios":[{"trioKey":"1058:1077:1336","augmentIds":["1058","1077","1336"],"winRate":0.65,"games":10453}]
	}`)
	provider, _ := r116aMayhemProvider(t, recorder, heroJSON)
	response, err := provider.loadMayhemDetail(context.Background(), "157")
	if err != nil {
		t.Fatal(err)
	}
	if len(response.ItemRanking) != 3 {
		t.Fatalf("item ranking = %#v", response.ItemRanking)
	}
	for index, want := range []int{3031, 3032, 6675} {
		row := response.ItemRanking[index]
		if row.Assets[0].ID != want {
			t.Fatalf("item row %d id = %d, want %d (名称反查会在这里走错装备)", index, row.Assets[0].ID, want)
		}
	}
	if response.ItemRanking[0].Assets[0].Name != response.ItemRanking[1].Assets[0].Name {
		t.Fatalf("duplicate names were not preserved: %#v", response.ItemRanking)
	}
	if !strings.Contains(response.ItemRanking[2].Assets[0].Name, "特殊/字符") {
		t.Fatalf("special characters were mangled: %q", response.ItemRanking[2].Assets[0].Name)
	}
	if response.ItemRanking[0].OfficialTier != 1 || response.ItemRanking[1].OfficialTier != 2 {
		t.Fatalf("official tiers = %d/%d", response.ItemRanking[0].OfficialTier, response.ItemRanking[1].OfficialTier)
	}
	// 目录不可用时不许编造图标路径，只保留真实 ID 与上游名称。
	if got := recorder.countPrefix("/cdn/"); got != 0 {
		t.Fatalf("unexpected cdn requests: %d", got)
	}
}

// 对抗变异（P1 第 1 条）：augmentDescription 为空时不能显示空描述，
// 要走既有的占位/整块隐藏降级，而不是把空串塞给用户。
func TestEmptyAugmentDescriptionDegradesToOfflinePlaceholder(t *testing.T) {
	recorder := &r116aRecorder{}
	heroJSON := []byte(`{
		"items":[{"itemId":"3153","itemName":"破败王者之刃","winRate":0.6,"games":445607,"tier":1,"hexLabel":"夯","hexScore":90}],
		"augments":[
			{"augmentId":"2095","augmentName":"掷骰狂人","augmentDescription":"","rarity":"棱彩","tier":1,"hexScore":95.8,"pairWinRate":0.679,"games":91113},
			{"augmentId":"1058","augmentName":"秘术冲拳","augmentDescription":"攻击特效使你的各个技能的冷却时间缩减1.25秒。","rarity":"棱彩","tier":1,"hexScore":90.1,"pairWinRate":0.638,"games":425371}
		],
		"trios":[{"trioKey":"1058:1077:1336","augmentIds":["1058","1077","1336"],"winRate":0.65,"games":10453}]
	}`)
	provider, _ := r116aMayhemProvider(t, recorder, heroJSON)
	response, err := provider.loadMayhemDetail(context.Background(), "157")
	if err != nil {
		t.Fatal(err)
	}
	if len(response.RecommendedAugments) != 2 {
		t.Fatalf("augments = %#v", response.RecommendedAugments)
	}
	if got := response.RecommendedAugments[0].Assets[0].Description; got != augmentOfflineDescription {
		t.Fatalf("empty description degraded to %q, want the offline placeholder %q", got, augmentOfflineDescription)
	}
	if got := response.RecommendedAugments[1].Assets[0].Description; got != "攻击特效使你的各个技能的冷却时间缩减1.25秒。" {
		t.Fatalf("real description was replaced: %q", got)
	}
	encoded, err := json.Marshal(response.RecommendedAugments)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), `"description":""`) {
		t.Fatalf("empty description leaked into the payload: %s", encoded)
	}
}

// ---------------------------------------------------------------------------
// P0-5 对抗变异：忘记 promote → 12 小时后强制回源、上游一抖就没数据
// ---------------------------------------------------------------------------

// refreshBuild 那条通道用的是 key+"|refresh" 且 persistDisk=false（hexdata.go:1079），
// 那条通道上只有 promote() 会把正文写到 {buildID}|{kind}|{id} 下。这条测试把时间推过
// 12h 软 TTL 并让上游 500，断言数据仍能从磁盘兜底。
//
// P2-3（R120）实测更正——原注释「忘记 promote 时这里必然失败」对 8 处 promote **一处
// 都不成立**：把 hexdata.go 里的 8 处 p.hexdata.promote( 逐个删掉（answer :1475、
// heroes :1864、meta :2105、postmatch :2654、hextech-insights :2871、augment :3148、
// rarity :3180、hero-json :3318）各跑一次整包，本测试 8 次全部 PASS。它断言的「13 小时
// 后仍能从磁盘兜底」由正常路径提供：load() 走 loadWithStatus(persistDisk=true)
// （hexdata.go:1096）本来就落盘；promote 只有在 refreshBuild 通道（persistDisk=false，
// hexdata.go:1079）才是唯一落盘者，而各 loader 先走 loadHexdataMeta，adoptMeta
// （hexdata.go:1406，:1418 刷新 BuildChecked）会把软 TTL 顶回去，那条分支在这些 loader
// 里基本走不到（探针：把 BuildChecked 拨到 13 小时前，删掉 promote 数据仍然落盘）。
//
// 真正能钉住 promote 的是另外三条测试，而且只覆盖 2 处：
//   - answer（hexdata.go:1475）→ TestHexdataColdRankingBudgetAndDiskRestart
//     （hexdata_test.go:478）与
//     TestLoadMayhemDetailColdHeroCostsFourHexdataRequestsWithWarmSiteMetadata
//     （本文件 :780）；
//   - meta（hexdata.go:2105）→ TestLoadMayhemDetailRequestsHeroJSONAndMetaOnly
//     （本文件 :651）。
//
// 其余 6 处（heroes / postmatch / hextech-insights / augment / rarity / hero-json）删掉后
// 整包失败集与基线完全一致，即**没有任何测试覆盖**。它们是无害的冗余防御，保留不删；
// 测试名里的「BecauseTheyArePromoted」只对上述 answer / meta 成立，不再被当成对这 6 处
// 的保护承诺。
func TestHexdataJSONKindsSurviveSoftTTLBecauseTheyArePromoted(t *testing.T) {
	recorder := &r116aRecorder{}
	provider, root := r116aMayhemProvider(t, recorder, r116aFixture(t, "hexdata-hero-157.json"))
	if _, err := provider.loadHexdataPostmatch(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.loadHexdataHextechInsights(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.loadMayhemDetail(context.Background(), "157"); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{
		hexdataTestBuild + "|meta|site",
		hexdataTestBuild + "|postmatch|all",
		hexdataTestBuild + "|hextech-insights|all",
		hexdataTestBuild + "|hero-json|157",
	} {
		entry, err := provider.cache.readDisk(key)
		if err != nil || len(entry.Data) == 0 {
			t.Fatalf("%s was not promoted to disk: %v", key, err)
		}
		if entry.Key != key {
			t.Fatalf("envelope key = %q, want %q", entry.Key, key)
		}
	}
	// 落盘文件名必须带 hexdata- 前缀（既有通道），这样 P3 才能按前缀扫到它们。
	files, err := filepath.Glob(filepath.Join(root, championDataCacheDirectory, "hexdata-*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) < 4 {
		t.Fatalf("hexdata- prefixed cache files = %d, want >= 4", len(files))
	}

	// 13 小时后：软 TTL 过期，上游整体 500。磁盘上有 promote 过的正文才能兜底。
	broken := &r116aRecorder{}
	down := championRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		broken.add(request)
		return &http.Response{StatusCode: http.StatusInternalServerError, Header: make(http.Header), Body: http.NoBody, Request: request}, nil
	})
	restarted := newHexdataBudgetProvider(t, root, down)
	restarted.championMeta[157] = championMetadata{ID: 157, Slug: "yasuo", NameZH: "疾风剑豪"}
	later := time.Now().Add(13 * time.Hour)
	restarted.cache.now = func() time.Time { return later }
	restarted.hexdata.now = func() time.Time { return later }
	restarted.hexdata.state.BuildChecked = time.Now().Add(-13 * time.Hour)
	if _, err := restarted.loadHexdataPostmatch(context.Background()); err != nil {
		t.Fatalf("postmatch had no disk fallback after the soft TTL: %v", err)
	}
	if _, err := restarted.loadHexdataHextechInsights(context.Background()); err != nil {
		t.Fatalf("hextech-insights had no disk fallback after the soft TTL: %v", err)
	}
	if _, err := restarted.loadMayhemDetail(context.Background(), "157"); err != nil {
		t.Fatalf("mayhem detail had no disk fallback after the soft TTL: %v", err)
	}
	if got := broken.countPrefix("/api/hexdata"); got == 0 {
		t.Fatal("the soft TTL did not force a revalidation request")
	}
}

// ---------------------------------------------------------------------------
// P2：两个全量聚合文件
// ---------------------------------------------------------------------------

func TestLoadHexdataPostmatchFetchesOnceAcrossTwoCalls(t *testing.T) {
	recorder := &r116aRecorder{}
	provider, _ := r116aMayhemProvider(t, recorder, r116aFixture(t, "hexdata-hero-157.json"))
	first, err := provider.loadHexdataPostmatch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	second, err := provider.loadHexdataPostmatch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// 判据 1：模拟对局页两次打开，实际只发 1 次 HTTP 请求。
	if got := recorder.count(hexdataPostmatchPath); got != 1 {
		t.Fatalf("postmatch requests = %d, want 1 (%v)", got, recorder.hexdataPaths())
	}
	if len(first.Heroes) != 173 || len(second.Heroes) != 173 {
		t.Fatalf("postmatch heroes = %d/%d, want 173", len(first.Heroes), len(second.Heroes))
	}
	row, ok := first.Heroes[157]
	if !ok {
		t.Fatal("postmatch is missing hero 157")
	}
	if math.Abs(row.KDA-2.597684) > 1e-6 || math.Abs(row.AvgDamage-41778.020045) > 1e-6 || row.PentaKills != 29056 {
		t.Fatalf("postmatch hero 157 = %#v", row)
	}
	if row.DoubleKills == 0 || row.TripleKills == 0 || row.QuadraKills == 0 {
		t.Fatalf("multi kill counters were lost: %#v", row)
	}
	if first.Citation.BuildID != hexdataTestBuild || first.Citation.CanonicalURL != "https://"+hexdataHost+hexdataAggregateCanonical {
		t.Fatalf("postmatch citation = %#v", first.Citation)
	}
	if first.FetchedAt.IsZero() {
		t.Fatal("postmatch fetchedAt is zero")
	}
}

func TestLoadHexdataHextechInsightsFetchesOnceAndParsesIDs(t *testing.T) {
	recorder := &r116aRecorder{}
	provider, _ := r116aMayhemProvider(t, recorder, r116aFixture(t, "hexdata-hero-157.json"))
	first, err := provider.loadHexdataHextechInsights(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.loadHexdataHextechInsights(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := recorder.count(hexdataHextechInsightsPath); got != 1 {
		t.Fatalf("hextech-insights requests = %d, want 1 (%v)", got, recorder.hexdataPaths())
	}
	// 判据 2：heroes 长度在 150~200 之间。
	if len(first.Heroes) < 150 || len(first.Heroes) > 200 {
		t.Fatalf("insights heroes = %d, want 150..200", len(first.Heroes))
	}
	if len(first.Augments) != 211 {
		t.Fatalf("insights augments = %d, want 211", len(first.Augments))
	}
	hero := first.Heroes[0]
	if hero.ChampionID != 1 || hero.ID != "1" || len(hero.TopItems) != 5 || len(hero.TopAugments) != 5 {
		t.Fatalf("insights hero[0] = %#v", hero)
	}
	if hero.TopItems[0].AssetID <= 0 || hero.TopAugments[0].AssetID <= 0 {
		t.Fatalf("insight asset ids were not converted: %#v %#v", hero.TopItems[0], hero.TopAugments[0])
	}
	if hero.TopAugments[0].Rarity == "" || hero.TopAugments[0].Confidence == "" {
		t.Fatalf("insight augment lost rarity/confidence: %#v", hero.TopAugments[0])
	}
	if first.Augments[0].AugmentID != 1 || first.Augments[0].CoverageHeroCount != 173 {
		t.Fatalf("insights augment[0] = %#v", first.Augments[0])
	}
	if first.Citation.BuildID != hexdataTestBuild || strings.Contains(first.Citation.CanonicalURL, "/api/") {
		t.Fatalf("insights citation = %#v", first.Citation)
	}
}

// ID 非法的条目必须被丢弃并计数，不能塞进 map 的 0 键。
func TestParseHexdataAggregatesDropUnparseableIDs(t *testing.T) {
	rows := make(map[string]map[string]float64, 160)
	for index := 1; index <= 160; index++ {
		rows[strconv.Itoa(index)] = map[string]float64{"kda": 3.1, "avgDamage": 40000}
	}
	rows["不是数字"] = map[string]float64{"kda": 1}
	rows["0"] = map[string]float64{"kda": 1}
	data, err := json.Marshal(rows)
	if err != nil {
		t.Fatal(err)
	}
	parsed, dropped, err := parseHexdataPostmatch(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed) != 160 || dropped != 2 {
		t.Fatalf("postmatch rows = %d dropped = %d, want 160/2", len(parsed), dropped)
	}
	if _, ok := parsed[0]; ok {
		t.Fatal("an unparseable hero id was stored under key 0")
	}
	insights, dropped, err := parseHexdataHextechInsights([]byte(`{"heroes":[{"id":"157","name":"疾风剑豪","tier":1,"topItems":[{"id":"脏数据","name":"x"}]}],"augments":[{"id":"2095","name":"掷骰狂人"},{"id":"abc","name":"脏数据"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(insights.Heroes) != 1 || insights.Heroes[0].ChampionID != 157 || len(insights.Augments) != 1 || dropped != 1 {
		t.Fatalf("insights = %#v dropped=%d", insights, dropped)
	}
	// topItems 里 ID 非法时保留字符串原样、AssetID 留 0，绝不猜一个 ID。
	if insights.Heroes[0].TopItems[0].AssetID != 0 || insights.Heroes[0].TopItems[0].ID != "脏数据" {
		t.Fatalf("insight item id was invented: %#v", insights.Heroes[0].TopItems[0])
	}
	if _, _, err := parseHexdataHextechInsights([]byte(`{"heroes":[],"augments":[]}`)); err == nil {
		t.Fatal("empty insights payload was accepted")
	}
}

// ---------------------------------------------------------------------------
// P1-4/6/7：死代码确实删干净了
// ---------------------------------------------------------------------------

func TestHexdataDeadHTMLChainIsGone(t *testing.T) {
	source, err := os.ReadFile("hexdata.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(source)
	// 只查「定义 / 调用」这两种真引用：注释里提到旧函数名是删除理由的记录，
	// 不算死引用；漏删的调用点编译期就会报错，这里再兜一层。
	for _, symbol := range []string{
		"p.decorateHexdataItems(", "func (p *championProvider) decorateHexdataItems(",
		"p.hydrateMayhemAugmentCopy(", "func (p *championProvider) hydrateMayhemAugmentCopy(",
		"p.mayhemAugmentCopy(", "func (p *championProvider) mayhemAugmentCopy(",
		"p.fetchMayhemAugmentCopy(", "func (p *championProvider) fetchMayhemAugmentCopy(",
		"p.mayhemAugmentSlugs(", "func (p *championProvider) mayhemAugmentSlugs(",
		"p.rememberAugmentSlugFailure(", "func (p *championProvider) rememberAugmentSlugFailure(",
		"parseHexdataHeroDetail(", "hexdataTierPattern", "mayhemAugmentCopyConcurrency",
		"mayhemAugmentCopyTTL", "mayhemAugmentSlugFailureTTL", "mayhemAugmentIDFloor",
		"hexdata_item_match", `load(ctx, "hero",`,
	} {
		if strings.Contains(body, symbol) {
			t.Fatalf("dead reference %q is still in hexdata.go", symbol)
		}
	}
	// 保留项：decorateHexdataAugments 继续按 ID 补 CommunityDragon 图标/rarity，
	// 且绝不改用 hexdata 自己 CDN 的 augmentIconUrl / itemImageUrl（Anti-scope 第 2 条）。
	if !strings.Contains(body, "func (p *championProvider) decorateHexdataAugments(") {
		t.Fatal("decorateHexdataAugments was deleted but must be kept")
	}
	for _, forbidden := range []string{"augmentIconUrl", "itemImageUrl"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("hexdata CDN field %q must not be used (Anti-scope 2)", forbidden)
		}
	}
	// 详情页抓取必须走 JSON kind。
	if !strings.Contains(body, `p.hexdata.load(ctx, "hero-json", strconv.Itoa(id), requestPath, "application/json", false)`) {
		t.Fatal("loadMayhemDetail no longer fetches /api/hexdata/heroes/{id} as hero-json")
	}
	if strings.Contains(body, `"hero", strconv.Itoa(id), canonical, "text/html`) {
		t.Fatal("loadMayhemDetail still fetches the HTML hero page")
	}
	// 两条 HTML 正则仍在白名单里：它们仍被榜单页/图鉴页的链接解析使用。
	if !strings.Contains(body, "hexdataHeroPathPattern.MatchString(path)") || !strings.Contains(body, "hexdataAugmentPathPattern.MatchString(path)") {
		t.Fatal("HTML path patterns were dropped from allowedPath without a replacement")
	}
}
