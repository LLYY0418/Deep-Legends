package main

import (
	"sync"
	"testing"
	"time"
)

func TestR283LPFirstEligibleAndCorrection(t *testing.T) {
	tracker, ref, queue, base, client := r182Start(t)
	first := base
	first.Wins++
	first.LeaguePoints += 18
	stable := first
	stable.LeaguePoints += 2
	calls, notified := 0, []int{}
	var mu sync.Mutex
	tracker.onRecorded = func(string) {
		mu.Lock()
		defer mu.Unlock()
		tracker.mu.Lock()
		defer tracker.mu.Unlock()
		notified = append(notified, tracker.history.Games["182001"].Delta)
	}
	tracker.capture(client, ref, func() ([]gameplayRank, EndpointCapability) {
		mu.Lock()
		defer mu.Unlock()
		if calls == 1 && (len(notified) != 1 || notified[0] != 18) {
			t.Fatal("first eligible read was not published before second read", notified)
		}
		snapshot := first
		if calls > 0 {
			snapshot = stable
		}
		calls++
		return []gameplayRank{r182Rank(snapshot, queue)}, EndpointCapability{State: capabilityAvailable, Path: "lcu:ranked"}
	})
	mu.Lock()
	defer mu.Unlock()
	if len(notified) != 2 || notified[0] != 18 || notified[1] != 20 {
		t.Fatal("missing stable correction", notified)
	}
}
func TestR283LPUnifiedPublicOutput(t *testing.T) {
	ref := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	a := &app{connected: true, summoner: Summoner{PUUID: ref}, lpTracker: r272Tracker(t, ref)}
	out := gameplayOverview{Player: gameplayPlayer{IsCurrent: true}, Matches: []gameplayMatch{{GameID: 1, QueueID: 420}}}
	a.publicizeOverviewReferences(&out)
	if out.Matches[0].LpDelta == nil || *out.Matches[0].LpDelta != -22 {
		t.Fatal("public output lost LP")
	}
}

func TestR283LPPendingIsGameScopedAndExpires(t *testing.T) {
	ref := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	tracker := r272Tracker(t, ref)
	now := tracker.now()
	tracker.now = func() time.Time { return now }
	tracker.pendingGames[tracker.accountHash(ref)+"|42"] = now.Add(90 * time.Second)
	rows := []gameplayMatch{{GameID: 42, QueueID: 420}, {GameID: 43, QueueID: 420}}
	tracker.annotate(rows, ref)
	if !rows[0].LpPending || rows[1].LpPending {
		t.Fatal(rows)
	}
	now = now.Add(91 * time.Second)
	tracker.annotate(rows, ref)
	if rows[0].LpPending {
		t.Fatal("pending exceeded90s")
	}
}
