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
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	convenienceSettingsFile = "convenience.json"
	watchSettingsVersion    = 3
	autoMatchMaxAttempts    = 3
)

var autoMatchRetryBackoff = [...]time.Duration{2 * time.Second, 4 * time.Second, 8 * time.Second}

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
	Enabled          bool   `json:"enabled"`
	Visibility       string `json:"visibility"`
	TeamComposition  bool   `json:"teamComposition"`
	AssignedPosition bool   `json:"assignedPosition"`
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
	ChampSelect   champSelectSettings      `json:"champSelect"`
}

type watchPendingAction struct {
	fireAt time.Time
	cancel context.CancelFunc
}

// convenienceSettings is the old renderer contract. It is deliberately kept
// narrow and is translated into watchSettings by the compatibility handler.
type convenienceSettings struct {
	AutoAccept    bool `json:"autoAccept"`
	AutoPlayAgain bool `json:"autoPlayAgain"`
	AutoReconnect bool `json:"autoReconnect"`
}

type watchRunner struct {
	// Snapshot-only lookup. Never load a catalog to compose a chat message.
	broadcastChampionNames func() map[int64]string
	now                    func() time.Time
	wait                   func(context.Context, time.Duration) error
	champDiagnosticSession string
	champDiagnosticSamples map[string]diagnosticSample
	writeContext           context.Context
	writeCancel            context.CancelFunc
	customSession          bool
	customObservation      [2]string
	mu                     sync.Mutex
	settings               watchSettings
	lastRun                map[string]time.Time
	pending                map[string]*watchPendingAction
	notify                 func(event string)
	observe                func(event map[string]any)
	honorInProgress        bool
	deferredPlayAgain      bool
	autoMatchStarted       bool
	autoMatchInFlight      bool
	autoMatchExhausted     bool
	autoMatchRetryDelays   []time.Duration
	lobbyShapeKeys         map[string]struct{}
	promotedForLobby       bool
	broadcastForSession    bool
	acceptedForReadyCheck  bool
	champSelect            champSelectRuntimeStore
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
			PositionBroadcast: watchBroadcastRule{Visibility: "self", AssignedPosition: true},
			Invitations:       watchInvitationRule{Policies: map[string]string{}},
			AutoMatchmaking:   watchMatchmakingRule{DelayMS: 5000, MinPartySize: 1},
		},
		Facade:      facadeLoginResetSettings{Rank: map[string]any{}},
		ChampSelect: defaultChampSelectSettings(),
	}
}

func newWatchRunner(store *localStore, notify func(event string)) *watchRunner {
	return &watchRunner{
		settings:    loadWatchSettings(store),
		lastRun:     make(map[string]time.Time),
		pending:     make(map[string]*watchPendingAction),
		notify:      notify,
		champSelect: newChampSelectRuntimeStore(),
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
	// Version 2 had no champSelect object. Use the current empty-sequence default,
	// while initializing the current per-mode defaults and preserving old rules.
	if _, current := keys["champSelect"]; !current {
		settings.ChampSelect = defaultChampSelectSettings()
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
	settings.Rules.PositionBroadcast.AssignedPosition = true // R139: 固定开启，不再提供用户开关。
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
	settings.ChampSelect = normalizeChampSelectSettings(settings.ChampSelect)
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
	clone.ChampSelect = cloneChampSelectSettings(settings.ChampSelect)
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
	if r.settings.Rules.AutoMatchmaking.Enabled != settings.Rules.AutoMatchmaking.Enabled {
		r.autoMatchExhausted = false
	}
	r.settings = normalizeWatchSettings(settings)
	normalized := cloneWatchSettings(r.settings)
	if !normalized.ChampSelect.Enabled && r.champSelect.writeCancel != nil {
		r.champSelect.writeCancel()
		r.champSelect.writeContext, r.champSelect.writeCancel = nil, nil
	}
	if !normalized.MasterEnabled && r.writeCancel != nil {
		r.writeCancel()
		r.writeContext = nil
		r.writeCancel = nil
	}
	cancels := make([]context.CancelFunc, 0, len(r.pending))
	for action, pending := range r.pending {
		if strings.HasPrefix(action, "champselect-") || !normalized.MasterEnabled || !watchActionEnabled(normalized, action) {
			cancels = append(cancels, pending.cancel)
			delete(r.pending, action)
			if action == "auto-matchmaking" {
				r.autoMatchInFlight = false
			}
		}
	}
	for action := range r.champSelect.decision {
		delete(r.champSelect.decision, action)
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
	case "position-broadcast":
		return settings.Rules.PositionBroadcast.Enabled
	default:
		return true
	}
}

func (r *watchRunner) handlePhase(client *LCUClient, phase string) {
	if r == nil || client == nil {
		return
	}
	r.mu.Lock()
	previousPhase := r.champSelect.phase
	r.mu.Unlock()
	r.handleChampSelectPhase(phase)
	settings := r.currentWatch()
	if phase == "None" {
		r.setCustomSession(false)
		r.cancelPending("auto-matchmaking")
		r.mu.Lock()
		if previousPhase != "None" {
			r.autoMatchExhausted = false
		}
		r.autoMatchStarted = false
		r.mu.Unlock()
	}
	if isEndOfGamePhase(phase) {
		r.cancelPending("auto-matchmaking")
		r.mu.Lock()
		r.autoMatchStarted = false
		r.autoMatchExhausted = false
		r.promotedForLobby = false
		r.broadcastForSession = false
		r.mu.Unlock()
	}
	if r.customPaused() {
		return
	}
	if !settings.MasterEnabled {
		r.mu.Lock()
		for action, pending := range r.pending {
			if !strings.HasPrefix(action, "champselect-") {
				pending.cancel()
				delete(r.pending, action)
				if action == "auto-matchmaking" {
					r.autoMatchInFlight = false
				}
			}
		}
		r.mu.Unlock()
		r.mu.Lock()
		r.acceptedForReadyCheck = false
		delete(r.lastRun, "accept")
		r.mu.Unlock()
		return
	}
	if phase != "ReadyCheck" {
		r.cancelPending("accept")
		r.mu.Lock()
		r.acceptedForReadyCheck = false
		delete(r.lastRun, "accept")
		r.mu.Unlock()
	}
	if phase != "Reconnect" {
		r.cancelPending("reconnect")
	}
	switch phase {
	case "Matchmaking":
		if settings.Rules.AutoAccept.Enabled {
			current := nativeAcceptWindowState()
			r.record(acceptWindowDiagnostic("matchmaking", current, current))
		}
	case "ReadyCheck":
		if settings.Rules.AutoAccept.Enabled {
			r.scheduleAccept(client, settings.Rules.AutoAccept.DelayMS)
		}
	case "WaitingForStats", "PreEndOfGame", "EndOfGame":
		r.record(map[string]any{"event": "endgame_trigger", "phase": phase, "play_again_enabled": settings.Rules.AutoPlayAgain.Enabled, "honor_enabled": settings.Rules.AutoHonor.Enabled})
		if settings.Rules.AutoPlayAgain.Enabled {
			delay := map[string]int{"WaitingForStats": 10000, "PreEndOfGame": 3250, "EndOfGame": 1575}[phase]
			r.schedulePlayAgain(client, delay)
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
	uri := strings.ToLower(event.URI)
	if uri == "/lol-lobby/v2/lobby" || uri == "/lol-gameflow/v1/session" {
		var payload map[string]any
		if json.Unmarshal(event.Data, &payload) == nil {
			r.observeCustomSession(payload, uri == "/lol-lobby/v2/lobby")
		}
	}
	if r.customPaused() {
		return
	}
	settings := r.currentWatch()
	if !settings.MasterEnabled {
		return
	}
	switch {
	case strings.HasPrefix(uri, "/lol-matchmaking/v1/ready-check") && settings.Rules.AutoAccept.Enabled:
		if readyCheckAwaitingResponse(event.Data) {
			r.scheduleAccept(client, settings.Rules.AutoAccept.DelayMS)
		}
	case strings.HasPrefix(uri, "/lol-honor-v2/v1/ballot") && settings.Rules.AutoHonor.Enabled:
		r.record(map[string]any{"event": "endgame_trigger", "source": "honor-ballot", "honor_enabled": true})
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

func readyCheckAwaitingResponse(raw json.RawMessage) bool {
	var state string
	if json.Unmarshal(raw, &state) != nil {
		var payload map[string]any
		if json.Unmarshal(raw, &payload) != nil {
			return false
		}
		state = anyString(payload, "state")
	}
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "inprogress", "pending":
		return true
	default:
		return false
	}
}

func (r *watchRunner) handleChampSelect(client *LCUClient, current Summoner) {
	if r == nil || client == nil {
		return
	}
	settings := r.currentWatch()
	if settings.MasterEnabled && settings.Rules.PositionBroadcast.Enabled && !r.customPaused() {
		r.mu.Lock()
		alreadyBroadcasting := r.broadcastForSession || (r.champSelect.phase != "" && r.champSelect.phase != "ChampSelect")
		ctx, cancel := context.WithCancel(context.Background())
		pending := &watchPendingAction{cancel: cancel}
		if !alreadyBroadcasting {
			r.broadcastForSession = true
			if r.pending == nil {
				r.pending = make(map[string]*watchPendingAction)
			}
			r.pending["position-broadcast"] = pending
		}
		r.mu.Unlock()
		if alreadyBroadcasting {
			cancel()
		} else {
			go func() {
				defer cancel()
				defer func() {
					r.mu.Lock()
					if r.pending["position-broadcast"] == pending {
						delete(r.pending, "position-broadcast")
					}
					r.mu.Unlock()
				}()
				// Retry incomplete first frames on a bounded clock, never on every
				// websocket event. Leaving champion select cancels reads and writes.
				for attempt := 0; attempt < 3; attempt++ {
					if !waitWatchDelay(ctx, time.Duration(attempt)*time.Second) {
						return
					}
					result := r.broadcastPositionContext(ctx, client, current, settings.Rules.PositionBroadcast)
					if ctx.Err() != nil || result == "fired" || result == "skipped_mode" || result == "skipped_custom" || result == "write_failed" {
						return
					}
				}
				if ctx.Err() == nil {
					r.emit("watch:skipped:position-broadcast:unavailable")
				}
			}()
		}
	}
	r.handleChampSelectAutomation(client)
}

func (r *watchRunner) schedulePlayAgain(client *LCUClient, delayMS int) {
	r.mu.Lock()
	if r.honorInProgress || r.customSession {
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

func (r *watchRunner) schedule(client *LCUClient, action string, delayMS int, method, path string, body any) bool {
	// A later settlement phase can advance the same pending action. Deduping
	// phase notifications must not preserve an obsolete ten-second deadline.
	if action == "play-again" {
		r.mu.Lock()
		previous := r.pending[action]
		earlier := previous != nil && r.clockNow().Add(time.Duration(delayMS)*time.Millisecond).Before(previous.fireAt)
		r.mu.Unlock()
		if earlier {
			return r.scheduleMarked(client, action, delayMS, method, path, body)
		}
	}
	if !r.markRun(action, 3*time.Second) {
		return false
	}
	return r.scheduleMarked(client, action, delayMS, method, path, body)
}

func (r *watchRunner) scheduleAccept(client *LCUClient, delayMS int) {
	r.mu.Lock()
	if r.acceptedForReadyCheck {
		r.mu.Unlock()
		return
	}
	r.mu.Unlock()
	if !r.markRun("accept", 3*time.Second) {
		return
	}
	r.mu.Lock()
	if r.acceptedForReadyCheck {
		r.mu.Unlock()
		return
	}
	r.acceptedForReadyCheck = true
	r.mu.Unlock()
	current := nativeAcceptWindowState()
	r.record(acceptWindowDiagnostic("ready-check", current, current))
	r.scheduleMarked(client, "accept", delayMS, http.MethodPost, "/lol-matchmaking/v1/ready-check/accept", nil)
}

func (r *watchRunner) scheduleMarked(client *LCUClient, action string, delayMS int, method, path string, body any) bool {
	ctx, cancel := context.WithCancel(context.Background())
	pending := &watchPendingAction{cancel: cancel, fireAt: r.clockNow().Add(time.Duration(delayMS) * time.Millisecond)}
	r.mu.Lock()
	if r.customSession {
		r.mu.Unlock()
		cancel()
		return false
	}
	if previous := r.pending[action]; previous != nil {
		if action == "play-again" && !pending.fireAt.Before(previous.fireAt) {
			r.mu.Unlock()
			cancel()
			return false
		}
		previous.cancel()
	}
	r.pending[action] = pending
	r.mu.Unlock()
	var focusTrace *acceptFocusTrace
	if action == "accept" {
		focusTrace = startAcceptFocusTrace(ctx, client, delayMS, r.record)
	}
	r.emit(fmt.Sprintf("watch:armed:%s:%d", action, delayMS))
	r.record(map[string]any{"event": "watch_action", "action": action, "result": "armed"})
	go func() {
		outcome := "not-attempted"
		attempted := false
		defer func() {
			if focusTrace != nil {
				focusTrace.emit(map[string]any{"event": "accept_focus_action_end", "outcome": outcome, "request_attempted": attempted, "context_canceled": ctx.Err() != nil, "status_code": focusTrace.httpStatus.Load()})
			}
		}()
		defer func() {
			r.mu.Lock()
			if r.pending[action] == pending {
				delete(r.pending, action)
			}
			r.mu.Unlock()
		}()
		if delayMS > 0 {
			wait := r.wait
			if wait == nil {
				wait = waitContext
			}
			if err := wait(ctx, time.Duration(delayMS)*time.Millisecond); err != nil {
				outcome = "canceled-during-delay"
				r.record(map[string]any{"event": "watch_action", "action": action, "result": "canceled"})
				r.emit("watch:canceled:" + action)
				return
			}
		}
		if ctx.Err() != nil || r.customPaused() {
			outcome = "canceled-or-custom-before-request"
			return
		}
		if action == "accept" && r.skipCustomReadyCheck(ctx, client) {
			outcome = "custom-ready-check"
			return
		}
		requestCtx, requestCancel := context.WithTimeout(ctx, 8*time.Second)
		defer requestCancel()
		if focusTrace != nil {
			focusTrace.beforeRequest()
			requestCtx = context.WithValue(requestCtx, acceptFocusStatusKey{}, &focusTrace.httpStatus)
		}
		if action == "reconnect" {
			var phase string
			if client.RequestJSON(requestCtx, http.MethodGet, "/lol-gameflow/v1/gameflow-phase", nil, &phase) != nil || phase != "Reconnect" {
				r.record(map[string]any{"event": "watch_action", "action": action, "result": "canceled", "reason": "reconnect-no-longer-active"})
				r.emit("watch:canceled:" + action)
				return
			}
		}
		requestStarted := time.Now()
		attempted = true
		requestErr := r.requestWatchJSON(requestCtx, client, method, path, body)
		outcome = "success"
		if requestErr != nil {
			outcome = "request-failed"
		}
		if focusTrace != nil {
			focusTrace.afterRequest(requestErr, time.Since(requestStarted))
		}
		if err := requestErr; err != nil {
			if r.customPaused() || ctx.Err() != nil {
				return
			}
			statusCode, errorCode, message := watchFailureDetail(err)
			if action == "accept" && r.readyCheckEnded(client) {
				outcome = "ready-check-ended"
				diagnostic := map[string]any{"event": "watch_action", "action": action, "result": "failed_stale"}
				if statusCode > 0 {
					diagnostic["status_code"] = statusCode
				}
				r.record(diagnostic)
				return
			}
			diagnostic := map[string]any{"event": "watch_action", "action": action, "result": "failed"}
			if statusCode > 0 {
				diagnostic["status_code"] = statusCode
			}
			if errorCode != "" {
				diagnostic["error_code"] = errorCode
			}
			r.record(diagnostic)
			r.emit(fmt.Sprintf("watch:failed:%s:%d:%s:%s", action, statusCode, watchEventField(errorCode), watchEventField(message)))
			return
		}
		r.record(map[string]any{"event": "watch_action", "action": action, "result": "fired"})
		r.emit("watch:fired:" + action)
	}()
	return true
}

// skipCustomReadyCheck performs one preflight read inside the already
// deduplicated accept job. It is not a poll: phase and websocket signals share
// this single job, and a custom ReadyCheck stays suppressed until the phase
// transition resets acceptedForReadyCheck.
func (r *watchRunner) skipCustomReadyCheck(ctx context.Context, client *LCUClient) bool {
	checkCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	var session map[string]any
	if client.RequestJSON(checkCtx, http.MethodGet, "/lol-gameflow/v1/session", nil, &session) != nil {
		return false
	}
	r.observeCustomSession(session, false)
	if !r.customPaused() {
		return false
	}
	r.record(map[string]any{"event": "watch_action", "action": "accept", "result": "skipped_custom"})
	return true
}

// matchmakingPreflight returns ready=false, terminal=false while the lobby is
// still settling. The caller retries that state without issuing matchmaking.
// We deliberately do not consume guessed readiness fields: lcu_lobby_shape
// records the actual keys first, and only the established queueId is enforced.
func (r *watchRunner) matchmakingPreflight(ctx context.Context, client *LCUClient) (ready bool, terminal bool) {
	checkCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	var lobby map[string]any
	if err := client.RequestJSON(checkCtx, http.MethodGet, "/lol-lobby/v2/lobby", nil, &lobby); err != nil {
		return false, false
	}
	r.recordLobbyShape(lobby)
	r.observeCustomSession(lobby, true)
	if r.customPaused() {
		r.record(map[string]any{"event": "watch_action", "action": "auto-matchmaking", "result": "skipped_custom", "reason": "custom"})
		r.emit("watch:skipped:auto-matchmaking:custom")
		return false, true
	}
	local, _ := lobby["localMember"].(map[string]any)
	leader, _ := anyBool(local, "isLeader")
	if !leader {
		r.record(map[string]any{"event": "watch_action", "action": "auto-matchmaking", "result": "skipped_not_leader", "reason": "not-leader"})
		r.emit("watch:skipped:auto-matchmaking:not-leader")
		return false, true
	}
	config, _ := lobby["gameConfig"].(map[string]any)
	custom, _ := anyBool(config, "isCustom")
	if custom {
		r.record(map[string]any{"event": "watch_action", "action": "auto-matchmaking", "result": "skipped_custom", "reason": "custom"})
		r.emit("watch:skipped:auto-matchmaking:custom")
		return false, true
	}
	if anyInt(config, "queueId", "queueID") <= 0 {
		return false, false
	}
	return true, false
}

func sortedWatchMapKeys(object map[string]any) []string {
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func (r *watchRunner) recordLobbyShape(lobby map[string]any) {
	if r == nil {
		return
	}
	local, _ := lobby["localMember"].(map[string]any)
	config, _ := lobby["gameConfig"].(map[string]any)
	topKeys := sortedWatchMapKeys(lobby)
	localKeys := sortedWatchMapKeys(local)
	configKeys := sortedWatchMapKeys(config)
	key := strings.Join(topKeys, ",") + "|" + strings.Join(localKeys, ",") + "|" + strings.Join(configKeys, ",")
	r.mu.Lock()
	if r.lobbyShapeKeys == nil {
		r.lobbyShapeKeys = make(map[string]struct{})
	}
	if _, exists := r.lobbyShapeKeys[key]; exists {
		r.mu.Unlock()
		return
	}
	if len(r.lobbyShapeKeys) >= 16 {
		r.lobbyShapeKeys = make(map[string]struct{})
	}
	r.lobbyShapeKeys[key] = struct{}{}
	r.mu.Unlock()
	r.record(map[string]any{"event": "lcu_lobby_shape", "top_level_keys": topKeys, "local_member_keys": localKeys, "game_config_keys": configKeys})
}

func waitWatchDelay(ctx context.Context, delay time.Duration) bool {
	if delay <= 0 {
		return ctx.Err() == nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func (r *watchRunner) autoMatchBackoff(attempt int) time.Duration {
	if attempt <= 0 {
		return 0
	}
	r.mu.Lock()
	delays := append([]time.Duration(nil), r.autoMatchRetryDelays...)
	r.mu.Unlock()
	if len(delays) > 0 {
		if attempt > len(delays) {
			return delays[len(delays)-1]
		}
		return delays[attempt-1]
	}
	if attempt > len(autoMatchRetryBackoff) {
		return autoMatchRetryBackoff[len(autoMatchRetryBackoff)-1]
	}
	return autoMatchRetryBackoff[attempt-1]
}

func (r *watchRunner) scheduleAutoMatchmaking(client *LCUClient, delayMS int) bool {
	if r == nil || client == nil {
		return false
	}
	ctx, cancel := context.WithCancel(context.Background())
	pending := &watchPendingAction{cancel: cancel}
	r.mu.Lock()
	reason := ""
	switch {
	case r.customSession:
		reason = "custom"
	case r.autoMatchStarted:
		reason = "started"
	case r.autoMatchInFlight:
		reason = "inflight"
	case r.autoMatchExhausted:
		reason = "exhausted"
	}
	if reason != "" {
		r.mu.Unlock()
		cancel()
		r.record(map[string]any{"event": "watch_action", "action": "auto-matchmaking", "result": "skipped_state", "reason": reason})
		if reason == "custom" {
			r.emit("watch:skipped:auto-matchmaking:custom")
		}
		return false
	}
	if r.pending == nil {
		r.pending = make(map[string]*watchPendingAction)
	}
	r.autoMatchInFlight = true
	r.pending["auto-matchmaking"] = pending
	r.mu.Unlock()
	r.emit(fmt.Sprintf("watch:armed:auto-matchmaking:%d", delayMS))
	r.record(map[string]any{"event": "watch_action", "action": "auto-matchmaking", "result": "armed"})
	go func() {
		defer func() {
			r.mu.Lock()
			if r.pending["auto-matchmaking"] == pending {
				delete(r.pending, "auto-matchmaking")
				r.autoMatchInFlight = false
			}
			r.mu.Unlock()
		}()
		canceled := func() {
			r.record(map[string]any{"event": "watch_action", "action": "auto-matchmaking", "result": "canceled"})
			r.emit("watch:canceled:auto-matchmaking")
		}
		if !waitWatchDelay(ctx, time.Duration(delayMS)*time.Millisecond) {
			canceled()
			return
		}
		// A canceled older job must not overwrite a new game's reset state.
		finish := func(exhausted bool) bool {
			r.mu.Lock()
			defer r.mu.Unlock()
			if r.pending["auto-matchmaking"] != pending || ctx.Err() != nil {
				return false
			}
			r.autoMatchStarted, r.autoMatchExhausted = !exhausted, exhausted
			return true
		}
		lastStatusCode := 0
		for attempt := 1; attempt <= autoMatchMaxAttempts; attempt++ {
			if r.customPaused() {
				canceled()
				return
			}
			ready, terminal := r.matchmakingPreflight(ctx, client)
			if terminal {
				return
			}
			if ctx.Err() != nil {
				canceled()
				return
			}
			if ready {
				requestCtx, requestCancel := context.WithTimeout(ctx, 8*time.Second)
				err := r.requestWatchJSON(requestCtx, client, http.MethodPost, "/lol-lobby/v2/lobby/matchmaking/search", nil)
				requestCancel()
				if r.customPaused() {
					return
				}
				if err == nil {
					if !finish(false) {
						canceled()
						return
					}
					r.record(map[string]any{"event": "watch_action", "action": "auto-matchmaking", "result": "fired", "attempts": attempt})
					r.emit("watch:fired:auto-matchmaking")
					return
				}
				statusCode, errorCode, message := watchFailureDetail(err)
				lastStatusCode = statusCode
				if statusCode != http.StatusBadRequest {
					if !finish(true) {
						canceled()
						return
					}
					diagnostic := map[string]any{"event": "watch_action", "action": "auto-matchmaking", "result": "failed", "attempts": attempt}
					if statusCode > 0 {
						diagnostic["status_code"] = statusCode
					}
					if errorCode != "" {
						diagnostic["error_code"] = errorCode
					}
					r.record(diagnostic)
					r.emit(fmt.Sprintf("watch:failed:auto-matchmaking:%d:%s:%s", statusCode, watchEventField(errorCode), watchEventField(message)))
					return
				}
				if attempt < autoMatchMaxAttempts {
					r.record(map[string]any{"event": "watch_action", "action": "auto-matchmaking", "result": "retrying", "attempt": attempt, "status_code": statusCode})
				}
			}
			if attempt == autoMatchMaxAttempts {
				if !finish(true) {
					canceled()
					return
				}
				diagnostic := map[string]any{"event": "watch_action", "action": "auto-matchmaking", "result": "gave_up", "attempts": attempt}
				if lastStatusCode > 0 {
					diagnostic["status_code"] = lastStatusCode
				}
				r.record(diagnostic)
				if lastStatusCode == 0 {
					r.emit("watch:skipped:auto-matchmaking:not-ready")
				} else {
					r.emit(fmt.Sprintf("watch:failed:auto-matchmaking:%d::retry-exhausted", lastStatusCode))
				}
				return
			}
			if !waitWatchDelay(ctx, r.autoMatchBackoff(attempt)) {
				canceled()
				return
			}
		}
	}()
	return true
}

// readyCheckEnded distinguishes a benign transition race from a real failure.
// An unavailable phase read is not treated as success: in that case the user
// still receives the original actionable error.
func (r *watchRunner) readyCheckEnded(client *LCUClient) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var phase string
	if err := client.RequestJSON(ctx, http.MethodGet, "/lol-gameflow/v1/gameflow-phase", nil, &phase); err != nil {
		return false
	}
	return !strings.EqualFold(strings.TrimSpace(phase), "ReadyCheck")
}

func watchFailureDetail(err error) (int, string, string) {
	var httpErr *LCUHTTPError
	if errors.As(err, &httpErr) {
		message := strings.TrimSpace(httpErr.Message)
		if message == "" {
			message = strings.TrimSpace(httpErr.ErrorCode)
		}
		if message == "" {
			message = fmt.Sprintf("客户端返回 HTTP %d", httpErr.StatusCode)
		}
		return httpErr.StatusCode, strings.TrimSpace(httpErr.ErrorCode), message
	}
	return 0, "", "本机客户端请求失败"
}

func watchEventField(value string) string {
	value = strings.NewReplacer("\r", " ", "\n", " ", ":", "；").Replace(strings.TrimSpace(value))
	if len([]rune(value)) > 160 {
		value = string([]rune(value)[:160])
	}
	return value
}

func (r *watchRunner) record(event map[string]any) {
	if r != nil && r.observe != nil {
		r.observe(event)
	}
}

func (r *watchRunner) markRun(action string, window time.Duration) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.customSession {
		return false
	}
	if last, ok := r.lastRun[action]; ok && r.clockNow().Sub(last) < window {
		return false
	}
	r.lastRun[action] = r.clockNow()
	return true
}

func (r *watchRunner) cancelPending(action string) {
	r.mu.Lock()
	pending := r.pending[action]
	delete(r.pending, action)
	if action == "auto-matchmaking" {
		r.autoMatchInFlight = false
	}
	r.mu.Unlock()
	if pending != nil {
		pending.cancel()
	}
}

func (r *watchRunner) emit(event string) {
	if r != nil && r.notify != nil {
		r.notify(event)
	}
}

func (r *watchRunner) handleHonor(client *LCUClient, ballotData json.RawMessage, current Summoner, rule watchHonorRule) {
	r.mu.Lock()
	if r.honorInProgress || r.customSession {
		r.mu.Unlock()
		return
	}
	r.honorInProgress = true
	if pending := r.pending["play-again"]; pending != nil {
		pending.cancel()
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
			// The earlier play-again job was canceled by this honor flow itself, so
			// this continuation is not a duplicate and may bypass markRun's window.
			r.scheduleMarked(client, "play-again", 500, http.MethodPost, "/lol-lobby/v2/play-again", nil)
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
	if r.customPaused() {
		return
	}
	recipient := chooseHonorRecipient(ballotData, party, current.PUUID, rule.Strategy)
	var actionErr error
	if recipient != "" && rule.Strategy != "abstain" {
		actionErr = r.requestWatchJSON(requestCtx, client, http.MethodPost, "/lol-honor/v1/honor", map[string]any{"honorType": "HEART", "recipientPuuid": recipient})
	}
	if actionErr == nil && !r.customPaused() {
		actionErr = r.requestWatchJSON(requestCtx, client, http.MethodPost, "/lol-honor/v1/ballot", nil)
	}
	if r.customPaused() {
		return
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
	if r.customPaused() {
		return
	}
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
	r.recordLobbyShape(lobby)
	r.observeCustomSession(lobby, true)
	if r.customPaused() {
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
		r.scheduleAutoMatchmaking(client, rules.AutoMatchmaking.DelayMS)
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
	if r.customPaused() {
		return
	}
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
		if r.customPaused() {
			return
		}
		path := "/lol-lobby/v2/received-invitations/" + id + "/" + policy
		if err := r.requestWatchJSON(ctx, client, http.MethodPost, path, nil); err != nil {
			if r.customPaused() {
				return
			}
			r.emit("watch:failed:invitations")
		} else {
			r.emit("watch:fired:invitations")
		}
	}
}

func (r *watchRunner) broadcastPosition(client *LCUClient, current Summoner, rule watchBroadcastRule) string {
	return r.broadcastPositionContext(context.Background(), client, current, rule)
}

func (r *watchRunner) broadcastPositionContext(parent context.Context, client *LCUClient, current Summoner, rule watchBroadcastRule) string {
	finish := func(result, reason string) string {
		r.record(map[string]any{"event": "watch_action", "action": "position-broadcast", "result": result, "reason": reason})
		return result
	}
	if r.customPaused() {
		return finish("skipped_custom", "custom-paused")
	}
	r.record(map[string]any{"event": "watch_action", "action": "position-broadcast", "result": "armed"})
	ctx, cancel := context.WithTimeout(parent, 8*time.Second)
	defer cancel()
	var session map[string]any
	if err := client.RequestJSON(ctx, http.MethodGet, "/lol-champ-select/v1/session", nil, &session); err != nil {
		return finish("failed", "session-read-failed")
	}
	queueID := anyInt(session, "queueId", "queueID")
	modeGroup := queueModeGroupFor(queueID, "", 0)
	if modeGroup != "aram" && modeGroup != "hextech-aram" && modeGroup != "hextech-classic" {
		var gameflow lcuGameflowSession
		if err := client.RequestJSON(ctx, http.MethodGet, "/lol-gameflow/v1/session", nil, &gameflow); err != nil {
			return finish("failed", "gameflow-read-failed")
		}
		mode := strings.ToUpper(strings.TrimSpace(gameflow.GameData.Queue.GameMode))
		mapID := gameflow.GameData.Queue.MapID
		if mapID == 0 {
			mapID = gameflow.Map.ID
		}
		modeGroup = queueModeGroupFor(gameflow.GameData.Queue.ID, mode, mapID)
		if !isARAMFamilyGameMode(mode) && modeGroup != "aram" && modeGroup != "hextech-aram" && modeGroup != "hextech-classic" {
			return finish("skipped_mode", "unsupported-mode")
		}
	}
	bench, _ := anyBool(session, "benchEnabled")
	if !bench {
		return finish("skipped_no_bench", "bench-unavailable")
	}
	localCellID := anyInt(session, "localPlayerCellId")
	team := ""
	myTeam := mapSliceFromPayload(session, "myTeam")
	for _, player := range myTeam {
		if anyInt(player, "cellId") == localCellID {
			team = strings.ToUpper(anyString(player, "team", "teamId", "teamID"))
			break
		}
	}
	if team == "" && len(myTeam) > 0 {
		team = strings.ToUpper(anyString(myTeam[0], "team", "teamId", "teamID"))
	}
	label := ""
	switch team {
	case "1", "ONE", "BLUE", "100":
		label = "蓝色方"
	case "2", "TWO", "RED", "200":
		label = "红色方"
	default:
		return finish("skipped_no_team", "team-unavailable")
	}
	var me map[string]any
	var conversations []map[string]any
	if client.RequestJSON(ctx, http.MethodGet, "/lol-chat/v1/me", nil, &me) != nil || client.RequestJSON(ctx, http.MethodGet, "/lol-chat/v1/conversations", nil, &conversations) != nil {
		return finish("failed", "chat-read-failed")
	}
	conversationID := ""
	for _, conversation := range conversations {
		kind := strings.ToLower(anyString(conversation, "type"))
		if strings.Contains(kind, "champion") || strings.Contains(kind, "champselect") {
			conversationID = anyString(conversation, "id")
			break
		}
	}
	if !safeLCUChatIdentifier(conversationID) {
		return finish("failed", "conversation-unavailable")
	}
	messageType := "celebration"
	if rule.Visibility == "team" {
		messageType = "chat"
	}
	body := map[string]any{
		"body": r.positionBroadcastMessage(label, session, rule), "fromId": anyString(me, "id"), "fromPid": "",
		"fromSummonerId": current.SummonerID, "id": "", "isHistorical": false, "timestamp": "", "type": messageType,
	}
	if r.customPaused() {
		return finish("skipped_custom", "custom-paused")
	}
	path := "/lol-chat/v1/conversations/" + url.PathEscape(conversationID) + "/messages"
	if err := r.requestWatchJSON(ctx, client, http.MethodPost, path, body); err != nil {
		if r.customPaused() {
			return finish("skipped_custom", "custom-paused")
		}
		if ctx.Err() == nil {
			r.emit("watch:failed:position-broadcast")
		}
		// Do not retry a POST with an uncertain outcome: it could duplicate chat.
		return finish("write_failed", "message-write-failed")
	} else {
		r.emit("watch:fired:position-broadcast")
		return finish("fired", "sent")
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

// Chat conversation IDs can be XMPP JIDs (room@conference.domain).
// Keep the stricter identifier contract for all other LCU routes.
func safeLCUChatIdentifier(value string) bool {
	if value != strings.TrimSpace(value) || strings.Count(value, "@") > 1 {
		return false
	}
	for _, part := range strings.Split(value, "@") {
		if !safeLCUIdentifier(part) {
			return false
		}
	}
	return len(value) <= 256
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
		http.Error(w, "自动规则不可用", http.StatusServiceUnavailable)
		return
	}
	if r.Method == http.MethodGet {
		runner.champDiagnostic("settings-read", "served", champSelectDecision{}, nil)
		respondJSON(w, struct {
			watchSettings
			CustomPaused     bool                   `json:"customPaused"`
			BroadcastOptions []watchBroadcastOption `json:"broadcastOptions"`
		}{runner.currentWatch(), runner.customPaused(), positionBroadcastOptions()})
		return
	}
	var request watchSettings
	if err := decodeJSONRequest(r, &request, 32<<10); err != nil {
		runner.record(map[string]any{"event": "watch_settings_failed", "stage": "decode", "error_kind": diagnosticErrorKind(err)})
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	request = normalizeWatchSettings(request)
	if err := saveWatchSettings(a.storage, request); err != nil {
		runner.record(map[string]any{"event": "watch_settings_failed", "stage": "persist", "error_kind": diagnosticErrorKind(err)})
		http.Error(w, "自动规则无法保存", http.StatusServiceUnavailable)
		return
	}
	runner.apply(request)
	runner.record(map[string]any{"event": "watch_settings_saved", "master_enabled": request.MasterEnabled, "custom_paused": runner.customPaused(), "rules": request.Rules})
	if client, _, err := a.gameplayClient(); err == nil {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		var phase string
		if err := client.RequestJSON(ctx, http.MethodGet, "/lol-gameflow/v1/gameflow-phase", nil, &phase); err == nil {
			runner.handlePhase(client, phase)
		}
		cancel()
		runner.handleChampSelectAutomation(client)
	}
	for id, group := range request.ChampSelect.Groups {
		runner.record(map[string]any{"event": "champselect_settings_saved", "group_id": id, "enabled": request.ChampSelect.Enabled, "ban_enabled": group.Ban.Enabled, "pick_enabled": group.Pick.Enabled, "config": group})
	}
	respondJSON(w, struct {
		watchSettings
		CustomPaused     bool                   `json:"customPaused"`
		BroadcastOptions []watchBroadcastOption `json:"broadcastOptions"`
	}{request, runner.customPaused(), positionBroadcastOptions()})
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

func (r *watchRunner) customPaused() bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.customSession
}
func (r *watchRunner) setCustomSession(custom bool) {
	r.mu.Lock()
	changed := r.customSession != custom
	r.customSession = custom
	if changed {
		if r.writeCancel != nil {
			r.writeCancel()
			r.writeContext = nil
			r.writeCancel = nil
		}
		for key, pending := range r.pending {
			pending.cancel()
			delete(r.pending, key)
		}
		r.autoMatchInFlight = false
		r.deferredPlayAgain = false
		r.promotedForLobby = false
		r.autoMatchStarted = false
		r.broadcastForSession = false
		r.acceptedForReadyCheck = false
		r.lastRun = make(map[string]time.Time)
		r.resetChampSelectRuntimeLocked()
	}
	r.mu.Unlock()
	if changed {
		state := "resumed"
		if custom {
			state = "paused_custom"
		}
		r.record(map[string]any{"event": "watch_session", "result": state, "master_enabled": r.currentWatch().MasterEnabled, "custom_paused": custom})
		r.emit("watch:session:" + state)
	}
}
func (r *watchRunner) observeCustomSession(value map[string]any, lobby bool) {
	key := "gameData"
	if lobby {
		key = "gameConfig"
	}
	data, _ := value[key].(map[string]any)
	custom, known := anyBool(data, "isCustom", "isCustomGame")
	if !known {
		custom, known = anyBool(value, "isCustom", "isCustomGame")
	}
	queue, _ := data["queue"].(map[string]any)
	queueID := anyInt(data, "queueId", "queueID")
	if queueID == 0 {
		queueID = anyInt(queue, "id")
	}
	// Some custom lobbies report isCustom=false; the known queue identity is
	// authoritative. Do not treat all bot queues as custom.
	if queueID == 3100 || queueID == 3110 || queueID == 3200 {
		custom, known = true, true
	} else if definition, ok := supportedQueueDefinition(queueID); ok && definition.ModeGroup == "custom" {
		custom, known = true, true
	}
	if strings.Contains(strings.ToUpper(anyString(queue, "type")), "CUSTOM") {
		custom, known = true, true
	}
	source := 0
	if lobby {
		source = 1
	}
	observation := fmt.Sprintf("%d:%t:%t", queueID, known, custom)
	r.mu.Lock()
	changed := r.customObservation[source] != observation
	r.customObservation[source] = observation
	r.mu.Unlock()
	if changed {
		r.record(map[string]any{"event": "watch_custom_classification", "lobby": lobby, "queue_id": queueID, "known": known, "custom": custom})
	}
	// Keep custom context through teardown/EndOfGame. Only a real new lobby
	// (or None phase) resumes rules; absent metadata is not evidence of normal play.
	if known && (custom || lobby) {
		r.setCustomSession(custom)
	}
}
func (r *watchRunner) refreshCustomSession(client *LCUClient) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var session map[string]any
	if client.RequestJSON(ctx, http.MethodGet, "/lol-gameflow/v1/session", nil, &session) == nil {
		r.observeCustomSession(session, false)
	}
}

// Admission and custom-session transitions use the same lock. A transition
// cancels every admitted HTTP write, including non-timer honor/chat/invitations.
// An HTTP request already received by LCU cannot be retracted by this process.
func (r *watchRunner) requestWatchJSON(ctx context.Context, client *LCUClient, method, path string, body any) error {
	r.mu.Lock()
	if r.customSession || !r.settings.MasterEnabled {
		custom, enabled := r.customSession, r.settings.MasterEnabled
		r.mu.Unlock()
		r.champDiagnostic("watch-write-gate", "blocked", champSelectDecision{}, map[string]any{"endpoint": diagnosticWatchEndpoint(path), "method": method, "blocked_custom": custom, "master_enabled": enabled})
		return context.Canceled
	}
	if r.writeContext == nil {
		r.writeContext, r.writeCancel = context.WithCancel(context.Background())
	}
	session := r.writeContext
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

func (r *watchRunner) clockNow() time.Time {
	if r.now != nil {
		return r.now()
	}
	return time.Now()
}
func waitContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
