package main

import (
	"bufio"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestR269CredentialScopesAndPUUIDRecovery(t *testing.T) {
	f := r98OverviewFixture(t, false)
	p := f.a.riot
	var accounts, mismatches atomic.Int32
	old := p.champions.client.Transport
	p.champions.client.Transport = gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if strings.Contains(r.URL.Path, "/by-riot-id/") {
			accounts.Add(1)
		}
		if strings.Contains(r.URL.Path, "/by-puuid/foreign-key-token") {
			mismatches.Add(1)
			return &http.Response{StatusCode: 400, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"message":"Exception decrypting"}`))}, nil
		}
		return old.RoundTrip(r)
	})
	id := riotLookupIdentity{"Fixture", "KR1", "foreign-key-token", riotCredentialScope(context.Background())}
	ctx := context.WithValue(context.Background(), riotLookupIdentityKey{}, id)
	stale := riotAccount{PUUID: id.puuid, GameName: id.name, TagLine: id.tag}
	if err := p.cachedPublicIdentity(ctx, "account:fixture#kr1", time.Hour, &stale, func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	p.accountMu.Lock()
	p.accountCache[riotAccountMemoryKey(ctx, id.name, id.tag)] = riotAccountCacheEntry{account: riotAccount{PUUID: id.puuid}, expiresAt: time.Now().Add(time.Hour)}
	p.accountMu.Unlock()
	for i := 0; i < 2; i++ {
		var ids []string
		if err := p.getScopedPUUID(ctx, p.clusterHost(), "/lol/match/v5/matches/by-puuid/foreign-key-token/ids", url.Values{"count": {"5"}}, &ids, riotResponseMax); err != nil {
			t.Fatal(err)
		}
		if len(ids) == 0 {
			t.Fatal("history absent")
		}
	}
	if accounts.Load() != 1 || mismatches.Load() != 2 {
		t.Fatalf("accounts=%d mismatch=%d", accounts.Load(), mismatches.Load())
	}
	direct := p.identityContextKey(ctx, "account:fixture#kr1")
	relay := p.identityContextKey(context.WithValue(ctx, riotRouteKey{}, "relay"), "account:fixture#kr1")
	if direct == relay {
		t.Fatal("credential scopes conflated")
	}
}

func TestR269PaginationIsDeltaAndTimeIsPushedDown(t *testing.T) {
	f := r98OverviewFixture(t, false)
	old := f.a.riot.champions.client.Transport
	var withTime atomic.Bool
	f.a.riot.champions.client.Transport = gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "/ids") {
			withTime.Store(r.URL.Query().Get("startTime") == "1700000000" && r.URL.Query().Get("endTime") == "1800000000")
		}
		return old.RoundTrip(r)
	})
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/gameplay/overview", strings.NewReader(`{"gameName":"Fixture","tagLine":"KR1","region":"kr","begIndex":20,"count":20,"startTime":1700000000,"endTime":1800000000}`))
	r.Header.Set("Accept", "application/x-ndjson")
	f.a.handleGameplayOverview(w, r)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"ranks", "masteries", "seasonOverall", "seasonChampionStats", "seasonStatsProgress", "overall", "championStats", "rankedQueues", "activityHours"} {
		if _, ok := payload[name]; ok {
			t.Fatal("first-page field repeated:", name)
		}
	}
	var matches []json.RawMessage
	_ = json.Unmarshal(payload["matches"], &matches)
	if len(matches) != 20 {
		t.Fatalf("need actual 20-match delta, got %d", len(matches))
	}
	if w.Body.Len() > 600*1024 {
		t.Fatal("oversized page", w.Body.Len())
	}
	if !withTime.Load() {
		t.Fatal("time condition not passed to Riot IDs")
	}
	full, err := f.a.loadRiotOverview(context.Background(), gameplayReference{GameName: "Fixture", TagLine: "KR1", Region: "kr"}, 20, 20)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(full)
	fields := map[string]json.RawMessage{}
	_ = json.Unmarshal(before, &fields)
	sizes := map[string]any{}
	for name, value := range fields {
		sizes[name] = map[string]any{"before_bytes": len(value), "delta_bytes": len(payload[name]), "repeated_in_delta": payload[name] != nil}
	}
	report := map[string]any{"fixture": "same official synthetic 20-match page; historical exported log does not contain response bodies and cannot establish 7–8MB field attribution", "before_response_bytes": len(before), "after_response_bytes": w.Body.Len(), "matches": len(matches), "fields": sizes}
	raw, _ := json.MarshalIndent(report, "", "  ")
	if path := os.Getenv("R269_PAGE_BYTES_REPORT"); path != "" {
		if err := os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("before_page_bytes=%d delta_page_bytes=%d matches=%d", len(before), w.Body.Len(), len(matches))
}

func TestR269RankFailureIsNotUnranked(t *testing.T) {
	f := r98OverviewFixture(t, false)
	p := f.a.riot
	old := p.champions.client.Transport
	p.champions.client.Transport = gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if strings.Contains(r.URL.Path, "/league/") {
			return &http.Response{StatusCode: 400, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"message":"Bad request"}`))}, nil
		}
		return old.RoundTrip(r)
	})
	ranks, capability := p.loadRiotRanks(context.Background(), "r98-subject")
	if capability.State != capabilityFailed || len(ranks) != 0 {
		t.Fatal(ranks, capability)
	}
}

func TestR269MayhemDiskTTLAndNoRecordCache(t *testing.T) {
	for _, body := range []string{`{"code":200,"data":{"rating":2001}}`, `{"code":400}`} {
		var calls atomic.Int32
		transport := aramkitRoundTripFunc(func(*http.Request) (*http.Response, error) { calls.Add(1); return aramkitJSONResponse(200, body), nil })
		now := time.Now()
		c := newAramkitTestClient(transport)
		c.now = func() time.Time { return now }
		c.disk = newPublicBinaryCache(&localStore{root: t.TempDir()}, "mayhem", 128, 4<<20)
		first := c.lookup(context.Background(), "Fixture", "CN")
		if !first.Available && first.UnavailableReason != "未收录" {
			t.Fatal(first)
		}
		restarted := newAramkitTestClient(transport)
		restarted.disk = c.disk
		restarted.now = c.now
		if first.Available {
			now = now.Add(5 * time.Hour)
		} else {
			now = now.Add(59 * time.Minute)
		}
		result := restarted.lookup(context.Background(), "Fixture", "CN")
		if calls.Load() != 1 || result.Available != first.Available {
			t.Fatal("disk cache missed", calls.Load(), result)
		}
		now = now.Add(2 * time.Hour)
		restarted.lookup(context.Background(), "Fixture", "CN")
		if calls.Load() != 2 {
			t.Fatal("expired cache reused")
		}
	}
}

func TestR269SGPBackoffAndEarlyProbe(t *testing.T) {
	p := newSGPProvider()
	client := &LCUClient{}
	p.token, p.tokenAt, p.tokenClient = "fixture", time.Now(), client
	now := time.Now()
	p.historyClock = func() time.Time { return now }
	var waits []time.Duration
	p.retryWait = func(_ context.Context, d time.Duration) error { waits = append(waits, d); return nil }
	calls := 0
	healthy := false
	p.http = &http.Client{Transport: sgpRoundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		if healthy || calls == 3 {
			return aramkitJSONResponse(200, `{"ok":true}`), nil
		}
		return aramkitJSONResponse(503, `{}`), nil
	})}
	var out map[string]any
	if err := p.getJSON(context.Background(), client, "HN1", "SUMMARY", "history", "https://fixture/history", &out); err != nil {
		t.Fatal(err)
	}
	if calls != 3 || len(waits) != 2 || waits[0] != 1500*time.Millisecond || waits[1] != 3*time.Second {
		t.Fatal(calls, waits)
	}
	if !p.historyCircuits["HN1"].until.IsZero() {
		t.Fatal("intermittent failures opened circuit")
	}
	calls = 10
	waits = nil
	if err := p.getJSON(context.Background(), client, "HN1", "SUMMARY", "history", "https://fixture/history", &out); err == nil {
		t.Fatal("outage missing")
	}
	if p.historyCircuits["HN1"].until.IsZero() {
		t.Fatal("continuous failures did not open circuit")
	}
	healthy = true
	now = now.Add(15 * time.Second)
	if err := p.getJSON(context.Background(), client, "HN1", "SUMMARY", "history", "https://fixture/history?count=1", &out); err != nil {
		t.Fatal("early probe failed", err)
	}
	if !p.historyCircuits["HN1"].until.IsZero() {
		t.Fatal("probe did not recover")
	}
}

func TestR269RequestCommonFieldsAndWindowCompaction(t *testing.T) {
	a := r175App(t)
	for i := 0; i < 100; i++ {
		a.recordDiagnostic(map[string]any{"event": "sgp_history_circuit", "state": "cooldown", "route": "SUMMARY", "server_id": "HN1"})
	}
	if len(r175Events(t, a, "sgp_history_circuit")) > 2 {
		t.Fatal("cooldown spam")
	}
	a.recordDiagnostic(map[string]any{"event": "riot_request", "route": "direct", "category": "match", "status": 200, "bytes": 200000, "ttfb_ms": 1000, "total_ms": 1200, "attempt": 1, "queued_ms": 0, "cancelled": false})
	rows := r175Events(t, a, "riot_request")
	row := rows[len(rows)-1]
	if _, err := time.Parse(time.RFC3339Nano, row["time"].(string)); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"log_seq", "run_id", "build_fingerprint"} {
		if _, exists := row[k]; !exists {
			t.Fatal("common field absent", k)
		}
	}
	raw, _ := json.Marshal(row)
	if len(raw) > 300 {
		t.Fatal("large diagnostic", len(raw))
	}
}

func TestR269TimelineBatchHasOneSummary(t *testing.T) {
	a := r175App(t)
	ctx := context.WithValue(context.Background(), timelineDiagnosticBatchKey{}, "synthetic-overview")
	for i := 0; i < 20; i++ {
		a.recordTimelineDiagnostic(ctx, map[string]any{"event": "match_timeline_failed", "source": "sgp", "reason": "http", "frames": i})
		a.recordTimelineDiagnostic(ctx, map[string]any{"event": "match_timeline_lcu_incomplete", "source": "lcu", "frames": i})
	}
	a.flushTimelineDiagnosticBatch("synthetic-overview")
	rows := r175Events(t, a, "match_timeline_summary")
	if len(rows) != 1 {
		t.Fatal(len(rows))
	}
	samples := rows[0]["samples"].([]any)
	if len(samples) > 3 {
		t.Fatal(samples)
	}
	if len(r175Events(t, a, "match_timeline_failed")) != 0 {
		t.Fatal("per-match failure logs")
	}
}

// Optional user-log replay uses the production compactor, window merge and JSONL encoder.
func TestR269DiagnosticReplay(t *testing.T) {
	input := os.Getenv("R269_REPLAY_INPUT")
	if input == "" {
		t.Skip("user-log replay input not supplied")
	}
	file, err := os.Open(input)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()
	type row struct {
		at    time.Time
		event map[string]any
		bytes int
	}
	rows := []row{}
	runs := map[string]bool{}
	scanner := bufio.NewScanner(gz)
	scanner.Buffer(make([]byte, 65536), 16<<20)
	for scanner.Scan() {
		var e map[string]any
		if json.Unmarshal(scanner.Bytes(), &e) != nil {
			continue
		}
		var at time.Time
		switch v := e["time"].(type) {
		case string:
			at, _ = time.Parse(time.RFC3339Nano, v)
		case float64:
			at = time.UnixMilli(int64(v))
		}
		if at.IsZero() {
			continue
		}
		rows = append(rows, row{at, e, len(scanner.Bytes()) + 1})
		if e["event"] == "app_start" && e["version"] == "0.12.81" {
			runs[fmt.Sprint(e["run_id"])] = true
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	var lo, hi time.Time
	for _, r := range rows {
		if runs[fmt.Sprint(r.event["run_id"])] {
			if lo.IsZero() || r.at.Before(lo) {
				lo = r.at
			}
			if r.at.After(hi) {
				hi = r.at
			}
		}
	}
	selected := []row{}
	for _, r := range rows {
		if runs[fmt.Sprint(r.event["run_id"])] || r.event["run_id"] == nil && r.event["event"] == "riot_request" && !r.at.Before(lo) && !r.at.After(hi) {
			selected = append(selected, r)
		}
	}
	sort.SliceStable(selected, func(i, j int) bool { return selected[i].at.Before(selected[j].at) })
	a := &app{}
	before, after, sequence := 0, 0, 0
	lcu := map[string][]map[string]any{}
	riot := map[string]map[string]any{}
	timeline := map[string]bool{}
	by := map[string]int{}
	oldFingerprint := buildFingerprint
	buildFingerprint = "0123456789ab"
	defer func() { buildFingerprint = oldFingerprint }()
	emit := func(e map[string]any, at time.Time) {
		sequence++
		e["log_seq"] = sequence
		if e["run_id"] == nil {
			e["run_id"] = "000000000000000000000000"
		}
		raw, err := marshalDiagnosticRecord(e, at)
		if err != nil {
			t.Fatal(err)
		}
		after += len(raw) + 1
		by[fmt.Sprint(e["event"])] += len(raw) + 1
	}
	for _, r := range selected {
		before += r.bytes
		e := r.event
		name := fmt.Sprint(e["event"])
		batch := fmt.Sprint(e["run_id"]) + ":" + fmt.Sprint(r.at.Unix()/60)
		switch name {
		case "lcu_request":
			key := fmt.Sprint(e["run_id"]) + ":" + fmt.Sprint(e["window_start"])
			e["count"] = intNumber(e["count"])
			d, _ := e["duration_ms"].(map[string]any)
			e["duration_ms"] = map[string]int64{"max": int64(intNumber(d["max"]))}
			lcu[key] = append(lcu[key], e)
			continue
		case "riot_match_items", "riot_match_mode":
			b := riot[batch]
			if b == nil {
				b = map[string]any{"event": "riot_matches_summary", "counts": map[string]int{}, "max_items": 0}
				riot[batch] = b
			}
			b["counts"].(map[string]int)[name]++
			b["max_items"] = max(intNumber(b["max_items"]), intNumber(e["items"]))
			continue
		case "match_timeline_lcu_incomplete", "match_timeline_failed", "match_timeline_client":
			for _, k := range []string{"frames", "events"} {
				e[k] = intNumber(e[k])
			}
			a.recordTimelineDiagnostic(context.WithValue(context.Background(), timelineDiagnosticBatchKey{}, batch), e)
			timeline[batch] = true
			continue
		}
		for _, k := range []string{"time", "log_seq", "build_fingerprint", "run_id"} {
			delete(e, k)
		}
		if compact, keep := a.compactDiagnostic(e, r.at); keep {
			emit(compact, r.at)
		}
	}
	for _, rows := range lcu {
		for _, e := range lcuRequestDiagnosticWindows(rows) {
			emit(e, hi)
		}
	}
	for _, e := range riot {
		emit(e, hi)
	}
	for batch := range timeline {
		b := a.timelineDiagnosticBatches[batch]
		if b != nil {
			emit(map[string]any{"event": "match_timeline_summary", "counts": b.Counts, "max_frames": b.MaxFrames, "max_events": b.MaxEvents, "samples": b.Samples}, hi)
		}
	}
	hours := hi.Sub(lo).Hours()
	report := map[string]any{"scope": "0.12.81 run IDs plus legacy Riot rows within UTC window; production JSONL encoder/compactor and LCU merger; missing legacy overview IDs conservatively grouped per run/minute", "estimate_only": true, "duration_hours": hours, "window_start": lo, "window_end": hi, "events": len(selected), "before_bytes": before, "after_bytes": after, "before_bytes_per_hour": float64(before) / hours, "after_bytes_per_hour": float64(after) / hours, "after_by_event": by}
	raw, _ := json.MarshalIndent(report, "", "  ")
	if output := os.Getenv("R269_REPLAY_OUTPUT"); output != "" {
		if err := os.WriteFile(output, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("replay events=%d before=%d after=%d after_bytes_per_hour=%.0f", len(selected), before, after, float64(after)/hours)
	if hours <= 0 || float64(after)/hours > 2.5*1024*1024 {
		t.Fatal("replay log budget exceeded")
	}
}
func intNumber(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	}
	return 0
}

func TestR269AccountFallbackResolvesInRelayScope(t *testing.T) {
	_, p, _ := r266Embedded(t)
	var direct, relay atomic.Int32
	p.champions.client = &http.Client{Transport: r196RoundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "relay.example" {
			relay.Add(1)
			return r206RelayResponse(200, []byte(`{"puuid":"relay-key-subject","gameName":"Fixture","tagLine":"KR1"}`)), nil
		}
		direct.Add(1)
		return r206RelayResponse(401, []byte(`{"status":{"message":"expired"}}`)), nil
	})}
	account, err := p.accountByRiotID(context.Background(), "Fixture", "KR1")
	if err != nil || account.scope != "relay" || account.PUUID != "relay-key-subject" || direct.Load() != 1 || relay.Load() != 1 {
		t.Fatal(account, err, direct.Load(), relay.Load())
	}
	if _, exists := p.accountCache["fixture\x1fkr1|credential:embedded"]; exists {
		t.Fatal("relay account leaked into embedded cache")
	}
}

func TestR269DecryptRecoveryKeepsReturnedCredentialScope(t *testing.T) {
	for _, accountStatus := range []int{200, 429} {
		t.Run(fmt.Sprint(accountStatus), func(t *testing.T) {
			_, p, _ := r266Embedded(t)
			var directAccounts, relayAccounts, wrongRelay, wrongDirect atomic.Int32
			p.champions.client = &http.Client{Transport: r196RoundTrip(func(r *http.Request) (*http.Response, error) {
				relay := r.URL.Host == "relay.example"
				if strings.Contains(r.URL.Path, "/by-riot-id/") {
					if relay {
						relayAccounts.Add(1)
						return r206RelayResponse(200, []byte(`{"puuid":"relay-key-subject","gameName":"Fixture","tagLine":"KR1"}`)), nil
					}
					directAccounts.Add(1)
					return r206RelayResponse(accountStatus, []byte(`{"puuid":"direct-key-subject","gameName":"Fixture","tagLine":"KR1"}`)), nil
				}
				if relay {
					if !strings.Contains(r.URL.Path, "/by-puuid/relay-key-subject/") {
						wrongRelay.Add(1)
						return r206RelayResponse(400, []byte(`{"message":"Exception decrypting"}`)), nil
					}
					return r206RelayResponse(200, []byte(`["KR_2691"]`)), nil
				}
				if strings.Contains(r.URL.Path, "relay-key-subject") {
					wrongDirect.Add(1)
				}
				return r206RelayResponse(400, []byte(`{"message":"Exception decrypting"}`)), nil
			})}
			ctx := context.WithValue(context.Background(), riotLookupIdentityKey{}, riotLookupIdentity{"Fixture", "KR1", "foreign-key-token", "embedded"})
			for i := 0; i < 2; i++ {
				var ids []string
				if err := p.get(ctx, p.clusterHost(), "/lol/match/v5/matches/by-puuid/foreign-key-token/ids", nil, &ids); err != nil || len(ids) != 1 {
					t.Fatal(ids, err)
				}
			}
			if directAccounts.Load() != 1 || relayAccounts.Load() != 1 || wrongRelay.Load() != 0 || wrongDirect.Load() != 0 {
				t.Fatal(directAccounts.Load(), relayAccounts.Load(), wrongRelay.Load(), wrongDirect.Load())
			}
		})
	}
}
func TestR269MayhemCachedSuccessRefreshesWithoutBlocking(t *testing.T) {
	started, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	c := newAramkitTestClient(aramkitRoundTripFunc(func(*http.Request) (*http.Response, error) {
		if calls.Add(1) == 2 {
			close(started)
			<-release
			defer close(done)
		}
		return aramkitJSONResponse(200, `{"code":200,"data":{"rating":2001}}`), nil
	}))
	c.refreshes = map[string]bool{}
	first := c.lookup(context.Background(), "Fixture", "CN")
	if !first.Available {
		t.Fatal(first)
	}
	at := time.Now()
	cached := c.lookup(context.Background(), "Fixture", "CN")
	if !cached.Available || time.Since(at) > 200*time.Millisecond {
		t.Fatal("cached success blocked")
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("silent refresh absent")
	}
	close(release)
	<-done
}

func TestR269DiagnosticMessageAndUserScope(t *testing.T) {
	a := r175App(t)
	body := `{"event":"browser_error_client","reason":"error","errorType":"TypeError","scriptName":"gameplay.js","line":51,"column":3,"message":"Cannot read Fixture#KR1 /Users/synthetic-user/private https://example.test/file?token=secret","stack":["at /Users/synthetic-user/gameplay.js:51:3","at runtime.js:22:4","at app.js:30:2","at fourth.js:4:1"],"repeatCount":40}`
	w := httptest.NewRecorder()
	a.handleClientDiagnostic(w, httptest.NewRequest("POST", "/api/diagnostics/client", strings.NewReader(body)))
	if w.Code != 204 {
		t.Fatal(w.Code, w.Body.String())
	}
	rows := r175Events(t, a, "browser_error_client")
	raw, _ := json.Marshal(rows)
	if strings.Contains(string(raw), "synthetic-user") || strings.Contains(string(raw), "token=secret") || strings.Contains(string(raw), "Fixture#KR1") {
		t.Fatal("unsafe diagnostic")
	}
	if len(rows) != 1 || len(rows[0]["stack"].([]any)) != 3 || rows[0]["repeat_count"] != float64(40) {
		t.Fatal(rows)
	}
	frames := safeBrowserFrames([]string{"at https://example.test/gameplay.js?token=secret:51:3", "at runtime.js:22:4", "at app.js:30:2", "at fourth.js:4:1"})
	if strings.Join(frames, ",") != "gameplay.js:51:3,runtime.js:22:4,app.js:30:2" {
		t.Fatal(frames)
	}
	s, p, _ := r266Embedded(t)
	s.mu.Lock()
	s.key = r266FakeKey
	s.mu.Unlock()
	scope := riotCredentialScope(context.Background())
	if !strings.HasPrefix(scope, "user:") || len(scope) != 13 || strings.Contains(scope, "RGAPI") {
		t.Fatal("unsafe credential scope")
	}
	direct := p.identityContextKey(context.Background(), "account:fixture#kr1")
	relay := p.identityContextKey(context.WithValue(context.Background(), riotRouteKey{}, "relay"), "account:fixture#kr1")
	if direct == relay {
		t.Fatal("identity scopes alias")
	}
}

func TestR269IdentityPinsKeyDuringLookup(t *testing.T) {
	s, p, _ := r266Embedded(t)
	s.mu.Lock()
	s.key = r266FakeKey
	s.mu.Unlock()
	ctx := riotPinnedIdentityContext(context.Background())
	scope := riotCredentialScope(ctx)
	s.mu.Lock()
	s.key = r266FakeKey + "-next"
	s.mu.Unlock()
	p.champions.client = &http.Client{Transport: r196RoundTrip(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("X-Riot-Token") != r266FakeKey {
			t.Fatal("in-flight lookup changed credential")
		}
		return r206RelayResponse(200, []byte(`{"puuid":"pinned-key-subject","gameName":"Fixture","tagLine":"KR1"}`)), nil
	})}
	account, err := p.accountByRiotID(ctx, "Fixture", "KR1")
	if err != nil || account.scope != scope || riotCredentialScope(context.Background()) == scope {
		t.Fatal(account, err, scope)
	}
	if _, ok := p.accountCache[riotAccountMemoryKey(ctx, "Fixture", "KR1")]; !ok {
		t.Fatal("result not cached in pinned scope")
	}
}
