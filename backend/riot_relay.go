package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"mime"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
)

// Public HTTPS origin; the Riot credential exists only in the Worker's Secret.
var riotRelayAddresses = []string{"https://riot.yinxiaobia.net"}

var errRiotRelayUnavailable = errors.New("战绩服务暂时不可用")
var errRiotRelayQuotaExhausted = fmt.Errorf("%w", errRiotRelayUnavailable)

type riotRelayState struct {
	mu              sync.Mutex
	entriesKey      string
	entryManager    *riotRelayEntryManager
	failures        int
	lastSuccess     time.Time
	networkFailures int
	config          string
	active          string
	nextTry         time.Time
	flight          chan struct{}
	quotaUntil      time.Time
	ipUntil         time.Time
	now             func() time.Time
	summaryAt       time.Time
	summary         riotRelayRequestSummary
	entrySummaries  map[string]*riotRelayRequestSummary
	summaryTimer    *time.Timer
	paths           map[string]riotRelayPathCooldown
	applications    map[string]time.Time
}

type riotRelayPathCooldown struct {
	Until time.Time
	Kind  string
}

type riotRelayRequestSummary struct {
	Requests         int `json:"requests"`
	TTFBMillis       []float64
	RateLimited      int `json:"rate_limited"`
	NotFound         int `json:"not_found"`
	FailurePaths     map[string]map[string]int
	Failures         map[string]int `json:"failures"`
	HTTPStatuses     map[string]int `json:"http_statuses"`
	Categories       map[string]int `json:"categories"`
	BusinessSamples  []relayDiagnosticSample
	BusinessRequests int
	HedgeTriggered   int
	HedgeWon         int
	HedgeSuppressed  int
}

type relayDiagnosticSample struct {
	At         time.Time `json:"at"`
	Failure    string    `json:"failure,omitempty"`
	Status     int       `json:"status,omitempty"`
	TTFBMillis float64   `json:"ttfb_ms,omitempty"`
}

func (s *riotRelayState) recordBusinessRequest(failure string, status int, category string, record func(map[string]any), label string, started time.Time, ttfb time.Duration) {
	sample := relayDiagnosticSample{At: started.UTC(), Failure: failure, Status: status, TTFBMillis: float64(ttfb) / float64(time.Millisecond)}
	s.recordRequestSample(failure, status, category, record, label, &sample)
}

// Only fixed route categories reach diagnostics; identity path arguments are
// never retained. Probe requests are counted as other.
func riotRelayRequestCategory(path string) string {
	if strings.HasSuffix(path, "/ids") {
		return "ids"
	}
	for _, route := range []struct{ prefix, category string }{
		{"/riot/account/v1/", "account"}, {"/lol/summoner/v4/", "summoner"},
		{"/lol/match/v5/", "match"}, {"/lol/league/v4/", "league"},
		{"/lol/spectator/v5/", "spectator"}, {"/lol/champion-mastery/v4/", "mastery"},
	} {
		if strings.HasPrefix(path, route.prefix) {
			return route.category
		}
	}
	return "other"
}

func (s *riotRelayState) nowLocked() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}

func (s *riotRelayState) requestErrorLocked() error {
	now := s.nowLocked()
	if now.Before(s.quotaUntil) {
		return errRiotRelayQuotaExhausted
	}
	until, kind := s.ipUntil, "ip"
	if now.Before(until) {
		return &riotStatusError{status: http.StatusTooManyRequests, retryAfter: int(math.Ceil(until.Sub(now).Seconds())), relayCooldown: kind, message: "Riot 接口限流中（HTTP 429），请稍后重试"}
	}
	return nil
}

func (s *riotRelayState) requestErrorFor(host, path string, platforms ...string) error {
	host = riotRelayRateHost(host, platforms)
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.requestErrorLocked(); err != nil {
		return err
	}
	if until := s.applications[host]; s.nowLocked().Before(until) {
		return &riotStatusError{status: 429, retryAfter: int(math.Ceil(until.Sub(s.nowLocked()).Seconds())), relayCooldown: "application", message: "Riot 接口限流中（HTTP 429），请稍后重试"}
	}
	if cold := s.paths[host+path]; s.nowLocked().Before(cold.Until) {
		return &riotStatusError{status: 429, retryAfter: int(math.Ceil(cold.Until.Sub(s.nowLocked()).Seconds())), relayCooldown: cold.Kind, message: "Riot 接口限流中（HTTP 429），请稍后重试"}
	}
	return nil
}

func (s *riotRelayState) cooldown(kind string, seconds int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	until := s.nowLocked().Add(time.Duration(seconds) * time.Second)
	if kind == "ip" && until.After(s.ipUntil) {
		s.ipUntil = until
	}
}

func (s *riotRelayState) observeCooldown(host, path, kind string, seconds int, platforms ...string) {
	host = riotRelayRateHost(host, platforms)
	if kind == "ip" {
		s.cooldown(kind, seconds)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.nowLocked()
	if kind == "application" {
		if s.applications == nil {
			s.applications = map[string]time.Time{}
		}
		until := now.Add(time.Duration(seconds) * time.Second)
		if until.After(s.applications[host]) {
			s.applications[host] = until
		}
		return
	}
	if s.paths == nil {
		s.paths = make(map[string]riotRelayPathCooldown)
	}
	for key, cold := range s.paths {
		if !now.Before(cold.Until) {
			delete(s.paths, key)
		}
	}
	key, until := host+path, now.Add(time.Duration(seconds)*time.Second)
	if until.After(s.paths[key].Until) {
		s.paths[key] = riotRelayPathCooldown{until, kind}
	}
	if len(s.paths) > 512 {
		oldest := key
		for k, cold := range s.paths {
			if cold.Until.Before(s.paths[oldest].Until) {
				oldest = k
			}
		}
		delete(s.paths, oldest)
	}
}

func riotRelayQuotaResponse(header http.Header, body []byte) bool {
	kind, _, err := mime.ParseMediaType(header.Get("Content-Type"))
	if err != nil || kind != "application/json" {
		return true
	}
	if bytes.Contains(bytes.ToLower(body), []byte("error 1027")) {
		return true
	}
	// Also recognize an explicitly JSON-labelled Cloudflare error, without
	// mistaking arbitrary match values such as gold=1027 for an error page.
	var failure struct {
		Code  int `json:"code"`
		Error struct {
			Code int `json:"code"`
		} `json:"error"`
	}
	return json.Unmarshal(body, &failure) == nil && (failure.Code == 1027 || failure.Error.Code == 1027)
}

func (s *riotRelayState) quotaExhausted(origin string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !strings.Contains("\n"+s.config+"\n", "\n"+origin+"\n") {
		return
	}
	now := s.nowLocked().UTC()
	s.quotaUntil = time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.UTC)
	s.active, s.nextTry = "", s.quotaUntil
}

// Aggregate actual outbound Worker requests (including probes), never page
// retries blocked locally. Flush an active window even if quota stops traffic.
func (s *riotRelayState) recordRequest(failure string, status int, category string, record func(map[string]any), labels ...string) {
	label := "A"
	if len(labels) > 0 {
		label = labels[0]
	}
	s.recordRequestSample(failure, status, category, record, label, nil)
}

func (s *riotRelayState) recordRequestSample(failure string, status int, category string, record func(map[string]any), label string, sample *relayDiagnosticSample) {
	switch category {
	case "account", "summoner", "match", "league", "spectator", "mastery":
	default:
		category = "other"
	}
	s.mu.Lock()
	now := s.nowLocked()
	if s.summaryAt.IsZero() {
		s.summaryAt = now
	}
	if s.summaryTimer == nil && record != nil {
		s.summaryTimer = time.AfterFunc(10*time.Minute, func() {
			defer recoverPanic("riotRelay.requestSummary")
			s.flushSummary(record)
		})
	}
	s.summary.Requests++
	if s.entrySummaries == nil {
		s.entrySummaries = map[string]*riotRelayRequestSummary{}
	}
	if s.entrySummaries[label] == nil {
		s.entrySummaries[label] = &riotRelayRequestSummary{}
	}
	per := s.entrySummaries[label]
	per.Requests++
	if sample != nil {
		per.BusinessRequests++
		if len(per.BusinessSamples) < 10000 {
			per.BusinessSamples = append(per.BusinessSamples, *sample)
		}
	}
	if per.Categories == nil {
		per.Categories = map[string]int{}
	}
	per.Categories[category]++
	if status > 0 {
		if per.HTTPStatuses == nil {
			per.HTTPStatuses = map[string]int{}
		}
		per.HTTPStatuses[fmt.Sprint(status)]++
	}
	if status == 429 {
		per.RateLimited++
	}
	if failure == "not_found" {
		per.NotFound++
	} else if failure != "" {
		if per.Failures == nil {
			per.Failures = map[string]int{}
		}
		per.Failures[failure]++
		if failure != "canceled" {
			if per.FailurePaths == nil {
				per.FailurePaths = map[string]map[string]int{}
			}
			if per.FailurePaths[failure] == nil {
				per.FailurePaths[failure] = map[string]int{}
			}
			per.FailurePaths[failure][category]++
		}
	}
	if s.summary.Categories == nil {
		s.summary.Categories = make(map[string]int)
	}
	switch category {
	case "account", "summoner", "match", "league", "spectator", "mastery":
	default:
		category = "other"
	}
	s.summary.Categories[category]++
	if status > 0 {
		if s.summary.HTTPStatuses == nil {
			s.summary.HTTPStatuses = make(map[string]int)
		}
		s.summary.HTTPStatuses[fmt.Sprint(status)]++
	}
	if status == http.StatusTooManyRequests {
		s.summary.RateLimited++
	}
	if failure == "not_found" {
		s.summary.NotFound++
	} else if failure != "" {
		if s.summary.Failures == nil {
			s.summary.Failures = make(map[string]int)
		}
		s.summary.Failures[failure]++
		if failure == "auth" || failure == "network" {
			if s.summary.FailurePaths == nil {
				s.summary.FailurePaths = map[string]map[string]int{}
			}
			if s.summary.FailurePaths[failure] == nil {
				s.summary.FailurePaths[failure] = map[string]int{}
			}
			s.summary.FailurePaths[failure][category]++
		}
	}
	s.mu.Unlock()
}

// Business latency samples contain only timing; never account/path arguments.
func (s *riotRelayState) recordTTFB(elapsed time.Duration, labels ...string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.summary.TTFBMillis) < 10000 {
		s.summary.TTFBMillis = append(s.summary.TTFBMillis, float64(elapsed)/float64(time.Millisecond))
	}
	label := "A"
	if len(labels) > 0 {
		label = labels[0]
	}
	if s.entrySummaries == nil {
		s.entrySummaries = map[string]*riotRelayRequestSummary{}
	}
	if s.entrySummaries[label] == nil {
		s.entrySummaries[label] = &riotRelayRequestSummary{}
	}
	per := s.entrySummaries[label]
	if len(per.TTFBMillis) < 10000 {
		per.TTFBMillis = append(per.TTFBMillis, float64(elapsed)/float64(time.Millisecond))
	}
}

func (s *riotRelayState) hedgeSummaryLocked(label string) *riotRelayRequestSummary {
	if s.entrySummaries == nil {
		s.entrySummaries = map[string]*riotRelayRequestSummary{}
	}
	if s.entrySummaries[label] == nil {
		s.entrySummaries[label] = &riotRelayRequestSummary{}
	}
	return s.entrySummaries[label]
}
func (s *riotRelayState) recordHedgeSuppressed(label string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hedgeSummaryLocked(label).HedgeSuppressed++
}
func (s *riotRelayState) recordHedge(primary, winner string, triggered bool) {
	if !triggered {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hedgeSummaryLocked(primary).HedgeTriggered++
	s.hedgeSummaryLocked(winner).HedgeWon++
}

func relayRequestSummaryEvent(summary riotRelayRequestSummary, label string, start, end time.Time, suppressed int) map[string]any {
	failures := summary.Failures
	if failures == nil {
		failures = map[string]int{}
	}
	statuses, categories := summary.HTTPStatuses, summary.Categories
	if statuses == nil {
		statuses = map[string]int{}
	}
	if categories == nil {
		categories = map[string]int{}
	}
	entry := map[string]any{"event": "riot_relay_request_summary", "relay_entry": label, "window_start_utc": start.UTC().Format(time.RFC3339Nano), "window_end_utc": end.UTC().Format(time.RFC3339Nano), "window_ms": end.Sub(start).Milliseconds(), "requests": summary.Requests, "rate_limited": summary.RateLimited, "failures": relaySummaryFailures(failures, summary.FailurePaths), "failure_categories": summary.FailurePaths, "not_found": summary.NotFound, "http_statuses": statuses, "categories": categories, "hedge_budget_suppressed": suppressed}
	entry["route"] = "relay"
	if label == "direct" {
		entry["route"] = "direct"
	}
	if len(summary.BusinessSamples) > 0 {
		entry["business_samples"] = summary.BusinessSamples
		entry["business_samples_truncated"] = summary.BusinessRequests > len(summary.BusinessSamples)
	}
	entry["hedge_triggered"] = summary.HedgeTriggered
	entry["hedge_won"] = summary.HedgeWon
	samples := append([]float64(nil), summary.TTFBMillis...)
	if len(samples) > 0 {
		sort.Float64s(samples)
		entry["ttfb_samples"] = len(samples)
		median := samples[len(samples)/2]
		if len(samples)%2 == 0 {
			median = (samples[len(samples)/2-1] + median) / 2
		}
		entry["ttfb_median_ms"] = median
		entry["ttfb_p90_ms"] = samples[int(math.Ceil(float64(len(samples))*.9))-1]
		entry["ttfb_values_ms"] = samples
	}
	return entry
}

func (s *riotRelayState) flushSummary(record func(map[string]any), partial ...bool) {
	exporting := len(partial) > 0 && partial[0]
	s.mu.Lock()
	now := s.nowLocked()
	if s.summaryAt.IsZero() || !exporting && now.Sub(s.summaryAt) < 10*time.Minute {
		s.mu.Unlock()
		return
	}
	entries := []map[string]any{}
	if len(s.entrySummaries) == 0 {
		entries = append(entries, relayRequestSummaryEvent(s.summary, "A", s.summaryAt, now, s.summary.HedgeSuppressed))
	} else {
		labels := []string{}
		for label := range s.entrySummaries {
			labels = append(labels, label)
		}
		sort.Strings(labels)
		for _, label := range labels {
			entries = append(entries, relayRequestSummaryEvent(*s.entrySummaries[label], label, s.summaryAt, now, s.entrySummaries[label].HedgeSuppressed))
		}
	}
	if s.summaryTimer != nil {
		s.summaryTimer.Stop()
	}
	s.summary, s.summaryAt, s.summaryTimer = riotRelayRequestSummary{}, time.Time{}, nil
	s.entrySummaries = nil
	s.mu.Unlock()
	if record != nil {
		for _, entry := range entries {
			if exporting {
				entry["partial"] = true
			}
			record(entry)
		}
	}
}

var riotRelays = &riotRelayState{}

func configuredRiotRelays() []string {
	var result []string
	for _, origin := range riotRelayAddresses {
		parsed, err := url.Parse(strings.TrimSpace(origin))
		if err == nil && parsed.Scheme == "https" && parsed.Hostname() != "" && parsed.User == nil && parsed.RawQuery == "" && parsed.Fragment == "" && (parsed.Path == "" || parsed.Path == "/") {
			result = append(result, strings.TrimSuffix(parsed.String(), "/"))
		}
	}
	return result
}

func riotHTTPClientWithoutRedirects(client *http.Client) *http.Client {
	copy := *client
	copy.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &copy
}

func (s *riotRelayState) ensureForce(ctx context.Context, client *http.Client, record func(map[string]any), force bool) (string, error) {
	origins := configuredRiotRelays()
	if len(origins) == 0 {
		return "", errRiotKeyMissing
	}
	config := strings.Join(origins, "\n")
	s.mu.Lock()
	if s.config != config {
		s.config, s.active, s.nextTry, s.flight = config, "", time.Time{}, nil
		s.lastSuccess = time.Time{}
		s.quotaUntil, s.ipUntil = time.Time{}, time.Time{}
		s.applications = nil
		s.paths = nil
		s.failures = 0
		s.networkFailures = 0
	}
	if err := s.requestErrorLocked(); err != nil {
		s.mu.Unlock()
		return "", err
	}
	if force && s.flight == nil && (s.lastSuccess.IsZero() || s.nowLocked().Sub(s.lastSuccess) >= 10*time.Minute) {
		s.active = ""
		s.nextTry = time.Time{}
	}
	if s.active != "" {
		active := s.active
		s.mu.Unlock()
		return active, nil
	}
	if !force && s.nowLocked().Before(s.nextTry) {
		s.mu.Unlock()
		return "", errRiotRelayUnavailable
	}
	flight := s.flight
	if flight == nil {
		flight = make(chan struct{})
		s.flight = flight
		// A canceled UI waiter must not poison the shared bounded probe.
		go func() {
			defer recoverPanic("riotRelay.probe")
			s.probe(origins, config, flight, client, record)
		}()
	}
	s.mu.Unlock()
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-flight:
	}
	s.mu.Lock()
	err := s.requestErrorLocked()
	active := s.active
	s.mu.Unlock()
	if err != nil {
		return "", err
	}
	if active == "" {
		return "", errRiotRelayUnavailable
	}
	return active, nil
}

func relayBackoff(failures int) time.Duration {
	return time.Duration(15<<min(3, max(0, failures-1))) * time.Second
}
func (s *riotRelayState) probe(origins []string, config string, flight chan struct{}, client *http.Client, record func(map[string]any)) {
	active := ""
	quota := false
	s.mu.Lock()
	recovering := s.failures > 0 || s.networkFailures > 0
	s.mu.Unlock()
	for _, origin := range origins {
		started := time.Now()
		connectMS, ttfbMS := int64(0), int64(0)
		stage := "headers"
		status := 0
		probeTimeout := false
		bytesReceived := 0
		var traceMu sync.Mutex
		var connectStart time.Time
		trace := &httptrace.ClientTrace{
			DNSStart:     func(httptrace.DNSStartInfo) { traceMu.Lock(); stage = "dns"; traceMu.Unlock() },
			ConnectStart: func(string, string) { traceMu.Lock(); stage = "connect"; connectStart = time.Now(); traceMu.Unlock() },
			ConnectDone: func(string, string, error) {
				traceMu.Lock()
				connectMS = time.Since(connectStart).Milliseconds()
				traceMu.Unlock()
			},
			TLSHandshakeStart:    func() { traceMu.Lock(); stage = "tls"; traceMu.Unlock() },
			WroteRequest:         func(httptrace.WroteRequestInfo) { traceMu.Lock(); stage = "headers"; traceMu.Unlock() },
			GotFirstResponseByte: func() { traceMu.Lock(); ttfbMS = time.Since(started).Milliseconds(); traceMu.Unlock() },
		}
		for _, path := range []string{"/health", "/r/kr/lol/status/v4/platform-data"} {
			ctx, cancel := context.WithTimeout(httptrace.WithClientTrace(context.Background(), trace), 8*time.Second)
			defer cancel()
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, origin+path, nil)
			if err != nil {
				break
			}
			response, err := riotHTTPClientWithoutRedirects(client).Do(req)
			if err != nil {
				s.recordRequest("network", 0, "other", record)
				var timeout net.Error
				probeTimeout = errors.As(err, &timeout) && timeout.Timeout()
				var dns *net.DNSError
				if errors.As(err, &dns) {
					traceMu.Lock()
					stage = "dns"
					traceMu.Unlock()
				}
				break
			}
			status = response.StatusCode
			if status == 503 {
				var received bytes.Buffer
				body, readErr := readLimited(io.TeeReader(response.Body, &received), 64<<10)
				bytesReceived += received.Len()
				if readErr != nil {
					traceMu.Lock()
					stage = "body"
					traceMu.Unlock()
				}
				if readErr == nil && riotRelayQuotaResponse(response.Header, body) {
					response.Body.Close()
					s.quotaExhausted(origin)
					quota = true
					s.recordRequest("quota_exhausted", status, "other", record)
					break
				}
			}
			// Health is header-only; close immediately so a slow body cannot turn
			// a reachable origin into a failure. Legacy Workers fall back on 404.
			response.Body.Close()
			cancel()
			if status >= 200 && status < 300 {
				active = origin
				s.recordRequest("", status, "other", record)
				break
			}
			s.recordRequest("http", status, "other", record)
			if path == "/health" && status == 404 {
				continue
			}
			break
		}
		traceMu.Lock()
		failureStage := stage
		cm, tm := connectMS, ttfbMS
		traceMu.Unlock()
		if active != "" {
			failureStage = ""
		}
		s.mu.Lock()
		backoff := relayBackoff(s.failures + 1)
		s.mu.Unlock()
		if active != "" {
			backoff = 0
		}
		if record != nil {
			result := "failed"
			if quota {
				result = "quota_exhausted"
			}
			if active != "" {
				result = "ok"
			}
			record(map[string]any{"event": "riot_relay_probe", "result": result, "duration_ms": time.Since(started).Milliseconds(), "http_status": status, "failure_stage": failureStage, "timeout": probeTimeout, "connect_ms": cm, "ttfb_ms": tm, "bytes": bytesReceived, "backoff_s": int(backoff.Seconds())})
		}
		if active != "" {
			break
		}
	}
	s.mu.Lock()
	if s.config == config && s.flight == flight {
		if s.nowLocked().Before(s.quotaUntil) {
			active = ""
		}
		if active == "" && !s.lastSuccess.IsZero() && s.nowLocked().Sub(s.lastSuccess) < 10*time.Minute {
			active = s.active
		}
		s.active, s.flight = active, nil
		if active == "" {
			s.failures++
			s.nextTry = s.nowLocked().Add(relayBackoff(s.failures))
		} else {
			s.failures = 0
			s.networkFailures = 0
			s.lastSuccess = s.nowLocked()
			s.nextTry = time.Time{}
		}
	}
	close(flight)
	s.mu.Unlock()
	if active != "" && recovering && record != nil {
		record(map[string]any{"event": "riot-relay-recovered"})
	}
}

func (s *riotRelayState) unavailable() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.nowLocked().Before(s.quotaUntil) || s.networkFailures >= 3 && s.nowLocked().Before(s.nextTry)
}

func (s *riotRelayState) succeeded(origin string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.nowLocked().Before(s.quotaUntil) {
		s.active = origin
		s.lastSuccess = s.nowLocked()
		s.networkFailures = 0
		s.failures = 0
		s.nextTry = time.Time{}
	}
}
func (s *riotRelayState) failed(origin string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active != origin {
		return
	}
	s.networkFailures++
	if s.networkFailures < 3 {
		return
	}
	s.active = ""
	s.failures++
	s.nextTry = s.nowLocked().Add(relayBackoff(s.failures))
}

func riotRelayCooldown(header http.Header) (string, int) {
	kind := header.Get("X-Relay-Cooldown")
	seconds := riotRetryAfter(header.Get("Retry-After"), time.Now(), 5)
	if kind == "" && seconds == 60 {
		kind = "ip"
	}
	if kind != "application" && kind != "method" && kind != "service" && kind != "ip" {
		kind = "service"
	}
	return kind, seconds
}

func riotRetryAfter(value string, now time.Time, fallback int) int {
	if n, err := time.ParseDuration(value + "s"); err == nil && n > 0 {
		return min(86400, max(1, int(math.Ceil(n.Seconds()))))
	}
	if stamp, err := http.ParseTime(value); err == nil {
		return min(86400, max(1, int(math.Ceil(stamp.Sub(now).Seconds()))))
	}
	return fallback
}

func (p *riotProvider) relayOrigin(ctx context.Context) (string, error) {
	return riotRelays.businessOrigin(ctx, p.champions.httpClient(), p.champions.diag)
}

func riotRelayEndpoint(origin, host, path string) (string, error) {
	region := riotHostRoute(host)
	if region == "" {
		return "", errRiotRelayUnavailable
	}
	return origin + "/r/" + region + path, nil
}

func riotRelayRateHost(host string, platforms []string) string {
	platform := riotRegionKR // Legacy health-probe callers explicitly select KR.
	if len(platforms) > 0 && isRiotRegion(platforms[0]) {
		platform = platforms[0]
	}
	return host + "|" + platform
}

func relaySummaryFailures(totals map[string]int, paths map[string]map[string]int) map[string]any {
	result := map[string]any{}
	for key, count := range totals {
		result[key] = count
	}
	for _, key := range []string{"auth", "network"} {
		if paths[key] != nil {
			result[key] = paths[key]
		}
	}
	return result
}

// R235: the first business request races a bounded shared probe. Only a
// three-network-error breaker or explicit quota/cooldown can block business.
func (s *riotRelayState) businessOrigin(ctx context.Context, client *http.Client, record func(map[string]any)) (string, error) {
	origins := configuredRiotRelays()
	if len(origins) == 0 {
		return "", errRiotKeyMissing
	}
	config := strings.Join(origins, "\n")
	s.mu.Lock()
	if s.config != config {
		s.config = config
		s.active = ""
		s.nextTry = time.Time{}
		s.lastSuccess = time.Time{}
		s.failures = 0
		s.networkFailures = 0
		s.flight = nil
		s.quotaUntil = time.Time{}
		s.ipUntil = time.Time{}
		s.paths = nil
		s.applications = nil
	}
	if err := s.requestErrorLocked(); err != nil {
		s.mu.Unlock()
		return "", err
	}
	if s.networkFailures >= 3 && s.nowLocked().Before(s.nextTry) {
		s.mu.Unlock()
		return "", errRiotRelayUnavailable
	}
	origin := s.active
	if origin == "" {
		origin = origins[0]
		s.active = origin
	}
	recent := !s.lastSuccess.IsZero() && s.nowLocked().Sub(s.lastSuccess) < 10*time.Minute
	if !recent && s.flight == nil {
		flight := make(chan struct{})
		s.flight = flight
		go func() { defer recoverPanic("relay.businessProbe"); s.probe(origins, config, flight, client, record) }()
	}
	s.mu.Unlock()
	return origin, nil
}
