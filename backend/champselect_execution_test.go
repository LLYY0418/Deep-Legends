package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

type executionPatch struct {
	Path string
	Body map[string]any
}
type executionFixture struct {
	mu          sync.Mutex
	r           *watchRunner
	c           *LCUClient
	session     lcuChampSelectSession
	bans, picks []int64
	grid        []champSelectGridChampion
	patches     []executionPatch
	events      []map[string]any
	applyWrites bool
	status      int
	failBanRead bool
	// P1-1（R120 复测）：可选的写冻结闸。非 nil 时，action PATCH 在传输层先等这个
	// channel 放行（或请求 ctx 取消——取消优先于放行，放行后还会复查 ctx，保证
	// 「已取消的写绝不落地」是确定性的）。不设置它的测试完全不受影响。
	patchGate chan struct{}
}

func newExecutionFixture(t *testing.T) *executionFixture {
	t.Helper()
	cell, ally := int64(7), int64(8)
	f := &executionFixture{applyWrites: true, status: 204, bans: []int64{-1}, picks: []int64{5, 804, 141, 104}}
	f.session = lcuChampSelectSession{GameID: 777, QueueID: 3110, LocalPlayerCellID: &cell,
		Timer:   lcuChampSelectTimer{Phase: "PLANNING", AdjustedTimeLeftInPhase: 30000},
		Actions: [][]lcuChampSelectAction{{{ID: 0, Type: "ban", ActorCellID: 7, IsInProgress: true}}, {{ID: 5, Type: "pick", ActorCellID: 7}}},
		MyTeam:  []lcuChampSelectPlayer{{CellID: &cell}, {CellID: &ally}}}
	for _, id := range f.picks {
		f.grid = append(f.grid, champSelectGridChampion{ID: id, Owned: true})
	}
	f.r = newWatchRunner(nil, nil)
	f.r.observe = func(e map[string]any) { f.mu.Lock(); defer f.mu.Unlock(); f.events = append(f.events, e) }
	f.r.handleChampSelectPhase("ChampSelect")
	f.configure(true, true, true, "show-then-lock")
	f.c = &LCUClient{baseURL: "https://127.0.0.1:2999", token: "fixture", http: &http.Client{Transport: gameplayRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method == http.MethodPatch {
			f.mu.Lock()
			gate := f.patchGate
			f.mu.Unlock()
			if gate != nil {
				select {
				case <-gate:
				case <-req.Context().Done():
				}
				if err := req.Context().Err(); err != nil {
					return nil, err
				}
			}
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		switch req.URL.Path {
		case champSelectLegacyAPI + "/implementation-active":
			return response2351(false), nil
		case champSelectAPI + "/session":
			return response2351(f.session), nil
		case champSelectAPI + "/all-grid-champions":
			return response2351(f.grid), nil
		case champSelectAPI + "/pickable-champion-ids":
			return response2351(f.picks), nil
		case champSelectAPI + "/bannable-champion-ids":
			if f.failBanRead {
				return &http.Response{StatusCode: 500, Body: io.NopCloser(strings.NewReader("{}"))}, nil
			}
			return response2351(f.bans), nil
		}
		if req.Method == http.MethodPost && strings.HasPrefix(req.URL.Path, champSelectAPI+"/session/bench/swap/") {
			id, _ := strconv.ParseInt(req.URL.Path[strings.LastIndex(req.URL.Path, "/")+1:], 10, 64)
			f.patches = append(f.patches, executionPatch{Path: req.URL.Path})
			if f.applyWrites {
				old := f.session.MyTeam[0].ChampionID
				f.session.MyTeam[0].ChampionID = id
				for i := range f.session.BenchChampions {
					if f.session.BenchChampions[i].ChampionID == id {
						f.session.BenchChampions[i].ChampionID = old
					}
				}
			}
			return &http.Response{StatusCode: f.status, Body: io.NopCloser(strings.NewReader(""))}, nil
		}
		if req.Method != "PATCH" || !strings.HasPrefix(req.URL.Path, champSelectAPI+"/session/actions/") {
			t.Errorf("unexpected request %s %s", req.Method, req.URL.Path)
			return response2351(nil), nil
		}
		var body map[string]any
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		f.patches = append(f.patches, executionPatch{req.URL.Path, body})
		id, _ := strconv.ParseInt(req.URL.Path[strings.LastIndex(req.URL.Path, "/")+1:], 10, 64)
		if f.applyWrites {
			for i := range f.session.Actions {
				for j := range f.session.Actions[i] {
					a := &f.session.Actions[i][j]
					if a.ID != id {
						continue
					}
					a.ChampionID = int64(body["championId"].(float64))
					a.Completed, _ = body["completed"].(bool)
					if a.Completed {
						a.IsInProgress = false
					}
					if a.Type == "pick" {
						f.session.MyTeam[0].ChampionPickIntent = a.ChampionID
					}
				}
			}
		}
		return &http.Response{StatusCode: f.status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))}, nil
	})}}
	t.Cleanup(func() { f.r.handleChampSelectPhase("Lobby") })
	return f
}

func (f *executionFixture) configure(ban, pick, avoid bool, strategy string) {
	s := f.r.currentWatch()
	s.ChampSelect.Enabled = true
	g := s.ChampSelect.Groups["practice"]
	g.Ban.Enabled, g.Pick.Enabled = ban, pick
	g.Ban.DelayMS, g.Pick.DelayMS = 0, 0
	lockDelay := 0 // These legacy execution tests exercise writes, not the user-facing wait.
	g.Pick.LockDelayMS = &lockDelay
	g.Ban.LockDelayMS = &lockDelay // R95: ban now has the same explicit lock wait.
	g.Ban.Strategy, g.Pick.Strategy = strategy, strategy
	g.Ban.AvoidTeammateIntent, g.Pick.AvoidTeammateIntent = avoid, avoid
	g.Ban.Champions["default"], g.Pick.Champions["default"] = []int64{141, 104}, []int64{5, 804}
	s.ChampSelect.Groups["practice"] = g
	f.r.apply(s)
}
func (f *executionFixture) tick(t *testing.T) {
	t.Helper()
	f.r.evaluateChampSelect(f.c, f.r.currentWatch().ChampSelect)
	deadline := time.Now().Add(time.Second)
	for {
		f.r.mu.Lock()
		busy := len(f.r.pending) > 0 || len(f.r.champSelect.inFlight) > 0
		f.r.mu.Unlock()
		if !busy {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("scheduler stuck")
		}
		time.Sleep(time.Millisecond)
	}
}
func (f *executionFixture) age() {
	f.r.mu.Lock()
	defer f.r.mu.Unlock()
	for id, s := range f.r.champSelect.submitted {
		s.At = time.Now().Add(-3 * time.Second)
		f.r.champSelect.submitted[id] = s
	}
}
func (f *executionFixture) count() int { f.mu.Lock(); defer f.mu.Unlock(); return len(f.patches) }
func (f *executionFixture) last() executionPatch {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.patches[len(f.patches)-1]
}

func TestExecutionPlanningNeverBansAndAutomaticallyIntentsWithoutToggle(t *testing.T) {
	f := newExecutionFixture(t)
	f.configure(true, false, true, "lock-now")
	f.bans = []int64{141, 104}
	f.tick(t)
	if f.count() != 0 {
		t.Fatal("ban submitted in PLANNING")
	}
	f.configure(true, true, true, "lock-now")
	f.session.MyTeam[1].ChampionID = 5
	f.tick(t)
	p := f.last()
	if p.Path != champSelectAPI+"/session/actions/5" || len(p.Body) != 1 || p.Body["championId"] != float64(804) {
		t.Fatalf("intent=%+v", p)
	}
	f.tick(t)
	if f.count() != 1 {
		t.Fatal("intent locked or repeated in PLANNING")
	}
	if !f.r.champSelectSnapshot().PickIntent {
		t.Fatal("UI missing intent phase")
	}
}

func TestExecutionReplay1030IntentThenSentinelBanHoverConfirmationAndLock(t *testing.T) {
	f := newExecutionFixture(t)
	f.session.MyTeam[1].ChampionID = 5
	f.tick(t) // Automatic Yunara intent, not a ban.
	f.bans = []int64{141, 104, 804}
	f.tick(t) // Save this session's positive evidence.
	f.session.Timer.Phase = "BAN_PICK"
	f.bans = []int64{-1}
	f.tick(t)
	if p := f.last(); p.Body["type"] != "ban" || p.Body["championId"] != float64(141) || p.Body["completed"] != false {
		t.Fatalf("ban hover=%+v", p)
	}
	f.tick(t)
	if p := f.last(); p.Body["championId"] != float64(141) || p.Body["completed"] != true {
		t.Fatalf("ban lock=%+v", p)
	}
	f.tick(t)
	f.session.Actions[1][0].IsInProgress = true
	f.tick(t)
	if p := f.last(); p.Body["type"] != "pick" || p.Body["championId"] != float64(804) || p.Body["completed"] != true {
		t.Fatalf("pick lock=%+v", p)
	}
	f.tick(t)
	if f.count() != 4 {
		t.Fatalf("writes=%d", f.count())
	}
	var applied int
	for _, e := range f.events {
		if e["stage"] == "confirmation" && e["reason"] == "applied" {
			applied++
		}
	}
	if applied != 4 {
		t.Fatalf("confirmed=%d", applied)
	}
	var namedRecords int
	for _, record := range f.r.champSelectSnapshot().Records {
		if strings.Contains(record.Message, "英雄 ") {
			if record.ChampionID == 0 {
				t.Fatalf("hero record missing structured champion: %+v", record)
			}
			namedRecords++
		}
	}
	if namedRecords < 4 {
		t.Fatalf("missing hero records: %d", namedRecords)
	}
}

func TestChampSelectRuntimeRecordChampionIDRoundTripAndLimit(t *testing.T) {
	r := newWatchRunner(nil, nil)
	r.handleChampSelectPhase("ChampSelect")
	for i := 0; i < 55; i++ {
		r.champSelectChampionLog("ok", "已确认选用英雄 804", 804)
	}
	r.champSelectLog("warn", "已顺延前 4 个不可用备选")
	snapshot := r.champSelectSnapshot()
	if len(snapshot.Records) != 50 || snapshot.Records[0].ChampionID != 804 || snapshot.Records[49].ChampionID != 0 {
		t.Fatalf("invalid runtime records: %+v", snapshot.Records)
	}
	data, err := json.Marshal(snapshot.Records)
	if err != nil {
		t.Fatal(err)
	}
	var records []map[string]any
	if err := json.Unmarshal(data, &records); err != nil {
		t.Fatal(err)
	}
	if records[0]["championId"] != float64(804) || records[49]["championId"] != nil {
		t.Fatalf("JSON contract: %s", data)
	}
	snapshot.Records[0].ChampionID = 141
	if r.champSelectSnapshot().Records[0].ChampionID != 804 {
		t.Fatal("snapshot mutation changed runtime records")
	}
	r.handleChampSelectPhase("Lobby")
	if len(r.champSelectSnapshot().Records) != 0 {
		t.Fatal("records not cleared after leaving champselect")
	}
	var absent *watchRunner
	absent.champSelectLog("ok", "no runner")
}

func TestExecutionCustomCompatibilityAlwaysConfirmsHoverEvenLockNow(t *testing.T) {
	f := newExecutionFixture(t)
	f.configure(true, false, true, "lock-now")
	f.session.Timer.Phase = "BAN_PICK"
	f.tick(t)
	if p := f.last(); p.Body["completed"] != false {
		t.Fatal("compatibility skipped hover")
	}
	f.tick(t)
	if p := f.last(); p.Body["completed"] != true {
		t.Fatal("confirmed hover not locked")
	}
	f.tick(t)
	if f.count() != 2 {
		t.Fatal("duplicate ban")
	}
}

func TestExecutionCompatibilityIsNotAWildcard(t *testing.T) {
	for _, kind := range []string{"empty", "read-failed", "no-action", "planning", "unknown-phase", "banned", "own-intent", "ally-selected", "unknown-hero"} {
		t.Run(kind, func(t *testing.T) {
			f := newExecutionFixture(t)
			f.configure(true, false, true, "lock-now")
			f.session.Timer.Phase = "BAN_PICK"
			s := f.r.currentWatch()
			g := s.ChampSelect.Groups["practice"]
			g.Ban.Champions["default"] = []int64{141}
			s.ChampSelect.Groups["practice"] = g
			f.r.apply(s)
			switch kind {
			case "empty":
				f.bans = []int64{}
			case "read-failed":
				f.failBanRead = true
			case "no-action":
				f.session.Actions[0][0].IsInProgress = false
			case "planning":
				f.session.Timer.Phase = "PLANNING"
			case "unknown-phase":
				f.session.Timer.Phase = ""
			case "banned":
				f.grid[2].SelectionStatus.IsBanned = true
			case "own-intent":
				f.session.MyTeam[0].ChampionPickIntent = 141
			case "ally-selected":
				f.session.MyTeam[1].ChampionID = 141
			case "unknown-hero":
				f.grid = nil
			}
			f.tick(t)
			if f.count() != 0 {
				t.Fatalf("unexpected compatibility write %s", kind)
			}
		})
	}
}

func TestExecutionIntentConflictSwitchAndManualTakeover(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		avoid, locked, manual bool
		want                  int64
	}{{"avoid", true, false, false, 804}, {"ignore-intent", false, false, false, 5}, {"locked-always-excluded", false, true, false, 804}, {"manual-protected", false, false, true, 0}} {
		t.Run(tc.name, func(t *testing.T) {
			f := newExecutionFixture(t)
			f.configure(false, true, tc.avoid, "lock-now")
			f.session.MyTeam[1].ChampionPickIntent = 5
			if tc.locked {
				f.session.MyTeam[1].ChampionID = 5
			}
			if tc.manual {
				f.session.Actions[1][0].ChampionID = 804
			}
			f.tick(t)
			if tc.want == 0 {
				if f.count() != 0 {
					t.Fatal("manual intent overwritten")
				}
				return
			}
			if p := f.last(); p.Body["championId"] != float64(tc.want) {
				t.Fatalf("intent=%+v", p)
			}
		})
	}
}

func TestExecutionRechecksAllyIntentAfterOwnIntent(t *testing.T) {
	f := newExecutionFixture(t)
	f.configure(false, true, true, "show-then-lock")
	f.tick(t)
	f.tick(t)
	f.session.MyTeam[1].ChampionPickIntent = 5
	f.tick(t)
	if p := f.last(); p.Body["championId"] != float64(804) || len(p.Body) != 1 {
		t.Fatalf("did not advance intent: %+v", p)
	}
}

func TestExecutionHTTPAckWithoutApplicationRetriesOnceAndNeverReportsSuccess(t *testing.T) {
	f := newExecutionFixture(t)
	f.configure(true, false, true, "lock-now")
	f.session.Timer.Phase = "BAN_PICK"
	f.applyWrites = false
	f.tick(t)
	f.tick(t)
	if f.count() != 1 {
		t.Fatal("retried before confirmation deadline")
	}
	f.age()
	f.tick(t)
	if f.count() != 2 {
		t.Fatalf("missing recovery: %d", f.count())
	}
	f.age()
	for i := 0; i < 5; i++ {
		f.tick(t)
	}
	if f.count() != 2 {
		t.Fatal("unbounded retries")
	}
	for _, e := range f.events {
		if e["result"] == "fired" {
			t.Fatal("reported unconfirmed success")
		}
	}
	for _, p := range f.patches {
		if p.Body["completed"] != false {
			t.Fatal("locked unconfirmed hover")
		}
	}
}

func TestExecutionTransportFailureAppliedByClientDoesNotDuplicate(t *testing.T) {
	f := newExecutionFixture(t)
	f.configure(false, true, true, "show-only")
	f.status = 500
	f.tick(t)
	f.tick(t)
	f.age()
	f.tick(t)
	if f.count() != 1 {
		t.Fatal("repeated an applied request after transport failure")
	}
}

func TestExecutionPreflightRechecksPhaseIntentAndOwnHover(t *testing.T) {
	f := newExecutionFixture(t)
	f.configure(true, true, true, "lock-now")
	f.r.champSelect.groupID = "practice"
	d := champSelectDecision{Action: champSelectActionBan, ActionID: 0, ChampionID: 141, QueueID: 3110, GameID: 777, LocalCellID: 7, SessionAPI: champSelectAPI}
	if f.r.champSelectRequestStillCurrent(context.Background(), f.c, d) {
		t.Fatal("planning ban preflight accepted")
	}
	f.session.Timer.Phase = "BAN_PICK"
	f.session.MyTeam[1].ChampionPickIntent = 141
	if f.r.champSelectRequestStillCurrent(context.Background(), f.c, d) {
		t.Fatal("ally intent ignored at preflight")
	}
	f.session.MyTeam[1].ChampionPickIntent = 0
	d.Completed = true
	d.ForceHover = true
	if f.r.champSelectRequestStillCurrent(context.Background(), f.c, d) {
		t.Fatal("missing hover accepted for lock")
	}
	d.Intent = true
	d.Completed = false
	d.Action = champSelectActionPick
	d.ActionID = 5
	d.ChampionID = 804
	if f.r.champSelectRequestStillCurrent(context.Background(), f.c, d) {
		t.Fatal("intent allowed after PLANNING")
	}
}

func TestExecutionOldBanEvidenceDoesNotLeakBetweenSessions(t *testing.T) {
	f := newExecutionFixture(t)
	g := map[int64]champSelectGridChampion{141: {ID: 141}, 104: {ID: 104}}
	f.r.champSelectBanAvailability(f.session, champSelectAPI, []int64{141}, g)
	f.session.Timer.Phase = "BAN_PICK"
	ids, source := f.r.champSelectBanAvailability(f.session, champSelectAPI, []int64{-1}, g)
	if len(ids) != 1 || source != "wildcard-session-evidence-hover" {
		t.Fatal(ids, source)
	}
	f.session.GameID++
	ids, source = f.r.champSelectBanAvailability(f.session, champSelectAPI, []int64{-1}, g)
	if len(ids) != 2 || source != "wildcard-grid-hover" {
		t.Fatal("old evidence leaked", ids, source)
	}
}

func TestExecutionLateAcknowledgementCannotReviveCanceledAction(t *testing.T) {
	for _, change := range []string{"master-off", "side-off", "manual-takeover", "new-session"} {
		t.Run(change, func(t *testing.T) {
			f := newExecutionFixture(t)
			started, release, disposed := make(chan struct{}), make(chan struct{}), make(chan struct{})
			var once sync.Once
			base := f.c.http.Transport
			f.c.http.Transport = gameplayRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.Method == http.MethodPatch {
					close(started)
					<-release
				}
				return base.RoundTrip(req)
			})
			observe := f.r.observe
			f.r.observe = func(e map[string]any) {
				observe(e)
				if e["stage"] == "write-disposition" {
					once.Do(func() { close(disposed) })
				}
			}
			f.r.evaluateChampSelect(f.c, f.r.currentWatch().ChampSelect)
			select {
			case <-started:
			case <-time.After(time.Second):
				t.Fatal("write never started")
			}
			switch change {
			case "master-off":
				s := f.r.currentWatch()
				s.ChampSelect.Enabled = false
				f.r.apply(s)
			case "side-off":
				f.configure(true, false, true, "show-only")
			case "manual-takeover":
				f.r.mu.Lock()
				f.r.champSelect.takeover["pick"] = true
				f.r.mu.Unlock()
			case "new-session":
				f.r.handleChampSelectPhase("Lobby")
				f.r.handleChampSelectPhase("ChampSelect")
			}
			close(release)
			select {
			case <-disposed:
			case <-time.After(time.Second):
				t.Fatal("callback never completed")
			}
			f.r.mu.Lock()
			n := len(f.r.champSelect.submitted)
			f.r.mu.Unlock()
			if n != 0 {
				t.Fatalf("late callback revived %s", change)
			}
		})
	}
}
