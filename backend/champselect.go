package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync/atomic"
	"time"
)

const (
	champSelectAPI         = "/lol-champ-select/v1"
	champSelectLegacyAPI   = "/lol-champ-select-legacy/v1"
	champSelectActionBan   = "champselect-ban"
	champSelectActionPick  = "champselect-pick"
	champSelectActionBench = "champselect-bench"
	champSelectActionTrade = "champselect-trade"
)

type champSelectSettings struct {
	Enabled bool                              `json:"enabled"`
	Groups  map[string]champSelectGroupConfig `json:"groups"`
}

type champSelectGroupConfig struct {
	Ban   champSelectSideConfig  `json:"ban"`
	Pick  champSelectSideConfig  `json:"pick"`
	Bench champSelectBenchConfig `json:"bench"`
}

type champSelectSideConfig struct {
	Enabled             bool               `json:"enabled"`
	Strategy            string             `json:"strategy"`
	DelayMS             int                `json:"delayMs"`
	LockDelayMS         *int               `json:"lockDelayMs,omitempty"`
	AvoidTeammateIntent bool               `json:"avoidTeammateIntent"`
	Champions           map[string][]int64 `json:"champions"`
	// Migration backup only; never used as an executable pool.
	LegacyLaneChampions map[string][]int64 `json:"legacyLaneChampions,omitempty"`
}

type champSelectBenchConfig struct {
	Enabled     bool `json:"enabled"`
	HoldMS      int  `json:"holdMs"`
	PreferFirst bool `json:"preferFirst"`
	HandleTrade bool `json:"handleTrade"`
}

type champSelectGroupDefinition struct {
	GroupID   string   `json:"groupId"`
	Name      string   `json:"name"`
	BanLimit  int      `json:"banLimit"`
	PickLimit int      `json:"pickLimit"`
	Positions []string `json:"positions"` // Pick pools; bans always use the shared default pool.
	HasBan    bool     `json:"hasBan"`
	HasBench  bool     `json:"hasBench"`
	HasTrade  bool     `json:"hasTrade"`
}

var champSelectGroupDefinitions = []champSelectGroupDefinition{
	{GroupID: "ranked", Name: "排位（单双 / 灵活）", BanLimit: 5, PickLimit: 5, Positions: []string{"top", "jungle", "middle", "bottom", "utility", "default"}, HasBan: true, HasTrade: true},
	{GroupID: "normal", Name: "普通征召", BanLimit: 5, PickLimit: 5, Positions: []string{"default"}, HasBan: true, HasTrade: true},
	{GroupID: "aram", Name: "大乱斗类", BanLimit: 0, PickLimit: 5, Positions: []string{"default"}, HasBench: true, HasTrade: true},
	{GroupID: "arena", Name: "斗魂竞技场", BanLimit: 3, PickLimit: 3, Positions: []string{"default"}, HasBan: true},
	{GroupID: "event", Name: "活动模式", BanLimit: 3, PickLimit: 3, Positions: []string{"default"}, HasBan: true, HasTrade: true},
	{GroupID: "practice", Name: "人机 / 自定义", BanLimit: 5, PickLimit: 5, Positions: []string{"default"}, HasBan: true, HasTrade: true},
}

type champSelectSubmitRecord struct {
	ChampionID   int64
	Completed    bool
	At           time.Time
	TraceID      string
	Confirmed    bool
	ConfirmedAt  time.Time
	HoverCleared bool
	Decision     champSelectDecision
}

type champSelectGridChampion struct {
	ID              int64                          `json:"id"`
	ChampionID      int64                          `json:"championId"`
	Owned           bool                           `json:"owned"`
	TeammatePicked  bool                           `json:"-"`
	TeammateIntent  bool                           `json:"-"`
	LocalIntent     bool                           `json:"-"`
	SelectionStatus champSelectGridSelectionStatus `json:"selectionStatus"`
}

func (champion champSelectGridChampion) resolvedID() int64 {
	if champion.ChampionID != 0 {
		return champion.ChampionID
	}
	return champion.ID
}

type champSelectGridSelectionStatus struct {
	IsBanned              bool `json:"isBanned"`
	PickIntented          bool `json:"pickIntented"`
	PickIntentedByMe      bool `json:"pickIntentedByMe"`
	PickedByOtherOrBanned bool `json:"pickedByOtherOrBanned"`
	SelectedByMe          bool `json:"selectedByMe"`
}

type champSelectDecision struct {
	TraceID            string
	SessionTrace       string
	Key                string
	Action             string
	ActionID           int64
	ChampionID         int64
	Completed          bool
	SessionAPI         string
	GameID             int64
	QueueID            int64
	LocalCellID        int64
	Intent             bool
	ForceHover         bool
	AvailabilitySource string
	RuntimeID          string
	FromChampionID     int64
}

type champSelectRuntimeRecord struct {
	At         time.Time `json:"at"`
	Kind       string    `json:"kind"`
	Message    string    `json:"message"`
	ChampionID int64     `json:"championId,omitempty"`
}

type champSelectRuntimeStore struct {
	runtimeID        string
	attempts         map[string]int
	banEvidence      map[int64]struct{}
	banEvidenceScope string
	banSource        string
	writeContext     context.Context
	writeCancel      context.CancelFunc
	phase            string
	active           bool
	processing       bool
	queued           bool
	groupID          string
	position         string
	remainingMS      int
	benchEnabled     bool
	sessionPaused    bool
	gridLoaded       bool
	grid             map[int64]champSelectGridChampion
	pickable         map[int64]struct{}
	bannable         map[int64]struct{}
	decision         map[string]champSelectDecision
	submitted        map[int64]champSelectSubmitRecord
	inFlight         map[int64]bool
	takeover         map[string]bool
	takeoverChampion map[string]int64
	benchObserved    bool
	benchChampionID  int64
	benchSwap        *champSelectBenchSubmission
	exhausted        map[string]bool
	benchFirstSeen   map[int64]time.Time
	records          []champSelectRuntimeRecord
	lastSession      lcuChampSelectSession
	diagnosticKey    string
	readFailures     map[string]bool
	candidateKey     string
	skippedKey       string
	sourceKey        string
}

type champSelectRuntimeResponse struct {
	TimerPhase         string                     `json:"timerPhase,omitempty"`
	PickIntent         bool                       `json:"pickIntent"`
	BanSource          string                     `json:"banSource,omitempty"`
	ActionType         string                     `json:"actionType,omitempty"`
	Active             bool                       `json:"active"`
	Phase              string                     `json:"phase"`
	GroupID            string                     `json:"groupId,omitempty"`
	Position           string                     `json:"position,omitempty"`
	RemainingMS        int                        `json:"remainingMs,omitempty"`
	BenchEnabled       bool                       `json:"benchEnabled"`
	SessionPaused      bool                       `json:"sessionPaused"`
	Unsupported        bool                       `json:"unsupported"`
	BanCapabilityKnown bool                       `json:"banCapabilityKnown"`
	HasBanAction       bool                       `json:"hasBanAction"`
	BanStates          map[string]string          `json:"banStates"`
	PickStates         map[string]string          `json:"pickStates"`
	Owned              map[string]bool            `json:"owned"`
	ActiveBanID        int64                      `json:"activeBanId,omitempty"`
	ActivePickID       int64                      `json:"activePickId,omitempty"`
	Records            []champSelectRuntimeRecord `json:"records"`
}

type champSelectOngoingSwap struct {
	ID                  int64  `json:"id"`
	RequesterChampionID int64  `json:"requesterChampionId"`
	ResponderChampionID int64  `json:"responderChampionId"`
	State               string `json:"state"`
}

func newChampSelectRuntimeStore() champSelectRuntimeStore {
	return champSelectRuntimeStore{
		runtimeID:        newDiagnosticTrace("cs-runtime"),
		attempts:         map[string]int{},
		banEvidence:      map[int64]struct{}{},
		grid:             map[int64]champSelectGridChampion{},
		pickable:         map[int64]struct{}{},
		bannable:         map[int64]struct{}{},
		decision:         map[string]champSelectDecision{},
		submitted:        map[int64]champSelectSubmitRecord{},
		inFlight:         map[int64]bool{},
		takeover:         map[string]bool{},
		takeoverChampion: map[string]int64{},
		exhausted:        map[string]bool{},
		benchFirstSeen:   map[int64]time.Time{},
	}
}

func champSelectGroupDefinitionFor(groupID string) (champSelectGroupDefinition, bool) {
	for _, definition := range champSelectGroupDefinitions {
		if definition.GroupID == groupID {
			return definition, true
		}
	}
	return champSelectGroupDefinition{}, false
}

func champSelectGroupDefinitionsForClient() []champSelectGroupDefinition {
	result := make([]champSelectGroupDefinition, len(champSelectGroupDefinitions))
	copy(result, champSelectGroupDefinitions)
	for index := range result {
		result[index].Positions = append([]string(nil), result[index].Positions...)
	}
	return result
}

func defaultChampSelectSettings() champSelectSettings {
	settings := champSelectSettings{Enabled: true, Groups: make(map[string]champSelectGroupConfig, len(champSelectGroupDefinitions))}
	for _, definition := range champSelectGroupDefinitions {
		lockDelay := 10000
		settings.Groups[definition.GroupID] = champSelectGroupConfig{
			Ban:   champSelectSideConfig{Strategy: "show-then-lock", DelayMS: 2000, AvoidTeammateIntent: true, Champions: map[string][]int64{"default": {}}},
			Pick:  champSelectSideConfig{Strategy: "show-then-lock", DelayMS: 500, LockDelayMS: &lockDelay, AvoidTeammateIntent: true, Champions: emptyChampSelectPools(definition)},
			Bench: champSelectBenchConfig{Enabled: definition.HasBench, HoldMS: 1000, PreferFirst: definition.HasBench},
		}
	}
	return settings
}

func emptyChampSelectPools(definition champSelectGroupDefinition) map[string][]int64 {
	pools := make(map[string][]int64, len(definition.Positions))
	for _, position := range definition.Positions {
		pools[position] = []int64{}
	}
	return pools
}

func normalizeChampSelectSettings(settings champSelectSettings) champSelectSettings {
	defaults := defaultChampSelectSettings()
	normalized := champSelectSettings{Enabled: settings.Enabled, Groups: make(map[string]champSelectGroupConfig, len(champSelectGroupDefinitions))}
	for _, definition := range champSelectGroupDefinitions {
		fallback := defaults.Groups[definition.GroupID]
		group, present := settings.Groups[definition.GroupID]
		if !present {
			normalized.Groups[definition.GroupID] = fallback
			continue
		}
		group.Ban = normalizeChampSelectSide(group.Ban, fallback.Ban, definition, "ban")
		group.Pick = normalizeChampSelectSide(group.Pick, fallback.Pick, definition, "pick")
		// One visible per-group control governs both sequences. Preserve an
		// existing enabled avoidance policy when migrating split settings.
		avoid := group.Ban.AvoidTeammateIntent || group.Pick.AvoidTeammateIntent
		group.Ban.AvoidTeammateIntent, group.Pick.AvoidTeammateIntent = avoid, avoid
		group.Bench.HoldMS = clampInt(group.Bench.HoldMS, 1000, 10000, 1000)
		if !definition.HasBan {
			group.Ban.Enabled = false
			group.Ban.Champions = emptyChampSelectPools(definition)
		}
		if !definition.HasBench {
			group.Bench.Enabled = false
		}
		if !definition.HasBench && !definition.HasTrade {
			group.Bench.PreferFirst = false
		}
		if !definition.HasTrade {
			group.Bench.HandleTrade = false
		}
		normalized.Groups[definition.GroupID] = group
	}
	return normalized
}

func normalizeChampSelectSide(side, fallback champSelectSideConfig, definition champSelectGroupDefinition, kind string) champSelectSideConfig {
	switch side.Strategy {
	case "show-only", "show-then-lock", "lock-now":
	default:
		side.Strategy = fallback.Strategy
	}
	limit := definition.BanLimit
	if kind == "pick" {
		limit = definition.PickLimit
	}
	side.DelayMS = clampInt(side.DelayMS, 0, 10000, 0)
	lockDelay := champSelectLockDelay(side)
	side.LockDelayMS = &lockDelay
	if kind == "ban" && len(definition.Positions) > 1 {
		side = migrateChampSelectSharedBan(side, definition)
	} else {
		side.LegacyLaneChampions = nil
	}
	allowed := make(map[string]struct{}, len(definition.Positions)+1)
	positions := definition.Positions
	if kind == "ban" {
		positions = []string{"default"}
	}
	for _, position := range positions {
		allowed[position] = struct{}{}
	}
	pools := make(map[string][]int64, len(allowed))
	for position := range allowed {
		seen := map[int64]struct{}{}
		for _, championID := range side.Champions[position] {
			if limit <= 0 {
				break
			}
			if championID <= 0 && !(definition.GroupID == "arena" && kind == "pick" && championID == -3) {
				continue
			}
			if _, duplicate := seen[championID]; duplicate {
				continue
			}
			seen[championID] = struct{}{}
			pools[position] = append(pools[position], championID)
			if len(pools[position]) == limit {
				break
			}
		}
		if pools[position] == nil {
			pools[position] = []int64{}
		}
	}
	side.Champions = pools
	return side
}

func cloneChampSelectSettings(settings champSelectSettings) champSelectSettings {
	clone := champSelectSettings{Enabled: settings.Enabled, Groups: make(map[string]champSelectGroupConfig, len(settings.Groups))}
	for groupID, group := range settings.Groups {
		group.Ban.Champions = cloneChampSelectPools(group.Ban.Champions)
		group.Ban.LegacyLaneChampions = cloneChampSelectPools(group.Ban.LegacyLaneChampions)
		group.Pick.Champions = cloneChampSelectPools(group.Pick.Champions)
		if group.Ban.LockDelayMS != nil {
			delay := *group.Ban.LockDelayMS
			group.Ban.LockDelayMS = &delay
		}
		if group.Pick.LockDelayMS != nil {
			delay := *group.Pick.LockDelayMS
			group.Pick.LockDelayMS = &delay
		}
		clone.Groups[groupID] = group
	}
	return clone
}

func cloneChampSelectPools(pools map[string][]int64) map[string][]int64 {
	clone := make(map[string][]int64, len(pools))
	for position, champions := range pools {
		clone[position] = append([]int64(nil), champions...)
	}
	return clone
}

func champSelectGroupID(queueID int64, gameMode string, mapID int64) string {
	mode := strings.ToUpper(strings.TrimSpace(gameMode))
	if group := champSelectQueueOverride(queueID, mode); group != "" {
		return group
	}
	if isARAMFamilyGameMode(mode) {
		return "aram"
	}
	modeGroup := queueModeGroupFor(queueID, mode, mapID)
	switch modeGroup {
	case "aram", "hextech-aram", "hextech-classic", "hextech-qualifier":
		return "aram"
	case "arena":
		return "arena"
	case "bots", "doombots", "custom":
		return "practice"
	case "urf", "nexus-blitz":
		return "event"
	}
	switch mode {
	case "CHERRY":
		if mapID == 0 || mapID == 30 {
			return "arena"
		}
	case "URF", "ARURF", "ONEFORALL", "ULTBOOK", "NEXUSBLITZ":
		return "event"
	case "BOT", "TUTORIAL", "PRACTICETOOL", "CUSTOM", "CUSTOM_GAME":
		return "practice"
	case "CLASSIC":
		if missingQueueID(queueID) {
			return "practice"
		}
	}
	if definition, ok := supportedQueueDefinition(queueID); ok {
		switch definition.ModeGroup {
		case "match", "clash":
			return "normal"
		case "bots", "doombots", "custom":
			return "practice"
		}
	}
	return ""
}

func champSelectPoolPosition(definition champSelectGroupDefinition, assigned string) string {
	assigned = strings.ToLower(strings.TrimSpace(assigned))
	if slices.Contains(definition.Positions, assigned) {
		return assigned
	}
	return "default"
}

func champSelectConfiguredPool(side champSelectSideConfig, definition champSelectGroupDefinition, assigned, kind string) ([]int64, string) {
	position := champSelectPoolPosition(definition, assigned)
	if kind == "ban" {
		position = "default"
	}
	return append([]int64(nil), side.Champions[position]...), position
}

func chooseChampSelectCandidate(side string, champions []int64, available map[int64]struct{}, grid map[int64]champSelectGridChampion, avoidTeammateIntent bool, arena bool) (int64, map[int64]string) {
	reasons := make(map[int64]string, len(champions))
	for _, championID := range champions {
		if side == "pick" && arena && championID == -3 {
			return championID, reasons
		}
		champion, found := grid[championID]
		if !found {
			reasons[championID] = "unknown"
			continue
		}
		if champion.TeammatePicked {
			reasons[championID] = "teammate-picked"
			continue
		}
		if side == "ban" && champion.LocalIntent {
			reasons[championID] = "own-intent"
			continue
		}
		if _, ok := available[championID]; !ok {
			if champion.SelectionStatus.IsBanned || champion.SelectionStatus.PickedByOtherOrBanned {
				reasons[championID] = "gone"
			} else {
				reasons[championID] = "unavailable"
			}
			continue
		}
		if side == "ban" && champion.SelectionStatus.IsBanned {
			reasons[championID] = "gone"
			continue
		}
		if side == "pick" && champion.SelectionStatus.PickedByOtherOrBanned {
			reasons[championID] = "gone"
			continue
		}
		if avoidTeammateIntent && (champion.TeammateIntent || champion.SelectionStatus.PickIntented && !champion.SelectionStatus.PickIntentedByMe) {
			reasons[championID] = "intent"
			continue
		}
		return championID, reasons
	}
	return 0, reasons
}

// Session selections are independent of the grid flags. In custom games the
// grid/pickable endpoint may still allow a champion already held by a bot.
// Never turn a missing grid entry into an eligible champion via team intent.
func mergeChampSelectTeamState(grid map[int64]champSelectGridChampion, session lcuChampSelectSession) {
	for _, player := range session.MyTeam {
		if session.LocalPlayerCellID != nil && player.CellID != nil && *player.CellID == *session.LocalPlayerCellID {
			if champion, ok := grid[player.ChampionPickIntent]; ok {
				champion.LocalIntent = true
				grid[player.ChampionPickIntent] = champion
			}
			continue
		}
		if champion, ok := grid[player.ChampionID]; ok && player.ChampionID > 0 {
			champion.TeammatePicked = true
			grid[player.ChampionID] = champion
		}
		if champion, ok := grid[player.ChampionPickIntent]; ok && player.ChampionPickIntent > 0 {
			champion.TeammateIntent = true
			grid[player.ChampionPickIntent] = champion
		}
	}
	for _, group := range session.Actions {
		for _, action := range group {
			if action.Type == "pick" && action.ChampionID > 0 {
				if champion, ok := grid[action.ChampionID]; ok {
					local := session.LocalPlayerCellID != nil && action.ActorCellID == *session.LocalPlayerCellID
					if local {
						champion.LocalIntent = true
					} else if action.Completed {
						champion.SelectionStatus.PickedByOtherOrBanned = true
					} else {
						for _, player := range session.MyTeam {
							if player.CellID != nil && *player.CellID == action.ActorCellID {
								champion.TeammateIntent = true
							}
						}
					}
					grid[action.ChampionID] = champion
				}
			}
			if action.Completed && action.Type == "ban" && action.ChampionID > 0 {
				if champion, ok := grid[action.ChampionID]; ok {
					champion.SelectionStatus.IsBanned = true
					champion.SelectionStatus.PickedByOtherOrBanned = true
					grid[action.ChampionID] = champion
				}
			}
		}
	}
}

func champSelectDelay(configuredMS, remainingMS int) int {
	if configuredMS < 0 {
		configuredMS = 0
	}
	if remainingMS >= 0 && configuredMS > remainingMS {
		return remainingMS
	}
	return configuredMS
}

func firstLocalChampSelectAction(session lcuChampSelectSession) (lcuChampSelectAction, bool) {
	if session.LocalPlayerCellID == nil {
		return lcuChampSelectAction{}, false
	}
	var selected lcuChampSelectAction
	found := false
	for _, group := range session.Actions {
		for _, action := range group {
			side := strings.ToLower(action.Type)
			if action.ActorCellID == *session.LocalPlayerCellID && !action.Completed && action.IsInProgress && (side == "ban" || side == "pick") {
				if found {
					return lcuChampSelectAction{}, false // Ambiguous client state: never guess a turn.
				}
				selected, found = action, true
			}
		}
	}
	return selected, found
}

// Custom draft is backed by the legacy implementation on some clients. Only
// use that module when the client explicitly marks it active and its session
// is the same game/player. Never substitute pickable IDs for bannable IDs.
func champSelectSessionSource(ctx context.Context, client *LCUClient, session lcuChampSelectSession) (lcuChampSelectSession, string, string) {
	if !queueUsesLegacyChampSelect(session.QueueID) {
		return session, champSelectAPI, "standard-queue"
	}
	if session.GameID <= 0 || session.LocalPlayerCellID == nil {
		return session, champSelectAPI, "missing-session-key"
	}
	var active bool
	if client.RequestJSON(ctx, http.MethodGet, champSelectLegacyAPI+"/implementation-active", nil, &active) != nil {
		return session, champSelectAPI, "legacy-probe-failed"
	}
	if !active {
		return session, champSelectAPI, "legacy-inactive"
	}
	var legacy lcuChampSelectSession
	if client.RequestJSON(ctx, http.MethodGet, champSelectLegacyAPI+"/session", nil, &legacy) != nil {
		return session, champSelectAPI, "legacy-session-failed"
	}
	if legacy.GameID != session.GameID || legacy.LocalPlayerCellID == nil || *legacy.LocalPlayerCellID != *session.LocalPlayerCellID ||
		(!unspecifiedQueueID(legacy.QueueID) && legacy.QueueID != session.QueueID) {
		return session, champSelectAPI, "legacy-session-mismatch"
	}
	legacy.QueueID = session.QueueID
	return legacy, champSelectLegacyAPI, "legacy-verified"
}

func champSelectSessionHasActionType(session lcuChampSelectSession, actionType string) bool {
	for _, group := range session.Actions {
		for _, action := range group {
			if strings.EqualFold(strings.TrimSpace(action.Type), actionType) {
				return true
			}
		}
	}
	return false
}

func champSelectAssignedPosition(session lcuChampSelectSession) string {
	if session.LocalPlayerCellID == nil {
		return ""
	}
	for _, player := range session.MyTeam {
		if player.CellID != nil && *player.CellID == *session.LocalPlayerCellID {
			return strings.ToLower(strings.TrimSpace(player.AssignedPosition))
		}
	}
	return ""
}

func champSelectCurrentChampion(session lcuChampSelectSession) int64 {
	if session.LocalPlayerCellID == nil {
		return 0
	}
	for _, player := range session.MyTeam {
		if player.CellID != nil && *player.CellID == *session.LocalPlayerCellID {
			return player.ChampionID
		}
	}
	return 0
}

func champSelectRemainingMS(session lcuChampSelectSession) int {
	value := session.Timer.AdjustedTimeLeftInPhase
	if value < 0 {
		return 0
	}
	if value == 0 && session.Timer.Phase == "" && session.Timer.TotalTimeInPhase == 0 {
		return -1
	}
	return int(value)
}

func (r *watchRunner) handleChampSelectPhase(phase string) {
	if r == nil {
		return
	}
	phase = strings.TrimSpace(phase)
	r.mu.Lock()
	previous := r.champSelect.phase
	r.champSelect.phase = phase
	entering := strings.EqualFold(phase, "ChampSelect") && !strings.EqualFold(previous, "ChampSelect")
	leaving := !strings.EqualFold(phase, "ChampSelect") && strings.EqualFold(previous, "ChampSelect")
	if entering || leaving {
		if pending := r.pending["position-broadcast"]; pending != nil {
			pending.cancel()
			delete(r.pending, "position-broadcast")
		}
		r.broadcastForSession = false
		if entering {
			r.champDiagnosticSession = newDiagnosticTrace("cs-session")
		}
		r.resetChampSelectRuntimeLocked()
		r.champSelect.phase = phase
	}
	r.mu.Unlock()
	if entering || leaving {
		r.champDiagnostic("phase", "transition", champSelectDecision{}, map[string]any{"previous_phase": previous, "entering": entering, "leaving": leaving})
	}
}

func (r *watchRunner) resetChampSelectRuntimeLocked() {
	if r.champSelect.writeCancel != nil {
		r.champSelect.writeCancel()
	}
	for _, action := range []string{champSelectActionBan, champSelectActionPick, champSelectActionBench, champSelectActionTrade} {
		if pending := r.pending[action]; pending != nil {
			pending.cancel()
			delete(r.pending, action)
		}
	}
	phase := r.champSelect.phase
	r.champSelect = newChampSelectRuntimeStore()
	r.champSelect.phase = phase
}

func (r *watchRunner) handleChampSelectAutomation(client *LCUClient) {
	if r == nil || client == nil {
		return
	}
	settings := r.currentWatch()
	if !settings.ChampSelect.Enabled {
		r.champDiagnostic("trigger", "disabled", champSelectDecision{}, nil)
		return
	}
	r.mu.Lock()
	if !strings.EqualFold(r.champSelect.phase, "ChampSelect") {
		r.mu.Unlock()
		r.champDiagnostic("trigger", "outside-champselect", champSelectDecision{}, nil)
		return
	}
	if r.champSelect.processing {
		r.champSelect.queued = true
		r.mu.Unlock()
		r.champDiagnostic("trigger", "coalesced", champSelectDecision{}, nil)
		return
	}
	if r.champSelect.sessionPaused {
		r.mu.Unlock()
		r.champDiagnostic("trigger", "session-paused", champSelectDecision{}, nil)
		return
	}
	r.champSelect.processing = true
	r.mu.Unlock()
	r.champDiagnostic("trigger", "evaluation-started", champSelectDecision{}, nil)
	go func() {
		for {
			r.evaluateChampSelect(client, r.currentWatch().ChampSelect)
			r.mu.Lock()
			if r.champSelect.queued {
				r.champSelect.queued = false
				r.mu.Unlock()
				continue
			}
			r.champSelect.processing = false
			r.mu.Unlock()
			return
		}
	}()
}

func (r *watchRunner) evaluateChampSelect(client *LCUClient, settings champSelectSettings) {
	started := time.Now()
	defer func() {
		r.champDiagnostic("evaluation-health", "completed", champSelectDecision{}, map[string]any{"slow": time.Since(started) > 2*time.Second})
	}()
	if !settings.Enabled {
		return
	}
	r.mu.Lock()
	phase := r.champSelect.phase
	paused := r.champSelect.sessionPaused
	r.mu.Unlock()
	if paused || !strings.EqualFold(phase, "ChampSelect") {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	var session lcuChampSelectSession
	if err := client.RequestJSON(ctx, http.MethodGet, "/lol-champ-select/v1/session", nil, &session); err != nil {
		r.champSelectReadFailed("session", err)
		return
	}
	groupID := champSelectGroupID(session.QueueID, "", 0)
	var gameflow lcuGameflowSession
	if groupID == "" {
		if err := client.RequestJSON(ctx, http.MethodGet, "/lol-gameflow/v1/session", nil, &gameflow); err == nil {
			mapID := gameflow.GameData.Queue.MapID
			if mapID == 0 {
				mapID = gameflow.Map.ID
			}
			mode := gameflow.GameData.Queue.GameMode
			if mode == "" {
				mode = gameflow.Map.GameMode
			}
			groupID = champSelectGroupID(gameflow.GameData.Queue.ID, mode, mapID)
		} else {
			r.champSelectReadFailed("gameflow-session", err)
		}
	}
	if groupID == "" {
		r.recordChampSelectEvaluation(session, groupID, settings)
		r.mu.Lock()
		first := r.champSelect.groupID != "unsupported"
		r.champSelect.active = true
		r.champSelect.groupID = "unsupported"
		r.champSelect.lastSession = session
		r.mu.Unlock()
		if first {
			r.champSelectLog("fail", "当前模式无对应分组，本局不接管")
			r.emit("watch:failed:champselect-pick:unsupported-mode")
		}
		return
	}
	definition, _ := champSelectGroupDefinitionFor(groupID)
	group := settings.Groups[groupID]
	session, sessionAPI, sourceReason := champSelectSessionSource(ctx, client, session)
	r.mu.Lock()
	if r.champSelect.active && (r.champSelect.lastSession.GameID != session.GameID ||
		(r.champSelect.lastSession.LocalPlayerCellID != nil && session.LocalPlayerCellID != nil && *r.champSelect.lastSession.LocalPlayerCellID != *session.LocalPlayerCellID)) {
		r.resetChampSelectRuntimeLocked()
	}
	r.mu.Unlock()
	r.observeChampSelectManualActions(session)
	r.observeChampSelectBench(session, definition, group, champSelectPoolPosition(definition, champSelectAssignedPosition(session)), "")
	r.recordChampSelectPostflight(session)
	r.reconcileChampSelectSubmissions(session)
	r.mu.Lock()
	r.champSelect.active = true
	r.champSelect.groupID = groupID
	r.champSelect.position = champSelectPoolPosition(definition, champSelectAssignedPosition(session))
	r.champSelect.lastSession = session
	r.champSelect.remainingMS = champSelectRemainingMS(session)
	r.mu.Unlock()
	r.recordChampSelectEvaluation(session, groupID, settings)
	// The grid includes live ban/pick/intent flags, not just a static catalog.
	// Re-read it for every evaluation; cached flags otherwise survive hover changes.
	{
		var rows []champSelectGridChampion
		if err := client.RequestJSON(ctx, http.MethodGet, "/lol-champ-select/v1/all-grid-champions", nil, &rows); err != nil {
			r.champSelectReadFailed("all-grid-champions", err)
			return
		}
		grid := make(map[int64]champSelectGridChampion, len(rows))
		for _, champion := range rows {
			if id := champion.resolvedID(); id > 0 {
				grid[id] = champion
			}
		}
		r.mu.Lock()
		r.champSelect.grid = grid
		r.champSelect.gridLoaded = true
		r.mu.Unlock()
	}
	var pickableIDs []int64
	pickableReady := true
	if err := client.RequestJSON(ctx, http.MethodGet, sessionAPI+"/pickable-champion-ids", nil, &pickableIDs); err != nil {
		r.champSelectReadFailed("pickable-champion-ids", err)
		r.champSelectLog("warn", "无法读取可选英雄列表，本次跳过禁用/选用判定")
		pickableIDs = nil
		pickableReady = false
	}
	bannableIDs := []int64{}
	hasBanAction := champSelectSessionHasActionType(session, "ban")
	banProbe := map[string]any{"event": "champselect_ban_probe", "queue_id": session.QueueID, "has_ban_action": hasBanAction, "requested": definition.HasBan && hasBanAction, "status_code": 0, "raw_count": 0, "raw_head": []int64{}}
	if definition.HasBan && hasBanAction {
		var status atomic.Int64
		err := client.RequestJSON(context.WithValue(ctx, lcuResponseStatusKey{}, &status), http.MethodGet, sessionAPI+"/bannable-champion-ids", nil, &bannableIDs)
		banProbe["raw_count"] = len(bannableIDs)
		banProbe["raw_head"] = append([]int64{}, bannableIDs[:min(3, len(bannableIDs))]...)
		banProbe["status_code"] = status.Load()
		if err != nil {
			banProbe["error_class"] = diagnosticErrorKind(err)
			r.record(banProbe)
			r.champSelectReadFailed("bannable-champion-ids", err)
			return
		}
	}
	r.record(banProbe)
	pickable := champSelectIDSet(pickableIDs)
	bannable := champSelectIDSet(bannableIDs)
	r.recordChampSelectSource(sessionAPI, sourceReason, session, pickableIDs, bannableIDs)
	assigned := champSelectAssignedPosition(session)
	position := champSelectPoolPosition(definition, assigned)
	remaining := champSelectRemainingMS(session)
	r.mu.Lock()
	r.champSelect.active = true
	r.champSelect.groupID = groupID
	r.champSelect.position = position
	r.champSelect.remainingMS = remaining
	r.champSelect.benchEnabled = session.BenchEnabled
	r.champSelect.pickable = pickable
	r.champSelect.bannable = bannable
	r.champSelect.lastSession = session
	grid := r.champSelect.grid
	mergeChampSelectTeamState(grid, session)
	r.mu.Unlock()
	bannable, banSource := r.champSelectBanAvailability(session, sessionAPI, bannableIDs, grid)
	r.mu.Lock()
	r.champSelect.bannable, r.champSelect.banSource = bannable, banSource
	r.mu.Unlock()
	r.mu.Lock()
	firstRecord := len(r.champSelect.records) == 0
	r.mu.Unlock()
	if firstRecord {
		poolDescription := "使用共用序列"
		if len(definition.Positions) > 1 {
			poolDescription = fmt.Sprintf("禁用使用共用序列，选用使用%s序列", champSelectPositionName(position))
		}
		r.champSelectLog("ok", fmt.Sprintf("进入英雄选择，识别为%s，%s", definition.Name, poolDescription))
	}
	r.evaluateChampSelectBench(client, session, definition, group, position)
	if !pickableReady {
		r.clearChampSelectDecision(champSelectActionBan)
		r.clearChampSelectDecision(champSelectActionPick)
		return
	}
	intent := strings.EqualFold(session.Timer.Phase, "PLANNING") && !session.BenchEnabled
	action, found := champSelectExecutableAction(session, intent)
	if intent {
		r.clearChampSelectDecision(champSelectActionBan)
	}
	if !found {
		r.champDiagnostic("candidate-gate", "no-executable-action-in-phase", champSelectDecision{}, map[string]any{"timer_phase": session.Timer.Phase, "local_cell_present": session.LocalPlayerCellID != nil, "action_groups": len(session.Actions)})
		r.clearChampSelectDecision(champSelectActionBan)
		r.clearChampSelectDecision(champSelectActionPick)
		return
	}
	side := strings.ToLower(strings.TrimSpace(action.Type))
	if side != "ban" && side != "pick" {
		return
	}
	if side == "ban" {
		r.clearChampSelectDecision(champSelectActionPick)
	} else {
		r.clearChampSelectDecision(champSelectActionBan)
	}
	if side == "ban" && !definition.HasBan {
		return
	}
	config := group.Pick
	available := pickable
	actionName := champSelectActionPick
	if side == "ban" {
		config = group.Ban
		available = bannable
		actionName = champSelectActionBan
	}
	if !config.Enabled {
		r.champDiagnostic("candidate-gate", "side-disabled", champSelectDecision{Action: actionName, ActionID: action.ID}, nil)
		r.clearChampSelectDecision(actionName)
		return
	}
	champions, poolPosition := champSelectConfiguredPool(config, definition, assigned, side)
	if len(champions) == 0 {
		r.champDiagnostic("candidate-gate", "pool-empty", champSelectDecision{Action: actionName, ActionID: action.ID}, map[string]any{"position": poolPosition})
		r.clearChampSelectDecision(actionName)
		return
	}
	r.mu.Lock()
	lastSubmission, submitted := r.champSelect.submitted[action.ID]
	alreadyYielded := r.champSelect.takeover[side]
	pendingDecision := r.champSelect.decision[actionName]
	submitting := r.champSelect.inFlight[action.ID]
	r.mu.Unlock()
	if pendingDecision.Action != "" && pendingDecision.Intent != intent {
		r.clearChampSelectDecision(actionName)
	}
	if alreadyYielded {
		r.champDiagnostic("candidate-gate", "manual-takeover", champSelectDecision{Action: actionName, ActionID: action.ID}, nil)
		return
	}
	// LCU may broadcast our hover before its HTTP acknowledgement arrives.
	// Wait for that write to settle instead of treating our own hover as manual.
	if submitting && pendingDecision.ActionID == action.ID && pendingDecision.ChampionID == action.ChampionID {
		return
	}
	if action.ChampionID != 0 && (!submitted || lastSubmission.ChampionID != action.ChampionID) {
		championID := lastSubmission.ChampionID
		if championID == 0 {
			championID = pendingDecision.ChampionID
		}
		r.yieldChampSelect(side, championID, "")
		return
	}
	if submitted && !lastSubmission.Confirmed {
		r.champDiagnostic("candidate-gate", "awaiting-application", lastSubmission.Decision, nil)
		return
	}
	candidate := int64(0)
	skipped := map[int64]string{}
	if config.Strategy == "show-then-lock" && action.ChampionID != 0 && submitted && lastSubmission.ChampionID == action.ChampionID && !lastSubmission.Completed {
		candidate, _ = chooseChampSelectCandidate(side, []int64{action.ChampionID}, available, grid, config.AvoidTeammateIntent, groupID == "arena")
	}
	if candidate == 0 {
		candidate, skipped = chooseChampSelectCandidate(side, champions, available, grid, config.AvoidTeammateIntent, groupID == "arena")
	}
	r.recordChampSelectCandidates(session, side, champions, candidate, skipped, len(available), len(grid))
	if candidate == 0 {
		r.clearChampSelectDecision(actionName)
		key := fmt.Sprintf("%s:%d", side, action.ID)
		r.mu.Lock()
		first := !r.champSelect.exhausted[key]
		r.champSelect.exhausted[key] = true
		r.mu.Unlock()
		if first {
			if side == "ban" && len(available) == 0 {
				r.champSelectLog("fail", "客户端禁用列表没有提供任何英雄 ID，并非队友已选择全部候选；已记录原始 ID 与会话状态，等待客户端更新")
			} else {
				r.champSelectLog("fail", fmt.Sprintf("%s序列本次无可用候选（客户端可用列表 %d 个），等待状态更新", champSelectSideName(side), len(available)))
			}
			r.record(map[string]any{"event": "watch_action", "action": actionName, "result": "exhausted"})
			r.emit("watch:failed:" + actionName + ":exhausted")
		}
		return
	}
	confirmedHover := submitted && lastSubmission.Confirmed && !lastSubmission.Completed && lastSubmission.ChampionID == candidate
	observedChampion := action.ChampionID
	if observedChampion == 0 && confirmedHover {
		observedChampion = candidate
	}
	completed := champSelectActionCompleted(config.Strategy, observedChampion, candidate, submitted, lastSubmission)
	forceHover := side == "ban" && strings.HasPrefix(banSource, "wildcard-")
	if intent || forceHover && !confirmedHover {
		completed = false
	}
	if submitted && lastSubmission.ChampionID == candidate && lastSubmission.Completed == completed {
		return
	}
	delay := 0
	switch {
	case intent:
		delay = champSelectDelay(config.DelayMS, remaining)
	case config.Strategy == "lock-now":
		delay = 0
	case !completed:
		delay = 0
	default:
		delay = champSelectDelay(champSelectHoverLockDelay(config, lastSubmission, time.Now()), remaining)
	}
	body := map[string]any{"type": side, "championId": candidate, "completed": completed}
	if intent {
		body = map[string]any{"championId": candidate}
	}
	path := fmt.Sprintf("%s/session/actions/%d", sessionAPI, action.ID)
	decision := champSelectDecision{Key: fmt.Sprintf("%s:%d:%d:%t", sessionAPI, action.ID, candidate, completed), Action: actionName, ActionID: action.ID, ChampionID: candidate, Completed: completed, SessionAPI: sessionAPI, GameID: session.GameID, QueueID: session.QueueID, LocalCellID: *session.LocalPlayerCellID}
	decision.Intent, decision.ForceHover, decision.AvailabilitySource = intent, forceHover, banSource
	decision.Key += fmt.Sprintf(":%s:%t", session.Timer.Phase, intent)
	if side == "pick" {
		decision.AvailabilitySource = "client-pickable"
	}
	r.scheduleChampSelectRequest(client, decision, delay, http.MethodPatch, path, body)
}

func (r *watchRunner) recordChampSelectSource(source, reason string, session lcuChampSelectSession, pickable, bannable []int64) {
	// Include the actual action shape so a missing turn is distinguishable from
	// an empty availability response, without exporting any player identifiers.
	local := make([]map[string]any, 0)
	actions := make([]map[string]any, 0)
	for _, group := range session.Actions {
		for _, action := range group {
			if len(actions) < 64 {
				actions = append(actions, map[string]any{"id": action.ID, "type": action.Type, "champion_id": action.ChampionID, "completed": action.Completed, "in_progress": action.IsInProgress, "is_local": session.LocalPlayerCellID != nil && action.ActorCellID == *session.LocalPlayerCellID})
			}
			if session.LocalPlayerCellID != nil && action.ActorCellID == *session.LocalPlayerCellID {
				local = append(local, map[string]any{"id": action.ID, "type": action.Type, "champion_id": action.ChampionID, "completed": action.Completed, "in_progress": action.IsInProgress})
			}
		}
	}
	event := map[string]any{"event": "champselect_source", "source": source, "reason": reason, "phase": session.Timer.Phase, "queue_id": session.QueueID, "local_actions": local, "pickable_count": len(champSelectIDSet(pickable)), "bannable_count": len(champSelectIDSet(bannable)), "bannable_raw_count": len(bannable)}
	// Numeric champion IDs only: never export names, PUUIDs or credentials.
	event["bannable_raw_ids"] = append([]int64{}, bannable[:min(len(bannable), 300)]...)
	event["actions"] = actions
	teams := make([]map[string]any, 0)
	for index, team := range [][]lcuChampSelectPlayer{session.MyTeam, session.TheirTeam} {
		for _, player := range team[:min(len(team), 20)] {
			teams = append(teams, map[string]any{"ally": index == 0, "is_local": session.LocalPlayerCellID != nil && player.CellID != nil && *player.CellID == *session.LocalPlayerCellID, "champion_id": player.ChampionID, "intent_id": player.ChampionPickIntent})
		}
	}
	event["team_champions"] = teams
	// List order changes with client sorting/filtering; it is not an
	// availability change. Keep the raw order in exported evidence only.
	semantic := make(map[string]any, len(event))
	for key, value := range event {
		semantic[key] = value
	}
	sortedIDs := append([]int64{}, bannable...)
	slices.Sort(sortedIDs)
	semantic["bannable_raw_ids"] = sortedIDs
	encoded, _ := json.Marshal(semantic)
	r.mu.Lock()
	changed := r.champSelect.sourceKey != string(encoded)
	r.champSelect.sourceKey = string(encoded)
	r.mu.Unlock()
	if changed {
		r.record(event)
	}
	r.champDiagnostic("source", reason, champSelectDecision{}, map[string]any{"source": source, "queue_id": session.QueueID, "ban_count": len(champSelectIDSet(bannable)), "pick_count": len(champSelectIDSet(pickable)), "local_action_count": len(local)})
}

func champSelectActionCompleted(strategy string, actionChampionID, candidate int64, submitted bool, last champSelectSubmitRecord) bool {
	return strategy == "lock-now" || (strategy == "show-then-lock" && actionChampionID == candidate && submitted && last.ChampionID == candidate && !last.Completed)
}

func (r *watchRunner) recordChampSelectCandidates(session lcuChampSelectSession, side string, pool []int64, selected int64, skipped map[int64]string, availableCount, gridCount int) {
	candidates := make([]map[string]any, 0, len(pool))
	for _, id := range pool {
		reason := skipped[id]
		if id == selected {
			reason = "selected"
		} else if reason == "" {
			reason = "later-priority"
		}
		candidates = append(candidates, map[string]any{"champion_id": id, "reason": reason})
	}
	r.mu.Lock()
	for _, candidate := range candidates {
		champion, present := r.champSelect.grid[candidate["champion_id"].(int64)]
		candidate["grid_present"] = present
		candidate["grid_status"] = champion.SelectionStatus
		candidate["teammate_picked"] = champion.TeammatePicked
		candidate["teammate_intent"] = champion.TeammateIntent
		candidate["own_intent"] = champion.LocalIntent
	}
	r.mu.Unlock()
	intent := side == "pick" && strings.EqualFold(session.Timer.Phase, "PLANNING")
	action, _ := champSelectExecutableAction(session, intent)
	event := map[string]any{"event": "champselect_candidates", "queue_id": session.QueueID, "side": side, "action_id": action.ID, "intent": intent, "timer_phase": session.Timer.Phase, "available_count": availableCount, "grid_count": gridCount, "candidates": candidates}
	r.champDiagnostic("candidates", "evaluated", champSelectDecision{Action: "champselect-" + side, ActionID: action.ID, ChampionID: selected, Intent: intent}, map[string]any{"candidates": candidates, "available_count": availableCount})
	encoded, _ := json.Marshal(event)
	r.mu.Lock()
	changed := r.champSelect.candidateKey != string(encoded)
	r.champSelect.candidateKey = string(encoded)
	r.mu.Unlock()
	if changed {
		r.record(event)
		r.emit("champselect:changed")
	}
	// UI skip explanations depend on the decision, not unrelated grid flags
	// or every polling tick. Detailed candidate diagnostics remain available.
	skipKey, _ := json.Marshal(map[string]any{"side": side, "action_id": action.ID, "intent": intent, "selected": selected, "skipped": skipped})
	r.mu.Lock()
	skipChanged := r.champSelect.skippedKey != string(skipKey)
	r.champSelect.skippedKey = string(skipKey)
	r.mu.Unlock()
	if skipChanged {
		labels := map[string]string{"unknown": "英雄网格未提供", "gone": "已禁用或已选定", "unavailable": "客户端可用列表未包含", "intent": "队友当前预选", "teammate-picked": "队友已经选择", "own-intent": "自己准备选用，不自动禁用"}
		for _, id := range pool {
			if reason := skipped[id]; reason != "" {
				r.champSelectChampionLog("warn", fmt.Sprintf("跳过英雄 %d：%s", id, labels[reason]), id)
			}
		}
		if selected != 0 && len(skipped) > 0 {
			r.champSelectLog("warn", fmt.Sprintf("已顺延前 %d 个不可用备选", len(skipped)))
		}
	}
}

func champSelectIDSet(values []int64) map[int64]struct{} {
	set := make(map[int64]struct{}, len(values))
	for _, value := range values {
		if value > 0 {
			set[value] = struct{}{}
		}
	}
	return set
}

func champSelectSideName(side string) string {
	if side == "ban" {
		return "禁用"
	}
	return "选用"
}

func champSelectPositionName(position string) string {
	return map[string]string{"top": "上单", "jungle": "打野", "middle": "中单", "bottom": "下路", "utility": "辅助", "default": "未分配位置"}[position]
}

func (r *watchRunner) scheduleChampSelectRequest(client *LCUClient, decision champSelectDecision, delayMS int, method, path string, body any) bool {
	// Action IDs are zero-based. Bench/trade operations are distinguished by
	// their kind, never by treating action 0 as a missing action.
	isBanPick := decision.Action == champSelectActionBan || decision.Action == champSelectActionPick
	ctx, cancel := context.WithCancel(context.Background())
	pending := &watchPendingAction{cancel: cancel}
	r.mu.Lock()
	decision.RuntimeID = r.champSelect.runtimeID
	if isBanPick && r.champSelect.attempts[decision.Key] >= champSelectMaxWriteAttempts {
		r.mu.Unlock()
		r.champDiagnostic("schedule", "attempt-limit", decision, nil)
		cancel()
		return false
	}
	if !r.settings.ChampSelect.Enabled || !strings.EqualFold(r.champSelect.phase, "ChampSelect") || r.champSelect.sessionPaused || r.champSelect.takeover[strings.TrimPrefix(decision.Action, "champselect-")] {
		r.mu.Unlock()
		r.champDiagnostic("schedule", "gate-blocked", decision, nil)
		cancel()
		return false
	}
	if current, ok := r.champSelect.decision[decision.Action]; ok && current.Key == decision.Key {
		r.mu.Unlock()
		r.champDiagnostic("schedule", "same-decision", current, nil)
		cancel()
		return false
	}
	if isBanPick && r.champSelect.inFlight[decision.ActionID] {
		r.mu.Unlock()
		r.champDiagnostic("schedule", "write-in-flight", decision, nil)
		cancel()
		return false
	}
	replaced := r.pending[decision.Action] != nil
	if previous := r.pending[decision.Action]; previous != nil {
		previous.cancel()
	}
	decision.TraceID = newDiagnosticTrace("cs-write")
	decision.SessionTrace = r.champDiagnosticSession
	r.pending[decision.Action] = pending
	r.champSelect.decision[decision.Action] = decision
	r.mu.Unlock()
	r.champDiagnostic("schedule", "armed", decision, map[string]any{"delay_ms": delayMS, "replaced_pending": replaced, "method": method, "endpoint": diagnosticWatchEndpoint(path)})
	r.emit(fmt.Sprintf("watch:armed:%s:%d", decision.Action, delayMS))
	verb := champSelectDecisionVerb(decision)
	r.champSelectChampionLog("ok", fmt.Sprintf("已排定：%.1f 秒后%s英雄 %d", float64(delayMS)/1000, verb, decision.ChampionID), decision.ChampionID)
	go func() {
		defer func() {
			r.mu.Lock()
			if r.pending[decision.Action] == pending {
				delete(r.pending, decision.Action)
			}
			if isBanPick && r.champSelect.runtimeID == decision.RuntimeID {
				delete(r.champSelect.inFlight, decision.ActionID)
			}
			r.mu.Unlock()
		}()
		if delayMS > 0 {
			timer := time.NewTimer(time.Duration(delayMS) * time.Millisecond)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				r.champDiagnostic("delay", "canceled", decision, nil)
				r.emit("watch:canceled:" + decision.Action)
				return
			case <-timer.C:
			}
		}
		if ctx.Err() != nil {
			r.champDiagnostic("delay", "canceled-before-start", decision, nil)
			return
		}
		r.mu.Lock()
		if r.champSelect.sessionPaused || r.champSelect.runtimeID != decision.RuntimeID || r.champSelect.takeover[strings.TrimPrefix(decision.Action, "champselect-")] {
			r.mu.Unlock()
			r.champDiagnostic("delay", "session-paused", decision, nil)
			return
		}
		if isBanPick {
			if record, ok := r.champSelect.submitted[decision.ActionID]; ok && record.ChampionID == decision.ChampionID && record.Completed == decision.Completed {
				r.mu.Unlock()
				r.champDiagnostic("delay", "already-submitted", decision, nil)
				return
			}
			if r.champSelect.inFlight[decision.ActionID] {
				r.mu.Unlock()
				r.champDiagnostic("delay", "write-in-flight", decision, nil)
				return
			}
			r.champSelect.inFlight[decision.ActionID] = true
		}
		r.mu.Unlock()
		requestCtx, requestCancel := context.WithTimeout(ctx, 8*time.Second)
		if (isBanPick && !r.champSelectRequestStillCurrent(requestCtx, client, decision)) || (decision.Action == champSelectActionBench && !r.champSelectBenchRequestStillCurrent(requestCtx, client, decision)) {
			requestCancel()
			r.mu.Lock()
			if r.champSelect.decision[decision.Action] == decision {
				delete(r.champSelect.decision, decision.Action)
			}
			r.mu.Unlock()
			r.record(map[string]any{"event": "watch_action", "action": decision.Action, "result": "canceled", "reason": "champselect-state-changed"})
			return
		}
		writeStarted := time.Now()
		r.mu.Lock()
		if r.champSelect.runtimeID != decision.RuntimeID || r.champSelect.takeover[strings.TrimPrefix(decision.Action, "champselect-")] || ctx.Err() != nil {
			r.mu.Unlock()
			requestCancel()
			return
		}
		if isBanPick {
			r.champSelect.attempts[decision.Key]++
		}
		if decision.Action == champSelectActionBench || (decision.Action == champSelectActionTrade && decision.Completed) {
			r.champSelect.benchSwap = &champSelectBenchSubmission{Decision: decision}
		}
		attempt := r.champSelect.attempts[decision.Key]
		r.mu.Unlock()
		r.champDiagnostic("write", "started", decision, map[string]any{"attempt": attempt, "attempt_limit": champSelectMaxWriteAttempts})
		err := r.requestChampSelectJSON(requestCtx, client, method, path, body)
		requestCancel()
		r.champDiagnostic("write-result", diagnosticStageError(err, "http-success"), decision, map[string]any{"duration_ms": time.Since(writeStarted).Milliseconds()})
		if !isBanPick {
			r.mu.Lock()
			if swap := r.champSelect.benchSwap; r.champSelect.runtimeID == decision.RuntimeID && swap != nil && swap.Decision.TraceID == decision.TraceID {
				swap.SettledAt = time.Now()
			}
			r.mu.Unlock()
		}
		if isBanPick {
			r.mu.Lock()
			current := r.champSelect.runtimeID == decision.RuntimeID && r.champSelect.decision[decision.Action].TraceID == decision.TraceID
			group := r.settings.ChampSelect.Groups[r.champSelect.groupID]
			sideEnabled := group.Pick.Enabled
			if decision.Action == champSelectActionBan {
				sideEnabled = group.Ban.Enabled
			}
			current = current && r.settings.ChampSelect.Enabled && sideEnabled && !r.champSelect.sessionPaused && !r.champSelect.takeover[strings.TrimPrefix(decision.Action, "champselect-")]
			if current && ctx.Err() == nil {
				// Even a timeout can have been applied by the client. Read back
				// before deciding whether the bounded retry is needed.
				r.champSelect.submitted[decision.ActionID] = champSelectSubmitRecord{ChampionID: decision.ChampionID, Completed: decision.Completed, At: time.Now(), TraceID: decision.TraceID, Decision: decision}
			}
			r.mu.Unlock()
			disposition := "discarded-after-cancellation"
			if current && ctx.Err() == nil {
				disposition = "awaiting-confirmation"
			}
			r.champDiagnostic("write-disposition", disposition, decision, nil)
			if current && ctx.Err() == nil {
				r.champSelectChampionLog("ok", fmt.Sprintf("已发送%s英雄 %d，等待客户端确认", verb, decision.ChampionID), decision.ChampionID)
			}
			return
		}
		r.mu.Lock()
		current := r.champSelect.runtimeID == decision.RuntimeID && !r.champSelect.takeover[strings.TrimPrefix(decision.Action, "champselect-")]
		r.mu.Unlock()
		if !current || ctx.Err() != nil {
			return
		}
		if err != nil {
			if ctx.Err() == nil {
				status, code, message := watchFailureDetail(err)
				r.mu.Lock()
				if r.champSelect.decision[decision.Action].TraceID == decision.TraceID {
					delete(r.champSelect.decision, decision.Action)
				}
				r.mu.Unlock()
				r.record(map[string]any{"event": "watch_action", "action": decision.Action, "result": "failed", "status_code": status, "error_code": code})
				r.champSelectLog("fail", fmt.Sprintf("%s失败：%s", champSelectActionVerb(decision.Action), message))
				r.emit(fmt.Sprintf("watch:failed:%s:%d:%s:%s", decision.Action, status, watchEventField(code), watchEventField(message)))
			}
			return
		}
		r.record(map[string]any{"event": "watch_action", "action": decision.Action, "result": "fired"})
		r.champSelectChampionLog("ok", fmt.Sprintf("已%s英雄 %d", champSelectActionVerb(decision.Action), decision.ChampionID), decision.ChampionID)
		r.emit("watch:fired:" + decision.Action)
	}()
	return true
}

func (r *watchRunner) champSelectRequestStillCurrent(ctx context.Context, client *LCUClient, decision champSelectDecision) (valid bool) {
	why := "allowed"
	checks := map[string]any{}
	defer func() { checks["allowed"] = valid; r.champDiagnostic("preflight", why, decision, checks) }()
	if decision.SessionAPI == "" { // Direct scheduler callers still use the master/session gates.
		return true
	}
	if decision.SessionAPI != champSelectAPI && decision.SessionAPI != champSelectLegacyAPI {
		why = "unrecognized-module"
		return false
	}
	if decision.SessionAPI == champSelectLegacyAPI {
		var active bool
		if err := client.RequestJSON(ctx, http.MethodGet, decision.SessionAPI+"/implementation-active", nil, &active); err != nil || !active {
			why = diagnosticStageError(err, "legacy-inactive")
			return false
		}
	}
	var session lcuChampSelectSession
	if err := client.RequestJSON(ctx, http.MethodGet, decision.SessionAPI+"/session", nil, &session); err != nil {
		why = "session-read-" + diagnosticErrorKind(err)
		return false
	}
	checks["same_game"] = session.GameID == decision.GameID
	checks["local_cell_present"] = session.LocalPlayerCellID != nil
	checks["same_cell"] = session.LocalPlayerCellID != nil && *session.LocalPlayerCellID == decision.LocalCellID
	checks["observed_queue_id"], checks["expected_queue_id"] = session.QueueID, decision.QueueID
	if session.GameID != decision.GameID ||
		session.LocalPlayerCellID == nil || *session.LocalPlayerCellID != decision.LocalCellID ||
		(session.QueueID != decision.QueueID && !(decision.SessionAPI == champSelectLegacyAPI && unspecifiedQueueID(session.QueueID))) {
		why = "session-identity-changed"
		return false
	}
	action, found := champSelectExecutableAction(session, decision.Intent)
	checks["timer_phase"], checks["intent"] = session.Timer.Phase, decision.Intent
	checks["unique_local_action"], checks["observed_action_id"], checks["observed_champion_id"] = found, action.ID, action.ChampionID
	if !found || action.ID != decision.ActionID || "champselect-"+strings.ToLower(action.Type) != decision.Action {
		why = "local-action-changed"
		return false
	}
	r.mu.Lock()
	config := r.settings.ChampSelect.Groups[r.champSelect.groupID].Pick
	if decision.Action == champSelectActionBan {
		config = r.settings.ChampSelect.Groups[r.champSelect.groupID].Ban
	}
	side := strings.TrimPrefix(decision.Action, "champselect-")
	allowed := config.Enabled && !r.champSelect.takeover[side]
	definition, _ := champSelectGroupDefinitionFor(r.champSelect.groupID)
	pool, poolPosition := champSelectConfiguredPool(config, definition, champSelectAssignedPosition(session), side)
	checks["pool_position"], checks["pool_size"] = poolPosition, len(pool)
	allowed = allowed && slices.Contains(pool, decision.ChampionID)
	if side == "ban" {
		allowed = allowed && definition.HasBan
	}
	last, submitted := r.champSelect.submitted[action.ID]
	checks["side_enabled"], checks["takeover"] = config.Enabled, r.champSelect.takeover[side]
	checks["has_submission"], checks["last_submit_champion_id"], checks["last_submit_completed"] = submitted, last.ChampionID, last.Completed
	r.mu.Unlock()
	// Replacing our own hover after an ally takes it is allowed; replacing a
	// manual hover remains forbidden.
	if champSelectManualActionChanged(session, action, last, submitted) {
		r.yieldChampSelect(side, decision.ChampionID, decision.RuntimeID)
		why = "manual-takeover"
		return false
	}
	if decision.ForceHover && decision.Completed && !(submitted && last.Confirmed && last.ChampionID == decision.ChampionID) {
		why = "compatibility-hover-unconfirmed"
		return false
	}
	if !allowed || decision.Completed && submitted && last.Confirmed && action.ChampionID != 0 && action.ChampionID != decision.ChampionID {
		why = "disabled-or-manual-takeover"
		return false
	}
	grid := map[int64]champSelectGridChampion{decision.ChampionID: {ID: decision.ChampionID}}
	mergeChampSelectTeamState(grid, session)
	champion := grid[decision.ChampionID]
	reason := ""
	if side == "pick" && champion.TeammatePicked {
		reason = "teammate-picked"
	}
	if champion.SelectionStatus.IsBanned {
		reason = "gone"
	}
	if config.AvoidTeammateIntent && champion.TeammateIntent {
		reason = "intent"
	}
	if reason != "" {
		why = reason
		r.record(map[string]any{"event": "champselect_preflight_rejected", "action": decision.Action, "action_id": decision.ActionID, "champion_id": decision.ChampionID, "reason": reason, "completed": decision.Completed})
		return false
	}
	if reason = r.champSelectFreshCandidate(ctx, client, session, decision, config); reason != "" {
		why = reason
		return false
	}
	return true
}

func (r *watchRunner) clearChampSelectDecision(action string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	pending := r.pending[action]
	decision := r.champSelect.decision[action]
	delete(r.pending, action)
	delete(r.champSelect.decision, action)
	r.mu.Unlock()
	if pending != nil {
		pending.cancel()
		r.champDiagnostic("cancel", "decision-cleared", decision, nil)
	}
}

func champSelectActionVerb(action string) string {
	return map[string]string{champSelectActionBan: "禁用", champSelectActionPick: "选用", champSelectActionBench: "交换", champSelectActionTrade: "处理交换"}[action]
}

func (r *watchRunner) evaluateChampSelectBench(client *LCUClient, session lcuChampSelectSession, definition champSelectGroupDefinition, group champSelectGroupConfig, position string) {
	if r.observeChampSelectBench(session, definition, group, position, "") {
		return
	}
	r.mu.Lock()
	awaitingSwap := false
	if swap := r.champSelect.benchSwap; swap != nil && !swap.Observed {
		awaitingSwap = swap.SettledAt.IsZero() || time.Since(swap.SettledAt) < 2*time.Second
		if !awaitingSwap {
			r.champSelect.benchSwap = nil
		}
	}
	r.mu.Unlock()
	if awaitingSwap {
		return
	}
	if !definition.HasBench || !session.BenchEnabled || !group.Bench.Enabled || len(session.BenchChampions) == 0 {
		r.champDiagnostic("bench-gate", "unavailable-or-disabled", champSelectDecision{}, map[string]any{"has_bench": definition.HasBench, "client_bench": session.BenchEnabled, "enabled": group.Bench.Enabled, "bench_count": len(session.BenchChampions)})
		r.clearChampSelectDecision(champSelectActionBench)
		return
	}
	pool := group.Pick.Champions[position]
	if len(pool) == 0 {
		r.clearChampSelectDecision(champSelectActionBench)
		return
	}
	now := time.Now()
	bench := make(map[int64]lcuChampSelectBenchChampion, len(session.BenchChampions))
	for _, champion := range session.BenchChampions {
		bench[champion.ChampionID] = champion
	}
	r.mu.Lock()
	for championID := range r.champSelect.benchFirstSeen {
		if _, present := bench[championID]; !present {
			delete(r.champSelect.benchFirstSeen, championID)
		}
	}
	for championID := range bench {
		if r.champSelect.benchFirstSeen[championID].IsZero() {
			r.champSelect.benchFirstSeen[championID] = now
		}
	}
	firstSeen := make(map[int64]time.Time, len(r.champSelect.benchFirstSeen))
	for championID, seen := range r.champSelect.benchFirstSeen {
		firstSeen[championID] = seen
	}
	r.mu.Unlock()
	current := champSelectCurrentChampion(session)
	target := champSelectBenchTarget(pool, bench, current, group.Bench.PreferFirst)
	if target == 0 {
		r.champDiagnostic("bench-gate", "no-preferred-target", champSelectDecision{}, map[string]any{"pool_count": len(pool), "bench_count": len(bench)})
		r.clearChampSelectDecision(champSelectActionBench)
		return
	}
	elapsed := int(now.Sub(firstSeen[target]) / time.Millisecond)
	delay := champSelectDelay(max(0, group.Bench.HoldMS-elapsed), champSelectRemainingMS(session))
	path := fmt.Sprintf("/lol-champ-select/v1/session/bench/swap/%d", target)
	if session.LocalPlayerCellID == nil {
		return
	}
	decision := champSelectDecision{Key: fmt.Sprintf("bench:%d", target), Action: champSelectActionBench, ChampionID: target, FromChampionID: current, SessionAPI: champSelectAPI, GameID: session.GameID, QueueID: session.QueueID, LocalCellID: *session.LocalPlayerCellID}
	r.scheduleChampSelectRequest(client, decision, delay, http.MethodPost, path, nil)
}

func champSelectBenchTarget(pool []int64, bench map[int64]lcuChampSelectBenchChampion, currentChampionID int64, preferFirst bool) int64 {
	currentRank := slices.Index(pool, currentChampionID)
	for index, championID := range pool {
		if _, available := bench[championID]; !available {
			continue
		}
		if preferFirst && currentRank >= 0 && index >= currentRank {
			continue
		}
		return championID
	}
	return 0
}

func (r *watchRunner) handleChampSelectTrade(client *LCUClient) {
	if r == nil || client == nil {
		return
	}
	settings := r.currentWatch()
	if !settings.ChampSelect.Enabled {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		var swap champSelectOngoingSwap
		if err := client.RequestJSON(ctx, http.MethodGet, "/lol-champ-select/v1/ongoing-champion-swap", nil, &swap); err != nil || swap.ID <= 0 {
			r.champDiagnostic("trade-gate", diagnosticStageError(err, "no-swap"), champSelectDecision{}, nil)
			return
		}
		state := strings.ToLower(strings.TrimSpace(swap.State))
		if state != "pending" && state != "requested" {
			r.champDiagnostic("trade-gate", "not-pending", champSelectDecision{}, nil)
			return
		}
		r.mu.Lock()
		groupID := r.champSelect.groupID
		position := r.champSelect.position
		session := r.champSelect.lastSession
		paused := r.champSelect.sessionPaused || r.champSelect.takeover["trade"]
		r.mu.Unlock()
		if groupID == "" || groupID == "unsupported" || paused {
			r.champDiagnostic("trade-gate", "mode-or-pause", champSelectDecision{}, nil)
			return
		}
		group := settings.ChampSelect.Groups[groupID]
		definition, known := champSelectGroupDefinitionFor(groupID)
		if !known || !definition.HasTrade || !group.Bench.HandleTrade {
			r.champDiagnostic("trade-gate", "disabled-or-unsupported", champSelectDecision{}, nil)
			return
		}
		pool := group.Pick.Champions[position]
		accept := champSelectShouldAcceptTrade(pool, champSelectCurrentChampion(session), swap.RequesterChampionID, group.Bench.PreferFirst)
		outcome := "decline"
		if accept {
			outcome = "accept"
		}
		path := fmt.Sprintf("/lol-champ-select/v1/session/champion-swaps/%d/%s", swap.ID, outcome)
		decision := champSelectDecision{Key: fmt.Sprintf("trade:%d:%s", swap.ID, outcome), Action: champSelectActionTrade, ChampionID: swap.RequesterChampionID, FromChampionID: champSelectCurrentChampion(session), Completed: accept}
		r.scheduleChampSelectRequest(client, decision, 0, http.MethodPost, path, nil)
	}()
}

// Champion-select automation has its own per-session pause and explicitly
// supports the practice group, which includes custom games. It therefore
// shares the master-switch cancellation context without inheriting the older
// workflow rules' blanket custom-game pause.
func (r *watchRunner) requestChampSelectJSON(ctx context.Context, client *LCUClient, method, path string, body any) error {
	r.mu.Lock()
	if !r.settings.ChampSelect.Enabled || !strings.EqualFold(r.champSelect.phase, "ChampSelect") || r.champSelect.sessionPaused {
		r.mu.Unlock()
		r.champDiagnostic("write-gate", "blocked", champSelectDecision{}, nil)
		return context.Canceled
	}
	if r.champSelect.writeContext == nil {
		r.champSelect.writeContext, r.champSelect.writeCancel = context.WithCancel(context.Background())
	}
	session := r.champSelect.writeContext
	r.mu.Unlock()
	requestCtx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(session, cancel)
	defer stop()
	defer cancel()
	if err := session.Err(); err != nil {
		return err
	}
	return client.RequestJSON(requestCtx, method, path, body, nil)
}

func champSelectShouldAcceptTrade(pool []int64, currentChampionID, requesterChampionID int64, preferFirst bool) bool {
	requesterRank := slices.Index(pool, requesterChampionID)
	currentRank := slices.Index(pool, currentChampionID)
	return requesterRank >= 0 && (currentRank < 0 || (preferFirst && requesterRank < currentRank))
}

// Export only automation gates/counts, never roster identities or full LCU data.
// Keep this separate from live recommendation diagnostics: no-target there
// does not describe the ban/pick engine.
func (r *watchRunner) recordChampSelectEvaluation(session lcuChampSelectSession, groupID string, settings champSelectSettings) {
	definition, _ := champSelectGroupDefinitionFor(groupID)
	position := champSelectPoolPosition(definition, champSelectAssignedPosition(session))
	group := settings.Groups[groupID]
	banPool, banPosition := champSelectConfiguredPool(group.Ban, definition, position, "ban")
	pickPool, pickPosition := champSelectConfiguredPool(group.Pick, definition, position, "pick")
	intent := strings.EqualFold(session.Timer.Phase, "PLANNING") && !session.BenchEnabled
	action, hasAction := champSelectExecutableAction(session, intent)
	event := map[string]any{
		"event": "champselect_evaluation", "queue_id": session.QueueID, "group_id": groupID,
		"position": position, "enabled": settings.Enabled, "supported": groupID != "",
		"ban_enabled": group.Ban.Enabled, "pick_enabled": group.Pick.Enabled,
		"ban_pool_size": len(banPool), "pick_pool_size": len(pickPool),
		"ban_pool_position": banPosition, "pick_pool_position": pickPosition,
		"bench_enabled": group.Bench.Enabled, "prefer_first": group.Bench.PreferFirst, "trade_enabled": group.Bench.HandleTrade,
		"has_local_action": hasAction, "action_type": strings.ToLower(action.Type),
		"timer_phase": session.Timer.Phase, "pick_intent": intent, "avoid_teammate_intent": group.Pick.AvoidTeammateIntent,
	}
	encoded, _ := json.Marshal(event)
	key := string(encoded)
	r.mu.Lock()
	if r.champSelect.diagnosticKey == key {
		r.mu.Unlock()
		return
	}
	r.champSelect.diagnosticKey = key
	r.mu.Unlock()
	r.record(event)
	side := strings.ToLower(action.Type)
	if hasAction && (side == "ban" || side == "pick") {
		config := group.Pick
		if side == "ban" {
			config = group.Ban
		}
		pool, _ := champSelectConfiguredPool(config, definition, position, side)
		if !config.Enabled {
			r.champSelectLog("warn", fmt.Sprintf("%s的自动%s开关未开启（已配置 %d 个英雄），本回合不执行", definition.Name, champSelectSideName(side), len(pool)))
			r.emit("watch:canceled:champselect-" + side)
		} else if len(pool) == 0 {
			r.champSelectLog("warn", fmt.Sprintf("%s的%s序列为空，本回合不执行", definition.Name, champSelectSideName(side)))
		}
	}
}

func (r *watchRunner) champSelectReadFailed(endpoint string, err error) {
	r.champDiagnostic("read-"+endpoint, diagnosticErrorKind(err), champSelectDecision{}, nil)
	status, _, _ := watchFailureDetail(err)
	key := fmt.Sprintf("%s:%d", endpoint, status)
	r.mu.Lock()
	if r.champSelect.readFailures == nil {
		r.champSelect.readFailures = map[string]bool{}
	}
	first := !r.champSelect.readFailures[key]
	r.champSelect.readFailures[key] = true
	r.mu.Unlock()
	if !first {
		return
	}
	// A 200 response may still fail JSON decoding. Status zero identifies a
	// transport/decode failure here without exporting arbitrary response bodies.
	r.record(map[string]any{"event": "champselect_read_failed", "endpoint": endpoint, "status_code": status})
	r.champSelectLog("fail", fmt.Sprintf("无法读取 %s，已暂停本次禁用/选用判定；请导出诊断日志", endpoint))
	r.emit("watch:failed:champselect-read:" + endpoint)
}

func (r *watchRunner) champSelectLog(kind, message string) {
	r.champSelectChampionLog(kind, message, 0)
}

// Keep the machine ID structured; the UI resolves the display name from the
// same current catalog as its champion cards, without changing diagnostic IDs.
func (r *watchRunner) champSelectChampionLog(kind, message string, championID int64) {
	if r == nil || strings.TrimSpace(message) == "" {
		return
	}
	r.mu.Lock()
	r.champSelect.records = append(r.champSelect.records, champSelectRuntimeRecord{At: time.Now(), Kind: kind, Message: message, ChampionID: championID})
	if len(r.champSelect.records) > 50 {
		r.champSelect.records = append([]champSelectRuntimeRecord(nil), r.champSelect.records[len(r.champSelect.records)-50:]...)
	}
	r.mu.Unlock()
}

func (r *watchRunner) champSelectSnapshot() champSelectRuntimeResponse {
	response := champSelectRuntimeResponse{BanStates: map[string]string{}, PickStates: map[string]string{}, Owned: map[string]bool{}}
	if r == nil {
		return response
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	response.Active = r.champSelect.active
	response.Phase = r.champSelect.phase
	response.TimerPhase = r.champSelect.lastSession.Timer.Phase
	response.PickIntent = strings.EqualFold(response.TimerPhase, "PLANNING") && !r.champSelect.lastSession.BenchEnabled
	response.BanSource = r.champSelect.banSource
	response.GroupID = r.champSelect.groupID
	response.Position = r.champSelect.position
	response.RemainingMS = r.champSelect.remainingMS
	response.BenchEnabled = r.champSelect.benchEnabled
	response.SessionPaused = r.champSelect.sessionPaused
	response.Unsupported = r.champSelect.groupID == "unsupported"
	if action, ok := champSelectExecutableAction(r.champSelect.lastSession, response.PickIntent); ok {
		response.ActionType = strings.ToLower(action.Type)
	}
	if r.champSelect.active {
		response.BanCapabilityKnown = true
		response.HasBanAction = champSelectSessionHasActionType(r.champSelect.lastSession, "ban")
	}
	response.Records = append([]champSelectRuntimeRecord(nil), r.champSelect.records...)
	for championID, champion := range r.champSelect.grid {
		key := fmt.Sprintf("%d", championID)
		response.Owned[key] = champion.Owned
		response.BanStates[key] = champSelectChampionState("ban", championID, r.champSelect.bannable, champion)
		if response.BanStates[key] == "available" && strings.HasPrefix(response.BanSource, "wildcard-") {
			response.BanStates[key] = "verify-hover"
		}
		response.PickStates[key] = champSelectChampionState("pick", championID, r.champSelect.pickable, champion)
	}
	if decision, ok := r.champSelect.decision[champSelectActionBan]; ok {
		response.ActiveBanID = decision.ChampionID
	}
	if decision, ok := r.champSelect.decision[champSelectActionPick]; ok {
		response.ActivePickID = decision.ChampionID
	}
	for side, championID := range r.champSelect.takeoverChampion {
		if championID <= 0 {
			continue
		}
		states := response.PickStates
		if side == "ban" {
			states = response.BanStates
		}
		states[fmt.Sprint(championID)] = "manual-takeover"
	}
	return response
}

func champSelectChampionState(side string, championID int64, available map[int64]struct{}, champion champSelectGridChampion) string {
	if champion.TeammatePicked {
		return "teammate-picked"
	}
	if side == "ban" && champion.LocalIntent {
		return "own-intent"
	}
	if champion.TeammateIntent || champion.SelectionStatus.PickIntented && !champion.SelectionStatus.PickIntentedByMe {
		return "intent"
	}
	if side == "ban" && champion.SelectionStatus.IsBanned || side == "pick" && champion.SelectionStatus.PickedByOtherOrBanned {
		return "gone"
	}
	if _, ok := available[championID]; ok {
		return "available"
	}
	if side == "ban" && len(available) == 0 {
		return "list-empty"
	}
	return "unavailable"
}

func (a *app) handleChampSelectGroups(w http.ResponseWriter, _ *http.Request) {
	respondJSON(w, champSelectGroupDefinitionsForClient())
}

func (a *app) handleChampSelectState(w http.ResponseWriter, _ *http.Request) {
	runner := a.activeWatch()
	if runner == nil {
		http.Error(w, "征召托管不可用", http.StatusServiceUnavailable)
		return
	}
	respondJSON(w, runner.champSelectSnapshot())
}

func (a *app) handleChampSelectPause(w http.ResponseWriter, request *http.Request) {
	runner := a.activeWatch()
	if runner == nil {
		http.Error(w, "征召托管不可用", http.StatusServiceUnavailable)
		return
	}
	var payload struct {
		Paused bool `json:"paused"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, request.Body, 1024)).Decode(&payload); err != nil {
		http.Error(w, "无效的暂停设置", http.StatusBadRequest)
		return
	}
	runner.mu.Lock()
	runner.champSelect.sessionPaused = payload.Paused
	for _, action := range []string{champSelectActionBan, champSelectActionPick, champSelectActionBench, champSelectActionTrade} {
		if pending := runner.pending[action]; payload.Paused && pending != nil {
			pending.cancel()
			delete(runner.pending, action)
			delete(runner.champSelect.decision, action)
		}
	}
	runner.mu.Unlock()
	state := "resumed"
	if payload.Paused {
		state = "paused"
	}
	runner.champSelectLog("warn", "本局征召托管已"+map[bool]string{true: "暂停", false: "恢复"}[payload.Paused])
	runner.emit("watch:session:champselect-" + state)
	respondJSON(w, runner.champSelectSnapshot())
}
