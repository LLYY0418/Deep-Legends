package main

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestR280ReviewProxyRecoveryAfterSpeedProbe(t *testing.T) {
	t.Setenv("DEEP_LEGENDS_SYSTEM_PROXY", "http://127.0.0.1:7890")
	p := newChampionProvider()
	rt := p.client.Transport.(*automaticNetworkTransport)
	defer rt.CloseIdleConnections()
	var clock atomic.Int64
	clock.Store(time.Now().UnixMilli())
	rt.now = func() time.Time { return time.UnixMilli(clock.Load()) }
	rt.probes = map[string]string{"riot": "https://kr.api.riotgames.com/"}
	var calls, recovered atomic.Int32
	var healthy atomic.Bool
	entered, release := make(chan struct{}, 1), make(chan struct{})
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	rt.recover = func() { recovered.Add(1) }
	rt.proxy = r196RoundTrip(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		if !healthy.Load() {
			return nil, &net.OpError{Op: "proxyconnect", Err: errors.New("fixture proxy unavailable")}
		}
		if r.Method != http.MethodHead {
			entered <- struct{}{}
			<-release
		}
		return updateResponse(200, []byte("proxy")), nil
	})
	rt.direct = r196RoundTrip(func(*http.Request) (*http.Response, error) { return updateResponse(200, []byte("direct")), nil })
	request := func() string {
		req, _ := http.NewRequest(http.MethodGet, "https://kr.api.riotgames.com/fixture", nil)
		res, err := rt.RoundTrip(req)
		if err != nil {
			return err.Error()
		}
		defer res.Body.Close()
		body, _ := io.ReadAll(res.Body)
		return string(body)
	}
	if got := request(); got != "direct" {
		t.Fatal(got)
	}
	r281WaitUntil(t, "speed probe did not finish", func() bool {
		rt.mu.Lock()
		defer rt.mu.Unlock()
		choice := rt.choices["riot"]
		return !choice.probing && !choice.at.IsZero()
	})
	before := calls.Load()
	if got := request(); got != "direct" || calls.Load() != before {
		t.Fatal("cooldown retried the proxy")
	}
	healthy.Store(true)
	clock.Add(61_000)
	done := make(chan string, 1)
	go func() { done <- request() }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		close(release)
		t.Fatal("speed selection masked the 60-second proxy recovery")
	}
	concurrent := make(chan string, 12)
	for i := 0; i < 12; i++ {
		go func() { concurrent <- request() }()
	}
	for i := 0; i < 12; i++ {
		select {
		case got := <-concurrent:
			if got != "direct" {
				t.Fatal("concurrent recovery requests should use direct", got)
			}
		case <-time.After(time.Second):
			t.Fatal("concurrent requests waited for the proxy recovery")
		}
	}
	if calls.Load() != before+1 {
		t.Fatal("more than one recovery attempt", calls.Load()-before)
	}
	close(release)
	if got := <-done; got != "proxy" {
		t.Fatal(got)
	}
	if recovered.Load() != 2 {
		t.Fatal("expected one fallback recovery and one proxy recovery", recovered.Load())
	}
}

func TestR280ReviewUnverifiedKeySurvivesRestart(t *testing.T) {
	s := r204KeyFixture(t)
	result, err := s.save(context.Background(), r204ValidationClient(0), r204FixtureKey)
	if err != nil || result != "network_error" {
		t.Fatal(result, err)
	}
	riotUserKeys = loadRiotKeyStore(s.store)
	if got := riotKeyState(); got != "unverified" {
		t.Fatalf("restart changed an unverified key to %s", got)
	}
	var notifications atomic.Int32
	riotUserKeys.verified = func() { notifications.Add(1) }
	riotUserKeys.observe(r204FixtureKey, http.StatusOK)
	riotUserKeys.observe(r204FixtureKey, http.StatusOK)
	riotUserKeys = loadRiotKeyStore(s.store)
	if riotKeyState() != "configured" || notifications.Load() != 1 {
		t.Fatal("successful verification was not persisted exactly once")
	}
}

func TestR280ReviewRecoveryFailureRenewsCooldown(t *testing.T) {
	t.Setenv("DEEP_LEGENDS_SYSTEM_PROXY", "http://127.0.0.1:7890")
	p := newChampionProvider()
	rt := p.client.Transport.(*automaticNetworkTransport)
	defer rt.CloseIdleConnections()
	now := time.Now()
	rt.now = func() time.Time { return now }
	rt.probes = map[string]string{}
	rt.failed = true
	rt.until = now.Add(-time.Second)
	rt.choices["riot"] = networkChoice{direct: true, at: now}
	var proxyCalls, directCalls atomic.Int32
	rt.proxy = r196RoundTrip(func(*http.Request) (*http.Response, error) {
		proxyCalls.Add(1)
		return nil, &net.OpError{Op: "proxyconnect", Err: errors.New("fixture proxy down")}
	})
	rt.direct = r196RoundTrip(func(*http.Request) (*http.Response, error) {
		directCalls.Add(1)
		return nil, errors.New("fixture direct down")
	})
	for i := 0; i < 3; i++ {
		if i == 2 {
			now = now.Add(61 * time.Second)
		}
		req, _ := http.NewRequest(http.MethodGet, "https://kr.api.riotgames.com/fixture", nil)
		if _, err := rt.RoundTrip(req); err == nil {
			t.Fatal("both failed routes unexpectedly succeeded")
		}
		want := int32(1)
		if i == 2 {
			want = 2
		}
		if proxyCalls.Load() != want || directCalls.Load() != int32(i+1) {
			t.Fatalf("attempt %d proxy=%d direct=%d", i, proxyCalls.Load(), directCalls.Load())
		}
	}
}

func TestR280ReviewProxyCanRecoverAfterASecondOutage(t *testing.T) {
	t.Setenv("DEEP_LEGENDS_SYSTEM_PROXY", "http://127.0.0.1:7890")
	p := newChampionProvider()
	rt := p.client.Transport.(*automaticNetworkTransport)
	defer rt.CloseIdleConnections()
	now := time.Now()
	rt.now = func() time.Time { return now }
	rt.probes = map[string]string{}
	var healthy atomic.Bool
	var notifications atomic.Int32
	rt.recover = func() { notifications.Add(1) }
	rt.proxy = r196RoundTrip(func(*http.Request) (*http.Response, error) {
		if !healthy.Load() {
			return nil, &net.OpError{Op: "proxyconnect", Err: errors.New("fixture proxy down")}
		}
		return updateResponse(200, []byte("proxy")), nil
	})
	rt.direct = r196RoundTrip(func(*http.Request) (*http.Response, error) { return updateResponse(200, []byte("direct")), nil })
	for cycle := 0; cycle < 2; cycle++ {
		for _, up := range []bool{false, true} {
			healthy.Store(up)
			if up {
				now = now.Add(61 * time.Second)
			}
			req, _ := http.NewRequest(http.MethodGet, "https://kr.api.riotgames.com/fixture", nil)
			res, err := rt.RoundTrip(req)
			if err != nil {
				t.Fatal(err)
			}
			body, _ := io.ReadAll(res.Body)
			res.Body.Close()
			want := "direct"
			if up {
				want = "proxy"
			}
			if string(body) != want || rt.recovering {
				t.Fatal("recovery reservation leaked across outage cycles", cycle, up)
			}
		}
	}
	if notifications.Load() != 4 {
		t.Fatal("expected one direct/proxy recovery event per outage", notifications.Load())
	}
}

func TestR280ReviewKeyReplacementClearAndLegacyStorage(t *testing.T) {
	s := r204KeyFixture(t)
	if _, err := s.save(context.Background(), r204ValidationClient(0), r204FixtureKey); err != nil {
		t.Fatal(err)
	}
	newKey := r204FixtureKey + "-replacement"
	if _, err := s.save(context.Background(), r204ValidationClient(0), newKey); err != nil {
		t.Fatal(err)
	}
	s.observe(r204FixtureKey, http.StatusOK)
	riotUserKeys = loadRiotKeyStore(s.store)
	if riotUserKeys.key != newKey || riotKeyState() != "unverified" {
		t.Fatal("old response verified the replacement credential")
	}
	s = riotUserKeys
	s.mu.Lock()
	err := s.writeLocked("")
	s.mu.Unlock()
	if err != nil || s.unverified {
		t.Fatal("clear must reset verification state", err)
	}
	riotUserKeys = loadRiotKeyStore(s.store)
	if riotUserKeys.key != "" || riotKeyState() != "unconfigured" {
		t.Fatal("cleared credential was restored")
	}
	legacy, err := protectRiotUserKey([]byte(r204FixtureKey))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.store.root, riotUserKeyFile), legacy, 0600); err != nil {
		t.Fatal(err)
	}
	riotUserKeys = loadRiotKeyStore(s.store)
	if riotUserKeys.key != r204FixtureKey || riotKeyState() != "configured" {
		t.Fatal("legacy credential format no longer loads")
	}
}
