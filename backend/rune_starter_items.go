package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"sync"
	"time"
)

const runeStarterRowLimit = specialistRunePlayerLimit * specialistRunePerPlayerMax

func isRuneTrinket(id int64) bool {
	switch id {
	case 3340, 3341, 3345, 3361, 3362, 3363, 3364:
		return true
	}
	return false
}

// Opening inventory is the player's net purchases in the first 90 seconds.
// Undo/sales inside that window remove items, duplicates remain, and trinkets
// are excluded. Later recalls cannot change this immutable opening inventory.
func participantStarterItems(frames []timelineFrame, participantID int64) []int64 {
	opening := make([]timelineFrame, 0, len(frames))
	for _, frame := range frames {
		var events []timelineEvent
		for _, event := range frame.Events {
			if event.Timestamp >= 0 && event.Timestamp <= 90_000 {
				events = append(events, event)
			}
		}
		opening = append(opening, timelineFrame{Events: events})
	}
	records, _ := participantTimelineRecords(opening, participantID)
	items := []int64{}
	for _, record := range records {
		if isRuneTrinket(record.itemID) {
			continue
		}
		if !record.sold {
			items = append(items, record.itemID)
			continue
		}
		for i := len(items) - 1; i >= 0; i-- {
			if items[i] == record.itemID {
				items = append(items[:i], items[i+1:]...)
				break
			}
		}
	}
	return items[:min(len(items), 6)]
}

type runeStarterRowRequest struct {
	Key      string `json:"key"`
	PlayedAt int64  `json:"playedAt"`
}

type runeStarterRequest struct {
	Source     string                  `json:"source"`
	ChampionID int                     `json:"championId"`
	Position   string                  `json:"position"`
	Rows       []runeStarterRowRequest `json:"rows"`
}

type runeStarterRow struct {
	Key            string  `json:"key"`
	PlayedAt       int64   `json:"playedAt"`
	StarterItemIDs []int64 `json:"starterItemIds"`
}

type runeStarterBatchKey struct{}
type runeStarterBatchStats struct {
	prefetched                   int
	mu                           sync.Mutex
	rateLimit, retryAfterSeconds int
}

func recordRuneStarterBatchError(ctx context.Context, err error) {
	batch, _ := ctx.Value(runeStarterBatchKey{}).(*runeStarterBatchStats)
	if batch == nil || !errors.Is(err, errThrottled) {
		return
	}
	retry := 5
	var status *riotStatusError
	if errors.As(err, &status) {
		retry = min(60, max(5, status.retryAfter))
	}
	batch.mu.Lock()
	batch.rateLimit++
	batch.retryAfterSeconds = max(batch.retryAfterSeconds, retry)
	batch.mu.Unlock()
}

// This second HTTP request starts only after the rune list has rendered. It
// accepts visible row keys, never arbitrary match IDs or account identifiers.
func (a *app) handleGameplayRuneStarters(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	var request runeStarterRequest
	batch := &runeStarterBatchStats{}
	cost := &riotOverviewCostTracker{}
	result := []runeStarterRow{}
	defer func() {
		batch.mu.Lock()
		rateLimit, retry, prefetched := batch.rateLimit, batch.retryAfterSeconds, batch.prefetched
		batch.mu.Unlock()
		cost.mu.Lock()
		queueWait := cost.queueWait.Milliseconds()
		cost.mu.Unlock()
		source := request.Source
		if source != "specialist" && source != "pro" {
			source = "unknown"
		}
		requested := min(runeStarterRowLimit, len(request.Rows))
		a.recordDiagnostic(map[string]any{"event": "rune_starter_batch", "source": source,
			"rows_requested": requested, "success": len(result), "rate_limit": rateLimit,
			"other_failed": max(0, requested-len(result)-rateLimit), "retry_after_s": retry, "queue_wait_ms": queueWait, "duration_ms": time.Since(started).Milliseconds(), "prefetched": prefetched})
	}()
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil || request.ChampionID <= 0 || request.ChampionID > 10000 || len(request.Rows) == 0 || len(request.Rows) > runeStarterRowLimit || (request.Source != "specialist" && request.Source != "pro") {
		http.Error(w, "出门装请求无效", http.StatusBadRequest)
		return
	}
	position, err := normalizeOPGGPosition(request.Position)
	if err != nil {
		http.Error(w, "推荐分路无效", http.StatusBadRequest)
		return
	}
	wanted := map[string]int64{}
	for _, row := range request.Rows {
		if row.Key == "" || len(row.Key) > 64 || row.PlayedAt <= 0 {
			http.Error(w, "对局记录无效", http.StatusBadRequest)
			return
		}
		wanted[row.Key] = row.PlayedAt
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	ctx = context.WithValue(ctx, runeStarterBatchKey{}, batch)
	ctx = context.WithValue(ctx, riotOverviewCostTrackerKey{}, cost)
	if request.Source == "specialist" {
		if a.riot != nil && riotKeyConfigured() {
			result = a.riot.specialistStarterRows(withRiotQueueLimit(ctx, 6*time.Second), int64(request.ChampionID), position, wanted)
		}
	} else {
		result = a.proRuneSource().starterRows(ctx, request.ChampionID, position, wanted)
	}
	response := map[string]any{"starters": result}
	if batch.retryAfterSeconds > 0 {
		response["retryAfterSeconds"] = batch.retryAfterSeconds
	}
	respondJSON(w, response)
}

func (p *riotProvider) specialistStarterRows(ctx context.Context, championID int64, position string, wanted map[string]int64) []runeStarterRow {
	p.specialistMu.Lock()
	rows := cloneSpecialistRunes(p.specialistCache[specialistRuneKey(championID, position)].runes)
	p.specialistMu.Unlock()
	selected := []gameplayRecommendationRune{}
	for _, row := range rows {
		if at, ok := wanted[row.Key]; ok && at == row.PlayedAt && validRiotMatchID(row.runeMatchID) && row.runeParticipantID > 0 {
			selected = append(selected, row)
		}
		if len(selected) == runeStarterRowLimit {
			break
		}
	}
	result := make([]runeStarterRow, len(selected))
	// This budget is separate from specialistRuneRequestBudget (36). At most
	// nine visible games may load, with two workers and bounded foreground admission.
	runeStarterParallel(ctx, selected, 2, func(index int, row gameplayRecommendationRune) {
		items, err := p.specialistMatchStarters(ctx, row.runeMatchID, row.runeParticipantID)
		recordRuneStarterBatchError(ctx, err)
		if err == nil {
			result[index] = runeStarterRow{row.Key, row.PlayedAt, items}
		}
	})
	return nonemptyStarterRows(result)
}

func (p *riotProvider) specialistMatchStarters(ctx context.Context, matchID string, participantID int64) ([]int64, error) {
	if !validRiotMatchID(matchID) || participantID <= 0 {
		return nil, errors.New("invalid starter match")
	}
	p.specialistMu.Lock()
	if p.starterCache == nil {
		var store *localStore
		if p.champions != nil && p.champions.cache != nil && p.champions.cache.dir != "" {
			store = &localStore{root: filepath.Dir(p.champions.cache.dir)}
		}
		p.starterCache = newPublicBinaryCache(store, "rune-starters", riotMatchCacheMax, 128<<20)
		p.starterFailures = map[string]time.Time{}
	}
	cache := p.starterCache
	failedAt := p.starterFailures[matchID]
	p.specialistMu.Unlock()
	if !failedAt.IsZero() && time.Since(failedAt) < 90*time.Second {
		return nil, errors.New("starter retry cooldown")
	}
	loaded, err := cache.loadWithStatus(ctx, "rune-starters-v1|"+matchID, 365*24*time.Hour, 0, true, func(ctx context.Context) ([]byte, error) {
		frames, err := p.matchTimeline(withRiotQueueLimit(ctx, 6*time.Second), matchID)
		if err != nil {
			if riotErrorStatus(err) == http.StatusTooManyRequests && !errors.Is(err, errThrottled) {
				err = fmt.Errorf("%w: %w", errThrottled, err)
			}
			return nil, err
		}
		participants := map[int64][]int64{}
		for _, frame := range frames {
			for _, event := range frame.Events {
				if event.ParticipantID > 0 {
					participants[event.ParticipantID] = nil
				}
			}
		}
		for id := range participants {
			participants[id] = participantStarterItems(frames, id)
		}
		// Cache only derived opening items for every participant, not multi-MB
		// timelines or player identities; the match key and cache have fixed bounds.
		data, err := json.Marshal(participants)
		if fromPrefetch, _ := ctx.Value(specialistStarterPrefetchContextKey{}).(bool); err == nil && fromPrefetch {
			p.specialistMu.Lock()
			state := p.starterPrefetches[matchID]
			state.success = true
			p.starterPrefetches[matchID] = state
			p.specialistMu.Unlock()
		}
		return data, err
	})
	if err != nil {
		if !errors.Is(err, errThrottled) {
			p.specialistMu.Lock()
			for id, at := range p.starterFailures {
				if time.Since(at) >= 90*time.Second || len(p.starterFailures) >= riotMatchCacheMax {
					delete(p.starterFailures, id)
				}
			}
			p.starterFailures[matchID] = time.Now()
			p.specialistMu.Unlock()
		}
		p.recordSpecialistDiagnostic(map[string]any{"event": "rune_starter_items", "source": "specialist", "outcome": "failed", "reason": specialistErrorKind(err)})
		return nil, err
	}
	p.specialistMu.Lock()
	prefetched := p.starterPrefetches[matchID].success
	p.specialistMu.Unlock()
	if prefetched {
		if batch, _ := ctx.Value(runeStarterBatchKey{}).(*runeStarterBatchStats); batch != nil {
			batch.mu.Lock()
			batch.prefetched++
			batch.mu.Unlock()
		}
	}
	var items map[int64][]int64
	if err := json.Unmarshal(loaded.data, &items); err != nil {
		return nil, err
	}
	p.recordSpecialistDiagnostic(map[string]any{"event": "rune_starter_items", "source": "specialist", "outcome": "success", "cache": loaded.state, "item_count": len(items[participantID])})
	return items[participantID], nil
}

func nonemptyStarterRows(rows []runeStarterRow) []runeStarterRow {
	result := []runeStarterRow{}
	for _, row := range rows {
		if row.Key != "" {
			if row.StarterItemIDs == nil {
				row.StarterItemIDs = []int64{}
			}
			result = append(result, row)
		}
	}
	return result
}

func runeStarterParallel[T any](ctx context.Context, rows []T, workers int, fn func(int, T)) {
	var wg sync.WaitGroup
	jobs := make(chan int)
	for i := 0; i < min(workers, len(rows)); i++ {
		wg.Add(1)
		go func() {
			defer recoverPanic("rune_starter_items.runeStarterParallel.1")

			defer wg.Done()
			for index := range jobs {
				if ctx.Err() == nil {
					func() { defer recoverPanic("runeStarterParallel.job"); fn(index, rows[index]) }()
				}
			}
		}()
	}
	for index := range rows {
		select {
		case jobs <- index:
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			return
		}
	}
	close(jobs)
	wg.Wait()
}

func proRuneRowKey(row proRuneRow) string {
	return fmt.Sprintf("pro-%s-%d", row.Game.ID, row.Participant.ParticipantID)
}

func proDetailsParticipantKeys(key string) string {
	switch key {
	case "participantId", "items", "perkMetadata", "summonerSpells", "spells",
		"summoner1Id", "summoner2Id", "summoner1ID", "summoner2ID", "spell1Id", "spell2Id",
		"summonerSpell1", "summonerSpell2", "summonerSpellOne", "summonerSpellTwo":
		return key
	}
	return "<unknown-key>"
}
