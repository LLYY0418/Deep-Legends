package main

import (
	"context"
	"encoding/json"
	"time"
)

// Ranked heads finish before mayhem starts; pages are folded on one goroutine.
// Each worker waits for its fold decision before requesting another page.
func (a *app) seasonScanHeadConcurrent(ctx context.Context, client *LCUClient, serverID, playerRef string, scan *seasonScanState, budget int, useCache bool) {
	scan.interrupted = false
	scan.scanID = time.Now().UnixNano()
	scan.cache.Paused = false
	pageSize := sgpPageSize
	// Incremental heads read one game at a time, stopping at the first saved ID.
	// Cold scans retain the existing batched path and background continuation.
	if len(scan.seen) > 0 && scan.cache.Complete {
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
		empty    []sgpEmptyHistoryEntry
	}
	pages := make(chan page, 2)
	done := make(chan string, 2)
	seed := map[string]bool{}
	for _, stream := range []string{"ranked", "flex", "mayhem"} {
		cursor := scan.cache.Streams[stream]
		if cursor.Complete {
			cursor.ResumeIndex = 0
			cursor.PendingIndex = 0
			scan.cache.Streams[stream] = cursor
		}
		seed[stream] = !cursor.Complete && cursor.ResumeIndex == 0
	}
	startStream := func(stream string) {
		go func(stream string) {
			defer func() { done <- stream }()
			defer a.recoverPanic("season.head")
			start := 0
			for n := 0; n < budget; n++ {
				var empty []sgpEmptyHistoryEntry
				infos, consumed, more, err := a.sgp.matchHistoryFilteredWithDecodeOn(ctx, client, serverID, playerRef, start, pageSize, seasonStreamTags[stream], useCache, &empty)
				reply := make(chan bool, 1)
				select {
				case pages <- page{stream, start, infos, consumed, more, err, reply, empty}:
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
	startStream("ranked")
	for workers := 3; workers > 0; {
		select {
		case stream := <-done:
			workers--
			if scan.onStream != nil {
				scan.onStream(scan, stream)
			}
			if stream == "ranked" {
				startStream("flex")
			}
			if stream == "flex" {
				startStream("mayhem")
			}
		case p := <-pages:
			if p.err != nil {
				scan.interrupted = true
				p.next <- false
				continue
			}
			decodeIncomplete := scan.seasonDecodeIncomplete(p.stream, len(p.infos), p.consumed, p.empty)
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
				seasonRecordRankResult(&scan.cache, info, playerRef)
				if scan.seen[info.GameID] {
					hit = true
					continue
				}
				scan.seen[info.GameID] = true
				scan.cache.GameIDs = append(scan.cache.GameIDs, info.GameID)
				scan.scanned++
				scan.newInfos[info.GameID] = info
				seasonStatsAccumulate(scan.stats, scan.queueStats, info, playerRef, scan.seasonStartMillis)
				seasonAccumulateChampionTable(&scan.cache, info, playerRef, scan.seasonStartMillis)
				seasonRecordRankedMatch(&scan.cache, info, playerRef)
			}
			if decodeIncomplete && !boundary {
				scan.interrupted = true
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
			scan.cache.Paused = scan.interrupted
			scan.cache.Streams[p.stream] = cursor
			scan.cache.Complete = len(scan.cache.Streams) == 3
			scan.cache.CappedByUpstream = false
			for _, c := range scan.cache.Streams {
				scan.cache.Complete = scan.cache.Complete && c.Complete
				scan.cache.CappedByUpstream = scan.cache.CappedByUpstream || c.CappedByUpstream
			}
			if scan.onPage != nil {
				scan.onPage(scan)
			}
			a.recordDiagnostic(map[string]any{"event": "season_backfill_round", "stream": p.stream, "head": true, "stop_reason": cursor.StopReason, "oldest_created_at": cursor.OldestCreatedAt, "capped_by_upstream": cursor.CappedByUpstream, "skipped_undecodable": scan.cache.SkippedUndecodable})
			p.next <- !decodeIncomplete && !boundary && p.more && !hit
		}
	}
	// A worker may choose the cancellation arm before delivering its error page.
	// Never mark an interrupted head as freshly checked in that case.
	if ctx.Err() != nil {
		scan.interrupted = true
	}
	scan.cache.Paused = scan.interrupted
}

func (a *app) publishSeasonSnapshot(reference gameplayReference, playerRef string, cache seasonStatsCache) {
	stats := seasonStatsResponse(cache.Stats)
	progress := seasonStatsProgress{Season: cache.Season, Scanned: seasonStatsCount(cache.Stats), Complete: cache.Complete, Collecting: !cache.Complete, TableSupported: len(cache.ChampionTable) > 0 && seasonStatsCount(cache.Stats) >= 20}
	progress.applyUpstreamBoundary(cache)
	event, _ := json.Marshal(map[string]any{"type": "season-progress", "season": cache.Season, "account": a.registerGameplayReferenceDetails(mergeGameplayReferences(reference, gameplayReference{PlayerRef: playerRef})), "scanned": progress.Scanned, "complete": progress.Complete, "paused": progress.Paused, "rankedComplete": progress.RankedComplete, "snapshot": map[string]any{"seasonChampionStats": stats, "seasonOverall": seasonStatsOverall(stats), "seasonStatsProgress": progress, "seasonQueueStats": cache.QueueStats}})
	a.broadcastEvent(string(event))
}
