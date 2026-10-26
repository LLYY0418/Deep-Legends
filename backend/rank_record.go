package main

import (
	"fmt"
	"time"
)

// Progress and aggregates always belong to the same persisted snapshot.
type seasonRankRecordSnapshot struct {
	progress seasonStatsProgress
	byQueue  map[int64]gameplayAggregate
	ranked   []seasonRankedMatch
	player   string
	branch   string
}

func (s seasonRankRecordSnapshot) withBranch(branch string) seasonRankRecordSnapshot {
	s.branch = branch
	return s
}

type seasonRankRecordValue struct{ wins, losses, winRate, games, scanned, upstreamWins int }
type seasonRankRecordPlayer struct {
	season  string
	touched uint64
	queues  map[int64]seasonRankRecordValue
}
type rankedRecordBucket struct {
	event  map[string]any
	minute time.Time
	count  int
}

func rankRecordQueue(queue string) (int64, string) {
	switch queue {
	case "RANKED_SOLO_5x5":
		return 420, "solo"
	case "RANKED_FLEX_SR":
		return 440, "flex"
	}
	return 0, ""
}

// applySeasonRankWinRateFallback is the sole record resolver. A cached derived
// record is reusable only while upstream wins and the season still agree.
func (a *app) applySeasonRankWinRateFallback(ranks []gameplayRank, capability EndpointCapability, snapshot seasonRankRecordSnapshot) ([]gameplayRank, EndpointCapability) {
	ranks = append([]gameplayRank(nil), ranks...)
	a.rankRecordMu.Lock()
	if a.rankRecordCache == nil {
		a.rankRecordCache = make(map[string]*seasonRankRecordPlayer)
	}
	var cached *seasonRankRecordPlayer
	if snapshot.player != "" {
		cached = a.rankRecordCache[snapshot.player]
		if cached != nil && cached.season != snapshot.progress.Season {
			delete(a.rankRecordCache, snapshot.player)
			cached = nil
		}
		allUpstream := true
		for _, rank := range ranks {
			if rank.WinRate < 0 || rank.seasonFallback {
				allUpstream = false
				break
			}
		}
		if allUpstream {
			delete(a.rankRecordCache, snapshot.player)
			cached = nil
		} else {
			if cached == nil {
				if len(a.rankRecordCache) >= 64 {
					oldest := ""
					var touched uint64
					for key, value := range a.rankRecordCache {
						if oldest == "" || value.touched < touched {
							oldest, touched = key, value.touched
						}
					}
					delete(a.rankRecordCache, oldest)
				}
				cached = &seasonRankRecordPlayer{season: snapshot.progress.Season, queues: make(map[int64]seasonRankRecordValue)}
				a.rankRecordCache[snapshot.player] = cached
			}
			a.rankRecordClock++
			cached.touched = a.rankRecordClock
		}
	}
	filled, games, wins, losses := 0, 0, 0, 0
	diagnostics := []map[string]any{}
	for i := range ranks {
		rank := &ranks[i]
		queue, label := rankRecordQueue(rank.QueueType)
		if queue == 0 {
			continue
		}
		officialWins := rank.Wins
		if rank.seasonFallback {
			officialWins = rank.OfficialWins
		}
		rank.OfficialWins = officialWins
		winsDelta := 0
		upstreamComplete := rank.WinRate >= 0 && !rank.seasonFallback
		rank.RecordSource = "upstream"
		rank.RecordPending = false
		if upstreamComplete {
			if cached != nil {
				delete(cached.queues, queue)
			}
		} else {
			rank.RecordSource = "none"
			aggregate, exists := snapshot.byQueue[queue]
			rank.RecordPending = !snapshot.progress.RankedComplete || snapshot.progress.Paused || snapshot.byQueue == nil && snapshot.progress.Scanned > 0
			var value seasonRankRecordValue
			source := ""
			previous, known := seasonRankRecordCached(cached, queue)
			if known {
				winsDelta = officialWins - previous.upstreamWins
			}
			absorbed := known && (aggregate.Games > previous.games || winsDelta != 0 && aggregate.Wins-previous.wins == winsDelta)
			if known && (snapshot.progress.Paused || !snapshot.progress.RankedComplete || !exists || aggregate.Games < previous.games || winsDelta != 0 && !absorbed) {
				value, source = previous, "season-cache"
			} else if snapshot.progress.RankedComplete && exists && aggregate.Games > 0 {
				value = seasonRankRecordValue{wins: aggregate.Wins, losses: aggregate.Losses, winRate: aggregate.WinRate, games: aggregate.Games, scanned: snapshot.progress.Scanned, upstreamWins: officialWins}
				source = "season"
				if cached != nil {
					cached.queues[queue] = value
				}
			} else if known {
				value, source = previous, "season-cache"
			}
			if source != "" {
				rank.Wins, rank.Losses, rank.WinRate = value.wins, value.losses, value.winRate
				rank.seasonFallback = true
				rank.RecordSource = source
				rank.RecordPending = false
				filled++
				games += value.games
				wins += value.wins
				losses += value.losses
			}
		}
		branch := snapshot.branch
		if branch == "" {
			branch = "normal"
		}
		filledBy := rank.RecordSource
		if filledBy == "upstream" {
			filledBy = "none"
		}
		diagnostics = append(diagnostics, map[string]any{"event": "ranked_record_resolved", "branch": branch, "queue": label, "upstream_complete": upstreamComplete, "filled_by": filledBy, "progress_complete": snapshot.progress.RankedComplete, "pending": rank.RecordPending, "cache_reused": rank.RecordSource == "season-cache", "wins_delta": winsDelta})
	}
	a.rankRecordMu.Unlock()
	for _, event := range diagnostics {
		a.recordDiagnostic(event)
	}
	if filled > 0 {
		capability.Detail = "上游未返回排位负场，已按赛季战绩聚合补全胜率"
		a.recordDiagnostic(map[string]any{"event": "sgp_ranked_stats_season_fallback", "ranks_filled": filled, "games": games, "wins": wins, "losses": losses})
	}
	return ranks, capability
}

func seasonRankRecordCached(player *seasonRankRecordPlayer, queue int64) (seasonRankRecordValue, bool) {
	if player == nil {
		return seasonRankRecordValue{}, false
	}
	value, ok := player.queues[queue]
	return value, ok
}

func (a *app) aggregateRankedRecordDiagnostic(event map[string]any, now time.Time) {
	key := fmt.Sprint(event["branch"], "|", event["queue"], "|", event["upstream_complete"], "|", event["filled_by"], "|", event["progress_complete"], "|", event["pending"], "|", event["cache_reused"], "|", event["wins_delta"])
	minute := now.UTC().Truncate(time.Minute)
	var previous map[string]any
	a.rankRecordMu.Lock()
	if a.rankedRecordBuckets == nil {
		a.rankedRecordBuckets = make(map[string]*rankedRecordBucket)
	}
	bucket := a.rankedRecordBuckets[key]
	if bucket != nil && !bucket.minute.Equal(minute) {
		previous = rankedRecordEvent(bucket)
		bucket = nil
	}
	if bucket == nil {
		copy := make(map[string]any, len(event))
		for k, v := range event {
			copy[k] = v
		}
		bucket = &rankedRecordBucket{event: copy, minute: minute}
		a.rankedRecordBuckets[key] = bucket
	}
	bucket.count++
	a.rankRecordMu.Unlock()
	if previous != nil {
		a.appendDiagnosticEvent(previous)
	}
}
func rankedRecordEvent(bucket *rankedRecordBucket) map[string]any {
	event := make(map[string]any, len(bucket.event)+2)
	for k, v := range bucket.event {
		event[k] = v
	}
	event["count"] = bucket.count
	event["window_start"] = bucket.minute.Format(time.RFC3339)
	return event
}
func (a *app) flushRankedRecordDiagnostics() {
	a.rankRecordMu.Lock()
	buckets := a.rankedRecordBuckets
	a.rankedRecordBuckets = nil
	a.rankRecordMu.Unlock()
	for _, bucket := range buckets {
		a.appendDiagnosticEvent(rankedRecordEvent(bucket))
	}
}
