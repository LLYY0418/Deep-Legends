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
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
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

type overviewLoadCostContextKey struct{}

type overviewLoadCost struct {
	mu               sync.Mutex
	requests         int
	bytes            int
	historyCalls     int
	historyCacheHits int
}

func (c *overviewLoadCost) addRequest(bytes int) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.requests++
	c.bytes += bytes
	c.mu.Unlock()
}

func (c *overviewLoadCost) addHistoryCall() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.historyCalls++
	c.mu.Unlock()
}

func (c *overviewLoadCost) addHistoryCacheHit() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.historyCacheHits++
	c.mu.Unlock()
}

func (c *overviewLoadCost) snapshot() (requests, bytes, historyCalls, historyCacheHits int) {
	if c == nil {
		return 0, 0, 0, 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.requests, c.bytes, c.historyCalls, c.historyCacheHits
}

func overviewCostFromContext(ctx context.Context) *overviewLoadCost {
	if ctx == nil {
		return nil
	}
	cost, _ := ctx.Value(overviewLoadCostContextKey{}).(*overviewLoadCost)
	return cost
}

type sgpProvider struct {
	http            *http.Client
	observe         func(map[string]any)
	rankedShapeOnce sync.Once
	// serverBases is copied per provider so tests and future runtime overrides
	// never mutate the package-level verified production table.
	serverBases map[string]string

	mu            sync.Mutex
	token         string
	tokenAt       time.Time
	tokenClient   *LCUClient
	sessionToken  string
	sessionAt     time.Time
	sessionOwner  *LCUClient
	failUntil     time.Time
	historyCache  map[string]sgpHistoryCacheEntry
	summonerCache map[string]sgpSummonerCacheEntry
}

type sgpHistoryCacheEntry struct {
	at       time.Time
	lastUsed time.Time
	games    []*riotMatchInfo
	consumed int
	more     bool
}

type sgpSummonerCacheEntry struct {
	at       time.Time
	summoner sgpSummoner
}

func newSGPProvider() *sgpProvider {
	serverBases := make(map[string]string, len(tencentSGPServers))
	for serverID, base := range tencentSGPServers {
		serverBases[serverID] = base
	}
	return &sgpProvider{
		// SGP 网关是国内直连域名，不走“英雄数据网络”的代理设置。
		http:          &http.Client{Timeout: 20 * time.Second},
		serverBases:   serverBases,
		historyCache:  make(map[string]sgpHistoryCacheEntry),
		summonerCache: make(map[string]sgpSummonerCacheEntry),
	}
}

func (p *sgpProvider) cachedSummoner(key string) (sgpSummoner, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	entry, ok := p.summonerCache[key]
	if !ok || time.Since(entry.at) > sgpCacheTTL {
		delete(p.summonerCache, key)
		return sgpSummoner{}, false
	}
	return entry.summoner, true
}

func (p *sgpProvider) cacheSummoner(key string, summoner sgpSummoner) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.summonerCache == nil {
		p.summonerCache = make(map[string]sgpSummonerCacheEntry)
	}
	if _, exists := p.summonerCache[key]; !exists && len(p.summonerCache) >= sgpCacheMax {
		oldestKey := ""
		var oldest time.Time
		for candidate, entry := range p.summonerCache {
			if oldestKey == "" || entry.at.Before(oldest) {
				oldestKey, oldest = candidate, entry.at
			}
		}
		delete(p.summonerCache, oldestKey)
	}
	p.summonerCache[key] = sgpSummonerCacheEntry{at: time.Now(), summoner: summoner}
}

func summonerCacheKey(source, serverID, playerRef string) string {
	key := strings.ToUpper(strings.TrimSpace(serverID)) + "|" + strings.TrimSpace(playerRef)
	return sourceScopedKey(source, key)
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
		var httpErr *LCUHTTPError
		if errors.As(err, &httpErr) {
			return "", fmt.Errorf("客户端 SGP 令牌端点返回 HTTP %d: %w", httpErr.StatusCode, err)
		}
		return "", fmt.Errorf("客户端 SGP 令牌请求失败: %w", err)
	}
	if strings.TrimSpace(payload.AccessToken) == "" {
		return "", errors.New("客户端 SGP 令牌端点响应成功，但 accessToken 字段为空")
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
		JSON json.RawMessage `json:"json"`
	} `json:"games"`
}

type sgpPartialHistoryError struct {
	Cause    error
	Returned int
	Consumed int
}

func (e *sgpPartialHistoryError) Error() string {
	return fmt.Sprintf("SGP 分页读取中断（已返回 %d 场、消费 %d 条）: %v", e.Returned, e.Consumed, e.Cause)
}

func (e *sgpPartialHistoryError) Unwrap() error { return e.Cause }

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

func (kind sgpTokenKind) diagnosticName() string {
	if kind == sgpTokenLeagueSession {
		return "session"
	}
	return "entitlements"
}

func diagnosticPayloadPrefixShape(body []byte) string {
	sample := bytes.TrimSpace(body)
	if len(sample) > 200 {
		sample = sample[:200]
	}
	if len(sample) == 0 {
		return "empty"
	}
	switch sample[0] {
	case '{':
		return "json_object"
	case '[':
		return "json_array"
	case '<':
		lower := bytes.ToLower(sample)
		if bytes.HasPrefix(lower, []byte("<!doctype html")) || bytes.HasPrefix(lower, []byte("<html")) {
			return "html"
		}
		return "markup"
	default:
		return "other"
	}
}

// diagnosticKeySet returns JSON object keys without retaining any values.
func diagnosticKeySet(raw json.RawMessage) []string {
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) != nil {
		return nil
	}
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func diagnosticKeyUnion(entries []json.RawMessage) []string {
	set := make(map[string]struct{})
	for _, entry := range entries {
		for _, key := range diagnosticKeySet(entry) {
			set[key] = struct{}{}
		}
	}
	if len(set) == 0 {
		return nil
	}
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func diagnosticPrivacy(value string) string {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "PUBLIC":
		return "PUBLIC"
	case "PRIVATE":
		return "PRIVATE"
	case "":
		return "UNKNOWN"
	default:
		return "OTHER"
	}
}

// rankedQueueEntries 把 queues 数组和 queueMap 对象统一成一组原始条目，
// 这样两种形状的排位载荷可以走同一套字段探测。
func rankedQueueEntries(raw json.RawMessage) []json.RawMessage {
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) != nil {
		return nil
	}
	var entries []json.RawMessage
	if value, ok := object["queues"]; ok {
		var list []json.RawMessage
		if json.Unmarshal(value, &list) == nil {
			entries = append(entries, list...)
		}
	}
	if value, ok := object["queueMap"]; ok {
		var mapped map[string]json.RawMessage
		if json.Unmarshal(value, &mapped) == nil {
			for _, item := range mapped {
				entries = append(entries, item)
			}
		}
	}
	return entries
}

// diagnosticRankedQueueSamples keeps only four reviewed scalar fields from the
// first solo-queue entry in each container. Recording queues and queueMap
// separately lets a single real-client sample reveal whether one is redacted.
func diagnosticRankedQueueSamples(raw json.RawMessage) map[string]any {
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) != nil {
		return nil
	}
	samples := make(map[string]any, 2)
	if value, ok := object["queues"]; ok {
		var entries []json.RawMessage
		if json.Unmarshal(value, &entries) == nil {
			for _, entry := range entries {
				if sample := diagnosticRankedQueueSample(entry, ""); sample != nil {
					samples["queues"] = sample
					break
				}
			}
		}
	}
	if value, ok := object["queueMap"]; ok {
		var entries map[string]json.RawMessage
		if json.Unmarshal(value, &entries) == nil {
			if entry, exists := entries["RANKED_SOLO_5x5"]; exists {
				if sample := diagnosticRankedQueueSample(entry, "RANKED_SOLO_5x5"); sample != nil {
					samples["queueMap"] = sample
				}
			} else {
				for queueType, entry := range entries {
					if sample := diagnosticRankedQueueSample(entry, queueType); sample != nil {
						samples["queueMap"] = sample
						break
					}
				}
			}
		}
	}
	if len(samples) == 0 {
		return nil
	}
	return samples
}

func diagnosticRankedQueueSample(raw json.RawMessage, fallbackQueueType string) map[string]any {
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) != nil {
		return nil
	}
	queueType := fallbackQueueType
	if value, ok := object["queueType"]; ok {
		_ = json.Unmarshal(value, &queueType)
	}
	if queueType != "RANKED_SOLO_5x5" {
		return nil
	}
	sample := make(map[string]any, 4)
	for _, key := range []string{"wins", "losses", "tier", "division"} {
		value, ok := object[key]
		if !ok {
			continue
		}
		var scalar any
		if json.Unmarshal(value, &scalar) == nil {
			switch scalar.(type) {
			case nil, bool, float64, string:
				sample[key] = scalar
			}
		}
	}
	return sample
}

func sgpNetworkErrorKind(err error) string {
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return "dns"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "timeout"
	}
	if errors.Is(err, syscall.ECONNREFUSED) {
		return "connection_refused"
	}
	return "other"
}

func (p *sgpProvider) recordObservation(event map[string]any) {
	if p != nil && p.observe != nil {
		p.observe(event)
	}
}

func (p *sgpProvider) getJSONWithToken(ctx context.Context, client *LCUClient, kind sgpTokenKind, serverID, route, requestPath, endpoint string, out any) error {
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
		started := time.Now()
		response, err := p.http.Do(request)
		if err != nil {
			// 即使连接在响应体到达前被取消，也算作一次已发出的 SGP 请求；
			// 这样 overview_load_cost 能完整反映 context canceled 的请求放大。
			overviewCostFromContext(ctx).addRequest(0)
			p.recordObservation(map[string]any{
				"event": "sgp_request", "method": http.MethodGet, "route": route, "path": requestPath,
				"http_status": 0, "duration_ms": time.Since(started).Milliseconds(),
				"retried": attempt > 0, "token_kind": kind.diagnosticName(), "body_bytes": 0,
				"error_kind": sgpNetworkErrorKind(err),
			})
			return fmt.Errorf("SGP 网关连接失败: %w", err)
		}
		body, readErr := readLimited(response.Body, sgpResponseMax)
		response.Body.Close()
		overviewCostFromContext(ctx).addRequest(len(body))
		diagnostic := map[string]any{
			"event": "sgp_request", "method": http.MethodGet, "route": route, "path": requestPath,
			"http_status": response.StatusCode, "duration_ms": time.Since(started).Milliseconds(),
			"retried": attempt > 0, "token_kind": kind.diagnosticName(), "body_bytes": len(body),
		}
		switch {
		case response.StatusCode == http.StatusOK:
			if readErr != nil {
				diagnostic["read_failed"] = true
				p.recordObservation(diagnostic)
				return readErr
			}
			if err := json.Unmarshal(body, out); err != nil {
				diagnostic["parse_failed"] = true
				diagnostic["payload_prefix_shape"] = diagnosticPayloadPrefixShape(body)
				diagnostic["payload_sample_bytes"] = min(len(body), 200)
				p.recordObservation(diagnostic)
				return errors.New("SGP 网关返回的数据无法解析")
			}
			p.recordObservation(diagnostic)
			return nil
		case response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden:
			p.recordObservation(diagnostic)
			continue
		default:
			p.recordObservation(diagnostic)
			return fmt.Errorf("SGP 网关返回 HTTP %d", response.StatusCode)
		}
	}
	return errors.New("SGP 访问令牌无效，请确认客户端已登录")
}

func (p *sgpProvider) getJSON(ctx context.Context, client *LCUClient, serverID, route, requestPath, endpoint string, out any) error {
	return p.getJSONWithToken(ctx, client, sgpTokenEntitlements, serverID, route, requestPath, endpoint, out)
}

// matchHistory 读取指定玩家的完整战绩（每场包含全部十名参与者）。
// 结果按 startIndex 起始，按需分页拉取，并做短期缓存以支撑对局页轮询。
// 除对局列表外还返回两个分页参数：consumed 是本次在服务器侧实际消费的
// 条目数（包含缺少 json 或参与者的对局，调用方用它推进下一页偏移量），
// more 表示服务器侧是否可能还有更早的对局。
func (p *sgpProvider) matchHistory(ctx context.Context, client *LCUClient, puuid string, start, count int, useCache bool) ([]*riotMatchInfo, int, bool, error) {
	platform, _, ok := p.available(client)
	if !ok {
		return nil, 0, false, errors.New("SGP 服务器不可用")
	}
	games, consumed, more, err := p.matchHistoryOn(ctx, client, platform, puuid, start, count, useCache)
	if err != nil && !isCancellation(err) {
		p.markFailure()
	}
	return games, consumed, more, err
}

// matchHistoryOn 与 matchHistory 相同，但明确指定国服子服务器；
// 跨服查询失败不会触发全局失败静默期。
func (p *sgpProvider) matchHistoryOn(ctx context.Context, client *LCUClient, serverID, puuid string, start, count int, useCache bool) ([]*riotMatchInfo, int, bool, error) {
	return p.matchHistoryFilteredOn(ctx, client, serverID, puuid, start, count, nil, useCache)
}

// matchHistoryFilteredOn adds the SGP server-side tag contract used by the
// official client ecosystem. Multiple queue tags are sent as an OR query; the
// caller validates the returned queues before treating the filter as supported.
func (p *sgpProvider) matchHistoryFilteredOn(ctx context.Context, client *LCUClient, serverID, puuid string, start, count int, tags []string, useCache bool) ([]*riotMatchInfo, int, bool, error) {
	overviewCostFromContext(ctx).addHistoryCall()
	serverID = strings.ToUpper(strings.TrimSpace(serverID))
	base, ok := p.serverBase(serverID)
	if !ok {
		return nil, 0, false, fmt.Errorf("未收录的国服子服务器：%s", serverID)
	}
	tags = normalizeSGPMatchHistoryTags(tags)
	cacheKey := sourceScopedKey(dataSourceSGP, fmt.Sprintf("%s|%s|%d|%d|%s", serverID, puuid, start, count, strings.Join(tags, ",")))
	if useCache {
		now := time.Now()
		p.mu.Lock()
		entry, ok := p.historyCache[cacheKey]
		if ok && now.Sub(entry.at) < sgpCacheTTL {
			entry.lastUsed = now
			p.historyCache[cacheKey] = entry
		} else if ok {
			delete(p.historyCache, cacheKey)
			ok = false
		}
		p.mu.Unlock()
		if ok {
			overviewCostFromContext(ctx).addHistoryCacheHit()
			return entry.games, entry.consumed, entry.more, nil
		}
	}
	games := make([]*riotMatchInfo, 0, count)
	// fetched 记录服务器侧的偏移量（含缺少 json 或参与者的条目），避免因个别
	// 对局数据不完整导致同一页被反复请求。
	fetched := 0
	lastPageFull := false
	var partialErr error
	for len(games) < count && fetched < count+sgpPageSize {
		pageSize := count - len(games)
		if pageSize > sgpPageSize {
			pageSize = sgpPageSize
		}
		query := url.Values{
			"startIndex": {strconv.Itoa(start + fetched)},
			"count":      {strconv.Itoa(pageSize)},
		}
		for _, tag := range tags {
			query.Add("tag", tag)
		}
		if len(tags) > 1 {
			query.Set("tagsQueryType", "OR")
		}
		// 玩家历史与单场 SUMMARY 是两条不同路由。腾讯 SGP 的玩家历史
		// 固定包含 /player/ 段；漏掉它会命中错误路由并回退到只含本人的
		// LCU 摘要，界面因而无法展示完整十人数据。
		endpoint := base + "/match-history-query/v1/products/lol/player/" + url.PathEscape(puuid) + "/SUMMARY?" + query.Encode()
		var page sgpMatchHistoryPage
		if err := p.getJSON(ctx, client, serverID, "SUMMARY", "/match-history-query/v1/products/lol/player/{puuid}/SUMMARY", endpoint, &page); err != nil {
			if len(games) > 0 {
				partialErr = err
				break
			}
			return nil, 0, false, err
		}
		participantKeys := make(map[string]struct{})
		for _, game := range page.Games {
			var info riotMatchInfo
			if len(game.JSON) == 0 || json.Unmarshal(game.JSON, &info) != nil {
				continue
			}
			var shape struct {
				Participants []json.RawMessage `json:"participants"`
			}
			if json.Unmarshal(game.JSON, &shape) == nil {
				for _, key := range diagnosticKeyUnion(shape.Participants) {
					participantKeys[key] = struct{}{}
				}
			}
			// 保留参与者为空但仍有 gameId 的条目，让上层完整性摘要和
			// 诊断日志能够明确指出 SGP 返回了空 roster；可展示列表会
			// 在转换前过滤它，避免渲染没有主体的空卡片。
			if info.GameID > 0 {
				games = append(games, &info)
			}
		}
		if len(participantKeys) > 0 {
			keys := make([]string, 0, len(participantKeys))
			for key := range participantKeys {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			p.recordObservation(map[string]any{"event": "sgp_participant_keys", "keys": keys})
		}
		fetched += len(page.Games)
		lastPageFull = len(page.Games) >= pageSize
		// A tagged request is already a pure queue page. Never fetch another
		// server page just to replace malformed entries; user pagination owns
		// the next startIndex.
		if len(tags) > 0 || !lastPageFull {
			break
		}
	}
	if partialErr != nil {
		return games, fetched, false, &sgpPartialHistoryError{Cause: partialErr, Returned: len(games), Consumed: fetched}
	}
	more := lastPageFull
	if useCache {
		now := time.Now()
		p.mu.Lock()
		p.historyCache[cacheKey] = sgpHistoryCacheEntry{at: now, lastUsed: now, games: games, consumed: fetched, more: more}
		for key, entry := range p.historyCache {
			if now.Sub(entry.at) >= sgpCacheTTL {
				delete(p.historyCache, key)
			}
		}
		for len(p.historyCache) > sgpCacheMax {
			oldestKey := ""
			var oldestAt time.Time
			for key, entry := range p.historyCache {
				lastUsed := entry.lastUsed
				if lastUsed.IsZero() {
					lastUsed = entry.at
				}
				if oldestKey == "" || lastUsed.Before(oldestAt) {
					oldestAt = lastUsed
					oldestKey = key
				}
			}
			if oldestKey == "" {
				break
			}
			delete(p.historyCache, oldestKey)
		}
		p.mu.Unlock()
	}
	return games, fetched, more, nil
}

func normalizeSGPMatchHistoryTags(tags []string) []string {
	result := make([]string, 0, min(len(tags), 4))
	seen := make(map[string]struct{}, len(tags))
	for _, tag := range tags {
		tag = strings.ToLower(strings.TrimSpace(tag))
		valid := tag == "ranked" || tag == "normal"
		if strings.HasPrefix(tag, "q_") {
			queueID, err := strconv.ParseInt(strings.TrimPrefix(tag, "q_"), 10, 64)
			valid = err == nil && queueID > 0 && queueID <= 10000
		}
		if !valid {
			continue
		}
		if _, exists := seen[tag]; exists {
			continue
		}
		seen[tag] = struct{}{}
		result = append(result, tag)
		if len(result) == 4 {
			break
		}
	}
	return result
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
	var details lcuGameTimeline
	if err := p.getJSON(ctx, client, strings.ToUpper(serverID), "DETAILS", "/match-history-query/v1/products/lol/{server_id}_{game_id}/DETAILS", endpoint, &details); err != nil {
		return nil, err
	}
	frames, framesWrapped := details.framesWithSource()
	eventCount, eventTypes := summarizeTimelineEventTypes(frames)
	p.recordObservation(map[string]any{
		"event": "sgp_timeline_payload", "route": "DETAILS",
		"http_status": http.StatusOK, "frames_wrapped": framesWrapped, "frame_count": len(frames),
		"event_count": eventCount, "event_types": eventTypes,
	})
	if len(frames) == 0 {
		return nil, errors.New("SGP 未返回该对局的时间线")
	}
	return frames, nil
}

/* ---------- 段位（leagues-ledge）与召唤师（summoner-ledge） ---------- */

// sgpRankedQueue 是 SGP 段位数据里的单个队列条目。部分响应会省略或
// 清空 losses，调用方必须验证胜负场次完整性后再展示胜率或计算 LP 差值。
type sgpRankedQueue struct {
	QueueType                  string `json:"queueType"`
	Tier                       string `json:"tier"`
	Rank                       string `json:"rank"`
	LeaguePoints               int    `json:"leaguePoints"`
	Wins                       int    `json:"wins"`
	Losses                     int    `json:"losses"`
	ProvisionalGamesRemaining  int    `json:"provisionalGamesRemaining"`
	HighestTier                string `json:"highestTier"`
	HighestRank                string `json:"highestRank"`
	PreviousSeasonAchievedTier string `json:"previousSeasonAchievedTier"`
	PreviousSeasonAchievedRank string `json:"previousSeasonAchievedRank"`
	PreviousSeasonEndTier      string `json:"previousSeasonEndTier"`
	PreviousSeasonEndRank      string `json:"previousSeasonEndRank"`
	PreviousSeasonHighestTier  string `json:"previousSeasonHighestTier"`
	PreviousSeasonHighestRank  string `json:"previousSeasonHighestRank"`
}

type sgpRankedStats struct {
	Queues                            []sgpRankedQueue `json:"queues"`
	HighestPreviousSeasonEndTier      string           `json:"highestPreviousSeasonEndTier"`
	HighestPreviousSeasonEndRank      string           `json:"highestPreviousSeasonEndRank"`
	HighestPreviousSeasonAchievedTier string           `json:"highestPreviousSeasonAchievedTier"`
	HighestPreviousSeasonAchievedRank string           `json:"highestPreviousSeasonAchievedRank"`
}

// rankedStats 读取玩家的排位数据（当前服务器；该接口无法跨服）。
func (p *sgpProvider) rankedStats(ctx context.Context, client *LCUClient, puuid string) (sgpRankedStats, error) {
	serverID, _, ok := p.available(client)
	if !ok {
		return sgpRankedStats{}, errors.New("SGP 服务器不可用")
	}
	return p.rankedStatsOn(ctx, client, serverID, puuid, true, "")
}

// rankedStatsOn 在当前登录的国服子服务器读取排位。接口路径虽然接受
// serverID，但 league-session 令牌不具备跨服查询能力；调用方必须先拒绝远端服务器。
func (p *sgpProvider) rankedStatsOn(ctx context.Context, client *LCUClient, serverID, puuid string, isSelf bool, privacy string) (sgpRankedStats, error) {
	serverID = strings.ToUpper(strings.TrimSpace(serverID))
	base, ok := p.serverBase(serverID)
	if !ok {
		return sgpRankedStats{}, fmt.Errorf("未收录的国服子服务器：%s", serverID)
	}
	endpoint := base + "/leagues-ledge/v2/rankedStats/puuid/" + url.PathEscape(puuid)
	var raw json.RawMessage
	if err := p.getJSONWithToken(ctx, client, sgpTokenLeagueSession, serverID, "RANKED", "/leagues-ledge/v2/rankedStats/puuid/{puuid}", endpoint, &raw); err != nil {
		return sgpRankedStats{}, err
	}
	var payload struct {
		Queues                            []json.RawMessage `json:"queues"`
		HighestPreviousSeasonEndTier      string            `json:"highestPreviousSeasonEndTier"`
		HighestPreviousSeasonEndRank      string            `json:"highestPreviousSeasonEndRank"`
		HighestPreviousSeasonAchievedTier string            `json:"highestPreviousSeasonAchievedTier"`
		HighestPreviousSeasonAchievedRank string            `json:"highestPreviousSeasonAchievedRank"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return sgpRankedStats{}, err
	}
	p.rankedShapeOnce.Do(func() {
		p.recordObservation(map[string]any{
			"event": "sgp_ranked_stats_shape", "is_self": isSelf,
			"privacy": diagnosticPrivacy(privacy), "body_bytes": len(raw), "queue_count": len(payload.Queues),
			"top_level_keys": diagnosticKeySet(raw), "queue_keys": diagnosticKeyUnion(payload.Queues),
			"sample_queue_values": diagnosticRankedQueueSamples(raw),
		})
	})
	stats := sgpRankedStats{
		Queues:                            make([]sgpRankedQueue, 0, len(payload.Queues)),
		HighestPreviousSeasonEndTier:      payload.HighestPreviousSeasonEndTier,
		HighestPreviousSeasonEndRank:      payload.HighestPreviousSeasonEndRank,
		HighestPreviousSeasonAchievedTier: payload.HighestPreviousSeasonAchievedTier,
		HighestPreviousSeasonAchievedRank: payload.HighestPreviousSeasonAchievedRank,
	}
	for _, entry := range payload.Queues {
		var queue sgpRankedQueue
		if json.Unmarshal(entry, &queue) == nil {
			stats.Queues = append(stats.Queues, queue)
		}
	}
	return stats, nil
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
	cacheKey := summonerCacheKey(dataSourceSGP, serverID, puuid)
	if cached, ok := p.cachedSummoner(cacheKey); ok {
		return cached, nil
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
		started := time.Now()
		response, doErr := p.http.Do(request)
		if doErr != nil {
			p.recordObservation(map[string]any{
				"event": "sgp_request", "method": http.MethodPost, "route": "SUMMONER", "path": "/summoner-ledge/v1/regions/{server_id}/summoners/puuids",
				"http_status": 0, "duration_ms": time.Since(started).Milliseconds(),
				"retried": attempt > 0, "token_kind": sgpTokenLeagueSession.diagnosticName(), "body_bytes": 0,
				"error_kind": sgpNetworkErrorKind(doErr),
			})
			return sgpSummoner{}, fmt.Errorf("SGP 网关连接失败: %w", doErr)
		}
		payload, readErr := readLimited(response.Body, sgpResponseMax)
		response.Body.Close()
		diagnostic := map[string]any{
			"event": "sgp_request", "method": http.MethodPost, "route": "SUMMONER", "path": "/summoner-ledge/v1/regions/{server_id}/summoners/puuids",
			"http_status": response.StatusCode, "duration_ms": time.Since(started).Milliseconds(),
			"retried": attempt > 0, "token_kind": sgpTokenLeagueSession.diagnosticName(), "body_bytes": len(payload),
		}
		switch {
		case response.StatusCode == http.StatusOK:
			if readErr != nil {
				diagnostic["read_failed"] = true
				p.recordObservation(diagnostic)
				return sgpSummoner{}, readErr
			}
			var summoners []sgpSummoner
			if json.Unmarshal(payload, &summoners) != nil {
				diagnostic["parse_failed"] = true
				diagnostic["payload_prefix_shape"] = diagnosticPayloadPrefixShape(payload)
				diagnostic["payload_sample_bytes"] = min(len(payload), 200)
				p.recordObservation(diagnostic)
				return sgpSummoner{}, errSGPSummonerNotFound
			}
			p.recordObservation(diagnostic)
			if len(summoners) == 0 {
				return sgpSummoner{}, errSGPSummonerNotFound
			}
			p.cacheSummoner(cacheKey, summoners[0])
			return summoners[0], nil
		case response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden:
			p.recordObservation(diagnostic)
			continue
		case response.StatusCode == http.StatusNotFound:
			p.recordObservation(diagnostic)
			return sgpSummoner{}, errSGPSummonerNotFound
		default:
			p.recordObservation(diagnostic)
			return sgpSummoner{}, fmt.Errorf("SGP 网关返回 HTTP %d", response.StatusCode)
		}
	}
	return sgpSummoner{}, errors.New("SGP 访问令牌无效，请确认客户端已登录")
}
