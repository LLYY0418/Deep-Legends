package main

// Caller holds r.mu. A round can touch at most three different pick champions.
// Each write step retains its existing two-attempt bound, so successful intent,
// hover and lock are not mistaken for three failures of the same champion.
func (r *watchRunner) champSelectPickBudgetLocked(actionID, championID int64) bool {
	attempts := r.champSelect.pickAttempts[actionID]
	return !r.champSelect.pickFailed[actionID][championID] && (attempts[championID] > 0 || len(attempts) < 3)
}
func (r *watchRunner) champSelectRecoverableCandidates(actionID int64, available map[int64]struct{}) map[int64]struct{} {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make(map[int64]struct{}, len(available))
	for id := range available {
		if r.champSelectPickBudgetLocked(actionID, id) {
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
