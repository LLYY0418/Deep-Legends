package main

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// R263 P2.2：斗魂 30 局胜率与吃鸡率。
//
// 首屏仍用现有 10 局（loadLiveSGPMatches 首页 + 队列过滤），先出小卡；斗魂再在
// 同一玩家的 goroutine 里补 startIndex=10、count=20 的过滤页，完成后由最终响应
// 带回 30 局统计。补页结果按「对局 + 玩家」缓存，整局只下载一次。
const (
	arenaLiveStatsGames     = 30
	arenaLiveExtensionStart = 10
	arenaLiveExtensionCount = arenaLiveStatsGames - arenaLiveExtensionStart
)

type gameplayArenaRecord struct {
	Games   int `json:"games"`
	TopHalf int `json:"topHalf"`
	Top1    int `json:"top1"`
}

// liveArenaRecord uses the same win rule as arenaPlacementResult (top half of
// the subteams), matching LeagueAkari's isCherryPlacementWin, and counts
// placement 1 as 吃鸡. Games without a placement are skipped.
func liveArenaRecord(matches []gameplayMatch, playerRef string, queueID int64) *gameplayArenaRecord {
	stats := &gameplayArenaRecord{}
	for _, match := range recentMatchesForPlayer(matches, playerRef, arenaLiveStatsGames, queueID) {
		subject := recentMatchSubject(match, playerRef)
		if subject == nil || subject.Placement <= 0 {
			continue
		}
		stats.Games++
		if match.Result == "win" {
			stats.TopHalf++
		}
		if subject.Placement == 1 {
			stats.Top1++
		}
	}
	if stats.Games == 0 {
		return nil
	}
	return stats
}

type arenaHistoryExtensionEntry struct {
	matches []gameplayMatch
	done    chan struct{}
}

type arenaHistoryExtensionCache struct {
	gameID  int64
	entries map[string]*arenaHistoryExtensionEntry
}

// extendLiveArenaHistory returns base merged with games 11–30 of the same
// server-filtered queue. It only runs when the first page was really filtered
// by the server (so start indices are in queue space) and was full.
func (a *app) extendLiveArenaHistory(ctx context.Context, client *LCUClient, reference gameplayReference, playerRef string, names map[int64]string, gameID, queueID int64, base livePlayerMatchesResult) []gameplayMatch {
	if a == nil || a.sgp == nil || client == nil || clientRiotPlatform(client) != "" || !validPlayerReference(playerRef) {
		return base.Matches
	}
	if !isArenaQueue(queueID, "") || base.Evidence == nil || !base.Evidence.QueueFiltered || base.Evidence.StopReason != "enough" {
		return base.Matches
	}
	if len(recentMatchesForPlayer(base.Matches, playerRef, arenaLiveExtensionStart, queueID)) < arenaLiveExtensionStart {
		return base.Matches
	}
	key := strconv.FormatInt(queueID, 10) + "|" + playerRef
	state := a.r263()
	state.mu.Lock()
	if state.extend.gameID != gameID || state.extend.entries == nil {
		state.extend = arenaHistoryExtensionCache{gameID: gameID, entries: map[string]*arenaHistoryExtensionEntry{}}
	}
	entry := state.extend.entries[key]
	leader := entry == nil
	if leader {
		entry = &arenaHistoryExtensionEntry{done: make(chan struct{})}
		state.extend.entries[key] = entry
	}
	state.mu.Unlock()
	if !leader {
		select {
		case <-entry.done:
		case <-ctx.Done():
			return base.Matches
		}
		if len(entry.matches) == 0 {
			return base.Matches
		}
		return mergeLivePlayerMatches(base.Matches, cloneGameplayMatches(entry.matches), playerRef)
	}
	extra := a.fetchLiveArenaExtension(ctx, client, reference, playerRef, names, queueID)
	state.mu.Lock()
	entry.matches = cloneGameplayMatches(extra)
	if extra == nil && state.extend.entries[key] == entry {
		// Failed or canceled: allow a later refresh to retry.
		delete(state.extend.entries, key)
	}
	state.mu.Unlock()
	close(entry.done)
	if len(extra) == 0 {
		return base.Matches
	}
	return mergeLivePlayerMatches(base.Matches, extra, playerRef)
}

func (a *app) fetchLiveArenaExtension(ctx context.Context, client *LCUClient, reference gameplayReference, playerRef string, names map[int64]string, queueID int64) []gameplayMatch {
	serverID, _, ok := a.sgp.available(client)
	if !ok {
		return nil
	}
	if reference.ServerID != "" {
		serverID = reference.ServerID
	}
	filter := "all"
	if definition, known := supportedQueueDefinition(queueID); known {
		filter = definition.Filter
	}
	started := time.Now()
	loadCtx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	infos, _, _, resolution, err := a.loadSGPMatchHistoryPage(loadCtx, client, serverID, playerRef, arenaLiveExtensionStart, arenaLiveExtensionCount, filter, sgpHistoryPageOptions{NoCache: true, QueueID: queueID, FallbackCount: arenaLiveExtensionCount})
	event := map[string]any{"event": "live_arena_history_extension", "queue_id": queueID, "elapsed_ms": time.Since(started).Milliseconds(), "server_filtered": resolution.ServerFiltered, "games": len(infos)}
	if err != nil {
		event["outcome"] = "failed"
		event["reason"] = safeDiagnosticReason(err)
		a.recordDiagnostic(event)
		return nil
	}
	if !resolution.ServerFiltered {
		// Start index 10 would point into the unfiltered list.
		event["outcome"] = "not_filtered"
		a.recordDiagnostic(event)
		return []gameplayMatch{}
	}
	matches := make([]gameplayMatch, 0, len(infos))
	for _, info := range infos {
		a.checkArenaGroupTruth(client, reference.ServerID, info)
		matches = append(matches, convertRiotMatchInfo(info, playerRef, names, nil, "", reference.ServerID))
	}
	event["outcome"] = "ok"
	a.recordDiagnostic(event)
	return matches
}

// R263 P4：对局页战绩小卡弹窗。
//
// 对局页每名玩家的最近战绩在加载时已经是完整的 gameplayMatch（SGP 摘要带全部
// 参与者），这里按「对局 → 玩家 → gameId」保留最近两局对局页的数据，弹窗直接
// 取用，不再请求网络。缺失时退回不可变单局缓存（SGP / LCU）。
type liveMatchIndex struct {
	games []liveMatchIndexGame
}

type liveMatchIndexGame struct {
	gameID  int64
	players map[string]map[int64]gameplayMatch
}

func (a *app) rememberLiveMatches(liveGameID int64, playerRef string, matches []gameplayMatch) {
	if a == nil || !validPlayerReference(playerRef) || len(matches) == 0 {
		return
	}
	state := a.r263()
	state.mu.Lock()
	defer state.mu.Unlock()
	index := -1
	for i := range state.matches.games {
		if state.matches.games[i].gameID == liveGameID {
			index = i
			break
		}
	}
	if index < 0 {
		state.matches.games = append([]liveMatchIndexGame{{gameID: liveGameID, players: map[string]map[int64]gameplayMatch{}}}, state.matches.games...)
		if len(state.matches.games) > 2 {
			state.matches.games = state.matches.games[:2]
		}
		index = 0
	}
	byID := state.matches.games[index].players[playerRef]
	if byID == nil {
		byID = map[int64]gameplayMatch{}
		state.matches.games[index].players[playerRef] = byID
	}
	for _, match := range matches {
		if match.GameID > 0 && len(match.Participants) > 0 {
			byID[match.GameID] = match
		}
	}
}

func (a *app) lookupLiveMatch(playerRef string, gameID int64) (gameplayMatch, bool) {
	state := a.r263()
	state.mu.Lock()
	defer state.mu.Unlock()
	var fallback gameplayMatch
	found := false
	for _, game := range state.matches.games {
		if match, ok := game.players[playerRef][gameID]; ok {
			return match, true
		}
		if !found {
			for _, byID := range game.players {
				if match, ok := byID[gameID]; ok && len(match.Participants) > 1 {
					fallback, found = match, true
					break
				}
			}
		}
	}
	return fallback, found
}

func cloneGameplayMatchDeep(match gameplayMatch) gameplayMatch {
	match.Participants = append([]gameplayParticipant(nil), match.Participants...)
	match.Teams = append([]gameplayTeam(nil), match.Teams...)
	return match
}

func liveMatchWithSubject(match gameplayMatch, playerRef string) (gameplayMatch, bool) {
	match = cloneGameplayMatchDeep(match)
	var subject *gameplayParticipant
	for index := range match.Participants {
		participant := &match.Participants[index]
		if participant.PlayerRef == playerRef || participant.reference.PlayerRef == playerRef {
			subject = participant
			break
		}
	}
	if subject == nil {
		return match, false
	}
	match.SubjectParticipantID = subject.ParticipantID
	if isArenaQueue(match.QueueID, match.GameMode) || match.ModeGroup == "arena" {
		applyArenaResults(&match)
	} else if match.Result != "remake" {
		match.Result = map[bool]string{true: "win", false: "loss"}[subject.Win]
	}
	return match, true
}

// handleGameplayLiveMatch serves GET /api/gameplay/live/match?gameId=&player=.
func (a *app) handleGameplayLiveMatch(w http.ResponseWriter, r *http.Request) {
	gameID, err := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("gameId")), 10, 64)
	publicRef := strings.TrimSpace(r.URL.Query().Get("player"))
	if err != nil || gameID <= 0 || publicRef == "" || len(publicRef) > 128 {
		http.Error(w, "查询参数无效", http.StatusBadRequest)
		return
	}
	reference, ok := a.resolveGameplayReferenceDetails(publicRef)
	if !ok {
		http.Error(w, "玩家信息已过期，请刷新对局页", http.StatusNotFound)
		return
	}
	playerRef := reference.PlayerRef
	source := "live-index"
	match, found := a.lookupLiveMatch(playerRef, gameID)
	if !found || len(match.Participants) < 2 {
		if loaded, ok := a.loadLiveMatchFromHistoryCache(r.Context(), reference, gameID); ok {
			match, found, source = loaded, true, "history-cache"
		}
	}
	if !found {
		a.recordDiagnostic(map[string]any{"event": "live_match_detail", "outcome": "not_found", "game_id": gameID})
		http.Error(w, "这场对局的详情暂不可用", http.StatusNotFound)
		return
	}
	match, hasSubject := liveMatchWithSubject(match, playerRef)
	if !hasSubject {
		a.recordDiagnostic(map[string]any{"event": "live_match_detail", "outcome": "no_subject", "game_id": gameID, "source": source})
		http.Error(w, "这场对局里找不到该玩家", http.StatusNotFound)
		return
	}
	applyMatchScores(&match)
	a.publicizeMatchReferences(&match)
	a.recordDiagnostic(map[string]any{"event": "live_match_detail", "outcome": "ok", "game_id": gameID, "source": source, "participants": len(match.Participants)})
	respondJSON(w, match)
}

func (a *app) loadLiveMatchFromHistoryCache(ctx context.Context, reference gameplayReference, gameID int64) (gameplayMatch, bool) {
	client, _, err := a.gameplayClient()
	if err != nil || client == nil {
		return gameplayMatch{}, false
	}
	names := a.overviewChampionNames(ctx)
	serverID := strings.ToUpper(strings.TrimSpace(reference.ServerID))
	if serverID == "" {
		serverID = strings.ToUpper(clientTencentServerID(client))
	}
	store, scope := client.historyGameStore()
	if serverID != "" {
		if info, ok := store.sgp(serverID, gameID); ok && len(info.Participants) > 1 {
			return convertRiotMatchInfo(info, reference.PlayerRef, names, nil, "", serverID), true
		}
	}
	if game, ok := store.lcu(scope, gameID, true); ok {
		return normalizeGameplayMatch(game, reference, names, nil), true
	}
	return gameplayMatch{}, false
}

// R263 fields are kept in embedded structs so the existing gameflow structs
// keep their alignment (and their history stays readable).
type liveR263ResponseFields struct {
	// ArenaSquadSize is the number of player slots per Arena card (3 in 1750).
	ArenaSquadSize int `json:"arenaSquadSize,omitempty"`
}

type liveR263PlayerFields struct {
	// ArenaSquadKey: mine / premade:A / premade:B … / empty (unknown).
	ArenaSquadKey    string `json:"arenaSquadKey,omitempty"`
	ArenaSquadSource string `json:"arenaSquadSource,omitempty"`
	// SoloRank/FlexRank: Summoner's Rift shows both under the name (LeagueAkari).
	SoloRank *gameplayRank `json:"soloRank,omitempty"`
	FlexRank *gameplayRank `json:"flexRank,omitempty"`
	// ArenaFame/ArenaRecord: Arena level + fame and 30-game top-half / first.
	ArenaFame   *gameplayArenaFame   `json:"arenaFame,omitempty"`
	ArenaRecord *gameplayArenaRecord `json:"arenaRecord,omitempty"`
}

type liveR263RecentGameFields struct {
	GameID    int64 `json:"gameId,omitempty"`
	QueueID   int64 `json:"queueId,omitempty"`
	Placement int   `json:"placement,omitempty"`
}
