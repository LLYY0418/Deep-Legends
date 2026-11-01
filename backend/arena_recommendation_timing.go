package main

import (
	"context"
	"net/http"
	"sync"
	"time"
)

type arenaRecommendationClock struct{ locked, started, ready time.Time }
type arenaRecommendationTimeline struct {
	mu         sync.Mutex
	entries    map[int64]arenaRecommendationClock
	inProgress time.Time
}
type arenaRecommendationTiming struct {
	LockedAt         *int64 `json:"locked_at"`
	PrewarmStartedAt *int64 `json:"prewarm_started_at"`
	PrewarmReadyAt   *int64 `json:"prewarm_ready_at"`
	InProgressAt     int64  `json:"in_progress_at"`
	RenderedAt       int64  `json:"rendered_at"`
	Source           string `json:"source"`
	CacheHit         bool   `json:"cache_hit"`
	Hedged           bool   `json:"hedged"`
}

func lockedArenaChampion(s lcuChampSelectSession) int64 {
	if s.LocalPlayerCellID == nil {
		return 0
	}
	for _, player := range s.MyTeam {
		if player.CellID != nil && *player.CellID == *s.LocalPlayerCellID && player.ChampionID > 0 {
			for _, turn := range s.Actions {
				for _, action := range turn {
					if action.ActorCellID == *s.LocalPlayerCellID && action.Type == "pick" && action.Completed && action.ChampionID == player.ChampionID {
						return player.ChampionID
					}
				}
			}
		}
	}
	return 0
}
func (a *app) prewarmLockedArena(s lcuChampSelectSession, queue int64, mode string) {
	id := lockedArenaChampion(s)
	if id == 0 || !isArenaQueue(queue, mode) {
		return
	}
	t := &a.arenaRecommendationTimeline
	t.mu.Lock()
	if t.entries == nil {
		t.entries = map[int64]arenaRecommendationClock{}
	}
	clock, exists := t.entries[id]
	if !exists {
		clock.locked = time.Now()
		clock.started = clock.locked
		t.entries[id] = clock
	}
	t.mu.Unlock()
	a.liveRecommendationPrewarmer.warm(liveRecommendationSeed{ChampionID: id, QueueID: queue, MapID: 30, GameMode: "CHERRY", Position: "other"})
}
func (a *app) prewarmArenaSession(ctx context.Context, c *LCUClient, s lcuChampSelectSession) {
	if lockedArenaChampion(s) == 0 {
		return
	}
	if s.QueueID != 0 {
		a.prewarmLockedArena(s, s.QueueID, "")
		return
	}
	a.goSafe("arena.locked-prewarm", func() {
		read, stop := context.WithTimeout(ctx, time.Second)
		defer stop()
		var flow lcuGameflowSession
		if c.RequestJSON(read, http.MethodGet, "/lol-gameflow/v1/session", nil, &flow) == nil {
			a.prewarmLockedArena(s, flow.GameData.Queue.ID, firstNonEmpty(flow.GameData.Queue.GameMode, flow.Map.GameMode))
		}
	})
}
func (a *app) arenaRecommendationReady(seed liveRecommendationSeed) {
	if !isArenaQueue(seed.QueueID, seed.GameMode) {
		return
	}
	t := &a.arenaRecommendationTimeline
	t.mu.Lock()
	defer t.mu.Unlock()
	clock, ok := t.entries[seed.ChampionID]
	if ok {
		clock.ready = time.Now()
		t.entries[seed.ChampionID] = clock
	}
}
func (a *app) arenaRecommendationTimingFor(id int64, d championDetailResponse) *arenaRecommendationTiming {
	t := &a.arenaRecommendationTimeline
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.inProgress.IsZero() {
		return nil
	}
	clock := t.entries[id]
	relative := func(v time.Time) *int64 {
		if v.IsZero() {
			return nil
		}
		ms := v.Sub(t.inProgress).Milliseconds()
		return &ms
	}
	return &arenaRecommendationTiming{LockedAt: relative(clock.locked), PrewarmStartedAt: relative(clock.started), PrewarmReadyAt: relative(clock.ready), InProgressAt: 0, RenderedAt: time.Since(t.inProgress).Milliseconds(), Source: d.Source, CacheHit: d.ArenaCacheHit, Hedged: d.ArenaHedged}
}

func (a *app) noteArenaRecommendationInProgress() {
	t := &a.arenaRecommendationTimeline
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.inProgress.IsZero() {
		t.inProgress = time.Now()
	}
}
func (a *app) arenaRecommendationOrigin() int64 {
	t := &a.arenaRecommendationTimeline
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.inProgress.IsZero() {
		return 0
	}
	return t.inProgress.UnixMilli()
}
