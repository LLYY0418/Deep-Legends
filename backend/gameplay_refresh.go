package main

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"
)

type gameplayFlowState struct {
	mu     sync.Mutex
	client *LCUClient
	phase  string
	ended  bool
	ctx    context.Context
	cancel context.CancelFunc
}

func (a *app) primeGameplayState(ctx context.Context, client *LCUClient) {
	probeCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	var phase string
	if client.RequestJSON(probeCtx, http.MethodGet, "/lol-gameflow/v1/gameflow-phase", nil, &phase) == nil {
		a.observeGameplayPhase(ctx, client, phase)
	}
}

func isEndOfGamePhase(phase string) bool {
	return phase == "WaitingForStats" || phase == "PreEndOfGame" || phase == "EndOfGame"
}

// One lifecycle per client/game. End phases may be duplicated or omitted by LCU.
func (a *app) observeGameplayPhase(ctx context.Context, client *LCUClient, phase string) {
	if phase != "GameStart" && phase != "InProgress" && phase != "Reconnect" {
		a.stopMayhemSamplerForClient("gameflow-left", client)
	}
	f := &a.gameplayFlow
	f.mu.Lock()
	changed := f.client != client || f.phase != phase
	if changed {
		if f.cancel != nil {
			f.cancel()
		}
		f.ctx, f.cancel = context.WithCancel(ctx)
	}
	phaseContext := f.ctx
	if f.client != client || phase == "ChampSelect" || phase == "InProgress" {
		f.ended = false
	}
	f.client, f.phase = client, phase
	ended := isEndOfGamePhase(phase) && !f.ended
	if ended {
		f.ended = true
	}
	f.mu.Unlock()
	if ended {
		a.invalidateOverviewPlayer(a.currentPlayerRef())
	}
	if !changed {
		return
	}
	a.observeArenaGroupingPhase(client, phase)
	a.liveSnapshots.invalidate()
	if isEndOfGamePhase(phase) {
		go func() {
			truthCtx, cancel := context.WithTimeout(phaseContext, 35*time.Second)
			defer cancel()
			a.finishArenaGroupTruth(truthCtx, client)
		}()
	}
	if phase != "ChampSelect" && phase != "GameStart" && phase != "InProgress" && phase != "Reconnect" {
		return
	}
	a.mu.RLock()
	current, valid := a.summoner, a.lcu == client
	a.mu.RUnlock()
	if !valid {
		return
	}
	go func() {
		// Only phase transitions prewarm; one shared flight, bounded by session
		// lifetime and a hard budget. No periodic upstream polling here.
		warmCtx, cancel := context.WithTimeout(phaseContext, 40*time.Second)
		defer cancel()
		started := time.Now()
		response := a.cachedGameplayLive(warmCtx, client, current, phase, true)
		a.recordDiagnostic(map[string]any{"event": "live_prewarm", "phase": phase, "players": len(response.Players), "duration_ms": time.Since(started).Milliseconds(), "error_kind": diagnosticErrorKind(warmCtx.Err())})
	}()
}

type liveSnapshotFlight struct {
	done     chan struct{}
	response gameplayLiveResponse
}
type liveSnapshotCache struct {
	mu              sync.Mutex
	client          *LCUClient
	identity, phase string
	at              time.Time
	warmPending     bool
	response        gameplayLiveResponse
	flight          *liveSnapshotFlight
	generation      uint64
	revision        uint64
	cancel          context.CancelFunc
}

func (c *liveSnapshotCache) invalidate() {
	c.mu.Lock()
	c.at = time.Time{}
	c.warmPending = false
	if c.cancel != nil {
		c.cancel()
		c.cancel = nil
	}
	c.generation++
	c.revision++
	c.flight = nil
	c.mu.Unlock()
}

func (a *app) cachedGameplayLive(ctx context.Context, client *LCUClient, current Summoner, phase string, warming ...bool) gameplayLiveResponse {
	c := &a.liveSnapshots
	isWarming := len(warming) > 0 && warming[0]
	c.mu.Lock()
	if ctx.Err() != nil {
		c.mu.Unlock()
		return gameplayLiveResponse{Phase: phase}
	}
	if c.client != client || c.identity != current.PUUID || c.phase != phase {
		if c.cancel != nil {
			c.cancel()
			c.cancel = nil
		}
		c.client, c.identity, c.phase = client, current.PUUID, phase
		c.at, c.flight = time.Time{}, nil
		c.warmPending = false
		c.generation++
		c.revision++
	}
	ttl := 20 * time.Second
	if phase == "ChampSelect" || phase == "GameStart" || c.response.ArenaGroupingRetryable {
		ttl = 3 * time.Second
	}
	// Complete in-game data is immutable for this game. Incomplete snapshots
	// still cache briefly, so events do not repeatedly assemble the roster.
	wholeGame := (phase == "InProgress" || phase == "Reconnect") && c.response.GameID != 0 && gameplayLiveSnapshotComplete(c.response)
	if !c.at.IsZero() && (wholeGame || time.Since(c.at) < ttl) {
		response := c.response
		// A clock tick can cover multiple writes, so at alone cannot detect replacement.
		generation, revision := c.generation, c.revision
		c.mu.Unlock()
		// LCU can omit intermediate phases. Verify the game identity before
		// consuming a warm response, even when the phase string is unchanged.
		var game lcuGameflowSession
		valid := false
		if phase == "ChampSelect" {
			var session lcuChampSelectSession
			valid = client.RequestJSON(ctx, http.MethodGet, champSelectAPI+"/session", nil, &session) == nil && session.GameID == response.GameID
		} else {
			valid = client.RequestJSON(ctx, http.MethodGet, "/lol-gameflow/v1/session", nil, &game) == nil && game.GameData.GameID == response.GameID
		}
		c.mu.Lock()
		if ctx.Err() != nil || c.generation != generation {
			c.mu.Unlock()
			return gameplayLiveResponse{Phase: phase}
		}
		if c.revision != revision {
			c.mu.Unlock()
			return a.cachedGameplayLive(ctx, client, current, phase)
		}
		if valid && c.warmPending && !isWarming {
			c.warmPending, c.at = false, time.Now()
			c.revision++
		} else if !valid {
			c.at, c.warmPending = time.Time{}, false
			c.revision++
		}
		c.mu.Unlock()
		if valid {
			return response
		}
		return a.cachedGameplayLive(ctx, client, current, phase)
	}
	if flight := c.flight; flight != nil {
		generation := c.generation
		c.mu.Unlock()
		select {
		case <-flight.done:
			c.mu.Lock()
			valid := c.generation == generation && ctx.Err() == nil
			c.mu.Unlock()
			if !valid {
				return gameplayLiveResponse{Phase: phase}
			}
			return flight.response
		case <-ctx.Done():
			return gameplayLiveResponse{Phase: phase}
		}
	}
	flight := &liveSnapshotFlight{done: make(chan struct{})}
	c.flight = flight
	generation := c.generation
	loadCtx, cancel := context.WithCancel(ctx)
	c.cancel = cancel
	c.mu.Unlock()
	defer cancel()
	response := a.loadGameplayLive(loadCtx, client, current, phase)
	c.mu.Lock()
	if c.generation == generation && c.flight == flight {
		c.flight = nil
		if loadCtx.Err() == nil {
			c.at, c.response = time.Now(), response
			c.warmPending = isWarming
			c.revision++
		}
	}
	if loadCtx.Err() != nil || c.generation != generation {
		response = gameplayLiveResponse{Phase: phase}
	}
	flight.response = response
	close(flight.done)
	c.mu.Unlock()
	return response
}

// Exact identity component, including paging/filter variants; detach matching
// flights so a late pre-game response cannot repopulate an invalidated cache.
func (a *app) invalidateOverviewPlayer(playerRef string) {
	if playerRef == "" || a.overviewQueries == nil {
		return
	}
	c := a.overviewQueries
	c.mu.Lock()
	defer c.mu.Unlock()
	matches := func(key string) bool {
		_, body, _ := strings.Cut(key, ":")
		parts := strings.Split(body, "|")
		return (len(parts) > 1 && parts[1] == playerRef) || (strings.HasPrefix(key, "riot:") && parts[0] == playerRef)
	}
	for key, element := range c.entries {
		if matches(key) {
			c.removeElementLocked(element)
		}
	}
	for key := range c.flights {
		if matches(key) {
			delete(c.flights, key)
		}
	}
}

func (p *sgpProvider) invalidatePlayerHistory(serverID, playerRef string) {
	if playerRef == "" {
		return
	}
	prefix := sourceScopedKey(dataSourceSGP, strings.ToUpper(strings.TrimSpace(serverID))+"|"+playerRef+"|")
	p.mu.Lock()
	defer p.mu.Unlock()
	p.historyGeneration++
	for key, entry := range p.historyCache {
		if strings.HasPrefix(key, prefix) {
			p.historyBytes -= entry.bytes
			delete(p.historyCache, key)
		}
	}
}

// Distinguish browser aborts from a backend soft budget in the next diagnostic export.
type overviewRequestContextKey struct{}

func gameplayCancellationScope(ctx context.Context) string {
	if ctx.Err() == nil {
		return "transport"
	}
	if request, ok := ctx.Value(overviewRequestContextKey{}).(context.Context); ok && request.Err() != nil {
		return "client-canceled"
	}
	if ctx.Err() == context.DeadlineExceeded {
		return "budget-timeout"
	}
	if ctx.Value(overviewRequestContextKey{}) != nil {
		return "loader-canceled"
	}
	return "request-canceled"
}

func gameplayLiveSnapshotComplete(response gameplayLiveResponse) bool {
	if !response.Available || len(response.Players) == 0 {
		return false
	}
	for _, p := range response.Players {
		if p.HistoryState == "pending" || p.HistoryState == "failed" {
			return false
		}
	}
	return !isArenaQueue(response.QueueID, response.GameMode) || response.ArenaGrouped || response.ArenaGroupingUnavailable && !response.ArenaGroupingRetryable
}
