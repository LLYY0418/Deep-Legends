// R116-D：推荐页克制/协同提示（P1-2/P1-3）+ 局内出装解析（P1-4 阶段一，
// 阶段二只测纯函数）+ 队伍画像缺口提示（P1-5）的验证。
//
// 数据一律用仓库里的真实上游夹具（backend/testdata/r116/），不手编期望值：
//   - hexdata-hero-157.json：weakAgainst/strongAgainst/teammateSynergies 各 7 条
//     （全部 evidence="supported"、qValue=0）、terminalItemTrios 前 10 条；
//   - hexdata-postmatch.json：173 英雄 × 22 项赛后指标。
//
// 三类断言：
//  1. 交集与阈值本身（纯函数，含工单要求的两处对抗变异）；
//  2. 请求预算——加了 P1-2/P1-3/P1-5 之后 hexdata 上游请求数一字不变
//     （Anti-scope 第 1 条，评审 4.2/4.4）；
//  3. 降级红线——取不到就不生成，绝不用 0 或猜测值顶替。
//
// P1-4 阶段二（下一步出装）已实现纯计算但**未接线**，本文件既测它的行为，
// 也钉住「它确实没有生产调用方」这条事实（TestR116DStageTwoStaysUnwired）。
package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
)

// ---------------------------------------------------------------------------
// 夹具装载
// ---------------------------------------------------------------------------

// r116dHeroDetail 读真实 hero-json 夹具并走生产解析路径（含 R116-A 的字符串 ID
// 安全转换与 terminalItemTrios 的 games 降序裁剪），保证测试消费的形状与线上一致。
func r116dHeroDetail(t *testing.T) hexdataHeroDetailV2 {
	t.Helper()
	detail, err := parseHexdataHeroJSON(r116aFixture(t, "hexdata-hero-157.json"))
	if err != nil {
		t.Fatalf("parse hero fixture: %v", err)
	}
	if len(detail.WeakAgainst) != 7 || len(detail.StrongAgainst) != 7 || len(detail.TeammateSynergies) != 7 {
		t.Fatalf("fixture shape drifted: weak=%d strong=%d synergy=%d", len(detail.WeakAgainst), len(detail.StrongAgainst), len(detail.TeammateSynergies))
	}
	return detail
}

// r116dPostmatch 读真实 postmatch 夹具（173 英雄）。
func r116dPostmatch(t *testing.T) (map[int]hexdataPostmatchRow, map[int]map[string]bool) {
	t.Helper()
	rows, present, dropped, err := parseHexdataPostmatchWithPresence(r116aFixture(t, "hexdata-postmatch.json"))
	if err != nil {
		t.Fatalf("parse postmatch fixture: %v", err)
	}
	if len(rows) != 173 || dropped != 0 {
		t.Fatalf("postmatch fixture = %d heroes / %d dropped, want 173/0", len(rows), dropped)
	}
	return rows, present
}

// r116dPostmatchRows 按 ID 顺序取出若干英雄的行，缺一个就直接失败——
// 「fixture 里没有这个英雄」必须炸在装配阶段，不能悄悄变成「阵容少一人」。
func r116dPostmatchRows(t *testing.T, heroes map[int]hexdataPostmatchRow, ids ...int) []hexdataPostmatchRow {
	t.Helper()
	rows := make([]hexdataPostmatchRow, 0, len(ids))
	for _, id := range ids {
		row, ok := heroes[id]
		if !ok {
			t.Fatalf("postmatch fixture is missing hero %d", id)
		}
		rows = append(rows, row)
	}
	return rows
}

// ---------------------------------------------------------------------------
// P1-2 / P1-3：交集、排序、上限、字段透传
// ---------------------------------------------------------------------------

// 工单验证判据 1：本人英雄的 weakAgainst 里有一条 opponentChampionId 命中当前
// 对局某个敌方英雄 → 生成且仅生成 1 条 MatchupNotice。
// 夹具里 weakAgainst[0] 是 200（虚空女皇），敌方 5 人里放一个 200、其余四个都不在
// 14 条名单里，所以必须恰好 1 条。
func TestGameplayMatchupNoticesSingleHitProducesExactlyOneNotice(t *testing.T) {
	detail := r116dHeroDetail(t)
	want := detail.WeakAgainst[0]
	if want.OpponentChampionID != 200 {
		t.Fatalf("fixture anchor moved: weakAgainst[0] = %d, want 200", want.OpponentChampionID)
	}
	notices := gameplayMatchupNotices(detail.WeakAgainst, detail.StrongAgainst, []int{200, 7, 8, 9, 10})
	if len(notices) != 1 {
		t.Fatalf("notices = %#v, want exactly 1", notices)
	}
	got := notices[0]
	if got.Direction != "weak" || got.OpponentID != 200 {
		t.Fatalf("notice identity = %q/%d, want weak/200", got.Direction, got.OpponentID)
	}
	if got.CounterDelta != want.CounterDelta {
		t.Fatalf("counterDelta = %v, want upstream %v (must be passed through unmodified)", got.CounterDelta, want.CounterDelta)
	}
	// 评审 6.5 硬性要求：evidence / confidenceLow / confidenceHigh 必须透传。
	if got.Evidence != want.Evidence || got.Evidence != "supported" {
		t.Fatalf("evidence = %q, want %q", got.Evidence, want.Evidence)
	}
	if got.ConfidenceLow != want.ConfidenceLow || got.ConfidenceHigh != want.ConfidenceHigh {
		t.Fatalf("confidence interval = [%v,%v], want [%v,%v]", got.ConfidenceLow, got.ConfidenceHigh, want.ConfidenceLow, want.ConfidenceHigh)
	}
}

// 工单验证判据 2：7 条 weakAgainst（+7 条 strongAgainst）均未命中当前 10 人 →
// MatchupNotices 为空。空必须是 nil / len==0，不能是一个占位元素。
func TestGameplayMatchupNoticesMissProducesNothing(t *testing.T) {
	detail := r116dHeroDetail(t)
	// 1..6 都不在夹具的 14 个对手 ID 里（200/90/895/122/12/44/31/29/350/901/145/81/76/22）。
	misses := []int{1, 2, 3, 5, 6}
	for _, id := range misses {
		for _, row := range append(append([]hexdataMatchupRow(nil), detail.WeakAgainst...), detail.StrongAgainst...) {
			if row.OpponentChampionID == id {
				t.Fatalf("fixture collision: %d is actually in the matchup table", id)
			}
		}
	}
	if notices := gameplayMatchupNotices(detail.WeakAgainst, detail.StrongAgainst, misses); len(notices) != 0 {
		t.Fatalf("misses produced notices: %#v", notices)
	}
	// 敌方阵容未知（ChampSelect 拿不到 theirTeam）时同样一条都不生成。
	if notices := gameplayMatchupNotices(detail.WeakAgainst, detail.StrongAgainst, nil); len(notices) != 0 {
		t.Fatalf("empty roster produced notices: %#v", notices)
	}
}

// P1-2：teammateSynergies 与我方队友求交。夹具里 teammateSynergies[0] 是 4（卡牌大师）。
// 本人英雄（157）必须先从队友集合里剔掉，否则「我与我自己协同」这种荒谬条目
// 会在前端出现。
func TestGameplaySynergyNoticesHitAndSelfExclusion(t *testing.T) {
	detail := r116dHeroDetail(t)
	want := detail.TeammateSynergies[0]
	if want.TeammateChampionID != 4 {
		t.Fatalf("fixture anchor moved: teammateSynergies[0] = %d, want 4", want.TeammateChampionID)
	}
	allyIDs := []int{157, 4, 12, 412, 222}
	teammates := gameplayRosterWithout(allyIDs, 157)
	if reflect.DeepEqual(teammates, []int{4, 12, 412, 222}) == false {
		t.Fatalf("self was not removed from the teammate roster: %#v", teammates)
	}
	notices := gameplaySynergyNotices(detail.TeammateSynergies, teammates)
	if len(notices) != 1 {
		t.Fatalf("notices = %#v, want exactly 1", notices)
	}
	got := notices[0]
	if got.TeammateID != 4 || got.SynergyDelta != want.SynergyDelta {
		t.Fatalf("notice = %d/%v, want 4/%v", got.TeammateID, got.SynergyDelta, want.SynergyDelta)
	}
	if got.Evidence != "supported" || got.ConfidenceLow != want.ConfidenceLow || got.ConfidenceHigh != want.ConfidenceHigh {
		t.Fatalf("evidence/confidence not passed through: %#v", got)
	}
	// 队友全不在 7 条名单里 → 一条都不生成（覆盖率 18.8% 的常态）。
	if notices := gameplaySynergyNotices(detail.TeammateSynergies, []int{1, 2, 3, 5}); len(notices) != 0 {
		t.Fatalf("misses produced notices: %#v", notices)
	}
}

// 排序与上限：各自最多 5 条，按效果量绝对值降序。构造 7 条全部命中的场景，
// 断言留下的是 |delta| 最大的 5 条且严格降序。
func TestGameplayRosterNoticesSortByAbsoluteDeltaAndCapAtFive(t *testing.T) {
	weak := make([]hexdataMatchupRow, 0, 7)
	deltas := []float64{-0.05, 0.01, -0.09, 0.03, -0.07, 0.02, -0.11}
	for index, delta := range deltas {
		weak = append(weak, hexdataMatchupRow{OpponentChampionID: 100 + index, CounterDelta: delta, Evidence: "supported"})
	}
	roster := []int{100, 101, 102, 103, 104, 105, 106}
	if len(roster) > gameplayRosterSideLimit {
		// 交集函数本身不限制 roster 长度（限制在解析查询串那一层），
		// 这条断言只是提醒：真实海斗一侧最多 5 人。
		t.Logf("roster length %d exceeds one side of a 5v5", len(roster))
	}
	notices := gameplayMatchupNotices(weak, nil, roster)
	if len(notices) != gameplayRosterNoticeLimit {
		t.Fatalf("notices = %d, want cap %d", len(notices), gameplayRosterNoticeLimit)
	}
	for index := 1; index < len(notices); index++ {
		left := absFloat(notices[index-1].CounterDelta)
		right := absFloat(notices[index].CounterDelta)
		if left < right {
			t.Fatalf("not sorted by |counterDelta| desc at %d: %v then %v", index, left, right)
		}
	}
	// 绝对值最大的两条必然是 -0.11（106）与 -0.09（102）。
	if notices[0].OpponentID != 106 || notices[1].OpponentID != 102 {
		t.Fatalf("top two = %d/%d, want 106/102", notices[0].OpponentID, notices[1].OpponentID)
	}

	synergies := make([]hexdataSynergyRow, 0, 7)
	for index := range deltas {
		synergies = append(synergies, hexdataSynergyRow{TeammateChampionID: 200 + index, SynergyDelta: deltas[index], Evidence: "supported"})
	}
	synergyRoster := []int{200, 201, 202, 203, 204, 205, 206}
	synergyNotices := gameplaySynergyNotices(synergies, synergyRoster)
	if len(synergyNotices) != gameplayRosterNoticeLimit {
		t.Fatalf("synergy notices = %d, want cap %d", len(synergyNotices), gameplayRosterNoticeLimit)
	}
	if synergyNotices[0].TeammateID != 206 || synergyNotices[1].TeammateID != 202 {
		t.Fatalf("synergy top two = %d/%d, want 206/202", synergyNotices[0].TeammateID, synergyNotices[1].TeammateID)
	}
}

// 工单笔误的回归钉：工单 P1-2/P1-3 的结构体示例写了
//
//	ConfidenceLow, ConfidenceHigh float64 `json:"confidenceLow,confidenceHigh"`
//
// Go 的 struct tag 不支持一个 tag 两个名字——那样字段名会被解析成
// "confidenceLow,confidenceHigh"，两个值在 JSON 里都永远匹配不上、被静默丢掉。
// 这条测试直接检查线上 JSON 的键名与取值，写错就红。
func TestGameplayRosterNoticeJSONTagsEmitBothConfidenceBounds(t *testing.T) {
	matchupData, err := json.Marshal(gameplayMatchupNotice{
		Direction: "weak", OpponentID: 200, CounterDelta: 0.03639,
		Evidence: "supported", ConfidenceLow: 0.525157, ConfidenceHigh: 0.535102,
	})
	if err != nil {
		t.Fatal(err)
	}
	var matchup map[string]any
	if err := json.Unmarshal(matchupData, &matchup); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]any{
		"direction": "weak", "opponentChampionId": float64(200), "counterDelta": 0.03639,
		"evidence": "supported", "confidenceLow": 0.525157, "confidenceHigh": 0.535102,
	} {
		got, ok := matchup[key]
		if !ok {
			t.Fatalf("matchup notice JSON is missing %q: %s", key, matchupData)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("matchup notice %q = %#v, want %#v (%s)", key, got, want, matchupData)
		}
	}
	if strings.Contains(string(matchupData), "confidenceLow,confidenceHigh") {
		t.Fatalf("the worklist tag typo leaked into the payload: %s", matchupData)
	}

	synergyData, err := json.Marshal(gameplaySynergyNotice{
		TeammateID: 4, SynergyDelta: 0.078605, Evidence: "supported", ConfidenceLow: 0.638803, ConfidenceHigh: 0.644849,
	})
	if err != nil {
		t.Fatal(err)
	}
	var synergy map[string]any
	if err := json.Unmarshal(synergyData, &synergy); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]any{
		"teammateChampionId": float64(4), "synergyDelta": 0.078605,
		"evidence": "supported", "confidenceLow": 0.638803, "confidenceHigh": 0.644849,
	} {
		got, ok := synergy[key]
		if !ok {
			t.Fatalf("synergy notice JSON is missing %q: %s", key, synergyData)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("synergy notice %q = %#v, want %#v (%s)", key, got, want, synergyData)
		}
	}
}

// evidence 不是 "supported" 时也必须原样透传（评审 6.5：万一上游未来放宽阈值
// 出现非显著项，前端要能正确处理）。这里钉住后端不做任何过滤或改写。
func TestGameplayRosterNoticesPassThroughWeakEvidence(t *testing.T) {
	weak := []hexdataMatchupRow{{OpponentChampionID: 200, CounterDelta: 0.01, Evidence: "underpowered", ConfidenceLow: 0.4, ConfidenceHigh: 0.6}}
	notices := gameplayMatchupNotices(weak, nil, []int{200})
	if len(notices) != 1 || notices[0].Evidence != "underpowered" {
		t.Fatalf("non-supported evidence was filtered or rewritten: %#v", notices)
	}
	synergies := []hexdataSynergyRow{{TeammateChampionID: 4, SynergyDelta: 0.01, Evidence: ""}}
	synergyNotices := gameplaySynergyNotices(synergies, []int{4})
	if len(synergyNotices) != 1 || synergyNotices[0].Evidence != "" {
		t.Fatalf("empty evidence was rewritten: %#v", synergyNotices)
	}
}

// 查询串解析：严格策略。任何一段非法就整串丢弃，绝不猜「大概是这几个」。
func TestGameplayRosterChampionIDsStrictParsing(t *testing.T) {
	tests := []struct {
		raw  string
		want []int
	}{
		{"", nil},
		{"   ", nil},
		{"157,4,12,412,222", []int{157, 4, 12, 412, 222}},
		{" 157 , 4 ", []int{157, 4}},
		{"4,4,5", []int{4, 5}},       // 去重，保持首次出现顺序
		{"157,4,12,412,222,29", nil}, // 超过一侧 5 人 → 整串丢弃
		{"157,x", nil},               // 非数字
		{"157,0", nil},               // 0 不是合法英雄 ID
		{"157,-3", nil},              // 负数
		{"10001", nil},               // 超出英雄 ID 上限
		{"157,,4", nil},              // 空段
	}
	for _, test := range tests {
		got := gameplayRosterChampionIDs(test.raw)
		if len(got) == 0 && len(test.want) == 0 {
			continue
		}
		if !reflect.DeepEqual(got, test.want) {
			t.Fatalf("gameplayRosterChampionIDs(%q) = %#v, want %#v", test.raw, got, test.want)
		}
	}
}

// 对抗变异（工单 P1-2/P1-3）：故意把交集判断写反——拿**队友** roster 去比
// weakAgainst/strongAgainst 的 opponentChampionId（等价于比较 teammateID 而不是
// 敌方 championId），构造出全零匹配的场景，确认测试能检测到「永远不命中」这种
// 隐蔽退化。
//
// 这条测试本身不会红：它把「变异体」与「生产实现」放在同一个 fixture 上对跑，
// 断言变异体确实退化成 0 条、而生产实现确实命中。如果哪天有人把生产实现写成
// 变异体这样，上面那几条「恰好 1 条」的判据会立刻红。
// 实际把源码改反、跑出问题输出、再还原的过程记在 docs/r116d-execution-ledger.md。
func TestGameplayMatchupNoticesNeverMatchDegenerationIsDetected(t *testing.T) {
	detail := r116dHeroDetail(t)
	// 队友 roster 必须与夹具的 14 个对手 ID（200/90/895/122/12/44/31 与
	// 29/350/901/145/81/76/22）完全不相交，否则「写反」的变异体会碰巧命中，
	// 这条对抗变异就测不出「永远不命中」了。157 亚索 / 4 卡牌 / 64 盲僧 /
	// 222 金克丝 / 412 锤石 均不在其中，而 4 同时是 teammateSynergies 的第一条。
	allyIDs := []int{157, 4, 64, 222, 412}
	enemyIDs := []int{200, 90, 7, 8, 9}

	// mutatedMatchupNotices 是「写反」的变异体：交集对象换成了队友 roster。
	mutatedMatchupNotices := func(weak, strong []hexdataMatchupRow, allyRoster []int) []gameplayMatchupNotice {
		roster := make(map[int]struct{}, len(allyRoster))
		for _, id := range allyRoster {
			roster[id] = struct{}{}
		}
		var notices []gameplayMatchupNotice
		for _, row := range append(append([]hexdataMatchupRow(nil), weak...), strong...) {
			if _, hit := roster[row.OpponentChampionID]; hit {
				notices = append(notices, gameplayMatchupNotice{Direction: "weak", OpponentID: row.OpponentChampionID})
			}
		}
		return notices
	}

	correct := gameplayMatchupNotices(detail.WeakAgainst, detail.StrongAgainst, enemyIDs)
	if len(correct) != 2 {
		t.Fatalf("correct implementation = %d notices, want 2 (200 weak + 90 weak)", len(correct))
	}
	mutated := mutatedMatchupNotices(detail.WeakAgainst, detail.StrongAgainst, allyIDs)
	if len(mutated) != 0 {
		t.Fatalf("the reversed intersection was supposed to match nothing, got %#v", mutated)
	}
	// 关键断言：同一个 fixture 下「正确 vs 变异」结果不同 → 测试网确实能分辨
	// 「永远不命中」这种退化，而不是碰巧两边都空。
	if len(correct) == len(mutated) {
		t.Fatal("mutation is undetectable: both implementations produced the same notice count")
	}

	// 同一套变异思路用在协同侧：拿敌方 roster 去比 teammateChampionId。
	mutatedSynergy := gameplaySynergyNotices(detail.TeammateSynergies, gameplayRosterWithout(enemyIDs, 157))
	correctSynergy := gameplaySynergyNotices(detail.TeammateSynergies, gameplayRosterWithout(allyIDs, 157))
	if len(mutatedSynergy) != 0 {
		t.Fatalf("reversed synergy intersection matched something: %#v", mutatedSynergy)
	}
	if len(correctSynergy) != 1 {
		t.Fatalf("correct synergy implementation = %d notices, want 1", len(correctSynergy))
	}
}

// ---------------------------------------------------------------------------
// P1-2 / P1-3 / P1-5：请求预算（Anti-scope 第 1 条）
// ---------------------------------------------------------------------------

// r116dRecommendationRun 用真实夹具跑一次完整的 /api/gameplay/recommendations，
// 返回响应与本次运行发出的全部 hexdata 上游路径。
func r116dRecommendationRun(t *testing.T, rawQuery string) (gameplayRecommendationsResponse, []string, *localStore) {
	t.Helper()
	recorder := &r116aRecorder{}
	provider, _ := r116aMayhemProvider(t, recorder, r116aFixture(t, "hexdata-hero-157.json"))
	// 处理器走 slug（metadata.Slug）而不是数字 ID，所以名字索引也要填上，
	// 否则 resolveChampionID 会去 loadCatalog 打外部主机。
	provider.championIDs["yasuo"] = 157
	provider.championKeys["yasuo"] = "Yasuo"
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "logs"), 0o700); err != nil {
		t.Fatal(err)
	}
	storage := trackTestStore(t, &localStore{root: root})
	a := &app{champions: provider, storage: storage}
	provider.diag = a.recordDiagnostic
	responseRecorder := httptest.NewRecorder()
	a.handleGameplayRecommendations(responseRecorder, httptest.NewRequest(http.MethodGet, "/api/gameplay/recommendations?"+rawQuery, nil))
	if responseRecorder.Code != http.StatusOK {
		t.Fatalf("status/body = %d/%q", responseRecorder.Code, responseRecorder.Body.String())
	}
	var response gameplayRecommendationsResponse
	if err := json.Unmarshal(responseRecorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v (%s)", err, responseRecorder.Body.String())
	}
	return response, recorder.hexdataPaths(), storage
}

const r116dBaseQuery = "championId=157&queueId=2400&gameMode=KIWI&mapId=12"

// 工单验证判据 3：整个对局页一次刷新，hexdata 相关请求数不因为加了 P1-2/P1-3/P1-5
// 而增加。做法是同一份夹具、同一个 provider 工厂，分别跑「不带 roster 参数」与
// 「带满 roster 参数」两次，逐条比对上游路径的多重集合——必须完全一致。
func TestGameplayRosterInsightsAddZeroHexdataRequests(t *testing.T) {
	withoutRoster := r116dBaseQuery + "&phase=InProgress"
	withRoster := r116dBaseQuery + "&phase=InProgress" +
		"&allyChampionIds=157,4,12,412,222&enemyChampionIds=200,90,7,8,9"

	baselineResponse, baselinePaths, _ := r116dRecommendationRun(t, withoutRoster)
	rosterResponse, rosterPaths, _ := r116dRecommendationRun(t, withRoster)

	if len(baselinePaths) == 0 {
		t.Fatalf("baseline made no hexdata requests at all, the comparison would be vacuous: %#v", baselinePaths)
	}
	sort.Strings(baselinePaths)
	sort.Strings(rosterPaths)
	if !reflect.DeepEqual(baselinePaths, rosterPaths) {
		t.Fatalf("hexdata request multiset changed:\n baseline = %#v\n with R116-D roster params = %#v", baselinePaths, rosterPaths)
	}
	// 逐个端点点名，避免「两边都退化成 0 条」这种假绿。
	for _, path := range []string{hexdataMetaPath, "/api/hexdata/heroes/157", hexdataPostmatchPath} {
		baseline := countStrings(baselinePaths, path)
		roster := countStrings(rosterPaths, path)
		if baseline < 1 {
			t.Fatalf("%s was never requested in the baseline (%d), fixture wiring is broken", path, baseline)
		}
		if roster != baseline {
			t.Fatalf("%s requests = %d with roster params, want %d (unchanged)", path, roster, baseline)
		}
	}
	// 最关键的一条：hero-json 必须恰好 1 次。10 个英雄各拉一次会让最坏排队
	// 11 秒 = ChampSelect 3 秒轮询周期的 3.67 倍，而 recordLoadFailure 对取消
	// 直接 return，熔断计数永远是 0（评审 4.2/4.4）。
	if got := countStrings(rosterPaths, "/api/hexdata/heroes/157"); got != 1 {
		t.Fatalf("hero-json requests = %d, want exactly 1 (%#v)", got, rosterPaths)
	}
	for _, path := range rosterPaths {
		if strings.HasPrefix(path, "/api/hexdata/heroes/") && path != "/api/hexdata/heroes/157" {
			t.Fatalf("a per-entity hero-json request leaked into the roster path: %s (%#v)", path, rosterPaths)
		}
	}

	// 同一次运行里功能确实生效了（否则「请求数不变」只是因为代码没跑）。
	if len(rosterResponse.Recommendations.MatchupNotices) != 2 {
		t.Fatalf("matchup notices = %#v, want 2 (200 + 90 from weakAgainst)", rosterResponse.Recommendations.MatchupNotices)
	}
	if len(rosterResponse.Recommendations.SynergyNotices) != 1 {
		t.Fatalf("synergy notices = %#v, want 1 (teammate 4)", rosterResponse.Recommendations.SynergyNotices)
	}
	if len(baselineResponse.Recommendations.MatchupNotices) != 0 || len(baselineResponse.Recommendations.SynergyNotices) != 0 {
		t.Fatalf("notices appeared without roster params: %#v / %#v",
			baselineResponse.Recommendations.MatchupNotices, baselineResponse.Recommendations.SynergyNotices)
	}
}

// Anti-scope 第 4 条：ChampSelect 阶段一律不展示「对面 5 人」相关的克制。
// 依据是 docs/r116-probe-findings.md §4.4 的 their_team_length 判据仍是「待填」，
// 探测未证实 → 按未证实处理。协同（只需要我方）不受影响，照常展示。
func TestGameplayRosterInsightsSuppressEnemiesDuringChampSelect(t *testing.T) {
	_, paths, storage := r116dRecommendationRun(t, r116dBaseQuery+"&phase=ChampSelect"+
		"&allyChampionIds=157,4,12,412,222&enemyChampionIds=200,90,7,8,9")
	if got := countStrings(paths, "/api/hexdata/heroes/157"); got != 1 {
		t.Fatalf("hero-json requests = %d, want 1", got)
	}
	logText := r116dDiagnosticText(t, storage)
	event := r116dLastDiagnosticEvent(t, logText, "gameplay_roster_insights")
	if event["matchup_phase_allowed"] != false {
		t.Fatalf("matchup_phase_allowed = %#v, want false during ChampSelect", event["matchup_phase_allowed"])
	}
	// 前端确实传了 5 个敌方 ID，是后端主动丢掉的——两个计数必须同时看，
	// 否则「前端没传」和「后端拦下」在日志里长得一样。
	if event["enemy_count_before_phase_guard"] != float64(5) || event["enemy_count"] != float64(0) {
		t.Fatalf("phase guard counters = before:%#v after:%#v, want 5/0",
			event["enemy_count_before_phase_guard"], event["enemy_count"])
	}
	if event["matchup_notices"] != float64(0) {
		t.Fatalf("matchup_notices = %#v, want 0 during ChampSelect", event["matchup_notices"])
	}
	if event["synergy_notices"] != float64(1) {
		t.Fatalf("synergy_notices = %#v, want 1 (P1-2 only needs our own team)", event["synergy_notices"])
	}
	if event["upstream_requests_added"] != float64(0) {
		t.Fatalf("upstream_requests_added = %#v, want 0", event["upstream_requests_added"])
	}
}

// InProgress 阶段 10 人可得，克制正常展示（工单 P1-2/P1-3 实现要求第 2 条）。
func TestGameplayRosterInsightsAllowEnemiesInProgress(t *testing.T) {
	response, _, storage := r116dRecommendationRun(t, r116dBaseQuery+"&phase=InProgress"+
		"&allyChampionIds=157,4,12,412,222&enemyChampionIds=200,90,7,8,9")
	directions := map[string]int{}
	for _, notice := range response.Recommendations.MatchupNotices {
		directions[notice.Direction]++
	}
	if directions["weak"] != 2 {
		t.Fatalf("weak notices = %#v, want 2", response.Recommendations.MatchupNotices)
	}
	event := r116dLastDiagnosticEvent(t, r116dDiagnosticText(t, storage), "gameplay_roster_insights")
	if event["matchup_phase_allowed"] != true {
		t.Fatalf("matchup_phase_allowed = %#v, want true during InProgress", event["matchup_phase_allowed"])
	}
	if event["phase"] != "InProgress" {
		t.Fatalf("phase = %#v, want InProgress", event["phase"])
	}
}

// 阶段白名单本身也要钉住：Reconnect 与 InProgress 同等对待（10 人同样可得），
// 其余阶段（含 ChampSelect / GameStart / 空值）一律不消费敌方阵容。
func TestGameplayRosterMatchupPhaseWhitelist(t *testing.T) {
	allowed := map[string]bool{}
	for _, phase := range gameplayRosterMatchupPhases {
		allowed[strings.ToLower(phase)] = true
	}
	for _, phase := range []string{"inprogress", "reconnect"} {
		if !allowed[phase] {
			t.Fatalf("%s must be allowed to consume the enemy roster", phase)
		}
	}
	for _, phase := range []string{"champselect", "gamestart", "", "none"} {
		if allowed[phase] {
			t.Fatalf("%s must NOT be allowed to consume the enemy roster (Anti-scope 4)", phase)
		}
	}
}

// 非海斗模式下 hero-json / postmatch 根本不在缓存里，只读缓存的取数入口必须
// 直接放弃：既不生成任何提示，也不发出任何上游请求。
func TestGameplayRosterInsightsStaySilentOutsideMayhem(t *testing.T) {
	recorder := &r116aRecorder{}
	provider := newChampionProvider()
	provider.cache = newChampionDataCache(nil)
	provider.hexdata = newHexdataClient(provider, nil)
	provider.client = &http.Client{Transport: championRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		recorder.add(request)
		return &http.Response{StatusCode: http.StatusNotFound, Header: make(http.Header), Body: http.NoBody, Request: request}, nil
	})}
	bundle := &gameplayRecommendationBundle{}
	query := r116dQueryValues("phase=InProgress&allyChampionIds=157,4,12,412,222&enemyChampionIds=200,90,7,8,9")
	a := &app{champions: provider}
	for _, mode := range []string{"ranked", "aram", "arena", "unsupported", ""} {
		a.gameplayApplyRosterInsights(context.Background(), bundle, query, provider, 157, mode)
	}
	if len(bundle.MatchupNotices) != 0 || len(bundle.SynergyNotices) != 0 || bundle.TeamPortrait != nil {
		t.Fatalf("non-mayhem modes produced roster insights: %#v", bundle)
	}
	if paths := recorder.hexdataPaths(); len(paths) != 0 {
		t.Fatalf("non-mayhem modes issued hexdata requests: %#v", paths)
	}
}

// 只读缓存的结构性保证：缓存里没有 hero-json 时，取数入口必须返回 false 且
// **绝不回源**。这是「新增请求数恒为 0」不依赖调用顺序的那一半。
func TestGameplayPeekCachedHexdataPageNeverHitsUpstream(t *testing.T) {
	recorder := &r116aRecorder{}
	provider, _ := r116aMayhemProvider(t, recorder, r116aFixture(t, "hexdata-hero-157.json"))
	// 缓存是空的：hero-json 与 postmatch 都没被拉过。
	if _, ok := gameplayHeroHexdataTables(context.Background(), provider, 157); ok {
		t.Fatal("hero-json was reported as cached on an empty cache")
	}
	if _, _, ok := gameplayPostmatchTable(context.Background(), provider); ok {
		t.Fatal("postmatch was reported as cached on an empty cache")
	}
	if paths := recorder.hexdataPaths(); len(paths) != 0 {
		t.Fatalf("the cache-only peek reached upstream: %#v", paths)
	}

	// 填进缓存之后再读：必须命中，且依然不发请求。
	if _, err := provider.loadMayhemDetail(context.Background(), "157"); err != nil {
		t.Fatalf("loadMayhemDetail: %v", err)
	}
	requestsAfterWarm := len(recorder.hexdataPaths())
	if requestsAfterWarm == 0 {
		t.Fatal("warming the cache issued no requests, the fixture is not exercising the loader")
	}
	detail, ok := gameplayHeroHexdataTables(context.Background(), provider, 157)
	if !ok || len(detail.WeakAgainst) != 7 {
		t.Fatalf("cached hero-json was not reused: ok=%v weak=%d", ok, len(detail.WeakAgainst))
	}
	if _, _, ok := gameplayPostmatchTable(context.Background(), provider); !ok {
		t.Fatal("cached postmatch was not reused")
	}
	if got := len(recorder.hexdataPaths()); got != requestsAfterWarm {
		t.Fatalf("cache-only peek issued %d extra requests", got-requestsAfterWarm)
	}
}

// ---------------------------------------------------------------------------
// P1-4 阶段一：playerlist 的 items 解析
// ---------------------------------------------------------------------------

// r116dPlayerList 拼一份海斗形状的 playerlist：10 人、position 恒为 "OTHER"
// （docs/r116-probe-findings.md §1.5 的斗魂基线是 position_values:{"OTHER":18}）、
// 无 subteam 字段，items 用斗魂实测的 itemID/slot/count/canUse/consumable 五键。
func r116dPlayerList(t *testing.T, entries ...map[string]any) []byte {
	t.Helper()
	data, err := json.Marshal(entries)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func r116dPlayerEntry(name string, items any) map[string]any {
	entry := map[string]any{
		"championName": "Yasuo", "rawChampionName": "Yasuo", "skinName": "default", "rawSkinName": "",
		"riotId": name, "riotIdGameName": name, "riotIdTagLine": "CN1", "summonerName": name,
		"isBot": false, "isDead": false, "level": 11, "position": "OTHER", "respawnTimer": 0,
		"skinID": 0, "team": "ORDER", "runes": map[string]any{}, "scores": map[string]any{},
		"summonerSpells": map[string]any{},
	}
	if items != nil {
		entry["items"] = items
	}
	return entry
}

func r116dItemElement(itemID, slot, count int, canUse, consumable bool) map[string]any {
	return map[string]any{"itemID": itemID, "slot": slot, "count": count, "canUse": canUse, "consumable": consumable}
}

// 工单 P1-4 阶段一第 1 条：读 items、存进 ItemsByIdentity，身份匹配复用
// liveClientEntryIdentityKeys（与 PositionByIdentity 同一套）。
func TestParseLiveClientPlayerListReadsItems(t *testing.T) {
	raw := r116dPlayerList(t,
		r116dPlayerEntry("alpha", []any{
			r116dItemElement(3153, 0, 1, false, false),
			r116dItemElement(6333, 1, 1, true, false),
			r116dItemElement(2003, 6, 2, false, true),
		}),
		r116dPlayerEntry("beta", []any{}),
		r116dPlayerEntry("gamma", nil),
	)
	snapshot, shape, err := parseLiveClientPlayerList(raw, 3)
	if err != nil {
		t.Fatal(err)
	}
	if shape.PlayerCount != 3 {
		t.Fatalf("player_count = %d, want 3", shape.PlayerCount)
	}
	// 身份键是 normalizeLiveClientPlayerName 的小写形式，"alpha#cn1" 与 "alpha" 都在。
	items := snapshot.ItemsByIdentity["alpha"]
	if len(items) != 3 {
		t.Fatalf("alpha items = %#v, want 3", items)
	}
	if items[0].ItemID != 3153 || items[0].Slot != 0 || items[0].Count != 1 || items[0].CanUse || items[0].Consumable {
		t.Fatalf("first item parsed wrong: %#v", items[0])
	}
	if items[1].ItemID != 6333 || !items[1].CanUse {
		t.Fatalf("second item parsed wrong: %#v", items[1])
	}
	if items[2].ItemID != 2003 || !items[2].Consumable || items[2].Count != 2 {
		t.Fatalf("consumable item parsed wrong: %#v", items[2])
	}
	// 顺序必须原样保留：重排就是编造上游没给的顺序。
	if items[0].ItemID != 3153 || items[1].ItemID != 6333 || items[2].ItemID != 2003 {
		t.Fatalf("item order was not preserved: %#v", items)
	}
	// "items": [] 与「没有 items 键」必须能区分：前者写空切片，后者不写键。
	betaItems, betaPresent := snapshot.ItemsByIdentity["beta"]
	if !betaPresent || len(betaItems) != 0 {
		t.Fatalf("empty items array was not distinguished from a missing key: present=%v %#v", betaPresent, betaItems)
	}
	if _, gammaPresent := snapshot.ItemsByIdentity["gamma"]; gammaPresent {
		t.Fatal("a player without the items key got an entry")
	}
	// 元素键名并集是判据二的证据之一，必须原样落进快照。
	if got := strings.Join(snapshot.ItemElementKeys, ","); got != "canUse,consumable,count,itemID,slot" {
		t.Fatalf("item element keys = %q, want the five Arena-baseline keys", got)
	}
	if snapshot.ItemElementsSeen != 3 || snapshot.ItemElementsSkipped != 0 {
		t.Fatalf("item element counters = seen:%d skipped:%d, want 3/0", snapshot.ItemElementsSeen, snapshot.ItemElementsSkipped)
	}
}

// 工单 P1-4 阶段一：解析必须容错。items 缺失 / 不是数组 / 元素不是对象 /
// itemID 不是数字，全部安全跳过并计入诊断，绝不 panic。
// 海斗的真实形状未经实测，所以键名大小写要宽松匹配。
func TestParseLiveClientItemsToleratesMalformedShapes(t *testing.T) {
	tests := []struct {
		name        string
		entry       map[string]any
		wantPresent bool
		wantItems   int
		wantSeen    int
		wantSkipped int
	}{
		{name: "no items key", entry: map[string]any{"riotId": "a"}, wantPresent: false},
		{name: "items is null", entry: map[string]any{"riotId": "a", "items": nil}, wantPresent: true},
		{name: "items is an object", entry: map[string]any{"riotId": "a", "items": map[string]any{"0": 3153}}, wantPresent: true},
		{name: "items is a string", entry: map[string]any{"riotId": "a", "items": "3153"}, wantPresent: true},
		{
			name:        "element is not an object",
			entry:       map[string]any{"riotId": "a", "items": []any{3153, "6333", []any{1}}},
			wantPresent: true, wantSeen: 3, wantSkipped: 3,
		},
		{
			name:        "itemID is not numeric",
			entry:       map[string]any{"riotId": "a", "items": []any{map[string]any{"itemID": "not-a-number", "slot": 0}}},
			wantPresent: true, wantSeen: 1, wantSkipped: 1,
		},
		{
			name:        "itemID missing",
			entry:       map[string]any{"riotId": "a", "items": []any{map[string]any{"slot": 1, "count": 1}}},
			wantPresent: true, wantSeen: 1, wantSkipped: 1,
		},
		{
			name:        "itemID is zero",
			entry:       map[string]any{"riotId": "a", "items": []any{map[string]any{"itemID": 0, "slot": 0}}},
			wantPresent: true, wantSeen: 1, wantSkipped: 1,
		},
		{
			name:        "itemID is fractional",
			entry:       map[string]any{"riotId": "a", "items": []any{map[string]any{"itemID": 3153.5}}},
			wantPresent: true, wantSeen: 1, wantSkipped: 1,
		},
		{
			// 上游在别处确实把 ID 写成字符串（hexdata 的 opponentChampionId 就是一例），
			// 数字字符串要认。
			name:        "itemID is a numeric string",
			entry:       map[string]any{"riotId": "a", "items": []any{map[string]any{"itemID": "3153", "slot": "2", "count": "1", "canUse": "true", "consumable": "false"}}},
			wantPresent: true, wantItems: 1, wantSeen: 1,
		},
		{
			// 键名大小写宽松匹配：itemId / itemid 都要认。
			name:        "lowercase key spelling",
			entry:       map[string]any{"riotId": "a", "items": []any{map[string]any{"itemid": 6333, "SLOT": 1}}},
			wantPresent: true, wantItems: 1, wantSeen: 1,
		},
		{
			name:        "mixed valid and invalid elements",
			entry:       map[string]any{"riotId": "a", "items": []any{r116dItemElement(3153, 0, 1, false, false), "junk", map[string]any{"itemID": -1}}},
			wantPresent: true, wantItems: 1, wantSeen: 3, wantSkipped: 2,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			items, _, seen, skipped, present := parseLiveClientItems(test.entry)
			if present != test.wantPresent {
				t.Fatalf("present = %v, want %v", present, test.wantPresent)
			}
			if len(items) != test.wantItems {
				t.Fatalf("items = %#v, want %d", items, test.wantItems)
			}
			if seen != test.wantSeen || skipped != test.wantSkipped {
				t.Fatalf("counters = seen:%d skipped:%d, want %d/%d", seen, skipped, test.wantSeen, test.wantSkipped)
			}
		})
	}
	// 整份 playerlist 都是垃圾也不能 panic，且既有的 position/分组行为不变。
	snapshot, shape, err := parseLiveClientPlayerList(r116dPlayerList(t,
		map[string]any{"riotId": "a", "items": "junk", "position": "OTHER"},
		map[string]any{"riotId": "b", "items": []any{nil, 1, "x"}, "position": "OTHER"},
	), 3)
	if err != nil {
		t.Fatalf("malformed playerlist returned an error: %v", err)
	}
	if shape.PlayerCount != 2 || snapshot.ItemElementsSkipped != 3 {
		t.Fatalf("shape/counters = %d/%d, want 2/3", shape.PlayerCount, snapshot.ItemElementsSkipped)
	}
}

// 易漏点（执行契约第 11 条）：新增 ItemsByIdentity 之后 cloneLiveClientSnapshot
// 必须深拷贝每个 slice，否则快照跨 goroutine 传递时会共享底层数组 —— 既丢数据
// 也触发 race。
func TestCloneLiveClientSnapshotDeepCopiesItems(t *testing.T) {
	original := liveClientSnapshot{
		OrderedIdentities:  [][]string{{"alpha"}},
		PositionByIdentity: map[string]string{"alpha": "MIDDLE"},
		ItemsByIdentity: map[string][]liveClientItem{
			"alpha": {{ItemID: 3153, Slot: 0, Count: 1}, {ItemID: 6333, Slot: 1, Count: 1}},
			"beta":  {{ItemID: 3031, Slot: 2, Count: 1}},
		},
		ItemElementKeys:     []string{"count", "itemID", "slot"},
		ItemElementsSeen:    3,
		ItemElementsSkipped: 1,
	}
	cloned := cloneLiveClientSnapshot(original)
	if !reflect.DeepEqual(cloned.ItemsByIdentity, original.ItemsByIdentity) {
		t.Fatalf("clone lost items: %#v", cloned.ItemsByIdentity)
	}
	if !reflect.DeepEqual(cloned.ItemElementKeys, original.ItemElementKeys) {
		t.Fatalf("clone lost element keys: %#v", cloned.ItemElementKeys)
	}
	if cloned.ItemElementsSeen != 3 || cloned.ItemElementsSkipped != 1 {
		t.Fatalf("clone lost counters: %d/%d", cloned.ItemElementsSeen, cloned.ItemElementsSkipped)
	}
	// 改克隆不能影响原件（浅拷贝会在这里露馅）。
	cloned.ItemsByIdentity["alpha"][0].ItemID = 9999
	cloned.ItemsByIdentity["beta"] = append(cloned.ItemsByIdentity["beta"], liveClientItem{ItemID: 123430})
	cloned.ItemElementKeys[0] = "mutated"
	if original.ItemsByIdentity["alpha"][0].ItemID != 3153 {
		t.Fatal("clone shares the item backing array with the original")
	}
	if len(original.ItemsByIdentity["beta"]) != 1 {
		t.Fatal("clone shares the beta slice with the original")
	}
	if original.ItemElementKeys[0] != "count" {
		t.Fatal("clone shares the element-key slice with the original")
	}

	// 并发读同一份快照的多个克隆（-race 下这条会真的跑并发）。
	var wait sync.WaitGroup
	for index := 0; index < 8; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			local := cloneLiveClientSnapshot(original)
			local.ItemsByIdentity["alpha"][0].Slot = index
			if len(local.ItemsByIdentity["alpha"]) != 2 {
				t.Errorf("concurrent clone lost items")
			}
		}(index)
	}
	wait.Wait()
	if original.ItemsByIdentity["alpha"][0].ItemID != 3153 {
		t.Fatal("concurrent cloning corrupted the shared snapshot")
	}
}

// items 查询与 position 查询同一套身份优先级（riotId 先于 summonerName），
// 且必须能区分「键存在但为空」与「键不存在」。
func TestLiveClientItemsForIdentitiesDistinguishesEmptyFromMissing(t *testing.T) {
	snapshot := liveClientSnapshot{ItemsByIdentity: map[string][]liveClientItem{
		"alpha":     {{ItemID: 3153}},
		"beta":      {},
		"alpha#cn1": {{ItemID: 3153}},
	}}
	items, source := liveClientItemsForIdentities(snapshot, []string{"alpha"}, []string{"alpha#CN1"})
	if len(items) != 1 || items[0].ItemID != 3153 || source != "riotId" {
		t.Fatalf("riotId lookup = %#v/%q, want one item via riotId", items, source)
	}
	empty, emptySource := liveClientItemsForIdentities(snapshot, []string{"beta"}, nil)
	if len(empty) != 0 || emptySource != "summonerName" {
		t.Fatalf("empty items = %#v/%q, want matched-but-empty via summonerName", empty, emptySource)
	}
	if missing, missingSource := liveClientItemsForIdentities(snapshot, []string{"gamma"}, nil); missing != nil || missingSource != "" {
		t.Fatalf("unknown identity = %#v/%q, want no match", missing, missingSource)
	}
	// 快照里一个 items 都没有（海斗没给这个字段）→ 直接不匹配，不做无谓遍历。
	if items, source := liveClientItemsForIdentities(liveClientSnapshot{}, []string{"alpha"}, nil); items != nil || source != "" {
		t.Fatalf("empty snapshot matched: %#v/%q", items, source)
	}
}

// 工单 P1-4 阶段一第 2 条：诊断事件 live_client_items_parsed 记录本人识别到的
// items 长度与 itemID 集合，且明确标注 ui_visible=false（生产 UI 不展示任何内容）。
func TestRecordLiveClientItemsParsedEmitsBoundedDiagnostic(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "logs"), 0o700); err != nil {
		t.Fatal(err)
	}
	storage := trackTestStore(t, &localStore{root: root})
	a := &app{storage: storage}
	snapshot := liveClientSnapshot{
		OrderedIdentities:   [][]string{{"alpha"}, {"beta"}},
		ItemElementKeys:     []string{"canUse", "consumable", "count", "itemID", "slot"},
		ItemElementsSeen:    5,
		ItemElementsSkipped: 1,
		ItemsByIdentity:     map[string][]liveClientItem{"alpha": {{ItemID: 6333}, {ItemID: 3153}}},
	}
	items := []liveClientItem{{ItemID: 6333, Slot: 1, Count: 1}, {ItemID: 3153, Slot: 0, Count: 1}}
	a.recordLiveClientItemsParsed(9001, "InProgress", snapshot, true, "riotId", items)
	// 同一套装备再记一次：去重键相同，不应产生第二条。
	a.recordLiveClientItemsParsed(9001, "InProgress", snapshot, true, "riotId", items)
	// 出装变了：必须产生新的一条（这正是真机要看的「解析是否随出装推进」）。
	a.recordLiveClientItemsParsed(9001, "InProgress", snapshot, true, "riotId", append(append([]liveClientItem(nil), items...), liveClientItem{ItemID: 123430, Slot: 2, Count: 1}))

	events := r116dDiagnosticEvents(t, storage, "live_client_items_parsed")
	if len(events) != 2 {
		t.Fatalf("events = %d, want 2 (deduplicated per item set)", len(events))
	}
	first := events[0]
	if first["game_id"] != float64(9001) || first["phase"] != "InProgress" {
		t.Fatalf("event identity = %#v", first)
	}
	if first["ui_visible"] != false {
		t.Fatalf("ui_visible = %#v, want false (stage one is diagnostics only)", first["ui_visible"])
	}
	if first["self_matched"] != true || first["identity_source"] != "riotId" {
		t.Fatalf("self match = %#v/%#v", first["self_matched"], first["identity_source"])
	}
	if first["self_item_count"] != float64(2) {
		t.Fatalf("self_item_count = %#v, want 2", first["self_item_count"])
	}
	ids, ok := first["self_item_ids"].([]any)
	if !ok || len(ids) != 2 || ids[0] != float64(3153) || ids[1] != float64(6333) {
		t.Fatalf("self_item_ids = %#v, want sorted [3153 6333]", first["self_item_ids"])
	}
	keys, ok := first["item_element_keys"].([]any)
	if !ok || len(keys) != 5 || keys[3] != "itemID" {
		t.Fatalf("item_element_keys = %#v, want the five observed keys", first["item_element_keys"])
	}
	if first["item_elements_seen"] != float64(5) || first["item_elements_skipped"] != float64(1) {
		t.Fatalf("element counters = %#v/%#v", first["item_elements_seen"], first["item_elements_skipped"])
	}
	if first["identities_with_items"] != float64(1) || first["player_count"] != float64(2) {
		t.Fatalf("coverage counters = %#v/%#v", first["identities_with_items"], first["player_count"])
	}
	second := events[1]
	if second["self_item_count"] != float64(3) {
		t.Fatalf("second event item count = %#v, want 3", second["self_item_count"])
	}

	// 本人没匹配上也要记：这正是「items 键在、身份匹配不上」与「items 键根本没有」
	// 的分界，少了它真机日志无法判读。
	a.recordLiveClientItemsParsed(9002, "InProgress", snapshot, false, "", nil)
	unmatched := r116dDiagnosticEvents(t, storage, "live_client_items_parsed")
	last := unmatched[len(unmatched)-1]
	if last["self_matched"] != false || last["self_item_count"] != float64(0) {
		t.Fatalf("unmatched event = %#v", last)
	}
}

// ---------------------------------------------------------------------------
// P1-4 阶段二：纯函数（已实现、未接线）
// ---------------------------------------------------------------------------

func TestGameplayOwnedTerminalItemIDsFiltersConsumablesAndJunk(t *testing.T) {
	items := []liveClientItem{
		{ItemID: 3153, Count: 1},
		{ItemID: 2003, Count: 2, Consumable: true}, // 药水
		{ItemID: 3364, Count: 1, Consumable: true}, // 眼位
		{ItemID: 0},  // 空槽
		{ItemID: -1}, // 垃圾
		{ItemID: 6333, Count: 1},
		{ItemID: 3153, Count: 1}, // 重复
	}
	got := gameplayOwnedTerminalItemIDs(items)
	want := []int{3153, 6333}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("owned ids = %#v, want %#v", got, want)
	}
	if ids := gameplayOwnedTerminalItemIDs(nil); ids != nil {
		t.Fatalf("nil items = %#v, want nil", ids)
	}
	if ids := gameplayOwnedTerminalItemIDs([]liveClientItem{{ItemID: 2003, Consumable: true}}); ids != nil {
		t.Fatalf("consumables only = %#v, want nil", ids)
	}
}

// 工单 P1-4 阶段二验证判据 2：构造「本人已出 A+B 两件，trio 表里有一条 A:B:C」
// 的 fixture，验证推荐出现 C 且带正确的 winRate/games。用的是真实夹具里的
// terminalItemTrios[0] = 3153:6333:123430（破败王者之刃 + 死亡之舞 + 毁坏仪式）。
func TestGameplayNextItemSuggestionMatchesTrioPrefix(t *testing.T) {
	detail := r116dHeroDetail(t)
	if len(detail.TerminalItemTrios) == 0 {
		t.Fatal("fixture has no terminalItemTrios")
	}
	first := detail.TerminalItemTrios[0]
	if first.TrioKey != "3153:6333:123430" || len(first.ItemIDs) != 3 {
		t.Fatalf("fixture anchor moved: %#v", first)
	}
	suggestion, ok := gameplayNextItemSuggestionFromTrios([]int{3153, 6333}, detail.TerminalItemTrios)
	if !ok {
		t.Fatal("prefix match failed on a trio that is literally the first row")
	}
	if suggestion.ItemID != 123430 {
		t.Fatalf("suggested item = %d, want 123430", suggestion.ItemID)
	}
	if suggestion.WinRate != first.WinRate || suggestion.Games != first.Games {
		t.Fatalf("winRate/games = %v/%d, want upstream %v/%d", suggestion.WinRate, suggestion.Games, first.WinRate, first.Games)
	}
	if suggestion.TrioKey != first.TrioKey {
		t.Fatalf("trioKey = %q, want %q", suggestion.TrioKey, first.TrioKey)
	}
	if !strings.Contains(suggestion.ItemName, "毁坏仪式") {
		t.Fatalf("item name = %q, want the upstream Chinese name", suggestion.ItemName)
	}
	// 上游顺序就是样本量优先级：已出 3031+3153 时应命中第 2 条 3031:3153:123430，
	// 而不是自己另排一套。
	second, ok := gameplayNextItemSuggestionFromTrios([]int{3031, 3153}, detail.TerminalItemTrios)
	if !ok || second.TrioKey != "3031:3153:123430" {
		t.Fatalf("upstream order was not respected: ok=%v %#v", ok, second)
	}
}

// 降级规则（工单 P1-4 阶段二第 5 条）：items 解析失败、结构不符、或没有任何
// trio 前缀匹配时返回 ok=false，调用方整行不渲染。绝不返回「大概是这件」。
func TestGameplayNextItemSuggestionDegradesToNothing(t *testing.T) {
	detail := r116dHeroDetail(t)
	trio := []hexdataTrioRow{{TrioKey: "1:2:3", ItemIDs: []int{1, 2, 3}, ItemNames: []string{"甲", "乙", "丙"}, WinRate: 0.6, Games: 1000}}
	tests := []struct {
		name  string
		owned []int
		trios []hexdataTrioRow
	}{
		{name: "no items at all", owned: nil, trios: trio},
		{name: "only one item", owned: []int{1}, trios: trio},
		{name: "two items but no trio", owned: []int{1, 2}, trios: nil},
		{name: "prefix does not match", owned: []int{1, 3}, trios: trio},
		{name: "already owns all three", owned: []int{1, 2, 3}, trios: trio},
		{name: "trio has wrong length", owned: []int{1, 2}, trios: []hexdataTrioRow{{TrioKey: "1:2", ItemIDs: []int{1, 2}}}},
		{name: "trio has a zero id", owned: []int{1, 2}, trios: []hexdataTrioRow{{TrioKey: "1:2:0", ItemIDs: []int{1, 2, 0}}}},
		{name: "owned ids are all junk", owned: []int{0, -1}, trios: trio},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			suggestion, ok := gameplayNextItemSuggestionFromTrios(test.owned, test.trios)
			if ok || suggestion != (gameplayNextItemSuggestion{}) {
				t.Fatalf("expected no suggestion, got ok=%v %#v", ok, suggestion)
			}
		})
	}
	// 真实夹具上也要验一次「已出两件但不构成任何 trio 前缀」。
	if _, ok := gameplayNextItemSuggestionFromTrios([]int{1001, 1053}, detail.TerminalItemTrios); ok {
		t.Fatal("boots + a component should not match any terminal trio prefix")
	}
}

// 阶段二「已实现但未接线」这条事实本身要有测试钉住：一旦有人把它接到生产路径上
// 而没有先回填 docs/r116-probe-findings.md 的判据，这条就会红，逼他去看注释。
func TestR116DStageTwoStaysUnwired(t *testing.T) {
	source, err := os.ReadFile("gameplay.go")
	if err != nil {
		t.Fatal(err)
	}
	// 注释里刻意点了这两个名字（说明接线条件与接线点），所以先把 // 注释行剔掉，
	// 只数真实代码里的出现次数：定义 1 次、调用 0 次。
	code := r116dStripLineComments(string(source))
	for _, name := range []string{"gameplayNextItemSuggestionFromTrios", "gameplayOwnedTerminalItemIDs"} {
		if got := strings.Count(code, name); got != 1 {
			t.Fatalf("%s appears %d times in gameplay.go code, want exactly 1 (the definition, no caller). "+
				"Wiring P1-4 stage two requires docs/r116-probe-findings.md §3.4/§5.3 to be filled in first.", name, got)
		}
	}
	// bundle 上不许出现「下一件」字段：那是接线时才加的东西。
	bundleType := reflect.TypeOf(gameplayRecommendationBundle{})
	for index := 0; index < bundleType.NumField(); index++ {
		field := bundleType.Field(index)
		if strings.Contains(strings.ToLower(field.Name), "nextitem") {
			t.Fatalf("gameplayRecommendationBundle gained %s before the probe verdict was recorded", field.Name)
		}
	}
	webSource, err := os.ReadFile(filepath.Join("web", "gameplay.js"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(webSource), "nextItem") {
		t.Fatal("gameplay.js renders a next-item row before the probe verdict was recorded")
	}
}

// ---------------------------------------------------------------------------
// P1-5：队伍画像缺口标签
// ---------------------------------------------------------------------------

// 工单验证判据：构造「5 人全是脆皮 AD 刺客」的 fixture → 生成「缺前排」标签。
// 用的是真实 postmatch 数据里的五个刺客：238 劫 / 91 刀锋 / 121 螳螂 /
// 55 卡特 / 84 阿卡丽。
func TestGameplayTeamPortraitFlagsGlassCannonComp(t *testing.T) {
	heroes, present := r116dPostmatch(t)
	averages := gameplayTeamPortraitAveragesFrom(heroes, present)
	ids := []int{238, 91, 121, 55, 84}
	labels := gameplayTeamPortraitLabels(r116dPostmatchRows(t, heroes, ids...), averages)
	keys := r116dLabelKeys(labels)
	if !keys["frontline"] {
		t.Fatalf("an all-assassin comp did not produce 缺前排: %#v (labels %#v)", keys, labels)
	}
	// 五个刺客的控制时长全部低于均值（实测比例 0.107~0.439），所以「缺控制」
	// 同样应该出现；「缺持续输出」不该出现（damageShare 比例 1.042~1.252）。
	if !keys["crowdControl"] {
		t.Fatalf("an all-assassin comp did not produce 缺控制: %#v", keys)
	}
	if keys["sustainedDamage"] {
		t.Fatalf("an all-assassin comp wrongly produced 缺持续输出: %#v", keys)
	}
	for _, label := range labels {
		if strings.TrimSpace(label.Label) == "" {
			t.Fatalf("empty label text in %#v", labels)
		}
	}
}

// 工单验证判据：构造「阵容均衡」的 fixture → **不生成任何标签**（不是生成一个
// 空标签）。58 鳄鱼（承伤 1.44× / 减伤 1.62×）+ 64 盲僧 + 84 阿卡丽（输出占比
// 1.04×）+ 222 金克丝（1.07×）+ 412 锤石（控制 1.61×）。
func TestGameplayTeamPortraitBalancedCompProducesNoLabels(t *testing.T) {
	heroes, present := r116dPostmatch(t)
	averages := gameplayTeamPortraitAveragesFrom(heroes, present)
	labels := gameplayTeamPortraitLabels(r116dPostmatchRows(t, heroes, 58, 64, 84, 222, 412), averages)
	if len(labels) != 0 {
		t.Fatalf("a balanced comp produced labels: %#v", labels)
	}
	if labels != nil {
		t.Fatalf("labels must be nil, not an empty non-nil slice: %#v", labels)
	}
}

// 对抗变异（第三处）：把标签生成改成「永远至少产出一个标签」，验证上面那条
// 「阵容均衡 → 不生成任何标签」的判据确实能抓到这种退化——否则「不生成」和
// 「生成了一个空标签」在测试里长得一样。
func TestGameplayTeamPortraitEmptyLabelDegenerationIsDetected(t *testing.T) {
	heroes, present := r116dPostmatch(t)
	averages := gameplayTeamPortraitAveragesFrom(heroes, present)
	rows := r116dPostmatchRows(t, heroes, 58, 64, 84, 222, 412)

	// mutatedLabels 是「总要产出点什么」的变异体：把三个判据的反向结论无条件
	// 塞一个占位标签进去（等价于前端渲染一个空标签）。
	mutatedLabels := func() []gameplayTeamPortraitLabel {
		labels := gameplayTeamPortraitLabels(rows, averages)
		if len(labels) == 0 {
			labels = append(labels, gameplayTeamPortraitLabel{Key: "frontline", Label: ""})
		}
		return labels
	}
	if got := mutatedLabels(); len(got) != 1 {
		t.Fatalf("mutation did not produce a placeholder label: %#v", got)
	}
	if got := gameplayTeamPortraitLabels(rows, averages); len(got) != 0 {
		t.Fatalf("production implementation produced labels for a balanced comp: %#v", got)
	}
}

// 阈值口径：必须是「相对全英雄均值的比例」，且与 R116-B P1-6 的
// mayhemPerformancePanel 同一个口径（字段没出现过的英雄不进池，不拿 0 顶替）。
func TestGameplayTeamPortraitAveragesMatchDocumentedMeans(t *testing.T) {
	heroes, present := r116dPostmatch(t)
	averages := gameplayTeamPortraitAveragesFrom(heroes, present)
	if averages.HeroCount != 173 {
		t.Fatalf("hero pool = %d, want 173", averages.HeroCount)
	}
	// 执行简报里 2026-09-20 实测的 173 英雄全量均值。这里用 1e-2 的宽容度：
	// 夹具是真实响应，均值必须对得上，但不要把小数点后十几位钉死。
	want := map[string]float64{
		"avgDamageTaken": 46151.55, "avgCcTime": 34.473, "avgDmgMitigated": 48924.14,
		"avgHealShield": 1595.04, "damageShare": 0.19214,
	}
	for key, expected := range want {
		got, ok := averages.Values[key]
		if !ok {
			t.Fatalf("averages are missing %q", key)
		}
		if absFloat(got-expected) > 1e-2*maxFloat(1, absFloat(expected)) {
			t.Fatalf("average[%s] = %v, want ~%v", key, got, expected)
		}
	}
	// 独立复算一遍算术均值，防止实现偷偷换了口径（例如改成中位数或加了权重）。
	for _, field := range gameplayTeamPortraitFields {
		var sum float64
		var count int
		for id, row := range heroes {
			if seen := present[id]; len(seen) > 0 && !seen[field.Key] {
				continue
			}
			sum += field.Value(row)
			count++
		}
		if count != 173 {
			t.Fatalf("field %s covers %d heroes, want 173 (fixture has no missing fields)", field.Key, count)
		}
		if absFloat(sum/float64(count)-averages.Values[field.Key]) > 1e-9 {
			t.Fatalf("field %s average is not the plain arithmetic mean", field.Key)
		}
	}
	// 缺失字段的英雄必须被排除在均值池外，不能拿 0 把均值拉低。
	partial := map[int]hexdataPostmatchRow{
		1: {AvgDamageTaken: 100, AvgCcTime: 10, AvgDmgMitigated: 100, AvgHealShield: 10, DamageShare: 0.2},
		2: {AvgDamageTaken: 300, AvgCcTime: 30, AvgDmgMitigated: 300, AvgHealShield: 30, DamageShare: 0.4},
	}
	partialPresent := map[int]map[string]bool{
		1: {"avgDamageTaken": true, "avgCcTime": true, "avgDmgMitigated": true, "avgHealShield": true, "damageShare": true},
		2: {"avgCcTime": true}, // 只有控制时长出现过，其余四项不许进池
	}
	partialAverages := gameplayTeamPortraitAveragesFrom(partial, partialPresent)
	if partialAverages.Values["avgDamageTaken"] != 100 {
		t.Fatalf("a hero missing avgDamageTaken was averaged as 0: %v", partialAverages.Values["avgDamageTaken"])
	}
	if partialAverages.Values["avgCcTime"] != 20 {
		t.Fatalf("avgCcTime average = %v, want 20", partialAverages.Values["avgCcTime"])
	}
	if _, ok := partialAverages.Values["damageShare"]; ok {
		// damageShare 只有英雄 1 出现过 → 均值就是它自己，不是 (0.2+0)/2。
		if partialAverages.Values["damageShare"] != 0.2 {
			t.Fatalf("damageShare average = %v, want 0.2", partialAverages.Values["damageShare"])
		}
	}
	if got := gameplayTeamPortraitAveragesFrom(nil, nil); got.HeroCount != 0 || len(got.Values) != 0 {
		t.Fatalf("empty pool produced %#v", got)
	}
}

// 证据不足明确降级：均值缺失或为 0 时整套判据失效，一个标签都不生成。
func TestGameplayTeamPortraitLabelsRequireUsableAverages(t *testing.T) {
	heroes, _ := r116dPostmatch(t)
	rows := r116dPostmatchRows(t, heroes, 238, 91, 121, 55, 84)
	for _, key := range []string{"avgDamageTaken", "avgDmgMitigated", "avgCcTime", "damageShare"} {
		broken := gameplayTeamPortraitAverages{Values: map[string]float64{
			"avgDamageTaken": 46151, "avgCcTime": 34, "avgDmgMitigated": 48924, "damageShare": 0.19,
		}}
		delete(broken.Values, key)
		if labels := gameplayTeamPortraitLabels(rows, broken); len(labels) != 0 {
			t.Fatalf("missing average %q still produced labels %#v", key, labels)
		}
		zeroed := gameplayTeamPortraitAverages{Values: map[string]float64{
			"avgDamageTaken": 46151, "avgCcTime": 34, "avgDmgMitigated": 48924, "damageShare": 0.19,
		}}
		zeroed.Values[key] = 0
		if labels := gameplayTeamPortraitLabels(rows, zeroed); len(labels) != 0 {
			t.Fatalf("zero average %q still produced labels %#v", key, labels)
		}
	}
	if labels := gameplayTeamPortraitLabels(nil, gameplayTeamPortraitAveragesFrom(heroes, nil)); labels != nil {
		t.Fatalf("empty roster produced labels %#v", labels)
	}
}

// 覆盖率门槛：我方 5 人里查不到 4 个 postmatch 就整块不下发。
func TestGameplayTeamPortraitFromRequiresEnoughResolvedHeroes(t *testing.T) {
	heroes, present := r116dPostmatch(t)
	full := gameplayTeamPortraitFrom([]int{238, 91, 121, 55, 84}, heroes, present)
	if full == nil {
		t.Fatal("an all-assassin comp produced no portrait")
	}
	if full.RosterSize != 5 || full.ResolvedHeroes != 5 || full.HeroPoolSize != 173 {
		t.Fatalf("portrait counters = %#v", full)
	}
	if !strings.Contains(full.MeasurementTechnique, "本地约定的启发式") || !strings.Contains(full.MeasurementTechnique, "不是上游官方口径") {
		t.Fatalf("measurement technique must declare the heuristic is local: %q", full.MeasurementTechnique)
	}
	if len(full.Labels) == 0 {
		t.Fatal("portrait has no labels")
	}
	// 4 个能解析（含一个不存在的 ID）→ 仍然出结论。
	if got := gameplayTeamPortraitFrom([]int{238, 91, 121, 55, 999999}, heroes, present); got == nil || got.ResolvedHeroes != 4 {
		t.Fatalf("four resolved heroes should still produce a portrait: %#v", got)
	}
	// 3 个 → 整块不下发。
	if got := gameplayTeamPortraitFrom([]int{238, 91, 121, 999998, 999999}, heroes, present); got != nil {
		t.Fatalf("three resolved heroes produced a portrait: %#v", got)
	}
	// 阵容均衡 → 不下发（不是下发一个空 labels 的对象）。
	if got := gameplayTeamPortraitFrom([]int{58, 64, 84, 222, 412}, heroes, present); got != nil {
		t.Fatalf("a balanced comp produced a portrait: %#v", got)
	}
	if got := gameplayTeamPortraitFrom(nil, heroes, present); got != nil {
		t.Fatalf("an empty roster produced a portrait: %#v", got)
	}
	if got := gameplayTeamPortraitFrom([]int{238, 91, 121, 55, 84}, nil, nil); got != nil {
		t.Fatalf("a missing postmatch table produced a portrait: %#v", got)
	}
}

// P1-5 走完整条 HTTP 路径：刺客阵容 → teamPortrait 出现在响应里；均衡阵容 → 不出现。
func TestGameplayTeamPortraitEndToEnd(t *testing.T) {
	assassin, _, _ := r116dRecommendationRun(t, r116dBaseQuery+"&phase=ChampSelect&allyChampionIds=157,238,91,121,55")
	portrait := assassin.Recommendations.TeamPortrait
	if portrait == nil {
		t.Fatal("an all-assassin comp produced no teamPortrait in the response")
	}
	keys := r116dLabelKeys(portrait.Labels)
	if !keys["frontline"] || !keys["crowdControl"] {
		t.Fatalf("portrait labels = %#v, want 缺前排 + 缺控制", portrait.Labels)
	}
	if keys["sustainedDamage"] {
		t.Fatalf("portrait wrongly claims 缺持续输出: %#v", portrait.Labels)
	}
	if portrait.ResolvedHeroes != 5 || portrait.HeroPoolSize != 173 {
		t.Fatalf("portrait counters = %#v", portrait)
	}

	balanced, _, _ := r116dRecommendationRun(t, r116dBaseQuery+"&phase=ChampSelect&allyChampionIds=58,64,84,222,412")
	if balanced.Recommendations.TeamPortrait != nil {
		t.Fatalf("a balanced comp produced a teamPortrait: %#v", balanced.Recommendations.TeamPortrait)
	}
}

// ---------------------------------------------------------------------------
// 小工具
// ---------------------------------------------------------------------------

func absFloat(value float64) float64 {
	if value < 0 {
		return -value
	}
	return value
}

func maxFloat(left, right float64) float64 {
	if left > right {
		return left
	}
	return right
}

func countStrings(values []string, want string) int {
	total := 0
	for _, value := range values {
		if value == want {
			total++
		}
	}
	return total
}

func r116dLabelKeys(labels []gameplayTeamPortraitLabel) map[string]bool {
	keys := make(map[string]bool, len(labels))
	for _, label := range labels {
		keys[label.Key] = true
	}
	return keys
}

func r116dQueryValues(raw string) map[string][]string {
	values, err := url.ParseQuery(raw)
	if err != nil {
		panic(err)
	}
	return values
}

func r116dDiagnosticText(t *testing.T, storage *localStore) string {
	t.Helper()
	data, err := storage.readDiagnosticLog()
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// r116dDiagnosticEvents 按出现顺序取出指定事件名的全部诊断记录。
func r116dDiagnosticEvents(t *testing.T, storage *localStore, event string) []map[string]any {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(r116dDiagnosticText(t, storage)), "\n")
	events := make([]map[string]any, 0, 4)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || !strings.Contains(line, `"event":"`+event+`"`) {
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(line), &payload); err != nil {
			t.Fatalf("diagnostic line is not JSON: %v (%s)", err, line)
		}
		events = append(events, payload)
	}
	if len(events) == 0 {
		t.Fatalf("no %q event in the diagnostic log:\n%s", event, strings.Join(lines, "\n"))
	}
	return events
}

func r116dLastDiagnosticEvent(t *testing.T, text, event string) map[string]any {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(text), "\n")
	for index := len(lines) - 1; index >= 0; index-- {
		line := strings.TrimSpace(lines[index])
		if line == "" || !strings.Contains(line, `"event":"`+event+`"`) {
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(line), &payload); err != nil {
			t.Fatalf("diagnostic line is not JSON: %v (%s)", err, line)
		}
		return payload
	}
	t.Fatalf("no %q event in the diagnostic log:\n%s", event, text)
	return nil
}

// r116dStripLineComments 去掉整行的 // 注释与行尾 // 注释，只留下真实代码。
// 用于「阶段二未接线」这类按出现次数判定的断言——文档注释里刻意点了函数名，
// 不剔掉就会把注释当成调用。
func r116dStripLineComments(source string) string {
	lines := strings.Split(source, "\n")
	for index, line := range lines {
		if position := strings.Index(line, "//"); position >= 0 {
			lines[index] = line[:position]
		}
	}
	return strings.Join(lines, "\n")
}
