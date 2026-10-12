package main

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// The 429 case checks repeated recovery on one originating context. Freeze
// that origin before the first request; accountByRiotID must still perform its
// real embedded-429 fallback and return the relay credential scope. The 200
// case exercises top-level automatic fallback and deliberately stays unpinned.
func r276RecoveryFixtureContext(ctx context.Context, status int) context.Context {
	if status == http.StatusTooManyRequests {
		return riotPinnedIdentityContext(ctx)
	}
	return ctx
}

func TestR276RecoveryScopeFixtureControlsCooldown(t *testing.T) {
	for _, pinned := range []bool{false, true} {
		name := "unpinned-route-switch"
		if pinned {
			name = "pinned-one-recovery-context"
		}
		t.Run(name, func(t *testing.T) {
			_, p, _ := r266Embedded(t)
			if p.identityDisk != nil {
				t.Fatal("this recovery fixture must not add persisted identity refreshes")
			}
			var clockMu sync.Mutex
			now := time.Now()
			p.limitNow = func() time.Time { clockMu.Lock(); defer clockMu.Unlock(); return now }
			p.limitSleep = func(_ context.Context, d time.Duration) error {
				clockMu.Lock()
				now = now.Add(d)
				clockMu.Unlock()
				return nil
			}
			var directAccounts, relayAccounts, wrongRelay, wrongDirect atomic.Int32
			started, release := make(chan struct{}), make(chan struct{})
			var releaseOnce sync.Once
			defer releaseOnce.Do(func() { close(release) })
			var firstTrace []string
			var traceMu sync.Mutex
			p.champions.client = &http.Client{Transport: r196RoundTrip(func(r *http.Request) (*http.Response, error) {
				relay := r.URL.Host == "relay.example"
				route := "direct"
				if relay {
					route = "relay"
				}
				kind := "match"
				if strings.Contains(r.URL.Path, "/by-riot-id/") {
					kind = "account"
				}
				traceMu.Lock()
				firstTrace = append(firstTrace, route+":"+kind)
				traceMu.Unlock()
				if kind == "account" {
					if relay {
						if relayAccounts.Add(1) == 1 {
							close(started)
							<-release
						}
						return r206RelayResponse(200, []byte(`{"puuid":"relay-key-subject","gameName":"Fixture","tagLine":"KR1"}`)), nil
					}
					directAccounts.Add(1)
					return r206RelayResponse(429, []byte(`{"status":{"message":"fixture quota"}}`)), nil
				}
				if relay {
					if !strings.Contains(r.URL.Path, "/by-puuid/relay-key-subject/") {
						wrongRelay.Add(1)
						return r206RelayResponse(400, []byte(`{"message":"Exception decrypting"}`)), nil
					}
					return r206RelayResponse(200, []byte(`["KR_2691"]`)), nil
				}
				if strings.Contains(r.URL.Path, "relay-key-subject") {
					wrongDirect.Add(1)
				}
				return r206RelayResponse(400, []byte(`{"message":"Exception decrypting"}`)), nil
			})}
			ctx := context.WithValue(context.Background(), riotLookupIdentityKey{}, riotLookupIdentity{"Fixture", "KR1", "foreign-key-token", riotCredentialScope(context.Background())})
			if pinned {
				ctx = r276RecoveryFixtureContext(ctx, 429)
			}
			done := make(chan error, 1)
			var firstIDs []string
			go func() {
				done <- p.get(ctx, p.clusterHost(), "/lol/match/v5/matches/by-puuid/foreign-key-token/ids", nil, &firstIDs)
			}()
			select {
			case <-started:
			case <-time.After(r274NoProgressLimit):
				t.Fatal("first account fallback did not reach controlled gate")
			}
			// Hold the global policy in its cooling state independent of elapsed wall
			// time. This exposes the two-origin fixture error without sleep or slow IO.
			riotUserKeys.mu.Lock()
			riotUserKeys.routeUntil = time.Now().Add(time.Hour)
			riotUserKeys.mu.Unlock()
			releaseOnce.Do(func() { close(release) })
			select {
			case err := <-done:
				if err != nil || len(firstIDs) != 1 {
					t.Fatal(firstIDs, err)
				}
			case <-time.After(r274NoProgressLimit):
				t.Fatal("first recovery stalled")
			}
			wantTrace := []string{"direct:match", "direct:account", "relay:account", "relay:match"}
			traceMu.Lock()
			trace := append([]string(nil), firstTrace...)
			traceMu.Unlock()
			if strings.Join(trace, ",") != strings.Join(wantTrace, ",") {
				t.Fatal("first fallback path changed", trace)
			}
			var ids []string
			if err := p.get(ctx, p.clusterHost(), "/lol/match/v5/matches/by-puuid/foreign-key-token/ids", nil, &ids); err != nil || len(ids) != 1 {
				t.Fatal(ids, err)
			}
			expectedRelay := int32(2)
			if pinned {
				expectedRelay = 1
			}
			if directAccounts.Load() != 1 || relayAccounts.Load() != expectedRelay || wrongRelay.Load() != 0 || wrongDirect.Load() != 0 {
				t.Fatal(directAccounts.Load(), relayAccounts.Load(), wrongRelay.Load(), wrongDirect.Load())
			}
		})
	}
}
