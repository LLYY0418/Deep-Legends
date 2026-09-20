package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestR87ArenaSentinelActuallyPatches(t *testing.T) {
	for _, queue := range []int64{1700, 1710, 1750} {
		t.Run(fmt.Sprint(queue), func(t *testing.T) {
			f := newExecutionFixture(t)
			f.session.QueueID = queue
			f.session.Timer.Phase = "BAN_PICK"
			f.session.Actions = [][]lcuChampSelectAction{{{ID: 5, Type: "pick", ActorCellID: 7, IsInProgress: true}}}
			s := f.r.currentWatch()
			g := s.ChampSelect.Groups["arena"]
			g.Pick.Enabled = true
			g.Pick.DelayMS = 0
			g.Pick.Strategy = "lock-now"
			g.Pick.Champions["default"] = []int64{-3}
			s.ChampSelect.Groups["arena"] = g
			f.r.apply(s)
			f.tick(t)
			if f.count() != 1 || f.last().Body["championId"] != float64(-3) {
				t.Fatalf("queue=%d patches=%+v", queue, f.patches)
			}
			if !isArenaQueue(queue, "CHERRY") {
				t.Fatal("queue table classification")
			}
		})
	}
}

// Scan every function in every champselect production file, not a function-name allowlist.
func TestR87ArenaLiteralBoundary(t *testing.T) {
	files, _ := filepath.Glob("champselect*.go")
	arenaFiles, _ := filepath.Glob("arena_live*.go")
	files = append(files, arenaFiles...)
	files = append(files, "gameplay_refresh.go")
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		if got := arenaLiteralComparisons(file, nil); len(got) > 0 {
			t.Errorf("%s literal queue comparisons: %v", file, got)
		}
	}
	if len(arenaLiteralComparisons("mutant.go", `package main; func newlyAdded(session struct{QueueID int})bool{return session.QueueID == 1700}`)) != 1 {
		t.Fatal("guard missed a newly added function")
	}
}

func TestR87WaitingForStatsInvalidatesOnlySubjectAndDetachesFlight(t *testing.T) {
	a := &app{summoner: Summoner{PUUID: "self"}, overviewQueries: newOverviewQueryCache()}
	c := &LCUClient{}
	a.lcu = c
	self := overviewQuerySnapshotKey(c, gameplayReference{ServerID: "HN1"}, "self", true, 0, 20, "all")
	other := overviewQuerySnapshotKey(c, gameplayReference{ServerID: "HN1"}, "other", false, 0, 20, "all")
	flight := &overviewQueryFlight{done: make(chan struct{})}
	a.overviewQueries.putLocked(self, overviewQueryCacheEntry{at: time.Now()})
	a.overviewQueries.putLocked(other, overviewQueryCacheEntry{at: time.Now()})
	a.overviewQueries.flights[self] = flight
	a.observeGameplayPhase(context.Background(), c, "WaitingForStats")
	if a.overviewQueries.entries[self] != nil || a.overviewQueries.entries[other] == nil || a.overviewQueries.flights[self] != nil {
		t.Fatal("targeted invalidation failed")
	}
	a.overviewQueries.complete(self, flight, gameplayOverview{}, nil)
	if a.overviewQueries.entries[self] != nil {
		t.Fatal("old flight repopulated cache")
	}
	a.overviewQueries.putLocked(self, overviewQueryCacheEntry{at: time.Now()})
	a.observeGameplayPhase(context.Background(), c, "PreEndOfGame")
	a.observeGameplayPhase(context.Background(), c, "EndOfGame")
	if a.overviewQueries.entries[self] == nil {
		t.Fatal("duplicate end phase invalidated twice")
	}
}

func TestR87PhasePrewarmMakesFirstLiveResponseFast(t *testing.T) {
	var sessions atomic.Int32
	c := &LCUClient{baseURL: "https://127.0.0.1:2999", token: "fixture", http: &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/lol-gameflow/v1/gameflow-phase":
			return response2351("InProgress"), nil
		case "/lol-gameflow/v1/session":
			sessions.Add(1)
			select {
			case <-r.Context().Done():
				return nil, r.Context().Err()
			case <-time.After(5 * time.Millisecond):
			}
			return response2351(map[string]any{"gameData": map[string]any{"gameId": 87, "queue": map[string]any{"id": 450, "gameMode": "ARAM", "mapId": 12}, "teamOne": []map[string]any{{"championId": 0, "puuid": strings.Repeat("r", 48), "summonerId": 87}}}}), nil
		default:
			time.Sleep(40 * time.Millisecond)
			return &http.Response{StatusCode: 404, Body: io.NopCloser(strings.NewReader("{}"))}, nil
		}
	})}}
	a := &app{lcu: c, connected: true, liveClientPlayerList: func(context.Context) ([]byte, int, error) { return nil, 0, errors.New("no fixture data") }}
	call := func() time.Duration {
		start := time.Now()
		w := httptest.NewRecorder()
		a.handleGameplayLive(w, httptest.NewRequest("GET", "/api/gameplay/live", nil))
		if w.Code != 200 {
			t.Fatal(w.Body.String())
		}
		return time.Since(start)
	}
	cold := call()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a.observeGameplayPhase(ctx, c, "InProgress")
	deadline := time.Now().Add(2 * time.Second)
	for {
		a.liveSnapshots.mu.Lock()
		ready := !a.liveSnapshots.at.IsZero()
		a.liveSnapshots.mu.Unlock()
		if ready {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("phase did not prewarm")
		}
		time.Sleep(time.Millisecond)
	}
	before := sessions.Load()
	warm := call()
	if sessions.Load() != before+1 || warm >= cold/2 {
		t.Fatalf("cold=%s warm=%s session requests=%d->%d", cold, warm, before, sessions.Load())
	}
	t.Logf("cold=%s warm=%s", cold, warm)
}

func TestR87CollectionSharesPayloadsAndDoesNotCacheEmptyAcrossRefreshes(t *testing.T) {
	var requests atomic.Int32
	c := &LCUClient{baseURL: "https://127.0.0.1:2999", token: "fixture", http: &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		requests.Add(1)
		return response2351([]map[string]any{{"id": 1001, "owned": true, "purchaseDate": "2026-09-01T00:00:00Z"}}), nil
	})}}
	reads := newCollectionReads(c)
	inventory := InventoryAPI{client: c, reads: reads}
	owned, _, err := inventory.OwnedSkinIDs(1, []Skin{{ID: 1001}})
	if err != nil || !owned[1001] {
		t.Fatalf("ownership=%v %v", owned, err)
	}
	dates, _ := inventory.SkinAcquisitionDates(1, owned)
	if requests.Load() != 1 || dates[1001] == "" {
		t.Fatalf("requests=%d dates=%v", requests.Load(), dates)
	}
	// A second refresh has its own payload set, so empty/transient results never stick.
	_, _ = newCollectionReads(c).get("/lol-champions/v1/inventories/1/skins-minimal")
	if requests.Load() != 2 {
		t.Fatal("data survived refresh boundary")
	}
	c.http.Transport = gameplayRoundTripFunc(func(*http.Request) (*http.Response, error) { return response2351([]any{}), nil })
	_, _, err = (InventoryAPI{client: c, reads: newCollectionReads(c)}).OwnedSkinIDs(1, []Skin{{ID: 1001}})
	if err == nil {
		t.Fatal("empty explicit sources treated as permanent zero ownership")
	}
}

func TestR87NineClaimsScanOnceEachAndTimeoutBudgetIncludesLock(t *testing.T) {
	var scans atomic.Int32
	c := &LCUClient{baseURL: "https://127.0.0.1:2999", token: "fixture", http: &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/lol-rewards/v1/grants" {
			scans.Add(1)
			return response2351([]map[string]any{{"info": map[string]any{"id": "r87", "status": "PENDING"}, "rewardGroup": map[string]any{"id": "group87", "rewards": []map[string]any{{"id": "reward87", "itemId": "item87", "itemType": "CURRENCY", "quantity": 1}}}}}), nil
		}
		return response2351([]any{}), nil
	})}}
	a := &app{connected: true, lcu: c}
	for range 9 {
		c.claimSettlements = claimSettlements{}
		w := httptest.NewRecorder()
		a.handleClaimExecute(w, httptest.NewRequest("POST", "/api/claim/execute", strings.NewReader(`{"items":[{"key":"grant:r87"}]}`)))
		var result claimExecuteResponse
		_ = json.Unmarshal(w.Body.Bytes(), &result)
		if w.Code != 200 || result.Succeeded != 1 {
			t.Fatalf("claim=%d %s", w.Code, w.Body.String())
		}
	}
	if scans.Load() != 9 {
		t.Fatalf("nine claims caused %d full scans", scans.Load())
	}
	c.claimExecutionMu.Lock()
	defer c.claimExecutionMu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	w := httptest.NewRecorder()
	start := time.Now()
	a.handleClaimExecute(w, httptest.NewRequest("POST", "/api/claim/execute", strings.NewReader(`{"items":[{"key":"grant:r87"}]}`)).WithContext(ctx))
	if w.Code != 504 || time.Since(start) > 200*time.Millisecond {
		t.Fatal("mutex wait ignored handler budget")
	}
}

func TestR87ReconnectRequiresSustainedPhase(t *testing.T) {
	r := newWatchRunner(nil, nil)
	defer r.handlePhase(&LCUClient{}, "Lobby")
	waiting := make(chan time.Duration, 1)
	canceled := make(chan struct{})
	r.wait = func(ctx context.Context, d time.Duration) error {
		waiting <- d
		<-ctx.Done()
		close(canceled)
		return ctx.Err()
	}
	s := r.currentWatch()
	s.MasterEnabled = true
	s.Rules.AutoReconnect.Enabled = true
	s.Rules.AutoReconnect.DelayMS = 0
	r.apply(s)
	r.handlePhase(&LCUClient{}, "Reconnect")
	select {
	case d := <-waiting:
		if d < 3*time.Second {
			t.Fatalf("reconnect armed after %s", d)
		}
	case <-time.After(time.Second):
		t.Fatal("no sustained wait")
	}
	r.handlePhase(&LCUClient{}, "WaitingForStats")
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("false reconnect not canceled")
	}
}

func TestR87PositionBroadcastFailureHasReason(t *testing.T) {
	r := newWatchRunner(nil, nil)
	var event map[string]any
	r.observe = func(e map[string]any) { event = e }
	c := &LCUClient{baseURL: "https://127.0.0.1:2999", token: "fixture", http: &http.Client{Transport: gameplayRoundTripFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("offline") })}}
	if r.broadcastPosition(c, Summoner{}, watchBroadcastRule{}) != "failed" || event["reason"] != "session-read-failed" {
		t.Fatalf("diagnostic=%v", event)
	}
}

func TestR87RosterLayoutPreservesMainAndStats(t *testing.T) {
	css, err := os.ReadFile("web/gameplay.css")
	if err != nil {
		t.Fatal(err)
	}
	for _, rule := range []string{
		".match-main { display: grid; align-self: start; min-width: 0; grid-template-columns: minmax(0,auto) minmax(0,96px) minmax(0,auto) minmax(0,1fr); align-items: center; gap: 9px 22px; }",
		".match-stats { display: grid; align-content: start; grid-template-rows: repeat(3,18px); gap: 5px; min-height: 54px; min-width: 0; justify-self: start; padding-left: 18px; border-left: 1px solid var(--line); }",
	} {
		if !strings.Contains(string(css), rule) {
			t.Fatalf("protected rule changed: %s", rule)
		}
	}
}
