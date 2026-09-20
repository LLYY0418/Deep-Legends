package main

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"
)

type champSelectBenchSubmission struct {
	Decision  champSelectDecision
	Observed  bool
	SettledAt time.Time
}

func champSelectLockDelay(config champSelectSideConfig) int {
	if config.LockDelayMS == nil {
		return 10000 // Also migrates saved configurations from before the separate lock delay.
	}
	return clampInt(*config.LockDelayMS, 0, 10000, 10000)
}

func champSelectHoverLockDelay(config champSelectSideConfig, hover champSelectSubmitRecord, now time.Time) int {
	delay := champSelectLockDelay(config)
	// A planning intent is not the player's active pick turn. Give that turn
	// its own window; repeated evaluations keep the already armed decision.
	if hover.Decision.Intent || hover.ConfirmedAt.IsZero() {
		return delay
	}
	return max(0, delay-int(now.Sub(hover.ConfirmedAt)/time.Millisecond))
}

// Caller holds r.mu. Cancellation and the latch happen together so a queued
// write cannot run between detecting the change and yielding to the player.
func (r *watchRunner) yieldChampSelectLocked(side string, championID int64) bool {
	if r.champSelect.takeover[side] {
		return false
	}
	sides := []string{side}
	if side != "ban" {
		sides = []string{"pick", "bench", "trade"}
	}
	for _, key := range sides {
		r.champSelect.takeover[key] = true
		action := "champselect-" + key
		if pending := r.pending[action]; pending != nil {
			pending.cancel()
			delete(r.pending, action)
		}
		delete(r.champSelect.decision, action)
	}
	if side != "ban" {
		side = "pick"
	}
	r.champSelect.takeoverChampion[side] = championID
	return true
}

func (r *watchRunner) reportChampSelectTakeover(side string, championID int64) {
	r.champSelectChampionLog("warn", "玩家主动切换，本轮已停止自动"+map[string]string{"ban": "禁用", "pick": "选用", "bench": "换人"}[side], championID)
	r.emit("watch:canceled:champselect-" + side)
}

func (r *watchRunner) yieldChampSelect(side string, championID int64, runtimeID string) {
	r.mu.Lock()
	changed := (runtimeID == "" || runtimeID == r.champSelect.runtimeID) && r.yieldChampSelectLocked(side, championID)
	r.mu.Unlock()
	if changed {
		r.reportChampSelectTakeover(side, championID)
	}
}

// Run before grid/availability reads and before submission reconciliation:
// those reads can fail, and reconciliation may retire an unacknowledged write.
// Neither should erase evidence of the player's intervening choice.
func (r *watchRunner) observeChampSelectManualActions(session lcuChampSelectSession) {
	if session.LocalPlayerCellID == nil {
		return
	}
	for _, actions := range session.Actions {
		for _, action := range actions {
			if action.ActorCellID != *session.LocalPlayerCellID {
				continue
			}
			side := strings.ToLower(action.Type)
			if side != "ban" && side != "pick" {
				continue
			}
			r.mu.Lock()
			last, submitted := r.champSelect.submitted[action.ID]
			pending := r.champSelect.decision["champselect-"+side]
			ownEcho := r.champSelect.inFlight[action.ID] && pending.ActionID == action.ID && pending.ChampionID == action.ChampionID
			tracking := submitted || pending.Action != "" && pending.ActionID == action.ID
			changed := tracking && !ownEcho && champSelectManualActionChanged(session, action, last, submitted)
			championID := last.ChampionID
			if championID == 0 {
				championID = pending.ChampionID
			}
			cleared := action.ChampionID == 0 && submitted && last.Confirmed && !last.Completed && last.ChampionID > 0 && last.Decision.ForceHover && !last.HoverCleared
			if cleared {
				last.HoverCleared = true
				r.champSelect.submitted[action.ID] = last
			}
			changed = changed && r.yieldChampSelectLocked(side, championID)
			r.mu.Unlock()
			if cleared {
				r.champDiagnostic("hover", "hover-cleared-by-client", last.Decision, map[string]any{"side": side, "champion_id": last.ChampionID, "since_confirmed_ms": max(0, time.Since(last.ConfirmedAt).Milliseconds())})
			}
			if changed {
				r.reportChampSelectTakeover(side, championID)
			}
		}
	}
}

func champSelectManualActionChanged(session lcuChampSelectSession, action lcuChampSelectAction, last champSelectSubmitRecord, submitted bool) bool {
	if action.ChampionID != 0 {
		return !submitted || action.ChampionID != last.ChampionID
	}
	if !submitted || !last.Confirmed || last.Completed || last.ChampionID <= 0 {
		return false
	}
	// Evaluation sets ForceHover only for wildcard bans requiring session
	// confirmation. The submitted decision retains that evidence for polling
	// and delayed-write preflight; a queue or mode alone is not sufficient.
	if last.Decision.ForceHover {
		return false
	}
	// An ordinary confirmed hover cleared by the player must yield. Preserve
	// recovery when the client clears a champion taken or banned by others.
	grid := map[int64]champSelectGridChampion{last.ChampionID: {ID: last.ChampionID}}
	mergeChampSelectTeamState(grid, session)
	champion := grid[last.ChampionID]
	return !champion.TeammatePicked && !champion.SelectionStatus.IsBanned
}

func (r *watchRunner) observeChampSelectBench(session lcuChampSelectSession, definition champSelectGroupDefinition, group champSelectGroupConfig, position, runtimeID string) bool {
	if !definition.HasBench || !session.BenchEnabled {
		return false
	}
	current := champSelectCurrentChampion(session)
	r.mu.Lock()
	if runtimeID != "" && runtimeID != r.champSelect.runtimeID {
		r.mu.Unlock()
		return true
	}
	previous := r.champSelect.benchChampionID
	observed := r.champSelect.benchObserved
	swap := r.champSelect.benchSwap
	pending := r.champSelect.decision[champSelectActionBench]
	// Remember that our POST was observed even if its HTTP acknowledgement is
	// still pending. Once observed, any subsequent change belongs to the user.
	ownEcho := swap != nil && !swap.Observed && current == swap.Decision.ChampionID
	if ownEcho {
		swap.Observed = true
	}
	tracking := slices.Contains(group.Pick.Champions[position], previous) || pending.Action != "" || swap != nil
	changed := group.Bench.Enabled && observed && previous > 0 && current > 0 && current != previous && tracking && !ownEcho
	championID := previous
	if !slices.Contains(group.Pick.Champions[position], championID) {
		championID = pending.ChampionID
		if championID == 0 && swap != nil {
			championID = swap.Decision.ChampionID
		}
	}
	if current > 0 {
		r.champSelect.benchObserved, r.champSelect.benchChampionID = true, current
	}
	changed = changed && r.yieldChampSelectLocked("bench", championID)
	yielded := r.champSelect.takeover["bench"]
	r.mu.Unlock()
	if changed {
		r.reportChampSelectTakeover("bench", championID)
	}
	return yielded
}

func (r *watchRunner) champSelectBenchRequestStillCurrent(ctx context.Context, client *LCUClient, decision champSelectDecision) bool {
	var session lcuChampSelectSession
	if err := client.RequestJSON(ctx, http.MethodGet, champSelectAPI+"/session", nil, &session); err != nil {
		return false
	}
	if session.GameID != decision.GameID || session.LocalPlayerCellID == nil || *session.LocalPlayerCellID != decision.LocalCellID || session.QueueID != decision.QueueID {
		return false
	}
	r.mu.Lock()
	if r.champSelect.runtimeID != decision.RuntimeID {
		r.mu.Unlock()
		return false
	}
	group := r.settings.ChampSelect.Groups[r.champSelect.groupID]
	definition, _ := champSelectGroupDefinitionFor(r.champSelect.groupID)
	position := r.champSelect.position
	r.mu.Unlock()
	if r.observeChampSelectBench(session, definition, group, position, decision.RuntimeID) {
		return false
	}
	if champSelectCurrentChampion(session) != decision.FromChampionID {
		r.yieldChampSelect("bench", decision.ChampionID, decision.RuntimeID)
		return false
	}
	if !definition.HasBench || !session.BenchEnabled || !group.Bench.Enabled || !slices.Contains(group.Pick.Champions[position], decision.ChampionID) {
		return false
	}
	for _, champion := range session.BenchChampions {
		if champion.ChampionID == decision.ChampionID {
			return ctx.Err() == nil
		}
	}
	r.champDiagnostic("bench-preflight", fmt.Sprintf("target-%d-no-longer-on-bench", decision.ChampionID), decision, nil)
	return false
}
