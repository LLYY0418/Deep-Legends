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
	failures        int
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
	summaryTimer    *time.Timer
	paths           map[string]riotRelayPathCooldown
	applications    map[string]time.Time
}

type riotRelayPathCooldown struct {
	Until time.Time
	Kind  string
}

type riotRelayRequestSummary struct {
	Requests     int            `json:"requests"`
	RateLimited  int            `json:"rate_limited"`
	NotFound     int            `json:"not_found"`
	Failures     map[string]int `json:"failures"`
	HTTPStatuses map[string]int `json:"http_statuses"`
	Categories   map[string]int `json:"categories"`
}

// Only fixed route categories reach diagnostics; identity path arguments are
// never retained. Probe requests are counted as other.
func riotRelayRequestCategory(path string) string {
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
func (s *riotRelayState) recordRequest(failure string, status int, category string, record func(map[string]any)) {
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
	}
	s.mu.Unlock()
}

func (s *riotRelayState) flushSummary(record func(map[string]any)) {
	s.mu.Lock()
	now := s.nowLocked()
	if s.summaryAt.IsZero() || now.Sub(s.summaryAt) < 10*time.Minute {
		s.mu.Unlock()
		return
	}
	failures := s.summary.Failures
	if failures == nil {
		failures = map[string]int{}
	}
	statuses, categories := s.summary.HTTPStatuses, s.summary.Categories
	if statuses == nil {
		statuses = map[string]int{}
	}
	if categories == nil {
		categories = map[string]int{}
	}
	entry := map[string]any{"event": "riot_relay_request_summary", "window_ms": now.Sub(s.summaryAt).Milliseconds(), "requests": s.summary.Requests, "rate_limited": s.summary.RateLimited, "failures": failures, "not_found": s.summary.NotFound, "http_statuses": statuses, "categories": categories}
	if s.summaryTimer != nil {
		s.summaryTimer.Stop()
	}
	s.summary, s.summaryAt, s.summaryTimer = riotRelayRequestSummary{}, time.Time{}, nil
	s.mu.Unlock()
	if record != nil {
		record(entry)
	}
}

func (s *riotRelayState) stopSummary() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.summaryTimer != nil {
		s.summaryTimer.Stop()
		s.summaryTimer = nil
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

func (s *riotRelayState) ensure(ctx context.Context, client *http.Client, record func(map[string]any)) (string, error) {
	return s.ensureForce(ctx, client, record, false)
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
	if force && s.flight == nil {
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
		ctx, cancel := context.WithTimeout(httptrace.WithClientTrace(context.Background(), trace), 8*time.Second)
		for _, path := range []string{"/health", "/r/kr/lol/status/v4/platform-data"} {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, origin+path, nil)
			if err != nil {
				break
			}
			response, err := riotHTTPClientWithoutRedirects(client).Do(req)
			if err != nil {
				s.recordRequest("network", 0, "other", record)
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
		cancel()
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
			record(map[string]any{"event": "riot_relay_probe", "result": result, "duration_ms": time.Since(started).Milliseconds(), "http_status": status, "failure_stage": failureStage, "connect_ms": cm, "ttfb_ms": tm, "bytes": bytesReceived, "backoff_s": int(backoff.Seconds())})
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
		s.active, s.flight = active, nil
		if active == "" {
			s.failures++
			s.nextTry = s.nowLocked().Add(relayBackoff(s.failures))
		} else {
			s.failures = 0
			s.networkFailures = 0
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
	return s.nowLocked().Before(s.quotaUntil) || s.active == "" && s.nowLocked().Before(s.nextTry)
}

func (s *riotRelayState) succeeded(origin string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active == origin {
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
	if s.networkFailures < 2 {
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
	return riotRelays.ensure(ctx, p.champions.httpClient(), p.champions.diag)
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
