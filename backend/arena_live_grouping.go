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

func (a *app) markRememberedArenaSquad(response *gameplayLiveResponse) {
	if a == nil || response == nil || !isArenaQueue(response.QueueID, response.GameMode) {
		return
	}
	a.arenaAlliesMu.RLock()
	remembered := append([]lcuLivePlayer(nil), a.arenaAllyPlayers...)
	gameID := a.arenaAllyGameID
	a.arenaAlliesMu.RUnlock()
	for i := range response.Players {
		response.Players[i].MySquad = false
	}
	if gameID != 0 && gameID != response.GameID {
		return
	}
	for i := range response.Players {
		player := response.Players[i]
		candidate := lcuLivePlayer{PUUID: player.reference.PlayerRef, SummonerID: player.reference.SummonerID, SummonerName: player.DisplayName, GameName: player.GameName, TagLine: player.TagLine}
		for _, ally := range remembered {
			matched := arenaSameIdentity(arenaObservedPlayer(ally), arenaObservedPlayer(candidate))
			if ally.SummonerID > 0 && candidate.SummonerID > 0 && ally.SummonerID == candidate.SummonerID {
				matched = true
			}
			if matched {
				response.Players[i].MySquad = true
				break
			}
		}
	}
}

func (a *app) applyArenaLiveGrouping(client *LCUClient, current Summoner, response *gameplayLiveResponse, raw []lcuLivePlayer, snapshot liveClientSnapshot) {
	squadSize := arenaSquadSize(response.QueueID)
	observedSession := *response
	observedSession.Players = append([]gameplayLivePlayer(nil), response.Players...)
	a.markRememberedArenaSquad(response)
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
	// Restore only known squad members corroborated by the live playerlist.
	// This is identity matching, never a position/order-derived assignment.
	for _, ally := range remembered {
		identity := arenaObservedPlayer(ally)
		present, observed := false, false
		for _, p := range response.Players {
			present = present || arenaSameIdentity(identity, arenaObservedPlayer(lcuLivePlayer{PUUID: p.reference.PlayerRef, GameName: p.GameName, TagLine: p.TagLine, SummonerName: p.DisplayName}))
		}
		for _, keys := range snapshot.OrderedIdentities {
			observed = observed || arenaSameIdentity(identity, arenaObservedIdentity{Keys: arenaExactNameKeys(keys)})
		}
		if present || !observed {
			continue
		}
		ref := normalizeGameplayReference(gameplayReference{PlayerRef: ally.PUUID, SummonerID: ally.SummonerID, GameName: ally.GameName, TagLine: ally.TagLine, DisplayName: ally.SummonerName, ServerID: clientTencentServerID(client)})
		response.Players = append(response.Players, gameplayLivePlayer{gameplayPlayer: gameplayPlayer{PlayerRef: ally.PUUID, GameName: ally.GameName, TagLine: ally.TagLine, DisplayName: ally.SummonerName, IsCurrent: ally.PUUID == current.PUUID, reference: ref}, MySquad: true, IsAlly: true, TeamID: 100, ChampionID: ally.ChampionID, ChampionLocked: ally.ChampionID > 0, HistoryState: "unavailable"})
	}
	a.markRememberedArenaSquad(response)
	// Verify what was actually marked, not merely what was remembered.
	marked := []lcuLivePlayer{}
	for _, p := range response.Players {
		if p.MySquad {
			marked = append(marked, lcuLivePlayer{PUUID: p.reference.PlayerRef, GameName: p.GameName, TagLine: p.TagLine, SummonerName: p.DisplayName})
		}
	}
	a.rememberArenaObservation(client, current, &observedSession, marked, snapshot)
	grouping := snapshot.Grouping
	// Array order and party IDs are not squad evidence. Only explicit client
	// subteam fields may populate ArenaGroup; known allies remain independently marked.
	if len(grouping.ByIdentity) == 0 {
		a.recordArenaOrderRejected(response, "none", "order-inference-disabled")
		// Retry unavailable playerlists for observation, never to infer squads.
		response.ArenaGroupingRetryable = len(snapshot.OrderedIdentities) == 0
		return
	}
	groups := make([]string, len(response.Players))
	for i, p := range response.Players {
		var player lcuLivePlayer
		if i < len(raw) {
			player = raw[i]
		}
		groups[i] = liveClientGroupingForPlayer(grouping, player, p)
	}
	if !validArenaGroupAssignments(groups, squadSize) {
		a.recordArenaOrderRejected(response, "live-client", "invalid-subteams")
		return
	}
	// Explicit fields still cannot contradict the known squad. A complete
	// remembered squad must also resolve before its candidate groups are trusted.
	verified, rejected := arenaAlliesCorroborate(groups, response.Players, remembered, squadSize, grouping, current.PUUID)
	if rejected || len(remembered) >= squadSize && !verified {
		a.recordArenaOrderRejected(response, "live-client", "allies-cross-blocks")
		return
	}
	for i := range response.Players {
		response.Players[i].ArenaGroup = groups[i]
	}
	response.ArenaGrouped = true
	response.ArenaGroupingUnavailable = false
	response.ArenaMascotMapping = arenaLiveMascotMappingAllowed(true, false, grouping)
	response.ArenaGroupSource = "live-client"
	response.ArenaGroupNames = maps.Clone(grouping.NameByGroup)
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
	fromLoading                          bool
	selfPUUID                            string
	client                               *LCUClient
	gameID                               int64
	queueID                              int64
	serverID, source                     string
	grouping                             liveClientArenaGrouping
	byPUUID                              map[string]string
	playerCount, squadSize, selfBlock    int
	verified, checked                    bool
	noneReported, truthObserved          bool
	sessionIdentities, mySquadIdentities []arenaObservedIdentity
	playerlistIdentities                 [][]string
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
			record.truthObserved, record.noneReported = old.truthObserved, old.noneReported
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

type arenaGroupTruthParticipant struct {
	PUUID        string
	RiotID       string
	SummonerName string
	SubteamID    int64
}

type arenaGroupTruthInput struct {
	Source       string
	GameID       int64
	QueueID      int64
	GameMode     string
	Participants []arenaGroupTruthParticipant
}

func arenaGroupTruthFromRiot(info *riotMatchInfo, source string) *arenaGroupTruthInput {
	if info == nil {
		return nil
	}
	truth := &arenaGroupTruthInput{Source: source, GameID: info.GameID, QueueID: info.QueueID, GameMode: info.GameMode}
	for _, p := range info.Participants {
		truth.Participants = append(truth.Participants, arenaGroupTruthParticipant{PUUID: p.PUUID, RiotID: strings.TrimSpace(p.RiotIDGameName) + "#" + strings.TrimSpace(p.RiotIDTagline), SummonerName: p.SummonerName, SubteamID: p.PlayerSubteamID})
	}
	return truth
}

// SGP already decodes its payload into riotMatchInfo. The source is determined
// by the caller, not by a Tencent region-name heuristic.
func (a *app) checkArenaGroupTruth(client *LCUClient, serverID string, info *riotMatchInfo) {
	a.checkArenaGroupTruthInput(client, serverID, arenaGroupTruthFromRiot(info, "sgp"))
}

func (a *app) checkArenaGroupTruthInput(client *LCUClient, serverID string, truth *arenaGroupTruthInput) {
	if truth == nil || !isArenaQueue(truth.QueueID, truth.GameMode) {
		return
	}
	a.arenaTruth.mu.Lock()
	defer a.arenaTruth.mu.Unlock()
	for i := range a.arenaTruth.records {
		r := &a.arenaTruth.records[i]
		if r.client != client || r.gameID != truth.GameID || r.queueID != truth.QueueID || r.checked || !strings.EqualFold(r.serverID, serverID) {
			continue
		}
		a.emitArenaObservationTruth(r, truth)
	}
}

// Only end phases run this bounded read. SGP may publish stats a few seconds
// after the gameflow event; normal overview reads also perform the same check.
func (a *app) finishArenaGroupTruth(ctx context.Context, client *LCUClient) {
	a.arenaTruth.mu.Lock()
	var record arenaInference
	for _, r := range a.arenaTruth.records {
		if r.client == client && !r.checked {
			record = r
		}
	}
	a.arenaTruth.mu.Unlock()
	if record.gameID == 0 {
		return
	}
	// Missing transport, cancellation and unpublished stats must all leave evidence.
	defer a.checkArenaGroupTruthInput(client, record.serverID, &arenaGroupTruthInput{Source: "none", GameID: record.gameID, QueueID: record.queueID, GameMode: "CHERRY"})
	if a.sgp == nil || record.selfPUUID == "" {
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
		a.recordDiagnostic(map[string]any{"event": "arena_champ_select_order_check", "game_id": response.GameID, "source": source, "champ_select_count": len(ids), "player_count": len(observed), "comparable_count": comparable, "matched_count": matched, "agreement": ratio, "conclusive": arenaChampOrderConclusive(comparable, arenaSquadSize(response.QueueID))})
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
