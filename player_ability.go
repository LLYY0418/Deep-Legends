package main

import (
	"math"
	"strings"
)

const minimumAbilitySampleGames = 3

type gameplayAbilityAccumulator struct {
	games                    int
	kills, deaths, assists   int
	damage, cs, gold, vision int
	teamKills, teamDamage    int
	duration                 int64
}

func (a *gameplayAbilityAccumulator) add(player gameplayParticipant, team gameplayTeam, duration int64) {
	a.games++
	a.kills += player.Kills
	a.deaths += player.Deaths
	a.assists += player.Assists
	a.damage += player.Damage
	a.cs += player.CS
	a.gold += player.Gold
	a.vision += player.VisionScore
	a.teamKills += team.Kills
	a.teamDamage += team.Damage
	a.duration += duration
}

func (a *gameplayAbilityAccumulator) addSide(side seasonRankedAbilitySide, duration int64) {
	a.games++
	a.kills += side.Kills
	a.deaths += side.Deaths
	a.assists += side.Assists
	a.damage += side.Damage
	a.cs += side.CS
	a.gold += side.Gold
	a.vision += side.Vision
	a.teamKills += side.TeamKills
	a.teamDamage += side.TeamDamage
	a.duration += duration
}

func abilityTeam(match gameplayMatch, teamID int64) gameplayTeam {
	for _, team := range match.Teams {
		if team.TeamID == teamID {
			return team
		}
	}
	team := gameplayTeam{TeamID: teamID}
	for _, player := range match.Participants {
		if player.TeamID != teamID {
			continue
		}
		team.Kills += player.Kills
		team.Damage += player.Damage
	}
	return team
}

func gameplayAbilityRank(ranks []gameplayRank, queueID int64) *gameplayRank {
	queueType := "RANKED_SOLO_5x5"
	if queueID == 440 {
		queueType = "RANKED_FLEX_SR"
	}
	for index := range ranks {
		if ranks[index].QueueType == queueType && strings.TrimSpace(ranks[index].Tier) != "" {
			return &ranks[index]
		}
	}
	return nil
}

func buildGameplayAbilityProfile(matches []gameplayMatch, playerRef string, ranks []gameplayRank, region string) *gameplayAbilityProfile {
	// 每个队列单独建立样本：单双排达到最低要求时优先采用；确实没有
	// 足够完整对局时再整体回退到灵活组排，不能把两个队列混成一张雷达图。
	for _, queueID := range []int64{420, 440} {
		if profile := buildGameplayAbilityProfileForQueue(matches, playerRef, ranks, region, queueID); profile != nil {
			return profile
		}
	}
	return nil
}

func buildGameplayAbilityProfileForQueue(matches []gameplayMatch, playerRef string, ranks []gameplayRank, region string, queueID int64) *gameplayAbilityProfile {
	playerStats, baselineStats, positions := gameplayAbilityStatsForQueue(matches, playerRef, queueID)
	return abilityProfileFrom(playerStats, baselineStats, positions, region, queueID)
}

// buildGameplayAbilityProfileForQueueWithSnapshot 在详情战绩样本不够时改用赛季
// 排位快照。首屏只拉 20 场详情，玩家最近 20 场里没有某个队列（灵活组排尤其
// 常见）时，「近 20 场排位」靠快照有数据、能力表现却空着，两张卡片自相矛盾。
// 快照里存的是与详情路径同口径的对位原料，因此两条路径可以互相替代。
func buildGameplayAbilityProfileForQueueWithSnapshot(matches []gameplayMatch, playerRef string, cached []seasonRankedMatch, ranks []gameplayRank, region string, queueID int64) *gameplayAbilityProfile {
	if profile := buildGameplayAbilityProfileForQueue(matches, playerRef, ranks, region, queueID); profile != nil {
		return profile
	}
	playerStats, baselineStats, positions := seasonAbilityStatsForQueue(cached, queueID)
	return abilityProfileFrom(playerStats, baselineStats, positions, region, queueID)
}

// seasonAbilityStatsForQueue 把赛季快照折叠成与 gameplayAbilityStatsForQueue
// 完全相同的三元组。没有 Ability 字段的老快照条目直接跳过。
func seasonAbilityStatsForQueue(cached []seasonRankedMatch, queueID int64) (gameplayAbilityAccumulator, gameplayAbilityAccumulator, map[string]int) {
	var playerStats, baselineStats gameplayAbilityAccumulator
	positions := make(map[string]int)
	for _, item := range cached {
		if item.QueueID != queueID || item.Ability == nil || item.Ability.Duration <= 0 {
			continue
		}
		position := strings.ToLower(strings.TrimSpace(item.Position))
		if position == "" || position == "other" {
			continue
		}
		playerStats.addSide(item.Ability.Player, item.Ability.Duration)
		positions[position]++
		if item.Ability.Opponent != nil {
			baselineStats.addSide(*item.Ability.Opponent, item.Ability.Duration)
		}
	}
	return playerStats, baselineStats, positions
}

func abilityProfileFrom(playerStats, baselineStats gameplayAbilityAccumulator, positions map[string]int, region string, queueID int64) *gameplayAbilityProfile {
	if playerStats.games < minimumAbilitySampleGames || baselineStats.games < minimumAbilitySampleGames {
		return nil
	}

	position := ""
	for key, count := range positions {
		if position == "" || count > positions[position] {
			position = key
		}
	}
	positionLabels := map[string]string{"top": "上路", "jungle": "打野", "middle": "中路", "bottom": "下路", "utility": "辅助"}
	baselineLabel := "近期同位置对手样本"
	sourceLabel := "七项指标参考 OP.GG · 本机国服对局计算"
	if region == riotRegionKR {
		sourceLabel = "七项指标参考 OP.GG · Riot 韩服对局计算"
	}
	metrics := abilityMetrics(playerStats, baselineStats)
	if len(metrics) != 7 {
		return nil
	}
	return &gameplayAbilityProfile{
		SampleGames: playerStats.games, BaselineGames: baselineStats.games,
		QueueID: queueID, QueueLabel: rankedQueueLabel(queueID),
		Position: position, PositionLabel: positionLabels[position], BaselineLabel: baselineLabel,
		SourceLabel: sourceLabel, Metrics: metrics,
	}
}

func gameplayAbilityStatsForQueue(matches []gameplayMatch, playerRef string, queueID int64) (gameplayAbilityAccumulator, gameplayAbilityAccumulator, map[string]int) {
	var playerStats, baselineStats gameplayAbilityAccumulator
	positions := make(map[string]int)
	for _, match := range matches {
		if match.Result != "win" && match.Result != "loss" || match.Duration <= 0 {
			continue
		}
		if match.QueueID != queueID {
			continue
		}
		subject, ok := matchSubject(match, playerRef)
		position := strings.ToLower(strings.TrimSpace(subject.Position))
		if !ok || position == "" || position == "other" {
			continue
		}
		playerStats.add(subject, abilityTeam(match, subject.TeamID), match.Duration)
		positions[position]++
		for _, candidate := range match.Participants {
			if candidate.TeamID == subject.TeamID || strings.ToLower(strings.TrimSpace(candidate.Position)) != position {
				continue
			}
			baselineStats.add(candidate, abilityTeam(match, candidate.TeamID), match.Duration)
			break
		}
	}
	return playerStats, baselineStats, positions
}

func gameplayAbilitySampleGamesForQueue(matches []gameplayMatch, playerRef string, queueID int64) int {
	playerStats, baselineStats, _ := gameplayAbilityStatsForQueue(matches, playerRef, queueID)
	return minAbilityGames(playerStats, baselineStats)
}

// gameplayAbilitySampleGamesForQueueWithSnapshot 与
// buildGameplayAbilityProfileForQueueWithSnapshot 取同一份样本，
// 否则前端会拿详情样本数去解释快照算出来的雷达。
func gameplayAbilitySampleGamesForQueueWithSnapshot(matches []gameplayMatch, playerRef string, cached []seasonRankedMatch, queueID int64) int {
	playerStats, baselineStats, _ := gameplayAbilityStatsForQueue(matches, playerRef, queueID)
	if games := minAbilityGames(playerStats, baselineStats); games >= minimumAbilitySampleGames {
		return games
	}
	snapshotPlayer, snapshotBaseline, _ := seasonAbilityStatsForQueue(cached, queueID)
	if snapshotGames := minAbilityGames(snapshotPlayer, snapshotBaseline); snapshotGames >= minimumAbilitySampleGames {
		return snapshotGames
	}
	return minAbilityGames(playerStats, baselineStats)
}

func minAbilityGames(playerStats, baselineStats gameplayAbilityAccumulator) int {
	if playerStats.games < baselineStats.games {
		return playerStats.games
	}
	return baselineStats.games
}

func abilityMetrics(player, baseline gameplayAbilityAccumulator) []gameplayAbilityMetric {
	type spec struct {
		key, label, description, unit string
		player, baseline              float64
		precision                     int
	}
	specs := []spec{
		{"kda", "KDA", "平均击杀与助攻相对于死亡的比例。", "", ratio(player.kills+player.assists, player.deaths), ratio(baseline.kills+baseline.assists, baseline.deaths), 2},
		{"killParticipation", "参团率", "参与击杀数占所在队伍总击杀的比例。", "%", abilityRatio(player.kills+player.assists, player.teamKills, 100), abilityRatio(baseline.kills+baseline.assists, baseline.teamKills, 100), 1},
		{"damageShare", "伤害占比", "对英雄伤害占所在队伍英雄总伤害的比例。", "%", abilityRatio(player.damage, player.teamDamage, 100), abilityRatio(baseline.damage, baseline.teamDamage, 100), 1},
		{"dpm", "DPM", "每分钟对英雄造成的平均伤害。", "", perMinute(player.damage, player.duration), perMinute(baseline.damage, baseline.duration), 0},
		{"csm", "CSM", "每分钟获得的小兵与野怪补刀数。", "", perMinute(player.cs, player.duration), perMinute(baseline.cs, baseline.duration), 2},
		{"gpm", "GPM", "每分钟获得的平均金币。", "", perMinute(player.gold, player.duration), perMinute(baseline.gold, baseline.duration), 0},
		{"vspm", "VSPM", "每分钟获得的平均视野得分。", "", perMinute(player.vision, player.duration), perMinute(baseline.vision, baseline.duration), 2},
	}
	metrics := make([]gameplayAbilityMetric, 0, len(specs))
	for _, item := range specs {
		if item.baseline <= 0 || math.IsNaN(item.player) || math.IsInf(item.player, 0) {
			continue
		}
		ratioToBaseline := item.player / item.baseline
		metrics = append(metrics, gameplayAbilityMetric{
			Key: item.key, Label: item.label, Description: item.description, Unit: item.unit,
			Player: abilityRound(item.player, item.precision), Baseline: abilityRound(item.baseline, item.precision),
			PlayerScore: abilityRound(math.Max(16, math.Min(100, 62*ratioToBaseline)), 1),
			Grade:       abilityGrade(ratioToBaseline),
		})
	}
	return metrics
}

func abilityRatio(numerator, denominator int, scale float64) float64 {
	if denominator <= 0 {
		return 0
	}
	return float64(numerator) * scale / float64(denominator)
}

func abilityRound(value float64, precision int) float64 {
	factor := math.Pow10(precision)
	return math.Round(value*factor) / factor
}

func abilityGrade(ratioToBaseline float64) string {
	switch {
	case ratioToBaseline >= 1.35:
		return "S"
	case ratioToBaseline >= 1.20:
		return "A+"
	case ratioToBaseline >= 1.10:
		return "A"
	case ratioToBaseline >= 1.03:
		return "A-"
	case ratioToBaseline >= 0.97:
		return "B+"
	case ratioToBaseline >= 0.90:
		return "B"
	case ratioToBaseline >= 0.82:
		return "B-"
	case ratioToBaseline >= 0.74:
		return "C+"
	case ratioToBaseline >= 0.66:
		return "C"
	default:
		return "C-"
	}
}

func gameplayRankTitleZH(rank gameplayRank) string {
	tier := strings.ToUpper(strings.TrimSpace(rank.Tier))
	name := map[string]string{
		"IRON": "黑铁", "BRONZE": "青铜", "SILVER": "白银", "GOLD": "黄金", "PLATINUM": "铂金",
		"EMERALD": "翡翠", "DIAMOND": "钻石", "MASTER": "大师", "GRANDMASTER": "宗师", "CHALLENGER": "王者",
	}[tier]
	if name == "" {
		name = rank.Tier
	}
	if rank.Division != "" && tier != "MASTER" && tier != "GRANDMASTER" && tier != "CHALLENGER" {
		name += " " + rank.Division
	}
	return strings.TrimSpace(name)
}
