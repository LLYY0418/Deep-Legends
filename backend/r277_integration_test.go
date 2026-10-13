package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestR277ReopenDiscoveryIdentityGatewayProductionPath(t *testing.T) {
	for _, delay := range []time.Duration{2 * time.Second, 20 * time.Second, 70 * time.Second} {
		for _, manual := range []bool{false, true} {
			t.Run(fmt.Sprintf("%v/manual=%v", delay, manual), func(t *testing.T) {
				var identityReads, gatewayReads atomic.Int32
				server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					switch r.URL.Path {
					case "/lol-summoner/v1/current-summoner":
						identityReads.Add(1)
						fmt.Fprint(w, `{"summonerId":17,"puuid":"r277-reopened","gameName":"Reopened","tagLine":"Fixture"}`)
					case sgpEndpointConfig:
						gatewayReads.Add(1)
						fmt.Fprint(w, `"https://hn10-k8s-sgp.lol.qq.com:21019"`)
					case sgpPlatformConfig:
						fmt.Fprint(w, `"HN10"`)
					default:
						w.WriteHeader(404)
						fmt.Fprint(w, `{}`)
					}
				}))
				defer server.Close()
				u, _ := url.Parse(server.URL)
				port, _ := strconv.Atoi(u.Port())
				next := newLCUClient(port, "fixture")
				defer next.Close()
				next.region = "TENCENT"
				next.rsoPlatform = "HN10"
				a, old, _ := r258ViewFixture()
				defer old.Close()
				a.storage = newFlowDiagnosticStore(t)
				a.refreshRequests = make(chan struct{}, 1)
				a.sgp = newSGPProvider()
				a.sgp.observe = a.recordDiagnostic
				a.sgp.gatewayAccount = func(c *LCUClient) string {
					a.mu.RLock()
					defer a.mu.RUnlock()
					if a.lcu != c || !a.clientSessionConnectedLocked() {
						return ""
					}
					return a.summoner.PUUID
				}
				start := time.Unix(1000, 0)
				now := start
				a.observeClientShutdownEvent(old, LCUEvent{URI: "/riotclient/pre-shutdown/begin"}, now)
				if a.markCollectionDirty("inventory") {
					t.Fatal("shutdown inventory refresh accepted")
				}
				// Reproduce the production failed-collection clear, not direct ready fields.
				a.mu.Lock()
				a.clearSnapshotLocked("old port unavailable")
				a.mu.Unlock()
				if manual {
					w := httptest.NewRecorder()
					a.handleRefresh(w, httptest.NewRequest("POST", "/api/refresh?source=user_refresh", nil))
					if w.Code != 202 {
						t.Fatalf("manual HTTP reread status=%d", w.Code)
					}
				} else {
					a.disconnectClient(old, "old stream ended")
				}
				if a.shutdownClient != nil {
					t.Fatal("old shutdown survived clear/manual reconnect")
				}
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				var clockMu sync.Mutex
				readyObserved := false
				discoveredAt := time.Time{}
				a.runConnectionManagerWith(ctx, connectionLoopOps{
					discover: func() (*LCUClient, LCUDiscoveryStatus, error) {
						clockMu.Lock()
						defer clockMu.Unlock()
						if now.Sub(start) < delay {
							return nil, LCUDiscoveryStatus{Result: "not-found"}, errors.New("new port not online")
						}
						discoveredAt = now
						return next, LCUDiscoveryStatus{Result: "connected", PortOpen: true}, nil
					},
					identity: a.refreshIdentityWithClient,
					wait: func(ctx context.Context, d time.Duration) bool {
						clockMu.Lock()
						defer clockMu.Unlock()
						now = now.Add(d)
						return ctx.Err() == nil
					},
					session: func(ctx context.Context, c *LCUClient) error {
						clockMu.Lock()
						defer clockMu.Unlock()
						deadline := time.After(3 * time.Second)
						for {
							if view := a.currentClientView(); view.State == "ready" {
								if c != next || view.ServerID != "HN10" || !view.SGPReady || a.shutdownClient != nil || now.Sub(discoveredAt) > 3*time.Second {
									t.Errorf("reopen not ready: %+v", view)
								}
								readyObserved = true
								cancel()
								return ctx.Err()
							}
							select {
							case <-deadline:
								t.Error("production readiness did not finish within 3s fixture deadline")
								cancel()
								return ctx.Err()
							case <-time.After(time.Millisecond):
							}
						}
					},
				})
				r274CloseApp(t, a, ctx)
				if !readyObserved || identityReads.Load() == 0 || gatewayReads.Load() == 0 {
					t.Fatalf("production chain missing: ready=%v identity=%d gateway=%d", readyObserved, identityReads.Load(), gatewayReads.Load())
				}
				t.Logf("controlled reopen=%v; new port=%d; identity=%d gateway=%d ready=%v", delay, port, identityReads.Load(), gatewayReads.Load(), readyObserved)
			})
		}
	}
}

func TestR277LazyScorePartsEndpointUsesCachedFullMatch(t *testing.T) {
	f := r98OverviewFixture(t, false)
	fixture, err := os.ReadFile("testdata/r272-synthetic-match.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtureMatch map[string]any
	if err := json.Unmarshal(fixture, &fixtureMatch); err != nil {
		t.Fatal(err)
	}
	fixtureMatch["metadata"].(map[string]any)["matchId"] = "KR_1"
	fixtureMatch["info"].(map[string]any)["gameId"] = 1
	fixture, err = json.Marshal(fixtureMatch)
	if err != nil {
		t.Fatal(err)
	}
	var downloads atomic.Int32
	cp := f.a.riot.champions
	original := cp.client.Transport
	cp.client.Transport = gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "/matches/KR_1") {
			downloads.Add(1)
			return r206RelayResponse(200, fixture), nil
		}
		return original.RoundTrip(r)
	})
	for attempt := 0; attempt < 2; attempt++ {
		w := httptest.NewRecorder()
		f.a.handleGameplayMatch(w, httptest.NewRequest("POST", "/api/gameplay/match", strings.NewReader(`{"gameId":1,"participantId":1,"region":"kr","refresh":"score-parts"}`)))
		if w.Code != 200 {
			t.Fatalf("lazy details status=%d: %s", w.Code, w.Body.String())
		}
		var match gameplayMatch
		if err := json.Unmarshal(w.Body.Bytes(), &match); err != nil {
			t.Fatal(err)
		}
		if len(match.Participants) == 0 || match.Participants[0].Score == nil || len(match.Participants[0].Score.Parts) == 0 {
			t.Fatalf("lazy details lost score parts: %s", w.Body.String())
		}
	}
	if downloads.Load() != 1 {
		t.Fatalf("lazy details forced %d downloads, want cached repeat", downloads.Load())
	}
}
