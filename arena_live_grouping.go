package main

import (
	"context"
	"maps"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

func liveClientArenaDistribution(distribution map[string]int, playerCount, squadSize int) bool {
	if squadSize < 2 || playerCount < 6 || playerCount > 18 || len(distribution) < 2 {
		return false
	}
	total := 0
	for _, count := range distribution {
		if count < 1 || count > squadSize {
			return false
		}
		total += count
	}
	return total == playerCount
}

func validArenaGroupAssignments(groups []string, squadSize int) bool {
	counts := map[string]int{}
	for _, group := range groups {
		if strings.TrimSpace(group) == "" {
			return false
		}
		counts[group]++
	}
	return liveClientArenaDistribution(counts, len(groups), squadSize)
}

func validateArenaLiveGroups(players []gameplayLivePlayer, squadSize int) bool {
	groups := make([]string, len(players))
	for i, p := range players {
		groups[i] = p.ArenaGroup
	}
	return validArenaGroupAssignments(groups, squadSize)
}

// Both sources use the same queue registry and require complete block boundaries.
// Premade party IDs are deliberately irrelevant to Arena squad membership.
func arenaSessionOrderGroups(queueID int64, players []lcuLivePlayer) ([]string, bool) {
	return arenaOrderGroups(len(players), arenaSquadSize(queueID))
}

func arenaRosterDivisible(count, squadSize int) bool { return squadSize > 0 && count%squadSize == 0 }

func arenaOrderGroups(count, squadSize int) ([]string, bool) {
	if squadSize < 2 || count < 6 || count > 18 || count%squadSize != 0 {
		return nil, false
	}
	groups := make([]string, count)
	for index := range groups {
		groups[index] = strconv.Itoa(index/squadSize + 1)
	}
	return groups, true
}

func orderGroupsFromLiveClient(snapshot liveClientSnapshot, squadSize int) (liveClientArenaGrouping, bool) {
	groups, ok := arenaOrderGroups(len(snapshot.OrderedIdentities), squadSize)
	if !ok {
		return liveClientArenaGrouping{}, false
	}
	grouping := liveClientArenaGrouping{Field: "order", ByIdentity: map[string]string{}, IdentifiedPlayers: len(groups)}
	ambiguous := map[string]bool{}
	for index, keys := range snapshot.OrderedIdentities {
		if len(keys) == 0 {
			return liveClientArenaGrouping{}, false
		}
		for _, key := range keys {
			if ambiguous[key] {
				continue
			}
			if old := grouping.ByIdentity[key]; old != "" && old != groups[index] {
				delete(grouping.ByIdentity, key)
				ambiguous[key] = true
				continue
			}
			grouping.ByIdentity[key] = groups[index]
		}
	}
	return grouping, true
}

// Resolved identity is shared by grouping and position matching. CN gameflow
// often supplies only a PUUID; names arrive from the summoner lookup.
func livePlayerIdentityValues(raw lcuLivePlayer, player gameplayLivePlayer) ([]string, []string) {
	names := []string{player.reference.DisplayName, player.DisplayName, raw.SummonerName}
	riotIDs := []string{}
	for _, pair := range [][2]string{{player.GameName, player.TagLine}, {raw.GameName, raw.TagLine}} {
		if strings.TrimSpace(pair[0]) != "" && strings.TrimSpace(pair[1]) != "" {
			riotIDs = append(riotIDs, pair[0]+"#"+pair[1])
		}
	}
	riotIDs = append(riotIDs, player.GameName, raw.GameName)
	return names, riotIDs
}

func liveClientGroupingForPlayer(grouping liveClientArenaGrouping, raw lcuLivePlayer, player gameplayLivePlayer) string {
	names, riotIDs := livePlayerIdentityValues(raw, player)
	// Prefer exact Riot IDs over potentially ambiguous base names.
	values := append(riotIDs, names...)
	for _, value := range values {
		if group := grouping.ByIdentity[strings.ToLower(strings.TrimSpace(value))]; group != "" {
			return group
		}
	}
	for _, value := range values {
		for _, key := range normalizeLiveClientPlayerName(value) {
			if group := grouping.ByIdentity[key]; group != "" {
				return group
			}
		}
	}
	return ""
}

// missing allies cannot certify order; contradictory allies must reject it.
func arenaAlliesCorroborate(groups []string, players []gameplayLivePlayer, remembered []lcuLivePlayer, squadSize int, grouping liveClientArenaGrouping, selfPUUID string) (verified, rejected bool) {
	block := ""
	observe := func(group string) {
		if group == "" {
			return
		}
		if block != "" && block != group {
			rejected = true
		}
		block = group
	}
	for i, p := range players {
		if p.IsCurrent || p.IsAlly {
			observe(groups[i])
		}
	}
	matched := 0
	hasSelf := false
	for _, ally := range remembered {
		group := ""
		for i, p := range players {
			if ally.PUUID != "" && ally.PUUID == p.reference.PlayerRef || ally.SummonerID > 0 && ally.SummonerID == p.reference.SummonerID {
				group = groups[i]
				break
			}
			keys := arenaAllyIdentityKeys(ally)
			for _, key := range arenaAllyIdentityKeys(lcuLivePlayer{SummonerName: p.DisplayName, GameName: p.GameName, TagLine: p.TagLine}) {
				for _, other := range keys {
					if key == other {
						group = groups[i]
					}
				}
			}
		}
		if group == "" {
			group = liveClientGroupingForPlayer(grouping, ally, gameplayLivePlayer{})
		}
		if group != "" {
			matched++
			if ally.PUUID != "" && ally.PUUID == selfPUUID {
				hasSelf = true
			}
			observe(group)
		}
	}
	for _, p := range players {
		hasSelf = hasSelf || p.IsCurrent
	}
	verified = hasSelf && len(remembered) == squadSize && matched == len(remembered) && !rejected
	return
}

func (a *app) applyArenaLiveGrouping(client *LCUClient, current Summoner, response *gameplayLiveResponse, raw []lcuLivePlayer, snapshot liveClientSnapshot) {
	squadSize := arenaSquadSize(response.QueueID)
	a.arenaAlliesMu.RLock()
	remembered := append([]lcuLivePlayer(nil), a.arenaAllyPlayers...)
	if a.arenaAllyGameID != 0 && a.arenaAllyGameID != response.GameID {
		remembered = nil
	}
	a.arenaAlliesMu.RUnlock()
	// A missing gameflow member can still be an ally in the complete playerlist.
	for i, ally := range remembered {
		present := false
		for _, p := range response.Players {
			if ally.PUUID != "" && ally.PUUID == p.reference.PlayerRef {
				present = true
				break
			}
		}
		if !present && ally.PUUID != "" {
			if summoner, cap := loadGameplaySummoner(client, gameplayReference{PlayerRef: ally.PUUID, SummonerID: ally.SummonerID}); cap.State == capabilityAvailable {
				remembered[i].SummonerName = summoner.DisplayName
				remembered[i].GameName = summoner.GameName
				remembered[i].TagLine = summoner.TagLine
			}
		}
	}
	grouping := snapshot.Grouping
	groups := make([]string, len(raw))
	source := "live-client"
	order := false
	if len(grouping.ByIdentity) > 0 {
		for i, p := range response.Players {
			groups[i] = liveClientGroupingForPlayer(grouping, raw[i], p)
		}
	} else if len(snapshot.OrderedIdentities) > 0 {
		var ok bool
		grouping, ok = orderGroupsFromLiveClient(snapshot, squadSize)
		if !ok {
			response.ArenaGroupingUnavailable = true
			a.recordArenaOrderRejected(response, "live-client-order", "invalid-blocks")
			return
		}
		for i, p := range response.Players {
			groups[i] = liveClientGroupingForPlayer(grouping, raw[i], p)
		}
		source, order = "live-client-order", true
	} else {
		var ok bool
		groups, ok = a.carriedArenaSessionGroups(client, current, response)
		if !ok {
			groups, ok = arenaSessionOrderGroups(response.QueueID, raw)
		}
		if !ok {
			a.recordArenaOrderRejected(response, "live-client", "playerlist-unavailable")
			reason := "invalid-blocks"
			if !arenaRosterDivisible(len(raw), squadSize) {
				reason = "roster-not-divisible"
			}
			a.recordArenaOrderRejected(response, "session-order", reason)
			return
		}
		source, order = "session-order", true
	}
	if !validArenaGroupAssignments(groups, squadSize) {
		a.recordArenaOrderRejected(response, source, "invalid-blocks")
		return
	}
	verified, rejected := arenaAlliesCorroborate(groups, response.Players, remembered, squadSize, grouping, current.PUUID)
	if order && rejected {
		response.ArenaGroupingUnavailable = true
		a.recordArenaOrderRejected(response, source, "allies-cross-blocks")
		return
	}
	// A remembered full squad must be accounted for before accepting an inference.
	if order && len(remembered) >= squadSize && !verified {
		a.recordArenaOrderRejected(response, source, "allies-unresolved")
		return
	}
	if source == "live-client-order" && !verified {
		source = "live-client-order-unverified"
	}
	for i := range response.Players {
		response.Players[i].ArenaGroup = groups[i]
	}
	response.ArenaGrouped = validateArenaLiveGroups(response.Players, squadSize)
	response.ArenaMascotMapping = arenaLiveMascotMappingAllowed(response.ArenaGrouped, order, grouping)
	response.ArenaGroupSource = source
	if !order {
		response.ArenaGroupNames = maps.Clone(grouping.NameByGroup)
	}
	if order && response.ArenaGrouped {
		a.rememberArenaInference(client, current, response, grouping, verified, squadSize)
	}
}

// Inferred block numbers are not client subteam IDs, even if a later refactor
// retains otherwise valid mascot metadata on an order-derived grouping.
func arenaLiveMascotMappingAllowed(grouped, order bool, grouping liveClientArenaGrouping) bool {
	return grouped && !order && arenaLiveClientMascotMapping(grouping)
}

func (a *app) recordArenaOrderRejected(response *gameplayLiveResponse, source, reason string) {
	response.ArenaGroupingUnavailable = true
	response.ArenaGroupingRetryable = reason != "allies-cross-blocks"
	a.appendDiagnosticEvent(map[string]any{"event": "arena_group_order_rejected", "game_id": response.GameID, "queue_id": response.QueueID, "phase": response.Phase, "source": source, "reason": reason, "player_count": len(response.Players), "squad_size": arenaSquadSize(response.QueueID)})
}

type arenaTruthState struct {
	mu                    sync.Mutex
	records               []arenaInference
	phaseClient           *LCUClient
	phase                 string
	phaseQueue, phaseGame int64
	attempted             bool
	champClient           *LCUClient
	champGame             int64
	champions             []int64
}
type arenaInference struct {
	fromLoading                       bool
	selfPUUID                         string
	client                            *LCUClient
	gameID                            int64
	queueID                           int64
	serverID, source                  string
	grouping                          liveClientArenaGrouping
	byPUUID                           map[string]string
	playerCount, squadSize, selfBlock int
	verified, checked                 bool
}

func (a *app) rememberArenaInference(client *LCUClient, current Summoner, response *gameplayLiveResponse, grouping liveClientArenaGrouping, verified bool, squadSize int) {
	record := arenaInference{fromLoading: response.Phase == "GameStart", selfPUUID: current.PUUID, client: client, gameID: response.GameID, queueID: response.QueueID, serverID: clientTencentServerID(client), source: response.ArenaGroupSource, grouping: cloneLiveClientArenaGrouping(grouping), byPUUID: map[string]string{}, playerCount: len(response.Players), squadSize: squadSize, verified: verified}
	if grouping.IdentifiedPlayers > 0 {
		record.playerCount = grouping.IdentifiedPlayers
	}
	for _, p := range response.Players {
		if p.reference.PlayerRef != "" {
			record.byPUUID[p.reference.PlayerRef] = p.ArenaGroup
		}
		if p.IsCurrent {
			record.selfBlock, _ = strconv.Atoi(p.ArenaGroup)
		}
	}
	a.arenaTruth.mu.Lock()
	defer a.arenaTruth.mu.Unlock()
	for i, old := range a.arenaTruth.records {
		if old.client == client && old.gameID == record.gameID {
			// A partial frame cannot erase complete boundaries captured while loading.
			if old.source == "session-order" && record.source == "session-order" && len(old.byPUUID) > len(record.byPUUID) {
				return
			}
			record.checked = old.checked
			record.fromLoading = record.fromLoading || old.fromLoading
			a.arenaTruth.records[i] = record
			return
		}
	}
	a.arenaTruth.records = append(a.arenaTruth.records, record)
	if len(a.arenaTruth.records) > 5 {
		a.arenaTruth.records = append([]arenaInference(nil), a.arenaTruth.records[len(a.arenaTruth.records)-5:]...)
	}
}

// Called on the existing SGP history/detail path, before public references replace
// raw identities. Only counts and block IDs reach diagnostics, never identities.
func (a *app) checkArenaGroupTruth(client *LCUClient, serverID string, info *riotMatchInfo) {
	if info == nil || !isArenaQueue(info.QueueID, info.GameMode) {
		return
	}
	a.arenaTruth.mu.Lock()
	defer a.arenaTruth.mu.Unlock()
	for i := range a.arenaTruth.records {
		record := &a.arenaTruth.records[i]
		if record.client != client || record.gameID != info.GameID || record.queueID != info.QueueID || record.checked || !strings.EqualFold(record.serverID, serverID) {
			continue
		}
		blocks := make([]int64, record.playerCount/record.squadSize)
		counts := make([]int, len(blocks))
		seen := map[string]bool{}
		subteams := map[int64]int{}
		partition, identity := true, true
		selfSubteam := int64(0)
		matched := 0
		for _, p := range info.Participants {
			group := record.byPUUID[p.PUUID]
			if group == "" {
				group = liveClientGroupingForPlayer(record.grouping, lcuLivePlayer{}, gameplayLivePlayer{gameplayPlayer: gameplayPlayer{GameName: p.RiotIDGameName, TagLine: p.RiotIDTagline, DisplayName: p.SummonerName}})
			}
			block, err := strconv.Atoi(group)
			if err != nil || block < 1 || block > len(blocks) || p.PlayerSubteamID <= 0 {
				continue
			}
			key := p.PUUID
			if key == "" {
				key = p.RiotIDGameName + "#" + p.RiotIDTagline
			}
			if seen[key] {
				continue
			}
			seen[key] = true
			matched++
			counts[block-1]++
			if old := blocks[block-1]; old != 0 && old != p.PlayerSubteamID {
				partition = false
			}
			blocks[block-1] = p.PlayerSubteamID
			if old := subteams[p.PlayerSubteamID]; old != 0 && old != block {
				partition = false
			}
			subteams[p.PlayerSubteamID] = block
			if p.PUUID != "" && p.PUUID == record.selfPUUID {
				selfSubteam = p.PlayerSubteamID
				record.selfBlock = block
			}
		}
		// Partial stats are not a negative truth verdict; a later detail may be complete.
		if matched < record.playerCount {
			return
		}
		for k, n := range counts {
			if n != record.squadSize {
				partition = false
			}
			if blocks[k] != int64(k+1) {
				identity = false
			}
		}
		record.checked = true
		a.appendDiagnosticEvent(map[string]any{"event": "arena_group_truth_check", "game_id": record.gameID, "queue_id": record.queueID, "source": record.source, "player_count": record.playerCount, "squad_size": record.squadSize, "partition_match": partition, "block_to_subteam": blocks, "block_to_subteam_identity": partition && identity, "self_block": record.selfBlock, "self_subteam_id": selfSubteam, "verified_by_allies": record.verified})
	}
}

// Only end phases run this bounded read. SGP may publish stats a few seconds
// after the gameflow event; normal overview reads also perform the same check.
func (a *app) finishArenaGroupTruth(ctx context.Context, client *LCUClient) {
	if a.sgp == nil {
		return
	}
	a.arenaTruth.mu.Lock()
	var record arenaInference
	for _, r := range a.arenaTruth.records {
		if r.client == client && !r.checked {
			record = r
		}
	}
	a.arenaTruth.mu.Unlock()
	if record.gameID == 0 || record.selfPUUID == "" {
		return
	}
	for _, delay := range []time.Duration{0, 2 * time.Second, 5 * time.Second, 10 * time.Second} {
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		infos, _, _, err := a.sgp.matchHistoryOn(ctx, client, record.serverID, record.selfPUUID, 0, 5, false)
		if ctx.Err() != nil {
			return
		}
		if err == nil {
			for _, info := range infos {
				a.checkArenaGroupTruth(client, record.serverID, info)
			}
		}
		a.arenaTruth.mu.Lock()
		done := false
		for _, r := range a.arenaTruth.records {
			if r.client == client && r.gameID == record.gameID {
				done = r.checked
			}
		}
		a.arenaTruth.mu.Unlock()
		if done {
			return
		}
	}
}

// Reuse established boundaries by identity; never cut a shortened roster again.
func (a *app) carriedArenaSessionGroups(client *LCUClient, current Summoner, response *gameplayLiveResponse) ([]string, bool) {
	a.arenaTruth.mu.Lock()
	defer a.arenaTruth.mu.Unlock()
	for _, record := range a.arenaTruth.records {
		if response.GameID == 0 || record.client != client || record.gameID != response.GameID || record.queueID != response.QueueID || record.selfPUUID != current.PUUID || record.source != "session-order" || !record.fromLoading || !strings.EqualFold(record.serverID, clientTencentServerID(client)) {
			continue
		}
		groups := make([]string, len(response.Players))
		for i, player := range response.Players {
			groups[i] = record.byPUUID[player.reference.PlayerRef]
		}
		if validArenaGroupAssignments(groups, record.squadSize) {
			return groups, true
		}
	}
	return nil, false
}

func (a *app) observeArenaGroupingPhase(client *LCUClient, phase string) {
	t := &a.arenaTruth
	t.mu.Lock()
	previous := gameplayLiveResponse{Phase: t.phase, QueueID: t.phaseQueue, GameID: t.phaseGame}
	missed := t.phaseClient == client && t.phase != phase && (t.phase == "GameStart" || t.phase == "InProgress" || t.phase == "Reconnect") && isArenaQueue(t.phaseQueue, "") && !t.attempted
	changed := t.phaseClient != client || t.phase != phase
	if t.phaseClient != client || changed && phase == "ChampSelect" {
		t.phaseQueue, t.phaseGame = 0, 0
	}
	if changed {
		t.phaseClient, t.phase, t.attempted = client, phase, false
	}
	t.mu.Unlock()
	if changed && phase == "GameStart" {
		a.lcuGameflowShapeDiagnosticMu.Lock()
		delete(a.lcuGameflowShapeDiagnosticKeys, "GameStart-first:0")
		a.lcuGameflowShapeDiagnosticMu.Unlock()
	}
	if missed {
		a.recordArenaOrderRejected(&previous, "phase", "phase-not-attempted")
	}
}
func (a *app) noteArenaPhaseRoster(client *LCUClient, response *gameplayLiveResponse) {
	t := &a.arenaTruth
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.phaseClient == nil {
		t.phaseClient, t.phase = client, response.Phase
	}
	if t.phaseClient == client && t.phase == response.Phase {
		t.phaseQueue, t.phaseGame = response.QueueID, response.GameID
	}
}
func (a *app) markArenaGroupingAttempt(client *LCUClient, response *gameplayLiveResponse) {
	a.noteArenaPhaseRoster(client, response)
	t := &a.arenaTruth
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.phaseClient == client && t.phase == response.Phase {
		t.attempted = true
	}
}
func (a *app) rememberArenaChampOrder(client *LCUClient, session lcuChampSelectSession) {
	players := append([]lcuChampSelectPlayer(nil), session.MyTeam...)
	slices.SortStableFunc(players, func(a, b lcuChampSelectPlayer) int {
		if a.CellID == nil || b.CellID == nil {
			return 0
		}
		return int(*a.CellID - *b.CellID)
	})
	ids := make([]int64, len(players))
	for i, p := range players {
		ids[i] = p.ChampionID
	}
	t := &a.arenaTruth
	t.mu.Lock()
	defer t.mu.Unlock()
	t.champClient, t.champGame, t.champions = client, session.GameID, ids
}
func (a *app) compareArenaChampOrder(client *LCUClient, response *gameplayLiveResponse, raw []lcuLivePlayer, snapshot liveClientSnapshot) {
	if response.Phase != "InProgress" {
		return
	}
	t := &a.arenaTruth
	t.mu.Lock()
	ids := append([]int64(nil), t.champions...)
	valid := t.champClient == client && (t.champGame == 0 || t.champGame == response.GameID)
	t.mu.Unlock()
	if !valid || len(ids) == 0 {
		return
	}
	emit := func(source string, observed []int64) {
		matched, comparable := 0, 0
		counts := map[int64]int{}
		for _, id := range ids {
			counts[id]++
		}
		for i, id := range observed {
			if id <= 0 || counts[id] != 1 {
				continue
			}
			comparable++
			if i < len(ids) && ids[i] == id {
				matched++
			}
		}
		ratio := float64(0)
		if comparable > 0 {
			ratio = float64(matched) / float64(comparable)
		}
		a.recordDiagnostic(map[string]any{"event": "arena_champ_select_order_check", "game_id": response.GameID, "source": source, "champ_select_count": len(ids), "player_count": len(observed), "comparable_count": comparable, "matched_count": matched, "agreement": ratio})
	}
	observed := make([]int64, len(raw))
	for i, p := range raw {
		observed[i] = p.ChampionID
	}
	emit("session", observed)
	if len(snapshot.OrderedIdentities) > 0 {
		observed = make([]int64, len(snapshot.OrderedIdentities))
		for i, keys := range snapshot.OrderedIdentities {
			for j, p := range response.Players {
				g := liveClientArenaGrouping{ByIdentity: map[string]string{}}
				for _, key := range keys {
					g.ByIdentity[key] = "match"
				}
				if liveClientGroupingForPlayer(g, raw[j], p) != "" {
					observed[i] = raw[j].ChampionID
					break
				}
			}
		}
		emit("playerlist", observed)
	}
}
