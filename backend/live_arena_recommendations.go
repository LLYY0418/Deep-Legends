package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"
)

type liveArenaDetailEntry struct {
	detail  championDetailResponse
	ready   chan struct{}
	until   time.Time
	err     error
	pending bool
	ranking *championRankingRow
	hedged  bool
}

type liveArenaSourceResult struct {
	detail  championDetailResponse
	err     error
	ranking *championRankingRow
}

// The first complete augment source publishes immediately. Remaining work owns
// an independent bounded context and may enrich the same cached snapshot.
func (p *championProvider) loadLiveArenaDetail(ctx context.Context, id int, tier string) (championDetailResponse, error) {
	key := fmt.Sprintf("%d:other:CHERRY:30:%s:none", id, tier)
	p.mu.Lock()
	if p.liveArenaDetails == nil {
		p.liveArenaDetails = map[string]*liveArenaDetailEntry{}
	}
	entry := p.liveArenaDetails[key]
	cacheHit := false
	if entry != nil && time.Now().Before(entry.until) {
		select {
		case <-entry.ready:
			cacheHit = true
		default:
		}
	}
	if entry == nil || !entry.pending && time.Now().After(entry.until) {
		for k, v := range p.liveArenaDetails {
			if !v.pending && time.Now().After(v.until) {
				delete(p.liveArenaDetails, k)
			}
		}
		if len(p.liveArenaDetails) >= 32 {
			p.mu.Unlock()
			return championDetailResponse{}, errors.New("arena recommendation capacity")
		}
		entry = &liveArenaDetailEntry{ready: make(chan struct{}), pending: true}
		p.liveArenaDetails[key] = entry
		go func() {
			defer recoverPanic("arena.recommendation-start")
			p.fillLiveArenaDetail(id, tier, entry)
		}()
	}
	ready := entry.ready
	p.mu.Unlock()
	select {
	case <-ctx.Done():
		return championDetailResponse{}, ctx.Err()
	case <-ready:
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	detail := entry.detail
	detail.ArenaPending = entry.pending
	detail.ArenaCacheHit = cacheHit
	detail.ArenaRanking = entry.ranking
	detail.ArenaHedged = entry.hedged
	return detail, entry.err
}
func (p *championProvider) fillLiveArenaDetail(id int, tier string, entry *liveArenaDetailEntry) {
	defer recoverPanic("arena.recommendation-race")
	defer func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		if entry.pending {
			entry.pending = false
			entry.until = time.Now().Add(5 * time.Second)
			if len(entry.detail.ArenaAugments) == 0 {
				entry.err = errors.New("arena recommendation interrupted")
			}
			select {
			case <-entry.ready:
			default:
				close(entry.ready)
			}
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	flag := &atomic.Bool{}
	ctx = context.WithValue(ctx, arenaHedgeFlagKey{}, flag)
	results := make(chan liveArenaSourceResult, 3)
	go func() {
		defer recoverPanic("arena.recommendation-your")
		d, e := p.liveArenaYourSource(ctx, id)
		results <- liveArenaSourceResult{detail: d, err: e}
	}()
	go func() {
		defer recoverPanic("arena.recommendation-op")
		d, e := p.liveArenaOPSource(ctx, id, tier)
		results <- liveArenaSourceResult{detail: d, err: e}
	}()
	go func() {
		defer recoverPanic("arena.recommendation-ranking")
		select {
		case <-entry.ready:
		case <-ctx.Done():
			results <- liveArenaSourceResult{err: ctx.Err()}
			return
		}
		rankings, err := p.loadArenaRankings(ctx)
		var row *championRankingRow
		if err == nil {
			for _, v := range rankings.Rows {
				if v.ChampionID == id {
					copy := v
					row = &copy
					break
				}
			}
		}
		results <- liveArenaSourceResult{ranking: row, err: err}
	}()
	published := false
	for i := 0; i < 3; i++ {
		var result liveArenaSourceResult
		select {
		case result = <-results:
		case <-ctx.Done():
			result.err = ctx.Err()
		}
		p.mu.Lock()
		entry.hedged = flag.Load()
		if result.ranking != nil {
			entry.ranking = result.ranking
		}
		if result.err == nil && len(result.detail.ArenaAugments) > 0 {
			// Prefer a larger complete row set; enrich missing build and synergy data.
			next := result.detail
			if published {
				next = mergeLiveArenaDetail(entry.detail, next)
			}
			entry.detail = next
			entry.until = time.Now().Add(3 * time.Minute)
			entry.err = nil
			if !published {
				published = true
				close(entry.ready)
			}
		} else if !published {
			entry.err = result.err
		}
		if i == 2 {
			entry.pending = false
			entry.until = time.Now().Add(3 * time.Minute)
			if !published {
				entry.until = time.Now().Add(5 * time.Second)
				if entry.err == nil {
					entry.err = errors.New("no complete arena augment source")
				}
				close(entry.ready)
			}
		}
		p.mu.Unlock()
	}
}
func (p *championProvider) liveArenaYourSource(ctx context.Context, id int) (championDetailResponse, error) {
	aggregate, items, at, err := p.loadArenaChampionAggregate(ctx, id)
	if err != nil {
		return championDetailResponse{}, err
	}
	groups, _, err := mapYourGGArenaAggregateAugmentsWithStats(aggregate.Response.Augments, p.arenaAugmentCatalogFast())
	if err != nil {
		return championDetailResponse{}, err
	}
	d := championDetailResponse{Mode: "arena", Region: "global", Source: "YOUR.GG aggregate", Patch: aggregate.Response.Version, FetchedAt: at, EntertainmentSample: true, ArenaAugmentGroups: groups, ArenaAugments: flattenArenaAugmentGroups(groups, 0)}
	d.Build.CoreItems = mapYourGGArenaAggregateItems(aggregate.Response.CoreItems, items)
	d.Build.PrismItems = mapYourGGArenaAggregateItems(aggregate.Response.PrismaticItems, items)
	decorateLiveArenaItems(&d)
	return d, nil
}
func (p *championProvider) liveArenaOPSource(ctx context.Context, id int, tier string) (championDetailResponse, error) {
	spec, path, query, err := opggDetailRequest("arena", id, "other", tier)
	if err != nil {
		return championDetailResponse{}, err
	}
	query, version, err := p.applyOPGGRequestVersion(ctx, spec, query)
	if err != nil {
		return championDetailResponse{}, err
	}
	data, at, err := p.fetchArenaWithMetadata(ctx, opggChampionHost, path, query, opggDetailCacheKey("arena", spec.Region, id, spec.requestPosition(""), tier, version))
	if err != nil {
		return championDetailResponse{}, err
	}
	var payload opggStructuredDetail
	if json.Unmarshal(data, &payload) != nil || payload.Data.Summary.ID != id {
		return championDetailResponse{}, errors.New("OP.GG champion detail response changed")
	}
	groups := p.structuredArenaAugmentGroupsWithCatalog(payload.Data.AugmentGroup, p.arenaAugmentCatalogFast(), nil)
	applyLocalArenaAugmentGrades(groups)
	d := championDetailResponse{Mode: "arena", Region: spec.Region, Source: "OP.GG JSON", Patch: payload.Meta.Version, FetchedAt: at, EntertainmentSample: true, ArenaAugmentGroups: groups, ArenaAugments: flattenArenaAugmentGroups(groups, 0), TeamCompositions: p.structuredSynergies(id, payload.Data.Synergies)}
	d.Build.CoreItems = p.structuredMetricsForKind(payload.Data.CoreItems, "item", "core", 15)
	d.Build.PrismItems = p.structuredMetricsForKind(payload.Data.PrismItems, "item", "prism", 10)
	d.Build.Boots = p.structuredMetricsForKind(payload.Data.Boots, "item", "boots", 7)
	decorateLiveArenaItems(&d)
	return d, nil
}

func decorateLiveArenaItems(d *championDetailResponse) {
	items := map[int64]gameplayItem{}
	for _, item := range bundledArenaItems() {
		items[item.ID] = item
	}
	for _, rows := range [][]championMetricRow{d.Build.CoreItems, d.Build.PrismItems, d.Build.Boots} {
		for i := range rows {
			for j := range rows[i].Assets {
				asset := &rows[i].Assets[j]
				if item, ok := items[int64(asset.ID)]; ok {
					asset.Name = item.Name
					asset.Description = item.Description
					if item.IconPath != "" {
						asset.Source = "communitydragon"
						asset.Path = item.IconPath
					}
				}
			}
		}
	}
}

func mergeLiveArenaDetail(old, next championDetailResponse) championDetailResponse {
	oldYour, nextYour := strings.Contains(old.Source, "YOUR.GG"), strings.Contains(next.Source, "YOUR.GG")
	if len(next.ArenaAugments) < len(old.ArenaAugments) || len(next.ArenaAugments) == len(old.ArenaAugments) && oldYour && !nextYour {
		next.ArenaAugments = old.ArenaAugments
		next.ArenaAugmentGroups = old.ArenaAugmentGroups
		next.Source = old.Source
	}
	if len(next.Build.CoreItems) == 0 || oldYour && !nextYour && len(old.Build.CoreItems) > 0 {
		next.Build.CoreItems = old.Build.CoreItems
	}
	if len(next.Build.PrismItems) == 0 || oldYour && !nextYour && len(old.Build.PrismItems) > 0 {
		next.Build.PrismItems = old.Build.PrismItems
	}
	if len(next.Build.Boots) == 0 {
		next.Build.Boots = old.Build.Boots
	}
	if len(next.TeamCompositions) == 0 {
		next.TeamCompositions = old.TeamCompositions
	}
	return next
}
