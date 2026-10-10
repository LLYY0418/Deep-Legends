package main

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestR265EveryDetailPublishesDerivedStatsAndRetryKeepsHistoryIDs(t *testing.T) {
	f := r98OverviewFixture(t, false)
	f.a.overviewQueries = newOverviewQueryCache()
	old := f.a.riot.champions.client.Transport
	var mu sync.Mutex
	failed := true
	var active, peak, requests atomic.Int32
	f.a.riot.champions.client.Transport = gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if strings.Contains(r.URL.Path, "/lol/match/v5/matches/KR_") {
			requests.Add(1)
			n := active.Add(1)
			defer active.Add(-1)
			for old := peak.Load(); n > old; old = peak.Load() {
				if peak.CompareAndSwap(old, n) {
					break
				}
			}
			time.Sleep(10 * time.Millisecond)
		}
		mu.Lock()
		fail := failed && strings.HasSuffix(r.URL.Path, "/KR_3")
		mu.Unlock()
		if fail {
			return &http.Response{StatusCode: 404, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{}`)), Request: r}, nil
		}
		return old.RoundTrip(r)
	})
	seen := map[int]bool{}
	ctx := context.WithValue(context.Background(), localOverviewCardsProgressKey{}, func(p gameplayOverview) {
		if p.HistoryRequested == 0 {
			return
		}
		if p.HistoryRequested != 10 || p.HistoryLoaded != len(p.Matches) || p.RecentRanked.Games != len(p.Matches) {
			t.Errorf("inconsistent incremental sample: requested=%d loaded=%d matches=%d ranked=%d", p.HistoryRequested, p.HistoryLoaded, len(p.Matches), p.RecentRanked.Games)
		}
		mu.Lock()
		seen[len(p.Matches)] = true
		mu.Unlock()
	})
	// Keep the legacy progress protocol present, so a failed detail is returned
	// as an error rather than silently described as a complete overview.
	ctx = context.WithValue(ctx, riotOverviewProgressKey{}, func(gameplayOverview) {})
	ref := gameplayReference{PlayerRef: "r98-subject", GameName: "Fixture", TagLine: "KR1", Region: "kr", Privacy: "PRIVATE"}
	partial, err := f.a.loadRiotOverviewDeduplicated(ctx, ref, 0, 10, false)
	if err == nil || len(partial.Matches) != 9 || !partial.Pagination.Partial {
		t.Fatalf("first request did not preserve nine verified matches: %v, %d, %+v", err, len(partial.Matches), partial.Pagination)
	}
	for k := 1; k <= 9; k++ {
		if !seen[k] {
			t.Errorf("no individual detail frame at k=%d", k)
		}
	}
	mu.Lock()
	failed = false
	mu.Unlock()
	retry := context.WithValue(ctx, overviewRetryDetailsKey{}, true)
	complete, err := f.a.loadRiotOverviewDeduplicated(retry, ref, 0, 10, true)
	if err != nil || len(complete.Matches) != 10 || complete.Pagination.Partial || complete.HistoryLoaded != 10 {
		t.Fatalf("retry did not complete the same page: %v, %+v", err, complete.Pagination)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.calls["matchIDs"] != 1 || f.calls["detail"] != 10 {
		t.Fatalf("retry reloaded history or already cached details: %+v", f.calls)
	}
	if requests.Load() != 11 || active.Load() != 0 || peak.Load() > int32(f.a.riot.detailConcurrency()) {
		t.Fatalf("detail retry exceeded existing slots or reread successes: requests=%d active=%d peak=%d limit=%d", requests.Load(), active.Load(), peak.Load(), f.a.riot.detailConcurrency())
	}
}
