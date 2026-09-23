package main

// R127 P1-c.3：一场已经结束的对局，平均段位不会再变。之前只有 90 秒的 OP.GG
// 页面内存缓存（opggTierCacheTTL），重开同一个韩服玩家要再等 2–4 秒重新取页。
// 这里按 gameId 把匹配到的平均段位落盘 7 天：命中时一个 OP.GG 请求都不发。
//
// 只缓存「确实匹配到段位」的结果——OP.GG 没收录或还没更新时，不能把「查不到」
// 当成永久结论写盘。文件里只有对局 ID 与公开的平均段位，不含账号标识，不外发。

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strconv"
	"time"
)

const (
	matchTierCacheEntries = 4000
	matchTierCacheBytes   = 8 << 20
	matchTierCacheTTL     = 7 * 24 * time.Hour
)

// matchTierCacheKey 复用韩服对局 ID 的形状（KR_<gameId>），与 riot-matches 的
// 键前缀区分开，落盘时按内容哈希命名。
func matchTierCacheKey(gameID int64) string {
	return "kr-match-tier-v1|KR_" + strconv.FormatInt(gameID, 10)
}

func (a *app) matchTierDiskCache() *championDataCache {
	a.matchTierCacheOnce.Do(func() {
		a.matchTierCache = newPublicBinaryCache(a.storage, "match-tiers", matchTierCacheEntries, matchTierCacheBytes)
	})
	return a.matchTierCache
}

func (a *app) readMatchTierCache(gameID int64) (*matchTiersResponse, bool) {
	if gameID <= 0 {
		return nil, false
	}
	cache := a.matchTierDiskCache()
	if cache == nil {
		return nil, false
	}
	entry, err := cache.readDisk(matchTierCacheKey(gameID))
	if err != nil || len(entry.Data) == 0 {
		return nil, false
	}
	if !entry.ExpiresAt.IsZero() && time.Now().After(entry.ExpiresAt) {
		// 过期就删掉：隐私声明写的是「保留 7 天」，不能只是读的时候装作没有，
		// 让文件在目录里一直躺着（目录未满时淘汰逻辑不会碰它）。
		_ = os.Remove(cache.pathFor(matchTierCacheKey(gameID)))
		return nil, false
	}
	var value matchTiersResponse
	if json.Unmarshal(entry.Data, &value) != nil || value.Tier == "" {
		return nil, false
	}
	return &value, true
}

func (a *app) writeMatchTierCache(gameID int64, value *matchTiersResponse) {
	if gameID <= 0 || value == nil || value.Tier == "" {
		return
	}
	cache := a.matchTierDiskCache()
	if cache == nil {
		return
	}
	data, err := json.Marshal(value)
	if err != nil {
		return
	}
	hash := sha256.Sum256(data)
	now := time.Now()
	_ = cache.writeDisk(championCacheEnvelope{Schema: championCacheSchema, Key: matchTierCacheKey(gameID),
		FetchedAt: now, ExpiresAt: now.Add(matchTierCacheTTL), StaleUntil: now.Add(matchTierCacheTTL),
		Hash: hex.EncodeToString(hash[:]), Data: data})
}

// splitCachedMatchTiers 先把长期缓存命中的对局挑出来，剩下的才需要去查 OP.GG。
// 命中的部分直接进响应，因此重开同一个玩家时平均段位是立即显示的。
func (a *app) splitCachedMatchTiers(matches []matchTierMatchRequest) (map[string]*matchTiersResponse, []matchTierMatchRequest, int) {
	response := make(map[string]*matchTiersResponse, len(matches))
	missing := make([]matchTierMatchRequest, 0, len(matches))
	hits := 0
	for _, match := range matches {
		if value, ok := a.readMatchTierCache(match.GameID); ok {
			response[strconv.FormatInt(match.GameID, 10)] = value
			hits++
			continue
		}
		missing = append(missing, match)
	}
	return response, missing, hits
}
