package main

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const reconnectSettlementDelayMS = 5000
const reconnectEventDataURL = "https://127.0.0.1:2999/liveclientdata/eventdata"

type watchReconnectSignal struct {
	ExitSignal                                     string
	ExitCode                                       *int64
	GameEndSeen, CrashHandlerSeen, ClientLogParsed bool
	LastEventName                                  string
	LastEventTime                                  float64
}

type watchReconnectKey struct {
	scope  string
	gameID int64
}

func reconnectGameKey(client *LCUClient, id int64) watchReconnectKey {
	// A new LCU connection must not grant another attempt in the same game.
	// Read cached routing only: this helper is also called under runner.mu.
	if client == nil {
		return watchReconnectKey{gameID: id}
	}
	client.mu.RLock()
	scope := strings.ToUpper(client.region) + "|" + strings.ToUpper(client.rsoPlatform)
	client.mu.RUnlock()
	return watchReconnectKey{scope: scope, gameID: id}
}

// Unknown routing while a client is rebinding cannot grant an extra attempt.
// Known, different servers may legitimately reuse a numeric game ID.
func reconnectGameRecorded(records map[watchReconnectKey]bool, key watchReconnectKey) bool {
	if records[key] {
		return true
	}
	for seen, recorded := range records {
		if recorded && seen.gameID == key.gameID && (key.scope == "|" || seen.scope == "|") {
			return true
		}
	}
	return false
}

type watchReconnectAttempt struct {
	client           *LCUClient
	pending          *watchPendingAction
	started          time.Time
	delay, effective int
	gameID           int64
	signal           watchReconnectSignal
	reason           string
	finished         bool
}

type watchReconnectMonitor struct {
	mu                sync.Mutex
	sampleMu          sync.Mutex
	client            *LCUClient
	cancel            context.CancelFunc
	phase             string
	gameID            int64
	signal            watchReconnectSignal
	tail              reconnectLogTail
	nextEvent         int64
	handlerSampled    bool
	handlerPresent    bool
	handlerAppearedAt time.Time
}

func reconnectSettlementPhase(phase string) bool {
	return isEndOfGamePhase(phase) || phase == "None" || phase == "Lobby"
}
func reconnectSignalOutcome(s watchReconnectSignal) string {
	if s.GameEndSeen {
		return "canceled-game-end"
	}
	if s.ExitSignal == "normal" {
		return "canceled-voluntary"
	}
	return ""
}
func mergeReconnectSignal(old, next watchReconnectSignal) watchReconnectSignal {
	old.GameEndSeen = old.GameEndSeen || next.GameEndSeen
	old.CrashHandlerSeen = old.CrashHandlerSeen || next.CrashHandlerSeen
	old.ClientLogParsed = old.ClientLogParsed || next.ClientLogParsed
	if next.LastEventName != "" {
		old.LastEventName, old.LastEventTime = next.LastEventName, next.LastEventTime
	}
	if next.ExitSignal == "normal" || old.ExitSignal != "normal" && next.ExitSignal == "crash" {
		old.ExitSignal = next.ExitSignal
		old.ExitCode = nil
		if next.ExitCode != nil {
			code := *next.ExitCode
			old.ExitCode = &code
		}
	}
	if old.ExitSignal != "normal" && old.CrashHandlerSeen {
		old.ExitSignal = "crash"
	}
	if old.ExitSignal == "" {
		old.ExitSignal = "unknown"
	}
	return old
}

// Runs before the legacy phase scheduler. Cancelling a timer is also a durable
// observation of settlement; a later stale Reconnect read cannot erase it.
func (r *watchRunner) observeReconnectPhase(client *LCUClient, phase string, settings watchSettings) {
	active := settings.MasterEnabled && settings.Rules.AutoReconnect.Enabled
	live := phase == "GameStart" || phase == "InProgress" || phase == "Reconnect"
	r.mu.Lock()
	if attempt := r.reconnectAttempt; attempt != nil && (attempt.client != client || phase != "Reconnect") {
		if reconnectSettlementPhase(phase) {
			attempt.reason = "canceled-settle"
			r.rememberReconnectGameLocked(reconnectGameKey(attempt.client, attempt.gameID), true)
		} else if attempt.reason == "" {
			attempt.reason = "canceled-phase-changed"
		}
		attempt.pending.cancel()
		r.reconnectAttempt = nil
	}
	monitor := r.reconnectMonitor
	if monitor != nil && monitor.client == client && reconnectSettlementPhase(phase) {
		monitor.mu.Lock()
		id := monitor.gameID
		monitor.mu.Unlock()
		r.rememberReconnectGameLocked(reconnectGameKey(client, id), true)
	}
	if monitor != nil && (!active || !live || monitor.client != client) {
		monitor.cancel()
		r.reconnectMonitor = nil
		monitor = nil
	}
	// Tests can inject only the same privacy-reduced signal value, without IO.
	if active && live && monitor == nil && r.reconnectSignals == nil && !r.customSession {
		parent, stop := client.sessionRequestContext(context.Background())
		ctx, cancel := context.WithCancel(parent)
		monitor = &watchReconnectMonitor{client: client, cancel: cancel, phase: phase, nextEvent: 0}
		r.reconnectMonitor = monitor
		go func() { defer recoverPanic("watch.reconnect.monitor"); defer stop(); monitor.run(ctx) }()
	}
	if monitor != nil {
		monitor.mu.Lock()
		monitor.phase = phase
		monitor.mu.Unlock()
	}
	r.mu.Unlock()
}

// Caller holds r.mu. Retain only a bounded set of game-scoped decisions.
func (r *watchRunner) rememberReconnectGameLocked(key watchReconnectKey, settled bool) {
	if key.gameID <= 0 {
		return
	}
	if r.reconnectFired == nil {
		r.reconnectFired = map[watchReconnectKey]bool{}
	}
	if r.reconnectSettled == nil {
		r.reconnectSettled = map[watchReconnectKey]bool{}
	}
	if !r.reconnectFired[key] && !r.reconnectSettled[key] {
		r.reconnectOrder = append(r.reconnectOrder, key)
	}
	if settled {
		r.reconnectSettled[key] = true
	} else {
		r.reconnectFired[key] = true
	}
	if len(r.reconnectOrder) > 16 {
		old := r.reconnectOrder[0]
		r.reconnectOrder = r.reconnectOrder[1:]
		delete(r.reconnectFired, old)
		delete(r.reconnectSettled, old)
	}
}

func reconnectSessionID(ctx context.Context, client *LCUClient) int64 {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	var session lcuGameflowSession
	if client.RequestJSON(ctx, http.MethodGet, "/lol-gameflow/v1/session", nil, &session) != nil {
		return 0
	}
	return session.GameData.GameID
}
func (r *watchRunner) readReconnectSignal(ctx context.Context, client *LCUClient) watchReconnectSignal {
	if ctx.Err() != nil {
		return watchReconnectSignal{ExitSignal: "unknown"}
	}
	if r.reconnectSignals != nil {
		return r.reconnectSignals(ctx, client)
	}
	r.mu.Lock()
	monitor := r.reconnectMonitor
	gameID := int64(0)
	if r.reconnectAttempt != nil && r.reconnectAttempt.client == client {
		gameID = r.reconnectAttempt.gameID
	}
	r.mu.Unlock()
	if monitor == nil || monitor.client != client {
		return watchReconnectSignal{ExitSignal: "unknown"}
	}
	monitor.sample(ctx)
	return monitor.snapshotForGame(gameID, time.Now())
}

func (m *watchReconnectMonitor) snapshotForGame(id int64, now time.Time) watchReconnectSignal {
	m.mu.Lock()
	defer m.mu.Unlock()
	if id <= 0 || m.gameID != id {
		return watchReconnectSignal{ExitSignal: "unknown"}
	}
	recentHandler := !m.handlerAppearedAt.IsZero() && now.Sub(m.handlerAppearedAt) >= 0 && now.Sub(m.handlerAppearedAt) <= 30*time.Second
	return mergeReconnectSignal(m.signal, watchReconnectSignal{CrashHandlerSeen: recentHandler})
}

func (m *watchReconnectMonitor) observeCrashHandler(present bool, now time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	// A handler already running at the first sample is not evidence about this
	// exit. Only a new appearance near this reconnect window is classified.
	if m.handlerSampled && present && !m.handlerPresent {
		m.handlerAppearedAt = now
	}
	m.handlerSampled, m.handlerPresent = true, present
}

func (r *watchRunner) scheduleReconnect(client *LCUClient, delay int) {
	effective := max(delay, reconnectSettlementDelayMS)
	r.mu.Lock()
	if r.customSession || !r.settings.MasterEnabled || !r.settings.Rules.AutoReconnect.Enabled {
		r.mu.Unlock()
		return
	}
	if previous := r.reconnectAttempt; previous != nil && previous.client == client {
		r.mu.Unlock()
		return
	}
	parent, stop := client.sessionRequestContext(context.Background())
	ctx, cancel := context.WithCancel(parent)
	pending := &watchPendingAction{cancel: cancel, fireAt: r.clockNow().Add(time.Duration(effective) * time.Millisecond)}
	attempt := &watchReconnectAttempt{client: client, pending: pending, started: r.clockNow(), delay: delay, effective: effective}
	if r.reconnectFired == nil {
		r.reconnectFired = map[watchReconnectKey]bool{}
	}
	r.pending["reconnect"] = pending
	r.reconnectAttempt = attempt
	r.mu.Unlock()
	r.emit("watch:armed:reconnect:" + strconv.Itoa(effective))
	r.record(map[string]any{"event": "watch_action", "action": "reconnect", "result": "armed"})
	go func() {
		defer recoverPanic("watch.reconnect.timer")
		defer stop()
		defer cancel()
		outcome := "canceled-phase-changed"
		var decidedAt time.Time
		posted := false
		httpStatus, confirmedBy := 0, "none"
		defer func() {
			r.mu.Lock()
			if attempt.reason != "" && !posted {
				outcome = attempt.reason
			}
			attempt.finished = true
			if outcome == "canceled-settle" {
				r.rememberReconnectGameLocked(reconnectGameKey(client, attempt.gameID), true)
			}
			signal := attempt.signal
			if decidedAt.IsZero() {
				decidedAt = r.clockNow()
			}
			elapsed := max(int64(0), decidedAt.Sub(attempt.started).Milliseconds())
			if r.pending["reconnect"] == pending {
				delete(r.pending, "reconnect")
			}
			r.mu.Unlock()
			diagnostic := map[string]any{"event": "watch_reconnect_decision", "reconnect_ms": elapsed, "outcome": outcome, "exit_signal": signal.ExitSignal, "game_end_seen": signal.GameEndSeen, "crash_handler_seen": signal.CrashHandlerSeen, "client_log_parsed": signal.ClientLogParsed, "delay_ms_setting": attempt.delay, "effective_delay_ms": attempt.effective, "http_status": httpStatus, "confirmed_by": confirmedBy}
			if signal.ExitSignal == "" {
				diagnostic["exit_signal"] = "unknown"
			}
			if signal.ExitCode != nil {
				diagnostic["exit_code"] = *signal.ExitCode
			}
			r.record(diagnostic)
			if !posted {
				r.record(map[string]any{"event": "watch_action", "action": "reconnect", "result": "canceled", "reason": "reconnect-no-longer-active"})
				r.emit("watch:canceled:reconnect")
			}
		}()
		// Start the continuous phase timer immediately; discovery never delays its
		// start or blocks the websocket callback. Initial ID is captured only once.
		initialized := make(chan struct{})
		go func() {
			defer recoverPanic("watch.reconnect.initial")
			defer close(initialized)
			id := reconnectSessionID(ctx, client)
			r.mu.Lock()
			if attempt.finished {
				r.mu.Unlock()
				return
			}
			attempt.gameID = id
			r.mu.Unlock()
			signal := r.readReconnectSignal(ctx, client)
			r.mu.Lock()
			if attempt.finished {
				r.mu.Unlock()
				return
			}
			attempt.signal = mergeReconnectSignal(attempt.signal, signal)
			reason := reconnectSignalOutcome(attempt.signal)
			if id > 0 && reconnectGameRecorded(r.reconnectFired, reconnectGameKey(client, id)) {
				reason = "canceled-once-per-game"
			}
			if id > 0 && reconnectGameRecorded(r.reconnectSettled, reconnectGameKey(client, id)) {
				reason = "canceled-settle"
			}
			if attempt.reason == "" && reason != "" {
				attempt.reason = reason
				cancel()
			}
			r.mu.Unlock()
		}()
		wait := r.wait
		if wait == nil {
			wait = waitContext
		}
		if wait(ctx, time.Duration(effective)*time.Millisecond) != nil {
			return
		}
		select {
		case <-initialized:
		case <-ctx.Done():
			return
		}
		if ctx.Err() != nil {
			return
		}
		signal := r.readReconnectSignal(ctx, client)
		r.mu.Lock()
		attempt.signal = mergeReconnectSignal(attempt.signal, signal)
		reason := reconnectSignalOutcome(attempt.signal)
		id := attempt.gameID
		r.mu.Unlock()
		if reason != "" {
			outcome = reason
			return
		}
		requestCtx, requestCancel := context.WithTimeout(ctx, 8*time.Second)
		defer requestCancel()
		var phase string
		if client.RequestJSON(requestCtx, http.MethodGet, "/lol-gameflow/v1/gameflow-phase", nil, &phase) != nil {
			return
		}
		if phase != "Reconnect" {
			if reconnectSettlementPhase(phase) {
				outcome = "canceled-settle"
			}
			return
		}
		if id <= 0 || reconnectSessionID(requestCtx, client) != id {
			return
		}
		r.mu.Lock()
		key := reconnectGameKey(client, id)
		allowed := ctx.Err() == nil && attempt.reason == "" && r.reconnectAttempt == attempt && !r.customSession && r.settings.MasterEnabled && r.settings.Rules.AutoReconnect.Enabled
		once := reconnectGameRecorded(r.reconnectFired, key)
		if allowed && !once {
			r.rememberReconnectGameLocked(key, false)
		}
		r.mu.Unlock()
		if once {
			outcome = "canceled-once-per-game"
			return
		}
		if !allowed {
			return
		}
		// One automatic attempt, including an uncertain HTTP result. Manual client
		// reconnect is independent and is never intercepted by this runner.
		posted = true
		decidedAt = r.clockNow()
		// The timer's cancellation ends admission, not an already sent POST.
		// Retain the LCU session lifetime and the existing write gate.
		postCtx, postCancel := context.WithTimeout(parent, 8*time.Second)
		var status atomic.Int64
		err := r.requestWatchJSON(context.WithValue(postCtx, lcuResponseStatusKey{}, &status), client, http.MethodPost, "/lol-gameflow/v1/reconnect", nil)
		postCancel()
		httpStatus = int(status.Load())
		result := "failed"
		if err == nil && httpStatus >= 200 && httpStatus < 300 {
			result, confirmedBy = "fired", "http"
		} else if confirmReconnectPhase(parent, client, id) {
			result, confirmedBy = "fired", "phase"
		}
		outcome = result
		r.record(map[string]any{"event": "watch_action", "action": "reconnect", "result": result})
		r.emit("watch:" + result + ":reconnect")
	}()
}

// A response can be lost after LCU has accepted the reconnect. Re-read both
// phase and identity within a separate, session-bound confirmation window.
func confirmReconnectPhase(parent context.Context, client *LCUClient, id int64) bool {
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	for ctx.Err() == nil {
		var phase string
		if client.RequestJSON(ctx, http.MethodGet, "/lol-gameflow/v1/gameflow-phase", nil, &phase) == nil &&
			(phase == "GameStart" || phase == "InProgress") && reconnectSessionID(ctx, client) == id {
			return true
		}
		if waitContext(ctx, 100*time.Millisecond) != nil {
			break
		}
	}
	return false
}

func (m *watchReconnectMonitor) run(ctx context.Context) {
	m.sample(ctx)
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.sample(ctx)
		}
	}
}
func (m *watchReconnectMonitor) sample(ctx context.Context) {
	m.sampleMu.Lock()
	defer m.sampleMu.Unlock()
	if ctx.Err() != nil {
		return
	}
	id := reconnectSessionID(ctx, m.client)
	if id <= 0 {
		return
	}
	m.mu.Lock()
	if id > 0 && id != m.gameID {
		m.gameID = id
		m.signal = watchReconnectSignal{}
		m.tail = reconnectLogTail{}
		m.nextEvent = 0
		m.handlerSampled, m.handlerPresent, m.handlerAppearedAt = false, false, time.Time{}
	}
	phase := m.phase
	nextEvent := m.nextEvent
	m.mu.Unlock()
	signal := m.tail.read()
	m.observeCrashHandler(nativeLeagueCrashHandlerSeen(), time.Now())
	if phase == "InProgress" || phase == "Reconnect" {
		name, eventTime, next, ended := readReconnectEvents(ctx, nextEvent, liveClientDataHTTPClient)
		signal.LastEventName, signal.LastEventTime, signal.GameEndSeen = name, eventTime, ended
		m.mu.Lock()
		m.nextEvent = max(m.nextEvent, next)
		m.mu.Unlock()
	}
	m.mu.Lock()
	m.signal = mergeReconnectSignal(m.signal, signal)
	m.mu.Unlock()
}

func readReconnectEvents(ctx context.Context, next int64, client *http.Client) (string, float64, int64, bool) {
	ctx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
	defer cancel()
	endpoint := reconnectEventDataURL
	if next > 0 {
		endpoint += "?eventID=" + strconv.FormatInt(next, 10)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", 0, next, false
	}
	response, err := client.Do(request)
	if err != nil {
		return "", 0, next, false
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", 0, next, false
	}
	// Decode only public event metadata; player fields are ignored and never kept.
	var payload struct {
		Events []struct {
			EventID   int64
			EventName string
			EventTime float64
		}
	}
	if json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&payload) != nil {
		return "", 0, next, false
	}
	name, eventTime, ended := "", float64(0), false
	for _, event := range payload.Events {
		if event.EventID < next || event.EventID < 0 || !reconnectEventNamePattern.MatchString(event.EventName) || event.EventTime < 0 || math.IsNaN(event.EventTime) || math.IsInf(event.EventTime, 0) {
			continue
		}
		if event.EventID >= next {
			name, eventTime = event.EventName, event.EventTime
			next = event.EventID + 1
		}
		ended = ended || event.EventName == "GameEnd"
	}
	return name, eventTime, next, ended
}

var reconnectEventNamePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,47}$`)
var reconnectExitCodePattern = regexp.MustCompile(`(?i)(?:exit(?:ed)?(?:\s+with)?[ _-]*code|exitcode)\s*[:=]?\s*(-?(?:0x[0-9a-f]+|[0-9]+))(?:\s|[,;.)]|$)`)

func parseReconnectExitSignal(text string) watchReconnectSignal {
	signal := watchReconnectSignal{ExitSignal: "unknown"}
	for _, line := range strings.Split(text, "\n") {
		lower := strings.ToLower(line)
		if !strings.Contains(lower, "gameflow") && !strings.Contains(lower, "game process") && !strings.Contains(lower, "league of legends") {
			continue
		}
		if match := reconnectExitCodePattern.FindStringSubmatch(line); len(match) == 2 {
			code, err := strconv.ParseInt(match[1], 0, 64)
			if err != nil {
				continue
			}
			signal.ExitCode = &code
			signal.ClientLogParsed = true
			signal.ExitSignal = "crash"
			if code == 0 {
				signal.ExitSignal = "normal"
			}
		} else if strings.Contains(lower, "user closed") || strings.Contains(lower, "user quit") || strings.Contains(lower, "normal exit") || strings.Contains(lower, "exited normally") {
			signal.ExitSignal = "normal"
			signal.ClientLogParsed = true
			signal.ExitCode = nil
		}
	}
	return signal
}

type reconnectLogTail struct {
	initialized bool
	roots       []string
	file        string
	offset      int64
}

func (tail *reconnectLogTail) read() watchReconnectSignal {
	if len(tail.roots) == 0 {
		query, _ := nativeLeagueProcessCommands()
		for _, lock := range targetedLockfileCandidates(query) {
			tail.roots = append(tail.roots, filepath.Join(filepath.Dir(lock), "Logs"))
		}
	}
	newest := ""
	var newestAt time.Time
	for _, root := range tail.roots {
		for _, dir := range []string{root, filepath.Join(root, "LeagueClient Logs")} {
			handle, err := os.Open(dir)
			if err != nil {
				continue
			}
			entries, _ := handle.ReadDir(512)
			handle.Close()
			for _, entry := range entries {
				name := strings.ToLower(entry.Name())
				if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || !strings.Contains(name, "leagueclient") || strings.Contains(name, "ux") || !strings.HasSuffix(name, ".log") {
					continue
				}
				info, err := entry.Info()
				if err == nil && info.ModTime().After(newestAt) {
					newestAt = info.ModTime()
					newest = filepath.Join(dir, entry.Name())
				}
			}
		}
	}
	if newest == "" {
		return watchReconnectSignal{ExitSignal: "unknown"}
	}
	file, err := os.Open(newest)
	if err != nil {
		return watchReconnectSignal{ExitSignal: "unknown"}
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return watchReconnectSignal{ExitSignal: "unknown"}
	}
	if !tail.initialized {
		tail.initialized = true
		tail.file = newest
		tail.offset = info.Size()
		return watchReconnectSignal{ExitSignal: "unknown"}
	}
	if tail.file != newest || tail.offset > info.Size() {
		tail.file = newest
		tail.offset = info.Size()
		return watchReconnectSignal{ExitSignal: "unknown"}
	}
	start := max(tail.offset, info.Size()-(64<<10))
	raw := make([]byte, info.Size()-start)
	n, _ := file.ReadAt(raw, start)
	raw = raw[:n]
	complete := strings.LastIndexByte(string(raw), '\n')
	if complete < 0 {
		return watchReconnectSignal{ExitSignal: "unknown"}
	}
	text := string(raw[:complete+1])
	if start > tail.offset {
		if first := strings.IndexByte(text, '\n'); first >= 0 {
			text = text[first+1:]
		}
	}
	tail.offset = start + int64(complete+1)
	return parseReconnectExitSignal(text)
}
