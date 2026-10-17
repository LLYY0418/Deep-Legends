package main

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestR284OutageRetainedTailAndExhaustion(t *testing.T) {
	a := r175App(t)
	ref := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	client := &LCUClient{region: "TENCENT", rsoPlatform: "HN1", platformProbe: true}
	a.sgp = newSGPProvider()
	a.sgp.historyCircuits = map[string]*sgpHistoryCircuit{"HN1": {until: time.Now().Add(time.Minute)}}
	snapshot := clientHistorySnapshot{}
	for id := int64(1); id <= 25; id++ {
		snapshot.Matches = append(snapshot.Matches, gameplayMatch{GameID: id, CreatedAt: 1700000000000 + id*1000, QueueID: 420})
	}
	raw, _ := json.Marshal(snapshot)
	if err := writeLocalStoreFile(a.storage, a.clientHistorySnapshotKey(clientRiotPlatform(client), "HN1", ref), raw); err != nil {
		t.Fatal(err)
	}
	first, _, head, _ := a.loadCurrentHistoryFast(context.Background(), client, gameplayReference{ServerID: "HN1"}, ref, 20, nil, nil)
	if len(first) != 20 || !head.HistoryServiceDown || !head.LocalHasMore {
		t.Fatalf("fast snapshot lost outage tail: %+v count=%d", head, len(first))
	}
	for _, filter := range []string{"all", "solo"} {
		for _, cursor := range []int{0, 20, 25} {
			rows, _, page := a.loadDetailedMatches(context.Background(), client, gameplayReference{ServerID: "HN1"}, ref, true, cursor, 20, filter, nil, nil)
			want := min(20, 25-cursor)
			if !page.HistoryServiceDown || page.LocalHasMore != (cursor == 0) || page.Count != want || len(rows) != want {
				t.Fatalf("%s cursor=%d page=%+v rows=%d", filter, cursor, page, len(rows))
			}
			if cursor == 20 && rows[0].GameID != 5 {
				t.Fatalf("tail repeated head: %+v", rows)
			}
		}
	}
}
