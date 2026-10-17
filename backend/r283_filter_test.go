package main

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestR283FallbackUnion(t *testing.T) {
	for _, tc := range []struct {
		filter string
		queue  int64
	}{{"solo", 420}, {"flex", 440}, {"arena", 1750}, {"hextech-aram", 2400}, {"more:urf", 900}} {
		all := []gameplayMatch{{GameID: 3, QueueID: tc.queue, CreatedAt: 300}, {GameID: 2, QueueID: tc.queue, CreatedAt: 200}, {GameID: 1, QueueID: tc.queue, CreatedAt: 100}, {GameID: 9, QueueID: 450}}
		got := mergeFallbackHistory(tc.filter, all, []gameplayMatch{{GameID: 2, QueueID: tc.queue, CreatedAt: 200}, {GameID: 4, QueueID: tc.queue, CreatedAt: 400}})
		if len(got) != 4 || got[0].GameID != 4 || got[3].GameID != 1 {
			t.Fatalf("%s missing cached rows: %+v", tc.filter, got)
		}
	}
}

func TestR283CoolingFallbackKeepsSnapshot(t *testing.T) {
	a := r175App(t)
	ref := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	now := time.Now()
	client := &LCUClient{region: "TENCENT", rsoPlatform: "HN1", platformProbe: true}
	a.sgp = &sgpProvider{historyClock: func() time.Time { return now }, historyCircuits: map[string]*sgpHistoryCircuit{"HN1": {until: now.Add(time.Minute)}}}
	for _, tc := range []struct {
		filter string
		queue  int64
	}{{"solo", 420}, {"flex", 440}, {"arena", 1750}} {
		snapshot := clientHistorySnapshot{Matches: []gameplayMatch{{GameID: 3, CreatedAt: 300, QueueID: tc.queue}, {GameID: 2, CreatedAt: 200, QueueID: tc.queue}, {GameID: 1, CreatedAt: 100, QueueID: tc.queue}}}
		raw, _ := json.Marshal(snapshot)
		key := a.clientHistorySnapshotKey(clientRiotPlatform(client), "HN1", ref)
		if err := writeLocalStoreFile(a.storage, key, raw); err != nil {
			t.Fatal(err)
		}
		rows, caps, page := a.loadDetailedMatches(context.Background(), client, gameplayReference{ServerID: "HN1"}, ref, true, 0, 20, tc.filter, nil, nil)
		if len(rows) != 3 || rows[0].GameID != 3 || !page.FilterFallback || len(caps) == 0 || caps[0].HistoryStatus != "official" {
			t.Fatalf("%s lost saved matches: rows=%v caps=%v page=%+v", tc.filter, rows, caps, page)
		}
		if missingHistoryMatches(tc.filter, snapshot.Matches, rows) != 0 || missingHistoryMatches(tc.filter, snapshot.Matches, nil) != 3 {
			t.Fatal("false missing_count")
		}
	}
}
