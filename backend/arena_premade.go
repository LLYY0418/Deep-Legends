package main

import (
	"sort"
	"strconv"
	"strings"
	"sync"
)

// R263 P1：斗魂按「预组队」分卡片。
//
// gameflow 的 teamParticipantId 是组队（同一房间排进来）编号，不是斗魂小队编号。
// R95 真机样本里同一个编号下出现过 6 人、4 人（1750 每队 3 人），同一局 ChampSelect
// 与 InProgress 的分布也不一致，所以这里只把人数在 2..squadSize 之间、且不跨越「我的
// 小队」真值的组当作预组队；其余一律按未知处理并记诊断。LeagueAkari 的
// mergedPremadeTeamMap 只要求 length > 1，这一点我们更严格。
//
// 战绩推断对齐 Akari 的 _inferPremadeTeams：两名都不在我小队里的玩家，在最近战绩
// 中同一个 subteamId 一起出现 ≥ threshold 局，视为同一预组队。

type arenaPremadeInput struct {
	Ref               string
	TeamParticipantID int64
	Mine              bool
	Matches           []gameplayMatch
}

type arenaPremadeGroup struct {
	Members []int
	Source  string
}

type arenaPremadeDrop struct {
	Size   int    `json:"size"`
	Reason string `json:"reason"`
}

// arenaPremadeGroups returns disjoint premade groups among non-mine players.
// Members are input indices in ascending order.
func arenaPremadeGroups(inputs []arenaPremadeInput, squadSize, threshold int) ([]arenaPremadeGroup, []arenaPremadeDrop) {
	if squadSize < 2 {
		squadSize = 2
	}
	if threshold <= 0 {
		threshold = livePremadeMinSharedGames
	}
	var dropped []arenaPremadeDrop
	parent := make([]int, len(inputs))
	for index := range parent {
		parent[index] = index
	}
	var find func(int) int
	find = func(value int) int {
		if parent[value] != value {
			parent[value] = find(parent[value])
		}
		return parent[value]
	}
	union := func(left, right int) {
		leftRoot, rightRoot := find(left), find(right)
		if leftRoot != rightRoot {
			if leftRoot < rightRoot {
				parent[rightRoot] = leftRoot
			} else {
				parent[leftRoot] = rightRoot
			}
		}
	}
	sessionMember := make([]bool, len(inputs))
	inferredMember := make([]bool, len(inputs))

	// 1. Direct lobby signal.
	bySignal := map[int64][]int{}
	signals := []int64{}
	for index, input := range inputs {
		if input.TeamParticipantID <= 0 {
			continue
		}
		if _, seen := bySignal[input.TeamParticipantID]; !seen {
			signals = append(signals, input.TeamParticipantID)
		}
		bySignal[input.TeamParticipantID] = append(bySignal[input.TeamParticipantID], index)
	}
	for _, signal := range signals {
		members := bySignal[signal]
		if len(members) < 2 {
			continue
		}
		if len(members) > squadSize {
			dropped = append(dropped, arenaPremadeDrop{Size: len(members), Reason: "exceeds_squad"})
			continue
		}
		mine, others := 0, 0
		for _, member := range members {
			if inputs[member].Mine {
				mine++
			} else {
				others++
			}
		}
		if mine > 0 && others > 0 {
			dropped = append(dropped, arenaPremadeDrop{Size: len(members), Reason: "crosses_my_squad"})
			continue
		}
		if mine > 0 {
			continue // my own lobby is already part of 我的小队
		}
		for _, member := range members[1:] {
			union(members[0], member)
		}
		for _, member := range members {
			sessionMember[member] = true
		}
	}

	// 2. History inference (same subteam in shared past games).
	for left := 0; left < len(inputs); left++ {
		if inputs[left].Mine || !validPlayerReference(inputs[left].Ref) {
			continue
		}
		for right := left + 1; right < len(inputs); right++ {
			if inputs[right].Mine || !validPlayerReference(inputs[right].Ref) {
				continue
			}
			if arenaSharedSubteamGames(inputs[left], inputs[right]) >= threshold {
				union(left, right)
				inferredMember[left], inferredMember[right] = true, true
			}
		}
	}

	components := map[int][]int{}
	roots := []int{}
	for index := range inputs {
		if inputs[index].Mine {
			continue
		}
		root := find(index)
		if _, seen := components[root]; !seen {
			roots = append(roots, root)
		}
		components[root] = append(components[root], index)
	}
	groups := []arenaPremadeGroup{}
	for _, root := range roots {
		members := components[root]
		if len(members) < 2 {
			continue
		}
		if len(members) > squadSize {
			dropped = append(dropped, arenaPremadeDrop{Size: len(members), Reason: "inferred_exceeds_squad"})
			continue
		}
		session, inferred := false, false
		for _, member := range members {
			session = session || sessionMember[member]
			inferred = inferred || inferredMember[member]
		}
		source := "session"
		switch {
		case session && inferred:
			source = "both"
		case inferred:
			source = "inferred"
		}
		sort.Ints(members)
		groups = append(groups, arenaPremadeGroup{Members: members, Source: source})
	}
	sort.SliceStable(groups, func(i, j int) bool {
		if len(groups[i].Members) != len(groups[j].Members) {
			return len(groups[i].Members) > len(groups[j].Members)
		}
		return groups[i].Members[0] < groups[j].Members[0]
	})
	return groups, dropped
}

// arenaSharedSubteamGames counts distinct past games where both players were
// on the same Arena subteam, looking at both players' histories.
func arenaSharedSubteamGames(left, right arenaPremadeInput) int {
	shared := map[int64]bool{}
	scan := func(owner, other arenaPremadeInput) {
		for _, match := range owner.Matches {
			if match.GameID <= 0 || shared[match.GameID] {
				continue
			}
			subject := recentMatchSubject(match, owner.Ref)
			if subject == nil || subject.SubteamID <= 0 {
				continue
			}
			for index := range match.Participants {
				participant := &match.Participants[index]
				if participant.PlayerRef == other.Ref && participant.SubteamID == subject.SubteamID {
					shared[match.GameID] = true
					break
				}
			}
		}
	}
	scan(left, right)
	scan(right, left)
	return len(shared)
}

// arenaPremadeLetter returns A..Z, then 27, 28, ... for very large lobbies.
func arenaPremadeLetter(index int) string {
	if index >= 0 && index < 26 {
		return string(rune('A' + index))
	}
	return strconv.Itoa(index + 1)
}

type arenaPremadeLabelState struct {
	gameID        int64
	labels        map[string]string
	next          int
	champSelect   map[string]int64
	champSelectID int64
	lastSignature string
	predicted     map[int64][][]string
}

type liveR263State struct {
	mu      sync.Mutex
	premade arenaPremadeLabelState
	fame    arenaFameCache
	extend  arenaHistoryExtensionCache
	matches liveMatchIndex
}

var liveR263States sync.Map // *app -> *liveR263State

func (a *app) r263() *liveR263State {
	if value, ok := liveR263States.Load(a); ok {
		return value.(*liveR263State)
	}
	value, _ := liveR263States.LoadOrStore(a, &liveR263State{})
	return value.(*liveR263State)
}

// rememberArenaChampSelectSignals stores the champ-select teamParticipantId map
// for the game that is about to start. Gameflow keeps the previous game's data
// during champ select, so the caller must already have matched the game IDs.
func (a *app) rememberArenaChampSelectSignals(gameID int64, rows []liveRosterEntry) {
	if a == nil || gameID <= 0 || len(rows) == 0 {
		return
	}
	signals := map[string]int64{}
	for _, row := range rows {
		ref := strings.TrimSpace(row.player.PUUID)
		if validPlayerReference(ref) && row.player.TeamParticipantID > 0 {
			signals[ref] = row.player.TeamParticipantID
		}
	}
	if len(signals) == 0 {
		return
	}
	state := a.r263()
	state.mu.Lock()
	state.premade.champSelect = signals
	state.premade.champSelectID = gameID
	state.mu.Unlock()
}

// applyArenaPremadeSquads assigns ArenaSquadKey for every player of an Arena
// response. inputsByRef carries the gameflow signal and the player's history.
func (a *app) applyArenaPremadeSquads(response *gameplayLiveResponse, inputsByRef map[string]arenaPremadeInput) {
	if a == nil || response == nil || !isArenaQueue(response.QueueID, response.GameMode) {
		return
	}
	squadSize := arenaSquadSize(response.QueueID)
	if squadSize < 2 {
		squadSize = 3
		if len(response.Players) <= 16 {
			squadSize = 2
		}
	}
	response.ArenaSquadSize = squadSize
	state := a.r263()
	state.mu.Lock()
	if state.premade.gameID != response.GameID || state.premade.labels == nil {
		champSelect, champSelectID := state.premade.champSelect, state.premade.champSelectID
		state.premade = arenaPremadeLabelState{gameID: response.GameID, labels: map[string]string{}, predicted: state.premade.predicted}
		if champSelectID == response.GameID {
			state.premade.champSelect, state.premade.champSelectID = champSelect, champSelectID
		}
	}
	champSelect := state.premade.champSelect
	state.mu.Unlock()

	inputs := make([]arenaPremadeInput, len(response.Players))
	preferredChampSelect := 0
	// An in-game id shared with anyone whose champ-select id is known has been
	// observed to merge lobbies (R95: one id over six players). Members without
	// a champ-select id cannot be trusted under such an id.
	contaminated := map[int64]bool{}
	for index := range response.Players {
		ref := response.Players[index].reference.PlayerRef
		if signal := champSelect[ref]; signal > 0 && inputsByRef[ref].TeamParticipantID > 0 {
			contaminated[inputsByRef[ref].TeamParticipantID] = true
		}
	}
	for index := range response.Players {
		player := &response.Players[index]
		player.ArenaSquadKey, player.ArenaSquadSource = "", ""
		ref := player.reference.PlayerRef
		input := inputsByRef[ref]
		input.Ref = ref
		input.Mine = player.MySquad || player.IsCurrent
		if signal, ok := champSelect[ref]; ok && signal > 0 {
			if signal != input.TeamParticipantID {
				preferredChampSelect++
			}
			input.TeamParticipantID = signal
		} else if contaminated[input.TeamParticipantID] {
			input.TeamParticipantID = 0
		}
		inputs[index] = input
	}
	groups, dropped := arenaPremadeGroups(inputs, squadSize, livePremadeMinSharedGames)

	state.mu.Lock()
	keys := make([]string, len(groups))
	for groupIndex, group := range groups {
		refs := make([]string, 0, len(group.Members))
		for _, member := range group.Members {
			refs = append(refs, inputs[member].Ref)
		}
		sort.Strings(refs)
		key := strings.Join(refs, "\x00")
		label := state.premade.labels[key]
		if label == "" {
			label = arenaPremadeLetter(state.premade.next)
			state.premade.next++
			state.premade.labels[key] = label
		}
		keys[groupIndex] = label
	}
	predicted := make([][]string, 0, len(groups))
	for _, group := range groups {
		refs := []string{}
		for _, member := range group.Members {
			refs = append(refs, inputs[member].Ref)
		}
		predicted = append(predicted, refs)
	}
	if response.GameID > 0 && len(predicted) > 0 {
		if state.premade.predicted == nil {
			state.premade.predicted = map[int64][][]string{}
		}
		if len(state.premade.predicted) >= 8 {
			for gameID := range state.premade.predicted {
				if gameID != response.GameID {
					delete(state.premade.predicted, gameID)
					break
				}
			}
		}
		state.premade.predicted[response.GameID] = predicted
	}
	state.mu.Unlock()

	for index := range response.Players {
		if inputs[index].Mine {
			response.Players[index].ArenaSquadKey = "mine"
		}
	}
	for groupIndex, group := range groups {
		for _, member := range group.Members {
			response.Players[member].ArenaSquadKey = "premade:" + keys[groupIndex]
			response.Players[member].ArenaSquadSource = group.Source
		}
	}

	unknown := 0
	groupStats := make([]map[string]any, 0, len(groups))
	for _, group := range groups {
		groupStats = append(groupStats, map[string]any{"size": len(group.Members), "source": group.Source})
	}
	for index := range response.Players {
		if response.Players[index].ArenaSquadKey == "" {
			unknown++
		}
	}
	signature := response.Phase + "|" + strconv.Itoa(len(response.Players)) + "|" + strconv.Itoa(unknown)
	for _, key := range keys {
		signature += "|" + key
	}
	for _, drop := range dropped {
		signature += "|" + drop.Reason + strconv.Itoa(drop.Size)
	}
	state.mu.Lock()
	changed := state.premade.lastSignature != signature
	state.premade.lastSignature = signature
	state.mu.Unlock()
	if changed {
		a.recordDiagnostic(map[string]any{
			"event": "arena_premade_groups", "game_id": response.GameID, "phase": response.Phase,
			"squad_size": squadSize, "player_count": len(response.Players), "groups": groupStats,
			"dropped": dropped, "unknown_count": unknown, "champ_select_signal_used": preferredChampSelect,
		})
	}
}

// recordArenaPremadeTruth compares the premade groups predicted for a finished
// game with the real subteams of that game. It records counts only.
func (a *app) recordArenaPremadeTruth(info *riotMatchInfo) {
	if a == nil || info == nil || info.GameID <= 0 {
		return
	}
	state := a.r263()
	state.mu.Lock()
	predicted := state.premade.predicted[info.GameID]
	if predicted != nil {
		delete(state.premade.predicted, info.GameID)
	}
	state.mu.Unlock()
	if len(predicted) == 0 {
		return
	}
	subteams := map[string]int64{}
	for _, participant := range info.Participants {
		if ref := strings.TrimSpace(participant.PUUID); ref != "" && participant.PlayerSubteamID > 0 {
			subteams[ref] = participant.PlayerSubteamID
		}
	}
	same, split, unresolved := 0, 0, 0
	for _, group := range predicted {
		first := int64(0)
		known, agreed := 0, true
		for _, ref := range group {
			subteam := subteams[ref]
			if subteam <= 0 {
				continue
			}
			known++
			if first == 0 {
				first = subteam
			} else if subteam != first {
				agreed = false
			}
		}
		switch {
		case known < 2:
			unresolved++
		case agreed:
			same++
		default:
			split++
		}
	}
	a.recordDiagnostic(map[string]any{"event": "arena_premade_truth", "game_id": info.GameID, "groups": len(predicted), "same_subteam": same, "split": split, "unresolved": unresolved})
}
