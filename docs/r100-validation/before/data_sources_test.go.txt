package main

import (
	"testing"
	"time"
)

func TestResolveRankDataSourcesMatrix(t *testing.T) {
	tests := []struct {
		name   string
		input  rankDataSourceInput
		want   []string
		reason string
	}{
		{"local prefers LCU", rankDataSourceInput{true, true, false, true}, []string{dataSourceLCU, dataSourceSGP}, "local-client-connected"},
		{"remote uses SGP", rankDataSourceInput{true, true, true, true}, []string{dataSourceSGP}, "cross-server-lcu-unavailable"},
		{"disconnected uses SGP", rankDataSourceInput{true, false, false, true}, []string{dataSourceSGP}, "lcu-unavailable"},
		{"invalid reference", rankDataSourceInput{false, true, false, true}, nil, "invalid-player-reference"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := resolveRankDataSources(test.input)
			if got.Reason != test.reason || len(got.Sources) != len(test.want) {
				t.Fatalf("decision = %#v", got)
			}
			for index := range test.want {
				if got.Sources[index] != test.want[index] {
					t.Fatalf("decision = %#v", got)
				}
			}
		})
	}
}

func TestResolveMatchHistoryDataSourcesMatrix(t *testing.T) {
	tests := []struct {
		name   string
		input  matchHistoryDataSourceInput
		want   []string
		reason string
	}{
		{"local prefers complete SGP roster", matchHistoryDataSourceInput{true, true, false, true}, []string{dataSourceSGP, dataSourceLCU}, "sgp-complete-roster"},
		{"remote never reads current LCU", matchHistoryDataSourceInput{true, true, true, true}, []string{dataSourceSGP}, "cross-server-sgp-only"},
		{"local falls back when SGP unavailable", matchHistoryDataSourceInput{true, true, false, false}, []string{dataSourceLCU}, "sgp-unavailable"},
		{"invalid reference", matchHistoryDataSourceInput{false, true, false, true}, nil, "invalid-player-reference"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := resolveMatchHistoryDataSources(test.input)
			if got.Reason != test.reason || len(got.Sources) != len(test.want) {
				t.Fatalf("decision = %#v", got)
			}
			for index := range test.want {
				if got.Sources[index] != test.want[index] {
					t.Fatalf("decision = %#v", got)
				}
			}
		})
	}
}

func TestResolveTimelineDataSourcesRejectsCrossServerLCU(t *testing.T) {
	local := resolveTimelineDataSources(timelineDataSourceInput{LCUConnected: true, SGPAvailable: true})
	remote := resolveTimelineDataSources(timelineDataSourceInput{LCUConnected: true, RemoteServer: true, SGPAvailable: true})
	if len(local.Sources) != 2 || local.Sources[0] != dataSourceLCU || local.Sources[1] != dataSourceSGP {
		t.Fatalf("local timeline decision = %#v", local)
	}
	if len(remote.Sources) != 1 || remote.Sources[0] != dataSourceSGP || remote.Reason != "cross-server-lcu-unavailable" {
		t.Fatalf("remote timeline decision = %#v", remote)
	}
}

func TestRankScoreCacheDoesNotCrossReadSources(t *testing.T) {
	cache := newRankScoreCache()
	player := "same-player"
	now := time.Now()
	cache.put(rankScoreCacheKey(dataSourceSGP, "HN1", player), rankScoreEntry{score: 1234, known: true, at: now})
	cache.put(rankScoreCacheKey(dataSourceLCU, "HN1", player), rankScoreEntry{score: 5678, known: true, at: now})
	sgp, sgpOK := cache.get(rankScoreCacheKey(dataSourceSGP, "HN1", player))
	lcu, lcuOK := cache.get(rankScoreCacheKey(dataSourceLCU, "HN1", player))
	if !sgpOK || !lcuOK || sgp.score != 1234 || lcu.score != 5678 {
		t.Fatalf("source-scoped rank cache values = sgp:%#v/%v lcu:%#v/%v", sgp, sgpOK, lcu, lcuOK)
	}
}

func TestTimelineCacheDoesNotCrossReadSources(t *testing.T) {
	cache := newMatchTimelineCache()
	sgpKey := matchTimelineCacheKey(dataSourceSGP, "cn:HN1", 42, 7)
	lcuKey := matchTimelineCacheKey(dataSourceLCU, "cn:HN1", 42, 7)
	cache.put(sgpKey, matchTimelineResponse{Available: true, Source: dataSourceSGP, ItemGroups: []timelineItemGroup{{Minute: 1, Events: []timelineItemEvent{{ItemID: 1001}}}}})
	cache.put(lcuKey, matchTimelineResponse{Available: true, Source: dataSourceLCU, ItemGroups: []timelineItemGroup{{Minute: 2, Events: []timelineItemEvent{{ItemID: 2003}}}}})
	sgp, sgpOK := cache.get(sgpKey)
	lcu, lcuOK := cache.get(lcuKey)
	if !sgpOK || !lcuOK || sgp.Source != dataSourceSGP || lcu.Source != dataSourceLCU || sgp.ItemGroups[0].Events[0].ItemID != 1001 || lcu.ItemGroups[0].Events[0].ItemID != 2003 {
		t.Fatalf("source-scoped timeline cache values = sgp:%#v/%v lcu:%#v/%v", sgp, sgpOK, lcu, lcuOK)
	}
}

func TestSummonerCacheDoesNotCrossReadSources(t *testing.T) {
	cache := newSGPProvider()
	player := "same-player-ref"
	sgpKey := summonerCacheKey(dataSourceSGP, "HN1", player)
	lcuKey := summonerCacheKey(dataSourceLCU, "HN1", player)
	cache.cacheSummoner(sgpKey, sgpSummoner{Name: "SGP value"})
	cache.cacheSummoner(lcuKey, sgpSummoner{Name: "LCU value"})
	sgp, sgpOK := cache.cachedSummoner(sgpKey)
	lcu, lcuOK := cache.cachedSummoner(lcuKey)
	if !sgpOK || !lcuOK || sgp.Name != "SGP value" || lcu.Name != "LCU value" {
		t.Fatalf("source-scoped summoner cache values = sgp:%#v/%v lcu:%#v/%v", sgp, sgpOK, lcu, lcuOK)
	}
}
