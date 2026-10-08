package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestR187InvalidatePlayerAllSourcesScopesAndFlights(t *testing.T) {
	cache := newRankScoreCache()
	ref, other := strings.Repeat("p", 48), strings.Repeat("p", 48)+"other"
	entry := rankScoreEntry{at: time.Now(), known: true, score: 1450}
	keys := []string{}
	for _, source := range []string{dataSourceLCU, dataSourceSGP, dataSourceRiot, "unknown"} {
		for _, scope := range []string{"", rankScoreTierOnlyScope} {
			key := rankScoreCacheKeyScoped(source, "HN1", ref, scope)
			keys = append(keys, key)
			cache.mu.Lock()
			cache.putLocked(key, entry)
			cache.mu.Unlock()
			cache.mu.Lock()
			cache.putLocked(rankScoreCacheKeyScoped(source, "HN1", other, scope), entry)
			cache.mu.Unlock()
		}
	}
	old, _ := cache.beginFlight(keys[0], context.Background())
	foreignKey := rankScoreCacheKeyScoped(dataSourceLCU, "HN1", other, "")
	foreign, _ := cache.beginFlight(foreignKey, context.Background())
	cache.invalidatePlayer(ref)
	for _, key := range keys {
		if _, ok := cache.get(key); ok {
			t.Fatal("stale entry survived", key)
		}
	}
	if len(cache.entries) != len(keys) || cache.recent.Len() != len(keys) {
		t.Fatal("another player's entries removed")
	}
	select {
	case <-old.done:
	case <-time.After(time.Second):
		t.Fatal("old waiter stranded")
	}
	if old.ctx.Err() != context.Canceled {
		t.Fatal("old upstream not canceled")
	}
	select {
	case <-foreign.done:
		t.Fatal("other player's flight canceled")
	default:
	}
	replacement, _ := cache.beginFlight(keys[0], context.Background())
	if cache.finishFlight(keys[0], old, entry, keys[0]) {
		t.Fatal("old flight accepted")
	}
	if cache.flights[keys[0]] != replacement {
		t.Fatal("old completion removed replacement")
	}
	if _, ok := cache.get(keys[0]); ok {
		t.Fatal("old flight repopulated cache")
	}
	if !cache.finishFlight(keys[0], replacement, rankScoreEntry{score: 1467, at: time.Now()}, keys[0]) {
		t.Fatal("replacement not accepted")
	}
	cache.finishFlight(foreignKey, foreign, entry)
	cache.invalidatePlayer("")
}

func TestR187InvalidatedWaiterAndLeaderReceiveFreshRank(t *testing.T) {
	ref := strings.Repeat("q", 48)
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	var calls atomic.Int32
	client := &LCUClient{baseURL: "http://lcu.local", token: "test", region: "TENCENT", rsoPlatform: "HN1", platformProbe: true}
	client.http = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		lp := 67
		if calls.Add(1) == 1 {
			close(started)
			<-release
			lp = 50
		}
		return r178JSON(map[string]any{"queues": []any{map[string]any{"queueType": "RANKED_FLEX_SR", "tier": "GOLD", "division": "II", "leaguePoints": lp, "wins": 11, "losses": 10}}}, 200), nil
	})}
	a := &app{rankScores: newRankScoreCache()}
	leader := make(chan rankScoreEntry, 1)
	go func() { leader <- a.playerRankScore(context.Background(), client, ref, true, "HN1", "") }()
	<-started
	waiter := make(chan rankScoreEntry, 1)
	waiterCtx := &r108WaitingContext{Context: context.Background(), waiting: make(chan struct{})}
	go func() { waiter <- a.playerRankScore(waiterCtx, client, ref, true, "HN1", "") }()
	<-waiterCtx.waiting
	a.invalidateRankPlayer(ref)
	select {
	case result := <-waiter:
		if len(result.ranks) != 1 || result.ranks[0].LeaguePoints != 67 {
			t.Fatal(result)
		}
	case <-time.After(time.Second):
		t.Fatal("waiter waited for old upstream")
	}
	once.Do(func() { close(release) })
	select {
	case result := <-leader:
		if len(result.ranks) != 1 || result.ranks[0].LeaguePoints != 67 {
			t.Fatal("leader returned old rank", result)
		}
	case <-time.After(time.Second):
		t.Fatal("leader stranded")
	}
	if calls.Load() != 2 {
		t.Fatal("replacement flight not shared", calls.Load())
	}
}

func TestR187ObserveRejectsPostCaptureRegressionAndFallbackNextGame(t *testing.T) {
	ref := strings.Repeat("s", 48)
	queue := "RANKED_FLEX_SR"
	before := lpSnapshot{Tier: "GOLD", Division: "II", LeaguePoints: 50, Wins: 10, Losses: 10}
	after := before
	after.Wins++
	after.LeaguePoints += 17
	tracker, hash := seededLPTracker(t, ref, queue, before)
	events := captureLPObservations(tracker)
	first := lpTestClient(t, 187001, 440)
	tracker.takeGameStart(first, ref, func() ([]gameplayRank, EndpointCapability) {
		return []gameplayRank{r182Rank(before, queue)}, EndpointCapability{State: capabilityAvailable, Path: "lcu:ranked"}
	})
	r182Capture(tracker, first, ref, queue, []lpSnapshot{after}, nil)
	if tracker.history.Games["187001"].Delta != 17 {
		t.Fatal(tracker.history.Games)
	}
	now := time.Now()
	tracker.now = func() time.Time { return now }
	for i := 0; i < 5; i++ {
		tracker.observe(ref, []gameplayRank{r182Rank(before, queue)})
	}
	if got := tracker.history.Baselines[hash][queue]; got != after {
		t.Fatal("post-capture baseline rolled back", got)
	}
	rejection, ok := lpObservation(*events, "lp_snapshot_rejected")
	if !ok || rejection["stage"] != "observe" || rejection["reason"] != "stale_regression" || lpObservationCount(*events, "lp_snapshot_rejected") != 1 {
		t.Fatal(*events)
	}
	now = now.Add(time.Minute)
	tracker.observe(ref, []gameplayRank{r182Rank(before, queue)})
	if lpObservationCount(*events, "lp_snapshot_rejected") != 2 {
		t.Fatal("rejection dedup did not expire")
	}
	// No next-game start snapshot: the preserved queue baseline still yields gap 1.
	next := after
	next.Losses++
	next.LeaguePoints -= 18
	r182Capture(tracker, lpTestClient(t, 187002, 440), ref, queue, []lpSnapshot{next}, nil)
	if got, ok := tracker.history.Games["187002"]; !ok || got.Delta != -18 {
		t.Fatal("fallback capture lost", tracker.history.Games)
	}
	for _, event := range *events {
		if event["event"] == "lp_capture_recorded" && event["baseline_source"] == "capture" && event["games_gap"] != 1 {
			t.Fatal(event)
		}
	}
	assertLPObservationPrivacy(t, *events, ref, hash, "187001", "187002", queue)
}

func TestR187ObserveAllowsSameGamesAndIncreasingGames(t *testing.T) {
	ref := strings.Repeat("t", 48)
	queue := "RANKED_FLEX_SR"
	before := lpSnapshot{Tier: "GOLD", Division: "II", LeaguePoints: 50, Wins: 10, Losses: 10}
	tracker, hash := seededLPTracker(t, ref, queue, before)
	events := captureLPObservations(tracker)
	same := before
	same.LeaguePoints = 60
	tracker.observe(ref, []gameplayRank{r182Rank(same, queue)})
	if tracker.history.Baselines[hash][queue] != same {
		t.Fatal("same-game adjustment rejected")
	}
	more := same
	more.Wins++
	more.LeaguePoints = 77
	tracker.observe(ref, []gameplayRank{r182Rank(more, queue)})
	if tracker.history.Baselines[hash][queue] != more || lpObservationCount(*events, "lp_baseline_written") != 2 || lpObservationCount(*events, "lp_snapshot_rejected") != 0 {
		t.Fatal(*events)
	}
}

type r187OverviewFixture struct {
	a       *app
	reads   atomic.Int32
	settled atomic.Bool
}

func newR187OverviewFixture(t *testing.T) *r187OverviewFixture {
	ref := strings.Repeat("u", 48)
	a, _ := newIdentityOverviewApp(t, 200, Summoner{PUUID: ref, GameName: "玩家", DisplayName: "玩家"})
	f := &r187OverviewFixture{a: a}
	original := a.lcu.http.Transport
	if original == nil {
		original = http.DefaultTransport
	}
	a.lcu.http = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/lol-ranked/v1/current-ranked-stats":
			f.reads.Add(1)
			wins, lp := 10, 50
			if f.settled.Load() {
				wins, lp = 11, 67
			}
			return r178JSON(map[string]any{"queues": []any{map[string]any{"queueType": "RANKED_FLEX_SR", "tier": "GOLD", "division": "II", "leaguePoints": lp, "wins": wins, "losses": 10}}}, 200), nil
		case "/lol-gameflow/v1/session":
			return r178JSON(map[string]any{}, 200), nil
		default:
			return original.RoundTrip(r)
		}
	})}
	a.rankScores = newRankScoreCache()
	a.overviewQueries = newOverviewQueryCache()
	return f
}
func assertR187OverviewRank(t *testing.T, response gameplayOverview, wins, lp int) {
	t.Helper()
	if len(response.Ranks) != 1 || response.Ranks[0].Wins != wins || response.Ranks[0].LeaguePoints != lp {
		t.Fatal(response.Ranks)
	}
}
func TestR187EndOfGameOverviewReadsFreshRank(t *testing.T) {
	f := newR187OverviewFixture(t)
	assertR187OverviewRank(t, callCurrentOverview(t, f.a, false), 10, 50)
	if f.reads.Load() != 1 {
		t.Fatal(f.reads.Load())
	}
	assertR187OverviewRank(t, callCurrentOverview(t, f.a, false), 10, 50)
	if f.reads.Load() != 1 {
		t.Fatal("fixture did not cache")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// The prewarm uses its normal cancellation path after the next phase arrives.
	release := make(chan struct{})
	f.a.gameplayPrewarmHook = func() { <-release }
	f.a.observeGameplayPhase(ctx, f.a.lcu, "InProgress")
	f.settled.Store(true)
	f.a.observeGameplayPhase(ctx, f.a.lcu, "EndOfGame")
	close(release)
	assertR187OverviewRank(t, callCurrentOverview(t, f.a, false), 11, 67)
	if f.reads.Load() != 2 {
		t.Fatal("settlement did not read rank upstream", f.reads.Load())
	}
}
func TestR187ForceOverviewReadsAndRepopulatesRankCache(t *testing.T) {
	f := newR187OverviewFixture(t)
	assertR187OverviewRank(t, callCurrentOverview(t, f.a, false), 10, 50)
	f.settled.Store(true)
	assertR187OverviewRank(t, callCurrentOverview(t, f.a, true), 11, 67)
	if f.reads.Load() != 2 {
		t.Fatal("manual refresh reused ranks", f.reads.Load())
	}
	entry, ok := f.a.rankScores.get(rankScoreCacheKeyScoped(dataSourceLCU, "HN1", f.a.summoner.PUUID, ""))
	if !ok || len(entry.ranks) != 1 || entry.ranks[0].LeaguePoints != 67 {
		t.Fatal("fresh ranks not cached", entry)
	}
}
func TestR187CaptureInvalidatesEveryCompletion(t *testing.T) {
	for _, mode := range []string{"record", "skip", "timeout", "session-failed", "unranked"} {
		t.Run(mode, func(t *testing.T) {
			tracker, ref, queue, before, client := r182Start(t)
			calls := 0
			tracker.invalidateRanks = func(got string) {
				if got != ref {
					t.Error("wrong player")
				}
				calls++
			}
			events := captureLPObservations(tracker)
			next := before
			next.Wins++
			next.LeaguePoints += 17
			switch mode {
			case "skip":
				next.Tier = "UNRANKED"
			case "timeout":
				next = before
			case "session-failed":
				client = &LCUClient{baseURL: "http://lcu.local", http: &http.Client{Transport: gameplayRoundTripFunc(func(*http.Request) (*http.Response, error) { return r178JSON(map[string]any{}, 503), nil })}}
			case "unranked":
				client = lpTestClient(t, 187010, 2400)
			}
			r182Capture(tracker, client, ref, queue, []lpSnapshot{next}, nil)
			if calls != 1 {
				t.Fatal("capture completion kept cache", calls, *events)
			}
			expected := map[string]string{"record": "lp_capture_recorded", "skip": "lp_capture_skipped", "timeout": "lp_capture_timeout", "session-failed": "lp_capture_ignored", "unranked": "lp_capture_ignored"}[mode]
			if _, ok := lpObservation(*events, expected); !ok {
				t.Fatal(fmt.Sprintf("fixture did not reach %s", expected), *events)
			}
		})
	}
}
func TestR187EndOfGameInvalidatesTierOnlyAndOtherSources(t *testing.T) {
	f := newR187OverviewFixture(t)
	keys := []string{}
	for _, source := range []string{dataSourceLCU, dataSourceSGP} {
		for _, scope := range []string{"", rankScoreTierOnlyScope} {
			key := rankScoreCacheKeyScoped(source, "HN1", f.a.summoner.PUUID, scope)
			keys = append(keys, key)
			f.a.rankScores.mu.Lock()
			f.a.rankScores.putLocked(key, rankScoreEntry{at: time.Now()})
			f.a.rankScores.mu.Unlock()
		}
	}
	// Even if WaitingForStats already passed, entering EndOfGame clears a later refill.
	f.a.gameplayFlow.client = f.a.lcu
	f.a.gameplayFlow.phase = "WaitingForStats"
	f.a.gameplayFlow.ended = true
	f.a.observeGameplayPhase(context.Background(), f.a.lcu, "EndOfGame")
	for _, key := range keys {
		if _, ok := f.a.rankScores.get(key); ok {
			t.Fatal("end phase kept source/scope", key)
		}
	}
}

func TestR187FallbackWithoutStartKeepsLatestQueueBaseline(t *testing.T) {
	ref := strings.Repeat("v", 48)
	queue := "RANKED_FLEX_SR"
	old := lpSnapshot{Tier: "GOLD", Division: "II", LeaguePoints: 50, Wins: 10, Losses: 10}
	tracker, _ := seededLPTracker(t, ref, queue, old)
	after := old
	after.Wins++
	after.LeaguePoints = 67
	r182Capture(tracker, lpTestClient(t, 187101, 440), ref, queue, []lpSnapshot{after}, nil)
	tracker.observe(ref, []gameplayRank{r182Rank(old, queue)})
	events := captureLPObservations(tracker)
	next := after
	next.Losses++
	next.LeaguePoints = 49
	r182Capture(tracker, lpTestClient(t, 187102, 440), ref, queue, []lpSnapshot{next}, nil)
	record, ok := tracker.history.Games["187102"]
	if !ok || record.Delta != -18 {
		t.Fatal("next no-start game used rolled-back baseline", tracker.history.Games, *events)
	}
	event, ok := lpObservation(*events, "lp_capture_recorded")
	if !ok || event["games_gap"] != 1 || event["baseline_source"] != "capture" {
		t.Fatal(*events)
	}
}
