package main

import (
	"context"
	"net/http"
	"sync"
	"time"
)

type postGameRevealState struct {
	mu            sync.Mutex
	client        *LCUClient
	snapshot      gameplayLiveResponse
	ctx           context.Context
	cancel        context.CancelFunc
	generation    uint64
	started       bool
	running       bool
	attempt       int
	wait          func(context.Context, time.Duration) bool
	now           func() time.Time
	expiresAt     time.Time
	expiryTimer   *time.Timer
	expiredClient *LCUClient
	expiredGameID int64
}

const postGameRevealRetention = 2 * time.Minute

// Called with mu held, so cancellation cannot clear a newer generation.
func (s *postGameRevealState) clearLocked() {
	if s.cancel != nil {
		s.cancel()
	}
	if s.expiryTimer != nil {
		s.expiryTimer.Stop()
	}
	s.expiryTimer = nil
	s.expiresAt = time.Time{}
	s.generation++
	s.client = nil
	s.snapshot = gameplayLiveResponse{}
	s.running, s.started = false, false
	s.attempt = 0
}

func (a *app) stopPostGameRevealForClient(client *LCUClient, reason string) {
	s := &a.postGameReveal
	s.mu.Lock()
	if client != nil && s.client != client {
		s.mu.Unlock()
		return
	}
	snapshot, attempt, running := s.snapshot, s.attempt, s.running
	s.clearLocked()
	if reason != "expired" {
		s.expiredClient, s.expiredGameID = nil, 0
	}
	s.mu.Unlock()
	if snapshot.GameID > 0 && (running || reason == "left_end_of_game") {
		a.postGameRevealDiagnostic(snapshot, attempt, 0, reason)
	}
}

func (a *app) expirePostGameReveal(client *LCUClient, generation uint64) {
	s := &a.postGameReveal
	s.mu.Lock()
	now := s.now
	if now == nil {
		now = time.Now
	}
	if s.client != client || s.generation != generation || s.expiresAt.IsZero() || now().Before(s.expiresAt) {
		s.mu.Unlock()
		return
	}
	snapshot, attempt := s.snapshot, s.attempt
	// Repeated EndOfGame observations must not resurrect the same expired roster.
	s.expiredClient, s.expiredGameID = s.client, snapshot.GameID
	s.clearLocked()
	s.mu.Unlock()
	a.postGameRevealDiagnostic(snapshot, attempt, 0, "expired")
}

func clonePostGameSnapshot(value gameplayLiveResponse) gameplayLiveResponse {
	value.Players = append([]gameplayLivePlayer(nil), value.Players...)
	return value
}
func hiddenRosterCount(value gameplayLiveResponse) int {
	count := 0
	for _, p := range value.Players {
		if p.Hidden {
			count++
		}
	}
	return count
}
func (a *app) postGameRevealDiagnostic(snapshot gameplayLiveResponse, attempt, revealed int, reason string) {
	a.recordDiagnostic(map[string]any{"event": "live_roster_post_game_reveal", "game_id": snapshot.GameID, "queue_id": snapshot.QueueID, "attempt": attempt, "hidden_count": hiddenRosterCount(snapshot), "revealed": revealed, "reason": reason})
}
func (a *app) stopPostGameReveal() {
	a.stopPostGameRevealForClient(nil, "stopped")
}
func (a *app) observePostGameReveal(parent context.Context, client *LCUClient, phase string) {
	if !isEndOfGamePhase(phase) {
		reason := "left_end_of_game"
		if phase == "ChampSelect" || phase == "GameStart" || phase == "InProgress" || phase == "Reconnect" {
			reason = "stopped"
		}
		a.stopPostGameRevealForClient(nil, reason)
		return
	}
	s := &a.postGameReveal
	s.mu.Lock()
	if s.client != nil && s.client != client {
		s.mu.Unlock()
		a.stopPostGameReveal()
		s.mu.Lock()
	}
	if s.client == nil {
		a.liveSnapshots.mu.Lock()
		snapshot := clonePostGameSnapshot(a.liveSnapshots.response)
		owner := a.liveSnapshots.client
		a.liveSnapshots.mu.Unlock()
		if owner != client || snapshot.GameID <= 0 || hiddenRosterCount(snapshot) == 0 {
			s.mu.Unlock()
			return
		}
		if s.expiredClient == client && s.expiredGameID == snapshot.GameID {
			s.mu.Unlock()
			return
		}
		s.client = client
		s.snapshot = snapshot
		s.generation++
		s.ctx, s.cancel = context.WithCancel(parent)
	}
	if s.started || phase == "WaitingForStats" {
		s.mu.Unlock()
		return
	}
	s.started = true
	s.running = true
	ctx, generation, snapshot := s.ctx, s.generation, clonePostGameSnapshot(s.snapshot)
	wait, now := s.wait, s.now
	if wait == nil {
		wait = gameSettingsWait
	}
	if now == nil {
		now = time.Now
	}
	s.mu.Unlock()
	a.goSafe("live-post-game-reveal", func() { a.runPostGameReveal(ctx, client, generation, snapshot, wait, now) })
}
func (a *app) postGameSnapshot(client *LCUClient, phase string, expectedGameID int64) (gameplayLiveResponse, bool) {
	if !isEndOfGamePhase(phase) {
		reason := "left_end_of_game"
		if phase == "ChampSelect" || phase == "GameStart" || phase == "InProgress" || phase == "Reconnect" {
			reason = "stopped"
		}
		a.stopPostGameRevealForClient(client, reason)
		return gameplayLiveResponse{}, false
	}
	s := &a.postGameReveal
	s.mu.Lock()
	generation := s.generation
	s.mu.Unlock()
	a.expirePostGameReveal(client, generation)
	s.mu.Lock()
	if s.client != client || s.snapshot.GameID <= 0 {
		s.mu.Unlock()
		return gameplayLiveResponse{}, false
	}
	if expectedGameID > 0 && s.snapshot.GameID != expectedGameID {
		s.mu.Unlock()
		a.stopPostGameReveal()
		return gameplayLiveResponse{}, false
	}
	snapshot := clonePostGameSnapshot(s.snapshot)
	s.mu.Unlock()
	snapshot.Phase = phase
	return snapshot, true
}
func (a *app) finishStoppedPostGameReveal(client *LCUClient, generation uint64) {
	s := &a.postGameReveal
	s.mu.Lock()
	if s.client != client || s.generation != generation || !s.running {
		s.mu.Unlock()
		return
	}
	snapshot, attempt := s.snapshot, s.attempt
	s.running = false
	s.mu.Unlock()
	a.postGameRevealDiagnostic(snapshot, attempt, 0, "stopped")
}
func (a *app) runPostGameReveal(ctx context.Context, client *LCUClient, generation uint64, snapshot gameplayLiveResponse, wait func(context.Context, time.Duration) bool, now func() time.Time) {
	s := &a.postGameReveal
	current := func() bool {
		s.mu.Lock()
		valid := s.client == client && s.generation == generation
		s.mu.Unlock()
		a.mu.RLock()
		connected := a.lcu == client
		a.mu.RUnlock()
		return valid && connected && ctx.Err() == nil
	}
	finish := func() {
		s.mu.Lock()
		if s.client == client && s.generation == generation {
			s.running = false
			s.expiresAt = now().Add(postGameRevealRetention)
			if s.expiryTimer != nil {
				s.expiryTimer.Stop()
			}
			s.expiryTimer = time.AfterFunc(postGameRevealRetention, func() { a.expirePostGameReveal(client, generation) })
		}
		s.mu.Unlock()
	}
	defer func() {
		if !current() {
			a.finishStoppedPostGameReveal(client, generation)
		}
		finish()
	}()
	started := now()
	// Bind the retained snapshot to the actual game that just ended, before any
	// history lookup. Never resolve hidden identities during an active phase.
	probe, cancel := context.WithTimeout(ctx, 8*time.Second)
	var session lcuGameflowSession
	err := client.RequestJSON(probe, http.MethodGet, "/lol-gameflow/v1/session", nil, &session)
	cancel()
	if err != nil || session.GameData.GameID != snapshot.GameID || !current() {
		if err == nil && session.GameData.GameID != snapshot.GameID {
			a.stopPostGameReveal()
		} else {
			a.finishStoppedPostGameReveal(client, generation)
		}
		return
	}
	for index, deadline := range []time.Duration{10 * time.Second, 40 * time.Second, 120 * time.Second} {
		if !wait(ctx, max(time.Duration(0), deadline-now().Sub(started))) || !current() {
			return
		}
		attempt := index + 1
		s.mu.Lock()
		if s.generation == generation {
			s.attempt = attempt
		}
		s.mu.Unlock()
		attemptCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		var phase string
		if client.RequestJSON(attemptCtx, http.MethodGet, "/lol-gameflow/v1/gameflow-phase", nil, &phase) != nil {
			cancel()
			a.finishStoppedPostGameReveal(client, generation)
			return
		}
		if !current() {
			cancel()
			return
		}
		if !isEndOfGamePhase(phase) {
			cancel()
			a.observePostGameReveal(ctx, client, phase)
			return
		}
		// A reconnect or missed phase event may already expose another game while
		// the phase remains unchanged. Rebind every attempt before reading history.
		var latest lcuGameflowSession
		if client.RequestJSON(attemptCtx, http.MethodGet, "/lol-gameflow/v1/session", nil, &latest) == nil && latest.GameData.GameID > 0 && latest.GameData.GameID != snapshot.GameID {
			cancel()
			a.stopPostGameReveal()
			return
		}
		a.mu.RLock()
		self := a.summoner
		a.mu.RUnlock()
		reference := gameplayReferenceFromSummoner(self)
		reference.ServerID = clientTencentServerID(client)
		// Identical loader/cache and official fallback chain to expanded recent
		// matches. Invalidate only this account's stale page, as overview force does.
		if a.sgp != nil {
			a.sgp.invalidatePlayerHistory(reference.ServerID, self.PUUID)
		}
		matches, _, _ := a.loadDetailedMatches(attemptCtx, client, reference, self.PUUID, true, 0, 20, "all", nil, nil)
		var match *gameplayMatch
		for i := range matches {
			if matches[i].GameID == snapshot.GameID {
				match = &matches[i]
				break
			}
		}
		if !current() {
			cancel()
			return
		}
		if match == nil || len(match.Participants) != 10 {
			cancel()
			a.postGameRevealDiagnostic(snapshot, attempt, 0, "match-not-ready")
			continue
		}
		next, revealed, reason := a.revealPostGamePlayers(attemptCtx, client, snapshot, *match, current)
		cancel()
		if !current() {
			return
		}
		s.mu.Lock()
		if s.client == client && s.generation == generation {
			s.snapshot = clonePostGameSnapshot(next)
		}
		s.mu.Unlock()
		a.postGameRevealDiagnostic(snapshot, attempt, revealed, reason)
		if revealed > 0 {
			a.broadcastEvent("live-post-game-reveal")
		}
		return
	}
}
func (a *app) revealPostGamePlayers(ctx context.Context, client *LCUClient, snapshot gameplayLiveResponse, match gameplayMatch, valid func() bool) (gameplayLiveResponse, int, string) {
	next := clonePostGameSnapshot(snapshot)
	used := map[string]bool{}
	for _, player := range snapshot.Players {
		ref := player.reference.PlayerRef
		if ref == "" {
			ref = player.PlayerRef
		}
		if ref != "" {
			used[ref] = true
		}
	}
	reason := "no-candidate"
	revealed := 0
	var names map[int64]string
	for i, hidden := range next.Players {
		if !hidden.Hidden || hidden.ChampionID <= 0 {
			continue
		}
		candidates := []gameplayParticipant{}
		for _, participant := range match.Participants {
			if participant.TeamID == hidden.TeamID && participant.ChampionID == hidden.ChampionID {
				candidates = append(candidates, participant)
			}
		}
		if len(candidates) > 1 {
			reason = "ambiguous"
			continue
		}
		if len(candidates) == 0 {
			continue
		}
		participant := candidates[0]
		ref := participant.reference.PlayerRef
		if ref == "" {
			ref = participant.PlayerRef
		}
		if participant.Hidden || !validPlayerReference(ref) || (participant.GameName == "" && participant.DisplayName == "") {
			reason = "participant-hidden"
			continue
		}
		if used[ref] {
			continue
		}
		if ctx.Err() != nil || !valid() {
			return snapshot, 0, "stopped"
		}
		reference := mergeGameplayReferences(participant.reference, gameplayReference{PlayerRef: ref, GameName: participant.GameName, TagLine: participant.TagLine, DisplayName: participant.DisplayName, ProfileIconID: participant.ProfileIconID, ServerID: clientTencentServerID(client)})
		summoner := summonerFromGameplayReference(reference)
		if loaded, capability := loadGameplaySummonerContext(ctx, client, reference); capability.State == capabilityAvailable {
			summoner = mergeSummonerIdentity(loaded, summoner)
			reference = mergeGameplayReferences(gameplayReferenceFromSummoner(summoner), reference)
		}
		player := hidden
		player.gameplayPlayer = gameplayPlayer{PlayerRef: ref, GameName: summoner.GameName, TagLine: summoner.TagLine, DisplayName: gameplayDisplayName(summoner), ProfileIconID: summoner.ProfileIconID, SummonerLevel: summoner.SummonerLevel, PrivateHistory: summoner.Privacy == "PRIVATE", IsCurrent: hidden.IsCurrent, Autofill: hidden.Autofill, reference: reference}
		player.Hidden = false
		player.IdentityUnresolved = false
		// Preserve the ordinary mode behavior: ARAM cards do not show ranked tiers.
		group := queueModeGroupFor(snapshot.QueueID, snapshot.GameMode, snapshot.MapID)
		if !isARAMFamilyGameMode(snapshot.GameMode) && group != "aram" && group != "hextech-aram" && group != "hextech-classic" {
			ranks := a.playerRankScore(ctx, client, ref, false, reference.ServerID, summoner.Privacy).ranks
			for j := range ranks {
				if snapshot.QueueID == 440 && ranks[j].QueueType == "RANKED_FLEX_SR" || snapshot.QueueID != 440 && ranks[j].QueueType == "RANKED_SOLO_5x5" {
					rank := ranks[j]
					player.Rank = &rank
					break
				}
			}
		}
		if names == nil {
			names = a.overviewChampionNames(ctx)
		}
		history := a.livePlayerMatchesForGame(ctx, client, reference, ref, false, names, a.liveHistoryFreshnessForGame(client, snapshot.GameID, snapshot.QueueID), hidden.TeamID, -1)
		player.ModeStats, player.RecentGames = liveRecentPlayerStats(history.Matches, ref, snapshot.QueueID)
		player.HistoryState = liveHistoryState(true, history)
		player.RecentPositions = liveRecentPositions(history.Matches, ref, snapshot.QueueID)
		if snapshot.QueueID == 420 || snapshot.QueueID == 440 {
			player.RecentRankedRecord = recentRankedRecord(player.RecentGames)
		}
		player.ProPlayer = a.matchProIdentity(a.proIdentitySnapshot(), "live", reference, snapshot.GameID)
		player.PlayerRef = a.registerGameplayReferenceDetails(reference)
		if ctx.Err() != nil || !valid() {
			return snapshot, 0, "stopped"
		}
		next.Players[i] = player
		used[ref] = true
		revealed++
	}
	if revealed > 0 {
		reason = "ok"
	}
	return next, revealed, reason
}
