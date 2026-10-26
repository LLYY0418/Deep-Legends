package main

import (
	"net/http"
	"slices"
)

func (a *app) recordRankRecordClientDiagnostic(w http.ResponseWriter, r clientDiagnosticRequest) {
	event := map[string]any{"event": r.Event, "reason": r.Reason}
	valid := true
	switch r.Event {
	case "rank_record_fill_retry":
		valid = slices.Contains([]string{"ok", "error"}, r.Reason) && r.RankRecordResult == r.Reason && r.DirtyAttempt >= 1 && r.DirtyAttempt <= 3
		event["attempt"], event["result"], event["foreign"] = r.DirtyAttempt, r.RankRecordResult, r.RankRecordForeign
	case "ranked_record_resolved":
		valid = r.Reason == "resolved" && slices.Contains([]string{"normal", "budget", "stream-first"}, r.RankRecordBranch) && slices.Contains([]string{"solo", "flex"}, r.RankRecordQueue) && slices.Contains([]string{"season", "season-cache", "none"}, r.RankRecordFilledBy)
		event["branch"], event["queue"], event["upstream_complete"], event["filled_by"], event["progress_complete"], event["pending"] = r.RankRecordBranch, r.RankRecordQueue, r.RankRecordUpstream, r.RankRecordFilledBy, r.RankRecordProgress, r.RankRecordPending
		event["cache_reused"], event["wins_delta"] = r.RankRecordCacheReused, r.RankRecordWinsDelta
	default:
		valid = slices.Contains([]string{"solo", "flex"}, r.RankRecordQueue) && slices.Contains([]string{"upstream", "season", "season-cache", "none"}, r.Source)
		event["queue"], event["source"], event["foreign"] = r.RankRecordQueue, r.Source, r.RankRecordForeign
		if r.Event == "rank_record_downgrade_blocked" {
			valid = valid && r.Reason == "retained"
		} else {
			valid = valid && slices.Contains([]string{"known", "collecting", "paused", "unavailable"}, r.ClientViewState) && r.ClientViewState == r.Reason
			event["state"] = r.ClientViewState
			if r.Event == "rank_record_state" {
				valid = valid && slices.Contains([]string{"initial", "refresh", "season-fill", "retained", "cache"}, r.RankRecordCause) && r.RankRecordSeq >= 1
				event["cause"], event["ranked_complete"], event["render_seq"] = r.RankRecordCause, r.RankRecordComplete, r.RankRecordSeq
			}
		}
	}
	if !valid {
		http.Error(w, "invalid rank record diagnostic", 400)
		return
	}
	a.recordDiagnostic(event)
	w.WriteHeader(http.StatusNoContent)
}
