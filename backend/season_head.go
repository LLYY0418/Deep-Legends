package main

import (
	"context"
	"encoding/json"
)

// R235: fetch the two head streams concurrently, but fold pages on one goroutine.
// Each worker waits for its fold decision before requesting another page.
func (a *app) seasonScanHeadConcurrent(ctx context.Context, client *LCUClient, serverID, playerRef string, scan *seasonScanState, budget int, useCache bool) {
	scan.interrupted = false
	pageSize := sgpPageSize
	// Incremental heads read one game at a time, stopping at the first saved ID.
	// Cold scans retain the existing batched path and background continuation.
	if len(scan.seen) > 0 {
		pageSize = 1
		budget = max(budget, seasonScanBackgroundPages*seasonBackfillMaxRounds*sgpPageSize)
	}
	if !isTencentClient(client) {
		scan.interrupted = true
		return
	}
	if scan.cache.Streams == nil {
		scan.cache.Streams = map[string]seasonStatsStream{}
	}
	scan.playerRef = playerRef
	if scan.newInfos == nil {
		scan.newInfos = map[int64]*riotMatchInfo{}
	}
	type page struct {
		stream   string
		offset   int
		infos    []*riotMatchInfo
		consumed int
		more     bool
		err      error
		next     chan bool
	}
	pages := make(chan page, 2)
	done := make(chan struct{}, 2)
	seed := map[string]bool{}
	for _, stream := range []string{"ranked", "mayhem"} {
		cursor := scan.cache.Streams[stream]
		if cursor.Complete {
			cursor.ResumeIndex = 0
			cursor.PendingIndex = 0
			scan.cache.Streams[stream] = cursor
		}
		seed[stream] = !cursor.Complete && cursor.ResumeIndex == 0
	}
	for _, stream := range []string{"ranked", "mayhem"} {
		go func(stream string) {
			defer func() { done <- struct{}{} }()
			defer a.recoverPanic("season.head")
			start := 0
			for n := 0; n < budget; n++ {
				infos, consumed, more, err := a.sgp.matchHistoryFilteredOn(ctx, client, serverID, playerRef, start, pageSize, seasonStreamTags[stream], useCache)
				reply := make(chan bool, 1)
				select {
				case pages <- page{stream, start, infos, consumed, more, err, reply}:
				case <-ctx.Done():
					return
				}
				select {
				case proceed := <-reply:
					if !proceed {
						return
					}
				case <-ctx.Done():
					return
				}
				start += consumed
			}
		}(stream)
	}
	for workers := 2; workers > 0; {
		select {
		case <-done:
			workers--
		case p := <-pages:
			if p.err != nil {
				scan.interrupted = true
				p.next <- false
				continue
			}
			decodeIncomplete := len(p.infos) < p.consumed
			if decodeIncomplete {
				scan.interrupted = true
			}
			if scan.newInfos == nil {
				scan.newInfos = map[int64]*riotMatchInfo{}
			}
			if scan.queueStats == nil {
				scan.queueStats = map[int64]gameplayAggregate{}
			}
			cursor := scan.cache.Streams[p.stream]
			hit, boundary := false, false
			for _, info := range p.infos {
				if info == nil || info.GameID <= 0 {
					continue
				}
				created := normalizeEpochMillis(info.GameCreation)
				if created > 0 && (cursor.OldestCreatedAt == 0 || created < cursor.OldestCreatedAt) {
					cursor.OldestCreatedAt = created
				}
				if created > 0 && created < scan.seasonStartMillis {
					boundary = true
					break
				}
				if scan.seen[info.GameID] {
					hit = true
					break
				}
				scan.seen[info.GameID] = true
				scan.cache.GameIDs = append(scan.cache.GameIDs, info.GameID)
				scan.scanned++
				scan.newInfos[info.GameID] = info
				seasonStatsAccumulate(scan.stats, scan.queueStats, info, playerRef, scan.seasonStartMillis)
				seasonAccumulateChampionTable(&scan.cache, info, playerRef, scan.seasonStartMillis)
				seasonRecordRankedMatch(&scan.cache, info, playerRef)
			}
			if decodeIncomplete {
				// Retain valid games without advancing past an undecodable entry.
				// A later refresh can retry it; the season must remain incomplete.
				cursor.Complete = false
				cursor.StopReason = "decode_failed"
			} else if boundary || !p.more {
				cursor.Complete = true
				cursor.ResumeIndex = 0
				cursor.PendingIndex = 0
				cursor.StopReason = "upstream_end"
				if boundary {
					cursor.StopReason = "season_start"
				} else if cursor.OldestCreatedAt > scan.seasonStartMillis {
					cursor.StopReason = "upstream_window"
					cursor.CappedByUpstream = true
				}
			} else if !cursor.Complete && seed[p.stream] {
				cursor.ResumeIndex = p.offset + p.consumed
				cursor.PendingIndex = cursor.ResumeIndex
			}
			scan.cache.Streams[p.stream] = cursor
			scan.cache.Complete = len(scan.cache.Streams) == 2
			scan.cache.CappedByUpstream = false
			for _, c := range scan.cache.Streams {
				scan.cache.Complete = scan.cache.Complete && c.Complete
				scan.cache.CappedByUpstream = scan.cache.CappedByUpstream || c.CappedByUpstream
			}
			if scan.onPage != nil {
				scan.onPage(scan)
			}
			a.recordDiagnostic(map[string]any{"event": "season_backfill_round", "stream": p.stream, "head": true, "stop_reason": cursor.StopReason, "oldest_created_at": cursor.OldestCreatedAt, "capped_by_upstream": cursor.CappedByUpstream})
			p.next <- !decodeIncomplete && !boundary && p.more && !hit
		}
	}
	// A worker may choose the cancellation arm before delivering its error page.
	// Never mark an interrupted head as freshly checked in that case.
	if ctx.Err() != nil {
		scan.interrupted = true
	}
}

func (a *app) publishSeasonSnapshot(reference gameplayReference, playerRef string, cache seasonStatsCache) {
	stats := seasonStatsResponse(cache.Stats)
	progress := seasonStatsProgress{Season: cache.Season, Scanned: seasonStatsCount(cache.Stats), Complete: cache.Complete, Collecting: !cache.Complete, TableSupported: len(cache.ChampionTable) > 0 && seasonStatsCount(cache.Stats) >= 20}
	progress.applyUpstreamBoundary(cache)
	event, _ := json.Marshal(map[string]any{"type": "season-progress", "season": cache.Season, "account": a.registerGameplayReferenceDetails(mergeGameplayReferences(reference, gameplayReference{PlayerRef: playerRef})), "scanned": progress.Scanned, "complete": progress.Complete, "snapshot": map[string]any{"seasonChampionStats": stats, "seasonOverall": seasonStatsOverall(stats), "seasonStatsProgress": progress, "seasonQueueStats": cache.QueueStats}})
	a.broadcastEvent(string(event))
}
