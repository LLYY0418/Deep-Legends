package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const champSelectMaxWriteAttempts = 2 // Initial attempt plus one fresh-state recovery.

func champSelectDecisionStep(d champSelectDecision) string {
	if d.Intent {
		return "intent"
	}
	if d.Action == champSelectActionBan || d.Action == champSelectActionPick {
		if d.Completed {
			return "lock"
		}
		return "hover"
	}
	return "swap"
}

func champSelectDecisionVerb(d champSelectDecision) string {
	verb := champSelectActionVerb(d.Action)
	switch champSelectDecisionStep(d) {
	case "intent":
		return "提前预选"
	case "hover":
		return "亮出" + verb
	case "lock":
		return verb + "并锁定"
	default:
		return verb
	}
}

// Custom draft marks a ban action in-progress during PLANNING as well. It is
// not a permission to ban: only pick intent may be written during that phase.
func champSelectExecutableAction(session lcuChampSelectSession, intent bool) (lcuChampSelectAction, bool) {
	phase := strings.ToUpper(strings.TrimSpace(session.Timer.Phase))
	if !intent {
		if phase != "BAN_PICK" {
			return lcuChampSelectAction{}, false
		}
		return firstLocalChampSelectAction(session)
	}
	if phase != "PLANNING" || session.LocalPlayerCellID == nil || session.BenchEnabled {
		return lcuChampSelectAction{}, false
	}
	for _, actions := range session.Actions {
		for _, action := range actions {
			if action.ActorCellID == *session.LocalPlayerCellID && action.Type == "pick" && !action.Completed {
				return action, true
			}
		}
	}
	return lcuChampSelectAction{}, false
}

// The [-1] sentinel can only propose a positive, fresh-grid candidate for
// an UNLOCKED ban hover during the local active ban turn.
// The candidate must survive fresh preflight and be echoed by the session
// before a lock is permitted. Failed/empty reads stay closed.
func (r *watchRunner) champSelectBanAvailability(session lcuChampSelectSession, api string, raw []int64, grid map[int64]champSelectGridChampion) (map[int64]struct{}, string) {
	ids := champSelectIDSet(raw)
	cell := int64(-1)
	if session.LocalPlayerCellID != nil {
		cell = *session.LocalPlayerCellID
	}
	scope := fmt.Sprintf("%s:%d:%d:%d", api, session.GameID, session.QueueID, cell)
	r.mu.Lock()
	if r.champSelect.banEvidenceScope != scope {
		r.champSelect.banEvidenceScope = scope
		r.champSelect.banEvidence = map[int64]struct{}{}
	}
	if len(ids) > 0 {
		r.champSelect.banEvidence = ids
	}
	evidence := r.champSelect.banEvidence
	r.mu.Unlock()
	source := "client-bannable"
	action, active := champSelectExecutableAction(session, false)
	if len(raw) == 1 && raw[0] == -1 && active && action.Type == "ban" {
		ids = map[int64]struct{}{}
		source = "wildcard-grid-hover"
		if len(evidence) > 0 {
			source = "wildcard-session-evidence-hover"
		}
		for id, champion := range grid {
			if id <= 0 || champion.SelectionStatus.IsBanned || champion.SelectionStatus.PickedByOtherOrBanned || champion.TeammatePicked || champion.LocalIntent {
				continue
			}
			if len(evidence) > 0 {
				if _, ok := evidence[id]; !ok {
					continue
				}
			}
			ids[id] = struct{}{}
		}
	}
	r.champDiagnostic("availability", source, champSelectDecision{}, map[string]any{"timer_phase": session.Timer.Phase, "queue_id": session.QueueID, "raw_count": len(raw), "raw_positive_count": len(champSelectIDSet(raw)), "evidence_count": len(evidence), "effective_count": len(ids), "requires_hover_confirmation": strings.HasPrefix(source, "wildcard-")})
	return ids, source
}

// This is execution reconciliation, separate from read-only diagnostic tracing.
// A transport acknowledgement does not consume the action permanently or emit
// business success. Normal session polling confirms it or unlocks ONE retry.
func (r *watchRunner) reconcileChampSelectSubmissions(session lcuChampSelectSession) {
	r.mu.Lock()
	records := make(map[int64]champSelectSubmitRecord, len(r.champSelect.submitted))
	for id, record := range r.champSelect.submitted {
		records[id] = record
	}
	r.mu.Unlock()
	for id, record := range records {
		d := record.Decision
		if d.Action == "" || record.Confirmed {
			continue
		}
		var observed lcuChampSelectAction
		found := false
		for _, actions := range session.Actions {
			for _, action := range actions {
				if action.ID == id {
					observed, found = action, true
				}
			}
		}
		applied := found && observed.ChampionID == record.ChampionID && (!record.Completed || observed.Completed)
		if !applied && time.Since(record.At) < 2*time.Second {
			continue
		}
		r.mu.Lock()
		current, exists := r.champSelect.submitted[id]
		if !exists || current.TraceID != record.TraceID || d.RuntimeID != r.champSelect.runtimeID {
			r.mu.Unlock()
			continue
		}
		attempts := r.champSelect.attempts[d.Key]
		if applied {
			current.Confirmed = true
			current.ConfirmedAt = time.Now()
			r.champSelect.submitted[id] = current
			if record.Completed && r.champSelect.decision[d.Action].TraceID == d.TraceID {
				delete(r.champSelect.decision, d.Action)
			}
		} else {
			if d.Action == champSelectActionPick {
				if r.champSelect.pickFailures[id] == nil {
					r.champSelect.pickFailures[id] = map[int64]int{}
				}
				r.champSelect.pickFailures[id][d.ChampionID]++
				attempts = r.champSelect.pickFailures[id][d.ChampionID]
			}
			if d.Action == champSelectActionPick && attempts >= champSelectMaxWriteAttempts {
				if r.champSelect.pickFailed[id] == nil {
					r.champSelect.pickFailed[id] = map[int64]bool{}
				}
				r.champSelect.pickFailed[id][d.ChampionID] = true
			}
			delete(r.champSelect.submitted, id)
			if r.champSelect.decision[d.Action].TraceID == d.TraceID {
				delete(r.champSelect.decision, d.Action)
			}
		}
		r.mu.Unlock()
		if applied {
			r.champDiagnostic("confirmation", "applied", d, map[string]any{"timer_phase": session.Timer.Phase, "attempt": attempts, "observed_champion_id": observed.ChampionID, "observed_completed": observed.Completed})
			verb := champSelectDecisionVerb(d)
			r.champSelectDecisionLog(d, "ok", fmt.Sprintf("已确认%s英雄 %d", verb, d.ChampionID), d.ChampionID)
			r.record(map[string]any{"event": "watch_action", "action": d.Action, "result": "fired", "trace_id": d.TraceID, "confirmed": true, "intent": d.Intent, "completed": d.Completed, "champion_id": d.ChampionID, "write_step": champSelectDecisionStep(d)})
			r.champSelectDecisionEmit(d, "watch:fired:"+d.Action)
		} else {
			_, executable := champSelectExecutableAction(session, d.Intent)
			reason := "retry-after-fresh-state"
			if !found || observed.Completed || !executable {
				reason = "action-or-phase-ended"
			} else if attempts >= champSelectMaxWriteAttempts {
				reason = "attempt-limit"
			}
			r.champDiagnostic("confirmation", reason, d, map[string]any{"timer_phase": session.Timer.Phase, "attempt": attempts, "observed_champion_id": observed.ChampionID, "observed_completed": observed.Completed})
			detail := map[string]string{"retry-after-fresh-state": "重新核验本回合后最多再尝试一次", "action-or-phase-ended": "回合已结束，不再补发", "attempt-limit": "已停止尝试，请手动操作"}[reason]
			r.mu.Lock()
			advancing := d.Action == champSelectActionPick && reason == "attempt-limit" && len(r.champSelect.pickAttempts[d.ActionID]) < 3
			r.mu.Unlock()
			if advancing {
				detail = ""
			}
			r.champSelectDecisionLog(d, "warn", fmt.Sprintf("英雄 %d 的请求未在客户端生效（第 %d/%d 次）%s", d.ChampionID, attempts, champSelectMaxWriteAttempts, map[bool]string{true: "：" + detail, false: ""}[detail != ""]), d.ChampionID)
			r.champSelectDecisionEmit(d, "watch:canceled:"+d.Action)
		}
	}
}

func (r *watchRunner) champSelectFreshCandidate(ctx context.Context, client *LCUClient, session lcuChampSelectSession, d champSelectDecision, config champSelectSideConfig) string {
	var rows []champSelectGridChampion
	if err := client.RequestJSON(ctx, http.MethodGet, champSelectAPI+"/all-grid-champions", nil, &rows); err != nil {
		return "grid-read-" + diagnosticErrorKind(err)
	}
	grid := map[int64]champSelectGridChampion{}
	for _, row := range rows {
		if id := row.resolvedID(); id > 0 {
			grid[id] = row
		}
	}
	mergeChampSelectTeamState(grid, session)
	side := strings.TrimPrefix(d.Action, "champselect-")
	endpoint := "/pickable-champion-ids"
	if side == "ban" {
		endpoint = "/bannable-champion-ids"
	}
	var raw []int64
	if side == "pick" {
		var err error
		raw, _, err = r.champSelectPickAvailability(ctx, client, session, d.SessionAPI)
		if err != nil {
			return "subset-or-availability-unavailable"
		}
		if session.AllowSubsetChampionPicks != (d.AvailabilitySource == "subset") {
			return "availability-mode-changed"
		}
	} else if err := client.RequestJSON(ctx, http.MethodGet, d.SessionAPI+endpoint, nil, &raw); err != nil {
		return "availability-read-" + diagnosticErrorKind(err)
	}
	available := champSelectIDSet(raw)
	if side == "ban" {
		var source string
		available, source = r.champSelectBanAvailability(session, d.SessionAPI, raw, grid)
		if strings.HasPrefix(source, "wildcard-") && d.Completed {
			r.mu.Lock()
			last := r.champSelect.submitted[d.ActionID]
			r.mu.Unlock()
			if !last.Confirmed || last.ChampionID != d.ChampionID {
				return "compatibility-hover-not-confirmed"
			}
		}
	}
	r.champDiagnostic("preflight-candidate", "fresh-snapshot", d, map[string]any{"timer_phase": session.Timer.Phase, "raw_count": len(raw), "effective_count": len(available), "grid_count": len(grid), "avoid_teammate_intent": config.AvoidTeammateIntent})
	r.mu.Lock()
	arena := r.champSelect.groupID == "arena"
	r.mu.Unlock()
	pool := []int64{d.ChampionID}
	if side == "pick" {
		available = r.champSelectRecoverableCandidates(d.ActionID, available)
		if session.AllowSubsetChampionPicks {
			r.mu.Lock()
			pool = append([]int64{}, config.Champions[r.champSelect.position]...)
			r.mu.Unlock()
			testPool, test := champSelectSubsetTestPool(pool, raw, champSelectSubsetTestFirstCard)
			if test != d.TestFirstCard {
				return "subset-test-mode-changed"
			}
			if test {
				pool = testPool
			}
		}
	}
	candidate, reasons := chooseChampSelectCandidate(side, pool, available, grid, config.AvoidTeammateIntent, arena)
	if candidate != d.ChampionID {
		if reason := reasons[d.ChampionID]; reason != "" {
			return reason
		}
		return "candidate-unavailable"
	}
	return ""
}

// R201 临时测试，真机验证后删除（另开工单）。
const champSelectSubsetTestFirstCard = true

// Test fallback depends on the offered/pool intersection, not whether a pool
// hero survives teammate/grid/retry gates. Never override a blocked pool hero.
func champSelectSubsetTestPool(pool, offered []int64, enabled bool) ([]int64, bool) {
	if !enabled {
		return pool, false
	}
	cards := champSelectIDSet(offered)
	for _, id := range pool {
		if _, ok := cards[id]; ok {
			return pool, false
		}
	}
	return offered, true
}
func (r *watchRunner) champSelectDecisionLog(d champSelectDecision, kind, message string, id int64) {
	if !d.TestFirstCard {
		r.champSelectChampionLog(kind, message, id)
	}
}
func (r *watchRunner) champSelectDecisionEmit(d champSelectDecision, event string) {
	if !d.TestFirstCard {
		r.emit(event)
	}
}
