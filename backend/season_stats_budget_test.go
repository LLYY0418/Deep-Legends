package main

// R116-E P1 磁盘预算 + P3 容量的验收测试。
//
// 工单 P1 判据：
//   - 构造一个人为超过 4 MiB 的 seasonStatsCache，验证写入前会先截断
//     RankedMatches 到上限，而不是写入失败或 panic；
//   - 目录健康检查诊断事件能正确报出总大小（用临时目录构造几个文件验证）。
//
// 工单 P3 判据 1：放开后单文件大小实测控制在 4 MiB 上限之内
// （用 2000 场海斗的合成 fixture 验证）。真机上「打到 2000 场」做不到，
// 所以这里用合成 fixture；真机那一条（120 KB 量级）在执行账本里标 `待真机验证`。

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// r116eOverBudgetCache 造一个明确超过 4 MiB 的缓存：
// Stats 与 GameIDs 是「绝不能丢」的部分，RankedMatches 与 AugmentSamples
// 是可以截断的部分。
func r116eOverBudgetCache() seasonStatsCache {
	cache := seasonStatsCache{
		SchemaVersion: seasonStatsCacheSchemaVersion, Source: seasonStatsSource,
		Season: "S26", AccountHash: "account", Complete: true,
	}
	for id := int64(1); id <= 173; id++ {
		cache.Stats = append(cache.Stats, gameplaySeasonChampionStat{
			ChampionID: id, ChampionName: strings.Repeat("英雄名", 4), Games: 40, Wins: 20,
			Kills: 8.5, Deaths: 4.2, Assists: 9.1, TotalCS: 6000, Duration: 72000,
			WinRate: 50, KDA: 4.19, CS: 148.2, CSPerMinute: 12.4,
		})
	}
	for id := int64(1); id <= 3000; id++ {
		cache.GameIDs = append(cache.GameIDs, 7000000000000+id)
	}
	// 5 个队列 × 900 条 = 4500 条单场快照（带能力雷达原料），远超
	// seasonRankedMatchLimit，也足以把整个文件顶过 4 MiB。
	for _, queueID := range []int64{420, 440, 2300, 2400, 3270} {
		for index := int64(0); index < 900; index++ {
			cache.RankedMatches = append(cache.RankedMatches, seasonRankedMatch{
				GameID: 7000000000000 + queueID*1000 + index, CreatedAt: 1700000000000 + index*1000,
				QueueID: queueID, ChampionID: 157, Win: index%2 == 0,
				Kills: 8, Deaths: 4, Assists: 9, TeamKills: 40, Position: "middle",
				Ability: &seasonRankedAbilitySample{
					Duration: 1800,
					Player:   seasonRankedAbilitySide{Kills: 8, Deaths: 4, Assists: 9, Damage: 22000, CS: 180, Gold: 14000, Vision: 20, TeamKills: 40, TeamDamage: 90000},
					Opponent: &seasonRankedAbilitySide{Kills: 3, Deaths: 6, Assists: 4, Damage: 12000, CS: 150, Gold: 11000, Vision: 15, TeamKills: 30, TeamDamage: 70000},
				},
			})
		}
	}
	for id := int64(1); id <= 20000; id++ {
		cache.AugmentSamples = append(cache.AugmentSamples, seasonAugmentSample{
			GameID: 7000000000000 + id, ChampionID: 157,
			AugmentIDs: []int64{5001, 5002, 5003, 5004},
			ItemIDs:    []int64{6672, 3153, 3031, 3006, 6333, 3156},
			Win:        id%2 == 0,
		})
	}
	return cache
}

// P1 判据 1：超限时先截断 RankedMatches，写入照常成功，不 panic、不丢 Stats。
func TestSeasonStatsBudgetTruncatesRankedMatchesInsteadOfRefusingTheWrite(t *testing.T) {
	cache := r116eOverBudgetCache()
	raw, err := json.Marshal(cache)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) <= seasonStatsFileBudgetBytes {
		t.Fatalf("fixture must start over budget, got %d bytes (budget %d)", len(raw), seasonStatsFileBudgetBytes)
	}
	t.Logf("P1 fixture: raw = %d bytes (%.2f MiB), budget = %d bytes", len(raw), float64(len(raw))/(1024*1024), seasonStatsFileBudgetBytes)

	data, report := marshalSeasonStatsWithinBudget(cache)
	if report.MarshalErr != nil {
		t.Fatalf("marshal failed: %v", report.MarshalErr)
	}
	if len(data) == 0 {
		t.Fatal("budget handling must never refuse to produce bytes")
	}
	if len(data) > seasonStatsFileBudgetBytes {
		t.Fatalf("written bytes = %d, still over the %d budget", len(data), seasonStatsFileBudgetBytes)
	}
	if !report.Truncated {
		t.Fatalf("report must say it truncated something: %#v", report)
	}
	// RankedMatches 是优先截断项，必须收到 seasonRankedMatchLimit。
	if report.RankedMatchesAfter != seasonRankedMatchLimit {
		t.Fatalf("ranked matches = %d, want the existing cap %d", report.RankedMatchesAfter, seasonRankedMatchLimit)
	}
	if len(report.Steps) == 0 || report.Steps[0] != "ranked_matches_capped" {
		t.Fatalf("ranked matches must be truncated first: %#v", report.Steps)
	}
	// Stats 与 GameIDs 一律不动。
	var written seasonStatsCache
	if err := json.Unmarshal(data, &written); err != nil {
		t.Fatalf("the truncated payload must still be valid JSON: %v", err)
	}
	if len(written.Stats) != len(cache.Stats) {
		t.Fatalf("stats were dropped: %d -> %d", len(cache.Stats), len(written.Stats))
	}
	if len(written.GameIDs) != len(cache.GameIDs) {
		t.Fatalf("game ids were dropped: %d -> %d", len(cache.GameIDs), len(written.GameIDs))
	}
	if written.RankedMatches[0].CreatedAt < written.RankedMatches[len(written.RankedMatches)-1].CreatedAt {
		t.Fatal("truncation must keep the newest matches first")
	}
	// 原 cache 不能被就地改坏（调用方 finishSeasonScan 之后还要用它报诊断）。
	if len(cache.RankedMatches) != 4500 || len(cache.AugmentSamples) != 20000 {
		t.Fatalf("the caller's cache was mutated: ranked=%d augments=%d", len(cache.RankedMatches), len(cache.AugmentSamples))
	}
}

// 端到端：saveSeasonStats 挂的就是这条预算，写入必须成功且落盘文件在预算内。
func TestSeasonStatsSaveAppliesTheBudgetEndToEnd(t *testing.T) {
	store := trackTestStore(t, &localStore{root: t.TempDir()})
	cache := r116eOverBudgetCache()
	report, err := store.saveSeasonStatsReported(cache)
	if err != nil {
		t.Fatalf("saveSeasonStats must not fail because of the budget: %v", err)
	}
	path := filepath.Join(store.root, seasonStatsFileKey(cache.Source, cache.AccountHash, cache.Season))
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() > seasonStatsFileBudgetBytes {
		t.Fatalf("file on disk = %d bytes, over the %d budget", info.Size(), seasonStatsFileBudgetBytes)
	}
	if int64(report.WrittenBytes) != info.Size() {
		t.Fatalf("report says %d bytes but the file is %d", report.WrittenBytes, info.Size())
	}
	// 读回来仍然是合法缓存（schema/source/account/season 都对得上）。
	loaded, err := store.loadSeasonStats(seasonStatsSource, "account", "S26")
	if err != nil {
		t.Fatalf("the truncated cache must still load: %v", err)
	}
	if len(loaded.RankedMatches) != seasonRankedMatchLimit || len(loaded.Stats) != len(cache.Stats) {
		t.Fatalf("unexpected reload: ranked=%d stats=%d", len(loaded.RankedMatches), len(loaded.Stats))
	}
	// 走 saveSeasonStats（不返回报告的那条）也必须同样安全。
	if err := store.saveSeasonStats(cache); err != nil {
		t.Fatalf("saveSeasonStats failed: %v", err)
	}
}

// 预算内的缓存一个字都不该动。
func TestSeasonStatsBudgetLeavesSmallCachesAlone(t *testing.T) {
	cache := seasonStatsCache{
		SchemaVersion: seasonStatsCacheSchemaVersion, Source: seasonStatsSource,
		Season: "S26", AccountHash: "account",
		Stats:          []gameplaySeasonChampionStat{{ChampionID: 157, Games: 3, Wins: 2}},
		RankedMatches:  []seasonRankedMatch{{GameID: 1, CreatedAt: 1, QueueID: 2400}},
		AugmentSamples: []seasonAugmentSample{{GameID: 1, ChampionID: 157, AugmentIDs: []int64{5001}, ItemIDs: []int64{6672}, Win: true}},
	}
	before, err := json.Marshal(cache)
	if err != nil {
		t.Fatal(err)
	}
	after, report := marshalSeasonStatsWithinBudget(cache)
	if report.Truncated || report.OverBudget || len(report.Steps) != 0 {
		t.Fatalf("a small cache must not be touched: %#v", report)
	}
	if string(before) != string(after) {
		t.Fatalf("payload changed:\n before=%s\n after =%s", before, after)
	}
	if report.RawBytes != report.WrittenBytes {
		t.Fatalf("raw = %d, written = %d, want equal", report.RawBytes, report.WrittenBytes)
	}
}

// 极端情况：可截断项全丢光仍然超限（Stats/GameIDs 本身撑爆）时，
// 照写不误并如实报 OverBudget——绝不 panic、绝不整体拒绝写入。
func TestSeasonStatsBudgetWritesAndReportsWhenNothingCanBeTrimmed(t *testing.T) {
	cache := seasonStatsCache{
		SchemaVersion: seasonStatsCacheSchemaVersion, Source: seasonStatsSource,
		Season: "S26", AccountHash: "account",
	}
	// 只堆 GameIDs：它既不是 RankedMatches 也不是 AugmentSamples，预算不许动它。
	for id := int64(0); id < 1200000; id++ {
		cache.GameIDs = append(cache.GameIDs, 7000000000000+id)
	}
	data, report := marshalSeasonStatsWithinBudget(cache)
	if report.MarshalErr != nil {
		t.Fatalf("marshal failed: %v", report.MarshalErr)
	}
	if len(data) == 0 {
		t.Fatal("the write must not be refused")
	}
	if !report.OverBudget {
		t.Fatalf("an untrimmable over-budget cache must be reported as such: %#v", report)
	}
	if report.RankedMatchesAfter != 0 || report.AugmentSamplesAfter != 0 {
		t.Fatalf("nothing was trimmable, counters must stay zero: %#v", report)
	}
	var written seasonStatsCache
	if err := json.Unmarshal(data, &written); err != nil {
		t.Fatalf("payload must stay valid JSON: %v", err)
	}
	if len(written.GameIDs) != len(cache.GameIDs) {
		t.Fatalf("game ids were dropped: %d -> %d", len(cache.GameIDs), len(written.GameIDs))
	}
	t.Logf("P1 untrimmable case: written = %d bytes, over budget = %v, steps = %v", report.WrittenBytes, report.OverBudget, report.Steps)
}

// P1 判据 2：目录健康检查能用临时目录构造几个文件验证总大小报得对。
func TestSeasonStatsDirectoryHealthReportsTotalSize(t *testing.T) {
	root := t.TempDir()
	// 目录还不存在：不是错误，只是「还没有缓存」。
	if health := seasonStatsDirectoryHealthFor(root); health.Files != 0 || health.TotalBytes != 0 || health.Err != "" {
		t.Fatalf("a missing season-stats directory must be a clean zero report: %#v", health)
	}
	dir := filepath.Join(root, "season-stats", "sgp")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	sizes := []int{1024, 2048, 4096}
	for index, size := range sizes {
		name := filepath.Join(dir, "account"+string(rune('a'+index))+"-s26.json")
		if err := os.WriteFile(name, make([]byte, size), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// 目录外的文件不该被算进来。
	if err := os.WriteFile(filepath.Join(root, "unrelated.json"), make([]byte, 999999), 0o600); err != nil {
		t.Fatal(err)
	}
	health := seasonStatsDirectoryHealthFor(root)
	if health.Files != 3 {
		t.Fatalf("files = %d, want 3 (%#v)", health.Files, health)
	}
	if health.TotalBytes != 1024+2048+4096 {
		t.Fatalf("total bytes = %d, want %d", health.TotalBytes, 1024+2048+4096)
	}
	if health.LargestBytes != 4096 {
		t.Fatalf("largest = %d, want 4096", health.LargestBytes)
	}
	// 只报相对路径：绝对路径里带本机用户名，不该进诊断事件。
	if health.LargestFile == "" || filepath.IsAbs(health.LargestFile) || strings.Contains(health.LargestFile, root) {
		t.Fatalf("largest file must be a relative path, got %q", health.LargestFile)
	}
	if health.OverBudgetFiles != 0 || health.Err != "" {
		t.Fatalf("unexpected health report: %#v", health)
	}
	if !health.Exists {
		t.Fatal("the directory exists now, the report must say so")
	}
	// 超过单文件上限的要被数出来。
	if err := os.WriteFile(filepath.Join(dir, "huge.json"), make([]byte, 1024), 0o600); err != nil {
		t.Fatal(err)
	}
	if health := seasonStatsDirectoryHealthFor(root); health.Files != 4 || health.TotalBytes != 1024+2048+4096+1024 {
		t.Fatalf("health did not pick up the new file: %#v", health)
	}
	// 空 root 不能 panic。
	if health := seasonStatsDirectoryHealthFor(""); health.Err == "" {
		t.Fatalf("an empty root must be reported, got %#v", health)
	}
}

// 一次性：同一个存储根只做一次体检，不同根各做一次。
func TestSeasonStatsDirectoryHealthRunsOncePerRoot(t *testing.T) {
	seasonStatsHealthMu.Lock()
	saved := seasonStatsHealthDone
	seasonStatsHealthDone = make(map[string]bool)
	seasonStatsHealthMu.Unlock()
	t.Cleanup(func() {
		seasonStatsHealthMu.Lock()
		seasonStatsHealthDone = saved
		seasonStatsHealthMu.Unlock()
	})

	first, second := t.TempDir(), t.TempDir()
	if !seasonStatsBudgetMarkChecked(first) {
		t.Fatal("the first check for a root must run")
	}
	if seasonStatsBudgetMarkChecked(first) {
		t.Fatal("the health check must be one-shot per root")
	}
	if !seasonStatsBudgetMarkChecked(second) {
		t.Fatal("a different root gets its own one-shot check")
	}
	if seasonStatsBudgetMarkChecked("") || seasonStatsBudgetMarkChecked("   ") {
		t.Fatal("an empty root must never be marked checked")
	}
}

// P3 判据 1：2000 场海斗的合成 fixture，单文件必须落在 4 MiB 上限之内。
// 顺带把实测字节数打出来，供执行账本引用（工单估的是 120 KB 量级）。
func TestSeasonStatsSynthetic2000MayhemGamesStayWithinBudget(t *testing.T) {
	cache := seasonStatsCache{
		SchemaVersion: seasonStatsCacheSchemaVersion, Source: seasonStatsSource,
		Season: "S26", AccountHash: "account", Complete: true,
	}
	stats := make(map[int64]*gameplaySeasonChampionStat)
	queueStats := make(map[int64]gameplayAggregate)
	start := seasonStartS26.UnixMilli()
	// 一季 2000 场海斗 + 300 场经典排位，全部走生产代码路径（不手搓结构体），
	// 这样测出来的字节数才是真机会写出来的形状。
	for id := int64(1); id <= 2000; id++ {
		queueID := seasonMayhemQueueIDs[int(id)%len(seasonMayhemQueueIDs)]
		augments := []int64{5000 + id%40, 5100 + id%37, 5200 + id%31}
		items := []int64{6672, 3153, 3031, 3006, 6333, 3156}
		info := r116eMayhemMatchInfo(7000000000000+id, queueID, augments, items, id%2 == 0)
		cache.GameIDs = append(cache.GameIDs, info.GameID)
		seasonStatsAccumulate(stats, queueStats, info, "subject", start)
		seasonRecordRankedMatch(&cache, info, "subject")
	}
	for id := int64(1); id <= 300; id++ {
		queueID := int64(420)
		if id%3 == 0 {
			queueID = 440
		}
		info := r116eMayhemMatchInfo(7100000000000+id, queueID, nil, []int64{6672, 3153, 3031, 3006, 6333, 3156}, id%2 == 0)
		cache.GameIDs = append(cache.GameIDs, info.GameID)
		seasonStatsAccumulate(stats, queueStats, info, "subject", start)
		seasonRecordRankedMatch(&cache, info, "subject")
	}
	cache.Stats = seasonStatsFinalize(stats, map[int64]string{157: "亚索"})
	cache.QueueStats = seasonStatsFinalizeQueues(queueStats)
	seasonTrimRankedMatches(&cache)
	augmentDropped := seasonTrimAugmentSamples(&cache)

	if len(cache.AugmentSamples) != 2000 {
		t.Fatalf("augment samples = %d, want 2000 (dropped %d)", len(cache.AugmentSamples), augmentDropped)
	}
	if len(cache.RankedMatches) != 5*seasonRankedMatchLimit {
		t.Fatalf("ranked matches = %d, want 5 queues x %d", len(cache.RankedMatches), seasonRankedMatchLimit)
	}

	data, report := marshalSeasonStatsWithinBudget(cache)
	if report.MarshalErr != nil {
		t.Fatalf("marshal failed: %v", report.MarshalErr)
	}
	if report.Truncated || report.OverBudget {
		t.Fatalf("a 2000-game season must fit without truncation: %#v", report)
	}
	if len(data) > seasonStatsFileBudgetBytes {
		t.Fatalf("file = %d bytes, over the %d budget", len(data), seasonStatsFileBudgetBytes)
	}
	t.Logf("P3 capacity (synthetic, 2000 mayhem + 300 ranked games): file = %d bytes = %.1f KB = %.2f MiB; budget = %d bytes; headroom = %.1fx",
		len(data), float64(len(data))/1024, float64(len(data))/(1024*1024), seasonStatsFileBudgetBytes, float64(seasonStatsFileBudgetBytes)/float64(len(data)))
	t.Logf("P3 breakdown: gameIds=%d stats=%d queueStats=%d rankedMatches=%d augmentSamples=%d",
		len(cache.GameIDs), len(cache.Stats), len(cache.QueueStats), len(cache.RankedMatches), len(cache.AugmentSamples))

	// 单条 augment 样本的平均字节数（用来复核 seasonAugmentSampleLimit 的推算）。
	samplesOnly, err := json.Marshal(cache.AugmentSamples)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("P3 augment samples: %d bytes for %d entries = %.0f B/场（工单按 300 B/场估，本实现只存必要字段）",
		len(samplesOnly), len(cache.AugmentSamples), float64(len(samplesOnly))/float64(len(cache.AugmentSamples)))

	// 端到端写一次，确认落盘大小与 report 一致。
	store := trackTestStore(t, &localStore{root: t.TempDir()})
	if err := store.saveSeasonStats(cache); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(store.root, seasonStatsFileKey(cache.Source, cache.AccountHash, cache.Season))
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != int64(len(data)) {
		t.Fatalf("file on disk = %d bytes, marshal produced %d", info.Size(), len(data))
	}
	// 这份缓存读回来必须能直接喂给 P2-6 的静态查询。
	loaded, err := store.loadSeasonStats(seasonStatsSource, "account", "S26")
	if err != nil {
		t.Fatal(err)
	}
	report2 := seasonAugmentBuildReportFor(loaded.AugmentSamples, 157, 0, seasonAugmentMinimumSample, seasonAugmentBuildGroupLimit, seasonAugmentBuildComboLimit)
	if len(report2.Groups) == 0 {
		t.Fatalf("the reloaded cache must still answer the P2-6 query: %#v", report2)
	}
	t.Logf("P2-6 over the reloaded 2000-game cache: sampleGames=%d groups=%d topGroup=augment %d with %d games / %d%% win rate",
		report2.SampleGames, len(report2.Groups), report2.Groups[0].AugmentID, report2.Groups[0].Games, report2.Groups[0].WinRate)
}

// ---------------------------------------------------------------------------
// P2-4 GE2 / GE2b（R120）：预算第二、三级处置的边界必须有独立测试
// ---------------------------------------------------------------------------

// r120BudgetCache 造一个各部分条数可控的赛季缓存。Stats 固定 173 条（真实上限），
// GameIDs / RankedMatches / AugmentSamples 的条数由调用方给，方便把载荷精确地
// 撑到预算的某一侧。所有条目都是唯一的 GameID，所以 seasonTrimAugmentSamples
// 的去重不会改变条数。
func r120BudgetCache(gameIDs, augmentSamples, rankedMatches int) seasonStatsCache {
	cache := seasonStatsCache{
		SchemaVersion: seasonStatsCacheSchemaVersion, Source: seasonStatsSource,
		Season: "S26", AccountHash: "account", Complete: true,
		UpdatedAt:   time.Unix(1760000000, 0).UTC(),
		ResumeIndex: 3, PendingIndex: 4,
		QueueStats: map[int64]gameplayAggregate{
			2400: {QueueID: 2400, QueueLabel: "海克斯大乱斗", Games: 40, Wins: 20, Losses: 20, WinRate: 50, Kills: 8.5, Deaths: 4.2, Assists: 9.1, KDA: 4.19},
		},
	}
	for id := int64(1); id <= 173; id++ {
		cache.Stats = append(cache.Stats, gameplaySeasonChampionStat{
			ChampionID: id, ChampionName: strings.Repeat("英雄名", 4), Games: 40, Wins: 20,
			Kills: 8.5, Deaths: 4.2, Assists: 9.1, TotalCS: 6000, Duration: 72000,
			WinRate: 50, KDA: 4.19, CS: 148.2, CSPerMinute: 12.4,
		})
	}
	for id := 0; id < gameIDs; id++ {
		cache.GameIDs = append(cache.GameIDs, 7000000000000+int64(id))
	}
	for index := 0; index < rankedMatches; index++ {
		cache.RankedMatches = append(cache.RankedMatches, seasonRankedMatch{
			GameID: 7200000000000 + int64(index), CreatedAt: 1700000000000 + int64(index)*1000,
			QueueID: 2400, ChampionID: 157, Win: index%2 == 0,
			Kills: 8, Deaths: 4, Assists: 9, TeamKills: 40, Position: "",
		})
	}
	for id := 1; id <= augmentSamples; id++ {
		cache.AugmentSamples = append(cache.AugmentSamples, seasonAugmentSample{
			GameID: 7100000000000 + int64(id), ChampionID: 157,
			AugmentIDs: []int64{5001, 5002, 5003},
			ItemIDs:    []int64{6672, 3153, 3031, 3006},
			Win:        id%2 == 0,
		})
	}
	return cache
}

func r120MarshalSize(t *testing.T, cache seasonStatsCache) int {
	t.Helper()
	data, err := json.Marshal(cache)
	if err != nil {
		t.Fatal(err)
	}
	return len(data)
}

func r120RequireStep(t *testing.T, report seasonStatsBudgetReport, step string) {
	t.Helper()
	for _, got := range report.Steps {
		if got == step {
			return
		}
	}
	t.Fatalf("处置步骤里没有 %q：%#v", step, report.Steps)
}

// GE2 的实测事实：在第二级处置（augment_samples_dropped 那一步）里顺手写一行
// trimmed.Stats = nil，整包测试全绿。原来的测试只断言「Stats 条数没少」，而且没有
// 任何输入真的走到第三级。这里把基座（Stats + GameIDs，两者都不可截断）单独撑到
// 超预算，逼处置走到第三级，再逐项比对写出来的内容：只允许 AugmentSamples 变化，
// RankedMatches 只能变成第一级裁剪后的那 seasonRankedMatchLimit 条，其余字段一律逐项相等。
func TestSeasonStatsBudgetThirdStageOnlyDropsAugmentSamples(t *testing.T) {
	cache := r120BudgetCache(400000, 800, 120)
	raw := r120MarshalSize(t, cache)
	if raw <= seasonStatsFileBudgetBytes {
		t.Fatalf("fixture 必须超预算，实际 %d bytes（预算 %d）", raw, seasonStatsFileBudgetBytes)
	}
	data, report := marshalSeasonStatsWithinBudget(cache)
	if report.MarshalErr != nil {
		t.Fatalf("marshal failed: %v", report.MarshalErr)
	}
	if !report.OverBudget {
		t.Fatalf("基座本身就超预算，处置必须走到第三级并如实报 OverBudget：%#v", report)
	}
	r120RequireStep(t, report, "ranked_matches_capped")
	r120RequireStep(t, report, "augment_samples_dropped")
	r120RequireStep(t, report, "written_over_budget")
	if report.AugmentSamplesAfter != 0 {
		t.Fatalf("第三级意味着样本已被全部丢弃，实际剩 %d", report.AugmentSamplesAfter)
	}
	var written seasonStatsCache
	if err := json.Unmarshal(data, &written); err != nil {
		t.Fatalf("第三级写出来的必须是合法 JSON：%v", err)
	}
	// Stats / GameIDs / QueueStats / 元数据：逐项相等，一条都不许动。
	if !reflect.DeepEqual(written.Stats, cache.Stats) {
		t.Fatalf("Stats 在第二/三级处置里被改动了：%d -> %d 条", len(cache.Stats), len(written.Stats))
	}
	if !reflect.DeepEqual(written.GameIDs, cache.GameIDs) {
		t.Fatalf("GameIDs 在第二/三级处置里被改动了：%d -> %d 条", len(cache.GameIDs), len(written.GameIDs))
	}
	if !reflect.DeepEqual(written.QueueStats, cache.QueueStats) {
		t.Fatalf("QueueStats 被改动了：%#v", written.QueueStats)
	}
	if written.SchemaVersion != cache.SchemaVersion || written.Source != cache.Source || written.Season != cache.Season ||
		written.AccountHash != cache.AccountHash || written.Complete != cache.Complete ||
		written.ResumeIndex != cache.ResumeIndex || written.PendingIndex != cache.PendingIndex ||
		!written.UpdatedAt.Equal(cache.UpdatedAt) {
		t.Fatalf("缓存元数据被改动了：%#v", written)
	}
	// RankedMatches：第一级之后就是 seasonRankedMatchLimit 条，第二/三级不许再动它。
	expected := cache
	seasonStatsCapRankedMatches(&expected, seasonRankedMatchLimit)
	if !reflect.DeepEqual(written.RankedMatches, expected.RankedMatches) {
		t.Fatalf("RankedMatches 与第一级裁剪结果不一致：写出 %d 条，期望 %d 条", len(written.RankedMatches), len(expected.RankedMatches))
	}
	if len(written.AugmentSamples) != 0 {
		t.Fatalf("第三级下样本应为空，实际 %d 条", len(written.AugmentSamples))
	}
	// 调用方手里的 cache 不能被就地改坏（finishSeasonScan 之后还要用它报诊断）。
	if len(cache.RankedMatches) != 120 || len(cache.AugmentSamples) != 800 || len(cache.Stats) != 173 {
		t.Fatalf("调用方的 cache 被就地修改了：ranked=%d augments=%d stats=%d", len(cache.RankedMatches), len(cache.AugmentSamples), len(cache.Stats))
	}
	t.Logf("GE2 第三级：raw=%d written=%d steps=%v ranked %d->%d augments %d->%d stats 保持 %d 条",
		report.RawBytes, report.WrittenBytes, report.Steps, report.RankedMatchesBefore, report.RankedMatchesAfter,
		report.AugmentSamplesBefore, report.AugmentSamplesAfter, len(written.Stats))
}

// GE2b 的实测事实：把第二级一上来写成 trimmed.AugmentSamples = nil（不再逐步减半），
// 整包测试同样全绿。这条测试构造「全量样本刚好超预算一点、砍掉一半就合规」的输入，
// 断言处置停在第一个能进预算的档位：结果非空，而且再多留一倍必然超限——
// 也就是「尽量保留」，既不是清空，也不是过度截断。
func TestSeasonStatsBudgetKeepsAsManyAugmentSamplesAsFit(t *testing.T) {
	const samples = seasonAugmentSampleLimit // 2000：与生产上限一致，去重后条数不变
	const ranked = 120                       // 超过 seasonRankedMatchLimit，确保第一级一定会触发
	base := r120MarshalSize(t, r120BudgetCache(0, 0, ranked))
	withSamples := r120MarshalSize(t, r120BudgetCache(0, samples, ranked))
	withGameIDs := r120MarshalSize(t, r120BudgetCache(10000, 0, ranked))
	// 单条开销必须用浮点量：GameID 实际占 14 字节，整数除法会算成 13，外推到
	// 28 万条就冲过预算约 250 KB，fixture 直接不成立。
	perSample := float64(withSamples-base) / float64(samples)
	perGameID := float64(withGameIDs-base) / 10000
	if perSample <= 0 || perGameID <= 0 {
		t.Fatalf("量不出单条开销：perSample=%v perGameID=%v", perSample, perGameID)
	}
	// 解出把基座撑进目标窗口需要多少条 GameID：
	//   全量样本 > 预算，且一半样本 <= 预算。不写死魔法数字，字段变了也不会悄悄失效。
	budget := float64(seasonStatsFileBudgetBytes)
	low := math.Ceil((budget-float64(base)-float64(samples)*perSample)/perGameID) + 1
	high := math.Floor((budget - float64(base) - float64(samples/2)*perSample) / perGameID)
	if high < low {
		t.Fatalf("构造不出「全量超限、砍半合规」的窗口：low=%v high=%v base=%d perSample=%v perGameID=%v", low, high, base, perSample, perGameID)
	}
	gameIDs := int((low + high) / 2)
	// 实测校正：JSON 里 null / omitempty 的边界会让线性外推差几 KB。收不进窗口就
	// 直接失败，绝不允许悄悄换一个不成立的 fixture 蒙过去。
	var cache seasonStatsCache
	var raw int
	for attempt := 0; ; attempt++ {
		cache = r120BudgetCache(gameIDs, samples, ranked)
		raw = r120MarshalSize(t, cache)
		halfProbe := cache
		halfProbe.AugmentSamples = halfProbe.AugmentSamples[:samples/2]
		halfSize := r120MarshalSize(t, halfProbe)
		if raw > seasonStatsFileBudgetBytes && halfSize <= seasonStatsFileBudgetBytes {
			t.Logf("GE2b 窗口：gameIDs=%d raw=%d 一半样本=%d 预算=%d（校正 %d 次，perSample=%.1fB perGameID=%.2fB）",
				gameIDs, raw, halfSize, seasonStatsFileBudgetBytes, attempt, perSample, perGameID)
			break
		}
		if attempt >= 64 {
			t.Fatalf("64 次校正后仍构造不出「全量超限、砍半合规」的窗口：gameIDs=%d raw=%d half=%d budget=%d", gameIDs, raw, halfSize, seasonStatsFileBudgetBytes)
		}
		if raw <= seasonStatsFileBudgetBytes {
			gameIDs += 256
		} else {
			gameIDs -= 256
		}
	}

	data, report := marshalSeasonStatsWithinBudget(cache)
	if report.MarshalErr != nil {
		t.Fatalf("marshal failed: %v", report.MarshalErr)
	}
	if len(data) > seasonStatsFileBudgetBytes {
		t.Fatalf("written = %d bytes，超过预算 %d", len(data), seasonStatsFileBudgetBytes)
	}
	if report.OverBudget {
		t.Fatalf("这个输入砍一半就能合规，不该报 OverBudget：%#v", report)
	}
	r120RequireStep(t, report, "ranked_matches_capped")
	r120RequireStep(t, report, "augment_samples_trimmed")
	kept := report.AugmentSamplesAfter
	if kept <= 0 {
		t.Fatalf("第二级不许把样本清空（GE2b）：%#v", report)
	}
	if kept >= samples {
		t.Fatalf("这个输入必须触发减半，实际保留了全部 %d 条", kept)
	}
	var written seasonStatsCache
	if err := json.Unmarshal(data, &written); err != nil {
		t.Fatal(err)
	}
	if len(written.AugmentSamples) != kept {
		t.Fatalf("报告说保留 %d 条，载荷里实际 %d 条", kept, len(written.AugmentSamples))
	}
	// 第二级只许动 AugmentSamples：Stats / GameIDs 逐项相等，RankedMatches 停在第一级结果。
	if !reflect.DeepEqual(written.Stats, cache.Stats) {
		t.Fatalf("Stats 被第二级改动了：%d -> %d 条", len(cache.Stats), len(written.Stats))
	}
	if !reflect.DeepEqual(written.GameIDs, cache.GameIDs) {
		t.Fatalf("GameIDs 被第二级改动了：%d -> %d 条", len(cache.GameIDs), len(written.GameIDs))
	}
	capped := cache
	seasonStatsCapRankedMatches(&capped, seasonRankedMatchLimit)
	if !reflect.DeepEqual(written.RankedMatches, capped.RankedMatches) {
		t.Fatalf("RankedMatches 与第一级裁剪结果不一致：%d vs %d 条", len(written.RankedMatches), len(capped.RankedMatches))
	}
	// 「刚好合规」的反证：上一档（保留两倍样本）必须超限，否则说明这一轮多丢了一半。
	previous := cache
	seasonTrimAugmentSamples(&previous)
	seasonStatsCapRankedMatches(&previous, seasonRankedMatchLimit)
	if len(previous.AugmentSamples) < 2*kept {
		t.Fatalf("去重后的样本不足上一档所需：%d < %d", len(previous.AugmentSamples), 2*kept)
	}
	previous.AugmentSamples = previous.AugmentSamples[:2*kept]
	if size := r120MarshalSize(t, previous); size <= seasonStatsFileBudgetBytes {
		t.Fatalf("上一档 %d 条样本（%d bytes）本来就能进预算，处置却只留了 %d 条：没有停在第一个合规档位", 2*kept, size, kept)
	}
	if len(cache.AugmentSamples) != samples || len(cache.RankedMatches) != ranked {
		t.Fatalf("调用方的 cache 被就地修改了：augments=%d ranked=%d", len(cache.AugmentSamples), len(cache.RankedMatches))
	}
	t.Logf("GE2b 刚好合规：raw=%d written=%d gameIDs=%d perSample=%.1fB 样本 %d->%d（上一档 %d 条超限）steps=%v",
		report.RawBytes, report.WrittenBytes, gameIDs, perSample, report.AugmentSamplesBefore, kept, 2*kept, report.Steps)
}
