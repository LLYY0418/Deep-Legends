package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

func r272Tracker(t *testing.T, ref string) *lpTracker {
	t.Helper()
	store := trackTestStore(t, &localStore{root: t.TempDir(), salt: bytes.Repeat([]byte{7}, 32)})
	tracker := newLPTracker(store)
	tracker.history.Games["1"] = lpGameRecord{AccountHash: tracker.accountHash(ref), QueueType: "RANKED_SOLO_5x5", Delta: -22, RecordedAt: 1}
	return tracker
}
func TestR272LPRevisionAndDeltaFrames(t *testing.T) {
	ref := strings.Repeat("a", 48)
	tracker := r272Tracker(t, ref)
	matches := []gameplayMatch{{GameID: 1, QueueID: 420}}
	applyMatchScores(&matches[0])
	before := matches[0].Revision
	seen := map[int64]string{}
	if len(compactOverviewProgress(gameplayOverview{Matches: matches}, seen).Matches) != 1 {
		t.Fatal("initial frame absent")
	}
	tracker.annotate(matches, ref)
	if matches[0].LpDelta == nil || *matches[0].LpDelta != -22 {
		t.Fatal("annotation absent")
	}
	if matches[0].Revision == before {
		t.Error("LP annotation did not change Revision")
	}
	if len(compactOverviewProgress(gameplayOverview{Matches: matches}, seen).Matches) != 1 {
		t.Error("LP delta frame suppressed by unchanged Revision")
	}
}
func TestR272LPAccountAndOldRecords(t *testing.T) {
	ref := strings.Repeat("a", 48)
	tracker := r272Tracker(t, ref)
	tracker.mu.Lock()
	tracker.history.SchemaVersion = 1
	tracker.persistLocked()
	tracker.mu.Unlock()
	restarted := newLPTracker(tracker.store)
	for _, input := range []struct {
		ref  string
		want bool
	}{{ref, true}, {strings.Repeat("b", 48), false}, {"", false}} {
		matches := []gameplayMatch{{GameID: 1, QueueID: 420}}
		restarted.annotate(matches, input.ref)
		if (matches[0].LpDelta != nil) != input.want {
			t.Fatalf("legacy account scope mismatch want=%v", input.want)
		}
	}
}
func TestR272LPRiotSelfRoute(t *testing.T) {
	f := r98OverviewFixture(t, false)
	localRef := strings.Repeat("a", 48)
	f.a.summoner = Summoner{PUUID: localRef, GameName: "Fixture", TagLine: "KR1"}
	f.a.lcu = &LCUClient{region: "KR", rsoPlatform: "KR", platformProbe: true}
	f.a.connected = true
	f.a.lpTracker = r272Tracker(t, localRef)
	result, err := f.a.loadRiotOverview(context.Background(), gameplayReference{GameName: "Fixture", TagLine: "KR1", Region: "kr"}, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Player.IsCurrent || len(result.Matches) != 1 || result.Matches[0].LpDelta == nil {
		t.Fatal("Riot self route omitted current-account annotation")
	}
}
func TestR272LPRecordedInvalidatesOverview(t *testing.T) {
	ref := strings.Repeat("a", 48)
	queue := "RANKED_SOLO_5x5"
	before := lpSnapshot{Tier: "GOLD", Division: "II", LeaguePoints: 50, Wins: 10, Losses: 10}
	after := before
	after.Losses++
	after.LeaguePoints = 28
	tracker, _ := seededLPTracker(t, ref, queue, before)
	a := &app{overviewQueries: newOverviewQueryCache(), connected: true, summoner: Summoner{PUUID: ref}}
	key := sourceScopedKey(dataSourceSGP+"+"+dataSourceLCU, "HN1|"+ref+"|true|0|20|all|")
	a.overviewQueries.putLocked(key, overviewQueryCacheEntry{at: time.Now()})
	tracker.onRecorded = a.onLPRecorded
	r182Capture(tracker, lpTestClient(t, 1, 420), ref, queue, []lpSnapshot{after}, nil)
	if tracker.history.Games["1"].Delta != -22 {
		t.Fatal("capture did not record fixture")
	}
	if a.overviewQueries.entries[key] != nil {
		t.Fatal("LP capture left stale overview snapshot")
	}
}
