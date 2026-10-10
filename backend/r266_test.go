package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const r266FakeKey = "RGAPI-00000000-0000-0000-0000-000000000000"

func r266Embedded(t *testing.T) (*riotKeyStore, *riotProvider, *[]map[string]any) {
	s := r206RelayFixture(t)
	riotEmbeddedKey = func() string { return r266FakeKey }
	events := []map[string]any{}
	c := newChampionProvider()
	c.diag = func(e map[string]any) { events = append(events, e) }
	return s, newRiotProvider(c), &events
}
func TestR266EmbeddedSourceAndBackgroundRelay(t *testing.T) {
	s, p, events := r266Embedded(t)
	direct, relay := 0, 0
	p.champions.client = &http.Client{Transport: r196RoundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "relay.example" {
			relay++
			if r.Header.Get("X-Riot-Token") != "" {
				t.Fatal("background token leaked")
			}
		} else {
			direct++
			if r.Header.Get("X-Riot-Token") != r266FakeKey {
				t.Fatal("direct credential missing")
			}
		}
		return r206RelayResponse(200, []byte(`{"ok":true}`)), nil
	})}
	var out any
	if riotKeySource() != "embedded" {
		t.Fatal(riotKeySource())
	}
	if err := p.get(context.Background(), p.platformHost(), "/lol/status/v4/platform-data", nil, &out); err != nil {
		t.Fatal(err)
	}
	if err := p.get(withRiotBackground(context.Background()), p.platformHost(), "/lol/status/v4/platform-data", nil, &out); err != nil {
		t.Fatal(err)
	}
	if direct != 1 || relay != 1 {
		t.Fatalf("routes direct=%d relay=%d", direct, relay)
	}
	s.key = "user-fixture"
	if riotKeySource() != "user" {
		t.Fatal("user priority")
	}
	t.Setenv("RIOT_API_KEY", "env-fixture")
	if riotKeySource() != "env" {
		t.Fatal("env priority")
	}
	raw, _ := json.Marshal(events)
	if strings.Contains(string(raw), r266FakeKey) || strings.Contains(string(raw), "RGAPI-") {
		t.Fatal("diagnostic credential leak")
	}
}
func TestR266EmbeddedNetworkCircuitAndProbe(t *testing.T) {
	s, p, _ := r266Embedded(t)
	direct, relay := 0, 0
	recoverDirect := false
	p.champions.client = &http.Client{Transport: r196RoundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "relay.example" {
			relay++
		} else {
			direct++
			if !recoverDirect {
				return nil, errors.New("synthetic network error")
			}
		}
		return r206RelayResponse(200, []byte(`{"ok":true}`)), nil
	})}
	var out any
	for i := 0; i < 4; i++ {
		if err := p.get(context.Background(), p.platformHost(), "/lol/status/v4/platform-data", nil, &out); err != nil {
			t.Fatal(err)
		}
	}
	if direct != 3 || relay != 4 {
		t.Fatalf("breaker requests direct=%d relay=%d", direct, relay)
	}
	s.mu.Lock()
	s.routeUntil = time.Now().Add(-time.Second)
	s.mu.Unlock()
	recoverDirect = true
	if err := p.get(context.Background(), p.platformHost(), "/lol/status/v4/platform-data", nil, &out); err != nil {
		t.Fatal(err)
	}
	if direct != 5 || relay != 4 {
		t.Fatal("light probe did not recover")
	}
}
func TestR266EmbeddedRejectedAndQuota(t *testing.T) {
	for _, status := range []int{401, 403, 429, 200} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			s, p, events := r266Embedded(t)
			direct, relay := 0, 0
			p.champions.client = &http.Client{Transport: r196RoundTrip(func(r *http.Request) (*http.Response, error) {
				code := 200
				if r.URL.Host == "relay.example" {
					relay++
				} else {
					direct++
					code = status
				}
				resp := r206RelayResponse(code, []byte(`{"ok":true}`))
				if code == 429 {
					resp.Header.Set("Retry-After", "2")
				}
				if code == 200 && r.URL.Host != "relay.example" {
					resp.Header.Set("X-App-Rate-Limit-Count", "81:120")
				}
				return resp, nil
			})}
			var out any
			for i := 0; i < 2; i++ {
				if err := p.get(context.Background(), p.platformHost(), "/lol/status/v4/platform-data", nil, &out); err != nil {
					t.Fatal(err)
				}
			}
			if direct != 1 || relay < 1 {
				t.Fatalf("guard failed direct=%d relay=%d", direct, relay)
			}
			if (status == 401 || status == 403) && !s.embeddedRejected {
				t.Fatal("rejected embedded key remained active")
			}
			raw, _ := json.Marshal(events)
			if (status == 401 || status == 403) && !strings.Contains(string(raw), "riot_key_rejected") {
				t.Fatal("rejection event missing")
			}
		})
	}
}

func TestR266RelayAttemptTimeoutIsDiagnosed(t *testing.T) {
	r206RelayFixture(t)
	c := newChampionProvider()
	var event map[string]any
	c.diag = func(e map[string]any) {
		if e["event"] == "riot_request" {
			event = e
		}
	}
	c.client = &http.Client{Transport: r196RoundTrip(func(r *http.Request) (*http.Response, error) {
		<-r.Context().Done()
		return nil, r.Context().Err()
	})}
	p := newRiotProvider(c)
	m := newRiotRelayEntryManager(riotRelayEntriesConfig{Mode: "fixed", Preferred: "A", Entries: []riotRelayEntry{{Label: "A", Hostname: "relay.example"}}}, 4)
	var headers atomic.Bool
	r := p.candidateAttempt(context.Background(), m, m.entry("A"), p.clusterHost(), "/lol/match/v5/matches/by-puuid/fixture/ids", nil, 1<<20, &headers, nil, nil, func() {})
	if r.err == nil || event["cancelled"] != true || event["cancel_reason"] != "timeout" {
		t.Fatalf("attempt timeout diagnostic: err=%v event=%v", r.err, event)
	}
}
func TestR266EmbeddedFallbackIsOneAdditionalRequest(t *testing.T) {
	_, p, _ := r266Embedded(t)
	direct, relay := 0, 0
	p.champions.client = &http.Client{Transport: r196RoundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "relay.example" {
			relay++
		} else {
			direct++
		}
		return nil, errors.New("synthetic connection failure")
	})}
	var out any
	if err := p.get(context.Background(), p.clusterHost(), "/lol/match/v5/matches/by-puuid/fixture/ids", nil, &out); err == nil || direct != 1 || relay != 1 {
		t.Fatalf("fallback requests direct=%d relay=%d", direct, relay)
	}
}
func TestR266EmbeddedMultiEntryFallbackIsOneAdditionalRequest(t *testing.T) {
	for _, response := range []string{"network", "429"} {
		t.Run(response, func(t *testing.T) {
			_, p, _ := r266Embedded(t)
			p.champions.diag = nil
			t.Setenv("DEEP_LEGENDS_RELAY_ENTRIES", `{"mode":"auto","preferred":"A","entries":[{"label":"A","hostname":"relay-a.example"},{"label":"B1","hostname":"relay-b.example"}]}`)
			m := riotRelays.entries(4)
			if m == nil {
				t.Fatal("multi-entry fixture rejected")
			}
			for i := 0; i < 4; i++ {
				m.beginGET()
			}
			var direct, relay atomic.Int32
			p.champions.client = &http.Client{Transport: r196RoundTrip(func(r *http.Request) (*http.Response, error) {
				if strings.HasPrefix(r.URL.Host, "relay-") {
					relay.Add(1)
					if r.Header.Get("X-Riot-Token") != "" {
						t.Error("fallback token leaked")
					}
					if response == "429" {
						r := r206RelayResponse(429, []byte(`{}`))
						r.Header.Set("Retry-After", "1")
						return r, nil
					}
				} else {
					direct.Add(1)
				}
				return nil, errors.New("synthetic connection failure")
			})}
			var out any
			if err := p.get(t.Context(), p.clusterHost(), "/lol/match/v5/matches/by-puuid/fixture/ids", nil, &out); err == nil || direct.Load() != 1 || relay.Load() != 1 {
				t.Fatalf("multi-entry fallback requests direct=%d relay=%d error=%v", direct.Load(), relay.Load(), err)
			}
		})
	}
}

func TestR266EmbeddedHTTPDateRetryAfter(t *testing.T) {
	s, p, _ := r266Embedded(t)
	now := time.Now()
	p.champions.client = &http.Client{Transport: r196RoundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "relay.example" {
			return r206RelayResponse(200, []byte(`{}`)), nil
		}
		response := r206RelayResponse(429, []byte(`{}`))
		response.Header.Set("Retry-After", now.Add(time.Minute).UTC().Format(http.TimeFormat))
		return response, nil
	})}
	var out any
	if err := p.get(t.Context(), p.platformHost(), "/lol/status/v4/platform-data", nil, &out); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	delay := s.routeUntil.Sub(now)
	s.mu.Unlock()
	if delay < 58*time.Second || delay > 61*time.Second {
		t.Fatalf("HTTP-date cooldown ignored: %s", delay)
	}
}
func TestR266ForegroundResumesAfterFiveSeconds(t *testing.T) {
	_, p, _ := r266Embedded(t)
	done := beginRiotForeground(p)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if p.waitForRiotForeground(ctx) == nil {
		t.Fatal("background not paused")
	}
	ended := time.Now()
	done()
	if delay := time.Until(p.foreground.resumeAt); delay < 4900*time.Millisecond || delay > 5*time.Second {
		t.Fatal(delay)
	}
	ctx2, cancel2 := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel2()
	if err := p.waitForRiotForeground(ctx2); err != nil {
		t.Fatal(err)
	}
	if time.Since(ended) < 5*time.Second {
		t.Fatal("resumed early")
	}
}
func TestR266PartialSummaryAndBoundedScoreDiagnostics(t *testing.T) {
	r206RelayFixture(t)
	events := []map[string]any{}
	riotRelays.recordBusinessRequest("", 200, "ids", func(e map[string]any) { events = append(events, e) }, "A", time.Now(), time.Millisecond)
	riotRelays.flushSummary(func(e map[string]any) { events = append(events, e) }, true)
	if len(events) != 1 || events[0]["partial"] != true {
		t.Fatal(events)
	}
	a := &app{storage: newFlowDiagnosticStore(t)}
	ctx, b := newMatchScoreBatch(context.Background())
	for i := 1; i <= 30; i++ {
		m := r216Game(r216Person(1, 100), r216Person(2, 200))
		m.GameID = int64(i)
		applyMatchScores(&m)
		a.recordMatchScores("riot", m, ctx)
	}
	a.finishMatchScoreBatch(b)
	raw, _ := a.storage.readDiagnosticLog()
	if strings.Count(string(raw), "match_score_computed") != 1 || !strings.Contains(string(raw), `"matches":30`) {
		t.Fatal(string(raw))
	}
	line, err := marshalDiagnosticRecord(map[string]any{"event": "huge", "rows": strings.Repeat("x", 20000)}, time.Now())
	if err != nil || len(line) > 4096 || !strings.Contains(string(line), `"truncated":true`) {
		t.Fatalf("line size=%d err=%v", len(line), err)
	}
	oversized := map[string]any{"event": strings.Repeat("e", 20000)}
	for i := 0; i < 100; i++ {
		oversized[strings.Repeat("k", 200)+string(rune(1000+i))] = i
	}
	line, err = marshalDiagnosticRecord(oversized, time.Now())
	var summary map[string]any
	if err != nil || len(line) > 4096 || json.Unmarshal(line, &summary) != nil || summary["truncated"] != true || summary["original_bytes"].(float64) < 40000 {
		t.Fatalf("minimal summary must retain original size within hard cap: bytes=%d err=%v", len(line), err)
	}
}
func TestR266IdentitylessMaterialDoesNotRetry(t *testing.T) {
	a := AccountData{Loot: []LootItem{{Blank: true, DataPending: true, StoreItemID: -1, LootID: "MATERIAL_-1"}}}
	if collectionDataPending(a) {
		t.Fatal("empty material retry")
	}
	kinds, samples, _ := collectionBlankDiagnostics(a)
	if len(kinds) != 0 || len(samples) != 0 {
		t.Fatal("empty material counted")
	}
	a.Loot[0].StoreItemID = 123
	a.Loot[0].LootID = "MATERIAL_123"
	if !collectionDataPending(a) {
		t.Fatal("real pending identity was suppressed")
	}
}

func TestR266DeferredColdCardIsNotForegroundLatency(t *testing.T) {
	a := r175App(t)
	now := time.Now()
	a.observeColdLaunchDiscovery(LCUDiscoveryStatus{ProcessCount: 1, ProcessAt: now, AttemptAt: now}, now)
	a.observeColdLaunchMilestone("self_tab_header_ms", now.Add(100*time.Millisecond))
	a.observeFirstMatchesCard("network", now.Add(time.Second), false)
	events := r175Events(t, a, "client_cold_launch_timeline")
	e := events[len(events)-1]
	if e["overview_visible_at_ready"] != false || e["matches_card_ms"] != float64(-1) || e["matches_card_deferred_ms"] != float64(1000) {
		t.Fatal(e)
	}
}
func TestR266CompactRequestAndSpecialEventSizes(t *testing.T) {
	var event map[string]any
	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(errors.New("superseded"))
	riotRequestEvent(func(e map[string]any) { event = e }, ctx, "direct", "summoner", "direct", 429, 24500*time.Millisecond, 24500*time.Millisecond, 24500*time.Millisecond, 8388608, 3)
	event["run_id"] = "fixture-run"
	event["log_seq"] = 123456
	line, err := marshalDiagnosticRecord(event, time.Now())
	if err != nil || len(line) > 300 {
		t.Fatalf("request bytes=%d %s", len(line), line)
	}
	rows := make([]any, 100)
	for i := range rows {
		rows[i] = map[string]any{"value": i}
	}
	for _, name := range []string{"refresh_succeeded", "objective_badge_state", "r99_read_probe"} {
		line, err = marshalDiagnosticRecord(map[string]any{"event": name, "rows": rows}, time.Now())
		if err != nil || len(line) > 2048 || !strings.Contains(string(line), `"count":100`) {
			t.Fatalf("%s bytes=%d", name, len(line))
		}
	}
}
func TestR266ProVisibilityAndAccountInterval(t *testing.T) {
	_, p, _ := r266Embedded(t)
	a := &app{riot: p}
	w := httptest.NewRecorder()
	a.handleProVisibility(w, httptest.NewRequest("POST", "/api/pro-players/visibility", strings.NewReader(`{"visible":false}`)))
	if w.Code != 204 || !p.proDirectoryHidden() {
		t.Fatal("directory visibility ignored")
	}
	ctx, cancel := context.WithTimeout(a.proBackgroundContext(context.Background()), 20*time.Millisecond)
	defer cancel()
	if waitProBackground(ctx) == nil {
		t.Fatal("hidden supplement was admitted")
	}
	now := time.Now()
	if !a.admitProAccountRefresh("fixture", now) || a.admitProAccountRefresh("fixture", now.Add(9*time.Minute)) || !a.admitProAccountRefresh("fixture", now.Add(10*time.Minute)) {
		t.Fatal("10 minute admission")
	}
}
func TestR266ProRiotIDResolutionAndRetriesJoinFlight(t *testing.T) {
	f := r98OverviewFixture(t, false)
	f.a.overviewQueries = newOverviewQueryCache()
	var foreignRequests atomic.Int32
	var accountLookupNanos atomic.Int64
	originalTransport := f.a.riot.champions.client.Transport
	f.a.riot.champions.client.Transport = gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if strings.Contains(r.URL.Path, "/by-riot-id/") {
			started := time.Now()
			response, err := originalTransport.RoundTrip(r)
			accountLookupNanos.Add(time.Since(started).Nanoseconds())
			return response, err
		}
		if strings.Contains(r.URL.Path, "/by-puuid/opgg-encrypted-subject") {
			foreignRequests.Add(1)
			return &http.Response{StatusCode: 400, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"message":"Exception decrypting"}`))}, nil
		}
		return originalTransport.RoundTrip(r)
	})
	f.a.proPlayers.teams = []opggProTeam{{ID: 371, Members: []opggProMember{{TeamID: 371, Nickname: "Rookie", RealName: "Song Eui-jin", Position: "middle", Authority: "PROGAMER", Summoners: []opggProAccount{{PUUID: "opgg-encrypted-subject", GameName: "Fixture", TagLine: "KR1", Region: "kr"}}}}}}
	ref := gameplayReference{GameName: "Fixture", TagLine: "KR1", Region: "kr", Privacy: "PRIVATE"}
	recorder := httptest.NewRecorder()
	f.a.handleGameplayOverview(recorder, httptest.NewRequest("POST", "/api/gameplay/overview", strings.NewReader(`{"gameName":"Fixture","tagLine":"KR1","region":"kr","count":5}`)))
	if recorder.Code != 200 {
		t.Fatalf("pro endpoint status=%d %s", recorder.Code, recorder.Body.String())
	}
	f.mu.Lock()
	accountCalls := f.calls["account"]
	f.mu.Unlock()
	if accountCalls != 1 || foreignRequests.Load() != 0 {
		t.Fatal("pro endpoint must resolve using this key")
	}
	t.Logf("official_account_requests_before=0 after=%d additional_lookup_ms=%.3f (synthetic server, not real Riot latency)", accountCalls, float64(accountLookupNanos.Load())/float64(time.Millisecond))
	ref.PlayerRef = "r98-subject"
	started, release := make(chan struct{}), make(chan struct{})
	old := f.a.riot.champions.client.Transport
	var once sync.Once
	f.a.riot.champions.client.Transport = gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "/ids") {
			once.Do(func() { close(started) })
			select {
			case <-release:
			case <-r.Context().Done():
				return nil, r.Context().Err()
			}
		}
		return old.RoundTrip(r)
	})
	errs := make(chan error, 2)
	go func() { _, e := f.a.loadRiotOverviewDeduplicated(context.Background(), ref, 0, 1, true); errs <- e }()
	<-started
	go func() { _, e := f.a.loadRiotOverviewDeduplicated(context.Background(), ref, 0, 1, true); errs <- e }()
	time.Sleep(20 * time.Millisecond)
	close(release)
	for i := 0; i < 2; i++ {
		if e := <-errs; e != nil {
			t.Fatal(e)
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.calls["account"] != 1 || f.calls["matchIDs"] != 2 || f.calls["detail"] != 5 {
		t.Fatal(f.calls)
	}
}
func TestR266IDsTimeoutHasBoundedRetry(t *testing.T) {
	f := r98OverviewFixture(t, false)
	calls := 0
	old := f.a.riot.champions.client.Transport
	f.a.riot.champions.client.Transport = gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "/ids") {
			calls++
			<-r.Context().Done()
			return nil, r.Context().Err()
		}
		return old.RoundTrip(r)
	})
	started := time.Now()
	_, err := f.a.loadRiotOverview(context.Background(), gameplayReference{PlayerRef: "r98-subject", Region: "kr"}, 20, 10)
	if err == nil || time.Since(started) > 24500*time.Millisecond || calls != 2 {
		t.Fatalf("timeout=%v calls=%d elapsed=%v", err, calls, time.Since(started))
	}
}
