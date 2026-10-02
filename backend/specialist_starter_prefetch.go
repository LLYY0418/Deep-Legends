package main

import (
	"context"
	"encoding/json"
	"time"
)

type specialistStarterPrefetchContextKey struct{}

type specialistStarterPrefetch struct {
	at      time.Time
	done    bool
	success bool
}

func (p *riotProvider) prefetchSpecialistStarters(ctx context.Context, rows []gameplayRecommendationRune) int {
	selected := []gameplayRecommendationRune{}
	p.specialistMu.Lock()
	if p.starterPrefetches == nil {
		p.starterPrefetches = map[string]specialistStarterPrefetch{}
	}
	for id, state := range p.starterPrefetches {
		if state.done && (time.Since(state.at) > specialistRuneCacheTTL || len(p.starterPrefetches) >= riotMatchCacheMax) {
			delete(p.starterPrefetches, id)
		}
	}
	for _, row := range rows[:min(3, len(rows))] {
		if !validRiotMatchID(row.runeMatchID) || row.runeParticipantID <= 0 {
			continue
		}
		if _, exists := p.starterPrefetches[row.runeMatchID]; exists {
			continue
		}
		p.starterPrefetches[row.runeMatchID] = specialistStarterPrefetch{at: time.Now()}
		selected = append(selected, row)
	}
	p.specialistMu.Unlock()
	if len(selected) == 0 {
		return 0
	}
	// Detached from the response, but bounded independently. Both workers use
	// specialistMatchStarters' cache singleflight and foreground six-second queue.
	preCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	preCtx = context.WithValue(preCtx, specialistStarterPrefetchContextKey{}, true)
	goSafe("specialist-starter-prefetch", func() {
		defer cancel()
		runeStarterParallel(preCtx, selected, 2, func(_ int, row gameplayRecommendationRune) {
			defer func() {
				p.specialistMu.Lock()
				state := p.starterPrefetches[row.runeMatchID]
				state.done = true
				p.starterPrefetches[row.runeMatchID] = state
				p.specialistMu.Unlock()
			}()
			_, _ = p.specialistMatchStarters(preCtx, row.runeMatchID, row.runeParticipantID)
		})
	})
	return len(selected)
}
func (p *riotProvider) attachReadySpecialistStarters(rows []gameplayRecommendationRune) {
	p.specialistMu.Lock()
	cache := p.starterCache
	p.specialistMu.Unlock()
	for i := range rows {
		data, ok := cache.lookupReady("rune-starters-v1|" + rows[i].runeMatchID)
		if !ok {
			continue
		}
		var participants map[int64][]int64
		if json.Unmarshal(data, &participants) == nil {
			if items, ok := participants[rows[i].runeParticipantID]; ok {
				rows[i].StarterItemIDs = append([]int64{}, items...)
			}
		}
	}
}
