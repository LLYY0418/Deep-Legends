package main

import "testing"

func abilityTestMatch(id int64) gameplayMatch {
	return gameplayMatch{
		GameID: id, Duration: 1200, QueueID: 420, Result: "win", SubjectParticipantID: 1,
		Participants: []gameplayParticipant{
			{ParticipantID: 1, TeamID: 100, PlayerRef: "subject", Position: "top", Kills: 4, Deaths: 2, Assists: 6, Damage: 8000, CS: 160, Gold: 9000, VisionScore: 20},
			{ParticipantID: 6, TeamID: 200, PlayerRef: "opponent", Position: "top", Kills: 2, Deaths: 3, Assists: 3, Damage: 5000, CS: 120, Gold: 7000, VisionScore: 15},
		},
		Teams: []gameplayTeam{
			{TeamID: 100, Kills: 20, Damage: 20000},
			{TeamID: 200, Kills: 15, Damage: 18000},
		},
	}
}

func TestBuildGameplayAbilityProfileUsesSamePositionRankedBaseline(t *testing.T) {
	matches := []gameplayMatch{abilityTestMatch(1), abilityTestMatch(2), abilityTestMatch(3)}
	ranks := []gameplayRank{{QueueType: "RANKED_SOLO_5x5", Tier: "DIAMOND", Division: "I"}}
	profile := buildGameplayAbilityProfile(matches, "subject", ranks, riotRegionKR)
	if profile == nil {
		t.Fatal("ability profile is nil")
	}
	if profile.SampleGames != 3 || profile.BaselineGames != 3 || profile.PositionLabel != "上路" {
		t.Fatalf("profile samples = %#v", profile)
	}
	if profile.QueueID != 420 || profile.QueueLabel != "单双排" {
		t.Fatalf("profile queue = %#v", profile)
	}
	if profile.BaselineLabel != "近期同位置对手样本" || profile.SourceLabel != "七项指标参考 OP.GG · Riot 韩服对局计算" {
		t.Fatalf("profile labels = %#v", profile)
	}
	cnProfile := buildGameplayAbilityProfile(matches, "subject", ranks, "")
	if cnProfile == nil || cnProfile.SourceLabel != "七项指标参考 OP.GG · 本机国服对局计算" {
		t.Fatalf("CN profile source = %#v", cnProfile)
	}
	if len(profile.Metrics) != 7 {
		t.Fatalf("metrics = %#v", profile.Metrics)
	}
	if got := profile.Metrics[0]; got.Key != "kda" || got.Player != 5 || got.Baseline != 1.67 || got.Grade != "S" {
		t.Fatalf("KDA metric = %#v", got)
	}
	if got := profile.Metrics[2]; got.Player != 40 || got.Baseline != 27.8 {
		t.Fatalf("damage share metric = %#v", got)
	}
}

func TestBuildGameplayAbilityProfileRequiresThreeComparableGames(t *testing.T) {
	matches := []gameplayMatch{abilityTestMatch(1), abilityTestMatch(2)}
	if got := buildGameplayAbilityProfile(matches, "subject", nil, ""); got != nil {
		t.Fatalf("short sample profile = %#v", got)
	}
}

func TestBuildGameplayAbilityProfileIgnoresNonRankedMatches(t *testing.T) {
	matches := []gameplayMatch{abilityTestMatch(1), abilityTestMatch(2), abilityTestMatch(3)}
	for index, queueID := range []int64{400, 450, 1700} {
		match := abilityTestMatch(int64(index + 4))
		match.QueueID = queueID
		matches = append(matches, match)
	}
	ranks := []gameplayRank{{QueueType: "RANKED_SOLO_5x5", Tier: "DIAMOND", Division: "I"}}
	profile := buildGameplayAbilityProfile(matches, "subject", ranks, "")
	if profile == nil {
		t.Fatal("ability profile is nil")
	}
	if profile.SampleGames != 3 || profile.BaselineGames != 3 {
		t.Fatalf("non-ranked matches affected ability samples = %#v", profile)
	}
}

func TestBuildGameplayAbilityProfileFallsBackToFlexWhenSoloSamplesAreInsufficient(t *testing.T) {
	matches := []gameplayMatch{abilityTestMatch(1), abilityTestMatch(2)}
	for id := int64(3); id <= 5; id++ {
		match := abilityTestMatch(id)
		match.QueueID = 440
		matches = append(matches, match)
	}
	ranks := []gameplayRank{
		{QueueType: "RANKED_SOLO_5x5", Tier: "DIAMOND", Division: "I"},
		{QueueType: "RANKED_FLEX_SR", Tier: "EMERALD", Division: "II"},
	}
	profile := buildGameplayAbilityProfile(matches, "subject", ranks, "")
	if profile == nil || profile.QueueID != 440 || profile.QueueLabel != "灵活组排" || profile.SampleGames != 3 {
		t.Fatalf("flex fallback profile = %#v", profile)
	}
	if profile.BaselineLabel != "近期同位置对手样本" {
		t.Fatalf("flex rank label = %#v", profile)
	}
}

func TestAbilityGradeRange(t *testing.T) {
	for _, test := range []struct {
		ratio float64
		want  string
	}{{1.4, "S"}, {1.2, "A+"}, {1.0, "B+"}, {0.85, "B-"}, {0.5, "C-"}} {
		if got := abilityGrade(test.ratio); got != test.want {
			t.Fatalf("abilityGrade(%v) = %q, want %q", test.ratio, got, test.want)
		}
	}
}

// 赛季扫描顺手记下的排位快照必须能独立撑起能力雷达。
// 首屏只拉 20 场详情，玩家最近 20 场里没有灵活组排时，「近 20 场排位」
// 靠这份快照有数据、能力表现却空着，两张卡片自相矛盾——这正是用户
// 「近 20 场排位有灵活数据，为什么能力表现没有」的成因。
func seasonAbilityTestMatch(gameID int64, queueID int64) seasonRankedMatch {
	return seasonRankedMatch{
		GameID: gameID, CreatedAt: gameID, QueueID: queueID, Position: "top", Win: true,
		Kills: 4, Deaths: 2, Assists: 6, TeamKills: 20,
		Ability: &seasonRankedAbilitySample{
			Duration: 1200,
			Player:   seasonRankedAbilitySide{Kills: 4, Deaths: 2, Assists: 6, Damage: 8000, CS: 160, Gold: 9000, Vision: 20, TeamKills: 20, TeamDamage: 20000},
			Opponent: &seasonRankedAbilitySide{Kills: 2, Deaths: 3, Assists: 3, Damage: 5000, CS: 120, Gold: 7000, Vision: 15, TeamKills: 15, TeamDamage: 18000},
		},
	}
}

func TestAbilityProfileFallsBackToSeasonSnapshotWhenDetailWindowHasNoFlexGames(t *testing.T) {
	// 详情窗口里一场灵活组排都没有（真机日志实测：队列只有 420/1750/2400）。
	detailMatches := []gameplayMatch{abilityTestMatch(1), abilityTestMatch(2), abilityTestMatch(3)}
	cached := []seasonRankedMatch{seasonAbilityTestMatch(11, 440), seasonAbilityTestMatch(12, 440), seasonAbilityTestMatch(13, 440)}

	if got := buildGameplayAbilityProfileForQueue(detailMatches, "subject", nil, riotRegionKR, 440); got != nil {
		t.Fatalf("detail-only flex profile should still be nil: %#v", got)
	}
	profile := buildGameplayAbilityProfileForQueueWithSnapshot(detailMatches, "subject", cached, nil, riotRegionKR, 440)
	if profile == nil {
		t.Fatal("flex ability profile must come from the season snapshot")
	}
	if profile.QueueID != 440 || profile.QueueLabel != "灵活组排" || profile.PositionLabel != "上路" {
		t.Fatalf("snapshot profile = %#v", profile)
	}
	if profile.SampleGames != 3 || profile.BaselineGames != 3 || len(profile.Metrics) != 7 {
		t.Fatalf("snapshot samples = %#v", profile)
	}
	// 口径必须与详情路径完全一致，否则单双排和灵活组排的雷达没法互相比较。
	detail := buildGameplayAbilityProfileForQueue(detailMatches, "subject", nil, riotRegionKR, 420)
	if detail == nil {
		t.Fatal("solo detail profile is nil")
	}
	for index := range detail.Metrics {
		if detail.Metrics[index].Player != profile.Metrics[index].Player || detail.Metrics[index].Baseline != profile.Metrics[index].Baseline {
			t.Fatalf("metric %s drifted between the detail and snapshot paths: %#v vs %#v", detail.Metrics[index].Key, detail.Metrics[index], profile.Metrics[index])
		}
	}
	if got := gameplayAbilitySampleGamesForQueueWithSnapshot(detailMatches, "subject", cached, 440); got != 3 {
		t.Fatalf("snapshot sample games = %d", got)
	}
}

func TestSeasonSnapshotAbilityIgnoresEntriesWithoutTheNewFields(t *testing.T) {
	// 老缓存（schemaVersion < 5）没有 Ability 字段，必须整条跳过而不是当成 0 场统计。
	legacy := []seasonRankedMatch{{QueueID: 440, Position: "top"}, {QueueID: 440, Position: "top"}, {QueueID: 440, Position: "top"}}
	if got := buildGameplayAbilityProfileForQueueWithSnapshot(nil, "subject", legacy, nil, riotRegionKR, 440); got != nil {
		t.Fatalf("legacy snapshot entries must not produce a profile: %#v", got)
	}
	// 有对位样本但不足 3 场时同样不出图。
	short := []seasonRankedMatch{seasonAbilityTestMatch(11, 440), seasonAbilityTestMatch(12, 440)}
	if got := buildGameplayAbilityProfileForQueueWithSnapshot(nil, "subject", short, nil, riotRegionKR, 440); got != nil {
		t.Fatalf("short snapshot sample must not produce a profile: %#v", got)
	}
	// 只有本人没有对位（同位置对手缺失）时也不出图：没有基线就没有雷达。
	noOpponent := []seasonRankedMatch{seasonAbilityTestMatch(11, 440), seasonAbilityTestMatch(12, 440), seasonAbilityTestMatch(13, 440)}
	for index := range noOpponent {
		noOpponent[index].Ability.Opponent = nil
	}
	if got := buildGameplayAbilityProfileForQueueWithSnapshot(nil, "subject", noOpponent, nil, riotRegionKR, 440); got != nil {
		t.Fatalf("snapshot without lane opponents must not produce a profile: %#v", got)
	}
	// 队列必须隔离：灵活组排的快照不能被拿去填单双排。
	if got := buildGameplayAbilityProfileForQueueWithSnapshot(nil, "subject", []seasonRankedMatch{seasonAbilityTestMatch(11, 440), seasonAbilityTestMatch(12, 440), seasonAbilityTestMatch(13, 440)}, nil, riotRegionKR, 420); got != nil {
		t.Fatalf("flex snapshot leaked into the solo queue: %#v", got)
	}
}

func TestSeasonRecordRankedMatchCapturesAbilitySample(t *testing.T) {
	info := &riotMatchInfo{
		GameID: 77, GameCreation: 1_700_000_000_000, QueueID: 440, GameDuration: 1200,
		Participants: []riotParticipant{
			{PUUID: "subject", TeamID: 100, ChampionID: 1, TeamPosition: "TOP", Kills: 4, Deaths: 2, Assists: 6, TotalDamageDealtToChampions: 8000, TotalMinionsKilled: 150, NeutralMinionsKilled: 10, GoldEarned: 9000, VisionScore: 20, Win: true},
			{PUUID: "mate", TeamID: 100, ChampionID: 2, TeamPosition: "MIDDLE", Kills: 16, TotalDamageDealtToChampions: 12000},
			{PUUID: "opponent", TeamID: 200, ChampionID: 3, TeamPosition: "TOP", Kills: 2, Deaths: 3, Assists: 3, TotalDamageDealtToChampions: 5000, TotalMinionsKilled: 110, NeutralMinionsKilled: 10, GoldEarned: 7000, VisionScore: 15},
			{PUUID: "other", TeamID: 200, ChampionID: 4, TeamPosition: "MIDDLE", Kills: 13, TotalDamageDealtToChampions: 13000},
		},
	}
	cache := &seasonStatsCache{}
	seasonRecordRankedMatch(cache, info, "subject")
	if len(cache.RankedMatches) != 1 {
		t.Fatalf("ranked matches = %#v", cache.RankedMatches)
	}
	sample := cache.RankedMatches[0].Ability
	if sample == nil || sample.Duration != 1200 {
		t.Fatalf("ability sample = %#v", sample)
	}
	if sample.Player.Damage != 8000 || sample.Player.CS != 160 || sample.Player.TeamKills != 20 || sample.Player.TeamDamage != 20000 {
		t.Fatalf("player side = %#v", sample.Player)
	}
	// 对位必须取敌方同位置那一个，取错队伍或取错位置都会让基线失去意义。
	if sample.Opponent == nil || sample.Opponent.Damage != 5000 || sample.Opponent.CS != 120 || sample.Opponent.TeamKills != 15 || sample.Opponent.TeamDamage != 18000 {
		t.Fatalf("opponent side = %#v", sample.Opponent)
	}
}

func TestBuildGameplayRankedQueuesUsesOnlyTheSharedRecentSample(t *testing.T) {
	detailMatches := []gameplayMatch{abilityTestMatch(1), abilityTestMatch(2), abilityTestMatch(3)}
	for index := range detailMatches {
		detailMatches[index].QueueID = 440
	}
	queues := buildGameplayRankedQueues(legacyRankedQueueTabs(detailMatches, detailMatches), "subject", nil, riotRegionKR)
	flex, ok := queues["440"]
	if !ok {
		t.Fatal("flex queue entry missing")
	}
	if flex.Ability == nil {
		t.Fatal("flex ability must use the same recent sample as the summary")
	}
	if flex.Ability.QueueID != 440 || flex.AbilitySampleGames != 3 {
		t.Fatalf("flex queue stats = %#v", flex)
	}
	if solo := queues["420"]; solo.Ability != nil || solo.AbilitySampleGames != 0 || solo.RecentRanked.Games != 0 {
		t.Fatalf("season data or flex data leaked into solo queue stats: %#v", solo)
	}
}

func TestBuildGameplayRankedQueuesCapsEveryRecentCardAtTwentyMatches(t *testing.T) {
	matches := make([]gameplayMatch, 0, 25)
	for id := int64(1); id <= 25; id++ {
		match := abilityTestMatch(id)
		match.QueueID = 420
		match.CreatedAt = id
		matches = append(matches, match)
	}

	solo := buildGameplayRankedQueues(legacyRankedQueueTabs(matches, matches), "subject", nil, riotRegionKR)["420"]
	if solo.RecentRanked == nil || solo.RecentRanked.Games != defaultMatchCount {
		t.Fatalf("recent ranked sample = %#v, want %d games", solo.RecentRanked, defaultMatchCount)
	}
	if solo.Ability == nil || solo.Ability.SampleGames != defaultMatchCount || solo.AbilitySampleGames != defaultMatchCount {
		t.Fatalf("ability sample did not share the 20-match window: %#v", solo)
	}
	if got := positionStatsGames(solo.Positions); got != defaultMatchCount {
		t.Fatalf("position sample games = %d, want %d", got, defaultMatchCount)
	}
}
