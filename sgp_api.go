package main

// sgp_api.go 通过腾讯国服的 SGP（Service Gateway Proxy）官方网关读取
// 完整对局数据。新版国服客户端的 /lol-match-history/v1/games/{gameId}
// 只返回本人一名参与者，导致概览、队伍分析、构建与“最近一起玩”无数据；
// 参考 LeagueAkari（MIT）的跨区实现改用 SGP：
//
//  1. 从本机客户端读取 entitlements 令牌（GET /entitlements/v1/token）；
//  2. 携带 Bearer 令牌请求所在子服务器的 match-history-query 接口，
//     一次返回整页对局的十人完整数据（Match-V5 风格 JSON）。
//
// 请求只发往固定的腾讯官方域名（*-sgp.lol.qq.com），只携带客户端签发的
// 令牌与被查询的 PUUID，不经过任何第三方服务器。

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// tencentSGPServers 映射国服子服务器（rso_platform_id）到 SGP 网关地址，
// 地址表来自 LeagueAkari 内置配置。
var tencentSGPServers = map[string]string{
	"HN1":    "https://hn1-k8s-sgp.lol.qq.com:21019",
	"HN10":   "https://hn10-k8s-sgp.lol.qq.com:21019",
	"TJ100":  "https://tj100-sgp.lol.qq.com:21019",
	"TJ101":  "https://tj101-sgp.lol.qq.com:21019",
	"NJ100":  "https://nj100-sgp.lol.qq.com:21019",
	"GZ100":  "https://gz100-sgp.lol.qq.com:21019",
	"CQ100":  "https://cq100-sgp.lol.qq.com:21019",
	"BGP2":   "https://bgp2-k8s-sgp.lol.qq.com:21019",
	"PBE":    "https://pbe-sgp.lol.qq.com:21019",
	"PREPBE": "https://prepbe-sgp.lol.qq.com:21019",
}

// tencentServerNames 是搜索栏里展示的国服子服务器中文名。
var tencentServerNames = map[string]string{
	"HN1":   "艾欧尼亚",
	"HN10":  "黑色玫瑰",
	"NJ100": "联盟一区",
	"GZ100": "联盟二区",
	"CQ100": "联盟三区",
	"TJ100": "联盟四区",
	"TJ101": "联盟五区",
	"BGP2":  "峡谷之巅",
	"PBE":   "体验服",
}

// tencentServerOrder 控制界面里服务器的展示顺序。
var tencentServerOrder = []string{"HN1", "HN10", "NJ100", "GZ100", "CQ100", "TJ100", "TJ101", "BGP2", "PBE"}

var errSGPSummonerNotFound = errors.New("该服务器没有这名玩家的资料")

func normalizeTencentServerID(value string) (string, bool) {
	serverID := strings.ToUpper(strings.TrimSpace(value))
	_, ok := tencentServerNames[serverID]
	return serverID, ok
}

func tencentServerName(serverID string) string {
	return tencentServerNames[strings.ToUpper(strings.TrimSpace(serverID))]
}

const (
	sgpPageSize     = 20
	sgpResponseMax  = 24 << 20
	sgpTokenTTL     = 90 * time.Second
	sgpFailureDelay = 45 * time.Second
	sgpCacheTTL     = 90 * time.Second
	sgpCacheMax     = 64
)

type sgpProvider struct {
	http *http.Client
	// serverBases is copied per provider so tests and future runtime overrides
	// never mutate the package-level verified production table.
	serverBases map[string]string

	mu           sync.Mutex
	token        string
	tokenAt      time.Time
	tokenClient  *LCUClient
	sessionToken string
	sessionAt    time.Time
	sessionOwner *LCUClient
	failUntil    time.Time
	historyCache map[string]sgpHistoryCacheEntry
}

type sgpHistoryCacheEntry struct {
	at       time.Time
	games    []*riotMatchInfo
	consumed int
	more     bool
}

func newSGPProvider() *sgpProvider {
	serverBases := make(map[string]string, len(tencentSGPServers))
	for serverID, base := range tencentSGPServers {
		serverBases[serverID] = base
	}
	return &sgpProvider{
		// SGP 网关是国内直连域名，不走“英雄数据网络”的代理设置。
		http:         &http.Client{Timeout: 20 * time.Second},
		serverBases:  serverBases,
		historyCache: make(map[string]sgpHistoryCacheEntry),
	}
}

func (p *sgpProvider) serverBase(serverID string) (string, bool) {
	if p == nil {
		return "", false
	}
	base, ok := p.serverBases[strings.ToUpper(strings.TrimSpace(serverID))]
	return base, ok && strings.TrimSpace(base) != ""
}

// available 判断当前客户端是否属于可用的国服 SGP 大区；
// 返回子服务器 ID（如 HN1）与对应的网关地址。
func (p *sgpProvider) available(client *LCUClient) (string, string, bool) {
	if p == nil || client == nil {
		return "", "", false
	}
	p.mu.Lock()
	failing := time.Now().Before(p.failUntil)
	p.mu.Unlock()
	if failing {
		return "", "", false
	}
	region, platform := client.platformInfo()
	if !strings.EqualFold(region, "TENCENT") || platform == "" {
		return "", "", false
	}
	base, ok := p.serverBase(platform)
	if !ok {
		return "", "", false
	}
	return strings.ToUpper(platform), base, true
}

func (p *sgpProvider) markFailure() {
	p.mu.Lock()
	p.failUntil = time.Now().Add(sgpFailureDelay)
	p.mu.Unlock()
}

func (p *sgpProvider) entitlementsToken(client *LCUClient, force bool) (string, error) {
	p.mu.Lock()
	if !force && p.token != "" && p.tokenClient == client && time.Since(p.tokenAt) < sgpTokenTTL {
		token := p.token
		p.mu.Unlock()
		return token, nil
	}
	p.mu.Unlock()
	var payload struct {
		AccessToken string `json:"accessToken"`
	}
	if err := client.GetJSON("/entitlements/v1/token", &payload); err != nil {
		return "", fmt.Errorf("客户端未提供 SGP 访问令牌: %w", err)
	}
	if strings.TrimSpace(payload.AccessToken) == "" {
		return "", errors.New("客户端返回的 SGP 访问令牌为空")
	}
	p.mu.Lock()
	p.token = payload.AccessToken
	p.tokenAt = time.Now()
	p.tokenClient = client
	p.mu.Unlock()
	return payload.AccessToken, nil
}

type sgpMatchHistoryPage struct {
	Games []struct {
		JSON *riotMatchInfo `json:"json"`
	} `json:"games"`
}

// leagueSessionToken 读取 league-session 令牌：段位（leagues-ledge）与
// 召唤师（summoner-ledge）接口要求这种令牌，与战绩用的 entitlements 不同。
func (p *sgpProvider) leagueSessionToken(client *LCUClient, force bool) (string, error) {
	p.mu.Lock()
	if !force && p.sessionToken != "" && p.sessionOwner == client && time.Since(p.sessionAt) < sgpTokenTTL {
		token := p.sessionToken
		p.mu.Unlock()
		return token, nil
	}
	p.mu.Unlock()
	var token string
	if err := client.GetJSON("/lol-league-session/v1/league-session-token", &token); err != nil {
		return "", fmt.Errorf("客户端未提供 league-session 令牌: %w", err)
	}
	if strings.TrimSpace(token) == "" {
		return "", errors.New("客户端返回的 league-session 令牌为空")
	}
	p.mu.Lock()
	p.sessionToken = token
	p.sessionAt = time.Now()
	p.sessionOwner = client
	p.mu.Unlock()
	return token, nil
}

// tokenKind 标记 getJSON 请求所需的令牌类型。
type sgpTokenKind int

const (
	sgpTokenEntitlements sgpTokenKind = iota
	sgpTokenLeagueSession
)

func (p *sgpProvider) getJSONWithToken(ctx context.Context, client *LCUClient, kind sgpTokenKind, endpoint string, out any) error {
	for attempt := 0; attempt < 2; attempt++ {
		var token string
		var err error
		if kind == sgpTokenLeagueSession {
			token, err = p.leagueSessionToken(client, attempt > 0)
		} else {
			token, err = p.entitlementsToken(client, attempt > 0)
		}
		if err != nil {
			return err
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return err
		}
		request.Header.Set("Authorization", "Bearer "+token)
		request.Header.Set("Accept", "application/json")
		response, err := p.http.Do(request)
		if err != nil {
			return fmt.Errorf("SGP 网关连接失败: %w", err)
		}
		body, readErr := readLimited(response.Body, sgpResponseMax)
		response.Body.Close()
		switch {
		case response.StatusCode == http.StatusOK:
			if readErr != nil {
				return readErr
			}
			if err := json.Unmarshal(body, out); err != nil {
				return errors.New("SGP 网关返回的数据无法解析")
			}
			return nil
		case response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden:
			continue
		default:
			return fmt.Errorf("SGP 网关返回 HTTP %d", response.StatusCode)
		}
	}
	return errors.New("SGP 访问令牌无效，请确认客户端已登录")
}

func (p *sgpProvider) getJSON(ctx context.Context, client *LCUClient, endpoint string, out any) error {
	return p.getJSONWithToken(ctx, client, sgpTokenEntitlements, endpoint, out)
}

// matchHistory 读取指定玩家的完整战绩（每场包含全部十名参与者）。
// 结果按 startIndex 起始，按需分页拉取，并做短期缓存以支撑对局页轮询。
// 除对局列表外还返回两个分页参数：consumed 是本次在服务器侧实际消费的
// 条目数（包含缺少 json 而被跳过的对局，调用方用它推进下一页偏移量），
// more 表示服务器侧是否可能还有更早的对局。
func (p *sgpProvider) matchHistory(ctx context.Context, client *LCUClient, puuid string, start, count int, useCache bool) ([]*riotMatchInfo, int, bool, error) {
	platform, _, ok := p.available(client)
	if !ok {
		return nil, 0, false, errors.New("SGP 服务器不可用")
	}
	games, consumed, more, err := p.matchHistoryOn(ctx, client, platform, puuid, start, count, useCache)
	if err != nil {
		p.markFailure()
	}
	return games, consumed, more, err
}

// matchHistoryOn 与 matchHistory 相同，但明确指定国服子服务器；
// 跨服查询失败不会触发全局失败静默期。
func (p *sgpProvider) matchHistoryOn(ctx context.Context, client *LCUClient, serverID, puuid string, start, count int, useCache bool) ([]*riotMatchInfo, int, bool, error) {
	serverID = strings.ToUpper(strings.TrimSpace(serverID))
	base, ok := p.serverBase(serverID)
	if !ok {
		return nil, 0, false, fmt.Errorf("未收录的国服子服务器：%s", serverID)
	}
	cacheKey := fmt.Sprintf("%s|%s|%d|%d", serverID, puuid, start, count)
	if useCache {
		p.mu.Lock()
		entry, ok := p.historyCache[cacheKey]
		p.mu.Unlock()
		if ok && time.Since(entry.at) < sgpCacheTTL {
			return entry.games, entry.consumed, entry.more, nil
		}
	}
	games := make([]*riotMatchInfo, 0, count)
	// fetched 记录服务器侧的偏移量（含缺少 json 的条目），避免因个别
	// 对局数据不完整导致同一页被反复请求。
	fetched := 0
	lastPageFull := false
	for len(games) < count && fetched < count+sgpPageSize {
		pageSize := count - len(games)
		if pageSize > sgpPageSize {
			pageSize = sgpPageSize
		}
		query := url.Values{
			"startIndex": {strconv.Itoa(start + fetched)},
			"count":      {strconv.Itoa(pageSize)},
		}
		endpoint := base + "/match-history-query/v1/products/lol/" + url.PathEscape(puuid) + "/SUMMARY?" + query.Encode()
		var page sgpMatchHistoryPage
		if err := p.getJSON(ctx, client, endpoint, &page); err != nil {
			if len(games) > 0 {
				break
			}
			return nil, 0, false, err
		}
		for _, game := range page.Games {
			if game.JSON != nil && game.JSON.GameID > 0 && len(game.JSON.Participants) > 0 {
				games = append(games, game.JSON)
			}
		}
		fetched += len(page.Games)
		lastPageFull = len(page.Games) >= pageSize
		if !lastPageFull {
			break
		}
	}
	more := lastPageFull
	if useCache {
		p.mu.Lock()
		p.historyCache[cacheKey] = sgpHistoryCacheEntry{at: time.Now(), games: games, consumed: fetched, more: more}
		if len(p.historyCache) > sgpCacheMax {
			oldestKey := ""
			oldestAt := time.Now()
			for key, entry := range p.historyCache {
				if entry.at.Before(oldestAt) {
					oldestAt = entry.at
					oldestKey = key
				}
			}
			delete(p.historyCache, oldestKey)
		}
		p.mu.Unlock()
	}
	return games, fetched, more, nil
}

// sgpGameDetails 是 DETAILS 端点的响应：与 Riot Match-V5 timeline 同构，
// 帧内事件包含装备购买 / 出售 / 撤销与技能加点。
type sgpGameDetails struct {
	JSON struct {
		Frames       []timelineFrame `json:"frames"`
		Participants []struct {
			ParticipantID int64  `json:"participantId"`
			PUUID         string `json:"puuid"`
		} `json:"participants"`
	} `json:"json"`
}

// gameDetails 读取单场对局的完整时间线（装备路线与技能加点用）。
func (p *sgpProvider) gameDetails(ctx context.Context, client *LCUClient, gameID int64) ([]timelineFrame, error) {
	platform, _, ok := p.available(client)
	if !ok {
		return nil, errors.New("SGP 服务器不可用")
	}
	return p.gameDetailsOn(ctx, client, platform, gameID)
}

func (p *sgpProvider) gameDetailsOn(ctx context.Context, client *LCUClient, serverID string, gameID int64) ([]timelineFrame, error) {
	base, ok := p.serverBase(serverID)
	if !ok {
		return nil, fmt.Errorf("未收录的国服子服务器：%s", serverID)
	}
	endpoint := base + "/match-history-query/v1/products/lol/" + url.PathEscape(fmt.Sprintf("%s_%d", strings.ToUpper(serverID), gameID)) + "/DETAILS"
	var details sgpGameDetails
	if err := p.getJSON(ctx, client, endpoint, &details); err != nil {
		return nil, err
	}
	if len(details.JSON.Frames) == 0 {
		return nil, errors.New("SGP 未返回该对局的时间线")
	}
	return details.JSON.Frames, nil
}

/* ---------- 段位（leagues-ledge）与召唤师（summoner-ledge） ---------- */

// sgpRankedQueue 是 SGP 段位数据里的单个队列条目：与本机客户端的
// ranked-stats 不同，这里的 wins/losses 都是完整的当季胜负场次
// （新版国服客户端的 ranked-stats 已不返回负场，胜率会错成 100%）。
type sgpRankedQueue struct {
	QueueType                 string `json:"queueType"`
	Tier                      string `json:"tier"`
	Rank                      string `json:"rank"`
	LeaguePoints              int    `json:"leaguePoints"`
	Wins                      int    `json:"wins"`
	Losses                    int    `json:"losses"`
	ProvisionalGamesRemaining int    `json:"provisionalGamesRemaining"`
}

// rankedStats 读取玩家的完整排位数据（当前服务器；该接口无法跨服）。
func (p *sgpProvider) rankedStats(ctx context.Context, client *LCUClient, puuid string) ([]sgpRankedQueue, error) {
	serverID, _, ok := p.available(client)
	if !ok {
		return nil, errors.New("SGP 服务器不可用")
	}
	return p.rankedStatsOn(ctx, client, serverID, puuid)
}

// rankedStatsOn 在当前登录的国服子服务器读取排位。接口路径虽然接受
// serverID，但 league-session 令牌不具备跨服查询能力；调用方必须先拒绝远端服务器。
func (p *sgpProvider) rankedStatsOn(ctx context.Context, client *LCUClient, serverID, puuid string) ([]sgpRankedQueue, error) {
	serverID = strings.ToUpper(strings.TrimSpace(serverID))
	base, ok := p.serverBase(serverID)
	if !ok {
		return nil, fmt.Errorf("未收录的国服子服务器：%s", serverID)
	}
	endpoint := base + "/leagues-ledge/v2/rankedStats/puuid/" + url.PathEscape(puuid)
	var payload struct {
		Queues []sgpRankedQueue `json:"queues"`
	}
	if err := p.getJSONWithToken(ctx, client, sgpTokenLeagueSession, endpoint, &payload); err != nil {
		return nil, err
	}
	return payload.Queues, nil
}

// sgpSummoner 是 summoner-ledge 返回的公开召唤师资料。
type sgpSummoner struct {
	PUUID         string `json:"puuid"`
	Name          string `json:"name"`
	ProfileIconID int64  `json:"profileIconId"`
	Level         int64  `json:"level"`
	Privacy       string `json:"privacy"`
}

// summonerByPUUIDOn 在指定国服子服务器上查询召唤师资料（支持跨服）。
func (p *sgpProvider) summonerByPUUIDOn(ctx context.Context, client *LCUClient, serverID, puuid string) (sgpSummoner, error) {
	serverID = strings.ToUpper(strings.TrimSpace(serverID))
	base, ok := p.serverBase(serverID)
	if !ok {
		return sgpSummoner{}, fmt.Errorf("未收录的国服子服务器：%s", serverID)
	}
	endpoint := base + "/summoner-ledge/v1/regions/" + url.PathEscape(strings.ToLower(serverID)) + "/summoners/puuids"
	body, err := json.Marshal([]string{puuid})
	if err != nil {
		return sgpSummoner{}, err
	}
	for attempt := 0; attempt < 2; attempt++ {
		token, tokenErr := p.leagueSessionToken(client, attempt > 0)
		if tokenErr != nil {
			return sgpSummoner{}, tokenErr
		}
		request, requestErr := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
		if requestErr != nil {
			return sgpSummoner{}, requestErr
		}
		request.Header.Set("Authorization", "Bearer "+token)
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Accept", "application/json")
		response, doErr := p.http.Do(request)
		if doErr != nil {
			return sgpSummoner{}, fmt.Errorf("SGP 网关连接失败: %w", doErr)
		}
		payload, readErr := readLimited(response.Body, sgpResponseMax)
		response.Body.Close()
		switch {
		case response.StatusCode == http.StatusOK:
			if readErr != nil {
				return sgpSummoner{}, readErr
			}
			var summoners []sgpSummoner
			if json.Unmarshal(payload, &summoners) != nil || len(summoners) == 0 {
				return sgpSummoner{}, errSGPSummonerNotFound
			}
			return summoners[0], nil
		case response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden:
			continue
		case response.StatusCode == http.StatusNotFound:
			return sgpSummoner{}, errSGPSummonerNotFound
		default:
			return sgpSummoner{}, fmt.Errorf("SGP 网关返回 HTTP %d", response.StatusCode)
		}
	}
	return sgpSummoner{}, errors.New("SGP 访问令牌无效，请确认客户端已登录")
}
