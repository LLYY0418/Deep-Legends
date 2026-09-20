package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type r98Fixture struct {
	a             *app
	mu            sync.Mutex
	calls         map[string]int
	events        []map[string]any
	delay         bool
	summonerGate  <-chan struct{}
	detailStarted chan struct{}
	detailOnce    sync.Once
}

func r98OverviewFixture(t *testing.T, delay bool) *r98Fixture {
	t.Helper()
	t.Setenv("RIOT_API_KEY", "RGAPI-fixture")
	f := &r98Fixture{calls: map[string]int{}, delay: delay, detailStarted: make(chan struct{})}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		kind := "detail"
		switch {
		case strings.Contains(path, "/accounts/"):
			kind = "account"
		case strings.Contains(path, "/summoner/"):
			kind = "summoner"
		case strings.HasSuffix(path, "/ids"):
			kind = "matchIDs"
		case strings.Contains(path, "/league/"):
			kind = "ranks"
		case strings.Contains(path, "/champion-mastery/"):
			kind = "mastery"
		}
		f.mu.Lock()
		f.calls[kind]++
		f.mu.Unlock()
		if kind == "summoner" && f.summonerGate != nil {
			select {
			case <-f.summonerGate:
			case <-r.Context().Done():
				return
			}
		}
		if kind == "detail" {
			f.detailOnce.Do(func() { close(f.detailStarted) })
		}
		if f.delay {
			d := map[string]time.Duration{"account": 40, "summoner": 450, "matchIDs": 90, "ranks": 80, "mastery": 80, "detail": 30}[kind]
			time.Sleep(d * time.Millisecond)
		}
		w.Header().Set("Content-Type", "application/json")
		switch kind {
		case "account":
			fmt.Fprint(w, `{"puuid":"r98-subject","gameName":"Fixture","tagLine":"KR1"}`)
		case "summoner":
			fmt.Fprint(w, `{"puuid":"r98-subject","profileIconId":1,"summonerLevel":760}`)
		case "matchIDs":
			start, _ := strconv.Atoi(r.URL.Query().Get("start"))
			count, _ := strconv.Atoi(r.URL.Query().Get("count"))
			ids := make([]string, count)
			for i := range ids {
				ids[i] = fmt.Sprintf("KR_%d", start+i+1)
			}
			_ = json.NewEncoder(w).Encode(ids)
		case "detail":
			id, _ := strconv.Atoi(path[strings.LastIndex(path, "_")+1:])
			fmt.Fprintf(w, `{"metadata":{"matchId":"KR_%d"},"info":{"gameId":%d,"queueId":420,"gameDuration":1800,"gameCreation":1700000000000,"participants":[{"puuid":"r98-subject","participantId":1,"teamId":100,"championId":1,"win":true,"kills":2,"deaths":3,"assists":4}]}}`, id, id)
		default:
			fmt.Fprint(w, `[]`)
		}
	}))
	t.Cleanup(server.Close)
	target, _ := url.Parse(server.URL)
	cp := newChampionProvider()
	cp.cache = newChampionDataCache(&localStore{root: t.TempDir()})
	cp.championMeta = map[int]championMetadata{1: {NameZH: "安妮"}}
	transport := server.Client().Transport
	cp.client = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		clone := r.Clone(r.Context())
		clone.URL.Scheme = target.Scheme
		clone.URL.Host = target.Host
		return transport.RoundTrip(clone)
	})}
	cp.diag = func(e map[string]any) { f.mu.Lock(); defer f.mu.Unlock(); f.events = append(f.events, e) }
	f.a = &app{riot: newRiotProvider(cp)}
	return f
}
func TestR98Timing(t *testing.T) {
	f := r98OverviewFixture(t, true)
	results := []map[string]any{}
	for _, scenario := range []string{"cold-search", "repeat-refresh"} {
		started := time.Now()
		phases := newOverviewPhaseTimings(started)
		ctx := context.WithValue(context.Background(), overviewPhasesContextKey{}, phases)
		previews := []map[string]any{}
		ctx = context.WithValue(ctx, riotOverviewProgressKey{}, func(p gameplayOverview) {
			previews = append(previews, map[string]any{"at_ms": time.Since(started).Milliseconds(), "matches": len(p.Matches)})
		})
		overview, err := f.a.loadRiotOverview(ctx, gameplayReference{GameName: "Fixture", TagLine: "KR1", Region: "kr", Privacy: "PRIVATE"}, 0, 20)
		if err != nil || len(overview.Matches) != 20 {
			t.Fatal(err, len(overview.Matches))
		}
		record := map[string]any{"scenario": scenario, "wall_ms": time.Since(started).Milliseconds(), "overview_phases_ms": phases.snapshot(time.Now()), "previews": previews}
		f.mu.Lock()
		for _, event := range f.events {
			if event["event"] == "riot_overview_cost" {
				record["riot_overview_cost"] = event
			}
		}
		calls := map[string]int{}
		for k, v := range f.calls {
			calls[k] = v
		}
		record["cumulative_http_calls"] = calls
		f.mu.Unlock()
		results = append(results, record)
	}
	raw, _ := json.MarshalIndent(results, "", "  ")
	t.Log(string(raw))
	if output := os.Getenv("R98_TIMING_OUTPUT"); output != "" {
		if err := os.WriteFile(output, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestR98OverviewEndpointCachesAndKeys(t *testing.T) {
	f := r98OverviewFixture(t, false)
	p := f.a.riot
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		if _, err := p.matchIDs(ctx, "subject", 0, 20); err != nil {
			t.Fatal(err)
		}
		if _, err := p.leagueEntries(ctx, "subject"); err != nil {
			t.Fatal(err)
		}
		if _, err := p.topMasteries(ctx, "subject", 5); err != nil {
			t.Fatal(err)
		}
	}
	for _, kind := range []string{"matchIDs", "ranks", "mastery"} {
		if f.calls[kind] != 1 {
			t.Fatalf("%s not cached: %d calls", kind, f.calls[kind])
		}
	}
	_, _ = p.matchIDs(ctx, "subject", 20, 20)
	_, _ = p.matchIDs(ctx, "subject", 0, 5)
	_, _ = p.matchIDsFiltered(ctx, "subject", 0, 20, 420, "")
	_, _ = p.matchIDsFiltered(ctx, "subject", 0, 20, 420, "ranked")
	_, _ = p.matchIDs(ctx, "other", 0, 20)
	_, _ = p.topMasteries(ctx, "subject", 10)
	_, _ = p.topMasteries(ctx, "other", 5)
	_, _ = p.leagueEntries(ctx, "other")
	if f.calls["matchIDs"] != 6 || f.calls["mastery"] != 3 || f.calls["ranks"] != 2 {
		t.Fatal("cache keys aliased pages/count/queue/type/players", f.calls)
	}
	// Validate actual envelopes and expire both layers; TTL is not just a constant.
	expected := map[string]time.Duration{"matchIDs": time.Minute, "ranks": 3 * time.Minute, "mastery": 30 * time.Minute}
	for identity, want := range map[string]time.Duration{"matchIDs:subject|count=20&start=0": expected["matchIDs"], "ranks:subject": expected["ranks"], "mastery:subject|count=5": expected["mastery"]} {
		hash := sha256.Sum256([]byte(identity))
		key := "riot-identity-v1|" + hex.EncodeToString(hash[:])
		cache := p.identityDisk
		cache.mu.Lock()
		entry, ok := cache.entries[key]
		if !ok {
			cache.mu.Unlock()
			t.Fatal("missing cache", identity)
		}
		if entry.ExpiresAt.Sub(entry.FetchedAt) != want {
			cache.mu.Unlock()
			t.Fatal("wrong TTL", identity, entry.ExpiresAt.Sub(entry.FetchedAt))
		}
		entry.ExpiresAt = time.Now().Add(-time.Second)
		entry.StaleUntil = entry.ExpiresAt
		cache.entries[key] = entry
		cache.mu.Unlock()
		if err := cache.writeDisk(entry); err != nil {
			t.Fatal(err)
		}
	}
	_, _ = p.matchIDs(ctx, "subject", 0, 20)
	_, _ = p.leagueEntries(ctx, "subject")
	_, _ = p.topMasteries(ctx, "subject", 5)
	if f.calls["matchIDs"] != 7 || f.calls["ranks"] != 3 || f.calls["mastery"] != 4 {
		t.Fatal("expired cache did not reload", f.calls)
	}
}

func TestR98SummonerDoesNotGateProgressAndPreviewKeepsAdvancing(t *testing.T) {
	f := r98OverviewFixture(t, true)
	f.a.riot.matchConcurrency = 1
	gate := make(chan struct{})
	f.summonerGate = gate
	var closeGate sync.Once
	defer closeGate.Do(func() { close(gate) })
	previews := make(chan gameplayOverview, 20)
	ctx := context.WithValue(context.Background(), riotOverviewProgressKey{}, func(p gameplayOverview) { previews <- p })
	done := make(chan error, 1)
	go func() {
		out, err := f.a.loadRiotOverview(ctx, gameplayReference{PlayerRef: "r98-subject", Region: "kr", Privacy: "PRIVATE"}, 0, 8)
		if err == nil && out.Player.SummonerLevel != 760 {
			err = fmt.Errorf("final profile missing")
		}
		done <- err
	}()
	select {
	case partial := <-previews:
		if len(partial.Matches) < 5 || len(partial.Matches) >= 8 {
			t.Fatal("bad first preview", len(partial.Matches))
		}
	case <-time.After(2 * time.Second):
		closeGate.Do(func() { close(gate) })
		<-done
		t.Fatal("summoner blocked details/preview")
	}
	closeGate.Do(func() { close(gate) })
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	counts := []int{}
	for len(previews) > 0 {
		counts = append(counts, len((<-previews).Matches))
	}
	if len(counts) < 2 || counts[len(counts)-1] != 8 {
		t.Fatal("one-shot preview latch remains", counts)
	}
}

func TestR98OverviewQueueBudgetReturnsRateLimit(t *testing.T) {
	f := r98OverviewFixture(t, false)
	p := f.a.riot
	p.longWindow = make([]time.Time, 90)
	for i := range p.longWindow {
		p.longWindow[i] = time.Now()
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := time.Now()
	done := make(chan error, 1)
	go func() {
		_, err := f.a.loadRiotOverview(ctx, gameplayReference{PlayerRef: "r98-subject", Region: "kr", Privacy: "PRIVATE"}, 0, 20)
		done <- err
	}()
	select {
	case err := <-done:
		var statusError *riotStatusError
		if !errors.As(err, &statusError) || riotErrorStatus(err) != 429 || statusError.retryAfter < 1 {
			t.Fatal("missing quota recovery", err)
		}
		if time.Since(started) > 5*time.Second {
			t.Fatal("overview exceeded queue budget")
		}
	case <-time.After(1500 * time.Millisecond):
		cancel()
		<-done
		t.Fatal("overview queue escape missing; stuck waiting for quota")
	}
	if len(f.calls) != 0 {
		t.Fatal("quota exhaustion still reached HTTP", f.calls)
	}
}

func TestR98LimiterFIFOHasOnlyOneSleepingHeadAndCanceledWaitersUseNoQuota(t *testing.T) {
	p := newRiotProvider(newChampionProvider())
	now := time.Now()
	p.limitNow = func() time.Time { return now }
	p.longWindow = make([]time.Time, 90)
	for i := range p.longWindow {
		p.longWindow[i] = now
	}
	release := make(chan struct{})
	var active, peak atomic.Int32
	p.limitSleep = func(ctx context.Context, _ time.Duration) error {
		n := active.Add(1)
		defer active.Add(-1)
		for old := peak.Load(); n > old && !peak.CompareAndSwap(old, n); old = peak.Load() {
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-release:
			p.limitMu.Lock()
			now = now.Add(2 * time.Minute)
			p.limitMu.Unlock()
			return nil
		}
	}
	const count = 24
	cancels := make([]context.CancelFunc, count)
	done := make(chan error, count)
	for i := 0; i < count; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		cancels[i] = cancel
		defer cancel()
		go func() { done <- p.wait(ctx) }()
		limit := time.Now().Add(time.Second)
		for {
			p.limitMu.Lock()
			n := len(p.limitQueue)
			p.limitMu.Unlock()
			if n == i+1 {
				break
			}
			if time.Now().After(limit) {
				t.Fatal("waiter missing")
			}
			time.Sleep(time.Millisecond)
		}
	}
	p.limitMu.Lock()
	queue := append([]*riotLimitWaiter(nil), p.limitQueue...)
	p.limitMu.Unlock()
	for i, waiter := range queue {
		select {
		case <-waiter.turn:
			if i != 0 {
				t.Fatal("follower woke with head")
			}
		default:
			if i == 0 {
				t.Fatal("head asleep before quota")
			}
		}
	}
	cancels[10]()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal("cancel failed", err)
	}
	if active.Load() != 1 || peak.Load() != 1 {
		t.Fatal("quota expiry thundering herd", active.Load(), peak.Load())
	}
	p.limitMu.Lock()
	tokens := len(p.longWindow)
	p.limitMu.Unlock()
	if tokens != 90 {
		t.Fatal("canceled waiter reserved quota")
	}
	close(release)
	for i := 1; i < count; i++ {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
	if peak.Load() != 1 {
		t.Fatal("more than one head sleeping")
	}
	p.limitMu.Lock()
	remaining := len(p.limitQueue)
	p.limitMu.Unlock()
	if remaining != 0 {
		t.Fatal("queue leak", remaining)
	}
}

func TestR98ChovyNullRegionFlightToOverview(t *testing.T) {
	var source []opggProTeam
	for _, team := range proRoster {
		raw := opggProTeam{ID: team.OPGGID, Name: team.Name, ShortName: team.Code, Members: []opggProMember{}}
		for _, player := range team.Players {
			member := proFixtureMember(team.OPGGID, player.Name, player.Names[0])
			for i := 0; i < 8; i++ {
				account := proFixtureAccount(fmt.Sprintf("R98%s%d", player.Name, i), fmt.Sprintf("fixture-private-%s-%d", player.Name, i), "", 0, 0)
				if player.Name == "Chovy" && i == 0 {
					account.GameName = "허거덩"
					account.TagLine = "0303"
				}
				member.Summoners = append(member.Summoners, account)
			}
			raw.Members = append(raw.Members, member)
		}
		source = append(source, raw)
	}
	teams, err := parseOPGGProPlayers(proR97RegionHTML(t, nil, true, source...))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.MkdirAll(root+"/logs", 0700); err != nil {
		t.Fatal(err)
	}
	a := &app{storage: trackTestStore(t, &localStore{root: root})}
	a.proPlayers.teams = teams
	if got := a.proIdentitySnapshot().candidates; got != 264 {
		t.Fatal("null-region directory pool collapsed", got)
	}
	response := gameplayOverview{Player: gameplayPlayer{PlayerRef: "fixture-private-Chovy-0", GameName: "허거덩", TagLine: "0303", Region: "kr", SummonerLevel: 760}}
	a.publicizeOverviewReferences(&response)
	if response.Player.ProPlayer == nil || response.Player.ProPlayer.PlayerName != "Chovy" || response.Player.ProPlayer.TeamCode != "GEN" {
		t.Fatal("Chovy did not match", response.Player.ProPlayer)
	}
	events := r90Events(t, a, "pro_identity_match")
	if len(events) != 1 || events[0]["matched_by"] != "puuid" || events[0]["candidates"] != float64(264) {
		t.Fatal("not exact stable-ID path", events)
	}
	encoded, _ := json.MarshalIndent(response, "", "  ")
	if strings.Contains(string(encoded), "fixture-private-") {
		t.Fatal("private PUUID leaked")
	}
	if output := os.Getenv("R98_CHOVY_OUTPUT"); output != "" {
		if err := os.WriteFile(output, encoded, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if !isNoisyDiagnosticEvent("pro_identity_match") {
		t.Fatal("per-participant event not sampled")
	}
}

// A real upstream 429 must keep its status/retry hint when the local queue
// budget interrupts Retry-After, while specialist callers keep their sentinel.
func TestR98Upstream429QueueBudgetKeepsTypedError(t *testing.T) {
	t.Setenv("RIOT_API_KEY", "RGAPI-fixture")
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()
	target, _ := url.Parse(server.URL)
	cp := newChampionProvider()
	transport := server.Client().Transport
	cp.client = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		clone := r.Clone(r.Context())
		clone.URL.Scheme, clone.URL.Host = target.Scheme, target.Host
		return transport.RoundTrip(clone)
	})}
	p := newRiotProvider(cp)
	started := time.Now()
	var out any
	err := p.get(withRiotQueueLimit(context.Background(), 30*time.Millisecond), riotClusterHost, "/r98-quota", nil, &out)
	var status *riotStatusError
	if !errors.Is(err, errThrottled) || !errors.As(err, &status) || status.status != 429 || status.retryAfter != 30 {
		t.Fatalf("lost throttle classification/retry: %v", err)
	}
	if calls.Load() != 1 || time.Since(started) > time.Second {
		t.Fatalf("queue escape retried or stalled: calls=%d elapsed=%v", calls.Load(), time.Since(started))
	}
}
