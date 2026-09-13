package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"regexp"
	"strings"
	"sync/atomic"
	"time"
)

var diagnosticTraceSequence atomic.Uint64
var flowClientTracePattern = regexp.MustCompile(`^cg-[0-9]{13}-[0-9]{1,6}$`)

func validFlowClientTrace(value string) bool { return flowClientTracePattern.MatchString(value) }

func newDiagnosticTrace(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, diagnosticTraceSequence.Add(1))
}

func diagnosticErrorKind(err error) string {
	if err == nil {
		return "none"
	}
	if errors.Is(err, context.Canceled) {
		return "canceled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	if errors.Is(err, errResponseLimitExceeded) {
		return "response-too-large"
	}
	if errors.Is(err, errSGPResponseDecode) {
		return "decode"
	}
	if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.ErrShortWrite) {
		return "incomplete-io"
	}
	var syntax *json.SyntaxError
	var shape *json.UnmarshalTypeError
	if errors.As(err, &syntax) || errors.As(err, &shape) {
		return "decode"
	}
	var upstream *sgpHTTPError
	if errors.As(err, &upstream) {
		return fmt.Sprintf("http-%d", upstream.StatusCode)
	}
	status, _, _ := watchFailureDetail(err)
	if status > 0 {
		return fmt.Sprintf("http-%d", status)
	}
	var network net.Error
	if errors.As(err, &network) {
		if network.Timeout() {
			return "timeout"
		}
		return "network"
	}
	return "other"
}

type diagnosticSample struct {
	Payload string
	At      time.Time
	Count   int
}

// No raw error text or session payloads. Repeated idle/gate observations emit a
// 30-second heartbeat with suppressed counts; transitions always emit at once.
func (r *watchRunner) champDiagnostic(stage, reason string, decision champSelectDecision, fields map[string]any) {
	if r == nil {
		return
	}
	event := map[string]any{"event": "champselect_trace", "diagnostic_schema": 3, "stage": stage, "reason": reason}
	for key, value := range fields {
		event[key] = value
	}
	r.mu.Lock()
	event["session_trace"] = r.champDiagnosticSession
	if decision.SessionTrace != "" {
		event["session_trace"] = decision.SessionTrace
	}
	event["phase"] = r.champSelect.phase
	if _, provided := event["timer_phase"]; !provided {
		event["timer_phase"] = r.champSelect.lastSession.Timer.Phase
	}
	event["group_id"] = r.champSelect.groupID
	event["champselect_enabled"] = r.settings.ChampSelect.Enabled
	event["watch_master_enabled"] = r.settings.MasterEnabled
	event["custom_paused"] = r.customSession
	event["session_paused"] = r.champSelect.sessionPaused
	if decision.Action != "" {
		event["action"], event["action_id"], event["champion_id"], event["completed"] = decision.Action, decision.ActionID, decision.ChampionID, decision.Completed
		event["trace_id"] = decision.TraceID
		event["intent"], event["availability_source"], event["requires_hover_confirmation"] = decision.Intent, decision.AvailabilitySource, decision.ForceHover
		if decision.TraceID != "" {
			event["write_step"] = champSelectDecisionStep(decision)
		}
	}
	encoded, _ := json.Marshal(event)
	key := stage + ":" + decision.Action
	if stage == "postflight" {
		key += fmt.Sprint(":", decision.ActionID)
	}
	if r.champDiagnosticSamples == nil {
		r.champDiagnosticSamples = map[string]diagnosticSample{}
	}
	previous := r.champDiagnosticSamples[key]
	if previous.Payload == string(encoded) && time.Since(previous.At) < 30*time.Second {
		previous.Count++
		r.champDiagnosticSamples[key] = previous
		r.mu.Unlock()
		return
	}
	event["previous_repeats"] = previous.Count
	if _, exists := r.champDiagnosticSamples[key]; !exists && len(r.champDiagnosticSamples) >= 256 {
		oldestKey := ""
		var oldest time.Time
		for candidate, sample := range r.champDiagnosticSamples {
			if oldest.IsZero() || sample.At.Before(oldest) {
				oldestKey, oldest = candidate, sample.At
			}
		}
		delete(r.champDiagnosticSamples, oldestKey)
	}
	r.champDiagnosticSamples[key] = diagnosticSample{Payload: string(encoded), At: time.Now()}
	r.mu.Unlock()
	r.record(event)
}

type currentGameTraceKey struct{}

func currentGameTrace(ctx context.Context) string {
	value, _ := ctx.Value(currentGameTraceKey{}).(string)
	return value
}
func (a *app) currentGameDiagnostic(ctx context.Context, stage, reason string, fields map[string]any) {
	event := map[string]any{"event": "current_game_trace", "diagnostic_schema": 4, "trace_id": currentGameTrace(ctx), "stage": stage, "reason": reason, "context_error": diagnosticErrorKind(ctx.Err())}
	for key, value := range fields {
		event[key] = value
	}
	a.recordDiagnostic(event)
}

func diagnosticStageError(err error, fallback string) string {
	if err != nil {
		return diagnosticErrorKind(err)
	}
	return fallback
}

// Diagnostic endpoint names, never URLs containing player identifiers.
func diagnosticWatchEndpoint(path string) string {
	for _, entry := range []struct{ prefix, label string }{
		{"/lol-champ-select/", "champ-select"}, {"/lol-champ-select-legacy/", "champ-select-legacy"},
		{"/lol-lobby/", "lobby"}, {"/lol-matchmaking/", "matchmaking"}, {"/lol-gameflow/", "gameflow"},
		{"/lol-honor", "honor"}, {"/lol-chat/", "chat"}, {"/lol-end-of-game/", "end-of-game"},
	} {
		if strings.HasPrefix(path, entry.prefix) {
			return entry.label
		}
	}
	return "other"
}

// Observe subsequent normal polling; diagnostics never issue an extra game
// write or reinterpret a successful HTTP response as an applied selection.
func (r *watchRunner) recordChampSelectPostflight(session lcuChampSelectSession) {
	r.mu.Lock()
	records := make(map[int64]champSelectSubmitRecord, len(r.champSelect.submitted))
	for id, record := range r.champSelect.submitted {
		records[id] = record
	}
	r.mu.Unlock()
	for id, record := range records {
		if record.At.IsZero() {
			continue
		}
		reason := "action-missing"
		var observed lcuChampSelectAction
		for _, actions := range session.Actions {
			for _, action := range actions {
				if action.ID == id {
					observed = action
					reason = "awaiting-state"
				}
			}
		}
		if reason != "action-missing" {
			if observed.ChampionID == record.ChampionID && (!record.Completed || observed.Completed) {
				reason = "applied"
			} else if time.Since(record.At) >= 2*time.Second {
				reason = "not-applied-after-2s"
			}
		}
		d := record.Decision
		if d.Action == "" {
			d = champSelectDecision{TraceID: record.TraceID, Action: "champselect-" + observed.Type, ActionID: id, ChampionID: record.ChampionID, Completed: record.Completed}
		}
		r.champDiagnostic("postflight", reason, d, map[string]any{"timer_phase": session.Timer.Phase, "observed_champion_id": observed.ChampionID, "observed_completed": observed.Completed, "observed_in_progress": observed.IsInProgress, "business_confirmed": record.Confirmed})
	}
}

func (r *watchRunner) recordFlowExportSnapshot() {
	if r == nil {
		return
	}
	r.mu.Lock()
	fields := map[string]any{"event": "automation_export_snapshot", "diagnostic_schema": 3, "session_trace": r.champDiagnosticSession, "watch_master_enabled": r.settings.MasterEnabled, "champselect_enabled": r.settings.ChampSelect.Enabled, "custom_paused": r.customSession, "session_paused": r.champSelect.sessionPaused, "phase": r.champSelect.phase, "pending_count": len(r.pending), "processing": r.champSelect.processing, "queued": r.champSelect.queued}
	suppressed := map[string]int{}
	for stage, sample := range r.champDiagnosticSamples {
		suppressed[stage] = sample.Count
	}
	fields["suppressed_since_last_emit"] = suppressed
	var source, candidates any
	_ = json.Unmarshal([]byte(r.champSelect.sourceKey), &source)
	_ = json.Unmarshal([]byte(r.champSelect.candidateKey), &candidates)
	fields["last_source"], fields["last_candidates"] = source, candidates
	r.mu.Unlock()
	r.record(fields)
}
