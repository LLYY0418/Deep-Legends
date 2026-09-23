package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// 赛季扫描顺手留下的单场快照必须和 seasonStatsAccumulate 用同一套判据
// （seasonStatsQueueAllowed：420/440 + 海克斯大乱斗 2300/2400/3270，且排除
// 重开局），否则「近 20 场」会和赛季英雄统计对不上账。R116-E 放开海斗后
// 这条同步约束对新增的三个队列同样适用。
func TestSeasonRecordRankedMatchMirrorsSeasonStatsCriteria(t *testing.T) {
	subject := riotParticipant{PUUID: "subject", TeamID: 100, ChampionID: 13, Win: true, Kills: 8, Deaths: 2, Assists: 6, TeamPosition: "MIDDLE"}
	mate := riotParticipant{PUUID: "mate", TeamID: 100, Kills: 4}
	enemy := riotParticipant{PUUID: "enemy", TeamID: 200, Kills: 20}
	base := func(id, queue int64) *riotMatchInfo {
		return &riotMatchInfo{GameID: id, QueueID: queue, GameDuration: 1800, GameCreation: seasonStartS26.Add(time.Hour).UnixMilli(), Participants: []riotParticipant{subject, mate, enemy}}
	}
	cache := &seasonStatsCache{}
	seasonRecordRankedMatch(cache, base(1, 420), "subject")
	seasonRecordRankedMatch(cache, base(2, 440), "subject")
	// 非排位队列（大乱斗 450）不能混进排位样本。
	seasonRecordRankedMatch(cache, base(3, 450), "subject")
	// 别人的对局里没有 subject，不该记任何东西。
	seasonRecordRankedMatch(cache, &riotMatchInfo{GameID: 4, QueueID: 420, GameDuration: 1800, GameCreation: seasonStartS26.Add(time.Hour).UnixMilli(), Participants: []riotParticipant{enemy}}, "subject")
	if len(cache.RankedMatches) != 2 {
		t.Fatalf("expected only the two ranked games, got %#v", cache.RankedMatches)
	}
	first := cache.RankedMatches[0]
	// 参团率要的是本队总击杀，不能把敌方的 20 杀算进来。
	if first.TeamKills != 12 {
		t.Fatalf("team kills = %d, want 12 (own team only)", first.TeamKills)
	}
	if first.Position != "middle" || first.Kills != 8 || first.Deaths != 2 || first.Assists != 6 || !first.Win {
		t.Fatalf("unexpected ranked snapshot: %#v", first)
	}
}

func TestSeasonQuerySnapshotCacheEvictsOldestEntry(t *testing.T) {
	a := &app{}
	now := time.Now()
	a.seasonBackfillMu.Lock()
	for index := 0; index < seasonQuerySnapshotsMax+1; index++ {
		a.cacheSeasonQuerySnapshotLocked(fmt.Sprintf("snapshot-%03d", index), now)
	}
	a.seasonBackfillMu.Unlock()
	if len(a.seasonQuerySnapshots) != seasonQuerySnapshotsMax {
		t.Fatalf("season query snapshot cache size = %d, want %d", len(a.seasonQuerySnapshots), seasonQuerySnapshotsMax)
	}
	if _, exists := a.seasonQuerySnapshots["snapshot-000"]; exists {
		t.Fatal("oldest season query snapshot was not evicted")
	}
}

func TestSeasonTrimRankedMatchesDedupesAndKeepsNewestFirst(t *testing.T) {
	cache := &seasonStatsCache{RankedMatches: []seasonRankedMatch{
		{GameID: 1, CreatedAt: 100}, {GameID: 3, CreatedAt: 300},
		{GameID: 1, CreatedAt: 100}, {GameID: 2, CreatedAt: 200},
	}}
	seasonTrimRankedMatches(cache)
	if len(cache.RankedMatches) != 3 {
		t.Fatalf("dedupe failed: %#v", cache.RankedMatches)
	}
	if cache.RankedMatches[0].GameID != 3 || cache.RankedMatches[2].GameID != 1 {
		t.Fatalf("not sorted newest-first: %#v", cache.RankedMatches)
	}
	cache.RankedMatches = nil
	for i := 0; i < seasonRankedMatchLimit+15; i++ {
		cache.RankedMatches = append(cache.RankedMatches, seasonRankedMatch{GameID: int64(i + 1), CreatedAt: int64(i + 1)})
	}
	seasonTrimRankedMatches(cache)
	if len(cache.RankedMatches) != seasonRankedMatchLimit {
		t.Fatalf("limit not applied: %d", len(cache.RankedMatches))
	}
}

func TestSeasonTrimRankedMatchesKeepsEachRankedQueue(t *testing.T) {
	cache := &seasonStatsCache{}
	for index := 0; index < 41; index++ {
		cache.RankedMatches = append(cache.RankedMatches, seasonRankedMatch{
			GameID: int64(index + 1), QueueID: 420, CreatedAt: int64(10_000 + index),
		})
	}
	for index := 0; index < 3; index++ {
		cache.RankedMatches = append(cache.RankedMatches, seasonRankedMatch{
			GameID: int64(100 + index), QueueID: 440, CreatedAt: int64(100 + index),
		})
	}
	snapshots := seasonTrimRankedMatches(cache)
	counts := map[int64]int{}
	for _, match := range cache.RankedMatches {
		counts[match.QueueID]++
	}
	if counts[420] != seasonRankedMatchLimit || counts[440] != 3 {
		t.Fatalf("per-queue snapshot was mixed before trimming: counts=%#v matches=%#v", counts, cache.RankedMatches)
	}
	if !snapshots[420].Capped || snapshots[440].Capped {
		t.Fatalf("snapshot cap diagnostics = %#v", snapshots)
	}
}

// 近 20 场排位必须能用赛季缓存补足样本：首屏那 20 条战绩里可能只有几场排位
// （真机日志出现过「实际 8 场」），但不能因此把同一场重复计两次。
func TestRecentRankedSummaryTopsUpFromSeasonCacheWithoutDoubleCounting(t *testing.T) {
	playerRef := "subject"
	live := []gameplayMatch{{
		GameID: 1, QueueID: 420, Result: "win", CreatedAt: 5000,
		Participants: []gameplayParticipant{
			{ParticipantID: 1, PlayerRef: playerRef, TeamID: 100, Position: "top", Kills: 6, Deaths: 1, Assists: 2, Win: true},
			{ParticipantID: 2, TeamID: 100, Kills: 4},
		},
	}}
	cached := []seasonRankedMatch{
		// 同一场（GameID 1）也在缓存里，必须去重，不能算成两场。
		{GameID: 1, CreatedAt: 5000, QueueID: 420, Win: true, Kills: 6, Deaths: 1, Assists: 2, TeamKills: 10, Position: "top"},
		{GameID: 2, CreatedAt: 4000, QueueID: 420, Win: false, Kills: 2, Deaths: 5, Assists: 3, TeamKills: 10, Position: "top"},
		// 有单双排样本时，灵活组排不能混进同一张统计卡。
		{GameID: 3, CreatedAt: 3000, QueueID: 440, Win: true, Kills: 4, Deaths: 4, Assists: 4, TeamKills: 10, Position: "jungle"},
		// 非排位队列不能混进来。
		{GameID: 4, CreatedAt: 2000, QueueID: 450, Win: true, Kills: 9, Deaths: 0, Assists: 9},
	}
	got := recentRankedSummary(live, playerRef, cached)
	if got.Games != 2 || got.Wins != 1 || got.Losses != 1 || got.QueueID != 420 {
		t.Fatalf("season cache top-up wrong: %#v", got)
	}
	// 只有首屏样本时结果应该只剩那一场，证明补足确实来自缓存。
	if bare := recentRankedSummary(live, playerRef, nil); bare.Games != 1 {
		t.Fatalf("live-only sample should stay at 1 game, got %#v", bare)
	}
}

func TestRankedSeasonStartDerivesAnnualSeason(t *testing.T) {
	season, start := currentRankedSeason(time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC))
	if season != "S26" || !start.Equal(seasonStartS26) {
		t.Fatalf("current season = %s %v", season, start)
	}
	season, start = currentRankedSeason(time.Date(2027, 2, 1, 0, 0, 0, 0, time.UTC))
	if season != "S27" || start.Year() != 2027 {
		t.Fatalf("derived season = %s %v", season, start)
	}
}

func TestSeasonStatsAggregateRanksOnlyAndOmitsPUUID(t *testing.T) {
	stats := map[int64]*gameplaySeasonChampionStat{}
	queueStats := map[int64]gameplayAggregate{}
	info := &riotMatchInfo{GameID: 1, GameCreation: seasonStartS26.Add(time.Hour).UnixMilli(), GameDuration: 1800, QueueID: 420, Participants: []riotParticipant{{PUUID: "subject", ChampionID: 13, Win: true, Kills: 8, Deaths: 2, Assists: 6, TotalMinionsKilled: 180, NeutralMinionsKilled: 20}}}
	seasonStatsAccumulate(stats, queueStats, info, "subject", seasonStartS26.UnixMilli())
	seasonStatsAccumulate(stats, queueStats, &riotMatchInfo{GameID: 3, GameCreation: seasonStartS26.Add(-time.Hour).UnixMilli(), QueueID: 420, Participants: []riotParticipant{{PUUID: "subject", ChampionID: 99, Win: true}}}, "subject", seasonStartS26.UnixMilli())
	seasonStatsAccumulate(stats, queueStats, &riotMatchInfo{GameID: 2, GameCreation: seasonStartS26.Add(time.Hour).UnixMilli(), QueueID: 450, Participants: info.Participants}, "subject", seasonStartS26.UnixMilli())
	rows := seasonStatsFinalize(stats, map[int64]string{13: "瑞兹"})
	if len(rows) != 1 || rows[0].Games != 1 || rows[0].Wins != 1 || rows[0].ChampionName != "瑞兹" {
		t.Fatalf("unexpected season rows: %#v", rows)
	}
	if rows[0].CS != 200 || rows[0].CSPerMinute != 6.7 || rows[0].TotalCS != 200 || rows[0].Duration != 1800 {
		t.Fatalf("season CS aggregate = %#v", rows[0])
	}
	overall := seasonStatsOverall(rows)
	if overall.CS != 200 || overall.CSPerMinute != 6.7 {
		t.Fatalf("season overall CS = %#v", overall)
	}
	queueStats = seasonStatsFinalizeQueues(queueStats)
	if queueStats[420].Games != 1 || queueStats[420].Wins != 1 || queueStats[420].Losses != 0 || queueStats[420].WinRate != 100 {
		t.Fatalf("season queue aggregate = %#v", queueStats)
	}
	encodedBytes, err := json.Marshal(rows)
	if err != nil {
		t.Fatal(err)
	}
	if encoded := string(encodedBytes); strings.Contains(encoded, "PUUID") || strings.Contains(encoded, "subject") {
		t.Fatalf("season aggregate leaked PUUID: %s", encoded)
	}
}

func TestSeasonStatsChampionKDAUsesPerGameAndWeightedOverall(t *testing.T) {
	stats := map[int64]*gameplaySeasonChampionStat{
		1: {ChampionID: 1, Games: 2, Wins: 1, Kills: 10, Deaths: 4, Assists: 6},
		2: {ChampionID: 2, Games: 1, Wins: 1, Kills: 10, Deaths: 2, Assists: 5},
	}
	rows := seasonStatsFinalize(stats, nil)
	if rows[0].Kills != 5 || rows[1].Kills != 10 {
		t.Fatalf("champion KDA was not normalized per game: %#v", rows)
	}
	overall := seasonStatsOverall(rows)
	if overall.Kills != 6.7 || overall.Deaths != 2 || overall.Assists != 3.7 {
		t.Fatalf("overall KDA was not weighted by games: %#v", overall)
	}
}

func TestSeasonStatsAccumulateUsesRequestedPlayerReference(t *testing.T) {
	requestedRef := strings.Repeat("other-player", 4)
	stats := map[int64]*gameplaySeasonChampionStat{}
	queueStats := map[int64]gameplayAggregate{}
	seasonStatsAccumulate(stats, queueStats, &riotMatchInfo{
		GameID: 77, GameCreation: seasonStartS26.Add(time.Hour).UnixMilli(), GameDuration: 1800, QueueID: 440,
		Participants: []riotParticipant{
			{PUUID: strings.Repeat("current-user", 4), ChampionID: 1, Win: true, Kills: 20},
			{PUUID: requestedRef, ChampionID: 13, Win: false, Kills: 2, Deaths: 5, Assists: 4},
		},
	}, requestedRef, seasonStartS26.UnixMilli())
	if len(stats) != 1 || stats[13] == nil || stats[13].Games != 1 || stats[13].Wins != 0 || stats[13].Kills != 2 {
		t.Fatalf("season stats did not isolate the requested non-current player: %#v", stats)
	}
	if _, leaked := stats[1]; leaked || queueStats[440].Games != 1 || queueStats[440].Wins != 0 {
		t.Fatalf("current player leaked into requested player's season stats: stats=%#v queues=%#v", stats, queueStats)
	}
}

func TestSeasonStatsAccumulateAllowsNilQueueStats(t *testing.T) {
	stats := map[int64]*gameplaySeasonChampionStat{}
	seasonStatsAccumulate(stats, nil, &riotMatchInfo{
		GameID: 78, GameCreation: seasonStartS26.Add(time.Hour).UnixMilli(), GameDuration: 1800, QueueID: 420,
		Participants: []riotParticipant{{PUUID: "subject", ChampionID: 13, Win: true}},
	}, "subject", seasonStartS26.UnixMilli())
	if stats[13] == nil || stats[13].Games != 1 || stats[13].Wins != 1 {
		t.Fatalf("nil queue aggregate prevented champion accumulation: %#v", stats)
	}
}

func TestSeasonStatsReadSGPTopLevelCSFields(t *testing.T) {
	var info riotMatchInfo
	if err := json.Unmarshal([]byte(`{"gameId":12,"gameCreation":1767859200000,"gameDuration":1500,"queueId":420,"participants":[{"puuid":"subject","championId":64,"totalMinionsKilled":143,"neutralMinionsKilled":27}]}`), &info); err != nil {
		t.Fatal(err)
	}
	stats := map[int64]*gameplaySeasonChampionStat{}
	seasonStatsAccumulate(stats, map[int64]gameplayAggregate{}, &info, "subject", seasonStartS26.UnixMilli())
	rows := seasonStatsFinalize(stats, nil)
	if len(rows) != 1 || rows[0].CS != 170 || rows[0].CSPerMinute != 6.8 {
		t.Fatalf("SGP CS fields were not aggregated: %#v", rows)
	}
}

func TestSeasonStatsExcludeRemakes(t *testing.T) {
	stats := map[int64]*gameplaySeasonChampionStat{}
	queueStats := map[int64]gameplayAggregate{}
	createdAt := seasonStartS26.Add(time.Hour).UnixMilli()
	seasonStatsAccumulate(stats, queueStats, &riotMatchInfo{
		GameID: 10, GameCreation: createdAt, GameDuration: 185, QueueID: 420, GameMode: "CLASSIC", GameType: "MATCHED_GAME",
		Participants: []riotParticipant{{PUUID: "subject", ChampionID: 64, Win: true, GameEndedInEarlySurrender: true}},
	}, "subject", seasonStartS26.UnixMilli())
	seasonStatsAccumulate(stats, queueStats, &riotMatchInfo{
		GameID: 11, GameCreation: createdAt, GameDuration: 180, QueueID: 420, GameMode: "CLASSIC", GameType: "MATCHED_GAME",
		Participants: []riotParticipant{{PUUID: "subject", ChampionID: 64}},
	}, "subject", seasonStartS26.UnixMilli())
	if len(stats) != 0 {
		t.Fatalf("remakes entered season stats: %#v", stats)
	}
	if len(queueStats) != 0 {
		t.Fatalf("remakes entered season queue stats: %#v", queueStats)
	}
}

func TestUnavailableSeasonStatsDoesNotClaimComplete(t *testing.T) {
	_, progress, matches, byQueue := (&app{}).loadSeasonChampionStats(
		t.Context(), nil, gameplayReference{}, Summoner{}, "invalid", nil,
	)
	if progress.Complete || !progress.Unavailable || progress.Message != "当前数据源不提供赛季统计" {
		t.Fatalf("unavailable progress = %#v", progress)
	}
	if matches != nil || byQueue != nil {
		t.Fatalf("unavailable season returned samples: matches=%#v queues=%#v", matches, byQueue)
	}
}

func TestSeasonStatsRejectOldCacheSchema(t *testing.T) {
	store := trackTestStore(t, &localStore{root: t.TempDir()})
	cache := seasonStatsCache{SchemaVersion: seasonStatsCacheSchemaVersion - 1, Source: seasonStatsSource, Season: "S26", AccountHash: "account"}
	if err := store.saveSeasonStats(cache); err != nil {
		t.Fatal(err)
	}
	if _, err := store.loadSeasonStats(seasonStatsSource, "account", "S26"); err == nil {
		t.Fatal("old season cache schema was accepted")
	}
	cache.SchemaVersion = seasonStatsCacheSchemaVersion
	if err := store.saveSeasonStats(cache); err != nil {
		t.Fatal(err)
	}
	if _, err := store.loadSeasonStats(seasonStatsSource, "account", "S26"); err != nil {
		t.Fatalf("current season cache schema was rejected: %v", err)
	}
}

func TestSeasonStatsCacheDoesNotCrossReadSources(t *testing.T) {
	store := trackTestStore(t, &localStore{root: t.TempDir()})
	cache := seasonStatsCache{
		SchemaVersion: seasonStatsCacheSchemaVersion, Source: dataSourceSGP,
		Season: "S26", AccountHash: "same-account", Complete: true,
	}
	if err := store.saveSeasonStats(cache); err != nil {
		t.Fatal(err)
	}
	if _, err := store.loadSeasonStats(dataSourceLCU, "same-account", "S26"); err == nil {
		t.Fatal("LCU read reused an SGP season cache")
	}
	if _, err := store.loadSeasonStats(dataSourceSGP, "same-account", "S26"); err != nil {
		t.Fatalf("SGP cache was not readable from its own source: %v", err)
	}
}

func TestSeasonScanPagesClearsResumeIndexWhenComplete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"games": []any{}})
	}))
	defer server.Close()

	client := &LCUClient{}
	provider := newSGPProvider()
	provider.http = server.Client()
	provider.serverBases["HN1"] = server.URL
	provider.token = "test-entitlements"
	provider.tokenAt = time.Now()
	provider.tokenClient = client
	scan := &seasonScanState{
		cache:             seasonStatsCache{ResumeIndex: 60, PendingIndex: 40},
		stats:             make(map[int64]*gameplaySeasonChampionStat),
		seen:              make(map[int64]bool),
		seasonStartMillis: seasonStartS26.UnixMilli(),
	}

	(&app{sgp: provider}).seasonScanPages(t.Context(), client, "HN1", "subject", scan, 1)
	if !scan.cache.Complete || scan.cache.ResumeIndex != 0 {
		t.Fatalf("completed scan retained a stale resume index: %#v", scan.cache)
	}
	foregroundKey := sgpHistoryPageCacheKey("HN1", "subject", 60, sgpPageSize, nil)
	provider.mu.Lock()
	_, cached := provider.historyCache[foregroundKey]
	provider.mu.Unlock()
	if !cached {
		t.Fatal("foreground season scan did not retain its SGP history page")
	}
}

// ---------------------------------------------------------------------------
// R116-E P0：放开队列过滤（海克斯大乱斗 2300/2400/3270）
// ---------------------------------------------------------------------------

func r116eMayhemMatchInfo(id, queue int64, augments []int64, items []int64, win bool) *riotMatchInfo {
	subject := riotParticipant{
		PUUID: "subject", TeamID: 100, ChampionID: 157, Win: win,
		Kills: 8, Deaths: 2, Assists: 6, TotalMinionsKilled: 40, GoldEarned: 12000,
	}
	for index, value := range augments {
		switch index {
		case 0:
			subject.PlayerAugment1 = value
		case 1:
			subject.PlayerAugment2 = value
		case 2:
			subject.PlayerAugment3 = value
		case 3:
			subject.PlayerAugment4 = value
		case 4:
			subject.PlayerAugment5 = value
		case 5:
			subject.PlayerAugment6 = value
		}
	}
	for index, value := range items {
		switch index {
		case 0:
			subject.Item0 = value
		case 1:
			subject.Item1 = value
		case 2:
			subject.Item2 = value
		case 3:
			subject.Item3 = value
		case 4:
			subject.Item4 = value
		case 5:
			subject.Item5 = value
		case 6:
			subject.Item6 = value
		}
	}
	return &riotMatchInfo{
		GameID: id, QueueID: queue, GameDuration: 1800,
		// GameID 在真实数据里是 7e12 量级，直接当小时数会溢出 time.Duration，
		// 取模只是为了让 fixture 的时间戳落在赛季起点之后的一个合理窗口里。
		GameCreation: seasonStartS26.Add(time.Duration(id%100000) * time.Hour).UnixMilli(),
		// 海克斯大乱斗没有 teamPosition/individualPosition，这一点是
		// 「海斗不出位置统计与能力雷达」的根因，fixture 必须如实反映。
		Participants: []riotParticipant{subject, {PUUID: "enemy", TeamID: 200, Kills: 20}},
	}
}

// P0 判据 1：海斗 fixture 跑 seasonStatsAccumulate，Stats / QueueStats 正确记入。
func TestSeasonStatsAccumulateRecordsHextechMayhem(t *testing.T) {
	stats := make(map[int64]*gameplaySeasonChampionStat)
	queueStats := make(map[int64]gameplayAggregate)
	start := seasonStartS26.UnixMilli()
	for index, queueID := range seasonMayhemQueueIDs {
		seasonStatsAccumulate(stats, queueStats, r116eMayhemMatchInfo(int64(index+1), queueID, []int64{5001}, []int64{6672}, index%2 == 0), "subject", start)
	}
	item := stats[157]
	if item == nil || item.Games != 3 || item.Wins != 2 {
		t.Fatalf("mayhem games were not accumulated into champion stats: %#v", item)
	}
	for _, queueID := range seasonMayhemQueueIDs {
		aggregate, ok := queueStats[queueID]
		if !ok || aggregate.QueueID != queueID || aggregate.Games != 1 {
			t.Fatalf("queue %d was not aggregated separately: %#v", queueID, aggregate)
		}
	}
	finalized := seasonStatsFinalizeQueues(queueStats)
	if finalized[2300].WinRate != 100 || finalized[2400].WinRate != 0 {
		t.Fatalf("mayhem queue win rates were not finalized: %#v", finalized)
	}
}

// P0 判据 1 的回归半边：420 对局的行为不变，且 3220「极地大乱斗」不会被误收。
func TestSeasonStatsAccumulateKeepsRankedBehaviourAndRejectsPlainARAM(t *testing.T) {
	stats := make(map[int64]*gameplaySeasonChampionStat)
	queueStats := make(map[int64]gameplayAggregate)
	start := seasonStartS26.UnixMilli()
	seasonStatsAccumulate(stats, queueStats, r116eMayhemMatchInfo(11, 420, nil, []int64{6672}, true), "subject", start)
	// 3220 是「极地大乱斗」(aram 组)，不是海克斯大乱斗，误收会把普通 ARAM
	// 的场次算进海斗口径。450 同理。
	for _, queueID := range []int64{3220, 450, 1700, 0} {
		seasonStatsAccumulate(stats, queueStats, r116eMayhemMatchInfo(12, queueID, []int64{5001}, nil, true), "subject", start)
		if _, ok := queueStats[queueID]; ok {
			t.Fatalf("queue %d must not be counted into season stats", queueID)
		}
	}
	item := stats[157]
	if item == nil || item.Games != 1 || item.Wins != 1 {
		t.Fatalf("420 behaviour changed: %#v", item)
	}
	if queueStats[420].Games != 1 || len(queueStats) != 1 {
		t.Fatalf("420 queue aggregation changed: %#v", queueStats)
	}
	if !seasonStatsQueueAllowed(420) || !seasonStatsQueueAllowed(440) {
		t.Fatal("classic ranked queues must stay allowed")
	}
	for _, queueID := range seasonMayhemQueueIDs {
		if !seasonStatsQueueAllowed(queueID) || !seasonMayhemQueue(queueID) {
			t.Fatalf("mayhem queue %d must be allowed and recognised", queueID)
		}
	}
	for _, queueID := range []int64{3220, 450, 1700, 0, -1} {
		if seasonStatsQueueAllowed(queueID) || seasonMayhemQueue(queueID) {
			t.Fatalf("queue %d must not be treated as season/mayhem queue", queueID)
		}
	}
}

// P0：两处判据必须同步——seasonRecordRankedMatch 与 seasonStatsAccumulate
// 收同一批队列，且海斗场次额外记一条 augment 样本、420 不记。
func TestSeasonRecordRankedMatchMirrorsMayhemCriteriaAndRecordsAugmentSample(t *testing.T) {
	cache := &seasonStatsCache{}
	seasonRecordRankedMatch(cache, r116eMayhemMatchInfo(21, 2400, []int64{5001, 5002, 5003}, []int64{6672, 3153, 3031, 3006, 0, 0, 3340}, true), "subject")
	seasonRecordRankedMatch(cache, r116eMayhemMatchInfo(22, 420, nil, []int64{6672}, false), "subject")
	seasonRecordRankedMatch(cache, r116eMayhemMatchInfo(23, 3220, []int64{5001}, nil, true), "subject")
	if len(cache.RankedMatches) != 2 {
		t.Fatalf("expected the mayhem game plus the 420 game, got %#v", cache.RankedMatches)
	}
	if cache.RankedMatches[0].QueueID != 2400 || cache.RankedMatches[1].QueueID != 420 {
		t.Fatalf("unexpected queue ids: %#v", cache.RankedMatches)
	}
	// 海斗没有分路：位置与能力雷达原料都必须是空的，不能编一个出来。
	if cache.RankedMatches[0].Position != "" || cache.RankedMatches[0].Ability != nil {
		t.Fatalf("mayhem snapshot must not invent a position or ability sample: %#v", cache.RankedMatches[0])
	}
	if len(cache.AugmentSamples) != 1 {
		t.Fatalf("only the mayhem game may produce an augment sample, got %#v", cache.AugmentSamples)
	}
	sample := cache.AugmentSamples[0]
	if sample.GameID != 21 || sample.ChampionID != 157 || !sample.Win {
		t.Fatalf("unexpected augment sample: %#v", sample)
	}
	if len(sample.AugmentIDs) != 3 || len(sample.ItemIDs) != 5 {
		t.Fatalf("zero slots must be dropped: %#v", sample)
	}
	if len(sample.AugmentIDs) > seasonAugmentSlots || len(sample.ItemIDs) > seasonItemSlots {
		t.Fatalf("slot bounds violated: %#v", sample)
	}
	// 判据同步：accumulate 收的队列集合必须与 record 完全一致。
	for _, queueID := range []int64{420, 440, 2300, 2400, 3270, 3220, 450, 0} {
		stats := make(map[int64]*gameplaySeasonChampionStat)
		probe := &seasonStatsCache{}
		info := r116eMayhemMatchInfo(31, queueID, []int64{5001}, nil, true)
		seasonStatsAccumulate(stats, nil, info, "subject", seasonStartS26.UnixMilli())
		seasonRecordRankedMatch(probe, info, "subject")
		if (len(stats) > 0) != (len(probe.RankedMatches) > 0) {
			t.Fatalf("queue %d: accumulate and record disagree (stats=%d ranked=%d)", queueID, len(stats), len(probe.RankedMatches))
		}
	}
}

// P0 判据 2：schema 升版后旧缓存必须触发重扫（失效逻辑在 loadSeasonStats）。
// 工单原文要求 7→8；并行 R117 会话已经占用了 8（K/D/A 改逐场均值），
// 所以本轮实际是 8→9，用户已裁决顺延。
func TestSeasonStatsSchemaVersionIsNineAndRejectsEveryOlderFile(t *testing.T) {
	if seasonStatsCacheSchemaVersion != 9 {
		t.Fatalf("seasonStatsCacheSchemaVersion = %d, want 9", seasonStatsCacheSchemaVersion)
	}
	store := trackTestStore(t, &localStore{root: t.TempDir()})
	for _, old := range []int{7, 8} {
		cache := seasonStatsCache{SchemaVersion: old, Source: seasonStatsSource, Season: "S26", AccountHash: "account"}
		if err := store.saveSeasonStats(cache); err != nil {
			t.Fatal(err)
		}
		if _, err := store.loadSeasonStats(seasonStatsSource, "account", "S26"); err == nil {
			t.Fatalf("schema %d cache was accepted; it must be rejected so the scan restarts", old)
		}
	}
	cache := seasonStatsCache{SchemaVersion: seasonStatsCacheSchemaVersion, Source: seasonStatsSource, Season: "S26", AccountHash: "account"}
	if err := store.saveSeasonStats(cache); err != nil {
		t.Fatal(err)
	}
	if _, err := store.loadSeasonStats(seasonStatsSource, "account", "S26"); err != nil {
		t.Fatalf("current schema was rejected: %v", err)
	}
}

// P3：样本条数有界，去重生效。
func TestSeasonTrimAugmentSamplesDedupesAndCaps(t *testing.T) {
	cache := &seasonStatsCache{}
	for id := int64(1); id <= seasonAugmentSampleLimit+250; id++ {
		cache.AugmentSamples = append(cache.AugmentSamples, seasonAugmentSample{GameID: id, ChampionID: 157, AugmentIDs: []int64{5001}})
	}
	// 同一场在增量追新与后台回补里各记一次。
	cache.AugmentSamples = append(cache.AugmentSamples, seasonAugmentSample{GameID: seasonAugmentSampleLimit + 249, ChampionID: 157, AugmentIDs: []int64{5001}})
	cache.AugmentSamples = append(cache.AugmentSamples, seasonAugmentSample{GameID: 0, ChampionID: 157})
	dropped := seasonTrimAugmentSamples(cache)
	if len(cache.AugmentSamples) != seasonAugmentSampleLimit {
		t.Fatalf("augment samples = %d, want %d", len(cache.AugmentSamples), seasonAugmentSampleLimit)
	}
	if dropped != 252 {
		t.Fatalf("dropped = %d, want 252 (1 duplicate + 1 invalid game id + 250 over the cap)", dropped)
	}
	// 保留的是 GameID 最大的那批（gameId 由平台递增分配，越大越新）。
	if cache.AugmentSamples[0].GameID != seasonAugmentSampleLimit+250 {
		t.Fatalf("newest sample was not kept first: %#v", cache.AugmentSamples[0])
	}
	if got := seasonTrimAugmentSamples(&seasonStatsCache{}); got != 0 {
		t.Fatalf("empty cache must be a no-op, dropped = %d", got)
	}
}

// ---------------------------------------------------------------------------
// R116-E P2-6：海克斯 → 出装 静态查询（纯函数）
// ---------------------------------------------------------------------------

func r116eAugmentSamples(championID int64, augmentID int64, games int, wins int, items []int64) []seasonAugmentSample {
	result := make([]seasonAugmentSample, 0, games)
	for index := 0; index < games; index++ {
		result = append(result, seasonAugmentSample{
			GameID: int64(index + 1), ChampionID: championID,
			AugmentIDs: []int64{augmentID}, ItemIDs: items, Win: index < wins,
		})
	}
	return result
}

// P3 判据 2 的前半：给定一个 augment ID，聚合出「最常出的装备组合 + 场次 + 胜率」。
func TestSeasonAugmentBuildInsightAggregatesCombosGamesAndWinRate(t *testing.T) {
	samples := r116eAugmentSamples(157, 5001, 12, 7, []int64{6672, 3153, 3031})
	samples = append(samples, r116eAugmentSamples(157, 5001, 4, 1, []int64{3031, 6672, 3153})...) // 同一套出装，槽位顺序不同
	samples = append(samples, seasonAugmentSample{GameID: 99, ChampionID: 157, AugmentIDs: []int64{5002}, ItemIDs: []int64{6672}, Win: true})
	samples = append(samples, r116eAugmentSamples(222, 5001, 30, 30, []int64{1111})...) // 别的英雄，必须被排除

	report := seasonAugmentBuildReportFor(samples, 157, 5001, 0, seasonAugmentBuildGroupLimit, seasonAugmentBuildComboLimit)
	if len(report.Groups) != 1 {
		t.Fatalf("expected exactly one group, got %#v", report.Groups)
	}
	group := report.Groups[0]
	if group.Games != 16 || group.Wins != 8 || group.WinRate != 50 {
		t.Fatalf("unexpected group totals: %#v", group)
	}
	// SampleGames 是「这个英雄的全部海斗样本」= 12 + 4 + 1（那条 5002 的），
	// 而 group.Games 只数选了 5001 的 16 场；两个数不同才对。
	if report.SampleGames != 17 {
		t.Fatalf("sample games must be champion-scoped, got %d", report.SampleGames)
	}
	if len(group.Combos) != 1 {
		t.Fatalf("slot order must not split one combo: %#v", group.Combos)
	}
	combo := group.Combos[0]
	if combo.Games != 16 || combo.Wins != 8 || combo.WinRate != 50 || combo.Share != 100 {
		t.Fatalf("unexpected combo: %#v", combo)
	}
	want := []int64{3031, 3153, 6672}
	for index, id := range want {
		if combo.ItemIDs[index] != id {
			t.Fatalf("combo item ids = %#v, want ascending %#v", combo.ItemIDs, want)
		}
	}
	if report.MinimumSample != seasonAugmentMinimumSample {
		t.Fatalf("minimum sample = %d, want the default %d", report.MinimumSample, seasonAugmentMinimumSample)
	}
}

// P3 判据 2 的后半：样本不足（< 可配置阈值）时整块不显示——纯函数返回 nil，
// 报告里没有任何分组。阈值可配置这一条也在这里断言。
func TestSeasonAugmentBuildInsightHidesEverythingBelowTheConfigurableThreshold(t *testing.T) {
	samples := r116eAugmentSamples(157, 5001, 9, 9, []int64{6672})
	if group := seasonAugmentBuildInsight(samples, 5001, seasonAugmentMinimumSample, seasonAugmentBuildComboLimit); group != nil {
		t.Fatalf("9 games must stay hidden under the default threshold of %d: %#v", seasonAugmentMinimumSample, group)
	}
	if report := seasonAugmentBuildReportFor(samples, 157, 5001, 0, 0, 0); len(report.Groups) != 0 {
		t.Fatalf("report must stay empty below the threshold: %#v", report)
	}
	// 阈值可配置：调到 5 场之后同一批样本就能显示。
	if group := seasonAugmentBuildInsight(samples, 5001, 5, seasonAugmentBuildComboLimit); group == nil || group.Games != 9 {
		t.Fatalf("configured threshold was ignored: %#v", group)
	}
	// 非正数阈值一律回落到默认值，不能让 ?minimumSample=0 把「1 场也算规律」放出来。
	if got := seasonAugmentMinimumSampleOr(0); got != seasonAugmentMinimumSample {
		t.Fatalf("seasonAugmentMinimumSampleOr(0) = %d, want %d", got, seasonAugmentMinimumSample)
	}
	if got := seasonAugmentMinimumSampleOr(-3); got != seasonAugmentMinimumSample {
		t.Fatalf("negative threshold must fall back, got %d", got)
	}
	if got := seasonAugmentMinimumSampleOr(25); got != 25 {
		t.Fatalf("positive threshold must be honoured, got %d", got)
	}
	// augmentID 非法时不返回任何「规律」。
	if group := seasonAugmentBuildInsight(samples, 0, 1, 3); group != nil {
		t.Fatalf("augment id 0 must not produce a group: %#v", group)
	}
}

// 不传 augmentId 时按「本人在这个英雄上用得最多的海克斯」排序，并受展示上限约束。
func TestSeasonAugmentBuildReportRanksGroupsAndRespectsLimits(t *testing.T) {
	var samples []seasonAugmentSample
	id := int64(0)
	for _, spec := range []struct {
		augmentID int64
		games     int
	}{
		{5001, 30}, {5002, 25}, {5003, 12}, {5004, 9}, {5005, 11}, {5006, 10}, {5007, 10}, {5008, 10},
	} {
		for index := 0; index < spec.games; index++ {
			id++
			samples = append(samples, seasonAugmentSample{GameID: id, ChampionID: 157, AugmentIDs: []int64{spec.augmentID}, ItemIDs: []int64{6672}, Win: index%2 == 0})
		}
	}
	report := seasonAugmentBuildReportFor(samples, 157, 0, 0, seasonAugmentBuildGroupLimit, seasonAugmentBuildComboLimit)
	if len(report.Groups) != seasonAugmentBuildGroupLimit {
		t.Fatalf("groups = %d, want the display cap %d", len(report.Groups), seasonAugmentBuildGroupLimit)
	}
	// 5004 只有 9 场（低于默认门槛 10），必须被跳过；顺序按场次降序。
	want := []int64{5001, 5002, 5003, 5005, 5006, 5007}
	for index, augmentID := range want {
		if report.Groups[index].AugmentID != augmentID {
			t.Fatalf("group order = %#v, want %#v", report.Groups[index].AugmentID, augmentID)
		}
	}
	if report.SampleGames != len(samples) {
		t.Fatalf("sample games = %d, want %d", report.SampleGames, len(samples))
	}
}

// P2-4 GE4（R120）：seasonAugmentSampleFor 的内层队列门禁必须有独立测试。
// 外层 seasonRecordRankedMatch 已经按 seasonStatsQueueAllowed 过滤过一遍，所以内层这道
// 是「冗余的第二道」——R120 实测把它删掉整包测试仍然全绿。冗余不等于可省：它是唯一
// 一个「即使调用方忘了过滤，也不会把经典排位或普通大乱斗写进海克斯样本」的保证，
// 而海克斯样本正是 P2-6「选了某个海克斯之后通常出什么」的全部原料，混进一场
// 没有海克斯的对局就会把出装联动算成假话。
func TestSeasonAugmentSampleForRejectsEveryNonMayhemQueue(t *testing.T) {
	// 非海斗队列一律不记录。450 / 3220 是大乱斗但没有海克斯，最容易混进来。
	for _, queueID := range []int64{420, 440, 450, 3220, 0, -1, 2299, 2301, 3271} {
		info := r116eMayhemMatchInfo(91, queueID, []int64{5001, 5002, 5003}, []int64{6672, 3153}, true)
		if sample := seasonAugmentSampleFor(info, "subject"); sample != nil {
			t.Fatalf("队列 %d 不是海克斯大乱斗，却记下了样本：%#v", queueID, sample)
		}
	}
	// 反向退化：门禁不能写成「一律不记录」。三个海斗队列都必须照记，
	// 判据只能来自 seasonMayhemQueueIDs 这一个真源。
	for _, queueID := range seasonMayhemQueueIDs {
		if !seasonMayhemQueue(queueID) {
			t.Fatalf("seasonMayhemQueue(%d) = false，与 seasonMayhemQueueIDs 自相矛盾", queueID)
		}
		info := r116eMayhemMatchInfo(92, queueID, []int64{5001, 5002, 5003}, []int64{6672, 3153, 3031, 3006, 0, 0, 3340}, true)
		sample := seasonAugmentSampleFor(info, "subject")
		if sample == nil {
			t.Fatalf("海克斯大乱斗队列 %d 必须记下样本", queueID)
		}
		if sample.GameID != 92 || sample.ChampionID != 157 || !sample.Win {
			t.Fatalf("队列 %d 的样本内容不对：%#v", queueID, sample)
		}
		if len(sample.AugmentIDs) != 3 || len(sample.ItemIDs) != 5 {
			t.Fatalf("零槽位必须被丢掉：augments=%#v items=%#v", sample.AugmentIDs, sample.ItemIDs)
		}
	}
	// 另外三个 nil 分支：本人不在参与者名单里、info 为 nil、这一场没有任何海克斯。
	if sample := seasonAugmentSampleFor(r116eMayhemMatchInfo(93, seasonMayhemPrimaryQueueID, []int64{5001}, nil, true), "someone-else"); sample != nil {
		t.Fatalf("本人不在参与者名单里时不许记样本：%#v", sample)
	}
	if sample := seasonAugmentSampleFor(nil, "subject"); sample != nil {
		t.Fatalf("info 为 nil 时不许记样本：%#v", sample)
	}
	if sample := seasonAugmentSampleFor(r116eMayhemMatchInfo(94, seasonMayhemPrimaryQueueID, nil, []int64{6672, 3153}, true), "subject"); sample != nil {
		t.Fatalf("没有海克斯的场次对 P2-6 没有贡献，不许记样本：%#v", sample)
	}
}
