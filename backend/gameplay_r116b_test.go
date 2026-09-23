package main

// R116-B 独立评审整改 B5 的后端测试：对局内推荐 bundle 不得携带 stages。
//
// 背景：P0-6 让详情页的每条海克斯行都带上 4 条阶段行（实测英雄 157 是 499 条、
// 约 97 KB），而 gameplayRecommendationsFromResolvedDetail 以前是把
// detail.RecommendedAugments 原样拷进 bundle 的——backend/web/gameplay.js 全文对
// "stages" 零命中，等于让延迟敏感的局内 overlay 白背一份没人渲染的数据，而台账
// 第 5 节只核算了详情页那一条链路（评审 B5）。
//
// 这条测试用真实的 loadMayhemDetail（真实夹具 backend/testdata/r116/）跑完整条
// 转换，钉住四件事：
//  1. bundle 的每一条海克斯行都没有阶段行，序列化后 JSON 里不出现 "stages" 键；
//  2. 除了 Stages，其它字段一个都没被误伤（局内推荐页要渲染胜率/官方档位/徽记）；
//  3. 详情页那份切片没有被反向清空（拷贝必须是值拷贝，它还要进缓存与详情响应）；
//  4. 体积确实变小了（剥离生效的可观测证据，具体数字用 -v 打出来记账）。

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestGameplayRecommendationBundleDropsAugmentStages(t *testing.T) {
	recorder := &r116aRecorder{}
	provider, _ := r116aMayhemProvider(t, recorder, r116aFixture(t, "hexdata-hero-157.json"))
	detail, err := provider.loadMayhemDetail(context.Background(), "157")
	if err != nil {
		t.Fatalf("loadMayhemDetail failed: %v", err)
	}
	source := r116bAugmentRow(t, detail.RecommendedAugments, 2095)
	if len(source.Stages) != 4 {
		t.Fatalf("详情页的 augment 2095 应该带 4 条阶段行，实际 %d 条", len(source.Stages))
	}

	bundle := gameplayRecommendationsFromChampionDetail(157, "", detail)
	if len(bundle.Augments) != len(detail.RecommendedAugments) {
		t.Fatalf("局内 bundle 的海克斯行数变了：%d，详情页是 %d", len(bundle.Augments), len(detail.RecommendedAugments))
	}
	for index, row := range bundle.Augments {
		if len(row.Stages) != 0 {
			t.Fatalf("bundle.Augments[%d] 仍然带着 %d 条阶段行", index, len(row.Stages))
		}
	}

	// 只剥阶段维度：局内推荐页要渲染的字段必须逐字保留。
	stripped := r116bAugmentRow(t, bundle.Augments, 2095)
	if stripped.WinRate != source.WinRate || stripped.DeltaWinRate != source.DeltaWinRate ||
		stripped.PickRate != source.PickRate || stripped.Score != source.Score ||
		stripped.Games != source.Games || stripped.HexLabel != source.HexLabel ||
		stripped.Grade != source.Grade || stripped.SampleTier != source.SampleTier ||
		stripped.WilsonLowerWinRate != source.WilsonLowerWinRate {
		t.Fatalf("剥 stages 时误伤了别的字段：\n got %#v\nwant %#v", stripped, source)
	}
	if len(stripped.Assets) != len(source.Assets) {
		t.Fatalf("资产列表被改动：%d vs %d", len(stripped.Assets), len(source.Assets))
	}

	// 值拷贝：详情页那份切片（还要进缓存与详情响应）不受影响。
	if len(r116bAugmentRow(t, detail.RecommendedAugments, 2095).Stages) != 4 {
		t.Fatal("detail.RecommendedAugments 的 stages 被 bundle 转换清掉了（必须是值拷贝）")
	}

	data, err := json.Marshal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `"stages"`) {
		t.Fatalf("局内 bundle 的 JSON 里仍然有 stages 键（%d B）", len(data))
	}

	// 体积记账：同一份 detail，海克斯段剥掉阶段维度前后各是多少。夹具只裁了 10 条
	// augment，真实英雄 126 条时这段是 97 KB（docs/r116b-execution-ledger.md 整改节）。
	before, err := json.Marshal(detail.RecommendedAugments)
	if err != nil {
		t.Fatal(err)
	}
	after, err := json.Marshal(bundle.Augments)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("海克斯段 JSON：详情页 %d B → 局内 %d B（省 %d B，%.1f%%，夹具 10 条 augment）",
		len(before), len(after), len(before)-len(after), float64(len(before)-len(after))*100/float64(max(1, len(before))))
	if len(after) >= len(before) {
		t.Fatalf("剥掉 stages 之后体积没有变小（%d → %d），说明剥离没有生效", len(before), len(after))
	}
}
