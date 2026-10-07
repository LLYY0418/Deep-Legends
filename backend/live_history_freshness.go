package main

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"sync"
	"time"
)

type liveHistoryFreshnessScope struct {
	mu              sync.Mutex
	client          *LCUClient
	gameID, queueID int64
	records         map[string]liveHistoryRecord
	previousGameID  int64
}

type liveHistoryRecord struct {
	latestGameID int64
	count        int
}
type liveHistoryRunGames struct {
	client                                       *LCUClient
	lastID, lastQueue, previousID, previousQueue int64
}

// Retained across end/lobby phases, scoped to this running client's session.
func (a *app) observeLiveHistoryGame(client *LCUClient, phase string, gameID, queueID int64) {
	a.liveHistoryFreshnessMu.Lock()
	defer a.liveHistoryFreshnessMu.Unlock()
	games := &a.liveHistoryRunGames
	if games.client != client {
		*games = liveHistoryRunGames{client: client}
	}
	if phase != "InProgress" || gameID <= 0 || queueID <= 0 || games.lastID == gameID {
		return
	}
	games.previousID, games.previousQueue = games.lastID, games.lastQueue
	games.lastID, games.lastQueue = gameID, queueID
}

// Read-only after a loader publishes its result. Kept with the 45-second cache
// and flight so diagnostics compare the same windows used for the displayed row.
type liveHistoryEvidence struct {
	LCU, SGP      []gameplayMatch
	SGPRequested  bool
	SGPOK         bool
	QueueFiltered bool
	PagesRead     int
	StopReason    string
	SGPMS, LCUMS  int64
	LCURequested  bool
}

func (a *app) clearLiveHistoryFreshness() {
	a.liveHistoryFreshnessMu.Lock()
	a.liveHistoryFreshness = nil
	a.liveHistoryFreshnessMu.Unlock()
}
func (a *app) liveHistoryFreshnessForGame(client *LCUClient, gameID, queueID int64) *liveHistoryFreshnessScope {
	a.liveHistoryFreshnessMu.Lock()
	defer a.liveHistoryFreshnessMu.Unlock()
	scope := a.liveHistoryFreshness
	if scope == nil || scope.client != client || scope.gameID != gameID || scope.queueID != queueID {
		scope = &liveHistoryFreshnessScope{client: client, gameID: gameID, queueID: queueID, records: map[string]liveHistoryRecord{}}
		games := a.liveHistoryRunGames
		if games.client == client {
			previousID, previousQueue := games.lastID, games.lastQueue
			if previousID == gameID {
				previousID, previousQueue = games.previousID, games.previousQueue
			}
			if previousID > 0 && previousID != gameID && previousQueue == queueID {
				scope.previousGameID = previousID
			}
		}
		a.liveHistoryFreshness = scope
	}
	return scope
}

func liveHistoryCacheKey(scope *liveHistoryFreshnessScope, playerRef string, isCurrent bool) string {
	return fmt.Sprintf("live-history:%p:%d:%d:%s:%t", scope.client, scope.gameID, scope.queueID, playerRef, isCurrent)
}

func (a *app) livePlayerMatchesForGame(ctx context.Context, client *LCUClient, reference gameplayReference, playerRef string, isCurrent bool, names map[int64]string, scope *liveHistoryFreshnessScope, team int64, slot int) livePlayerMatchesResult {
	if scope == nil {
		return a.livePlayerMatches(ctx, client, reference, playerRef, isCurrent, names)
	}
	result := a.cachedLivePlayerMatches(ctx, liveHistoryCacheKey(scope, playerRef, isCurrent), func(loadCtx context.Context) livePlayerMatchesResult {
		return a.loadLivePlayerMatchesForPrevious(loadCtx, client, reference, playerRef, isCurrent, names, scope.queueID, scope.previousGameID)
	})
	if slot >= 0 {
		a.recordLiveHistoryFreshness(result, playerRef, isCurrent, scope, team, slot)
	}
	return result
}

func (a *app) recordLiveHistoryFreshness(result livePlayerMatchesResult, playerRef string, isCurrent bool, scope *liveHistoryFreshnessScope, team int64, slot int) {
	if scope == nil {
		return
	}
	shown := recentLiveMatchesForPlayer(result.Matches, playerRef, scope.queueID, time.Now())
	var latest int64
	if len(shown) > 0 {
		latest = shown[0].GameID
	}
	scope.mu.Lock()
	key := playerRef + "\x00" + strconv.FormatBool(isCurrent)
	record := scope.records[key]
	if record.count >= 6 || record.count > 0 && record.latestGameID == latest {
		scope.mu.Unlock()
		return
	}
	record.count++
	record.latestGameID = latest
	scope.records[key] = record
	scope.mu.Unlock()
	event := liveHistoryFreshnessDiagnostic(result, playerRef, scope.queueID, team, slot, isCurrent, time.Now())
	event["load_index"] = record.count
	if isCurrent && scope.previousGameID > 0 {
		lcu, sgp := result.Matches, []gameplayMatch(nil)
		if result.Evidence != nil {
			lcu, sgp = result.Evidence.LCU, result.Evidence.SGP
		}
		event["prev_game_in_lcu"] = liveHistoryContainsGame(lcu, scope.previousGameID)
		if result.Evidence != nil && !result.Evidence.LCURequested {
			event["prev_game_in_lcu"] = nil
		}
		event["prev_game_in_sgp"] = liveHistoryContainsGame(sgp, scope.previousGameID)
		event["prev_game_shown"] = liveHistoryContainsGame(shown, scope.previousGameID)
	}
	a.recordDiagnostic(event)
}

func liveHistoryContainsGame(matches []gameplayMatch, gameID int64) bool {
	for _, match := range matches {
		if match.GameID == gameID {
			return true
		}
	}
	return false
}

// Slots follow the default classic details layout: known lanes first, and self
// first for tied/unknown lanes. Compute after snapshot/Live Client corrections.
func liveHistoryRosterSlots(players []gameplayLivePlayer, aligned bool) []int {
	slots := make([]int, len(players))
	teams := map[int64][]int{}
	for i, player := range players {
		teams[player.TeamID] = append(teams[player.TeamID], i)
	}
	positions := map[string]int{"top": 1, "jungle": 2, "middle": 3, "bottom": 4, "utility": 5}
	for _, indices := range teams {
		sort.SliceStable(indices, func(i, j int) bool {
			left, right := players[indices[i]], players[indices[j]]
			if aligned {
				l, r := positions[left.Position], positions[right.Position]
				if l == 0 {
					l = 6
				}
				if r == 0 {
					r = 6
				}
				if l != r {
					return l < r
				}
			}
			return left.IsCurrent && !right.IsCurrent
		})
		for slot, i := range indices {
			slots[i] = slot
		}
	}
	return slots
}

// SUMMARY only. useCache=false deliberately avoids both reading and writing the
// shared five-minute SGP page cache; the live roster has its own 45-second cache.
func liveHistoryQueue(queues []int64) int64 {
	if len(queues) > 0 {
		return queues[0]
	}
	return 0
}

func recentLiveMatchesForPlayer(matches []gameplayMatch, playerRef string, queueID int64, now time.Time) []gameplayMatch {
	return recentMatchesForPlayer(matches, playerRef, 10, queueID)
}

func liveHistoryPageStop(matches []gameplayMatch, playerRef string, queueID int64, now time.Time, more bool, pages int) string {
	if len(recentLiveMatchesForPlayer(matches, playerRef, queueID, now)) >= 10 {
		return "enough"
	}
	if !more {
		return "exhausted"
	}
	if pages >= 4 {
		return "page_limit"
	}
	return ""
}

func (a *app) loadLiveSGPMatches(ctx context.Context, client *LCUClient, reference gameplayReference, playerRef string, names map[int64]string, queues ...int64) (livePlayerMatchesResult, bool) {
	if a.sgp == nil {
		return livePlayerMatchesResult{State: "unavailable"}, false
	}
	serverID, _, ok := a.sgp.available(client)
	if !ok {
		return livePlayerMatchesResult{State: "unavailable"}, false
	}
	if reference.ServerID != "" {
		serverID = reference.ServerID
	}
	queueID := liveHistoryQueue(queues)
	filter := "all"
	if definition, known := supportedQueueDefinition(queueID); known {
		filter = definition.Filter
	}
	evidence := &liveHistoryEvidence{StopReason: "exhausted"}
	matches := []gameplayMatch{}
	now, offset := time.Now(), 0
	pageLimit := 4
	definition, known := supportedQueueDefinition(queueID)
	if queueID == 0 || queueID == 3140 || queueID == 3110 || known && (definition.ModeGroup == "custom" || definition.ModeGroup == "bots" || definition.ModeGroup == "doombots") {
		pageLimit = 1
	}
	for page := 0; page < pageLimit; page++ {
		count, pageFilter := 30, "all"
		if page == 0 && queueID > 0 {
			count, pageFilter = 10, filter
		}
		infos, consumed, more, resolution, err := a.loadSGPMatchHistoryPage(ctx, client, serverID, playerRef, offset, count, pageFilter, sgpHistoryPageOptions{NoCache: true, QueueID: queueID, FallbackCount: 30})
		evidence.PagesRead++
		if page == 0 {
			evidence.QueueFiltered = resolution.ServerFiltered
		}
		for _, info := range infos {
			a.checkArenaGroupTruth(client, reference.ServerID, info)
			matches = append(matches, convertRiotMatchInfo(info, playerRef, names, nil, "", reference.ServerID))
		}
		if err != nil && len(matches) == 0 {
			return livePlayerMatchesResult{State: "failed", Evidence: evidence}, false
		}
		evidence.StopReason = liveHistoryPageStop(matches, playerRef, queueID, now, more, evidence.PagesRead)
		if evidence.StopReason == "" && evidence.PagesRead >= pageLimit {
			evidence.StopReason = "page_limit"
		}
		if evidence.QueueFiltered || queueID <= 0 || err != nil || consumed <= 0 || evidence.StopReason != "" {
			if evidence.StopReason == "" {
				evidence.StopReason = "exhausted"
			}
			break
		}
		offset += consumed
	}
	state := "ok"
	if len(matches) == 0 {
		state = "empty"
	}
	return livePlayerMatchesResult{Matches: matches, State: state, Source: "sgp", Evidence: evidence}, true
}

// Both adapters directly preserve the numeric LCU gameId / SGP json.gameId.
// Never parse a region-prefixed metadata matchId as a gameId. Only positive IDs
// are deduplicated; missing IDs cannot safely identify the same game.
func mergeLivePlayerMatches(lcu, sgp []gameplayMatch, playerRef string) []gameplayMatch {
	merged := make([]gameplayMatch, 0, len(lcu)+len(sgp))
	byID := map[int64]int{}
	for _, window := range [][]gameplayMatch{lcu, sgp} {
		for _, match := range window {
			if index, exists := byID[match.GameID]; match.GameID > 0 && exists {
				if liveMatchCompleteness(match, playerRef) > liveMatchCompleteness(merged[index], playerRef) {
					merged[index] = match
				}
				continue
			}
			if match.GameID > 0 {
				byID[match.GameID] = len(merged)
			}
			merged = append(merged, match)
		}
	}
	sort.SliceStable(merged, func(i, j int) bool { return merged[i].CreatedAt > merged[j].CreatedAt })
	return merged
}

func liveMatchCompleteness(match gameplayMatch, playerRef string) int {
	// A usable subject/result/queue outweigh optional metadata and roster size.
	quality := min(len(match.Participants), 15)
	if recentMatchSubject(match, playerRef) != nil {
		quality += 256
	}
	if match.Result == "win" || match.Result == "loss" {
		quality += 128
	}
	if match.QueueID > 0 {
		quality += 64
	}
	if match.CreatedAt > 0 {
		quality += 32
	}
	if match.Duration > 0 {
		quality += 16
	}
	return quality
}

func liveNewestTimestamp(matches []gameplayMatch, queueID int64) int64 {
	var newest int64
	for _, match := range matches {
		if queueID > 0 && match.QueueID != queueID {
			continue
		}
		if match.CreatedAt > newest {
			newest = match.CreatedAt
		}
	}
	return newest
}
func liveNewestAgeMinutes(matches []gameplayMatch, queueID int64, now time.Time) *float64 {
	newest := liveNewestTimestamp(matches, queueID)
	if newest <= 0 {
		return nil
	}
	age := round2(max(0, float64(now.UnixMilli()-newest)/60000))
	return &age
}

// Derive the diagnostic header count through the same aggregate used by the row.
func liveHistoryHeaderGames(matches []gameplayMatch, playerRef string, queueID int64) int {
	stats, _ := liveRecentPlayerStats(matches, playerRef, queueID)
	return stats.Games
}
func liveOldestAgeDays(matches []gameplayMatch, now time.Time) *float64 {
	var oldest int64
	for _, match := range matches {
		if match.CreatedAt > 0 && (oldest == 0 || match.CreatedAt < oldest) {
			oldest = match.CreatedAt
		}
	}
	if oldest == 0 {
		return nil
	}
	age := round2(max(0, float64(now.UnixMilli()-oldest)/float64(24*time.Hour/time.Millisecond)))
	return &age
}
func liveHistoryFreshnessDiagnostic(result livePlayerMatchesResult, playerRef string, queueID, team int64, slot int, isCurrent bool, now time.Time) map[string]any {
	lcu := result.Matches
	if result.Evidence != nil {
		lcu = result.Evidence.LCU
	}
	queueGames, unmatched, skipped := 0, 0, 0
	for _, match := range lcu {
		if queueID > 0 && match.QueueID != queueID {
			continue
		}
		queueGames++
		// Independent counts: a missing subject can also cause an unknown result.
		if recentMatchSubject(match, playerRef) == nil {
			unmatched++
		}
		if match.Result != "win" && match.Result != "loss" {
			skipped++
		}
	}
	shown := recentLiveMatchesForPlayer(result.Matches, playerRef, queueID, now)
	event := map[string]any{"event": "live_history_freshness", "team": team, "slot": slot, "is_current": isCurrent,
		"source": result.Source, "window_games": len(lcu), "queue_games": queueGames,
		"newest_any_age_min": liveNewestAgeMinutes(lcu, 0, now), "newest_queue_age_min": liveNewestAgeMinutes(lcu, queueID, now),
		"lcu_newest_queue_age_min": liveNewestAgeMinutes(lcu, queueID, now), "subject_unmatched": unmatched, "remake_skipped": skipped,
		"shown_newest_age_min": liveNewestAgeMinutes(shown, 0, now), "oldest_shown_age_days": liveOldestAgeDays(shown, now), "header_games": liveHistoryHeaderGames(result.Matches, playerRef, queueID), "shown_count": len(shown), "queue_filtered": false, "pages_read": 0, "stop_reason": liveHistoryPageStop(result.Matches, playerRef, queueID, now, false, 0)}
	if evidence := result.Evidence; evidence != nil && evidence.SGPRequested {
		event["queue_filtered"], event["pages_read"] = evidence.QueueFiltered, evidence.PagesRead
		if len(shown) >= 10 {
			event["stop_reason"] = "enough"
		} else if evidence.StopReason != "" {
			event["stop_reason"] = evidence.StopReason
		}
		if !evidence.SGPOK {
			event["sample_failed"] = true
		} else {
			event["sgp_newest_any_age_min"] = liveNewestAgeMinutes(evidence.SGP, 0, now)
			event["sgp_newest_queue_age_min"] = liveNewestAgeMinutes(evidence.SGP, queueID, now)
			event["missing_newer_any"] = liveMissingNewerGames(lcu, evidence.SGP, 0)
			event["missing_newer_in_queue"] = liveMissingNewerGames(lcu, evidence.SGP, queueID)
		}
	}
	return event
}
func liveMissingNewerGames(lcu, sgp []gameplayMatch, queueID int64) any {
	newest := liveNewestTimestamp(lcu, queueID)
	// Without a dated LCU baseline, newer-than-LCU cannot be established.
	if newest <= 0 {
		return nil
	}
	known := map[int64]bool{}
	for _, match := range lcu {
		known[match.GameID] = true
	}
	newer := map[int64]bool{}
	for _, match := range sgp {
		if queueID > 0 && match.QueueID != queueID {
			continue
		}
		if match.GameID > 0 && match.CreatedAt > newest && !known[match.GameID] {
			newer[match.GameID] = true
		}
	}
	return len(newer)
}
