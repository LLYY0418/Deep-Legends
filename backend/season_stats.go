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

// 7：持久化键与载荷都记录来源。不同来源的对局结构和统计口径不能共用缓存。
const seasonStatsCacheSchemaVersion = 7

const seasonStatsSource = dataSourceSGP

const (
	// 前台只允许翻这么多页：每页 20 场、实测约 2.8MB / 450ms，2 页足够把
	// 上次之后的新对局补齐，又不会让总览首屏等待超过 1 秒。
	seasonScanForegroundPages = 2
	// 后台单轮回补的页数上限。给得比前台大得多（约 30MB / 10 秒），
	// 但仍然有界，避免一次把整季几百场全压在一个请求里。
	seasonScanBackgroundPages = 12
	// 排位样本保留条数：近 20 场排位只要 20 条，多留一些做筛选余量。
	seasonRankedMatchLimit = 40
	// 单轮后台回补的总时限，防止上游变慢时任务无限期挂着。
	seasonBackfillTimeout = 90 * time.Second
)

// seasonRankedMatch 是从赛季扫描里顺手留下的排位单场快照。
// 赛季扫描本来就要把每一场的 JSON 解出来，这里多记几个字段是零额外网络开销的；
// 有了它，「近 20 场排位」就不再受限于首屏那 20 条战绩里恰好有几场排位。
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
	Kills        int     `json:"kills"`
	Deaths       int     `json:"deaths"`
	Assists      int     `json:"assists"`
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
	// QueueStats 与 Stats 来自同一次整季遍历，只把 420/440 分开聚合。
	// 排位接口缺少负场时只能使用对应队列的数据，不能使用两队列合并总量。
	QueueStats map[int64]gameplayAggregate `json:"queueStats,omitempty"`
	// RankedMatches 是扫描途中顺带留下的排位单场快照（最新在前）。
	RankedMatches []seasonRankedMatch `json:"rankedMatches,omitempty"`
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

func (s *localStore) saveSeasonStats(cache seasonStatsCache) error {
	if s == nil || strings.TrimSpace(cache.AccountHash) == "" || strings.TrimSpace(cache.Season) == "" {
		return errors.New("season stats storage unavailable")
	}
	data, err := json.Marshal(cache)
	if err != nil {
		return err
	}
	return writeLocalStoreFile(s, seasonStatsFileKey(cache.Source, cache.AccountHash, cache.Season), data)
}

func seasonStatsAccumulate(stats map[int64]*gameplaySeasonChampionStat, queueStats map[int64]gameplayAggregate, info *riotMatchInfo, playerRef string, seasonStart int64) {
	if info == nil || (info.QueueID != 420 && info.QueueID != 440) {
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
		item.Kills += participant.Kills
		item.Deaths += participant.Deaths
		item.Assists += participant.Assists
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

// seasonRecordRankedMatch 在赛季扫描途中顺手记下一场排位的关键指标。
// 判据与 seasonStatsAccumulate 保持一致（只认单双排 420 / 灵活组排 440，
// 且排除重开局），否则「近 20 场排位」和赛季英雄统计会对不上账。
func seasonRecordRankedMatch(cache *seasonStatsCache, info *riotMatchInfo, playerRef string) {
	if info == nil || (info.QueueID != 420 && info.QueueID != 440) || riotMatchInfoIsRemake(info) {
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
			item.KDA = round2(ratio(item.Kills+item.Assists, item.Deaths))
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

func seasonStatsOverall(items []gameplaySeasonChampionStat) gameplayAggregate {
	var result gameplayAggregate
	totalKills, totalDeaths, totalAssists, totalCS := 0, 0, 0, 0
	var totalDuration int64
	for _, item := range items {
		result.Games += item.Games
		result.Wins += item.Wins
		totalKills += item.Kills
		totalDeaths += item.Deaths
		totalAssists += item.Assists
		totalCS += item.TotalCS
		totalDuration += item.Duration
	}
	result.Losses = result.Games - result.Wins
	if result.Games > 0 {
		result.WinRate = int(float64(result.Wins)*100/float64(result.Games) + 0.5)
		result.Kills = round1(float64(totalKills) / float64(result.Games))
		result.Deaths = round1(float64(totalDeaths) / float64(result.Games))
		result.Assists = round1(float64(totalAssists) / float64(result.Games))
		result.KDA = round2(ratio(totalKills+totalAssists, totalDeaths))
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
		strings.ToUpper(strings.TrimSpace(serverID)), strings.TrimSpace(playerRef), strings.TrimSpace(season), "queues=420,440",
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
	scan.cache.UpdatedAt = time.Now().UTC()
	if accountHash != "" && a.storage != nil {
		_ = a.storage.saveSeasonStats(scan.cache)
	}
	solo, flex := snapshots[420], snapshots[440]
	a.recordDiagnostic(map[string]any{
		"event": "season_ranked_snapshot", "season": scan.cache.Season,
		"solo": solo.Count, "flex": flex.Count,
		"capped_solo": solo.Capped, "capped_flex": flex.Capped,
		"oldest_solo": solo.Oldest, "oldest_flex": flex.Oldest,
	})
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
		a.seasonScanPagesWithHistoryCache(ctx, client, serverID, playerRef, scan, seasonScanBackgroundPages, false)
		a.finishSeasonScan(scan, names, accountHash)
		a.recordDiagnostic(map[string]any{
			"event": "season_backfill_round", "season": season, "scanned": scan.scanned,
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
	}()
}
