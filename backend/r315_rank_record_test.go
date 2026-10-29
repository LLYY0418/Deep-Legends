package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestR315CountsOnlySplitMatchesAndIgnoresOfficialWins(t *testing.T) {
	start := int64(1785351600000)
	snapshot := seasonRankRecordSnapshot{player: "HN1|r315", progress: seasonStatsProgress{Season: "S26", RankedComplete: true, Scanned: 500}, byQueue: map[int64]gameplayAggregate{420: {Games: 500, Wins: 400, Losses: 100}}}
	snapshot.ranked = append(snapshot.ranked, seasonRankedMatch{CreatedAt: start - 1, QueueID: 420, Win: true})
	for i := 0; i < 29; i++ {
		snapshot.ranked = append(snapshot.ranked, seasonRankedMatch{GameID: int64(i + 1), CreatedAt: start + int64(i), QueueID: 420, Win: i < 11})
	}
	for i := 0; i < 80; i++ {
		snapshot.ranked = append(snapshot.ranked, seasonRankedMatch{GameID: int64(i + 100), CreatedAt: start + int64(i), QueueID: 440, Win: i < 35})
	}
	a := &app{}
	for i := 0; i < 11; i++ {
		out, _ := a.applySeasonRankWinRateFallback([]gameplayRank{{QueueType: "RANKED_SOLO_5x5", Wins: 49 + i%2, WinRate: -1}, {QueueType: "RANKED_FLEX_SR", Wins: 154, WinRate: -1}}, EndpointCapability{}, snapshot)
		for j, want := range [][3]int{{11, 18, 38}, {35, 45, 44}} {
			if r := out[j]; r.Wins != want[0] || r.Losses != want[1] || r.WinRate != want[2] || r.RecordPending {
				t.Fatalf("refresh %d: %+v", i, r)
			}
		}
	}
}

func TestR315SplitSourceAndFutureSeason(t *testing.T) {
	before := time.UnixMilli(rankSplitStart + 1)
	after := time.UnixMilli(rankSplitEnd + 1)
	for _, c := range []struct {
		raw    string
		now    time.Time
		start  int64
		source string
	}{
		{`{"seasons":{"RANKED_SOLO_5x5":{"currentSeasonId":23,"currentSeasonStart":1785351600000,"currentSeasonEnd":1799247600000}}}`, before, rankSplitStart, "sgp"},
		{`{}`, before, rankSplitStart, "constant"},
		{`{}`, after, rankSplitEnd, "sgp-end"},
		{`{"seasons":{"RANKED_SOLO_5x5":{"currentSeasonStart":1785351600000,"currentSeasonEnd":1799247600000}}}`, after, rankSplitEnd, "sgp-end"},
		{`{"seasons":{"RANKED_SOLO_5x5":{"currentSeasonStart":1785351600000}}}`, after, rankSplitEnd, "sgp-end"},
		{`{"seasons":{"RANKED_SOLO_5x5":{"currentSeasonId":24,"currentSeasonStart":1799262000000}}}`, after, 1799262000000, "sgp"},
	} {
		var stats sgpRankedStats
		if err := json.Unmarshal([]byte(c.raw), &stats); err != nil {
			t.Fatal(err)
		}
		start, source := resolveRankSplit(stats.Seasons["RANKED_SOLO_5x5"], c.now)
		if start != c.start || source != c.source {
			t.Fatalf("%s: %d %s", c.raw, start, source)
		}
	}
}
func TestR315CompactRecordsSurviveRecentAndDiskCaps(t *testing.T) {
	cache := seasonStatsCache{RankRecords: []seasonRankedRecord{}, Stats: []gameplaySeasonChampionStat{{ChampionName: strings.Repeat("x", seasonStatsFileBudgetBytes)}}}
	for i := 0; i < 120; i++ {
		cache.RankRecords = append(cache.RankRecords, seasonRankedRecord{GameID: int64(i + 1), CreatedAt: rankSplitStart + int64(i), QueueID: 420, Win: i < 11})
		cache.RankedMatches = append(cache.RankedMatches, seasonRankedMatch{GameID: int64(i + 1), CreatedAt: rankSplitStart + int64(i), QueueID: 420, Win: i < 11})
	}
	seasonTrimRankedMatches(&cache)
	if len(cache.RankedMatches) != 40 {
		t.Fatal(len(cache.RankedMatches))
	}
	raw, report := marshalSeasonStatsWithinBudget(cache)
	if report.MarshalErr != nil {
		t.Fatal(report.MarshalErr)
	}
	var loaded seasonStatsCache
	if err := json.Unmarshal(raw, &loaded); err != nil {
		t.Fatal(err)
	}
	p := seasonStatsProgress{RankedComplete: true}
	p.applyUpstreamBoundary(loaded)
	p.RankedComplete = true
	out, _ := (&app{}).applySeasonRankWinRateFallback([]gameplayRank{{QueueType: "RANKED_SOLO_5x5", WinRate: -1}}, EndpointCapability{}, seasonRankRecordSnapshot{progress: p, ranked: loaded.RankedMatches})
	if len(loaded.RankRecords) != 120 || out[0].Wins != 11 || out[0].Losses != 109 {
		t.Fatal(len(loaded.RankRecords), out)
	}
}
func TestR315OldCacheMigrationKeepsAggregatesAndRequiresRankRescan(t *testing.T) {
	store := trackTestStore(t, &localStore{root: t.TempDir()})
	cache := seasonStatsCache{SchemaVersion: 15, Source: seasonStatsSource, Season: "S26", AccountHash: "migration", Complete: true, GameIDs: []int64{1}, Stats: []gameplaySeasonChampionStat{{ChampionID: 1, Games: 1, Wins: 1}}, QueueStats: map[int64]gameplayAggregate{420: {Games: 1, Wins: 1}}, Streams: map[string]seasonStatsStream{"ranked": {Complete: true}, "flex": {Complete: true}, "mayhem": {Complete: true}}}
	if _, err := store.saveSeasonStatsReported(cache); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.loadSeasonStats(seasonStatsSource, "migration", "S26")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Complete || loaded.Streams["ranked"].Complete || loaded.Streams["flex"].Complete || !loaded.Streams["mayhem"].Complete || loaded.Stats[0].Games != 1 || len(loaded.GameIDs) != 1 {
		t.Fatal(loaded)
	}
	info := &riotMatchInfo{GameID: 1, QueueID: 420, GameCreation: rankSplitStart, GameDuration: 1800, Participants: []riotParticipant{{PUUID: "p", Win: true}}}
	seasonRecordRankResult(&loaded, info, "p")
	seasonRecordRankResult(&loaded, info, "p")
	if len(loaded.RankRecords) != 1 || loaded.QueueStats[420].Games != 1 {
		t.Fatal(loaded)
	}
}

func r315Matches(w, l int, q int64) []seasonRankedMatch {
	out := make([]seasonRankedMatch, 0, w+l)
	for i := 0; i < w+l; i++ {
		out = append(out, seasonRankedMatch{GameID: q*100000 + int64(i+1), QueueID: q, CreatedAt: rankSplitStart + int64(i), Win: i < w})
	}
	return out
}

func r315SeedRankCache(cache seasonStatsCache) seasonStatsCache {
	for q, v := range cache.QueueStats {
		for _, r := range r315Matches(v.Wins, v.Losses, q) {
			cache.RankRecords = append(cache.RankRecords, seasonRankedRecord{GameID: r.GameID, QueueID: q, CreatedAt: r.CreatedAt, Win: r.Win})
		}
	}
	return cache
}
