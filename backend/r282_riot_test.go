package main

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestR282Request403DoesNotRejectHealthyEmbeddedKey(t *testing.T) {
	s, p, events := r266Embedded(t)
	var probes, direct atomic.Int64
	p.champions.client = &http.Client{Transport: r196RoundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "relay.example" {
			t.Fatal("healthy embedded key must stay direct")
		}
		code := 200
		if strings.Contains(r.URL.Path, "matches/JP1_1234567890") {
			code = 403
		} else if r.URL.Path == "/lol/status/v4/platform-data" {
			probes.Add(1)
		} else {
			direct.Add(1)
		}
		return r206RelayResponse(code, []byte(`{"puuid":"r282-kr-reference","gameName":"Fixture","tagLine":"KR1"}`)), nil
	})}
	var out any
	if err := p.get(context.Background(), p.clusterHost(), "/lol/match/v5/matches/JP1_1234567890", nil, &out); err == nil {
		t.Fatal("single forbidden match must fail")
	}
	if _, src := s.effective(); src != "embedded" {
		t.Fatalf("source=%s", src)
	}
	for i := 0; i < 3; i++ {
		if _, err := p.accountByRiotID(context.Background(), "Korean"+strconv.Itoa(i), "KR1"); err != nil {
			t.Fatal(err)
		}
	}
	if probes.Load() != 1 || direct.Load() != 3 {
		t.Fatalf("probes=%d direct=%d", probes.Load(), direct.Load())
	}
	for _, e := range *events {
		if e["event"] == "riot_request" && e["status"] == 403 {
			if e["path_shape"] != "match/v5/matches/JP1_{digits:10}" || e["source"] != "overview" {
				t.Fatalf("shape/source %v", e)
			}
		}
	}
}
func TestR282ConfirmedAuthCooldownReprobesAndRecovers(t *testing.T) {
	s, p, _ := r266Embedded(t)
	now := time.Now()
	s.now = func() time.Time { return now }
	reject := true
	var probes atomic.Int64
	p.champions.client = &http.Client{Transport: r196RoundTrip(func(r *http.Request) (*http.Response, error) {
		code := 200
		if r.URL.Host != "relay.example" {
			if strings.HasSuffix(r.URL.Path, "platform-data") {
				probes.Add(1)
			}
			if reject {
				code = 403
			}
		}
		return r206RelayResponse(code, []byte(`{"ok":true}`)), nil
	})}
	var out any
	if err := p.get(context.Background(), p.clusterHost(), "/lol/match/v5/matches/JP1_1234567890", nil, &out); err != nil {
		t.Fatal(err)
	}
	if _, src := s.effective(); src != "relay" || riotKeyState() != "cooldown" {
		t.Fatalf("source=%s state=%s", src, riotKeyState())
	}
	if probes.Load() != 1 {
		t.Fatal(probes.Load())
	}
	now = now.Add(10*time.Minute + time.Second)
	if riotKeyState() == "cooldown" {
		t.Fatal("expired cooldown still presented as active")
	}
	reject = false
	if err := p.get(context.Background(), p.platformHost(), "/lol/summoner/v4/summoners/me", nil, &out); err != nil {
		t.Fatal(err)
	}
	if _, src := s.effective(); src != "embedded" || probes.Load() != 2 || s.embeddedRejected {
		t.Fatalf("source=%s probes=%d rejected=%v", src, probes.Load(), s.embeddedRejected)
	}
}

func TestR282LCUMatchEligibilityAndPrivatePathShape(t *testing.T) {
	for _, m := range []gameplayMatch{{GameID: 1, QueueID: 0}, {GameID: 1, QueueID: 420, GameType: "CUSTOM_GAME"}, {GameID: 1, QueueID: 420, GameMode: "PRACTICETOOL"}} {
		if riotPublicMatchEligible(m) {
			t.Fatal("non-public match reached Riot", m)
		}
	}
	if !riotPublicMatchEligible(gameplayMatch{GameID: 1, QueueID: 420, GameType: "MATCHED_GAME"}) {
		t.Fatal("ranked match rejected")
	}
	for _, path := range []string{"/lol/match/v5/matches/KR_private", "/riot/account/v1/accounts/by-riot-id/private-name/tag", "/lol/match/v5/matches/by-puuid/private-puuid/ids"} {
		if strings.Contains(riotPathShape(path), "private") {
			t.Fatal("identity leaked")
		}
	}
}

func TestR282CredentialReplacementDiscardsOldProbe(t *testing.T) {
	s, p, _ := r266Embedded(t)
	key, source := s.effective()
	entered, release := make(chan struct{}), make(chan struct{})
	p.champions.client = &http.Client{Transport: r196RoundTrip(func(r *http.Request) (*http.Response, error) {
		close(entered)
		<-release
		return r206RelayResponse(200, []byte(`{}`)), nil
	})}
	result := make(chan bool, 1)
	go func() { result <- p.confirmRiotCredential(context.Background(), key, source) }()
	<-entered
	s.mu.Lock()
	s.key = "RGAPI-replacement"
	s.mu.Unlock()
	close(release)
	if <-result {
		t.Fatal("old credential confirmation accepted after replacement")
	}
	if current, src := s.effective(); current != "RGAPI-replacement" || src != "user" {
		t.Fatal("replacement lost")
	}
}

func TestR282OlderForeignHistoryKeepsBothTimeBounds(t *testing.T) {
	_, p, _ := r266Embedded(t)
	var seen atomic.Bool
	p.champions.client = &http.Client{Transport: r196RoundTrip(func(r *http.Request) (*http.Response, error) {
		q := r.URL.Query()
		if q.Get("startTime") != "1" || q.Get("endTime") != "1700000000" || q.Get("queue") != "" {
			t.Errorf("lost older unfiltered bounds: %v", q)
		}
		seen.Store(true)
		return r206RelayResponse(200, []byte(`[]`)), nil
	})}
	ctx := context.WithValue(context.Background(), riotHistoryTimeRangeKey{}, riotHistoryTimeRange{Start: 1, End: 1700000000})
	if _, err := p.matchIDsFiltered(ctx, "r282-private-reference", 0, 20, 0, ""); err != nil || !seen.Load() {
		t.Fatal(err, seen.Load())
	}
}

func TestR282UserCredentialDoesNotInheritEmbeddedCooldown(t *testing.T) {
	s, _, _ := r266Embedded(t)
	s.mu.Lock()
	s.embeddedRejected = true
	s.authUntil = time.Now().Add(time.Minute)
	s.key = "RGAPI-r282-user-fixture"
	s.mu.Unlock()
	if state := riotKeyState(); state != "configured" {
		t.Fatalf("user key inherited embedded state: %s", state)
	}
}
