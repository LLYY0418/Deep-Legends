package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"runtime/pprof"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestR86RiotMatchSingleflightAndCanceledWaiter(t *testing.T) {
	t.Setenv("RIOT_API_KEY", "RGAPI-test")
	var calls atomic.Int32
	started, release := make(chan struct{}, 8), make(chan struct{})
	champions := newChampionProvider()
	champions.client = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		started <- struct{}{}
		select {
		case <-release:
		case <-r.Context().Done():
			return nil, r.Context().Err()
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"metadata":{"matchId":"KR_1"},"info":{"participants":[{"championId":1}]}}`)), Header: make(http.Header), Request: r}, nil
	})}
	provider := newRiotProvider(champions)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			match, _, err := provider.matchByIDWithCache(context.Background(), "KR_1")
			if err != nil || match == nil {
				t.Errorf("match=%v error=%v", match, err)
			}
		}()
	}
	<-started
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := provider.matchByIDWithCache(ctx, "KR_1"); !errors.Is(err, context.Canceled) {
		t.Errorf("canceled waiter=%v", err)
	}
	// Keep the leader blocked while every concurrent caller joins its flight.
	time.Sleep(30 * time.Millisecond)
	close(release)
	wg.Wait()
	if n := calls.Load(); n != 1 {
		t.Fatalf("same match fetched %d times", n)
	}
	if len(provider.matchFlights) != 0 {
		t.Fatal("completed flight retained")
	}
}

type r86NotifyingContext struct {
	context.Context
	checked chan struct{}
}

func (c r86NotifyingContext) Err() error {
	select {
	case c.checked <- struct{}{}:
	default:
	}
	return c.Context.Err()
}

func TestR86CanceledQuotaWaitDoesNotReserveTokens(t *testing.T) {
	provider := newRiotProvider(newChampionProvider())
	base, cancel := context.WithCancel(context.Background())
	ctx := r86NotifyingContext{base, make(chan struct{}, 1)}
	provider.limitMu.Lock()
	done := make(chan error, 1)
	go func() { done <- provider.wait(ctx) }()
	<-ctx.checked
	cancel()
	provider.limitMu.Unlock()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("wait=%v", err)
	}
	if len(provider.shortWindow) != 0 || len(provider.longWindow) != 0 {
		t.Fatal("canceled reservation consumed quota")
	}
}

func TestR86ChampionNamesNegativeCacheAndExpiry(t *testing.T) {
	var calls atomic.Int32
	provider := newChampionProvider()
	provider.client = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		return &http.Response{StatusCode: 500, Body: io.NopCloser(strings.NewReader("unavailable")), Header: make(http.Header), Request: r}, nil
	})}
	for i := 0; i < 5; i++ {
		if got := provider.championNamesZH(context.Background()); len(got) != 0 {
			t.Fatal("failed catalog produced names")
		}
	}
	// loadCatalog stops at the first failed /versions.json request, not three requests.
	if calls.Load() != 1 {
		t.Fatalf("upstream requests=%d, want 1", calls.Load())
	}
	provider.mu.Lock()
	provider.catalogNamesFailUntil = time.Now().Add(-time.Second)
	provider.mu.Unlock()
	provider.championNamesZH(context.Background())
	if calls.Load() != 2 {
		t.Fatal("expired negative cache prevented retry")
	}
}

func TestR86RankCacheDoesNotAcquireApplicationLock(t *testing.T) {
	for _, cache := range []*rankScoreCache{nil, newRankScoreCache()} {
		a := &app{rankScores: cache}
		a.mu.Lock()
		done := make(chan struct{})
		go func() {
			var wg sync.WaitGroup
			for i := 0; i < 100; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					entry, _ := a.playerRankScoreWithCacheStatus(context.Background(), nil, "", false, "", "")
					if entry.known {
						t.Error("unknown player acquired rank")
					}
				}()
			}
			wg.Wait()
			close(done)
		}()
		select {
		case <-done:
			a.mu.Unlock()
		case <-time.After(2 * time.Second):
			a.mu.Unlock()
			<-done
			t.Fatal("rank cache blocked on application lock")
		}
	}
}

func TestR86RiotOverviewCancellationStopsQueuedDetails(t *testing.T) {
	t.Setenv("RIOT_API_KEY", "RGAPI-test")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var detailCalls atomic.Int32
	champions := newChampionProvider()
	champions.championMeta = map[int]championMetadata{1: {NameZH: "测试"}}
	champions.client = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		body := "[]"
		switch {
		case strings.Contains(r.URL.Path, "/lol/summoner/v4/"):
			body = `{"puuid":"subject","summonerLevel":100}`
		case strings.HasSuffix(r.URL.Path, "/ids"):
			var ids []string
			for i := 0; i < 30; i++ {
				ids = append(ids, fmt.Sprintf("\"KR_%d\"", i))
			}
			body = "[" + strings.Join(ids, ",") + "]"
		case strings.Contains(r.URL.Path, "/lol/match/v5/matches/"):
			if detailCalls.Add(1) == 5 {
				cancel()
			}
			body = `{"metadata":{"matchId":"fixture"},"info":{"gameId":1,"queueId":420,"gameDuration":1800,"participants":[{"puuid":"subject","participantId":1,"teamId":100,"championId":1}]}}`
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})}
	provider := newRiotProvider(champions)
	a := &app{riot: provider}
	_, _ = a.loadRiotOverview(ctx, gameplayReference{PlayerRef: "subject", Region: riotRegionKR}, 0, 30)
	if got := detailCalls.Load(); got < 5 || got > 8 {
		t.Fatalf("detail calls=%d, want 5..8", got)
	}
	// Includes four identity/list/profile requests in addition to match details.
	if got := len(provider.longWindow); got > 12 {
		t.Fatalf("quota reservations=%d, want <=8 details + 4 profile requests", got)
	}
}

func TestR86RiotQueuedDetailsExitWhileActiveRequestsHoldSlots(t *testing.T) {
	t.Setenv("DEEP_LEGENDS_RIOT_MATCH_CONCURRENCY", "4")
	t.Setenv("RIOT_API_KEY", "RGAPI-test")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{}, 4)
	release := make(chan struct{})
	finished := make(chan error, 1)
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	var entered, completed atomic.Int32
	champions := newChampionProvider()
	champions.championMeta = map[int]championMetadata{1: {NameZH: "测试"}}
	champions.client = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		body := "[]"
		switch {
		case strings.Contains(r.URL.Path, "/lol/summoner/v4/"):
			body = `{"puuid":"subject","summonerLevel":100}`
		case strings.HasSuffix(r.URL.Path, "/ids"):
			body = `["KR_1","KR_2","KR_3","KR_4","KR_5","KR_6","KR_7","KR_8","KR_9","KR_10","KR_11","KR_12"]`
		case strings.Contains(r.URL.Path, "/lol/match/v5/matches/"):
			entered.Add(1)
			started <- struct{}{}
			// Deliberately keep in-flight HTTP work alive after cancellation, until
			// the test releases it. Bottom-layer p.wait cannot free these four slots.
			<-release
			completed.Add(1)
			body = `{"metadata":{"matchId":"fixture"},"info":{"gameId":1,"participants":[]}}`
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})}
	a := &app{riot: newRiotProvider(champions)}
	// Goroutines inherit pprof labels. Count only this invocation's detail workers,
	// not process-wide NumGoroutine, stack addresses, or a fragile funcN ordinal.
	pprof.Do(ctx, pprof.Labels("r86-add-3", t.Name()), func(ctx context.Context) {
		go func() {
			_, err := a.loadRiotOverview(ctx, gameplayReference{PlayerRef: "subject", Region: riotRegionKR}, 1, 12)
			finished <- err
		}()
	})
	t.Cleanup(func() {
		cancel()
		releaseOnce.Do(func() { close(release) })
		select {
		case err := <-finished:
			if !errors.Is(err, context.Canceled) || completed.Load() != 4 {
				t.Errorf("active requests did not finish normally after release: completed=%d err=%v", completed.Load(), err)
			}
		case <-time.After(3 * time.Second):
			t.Error("overview workers leaked after releasing the fake upstream")
		}
	})
	for i := 0; i < 4; i++ {
		select {
		case <-started:
		case <-time.After(3 * time.Second):
			t.Fatal("could not fill the four detail slots")
		}
	}
	workers := func() (details, labeled int) {
		var profile bytes.Buffer
		if err := pprof.Lookup("goroutine").WriteTo(&profile, 1); err != nil {
			t.Fatal(err)
		}
		for _, stack := range strings.Split(profile.String(), "\n\n") {
			if !strings.Contains(stack, `"r86-add-3":"`+t.Name()+`"`) {
				continue
			}
			var count int
			var header string
			for _, line := range strings.Split(stack, "\n") {
				if strings.Contains(line, " @ ") {
					header = line
					break
				}
			}
			if _, err := fmt.Sscanf(header, "%d @", &count); err != nil {
				t.Fatalf("cannot parse labeled goroutine count: %v\n%s", err, stack)
			}
			labeled += count
			if strings.Contains(stack, "loadRiotOverview.func") {
				details += count
			}
		}
		return details, labeled
	}
	waitForWorkers := func(want int) {
		t.Helper()
		deadline := time.Now().Add(2 * time.Second)
		for {
			got, labeled := workers()
			if got == want && labeled == want+1 { // Include the waiting overview parent.
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("detail workers=%d, want %d while all four active slots remain held; total labeled=%d, want %d", got, want, labeled, want+1)
			}
			time.Sleep(time.Millisecond)
		}
	}
	waitForWorkers(12) // Four active HTTP requests + eight semaphore waiters.
	cancel()
	waitForWorkers(4) // A bare semaphore send leaves all twelve alive and MUST fail.
	if entered.Load() != 4 || completed.Load() != 0 {
		t.Fatalf("active requests were affected: entered=%d completed=%d", entered.Load(), completed.Load())
	}
	select {
	case err := <-finished:
		finished <- err // Keep cleanup able to inspect the terminal result.
		t.Fatal("overview returned before the four active requests were released")
	default:
	}
	// Keep the four requests blocked through every assertion; cleanup releases them.
}

func TestR86PlayAgainEarlierPhaseRearmsWithoutDuplicateOrPostponement(t *testing.T) {
	posted := make(chan time.Time, 4)
	client := &LCUClient{baseURL: "https://127.0.0.1", token: "test", http: &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method == http.MethodPost && r.URL.Path == "/lol-lobby/v2/play-again" {
			posted <- time.Now()
		}
		return &http.Response{StatusCode: 204, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
	})}}
	runner := newWatchRunner(nil, nil)
	settings := defaultWatchSettings()
	settings.MasterEnabled = true
	settings.Rules.AutoPlayAgain.Enabled = true
	settings.Rules.AutoHonor.Enabled = false
	settings.Rules.SkipCelebration.Enabled = false
	runner.apply(settings)
	defer runner.cancelPending("play-again")
	runner.handlePhase(client, "WaitingForStats")
	runner.mu.Lock()
	first := runner.pending["play-again"]
	runner.mu.Unlock()
	if first == nil {
		t.Fatal("first phase did not arm")
	}
	start := time.Now()
	runner.handlePhase(client, "EndOfGame")
	runner.mu.Lock()
	earlier := runner.pending["play-again"]
	runner.mu.Unlock()
	if earlier == first || !earlier.fireAt.Before(first.fireAt) {
		t.Fatal("shorter phase was swallowed by dedupe")
	}
	runner.schedulePlayAgain(client, 10000)
	runner.mu.Lock()
	same := runner.pending["play-again"]
	runner.mu.Unlock()
	if same != earlier {
		t.Fatal("late phase postponed the earliest timer")
	}
	select {
	case at := <-posted:
		if at.Sub(start) < 1500*time.Millisecond {
			t.Fatal("settle delay skipped")
		}
	case <-time.After(2500 * time.Millisecond):
		t.Fatal("play-again retained obsolete ten-second delay")
	}
	select {
	case <-posted:
		t.Fatal("duplicate play-again")
	default:
	}
}
