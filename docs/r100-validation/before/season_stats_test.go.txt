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

// 赛季扫描顺手留下的排位快照必须和 seasonStatsAccumulate 用同一套判据
// （只认 420/440、排除重开局），否则「近 20 场排位」会和赛季英雄统计对不上账。
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
