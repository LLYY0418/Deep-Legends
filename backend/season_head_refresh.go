package main

import (
	"context"
	"path/filepath"
	"time"
)

func (a *app) recordSeasonHeadRecent(fresh bool, scanned int, complete bool) {
	a.recordDiagnostic(map[string]any{"event": "season_stats_head_refresh", "fresh": fresh, "use_history_cache": false, "skipped": true, "skip_reason": "recent", "new_games": 0, "sgp_requests": 0, "sgp_bytes": 0, "sgp_history_calls": 0, "sgp_history_cache_hits": 0, "decode_failed": 0, "scanned": scanned, "complete": complete})
}

// The unfiltered marker includes games outside the season's tracked queues.
// It has its own timestamp: historical backfill writes cannot renew this gate.
func (a *app) refreshSeasonHead(ctx context.Context, client *LCUClient, reference gameplayReference, player Summoner, playerRef string, names map[int64]string) (seasonStatsProgress, bool, string, int) {
	season, _ := currentRankedSeason(time.Now())
	hash := a.storage.accountHash(player)
	cache, _ := a.storage.loadSeasonStats(seasonStatsSource, hash, season)
	progress := seasonStatsProgress{Season: season, Scanned: seasonStatsCount(cache.Stats), Complete: cache.Complete, Collecting: !cache.Complete}
	progress.applyUpstreamBoundary(cache)
	infos, consumed, _, err := a.sgp.matchHistoryFilteredOn(ctx, client, reference.ServerID, playerRef, 0, 1, nil, false)
	if err != nil || len(infos) < consumed {
		progress.Message = "赛季统计暂时中断，已保留当前进度"
		return progress, false, "", 0
	}
	latest := int64(0)
	if len(infos) > 0 && infos[0] != nil {
		latest = infos[0].GameID
	}
	known := !cache.HeadCheckedAt.IsZero() && latest == cache.HeadGameID
	// Existing snapshots predate the marker. An observed saved ID is sufficient;
	// unsupported modes will establish their own marker after the first scan.
	if latest > 0 && cache.HeadGameID == 0 {
		for _, id := range cache.GameIDs {
			if id == latest {
				known = true
				break
			}
		}
	}
	if known {
		cache.HeadGameID, cache.HeadCheckedAt = latest, time.Now()
		if err := a.storage.saveSeasonHeadMarker(seasonStatsSource, hash, season, latest, time.Now()); err != nil {
			a.recordDiagnostic(map[string]any{"event": "season_head_marker_save_failed", "error_kind": "local_write"})
		}
		return progress, true, "no_new_game", 0
	}
	_, progress, _, _ = a.loadSeasonChampionStatsWithHistoryCache(ctx, client, reference, player, playerRef, names, false)
	final, err := a.storage.loadSeasonStats(seasonStatsSource, hash, season)
	if err != nil {
		return progress, false, "", 0
	}
	seen := map[int64]bool{}
	for _, id := range cache.GameIDs {
		seen[id] = true
	}
	added := 0
	for _, id := range final.GameIDs {
		if !seen[id] {
			added++
		}
	}
	if progress.Message != "赛季统计暂时中断，已保留当前进度" {
		final.HeadGameID, final.HeadCheckedAt = latest, time.Now()
		if err := a.storage.saveSeasonHeadMarker(seasonStatsSource, hash, season, latest, time.Now()); err != nil {
			a.recordDiagnostic(map[string]any{"event": "season_head_marker_save_failed", "error_kind": "local_write"})
		}
	}
	return progress, false, "", added
}

// Update just the marker under the same file lock as backfill/aggregate merges.
func (s *localStore) saveSeasonHeadMarker(source, hash, season string, id int64, at time.Time) error {
	lock := seasonWriteLock(filepath.Join(s.root, seasonStatsFileKey(source, hash, season)))
	lock.Lock()
	defer lock.Unlock()
	cache, err := s.loadSeasonStats(source, hash, season)
	if err != nil {
		return err
	}
	if cache.HeadCheckedAt.After(at) {
		return nil
	}
	cache.HeadGameID, cache.HeadCheckedAt = id, at
	data, report := marshalSeasonStatsWithinBudget(cache)
	if report.MarshalErr != nil {
		return report.MarshalErr
	}
	return writeLocalStoreFile(s, seasonStatsFileKey(source, hash, season), data)
}
