package main

// watch_rules.go owns the opt-in LCU automation rules used by the Suite page.
// The legacy gameplay/convenience route is kept as a forwarding compatibility
// layer; this runner is the only writer of convenience.json.

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	convenienceSettingsFile = "convenience.json"
	watchSettingsVersion    = 2
)

type watchTimedRule struct {
	Enabled bool `json:"enabled"`
	DelayMS int  `json:"delayMs"`
}

type watchToggleRule struct {
	Enabled bool `json:"enabled"`
}

type watchHonorRule struct {
	Enabled  bool   `json:"enabled"`
	Strategy string `json:"strategy"`
}

type watchBroadcastRule struct {
	Enabled    bool   `json:"enabled"`
	Visibility string `json:"visibility"`
}

type watchInvitationRule struct {
	Enabled  bool              `json:"enabled"`
	Policies map[string]string `json:"policies"`
}

type watchMatchmakingRule struct {
	Enabled      bool `json:"enabled"`
	DelayMS      int  `json:"delayMs"`
	MinPartySize int  `json:"minPartySize"`
}

type facadeLoginResetSettings struct {
	StatusMessageEnabled bool           `json:"statusMessageEnabled"`
	StatusMessage        string         `json:"statusMessage"`
	RankEnabled          bool           `json:"rankEnabled"`
	Rank                 map[string]any `json:"rank,omitempty"`
}

type watchRuleSet struct {
	AutoAccept        watchTimedRule       `json:"autoAccept"`
	AutoReconnect     watchTimedRule       `json:"autoReconnect"`
	AutoPlayAgain     watchToggleRule      `json:"autoPlayAgain"`
	AutoHonor         watchHonorRule       `json:"autoHonor"`
	SkipCelebration   watchToggleRule      `json:"skipCelebration"`
	PositionBroadcast watchBroadcastRule   `json:"positionBroadcast"`
	PromoteLeader     watchToggleRule      `json:"promoteLeader"`
	Invitations       watchInvitationRule  `json:"invitations"`
	AutoMatchmaking   watchMatchmakingRule `json:"autoMatchmaking"`
}

type watchSettings struct {
	SchemaVersion int                      `json:"schemaVersion"`
	MasterEnabled bool                     `json:"masterEnabled"`
	Rules         watchRuleSet             `json:"rules"`
	Facade        facadeLoginResetSettings `json:"facade"`
}

// convenienceSettings is the old renderer contract. It is deliberately kept
// narrow and is translated into watchSettings by the compatibility handler.
type convenienceSettings struct {
	AutoAccept    bool `json:"autoAccept"`
	AutoPlayAgain bool `json:"autoPlayAgain"`
	AutoReconnect bool `json:"autoReconnect"`
}

type watchRunner struct {
	mu                  sync.Mutex
	settings            watchSettings
	lastRun             map[string]time.Time
	pending             map[string]context.CancelFunc
	notify              func(event string)
	honorInProgress     bool
	deferredPlayAgain   bool
	autoMatchStarted    bool
	promotedForLobby    bool
	broadcastForSession bool
}

// Alias the old name so focused legacy tests and downstream integrations keep
// compiling while all live code uses watchRunner.
type convenienceRunner = watchRunner

func defaultWatchSettings() watchSettings {
	return watchSettings{
		SchemaVersion: watchSettingsVersion,
		MasterEnabled: true,
		Rules: watchRuleSet{
			AutoAccept:        watchTimedRule{DelayMS: 1500},
			AutoReconnect:     watchTimedRule{DelayMS: 10000},
			AutoHonor:         watchHonorRule{Strategy: "prefer-party"},
			PositionBroadcast: watchBroadcastRule{Visibility: "self"},
			Invitations:       watchInvitationRule{Policies: map[string]string{}},
			AutoMatchmaking:   watchMatchmakingRule{DelayMS: 5000, MinPartySize: 1},
		},
		Facade: facadeLoginResetSettings{Rank: map[string]any{}},
	}
}

func newWatchRunner(store *localStore, notify func(event string)) *watchRunner {
	return &watchRunner{
		settings: loadWatchSettings(store),
		lastRun:  make(map[string]time.Time),
		pending:  make(map[string]context.CancelFunc),
		notify:   notify,
	}
}

func newConvenienceRunner(store *localStore, notify func(event string)) *convenienceRunner {
	return newWatchRunner(store, notify)
}

func loadWatchSettings(store *localStore) watchSettings {
	settings := defaultWatchSettings()
	if store == nil {
		return settings
	}
	store.mu.Lock()
	data, err := os.ReadFile(filepath.Join(store.root, convenienceSettingsFile))
	store.mu.Unlock()
	if err != nil || len(data) > 32<<10 {
		return settings
	}
	var keys map[string]json.RawMessage
	if json.Unmarshal(data, &keys) != nil || json.Unmarshal(data, &settings) != nil {
		return defaultWatchSettings()
	}
	// Version 1 stored the three original rules as top-level booleans.
	if _, current := keys["rules"]; !current {
		var legacy convenienceSettings
		if json.Unmarshal(data, &legacy) == nil {
			settings.Rules.AutoAccept.Enabled = legacy.AutoAccept
			settings.Rules.AutoPlayAgain.Enabled = legacy.AutoPlayAgain
			settings.Rules.AutoReconnect.Enabled = legacy.AutoReconnect
			if legacy.AutoAccept || legacy.AutoPlayAgain || legacy.AutoReconnect {
				settings.MasterEnabled = true
			}
		}
	}
	return normalizeWatchSettings(settings)
}

func loadConvenienceSettings(store *localStore) convenienceSettings {
	settings := loadWatchSettings(store)
	return legacyConvenienceSettings(settings)
}

func saveWatchSettings(store *localStore, settings watchSettings) error {
	if store == nil {
		return errors.New("本地存储不可用")
	}
	settings = normalizeWatchSettings(settings)
	data, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	return atomicWriteFile(filepath.Join(store.root, convenienceSettingsFile), data, 0o600)
}

func saveConvenienceSettings(store *localStore, legacy convenienceSettings) error {
	settings := loadWatchSettings(store)
	settings.Rules.AutoAccept.Enabled = legacy.AutoAccept
	settings.Rules.AutoPlayAgain.Enabled = legacy.AutoPlayAgain
	settings.Rules.AutoReconnect.Enabled = legacy.AutoReconnect
	return saveWatchSettings(store, settings)
}

func normalizeWatchSettings(settings watchSettings) watchSettings {
	settings.SchemaVersion = watchSettingsVersion
	settings.Rules.AutoAccept.DelayMS = clampInt(settings.Rules.AutoAccept.DelayMS, 0, 10000, 1500)
	settings.Rules.AutoReconnect.DelayMS = clampInt(settings.Rules.AutoReconnect.DelayMS, 3000, 30000, 10000)
	settings.Rules.AutoMatchmaking.DelayMS = clampInt(settings.Rules.AutoMatchmaking.DelayMS, 0, 30000, 5000)
	settings.Rules.AutoMatchmaking.MinPartySize = clampInt(settings.Rules.AutoMatchmaking.MinPartySize, 1, 5, 1)
	switch settings.Rules.AutoHonor.Strategy {
	case "prefer-party", "party-only", "any-teammate", "abstain":
	default:
		settings.Rules.AutoHonor.Strategy = "prefer-party"
	}
	switch settings.Rules.PositionBroadcast.Visibility {
	case "self", "team":
	default:
		settings.Rules.PositionBroadcast.Visibility = "self"
	}
	if settings.Rules.Invitations.Policies == nil {
		settings.Rules.Invitations.Policies = map[string]string{}
	}
	for queue, policy := range settings.Rules.Invitations.Policies {
		switch policy {
		case "accept", "decline", "ignore":
		default:
			delete(settings.Rules.Invitations.Policies, queue)
		}
	}
	if len(settings.Facade.StatusMessage) > 200 {
		settings.Facade.StatusMessage = settings.Facade.StatusMessage[:200]
	}
	if settings.Facade.Rank == nil {
		settings.Facade.Rank = map[string]any{}
	}
	return settings
}

func clampInt(value, minimum, maximum, fallback int) int {
	if value == 0 && minimum > 0 {
		value = fallback
	}
	if value < minimum {
		return minimum
	}
	if value > maximum {
		return maximum
	}
	return value
}

func legacyConvenienceSettings(settings watchSettings) convenienceSettings {
	return convenienceSettings{
		AutoAccept:    settings.Rules.AutoAccept.Enabled,
		AutoPlayAgain: settings.Rules.AutoPlayAgain.Enabled,
		AutoReconnect: settings.Rules.AutoReconnect.Enabled,
	}
}

func cloneWatchSettings(settings watchSettings) watchSettings {
	clone := settings
	clone.Rules.Invitations.Policies = make(map[string]string, len(settings.Rules.Invitations.Policies))
	for key, value := range settings.Rules.Invitations.Policies {
		clone.Rules.Invitations.Policies[key] = value
	}
	clone.Facade.Rank = make(map[string]any, len(settings.Facade.Rank))
	for key, value := range settings.Facade.Rank {
		clone.Facade.Rank[key] = value
	}
	return clone
}

func (r *watchRunner) currentWatch() watchSettings {
	if r == nil {
		return defaultWatchSettings()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return cloneWatchSettings(r.settings)
}

func (r *watchRunner) current() convenienceSettings {
	return legacyConvenienceSettings(r.currentWatch())
}

func (r *watchRunner) apply(value any) {
	if r == nil {
		return
	}
	settings := r.currentWatch()
	switch typed := value.(type) {
	case watchSettings:
		settings = typed
	case convenienceSettings:
		settings.Rules.AutoAccept.Enabled = typed.AutoAccept
		settings.Rules.AutoPlayAgain.Enabled = typed.AutoPlayAgain
		settings.Rules.AutoReconnect.Enabled = typed.AutoReconnect
	default:
		return
	}
	r.mu.Lock()
	r.settings = normalizeWatchSettings(settings)
	normalized := cloneWatchSettings(r.settings)
	cancels := make([]context.CancelFunc, 0, len(r.pending))
	for action, cancel := range r.pending {
		if !normalized.MasterEnabled || !watchActionEnabled(normalized, action) {
			cancels = append(cancels, cancel)
			delete(r.pending, action)
		}
	}
	r.mu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
}

func watchActionEnabled(settings watchSettings, action string) bool {
	switch action {
	case "accept":
		return settings.Rules.AutoAccept.Enabled
	case "reconnect":
		return settings.Rules.AutoReconnect.Enabled
	case "play-again":
		return settings.Rules.AutoPlayAgain.Enabled
	case "skip-celebration":
		return settings.Rules.SkipCelebration.Enabled
	case "promote-leader":
		return settings.Rules.PromoteLeader.Enabled
	case "auto-matchmaking":
		return settings.Rules.AutoMatchmaking.Enabled
	default:
		return true
	}
}

func (r *watchRunner) handlePhase(client *LCUClient, phase string) {
	if r == nil || client == nil {
		return
	}
	settings := r.currentWatch()
	if !settings.MasterEnabled {
		r.cancelAllPending()
		return
	}
	if phase != "ReadyCheck" {
		r.cancelPending("accept")
	}
	if phase != "Reconnect" {
		r.cancelPending("reconnect")
	}
	switch phase {
	case "ReadyCheck":
		if settings.Rules.AutoAccept.Enabled {
			r.schedule(client, "accept", settings.Rules.AutoAccept.DelayMS, http.MethodPost, "/lol-matchmaking/v1/ready-check/accept", nil)
		}
	case "WaitingForStats", "PreEndOfGame", "EndOfGame":
		if settings.Rules.AutoPlayAgain.Enabled {
			delay := map[string]int{"WaitingForStats": 10000, "PreEndOfGame": 3250, "EndOfGame": 1575}[phase]
			r.schedulePlayAgain(client, delay)
		}
		if phase == "EndOfGame" {
			r.mu.Lock()
			r.autoMatchStarted = false
			r.promotedForLobby = false
			r.broadcastForSession = false
			r.mu.Unlock()
		}
	case "Reconnect":
		if settings.Rules.AutoReconnect.Enabled {
			r.schedule(client, "reconnect", settings.Rules.AutoReconnect.DelayMS, http.MethodPost, "/lol-gameflow/v1/reconnect", nil)
		}
	}
}

func (r *watchRunner) handleEvent(client *LCUClient, event LCUEvent, current Summoner) {
	if r == nil || client == nil {
		return
	}
	settings := r.currentWatch()
	if !settings.MasterEnabled {
		return
	}
	uri := strings.ToLower(event.URI)
	switch {
	case strings.HasPrefix(uri, "/lol-honor-v2/v1/ballot") && settings.Rules.AutoHonor.Enabled:
		go r.handleHonor(client, event.Data, current, settings.Rules.AutoHonor)
	case strings.HasPrefix(uri, "/lol-pre-end-of-game/v1/currentsequenceevent") && settings.Rules.SkipCelebration.Enabled:
		var payload struct {
			Name string `json:"name"`
		}
		if json.Unmarshal(event.Data, &payload) == nil && payload.Name == "missions-celebration" {
			r.schedule(client, "skip-celebration", 0, http.MethodPost, "/lol-pre-end-of-game/v1/complete/missions-celebration", nil)
		}
	case strings.HasPrefix(uri, "/lol-lobby/v2/received-invitations") && settings.Rules.Invitations.Enabled:
		go r.handleInvitations(client, settings.Rules.Invitations)
	case strings.HasPrefix(uri, "/lol-lobby/v2/lobby"):
		go r.handleLobby(client, current, settings.Rules)
	}
}

func (r *watchRunner) handleChampSelect(client *LCUClient, current Summoner) {
	settings := r.currentWatch()
	if r == nil || client == nil || !settings.MasterEnabled || !settings.Rules.PositionBroadcast.Enabled {
		return
	}
	r.mu.Lock()
	if r.broadcastForSession {
		r.mu.Unlock()
		return
	}
	r.broadcastForSession = true
	r.mu.Unlock()
	go r.broadcastPosition(client, current, settings.Rules.PositionBroadcast)
}

func (r *watchRunner) schedulePlayAgain(client *LCUClient, delayMS int) {
	r.mu.Lock()
	if r.honorInProgress {
		r.deferredPlayAgain = true
		r.mu.Unlock()
		r.emit("watch:priority:honoring")
		return
	}
	r.mu.Unlock()
	// The original runner's 1200 ms settle delay remains the lower bound.
	if delayMS < 1200 {
		delayMS = 1200
	}
	r.schedule(client, "play-again", delayMS, http.MethodPost, "/lol-lobby/v2/play-again", nil)
}

func (r *watchRunner) schedule(client *LCUClient, action string, delayMS int, method, path string, body any) {
	if !r.markRun(action, 3*time.Second) {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	r.mu.Lock()
	if previous := r.pending[action]; previous != nil {
		previous()
	}
	r.pending[action] = cancel
	r.mu.Unlock()
	r.emit(fmt.Sprintf("watch:armed:%s:%d", action, delayMS))
	go func() {
		defer func() {
			r.mu.Lock()
			delete(r.pending, action)
			r.mu.Unlock()
		}()
		if delayMS > 0 {
			timer := time.NewTimer(time.Duration(delayMS) * time.Millisecond)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
			}
		}
		requestCtx, requestCancel := context.WithTimeout(ctx, 8*time.Second)
		defer requestCancel()
		if err := client.RequestJSON(requestCtx, method, path, body, nil); err != nil {
			r.emit("watch:failed:" + action)
			return
		}
		r.emit("watch:fired:" + action)
	}()
}

func (r *watchRunner) run(client *LCUClient, action, method, path string) {
	delay := 0
	if action == "play-again" {
		delay = 1200
	}
	r.schedule(client, action, delay, method, path, nil)
}

func (r *watchRunner) markRun(action string, window time.Duration) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if last, ok := r.lastRun[action]; ok && time.Since(last) < window {
		return false
	}
	r.lastRun[action] = time.Now()
	return true
}

func (r *watchRunner) cancelPending(action string) {
	r.mu.Lock()
	cancel := r.pending[action]
	delete(r.pending, action)
	r.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (r *watchRunner) cancelAllPending() {
	r.mu.Lock()
	cancels := make([]context.CancelFunc, 0, len(r.pending))
	for action, cancel := range r.pending {
		cancels = append(cancels, cancel)
		delete(r.pending, action)
	}
	r.mu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
}

func (r *watchRunner) emit(event string) {
	if r != nil && r.notify != nil {
		r.notify(event)
	}
}

func (r *watchRunner) handleHonor(client *LCUClient, ballotData json.RawMessage, current Summoner, rule watchHonorRule) {
	r.mu.Lock()
	if r.honorInProgress {
		r.mu.Unlock()
		return
	}
	r.honorInProgress = true
	if cancel := r.pending["play-again"]; cancel != nil {
		cancel()
		delete(r.pending, "play-again")
		r.deferredPlayAgain = true
	}
	r.mu.Unlock()
	r.emit("watch:priority:honoring")
	defer func() {
		r.mu.Lock()
		r.honorInProgress = false
		deferred := r.deferredPlayAgain
		r.deferredPlayAgain = false
		r.mu.Unlock()
		if deferred && r.currentWatch().MasterEnabled && r.currentWatch().Rules.AutoPlayAgain.Enabled {
			r.schedule(client, "play-again", 500, http.MethodPost, "/lol-lobby/v2/play-again", nil)
		}
	}()

	requestCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	var party any
	if rule.Strategy != "abstain" {
		if err := client.RequestJSON(requestCtx, http.MethodGet, "/lol-lobby/v2/party/eog-status", nil, &party); err != nil {
			r.emit("watch:failed:auto-honor")
			return
		}
	}
	recipient := chooseHonorRecipient(ballotData, party, current.PUUID, rule.Strategy)
	var actionErr error
	if recipient != "" && rule.Strategy != "abstain" {
		actionErr = client.RequestJSON(requestCtx, http.MethodPost, "/lol-honor/v1/honor", map[string]any{"honorType": "HEART", "recipientPuuid": recipient}, nil)
	}
	if actionErr == nil {
		actionErr = client.RequestJSON(requestCtx, http.MethodPost, "/lol-honor/v1/ballot", nil, nil)
	}
	if actionErr != nil {
		r.emit("watch:failed:auto-honor")
	} else {
		r.emit("watch:fired:auto-honor")
	}
}

type honorCandidate struct {
	PUUID      string
	TeamID     int64
	Enemy      bool
	Ally       bool
	Eligible   bool
	PartyMatch bool
}

func chooseHonorRecipient(ballotData json.RawMessage, party any, selfPUUID, strategy string) string {
	if strategy == "abstain" {
		return ""
	}
	partyIDs := collectStringFields(party, "puuid")
	delete(partyIDs, selfPUUID)
	var raw any
	_ = json.Unmarshal(ballotData, &raw)
	candidates := collectHonorCandidates(raw)
	for index := range candidates {
		candidates[index].PartyMatch = partyIDs[candidates[index].PUUID]
	}
	for _, candidate := range candidates {
		if candidate.PUUID != "" && candidate.PUUID != selfPUUID && candidate.Eligible && !candidate.Enemy && candidate.PartyMatch {
			return candidate.PUUID
		}
	}
	if strategy == "party-only" {
		return ""
	}
	for _, candidate := range candidates {
		// Unknown team data is not enough evidence; this guard prevents enemy votes.
		if candidate.PUUID != "" && candidate.PUUID != selfPUUID && candidate.Eligible && !candidate.Enemy && candidate.Ally {
			return candidate.PUUID
		}
	}
	return ""
}

func collectHonorCandidates(value any) []honorCandidate {
	result := []honorCandidate{}
	var walk func(any)
	walk = func(node any) {
		switch typed := node.(type) {
		case []any:
			for _, child := range typed {
				walk(child)
			}
		case map[string]any:
			puuid := anyString(typed, "puuid", "recipientPuuid")
			if puuid != "" {
				eligible, knownEligible := anyBool(typed, "eligible", "isEligible", "honorable")
				if !knownEligible {
					eligible = true
				}
				enemy, _ := anyBool(typed, "isEnemy", "isOpposingTeam", "enemy")
				ally, _ := anyBool(typed, "isAlly", "isTeammate", "ally")
				result = append(result, honorCandidate{PUUID: puuid, TeamID: anyInt(typed, "teamId", "teamID"), Enemy: enemy, Ally: ally, Eligible: eligible})
			}
			for _, child := range typed {
				walk(child)
			}
		}
	}
	walk(value)
	return result
}

func (r *watchRunner) handleLobby(client *LCUClient, current Summoner, rules watchRuleSet) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	var lobby map[string]any
	if err := client.RequestJSON(ctx, http.MethodGet, "/lol-lobby/v2/lobby", nil, &lobby); err != nil {
		if rules.PromoteLeader.Enabled {
			r.emit("watch:failed:promote-leader")
		}
		if rules.AutoMatchmaking.Enabled {
			r.emit("watch:failed:auto-matchmaking")
		}
		return
	}
	local, _ := lobby["localMember"].(map[string]any)
	leader, _ := anyBool(local, "isLeader")
	if !leader {
		return
	}
	members, _ := lobby["members"].([]any)
	if rules.PromoteLeader.Enabled {
		r.mu.Lock()
		already := r.promotedForLobby
		if !already {
			r.promotedForLobby = true
		}
		r.mu.Unlock()
		if !already {
			candidates := make([]int64, 0, len(members))
			ready := make([]int64, 0, len(members))
			for _, item := range members {
				member, _ := item.(map[string]any)
				id := anyInt(member, "summonerId", "summonerID")
				if id <= 0 || id == current.SummonerID {
					continue
				}
				candidates = append(candidates, id)
				if value, _ := anyBool(member, "isReady", "ready"); value {
					ready = append(ready, id)
				}
			}
			if len(ready) > 0 {
				candidates = ready
			}
			if id, ok := secureRandomMember(candidates); ok {
				path := "/lol-lobby/v2/lobby/members/" + strconv.FormatInt(id, 10) + "/promote"
				r.schedule(client, "promote-leader", 0, http.MethodPost, path, nil)
			}
		}
	}
	if rules.AutoMatchmaking.Enabled && len(members) >= rules.AutoMatchmaking.MinPartySize {
		r.mu.Lock()
		started := r.autoMatchStarted
		if !started {
			r.autoMatchStarted = true
		}
		r.mu.Unlock()
		if !started {
			r.schedule(client, "auto-matchmaking", rules.AutoMatchmaking.DelayMS, http.MethodPost, "/lol-lobby/v2/lobby/matchmaking/search", nil)
		}
	}
}

func secureRandomMember(values []int64) (int64, bool) {
	if len(values) == 0 {
		return 0, false
	}
	index, err := cryptorand.Int(cryptorand.Reader, big.NewInt(int64(len(values))))
	if err != nil {
		return values[0], true
	}
	return values[index.Int64()], true
}

func (r *watchRunner) handleInvitations(client *LCUClient, rule watchInvitationRule) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	var invitations []map[string]any
	if err := client.RequestJSON(ctx, http.MethodGet, "/lol-lobby/v2/received-invitations", nil, &invitations); err != nil {
		r.emit("watch:failed:invitations")
		return
	}
	for _, invitation := range invitations {
		id := anyString(invitation, "invitationId", "id")
		if !safeLCUIdentifier(id) || !r.markRun("invitation:"+id, 10*time.Minute) {
			continue
		}
		queue := strconv.FormatInt(anyInt(invitation, "queueId", "queueID"), 10)
		policy := rule.Policies[queue]
		if policy == "" {
			policy = rule.Policies["default"]
		}
		if policy != "accept" && policy != "decline" {
			continue
		}
		path := "/lol-lobby/v2/received-invitations/" + id + "/" + policy
		if err := client.RequestJSON(ctx, http.MethodPost, path, nil, nil); err != nil {
			r.emit("watch:failed:invitations")
		} else {
			r.emit("watch:fired:invitations")
		}
	}
}

func (r *watchRunner) broadcastPosition(client *LCUClient, current Summoner, rule watchBroadcastRule) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	var session map[string]any
	if err := client.RequestJSON(ctx, http.MethodGet, "/lol-champ-select/v1/session", nil, &session); err != nil {
		r.emit("watch:failed:position-broadcast")
		return
	}
	gameData, _ := session["gameData"].(map[string]any)
	mode := strings.ToUpper(anyString(gameData, "gameMode"))
	bench, _ := anyBool(session, "benchEnabled")
	if !bench || (mode != "ARAM" && mode != "KIWI") {
		return
	}
	team := strings.ToUpper(anyString(session, "team", "teamId", "teamID"))
	if team == "" {
		team = strings.ToUpper(anyString(gameData, "team", "teamId", "teamID"))
	}
	label := ""
	switch team {
	case "1", "ONE", "BLUE", "100":
		label = "蓝色方"
	case "2", "TWO", "RED", "200":
		label = "红色方"
	default:
		r.emit("watch:failed:position-broadcast")
		return
	}
	var me map[string]any
	var conversations []map[string]any
	if client.RequestJSON(ctx, http.MethodGet, "/lol-chat/v1/me", nil, &me) != nil || client.RequestJSON(ctx, http.MethodGet, "/lol-chat/v1/conversations", nil, &conversations) != nil {
		r.emit("watch:failed:position-broadcast")
		return
	}
	conversationID := ""
	for _, conversation := range conversations {
		kind := strings.ToLower(anyString(conversation, "type"))
		if strings.Contains(kind, "champion") || strings.Contains(kind, "champselect") {
			conversationID = anyString(conversation, "id")
			break
		}
	}
	if !safeLCUIdentifier(conversationID) {
		r.emit("watch:failed:position-broadcast")
		return
	}
	messageType := "celebration"
	if rule.Visibility == "team" {
		messageType = "chat"
	}
	body := map[string]any{
		"body": "当前阵营位置：" + label, "fromId": anyString(me, "id"), "fromPid": "",
		"fromSummonerId": current.SummonerID, "id": "", "isHistorical": false, "timestamp": "", "type": messageType,
	}
	path := "/lol-chat/v1/conversations/" + conversationID + "/messages"
	if err := client.RequestJSON(ctx, http.MethodPost, path, body, nil); err != nil {
		r.emit("watch:failed:position-broadcast")
	} else {
		r.emit("watch:fired:position-broadcast")
	}
}

func collectStringFields(value any, field string) map[string]bool {
	result := map[string]bool{}
	var walk func(any)
	walk = func(node any) {
		switch typed := node.(type) {
		case []any:
			for _, child := range typed {
				walk(child)
			}
		case map[string]any:
			if value := anyString(typed, field); value != "" {
				result[value] = true
			}
			for _, child := range typed {
				walk(child)
			}
		}
	}
	walk(value)
	return result
}

func anyString(value map[string]any, keys ...string) string {
	for _, key := range keys {
		if raw, ok := value[key]; ok {
			switch typed := raw.(type) {
			case string:
				return strings.TrimSpace(typed)
			case json.Number:
				return typed.String()
			case float64:
				return strconv.FormatInt(int64(typed), 10)
			}
		}
	}
	return ""
}

func anyInt(value map[string]any, keys ...string) int64 {
	for _, key := range keys {
		switch typed := value[key].(type) {
		case float64:
			return int64(typed)
		case json.Number:
			parsed, _ := typed.Int64()
			return parsed
		case string:
			parsed, _ := strconv.ParseInt(typed, 10, 64)
			return parsed
		}
	}
	return 0
}

func anyBool(value map[string]any, keys ...string) (bool, bool) {
	for _, key := range keys {
		if raw, ok := value[key]; ok {
			switch typed := raw.(type) {
			case bool:
				return typed, true
			case string:
				parsed, err := strconv.ParseBool(typed)
				return parsed, err == nil
			}
		}
	}
	return false, false
}

func safeLCUIdentifier(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 256 || value == "." || value == ".." {
		return false
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') || (character >= '0' && character <= '9') || strings.ContainsRune("-_.:", character) {
			continue
		}
		return false
	}
	return true
}

func (a *app) activeWatch() *watchRunner {
	if a == nil {
		return nil
	}
	if a.watch != nil {
		return a.watch
	}
	return a.convenience
}

func (a *app) handleWatchRules(w http.ResponseWriter, r *http.Request) {
	runner := a.activeWatch()
	if runner == nil {
		http.Error(w, "值守规则不可用", http.StatusServiceUnavailable)
		return
	}
	if r.Method == http.MethodGet {
		respondJSON(w, runner.currentWatch())
		return
	}
	var request watchSettings
	if err := decodeJSONRequest(r, &request, 32<<10); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	request = normalizeWatchSettings(request)
	if err := saveWatchSettings(a.storage, request); err != nil {
		http.Error(w, "值守规则无法保存", http.StatusServiceUnavailable)
		return
	}
	runner.apply(request)
	respondJSON(w, request)
}

func (a *app) handleGameplayConvenience(w http.ResponseWriter, r *http.Request) {
	// Compatibility forwarding: this route never owns its own copy of settings.
	runner := a.activeWatch()
	if runner == nil {
		if r.Method == http.MethodGet {
			respondJSON(w, convenienceSettings{})
			return
		}
		http.Error(w, "便捷设置不可用", http.StatusServiceUnavailable)
		return
	}
	if r.Method == http.MethodGet {
		respondJSON(w, runner.current())
		return
	}
	var request convenienceSettings
	if err := decodeJSONRequest(r, &request, 4<<10); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	settings := runner.currentWatch()
	settings.Rules.AutoAccept.Enabled = request.AutoAccept
	settings.Rules.AutoPlayAgain.Enabled = request.AutoPlayAgain
	settings.Rules.AutoReconnect.Enabled = request.AutoReconnect
	if err := saveWatchSettings(a.storage, settings); err != nil {
		http.Error(w, "便捷设置无法保存", http.StatusServiceUnavailable)
		return
	}
	runner.apply(settings)
	respondJSON(w, request)
}
