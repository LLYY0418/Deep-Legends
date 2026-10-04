package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestR204PartialInstallTiming(t *testing.T) {
	for _, kind := range []string{"partial", "complete", "parse", "skew"} {
		t.Run(kind, func(t *testing.T) {
			stages := map[string]any{}
			for i, key := range updateInstallTimingStages {
				stages[key] = int64(1000 + i*100)
			}
			if kind == "partial" {
				stages["parent_exited"] = nil
			}
			if kind == "skew" {
				stages["uninstall_old_done"] = int64(1001)
			}
			raw, _ := json.Marshal(stages)
			if kind == "parse" {
				raw = []byte("{")
			}
			root := t.TempDir()
			os.WriteFile(filepath.Join(root, "update-install-timing.json"), raw, 0600)
			var e map[string]any
			consumeUpdateInstallTiming(root, func(v map[string]any) { e = v })
			switch kind {
			case "partial":
				if e["result"] != "partial" || e["reason"] != "missing_fields" || e["uninstall_old_ms"] != int64(100) || e["extract_ms"] != int64(100) || e["stages_ms"].(map[string]any)["installer_start_to_parent_exited"] != nil {
					t.Fatal(e)
				}
			case "complete":
				if e["result"] != "ok" {
					t.Fatal(e)
				}
			case "parse":
				if e["reason"] != "parse_error" {
					t.Fatal(e)
				}
			case "skew":
				if e["reason"] != "clock_skew" {
					t.Fatal(e)
				}
			}
		})
	}
}
func TestR204ConfirmedPickThenHeroChangeNoFalseFailure(t *testing.T) {
	f := newR78ChampSelectFixture(t, "practice", "pick", 0, []int64{5}, map[int64]champSelectGridSelectionStatus{5: {}})
	f.runner.champSelect.submitted[42] = champSelectSubmitRecord{ChampionID: 5, Completed: true, Confirmed: true, At: time.Now().Add(-3 * time.Second)}
	f.runner.observe = func(e map[string]any) {
		if e["stage"] == "postflight" && e["reason"] == "not-applied-after-2s" {
			t.Fatal("confirmed selection misreported", e)
		}
	}
	f.session.Actions[0][0].ChampionID = 13
	f.runner.recordChampSelectPostflight(*f.session)
	f.session.Actions = nil
	f.runner.recordChampSelectPostflight(*f.session)
	for _, file := range []string{"champselect.go", "champselect_execution.go"} {
		data, _ := os.ReadFile(file)
		if strings.Contains(string(data), "champSelectSubsetTestFirstCard") || strings.Contains(string(data), "test-first-card") {
			t.Fatal("temporary test behavior remains")
		}
	}
}
func TestR204CustomHistoryOnePage(t *testing.T) {
	for _, queue := range []int64{3140, 3100, 3110, 830} {
		f := r180Fixture(t)
		ref := r161Ref(1)
		calls := 0
		f.a.sgp.http = &http.Client{Transport: sgpRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			calls++
			games := []any{}
			for i := int64(1); i <= 30; i++ {
				games = append(games, map[string]any{"json": f.sgpGame(i, 440, ref)})
			}
			return r178JSON(map[string]any{"games": games}, 200), nil
		})}
		result, ok := f.a.loadLiveSGPMatches(context.Background(), f.c, gameplayReference{ServerID: "HN1"}, ref, nil, queue)
		if !ok || result.Evidence.PagesRead != 1 || calls > 2 {
			t.Fatal(queue, result.Evidence, calls)
		}
	}
}
func TestR204EnoughSGPSkipsLCUExceptSelf(t *testing.T) {
	for _, self := range []bool{false, true} {
		f := r180Fixture(t)
		ref := r161Ref(1)
		if self {
			ref = f.a.summoner.PUUID
		}
		result := f.a.loadLivePlayerMatches(context.Background(), f.c, gameplayReference{PlayerRef: ref, ServerID: "HN1"}, ref, self, nil, 440)
		if len(recentLiveMatchesForPlayer(result.Matches, ref, 440, time.Now())) != 10 {
			t.Fatal("ten usable games missing")
		}
		if !self && f.lcuCalls.Load() != 0 || self && f.lcuCalls.Load() != 1 {
			t.Fatal("LCU request mismatch", self, f.lcuCalls.Load())
		}
		if result.Evidence == nil || result.Evidence.SGPMS < 0 || result.Evidence.LCUMS < 0 {
			t.Fatal("missing timing evidence")
		}
	}
}
func TestR204LiveProgressReplayAndCancellation(t *testing.T) {
	a := &app{}
	client := &LCUClient{}
	self := Summoner{PUUID: r161Ref(0)}
	blocked := make(chan struct{})
	started := make(chan struct{})
	done := make(chan struct{})
	a.gameplayLiveLoader = func(ctx context.Context, _ *LCUClient, _ Summoner, phase string) gameplayLiveResponse {
		ready := gameplayLiveResponse{Available: true, GameID: 204, Phase: phase, Players: []gameplayLivePlayer{{gameplayPlayer: gameplayPlayer{PlayerRef: "player-anonymous"}, HistoryState: "ok"}, {HistoryState: "pending"}}}
		publishLiveProgress(ctx, ready)
		close(started)
		<-blocked
		ready.Players[1].HistoryState = "ok"
		publishLiveProgress(ctx, ready)
		return ready
	}
	progress := make(chan gameplayLiveResponse, 4)
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), liveProgressListenerKey{}, func(r gameplayLiveResponse) { progress <- r }))
	defer cancel()
	go func() { a.cachedGameplayLive(ctx, client, self, "ChampSelect"); close(done) }()
	<-started
	select {
	case value := <-progress:
		if value.Players[0].HistoryState != "ok" || value.Players[1].HistoryState != "pending" {
			t.Fatal("did not publish early")
		}
	case <-time.After(time.Second):
		t.Fatal("waited for all players")
	}
	replay := make(chan gameplayLiveResponse, 2)
	joinCtx, joinCancel := context.WithCancel(context.WithValue(context.Background(), liveProgressListenerKey{}, func(r gameplayLiveResponse) { replay <- r }))
	joinDone := make(chan struct{})
	go func() { a.cachedGameplayLive(joinCtx, client, self, "ChampSelect"); close(joinDone) }()
	select {
	case <-replay:
	case <-time.After(time.Second):
		t.Fatal("joined flight did not replay")
	}
	joinCancel()
	<-joinDone
	a.liveSnapshots.invalidate()
	close(blocked)
	<-done
	if len(progress) != 0 || len(replay) != 0 {
		t.Fatal("cancelled flight published a late player")
	}
}
