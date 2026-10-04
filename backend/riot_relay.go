package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"mime"
	"net/http"
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
	mu               sync.Mutex
	config           string
	active           string
	nextTry          time.Time
	flight           chan struct{}
	quotaUntil       time.Time
	applicationUntil time.Time
	ipUntil          time.Time
	now              func() time.Time
	summaryAt        time.Time
	summary          riotRelayRequestSummary
	summaryTimer     *time.Timer
	paths            map[string]riotRelayPathCooldown
}

type riotRelayPathCooldown struct {
	Until time.Time
	Kind  string
}

type riotRelayRequestSummary struct {
	Requests    int            `json:"requests"`
	RateLimited int            `json:"rate_limited"`
	Failures    map[string]int `json:"failures"`
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
	until, kind := s.applicationUntil, "application"
	if s.ipUntil.After(until) {
		until, kind = s.ipUntil, "ip"
	}
	if now.Before(until) {
		return &riotStatusError{status: http.StatusTooManyRequests, retryAfter: int(math.Ceil(until.Sub(now).Seconds())), relayCooldown: kind, message: "Riot 接口限流中（HTTP 429），请稍后重试"}
	}
	return nil
}

func (s *riotRelayState) requestErrorFor(host, path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.requestErrorLocked(); err != nil {
		return err
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
	if kind == "application" && until.After(s.applicationUntil) {
		s.applicationUntil = until
	}
	if kind == "ip" && until.After(s.ipUntil) {
		s.ipUntil = until
	}
}

func (s *riotRelayState) observeCooldown(host, path, kind string, seconds int) {
	if kind == "application" || kind == "ip" {
		s.cooldown(kind, seconds)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.nowLocked()
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
func (s *riotRelayState) recordRequest(failure string, status int, record func(map[string]any)) {
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
	if status == http.StatusTooManyRequests {
		s.summary.RateLimited++
	}
	if failure != "" {
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
	entry := map[string]any{"event": "riot_relay_request_summary", "window_ms": now.Sub(s.summaryAt).Milliseconds(), "requests": s.summary.Requests, "rate_limited": s.summary.RateLimited, "failures": failures}
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
	origins := configuredRiotRelays()
	if len(origins) == 0 {
		return "", errRiotKeyMissing
	}
	config := strings.Join(origins, "\n")
	s.mu.Lock()
	if s.config != config {
		s.config, s.active, s.nextTry, s.flight = config, "", time.Time{}, nil
		s.quotaUntil, s.applicationUntil, s.ipUntil = time.Time{}, time.Time{}, time.Time{}
		s.paths = nil
	}
	if err := s.requestErrorLocked(); err != nil {
		s.mu.Unlock()
		return "", err
	}
	if s.active != "" {
		active := s.active
		s.mu.Unlock()
		return active, nil
	}
	if s.nowLocked().Before(s.nextTry) {
		s.mu.Unlock()
		return "", errRiotRelayUnavailable
	}
	flight := s.flight
	if flight == nil {
		flight = make(chan struct{})
		s.flight = flight
		// A canceled UI waiter must not poison the shared three-second probe.
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

func (s *riotRelayState) probe(origins []string, config string, flight chan struct{}, client *http.Client, record func(map[string]any)) {
	started := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	active, status := "", 0
	quota := false
	defer func() {
		s.mu.Lock()
		if s.config == config && s.flight == flight {
			if s.nowLocked().Before(s.quotaUntil) {
				active = ""
			}
			s.active, s.flight = active, nil
			if active == "" && !s.nowLocked().Before(s.nextTry) {
				s.nextTry = s.nowLocked().Add(5 * time.Minute)
			}
		}
		close(flight)
		s.mu.Unlock()
	}()
	for _, origin := range origins {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, origin+"/r/kr/lol/status/v4/platform-data", nil)
		if err != nil {
			continue
		}
		req.Header.Set("Accept", "application/json")
		response, err := riotHTTPClientWithoutRedirects(client).Do(req)
		if err != nil {
			s.recordRequest("network", 0, record)
			continue
		}
		status = response.StatusCode
		body, err := readLimited(response.Body, 64<<10)
		response.Body.Close()
		if riotRelayQuotaResponse(response.Header, body) {
			quota = true
			s.quotaExhausted(origin)
			s.recordRequest("quota_exhausted", status, record)
			break
		}
		failure := ""
		if err != nil {
			failure = "read"
		} else if !json.Valid(body) {
			failure = "invalid_json"
		} else if status != http.StatusOK {
			failure = "http"
		}
		s.recordRequest(failure, status, record)
		if status == http.StatusTooManyRequests {
			kind, seconds := riotRelayCooldown(response.Header)
			s.observeCooldown(riotPlatformHost, "/lol/status/v4/platform-data", kind, seconds)
			active = origin
			break
		}
		if status == http.StatusOK && err == nil && json.Valid(body) {
			active = origin
			break
		}
	}
	result := "ok"
	if active == "" {
		result = "failed"
	}
	if quota {
		result = "quota_exhausted"
	}
	if record != nil {
		record(map[string]any{"event": "riot_relay_probe", "result": result, "duration_ms": time.Since(started).Milliseconds(), "http_status": status})
	}
}

func (s *riotRelayState) unavailable() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.nowLocked().Before(s.quotaUntil) || s.active == "" && s.nowLocked().Before(s.nextTry)
}

func (s *riotRelayState) failed(origin string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active == origin {
		s.active = ""
		if until := s.nowLocked().Add(5 * time.Minute); until.After(s.nextTry) {
			s.nextTry = until
		}
	}
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
	region := ""
	switch host {
	case riotClusterHost:
		region = "asia"
	case riotPlatformHost:
		region = "kr"
	default:
		return "", errRiotRelayUnavailable
	}
	return origin + "/r/" + region + path, nil
}
