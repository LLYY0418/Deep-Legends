package main

import (
	"strings"
	"testing"
)

// R132 P4：OP.GG 实时阵容不按分路排序，辅助排第一、上单排最后时要重排成上野中下辅。
func TestR132CurrentGamePlayersOrderedTopToSupport(t *testing.T) {
	player := func(name, preferred string, spells ...int64) currentGamePlayer {
		return currentGamePlayer{gameplayPlayer: gameplayPlayer{GameName: name}, PreferredPosition: preferred, Spells: spells}
	}
	blue := []currentGamePlayer{
		player("support", "utility", 4, 14), player("jungle", "jungle", 4, 11), player("mid", "middle", 4, 14),
		player("adc", "bottom", 4, 7), player("top", "top", 4, 12),
	}
	got := []string{}
	for _, item := range orderCurrentGamePlayersByRole(blue) {
		got = append(got, item.GameName)
	}
	if strings.Join(got, ",") != "top,jungle,mid,adc,support" {
		t.Fatalf("order = %v", got)
	}
	// 没有任何分路依据时保持来源顺序。
	unknown := []currentGamePlayer{player("a", ""), player("b", ""), player("c", "")}
	for index, item := range orderCurrentGamePlayersByRole(unknown) {
		if item.GameName != unknown[index].GameName {
			t.Fatalf("unknown roster reordered: %v", item.GameName)
		}
	}
}

// R132 P2：DPM 767 对 988（比值 0.78，C+）在雷达图上必须看得出差距，
// 提示框只保留指标本身的解释，不再附带图形值公式。
func TestR132AbilityRadarShowsRealGapWithoutFormulaText(t *testing.T) {
	if score := abilityRadarScore(767, 988); score > 40 || score < 35 {
		t.Fatalf("767 vs 988 radar score = %.1f, want a visible gap below 40", score)
	}
	if abilityRadarScore(5, 5) != 50 || abilityRadarScore(0, 5) != 0 {
		t.Fatal("baseline must stay 50 and a real zero must stay 0")
	}
	var p, b gameplayAbilityAccumulator
	item := abilityTestMatch(1).Participants[0]
	team := gameplayTeam{Kills: 20, Damage: 20000}
	p.add(item, team, 1200)
	b.add(item, team, 1200)
	for _, metric := range abilityMetrics(p, b) {
		if strings.Contains(metric.Description, "图形值") || strings.Contains(metric.Description, "百分位") {
			t.Fatalf("%s tooltip still carries formula text: %q", metric.Key, metric.Description)
		}
	}
}
