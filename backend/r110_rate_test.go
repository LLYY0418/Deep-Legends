package main

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func r110RateContext(host, path string) context.Context {
	return context.WithValue(context.Background(), riotRateScopeKey{}, riotRequestRateScope(host, path))
}
func r110Advertise(p *riotProvider, host, path, limit, count string) {
	scope := riotRequestRateScope(host, path)
	p.beginRiotRateRequest(scope)
	p.observeRiotRate(scope, http.Header{"X-App-Rate-Limit": {limit}, "X-App-Rate-Limit-Count": {count}}, 200)
}
func TestR110ProductionHeadersReplacePersonalCapAndHostsHaveSeparateBudgets(t *testing.T) {
	now := time.Now()
	p := &riotProvider{limitNow: func() time.Time { return now }}
	r110Advertise(p, "asia", "/lol/match/v5/matches/KR_1", "500:10,30000:600", "1:10,1:600")
	ctx := r110RateContext("asia", "/lol/match/v5/matches/KR_2")
	for i := 0; i < 150; i++ {
		if err := p.wait(ctx); err != nil {
			t.Fatal(i, err)
		}
	}
	r110Advertise(p, "kr", "/lol/league/v4/entries/by-puuid/id", "20:1,100:120", "100:120")
	if err := p.wait(withRiotSingleWaitLimit(r110RateContext("kr", "/lol/league/v4/entries/by-puuid/other"), time.Millisecond)); !errors.Is(err, errThrottled) {
		t.Fatal("spent regional budget accepted", err)
	}
	if err := p.wait(ctx); err != nil {
		t.Fatal("other region blocked", err)
	}
}
func TestR110MethodQuotaAndShared429Cooldown(t *testing.T) {
	now := time.Now()
	p := &riotProvider{limitNow: func() time.Time { return now }}
	scope := riotRequestRateScope("asia", "/lol/match/v5/matches/KR_1")
	p.beginRiotRateRequest(scope)
	p.observeRiotRate(scope, http.Header{"X-App-Rate-Limit": {"1000:120"}, "X-App-Rate-Limit-Count": {"1:120"}, "X-Method-Rate-Limit": {"1:10"}, "X-Method-Rate-Limit-Count": {"1:10"}}, 200)
	detail := withRiotSingleWaitLimit(r110RateContext("asia", "/lol/match/v5/matches/KR_999"), time.Millisecond)
	if err := p.wait(detail); !errors.Is(err, errThrottled) {
		t.Fatal("method family not shared", err)
	}
	ids := r110RateContext("asia", "/lol/match/v5/matches/by-puuid/a/ids")
	if err := p.wait(ids); err != nil {
		t.Fatal("other method blocked", err)
	}
	p.beginRiotRateRequest(scope)
	p.observeRiotRate(scope, http.Header{"Retry-After": {"30"}, "X-Rate-Limit-Type": {"application"}}, 429)
	if err := p.wait(withRiotSingleWaitLimit(ids, time.Millisecond)); !errors.Is(err, errThrottled) {
		t.Fatal("429 not shared", err)
	}
	now = now.Add(31 * time.Second)
	if err := p.wait(detail); err != nil {
		t.Fatal("cooldown did not recover", err)
	}
}
func TestR110OutOfOrderCountsKeepReservationsAndBackgroundYields(t *testing.T) {
	now := time.Now()
	p := &riotProvider{limitNow: func() time.Time { return now }}
	r110Advertise(p, "asia", "/method", "100:120", "29:120")
	ctx := r110RateContext("asia", "/method")
	for i := 0; i < 6; i++ {
		if err := p.wait(ctx); err != nil {
			t.Fatal(err)
		}
	}
	r110Advertise(p, "asia", "/method", "100:120", "20:120")
	if err := p.admitRiotBackground(ctx); !errors.Is(err, errThrottled) {
		t.Fatal("background used foreground reserve", err)
	}
	for i := 35; i < 100; i++ {
		if err := p.wait(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if err := p.wait(withRiotSingleWaitLimit(ctx, time.Millisecond)); !errors.Is(err, errThrottled) {
		t.Fatal("lower response count erased reservations", err)
	}
	now = now.Add(121 * time.Second)
	if err := p.wait(ctx); err != nil {
		t.Fatal("window did not reset", err)
	}
}
func TestR110InvalidRateHeadersKeepConservativeFallback(t *testing.T) {
	p := &riotProvider{}
	r110Advertise(p, "asia", "/method", "broken,0:5,2:-1,3:99999999999", "NaN")
	// Invalid/zero limits must not create an unlimited host budget.
	if h := p.rateHosts["asia"]; h != nil && len(h.app.windows) == 0 {
		t.Fatal("invalid headers bypassed fallback")
	}
}

func TestR110HTTPAdmissionsLearnProductionHeadersUnderConcurrency(t *testing.T) {
	var calls atomic.Int32
	p := r99SeedProvider(t, t.TempDir(), func(r *http.Request) (*http.Response, error) {
		count := strconv.Itoa(int(calls.Add(1)))
		response := r99Response(`{}`)
		response.Header.Set("X-App-Rate-Limit", "500:10,30000:600")
		response.Header.Set("X-App-Rate-Limit-Count", count+":10,"+count+":600")
		return response, nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var first map[string]any
	if err := p.get(ctx, "asia", "/fixture", nil, &first); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for n := 0; n < 8; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 20; i++ {
				var out map[string]any
				if err := p.get(ctx, "asia", "/fixture", nil, &out); err != nil {
					t.Error(err)
					return
				}
			}
		}()
	}
	wg.Wait()
	if calls.Load() != 161 {
		t.Fatal("production requests still hit personal limit", calls.Load())
	}
}
