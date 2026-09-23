package main

// R116-E P1：season-stats 磁盘预算。
//
// 为什么必须新建而不是「复核 binary_disk_budget.go」：那套治理的是图片缓存
// championDataCache.strictDisk（只被 champion_images.go 的 newPublicBinaryCache
// 打开），是为高频写入的二进制图片设计的严格记账。season-stats 是完全不同的
// 存储——低频的整文件 JSON 重写，走 storage.go 的 writeLocalStoreFile →
// atomicWriteFile，改之前全流程没有任何大小/条目上限，目录也没有被任何 prune
// 逻辑扫描过（工单 P1「现状证据」第 2 条、评审第 2 节均已核实）。
//
// Anti-scope 第 3 条：这里的逻辑不许塞进 binary_disk_budget.go，两套存储混在
// 一起只会增加不必要的耦合。
//
// 容量口径（按真实的 20~50 场/页波动估算，不是工单里那个错误的「20 场/页」）：
//
//	前台 2 页 = 40~100 场，后台 12 页 = 240~600 场（见 season_stats.go 常量块）。
//	放开海斗后单文件构成：
//	  GameIDs        整季 2000 场 × ~15 B          ≈  30 KB（本来就全队列记录）
//	  Stats          ≤173 条英雄聚合               ≈  15 KB
//	  QueueStats     5 个队列                      ≈ 0.5 KB
//	  RankedMatches  5 队列 × 40 条 × ~450 B       ≈  90 KB
//	  AugmentSamples 2000 条 × ~130 B              ≈ 260 KB
//	  合计                                          ≈ 400 KB
//
// 4 MiB 的单文件上限对这个量级留了约 10 倍余量，正常情况永远碰不到；它的意义
// 是兜住「上游字段膨胀 / 样本上限被改大 / 某个账号异常」这类意外，不让一个
// 挂在总览首屏路径上的同步全量重写无界增长。

import (
	"encoding/json"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// seasonStatsFileBudgetBytes 是单个 season-stats 缓存文件的大小上限（4 MiB）。
const seasonStatsFileBudgetBytes = 4 * 1024 * 1024

// seasonStatsBudgetReport 记录一次写入的预算处置结果，供诊断事件使用。
// 排障时要能一眼看出「这一轮到底截断了什么、最后写了多大」。
type seasonStatsBudgetReport struct {
	// BudgetBytes 是当时生效的上限，写进日志免得日后改了常量对不上账。
	BudgetBytes int `json:"budgetBytes"`
	// RawBytes 是未做任何截断时的序列化大小。
	RawBytes int `json:"rawBytes"`
	// WrittenBytes 是真正落盘的大小。
	WrittenBytes         int `json:"writtenBytes"`
	RankedMatchesBefore  int `json:"rankedMatchesBefore"`
	RankedMatchesAfter   int `json:"rankedMatchesAfter"`
	AugmentSamplesBefore int `json:"augmentSamplesBefore"`
	AugmentSamplesAfter  int `json:"augmentSamplesAfter"`
	// Truncated 表示为了进预算丢过东西。
	Truncated bool `json:"truncated"`
	// OverBudget 表示所有可截断项都用尽了仍然超限。此时仍然写入
	// （Stats 是用户唯一拿不回来的数据），只把事实报出来。
	OverBudget bool     `json:"overBudget"`
	MarshalErr error    `json:"-"`
	Steps      []string `json:"steps,omitempty"`
}

// marshalSeasonStatsWithinBudget 序列化赛季缓存，超过 seasonStatsFileBudgetBytes
// 时按固定顺序截断，返回落盘字节与处置报告。
//
// 处置顺序（工单 P1 第 1 条）：
//  1. 先把 RankedMatches 收到最新的 seasonRankedMatchLimit 条——这是既有上限，
//     本来就有这个语义，只是按队列各留 40 条，这里收紧成全局 40 条；
//  2. 再把 AugmentSamples 去重排序并逐次减半；
//  3. 都不够就照原样写入并置 OverBudget。
//
// 绝不做的事：整体拒绝写入、丢弃 Stats、panic。
// 注意 cache 是值拷贝，但切片共享底层数组——这里只做重新切片（reslice），
// 不改任何元素，所以不会影响调用方手里的 cache。
func marshalSeasonStatsWithinBudget(cache seasonStatsCache) ([]byte, seasonStatsBudgetReport) {
	report := seasonStatsBudgetReport{
		BudgetBytes:          seasonStatsFileBudgetBytes,
		RankedMatchesBefore:  len(cache.RankedMatches),
		RankedMatchesAfter:   len(cache.RankedMatches),
		AugmentSamplesBefore: len(cache.AugmentSamples),
		AugmentSamplesAfter:  len(cache.AugmentSamples),
	}
	trimmed := cache
	data, err := json.Marshal(trimmed)
	if err != nil {
		report.MarshalErr = err
		return nil, report
	}
	report.RawBytes = len(data)
	if len(data) <= seasonStatsFileBudgetBytes {
		report.WrittenBytes = len(data)
		return data, report
	}

	// 第 1 级：截断 RankedMatches（优先项）。
	seasonStatsCapRankedMatches(&trimmed, seasonRankedMatchLimit)
	report.RankedMatchesAfter = len(trimmed.RankedMatches)
	if report.RankedMatchesAfter != report.RankedMatchesBefore {
		report.Truncated = true
		report.Steps = append(report.Steps, "ranked_matches_capped")
	}
	if data, err = json.Marshal(trimmed); err != nil {
		report.MarshalErr = err
		return nil, report
	}
	if len(data) <= seasonStatsFileBudgetBytes {
		report.WrittenBytes = len(data)
		return data, report
	}

	// 第 2 级：AugmentSamples 去重 + 排序 + 逐次减半。样本是 P2-6 的原料，
	// 丢了可以在下一次整季重扫时恢复，所以排在 RankedMatches 之后、Stats 之前。
	seasonTrimAugmentSamples(&trimmed)
	for len(trimmed.AugmentSamples) > 0 {
		report.AugmentSamplesAfter = len(trimmed.AugmentSamples)
		if data, err = json.Marshal(trimmed); err != nil {
			report.MarshalErr = err
			return nil, report
		}
		if len(data) <= seasonStatsFileBudgetBytes {
			report.Truncated = true
			report.Steps = append(report.Steps, "augment_samples_trimmed")
			report.WrittenBytes = len(data)
			return data, report
		}
		trimmed.AugmentSamples = trimmed.AugmentSamples[:len(trimmed.AugmentSamples)/2]
	}
	report.AugmentSamplesAfter = len(trimmed.AugmentSamples)
	report.Truncated = true
	report.Steps = append(report.Steps, "augment_samples_dropped")

	// 第 3 级：可截断项已用尽。Stats 与 GameIDs 一律不动，照原样写入。
	data, err = json.Marshal(trimmed)
	if err != nil {
		report.MarshalErr = err
		return nil, report
	}
	report.WrittenBytes = len(data)
	report.OverBudget = len(data) > seasonStatsFileBudgetBytes
	if report.OverBudget {
		report.Steps = append(report.Steps, "written_over_budget")
	}
	return data, report
}

// seasonStatsCapRankedMatches 把 RankedMatches 按 CreatedAt 倒序裁到 limit 条。
// 与 seasonTrimRankedMatches 的区别：那个是「每个队列各留 40 条」（正常口径），
// 这个是磁盘兜底用的「全局只留 40 条」，只在超限时才走到。
func seasonStatsCapRankedMatches(cache *seasonStatsCache, limit int) {
	if limit <= 0 || len(cache.RankedMatches) <= limit {
		return
	}
	items := append([]seasonRankedMatch(nil), cache.RankedMatches...)
	sort.SliceStable(items, func(i, j int) bool { return items[i].CreatedAt > items[j].CreatedAt })
	cache.RankedMatches = items[:limit]
}

// seasonStatsDirectoryHealth 是 season-stats/ 目录的一次性体检结果。
type seasonStatsDirectoryHealth struct {
	Root            string `json:"root"`
	Exists          bool   `json:"exists"`
	Files           int    `json:"files"`
	TotalBytes      int64  `json:"totalBytes"`
	LargestBytes    int64  `json:"largestBytes"`
	LargestFile     string `json:"largestFile,omitempty"`
	OverBudgetFiles int    `json:"overBudgetFiles"`
	// Err 只记录「读目录本身失败」；目录不存在不算错误（新用户还没有缓存）。
	Err string `json:"err,omitempty"`
}

// seasonStatsDirectoryHealthFor 统计 <root>/season-stats 的总大小与最大单文件。
// 纯读操作，不发请求、不写任何东西，可以拿临时目录直接测。
func seasonStatsDirectoryHealthFor(root string) seasonStatsDirectoryHealth {
	health := seasonStatsDirectoryHealth{Root: root}
	if strings.TrimSpace(root) == "" {
		health.Err = "storage root unavailable"
		return health
	}
	dir := filepath.Join(root, "season-stats")
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			return infoErr
		}
		health.Files++
		health.TotalBytes += info.Size()
		if info.Size() > health.LargestBytes {
			health.LargestBytes = info.Size()
			// 记相对路径而不是绝对路径：绝对路径里带用户名，
			// 诊断事件不该把本机目录结构原样带出去。
			if relative, relErr := filepath.Rel(dir, path); relErr == nil {
				health.LargestFile = relative
			} else {
				health.LargestFile = entry.Name()
			}
		}
		if info.Size() > seasonStatsFileBudgetBytes {
			health.OverBudgetFiles++
		}
		return nil
	})
	if err != nil {
		// 目录还没被创建过是完全正常的（新用户 / 从未打开过总览页）。
		if strings.Contains(err.Error(), "no such file or directory") || strings.Contains(err.Error(), "cannot find the file") || strings.Contains(err.Error(), "cannot find the path") {
			return health
		}
		health.Err = safeDiagnosticReason(err)
	}
	health.Exists = health.Files > 0 || health.Err == ""
	return health
}

var (
	seasonStatsHealthMu   sync.Mutex
	seasonStatsHealthDone = make(map[string]bool)
)

// seasonStatsBudgetMarkChecked 保证每个存储根只做一次目录体检。
// 工单 P1 要求「一次性的目录级健康检查（应用启动或首次扫描时跑一次）」，
// 明确不要做成实时监控，所以这里用最简单的进程内去重。
// app 结构体在 main.go（本轮禁触），因此不往 app 上加字段。
func seasonStatsBudgetMarkChecked(root string) bool {
	if strings.TrimSpace(root) == "" {
		return false
	}
	seasonStatsHealthMu.Lock()
	defer seasonStatsHealthMu.Unlock()
	if seasonStatsHealthDone == nil {
		seasonStatsHealthDone = make(map[string]bool)
	}
	if seasonStatsHealthDone[root] {
		return false
	}
	seasonStatsHealthDone[root] = true
	return true
}

// checkSeasonStatsDirectoryHealth 在首次赛季扫描收尾时跑一次目录体检并记诊断。
func (a *app) checkSeasonStatsDirectoryHealth() {
	if a == nil || a.storage == nil {
		return
	}
	root := a.storage.root
	if !seasonStatsBudgetMarkChecked(root) {
		return
	}
	health := seasonStatsDirectoryHealthFor(root)
	event := map[string]any{
		"event": "season_stats_directory_health",
		"files": health.Files, "total_bytes": health.TotalBytes,
		"budget_bytes":      seasonStatsFileBudgetBytes,
		"largest_bytes":     health.LargestBytes,
		"over_budget_files": health.OverBudgetFiles,
	}
	if health.LargestFile != "" {
		event["largest_file"] = health.LargestFile
	}
	if health.Err != "" {
		event["reason"] = health.Err
	}
	a.recordDiagnostic(event)
}
