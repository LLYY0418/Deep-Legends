package main

import (
	"container/list"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var seasonStartS26 = time.Date(2026, 1, 8, 0, 0, 0, 0, time.UTC)

// seasonStatsCacheSchemaVersion 是缓存的口径版本。持久化键与载荷都记录来源，
// 不同来源的对局结构和统计口径不能共用缓存；口径一变就升版本号，让旧文件在
// loadSeasonStats 里被判为无效并触发重扫。逐版演进：
//
//	7  引入「来源」维度（键与载荷都带 source）。
//	8  gameplaySeasonChampionStat 的 Kills/Deaths/Assists 由「整季累计」改成
//	   「逐场平均」（float64），KDA 改用 ratioFloat，旧缓存的数字口径不兼容。
//	9  放开队列过滤：海克斯大乱斗（2300/2400/3270）计入 Stats/QueueStats/
//	   RankedMatches，并新增 AugmentSamples 逐场海克斯与成装快照。
const seasonStatsCacheSchemaVersion = 9

const seasonStatsSource = dataSourceSGP

const (
	// 前台只允许翻这么多页。单页大小不是固定值：正常路径走 sgpPageSize = 50
	// （sgp_api.go），命中缓存回退时会收窄到 sgpFallbackPageSize = 20
	// （matchHistoryFilteredOn），所以 2 页真实覆盖 40~100 场。这个页数的意义
	// 是把上次之后的新对局补齐，又不会让总览首屏等待超过 1 秒。
	seasonScanForegroundPages = 2
	// 后台单轮回补的页数上限。同样按 20~50 场/页波动，12 页真实覆盖
	// 240~600 场；仍然有界，避免一次把整季几百场全压在一个请求里。
	seasonScanBackgroundPages = 12
	// 排位样本保留条数：近 20 场排位只要 20 条，多留一些做筛选余量。
	// seasonTrimRankedMatches 按队列各自裁到这个上限，所以放开海斗之后
	// RankedMatches 的总条数上限是 5 个队列 × 40 = 200 条。
	seasonRankedMatchLimit = 40
	// 后台回补任务的总时限，防止上游变慢时任务无限期挂着。
	seasonBackfillTimeout = 90 * time.Second
	// R132 P1：同一个回补任务最多连续跑几轮（12 页 × 20~50 场/轮，6 轮覆盖
	// 1440~3600 场），轮间短暂停顿，避免连续几十页压在 SGP 上。
	seasonBackfillMaxRounds = 6
)

// seasonBackfillRoundPause 是轮间停顿（变量便于测试调小）。
var seasonBackfillRoundPause = 100 * time.Millisecond

// 计入个人赛季统计的队列。420/440 是经典排位；2300/2400/3270 是海克斯大乱斗
// ——queue_groups.go:120-124 里这三个 ID 的官方中文名都是「海克斯大乱斗」，
// 同属 hextech-aram 组，所以 UI 上合并成一个页签。
// ⚠️ 3220 是「极地大乱斗」(aram 组)，不是海斗，不在这里；误加会把普通 ARAM
// 的场次算进海克斯大乱斗的口径里。
const (
	seasonQueueSoloDuo = int64(420)
	seasonQueueFlex    = int64(440)
)

// seasonMayhemQueueIDs 是海克斯大乱斗的全部队列 ID（升序）。
var seasonMayhemQueueIDs = []int64{2300, 2400, 3270}

// seasonMayhemPrimaryQueueID 是海斗页签在前端的标识 ID。三个海斗队列合并成
// 一个页签，需要一个稳定的 key；取组内最小的常规 5v5 海斗队列 2400 之外，
// 这里选 2300（组内第一个），语义是「页签标识」而不是「只统计这个队列」。
const seasonMayhemPrimaryQueueID = int64(2300)

// seasonStatsQueueAllowed 是赛季统计的唯一队列判据。
// seasonStatsAccumulate 与 seasonRecordRankedMatch 必须同时用它：判据一旦
// 分叉，「近 20 场」和赛季英雄统计就会对不上账（R116-E 之前这条约束只覆盖
// 420/440，放开海斗后对新增的三个队列同样适用）。
func seasonStatsQueueAllowed(queueID int64) bool {
	if queueID == seasonQueueSoloDuo || queueID == seasonQueueFlex {
		return true
	}
	return seasonMayhemQueue(queueID)
}

// seasonMayhemQueue 判断一个队列是否属于海克斯大乱斗组。
func seasonMayhemQueue(queueID int64) bool {
	for _, id := range seasonMayhemQueueIDs {
		if id == queueID {
			return true
		}
	}
	return false
}

// seasonRankedMatch 是从赛季扫描里顺手留下的单场快照（经典排位 + 海克斯大乱斗）。
// 赛季扫描本来就要把每一场的 JSON 解出来，这里多记几个字段是零额外网络开销的；
// 有了它，「近 20 场排位」就不再受限于首屏那 20 条战绩里恰好有几场排位，
// 海克斯大乱斗页签也不必再额外打一次带队列过滤的战绩请求。
type seasonRankedMatch struct {
	GameID     int64  `json:"gameId"`
	CreatedAt  int64  `json:"createdAt"`
	QueueID    int64  `json:"queueId"`
	ChampionID int64  `json:"championId"`
	Win        bool   `json:"win"`
	Kills      int    `json:"kills"`
	Deaths     int    `json:"deaths"`
	Assists    int    `json:"assists"`
	TeamKills  int    `json:"teamKills"`
	Position   string `json:"position,omitempty"`
	// Ability 是七维能力雷达所需的全部输入，在赛季扫描时顺手算好。
	// 能力雷达原本只吃首屏那 20 场详情战绩，只要玩家最近 20 场里没有
	// 灵活组排，灵活那一栏就永远是空的（而近 20 场排位却因为有这个快照
	// 兜底而有数据，两张卡片自相矛盾）。多存这几个数是零额外网络开销的。
	Ability *seasonRankedAbilitySample `json:"ability,omitempty"`
}

// seasonRankedAbilitySide 是一名选手在这一场里的能力雷达原料。
type seasonRankedAbilitySide struct {
	Kills      int `json:"kills"`
	Deaths     int `json:"deaths"`
	Assists    int `json:"assists"`
	Damage     int `json:"damage"`
	CS         int `json:"cs"`
	Gold       int `json:"gold"`
	Vision     int `json:"vision"`
	TeamKills  int `json:"teamKills"`
	TeamDamage int `json:"teamDamage"`
}

// seasonRankedAbilitySample 把玩家本人与其同位置对位的那名对手一起存下来；
// 基线口径必须与 gameplayAbilityStatsForQueue 完全一致（同位置、敌方队伍、
// 每场只取第一个匹配到的对手），否则两条路径算出来的雷达会对不上。
type seasonRankedAbilitySample struct {
	Duration int64                    `json:"duration"`
	Player   seasonRankedAbilitySide  `json:"player"`
	Opponent *seasonRankedAbilitySide `json:"opponent,omitempty"`
}

// 海克斯大乱斗一场的海克斯槽位与装备槽位数量。样本只保留非零槽位，
// 但槽位数本身是有界的：6 个海克斯 + 7 个装备（含饰品栏）。
const (
	seasonAugmentSlots = 6
	seasonItemSlots    = 7
)

// seasonAugmentSampleLimit 是本人海斗样本的条数上限。推算过程（工单 P3 只给了
// 量级，这里给出可复核的算术）：
//
//	单条 JSON 实测约 130 B（gameId 14 位 + championId + 3~6 个海克斯 ID
//	+ 6 个装备 ID + win），2000 条 ≈ 260 KB。
//	加上 RankedMatches（5 队列 × 40 条 × ~450 B ≈ 90 KB）、Stats（≤173 条
//	≈ 15 KB）、QueueStats（5 条 ≈ 0.5 KB）、GameIDs（整季 2000 场 × 15 B
//	≈ 30 KB），单文件合计约 400 KB，对 seasonStatsFileBudgetBytes 的 4 MiB
//	上限留了约 10 倍余量。
//
// 工单算的「精简路线 300 B/场 × 2000 场 = 0.6 MB」把伤害/经济等字段也算进去了，
// 本实现按 Anti-scope 第 4 条只存必要字段，所以真实占用更低；上限仍取 2000，
// 与工单的量级一致，也覆盖「一季 2000 场海斗的高频玩家」这个估算基准。
const seasonAugmentSampleLimit = 2000

// seasonAugmentSample 是一场海克斯大乱斗里本人的海克斯与成装快照，
// 与 seasonRankedMatch.Ability 同属「赛季扫描顺手记」的模式：扫描本来就要
// 解出这一场的 participant JSON，多留几个字段是零额外网络请求。
//
// 只存 P2-6「选了某个海克斯之后通常出什么」这一个静态查询必要的字段。
// 伤害/承伤/经济/22 项 postmatch 指标一律不存——那是英雄级统计，走
// R116-A 的 /api/hexdata/postmatch，不在个人对局记录里重复存一份
// （Anti-scope 第 4 条）。
//
// 只在海克斯大乱斗队列（2300/2400/3270）记录，经典排位没有海克斯。
type seasonAugmentSample struct {
	GameID     int64 `json:"gameId"`
	ChampionID int64 `json:"championId"`
	// AugmentIDs 按槽位顺序保留非零海克斯 ID，最多 seasonAugmentSlots 个。
	AugmentIDs []int64 `json:"augmentIds,omitempty"`
	// ItemIDs 按槽位顺序保留非零装备 ID，最多 seasonItemSlots 个。
	ItemIDs []int64 `json:"itemIds,omitempty"`
	Win     bool    `json:"win"`
}

// seasonAugmentSampleFor 从一场海斗对局里抽出本人的海克斯与成装。
// 不是海斗队列、或本人不在参与者名单里时返回 nil（不记录）。
func seasonAugmentSampleFor(info *riotMatchInfo, playerRef string) *seasonAugmentSample {
	if info == nil || !seasonMayhemQueue(info.QueueID) {
		return nil
	}
	for _, participant := range info.Participants {
		if participant.PUUID != playerRef || participant.ChampionID <= 0 {
			continue
		}
		augments := make([]int64, 0, seasonAugmentSlots)
		for _, id := range []int64{
			participant.PlayerAugment1, participant.PlayerAugment2, participant.PlayerAugment3,
			participant.PlayerAugment4, participant.PlayerAugment5, participant.PlayerAugment6,
		} {
			if id > 0 {
				augments = append(augments, id)
			}
		}
		items := make([]int64, 0, seasonItemSlots)
		for _, id := range []int64{
			participant.Item0, participant.Item1, participant.Item2, participant.Item3,
			participant.Item4, participant.Item5, participant.Item6,
		} {
			if id > 0 {
				items = append(items, id)
			}
		}
		// 没有海克斯的场次对 P2-6 的「选了 X 之后出什么」没有贡献，不留。
		if len(augments) == 0 {
			return nil
		}
		return &seasonAugmentSample{
			GameID: info.GameID, ChampionID: participant.ChampionID,
			AugmentIDs: augmentIDsOrNil(augments), ItemIDs: augmentIDsOrNil(items),
			Win: participant.Win,
		}
	}
	return nil
}

// augmentIDsOrNil 让空切片序列化成 omitempty 的「字段缺失」而不是 `[]`，
// 2000 场样本每场省下 20 字节左右的空数组开销。
func augmentIDsOrNil(ids []int64) []int64 {
	if len(ids) == 0 {
		return nil
	}
	return ids
}

// seasonTrimAugmentSamples 去重后裁到 seasonAugmentSampleLimit，返回被丢弃的条数。
// 与 seasonTrimRankedMatches 一样必须去重——同一场可能在增量追新与后台回补里
// 各记一次。
//
// 排序口径说明：样本按工单 Anti-scope 第 4 条只存必要字段，没有 createdAt，
// 而追加顺序也不等于时间顺序（后台回补是往回翻旧页、追加在末尾）。所以裁剪
// 按 GameID 降序保留：Riot/SGP 的 gameId 是平台侧递增分配的，同一账号同一
// 服务器下 GameID 越大越新。这个假设只用于「超过 2000 条时先丢哪些」，
// 不参与任何对外展示的数字计算；即使假设失效，影响也只是被淘汰的场次选择，
// 不会让任何统计口径变成假的。
func seasonTrimAugmentSamples(cache *seasonStatsCache) int {
	if len(cache.AugmentSamples) == 0 {
		return 0
	}
	unique := make(map[int64]seasonAugmentSample, len(cache.AugmentSamples))
	for _, item := range cache.AugmentSamples {
		if item.GameID <= 0 {
			continue
		}
		unique[item.GameID] = item
	}
	result := make([]seasonAugmentSample, 0, len(unique))
	for _, item := range unique {
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].GameID > result[j].GameID })
	dropped := len(cache.AugmentSamples) - len(result)
	if len(result) > seasonAugmentSampleLimit {
		dropped += len(result) - seasonAugmentSampleLimit
		result = result[:seasonAugmentSampleLimit]
	}
	cache.AugmentSamples = result
	return dropped
}

func currentRankedSeason(now time.Time) (string, time.Time) {
	start := seasonStartS26
	for now.Before(start) {
		start = start.AddDate(-1, 0, 0)
	}
	for !now.Before(start.AddDate(1, 0, 0)) {
		start = start.AddDate(1, 0, 0)
	}
	return fmt.Sprintf("S%d", start.Year()%100), start
}

type gameplaySeasonChampionStat struct {
	ChampionID   int64   `json:"championId"`
	ChampionName string  `json:"championName,omitempty"`
	Games        int     `json:"games"`
	Wins         int     `json:"wins"`
	Kills        float64 `json:"kills"`
	Deaths       float64 `json:"deaths"`
	Assists      float64 `json:"assists"`
	TotalCS      int     `json:"totalCS,omitempty"`
	Duration     int64   `json:"duration,omitempty"`
	WinRate      int     `json:"winRate"`
	KDA          float64 `json:"kda"`
	CS           float64 `json:"cs"`
	CSPerMinute  float64 `json:"csPerMinute"`
}

type seasonStatsProgress struct {
	Season      string `json:"season"`
	Scanned     int    `json:"scanned"`
	Complete    bool   `json:"complete"`
	Collecting  bool   `json:"collecting,omitempty"`
	Unavailable bool   `json:"unavailable,omitempty"`
	Message     string `json:"message,omitempty"`
}

type seasonStatsCache struct {
	SchemaVersion int                          `json:"schemaVersion"`
	Source        string                       `json:"source"`
	Season        string                       `json:"season"`
	AccountHash   string                       `json:"accountHash"`
	GameIDs       []int64                      `json:"gameIds"`
	Stats         []gameplaySeasonChampionStat `json:"stats"`
	// QueueStats 与 Stats 来自同一次整季遍历，按队列分开聚合（420/440 与
	// 海克斯大乱斗的 2300/2400/3270，判据见 seasonStatsQueueAllowed）。
	// 排位接口缺少负场时只能使用对应队列的数据，不能使用多队列合并总量。
	QueueStats map[int64]gameplayAggregate `json:"queueStats,omitempty"`
	// RankedMatches 是扫描途中顺带留下的单场快照（最新在前）。
	RankedMatches []seasonRankedMatch `json:"rankedMatches,omitempty"`
	// AugmentSamples 是本人海克斯大乱斗逐场的海克斯与成装快照，只服务
	// P2-6「选了某个海克斯之后通常出什么」这一个本地静态查询。
	// 上限 seasonAugmentSampleLimit，磁盘兜底见 season_stats_budget.go。
	AugmentSamples []seasonAugmentSample `json:"augmentSamples,omitempty"`
	// ResumeIndex 是下一轮回补该从哪个服务器偏移量继续；
	// PendingIndex 在增量追新期间临时保存它，避免头部扫描把它清成 0。
	ResumeIndex  int       `json:"resumeIndex,omitempty"`
	PendingIndex int       `json:"pendingIndex,omitempty"`
	UpdatedAt    time.Time `json:"updatedAt"`
	Complete     bool      `json:"complete"`
}

func seasonStatsFileKey(source, accountHash, season string) string {
	// A directory is used instead of a literal colon so the scoped key remains
	// valid on Windows while still keeping source as the first key dimension.
	return filepath.Join("season-stats", strings.ToLower(strings.TrimSpace(source)), accountHash+"-"+strings.ToLower(season)+".json")
}

func (s *localStore) loadSeasonStats(source, accountHash, season string) (seasonStatsCache, error) {
	if s == nil || strings.TrimSpace(accountHash) == "" {
		return seasonStatsCache{}, errors.New("season stats storage unavailable")
	}
	data, err := readLocalStoreFile(s, seasonStatsFileKey(source, accountHash, season))
	if err != nil {
		return seasonStatsCache{}, err
	}
	var cache seasonStatsCache
	if err := json.Unmarshal(data, &cache); err != nil || cache.SchemaVersion != seasonStatsCacheSchemaVersion || cache.Source != source || cache.AccountHash != accountHash || cache.Season != season {
		return seasonStatsCache{}, errors.New("invalid season stats cache")
	}
	return cache, nil
}

// saveSeasonStats 写入前先过一遍磁盘预算（season_stats_budget.go）。
// 预算的处置顺序是「先截断 RankedMatches，再截断 AugmentSamples」，
// 绝不整体拒绝写入、绝不丢弃 Stats、绝不 panic——赛季英雄统计是用户
// 唯一拿不回来的东西（重扫要几十页请求），宁可少留几场单场快照。
func (s *localStore) saveSeasonStats(cache seasonStatsCache) error {
	_, err := s.saveSeasonStatsReported(cache)
	return err
}

// saveSeasonStatsReported 与 saveSeasonStats 同一条写入路径，额外把预算处置
// 结果交回调用方记诊断事件（排障时能看出这一轮到底截断了什么）。
func (s *localStore) saveSeasonStatsReported(cache seasonStatsCache) (seasonStatsBudgetReport, error) {
	if s == nil || strings.TrimSpace(cache.AccountHash) == "" || strings.TrimSpace(cache.Season) == "" {
		return seasonStatsBudgetReport{}, errors.New("season stats storage unavailable")
	}
	data, report := marshalSeasonStatsWithinBudget(cache)
	if report.MarshalErr != nil {
		return report, report.MarshalErr
	}
	return report, writeLocalStoreFile(s, seasonStatsFileKey(cache.Source, cache.AccountHash, cache.Season), data)
}

func seasonStatsAccumulate(stats map[int64]*gameplaySeasonChampionStat, queueStats map[int64]gameplayAggregate, info *riotMatchInfo, playerRef string, seasonStart int64) {
	// 队列判据必须与 seasonRecordRankedMatch 完全一致，两处都走
	// seasonStatsQueueAllowed（420/440 + 海克斯大乱斗 2300/2400/3270）。
	if info == nil || !seasonStatsQueueAllowed(info.QueueID) {
		return
	}
	created := normalizeEpochMillis(info.GameCreation)
	if created <= 0 || created < seasonStart {
		return
	}
	if riotMatchInfoIsRemake(info) {
		return
	}
	for _, participant := range info.Participants {
		if participant.PUUID != playerRef || participant.ChampionID <= 0 {
			continue
		}
		item := stats[participant.ChampionID]
		if item == nil {
			item = &gameplaySeasonChampionStat{ChampionID: participant.ChampionID}
			stats[participant.ChampionID] = item
		}
		item.Games++
		if participant.Win {
			item.Wins++
		}
		item.Kills += float64(participant.Kills)
		item.Deaths += float64(participant.Deaths)
		item.Assists += float64(participant.Assists)
		item.TotalCS += participant.TotalMinionsKilled + participant.NeutralMinionsKilled
		item.Duration += riotMatchDurationSeconds(info)
		if queueStats != nil {
			queue := queueStats[info.QueueID]
			queue.QueueID = info.QueueID
			queue.Games++
			if participant.Win {
				queue.Wins++
			}
			queueStats[info.QueueID] = queue
		}
	}
}

// seasonRecordRankedMatch 在赛季扫描途中顺手记下一场对局的关键指标。
// 判据与 seasonStatsAccumulate 保持一致——两处都走 seasonStatsQueueAllowed
// （单双排 420 / 灵活组排 440 / 海克斯大乱斗 2300、2400、3270，且都排除
// 重开局），否则「近 20 场」和赛季英雄统计会对不上账。R116-E 放开海斗时
// 这条同步约束对新增的三个队列同样适用，所以判据收敛成了一个函数。
//
// 海斗场次额外记一条 seasonAugmentSample（海克斯 + 成装），供 P2-6 的
// 「选了某个海克斯之后通常出什么」静态查询使用；420/440 没有海克斯，不记。
func seasonRecordRankedMatch(cache *seasonStatsCache, info *riotMatchInfo, playerRef string) {
	if info == nil || !seasonStatsQueueAllowed(info.QueueID) || riotMatchInfoIsRemake(info) {
		return
	}
	created := normalizeEpochMillis(info.GameCreation)
	if created <= 0 {
		return
	}
	for _, participant := range info.Participants {
		if participant.PUUID != playerRef {
			continue
		}
		teamKills := 0
		for _, mate := range info.Participants {
			if mate.TeamID == participant.TeamID {
				teamKills += mate.Kills
			}
		}
		position := strings.ToLower(strings.TrimSpace(participant.TeamPosition))
		if position == "" {
			position = strings.ToLower(strings.TrimSpace(participant.IndividualPosition))
		}
		cache.RankedMatches = append(cache.RankedMatches, seasonRankedMatch{
			GameID: info.GameID, CreatedAt: created, QueueID: info.QueueID,
			ChampionID: participant.ChampionID, Win: participant.Win,
			Kills: participant.Kills, Deaths: participant.Deaths, Assists: participant.Assists,
			TeamKills: teamKills, Position: position,
			Ability: seasonRankedAbilitySampleFor(info, participant, position),
		})
		if sample := seasonAugmentSampleFor(info, playerRef); sample != nil {
			cache.AugmentSamples = append(cache.AugmentSamples, *sample)
		}
		return
	}
}

func seasonRankedAbilitySideFor(info *riotMatchInfo, participant riotParticipant) seasonRankedAbilitySide {
	teamKills, teamDamage := 0, 0
	for _, mate := range info.Participants {
		if mate.TeamID != participant.TeamID {
			continue
		}
		teamKills += mate.Kills
		teamDamage += mate.TotalDamageDealtToChampions
	}
	return seasonRankedAbilitySide{
		Kills: participant.Kills, Deaths: participant.Deaths, Assists: participant.Assists,
		Damage: participant.TotalDamageDealtToChampions,
		CS:     participant.TotalMinionsKilled + participant.NeutralMinionsKilled,
		Gold:   participant.GoldEarned, Vision: participant.VisionScore,
		TeamKills: teamKills, TeamDamage: teamDamage,
	}
}

// seasonRankedAbilitySampleFor 记录本人与同位置对手的能力雷达原料。
// 缺少位置或时长时返回 nil——没有对位就没有基线，宁可这一场不参与统计。
func seasonRankedAbilitySampleFor(info *riotMatchInfo, participant riotParticipant, position string) *seasonRankedAbilitySample {
	duration := riotMatchDurationSeconds(info)
	if !abilityPositionKnown(position) || duration <= 0 {
		return nil
	}
	sample := &seasonRankedAbilitySample{Duration: duration, Player: seasonRankedAbilitySideFor(info, participant)}
	for _, candidate := range info.Participants {
		if candidate.TeamID == participant.TeamID {
			continue
		}
		candidatePosition := strings.ToLower(strings.TrimSpace(candidate.TeamPosition))
		if candidatePosition == "" {
			candidatePosition = strings.ToLower(strings.TrimSpace(candidate.IndividualPosition))
		}
		if candidatePosition != position {
			continue
		}
		if sample.Opponent != nil {
			return nil
		}
		opponent := seasonRankedAbilitySideFor(info, candidate)
		sample.Opponent = &opponent
	}
	return sample
}

type seasonRankedQueueSnapshot struct {
	Count  int
	Capped bool
	Oldest int64
}

// seasonTrimRankedMatches 去重后按队列分别裁到上限，再合并为时间倒序。
// 扫描是分批的，同一场可能在增量追新与后台回补里各记一次，必须去重。
func seasonTrimRankedMatches(cache *seasonStatsCache) map[int64]seasonRankedQueueSnapshot {
	if len(cache.RankedMatches) == 0 {
		return nil
	}
	unique := make(map[int64]seasonRankedMatch, len(cache.RankedMatches))
	for _, item := range cache.RankedMatches {
		if item.GameID > 0 {
			unique[item.GameID] = item
		}
	}
	groups := make(map[int64][]seasonRankedMatch)
	for _, item := range unique {
		groups[item.QueueID] = append(groups[item.QueueID], item)
	}
	snapshots := make(map[int64]seasonRankedQueueSnapshot, len(groups))
	merged := make([]seasonRankedMatch, 0, len(unique))
	for queueID, items := range groups {
		sort.SliceStable(items, func(i, j int) bool { return items[i].CreatedAt > items[j].CreatedAt })
		snapshot := seasonRankedQueueSnapshot{Capped: len(items) > seasonRankedMatchLimit}
		if snapshot.Capped {
			items = items[:seasonRankedMatchLimit]
		}
		snapshot.Count = len(items)
		if len(items) > 0 {
			snapshot.Oldest = items[len(items)-1].CreatedAt
		}
		snapshots[queueID] = snapshot
		merged = append(merged, items...)
	}
	sort.SliceStable(merged, func(i, j int) bool { return merged[i].CreatedAt > merged[j].CreatedAt })
	cache.RankedMatches = merged
	return snapshots
}

func seasonStatsFinalizeQueues(stats map[int64]gameplayAggregate) map[int64]gameplayAggregate {
	for queueID, item := range stats {
		item.Losses = item.Games - item.Wins
		if item.Games > 0 {
			item.WinRate = int(float64(item.Wins)*100/float64(item.Games) + 0.5)
		}
		stats[queueID] = item
	}
	return stats
}

func seasonStatsFinalize(stats map[int64]*gameplaySeasonChampionStat, names map[int64]string) []gameplaySeasonChampionStat {
	result := make([]gameplaySeasonChampionStat, 0, len(stats))
	for id, item := range stats {
		item.ChampionName = championName(names, id)
		if item.Games > 0 {
			item.WinRate = int(float64(item.Wins)*100/float64(item.Games) + 0.5)
			item.Kills = round1(item.Kills / float64(item.Games))
			item.Deaths = round1(item.Deaths / float64(item.Games))
			item.Assists = round1(item.Assists / float64(item.Games))
			item.KDA = round2(ratioFloat(item.Kills+item.Assists, item.Deaths))
			item.CS = round1(float64(item.TotalCS) / float64(item.Games))
			item.CSPerMinute = round1(perMinute(item.TotalCS, item.Duration))
		}
		result = append(result, *item)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Games != result[j].Games {
			return result[i].Games > result[j].Games
		}
		return result[i].WinRate > result[j].WinRate
	})
	return result

}
func ratioFloat(numerator, denominator float64) float64 {
	if denominator <= 0 {
		return numerator
	}
	return numerator / denominator
}

func seasonStatsOverall(items []gameplaySeasonChampionStat) gameplayAggregate {
	var result gameplayAggregate
	totalKills, totalDeaths, totalAssists, totalCS := 0.0, 0.0, 0.0, 0
	var totalDuration int64
	for _, item := range items {
		result.Games += item.Games
		result.Wins += item.Wins
		totalKills += item.Kills * float64(item.Games)
		totalDeaths += item.Deaths * float64(item.Games)
		totalAssists += item.Assists * float64(item.Games)
		totalCS += item.TotalCS
		totalDuration += item.Duration
	}
	result.Losses = result.Games - result.Wins
	if result.Games > 0 {
		result.WinRate = int(float64(result.Wins)*100/float64(result.Games) + 0.5)
		result.Kills = round1(float64(totalKills) / float64(result.Games))
		result.Deaths = round1(float64(totalDeaths) / float64(result.Games))
		result.Assists = round1(float64(totalAssists) / float64(result.Games))
		result.KDA = round2(ratioFloat(totalKills+totalAssists, totalDeaths))
		result.CS = round1(float64(totalCS) / float64(result.Games))
		result.CSPerMinute = round1(perMinute(totalCS, totalDuration))
	}
	return result
}

func (a *app) loadSeasonChampionStats(ctx context.Context, client *LCUClient, reference gameplayReference, player Summoner, playerRef string, names map[int64]string) ([]gameplaySeasonChampionStat, seasonStatsProgress, []seasonRankedMatch, map[int64]gameplayAggregate) {
	return a.loadSeasonChampionStatsWithHistoryCache(ctx, client, reference, player, playerRef, names, true)
}

func (a *app) loadSeasonChampionStatsWithHistoryCache(ctx context.Context, client *LCUClient, reference gameplayReference, player Summoner, playerRef string, names map[int64]string, useHistoryCache bool) ([]gameplaySeasonChampionStat, seasonStatsProgress, []seasonRankedMatch, map[int64]gameplayAggregate) {
	season, seasonStart := currentRankedSeason(time.Now())
	progress := seasonStatsProgress{Season: season, Collecting: true}
	serverID := strings.ToUpper(strings.TrimSpace(reference.ServerID))
	if serverID == "" || a.sgp == nil || !validPlayerReference(playerRef) {
		// “没查到”和“查过了确实没有”是两件事。只有完整扫描后的空结果
		// 才能断言未发现对局；数据源不可用必须显式告诉前端。
		return nil, seasonStatsProgress{Season: season, Unavailable: true, Message: "当前数据源不提供赛季统计"}, nil, nil
	}
	accountHash := ""
	if a.storage != nil {
		accountHash = a.storage.accountHash(player)
	}
	cache := seasonStatsCache{SchemaVersion: seasonStatsCacheSchemaVersion, Source: seasonStatsSource, Season: season, AccountHash: accountHash}
	if accountHash != "" {
		if loaded, err := a.storage.loadSeasonStats(seasonStatsSource, accountHash, season); err == nil {
			cache = loaded
		}
	}
	stats := make(map[int64]*gameplaySeasonChampionStat)
	for _, item := range cache.Stats {
		copy := item
		stats[item.ChampionID] = &copy
	}
	queueStats := make(map[int64]gameplayAggregate, len(cache.QueueStats))
	for queueID, item := range cache.QueueStats {
		queueStats[queueID] = item
	}
	seen := make(map[int64]bool, len(cache.GameIDs))
	for _, id := range cache.GameIDs {
		seen[id] = true
	}
	scan := &seasonScanState{
		cache: cache, stats: stats, queueStats: queueStats, seen: seen,
		seasonStartMillis: seasonStart.UnixMilli(),
	}
	// 首屏只做「增量头部扫描」：从最新一页往回扫，撞到已缓存的对局就停。
	// 老玩家第一次打开时缓存是空的，整季回补会是几十页 × 每页约 2.8MB
	// （实测单账号一次 51 页 159MB / 24 秒），绝不能挂在总览响应上，
	// 所以回补改成后台任务，进度通过 progress.Complete / Message 透出。
	a.seasonScanPagesWithHistoryCache(ctx, client, serverID, playerRef, scan, seasonScanForegroundPages, useHistoryCache)
	a.finishSeasonScan(scan, names, accountHash)
	progress.Scanned = scan.scanned
	progress.Complete = scan.cache.Complete
	progress.Collecting = !progress.Complete
	if scan.interrupted {
		progress.Message = "赛季统计暂时中断，已保留当前进度"
	}
	if !scan.cache.Complete {
		a.startSeasonBackfill(client, reference, player, playerRef, names, serverID, accountHash, season, seasonStart)
	}
	if progress.Scanned == 0 && len(scan.cache.GameIDs) > 0 {
		progress.Scanned = len(scan.cache.GameIDs)
	}
	if progress.Message == "" {
		if progress.Complete {
			progress.Message = fmt.Sprintf("已统计 %d 场", progress.Scanned)
		} else {
			progress.Message = fmt.Sprintf("已统计 %d 场，正在后台补全本赛季", progress.Scanned)
		}
	}
	return scan.cache.Stats, progress, scan.cache.RankedMatches, scan.cache.QueueStats
}

// loadSeasonChampionStatsSnapshot is deliberately network-free. Overview may
// read an existing persisted snapshot, but it never waits for season history.
func (a *app) loadSeasonChampionStatsSnapshot(reference gameplayReference, player Summoner, playerRef string) ([]gameplaySeasonChampionStat, seasonStatsProgress, []seasonRankedMatch, map[int64]gameplayAggregate) {
	season, _ := currentRankedSeason(time.Now())
	serverID := strings.ToUpper(strings.TrimSpace(reference.ServerID))
	if serverID == "" || a.sgp == nil || !validPlayerReference(playerRef) {
		return nil, seasonStatsProgress{Season: season, Unavailable: true, Message: "当前数据源不提供赛季统计"}, nil, nil
	}
	progress := seasonStatsProgress{Season: season, Collecting: true, Message: "正在后台读取本赛季对局"}
	if a.storage == nil {
		return nil, progress, nil, nil
	}
	accountHash := a.storage.accountHash(player)
	if accountHash == "" {
		return nil, progress, nil, nil
	}
	cache, err := a.storage.loadSeasonStats(seasonStatsSource, accountHash, season)
	if err != nil {
		return nil, progress, nil, nil
	}
	progress.Scanned = len(cache.GameIDs)
	progress.Complete = cache.Complete
	progress.Collecting = !cache.Complete
	if cache.Complete {
		progress.Message = fmt.Sprintf("已统计 %d 场", progress.Scanned)
	} else {
		progress.Message = fmt.Sprintf("已统计 %d 场，正在后台补全本赛季", progress.Scanned)
	}
	return cache.Stats, progress, cache.RankedMatches, cache.QueueStats
}

const (
	seasonQueryDedupTTL     = 10 * time.Second
	seasonQuerySnapshotsMax = 512
)

func seasonQuerySnapshotKey(serverID, playerRef, season string) string {
	return sourceScopedKey(seasonStatsSource, strings.Join([]string{
		// 队列范围是缓存键的一部分：口径变了就必须换键，否则同一进程里
		// 放开前后的两轮扫描会互相命中去重快照。
		strings.ToUpper(strings.TrimSpace(serverID)), strings.TrimSpace(playerRef), strings.TrimSpace(season), "queues=420,440,2300,2400,3270",
	}, "|"))
}

// startSeasonStatsRefresh runs the old foreground scan outside the overview
// request. Query snapshot, persisted cache and seasonBackfills provide the
// three deduplication gates required for repeated identical overview loads.
func (a *app) startSeasonStatsRefresh(client *LCUClient, reference gameplayReference, player Summoner, playerRef string, names map[int64]string) {
	season, _ := currentRankedSeason(time.Now())
	serverID := strings.ToUpper(strings.TrimSpace(reference.ServerID))
	if client == nil || serverID == "" || a.sgp == nil || a.storage == nil || !validPlayerReference(playerRef) {
		return
	}
	accountHash := a.storage.accountHash(player)
	if accountHash != "" {
		if cached, err := a.storage.loadSeasonStats(seasonStatsSource, accountHash, season); err == nil && cached.Complete && time.Since(cached.UpdatedAt) < seasonQueryDedupTTL {
			return
		}
	}
	key := seasonQuerySnapshotKey(serverID, playerRef, season)
	now := time.Now()
	a.seasonBackfillMu.Lock()
	if previous := a.seasonQuerySnapshotLocked(key, now); !previous.IsZero() && now.Sub(previous) < seasonQueryDedupTTL {
		a.seasonBackfillMu.Unlock()
		return
	}
	a.cacheSeasonQuerySnapshotLocked(key, now)
	flightKey := "overview:" + key
	if a.seasonBackfills == nil {
		a.seasonBackfills = make(map[string]struct{})
	}
	if _, running := a.seasonBackfills[flightKey]; running {
		a.seasonBackfillMu.Unlock()
		return
	}
	a.seasonBackfills[flightKey] = struct{}{}
	a.seasonBackfillMu.Unlock()

	go func() {
		defer func() {
			a.seasonBackfillMu.Lock()
			delete(a.seasonBackfills, flightKey)
			a.seasonBackfillMu.Unlock()
		}()
		ctx, cancel := context.WithTimeout(context.Background(), seasonBackfillTimeout)
		defer cancel()
		_, progress, _, _ := a.loadSeasonChampionStatsWithHistoryCache(ctx, client, reference, player, playerRef, names, true)
		publicRef := a.registerGameplayReferenceDetails(mergeGameplayReferences(reference, gameplayReference{PlayerRef: playerRef}))
		progressEvent, _ := json.Marshal(map[string]any{
			"type": "season-progress", "season": season, "scanned": progress.Scanned,
			"complete": progress.Complete, "account": publicRef,
		})
		a.clearOverviewQuerySnapshots()
		a.broadcastEvent(string(progressEvent))
	}()
}

func (a *app) seasonQuerySnapshotLocked(key string, now time.Time) time.Time {
	previous := a.seasonQuerySnapshots[key]
	if previous.IsZero() {
		return time.Time{}
	}
	if now.Sub(previous) >= seasonQueryDedupTTL {
		a.removeSeasonQuerySnapshotLocked(key)
		return time.Time{}
	}
	if element := a.seasonQuerySnapshotEntries[key]; element != nil {
		a.seasonQuerySnapshotOrder.MoveToFront(element)
	}
	return previous
}

func (a *app) cacheSeasonQuerySnapshotLocked(key string, at time.Time) {
	if a.seasonQuerySnapshots == nil {
		a.seasonQuerySnapshots = make(map[string]time.Time)
	}
	if a.seasonQuerySnapshotOrder == nil {
		a.seasonQuerySnapshotOrder = list.New()
	}
	if a.seasonQuerySnapshotEntries == nil {
		a.seasonQuerySnapshotEntries = make(map[string]*list.Element)
	}
	a.seasonQuerySnapshots[key] = at
	if element := a.seasonQuerySnapshotEntries[key]; element != nil {
		a.seasonQuerySnapshotOrder.MoveToFront(element)
	} else {
		a.seasonQuerySnapshotEntries[key] = a.seasonQuerySnapshotOrder.PushFront(key)
	}
	for len(a.seasonQuerySnapshots) > seasonQuerySnapshotsMax {
		back := a.seasonQuerySnapshotOrder.Back()
		if back == nil {
			break
		}
		a.removeSeasonQuerySnapshotLocked(back.Value.(string))
	}
}

func (a *app) removeSeasonQuerySnapshotLocked(key string) {
	delete(a.seasonQuerySnapshots, key)
	if element := a.seasonQuerySnapshotEntries[key]; element != nil {
		a.seasonQuerySnapshotOrder.Remove(element)
		delete(a.seasonQuerySnapshotEntries, key)
	}
}

// seasonScanState 把一次扫描的可变状态收在一起，让前台增量扫描与后台回补
// 复用同一段翻页逻辑。
type seasonScanState struct {
	cache             seasonStatsCache
	stats             map[int64]*gameplaySeasonChampionStat
	queueStats        map[int64]gameplayAggregate
	seen              map[int64]bool
	seasonStartMillis int64
	scanned           int
	interrupted       bool
}

// seasonScanPages 从 cache.ResumeIndex 开始往回翻页，最多翻 budget 页。
// 返回时 cache.ResumeIndex 指向下次该继续的服务器偏移量。
//
// ★这里有个曾经写错过的判据：老逻辑「撞到已缓存对局就把 Complete 置真」，
// 那是给"缓存已完整、只需增量追新"场景写的。一旦扫描是分批的（前台一批、
// 后台若干批），上一批留下的缓存会让下一批在第一页就撞上并误判为完整，
// 整季的尾巴永远补不上。所以只有真正扫到赛季起点之前、或上游没有更多数据时
// 才算完整；撞到缓存只是说明"头部这一段已经有了"，要跳到 ResumeIndex 继续。
func (a *app) seasonScanPages(ctx context.Context, client *LCUClient, serverID, playerRef string, scan *seasonScanState, budget int) {
	a.seasonScanPagesWithHistoryCache(ctx, client, serverID, playerRef, scan, budget, true)
}

func (a *app) seasonScanPagesWithHistoryCache(ctx context.Context, client *LCUClient, serverID, playerRef string, scan *seasonScanState, budget int, useHistoryCache bool) {
	start := scan.cache.ResumeIndex
	headScan := start == 0
	for page := 0; page < budget; page++ {
		infos, consumed, more, err := a.sgp.matchHistoryOn(ctx, client, serverID, playerRef, start, sgpPageSize, useHistoryCache)
		if err != nil {
			scan.interrupted = true
			return
		}
		if consumed <= 0 {
			scan.cache.Complete = true
			scan.cache.ResumeIndex = 0
			return
		}
		cachedHit := false
		reachedSeasonStart := false
		for _, info := range infos {
			if info == nil || info.GameID <= 0 {
				continue
			}
			created := normalizeEpochMillis(info.GameCreation)
			if created > 0 && created < scan.seasonStartMillis {
				reachedSeasonStart = true
				break
			}
			if scan.seen[info.GameID] {
				cachedHit = true
				continue
			}
			scan.seen[info.GameID] = true
			scan.cache.GameIDs = append(scan.cache.GameIDs, info.GameID)
			scan.scanned++
			seasonStatsAccumulate(scan.stats, scan.queueStats, info, playerRef, scan.seasonStartMillis)
			seasonRecordRankedMatch(&scan.cache, info, playerRef)
		}
		start += consumed
		scan.cache.ResumeIndex = start
		if reachedSeasonStart || !more {
			scan.cache.Complete = true
			scan.cache.ResumeIndex = 0
			return
		}
		// 增量追新（从 0 开始扫）撞到缓存说明新对局已经全部补齐，可以停；
		// 但这不代表整季完整，Complete 交给缓存里原本的标记，别在这里置真。
		if headScan && cachedHit {
			scan.cache.ResumeIndex = scan.cache.PendingIndex
			return
		}
	}
	scan.cache.PendingIndex = scan.cache.ResumeIndex
}

func (a *app) finishSeasonScan(scan *seasonScanState, names map[int64]string, accountHash string) {
	scan.cache.Stats = seasonStatsFinalize(scan.stats, names)
	scan.cache.QueueStats = seasonStatsFinalizeQueues(scan.queueStats)
	snapshots := seasonTrimRankedMatches(&scan.cache)
	augmentDropped := seasonTrimAugmentSamples(&scan.cache)
	scan.cache.UpdatedAt = time.Now().UTC()
	// 一次性的目录级健康检查（工单 P1 第 1 条第 3 项）：挂在首次扫描收尾，
	// 每个存储根只跑一次，不做实时监控。
	a.checkSeasonStatsDirectoryHealth()
	var budget seasonStatsBudgetReport
	if accountHash != "" && a.storage != nil {
		report, err := a.storage.saveSeasonStatsReported(scan.cache)
		budget = report
		if err != nil {
			// 写入失败不能静默：赛季缓存是几十页请求换来的，丢了要能从日志看出来。
			a.recordDiagnostic(map[string]any{
				"event": "season_stats_save_failed", "season": scan.cache.Season,
				"reason": safeDiagnosticReason(err), "bytes": report.WrittenBytes,
			})
		}
	}
	solo, flex := snapshots[seasonQueueSoloDuo], snapshots[seasonQueueFlex]
	mayhem := seasonMayhemSnapshot(snapshots)
	a.recordDiagnostic(map[string]any{
		"event": "season_ranked_snapshot", "season": scan.cache.Season,
		"solo": solo.Count, "flex": flex.Count,
		"capped_solo": solo.Capped, "capped_flex": flex.Capped,
		"oldest_solo": solo.Oldest, "oldest_flex": flex.Oldest,
		// 海斗三个队列（2300/2400/3270）合并成一个页签，诊断里也合并报，
		// 但保留分队列明细，方便排查「某一局算到哪个 ID 上了」。
		"mayhem": mayhem.Count, "capped_mayhem": mayhem.Capped, "oldest_mayhem": mayhem.Oldest,
		"augment_samples": len(scan.cache.AugmentSamples), "augment_dropped": augmentDropped,
		"file_bytes": budget.WrittenBytes, "file_budget_bytes": seasonStatsFileBudgetBytes,
		"file_truncated": budget.Truncated, "file_over_budget": budget.OverBudget,
	})
}

// seasonMayhemSnapshot 把海斗三个队列各自的快照合并成页签级的一个数。
func seasonMayhemSnapshot(snapshots map[int64]seasonRankedQueueSnapshot) seasonRankedQueueSnapshot {
	var merged seasonRankedQueueSnapshot
	for _, queueID := range seasonMayhemQueueIDs {
		item := snapshots[queueID]
		merged.Count += item.Count
		merged.Capped = merged.Capped || item.Capped
		if item.Oldest == 0 || (merged.Oldest != 0 && item.Oldest < merged.Oldest) {
			merged.Oldest = item.Oldest
		}
	}
	return merged
}

// startSeasonBackfill 在后台继续把本赛季剩余的对局补进缓存。
// 单飞：同一账号同一赛季只允许一个回补任务在跑，否则用户连点几次刷新
// 会叠出好几条几十 MB 的下载流。任务用自己的 context，不跟着 HTTP 请求
// 被取消——否则用户一切页面回补就永远补不完。
func (a *app) startSeasonBackfill(client *LCUClient, reference gameplayReference, player Summoner, playerRef string, names map[int64]string, serverID, accountHash, season string, seasonStart time.Time) {
	if accountHash == "" || a.storage == nil {
		return
	}
	key := sourceScopedKey(seasonStatsSource, accountHash+"|"+season)
	a.seasonBackfillMu.Lock()
	if a.seasonBackfills == nil {
		a.seasonBackfills = make(map[string]struct{})
	}
	if _, running := a.seasonBackfills[key]; running {
		a.seasonBackfillMu.Unlock()
		return
	}
	a.seasonBackfills[key] = struct{}{}
	a.seasonBackfillMu.Unlock()

	go func() {
		defer func() {
			a.seasonBackfillMu.Lock()
			delete(a.seasonBackfills, key)
			a.seasonBackfillMu.Unlock()
		}()
		ctx, cancel := context.WithTimeout(context.Background(), seasonBackfillTimeout)
		defer cancel()
		cache, err := a.storage.loadSeasonStats(seasonStatsSource, accountHash, season)
		if err != nil || cache.Complete {
			return
		}
		stats := make(map[int64]*gameplaySeasonChampionStat, len(cache.Stats))
		for _, item := range cache.Stats {
			copy := item
			stats[item.ChampionID] = &copy
		}
		queueStats := make(map[int64]gameplayAggregate, len(cache.QueueStats))
		for queueID, item := range cache.QueueStats {
			queueStats[queueID] = item
		}
		seen := make(map[int64]bool, len(cache.GameIDs))
		for _, id := range cache.GameIDs {
			seen[id] = true
		}
		scan := &seasonScanState{cache: cache, stats: stats, queueStats: queueStats, seen: seen, seasonStartMillis: seasonStart.UnixMilli()}
		// R132 P1：一个后台任务连续跑完整季，不再「一轮 12 页就退出、等下一次
		// 总览请求再续」。旧实现依赖前端收到进度事件后重新加载总览来触发下一轮，
		// 前端 10 秒节流、切走页面或页签在加载中都会让回补停在半路，国服排位卡
		// （客户端不给负场，只能等整季统计补胜率）就一直停在「正在统计中」。
		for round := 1; round <= seasonBackfillMaxRounds; round++ {
			before := scan.scanned
			a.seasonScanPagesWithHistoryCache(ctx, client, serverID, playerRef, scan, seasonScanBackgroundPages, false)
			a.finishSeasonScan(scan, names, accountHash)
			a.recordDiagnostic(map[string]any{
				"event": "season_backfill_round", "season": season, "scanned": scan.scanned - before,
				"round": round, "total_games": len(scan.cache.GameIDs),
				"resume_index": scan.cache.ResumeIndex, "complete": scan.cache.Complete,
				"interrupted": scan.interrupted, "ranked_samples": len(scan.cache.RankedMatches),
			})
			progressEvent, _ := json.Marshal(map[string]any{
				"type": "season-progress", "season": season,
				"scanned": len(scan.cache.GameIDs), "complete": scan.cache.Complete,
				"account": a.registerGameplayReferenceDetails(mergeGameplayReferences(reference, gameplayReference{PlayerRef: playerRef})),
			})
			a.clearOverviewQuerySnapshots()
			a.broadcastEvent(string(progressEvent))
			if scan.cache.Complete || scan.interrupted || scan.scanned == before || ctx.Err() != nil {
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(seasonBackfillRoundPause):
			}
		}
	}()
}

// ---------------------------------------------------------------------------
// P2-6：海克斯 → 出装 静态查询
//
// 这是对「本人历史海斗对局」的本地聚合，不是局内实时联动。局内形态的三个
// 输入（已选海克斯 / 当前阶段 / 系统候选）在仓库现状下全部没有通道，已被
// R116-探测与评审第 3 节证伪（Anti-scope 第 1 条）。
//
// 下面全部是无副作用的纯函数：不读磁盘、不发请求、不看时间，方便直接测试。
// ---------------------------------------------------------------------------

const (
	// seasonAugmentMinimumSample 是海克斯分组的最小样本门槛。
	// 工单建议「复用 hexdataMinimumSample 类似的量级但按个人样本调低，比如
	// 10 场」：hexdataMinimumSample = 250 是全服英雄级口径，个人样本一个赛季
	// 打到 2000 场海斗已经是高频玩家，10 场是「看得出倾向、又不至于把 1 场
	// 当规律」的下限。门槛可由调用方覆盖（HTTP 层暴露成查询参数）。
	seasonAugmentMinimumSample = 10
	// 一次最多返回几个海克斯分组、每个分组最多几套成装。都是展示上限，
	// 不影响聚合口径本身。
	seasonAugmentBuildGroupLimit = 6
	seasonAugmentBuildComboLimit = 3
)

// seasonAugmentBuildCombo 是「选了某个海克斯之后最常出的一套成装」。
// ItemIDs 升序排列（与游戏内的装备栏顺序无关），保证同一套出装不管槽位怎么
// 摆都聚成一行。含饰品栏——样本存的就是最终 7 个槽位，不在聚合时偷偷丢字段。
type seasonAugmentBuildCombo struct {
	ItemIDs   []int64  `json:"itemIds"`
	ItemNames []string `json:"itemNames,omitempty"`
	Games     int      `json:"games"`
	Wins      int      `json:"wins"`
	WinRate   int      `json:"winRate"`
	// Share 是这套出装占「该海克斯全部场次」的百分比。
	Share int `json:"share"`
}

// seasonAugmentBuildGroup 是一个海克斯 ID 的聚合结果。
type seasonAugmentBuildGroup struct {
	AugmentID int64                     `json:"augmentId"`
	Games     int                       `json:"games"`
	Wins      int                       `json:"wins"`
	WinRate   int                       `json:"winRate"`
	Combos    []seasonAugmentBuildCombo `json:"combos"`
}

// seasonAugmentBuildReport 是 P2-6 静态查询的完整结果。
// SampleGames 是「这个英雄的海斗样本总数」，Groups 为空表示没有任何分组
// 达到 minimumSample —— 前端此时整块不显示（工单 P3 判据第 2 条）。
type seasonAugmentBuildReport struct {
	ChampionID    int64                     `json:"championId"`
	SampleGames   int                       `json:"sampleGames"`
	MinimumSample int                       `json:"minimumSample"`
	Groups        []seasonAugmentBuildGroup `json:"groups,omitempty"`
}

// seasonAugmentMinimumSampleOr 把「可配置门槛」收敛成一个函数：
// 非正数一律回落到默认值，避免调用方传 0 导致「1 场也算规律」。
func seasonAugmentMinimumSampleOr(configured int) int {
	if configured > 0 {
		return configured
	}
	return seasonAugmentMinimumSample
}

// seasonAugmentComboKey 把一套出装折叠成稳定的分组键（ID 升序）。
func seasonAugmentComboKey(itemIDs []int64) string {
	sorted := append([]int64(nil), itemIDs...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	parts := make([]string, 0, len(sorted))
	for _, id := range sorted {
		parts = append(parts, fmt.Sprintf("%d", id))
	}
	return strings.Join(parts, ",")
}

// seasonAugmentBuildInsight 是工单判据里的那条查询：给定一个海克斯 ID，
// 从本人历史海斗样本里聚合出「选了它之后最常出的装备组合 + 场次 + 胜率」。
// 样本不足 minimumSample 时返回 nil（调用方整块不显示），绝不返回一个
// 用 1、2 场凑出来的「规律」。
func seasonAugmentBuildInsight(samples []seasonAugmentSample, augmentID int64, minimumSample, comboLimit int) *seasonAugmentBuildGroup {
	if augmentID <= 0 {
		return nil
	}
	minimumSample = seasonAugmentMinimumSampleOr(minimumSample)
	type comboCounter struct {
		itemIDs []int64
		games   int
		wins    int
	}
	combos := make(map[string]*comboCounter)
	group := &seasonAugmentBuildGroup{AugmentID: augmentID}
	for _, sample := range samples {
		if !seasonAugmentSampleHasAugment(sample, augmentID) {
			continue
		}
		group.Games++
		if sample.Win {
			group.Wins++
		}
		key := seasonAugmentComboKey(sample.ItemIDs)
		counter := combos[key]
		if counter == nil {
			sorted := append([]int64(nil), sample.ItemIDs...)
			sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
			counter = &comboCounter{itemIDs: sorted}
			combos[key] = counter
		}
		counter.games++
		if sample.Win {
			counter.wins++
		}
	}
	if group.Games < minimumSample {
		return nil
	}
	group.WinRate = seasonWinRatePercent(group.Wins, group.Games)
	rows := make([]seasonAugmentBuildCombo, 0, len(combos))
	for _, counter := range combos {
		rows = append(rows, seasonAugmentBuildCombo{
			ItemIDs: counter.itemIDs, Games: counter.games, Wins: counter.wins,
			WinRate: seasonWinRatePercent(counter.wins, counter.games),
			Share:   seasonWinRatePercent(counter.games, group.Games),
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Games != rows[j].Games {
			return rows[i].Games > rows[j].Games
		}
		return seasonAugmentComboKey(rows[i].ItemIDs) < seasonAugmentComboKey(rows[j].ItemIDs)
	})
	if comboLimit > 0 && len(rows) > comboLimit {
		rows = rows[:comboLimit]
	}
	group.Combos = rows
	return group
}

// seasonAugmentSampleHasAugment 判断一场样本里是否选了某个海克斯。
func seasonAugmentSampleHasAugment(sample seasonAugmentSample, augmentID int64) bool {
	for _, id := range sample.AugmentIDs {
		if id == augmentID {
			return true
		}
	}
	return false
}

// seasonAugmentBuildReportFor 把查询收成英雄详情页「构筑」tab 能直接用的一份
// 报告：先按英雄过滤（详情页是英雄语境，把别的英雄的出装混进来就是伪造），
// 再按「本人实际用过的海克斯」分组，逐个跑 seasonAugmentBuildInsight。
//
// augmentID > 0 时只返回那一个分组（工单判据里的「给定一个 augment ID」）；
// 为 0 时返回本人在这个英雄上用得最多、且达到门槛的前 groupLimit 个分组。
func seasonAugmentBuildReportFor(samples []seasonAugmentSample, championID, augmentID int64, minimumSample, groupLimit, comboLimit int) seasonAugmentBuildReport {
	minimumSample = seasonAugmentMinimumSampleOr(minimumSample)
	report := seasonAugmentBuildReport{ChampionID: championID, MinimumSample: minimumSample}
	scoped := make([]seasonAugmentSample, 0, len(samples))
	for _, sample := range samples {
		if championID > 0 && sample.ChampionID != championID {
			continue
		}
		report.SampleGames++
		scoped = append(scoped, sample)
	}
	if augmentID > 0 {
		if group := seasonAugmentBuildInsight(scoped, augmentID, minimumSample, comboLimit); group != nil {
			report.Groups = []seasonAugmentBuildGroup{*group}
		}
		return report
	}
	used := make(map[int64]int, 8)
	order := make([]int64, 0, 8)
	for _, sample := range scoped {
		for _, id := range sample.AugmentIDs {
			if id <= 0 {
				continue
			}
			if _, seen := used[id]; !seen {
				order = append(order, id)
			}
			used[id]++
		}
	}
	sort.Slice(order, func(i, j int) bool {
		if used[order[i]] != used[order[j]] {
			return used[order[i]] > used[order[j]]
		}
		return order[i] < order[j]
	})
	for _, id := range order {
		group := seasonAugmentBuildInsight(scoped, id, minimumSample, comboLimit)
		if group == nil {
			continue
		}
		report.Groups = append(report.Groups, *group)
		if groupLimit > 0 && len(report.Groups) >= groupLimit {
			break
		}
	}
	return report
}

// seasonWinRatePercent 与仓库既有的胜率取整口径一致（四舍五入到整数百分比）。
func seasonWinRatePercent(wins, games int) int {
	if games <= 0 {
		return 0
	}
	return int(float64(wins)*100/float64(games) + 0.5)
}
