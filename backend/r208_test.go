package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type r208Clock struct {
	mu    sync.Mutex
	stamp time.Time
}

func (c *r208Clock) now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.stamp }
func (c *r208Clock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stamp = c.stamp.Add(d)
}

func r208Relay(t *testing.T, transport http.RoundTripper) (*riotProvider, *r208Clock, *riotKeyStore) {
	t.Helper()
	store := r206RelayFixture(t)
	c := &r208Clock{stamp: time.Date(2026, 10, 4, 23, 59, 30, 0, time.UTC)}
	riotRelays.now = c.now
	riotRelays.lastSuccess = c.now()
	champions := newChampionProvider()
	champions.client = &http.Client{Transport: transport}
	return newRiotProvider(champions), c, store
}

func TestR208RelayQuotaDaily(t *testing.T) {
	for _, probe := range []bool{true, false} {
		for _, kind := range []string{"html", "non-json-type", "1027-json"} {
			t.Run(fmt.Sprintf("probe=%t/%s", probe, kind), func(t *testing.T) {
				var calls atomic.Int32
				p, clock, store := r208Relay(t, r196RoundTrip(func(r *http.Request) (*http.Response, error) {
					n := calls.Add(1)
					if n > 1 || r.URL.Host == riotPlatformHost {
						return r206RelayResponse(200, []byte(`{"ok":true}`)), nil
					}
					response := r206RelayResponse(503, []byte(`<html>daily quota exhausted</html>`))
					switch kind {
					case "html":
						response.Header.Set("Content-Type", "text/html")
					case "non-json-type":
						response.Header.Set("Content-Type", "text/plain")
						response.Body = r206RelayResponse(200, []byte(`{}`)).Body
					case "1027-json":
						response.Body = r206RelayResponse(200, []byte(`{"code":1027}`)).Body
					}
					return response, nil
				}))
				var events []map[string]any
				var eventsMu sync.Mutex
				p.champions.diag = func(e map[string]any) {
					eventsMu.Lock()
					defer eventsMu.Unlock()
					events = append(events, e)
				}
				var out map[string]any
				var err error
				if probe {
					riotRelays.mu.Lock()
					riotRelays.active = ""
					riotRelays.lastSuccess = time.Time{}
					riotRelays.mu.Unlock()
					_, err = riotRelays.ensureForce(t.Context(), p.champions.httpClient(), p.champions.diag, false)
				} else {
					err = p.get(t.Context(), riotPlatformHost, "/lol/status/v4/platform-data", nil, &out)
				}
				if err == nil {
					t.Fatal("quota response must fail")
				}
				if !errors.Is(err, errRiotRelayQuotaExhausted) || riotErrorStatus(err) != 503 || riotHTTPErrorBody(err).Error != "战绩服务暂时不可用" || riotHTTPErrorBody(err).Kind != "quota_exhausted" {
					t.Fatalf("quota classification: %v %#v", err, riotHTTPErrorBody(err))
				}
				initial := calls.Load()
				clock.advance(29 * time.Second)
				for i := 0; i < 5; i++ {
					if err = p.get(t.Context(), riotClusterHost, "/riot/account/v1/accounts/by-puuid/another", nil, &out); !errors.Is(err, errRiotRelayQuotaExhausted) {
						t.Fatal(err)
					}
				}
				if calls.Load() != initial {
					t.Fatal("daily quota retried before UTC reset", calls.Load())
				}
				if !riotRelays.unavailable() {
					t.Fatal("UI must report unavailable")
				}
				found := false
				eventsMu.Lock()
				snapshot := append([]map[string]any(nil), events...)
				eventsMu.Unlock()
				for _, e := range snapshot {
					if e["event"] == "riot_relay_probe" && e["result"] == "quota_exhausted" {
						found = true
					}
				}
				if !found {
					t.Fatal("quota diagnostic missing", snapshot)
				}
				store.mu.Lock()
				err = store.writeLocked(r204FixtureKey)
				store.mu.Unlock()
				if err != nil {
					t.Fatal(err)
				}
				if err = p.get(t.Context(), riotPlatformHost, "/lol/status/v4/platform-data", nil, &out); err != nil {
					t.Fatal("saved key affected by relay quota", err)
				}
				store.mu.Lock()
				err = store.writeLocked("")
				store.mu.Unlock()
				if err != nil {
					t.Fatal(err)
				}
				clock.advance(time.Second)
				if err = p.get(t.Context(), riotPlatformHost, "/lol/status/v4/platform-data", nil, &out); err != nil {
					t.Fatal("UTC reset did not recover", err)
				}
			})
		}
	}
}

func TestR208RelayApplicationSharedAcrossProviders(t *testing.T) {
	var calls atomic.Int32
	transport := r196RoundTrip(func(*http.Request) (*http.Response, error) {
		if calls.Add(1) == 1 {
			r := r206RelayResponse(429, []byte(`{}`))
			r.Header.Set("Retry-After", "7")
			r.Header.Set("X-Relay-Cooldown", "application")
			return r, nil
		}
		return r206RelayResponse(200, []byte(`{}`)), nil
	})
	p, clock, _ := r208Relay(t, transport)
	var out map[string]any
	err := p.get(t.Context(), riotPlatformHost, "/lol/status/v4/platform-data", nil, &out)
	if body := riotHTTPErrorBody(err); body.CooldownScope != "application" || body.RetryAfter != 7 {
		t.Fatal(body)
	}
	other := newRiotProvider(p.champions)
	clock.advance(5 * time.Second)
	for _, host := range []string{riotPlatformHost} {
		err = other.get(t.Context(), host, "/riot/account/v1/accounts/by-puuid/other", nil, &out)
		if body := riotHTTPErrorBody(err); body.RetryAfter != 2 || body.CooldownScope != "application" {
			t.Fatal(body)
		}
	}
	if calls.Load() != 1 {
		t.Fatal("same-platform cooldown bypassed", calls.Load())
	}
	clock.advance(2 * time.Second)
	if err = other.get(t.Context(), riotClusterHost, "/riot/account/v1/accounts/by-puuid/other", nil, &out); err != nil || calls.Load() != 2 {
		t.Fatal("application cooldown not expired", err, calls.Load())
	}
}

func TestR208RelayDiskBeforeQuotaAndProbe(t *testing.T) {
	var calls atomic.Int32
	p, clock, _ := r208Relay(t, r196RoundTrip(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return nil, errors.New("disk cache must precede relay I/O")
	}))
	riotRelays.mu.Lock()
	riotRelays.quotaUntil = clock.now().Add(time.Minute)
	riotRelays.mu.Unlock()
	p.matchDisk = newPublicBinaryCache(&localStore{root: t.TempDir()}, "riot-matches", 100, 1<<20)
	var match riotMatch
	if err := json.Unmarshal([]byte(`{"metadata":{"matchId":"KR_208"},"info":{"gameId":208,"participants":[{"puuid":"subject","participantId":1}]}}`), &match); err != nil {
		t.Fatal(err)
	}
	p.persistRiotMatch("riot-match-v4|KR_208", &match)
	tracker := &riotOverviewCostTracker{}
	ctx := context.WithValue(t.Context(), riotOverviewCostTrackerKey{}, tracker)
	got, source, err := p.matchByIDWithCache(ctx, "KR_208")
	if err != nil || got == nil || source != "disk" || tracker.matchesFromDisk != 1 || calls.Load() != 0 {
		t.Fatal("relay skipped disk", source, err, calls.Load())
	}
}

func TestR208IPPartialResumeWithoutRepeatedSuccessfulDetails(t *testing.T) {
	var mu sync.Mutex
	calls := map[string]int{}
	successes := 0
	loaded := map[string]int{}
	limited := false
	p, clock, _ := r208Relay(t, r196RoundTrip(func(r *http.Request) (*http.Response, error) {
		mu.Lock()
		defer mu.Unlock()
		path := r.URL.Path
		calls[path]++
		switch {
		case path == "/health" || strings.HasSuffix(path, "/platform-data"):
			return r206RelayResponse(200, []byte(`{}`)), nil
		case strings.Contains(path, "/accounts/by-riot-id/"):
			return r206RelayResponse(200, []byte(`{"puuid":"subject","gameName":"Fixture","tagLine":"KR1"}`)), nil
		case strings.Contains(path, "/summoners/by-puuid/"):
			return r206RelayResponse(200, []byte(`{"puuid":"subject","summonerLevel":99}`)), nil
		case strings.HasSuffix(path, "/ids"):
			return r206RelayResponse(200, []byte(`["KR_2081","KR_2082","KR_2083","KR_2084","KR_2085","KR_2086"]`)), nil
		case strings.Contains(path, "/matches/KR_"):
			if successes == 4 && !limited {
				limited = true
				r := r206RelayResponse(429, []byte(`{}`))
				r.Header.Set("Retry-After", "60")
				return r, nil
			}
			successes++
			loaded[path] = 1
			id := path[strings.LastIndex(path, "/")+1:]
			return r206RelayResponse(200, []byte(fmt.Sprintf(`{"metadata":{"matchId":%q},"info":{"gameId":208,"queueId":420,"gameDuration":1800,"participants":[{"puuid":"subject","participantId":1,"teamId":100,"championId":1}]}}`, id))), nil
		default:
			return nil, fmt.Errorf("unexpected route category")
		}
	}))
	p.matchConcurrency = 1
	p.champions.championMeta = map[int]championMetadata{1: {NameZH: "黑暗之女"}}
	p.champions.cache = newChampionDataCache(&localStore{root: t.TempDir()})
	p.identityDisk = newRiotIdentityCache(p.champions)
	p.matchDisk = newRiotMatchDiskCache(p.champions)
	a := &app{riot: p}
	preview := 0
	ctx := context.WithValue(t.Context(), riotOverviewProgressKey{}, func(page gameplayOverview) { preview = max(preview, len(page.Matches)) })
	ref := gameplayReference{PlayerRef: "subject", GameName: "Fixture", TagLine: "KR1", Region: riotRegionKR, Privacy: "PRIVATE"}
	page, err := a.loadRiotOverview(ctx, ref, 1, 6)
	if err == nil || riotErrorStatus(err) != 429 || riotHTTPErrorBody(err).RetryAfter != 60 || riotHTTPErrorBody(err).CooldownScope != "ip" || preview != 4 || len(page.Matches) != 4 {
		t.Fatalf("partial not preserved: preview=%d matches=%d err=%v", preview, len(page.Matches), err)
	}
	initiallyLoaded := map[string]int{}
	for path := range loaded {
		initiallyLoaded[path] = 1
	}
	before := len(calls)
	clock.advance(59 * time.Second)
	if _, err = a.loadRiotOverview(ctx, ref, 1, 6); riotErrorStatus(err) != 429 || len(calls) != before {
		t.Fatal("retried before 60 seconds", err, calls)
	}
	clock.advance(time.Second)
	page, err = a.loadRiotOverview(ctx, ref, 1, 6)
	if err != nil || len(page.Matches) != 6 {
		t.Fatal("remaining matches not completed", len(page.Matches), err)
	}
	for path := range initiallyLoaded {
		if calls[path] != 1 {
			t.Fatal("successful match requested twice", path, calls[path])
		}
	}
}

func TestR208RelaySummaryTenMinuteWindow(t *testing.T) {
	c := &r208Clock{stamp: time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)}
	s := &riotRelayState{now: c.now}
	t.Cleanup(func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.summaryTimer != nil {
			s.summaryTimer.Stop()
			s.summaryTimer = nil
		}
	})
	var rows []map[string]any
	record := func(e map[string]any) { rows = append(rows, e) }
	s.recordRequest("", 200, "other", record)
	s.recordRequest("", 429, "other", record)
	s.recordRequest("network", 0, "other", record)
	s.recordRequest("quota_exhausted", 503, "other", record)
	c.advance(10*time.Minute - time.Millisecond)
	s.flushSummary(record)
	if len(rows) != 0 {
		t.Fatal("summary emitted too early")
	}
	c.advance(time.Millisecond)
	s.flushSummary(record)
	s.flushSummary(record)
	if len(rows) != 1 || rows[0]["requests"] != 4 || rows[0]["rate_limited"] != 1 {
		t.Fatal(rows)
	}
	failures := rows[0]["failures"].(map[string]any)
	if failures["network"].(map[string]int)["other"] != 1 || failures["quota_exhausted"] != 1 {
		t.Fatal(rows)
	}
	raw, _ := json.Marshal(rows)
	if strings.Contains(string(raw), "relay.example") || strings.Contains(string(raw), "puuid") {
		t.Fatal("identity in summary")
	}
}

func TestR208QuotaUTCAndConfiguredOrigin(t *testing.T) {
	clock := &r208Clock{stamp: time.Date(2026, 10, 5, 7, 59, 30, 0, time.FixedZone("CST", 8*3600))}
	for _, origin := range []string{"https://a.example", "https://b.example"} {
		s := &riotRelayState{config: "https://a.example\nhttps://b.example", now: clock.now}
		s.quotaExhausted(origin)
		if s.quotaUntil.Sub(clock.now()) != 30*time.Second {
			t.Fatal("reset must be UTC midnight for either configured origin", s.quotaUntil)
		}
	}
	s := &riotRelayState{config: "https://a.example\nhttps://b.example", now: clock.now}
	s.quotaExhausted("https://unconfigured.example")
	if !s.quotaUntil.IsZero() {
		t.Fatal("obsolete/unconfigured origin changed shared state")
	}
}

func TestR208QueuedRequestChecksSharedCooldownBeforeIO(t *testing.T) {
	var calls atomic.Int32
	p, _, _ := r208Relay(t, r196RoundTrip(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return r206RelayResponse(200, []byte(`{}`)), nil
	}))
	if _, err := p.relayOrigin(t.Context()); err != nil {
		t.Fatal(err)
	}
	stamp := time.Now()
	p.limitNow = func() time.Time { return stamp }
	for i := 0; i < 15; i++ {
		p.shortWindow = append(p.shortWindow, stamp)
	}
	p.limitSleep = func(context.Context, time.Duration) error {
		riotRelays.observeCooldown(riotClusterHost, "/riot/account/v1/accounts/by-puuid/queued", "application", 7, "kr")
		stamp = stamp.Add(time.Second)
		return nil
	}
	var out map[string]any
	err := p.get(t.Context(), riotClusterHost, "/riot/account/v1/accounts/by-puuid/queued", nil, &out)
	if err == nil || riotHTTPErrorBody(err).CooldownScope != "application" || calls.Load() != 0 {
		t.Fatal("queued request escaped cooldown", err, calls.Load())
	}
}
