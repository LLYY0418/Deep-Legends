package main

import "fmt"

// Caller holds r.mu. A round can touch at most three different pick champions.
// Each write step retains its existing two-attempt bound, so successful intent,
// hover and lock are not mistaken for three failures of the same champion.
func (r *watchRunner) champSelectPickBudgetLocked(actionID, championID int64, step string) bool {
	attempts := r.champSelect.pickAttempts[actionID]
	return !r.champSelect.pickFailed[champSelectPickStepKey(actionID, step)][championID] && (attempts[championID] > 0 || len(attempts) < 3)
}
func (r *watchRunner) champSelectRecoverableCandidates(actionID int64, available map[int64]struct{}, step string) map[int64]struct{} {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make(map[int64]struct{}, len(available))
	for id := range available {
		if r.champSelectPickBudgetLocked(actionID, id, step) {
			result[id] = struct{}{}
		}
	}
	return result
}
func champSelectLocalPickUnfinished(session lcuChampSelectSession) bool {
	if champSelectCurrentChampion(session) == 0 || session.LocalPlayerCellID == nil {
		return true
	}
	for _, actions := range session.Actions {
		for _, action := range actions {
			if action.ActorCellID == *session.LocalPlayerCellID && action.Type == "pick" && !action.Completed {
				return true
			}
		}
	}
	return false
}
func champSelectBenchIDs(session lcuChampSelectSession) []int64 {
	ids := make([]int64, 0, len(session.BenchChampions))
	for _, hero := range session.BenchChampions {
		if hero.ChampionID > 0 {
			ids = append(ids, hero.ChampionID)
		}
	}
	return ids
}

// Failure budgets belong to a write step; distinct-candidate limits still belong to the action.
func champSelectPickStepKey(actionID int64, step string) string {
	return fmt.Sprintf("%d:%s", actionID, step)
}
func (r *watchRunner) champSelectRecoverablePool(actionID int64, pool []int64, step string) []int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make([]int64, 0, len(pool))
	for _, id := range pool {
		if r.champSelectPickBudgetLocked(actionID, id, step) {
			result = append(result, id)
		}
	}
	return result
}
func (r *watchRunner) champSelectStopped(d champSelectDecision, reason string) {
	if d.Action != champSelectActionPick || d.Intent {
		return
	}
	key := fmt.Sprintf("stopped:%d", d.ActionID)
	r.mu.Lock()
	first := !r.champSelect.exhausted[key]
	r.champSelect.exhausted[key] = true
	delete(r.champSelect.decision, d.Action)
	r.mu.Unlock()
	if first {
		r.champSelectDecisionLog(d, "fail", fmt.Sprintf("英雄 %d 自动选用已停止，请手动选择", d.ChampionID), d.ChampionID)
		r.champDiagnostic("stopped", reason, d, nil)
		r.champSelectDecisionEmit(d, "watch:failed:"+d.Action+":stopped")
	}
}
