package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Deployment owner fills HTTPS origins before a subsequent release.
// Empty is unconfigured; never contact a fictional placeholder domain.
var riotRelayAddresses = []string{}

var errRiotRelayUnavailable = errors.New("战绩服务暂时不可用")

type riotRelayState struct {
	mu      sync.Mutex
	config  string
	active  string
	nextTry time.Time
	flight  chan struct{}
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
	}
	if s.active != "" {
		active := s.active
		s.mu.Unlock()
		return active, nil
	}
	if time.Now().Before(s.nextTry) {
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
	active := s.active
	s.mu.Unlock()
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
	defer func() {
		s.mu.Lock()
		if s.config == config && s.flight == flight {
			s.active, s.flight = active, nil
			if active == "" {
				s.nextTry = time.Now().Add(5 * time.Minute)
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
			continue
		}
		status = response.StatusCode
		body, err := readLimited(response.Body, 64<<10)
		response.Body.Close()
		if status == http.StatusOK && err == nil && json.Valid(body) {
			active = origin
			break
		}
	}
	result := "ok"
	if active == "" {
		result = "failed"
	}
	if record != nil {
		record(map[string]any{"event": "riot_relay_probe", "result": result, "duration_ms": time.Since(started).Milliseconds(), "http_status": status})
	}
}

func (s *riotRelayState) unavailable() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.active == "" && time.Now().Before(s.nextTry)
}

func (s *riotRelayState) failed(origin string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active == origin {
		s.active, s.nextTry = "", time.Now().Add(5*time.Minute)
	}
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
