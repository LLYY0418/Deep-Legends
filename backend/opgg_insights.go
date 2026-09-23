package main

// opgg_insights.go 用 OP.GG 的对局数据为韩服战绩补充“平均段位”。
// 韩服无法像国服那样通过本机客户端逐人查询排位（Riot 官方接口按人
// 查询会超出配额），而 OP.GG 召唤师页的 games Server Action 一次返回
// 整页对局、每场都带 average_tier，正好与战绩列表一一对应。
//
// 与 opggResolveRiotID 相同，请求发往 op.gg 主域名（部分网络环境下
// lol-web-api.op.gg 不可达），走 championProvider 的 HTTP 通道以继承
// “英雄数据网络”的代理设置。OP.GG 返回的对局 id 是不透明哈希，无法
// 直接对应 Riot 的 gameId，这里按「开局时间 + 对局时长」匹配。
// 暂时失败与来源未提供段位分开处理，避免将失败缓存成无段位。

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// opggGamesAction 是召唤师页“对局列表”Server Action 的动作标识，
// 抓包自 op.gg 召唤师页。随 OP.GG 前端发版可能变化，失效时静默降级。
const opggGamesAction = "409a2b9ca50d15e50a4dace93552e3a40113dc2753"

const (
	opggGamesPageSize    = 20
	opggGamesMaxPages    = 4
	opggTierCacheTTL     = 90 * time.Second
	opggTierFailureTTL   = 30 * time.Second
	opggTierCacheMax     = 32
	opggGamesTimeSlack   = 180 * 1000 // 开局时间匹配容差（毫秒）
	opggGamesSpanSlack   = 20         // 时长匹配容差（秒）
	opggGamesResponseMax = 8 << 20
	opggHistoryCacheTTL  = 30 * time.Minute
	opggHistoryEmptyTTL  = 10 * time.Minute
	opggHistoryCacheMax  = 32
	// R127 P1-c.5：OP.GG 最新一行比 Riot 最新一场早超过这个秒数，就认定来源还没
	// 收录最近的对局（OP.GG 不会自动更新，需要有人点更新）。
	opggSourceStaleSeconds = 30 * 60
)

type opggGameTier struct {
	createdAt int64 // 开局时间（毫秒）
	duration  int64 // 秒
	tier      matchTiersResponse
}

type opggTierCacheEntry struct {
	at            time.Time
	games         []opggGameTier
	coveredOldest int64
	cursor        string
	profileID     string
	exhausted     bool
	err           error
	attemptedAt   time.Time
}

type opggTierFlight struct {
	done chan struct{}
}

type opggHistoryCacheEntry struct {
	at        time.Time
	expiresAt time.Time
	ranks     []gameplayHistoricalRank
	empty     bool
}

func opggHistoryEntryFresh(entry opggHistoryCacheEntry, now time.Time) bool {
	expiresAt := entry.expiresAt
	if expiresAt.IsZero() {
		expiresAt = entry.at.Add(opggHistoryCacheTTL)
	}
	return now.Before(expiresAt)
}

type opggHistoryFlight struct {
	done chan struct{}
}

// opggInsights 缓存按玩家抓取的 OP.GG 对局段位数据。
type opggInsights struct {
	seasons        map[string]opggSeasonEntry
	seasonFlights  map[string]*opggSeasonFlight
	mu             sync.Mutex
	tiers          map[string]opggTierCacheEntry
	flights        map[string]*opggTierFlight
	histories      map[string]opggHistoryCacheEntry
	historyFlights map[string]*opggHistoryFlight
	historyStarts  map[string]struct{}
}

func newOPGGInsights() *opggInsights {
	return &opggInsights{
		tiers: make(map[string]opggTierCacheEntry), flights: make(map[string]*opggTierFlight),
		histories: make(map[string]opggHistoryCacheEntry), historyFlights: make(map[string]*opggHistoryFlight), historyStarts: make(map[string]struct{}),
	}
}

var (
	opggSeasonPattern = regexp.MustCompile(`"season":"(S(20[0-9]{2})(?: S[1-3])?)\s*","rank_entries":`)
	opggRankPattern   = regexp.MustCompile(`"rank_info":\{"tier":"([^"]*)","value":"([^"]*)","division":(?:(\d+)|"\$undefined"|null),"lp":?(?:"([^"]*)"|(\d+)|null)`)
)

func parseOPGGHistoricalRanks(data []byte) []gameplayHistoricalRank {
	decoded := strings.ReplaceAll(string(data), `\"`, `"`)
	matches := opggSeasonPattern.FindAllStringSubmatchIndex(decoded, -1)
	result := make([]gameplayHistoricalRank, 0, len(matches))
	seen := make(map[string]bool)
	for index, match := range matches {
		season := strings.TrimSpace(decoded[match[2]:match[3]])
		year, _ := strconv.Atoi(decoded[match[4]:match[5]])
		if year < 2023 || seen[season] {
			continue
		}
		end := len(decoded)
		if index+1 < len(matches) {
			end = matches[index+1][0]
		} else if end > match[1]+2400 {
			end = match[1] + 2400
		}
		rankMatch := opggRankPattern.FindStringSubmatch(decoded[match[1]:end])
		if len(rankMatch) == 0 {
			continue
		}
		tier := strings.ToUpper(strings.TrimSpace(rankMatch[2]))
		if tier == "" || strings.EqualFold(strings.TrimSpace(rankMatch[2]), "Unranked") {
			continue
		}
		division := ""
		if rawDivision, err := strconv.Atoi(rankMatch[3]); err == nil && rawDivision >= 1 && rawDivision <= 4 {
			division = opggRomanDivision(rawDivision)
		}
		lpText := strings.ReplaceAll(strings.TrimSpace(rankMatch[4]+rankMatch[5]), ",", "")
		var leaguePoints *int
		if lp, err := strconv.Atoi(lpText); err == nil {
			leaguePoints = &lp
		}
		result = append(result, gameplayHistoricalRank{
			Season: season, QueueType: "RANKED_SOLO_5x5", Tier: tier,
			Division: division, LeaguePoints: leaguePoints,
		})
		seen[season] = true
	}
	return result
}

func (a *app) fetchOPGGHistoricalRanks(ctx context.Context, slug string) ([]gameplayHistoricalRank, error) {
	if a.champions == nil || !a.champions.featureGates.enabled(featureGateOPGG) {
		return nil, errors.New("opgg historical ranks disabled")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://op.gg/zh-cn/lol/summoners/kr/"+slug, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "text/html")
	request.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0 Safari/537.36")
	response, err := a.champions.httpClient().Do(request)
	if err != nil {
		return nil, err
	}
	data, readErr := readLimited(response.Body, opggGamesResponseMax)
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, errors.New("opgg historical ranks HTTP error")
	}
	if readErr != nil {
		return nil, readErr
	}
	return parseOPGGHistoricalRanks(data), nil
}

func (a *app) opggHistoricalRanks(ctx context.Context, gameName, tagLine, puuid, privacy string) []gameplayHistoricalRank {
	if a.champions == nil || a.opgg == nil || strings.EqualFold(strings.TrimSpace(privacy), "PRIVATE") || strings.TrimSpace(gameName) == "" || !validPlayerReference(puuid) {
		return nil
	}
	var stale []gameplayHistoricalRank
	cacheKey := sourceScopedKey(dataSourceOPGG, puuid)
	for {
		a.opgg.mu.Lock()
		entry, cached := a.opgg.histories[cacheKey]
		if cached {
			stale = append(stale[:0], entry.ranks...)
		}
		if cached && opggHistoryEntryFresh(entry, time.Now()) {
			ranks := append([]gameplayHistoricalRank(nil), entry.ranks...)
			a.opgg.mu.Unlock()
			return ranks
		}
		if flight := a.opgg.historyFlights[cacheKey]; flight != nil {
			done := flight.done
			a.opgg.mu.Unlock()
			select {
			case <-done:
				continue
			case <-ctx.Done():
				return stale
			}
		}
		a.opgg.historyFlights[cacheKey] = &opggHistoryFlight{done: make(chan struct{})}
		a.opgg.mu.Unlock()
		break
	}

	slug := url.PathEscape(gameName + "-" + tagLine)
	ranks, fetchErr := a.fetchOPGGHistoricalRanks(ctx, slug)
	a.opgg.mu.Lock()
	if fetchErr == nil {
		if _, exists := a.opgg.histories[cacheKey]; !exists && len(a.opgg.histories) >= opggHistoryCacheMax {
			oldestKey := ""
			oldestAt := time.Now()
			for key, entry := range a.opgg.histories {
				if entry.at.Before(oldestAt) {
					oldestAt, oldestKey = entry.at, key
				}
			}
			delete(a.opgg.histories, oldestKey)
		}
		now := time.Now()
		ttl := opggHistoryCacheTTL
		if len(ranks) == 0 {
			ttl = opggHistoryEmptyTTL
		}
		a.opgg.histories[cacheKey] = opggHistoryCacheEntry{at: now, expiresAt: now.Add(ttl), ranks: append([]gameplayHistoricalRank(nil), ranks...), empty: len(ranks) == 0}
	}
	flight := a.opgg.historyFlights[cacheKey]
	delete(a.opgg.historyFlights, cacheKey)
	if flight != nil {
		close(flight.done)
	}
	a.opgg.mu.Unlock()
	if fetchErr != nil {
		return stale
	}
	return ranks
}

func (a *app) cachedOPGGHistoricalRanks(puuid string) []gameplayHistoricalRank {
	if a.champions == nil || !a.champions.featureGates.enabled(featureGateOPGG) || a.opgg == nil || !validPlayerReference(puuid) {
		return nil
	}
	key := sourceScopedKey(dataSourceOPGG, puuid)
	a.opgg.mu.Lock()
	entry, ok := a.opgg.histories[key]
	if ok && !opggHistoryEntryFresh(entry, time.Now()) {
		ok = false
	}
	var ranks []gameplayHistoricalRank
	if ok && !entry.empty {
		ranks = append([]gameplayHistoricalRank(nil), entry.ranks...)
	}
	a.opgg.mu.Unlock()
	return ranks
}

func (a *app) startOPGGHistoricalRanks(reference gameplayReference, gameName, tagLine, puuid, privacy string) {
	if a.champions == nil || !a.champions.featureGates.enabled(featureGateOPGG) || a.opgg == nil || strings.EqualFold(strings.TrimSpace(privacy), "PRIVATE") || strings.TrimSpace(gameName) == "" || !validPlayerReference(puuid) {
		return
	}
	key := sourceScopedKey(dataSourceOPGG, puuid)
	a.opgg.mu.Lock()
	if entry, ok := a.opgg.histories[key]; ok && opggHistoryEntryFresh(entry, time.Now()) {
		a.opgg.mu.Unlock()
		return
	}
	if _, running := a.opgg.historyStarts[key]; running || a.opgg.historyFlights[key] != nil {
		a.opgg.mu.Unlock()
		return
	}
	a.opgg.historyStarts[key] = struct{}{}
	a.opgg.mu.Unlock()
	go func() {
		defer func() {
			a.opgg.mu.Lock()
			delete(a.opgg.historyStarts, key)
			a.opgg.mu.Unlock()
		}()
		ctx, cancel := context.WithTimeout(context.Background(), 7*time.Second)
		defer cancel()
		started := time.Now()
		ranks := a.opggHistoricalRanks(ctx, gameName, tagLine, puuid, privacy)
		a.recordDiagnostic(map[string]any{"event": "opgg_historical_cost", "duration_ms": time.Since(started).Milliseconds(), "background": true, "count": len(ranks)})
		if len(ranks) == 0 {
			return
		}
		payload, _ := json.Marshal(map[string]any{
			"type": "historical-ranks", "account": a.registerGameplayReferenceDetails(mergeGameplayReferences(reference, gameplayReference{PlayerRef: puuid})),
			"count": len(ranks), "historicalRanks": ranks,
		})
		a.clearOverviewQuerySnapshots()
		a.broadcastEvent(string(payload))
	}()
}

type opggGamesRequest struct {
	Locale   string `json:"locale"`
	Region   string `json:"region"`
	PUUID    string `json:"puuid"`
	GameType string `json:"gameType"`
	EndedAt  string `json:"endedAt"`
	Champion string `json:"champion"`
}

type opggGameRow struct {
	CreatedAt   string `json:"created_at"`
	GameLength  int64  `json:"game_length"`
	AverageTier struct {
		Tier     string `json:"tier"`
		Division int    `json:"division"`
		LP       int    `json:"lp"`
	} `json:"average_tier"`
}

func opggRomanDivision(division int) string {
	switch division {
	case 1:
		return "I"
	case 2:
		return "II"
	case 3:
		return "III"
	case 4:
		return "IV"
	}
	return ""
}

// Parse only the action result and the three fields needed for matching. A page
// may mix average_tier objects with "$undefined" or Flight references; an absent
// tier on one game must not invalidate all the other games in the page.
func parseOPGGGamesPage(data []byte) ([]opggGameTier, string, error) {
	p := &opggPlayerPage{rows: map[string]any{}}
	for _, line := range bytes.Split(data, []byte("\n")) {
		id, raw, ok := bytes.Cut(line, []byte(":"))
		var value any
		if ok && json.Unmarshal(raw, &value) == nil {
			p.rows[string(id)] = value
		}
	}
	root, _ := p.rows["0"].(map[string]any)
	action, _ := root["a"].(string)
	if !strings.HasPrefix(action, "$@") {
		return nil, "", errors.New("opgg-games-action")
	}
	payload, _ := p.rows[strings.TrimPrefix(action, "$@")].(map[string]any)
	rows, ok := payload["data"].([]any)
	if !ok || len(rows) > 100 {
		return nil, "", errors.New("opgg-games-data")
	}
	games := make([]opggGameTier, 0, len(rows))
	cursor := ""
	for _, value := range rows {
		node, ok := value.(map[string]any)
		if !ok {
			return nil, "", errors.New("opgg-games-row")
		}
		budget := 200
		selected, err := p.resolve(map[string]any{"created_at": node["created_at"], "game_length": node["game_length"], "average_tier": node["average_tier"]}, 0, &budget)
		if err != nil {
			return nil, "", err
		}
		encoded, _ := json.Marshal(selected)
		var row opggGameRow
		if json.Unmarshal(encoded, &row) != nil {
			return nil, "", errors.New("opgg-games-tier")
		}
		started, err := time.Parse(time.RFC3339, row.CreatedAt)
		if err != nil || row.GameLength <= 0 {
			return nil, "", errors.New("opgg-games-time")
		}
		cursor = row.CreatedAt
		tier := strings.ToUpper(strings.TrimSpace(row.AverageTier.Tier))
		division := opggRomanDivision(row.AverageTier.Division)
		if _, valid := rankTierBases[tier]; !valid {
			tier, division = "", ""
		}
		if tier == "MASTER" || tier == "GRANDMASTER" || tier == "CHALLENGER" {
			division = ""
		}
		games = append(games, opggGameTier{createdAt: started.UnixMilli(), duration: row.GameLength,
			tier: matchTiersResponse{Tier: tier, Division: division, LP: row.AverageTier.LP}})
	}
	if len(rows) < opggGamesPageSize {
		cursor = ""
	}
	return games, cursor, nil
}

func (a *app) opggFetchGamesPage(ctx context.Context, ref gameplayReference, profileID, endedAt string) ([]opggGameTier, string, error) {
	body, _ := json.Marshal([]opggGamesRequest{{Locale: "zh-cn", Region: "kr", PUUID: profileID, GameType: "TOTAL", EndedAt: endedAt, Champion: ""}})
	data, err := a.readOPGGPlayerPage(ctx, ref, http.MethodPost, opggGamesAction, body)
	if err != nil {
		return nil, "", err
	}
	return parseOPGGGamesPage(data)
}

// Resolve the provider-specific ID once per fresh cache, then reuse the page
// cursor for older visible matches. Unknown tiers still count toward coverage.
func (a *app) opggGameTiers(ctx context.Context, gameName, tagLine, puuid string, oldest int64, maxPages ...int) (result []opggGameTier, resultErr error) {
	// R127 P1-c.4：预热只取第一页。oldest=0 时下面的覆盖判断和提前停止条件都不
	// 成立，不限页数就会一路拉满 4 页，还会把前台请求一起拖住（它等同一个 flight）。
	pageLimit := opggGamesMaxPages
	if len(maxPages) > 0 && maxPages[0] > 0 && maxPages[0] < pageLimit {
		pageLimit = maxPages[0]
	}
	if a.champions == nil || a.opgg == nil || !a.champions.featureGates.enabled(featureGateOPGG) || strings.TrimSpace(gameName) == "" || !validPlayerReference(puuid) {
		return nil, errors.New("opgg-games-unavailable")
	}
	started := time.Now()
	stage, pages, cacheHit := "cache", 0, false
	defer func() {
		a.recordDiagnostic(map[string]any{"event": "opgg_match_tiers_cost", "duration_ms": time.Since(started).Milliseconds(), "stage": stage, "pages": pages, "cache_hit": cacheHit, "rows": len(result), "success": resultErr == nil, "failure": supplementFailureCode(resultErr)})
	}()
	// Bind cached identities to both the trusted Riot ID and our internal ID.
	key := sourceScopedKey(dataSourceOPGG, puuid+":"+strings.ToLower(gameName+"#"+tagLine))
	var entry opggTierCacheEntry
	for {
		a.opgg.mu.Lock()
		entry = a.opgg.tiers[key]
		fresh := !entry.at.IsZero() && time.Since(entry.at) < opggTierCacheTTL
		covered := entry.exhausted || oldest > 0 && entry.coveredOldest > 0 && oldest >= entry.coveredOldest
		if fresh && covered {
			a.opgg.mu.Unlock()
			cacheHit = true
			return append([]opggGameTier(nil), entry.games...), nil
		}
		if entry.err != nil && time.Since(entry.attemptedAt) < opggTierFailureTTL {
			a.opgg.mu.Unlock()
			cacheHit = true
			return append([]opggGameTier(nil), entry.games...), entry.err
		}
		if flight := a.opgg.flights[key]; flight != nil {
			done := flight.done
			a.opgg.mu.Unlock()
			select {
			case <-done:
				continue
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		a.opgg.flights[key] = &opggTierFlight{done: make(chan struct{})}
		a.opgg.mu.Unlock()
		if !fresh {
			entry = opggTierCacheEntry{}
		}
		break
	}
	ref := gameplayReference{Region: riotRegionKR, GameName: gameName, TagLine: tagLine}
	entry.err = nil
	if entry.profileID == "" {
		stage = "page-identity"
		data, err := a.readOPGGPlayerPage(ctx, ref, http.MethodGet, "", nil)
		entry.err = err
		if err == nil {
			profile, parseErr := parseOPGGPlayerPage(data, ref)
			entry.err = parseErr
			if parseErr == nil {
				entry.profileID = profile.puuid
			}
		}
	}
	for page := 0; entry.err == nil && page < pageLimit; page++ {
		stage = "games-page"
		games, cursor, err := a.opggFetchGamesPage(ctx, ref, entry.profileID, entry.cursor)
		pages++
		if err != nil {
			entry.err = err
			break
		}
		if cursor != "" && cursor == entry.cursor {
			entry.err = errors.New("opgg-games-cursor")
			break
		}
		entry.games = append(entry.games, games...)
		entry.cursor, entry.exhausted = cursor, cursor == ""
		if len(games) > 0 {
			entry.coveredOldest = games[len(games)-1].createdAt
		}
		if entry.at.IsZero() {
			entry.at = time.Now()
		}
		if entry.exhausted || oldest > 0 && entry.coveredOldest <= oldest {
			break
		}
	}
	entry.attemptedAt = time.Now()
	// Restart after a large window instead of retaining an unbounded history
	// or falsely marking discarded newer rows as covered.
	if len(entry.games) > 1000 {
		entry.at = time.Time{}
	}
	a.opgg.mu.Lock()
	if _, exists := a.opgg.tiers[key]; !exists && len(a.opgg.tiers) >= opggTierCacheMax {
		oldestKey := ""
		oldestAt := time.Now()
		for k, v := range a.opgg.tiers {
			if v.attemptedAt.Before(oldestAt) {
				oldestAt, oldestKey = v.attemptedAt, k
			}
		}
		delete(a.opgg.tiers, oldestKey)
	}
	a.opgg.tiers[key] = entry
	flight := a.opgg.flights[key]
	delete(a.opgg.flights, key)
	close(flight.done)
	a.opgg.mu.Unlock()
	if entry.err == nil {
		stage = "complete"
	}
	return append([]opggGameTier(nil), entry.games...), entry.err
}

// R127 P1-c.2：OP.GG 的 created_at 到底是什么时间基准，之前从没用真实数据核对过，
// 现有测试两边都是手写的同一个基准，所以全绿也发现不了 23 场一场都对不上。
//
// 用仓库里的真实抓包 backend/testdata/r112-opgg-average-tiers.txt（20 行，含
// created_at 与 game_length）做相邻对局重叠检验：
//   - 当 created_at 是「开局时间」：出现 3 处不可能的重叠（-21s、-105s、-574s）；
//   - 当 created_at 是「结束时间」：0 重叠，相邻间隔 158s–63341s 全部合理。
//
// 所以 end 是最可能的基准。但这里仍然把 Riot 的三个时间（结束 / 开局 / 房间创建）
// 都比较一遍、取差值最小的那个，不把结论钉死在单一假设上；命中所用基准会写进
// opgg_match_tiers_result 诊断，下一份真机日志可以直接证实或推翻。
const (
	opggTierBaseEnd      = "end"
	opggTierBaseStart    = "start"
	opggTierBaseCreation = "creation"
)

type opggTierTimeBase struct {
	name  string
	value int64
}

type opggTierMatch struct {
	tier     *matchTiersResponse
	bestGap  int64  // 命中行与所用基准的时间差（毫秒）；-1 表示一行都没对上
	bestBase string // 命中所用的 Riot 时间基准
}

func opggTierTimeBases(request matchTierMatchRequest) []opggTierTimeBase {
	bases := make([]opggTierTimeBase, 0, 3)
	for _, candidate := range []opggTierTimeBase{
		{opggTierBaseEnd, request.EndAt},
		{opggTierBaseStart, request.StartAt},
		{opggTierBaseCreation, request.CreatedAt},
	} {
		if candidate.value > 0 {
			bases = append(bases, candidate)
		}
	}
	return bases
}

// matchOPGGAverageTier 保留旧的两参数形状（只有 gameCreation 可用时的兼容入口）。
// 生产路径走 matchOPGGAverageTierDetailed。
func matchOPGGAverageTier(createdAt, duration int64, games []opggGameTier) *matchTiersResponse {
	return matchOPGGAverageTierDetailed(matchTierMatchRequest{CreatedAt: createdAt, Duration: duration}, games).tier
}

// matchOPGGAverageTierDetailed 按时间和时长把一场 Riot 对局映射到 OP.GG 的不透明
// 对局记录。返回副本，避免调用方持有缓存切片内部字段的地址。
func matchOPGGAverageTierDetailed(request matchTierMatchRequest, games []opggGameTier) opggTierMatch {
	result := opggTierMatch{bestGap: -1}
	if request.Duration <= 0 {
		return result
	}
	bases := opggTierTimeBases(request)
	if len(bases) == 0 {
		return result
	}
	var best *opggGameTier
	bestGap := int64(opggGamesTimeSlack + 1)
	bestBase := ""
	for index := range games {
		candidate := &games[index]
		spanGap := request.Duration - candidate.duration
		if spanGap < 0 {
			spanGap = -spanGap
		}
		if spanGap > opggGamesSpanSlack {
			continue
		}
		for _, base := range bases {
			gap := base.value - candidate.createdAt
			if gap < 0 {
				gap = -gap
			}
			if gap <= opggGamesTimeSlack && gap < bestGap {
				best, bestGap, bestBase = candidate, gap, base.name
			}
		}
	}
	if best == nil {
		return result
	}
	result.bestGap, result.bestBase = bestGap, bestBase
	if best.tier.Tier == "" {
		// 行对上了但 OP.GG 没给平均段位：不算命中，差值仍留给诊断。
		return result
	}
	value := best.tier
	result.tier = &value
	return result
}

// opggNearestRowGap 返回不受容差限制的最小时间差与时长差（秒），只用于诊断
// 「最近的一行到底差了多少」，不参与匹配判定。
func opggNearestRowGap(request matchTierMatchRequest, games []opggGameTier) (int64, int64) {
	bestTime, bestSpan := int64(-1), int64(-1)
	bases := opggTierTimeBases(request)
	if len(bases) == 0 || len(games) == 0 {
		return bestTime, bestSpan
	}
	for index := range games {
		candidate := &games[index]
		gap := int64(-1)
		for _, base := range bases {
			diff := base.value - candidate.createdAt
			if diff < 0 {
				diff = -diff
			}
			if gap < 0 || diff < gap {
				gap = diff
			}
		}
		// 时间差与时长差必须来自同一行。分别取最小值会拼出一个根本不存在的组合
		// （最近的那行时长差 100 秒、另一行时长差 0），诊断就失真了。
		if bestTime < 0 || gap < bestTime {
			spanGap := request.Duration - candidate.duration
			if spanGap < 0 {
				spanGap = -spanGap
			}
			bestTime, bestSpan = gap/1000, spanGap
		}
	}
	return bestTime, bestSpan
}

// startOPGGGameTiers 在打开韩服玩家总览时就并行预热 OP.GG 的对局列表（R127
// P1-c.4）。Riot ID 一开始就有，不必等战绩卡片渲染完、用户滚动到可见才发第一次
// 请求——那一次要 1.5–3.6 秒，正好就是「总览出来了、平均段位还在转」的那段。
//
// oldest 传 0：只预热第一页。需要更老的对局时前台请求会按现有逻辑继续翻页，
// opggGameTiers 自身的 flight 合并保证不会重复取页。
func (a *app) startOPGGGameTiers(gameName, tagLine, puuid string) {
	if a.champions == nil || a.opgg == nil || !a.champions.featureGates.enabled(featureGateOPGG) ||
		strings.TrimSpace(gameName) == "" || !validPlayerReference(puuid) {
		return
	}
	key := sourceScopedKey(dataSourceOPGG, puuid+":"+strings.ToLower(gameName+"#"+tagLine))
	a.opgg.mu.Lock()
	entry := a.opgg.tiers[key]
	fresh := !entry.at.IsZero() && time.Since(entry.at) < opggTierCacheTTL
	running := a.opgg.flights[key] != nil
	a.opgg.mu.Unlock()
	if fresh || running {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), matchTiersOPGGTimeout)
		defer cancel()
		_, _ = a.opggGameTiers(ctx, gameName, tagLine, puuid, 0, 1)
	}()
}
