package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

func newBenchTakeoverFixture(t *testing.T) *executionFixture {
	f := newExecutionFixture(t)
	f.session.QueueID = 450
	f.session.Actions = nil
	f.session.BenchEnabled = true
	f.session.Timer.Phase = "FINALIZATION"
	f.session.MyTeam[0].ChampionID = 104
	f.session.BenchChampions = []lcuChampSelectBenchChampion{{ChampionID: 5}}
	s := f.r.currentWatch()
	g := s.ChampSelect.Groups["aram"]
	g.Pick.Champions["default"] = []int64{5, 804}
	g.Bench.Enabled = true
	s.ChampSelect.Groups["aram"] = g
	f.r.apply(s)
	f.r.mu.Lock()
	f.r.champSelect.benchFirstSeen[5] = time.Now().Add(-2 * time.Second)
	f.r.mu.Unlock()
	return f
}

func (f *executionFixture) assertTakeover(t *testing.T, championID string) {
	t.Helper()
	f.r.mu.Lock()
	yielded := f.r.champSelect.takeover["pick"] && f.r.champSelect.takeover["bench"] && f.r.champSelect.takeover["trade"]
	pending := len(f.r.pending)
	f.r.mu.Unlock()
	state := f.r.champSelectSnapshot()
	if !yielded || pending != 0 || state.PickStates[championID] != "manual-takeover" || state.ActivePickID != 0 {
		t.Fatalf("manual change not retained: yielded=%t pending=%d state=%+v", yielded, pending, state)
	}
}

func TestTakeoverBenchManualSwapStopsOnceAndNextGameResets(t *testing.T) {
	f := newBenchTakeoverFixture(t)
	f.tick(t) // Tool takes champion 5.
	f.tick(t) // Read back our own swap, never yield to ourselves.
	if f.count() != 1 || f.r.champSelectSnapshot().PickStates["5"] == "manual-takeover" {
		t.Fatal("own swap treated as manual")
	}
	f.mu.Lock()
	f.session.MyTeam[0].ChampionID = 141
	f.session.BenchChampions = nil // Must observe even with an empty bench.
	f.mu.Unlock()
	f.tick(t)
	f.assertTakeover(t, "5")
	for _, id := range []int64{141, 804, 5, 141} {
		f.mu.Lock()
		f.session.MyTeam[0].ChampionID = id
		f.session.BenchChampions = []lcuChampSelectBenchChampion{{ChampionID: 5}, {ChampionID: 804}}
		f.mu.Unlock()
		f.tick(t)
	}
	if f.count() != 1 {
		t.Fatal("tool fought the user's manual choice")
	}
	count := 0
	for _, record := range f.r.champSelectSnapshot().Records {
		if strings.Contains(record.Message, "玩家主动切换") {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("takeover must be logged once: %d", count)
	}
	f.r.handleChampSelectPhase("Lobby")
	f.r.handleChampSelectPhase("ChampSelect")
	f.r.mu.Lock()
	f.r.champSelect.benchFirstSeen[5] = time.Now().Add(-2 * time.Second)
	f.r.mu.Unlock()
	f.tick(t)
	if f.count() != 2 {
		t.Fatal("next game did not resume automation")
	}
}

func TestTakeoverBenchPendingSwapRechecksManualChoiceAndSession(t *testing.T) {
	for _, change := range []string{"manual", "new-game", "target-gone"} {
		t.Run(change, func(t *testing.T) {
			f := newBenchTakeoverFixture(t)
			f.r.mu.Lock()
			f.r.champSelect.benchFirstSeen[5] = time.Now().Add(-850 * time.Millisecond)
			f.r.mu.Unlock()
			f.r.evaluateChampSelect(f.c, f.r.currentWatch().ChampSelect)
			f.mu.Lock()
			switch change {
			case "manual":
				f.session.MyTeam[0].ChampionID = 141
			case "new-game":
				f.session.GameID++
			case "target-gone":
				f.session.BenchChampions = nil
			}
			f.mu.Unlock()
			waitTakeoverIdle(t, f)
			if f.count() != 0 {
				t.Fatal("stale bench request reached the client")
			}
			if change == "manual" {
				f.assertTakeover(t, "5")
				f.tick(t)
				if f.count() != 0 {
					t.Fatal("manual preflight did not latch")
				}
			}
		})
	}
}

func TestTakeoverBenchUnappliedSwapDoesNotBlockNewTargets(t *testing.T) {
	f := newBenchTakeoverFixture(t)
	f.applyWrites = false
	f.tick(t)
	f.tick(t)
	if f.count() != 1 {
		t.Fatal("duplicate swap before readback")
	}
	f.r.mu.Lock()
	f.r.champSelect.benchSwap.SettledAt = time.Now().Add(-3 * time.Second)
	f.r.champSelect.benchFirstSeen[804] = time.Now().Add(-2 * time.Second)
	f.r.mu.Unlock()
	f.mu.Lock()
	f.applyWrites = true
	f.session.BenchChampions = []lcuChampSelectBenchChampion{{ChampionID: 804}}
	f.mu.Unlock()
	f.tick(t)
	f.tick(t)
	if f.count() != 2 || f.r.champSelectSnapshot().PickStates["804"] == "manual-takeover" {
		t.Fatal("unapplied acknowledgement disabled future bench swaps")
	}
}

func TestTakeoverClearingNonWildcardHoverYieldsButUnavailableHeroCanRecover(t *testing.T) {
	for group, queue := range map[string]int64{"ranked": 420, "normal": 400, "arena": 1700} {
		for _, removal := range []string{"manual", "teammate-picked", "banned"} {
			t.Run(group+"/"+removal, func(t *testing.T) {
				f := prepareTakeoverPick(t, group, queue, nil)
				f.tick(t)
				f.r.evaluateChampSelect(f.c, f.r.currentWatch().ChampSelect)
				f.r.mu.Lock()
				last, armed := f.r.champSelect.submitted[5], f.r.pending[champSelectActionPick] != nil
				f.r.mu.Unlock()
				if !last.Confirmed || last.Decision.ForceHover || !armed {
					t.Fatalf("fixture must have a confirmed non-wildcard hover and armed lock: %+v armed=%t", last, armed)
				}
				f.mu.Lock()
				f.session.Actions[0][0].ChampionID = 0
				f.session.MyTeam[0].ChampionPickIntent = 0
				switch removal {
				case "teammate-picked":
					f.session.MyTeam[1].ChampionID = 5
				case "banned":
					f.session.Actions = append(f.session.Actions, []lcuChampSelectAction{{ID: 10, Type: "ban", ActorCellID: 8, ChampionID: 5, Completed: true}})
				}
				f.mu.Unlock()
				f.r.evaluateChampSelect(f.c, f.r.currentWatch().ChampSelect)
				if removal == "manual" {
					f.assertTakeover(t, "5")
					f.tick(t)
					if f.count() != 1 {
						t.Fatal("tool overwrote the player's cleared hover")
					}
				} else {
					waitTakeoverIdle(t, f)
					if f.count() != 2 || f.last().Body["championId"] != float64(804) || f.r.champSelectSnapshot().PickStates["5"] == "manual-takeover" {
						t.Fatal("client removal of unavailable hero prevented fallback")
					}
				}
				f.mu.Lock()
				defer f.mu.Unlock()
				for _, event := range f.events {
					if event["reason"] == "hover-cleared-by-client" {
						t.Fatal("non-wildcard clear was mislabeled as compatibility hover clear")
					}
				}
			})
		}
	}
}

func TestTakeoverBenchOwnEchoBeforeLateAckAndAutomaticUpgrade(t *testing.T) {
	f := newBenchTakeoverFixture(t)
	started, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var once sync.Once
	t.Cleanup(func() { once.Do(func() { close(release) }) })
	base := f.c.http.Transport
	f.c.http.Transport = gameplayRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		response, err := base.RoundTrip(req)
		if req.Method == http.MethodPost {
			close(started)
			<-release
			close(done)
		}
		return response, err
	})
	f.r.evaluateChampSelect(f.c, f.r.currentWatch().ChampSelect)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("bench write never started")
	}
	f.r.evaluateChampSelect(f.c, f.r.currentWatch().ChampSelect)
	if f.r.champSelectSnapshot().PickStates["5"] == "manual-takeover" {
		t.Fatal("own echo yielded")
	}
	f.mu.Lock()
	f.session.MyTeam[0].ChampionID = 141
	f.session.BenchChampions = []lcuChampSelectBenchChampion{{ChampionID: 5}}
	f.mu.Unlock()
	f.r.evaluateChampSelect(f.c, f.r.currentWatch().ChampSelect)
	f.assertTakeover(t, "5")
	once.Do(func() { close(release) })
	<-done
	f.tick(t)
	f.assertTakeover(t, "5")
	if f.count() != 1 {
		t.Fatal("late acknowledgement revived a swap")
	}

	// Lower-priority configured champion -> higher-priority automatic swap.
	upgrade := newBenchTakeoverFixture(t)
	upgrade.session.MyTeam[0].ChampionID = 804
	upgrade.tick(t)
	upgrade.tick(t)
	if upgrade.count() != 1 || upgrade.r.champSelectSnapshot().PickStates["804"] == "manual-takeover" {
		t.Fatal("automatic priority upgrade yielded")
	}
}

func waitTakeoverIdle(t *testing.T, f *executionFixture) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		f.r.mu.Lock()
		idle := len(f.r.pending) == 0 && len(f.r.champSelect.inFlight) == 0
		f.r.mu.Unlock()
		if idle {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("request did not finish")
}

func prepareTakeoverPick(t *testing.T, group string, queue int64, lockDelay *int) *executionFixture {
	f := newExecutionFixture(t)
	f.session.QueueID = queue
	f.session.Timer.Phase = "BAN_PICK"
	f.session.Actions = [][]lcuChampSelectAction{{{ID: 5, Type: "pick", ActorCellID: 7, IsInProgress: true}}}
	s := f.r.currentWatch()
	g := s.ChampSelect.Groups[group]
	g.Pick.Enabled, g.Pick.DelayMS, g.Pick.LockDelayMS = true, 0, lockDelay
	g.Pick.Champions["default"] = []int64{5, 804}
	s.ChampSelect.Groups[group] = g
	f.r.apply(s)
	return f
}

func TestTakeoverPickTenSecondsAfterHoverAndManualChangeYields(t *testing.T) {
	for group, queue := range map[string]int64{"ranked": 420, "normal": 400, "arena": 1700} {
		t.Run(group, func(t *testing.T) {
			f := prepareTakeoverPick(t, group, queue, nil)
			f.tick(t) // Initial hover is prompt; the ten seconds is not before hover.
			if f.count() != 1 || f.last().Body["completed"] != false {
				t.Fatal("initial hover missing")
			}
			f.r.evaluateChampSelect(f.c, f.r.currentWatch().ChampSelect)
			f.r.mu.Lock()
			first := f.r.champSelect.decision[champSelectActionPick]
			f.r.mu.Unlock()
			for i := 0; i < 3; i++ {
				f.r.evaluateChampSelect(f.c, f.r.currentWatch().ChampSelect)
			}
			f.r.mu.Lock()
			same := f.r.champSelect.decision[champSelectActionPick].TraceID == first.TraceID
			f.r.mu.Unlock()
			if !same || !first.Completed || f.count() != 1 {
				t.Fatal("polling reset the lock or locked early")
			}
			f.mu.Lock()
			delay := -1
			for _, e := range f.events {
				if e["stage"] == "schedule" && e["reason"] == "armed" && e["delay_ms"] != nil {
					if n, ok := e["delay_ms"].(int); ok && n > delay {
						delay = n
					}
				}
			}
			f.mu.Unlock()
			if delay < 9900 || delay > 10000 {
				t.Fatalf("lock delay = %d", delay)
			}
			f.mu.Lock()
			f.session.Actions[0][0].ChampionID = 804 // Even a different configured champion is manual.
			f.mu.Unlock()
			f.tick(t)
			f.assertTakeover(t, "5")
			f.mu.Lock()
			f.session.Actions[0][0].ChampionID = 5 // Returning manually does not rearm the tool.
			f.mu.Unlock()
			f.tick(t)
			if f.count() != 1 {
				t.Fatal("manual choice overridden")
			}
		})
	}
}

func TestTakeoverPickLockTimerActuallyWaitsAndPreflightLatches(t *testing.T) {
	for _, manual := range []bool{false, true} {
		t.Run(map[bool]string{false: "lock", true: "manual-before-lock"}[manual], func(t *testing.T) {
			delay := 120
			f := prepareTakeoverPick(t, "arena", 1700, &delay)
			f.tick(t)
			start := time.Now()
			f.r.evaluateChampSelect(f.c, f.r.currentWatch().ChampSelect)
			if manual {
				f.mu.Lock()
				f.session.Actions[0][0].ChampionID = 804
				f.mu.Unlock()
			}
			waitTakeoverIdle(t, f)
			if time.Since(start) < 100*time.Millisecond {
				t.Fatal("lock did not wait after hover")
			}
			if manual {
				f.assertTakeover(t, "5")
				if f.count() != 1 {
					t.Fatal("manual hover got locked")
				}
			} else if f.count() != 2 || f.last().Body["completed"] != true {
				t.Fatal("unchanged hover did not lock")
			}
		})
	}
}

func TestTakeoverLockDelayMigrationAndTimeAnchor(t *testing.T) {
	var old champSelectSettings
	if err := json.Unmarshal([]byte(`{"enabled":true,"groups":{"normal":{"pick":{"enabled":true,"strategy":"show-then-lock","delayMs":500,"champions":{"default":[5]}}}}}`), &old); err != nil {
		t.Fatal(err)
	}
	s := normalizeChampSelectSettings(old)
	if champSelectLockDelay(s.Groups["normal"].Pick) != 10000 || s.Groups["normal"].Pick.DelayMS != 500 {
		t.Fatal("old settings did not gain separate ten-second lock wait")
	}
	copy := cloneChampSelectSettings(s)
	*copy.Groups["normal"].Pick.LockDelayMS = 3000
	if *s.Groups["normal"].Pick.LockDelayMS != 10000 {
		t.Fatal("lock delay clone aliases live configuration")
	}
	now := time.Now()
	hover := champSelectSubmitRecord{ConfirmedAt: now.Add(-4 * time.Second)}
	if got := champSelectHoverLockDelay(s.Groups["normal"].Pick, hover, now); got != 6000 {
		t.Fatalf("remaining wait=%d", got)
	}
	hover.Decision.Intent = true
	if got := champSelectHoverLockDelay(s.Groups["normal"].Pick, hover, now); got != 10000 {
		t.Fatal("planning time consumed active pick window")
	}
}

func TestTakeoverDefaultPickWaitsTenRealSecondsBeforeLock(t *testing.T) {
	f := prepareTakeoverPick(t, "arena", 1700, nil)
	f.tick(t)
	// Even a short client timer must not silently shorten the player's window.
	f.mu.Lock()
	f.session.Timer.AdjustedTimeLeftInPhase = 1000
	f.mu.Unlock()
	start := time.Now()
	f.r.evaluateChampSelect(f.c, f.r.currentWatch().ChampSelect)
	deadline := start.Add(12 * time.Second)
	for time.Now().Before(deadline) {
		if f.count() == 2 {
			if time.Since(start) < 10*time.Second || f.last().Body["completed"] != true {
				t.Fatal("default lock fired early")
			}
			waitTakeoverIdle(t, f)
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("default lock did not fire")
}
