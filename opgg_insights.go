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
	opggHistoryCacheMax  = 32
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
	at    time.Time
	ranks []gameplayHistoricalRank
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

func (a *app) fetchOPGGHistoricalRanks(ctx context.Context, slug string) []gameplayHistoricalRank {
	if a.champions == nil || !a.champions.featureGates.enabled(featureGateOPGG) {
		return nil
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://op.gg/zh-cn/lol/summoners/kr/"+slug, nil)
	if err != nil {
		return nil
	}
	request.Header.Set("Accept", "text/html")
	request.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0 Safari/537.36")
	response, err := a.champions.httpClient().Do(request)
	if err != nil {
		return nil
	}
	data, readErr := readLimited(response.Body, opggGamesResponseMax)
	response.Body.Close()
	if response.StatusCode != http.StatusOK || readErr != nil {
		return nil
	}
	return parseOPGGHistoricalRanks(data)
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
		if cached && time.Since(entry.at) < opggHistoryCacheTTL {
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
	ranks := a.fetchOPGGHistoricalRanks(ctx, slug)
	a.opgg.mu.Lock()
	if len(ranks) > 0 {
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
		a.opgg.histories[cacheKey] = opggHistoryCacheEntry{at: time.Now(), ranks: append([]gameplayHistoricalRank(nil), ranks...)}
	}
	flight := a.opgg.historyFlights[cacheKey]
	delete(a.opgg.historyFlights, cacheKey)
	if flight != nil {
		close(flight.done)
	}
	a.opgg.mu.Unlock()
	if len(ranks) == 0 {
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
	a.opgg.mu.Unlock()
	if !ok {
		return nil
	}
	return append([]gameplayHistoricalRank(nil), entry.ranks...)
}

func (a *app) startOPGGHistoricalRanks(reference gameplayReference, gameName, tagLine, puuid, privacy string) {
	if a.champions == nil || !a.champions.featureGates.enabled(featureGateOPGG) || a.opgg == nil || strings.EqualFold(strings.TrimSpace(privacy), "PRIVATE") || strings.TrimSpace(gameName) == "" || !validPlayerReference(puuid) {
		return
	}
	key := sourceScopedKey(dataSourceOPGG, puuid)
	a.opgg.mu.Lock()
	if entry, ok := a.opgg.histories[key]; ok && time.Since(entry.at) < opggHistoryCacheTTL {
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
func (a *app) opggGameTiers(ctx context.Context, gameName, tagLine, puuid string, oldest int64) (result []opggGameTier, resultErr error) {
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
	for page := 0; entry.err == nil && page < opggGamesMaxPages; page++ {
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

// matchOPGGAverageTier 按时间和时长把一场 Riot 对局映射到 OP.GG 的
// 不透明对局记录。返回副本，避免调用方持有缓存切片内部字段的地址。
func matchOPGGAverageTier(createdAt, duration int64, games []opggGameTier) *matchTiersResponse {
	if createdAt <= 0 || duration <= 0 {
		return nil
	}
	var best *opggGameTier
	bestGap := int64(opggGamesTimeSlack + 1)
	for index := range games {
		candidate := &games[index]
		gap := createdAt - candidate.createdAt
		if gap < 0 {
			gap = -gap
		}
		spanGap := duration - candidate.duration
		if spanGap < 0 {
			spanGap = -spanGap
		}
		if gap <= opggGamesTimeSlack && spanGap <= opggGamesSpanSlack && gap < bestGap {
			best, bestGap = candidate, gap
		}
	}
	if best == nil || best.tier.Tier == "" {
		return nil
	}
	value := best.tier
	return &value
}
