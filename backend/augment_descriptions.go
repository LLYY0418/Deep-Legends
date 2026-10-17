package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"sync"
	"time"
)

// Foreground reads never wait for the CDN when a usable local description exists.
type augmentDescriptionFlights struct {
	sync.Mutex
	active map[int64]bool
}

func augmentDescriptionKey(id int64) string {
	return "normalized-augments-v1|description:" + strconv.FormatInt(id, 10)
}
func augmentDescriptionFailure(id int64, err error) gameplayAugmentDescription {
	kind := "no_response"
	if errors.Is(err, errCommunityDragonAugmentBackoff) {
		kind = "backoff_skipped"
	} else if errors.Is(err, context.DeadlineExceeded) || championProviderErrorKind(err) == "timeout" {
		kind = "timeout"
	}
	return gameplayAugmentDescription{ID: id, Status: "retry", Source: "none", ErrorKind: kind}
}
func augmentDescriptionFromRows(id int64, rows []gameplayAugment, source string) gameplayAugmentDescription {
	for _, row := range rows {
		if row.ID == id {
			if text := augmentEffectDescription(row.Description); text != "" {
				return gameplayAugmentDescription{ID: id, Description: text, Status: "ok", Source: source}
			}
		}
	}
	return gameplayAugmentDescription{ID: id, Status: "unavailable", Source: "none", ErrorKind: "not_found"}
}
func (p *championProvider) cachedAugmentDescription(id int64) (gameplayAugmentDescription, bool) {
	if p.cache == nil {
		return gameplayAugmentDescription{}, false
	}
	c := p.cache
	key := augmentDescriptionKey(id)
	c.mu.Lock()
	entry := c.entries[key]
	c.mu.Unlock()
	if len(entry.Data) == 0 {
		entry, _ = c.readDisk(key)
	}
	var item gameplayAugmentDescription
	if !c.cacheNow().Before(entry.StaleUntil) || len(entry.Data) == 0 || json.Unmarshal(entry.Data, &item) != nil || item.ID != id || item.Status != "ok" || augmentEffectDescription(item.Description) == "" {
		return item, false
	}
	item.Source = "cache"
	if !c.cacheNow().Before(entry.ExpiresAt) {
		item.Source = "stale_cache"
	}
	return item, true
}
func (p *championProvider) saveAugmentDescription(item gameplayAugmentDescription) {
	if p.cache == nil || item.Status != "ok" {
		return
	}
	data, err := json.Marshal(item)
	if err != nil {
		return
	}
	now := p.cache.cacheNow()
	sum := sha256.Sum256(data)
	entry := championCacheEnvelope{Schema: championCacheSchema, Key: augmentDescriptionKey(item.ID), FetchedAt: now, ExpiresAt: now.Add(7 * 24 * time.Hour), StaleUntil: now.Add(365 * 24 * time.Hour), Hash: hex.EncodeToString(sum[:]), Data: data}
	p.cache.mu.Lock()
	p.cache.storeMemoryLocked(entry.Key, entry)
	p.cache.mu.Unlock()
	p.cache.enqueueDiskWrite(entry)
}
func (p *championProvider) onlineAugmentDescription(ctx context.Context, id int64) gameplayAugmentDescription {
	if id >= 1000 {
		description, err := p.gameplayAugmentDescription(ctx, id)
		if err != nil {
			return augmentDescriptionFailure(id, err)
		}
		return augmentDescriptionFromRows(id, []gameplayAugment{{ID: id, Description: description}}, "hexdata")
	}
	requestCtx, cancel := context.WithTimeout(ctx, communityDragonAugmentRequestBudget)
	defer cancel()
	result, err := p.fetchCommunityDragonAugmentPartResult(requestCtx, ctx, "arena", "/latest/cdragon/arena/zh_cn.json")
	if err != nil {
		return augmentDescriptionFailure(id, err)
	}
	rows, err := normalizeArenaAugmentDescriptions(result.data)
	if err != nil {
		return augmentDescriptionFailure(id, err)
	}
	item := augmentDescriptionFromRows(id, rows, "cdragon")
	if item.Status != "ok" && result.upstreamErr != nil {
		return augmentDescriptionFailure(id, result.upstreamErr)
	}
	return item
}
func (p *championProvider) refreshAugmentDescription(id int64) {
	f := &p.augmentDescriptions
	f.Lock()
	if f.active[id] {
		f.Unlock()
		return
	}
	if f.active == nil {
		f.active = map[int64]bool{}
	}
	f.active[id] = true
	f.Unlock()
	goSafe("augment-description-refresh", func() {
		defer func() { f.Lock(); delete(f.active, id); f.Unlock() }()
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		item := p.onlineAugmentDescription(ctx, id)
		p.saveAugmentDescription(item)
	})
}
func (p *championProvider) resolveAugmentDescription(ctx context.Context, id int64, local func(context.Context) []gameplayAugment) gameplayAugmentDescription {
	if item, ok := p.cachedAugmentDescription(id); ok {
		if item.Source == "stale_cache" {
			p.refreshAugmentDescription(id)
		}
		return item
	}
	if local != nil {
		if item := augmentDescriptionFromRows(id, local(ctx), "lcu"); item.Status == "ok" {
			p.saveAugmentDescription(item)
			return item
		}
	}
	if item := augmentDescriptionFromRows(id, bundledAugmentDescriptions(), "bundled"); item.Status == "ok" {
		p.refreshAugmentDescription(id)
		return item
	}
	item := p.onlineAugmentDescription(ctx, id)
	p.saveAugmentDescription(item)
	return item
}
