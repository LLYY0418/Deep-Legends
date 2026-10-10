package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func r265TLSRelays(t *testing.T, mode string, delay time.Duration) (*riotProvider, *riotRelayEntryManager, *atomic.Int32, *atomic.Int32, <-chan struct{}) {
	t.Helper()
	r206RelayFixture(t)
	var slowCalls, fastCalls atomic.Int32
	canceled := make(chan struct{}, 20)
	makeServer := func(label string, calls *atomic.Int32, wait time.Duration) *httptest.Server {
		return httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			if !strings.HasPrefix(r.Host, "example.com:") || r.TLS.ServerName != "example.com" || r.Header.Get("X-Riot-Token") != "" || r.Header.Get("X-Riot-Platform") != "kr" {
				t.Errorf("Host/SNI/headers changed: host=%s sni=%s headers=%v", r.Host, r.TLS.ServerName, r.Header)
			}
			if wait > 0 {
				select {
				case <-time.After(wait):
				case <-r.Context().Done():
					canceled <- struct{}{}
					return
				}
			}
			w.Header().Set("Content-Type", "application/json")
			if label == "429" {
				w.Header().Set("Retry-After", "30")
				w.Header().Set("X-Relay-Cooldown", "application")
				w.WriteHeader(429)
			}
			fmt.Fprintf(w, `{"entry":%q}`, label)
		}))
	}
	primaryLabel := "A"
	if mode == "429" {
		primaryLabel = "429"
		mode = "auto"
	}
	a, b := makeServer(primaryLabel, &slowCalls, delay), makeServer("B1", &fastCalls, 0)
	t.Cleanup(a.Close)
	t.Cleanup(b.Close)
	pool := x509.NewCertPool()
	pool.AddCert(a.Certificate())
	pool.AddCert(b.Certificate())
	cp := newChampionProvider()
	cp.client = &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}}}
	entry := func(label, origin string) riotRelayEntry {
		_, port, _ := net.SplitHostPort(strings.TrimPrefix(origin, "https://"))
		return riotRelayEntry{Label: label, Hostname: "example.com:" + port, IP: "127.0.0.1"}
	}
	c := riotRelayEntriesConfig{Mode: mode, Preferred: "A", Entries: []riotRelayEntry{entry("A", a.URL), entry("B1", b.URL)}}
	m := newRiotRelayEntryManager(c, 4)
	return newRiotProvider(cp), m, &slowCalls, &fastCalls, canceled
}
func TestR265TLSHedgeBackupWinsAndCancelsSlowWithSharedSlots(t *testing.T) {
	p, m, a, b, canceled := r265TLSRelays(t, "auto", 5*time.Second)
	// Four earlier logical GETs make the fifth request eligible for the 20%
	// budget. These are explicit warm-up fixtures, outside the measured request.
	for i := 0; i < 4; i++ {
		m.beginGET()
	}
	shared := make(chan struct{}, 4)
	shared <- struct{}{}
	ctx := context.WithValue(t.Context(), riotDetailSlotsKey{}, shared)
	started := time.Now()
	r := p.candidateGET(ctx, m, m.choose(nil), riotClusterHost, "/lol/match/v5/matches/KR_265", nil, 1<<20)
	if r.err != nil || r.entry.Label != "B1" || a.Load() != 1 || b.Load() != 1 {
		t.Fatalf("backup did not win exactly two requests: %+v %d %d", r, a.Load(), b.Load())
	}
	if time.Since(started) < 1500*time.Millisecond || time.Since(started) > 3*time.Second {
		t.Fatal("hedge threshold changed", time.Since(started))
	}
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("slow request was not canceled")
	}
	if len(m.slots) != 0 || len(shared) != 1 {
		t.Fatal("hedge leaked slots", len(m.slots), len(shared))
	}
}
func TestR265TLSNoHedgeForNormal429BudgetFixedOrSingle(t *testing.T) {
	for _, test := range []struct {
		name, mode   string
		delay        time.Duration
		seed, single bool
	}{{"normal", "auto", 0, true, false}, {"429", "429", 0, true, false}, {"budget", "auto", 1700 * time.Millisecond, false, false}, {"fixed", "fixed", 1700 * time.Millisecond, true, false}, {"single", "auto", 1700 * time.Millisecond, true, true}} {
		t.Run(test.name, func(t *testing.T) {
			p, m, a, b, _ := r265TLSRelays(t, test.mode, test.delay)
			if test.seed {
				for i := 0; i < 4; i++ {
					m.beginGET()
				}
			}
			if test.single {
				m.config.Entries = m.config.Entries[:1]
			}
			r := p.candidateGET(t.Context(), m, m.choose(nil), riotClusterHost, "/lol/match/v5/matches/KR_265", nil, 1<<20)
			if r.err != nil || a.Load() != 1 || b.Load() != 0 {
				t.Fatal("unexpected hedge", r.err, a.Load(), b.Load())
			}
			if test.name == "429" && r.status != 429 {
				t.Fatal(r.status)
			}
		})
	}
}
func TestR265RollingBudgetConcurrentReservationsAndHysteresis(t *testing.T) {
	c := riotRelayEntriesConfig{Mode: "auto", Preferred: "A", Entries: []riotRelayEntry{{Label: "A"}, {Label: "B1"}}}
	m := newRiotRelayEntryManager(c, 4)
	var tickets []*relayHedgeTicket
	for i := 0; i < 20; i++ {
		tickets = append(tickets, m.beginGET())
	}
	var granted atomic.Int32
	var wait sync.WaitGroup
	for _, ticket := range tickets {
		wait.Add(1)
		go func(ticket *relayHedgeTicket) {
			defer wait.Done()
			if _, ok := m.reserveHedge("A", ticket); ok {
				granted.Add(1)
			}
		}(ticket)
	}
	wait.Wait()
	if granted.Load() != 4 {
		t.Fatal("rolling budget bypassed", granted.Load())
	}
	for i := 0; i < 5; i++ {
		m.observe("A", false, 2*time.Second, false)
		m.observe("B1", false, time.Second, false)
	}
	if m.choose(nil).Label != "A" {
		t.Fatal("switched before two cycles")
	}
	for i := 0; i < 5; i++ {
		m.observe("A", false, 2*time.Second, false)
	}
	if m.choose(nil).Label != "B1" {
		t.Fatal("better entry never selected")
	}
	// Alternating better contenders reset hysteresis instead of switching.
	m.current = "A"
	m.contender = ""
	m.betterCycles = 0
	m.completed = 0
	for cycle := 0; cycle < 6; cycle++ {
		m.mu.Lock()
		a, b := time.Second, 2*time.Second
		if cycle%2 == 0 {
			a, b = b, a
		}
		m.runtime["A"].samples = make([]relayBusinessSample, 5)
		m.runtime["B1"].samples = make([]relayBusinessSample, 5)
		for i := 0; i < 5; i++ {
			m.runtime["A"].samples[i].ttfb = a
			m.runtime["B1"].samples[i].ttfb = b
		}
		m.mu.Unlock()
		for i := 0; i < 5; i++ {
			m.observe("A", false, a, false)
		}
		if m.choose(nil).Label != "A" {
			t.Fatal("alternating metrics caused flapping")
		}
	}
}
func TestR265CertificateFailureFallbackAndHiddenConfig(t *testing.T) {
	p, m, a, b, _ := r265TLSRelays(t, "fixed", 0)
	m.current = "B1"
	m.config.Preferred = "B1"
	// B1 deliberately has an untrusted certificate; A remains explicitly trusted.
	base := p.champions.client.Transport.(*http.Transport)
	good := m.client(p.champions.client, m.entry("A"))
	bad := *p.champions.client
	badTLS := base.Clone()
	badTLS.TLSClientConfig = &tls.Config{RootCAs: x509.NewCertPool()}
	bad.Transport = badTLS
	m.clients[&bad] = map[string]*http.Client{"A": good}
	p.champions.client = &bad
	var data map[string]any
	if err := p.getCandidateRelay(t.Context(), m, riotClusterHost, "/riot/account/v1/accounts/by-riot-id/fixture/KR1", nil, &data, 1<<20); err != nil {
		t.Fatal("failed to fall back to A", err)
	}
	if data["entry"] != "A" || m.choose(nil).Label != "A" || a.Load() != 1 || b.Load() != 0 {
		t.Fatal("certificate error accepted or fallback missing", data, a.Load(), b.Load())
	}
	if badTLS.TLSClientConfig.InsecureSkipVerify {
		t.Fatal("TLS verification disabled")
	}
	for _, raw := range []string{`{"mode":"bad","entries":[]}`, `{"mode":"auto","preferred":"B1","entries":[{"label":"A","hostname":"host/path"}]}`, `{"entries":[{"label":"A","hostname":"host","ip":"invalid"}]}`} {
		if _, ok := parseRiotRelayEntries(raw); ok {
			t.Fatal("invalid config accepted")
		}
	}
	raw, _ := json.Marshal(riotRelayEntriesConfig{Mode: "fixed", Preferred: "A", Entries: []riotRelayEntry{{Label: "A", Hostname: "riot.yinxiaobia.net"}}})
	if c, ok := parseRiotRelayEntries(string(raw)); !ok || c.Mode != "fixed" || c.Preferred != "A" {
		t.Fatal(c, ok)
	}
}

func TestR265DefaultFixedARequestBytesUnchanged(t *testing.T) {
	r206RelayFixture(t)
	t.Setenv("DEEP_LEGENDS_RELAY_ENTRIES", "")
	cp := newChampionProvider()
	var requests []string
	cp.client = &http.Client{Transport: r196RoundTrip(func(r *http.Request) (*http.Response, error) {
		var wire bytes.Buffer
		if err := r.Write(&wire); err != nil {
			t.Fatal(err)
		}
		requests = append(requests, wire.String())
		return r206RelayResponse(200, []byte(`{"ok":true}`)), nil
	})}
	p := newRiotProvider(cp)
	var out map[string]any
	for _, raw := range []string{"", `{"mode":"fixed","preferred":"A","entries":[{"label":"A","hostname":"relay.example"},{"label":"B1","hostname":"candidate.example"}]}`} {
		t.Setenv("DEEP_LEGENDS_RELAY_ENTRIES", raw)
		if err := p.get(t.Context(), riotClusterHost, "/riot/account/v1/accounts/by-riot-id/fixture/KR1", nil, &out); err != nil {
			t.Fatal(err)
		}
	}
	if len(requests) != 2 || requests[0] != requests[1] {
		t.Fatalf("fixed+A wire changed:\n%v", requests)
	}
	if strings.Contains(requests[1], "X-Riot-Token") || strings.Contains(requests[1], "candidate.example") {
		t.Fatal("fixed A used candidate or credential")
	}
}

func TestR265QueuedHedgesCannotAccumulatePastActualRollingBudget(t *testing.T) {
	m := newRiotRelayEntryManager(riotRelayEntriesConfig{Mode: "auto", Preferred: "A", Entries: []riotRelayEntry{{Label: "A"}, {Label: "B1"}}}, 8)
	var pending []*relayHedgeTicket
	for i := 0; i < 20; i++ {
		pending = append(pending, m.beginGET())
	}
	pending = pending[16:]
	for _, ticket := range pending {
		if _, ok := m.reserveHedge("A", ticket); !ok {
			t.Fatal("expected four initial reservations")
		}
	}
	var fresh *relayHedgeTicket
	for i := 0; i < 25; i++ {
		fresh = m.beginGET()
	}
	if _, ok := m.reserveHedge("A", fresh); ok {
		t.Fatal("queued reservations were forgotten after logical window shifted")
	}
	for _, ticket := range pending {
		if !m.commitHedge(ticket) {
			t.Fatal("reserved hedge unexpectedly lost budget")
		}
		m.mu.Lock()
		n, total := m.actualHedgesLocked(), len(m.actualWindow)
		m.mu.Unlock()
		if n*5 > total {
			t.Fatalf("actual outbound rolling ratio exceeded 20%%: %d/%d", n, total)
		}
	}
	if _, ok := m.reserveHedge("A", fresh); ok {
		t.Fatal("exhausted actual budget admitted another hedge")
	}
}

func TestR265NetworkFallbackAfterThreeAndSummarySeparation(t *testing.T) {
	c := riotRelayEntriesConfig{Mode: "fixed", Preferred: "B1", Entries: []riotRelayEntry{{Label: "A", Hostname: "a.invalid"}, {Label: "B1", Hostname: "b.invalid"}}}
	m := newRiotRelayEntryManager(c, 4)
	for i := 0; i < 2; i++ {
		m.networkFailure("B1", fmt.Errorf("connection reset"))
		if m.choose(nil).Label != "B1" {
			t.Fatal("fell back before three failures")
		}
	}
	m.networkFailure("B1", fmt.Errorf("connection reset"))
	if m.choose(nil).Label != "A" || time.Until(m.runtime["B1"].until) < 29*time.Second {
		t.Fatal("three network failures did not fall back with backoff")
	}
	now := time.Now()
	s := &riotRelayState{now: func() time.Time { return now }}
	s.recordBusinessRequest("network", 0, "match", nil, "B1", now, 0)
	s.recordBusinessRequest("", 200, "match", nil, "A", now, 100*time.Millisecond)
	s.recordHedgeSuppressed("A")
	s.recordHedge("A", "B1", true)
	now = now.Add(10 * time.Minute)
	var events []map[string]any
	s.flushSummary(func(e map[string]any) { events = append(events, e) })
	if len(events) != 2 {
		t.Fatal(events)
	}
	for _, e := range events {
		if e["requests"] != 1 {
			t.Fatal(e)
		}
		if e["relay_entry"] == "A" && (e["hedge_budget_suppressed"] != 1 || e["hedge_triggered"] != 1 || e["hedge_won"] != 0) {
			t.Fatal(e)
		}
		if e["relay_entry"] == "B1" && (e["hedge_budget_suppressed"] != 0 || e["hedge_won"] != 1) {
			t.Fatal(e)
		}
	}
}

func TestR265TabCancellationCancelsBothTLSRequests(t *testing.T) {
	p, m, a, b, canceled := r265TLSRelays(t, "auto", 5*time.Second)
	// Occupy the backup until cancellation too.
	// Keep the primary real TLS request; hold its cached backup until cancellation.
	backup := m.client(p.champions.httpClient(), m.entry("B1"))
	backup.Transport = gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		b.Add(1)
		<-r.Context().Done()
		return nil, r.Context().Err()
	})
	for i := 0; i < 4; i++ {
		m.beginGET()
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan relayAttemptResult, 1)
	go func() {
		done <- p.candidateGET(ctx, m, m.choose(nil), riotClusterHost, "/lol/match/v5/matches/KR_265", nil, 1<<20)
	}()
	deadline := time.Now().Add(3 * time.Second)
	for (a.Load() != 1 || b.Load() != 1) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if a.Load() != 1 || b.Load() != 1 {
		cancel()
		t.Fatal("both requests did not start")
	}
	cancel()
	select {
	case r := <-done:
		if r.err != context.Canceled {
			t.Fatal(r.err)
		}
	case <-time.After(time.Second):
		t.Fatal("tab cancellation left hedges in flight")
	}
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("primary TLS request not canceled")
	}
	if len(m.slots) != 0 {
		t.Fatal("slots leaked")
	}
}

func TestR265CandidateNonJSONBusinessStatusesDoNotBecomeQuota(t *testing.T) {
	for _, status := range []int{200, 404, 429, 503} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			r206RelayFixture(t)
			cp := newChampionProvider()
			cp.client = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
				response := r206RelayResponse(status, []byte(`{}`))
				response.Header.Del("Content-Type")
				return response, nil
			})}
			p := newRiotProvider(cp)
			m := newRiotRelayEntryManager(riotRelayEntriesConfig{Mode: "fixed", Preferred: "A", Entries: []riotRelayEntry{{Label: "A", Hostname: "a.invalid"}}}, 4)
			var headers atomic.Bool
			r := p.candidateAttempt(t.Context(), m, m.entry("A"), riotClusterHost, "/lol/match/v5/matches/KR_265", nil, 1<<20, &headers, nil, nil, func() {})
			if (r.failure == "quota_exhausted") != (status == 503) {
				t.Fatalf("non-JSON business result misclassified: status=%d failure=%s", status, r.failure)
			}
		})
	}
}
