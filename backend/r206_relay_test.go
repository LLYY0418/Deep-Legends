package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func r206RelayFixture(t *testing.T) *riotKeyStore {
	t.Helper()
	store := r204KeyFixture(t)
	addresses, state := riotRelayAddresses, riotRelays
	riotRelayAddresses, riotRelays = []string{"https://relay.example"}, &riotRelayState{config: "https://relay.example", active: "https://relay.example", lastSuccess: time.Now()}
	owned := riotRelays
	t.Cleanup(func() {
		owned.mu.Lock()
		flight := owned.flight
		owned.mu.Unlock()
		if flight != nil {
			select {
			case <-flight:
			case <-time.After(20 * time.Second):
				t.Error("relay probe leaked")
			}
		}
		owned.stopSummary()
		riotRelayAddresses, riotRelays = addresses, state
	})
	return store
}

func r206RelayResponse(status int, body []byte) *http.Response {
	r := updateResponse(status, body)
	r.Header.Set("Content-Type", "application/json")
	return r
}

func TestR206RelayRoutingNoTokenAndDirectPriority(t *testing.T) {
	store := r206RelayFixture(t)
	champions := newChampionProvider()
	var requests []*http.Request
	champions.client = &http.Client{Transport: r196RoundTrip(func(r *http.Request) (*http.Response, error) {
		requests = append(requests, r)
		return r206RelayResponse(200, []byte(`{"ok":true}`)), nil
	})}
	p := newRiotProvider(champions)
	var out map[string]any
	path := "/riot/account/v1/accounts/by-riot-id/" + url.PathEscape("한 글") + "/KR1"
	if err := p.get(context.Background(), riotClusterHost, path, url.Values{"start": {"0"}}, &out); err != nil {
		t.Fatal(err)
	}
	if err := p.get(context.Background(), riotPlatformHost, "/lol/status/v4/platform-data", nil, &out); err != nil {
		t.Fatal(err)
	}
	if len(requests) != 2 || requests[0].URL.EscapedPath() != "/r/asia"+path || requests[0].URL.RawQuery != "start=0" {
		t.Fatal("relay mapping/probe", requests)
	}
	for _, r := range requests {
		if _, present := r.Header["X-Riot-Token"]; present || r.URL.Host != "relay.example" {
			t.Fatal("relay received credential or wrong host")
		}
	}
	if riotKeySource() != "relay" || !riotKeyConfigured() || riotKeyState() != "configured" {
		t.Fatal("relay source not configured")
	}
	store.mu.Lock()
	err := store.writeLocked(r204FixtureKey)
	store.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if err := p.get(context.Background(), riotPlatformHost, "/lol/status/v4/platform-data", nil, &out); err != nil {
		t.Fatal(err)
	}
	last := requests[len(requests)-1]
	if last.URL.Host != riotPlatformHost || last.Header.Get("X-Riot-Token") != r204FixtureKey || riotKeySource() != "user" {
		t.Fatal("saved key must bypass relay")
	}
	t.Setenv("RIOT_API_KEY", "fixture-env")
	if err := p.get(context.Background(), riotPlatformHost, "/lol/status/v4/platform-data", nil, &out); err != nil {
		t.Fatal(err)
	}
	if requests[len(requests)-1].Header.Get("X-Riot-Token") != "fixture-env" || riotKeySource() != "env" {
		t.Fatal("environment priority")
	}
}

func TestR206RelayEmptyAndFailureCooldown(t *testing.T) {
	r206RelayFixture(t)
	state := &riotRelayState{}
	calls := 0
	var events []map[string]any
	client := &http.Client{Transport: r196RoundTrip(func(r *http.Request) (*http.Response, error) {
		calls++
		return r206RelayResponse(503, []byte(`{}`)), nil
	})}
	riotRelayAddresses = nil
	if _, err := state.ensure(t.Context(), client, nil); !errors.Is(err, errRiotKeyMissing) || calls != 0 {
		t.Fatal(err, calls)
	}
	riotRelayAddresses = []string{"https://relay.example"}
	for i := 0; i < 2; i++ {
		if _, err := state.ensure(t.Context(), client, func(row map[string]any) { events = append(events, row) }); !errors.Is(err, errRiotRelayUnavailable) {
			t.Fatal(err)
		}
	}
	if calls != 1 || len(events) != 1 {
		t.Fatal(calls, events)
	}
	data, _ := json.Marshal(events)
	if strings.Contains(string(data), "relay.example") || events[0]["result"] != "failed" {
		t.Fatal(string(data))
	}
	state.mu.Lock()
	remaining := time.Until(state.nextTry)
	state.nextTry = time.Now().Add(-time.Second)
	state.mu.Unlock()
	if remaining < 14*time.Second || remaining > 15*time.Second {
		t.Fatal(remaining)
	}
	state.ensure(t.Context(), client, nil)
	if calls != 2 {
		t.Fatal(calls)
	}
}

func TestR206Relay429PreservesBackoff(t *testing.T) {
	r206RelayFixture(t)
	champions := newChampionProvider()
	calls := 0
	champions.client = &http.Client{Transport: r196RoundTrip(func(r *http.Request) (*http.Response, error) {
		calls++

		response := r206RelayResponse(429, []byte(`{}`))
		response.Header.Set("Retry-After", "7")
		return response, nil
	})}
	p := newRiotProvider(champions)
	var out map[string]any
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	err := p.get(ctx, riotPlatformHost, "/lol/status/v4/platform-data", nil, &out)
	var status *riotStatusError
	if !errors.As(err, &status) || status.status != 429 || status.retryAfter != 7 || calls != 1 {
		t.Fatal("relay quota bypassed", err, calls)
	}
	ctx2, cancel2 := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel2()
	_ = p.get(ctx2, riotPlatformHost, "/lol/status/v4/platform-data", nil, &out)
	if calls != 1 {
		t.Fatal("backoff retried upstream")
	}
}
