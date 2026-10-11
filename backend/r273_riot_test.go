package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

type r273Probe struct {
	mu            sync.Mutex
	started       time.Time
	phases        *overviewPhaseTimings
	spans         map[string][][2]int64
	provider      *riotProvider
	initialState  map[string]any
	beforeState   map[string]any
	unknownPhases []string
}

var r273SpanNames = []string{"detail_worker", "detail_cache", "detail_cache_lock", "detail_store_lock", "detail_disk_write", "limiter_admission", "limiter_queue", "rate_wait", "foreground_wait", "profile_wait"}

func r273TimingContext(providers ...*riotProvider) (context.Context, *r273Probe) {
	started := time.Now()
	p := &r273Probe{started: started, phases: newOverviewPhaseTimings(started), spans: map[string][][2]int64{}}
	if len(providers) > 0 {
		p.provider = providers[0]
		p.initialState = r273ProviderState(p.provider)
	}
	for _, name := range r273SpanNames {
		p.spans[name] = [][2]int64{}
	}
	ctx := context.WithValue(context.Background(), overviewPhasesContextKey{}, p.phases)
	ctx = context.WithValue(ctx, riotTimingProbeKey{}, riotTimingProbe(func(phase string, start, end time.Time) {
		p.mu.Lock()
		defer p.mu.Unlock()
		if _, ok := p.spans[phase]; !ok {
			p.unknownPhases = append(p.unknownPhases, phase)
			return
		}
		offset := start.Sub(started).Microseconds()
		if end.IsZero() {
			p.spans[phase] = append(p.spans[phase], [2]int64{offset, -1})
			return
		}
		for i, row := range p.spans[phase] {
			if row[0] == offset && row[1] == -1 {
				p.spans[phase][i][1] = end.Sub(started).Microseconds()
				return
			}
		}
		p.unknownPhases = append(p.unknownPhases, "unpaired:"+phase)
	}))
	return ctx, p
}
func r273TimingJSON(f *r98Fixture, p *r273Probe, stage string, started time.Time) ([]byte, error) {
	p.mu.Lock()
	spans := map[string][][2]int64{}
	active := map[string]int{}
	snapshotNow := time.Now()
	for name, rows := range p.spans {
		spans[name] = append([][2]int64{}, rows...)
		for i, row := range spans[name] {
			if row[1] == -1 {
				spans[name][i][1] = snapshotNow.Sub(p.started).Microseconds()
				active[name]++
			}
		}
	}
	unknown := append([]string(nil), p.unknownPhases...)
	p.mu.Unlock()
	if len(unknown) > 0 {
		return nil, fmt.Errorf("unregistered timing phase: %v", unknown)
	}
	f.mu.Lock()
	requests := []map[string]any{}
	costs := []map[string]any{}
	for _, e := range f.events {
		if e["event"] == "riot_request" {
			requests = append(requests, r273SummaryFields(e, []string{"event", "route", "category", "entry", "status", "ttfb_ms", "total_ms", "bytes", "queued_ms", "attempt", "cancelled", "cancel_reason", "foreground"}))
		}
		if e["event"] == "riot_overview_cost" {
			costs = append(costs, r273SummaryFields(e, []string{"event", "cancel_reason", "duration_ms", "matches_requested", "matches_loaded", "matches_failed", "first_error_kind", "error_kind", "rate_limited_count", "limiter_queue_ms", "matches_from_disk", "matches_from_memory", "matches_from_network", "details_inflight_peak", "detail_concurrency"}))
		}
	}
	f.mu.Unlock()
	before := p.beforeState
	if before == nil {
		before = p.initialState
	}
	return json.Marshal(map[string]any{"stage": stage, "wall_us": time.Since(started).Microseconds(), "phases": p.phases.snapshot(time.Now()), "spans_us": spans, "active_spans": active, "requests": requests, "costs": costs, "state_initial": p.initialState, "state_before": before, "state_after": r273ProviderState(f.a.riot)})
}
func r273SummaryFields(event map[string]any, names []string) map[string]any {
	out := map[string]any{}
	for _, name := range names {
		if value, ok := event[name]; ok {
			out[name] = value
		}
	}
	return out
}
func r273LogTimings(t *testing.T, f *r98Fixture, p *r273Probe, stage string, started time.Time) {
	t.Helper()
	raw, err := r273TimingJSON(f, p, stage, started)
	if err != nil {
		t.Errorf("encode failure timing evidence: %v", err)
		return
	}
	if output := os.Getenv("R273_TIMING_OUTPUT"); output != "" {
		r273OutputMu.Lock()
		file, err := os.OpenFile(output, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
		if err == nil {
			_, err = file.Write(append(raw, '\n'))
			closeErr := file.Close()
			if err == nil {
				err = closeErr
			}
		}
		r273OutputMu.Unlock()
		if err != nil {
			t.Errorf("write explicit timing evidence: %v", err)
		}
	}
	t.Cleanup(func() {
		if t.Failed() {
			t.Log("R273_FAILURE_TIMING " + string(raw))
		}
	})
}

// Repeat only the cold foreground measurement here; the original five-player
// full test still exercises background completion, cache reuse and all budgets.
func TestR273ColdKRSegmentTiming(t *testing.T) {
	for i := 0; i < 5; i++ {
		t.Run(string(rune('0'+i)), func(t *testing.T) {
			f := r98OverviewFixture(t, true)
			ref := gameplayReference{GameName: "Fixture", TagLine: "KR1", Region: "kr"}
			ctx, p := r273TimingContext(f.a.riot)
			started := time.Now()
			first, err := f.a.loadRiotOverview(ctx, ref, 0, 10)
			firstElapsed := time.Since(started)
			r273LogTimings(t, f, p, "cold-first-ten", started)
			if err != nil || len(first.Matches) != 10 || firstElapsed > 3*time.Second {
				t.Fatal("cold first10 budget", len(first.Matches), firstElapsed, err)
			}
		})
	}
}

func TestR273FreshProvidersDoNotShareForegroundOrLimitState(t *testing.T) {
	first := r98OverviewFixture(t, false)
	next := r98OverviewFixture(t, false)
	a, b := first.a.riot, next.a.riot
	if a == b || a.foreground == b.foreground || a.puuidState == b.puuidState || a.matchDisk == b.matchDisk || a.identityDisk == b.identityDisk {
		t.Fatal("cold fixtures share provider or mutable cache/gate state")
	}
	a.foreground.active = 1
	a.foreground.resumeAt = time.Now().Add(time.Hour)
	a.shortWindow = make([]time.Time, 15)
	a.longWindow = make([]time.Time, 90)
	for i := range a.shortWindow {
		a.shortWindow[i] = time.Now()
	}
	for i := range a.longWindow {
		a.longWindow[i] = time.Now()
	}
	a.matchFlights = map[string]*riotMatchFlight{"KR_1": {done: make(chan struct{})}}
	ctx, probe := r273TimingContext(next.a.riot)
	started := time.Now()
	out, err := next.a.loadRiotOverview(ctx, gameplayReference{GameName: "Fixture", TagLine: "KR1", Region: "kr"}, 0, 10)
	r273LogTimings(t, next, probe, "independent-provider", started)
	if err != nil || len(out.Matches) != 10 {
		t.Fatal("previous provider state blocked new foreground", len(out.Matches), err)
	}
	if b.foreground.active != 0 || len(b.limitQueue) != 0 || len(b.matchFlights) != 0 {
		t.Fatal("completed foreground left shared in-flight state")
	}
}

func TestR273ForegroundDoesNotUseBackgroundResumeWindow(t *testing.T) {
	p := newRiotProvider(nil)
	p.foreground.resumeAt = time.Now().Add(time.Hour)
	ctx, probe := r273TimingContext(p)
	bounded, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	if err := p.wait(withRiotSingleWaitLimit(bounded, 5*time.Second)); err != nil {
		t.Fatal("foreground waited for background resume", err)
	}
	if len(probe.spans["foreground_wait"]) != 0 || len(p.shortWindow) != 1 {
		t.Fatal("foreground entered background resume gate", probe.spans)
	}
}

func TestR273ForegroundOvertakesBackgroundResumeFlight(t *testing.T) {
	for _, kind := range []string{"account", "summoner", "ids", "detail", "client-identity"} {
		t.Run(kind, func(t *testing.T) {
			f := r98OverviewFixture(t, false)
			p := f.a.riot
			fixedNow := time.Unix(273, 0)
			p.foreground.now = func() time.Time { return fixedNow }
			ctx, probe := r273TimingContext(p)
			started := time.Now()
			defer r273LogTimings(t, f, probe, "controlled-overlap", started)
			finished := beginRiotForeground(p)
			finished()
			if p.foreground.resumeAt != fixedNow.Add(5*time.Second) {
				t.Fatal("previous foreground did not leave resume window")
			}
			entered := make(chan struct{})
			var once sync.Once
			bgCtx, bgProbe := r273TimingContext(p)
			defer r273LogTimings(t, f, bgProbe, "controlled-background", started)
			bg, cancel := context.WithCancel(context.WithValue(withRiotBackground(bgCtx), riotForegroundWaitKey{}, func(ctx context.Context, delay time.Duration) error {
				if delay <= 0 {
					t.Error("background did not enter resume wait")
				}
				once.Do(func() { close(entered) })
				<-ctx.Done()
				return ctx.Err()
			}))
			defer cancel()
			bgResult := make(chan error, 1)
			ref := gameplayReference{GameName: "Fixture", TagLine: "KR1", Region: "kr"}
			go func() {
				var err error
				switch kind {
				case "account":
					_, err = p.accountByRiotID(bg, "Fixture", "KR1")
				case "summoner":
					_, err = p.summonerByPUUID(bg, "r98-subject")
				case "ids":
					_, err = p.matchIDs(bg, "r98-subject", 0, 10)
				case "detail":
					_, _, err = p.matchByIDWithCache(bg, "KR_1")
				case "client-identity":
					_, err = f.a.resolveClientRiotPUUID(bg, nil, "kr", "anonymous-lcu", ref)
				}
				bgResult <- err
			}()
			select {
			case <-entered:
			case <-time.After(2 * time.Second):
				t.Fatal("background never reached controlled resume gate")
			}
			probe.beforeState = r273ProviderState(p)
			foreground, stop := context.WithTimeout(ctx, 2*time.Second)
			defer stop()
			if kind == "client-identity" {
				public, err := f.a.resolveClientRiotPUUID(foreground, nil, "kr", "anonymous-lcu", ref)
				if err != nil {
					t.Fatal("foreground client identity waited for background gate", err)
				}
				ref.PlayerRef = public
			}
			out, err := f.a.loadRiotOverview(foreground, ref, 0, 10)
			if err != nil || len(out.Matches) != 10 {
				t.Fatal("foreground joined background resume-window flight", len(out.Matches), err)
			}
			select {
			case err := <-bgResult:
				if err != nil {
					t.Fatal("background caller lost shared result after handoff", err)
				}
			case <-foreground.Done():
				t.Fatal("background did not receive foreground shared result")
			}
			f.mu.Lock()
			defer f.mu.Unlock()
			if f.calls["detail"] != 10 {
				t.Fatal("controlled handoff duplicated detail HTTP", f.calls["detail"])
			}
		})
	}
}

var r273OutputMu sync.Mutex

func r273ProviderState(p *riotProvider) map[string]any {
	if p == nil {
		return nil
	}
	state := map[string]any{}
	p.foreground.Lock()
	state["foreground_active"] = p.foreground.active
	state["resume_remaining_ms"] = max(int64(0), p.foreground.resumeAt.Sub(p.foreground.nowLocked()).Milliseconds())
	p.foreground.Unlock()
	p.limitMu.Lock()
	state["limiter_waiters"] = len(p.limitQueue)
	state["short_window_tokens"] = len(p.shortWindow)
	state["long_window_tokens"] = len(p.longWindow)
	state["rate_scopes"] = len(p.rateHosts)
	p.limitMu.Unlock()
	p.cacheMu.Lock()
	state["match_flights"] = len(p.matchFlights)
	state["match_cache"] = len(p.matchCache)
	state["match_generation"] = p.matchGeneration
	p.cacheMu.Unlock()
	p.accountMu.Lock()
	state["account_flights"] = len(p.accountFlights)
	state["account_cache"] = len(p.accountCache)
	p.accountMu.Unlock()
	if p.identityDisk != nil {
		p.identityDisk.mu.Lock()
		state["identity_flights"] = len(p.identityDisk.flights)
		state["identity_cache"] = len(p.identityDisk.entries)
		p.identityDisk.mu.Unlock()
	}
	count := 0
	clientHistoryFlights.Range(func(_, _ any) bool { count++; return true })
	state["process_client_history_flights"] = count
	return state
}

func TestR273FailureTimingSchemaAndPrivacy(t *testing.T) {
	f := r98OverviewFixture(t, false)
	ctx, probe := r273TimingContext(f.a.riot)
	started := time.Now()
	out, err := f.a.loadRiotOverview(ctx, gameplayReference{GameName: "Fixture", TagLine: "KR1", Region: "kr"}, 0, 10)
	if err != nil || len(out.Matches) != 10 {
		t.Fatal("diagnostic fixture", err)
	}
	f.mu.Lock()
	for _, e := range f.events {
		e["player_name"] = "R273_SECRET_PLAYER"
		e["puuid"] = "R273_SECRET_PUUID"
		e["match_id"] = "R273_SECRET_MATCH"
		e["key"] = "R273_SECRET_KEY"
		e["body"] = "R273_SECRET_BODY"
	}
	f.mu.Unlock()
	raw, err := r273TimingJSON(f, probe, "schema-check", started)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"R273_SECRET_", "r98-subject", "KR_1", "Fixture", "anonymous-lcu"} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("timing payload leaked identifier")
		}
	}
	var data map[string]json.RawMessage
	if err = json.Unmarshal(raw, &data); err != nil {
		t.Fatal(err)
	}
	var spans map[string][][2]int64
	if err = json.Unmarshal(data["spans_us"], &spans); err != nil {
		t.Fatal(err)
	}
	for _, name := range r273SpanNames {
		if _, ok := spans[name]; !ok {
			t.Fatalf("missing registered span %s", name)
		}
	}
	for _, name := range []string{"detail_worker", "detail_cache", "limiter_admission", "limiter_queue", "profile_wait"} {
		if len(spans[name]) == 0 {
			t.Fatalf("span never observed: %s", name)
		}
	}
	if len(spans["detail_worker"]) != 10 {
		t.Fatal("missing per-detail timing", len(spans["detail_worker"]))
	}
	var phases map[string]json.RawMessage
	if err = json.Unmarshal(data["phases"], &phases); err != nil {
		t.Fatal(err)
	}
	var phaseSpans map[string][][2]int64
	if err = json.Unmarshal(phases["spans"], &phaseSpans); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"account", "summoner", "matchIDs", "details"} {
		if len(phaseSpans[name]) == 0 {
			t.Fatalf("overview phase never observed: %s", name)
		}
	}
	for _, name := range []string{"state_initial", "state_before", "state_after"} {
		if string(data[name]) == "null" || len(data[name]) == 0 {
			t.Fatal("missing provider snapshot", name)
		}
	}
	done := startRiotTimingProbe(ctx, "typo-unregistered")
	done()
	if _, err = r273TimingJSON(f, probe, "invalid-schema", started); err == nil {
		t.Fatal("unknown probe phase silently accepted")
	}
}
