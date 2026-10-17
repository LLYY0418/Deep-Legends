package main

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

// The cursor consumes source rows, including rows rejected by the mode filter.
// Overlapping snapshot/cache pages must not reinsert the snapshot on each page.
func TestR284RetainedHistoryPagesConsumeUnionOnce(t *testing.T) {
	a := r175App(t)
	ref := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	now := time.Now()
	client := &LCUClient{region: "TENCENT", rsoPlatform: "HN1", platformProbe: true}
	a.sgp = newSGPProvider()
	a.sgp.historyClock = func() time.Time { return now }
	a.sgp.historyCircuits = map[string]*sgpHistoryCircuit{"HN1": {until: now.Add(time.Minute)}}
	created := func(id int64) int64 { return 1700000000000 + id*1000 }
	queue := func(id int64) int64 {
		if id%2 == 1 {
			return 420
		}
		return 440
	}
	snapshot := clientHistorySnapshot{}
	for _, id := range []int64{9, 8, 7, 6} {
		snapshot.Matches = append(snapshot.Matches, gameplayMatch{GameID: id, CreatedAt: created(id), QueueID: queue(id)})
	}
	raw, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeLocalStoreFile(a.storage, a.clientHistorySnapshotKey(clientRiotPlatform(client), "HN1", ref), raw); err != nil {
		t.Fatal(err)
	}
	for _, ids := range [][]int64{{8, 7, 6, 5}, {5, 4, 3, 2, 1}} {
		for _, id := range ids {
			a.sgp.historyGamesLocked().putSGP("HN1", &riotMatchInfo{GameID: id, GameCreation: created(id), QueueID: queue(id)})
		}
		key := sgpHistoryPageCacheKey("HN1", ref, int(ids[0]), len(ids), nil)
		a.sgp.historyCache[key] = sgpHistoryCacheEntry{gameIDs: ids, consumed: len(ids), more: true}
	}
	for _, filter := range []string{"all", "solo"} {
		cursor := 0
		var got []int64
		for pageNo := 0; pageNo < 3; pageNo++ {
			rows, consumed, more := a.retainedFilterHistory(client, gameplayReference{ServerID: "HN1"}, ref, filter, cursor, 4, nil, nil)
			if consumed != min(4, 9-cursor) || more != (pageNo < 2) {
				t.Fatalf("%s page %d consumed=%d more=%v", filter, pageNo, consumed, more)
			}
			for _, row := range rows {
				got = append(got, row.GameID)
			}
			if filter == "solo" {
				detailed, _, page := a.loadDetailedMatches(context.Background(), client, gameplayReference{ServerID: "HN1"}, ref, true, cursor, 4, filter, nil, nil)
				if page.Count != consumed || page.HasMore != more || !page.FilterFallback || !reflect.DeepEqual(detailed, rows) {
					t.Fatalf("raw cursor must advance by consumed rows: page=%+v rows=%v want=%v", page, detailed, rows)
				}
			}
			cursor += consumed
		}
		want := []int64{9, 8, 7, 6, 5, 4, 3, 2, 1}
		if filter == "solo" {
			want = []int64{9, 7, 5, 3, 1}
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%s duplicate or missing retained rows: got=%v want=%v", filter, got, want)
		}
		rows, n, more := a.retainedFilterHistory(client, gameplayReference{ServerID: "HN1"}, ref, filter, cursor, 4, nil, nil)
		if len(rows) != 0 || n != 0 || more {
			t.Fatalf("exhausted cursor reinserts head: rows=%v consumed=%d more=%v", rows, n, more)
		}
	}
}
