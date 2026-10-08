package main

import (
	"testing"
	"time"
)

func TestR258ChampSelectTickerOnlyInConfirmedLivePhase(t *testing.T) {
	a, client, _ := r258ViewFixture()
	poll := &champSelectPolling{interval: 5 * time.Millisecond}
	defer poll.stop()
	phase := func(value string) {
		a.gameplayFlow.mu.Lock()
		a.gameplayFlow.client = client
		a.gameplayFlow.phase = value
		a.gameplayFlow.mu.Unlock()
	}
	for _, value := range []string{"", "None", "Lobby", "Matchmaking", "InProgress", "Reconnect"} {
		phase(value)
		if poll.sync(a, client) || poll.ticker != nil || poll.ticks != nil {
			t.Fatalf("ticker exists in %q", value)
		}
	}
	phase("ChampSelect")
	if !poll.sync(a, client) || poll.ticker == nil {
		t.Fatal("selection ticker absent")
	}
	select {
	case <-poll.ticks:
	case <-time.After(time.Second):
		t.Fatal("active selection never ticks")
	}
	original := poll.ticker
	poll.sync(a, client)
	if poll.ticker != original {
		t.Fatal("same phase restarted ticker")
	}
	phase("GameStart")
	poll.sync(a, client)
	if poll.ticker != nil || poll.ticks != nil {
		t.Fatal("ticker retained after leaving selection")
	}
	phase("ChampSelect")
	a.eventStream = false
	poll.sync(a, client)
	if poll.ticker != nil {
		t.Fatal("closed stream kept selection timer")
	}
	a.eventStream = true
	poll.sync(a, client)
	a.shutdownClient = client
	poll.sync(a, client)
	if poll.ticker != nil {
		t.Fatal("shutdown kept selection timer")
	}
}

func TestR258PhaseSubscriptionCoalescesAndDoesNotDetachSuccessor(t *testing.T) {
	a, client, _ := r258ViewFixture()
	old, unsubscribe := a.subscribeGameplayPhase(client)
	a.signalGameplayPhase(&LCUClient{})
	if len(old) != 0 {
		t.Fatal("old/foreign client sent phase pulse")
	}
	a.signalGameplayPhase(client)
	a.signalGameplayPhase(client)
	if len(old) != 1 {
		t.Fatal("phase changes did not coalesce")
	}
	nextClient := &LCUClient{}
	next, done := a.subscribeGameplayPhase(nextClient)
	defer done()
	unsubscribe()
	a.signalGameplayPhase(nextClient)
	if len(next) != 1 {
		t.Fatal("old unsubscribe removed successor")
	}
}

func TestR258IdlePrefetchWaitsContinuousIdleAndDoesNotDuplicateCollection(t *testing.T) {
	a, client, _ := r258ViewFixture()
	a.refreshRequests = make(chan struct{}, 1)
	phase := func(value string) {
		a.gameplayFlow.mu.Lock()
		a.gameplayFlow.client, a.gameplayFlow.phase = client, value
		a.gameplayFlow.mu.Unlock()
	}
	start := time.Now()
	phase("None")
	if a.maybeStartupPrefetch(client, start) || a.maybeStartupPrefetch(client, start.Add(9*time.Second)) {
		t.Fatal("prefetch before ten seconds")
	}
	phase("InProgress")
	a.maybeStartupPrefetch(client, start.Add(10*time.Second))
	phase("Lobby")
	a.maybeStartupPrefetch(client, start.Add(11*time.Second))
	if a.maybeStartupPrefetch(client, start.Add(20*time.Second)) {
		t.Fatal("non-idle gap retained old deadline")
	}
	if !a.maybeStartupPrefetch(client, start.Add(21*time.Second)) || len(a.refreshRequests) != 1 {
		t.Fatal("continuous idle never prefetched")
	}
	a.startupPrefetchDone = false
	a.maybeStartupPrefetch(client, start.Add(22*time.Second))
	if !a.startupPrefetchDone || len(a.refreshRequests) != 1 {
		t.Fatal("existing collection work was not consumed once")
	}
}
