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
// 任一环节失败都静默降级：界面上该行显示“—”，不影响战绩本身。

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
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
	opggTierCacheMax     = 32
	opggGamesTimeSlack   = 180 * 1000 // 开局时间匹配容差（毫秒）
	opggGamesSpanSlack   = 20         // 时长匹配容差（秒）
	opggGamesResponseMax = 8 << 20
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
}

type opggTierFlight struct {
	done chan struct{}
}

// opggInsights 缓存按玩家抓取的 OP.GG 对局段位数据。
type opggInsights struct {
	mu      sync.Mutex
	tiers   map[string]opggTierCacheEntry
	flights map[string]*opggTierFlight
}

func newOPGGInsights() *opggInsights {
	return &opggInsights{tiers: make(map[string]opggTierCacheEntry), flights: make(map[string]*opggTierFlight)}
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

// opggFetchGamesPage 请求一页对局，返回解析后的段位行与下一页游标。
func (a *app) opggFetchGamesPage(ctx context.Context, slug, puuid, endedAt string) ([]opggGameTier, string) {
	body, err := json.Marshal([]opggGamesRequest{{Locale: "zh-cn", Region: "kr", PUUID: puuid, GameType: "TOTAL", EndedAt: endedAt, Champion: ""}})
	if err != nil {
		return nil, ""
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://op.gg/zh-cn/lol/summoners/kr/"+slug, bytes.NewReader(body))
	if err != nil {
		return nil, ""
	}
	request.Header.Set("Next-Action", opggGamesAction)
	request.Header.Set("Content-Type", "text/plain;charset=UTF-8")
	request.Header.Set("Accept", "text/x-component")
	request.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0 Safari/537.36")
	response, err := a.champions.httpClient().Do(request)
	if err != nil {
		return nil, ""
	}
	data, readErr := readLimited(response.Body, opggGamesResponseMax)
	response.Body.Close()
	if response.StatusCode != http.StatusOK || readErr != nil {
		return nil, ""
	}
	// Server Action 返回 text/x-component 流，每行形如 “<id>:<JSON>”；
	// 对局数组在包含 average_tier 的那一行。
	for _, line := range strings.Split(string(data), "\n") {
		colon := strings.Index(line, ":")
		if colon <= 0 || !strings.Contains(line, `"average_tier"`) {
			continue
		}
		var payload struct {
			Data []opggGameRow `json:"data"`
		}
		if json.Unmarshal([]byte(line[colon+1:]), &payload) != nil || len(payload.Data) == 0 {
			return nil, ""
		}
		games := make([]opggGameTier, 0, len(payload.Data))
		cursor := ""
		for _, row := range payload.Data {
			cursor = row.CreatedAt
			started, parseErr := time.Parse(time.RFC3339, row.CreatedAt)
			tier := strings.ToUpper(strings.TrimSpace(row.AverageTier.Tier))
			if parseErr != nil || tier == "" {
				continue
			}
			games = append(games, opggGameTier{
				createdAt: started.UnixMilli(),
				duration:  row.GameLength,
				tier:      matchTiersResponse{Tier: tier, Division: opggRomanDivision(row.AverageTier.Division), LP: row.AverageTier.LP},
			})
		}
		if len(payload.Data) < opggGamesPageSize {
			cursor = ""
		}
		return games, cursor
	}
	return nil, ""
}

// opggGameTiers 从最新一页开始翻对局，直到覆盖 oldest（毫秒）之前的
// 场次或达到页数上限；结果按玩家短期缓存。
func (a *app) opggGameTiers(ctx context.Context, gameName, tagLine, puuid string, oldest int64) []opggGameTier {
	if a.champions == nil || a.opgg == nil || strings.TrimSpace(gameName) == "" || !validPlayerReference(puuid) {
		return nil
	}
	var stale []opggGameTier
	for {
		a.opgg.mu.Lock()
		entry, cached := a.opgg.tiers[puuid]
		if cached {
			stale = append(stale[:0], entry.games...)
		}
		fresh := cached && time.Since(entry.at) < opggTierCacheTTL
		covered := entry.coveredOldest == 0 || oldest > 0 && oldest >= entry.coveredOldest
		if fresh && covered {
			games := append([]opggGameTier(nil), entry.games...)
			a.opgg.mu.Unlock()
			return games
		}
		if flight := a.opgg.flights[puuid]; flight != nil {
			done := flight.done
			a.opgg.mu.Unlock()
			select {
			case <-done:
				continue
			case <-ctx.Done():
				return stale
			}
		}
		a.opgg.flights[puuid] = &opggTierFlight{done: make(chan struct{})}
		a.opgg.mu.Unlock()
		break
	}

	slug := url.PathEscape(gameName + "-" + tagLine)
	games := make([]opggGameTier, 0, opggGamesPageSize)
	cursor := ""
	for page := 0; page < opggGamesMaxPages && ctx.Err() == nil; page++ {
		pageGames, nextCursor := a.opggFetchGamesPage(ctx, slug, puuid, cursor)
		if len(pageGames) == 0 {
			break
		}
		games = append(games, pageGames...)
		oldestFetched := pageGames[len(pageGames)-1].createdAt
		if nextCursor == "" || (oldest > 0 && oldestFetched < oldest) {
			break
		}
		cursor = nextCursor
	}
	a.opgg.mu.Lock()
	if len(games) > 0 {
		if _, exists := a.opgg.tiers[puuid]; !exists && len(a.opgg.tiers) >= opggTierCacheMax {
			oldestKey := ""
			oldestAt := time.Now()
			for key, entry := range a.opgg.tiers {
				if entry.at.Before(oldestAt) {
					oldestAt = entry.at
					oldestKey = key
				}
			}
			delete(a.opgg.tiers, oldestKey)
		}
		a.opgg.tiers[puuid] = opggTierCacheEntry{at: time.Now(), games: games, coveredOldest: oldest}
	}
	flight := a.opgg.flights[puuid]
	delete(a.opgg.flights, puuid)
	if flight != nil {
		close(flight.done)
	}
	a.opgg.mu.Unlock()
	if len(games) == 0 {
		return stale
	}
	return append([]opggGameTier(nil), games...)
}

// annotateOPGGAverageTiers 给韩服战绩补充平均段位：OP.GG 的对局 id
// 无法对应 Riot gameId，按「开局时间 ±3 分钟 + 时长 ±20 秒」匹配。
func (a *app) annotateOPGGAverageTiers(ctx context.Context, gameName, tagLine, puuid string, matches []gameplayMatch) {
	if len(matches) == 0 {
		return
	}
	boundedCtx, cancel := context.WithTimeout(ctx, 9*time.Second)
	defer cancel()
	oldest := int64(0)
	for _, match := range matches {
		if match.CreatedAt > 0 && (oldest == 0 || match.CreatedAt < oldest) {
			oldest = match.CreatedAt
		}
	}
	games := a.opggGameTiers(boundedCtx, gameName, tagLine, puuid, oldest)
	if len(games) == 0 {
		return
	}
	for index := range matches {
		match := &matches[index]
		match.AverageTier = matchOPGGAverageTier(match.CreatedAt, match.Duration, games)
	}
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
	if best == nil {
		return nil
	}
	value := best.tier
	return &value
}
