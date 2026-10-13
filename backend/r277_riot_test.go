package main

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func r277ControlledWait(ctx context.Context, now *time.Time, elapsed *time.Duration, ceiling time.Duration) context.Context {
	var mu sync.Mutex
	return context.WithValue(ctx, riotForegroundWaitKey{}, func(ctx context.Context, delay time.Duration) error {
		if delay <= 0 {
			return errors.New("non-positive wait")
		}
		mu.Lock()
		defer mu.Unlock()
		*elapsed += delay
		*now = now.Add(delay)
		if *elapsed > ceiling {
			return context.DeadlineExceeded
		}
		return ctx.Err()
	})
}

func TestR277HiddenDirectoryOverviewProgress(t *testing.T) {
	f := r98OverviewFixture(t, false)
	p := f.a.riot
	now := time.Unix(277, 0)
	p.foreground.now = func() time.Time { return now }
	p.foreground.visibilityKnown, p.foreground.directoryVisible = true, false
	elapsed := time.Duration(0)
	bg := r277ControlledWait(withRiotBackground(context.Background()), &now, &elapsed, 3*time.Second)
	ids, err := p.matchIDs(bg, "r98-subject", 10, 10)
	if err != nil || len(ids) != 10 {
		t.Fatalf("hidden directory blocked overview background: ids=%d err=%v controlled_wait=%v", len(ids), err, elapsed)
	}
	ids, err = p.matchIDs(context.Background(), "r98-subject", 20, 10)
	if err != nil || len(ids) != 10 {
		t.Fatalf("foreground pagination failed: ids=%d err=%v", len(ids), err)
	}
	elapsed = 0
	pro := r277ControlledWait(f.a.proBackgroundContext(withRiotBackground(context.Background())), &now, &elapsed, 3*time.Second)
	if err := waitProBackground(pro); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("hidden pro seed gate opened: %v", err)
	}
	// Pro-owned HTTP cannot bypass the owner gate either.
	elapsed = 0
	if err := p.wait(pro); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("pro-owned HTTP escaped hidden directory: %v", err)
	}
	t.Logf("overview background and foreground ids progressed; hidden pro seed still blocked")
}

func TestR277OverviewYieldBounded(t *testing.T) {
	for _, state := range []string{"active", "recovery"} {
		t.Run(state, func(t *testing.T) {
			p := newRiotProvider(nil)
			now := time.Unix(277, 0)
			p.foreground.now = func() time.Time { return now }
			if state == "active" {
				p.foreground.active = 1
			} else {
				p.foreground.resumeAt = now.Add(5 * time.Second)
			}
			elapsed := time.Duration(0)
			ctx := r277ControlledWait(withRiotBackground(context.Background()), &now, &elapsed, 3*time.Second)
			if err := p.wait(ctx); err != nil {
				t.Fatalf("overview background did not continue after bounded yield: %v elapsed=%v", err, elapsed)
			}
			if elapsed <= 0 || elapsed > 3*time.Second || len(p.longWindow) != 1 {
				t.Fatalf("yield/admission invalid: elapsed=%v tokens=%d", elapsed, len(p.longWindow))
			}
			t.Logf("controlled yield=%v; continued with background quota", elapsed)
		})
	}
}

func TestR277InternalWaitTimeoutNotNetwork(t *testing.T) {
	p := newRiotProvider(nil)
	p.foreground.active = 1
	now, elapsed := time.Unix(277, 0), time.Duration(0)
	ctx := r277ControlledWait(withRiotBackground(context.Background()), &now, &elapsed, 0)
	err := p.wait(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	body := riotHTTPErrorBody(err, "kr")
	if body.Kind != "internal-wait" || strings.Contains(body.Error, "数据读取超时") {
		t.Fatalf("internal scheduling mislabeled network timeout: %+v", body)
	}
	body = riotHTTPErrorBody(context.DeadlineExceeded, "kr")
	if !strings.Contains(body.Error, "数据读取超时") {
		t.Fatalf("real network timeout lost label: %+v", body)
	}
}

func r277OverflowFixture(t *testing.T, source string) (*riotProvider, *time.Time, *[]*http.Request, *[]map[string]any) {
	store := r206RelayFixture(t)
	t.Setenv("DEEP_LEGENDS_RELAY_ENTRIES", "")
	if source == "user" {
		store.key = "RGAPI-r277-local-user"
	} else {
		riotEmbeddedKey = func() string { return "RGAPI-r277-local-embedded" }
	}
	now := time.Now()
	requests, events := []*http.Request{}, []map[string]any{}
	champions := newChampionProvider()
	var captureMu sync.Mutex
	champions.diag = func(e map[string]any) {
		captureMu.Lock()
		defer captureMu.Unlock()
		events = append(events, e)
	}
	champions.client = &http.Client{Transport: r196RoundTrip(func(r *http.Request) (*http.Response, error) {
		captureMu.Lock()
		defer captureMu.Unlock()
		requests = append(requests, r)
		return r206RelayResponse(200, []byte(`[]`)), nil
	})}
	p := newRiotProvider(champions)
	p.limitNow = func() time.Time { return now }
	p.limitSleep = func(context.Context, time.Duration) error {
		t.Error("foreground waited on exhausted local app window")
		return context.DeadlineExceeded
	}
	scope := riotRequestRateScope(p.clusterHost(), "/lol/match/v5/matches/by-puuid/r277/ids")
	h := http.Header{"X-App-Rate-Limit": {"20:1,100:120"}, "X-App-Rate-Limit-Count": {"0:1,100:120"}}
	p.observeRiotRate(scope, h, 200)
	return p, &now, &requests, &events
}

func TestR277ForegroundQuotaOverflow(t *testing.T) {
	for _, source := range []string{"user", "embedded"} {
		t.Run(source, func(t *testing.T) {
			p, now, requests, events := r277OverflowFixture(t, source)
			for _, start := range []int{0, 10} {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				_, err := p.matchIDs(ctx, "r277", start, 10)
				cancel()
				if err != nil {
					t.Fatalf("foreground overflow failed source=%s: %v", source, err)
				}
			}
			if len(*requests) != 2 {
				t.Fatalf("unexpected requests: %d", len(*requests))
			}
			for _, r := range *requests {
				if r.URL.Host != "relay.example" || r.Header.Get("X-Riot-Token") != "" {
					t.Fatal("overflow did not use separate server credential without local token")
				}
			}
			switches := 0
			for _, e := range *events {
				if e["event"] == "riot_route_switch" && e["reason"] == "quota-overflow" {
					switches++
					if e["same_credential"] != false || e["source"] != source {
						t.Fatalf("overflow diagnostic invalid: %+v", e)
					}
				}
			}
			if switches != 1 {
				t.Fatalf("route switch not deduplicated per exhausted window: %d", switches)
			}
			*now = now.Add(121 * time.Second)
			if _, err := p.matchIDs(context.Background(), "r277", 20, 10); err != nil {
				t.Fatal(err)
			}
			last := (*requests)[len(*requests)-1]
			if last.URL.Host != p.clusterHost() || last.Header.Get("X-Riot-Token") == "" {
				t.Fatal("recovered local app window did not return to direct")
			}
			t.Logf("source=%s foreground overflow x2; switch dedup=1; recovered direct", source)
		})
	}
}

func TestR277OverflowRelay429Countdown(t *testing.T) {
	for _, source := range []string{"user", "embedded"} {
		t.Run(source, func(t *testing.T) {
			p, _, requests, _ := r277OverflowFixture(t, source)
			var captureMu sync.Mutex
			p.champions.client.Transport = r196RoundTrip(func(r *http.Request) (*http.Response, error) {
				captureMu.Lock()
				defer captureMu.Unlock()
				*requests = append(*requests, r)
				response := r206RelayResponse(429, []byte(`{}`))
				response.Header.Set("Retry-After", "17")
				response.Header.Set("X-Rate-Limit-Type", "application")
				return response, nil
			})
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			_, err := p.matchIDs(ctx, "r277", 0, 10)
			body := riotHTTPErrorBody(err, "kr")
			if len(*requests) != 1 || (*requests)[0].URL.Host != "relay.example" || body.Kind != "rate-limited" || body.RetryAfter != 17 {
				t.Fatalf("relay 429 lost genuine countdown: requests=%d body=%+v err=%v", len(*requests), body, err)
			}
		})
	}
}

func TestR277BackgroundNeverQuotaOverflows(t *testing.T) {
	for _, source := range []string{"user", "embedded"} {
		t.Run(source, func(t *testing.T) {
			p, _, requests, events := r277OverflowFixture(t, source)
			_, err := p.matchIDs(withRiotBackground(context.Background()), "r277", 0, 10)
			if !errors.Is(err, errThrottled) || len(*requests) != 0 {
				t.Fatalf("background escaped quota queue: err=%v requests=%d", err, len(*requests))
			}
			for _, e := range *events {
				if e["reason"] == "quota-overflow" {
					t.Fatal("background overflow diagnostic")
				}
			}
		})
	}
}

func TestR277UnhealthyRelayKeepsLocalCountdown(t *testing.T) {
	for _, source := range []string{"user", "embedded"} {
		t.Run(source, func(t *testing.T) {
			p, _, requests, _ := r277OverflowFixture(t, source)
			riotRelays.lastSuccess = time.Now().Add(-11 * time.Minute)
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			_, err := p.matchIDs(ctx, "r277", 0, 10)
			if err == nil {
				t.Fatalf("unhealthy relay used despite local exhaustion: requests=%d", len(*requests))
			}
			body := riotHTTPErrorBody(err, "kr")
			if len(*requests) != 0 || body.Kind != "rate-limited" || body.RetryAfter != 120 {
				t.Fatalf("unhealthy relay used or local countdown lost: requests=%d body=%+v", len(*requests), body)
			}
		})
	}
}

func TestR277TimeRangeIDsBoundary(t *testing.T) {
	f := r98OverviewFixture(t, false)
	p := f.a.riot
	var requests atomic.Int64
	p.champions.client.Transport = r196RoundTrip(func(r *http.Request) (*http.Response, error) {
		requests.Add(1)
		if r.URL.Query().Get("startTime") != "1000" || r.URL.Query().Get("endTime") != "2000" {
			t.Errorf("time range not pushed into ids: %s", r.URL.RawQuery)
		}
		return r206RelayResponse(200, []byte(`[]`)), nil
	})
	ctx := context.WithValue(context.Background(), riotHistoryTimeRangeKey{}, riotHistoryTimeRange{1000, 2000})
	ids, err := p.matchIDs(ctx, "r277", 0, 10)
	if err != nil || len(ids) != 0 || requests.Load() != 1 {
		t.Fatal(ids, requests.Load(), err)
	}
	pagination := riotOverviewPagination(0, len(ids), 10, "")
	if pagination.HasMore {
		t.Fatal("empty bounded ids page did not terminate pagination")
	}
}

func TestR277EmbeddedHeaderHintCannotPinRelayAfterRecovery(t *testing.T) {
	p, now, requests, events := r277OverflowFixture(t, "embedded")
	h := http.Header{"X-App-Rate-Limit": {"100:120"}, "X-App-Rate-Limit-Count": {"100:120"}}
	p.observeEmbeddedQuota(h, 200)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := p.matchIDs(ctx, "r277", 0, 10); err != nil {
		t.Fatal(err)
	}
	switches := 0
	for _, e := range *events {
		if e["event"] == "riot_route_switch" && e["reason"] == "quota-overflow" {
			switches++
		}
	}
	if switches != 1 {
		t.Fatal("embedded header hint bypassed overflow diagnostic", switches)
	}
	*now = now.Add(121 * time.Second)
	if _, err := p.matchIDs(context.Background(), "r277", 10, 10); err != nil {
		t.Fatal(err)
	}
	if len(*requests) != 2 || (*requests)[1].URL.Host != p.clusterHost() {
		t.Fatal("embedded header hint pinned relay after local recovery")
	}
}

func TestR277OverflowDoesNotJoinBlockedLocalFIFO(t *testing.T) {
	for _, source := range []string{"user", "embedded"} {
		t.Run(source, func(t *testing.T) {
			p, _, requests, _ := r277OverflowFixture(t, source)
			p.limitQueue = []*riotLimitWaiter{{turn: make(chan struct{})}}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			if _, err := p.matchIDs(ctx, "r277", 0, 10); err != nil {
				t.Fatal("foreground joined blocked local FIFO", err)
			}
			if len(*requests) != 1 || (*requests)[0].URL.Host != "relay.example" || len(p.limitQueue) != 1 {
				t.Fatal("overflow altered local queue or did not use relay")
			}
		})
	}
}

func TestR277MethodQuotaDoesNotOverflow(t *testing.T) {
	p, now, requests, _ := r277OverflowFixture(t, "user")
	scope := riotRequestRateScope(p.clusterHost(), "/lol/match/v5/matches/by-puuid/r277/ids")
	p.rateHosts[scope.host].app.windows[120*time.Second].used = 0
	p.rateHosts[scope.host].methods[scope.method] = &riotRateBucket{windows: map[time.Duration]*riotRateWindow{120 * time.Second: {limit: 10, used: 10, reset: now.Add(103 * time.Second)}}}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err := p.matchIDs(ctx, "r277", 0, 10)
	body := riotHTTPErrorBody(err, "kr")
	if len(*requests) != 0 || body.Kind != "rate-limited" || body.RetryAfter != 103 {
		t.Fatalf("method quota improperly overflowed: requests=%d body=%+v", len(*requests), body)
	}
}

// No wall-clock timeout hides a local wait here: the injected limiter clock
// observes the requested sleep and fails immediately if a foreground queues.
func TestR277ControlledClockOverflowNoLocalWait(t *testing.T) {
	for _, source := range []string{"user", "embedded"} {
		t.Run(source, func(t *testing.T) {
			p, _, requests, _ := r277OverflowFixture(t, source)
			var waited atomic.Int64
			p.limitSleep = func(ctx context.Context, delay time.Duration) error {
				waited.Add(int64(delay))
				return context.DeadlineExceeded
			}
			_, err := p.matchIDs(context.Background(), "r277", 0, 10)
			if waited.Load() != 0 {
				t.Fatalf("foreground no-local-wait violated: requested=%v source=%s", time.Duration(waited.Load()), source)
			}
			if err != nil || len(*requests) != 1 || (*requests)[0].URL.Host != "relay.example" {
				t.Fatalf("overflow result invalid: err=%v requests=%d", err, len(*requests))
			}
		})
	}
}
