package main

// rank_insights.go 提供战绩卡的段位类数据：
//
//  1. 段位 <-> 绝对分数换算（黑铁 IV 0 分起，每个小段 100 分、每个大段 400 分，
//     大师及以上为 2800 + 胜点），供“平均段位”与 LP 追踪器共用；
//  2. POST /api/gameplay/match-tiers：当前登录的国服服务器按对局参与者逐人
//     读取本机客户端排位数据，跨服明确不支持；韩服按当前玩家一次读取
//     OP.GG 对局页，再批量匹配整页战绩。两条链路都在首屏返回后异步触发。

import (
	"container/list"
	"context"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

var rankTierBases = map[string]int{
	"IRON": 0, "BRONZE": 400, "SILVER": 800, "GOLD": 1200,
	"PLATINUM": 1600, "EMERALD": 2000, "DIAMOND": 2400,
	"MASTER": 2800, "GRANDMASTER": 2800, "CHALLENGER": 2800,
}

var rankDivisionOffsets = map[string]int{"IV": 0, "III": 100, "II": 200, "I": 300}

// rankTierOrder 用于把平均分数映射回段位名称（大师以上不细分）。
var rankTierOrder = []string{"IRON", "BRONZE", "SILVER", "GOLD", "PLATINUM", "EMERALD", "DIAMOND"}

func rankAbsoluteScore(tier, division string, leaguePoints int) (int, bool) {
	base, ok := rankTierBases[strings.ToUpper(strings.TrimSpace(tier))]
	if !ok {
		return 0, false
	}
	offset := 0
	if base < 2800 {
		offset = rankDivisionOffsets[strings.ToUpper(strings.TrimSpace(division))]
	}
	if leaguePoints < 0 {
		leaguePoints = 0
	}
	return base + offset + leaguePoints, true
}

func rankFromScore(score int) (string, string) {
	if score >= 2800 {
		return "MASTER", ""
	}
	if score < 0 {
		score = 0
	}
	tier := rankTierOrder[min(score/400, len(rankTierOrder)-1)]
	divisionIndex := (score % 400) / 100
	division := [4]string{"IV", "III", "II", "I"}[divisionIndex]
	return tier, division
}

/* ---------- 平均段位端点 ---------- */

const (
	rankScoreCacheTTL         = 10 * time.Minute
	rankScoreNegativeCacheTTL = 60 * time.Second
	rankScoreCacheMax         = 600
	// 斗魂竞技场一场最多 21 名玩家，上限按最大模式放宽。
	matchTiersMaxRefs     = 24
	matchTiersMaxMatches  = 50
	matchTiersOPGGTimeout = 9 * time.Second
	// R127 P1-b.2：并发从 4 提到 8。平均段位改为 SGP 优先后，这条链路实测
	// 60–130ms 一次，扛得住 8 并发；回退到本机客户端时仍受 LCU 自身吞吐限制。
	// 注意这个常量同时被专精符文的对手段位查询复用。
	matchTiersRankConcurrency = 8
)

type rankScoreEntry struct {
	score        int
	known        bool
	source       string
	tier         string
	division     string
	winRate      int
	winRateKnown bool
	at           time.Time
	ranks        []gameplayRank
	milestones   *gameplayRankMilestones
	capability   EndpointCapability
	negative     bool
}

type rankScoreCache struct {
	mu            sync.Mutex
	entries       map[string]*list.Element
	recent        *list.List
	flights       map[string]*rankScoreFlight
	evictionSteps uint64
}

type rankScoreCacheItem struct {
	key   string
	entry rankScoreEntry
}

type rankScoreFlight struct {
	done  chan struct{}
	entry rankScoreEntry
}

func newRankScoreCache() *rankScoreCache {
	return &rankScoreCache{
		entries: make(map[string]*list.Element),
		recent:  list.New(),
		flights: make(map[string]*rankScoreFlight),
	}
}

func (c *rankScoreCache) get(playerRef string) (rankScoreEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	element, ok := c.entries[playerRef]
	if !ok {
		return rankScoreEntry{}, false
	}
	item := element.Value.(rankScoreCacheItem)
	ttl := rankScoreCacheTTL
	if item.entry.negative {
		ttl = rankScoreNegativeCacheTTL
	}
	if time.Since(item.entry.at) > ttl {
		c.removeElement(element)
		return rankScoreEntry{}, false
	}
	c.recent.MoveToFront(element)
	return item.entry, true
}

func (c *rankScoreCache) put(playerRef string, entry rankScoreEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if element, ok := c.entries[playerRef]; ok {
		element.Value = rankScoreCacheItem{key: playerRef, entry: entry}
		c.recent.MoveToFront(element)
		return
	}
	c.entries[playerRef] = c.recent.PushFront(rankScoreCacheItem{key: playerRef, entry: entry})
	if len(c.entries) > rankScoreCacheMax {
		c.removeElement(c.recent.Back())
	}
}

func (c *rankScoreCache) removeElement(element *list.Element) {
	if element == nil {
		return
	}
	item := element.Value.(rankScoreCacheItem)
	delete(c.entries, item.key)
	c.recent.Remove(element)
	c.evictionSteps++
}

func (c *rankScoreCache) beginFlight(key string) (*rankScoreFlight, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if flight, ok := c.flights[key]; ok {
		return flight, false
	}
	flight := &rankScoreFlight{done: make(chan struct{})}
	c.flights[key] = flight
	return flight, true
}

func (c *rankScoreCache) finishFlight(key string, flight *rankScoreFlight, entry rankScoreEntry) {
	c.mu.Lock()
	flight.entry = entry
	delete(c.flights, key)
	close(flight.done)
	c.mu.Unlock()
}

// playerRankScore 读取单名玩家当前的排位绝对分数（单双排优先，其次灵活组排）。
func (a *app) playerRankScore(ctx context.Context, client *LCUClient, playerRef string, isCurrent bool, serverID, privacy string) rankScoreEntry {
	entry, _ := a.playerRankScoreWithCacheStatus(ctx, client, playerRef, isCurrent, serverID, privacy)
	return entry
}

// playerRankScoreWithCacheStatus exposes only whether the lookup avoided a new
// upstream request. It never exposes the cache key or player reference to
// diagnostics.
//
// R127 P1-b.1：可选的 tierOnly 表示「只要段位 + 小段 + 胜点，不需要胜负场」。
// 平均段位走这个模式：有 SGP 时直接 SGP 优先，也不再因为「胜负场未完整验证」
// 多补一次查询。它的缓存键带独立作用域，不会把没有胜负场的结果喂给个人资料页
// 等需要胜负场的调用方。用变参是为了保持既有调用点与护栏测试不变。
func (a *app) playerRankScoreWithCacheStatus(ctx context.Context, client *LCUClient, playerRef string, isCurrent bool, serverID, privacy string, tierOnly ...bool) (rankScoreEntry, bool) {
	tierScope := len(tierOnly) > 0 && tierOnly[0]
	a.rankScoresOnce.Do(func() {
		if a.rankScores == nil {
			a.rankScores = newRankScoreCache()
		}
	})
	cache := a.rankScores
	serverID = strings.ToUpper(strings.TrimSpace(serverID))
	useRiot := serverID == "KR" && a.riot != nil && validPlayerReference(playerRef)
	preferredSource := dataSourceRiot
	cacheScope := ""
	if tierScope {
		cacheScope = rankScoreTierOnlyScope
	}
	if !useRiot {
		decision := resolveRankDataSources(rankDataSourceInput{
			PlayerReferenceValid: validPlayerReference(playerRef), LCUConnected: client != nil,
			RemoteServer: isRemoteTencentServer(client, serverID), SGPAvailable: a.sgp != nil && serverID != "",
		})
		sources := decision.Sources
		if tierScope {
			sources = rankSourcesTierOnly(sources)
		}
		preferredSource = "unknown"
		if len(sources) > 0 {
			preferredSource = sources[0]
		}
	}
	cacheKey := rankScoreCacheKeyScoped(preferredSource, serverID, playerRef, cacheScope)
	if entry, ok := cache.get(cacheKey); ok {
		return entry, true
	}
	flight, leader := cache.beginFlight(cacheKey)
	if !leader {
		select {
		case <-flight.done:
			return flight.entry, true
		case <-ctx.Done():
			return rankScoreEntry{}, false
		}
	}
	// A previous leader may have populated the cache between get() and
	// beginFlight(). Recheck after winning leadership to close that race.
	if cached, ok := cache.get(cacheKey); ok {
		cache.finishFlight(cacheKey, flight, cached)
		return cached, true
	}
	entry := rankScoreEntry{at: time.Now()}
	defer func() { cache.finishFlight(cacheKey, flight, entry) }()
	var ranks []gameplayRank
	var milestones *gameplayRankMilestones
	var capability EndpointCapability
	if useRiot {
		ranks, capability = a.riot.loadRiotRanks(ctx, playerRef)
	} else {
		ranks, milestones, capability = a.loadRanksWithFallback(ctx, client, playerRef, isCurrent, serverID, privacy, tierScope)
	}
	entry.ranks = append([]gameplayRank(nil), ranks...)
	entry.milestones = milestones
	entry.capability = capability
	if capability.State == capabilityAvailable {
		for _, queue := range []string{"RANKED_SOLO_5x5", "RANKED_FLEX_SR"} {
			for _, rank := range ranks {
				if rank.QueueType != queue {
					continue
				}
				if score, ok := rankAbsoluteScore(rank.Tier, rank.Division, rank.LeaguePoints); ok {
					entry.score, entry.known = score, true
					entry.tier = rank.Tier
					entry.division = rank.Division
					if rank.WinRate >= 0 {
						entry.winRate = rank.WinRate
						entry.winRateKnown = true
					}
				}
				break
			}
			if entry.known {
				break
			}
		}
		// 已定级与未定级都缓存。实际来源键继续隔离 LCU/SGP 的原始结果；
		// 首选来源键代表这次稳定的数据源决策入口，使 LCU -> SGP fallback
		// 能在下一次相同决策下命中，而不是重新请求两端。
		entry.source = capabilitySource(capability)
		actualKey := rankScoreCacheKeyScoped(entry.source, serverID, playerRef, cacheScope)
		cache.put(actualKey, entry)
		if actualKey != cacheKey {
			cache.put(cacheKey, entry)
		}
	} else {
		entry.source = preferredSource
		entry.negative = true
		cache.put(cacheKey, entry)
	}
	return entry, false
}

var globalMatchTiersRankSemaphore = make(chan struct{}, matchTiersRankConcurrency)

// rankScoreTierOnlyScope 把「只要段位」的结果与需要胜负场的结果分开存放，
// 避免平均段位的缓存条目被个人资料页当成含胜负场的完整结果复用。
const rankScoreTierOnlyScope = "tier-only"

func rankScoreCacheKey(source, serverID, playerRef string) string {
	return rankScoreCacheKeyScoped(source, serverID, playerRef, "")
}

func rankScoreCacheKeyScoped(source, serverID, playerRef, scope string) string {
	if scope != "" {
		source += "|" + scope
	}
	return sourceScopedKey(source, strings.ToUpper(strings.TrimSpace(serverID))+"|"+strings.TrimSpace(playerRef))
}

// rankSourcesTierOnly 把 SGP 提到最前面。平均段位只要段位/小段/胜点：SGP 的
// leagues-ledge/v2/rankedStats 实测 60–130ms，而本机客户端
// /lol-ranked/v1/ranked-stats 中位 550ms、最长 2.4 秒，LCU 只做后备。
func rankSourcesTierOnly(sources []string) []string {
	reordered := make([]string, 0, len(sources))
	for _, source := range sources {
		if source == dataSourceSGP {
			reordered = append(reordered, source)
		}
	}
	for _, source := range sources {
		if source != dataSourceSGP {
			reordered = append(reordered, source)
		}
	}
	return reordered
}

type matchTierMatchRequest struct {
	GameID    int64 `json:"gameId"`
	CreatedAt int64 `json:"createdAt"`
	Duration  int64 `json:"duration"`
	// R127 P1-c.2：Riot 的真正开局与结束时间（毫秒）。CreatedAt 是 gameCreation
	// （房间创建时间），排位里比真正开局早 BP + 读条 2–4 分钟，只靠它去对 OP.GG
	// 会整页对不上（日志里 23 场 0 命中）。三个基准都参与匹配，取差值最小的。
	StartAt int64 `json:"startAt,omitempty"`
	EndAt   int64 `json:"endAt,omitempty"`
}

type matchTiersRequest struct {
	// PlayerRefs 是原有国服单场契约。
	PlayerRefs []string `json:"playerRefs,omitempty"`
	// 以下字段组成韩服整页批量契约。PlayerRef 必须是本会话已登记的
	// 公开引用；GameName/TagLine 只用于与引用中保存的 Riot ID 互补。
	Region    string                  `json:"region,omitempty"`
	ServerID  string                  `json:"serverId,omitempty"`
	PlayerRef string                  `json:"playerRef,omitempty"`
	GameName  string                  `json:"gameName,omitempty"`
	TagLine   string                  `json:"tagLine,omitempty"`
	Matches   []matchTierMatchRequest `json:"matches,omitempty"`
}

type matchTiersResponse struct {
	Tier     string `json:"tier,omitempty"`
	Division string `json:"division,omitempty"`
	// LP 仅在大师及以上有意义（OP.GG 韩服数据提供），界面附加展示。
	LP        int                        `json:"lp,omitempty"`
	Score     int                        `json:"score,omitempty"`
	Samples   int                        `json:"samples"`
	Players   map[string]matchTierPlayer `json:"players,omitempty"`
	CacheHits int                        `json:"cacheHits"`
	// R127 P1-c.5：来源确实还没收录这场（OP.GG 最新一行明显早于 Riot 最新一场）
	// 时置位。前端据此显示「来源暂未收录」，而不是一条让人以为出 bug 的横线。
	SourceStale bool `json:"sourceStale,omitempty"`
}

type matchTierPlayer struct {
	Score    int    `json:"score"`
	Tier     string `json:"tier,omitempty"`
	Division string `json:"division,omitempty"`
}

func (a *app) handleGameplayMatchTiers(w http.ResponseWriter, r *http.Request) {
	var request matchTiersRequest
	if err := decodeJSONRequest(r, &request, 16<<10); err != nil {
		http.Error(w, "查询参数无效", http.StatusBadRequest)
		return
	}
	region := strings.ToLower(strings.TrimSpace(request.Region))
	if region != "" || len(request.Matches) > 0 {
		if region != riotRegionKR {
			http.Error(w, "仅支持韩服批量段位查询", http.StatusBadRequest)
			return
		}
		a.handleRiotMatchTiers(w, r, request)
		return
	}
	if len(request.PlayerRefs) > matchTiersMaxRefs {
		request.PlayerRefs = request.PlayerRefs[:matchTiersMaxRefs]
	}
	client, current, err := a.gameplayClient()
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	type task struct {
		publicRef string
		playerRef string
		isCurrent bool
		serverID  string
		privacy   string
	}
	requestedServerID := ""
	if strings.TrimSpace(request.ServerID) != "" {
		var ok bool
		requestedServerID, ok = normalizeTencentServerID(request.ServerID)
		if !ok {
			http.Error(w, "国服服务器无效", http.StatusBadRequest)
			return
		}
	}
	tasks := make([]task, 0, len(request.PlayerRefs))
	seen := make(map[string]bool, len(request.PlayerRefs))
	for _, publicRef := range request.PlayerRefs {
		reference, ok := a.resolveGameplayReferenceDetails(publicRef)
		// 韩服玩家来自 Riot 接口，本机客户端查不到，直接跳过。
		if !ok || strings.EqualFold(reference.Region, riotRegionKR) || seen[reference.PlayerRef] {
			continue
		}
		serverID := reference.ServerID
		if serverID == "" {
			serverID = requestedServerID
		}
		if requestedServerID != "" && serverID != "" && requestedServerID != serverID {
			http.Error(w, "玩家引用与所选服务器不一致", http.StatusBadRequest)
			return
		}
		seen[reference.PlayerRef] = true
		tasks = append(tasks, task{publicRef: publicRef, playerRef: reference.PlayerRef, isCurrent: reference.PlayerRef == current.PUUID && !isRemoteTencentServer(client, serverID), serverID: serverID, privacy: reference.Privacy})
	}
	scores := make([]int, 0, len(tasks))
	players := make(map[string]matchTierPlayer, len(tasks))
	cacheHits := 0
	var scoresMu sync.Mutex
	var wait sync.WaitGroup
	for _, item := range tasks {
		wait.Add(1)
		go func(item task) {
			defer wait.Done()
			select {
			case globalMatchTiersRankSemaphore <- struct{}{}:
			case <-r.Context().Done():
				return
			}
			defer func() { <-globalMatchTiersRankSemaphore }()
			entry, cacheHit := a.playerRankScoreWithCacheStatus(r.Context(), client, item.playerRef, item.isCurrent, item.serverID, item.privacy, true)
			scoresMu.Lock()
			if cacheHit {
				cacheHits++
			}
			if entry.known {
				scores = append(scores, entry.score)
				tier, division := rankFromScore(entry.score)
				players[item.publicRef] = matchTierPlayer{Score: entry.score, Tier: tier, Division: division}
			}
			scoresMu.Unlock()
		}(item)
	}
	wait.Wait()
	response := matchTiersResponse{Samples: len(scores), Players: players, CacheHits: cacheHits}
	if len(scores) > 0 {
		total := 0
		for _, score := range scores {
			total += score
		}
		average := total / len(scores)
		response.Score = average
		response.Tier, response.Division = rankFromScore(average)
	}
	respondJSON(w, response)
}

// handleRiotMatchTiers 使用当前韩服玩家自己的 OP.GG 对局列表匹配一页
// Riot 战绩。它不依赖本机客户端，也不会接受渲染层直接提交的稳定 PUUID。
func (a *app) handleRiotMatchTiers(w http.ResponseWriter, r *http.Request, request matchTiersRequest) {
	if len(request.Matches) == 0 || len(request.Matches) > matchTiersMaxMatches {
		http.Error(w, "对局数量无效", http.StatusBadRequest)
		return
	}
	reference, ok := a.resolveGameplayReferenceDetails(request.PlayerRef)
	if !ok || !strings.EqualFold(reference.Region, riotRegionKR) {
		http.Error(w, "玩家引用无效或已过期", http.StatusNotFound)
		return
	}
	gameName := strings.TrimSpace(reference.GameName)
	tagLine := strings.TrimSpace(reference.TagLine)
	if gameName == "" {
		gameName = strings.TrimSpace(request.GameName)
	}
	if tagLine == "" {
		tagLine = strings.TrimSpace(request.TagLine)
	}
	if gameName == "" || tagLine == "" {
		http.Error(w, "韩服玩家的 Riot ID 不完整", http.StatusBadRequest)
		return
	}

	response := make(map[string]*matchTiersResponse, len(request.Matches))
	matches := make([]matchTierMatchRequest, 0, len(request.Matches))
	oldest := int64(0)
	for _, match := range request.Matches {
		if match.GameID <= 0 {
			continue
		}
		key := strconv.FormatInt(match.GameID, 10)
		if _, exists := response[key]; exists {
			continue
		}
		response[key] = nil
		matches = append(matches, match)
		// 覆盖范围按最早的可用时间基准算：CreatedAt 是房间创建时间，通常最早，
		// 缺失时退到开局/结束时间，避免整页请求被判成无效。
		for _, base := range opggTierTimeBases(match) {
			if oldest == 0 || base.value < oldest {
				oldest = base.value
			}
		}
	}
	if len(matches) == 0 || oldest == 0 {
		respondJSON(w, response)
		return
	}
	// R127 P1-c.3：先吃 7 天长期缓存。命中的对局一个 OP.GG 请求都不用发，
	// 重开同一个韩服玩家时平均段位是立即显示的。
	cached, pending, cacheHits := a.splitCachedMatchTiers(matches)
	for key, value := range cached {
		response[key] = value
	}
	if len(pending) == 0 {
		a.recordDiagnostic(map[string]any{
			"event": "opgg_match_tiers_result", "matches": len(matches),
			"matched": cacheHits, "missing": 0, "cache_hits": cacheHits, "opgg_requests": 0,
			"rows": 0, "rows_with_tier": 0, "matched_bases": map[string]int{},
		})
		respondJSON(w, response)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), matchTiersOPGGTimeout)
	defer cancel()
	games, err := a.opggGameTiers(ctx, gameName, tagLine, reference.PlayerRef, oldest)
	if err != nil {
		w.Header().Set("Retry-After", "30")
		http.Error(w, "平均段位来源暂不可用，请稍后重试", http.StatusServiceUnavailable)
		return
	}
	matched := 0
	bases := map[string]int{}
	unmatchedTimeGaps := make([]int64, 0, len(matches))
	unmatchedSpanGaps := make([]int64, 0, len(matches))
	rowsWithTier, newestRow, newestRiot := 0, int64(0), int64(0)
	for index := range games {
		if games[index].tier.Tier != "" {
			rowsWithTier++
		}
		if games[index].createdAt > newestRow {
			newestRow = games[index].createdAt
		}
	}
	for _, match := range pending {
		for _, base := range opggTierTimeBases(match) {
			if base.value > newestRiot {
				newestRiot = base.value
			}
		}
		result := matchOPGGAverageTierDetailed(match, games)
		if result.tier != nil {
			response[strconv.FormatInt(match.GameID, 10)] = result.tier
			// 已结束的对局平均段位不会再变，落盘 7 天，重开时不再取 OP.GG 页。
			a.writeMatchTierCache(match.GameID, result.tier)
			matched++
			bases[result.bestBase]++
			continue
		}
		// R127 P1-c.1：未匹配的对局记下与最近一行差了多少秒（只记秒数，不记任何
		// ID）。否则「23 场一场都没对上」在日志里只是一个 0，根本没法判断是时间
		// 基准错了、还是 OP.GG 那边还没收录这几场。
		timeGap, spanGap := opggNearestRowGap(match, games)
		unmatchedTimeGaps = append(unmatchedTimeGaps, timeGap)
		unmatchedSpanGaps = append(unmatchedSpanGaps, spanGap)
	}
	sourceLag := int64(0)
	if newestRow > 0 && newestRiot > 0 {
		// OP.GG 最新一行比 Riot 最新一场早了多久。明显为正说明 OP.GG 还没收录
		// 最近的对局（它不会自动更新），不是匹配算法的问题。
		sourceLag = (newestRiot - newestRow) / 1000
	}
	if sourceLag > opggSourceStaleSeconds {
		// R127 P1-c.5：来源确实落后时明确告诉前端，界面显示「来源暂未收录」，
		// 而不是一条让人以为出 bug 的横线。
		for _, match := range pending {
			key := strconv.FormatInt(match.GameID, 10)
			if response[key] == nil {
				response[key] = &matchTiersResponse{SourceStale: true}
			}
		}
	}
	event := map[string]any{
		"event": "opgg_match_tiers_result", "matches": len(matches),
		"matched": matched + cacheHits, "missing": len(pending) - matched,
		// R127 P1-c.3：命中长期缓存的场次与本次是否真的发了 OP.GG 请求。
		"cache_hits": cacheHits, "opgg_requests": 1,
		// OP.GG 这一页有多少行、其中多少行真的带平均段位（R112 实测约 7/20）。
		"rows": len(games), "rows_with_tier": rowsWithTier,
		// 命中用的是哪个 Riot 时间基准：真机日志可以直接证实或推翻 end 假设。
		"matched_bases": bases,
	}
	if len(unmatchedTimeGaps) > 0 {
		event["unmatched_time_gap_seconds"] = unmatchedTimeGaps
		event["unmatched_duration_gap_seconds"] = unmatchedSpanGaps
	}
	if sourceLag != 0 {
		event["source_lag_seconds"] = sourceLag
	}
	a.recordDiagnostic(event)
	respondJSON(w, response)
}
