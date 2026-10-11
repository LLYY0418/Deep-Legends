package main

import (
	"strings"
)

// Route identity is verified before selecting the client's salted LP account.
// Riot PUUIDs may belong to another credential; they are never used as LP keys.
func (a *app) riotOverviewIsCurrent(player gameplayPlayer) bool {
	a.mu.RLock()
	current, client, connected := a.summoner, a.lcu, a.connected
	a.mu.RUnlock()
	return connected && client != nil && current.PUUID != "" && current.GameName != "" && current.TagLine != "" &&
		strings.EqualFold(clientRiotPlatform(client), player.Region) && strings.EqualFold(current.GameName, player.GameName) && strings.EqualFold(current.TagLine, player.TagLine)
}
func (a *app) annotateOverviewLP(response *gameplayOverview, path string, quiet ...bool) {
	if response == nil {
		return
	}
	if path == "riot" || path == "cache" && isRiotRegion(response.Player.Region) {
		response.Player.IsCurrent = a.riotOverviewIsCurrent(response.Player)
	}
	ref := ""
	if response.Player.IsCurrent {
		ref = a.currentPlayerRef()
	}
	if len(quiet) > 0 && quiet[0] {
		if ref != "" && a.lpTracker != nil {
			a.lpTracker.annotate(response.Matches, ref)
		}
		return
	}
	a.annotateLP(response.Matches, ref, path, response.Player.IsCurrent)
}
func (a *app) annotateLP(matches []gameplayMatch, ref, path string, isCurrent bool) {
	ranked, available, matched := 0, 0, 0
	for _, m := range matches {
		if _, ok := lpRankedQueues[m.QueueID]; ok && m.Result != "remake" {
			ranked++
		}
	}
	reason := "not_current"
	if isCurrent {
		reason = "empty_ref"
		if ref != "" && a.lpTracker != nil {
			hash := a.lpTracker.accountHash(ref)
			a.lpTracker.mu.Lock()
			for _, record := range a.lpTracker.history.Games {
				if record.AccountHash == hash {
					available++
				}
			}
			a.lpTracker.mu.Unlock()
			a.lpTracker.annotate(matches, ref)
			for _, m := range matches {
				if m.LpDelta != nil {
					matched++
				}
			}
			reason = ""
			if matched == 0 {
				reason = "no_record"
				if available == 0 {
					a.lpTracker.mu.Lock()
					anyRecords := len(a.lpTracker.history.Games) > 0
					a.lpTracker.mu.Unlock()
					if anyRecords {
						reason = "account_mismatch"
					}
				}
			}
		}
	}
	a.recordDiagnostic(map[string]any{"event": "lp_annotate", "path": path, "is_current": isCurrent, "ranked_matches": ranked, "records_available": available, "matched": matched, "skipped_reason": reason})
}
func (a *app) onLPRecorded(ref string) {
	if ref == "" || ref != a.currentPlayerRef() {
		return
	}
	a.invalidateOverviewPlayer(ref)
	if cache := a.overviewQueries; cache != nil {
		cache.mu.Lock()
		for _, element := range cache.entries {
			if element.Value.(overviewQueryCacheItem).entry.response.Player.IsCurrent {
				cache.removeElementLocked(element)
			}
		}
		cache.mu.Unlock()
	}
	// Consumers fetch fresh annotation; client-history files remain valid because
	// annotation is applied after every disk snapshot read as well.
	a.broadcastEvent(`{"type":"lp-recorded"}`)
}
