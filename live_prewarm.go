package main

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type liveRecommendationSeed struct {
	ChampionID int64
	QueueID    int64
	MapID      int64
	GameMode   string
	Position   string
}

type livePrewarmEntry struct {
	until time.Time
	busy  bool
}

// Warm the existing provider cache as soon as the current champion is known.
// Roster/history reads never wait for this optional work. Requests on the page
// reuse the provider's cache and in-flight deduplication.
type liveRecommendationPrewarmer struct {
	mu      sync.Mutex
	entries map[string]livePrewarmEntry
	load    func(context.Context, liveRecommendationSeed) error
	report  func(map[string]any)
}

func newLiveRecommendationPrewarmer(p *championProvider, report func(map[string]any)) *liveRecommendationPrewarmer {
	return &liveRecommendationPrewarmer{report: report, load: func(ctx context.Context, seed liveRecommendationSeed) error {
		resolution := resolveGameplayRecommendationMode(seed.QueueID, seed.GameMode, seed.MapID)
		spec := opggModeSpecs[resolution.InternalMode]
		tier, err := gameplayRecommendationTier(spec, "")
		if err != nil {
			return err
		}
		meta, err := p.championMetadataByID(ctx, int(seed.ChampionID))
		if err != nil {
			return err
		}
		_, err = p.loadDetail(ctx, resolution.InternalMode, meta.Slug, spec.requestPosition(seed.Position), tier)
		return err
	}}
}

func (p *liveRecommendationPrewarmer) warm(seed liveRecommendationSeed) bool {
	if p == nil || p.load == nil || seed.ChampionID <= 0 {
		return false
	}
	resolution := resolveGameplayRecommendationMode(seed.QueueID, seed.GameMode, seed.MapID)
	spec, ok := opggModeSpecs[resolution.InternalMode]
	if !ok {
		return false
	}
	if spec.PositionMode == opggPositionRequired {
		position, err := normalizeOPGGPosition(seed.Position)
		if err != nil || position == "" {
			return false
		}
		seed.Position = position
	} else {
		seed.Position = spec.requestPosition("")
	}
	key := fmt.Sprintf("%s:%d:%s", resolution.InternalMode, seed.ChampionID, seed.Position)
	now := time.Now()
	p.mu.Lock()
	if p.entries == nil {
		p.entries = make(map[string]livePrewarmEntry)
	}
	if entry := p.entries[key]; entry.busy || now.Before(entry.until) {
		p.mu.Unlock()
		return false
	}
	for oldKey, entry := range p.entries {
		if !entry.busy && !now.Before(entry.until) {
			delete(p.entries, oldKey)
		}
	}
	if len(p.entries) >= 32 {
		p.mu.Unlock()
		return false
	}
	p.entries[key] = livePrewarmEntry{busy: true}
	p.mu.Unlock()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		start := time.Now()
		err := p.load(ctx, seed)
		ttl, status := 3*time.Minute, "ready"
		if err != nil {
			ttl, status = 30*time.Second, "failed"
		}
		p.mu.Lock()
		p.entries[key] = livePrewarmEntry{until: time.Now().Add(ttl)}
		p.mu.Unlock()
		if p.report != nil {
			p.report(map[string]any{"event": "live_recommendations_prewarm", "status": status, "champion_id": seed.ChampionID, "internal_mode": resolution.InternalMode, "duration_ms": time.Since(start).Milliseconds()})
		}
	}()
	return true
}
