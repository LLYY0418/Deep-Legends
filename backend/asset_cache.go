package main

import (
	"context"
	"errors"
	"strings"
	"time"
)

const (
	assetCacheMaxEntries = 1200
	assetCacheMaxBytes   = 256 * 1024 * 1024
	// R127 P1-a.2：远程主机整体退避。DeadlineExceeded 以前完全不记失败，同一个
	// 图标每被请求一次就重新等满整个远程超时；国内访问 raw.communitydragon.org
	// 时一屏大乱斗能烧掉约 70 秒。改为按主机统计连续超时，达到阈值后在退避窗口
	// 内该主机的请求直接快速失败。
	assetHostTimeoutThreshold = 3
	assetHostBackoffTTL       = 60 * time.Second
)

var errCachedAssetFailure = errors.New("asset temporarily unavailable")

type assetFlight struct {
	done       chan struct{}
	data       []byte
	err        error
	generation uint64
}

func (a *app) loadAsset(ctx context.Context, key string, maxEntrySize int, negativeTTL time.Duration, loader func(context.Context) ([]byte, error)) ([]byte, error) {
	a.assetCacheMu.Lock()
	if data, ok := a.assetCache[key]; ok {
		recordAssetCacheState(ctx, "memory")
		a.assetCacheMu.Unlock()
		return data, nil
	}
	if until := a.assetFailureUntil[key]; !until.IsZero() {
		if time.Now().Before(until) {
			recordAssetCacheState(ctx, "negative")
			a.assetCacheMu.Unlock()
			return nil, errCachedAssetFailure
		}
		delete(a.assetFailureUntil, key)
	}
	if flight := a.assetFlights[key]; flight != nil {
		recordAssetCacheState(ctx, "shared")
		done := flight.done
		a.assetCacheMu.Unlock()
		select {
		case <-done:
			if ctx.Err() == nil && (errors.Is(flight.err, context.Canceled) || errors.Is(flight.err, context.DeadlineExceeded)) {
				return a.loadAsset(ctx, key, maxEntrySize, negativeTTL, loader)
			}
			return flight.data, flight.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if a.assetFlights == nil {
		a.assetFlights = make(map[string]*assetFlight)
	}
	flight := &assetFlight{done: make(chan struct{}), generation: a.assetCacheGeneration}
	a.assetFlights[key] = flight
	a.assetCacheMu.Unlock()

	data, err := loader(ctx)

	a.assetCacheMu.Lock()
	flight.data = data
	flight.err = err
	cacheIsCurrent := flight.generation == a.assetCacheGeneration
	if err == nil && cacheIsCurrent {
		delete(a.assetFailureUntil, key)
		if maxEntrySize > 0 && len(data) <= maxEntrySize {
			a.storeAssetLocked(key, data)
		}
	} else if err != nil && cacheIsCurrent && negativeTTL > 0 && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		if a.assetFailureUntil == nil {
			a.assetFailureUntil = make(map[string]time.Time)
		}
		a.assetFailureUntil[key] = time.Now().Add(negativeTTL)
	}
	delete(a.assetFlights, key)
	close(flight.done)
	a.assetCacheMu.Unlock()
	return data, err
}

func (a *app) storeAssetLocked(key string, data []byte) {
	if a.assetCache == nil {
		a.assetCache = make(map[string][]byte)
	}
	if previous, ok := a.assetCache[key]; ok {
		a.assetCacheBytes -= len(previous)
	} else {
		a.assetCacheOrder = append(a.assetCacheOrder, key)
	}
	for len(a.assetCache) >= assetCacheMaxEntries || a.assetCacheBytes+len(data) > assetCacheMaxBytes {
		if len(a.assetCacheOrder) > 0 {
			oldest := a.assetCacheOrder[0]
			a.assetCacheOrder = a.assetCacheOrder[1:]
			if previous, ok := a.assetCache[oldest]; ok {
				a.assetCacheBytes -= len(previous)
				delete(a.assetCache, oldest)
			}
			continue
		}
		removed := false
		for existingKey, previous := range a.assetCache {
			a.assetCacheBytes -= len(previous)
			delete(a.assetCache, existingKey)
			removed = true
			break
		}
		if !removed {
			break
		}
	}
	a.assetCache[key] = data
	a.assetCacheBytes += len(data)
}

/* ---------- R127 P1-a.2：远程主机级退避 ---------- */

// assetHostBlocked 报告该主机是否处于退避窗口内。退避只由连续超时触发：
// 主机能给出任何明确响应（成功或 404/403）都说明它可达，计数立刻清零。
func (a *app) assetHostBlocked(host string) bool {
	if strings.TrimSpace(host) == "" {
		return false
	}
	a.assetCacheMu.RLock()
	defer a.assetCacheMu.RUnlock()
	until := a.assetHostBackoffUntil[host]
	return !until.IsZero() && time.Now().Before(until)
}

func (a *app) observeAssetHostResult(host string, err error) {
	if strings.TrimSpace(host) == "" {
		return
	}
	a.assetCacheMu.Lock()
	if a.assetHostTimeouts == nil {
		a.assetHostTimeouts = make(map[string]int)
	}
	if a.assetHostBackoffUntil == nil {
		a.assetHostBackoffUntil = make(map[string]time.Time)
	}
	if err != nil && (errors.Is(err, errCachedAssetFailure) || errors.Is(err, errCommunityImageCandidatesExhausted)) {
		// 快速失败本身就是退避的产物，「整组候选都失败」是聚合结论（每个候选已经
		// 各自归因过了）。两者都不是关于这台主机的新鲜证据：既不清零也不累加，
		// 否则外层聚合错误会把内层刚建立的退避立刻抹掉，退避形同虚设。
		a.assetCacheMu.Unlock()
		return
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		// 成功、404、连接被拒等都是「主机有回应」，不属于退避要治的病症。
		// 请求方自己取消（Canceled）同样不计入，避免把前端离开页面当成主机故障。
		delete(a.assetHostTimeouts, host)
		delete(a.assetHostBackoffUntil, host)
		a.assetCacheMu.Unlock()
		return
	}
	a.assetHostTimeouts[host]++
	consecutive := a.assetHostTimeouts[host]
	backoff := consecutive >= assetHostTimeoutThreshold
	if backoff {
		a.assetHostBackoffUntil[host] = time.Now().Add(assetHostBackoffTTL)
		a.assetHostTimeouts[host] = 0
	}
	a.assetCacheMu.Unlock()
	if backoff {
		a.recordDiagnostic(map[string]any{
			"event": "asset_host_backoff", "host": host, "consecutive_timeouts": consecutive,
			"backoff_seconds": int(assetHostBackoffTTL / time.Second),
		})
	}
}

// loadAssetFromHost 与 loadAsset 相同，但把超时归因到具体远程主机：连续超时达到
// assetHostTimeoutThreshold 次后，退避窗口内该主机的请求直接快速失败，不再每张都
// 等满远程超时。本机 LCU 资源继续用 loadAsset（没有远程主机可归因）。
//
// 退避判定放在 loader 内部，也就是只在内存、磁盘与共享 flight 都没命中、真的要
// 出网时才生效。放在外面会让主机退避把已经缓存在本地的图标一起打成占位——那些
// 图根本不需要联网。
func (a *app) loadAssetFromHost(ctx context.Context, host, key string, maxEntrySize int, negativeTTL time.Duration, loader func(context.Context) ([]byte, error)) ([]byte, error) {
	if strings.TrimSpace(host) == "" {
		return a.loadAsset(ctx, key, maxEntrySize, negativeTTL, loader)
	}
	reachedHost := false
	data, err := a.loadAsset(ctx, key, maxEntrySize, negativeTTL, func(loadContext context.Context) ([]byte, error) {
		if a.assetHostBlocked(host) {
			recordAssetCacheState(loadContext, "host-backoff")
			return nil, errCachedAssetFailure
		}
		reachedHost = true
		return loader(loadContext)
	})
	// 只有真的出过网才算关于这台主机的新鲜证据：内存/磁盘命中与退避期间的快速
	// 失败都不能证明主机可达，否则一次缓存命中就会把退避抹掉，退避等于没有。
	if reachedHost {
		a.observeAssetHostResult(host, err)
	}
	return data, err
}
