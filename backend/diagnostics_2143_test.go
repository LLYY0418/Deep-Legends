package main

import (
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func wait2143Evaluation(t *testing.T, runner *watchRunner) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		runner.mu.Lock()
		busy := runner.champSelect.processing
		runner.mu.Unlock()
		if !busy {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("evaluation did not finish")
}

func Test2143CustomBanAvailabilityUpdatesAndPickWithWatchOff(t *testing.T) {
	f := newR78ChampSelectFixture(t, "practice", "ban", 0, []int64{141, 61}, map[int64]champSelectGridSelectionStatus{141: {}, 61: {}})
	f.session.QueueID = 3110
	f.runner.setCustomSession(true)
	f.enable("practice", "ban", []int64{141})
	f.enable("practice", "pick", []int64{61})
	s := f.runner.currentWatch()
	s.MasterEnabled = false
	g := s.ChampSelect.Groups["practice"]
	g.Ban.Strategy, g.Pick.Strategy = "lock-now", "lock-now"
	s.ChampSelect.Groups["practice"] = g
	f.runner.apply(s)
	var ready atomic.Bool
	transport := f.client.http.Transport
	f.client.http.Transport = gameplayRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if strings.HasSuffix(req.URL.Path, "bannable-champion-ids") && !ready.Load() {
			return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`[]`))}, nil
		}
		return transport.RoundTrip(req)
	})
	f.runner.handleChampSelectAutomation(f.client)
	wait2143Evaluation(t, f.runner)
	if f.patches.Load() != 0 {
		t.Fatal("submitted without client availability")
	}
	if f.runner.champSelectSnapshot().BanStates["141"] != "list-empty" {
		t.Fatal("wrong unavailable state")
	}
	ready.Store(true)
	if !isChampSelectAutomationEvent("/lol-champ-select/v1/bannable-champion-ids") {
		t.Fatal("availability event ignored")
	}
	f.runner.handleChampSelectAutomation(f.client)
	ban := f.waitPatch(t)
	wait2143Evaluation(t, f.runner)
	if ban.Type != "ban" || ban.ChampionID != 141 || !ban.Completed {
		t.Fatalf("ban=%+v", ban)
	}
	// Next client-authorized action; never pick early merely because ban exhausted.
	f.session.Actions[0][0] = lcuChampSelectAction{ID: 43, ActorCellID: 7, Type: "pick", IsInProgress: true}
	f.runner.handleChampSelectAutomation(f.client)
	pick := f.waitPatch(t)
	wait2143Evaluation(t, f.runner)
	if pick.Type != "pick" || pick.ChampionID != 61 || !pick.Completed {
		t.Fatalf("pick=%+v", pick)
	}
	if !f.runner.customPaused() {
		t.Fatal("champselect resumed watch automation")
	}
}

func Test2143TeammateChangingHoverReleasesPreviousCandidate(t *testing.T) {
	f := newR78ChampSelectFixture(t, "practice", "ban", 0, []int64{141}, map[int64]champSelectGridSelectionStatus{141: {}})
	f.enable("practice", "ban", []int64{141})
	cell := int64(8)
	f.session.MyTeam = append(f.session.MyTeam, lcuChampSelectPlayer{CellID: &cell, ChampionPickIntent: 141})
	f.runner.evaluateChampSelect(f.client, f.runner.currentWatch().ChampSelect)
	if f.patches.Load() != 0 {
		t.Fatal("banned teammate hover")
	}
	f.session.MyTeam[1].ChampionPickIntent = 0
	if got := f.waitAfterEvaluate(t); got.ChampionID != 141 {
		t.Fatalf("hover retained: %+v", got)
	}
}

func Test2143OwnHoverEventBeforeHTTPAckDoesNotYield(t *testing.T) {
	f := newR78ChampSelectFixture(t, "normal", "pick", 61, []int64{61}, map[int64]champSelectGridSelectionStatus{61: {}})
	f.enable("normal", "pick", []int64{61})
	f.runner.mu.Lock()
	f.runner.champSelect.inFlight[42] = true
	f.runner.champSelect.decision[champSelectActionPick] = champSelectDecision{ActionID: 42, ChampionID: 61}
	f.runner.mu.Unlock()
	f.runner.evaluateChampSelect(f.client, f.runner.currentWatch().ChampSelect)
	f.runner.mu.Lock()
	yielded := f.runner.champSelect.takeover["pick"]
	f.runner.mu.Unlock()
	if yielded {
		t.Fatal("our pending hover was treated as user takeover")
	}
}

func Test2143EarlierUnfinishedInactiveBanDoesNotHideActivePick(t *testing.T) {
	f := newR78ChampSelectFixture(t, "practice", "pick", 0, []int64{61}, map[int64]champSelectGridSelectionStatus{61: {}})
	f.enable("practice", "pick", []int64{61})
	f.session.Actions = append([][]lcuChampSelectAction{{{ID: 1, ActorCellID: 7, Type: "ban", Completed: false, IsInProgress: false}}}, f.session.Actions...)
	if got := f.waitAfterEvaluate(t); got.Type != "pick" || got.ChampionID != 61 {
		t.Fatalf("pick=%+v", got)
	}
	f.session.Actions[0][0].IsInProgress = true
	if _, ok := firstLocalChampSelectAction(*f.session); ok {
		t.Fatal("ambiguous simultaneous local actions accepted")
	}
}
