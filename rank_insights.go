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
	// Four concurrent LCU ranked requests keep a 10-player match responsive
	// without flooding the local client during reconnect or champion select.
	matchTiersRankConcurrency = 4
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
func (a *app) playerRankScoreWithCacheStatus(ctx context.Context, client *LCUClient, playerRef string, isCurrent bool, serverID, privacy string) (rankScoreEntry, bool) {
	a.rankScoresOnce.Do(func() {
		if a.rankScores == nil {
			a.rankScores = newRankScoreCache()
		}
	})
	cache := a.rankScores
	serverID = strings.ToUpper(strings.TrimSpace(serverID))
	useRiot := serverID == "KR" && a.riot != nil && validPlayerReference(playerRef)
	preferredSource := dataSourceRiot
	if !useRiot {
		decision := resolveRankDataSources(rankDataSourceInput{
			PlayerReferenceValid: validPlayerReference(playerRef), LCUConnected: client != nil,
			RemoteServer: isRemoteTencentServer(client, serverID), SGPAvailable: a.sgp != nil && serverID != "",
		})
		preferredSource = "unknown"
		if len(decision.Sources) > 0 {
			preferredSource = decision.Sources[0]
		}
	}
	cacheKey := rankScoreCacheKey(preferredSource, serverID, playerRef)
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
		ranks, milestones, capability = a.loadRanksWithFallback(ctx, client, playerRef, isCurrent, serverID, privacy)
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
		actualKey := rankScoreCacheKey(entry.source, serverID, playerRef)
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

func rankScoreCacheKey(source, serverID, playerRef string) string {
	return sourceScopedKey(source, strings.ToUpper(strings.TrimSpace(serverID))+"|"+strings.TrimSpace(playerRef))
}

type matchTierMatchRequest struct {
	GameID    int64 `json:"gameId"`
	CreatedAt int64 `json:"createdAt"`
	Duration  int64 `json:"duration"`
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
			entry, cacheHit := a.playerRankScoreWithCacheStatus(r.Context(), client, item.playerRef, item.isCurrent, item.serverID, item.privacy)
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
		if match.CreatedAt > 0 && (oldest == 0 || match.CreatedAt < oldest) {
			oldest = match.CreatedAt
		}
	}
	if len(matches) == 0 || oldest == 0 {
		respondJSON(w, response)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), matchTiersOPGGTimeout)
	defer cancel()
	games := a.opggGameTiers(ctx, gameName, tagLine, reference.PlayerRef, oldest)
	for _, match := range matches {
		if value := matchOPGGAverageTier(match.CreatedAt, match.Duration, games); value != nil {
			response[strconv.FormatInt(match.GameID, 10)] = value
		}
	}
	respondJSON(w, response)
}
