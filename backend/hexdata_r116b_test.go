package main

// R116-B P0-6 的后端下发测试（第二轮重做补齐的部分）。
//
// 工单 P0-6 要求「点击阶段 chip 后前端本地对已加载的 stages[] 重新渲染，不重新
// 发请求」，所以 augments[].stages[1..4] 必须随详情页响应一次下发。这三条测试
// 钉住三件事：
//  1. 正样例：stage 行的单位与父行一致（winRate/stageBaselineWinRate 是百分数，
//     deltaWinRate/wilsonLowerWinRate 保持上游 0..1），官方字母档位由后端算好直出，
//     且 sampleTier 由 meta.samplePolicy 算好直出；
//  2. 负样例：上游缺某个阶段时不补零行（实测阶段 1 只有 121/126 条 augment 有
//     stage 行）——补 0 会被前端渲染成「胜率 0%」，是显示不存在的数据；
//  3. 序列化：stages 只挂在海克斯行上，装备行/召唤师技能行靠 omitempty 不下发。

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"testing"
)

// r116bHeroFixtureWithoutStage 复制真实夹具，然后把指定 augment 的某个 stage 行
// 删掉，复现上游的真实缺失形态。dropStage <= 0 表示把整条 stages 删空。
func r116bHeroFixtureWithoutStage(t *testing.T, augmentIndex, dropStage int) []byte {
	t.Helper()
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(r116aFixture(t, "hexdata-hero-157.json"), &payload); err != nil {
		t.Fatal(err)
	}
	var augments []map[string]any
	if err := json.Unmarshal(payload["augments"], &augments); err != nil {
		t.Fatal(err)
	}
	if augmentIndex >= len(augments) {
		t.Fatalf("fixture only has %d augments", len(augments))
	}
	if dropStage <= 0 {
		delete(augments[augmentIndex], "stages")
	} else {
		stages, ok := augments[augmentIndex]["stages"].([]any)
		if !ok {
			t.Fatal("fixture augment has no stages")
		}
		kept := make([]any, 0, len(stages))
		for _, entry := range stages {
			row, ok := entry.(map[string]any)
			if !ok {
				t.Fatal("fixture stage row is not an object")
			}
			if int(r116bNumberOf(row["stage"])) == dropStage {
				continue
			}
			kept = append(kept, row)
		}
		if len(kept) == len(stages) {
			t.Fatalf("stage %d was not present in the fixture", dropStage)
		}
		augments[augmentIndex]["stages"] = kept
	}
	rewritten, err := json.Marshal(augments)
	if err != nil {
		t.Fatal(err)
	}
	payload["augments"] = rewritten
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func r116bNumberOf(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case int:
		return float64(typed)
	case json.Number:
		parsed, err := typed.Float64()
		if err != nil {
			return 0
		}
		return parsed
	default:
		return 0
	}
}

func r116bAugmentRow(t *testing.T, rows []championMetricRow, augmentID int) championMetricRow {
	t.Helper()
	for _, row := range rows {
		if len(row.Assets) > 0 && row.Assets[0].ID == augmentID {
			return row
		}
	}
	t.Fatalf("augment %d is not in the response (%d rows)", augmentID, len(rows))
	return championMetricRow{}
}

func r116bStage(t *testing.T, stages []championMetricStageRow, stage int) championMetricStageRow {
	t.Helper()
	for _, row := range stages {
		if row.Stage == stage {
			return row
		}
	}
	t.Fatalf("stage %d is missing from %+v", stage, stages)
	return championMetricStageRow{}
}

func r116bClose(t *testing.T, label string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-6 {
		t.Fatalf("%s = %v, want %v", label, got, want)
	}
}

// 正样例：夹具里 augmentId 2095（掷骰狂人）的四个阶段，逐字段核对单位。
// 上游原值取自 backend/testdata/r116/hexdata-hero-157.json（真实响应裁剪版）。
func TestLoadMayhemDetailEmitsAugmentStagesInParentUnits(t *testing.T) {
	recorder := &r116aRecorder{}
	provider, _ := r116aMayhemProvider(t, recorder, r116aFixture(t, "hexdata-hero-157.json"))
	response, err := provider.loadMayhemDetail(context.Background(), "157")
	if err != nil {
		t.Fatalf("loadMayhemDetail failed: %v", err)
	}
	row := r116bAugmentRow(t, response.RecommendedAugments, 2095)
	if len(row.Stages) != 4 {
		t.Fatalf("stages = %d, want 4 (%+v)", len(row.Stages), row.Stages)
	}
	// 阶段顺序保持上游原样（1..4），前端按 stage 号取行，不依赖下标。
	for index, stage := range row.Stages {
		if stage.Stage != index+1 {
			t.Fatalf("stages[%d].Stage = %d, want %d", index, stage.Stage, index+1)
		}
	}

	first := r116bStage(t, row.Stages, 1)
	// 上游 stage1：winRate 0.705363 / pickRate 0.02498 / stageBaselineWinRate
	// 0.567575 / deltaWinRate 0.137788 / wilsonLowerWinRate 0.701679 / games 59256
	// / hexTier hang。pickRate 曾被评审整改 B6 以「前端零渲染」裁掉，主控按工单
	// P0-6「实现要求」第 1 条的字面要求恢复（阶段 chip 必须能重渲染这三个值）。
	r116bClose(t, "stage1 winRate", first.WinRate, 70.5363)
	r116bClose(t, "stage1 pickRate", first.PickRate, 2.498)
	r116bClose(t, "stage1 baseline", first.StageBaselineWinRate, 56.7575)
	r116bClose(t, "stage1 deltaWinRate stays 0..1", first.DeltaWinRate, 0.137788)
	r116bClose(t, "stage1 wilsonLowerWinRate stays 0..1", first.WilsonLowerWinRate, 0.701679)
	if first.Games != 59256 {
		t.Fatalf("stage1 games = %d, want 59256", first.Games)
	}
	if first.HexLabel != "夯" {
		t.Fatalf("stage1 hexLabel = %q, want 夯", first.HexLabel)
	}
	// 评审整改 B4：阶段行带自己的官方字母档位（hang → S），映射复用父行那一个
	// hexdataOfficialGrade，前端不复制对照表。
	if first.Grade != "S" {
		t.Fatalf("stage1 grade = %q, want S (hexTier hang)", first.Grade)
	}
	// sampleTier 走 meta.samplePolicy（high ≥1000 / medium 250..999 / low <250）。
	if first.SampleTier != "high" {
		t.Fatalf("stage1 sampleTier = %q, want high", first.SampleTier)
	}
	third := r116bStage(t, row.Stages, 3)
	if third.Games != 373 || third.SampleTier != "medium" {
		t.Fatalf("stage3 = games %d tier %q, want 373/medium", third.Games, third.SampleTier)
	}
	if third.HexLabel != "顶级" {
		t.Fatalf("stage3 hexLabel = %q, want 顶级", third.HexLabel)
	}
	// 评审整改 B4 的实测矛盾点：这条 augment 父行是 hang（→ S），阶段 3 是 top
	// （→ A / 中文「顶级」）。阶段视图的徽章必须跟着阶段行走，否则同一张卡会同时
	// 显示「徽章 S」与「官方档位 顶级」。
	if third.Grade != "A" {
		t.Fatalf("stage3 grade = %q, want A (hexTier top)", third.Grade)
	}
	fourth := r116bStage(t, row.Stages, 4)
	if fourth.Games != 250 || fourth.SampleTier != "medium" {
		t.Fatalf("stage4 = games %d tier %q, want 250/medium", fourth.Games, fourth.SampleTier)
	}
	if fourth.Grade != "A" {
		t.Fatalf("stage4 grade = %q, want A (hexTier top)", fourth.Grade)
	}
	// 父行的单位没被 stage 逻辑带偏（P0-1/P0-2 的既有契约），官方档位仍然是 S。
	r116bClose(t, "parent winRate", row.WinRate, 67.9036)
	r116bClose(t, "parent deltaWinRate", row.DeltaWinRate, 0.111527)
	if row.SampleTier != "high" {
		t.Fatalf("parent sampleTier = %q, want high", row.SampleTier)
	}
	if row.Grade != "S" {
		t.Fatalf("parent grade = %q, want S", row.Grade)
	}
	// 装备行不下发 stages（上游 items[] 没有阶段维度）。
	for index, item := range response.ItemRanking {
		if len(item.Stages) != 0 {
			t.Fatalf("itemRanking[%d] carries %d stage rows", index, len(item.Stages))
		}
	}
}

// 负样例：上游缺阶段 1 时，该 augment 只有 3 条 stage 行，绝不用 0 补齐第 1 阶段。
func TestLoadMayhemDetailKeepsMissingAugmentStagesAbsent(t *testing.T) {
	recorder := &r116aRecorder{}
	provider, _ := r116aMayhemProvider(t, recorder, r116bHeroFixtureWithoutStage(t, 0, 1))
	response, err := provider.loadMayhemDetail(context.Background(), "157")
	if err != nil {
		t.Fatalf("loadMayhemDetail failed: %v", err)
	}
	row := r116bAugmentRow(t, response.RecommendedAugments, 2095)
	if len(row.Stages) != 3 {
		t.Fatalf("stages = %d, want 3 (stage 1 must stay absent)", len(row.Stages))
	}
	for _, stage := range row.Stages {
		if stage.Stage == 1 {
			t.Fatalf("stage 1 was invented: %+v", stage)
		}
		if stage.Stage <= 0 || stage.Stage > 4 {
			t.Fatalf("stage number out of range: %+v", stage)
		}
		if stage.WinRate == 0 && stage.Games == 0 {
			t.Fatalf("stage %d is a zero-filled placeholder: %+v", stage.Stage, stage)
		}
	}
	// 父行本身仍然下发：缺一个阶段不等于这条 augment 不可用。
	r116bClose(t, "parent winRate", row.WinRate, 67.9036)
}

// 负样例：整条 stages 缺失时下发空切片（omitempty → JSON 里没有 stages 键），
// 前端因此在阶段视图里整块隐藏这条 augment，而不是渲染 0%。
func TestLoadMayhemDetailWithoutAnyStageRowsOmitsStagesKey(t *testing.T) {
	recorder := &r116aRecorder{}
	provider, _ := r116aMayhemProvider(t, recorder, r116bHeroFixtureWithoutStage(t, 0, 0))
	response, err := provider.loadMayhemDetail(context.Background(), "157")
	if err != nil {
		t.Fatalf("loadMayhemDetail failed: %v", err)
	}
	row := r116bAugmentRow(t, response.RecommendedAugments, 2095)
	if len(row.Stages) != 0 {
		t.Fatalf("stages = %+v, want empty", row.Stages)
	}
	data, err := json.Marshal(row)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `"stages"`) {
		t.Fatalf("empty stages must be omitted, got %s", data)
	}
	// 其他 augment 的阶段数据不受影响。
	other := 0
	for _, candidate := range response.RecommendedAugments {
		if len(candidate.Stages) == 4 {
			other++
		}
	}
	if other == 0 {
		t.Fatal("every augment lost its stages")
	}
}

// 序列化契约：前端消费的是 JSON，字段名与「只挂海克斯行」这两条都要钉住。
func TestMayhemDetailResponseSerializesAugmentStagesForFrontend(t *testing.T) {
	recorder := &r116aRecorder{}
	provider, _ := r116aMayhemProvider(t, recorder, r116aFixture(t, "hexdata-hero-157.json"))
	response, err := provider.loadMayhemDetail(context.Background(), "157")
	if err != nil {
		t.Fatalf("loadMayhemDetail failed: %v", err)
	}
	data, err := json.Marshal(r116bAugmentRow(t, response.RecommendedAugments, 2095))
	if err != nil {
		t.Fatal(err)
	}
	payload := string(data)
	for _, want := range []string{`"stages":[{`, `"stage":1,`, `"winRate":`, `"deltaWinRate":`, `"wilsonLowerWinRate":`, `"stageBaselineWinRate":`, `"games":`, `"hexLabel":"夯"`, `"grade":"S"`, `"sampleTier":"high"`} {
		if !strings.Contains(payload, want) {
			t.Fatalf("serialized augment row is missing %s: %s", want, payload)
		}
	}
	// pickRate 父行与阶段行都下发：工单 P0-6「实现要求」第 1 条明写点击阶段 chip 后
	// 要重新渲染该阶段的 winRate/deltaWinRate/pickRate 三个值。评审整改 B6 曾以
	// 「前端零渲染」为由把阶段行的 pickRate 裁掉，主控判定**工单的字面要求优先于
	// 评审的省体积建议**，已恢复下发，并在阶段视图里渲染成「选取率」一格
	// （champions.js 的海克斯卡；汇总视图不含这一格，避免超范围的视觉改动）。
	if !strings.Contains(payload, `"pickRate":`) {
		t.Fatalf("parent augment row lost pickRate: %s", payload)
	}
	// 上游 stage 行的 wins/tier/hexTier/hexScore/recommendationScore/hexTierColor
	// 已裁剪掉：前端一个都不渲染，hexTier 还是内部枚举名（"hang"），下发只会诱导
	// 展示；阶段视图要的官方档位以 grade（字母）+ hexLabel（中文）下发。
	// 注意这里只查 stages 这一段——父行本身就带 hexTier/score/officialTier/pickRate。
	stagePayload, err := json.Marshal(r116bAugmentRow(t, response.RecommendedAugments, 2095).Stages)
	if err != nil {
		t.Fatal(err)
	}
	for _, unwanted := range []string{`"wins"`, `"tier"`, `"hexTier"`, `"hexScore"`, `"recommendationScore"`, `"hexTierColor"`} {
		if strings.Contains(string(stagePayload), unwanted) {
			t.Fatalf("stage payload should not carry %s: %s", unwanted, stagePayload)
		}
	}
	// 正向钉住 P0-6：阶段行确实带着自己的 pickRate。少了它，阶段 chip 就只能重渲染
	// 三个值里的两个，工单「实现要求」第 1 条不成立。
	if !strings.Contains(string(stagePayload), `"pickRate":`) {
		t.Fatalf("stage payload lost pickRate; P0-6 requires the stage chip to re-render winRate/deltaWinRate/pickRate: %s", stagePayload)
	}
	if len(response.ItemRanking) == 0 {
		t.Fatal("fixture produced no item rows")
	}
	itemPayload, err := json.Marshal(response.ItemRanking[0])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(itemPayload), `"stages"`) {
		t.Fatalf("item rows must not carry stages: %s", itemPayload)
	}
}
