package main

import (
	"context"
	"math"
	"sort"
	"strings"
	"sync"
	"testing"
)

// R116-F：P1「英雄专属阶段 × 稀有度概率」与 P2「负向推荐（慎选）判据」的验证。
//
// 正样例全部吃真实上游数据：testdata/r116/hexdata-hero-157-stage-rarity.json 是
// /api/hexdata/heroes/157（疾风剑豪，patch 16.18）真实响应里 augments[] 全量 126 条
// 的字段投影。同目录的 hexdata-hero-157.json 只保留了前 10 条 augments，用它算不出
// 工单 P1 的「三组之和 = 1.0 ± 0.01」——那条判据只在阶段内全量聚合时才成立。
//
// 三条期望值（1.000003 / 121 条 / 3.791613）都是 2026-09-20 抓取的真实响应上
// 独立核算出来的，写死在这里当回归钉子：上游改 pickRate 口径时这几条会先红。

const r116fStageRarityFixture = "hexdata-hero-157-stage-rarity.json"

func r116fHero157Augments(t *testing.T) []hexdataAugmentRowV2 {
	t.Helper()
	detail, _, err := parseHexdataHeroJSONWithStats(r116aFixture(t, r116fStageRarityFixture))
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Augments) != 126 {
		t.Fatalf("augments = %d, want 126（夹具必须是全量投影）", len(detail.Augments))
	}
	return detail.Augments
}

// r116fSampleSnapshot 用真实 meta 响应里的 samplePolicy：P2 判据③必须复用 R116-B
// 已落地的分档（阈值来自 meta），测试里也不许硬编码 250/1000。
func r116fSampleSnapshot(t *testing.T) hexdataMetaSnapshot {
	t.Helper()
	snapshot, err := parseHexdataMeta(r116aFixture(t, "hexdata-meta.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !snapshot.SamplePolicy.usable() {
		t.Fatalf("真实 meta 的 samplePolicy 不可用：%#v", snapshot.SamplePolicy)
	}
	return snapshot
}

func r116fCloseTo(t *testing.T, label string, got, want, tolerance float64) {
	t.Helper()
	if math.Abs(got-want) > tolerance {
		t.Fatalf("%s = %.9f, want %.9f (±%.9f)", label, got, want, tolerance)
	}
}

// ---------------------------------------------------------------------------
// P1 判据：某阶段所有海克斯按 rarity 分组求和后，三个概率之和应为 1.0（±0.01）
// ---------------------------------------------------------------------------

func TestMayhemHeroStageRaritySumsToOnePerStageOnRealHero157(t *testing.T) {
	augments := r116fHero157Augments(t)
	distribution := computeHeroStageRarityDistribution(augments)
	if len(distribution) != 4 {
		t.Fatalf("阶段数 = %d（%v），want 4", len(distribution), distribution)
	}
	// 真实响应上独立核算出来的期望值（上游 0..1 原值，未归一化）。
	expected := map[int]map[string]float64{
		1: {"prismatic": 0.469768, "gold": 0.449009, "silver": 0.081226},
		2: {"prismatic": 0.275884, "gold": 0.437865, "silver": 0.286254},
		3: {"prismatic": 0.271517, "gold": 0.439486, "silver": 0.288998},
		4: {"prismatic": 0.272221, "gold": 0.437424, "silver": 0.290356},
	}
	expectedTotals := map[int]float64{1: 1.000003, 2: 1.000003, 3: 1.000001, 4: 1.000001}
	for stage := 1; stage <= 4; stage++ {
		groups, ok := distribution[stage]
		if !ok {
			t.Fatalf("阶段 %d 缺失", stage)
		}
		if len(groups) != 3 {
			t.Fatalf("阶段 %d 的稀有度分组 = %#v，want 棱彩/黄金/白银 三组", stage, groups)
		}
		total := groups["prismatic"] + groups["gold"] + groups["silver"]
		// 工单 P1 的验证判据本体。
		if math.Abs(total-1.0) > 0.01 {
			t.Fatalf("阶段 %d 三组之和 = %.6f，超出 1.0 ± 0.01", stage, total)
		}
		r116fCloseTo(t, "阶段总和", total, expectedTotals[stage], 1e-6)
		for rarity, want := range expected[stage] {
			r116fCloseTo(t, "阶段 "+string(rune('0'+stage))+" "+rarity, groups[rarity], want, 1e-6)
		}
	}
	// 下发结构：百分数（与全服口径 hexdataRarityStage 同单位）、阶段升序、带条数与三组之和。
	rows := hexdataHeroStageRarityRows(augments)
	if len(rows) != 4 {
		t.Fatalf("下发行数 = %d（%#v），want 4", len(rows), rows)
	}
	wantAugments := []int{121, 126, 126, 126}
	for index, row := range rows {
		if row.Stage != index+1 {
			t.Fatalf("第 %d 行的阶段号 = %d，want %d（必须升序）", index, row.Stage, index+1)
		}
		if row.Augments != wantAugments[index] {
			t.Fatalf("阶段 %d 的条数 = %d，want %d", row.Stage, row.Augments, wantAugments[index])
		}
		r116fCloseTo(t, "下发行三组之和", row.Total, expectedTotals[row.Stage]*100, 1e-4)
		r116fCloseTo(t, "下发棱彩", row.Prismatic, expected[row.Stage]["prismatic"]*100, 1e-4)
		r116fCloseTo(t, "下发黄金", row.Gold, expected[row.Stage]["gold"]*100, 1e-4)
		r116fCloseTo(t, "下发白银", row.Silver, expected[row.Stage]["silver"]*100, 1e-4)
		if row.Total <= 0 {
			t.Fatalf("阶段 %d 的 Total = %v，取不到就该整块不下发而不是下发 0", row.Stage, row.Total)
		}
	}
}

// 陷阱①：阶段 1 只有 121/126 条 augment 有 stage 行。缺失的 stage 行必须跳过，
// 绝不当成 pickRate=0 计入——补 0 既稀释概率，又让「三组之和 = 1.0」在阶段 1 失效。
func TestMayhemHeroStageRaritySkipsMissingStageRowsInsteadOfZeroFilling(t *testing.T) {
	augments := r116fHero157Augments(t)
	missing := make([]string, 0, 5)
	for _, augment := range augments {
		present := false
		for _, stage := range augment.Stages {
			if stage.Stage == 1 {
				present = true
				break
			}
		}
		if !present {
			missing = append(missing, augment.AugmentName)
		}
	}
	sort.Strings(missing)
	wantMissing := []string{"男爵之手", "属性！", "闪光弹", "面包和果酱", "闪闪现现"}
	sort.Strings(wantMissing)
	if strings.Join(missing, "|") != strings.Join(wantMissing, "|") {
		t.Fatalf("缺阶段 1 的 augment = %v，want %v", missing, wantMissing)
	}
	rows := hexdataHeroStageRarityRows(augments)
	if rows[0].Augments != 121 {
		t.Fatalf("阶段 1 的条数 = %d，want 121（不是 126：5 条上游就没有阶段 1 行）", rows[0].Augments)
	}
	r116fCloseTo(t, "阶段 1 三组之和", rows[0].Total, 100.0003, 1e-3)

	// 合成用例把「跳过」与「补 0」区分开：补 0 的实现会让阶段 1 出现一个 gold 键。
	synthetic := []hexdataAugmentRowV2{
		{AugmentID: 1, Rarity: "棱彩", Stages: []hexdataAugmentStageRow{{Stage: 1, PickRate: 0.6}, {Stage: 2, PickRate: 0.4}}},
		{AugmentID: 2, Rarity: "黄金", Stages: []hexdataAugmentStageRow{{Stage: 2, PickRate: 0.6}}},
	}
	distribution := computeHeroStageRarityDistribution(synthetic)
	if len(distribution[1]) != 1 {
		t.Fatalf("阶段 1 的分组 = %#v，缺失的 stage 行被补成 pickRate=0 计入了", distribution[1])
	}
	if _, ok := distribution[1]["gold"]; ok {
		t.Fatal("阶段 1 里出现了没有 stage 行的那条 augment 的稀有度键")
	}
	r116fCloseTo(t, "合成阶段 1 棱彩", distribution[1]["prismatic"], 0.6, 1e-12)
	r116fCloseTo(t, "合成阶段 2 棱彩", distribution[2]["prismatic"], 0.4, 1e-12)
	r116fCloseTo(t, "合成阶段 2 黄金", distribution[2]["gold"], 0.6, 1e-12)
	syntheticRows := hexdataHeroStageRarityRows(synthetic)
	if len(syntheticRows) != 2 || syntheticRows[0].Augments != 1 || syntheticRows[1].Augments != 2 {
		t.Fatalf("合成用例的条数 = %#v，want 阶段1=1 / 阶段2=2", syntheticRows)
	}
}

// 陷阱②：绝对不能用 augment 顶层的 pickRate 算稀有度分布——它是跨阶段选取率，
// 实测 126 条求和 = 3.791613，按它分组会得到「三组之和 3.79」这种一眼假的结果。
func TestMayhemHeroStageRarityMustNotUseTopLevelPickRate(t *testing.T) {
	augments := r116fHero157Augments(t)
	wrong := make(map[string]float64, 3)
	wrongTotal := 0.0
	for _, augment := range augments {
		rarity := hexdataRarityLabel(augment.Rarity)
		if rarity == "" {
			continue
		}
		wrong[rarity] += augment.PickRate
		wrongTotal += augment.PickRate
	}
	r116fCloseTo(t, "顶层 pickRate 求和（错误字段）", wrongTotal, 3.791613, 1e-6)
	r116fCloseTo(t, "顶层 pickRate 棱彩（错误字段）", wrong["prismatic"], 1.235358, 1e-6)
	r116fCloseTo(t, "顶层 pickRate 黄金（错误字段）", wrong["gold"], 1.671008, 1e-6)
	r116fCloseTo(t, "顶层 pickRate 白银（错误字段）", wrong["silver"], 0.885247, 1e-6)
	if math.Abs(wrongTotal-1.0) <= 0.01 {
		t.Fatal("顶层 pickRate 竟然落在 1.0 ± 0.01 内：夹具或上游口径变了，这条陷阱测试需要重新核对")
	}
	// 生产函数用的是 stages[].pickRate，四个阶段都满足工单判据。
	for stage, groups := range computeHeroStageRarityDistribution(augments) {
		total := groups["prismatic"] + groups["gold"] + groups["silver"]
		if math.Abs(total-1.0) > 0.01 {
			t.Fatalf("阶段 %d 用 stages[].pickRate 求和 = %.6f，判据不成立", stage, total)
		}
	}
}

// 降级路径：未知稀有度整条跳过（表现为该阶段 Total < 100%，响应里可见），
// 非法阶段号整条跳过，三组全 0 的阶段不下发，空输入返回 nil。
func TestMayhemHeroStageRarityDegradesOnUnknownRarityAndIllegalStage(t *testing.T) {
	if got := computeHeroStageRarityDistribution(nil); len(got) != 0 {
		t.Fatalf("空输入 = %#v，want 空 map", got)
	}
	if got := hexdataHeroStageRarityRows(nil); got != nil {
		t.Fatalf("空输入 = %#v，want nil（前端整块不渲染）", got)
	}
	rows := []hexdataAugmentRowV2{
		{AugmentID: 1, Rarity: "翡翠", Stages: []hexdataAugmentStageRow{{Stage: 1, PickRate: 0.9}}},
		{AugmentID: 2, Rarity: "白银", Stages: []hexdataAugmentStageRow{{Stage: 0, PickRate: 0.5}, {Stage: -1, PickRate: 0.5}, {Stage: 3, PickRate: 0}}},
	}
	distribution := computeHeroStageRarityDistribution(rows)
	for _, stage := range []int{-1, 0, 1} {
		if _, ok := distribution[stage]; ok {
			t.Fatalf("阶段 %d 不该出现在分布里：%#v", stage, distribution[stage])
		}
	}
	if got := hexdataHeroStageRarityRows(rows); got != nil {
		t.Fatalf("三组全 0 的阶段应该整块不下发，得到 %#v", got)
	}
	// 上游新增第四种稀有度时：这一阶段的 Total 会明显小于 100%，是可见的降级，
	// 而不是多出一个空字符串键被前端渲染成「未知 0.00%」。
	mixed := []hexdataAugmentRowV2{
		{AugmentID: 1, Rarity: "棱彩", Stages: []hexdataAugmentStageRow{{Stage: 1, PickRate: 0.4}}},
		{AugmentID: 2, Rarity: "翡翠", Stages: []hexdataAugmentStageRow{{Stage: 1, PickRate: 0.6}}},
	}
	got := hexdataHeroStageRarityRows(mixed)
	if len(got) != 1 {
		t.Fatalf("行数 = %#v，want 1", got)
	}
	r116fCloseTo(t, "未知稀有度阶段的 Total", got[0].Total, 40, 1e-9)
	if got[0].Augments != 1 {
		t.Fatalf("条数 = %d，want 1（未知稀有度那条不计入）", got[0].Augments)
	}
	if got[0].Prismatic <= 0 || got[0].Gold != 0 || got[0].Silver != 0 {
		t.Fatalf("未知稀有度被塞进了某一组：%#v", got[0])
	}
}

// ---------------------------------------------------------------------------
// P2：负向推荐（慎选）三条判据
// ---------------------------------------------------------------------------

func r116fMetricRow(id int, name, rarity string, pickRate, deltaWinRate float64, games int, snapshot hexdataMetaSnapshot) championMetricRow {
	rows := []championMetricRow{{
		Assets:       []championAsset{{ID: id, Kind: "augment", Name: name, Source: "hexdata"}},
		Rarity:       rarity,
		PickRate:     pickRate,
		DeltaWinRate: deltaWinRate,
		Games:        games,
	}}
	// sampleTier 一律由 R116-B 的 applyHexdataSampleTiers 按 meta.samplePolicy 算，
	// 测试不自造分档（判据③复用的就是这个字段的产出）。
	applyHexdataSampleTiers(rows, snapshot)
	return rows[0]
}

func TestMayhemNegativeRecommendationRequiresAllThreeCriteria(t *testing.T) {
	snapshot := r116fSampleSnapshot(t)
	row := func(id int, rarity string, pickRate, deltaWinRate float64, games int) championMetricRow {
		return r116fMetricRow(id, "海克斯", rarity, pickRate, deltaWinRate, games, snapshot)
	}
	// 工单验证判据正样例：选取率 30%、deltaWinRate -0.05、样本 5000 → 生成慎选提示。
	rows := []championMetricRow{row(1, "gold", 30, -0.05, 5000), row(2, "gold", 5, 0.02, 5000), row(3, "gold", 4, 0.01, 5000)}
	if rows[0].SampleTier != "high" {
		t.Fatalf("样本 5000 场应该被 meta.samplePolicy 分成 high，实际 %q", rows[0].SampleTier)
	}
	if got := markMayhemNegativeRecommendations(rows); got != 1 {
		t.Fatalf("命中条数 = %d，want 1", got)
	}
	if !rows[0].Caution {
		t.Fatal("工单正样例（选取率 30%、deltaWinRate -0.05、样本 5000）没有生成慎选提示")
	}
	r116fCloseTo(t, "同稀有度中位数", rows[0].CautionMedianPickRate, 5, 1e-9)
	if rows[1].Caution || rows[2].Caution {
		t.Fatalf("deltaWinRate 为正的行被标了慎选：%#v", rows)
	}

	// 工单验证判据反样例：选取率高但样本低 → 不生成（低样本不作为慎选判据）。
	low := []championMetricRow{row(1, "gold", 30, -0.05, 10), row(2, "gold", 5, 0.02, 5000), row(3, "gold", 4, 0.01, 5000)}
	if low[0].SampleTier != "low" {
		t.Fatalf("样本 10 场应该被分成 low，实际 %q", low[0].SampleTier)
	}
	if got := markMayhemNegativeRecommendations(low); got != 0 {
		t.Fatalf("低样本行命中了 %d 条，want 0", got)
	}
	if low[0].Caution || low[0].CautionMedianPickRate != 0 {
		t.Fatalf("低样本行被当成慎选信号：%#v", low[0])
	}

	// 判据①是「超过」中位数：等于中位数不生成。
	tie := []championMetricRow{row(1, "gold", 5, -0.05, 5000), row(2, "gold", 5, 0.02, 5000), row(3, "gold", 4, 0.01, 5000)}
	if got := markMayhemNegativeRecommendations(tie); got != 0 {
		t.Fatalf("选取率恰好等于中位数的行命中了 %d 条，want 0（判据①要求严格超过）", got)
	}
	// 判据②：deltaWinRate = 0 不算「低于基准」，不生成。
	zero := []championMetricRow{row(1, "gold", 30, 0, 5000), row(2, "gold", 5, 0.02, 5000), row(3, "gold", 4, 0.01, 5000)}
	if got := markMayhemNegativeRecommendations(zero); got != 0 {
		t.Fatalf("deltaWinRate = 0 的行命中了 %d 条，want 0", got)
	}
	// Rarity 为空的行（上游缺字段 / OP.GG RSC 回退行）既不进池子也不判定。
	unknown := []championMetricRow{row(1, "", 30, -0.05, 5000), row(2, "gold", 5, 0.02, 5000)}
	if got := markMayhemNegativeRecommendations(unknown); got != 0 {
		t.Fatalf("无稀有度的行命中了 %d 条，want 0", got)
	}
	// 幂等：重复调用不累积，也不残留上一轮的标记。
	repeat := []championMetricRow{row(1, "gold", 30, -0.05, 5000), row(2, "gold", 5, 0.02, 5000), row(3, "gold", 4, 0.01, 5000)}
	first := markMayhemNegativeRecommendations(repeat)
	repeat[0].DeltaWinRate = 0.03
	second := markMayhemNegativeRecommendations(repeat)
	if first != 1 || second != 0 || repeat[0].Caution || repeat[0].CautionMedianPickRate != 0 {
		t.Fatalf("判据不幂等：first=%d second=%d row=%#v", first, second, repeat[0])
	}
	// 没有任何可用稀有度池时直接返回 0，不 panic。
	if got := markMayhemNegativeRecommendations(nil); got != 0 {
		t.Fatalf("空输入命中 %d 条，want 0", got)
	}
}

func TestMayhemNegativeRecommendationMedianIsGroupedByRarity(t *testing.T) {
	// 棱彩/黄金的天然选取率量级不同：混在一起算一个中位数，黄金那条永远判不出来。
	rows := []championMetricRow{
		{Rarity: "prismatic", PickRate: 20, DeltaWinRate: -0.05, SampleTier: "high"},
		{Rarity: "prismatic", PickRate: 22, DeltaWinRate: -0.05, SampleTier: "high"},
		{Rarity: "prismatic", PickRate: 24, DeltaWinRate: 0.05, SampleTier: "high"},
		{Rarity: "gold", PickRate: 1, DeltaWinRate: -0.05, SampleTier: "high"},
		{Rarity: "gold", PickRate: 2, DeltaWinRate: -0.05, SampleTier: "high"},
		{Rarity: "gold", PickRate: 3, DeltaWinRate: -0.02, SampleTier: "high"},
	}
	medians := mayhemPickRateMedianByRarity(rows)
	r116fCloseTo(t, "棱彩中位数", medians["prismatic"], 22, 1e-9)
	r116fCloseTo(t, "黄金中位数", medians["gold"], 2, 1e-9)
	if len(medians) != 2 {
		t.Fatalf("中位数池 = %#v，want 两个稀有度", medians)
	}
	if got := markMayhemNegativeRecommendations(rows); got != 1 || !rows[5].Caution {
		t.Fatalf("命中 = %d 条（%#v），want 只有黄金选取率 3 那条", got, rows)
	}
	// 混合池的中位数是 (3+20)/2 = 11.5：黄金的 3 会被判成「不超过中位数」，判据失效。
	r116fCloseTo(t, "混合池中位数（错误做法）", mayhemMedian([]float64{20, 22, 24, 1, 2, 3}), 11.5, 1e-9)
	// 偶数个元素按标准定义取中间两数的平均；奇数个取中间那个；空池返回 0。
	r116fCloseTo(t, "偶数个中位数", mayhemMedian([]float64{1, 2, 3, 10}), 2.5, 1e-9)
	r116fCloseTo(t, "奇数个中位数", mayhemMedian([]float64{7, 1, 3}), 3, 1e-9)
	r116fCloseTo(t, "单个元素中位数", mayhemMedian([]float64{4}), 4, 1e-9)
	r116fCloseTo(t, "空池中位数", mayhemMedian(nil), 0, 1e-9)
	input := []float64{3, 1, 2}
	mayhemMedian(input)
	if input[0] != 3 || input[1] != 1 || input[2] != 2 {
		t.Fatalf("mayhemMedian 修改了入参：%v", input)
	}
	// Rarity 为空的行不进池子（没有「同稀有度」就谈不上同稀有度中位数）。
	pools := mayhemPickRateMedianByRarity([]championMetricRow{{Rarity: "", PickRate: 99}, {Rarity: "  ", PickRate: 98}})
	if len(pools) != 0 {
		t.Fatalf("空稀有度进了中位数池：%#v", pools)
	}
}

func TestMayhemNegativeRecommendationOnRealHero157Augments(t *testing.T) {
	augments := r116fHero157Augments(t)
	snapshot := r116fSampleSnapshot(t)
	rows := hexdataAugmentMetricRows(augments)
	if len(rows) != 126 {
		t.Fatalf("过滤后行数 = %d，want 126（真实数据里最小 games = 914，全部超过 hexdataMinimumSample）", len(rows))
	}
	applyHexdataSampleTiers(rows, snapshot)
	medians := mayhemPickRateMedianByRarity(rows)
	r116fCloseTo(t, "棱彩中位数", medians["prismatic"], 1.619, 1e-9)
	r116fCloseTo(t, "黄金中位数", medians["gold"], 2.346, 1e-9)
	r116fCloseTo(t, "白银中位数", medians["silver"], 1.42015, 1e-9)
	flagged := markMayhemNegativeRecommendations(rows)
	if flagged != 37 {
		t.Fatalf("真实数据命中 = %d 条，want 37（执行账本里的独立核算值）", flagged)
	}
	// 每一条命中行都必须同时满足三条判据；每一条未命中行都必须至少违反一条。
	for index := range rows {
		row := rows[index]
		median := medians[row.Rarity]
		want := row.PickRate > median && row.DeltaWinRate < 0 && row.SampleTier != "low"
		if row.Caution != want {
			t.Fatalf("第 %d 行（%s）Caution = %v，按三条判据应该是 %v：%#v", index, row.Assets[0].Name, row.Caution, want, row)
		}
		if !want {
			continue
		}
		if row.SampleTier == "low" || row.SampleTier == "" {
			t.Fatalf("命中行的样本分档 = %q，判据③要求非 low 且必须已知", row.SampleTier)
		}
		if row.CautionMedianPickRate != median || median <= 0 {
			t.Fatalf("命中行带的中位数 = %v，want %v", row.CautionMedianPickRate, median)
		}
	}
	// 真实数据里 126 条全部 games ≥ 914，所以判据③这一轮没有过滤掉任何行——
	// 这条钉子保证「低样本被排除」不是靠数据碰巧为空蒙过去的（合成用例另有覆盖）。
	lowSample := 0
	for index := range rows {
		if rows[index].SampleTier == "low" {
			lowSample++
		}
	}
	if lowSample != 0 {
		t.Fatalf("真实数据里出现了 %d 条 low 样本行，夹具口径变了，请重新核对期望值", lowSample)
	}
}

// ---------------------------------------------------------------------------
// 端到端接线：详情页响应真的带上了 P1 分布与 P2 标记，并写了诊断事件
// ---------------------------------------------------------------------------

func TestLoadMayhemDetailCarriesHeroStageRarityAndCautionFlags(t *testing.T) {
	recorder := &r116aRecorder{}
	provider, _ := r116aMayhemProvider(t, recorder, r116aFixture(t, r116fStageRarityFixture))
	var eventsMu sync.Mutex
	events := make([]map[string]any, 0, 8)
	provider.diag = func(event map[string]any) {
		eventsMu.Lock()
		defer eventsMu.Unlock()
		events = append(events, event)
	}
	response, err := provider.loadMayhemDetail(context.Background(), "157")
	provider.hexdata.pruneWait.Wait()
	if err != nil {
		t.Fatalf("loadMayhemDetail failed: %v", err)
	}
	// P1：英雄专属阶段概率随详情页一次下发，没有为它多打一条上游请求。
	if len(response.HeroStageRarity) != 4 {
		t.Fatalf("heroStageRarity = %#v，want 4 个阶段", response.HeroStageRarity)
	}
	if response.HeroStageRarity[0].Augments != 121 {
		t.Fatalf("阶段 1 条数 = %d，want 121", response.HeroStageRarity[0].Augments)
	}
	for _, row := range response.HeroStageRarity {
		if math.Abs(row.Total-100) > 1 {
			t.Fatalf("阶段 %d 三组之和 = %.6f%%，超出 100%% ± 1 个百分点", row.Stage, row.Total)
		}
	}
	if got := recorder.count("/api/hexdata/heroes/157"); got != 1 {
		t.Fatalf("hero-json 请求 = %d 次，want 1（P1 是纯本地聚合，不许加请求）", got)
	}
	// P2：慎选标记落在用户实际看到的那一份海克斯行上。
	flagged := 0
	for index := range response.RecommendedAugments {
		if response.RecommendedAugments[index].Caution {
			flagged++
			if response.RecommendedAugments[index].CautionMedianPickRate <= 0 {
				t.Fatalf("命中行没带中位数：%#v", response.RecommendedAugments[index])
			}
		}
	}
	if flagged != 37 {
		t.Fatalf("详情页命中 = %d 条，want 37", flagged)
	}
	// 口径说明落在常驻页脚（与既有说明拼在一起，不是新写一份渲染）。
	if !strings.Contains(response.MeasurementTechnique, "该英雄专属品质概率") {
		t.Fatalf("measurementTechnique 缺少 P1 的口径说明：%q", response.MeasurementTechnique)
	}
	if !strings.Contains(response.MeasurementTechnique, "不代表抽取、刷新或保底概率") {
		t.Fatalf("measurementTechnique 缺少「不代表抽取概率」的降级声明：%q", response.MeasurementTechnique)
	}
	// 诊断事件：两条都要有，排障时才看得出是哪一条判据把行过滤光了。
	var rarityEvent, cautionEvent map[string]any
	for _, event := range events {
		switch event["event"] {
		case "mayhem_stage_rarity":
			rarityEvent = event
		case "mayhem_caution":
			cautionEvent = event
		}
	}
	if rarityEvent == nil || cautionEvent == nil {
		t.Fatalf("诊断事件缺失：rarity=%#v caution=%#v", rarityEvent, cautionEvent)
	}
	if rarityEvent["deviatingStages"] != 0 {
		t.Fatalf("有阶段的三组之和偏离 1.0 超过 ±0.01：%#v", rarityEvent)
	}
	if cautionEvent["flagged"] != 37 || cautionEvent["rows"] != 126 || cautionEvent["samplePolicyUsable"] != true {
		t.Fatalf("慎选诊断事件 = %#v", cautionEvent)
	}
}

// samplePolicy 不可用时整块跳过：那时所有行的 SampleTier 都是空串，判据③
// 「!= low」会对每一行都成立，等于把「不知道样本量」当成「不是低样本」。
func TestLoadMayhemDetailSkipsCautionFlagsWhenSamplePolicyIsUnavailable(t *testing.T) {
	snapshot := hexdataMetaSnapshot{}
	if snapshot.SamplePolicy.usable() {
		t.Fatal("空快照的 samplePolicy 不该被判成可用")
	}
	rows := []championMetricRow{
		{Rarity: "gold", PickRate: 30, DeltaWinRate: -0.05, Games: 5000},
		{Rarity: "gold", PickRate: 5, DeltaWinRate: 0.02, Games: 5000},
		{Rarity: "gold", PickRate: 4, DeltaWinRate: 0.01, Games: 5000},
	}
	applyHexdataSampleTiers(rows, snapshot)
	for index := range rows {
		if rows[index].SampleTier != "" {
			t.Fatalf("策略不可用时 SampleTier = %q，want 空串", rows[index].SampleTier)
		}
	}
	// 判据函数本身仍然按字面执行（SampleTier 为空 != "low"），所以调用方必须
	// 先判断策略可用——这条测试钉住「不调用」这个前提是有意义的。
	if got := markMayhemNegativeRecommendations(rows); got != 1 {
		t.Fatalf("没有分档信息时判据③形同虚设，命中 = %d 条（这正是调用方要跳过的原因）", got)
	}
}
