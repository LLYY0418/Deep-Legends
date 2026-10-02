package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
)

func liveClientMapValueString(entry map[string]any, name string) (string, bool) {
	value, ok := liveClientMapValue(entry, name)
	if !ok {
		return "", false
	}
	text, ok := value.(string)
	return strings.TrimSpace(text), ok
}

// Gameflow can omit players after the local Live Client playerlist has all ten.
// Only an exact Riot ID lookup may supply a missing PUUID. In supported 5v5
// PvP queues an anonymous slot may be shown without identity or history once
// its team is verified against the current player.
func liveTenPlayerRosterQueue(queueID int64) bool {
	if queueID == seasonQueueSoloDuo || queueID == seasonQueueFlex {
		return true
	}
	definition, ok := supportedQueueDefinition(queueID)
	if !ok {
		return false
	}
	switch definition.ModeGroup {
	case "solo", "flex", "match", "aram", "hextech-aram", "clash", "urf":
		return true
	default:
		return false
	}
}

func liveAnonymousRosterQueue(queueID int64) bool {
	return liveTenPlayerRosterQueue(queueID)
}

type liveRosterEntry = struct {
	player lcuLivePlayer
	team   int64
}
type liveRosterRecoveryStats struct {
	AnonymousCount  int
	NamedPresent    int
	NamedAppended   int
	UnresolvedNamed int
	Cached          bool
}
type liveRosterRecoveryScope struct {
	GameID   int64
	QueueID  int64
	Phase    string
	RawCount int
}
type liveRosterRecoveryResult struct {
	Players  []liveRosterEntry
	Appended int
	Attempts int
	Reason   string
	Stats    liveRosterRecoveryStats
}
type liveRosterRecoveryFlight struct {
	Done   chan struct{}
	Result liveRosterRecoveryResult
}
type liveRosterRecoveryCache struct {
	Mu                     sync.Mutex
	GameID                 int64
	Client                 *LCUClient
	Entries                map[string]*liveRosterRecoveryFlight
	QueueUnsupportedLogged bool
}

func (a *app) clearLiveRosterRecovery() {
	a.liveRosterRecovery.Mu.Lock()
	a.liveRosterRecovery.GameID = 0
	a.liveRosterRecovery.Client = nil
	a.liveRosterRecovery.Entries = nil
	a.liveRosterRecovery.QueueUnsupportedLogged = false
	a.liveRosterRecovery.Mu.Unlock()
}

func (a *app) prepareLiveRosterRecovery(client *LCUClient, gameID int64) {
	cache := &a.liveRosterRecovery
	cache.Mu.Lock()
	defer cache.Mu.Unlock()
	if cache.GameID != gameID || cache.Client != client {
		cache.GameID, cache.Client = gameID, client
		cache.Entries = make(map[string]*liveRosterRecoveryFlight)
		cache.QueueUnsupportedLogged = false
	}
}

// R194: distinguish unsupported queues from a failed recovery, once per game.
func (a *app) recordUnsupportedLiveRosterQueue(client *LCUClient, scope liveRosterRecoveryScope, playerlistCount int) {
	cache := &a.liveRosterRecovery
	cache.Mu.Lock()
	if cache.GameID != scope.GameID || cache.Client != client || cache.QueueUnsupportedLogged {
		cache.Mu.Unlock()
		return
	}
	cache.QueueUnsupportedLogged = true
	cache.Mu.Unlock()
	a.recordDiagnostic(map[string]any{"event": "live_roster_recovery", "game_id": scope.GameID, "queue_id": scope.QueueID, "phase": scope.Phase, "raw_count": scope.RawCount, "playerlist_count": playerlistCount, "appended": 0, "alias_attempts": 0, "reason": "queue-unsupported", "anonymous_count": 0, "named_present": 0, "named_appended": 0, "unresolved_named": 0, "cached": false})
}

// Fingerprints stay in memory. Include position/champion data too so a changed
// anonymous slot never inherits a previous slot's portrait or lane.
func liveRosterRecoveryKey(current Summoner, raw []liveRosterEntry, snapshot liveClientSnapshot, names map[int64]string, allowAnonymous bool) string {
	live := make([]string, 0, len(snapshot.RosterPlayers))
	for _, p := range snapshot.RosterPlayers {
		encoded, _ := json.Marshal([]any{strings.ToUpper(strings.TrimSpace(p.Team)), strings.ToLower(p.GameName), strings.ToLower(p.TagLine), p.Position, p.ChampionName, p.IsBot})
		live = append(live, string(encoded))
	}
	players := make([]string, 0, len(raw))
	for _, p := range raw {
		encoded, _ := json.Marshal(struct {
			Player lcuLivePlayer
			Team   int64
		}{p.player, p.team})
		players = append(players, string(encoded))
	}
	sort.Strings(live)
	sort.Strings(players)
	encoded, _ := json.Marshal([]any{current.PUUID, current.GameName, current.TagLine, allowAnonymous, live, players, names})
	return fmt.Sprintf("%x", sha256.Sum256(encoded))
}
func cloneLiveRosterRecovery(result liveRosterRecoveryResult) liveRosterRecoveryResult {
	result.Players = append([]liveRosterEntry(nil), result.Players...)
	return result
}
func (a *app) cachedClassicLiveRoster(ctx context.Context, client *LCUClient, current Summoner, raw []liveRosterEntry, snapshot liveClientSnapshot, names map[int64]string, allowAnonymous bool, scope liveRosterRecoveryScope) (result liveRosterRecoveryResult) {
	key := liveRosterRecoveryKey(current, raw, snapshot, names, allowAnonymous)
	cache := &a.liveRosterRecovery
	cache.Mu.Lock()
	if cache.GameID != scope.GameID || cache.Client != client {
		cache.GameID, cache.Client = scope.GameID, client
		cache.Entries = make(map[string]*liveRosterRecoveryFlight)
		cache.QueueUnsupportedLogged = false
	}
	if flight := cache.Entries[key]; flight != nil {
		cache.Mu.Unlock()
		select {
		case <-ctx.Done():
			return liveRosterRecoveryResult{Players: append([]liveRosterEntry(nil), raw...), Reason: "cancelled", Stats: liveRosterRecoveryStats{Cached: true}}
		case <-flight.Done:
			result = cloneLiveRosterRecovery(flight.Result)
			result.Stats.Cached = true
			return result
		}
	}
	entries := cache.Entries
	flight := &liveRosterRecoveryFlight{Done: make(chan struct{})}
	entries[key] = flight
	cache.Mu.Unlock()
	finished := false
	defer func() {
		cache.Mu.Lock()
		if !finished {
			// A panic must release followers and permit a fresh attempt.
			flight.Result = liveRosterRecoveryResult{Players: append([]liveRosterEntry(nil), raw...), Reason: "interrupted"}
			delete(entries, key)
		}
		close(flight.Done)
		cache.Mu.Unlock()
	}()
	result.Players, result.Appended, result.Attempts, result.Reason = a.computeClassicLiveRoster(ctx, client, current, raw, snapshot, names, allowAnonymous, &result.Stats)
	cache.Mu.Lock()
	flight.Result = cloneLiveRosterRecovery(result)
	// Cancelled loaders can be retried and cannot publish a diagnostic as a result.
	if ctx.Err() != nil {
		delete(entries, key)
	}
	active := cache.GameID == scope.GameID && cache.Client == client && cache.Entries[key] == flight
	cache.Mu.Unlock()
	finished = true
	if active && ctx.Err() == nil {
		a.recordDiagnostic(map[string]any{"event": "live_roster_recovery", "game_id": scope.GameID, "queue_id": scope.QueueID, "phase": scope.Phase, "raw_count": scope.RawCount, "playerlist_count": len(snapshot.RosterPlayers), "appended": result.Appended, "alias_attempts": result.Attempts, "reason": result.Reason, "anonymous_count": result.Stats.AnonymousCount, "named_present": result.Stats.NamedPresent, "named_appended": result.Stats.NamedAppended, "unresolved_named": result.Stats.UnresolvedNamed, "cached": result.Stats.Cached})
	}
	return cloneLiveRosterRecovery(result)
}
func (a *app) recoverClassicLiveRoster(ctx context.Context, client *LCUClient, current Summoner, raw []liveRosterEntry, snapshot liveClientSnapshot, names map[int64]string, allowAnonymous bool, scopes ...liveRosterRecoveryScope) ([]liveRosterEntry, int, int, string) {
	if len(scopes) > 0 && scopes[0].GameID > 0 {
		result := a.cachedClassicLiveRoster(ctx, client, current, raw, snapshot, names, allowAnonymous, scopes[0])
		return result.Players, result.Appended, result.Attempts, result.Reason
	}
	return a.computeClassicLiveRoster(ctx, client, current, raw, snapshot, names, allowAnonymous, &liveRosterRecoveryStats{})
}
func (a *app) computeClassicLiveRoster(ctx context.Context, client *LCUClient, current Summoner, raw []liveRosterEntry, snapshot liveClientSnapshot, championNames map[int64]string, allowAnonymous bool, stats *liveRosterRecoveryStats) ([]liveRosterEntry, int, int, string) {
	if len(raw) >= 10 || len(snapshot.RosterPlayers) != 10 {
		return raw, 0, 0, "playerlist-incomplete"
	}
	selfTeam := int64(0)
	for _, entry := range raw {
		if strings.EqualFold(visibleLivePlayerReference(entry.player), current.PUUID) {
			selfTeam = entry.team
			break
		}
	}
	if selfTeam != 100 && selfTeam != 200 || current.GameName == "" || current.TagLine == "" {
		return raw, 0, 0, "self-unverified"
	}
	selfLiveTeam := ""
	teamCounts := map[string]int{}
	for _, entry := range snapshot.RosterPlayers {
		team := strings.ToUpper(strings.TrimSpace(entry.Team))
		teamCounts[team]++
		if strings.EqualFold(entry.GameName, current.GameName) && strings.EqualFold(entry.TagLine, current.TagLine) {
			if selfLiveTeam != "" {
				return raw, 0, 0, "self-ambiguous"
			}
			selfLiveTeam = team
		}
	}
	if teamCounts["ORDER"] != 5 || teamCounts["CHAOS"] != 5 || selfLiveTeam != "ORDER" && selfLiveTeam != "CHAOS" {
		return raw, 0, 0, "teams-unverified"
	}
	targetTeam := int64(100)
	if teamCountsInRaw(raw, 100) == 5 {
		targetTeam = 200
	}
	if teamCountsInRaw(raw, targetTeam) >= 5 || teamCountsInRaw(raw, 100)+teamCountsInRaw(raw, 200) != len(raw) {
		return raw, 0, 0, "no-short-team"
	}
	targetLiveTeam := selfLiveTeam
	if targetTeam != selfTeam {
		if selfLiveTeam == "ORDER" {
			targetLiveTeam = "CHAOS"
		} else {
			targetLiveTeam = "ORDER"
		}
	}
	serverID := clientTencentServerID(client)
	if serverID == "" {
		return raw, 0, 0, "server-unverified"
	}
	missing := 5 - teamCountsInRaw(raw, targetTeam)
	attempts := 0
	anonymous := make([]liveClientRosterPlayer, 0, missing)
	unresolvedNamed := 0
	championIDForName := func(name string) int64 {
		if name == "" {
			return 0
		}
		championID := int64(0)
		for id, candidate := range championNames {
			if strings.EqualFold(strings.TrimSpace(candidate), strings.TrimSpace(name)) {
				if championID != 0 {
					return 0
				}
				championID = id
			}
		}
		return championID
	}
	pending := make([]struct {
		player lcuLivePlayer
		team   int64
	}, 0, missing)
	seenRiotIDs := map[string]bool{}
	for _, entry := range snapshot.RosterPlayers {
		if ctx.Err() != nil {
			break
		}
		if !strings.EqualFold(entry.Team, targetLiveTeam) {
			continue
		}
		if entry.GameName == "" || entry.TagLine == "" {
			if entry.IsBot {
				continue
			}
			anonymous = append(anonymous, entry)
			stats.AnonymousCount++
			continue
		}
		key := strings.ToLower(entry.GameName + "#" + entry.TagLine)
		if seenRiotIDs[key] {
			return raw, 0, attempts, "duplicate-riot-id"
		}
		seenRiotIDs[key] = true
		attempts++
		identity, err := a.resolveTencentRiotID(ctx, client, entry.GameName, entry.TagLine, serverID)
		if err != nil || !validPlayerReference(identity.PlayerRef) {
			unresolvedNamed++
			stats.UnresolvedNamed++
			continue
		}
		present := false
		for _, candidate := range raw {
			if strings.EqualFold(visibleLivePlayerReference(candidate.player), identity.PlayerRef) {
				if candidate.team != targetTeam {
					return raw, 0, attempts, "team-conflict"
				}
				present = true
				break
			}
		}
		for _, candidate := range pending {
			if strings.EqualFold(candidate.player.PUUID, identity.PlayerRef) {
				return raw, 0, attempts, "duplicate-puuid"
			}
		}
		if present {
			stats.NamedPresent++
			continue
		}
		championID := championIDForName(entry.ChampionName)
		pending = append(pending, struct {
			player lcuLivePlayer
			team   int64
		}{player: lcuLivePlayer{PUUID: identity.PlayerRef, GameName: entry.GameName, TagLine: entry.TagLine, SummonerName: entry.GameName, SelectedPosition: entry.Position, ChampionID: championID}, team: targetTeam})
	}
	if len(pending) > missing {
		return raw, 0, attempts, "overfilled-team"
	}
	stats.NamedAppended = len(pending)
	remaining := missing - len(pending)
	if allowAnonymous && remaining > 0 && len(anonymous) == remaining && unresolvedNamed == 0 && ctx.Err() == nil {
		for index := 0; index < remaining; index++ {
			// Team and cardinality are known; the player's identity and history are not.
			pending = append(pending, struct {
				player lcuLivePlayer
				team   int64
			}{player: lcuLivePlayer{NameVisibilityType: "HIDDEN", SelectedPosition: anonymous[index].Position, ChampionID: championIDForName(anonymous[index].ChampionName)}, team: targetTeam})
		}
		raw = append(raw, pending...)
		return raw, len(pending), attempts, "anonymous-placeholder"
	}
	raw = append(raw, pending...)
	if len(pending) == missing {
		return raw, len(pending), attempts, "resolved"
	}
	return raw, len(pending), attempts, "partial"
}

func teamCountsInRaw(raw []struct {
	player lcuLivePlayer
	team   int64
}, team int64) int {
	count := 0
	for _, entry := range raw {
		if entry.team == team {
			count++
		}
	}
	return count
}
