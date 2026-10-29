package main

import (
	"fmt"
	"math"
	"strings"
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

type sgpRankedSeason struct {
	CurrentSeasonID    int   `json:"currentSeasonId"`
	CurrentSeasonStart int64 `json:"currentSeasonStart"`
	CurrentSeasonEnd   int64 `json:"currentSeasonEnd"`
	NextSeasonStart    int64 `json:"nextSeasonStart"`
}

const rankSplitStart int64 = 1785351600000
const rankSplitEnd int64 = 1799247600000

func resolveRankSplit(season sgpRankedSeason, now time.Time) (int64, string) {
	if season.CurrentSeasonStart <= 0 {
		if now.UnixMilli() < rankSplitEnd {
			return rankSplitStart, "constant"
		}
		return rankSplitEnd, "sgp-end"
	}
	end := season.CurrentSeasonEnd
	if end <= season.CurrentSeasonStart {
		end = 0
	}
	if end == 0 && season.CurrentSeasonStart < rankSplitEnd {
		end = rankSplitEnd
	}
	if end == 0 || now.UnixMilli() < end {
		return season.CurrentSeasonStart, "sgp"
	}
	if season.NextSeasonStart >= end && season.NextSeasonStart <= now.UnixMilli() {
		return season.NextSeasonStart, "sgp-next"
	}
	return end, "sgp-end"
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

// The backend alone resolves CN records from complete, timestamped history.
// Upstream annual wins never participate in the current split's counts.
func (a *app) applySeasonRankWinRateFallback(ranks []gameplayRank, capability EndpointCapability, snapshot seasonRankRecordSnapshot) ([]gameplayRank, EndpointCapability) {
	ranks = append([]gameplayRank(nil), ranks...)
	seasons, persisted := a.rankSplitSnapshot(snapshot.player)
	records := snapshot.ranked
	if snapshot.progress.rankRecords != nil {
		records = snapshot.progress.rankRecords
	}
	for i := range ranks {
		rank := &ranks[i]
		queue, label := rankRecordQueue(rank.QueueType)
		if queue == 0 {
			continue
		}
		cn := rank.countryRank || strings.Contains(snapshot.player, "|") && tencentServerName(strings.SplitN(snapshot.player, "|", 2)[0]) != ""
		upstreamComplete := rank.WinRate >= 0 && !rank.seasonFallback && !cn
		rank.RecordSource = "upstream"
		rank.RecordPending = false
		if upstreamComplete {
			continue
		}
		start, source := resolveRankSplit(seasons[rank.QueueType], time.Now())
		split := seasons[rank.QueueType]
		id := split.CurrentSeasonID
		if source == "sgp-next" {
			id++
		}
		if source == "constant" {
			id = 23
		}
		splitLabel, idResult, dateResult := rankSplitLabel(id, start)
		if source == "sgp-end" {
			splitLabel = ""
		}
		if persisted && source == "sgp" {
			source = "persisted"
		}
		if splitLabel == "" {
			a.recordDiagnostic(map[string]any{"event": "rank_split_label_uncertain", "id": id, "start": time.UnixMilli(start).UTC().Format(time.RFC3339), "id_result": idResult, "date_result": dateResult})
		}
		rank.RecordSplitStart, rank.RecordSplitSource = start, source
		rank.RecordSplitID, rank.RecordSplitLabel = id, splitLabel
		date := time.UnixMilli(start).In(time.FixedZone("CST", 8*3600))
		rank.RecordSplitFrom = fmt.Sprintf("%d月%d日", date.Month(), date.Day())
		if !rank.seasonFallback {
			rank.OfficialWins = rank.Wins
		}
		rank.Wins, rank.Losses, rank.WinRate = 0, 0, -1
		rank.RecordSource = "none"
		rank.seasonFallback = true
		rank.RecordPending = !snapshot.progress.RankedComplete || snapshot.progress.Paused
		games := 0
		if !rank.RecordPending {
			seen := map[int64]bool{}
			for _, match := range records {
				if match.QueueID != queue || match.CreatedAt < start {
					continue
				}
				if match.GameID > 0 && seen[match.GameID] {
					continue
				}
				seen[match.GameID] = true
				games++
				if match.Win {
					rank.Wins++
				} else {
					rank.Losses++
				}
			}
			rank.RecordSource = "season"
			if games > 0 {
				rank.WinRate = int(math.Round(float64(rank.Wins) * 100 / float64(games)))
			}
			capability.Detail = "国服排位显示当前赛段已读取的胜负"
		}
		branch := snapshot.branch
		if branch == "" {
			branch = "normal"
		}
		a.recordDiagnostic(map[string]any{"event": "ranked_record_resolved", "branch": branch, "queue": label, "upstream_complete": false, "filled_by": rank.RecordSource, "progress_complete": snapshot.progress.RankedComplete, "pending": rank.RecordPending, "split_start": time.UnixMilli(start).UTC().Format(time.RFC3339), "split_source": source, "split_id": id, "split_label": splitLabel, "split_games": games})
	}
	return ranks, capability
}

func (a *app) aggregateRankedRecordDiagnostic(event map[string]any, now time.Time) {
	key := fmt.Sprint(event["branch"], "|", event["queue"], "|", event["upstream_complete"], "|", event["filled_by"], "|", event["progress_complete"], "|", event["pending"], "|", event["cache_reused"], "|", event["wins_delta"], "|", event["split_source"], "|", event["split_start"], "|", event["split_games"])
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
