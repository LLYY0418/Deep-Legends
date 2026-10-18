package main

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"sync"
	"time"
)

const gsmRequestBudget = 4 * time.Second

func liveGSMRosterSize(queueID int64) int {
	switch queueID {
	case 1750:
		return 18
	case 1700, 1710:
		return 16
	}
	if liveTenPlayerRosterQueue(queueID) {
		return 10
	}
	return 0
}

// Read only roster fields. Reconnect credentials, encryption keys, server IPs,
// room passwords and JWTs in the upstream response are deliberately ignored.
type sgpGSMResponse struct {
	Game struct {
		ID         int64           `json:"id"`
		QueueID    int64           `json:"gameQueueConfigId"`
		MapID      int64           `json:"mapId"`
		GameMode   string          `json:"gameMode"`
		GameState  string          `json:"gameState"`
		TeamOne    []lcuLivePlayer `json:"teamOne"`
		TeamTwo    []lcuLivePlayer `json:"teamTwo"`
		Selections []struct {
			PUUID      string `json:"puuid"`
			ChampionID int64  `json:"championId"`
			Spell1ID   int64  `json:"spell1Id"`
			Spell2ID   int64  `json:"spell2Id"`
		} `json:"playerChampionSelections"`
	} `json:"game"`
}

type sgpGSMKey struct {
	Client          *LCUClient
	Server, Player  string
	GameID, QueueID int64
}
type sgpGSMResult struct {
	Players       []liveRosterEntry
	State, Reason string
	Cached        bool
	Stale         bool
}
type sgpGSMEntry struct {
	At         time.Time
	TTL        time.Duration
	Result     sgpGSMResult
	RetryAfter time.Time
}
type sgpGSMFlight struct {
	Done   chan struct{}
	Result sgpGSMResult
}
type sgpGSMCache struct {
	mu      sync.Mutex
	entries map[sgpGSMKey]sgpGSMEntry
	flights map[sgpGSMKey]*sgpGSMFlight
}

func cloneGSMResult(result sgpGSMResult) sgpGSMResult {
	result.Players = append([]liveRosterEntry(nil), result.Players...)
	return result
}

// A current-player request, scoped to the exact game/account/client/server.
// Success caches for 30s (partial roster 3s), confirmed absence for 10s,
// failure for 5s. A canceled request is never cached. All caches are in memory.
func (p *sgpProvider) liveGameRoster(ctx context.Context, client *LCUClient, server, player string, gameID, queueID int64) (result sgpGSMResult) {
	server, player = strings.ToUpper(strings.TrimSpace(server)), strings.TrimSpace(player)
	if ctx.Err() != nil {
		return sgpGSMResult{State: capabilityCanceled, Reason: "canceled"}
	}
	if p == nil || client == nil || server == "" || !validPlayerReference(player) || player == emptyLCUPlayerPUUID || gameID <= 0 || liveGSMRosterSize(queueID) == 0 {
		return sgpGSMResult{State: capabilityUnsupported, Reason: "scope-unavailable"}
	}
	key := sgpGSMKey{client, server, player, gameID, queueID}
	c := &p.gsm
	c.mu.Lock()
	if entry, ok := c.entries[key]; ok && (time.Since(entry.At) < entry.TTL || time.Now().Before(entry.RetryAfter)) {
		c.mu.Unlock()
		result = cloneGSMResult(entry.Result)
		result.Cached = true
		if time.Since(entry.At) >= entry.TTL {
			result.Stale, result.Reason = true, "stale-verified"
		}
		return result
	}
	if flight := c.flights[key]; flight != nil {
		c.mu.Unlock()
		select {
		case <-ctx.Done():
			return sgpGSMResult{State: capabilityCanceled, Reason: "canceled"}
		case <-flight.Done:
			result = cloneGSMResult(flight.Result)
			result.Cached = true
			return result
		}
	}
	if len(c.flights) >= 8 {
		c.mu.Unlock()
		return sgpGSMResult{State: capabilityPending, Reason: "busy"}
	}
	if c.flights == nil {
		c.flights = make(map[sgpGSMKey]*sgpGSMFlight)
	}
	flight := &sgpGSMFlight{Done: make(chan struct{})}
	c.flights[key] = flight
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		delete(c.flights, key)
		if ctx.Err() == nil && result.State != "" && result.State != capabilityCanceled {
			// Failure may back off, but cannot erase a verified same-game roster or
			// refresh its original success TTL. Retain it for at most five minutes.
			if previous, ok := c.entries[key]; ok && result.State == capabilityFailed && previous.Result.State == capabilityAvailable && time.Since(previous.At) < 5*time.Minute {
				previous.RetryAfter = time.Now().Add(5 * time.Second)
				c.entries[key] = previous
				result = cloneGSMResult(previous.Result)
				result.Cached, result.Stale, result.Reason = true, true, "stale-verified"
				flight.Result = cloneGSMResult(result)
				close(flight.Done)
				return
			}
			ttl := 5 * time.Second
			if result.State == capabilityAvailable {
				ttl = 30 * time.Second
				if len(result.Players) < liveGSMRosterSize(queueID) {
					ttl = 3 * time.Second
				}
			}
			if result.Reason == "no-active-game" {
				ttl = 10 * time.Second
			}
			if c.entries == nil {
				c.entries = make(map[sgpGSMKey]sgpGSMEntry)
			}
			if len(c.entries) >= 16 {
				var oldest sgpGSMKey
				var at time.Time
				for k, e := range c.entries {
					if at.IsZero() || e.At.Before(at) {
						oldest, at = k, e.At
					}
				}
				delete(c.entries, oldest)
			}
			c.entries[key] = sgpGSMEntry{At: time.Now(), TTL: ttl, Result: cloneGSMResult(result)}
		}
		flight.Result = cloneGSMResult(result)
		close(flight.Done)
	}()
	requestCtx, cancel := context.WithTimeout(ctx, gsmRequestBudget)
	defer cancel()
	base, ok := p.serverBaseOn(requestCtx, client, server)
	if !ok {
		return sgpGSMResult{State: capabilityUnsupported, Reason: "gateway-unavailable"}
	}
	// Akari's Tencent _getSubId returns the uppercase rsoPlatformId (HN1).
	path := "/gsm/v1/ledge/region/" + server + "/puuid/" + url.PathEscape(player)
	var response sgpGSMResponse
	err := p.getJSONWithToken(requestCtx, client, sgpTokenLeagueSession, server, "GSM", "/gsm/v1/ledge/region/{server}/puuid/{player}", base+path, &response)
	if err != nil {
		if ctx.Err() != nil {
			return sgpGSMResult{State: capabilityCanceled, Reason: "canceled"}
		}
		var httpErr *sgpHTTPError
		if errors.As(err, &httpErr) && httpErr.StatusCode == 404 {
			return sgpGSMResult{State: capabilityUnsupported, Reason: "no-active-game"}
		}
		return sgpGSMResult{State: capabilityFailed, Reason: "request-failed"}
	}
	return validateGSMRoster(response, player, gameID, queueID)
}

func validateGSMRoster(response sgpGSMResponse, current string, gameID, queueID int64) sgpGSMResult {
	reject := func(reason string) sgpGSMResult { return sgpGSMResult{State: capabilityFailed, Reason: reason} }
	game := response.Game
	if game.ID != gameID || gameID <= 0 {
		return reject("game-mismatch")
	}
	if game.QueueID != queueID {
		return reject("queue-mismatch")
	}
	switch strings.ToUpper(game.GameState) {
	case "TERMINATED", "TERMINATED_IN_ERROR", "FINISHED", "END_OF_GAME":
		return reject("game-ended")
	}
	limit := liveGSMRosterSize(queueID)
	arena := isArenaQueue(queueID, "")
	if arena && !strings.EqualFold(game.GameMode, "CHERRY") {
		return reject("mode-mismatch")
	}
	if limit == 0 || len(game.TeamOne)+len(game.TeamTwo) == 0 || len(game.TeamOne)+len(game.TeamTwo) > limit || !arena && (len(game.TeamOne) > 5 || len(game.TeamTwo) > 5) {
		return reject("roster-invalid")
	}
	seen := map[string]bool{}
	rows := make([]liveRosterEntry, 0, limit)
	for side, members := range [][]lcuLivePlayer{game.TeamOne, game.TeamTwo} {
		for _, member := range members {
			member.PUUID = strings.TrimSpace(member.PUUID)
			if arena && (member.PUUID == "" || member.PUUID == emptyLCUPlayerPUUID) {
				continue // Akari filters anonymous GSM slots; retain other verified identities.
			}
			if !validPlayerReference(member.PUUID) || member.PUUID == emptyLCUPlayerPUUID || seen[member.PUUID] {
				return reject("identity-invalid")
			}
			seen[member.PUUID] = true
			// GSM supplies real PUUIDs, never an obfuscated identity transform.
			member.ObfuscatedPUUID, member.ObfuscatedSummonerID, member.NameVisibilityType = "", 0, ""
			team := int64(100 + side*100)
			if arena {
				team = 100 // CHERRY is TEAM-ALL; GSM sides are not Arena squads.
			}
			rows = append(rows, liveRosterEntry{member, team})
		}
	}
	if !seen[current] {
		return reject("current-player-missing")
	}
	selections := map[string]bool{}
	for _, selected := range game.Selections {
		if arena && (selected.PUUID == "" || selected.PUUID == emptyLCUPlayerPUUID) {
			continue
		}
		if !seen[selected.PUUID] || selections[selected.PUUID] {
			return reject("selection-invalid")
		}
		selections[selected.PUUID] = true
		for i := range rows {
			if rows[i].player.PUUID != selected.PUUID {
				continue
			}
			if rows[i].player.ChampionID > 0 && selected.ChampionID > 0 && rows[i].player.ChampionID != selected.ChampionID {
				return reject("champion-conflict")
			}
			if rows[i].player.ChampionID == 0 {
				rows[i].player.ChampionID = selected.ChampionID
			}
			rows[i].player.Spell1ID, rows[i].player.Spell2ID = selected.Spell1ID, selected.Spell2ID
		}
	}
	return sgpGSMResult{State: capabilityAvailable, Reason: "verified", Players: rows}
}

func liveGSMRosterNeeded(raw []liveRosterEntry, queueID int64) bool {
	if len(raw) < liveGSMRosterSize(queueID) {
		return true
	}
	for _, row := range raw {
		ref := visibleLivePlayerReference(row.player)
		if !validPlayerReference(ref) || ref == emptyLCUPlayerPUUID {
			return true
		}
	}
	return false
}

// Existing current-game values win. A conflicting real identity/side/champion
// rejects the supplement; anonymous rows need a unique same-team champion match.
func mergeGSMLiveRoster(raw, supplemental []liveRosterEntry) ([]liveRosterEntry, int, int, string) {
	for _, source := range supplemental {
		for _, row := range raw {
			if source.player.PUUID == visibleLivePlayerReference(row.player) && (source.team != row.team || source.player.ChampionID > 0 && row.player.ChampionID > 0 && source.player.ChampionID != row.player.ChampionID) {
				return raw, 0, 0, "roster-conflict"
			}
		}
	}
	rows := append([]liveRosterEntry(nil), raw...)
	appended, restored := 0, 0
	for _, source := range supplemental {
		index, anonymous, championCount := -1, -1, 0
		for i, row := range rows {
			if source.player.PUUID == visibleLivePlayerReference(row.player) {
				index = i
				break
			}
			if row.team == source.team && source.player.ChampionID > 0 && row.player.ChampionID == source.player.ChampionID {
				championCount++
				ref := visibleLivePlayerReference(row.player)
				if !validPlayerReference(ref) || ref == emptyLCUPlayerPUUID {
					anonymous = i
				}
			}
		}
		if index < 0 {
			if championCount == 1 && anonymous >= 0 {
				rows[anonymous].player = source.player
				restored++
				continue
			}
			if championCount > 0 || teamCountsInRaw(rows, source.team) >= 5 || source.player.ChampionID <= 0 {
				continue
			}
			rows = append(rows, source)
			appended++
			restored++
			continue
		}
		mergeGSMPlayerDetails(&rows[index].player, source.player)
	}
	return rows, appended, restored, "merged"
}

func mergeGSMPlayerDetails(p *lcuLivePlayer, source lcuLivePlayer) {
	if p.ChampionID <= 0 {
		p.ChampionID = source.ChampionID
	}
	if p.Spell1ID <= 0 {
		p.Spell1ID = source.Spell1ID
	}
	if p.Spell2ID <= 0 {
		p.Spell2ID = source.Spell2ID
	}
	if p.SelectedPosition == "" {
		p.SelectedPosition = source.SelectedPosition
	}
	if p.SelectedRole == "" {
		p.SelectedRole = source.SelectedRole
	}
	if p.TeamParticipantID <= 0 {
		p.TeamParticipantID = source.TeamParticipantID
	}
	if p.SummonerID <= 0 {
		p.SummonerID = source.SummonerID
	}
	if p.ProfileIconID <= 0 {
		p.ProfileIconID = source.ProfileIconID
	}
	if p.SummonerName == "" {
		p.SummonerName = source.SummonerName
	}
}

func (a *app) supplementLiveGSMRoster(ctx context.Context, client *LCUClient, current Summoner, response *gameplayLiveResponse, raw []liveRosterEntry) []liveRosterEntry {
	if !gameplayIdentityPhase(response.Phase) || liveGSMRosterSize(response.QueueID) == 0 || !liveGSMRosterNeeded(raw, response.QueueID) {
		return raw
	}
	server := clientTencentServerID(client)
	if a.sgp == nil || server == "" {
		reason := "server-unavailable"
		if a.sgp == nil {
			reason = "provider-unavailable"
		}
		a.recordDiagnostic(map[string]any{"event": "live_gsm_roster", "game_id": response.GameID, "queue_id": response.QueueID, "phase": response.Phase, "state": capabilityUnsupported, "reason": reason})
		return raw
	}
	result := a.sgp.liveGameRoster(ctx, client, server, current.PUUID, response.GameID, response.QueueID)
	appended, restored, reason := 0, 0, result.Reason
	if result.State == capabilityAvailable && ctx.Err() == nil {
		if isArenaQueue(response.QueueID, response.GameMode) {
			raw, appended, restored, reason = mergeGSMArenaRoster(raw, result.Players, liveGSMRosterSize(response.QueueID))
		} else {
			raw, appended, restored, reason = mergeGSMLiveRoster(raw, result.Players)
		}
		if reason == "roster-conflict" {
			result.State = capabilityFailed
		}
		response.MergeAppended += appended
	}
	response.Capabilities = append(response.Capabilities, EndpointCapability{Name: "live-gsm-roster", Path: "/gsm/v1/ledge/region/{server}/puuid/{player}", State: result.State, Count: len(result.Players)})
	a.recordDiagnostic(map[string]any{"event": "live_gsm_roster", "game_id": response.GameID, "queue_id": response.QueueID, "phase": response.Phase, "state": result.State, "reason": reason, "players": len(result.Players), "restored": restored, "appended": appended, "cached": result.Cached, "stale": result.Stale})
	return raw
}
