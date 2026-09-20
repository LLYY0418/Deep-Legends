package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestR87PrewarmRejectsDifferentGameWithoutPhaseEvent(t *testing.T) {
	c := &LCUClient{baseURL: "https://127.0.0.1:2999", token: "fixture", http: &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/lol-gameflow/v1/session" {
			return response2351(map[string]any{"gameData": map[string]any{"gameId": 2, "queue": map[string]any{"id": 450, "gameMode": "ARAM"}}}), nil
		}
		return response2351([]any{}), nil
	})}}
	a := &app{liveClientPlayerList: func(context.Context) ([]byte, int, error) { return nil, 0, errors.New("no fixture") }}
	a.liveSnapshots = liveSnapshotCache{client: c, identity: "self", phase: "InProgress", at: time.Now(), response: gameplayLiveResponse{GameID: 1, Phase: "InProgress", Available: true}}
	response := a.cachedGameplayLive(context.Background(), c, Summoner{PUUID: "self"}, "InProgress")
	if response.GameID != 2 {
		t.Fatalf("stale warm game=%d", response.GameID)
	}
}

func TestR87PrewarmSurvivesLongHiddenWindow(t *testing.T) {
	c := &LCUClient{baseURL: "https://127.0.0.1:2999", token: "fixture", http: &http.Client{Transport: gameplayRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return response2351(map[string]any{"gameData": map[string]any{"gameId": 2}}), nil
	})}}
	a := &app{}
	a.liveSnapshots = liveSnapshotCache{client: c, identity: "self", phase: "InProgress", warmPending: true, at: time.Now().Add(-10 * time.Minute), response: gameplayLiveResponse{GameID: 2, Phase: "InProgress", Available: true}}
	response := a.cachedGameplayLive(context.Background(), c, Summoner{PUUID: "self"}, "InProgress")
	if !response.Available || response.GameID != 2 {
		t.Fatalf("warm result expired while window hidden: %+v", response)
	}
	if a.liveSnapshots.at.IsZero() || a.liveSnapshots.warmPending {
		t.Fatal("first foreground read must start the reusable TTL")
	}
}

func TestR87OldPhaseWarmCannotReplaceNewPhaseFlight(t *testing.T) {
	c := &LCUClient{}
	a := &app{lcu: c}
	// Non-loading phases exercise the same lifetime transition without launching IO.
	a.observeGameplayPhase(context.Background(), c, "Lobby")
	oldPhase := a.gameplayFlow.ctx
	a.observeGameplayPhase(context.Background(), c, "Matchmaking")
	if oldPhase.Err() != context.Canceled {
		t.Fatal("previous phase lifetime survived a transition")
	}
	flight := &liveSnapshotFlight{done: make(chan struct{})}
	a.liveSnapshots.flight = flight
	a.cachedGameplayLive(oldPhase, c, Summoner{}, "ChampSelect", true)
	if a.liveSnapshots.flight != flight {
		t.Fatal("late old prewarm detached the current flight")
	}
}

func TestR87ReconnectPreflightRejectsLostEndEvent(t *testing.T) {
	for _, phase := range []string{"Reconnect", "WaitingForStats"} {
		t.Run(phase, func(t *testing.T) {
			var writes atomic.Int32
			c := &LCUClient{baseURL: "https://127.0.0.1:2999", token: "fixture", http: &http.Client{Transport: gameplayRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.Method == "POST" {
					writes.Add(1)
					return response2351(nil), nil
				}
				return response2351(phase), nil
			})}}
			r := newWatchRunner(nil, nil)
			t.Cleanup(func() { r.cancelPending("reconnect") })
			waiting := make(chan time.Duration, 1)
			release := make(chan struct{})
			done := make(chan struct{}, 1)
			r.wait = func(ctx context.Context, d time.Duration) error {
				waiting <- d
				select {
				case <-release:
					return nil
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			r.observe = func(e map[string]any) {
				if e["result"] == "fired" || e["reason"] == "reconnect-no-longer-active" {
					done <- struct{}{}
				}
			}
			s := r.currentWatch()
			s.MasterEnabled = true
			s.Rules.AutoReconnect.Enabled = true
			s.Rules.AutoReconnect.DelayMS = 0
			r.apply(s)
			r.handlePhase(c, "Reconnect")
			select {
			case d := <-waiting:
				if d < 3*time.Second {
					t.Fatal("no debounce")
				}
			case <-time.After(time.Second):
				t.Fatal("not scheduled")
			}
			if writes.Load() != 0 {
				t.Fatal("wrote during debounce")
			}
			close(release)
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("no preflight completion")
			}
			want := int32(0)
			if phase == "Reconnect" {
				want = 1
			}
			if writes.Load() != want {
				t.Fatalf("phase=%s writes=%d", phase, writes.Load())
			}
		})
	}
}

func TestR87BannableProbeKeepsHTTPStatusAndActionGateSeparate(t *testing.T) {
	for _, status := range []int{0, 200, 204, 503} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			f := newExecutionFixture(t)
			if status == 0 {
				f.session.Actions = [][]lcuChampSelectAction{{{ID: 5, Type: "pick", ActorCellID: 7}}}
			}
			original := f.c.http.Transport
			f.c.http.Transport = gameplayRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				if strings.HasSuffix(req.URL.Path, "/bannable-champion-ids") {
					return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(`[141]`))}, nil
				}
				return original.RoundTrip(req)
			})
			f.r.evaluateChampSelect(f.c, f.r.currentWatch().ChampSelect)
			f.mu.Lock()
			defer f.mu.Unlock()
			var probe map[string]any
			for _, e := range f.events {
				if e["event"] == "champselect_ban_probe" {
					probe = e
				}
			}
			if probe == nil || fmt.Sprint(probe["status_code"]) != fmt.Sprint(status) || probe["requested"] != (status != 0) {
				t.Fatalf("status=%d probe=%v", status, probe)
			}
		})
	}
}

func TestR87CollectionThreeComparisonsAndBoundedPendingRetry(t *testing.T) {
	var requests atomic.Int32
	c := &LCUClient{baseURL: "https://127.0.0.1:2999", token: "fixture", http: &http.Client{Transport: gameplayRoundTripFunc(func(*http.Request) (*http.Response, error) {
		requests.Add(1)
		time.Sleep(8 * time.Millisecond)
		return response2351([]map[string]any{{"id": 1001, "owned": true, "purchaseDate": "2026-09-01T00:00:00Z"}, {"id": 1002, "owned": true, "purchaseDate": "2026-09-02T00:00:00Z"}}), nil
	})}}
	run := func(shared bool) time.Duration {
		start := time.Now()
		api := NewInventoryAPI(c)
		if shared {
			api.reads = newCollectionReads(c)
		}
		owned, _, err := api.OwnedSkinIDs(1, []Skin{{ID: 1001}})
		if err != nil {
			t.Fatal(err)
		}
		chromas, _ := loadOwnedChromaIDs(c, 1, []Chroma{{ID: 1002}}, api.reads)
		dates, _ := api.SkinAcquisitionDates(1, owned)
		chromaDates, _ := api.SkinAcquisitionDates(1, chromas)
		if dates[1001] == "" || chromaDates[1002] == "" {
			t.Fatal("lost dates")
		}
		return time.Since(start)
	}
	var old, newTimes []time.Duration
	for range 3 {
		old = append(old, run(false))
		requests.Store(0)
		newTimes = append(newTimes, run(true))
		if requests.Load() != 1 {
			t.Fatalf("optimized endpoint reads=%d", requests.Load())
		}
	}
	slices.Sort(old)
	slices.Sort(newTimes)
	if newTimes[1] >= old[1]*65/100 {
		t.Fatalf("median regression old=%s new=%s", old[1], newTimes[1])
	}
	t.Logf("mock medians old=%s new=%s improvement=%.1f%%", old[1], newTimes[1], 100*(1-float64(newTimes[1])/float64(old[1])))
	pending := AccountData{Loot: []LootItem{{DataPending: true}}}
	a := &app{lcu: c, connected: true, account: pending, refreshRequests: make(chan struct{}, 1)}
	for range 3 {
		a.scheduleCollectionDataRetry(c, pending)
		if a.collectionDataRetry == nil {
			t.Fatal("retry missing")
		}
		a.collectionDataRetry.Reset(time.Millisecond)
		select {
		case <-a.refreshRequests:
		case <-time.After(time.Second):
			t.Fatal("retry not requested")
		}
		a.mu.Lock()
		a.collectionRefreshPending = false
		a.mu.Unlock()
	}
	a.scheduleCollectionDataRetry(c, pending)
	if a.collectionDataRetry != nil {
		a.collectionDataRetry.Stop()
		t.Fatal("unbounded fourth retry")
	}
}

func TestR87ClaimHandlerSuppliesOwnBudget(t *testing.T) {
	var bounded atomic.Bool
	c := &LCUClient{baseURL: "https://127.0.0.1:2999", token: "fixture", http: &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		deadline, ok := r.Context().Deadline()
		bounded.Store(ok && time.Until(deadline) <= 12*time.Second)
		return response2351([]any{}), nil
	})}}
	a := &app{lcu: c, connected: true}
	w := httptest.NewRecorder()
	a.handleClaimExecute(w, httptest.NewRequest("POST", "/api/claim/execute", strings.NewReader(`{"items":[{"key":"grant:fixture"}]}`)))
	if !bounded.Load() {
		t.Fatal("canonical scan has no handler budget")
	}
}

func TestR87PositionBroadcastEveryFailureReason(t *testing.T) {
	for _, reason := range []string{"session-read-failed", "gameflow-read-failed", "chat-read-failed", "conversation-unavailable", "message-write-failed", "team-unavailable"} {
		t.Run(reason, func(t *testing.T) {
			r := newWatchRunner(nil, nil)
			var events []map[string]any
			r.observe = func(e map[string]any) { events = append(events, e) }
			c := &LCUClient{baseURL: "https://127.0.0.1:2999", token: "fixture", http: &http.Client{Transport: gameplayRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				fail := func() (*http.Response, error) { return nil, errors.New("offline") }
				switch req.URL.Path {
				case "/lol-champ-select/v1/session":
					if reason == "session-read-failed" {
						return fail()
					}
					queue := 450
					if reason == "gameflow-read-failed" {
						queue = 0
					}
					team := "100"
					if reason == "team-unavailable" {
						team = ""
					}
					return response2351(map[string]any{"queueId": queue, "benchEnabled": true, "localPlayerCellId": 1, "myTeam": []map[string]any{{"cellId": 1, "team": team}}}), nil
				case "/lol-gameflow/v1/session":
					return fail()
				case "/lol-chat/v1/me":
					if reason == "chat-read-failed" {
						return fail()
					}
					return response2351(map[string]any{"id": "me"}), nil
				case "/lol-chat/v1/conversations":
					if reason == "conversation-unavailable" {
						return response2351([]any{}), nil
					}
					return response2351([]map[string]any{{"id": "champ-select", "type": "championSelect"}}), nil
				default:
					return fail()
				}
			})}}
			r.broadcastPosition(c, Summoner{}, watchBroadcastRule{})
			if len(events) < 2 || events[len(events)-1]["reason"] != reason {
				t.Fatalf("reason=%s events=%v", reason, events)
			}
		})
	}
}

func TestR87SGPForceInvalidationRejectsOldFlightAndKeepsOtherPlayer(t *testing.T) {
	p := newSGPProvider()
	p.cacheHistoryPage("HN1", "self", 0, 20, nil, sgpHistoryCacheEntry{bytes: 1})
	p.cacheHistoryPage("HN1", "other", 0, 20, nil, sgpHistoryCacheEntry{bytes: 2})
	old := p.historyGeneration
	p.invalidatePlayerHistory("HN1", "self")
	p.cacheHistoryPage("HN1", "self", 0, 20, nil, sgpHistoryCacheEntry{bytes: 1}, old)
	if _, _, ok := p.cachedHistoryPage("HN1", "self", 0, 20, nil); ok {
		t.Fatal("late response restored old page")
	}
	if _, _, ok := p.cachedHistoryPage("HN1", "other", 0, 20, nil); !ok {
		t.Fatal("other player invalidated")
	}
}

func TestR87CancellationScopes(t *testing.T) {
	request, cancel := context.WithCancel(context.Background())
	budget, stop := context.WithDeadline(context.WithValue(request, overviewRequestContextKey{}, request), time.Now().Add(-time.Second))
	defer stop()
	if gameplayCancellationScope(budget) != "budget-timeout" {
		t.Fatal("budget scope")
	}
	cancel()
	if gameplayCancellationScope(budget) != "client-canceled" {
		t.Fatal("request scope")
	}
}

func TestR87ClientDiagnosticsReachExportWithLifecycleFields(t *testing.T) {
	for _, section := range []string{"overview", "live", "champions", "favorites", "suite"} {
		t.Run(section, func(t *testing.T) {
			store := newFlowDiagnosticStore(t)
			a := &app{storage: store}
			w := httptest.NewRecorder()
			body := fmt.Sprintf(`{"event":"live_refresh_client","reason":"queue","source":"sse","section":%q,"hidden":true,"phaseChanged":true,"liveRefreshQueued":true}`, section)
			a.handleClientDiagnostic(w, httptest.NewRequest("POST", "/api/diagnostics/client", strings.NewReader(body)))
			data, err := store.readDiagnosticLog()
			var event map[string]any
			if w.Code != 204 || err != nil || json.Unmarshal(data, &event) != nil || event["section"] != section || event["source"] != "sse" || event["hidden"] != true || event["phase_changed"] != true || event["live_refresh_queued"] != true {
				t.Fatalf("status=%d exported=%s error=%v", w.Code, data, err)
			}
		})
	}
	store := newFlowDiagnosticStore(t)
	a := &app{storage: store}
	w := httptest.NewRecorder()
	a.handleClientDiagnostic(w, httptest.NewRequest("POST", "/api/diagnostics/client", strings.NewReader(`{"event":"claim_progress_client","reason":"heartbeat","claiming":true,"done":2,"total":9,"durationMs":5000}`)))
	data, _ := store.readDiagnosticLog()
	var event map[string]any
	if w.Code != 204 || json.Unmarshal(data, &event) != nil || event["claiming"] != true || event["done"] != float64(2) || event["total"] != float64(9) || event["duration_ms"] != float64(5000) {
		t.Fatalf("claim heartbeat missing: %s", data)
	}
}

func TestR87ClaimTimeoutBeforeScanStillPairsItemDiagnostics(t *testing.T) {
	store := newFlowDiagnosticStore(t)
	c := &LCUClient{}
	a := &app{storage: store, lcu: c, connected: true}
	c.claimExecutionMu.Lock()
	defer c.claimExecutionMu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	w := httptest.NewRecorder()
	a.handleClaimExecute(w, httptest.NewRequest("POST", "/api/claim/execute", strings.NewReader(`{"items":[{"key":"one"},{"key":"two"}]}`)).WithContext(ctx))
	data, _ := store.readDiagnosticLog()
	var begins, ends int
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var event map[string]any
		if json.Unmarshal([]byte(line), &event) != nil || event["event"] != "claim_item" {
			continue
		}
		if event["trace_id"] == "" || event["trace_id"] == nil {
			t.Fatal("missing claim trace")
		}
		if event["stage"] == "begin" {
			begins++
		} else {
			ends++
			if event["timed_out"] != true {
				t.Fatal("timeout not recorded")
			}
		}
	}
	if w.Code != 504 || begins != 2 || ends != 2 {
		t.Fatalf("status=%d begin/end=%d/%d log=%s", w.Code, begins, ends, data)
	}
}
