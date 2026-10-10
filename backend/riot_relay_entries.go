package main

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

// Hidden, process-local configuration. An absent configuration takes the exact
// 0.12.79 path. No candidate hostname, IP, or player identity enters diagnostics.
type riotRelayEntry struct {
	Label    string `json:"label"`
	Hostname string `json:"hostname"`
	IP       string `json:"ip,omitempty"`
}

func (e riotRelayEntry) origin() string { return "https://" + e.Hostname }

type riotRelayEntriesConfig struct {
	Mode      string           `json:"mode"`
	Preferred string           `json:"preferred"`
	Entries   []riotRelayEntry `json:"entries"`
}

func parseRiotRelayEntries(raw string) (riotRelayEntriesConfig, bool) {
	var c riotRelayEntriesConfig
	if raw == "" || len(raw) > 8192 || json.Unmarshal([]byte(raw), &c) != nil {
		return c, false
	}
	if c.Mode == "" {
		c.Mode = "fixed"
	}
	if c.Preferred == "" {
		c.Preferred = "A"
	}
	if c.Mode != "fixed" && c.Mode != "auto" || len(c.Entries) == 0 || len(c.Entries) > 3 {
		return c, false
	}
	seen := map[string]bool{}
	for _, e := range c.Entries {
		if e.Label != "A" && e.Label != "B1" && e.Label != "B2" || seen[e.Label] || strings.TrimSpace(e.Hostname) != e.Hostname {
			return c, false
		}
		u, err := url.Parse(e.origin())
		if err != nil || u.Hostname() == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.Host != e.Hostname || e.IP != "" && net.ParseIP(e.IP) == nil {
			return c, false
		}
		seen[e.Label] = true
	}
	return c, seen["A"] && seen[c.Preferred]
}

type relayBusinessSample struct {
	failed bool
	ttfb   time.Duration
}
type relayEntryRuntime struct {
	samples               []relayBusinessSample
	consecutive, backoffs int
	until                 time.Time
	probing               bool
}
type riotRelayEntryManager struct {
	mu                      sync.Mutex
	config                  riotRelayEntriesConfig
	current, contender      string
	betterCycles, completed int
	runtime                 map[string]*relayEntryRuntime
	budget                  []*relayHedgeTicket // Last 20 logical GETs; one reservation per GET.
	actualWindow            []bool
	reservedHedges          int
	slots                   chan struct{}
	clients                 map[*http.Client]map[string]*http.Client
	record                  func(map[string]any)
}

func newRiotRelayEntryManager(c riotRelayEntriesConfig, concurrency int) *riotRelayEntryManager {
	m := &riotRelayEntryManager{config: c, current: c.Preferred, runtime: map[string]*relayEntryRuntime{}, slots: make(chan struct{}, concurrency), clients: map[*http.Client]map[string]*http.Client{}}
	for _, e := range c.Entries {
		m.runtime[e.Label] = &relayEntryRuntime{}
	}
	return m
}
func (s *riotRelayState) entries(concurrency int) *riotRelayEntryManager {
	raw := os.Getenv("DEEP_LEGENDS_RELAY_ENTRIES")
	c, ok := parseRiotRelayEntries(raw)
	if !ok {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.entriesKey != raw {
		s.entriesKey = raw
		s.entryManager = newRiotRelayEntryManager(c, concurrency)
	}
	return s.entryManager
}
func (m *riotRelayEntryManager) entry(label string) riotRelayEntry {
	for _, e := range m.config.Entries {
		if e.Label == label {
			return e
		}
	}
	return m.config.Entries[0]
}
func relayEntryMedian(samples []relayBusinessSample) time.Duration {
	var times []time.Duration
	for _, s := range samples {
		if s.ttfb > 0 {
			times = append(times, s.ttfb)
		}
	}
	if len(times) == 0 {
		return 0
	}
	sort.Slice(times, func(i, j int) bool { return times[i] < times[j] })
	n := len(times)
	if n%2 == 0 {
		return (times[n/2-1] + times[n/2]) / 2
	}
	return times[n/2]
}
func relayEntryFailureRate(samples []relayBusinessSample) float64 {
	n := 0
	for _, s := range samples {
		if s.failed {
			n++
		}
	}
	return float64(n) / float64(max(1, len(samples)))
}
func (m *riotRelayEntryManager) emit(e map[string]any) {
	m.mu.Lock()
	record := m.record
	m.mu.Unlock()
	if record != nil {
		record(e)
	}
}
func (m *riotRelayEntryManager) choose(record func(map[string]any)) riotRelayEntry {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.record = record
	return m.entry(m.current)
}
func (m *riotRelayEntryManager) observe(label string, failed bool, ttfb time.Duration, canceled bool) {
	if canceled {
		return
	}
	m.mu.Lock()
	s := m.runtime[label]
	s.samples = append(s.samples, relayBusinessSample{failed, ttfb})
	if len(s.samples) > 20 {
		s.samples = s.samples[len(s.samples)-20:]
	}
	m.completed++
	if !failed {
		s.consecutive = 0
	}
	var event map[string]any
	if m.config.Mode == "auto" && m.completed%5 == 0 {
		best := m.current
		old := m.runtime[best]
		ready := true
		for _, e := range m.config.Entries {
			if len(m.runtime[e.Label].samples) < 5 {
				ready = false
			}
		}
		if ready && len(old.samples) >= 5 {
			for _, e := range m.config.Entries {
				candidate := m.runtime[e.Label]
				if len(candidate.samples) < 5 || time.Now().Before(candidate.until) {
					continue
				}
				a, b := relayEntryFailureRate(candidate.samples), relayEntryFailureRate(m.runtime[best].samples)
				if a < b || a == b && relayEntryMedian(candidate.samples) > 0 && (relayEntryMedian(m.runtime[best].samples) == 0 || relayEntryMedian(candidate.samples) < relayEntryMedian(m.runtime[best].samples)) {
					best = e.Label
				}
			}
		}
		if best == m.current {
			m.contender = ""
			m.betterCycles = 0
		} else {
			if m.contender == best {
				m.betterCycles++
			} else {
				m.contender = best
				m.betterCycles = 1
			}
			if m.betterCycles >= 2 {
				previous := m.current
				m.current = best
				m.contender = ""
				m.betterCycles = 0
				event = map[string]any{"event": "relay_entry_switch", "reason": "rolling_business", "from": previous, "to": best, "from_failure_rate": relayEntryFailureRate(m.runtime[previous].samples), "to_failure_rate": relayEntryFailureRate(m.runtime[best].samples), "from_ttfb_median_ms": relayEntryMedian(m.runtime[previous].samples).Milliseconds(), "to_ttfb_median_ms": relayEntryMedian(m.runtime[best].samples).Milliseconds()}
			}
		}
	}
	m.mu.Unlock()
	if event != nil {
		m.emit(event)
	}
}
func (m *riotRelayEntryManager) networkFailure(label string, err error) {
	var timeout net.Error
	var authority x509.UnknownAuthorityError
	var hostname x509.HostnameError
	var invalid x509.CertificateInvalidError
	immediate := errors.As(err, &timeout) && timeout.Timeout() || errors.As(err, &authority) || errors.As(err, &hostname) || errors.As(err, &invalid)
	m.mu.Lock()
	s := m.runtime[label]
	s.consecutive++
	var event map[string]any
	if immediate || s.consecutive >= 3 {
		s.backoffs++
		s.until = time.Now().Add(time.Duration(30<<min(2, s.backoffs-1)) * time.Second)
		if label != "A" && m.current == label {
			m.current = "A"
			m.contender = ""
			m.betterCycles = 0
			reason := "network_failures"
			if immediate {
				reason = "timeout_or_certificate"
			}
			event = map[string]any{"event": "relay_entry_switch", "reason": reason, "from": label, "to": "A", "backoff_s": int(time.Until(s.until).Seconds()) + 1}
		}
	}
	m.mu.Unlock()
	if event != nil {
		m.emit(event)
	}
}

type relayHedgeTicket struct{ hedged, reserved bool }

func (m *riotRelayEntryManager) beginGET() *relayHedgeTicket {
	ticket := &relayHedgeTicket{}
	m.addGET(ticket)
	return ticket
}
func (m *riotRelayEntryManager) addGET(ticket *relayHedgeTicket) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.budget) >= 20 {
		m.budget = m.budget[1:]
	}
	m.budget = append(m.budget, ticket)
	m.appendActualLocked(false)
}
func (m *riotRelayEntryManager) appendActualLocked(hedge bool) {
	if len(m.actualWindow) >= 20 {
		m.actualWindow = m.actualWindow[1:]
	}
	m.actualWindow = append(m.actualWindow, hedge)
}
func (m *riotRelayEntryManager) actualHedgesLocked() int {
	n := 0
	for _, h := range m.actualWindow {
		if h {
			n++
		}
	}
	return n
}
func (m *riotRelayEntryManager) releaseHedgeReservation(ticket *relayHedgeTicket) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if ticket.reserved {
		ticket.reserved = false
		m.reservedHedges--
	}
}
func (m *riotRelayEntryManager) commitHedge(ticket *relayHedgeTicket) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !ticket.reserved {
		return false
	}
	ticket.reserved = false
	m.reservedHedges--
	if (m.actualHedgesLocked()+m.reservedHedges+1)*5 > min(20, len(m.actualWindow)+1) {
		ticket.hedged = false
		return false
	}
	m.appendActualLocked(true)
	return true
}
func (m *riotRelayEntryManager) budgetBlocked(ticket *relayHedgeTicket) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	hedges, found := 0, false
	for _, h := range m.budget {
		if h.hedged {
			hedges++
		}
		if h == ticket {
			found = true
		}
	}
	return !found || ticket.hedged || (hedges+1)*5 > len(m.budget) || (m.actualHedgesLocked()+m.reservedHedges+1)*5 > min(20, len(m.actualWindow)+1)
}
func (m *riotRelayEntryManager) reserveHedge(primary string, ticket *relayHedgeTicket) (riotRelayEntry, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.config.Mode != "auto" || len(m.config.Entries) < 2 {
		return riotRelayEntry{}, false
	}
	hedges, found := 0, false
	for _, h := range m.budget {
		if h.hedged {
			hedges++
		}
		if h == ticket {
			found = true
		}
	}
	if !found || ticket.hedged || (hedges+1)*5 > len(m.budget) || (m.actualHedgesLocked()+m.reservedHedges+1)*5 > min(20, len(m.actualWindow)+1) {
		return riotRelayEntry{}, false
	}
	for _, e := range m.config.Entries {
		if e.Label == primary || time.Now().Before(m.runtime[e.Label].until) {
			continue
		}
		ticket.hedged = true
		ticket.reserved = true
		m.reservedHedges++
		return e, true
	}
	return riotRelayEntry{}, false
}
func (m *riotRelayEntryManager) threshold(label string) time.Duration {
	m.mu.Lock()
	defer m.mu.Unlock()
	return min(4*time.Second, max(1500*time.Millisecond, relayEntryMedian(m.runtime[label].samples)*3/2))
}
func (m *riotRelayEntryManager) client(base *http.Client, e riotRelayEntry) *http.Client {
	if e.IP == "" {
		return riotHTTPClientWithoutRedirects(base)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.clients[base] == nil {
		if len(m.clients) >= 8 {
			m.clients = map[*http.Client]map[string]*http.Client{}
		}
		m.clients[base] = map[string]*http.Client{}
	}
	if c := m.clients[base][e.Label]; c != nil {
		return c
	}
	transport, ok := base.Transport.(*http.Transport)
	if !ok {
		transport = http.DefaultTransport.(*http.Transport)
	}
	t := transport.Clone()
	if t.TLSClientConfig != nil {
		t.TLSClientConfig = t.TLSClientConfig.Clone()
		parsed, _ := url.Parse(e.origin())
		t.TLSClientConfig.ServerName = parsed.Hostname()
	}
	dial := t.DialContext
	if dial == nil {
		dial = (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	}
	t.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		parsed, _ := url.Parse(e.origin())
		if err == nil && strings.EqualFold(host, parsed.Hostname()) {
			address = net.JoinHostPort(e.IP, port)
		}
		return dial(ctx, network, address)
	}
	copy := *base
	copy.Transport = t
	copy.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	m.clients[base][e.Label] = &copy
	return &copy
}

type riotDetailSlotsKey struct{}
