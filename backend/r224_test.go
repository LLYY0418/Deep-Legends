package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func r224PickFixture(t *testing.T, pool []int64, arena bool, strategy string) *executionFixture {
	f := newExecutionFixture(t)
	f.configure(false, true, true, strategy)
	settings := f.r.currentWatch()
	group := settings.ChampSelect.Groups["practice"]
	group.Pick.Champions["default"] = pool
	group.Ban.Enabled = false
	groupID := "practice"
	if arena {
		groupID = "arena"
		f.session.QueueID = 1750
	}
	settings.ChampSelect.Groups[groupID] = group
	f.r.apply(settings)
	f.session.Actions[0][0].Completed = true
	return f
}
func r224StartPick(f *executionFixture) {
	f.session.Timer.Phase = "BAN_PICK"
	f.session.Actions[1][0].IsInProgress = true
}

func TestR224BraverySkipsPlanningThenExecutesStrategies(t *testing.T) {
	for _, strategy := range []string{"lock-now", "show-then-lock", "show-only"} {
		t.Run(strategy, func(t *testing.T) {
			f := r224PickFixture(t, []int64{-3}, true, strategy)
			for i := 0; i < 3; i++ {
				f.tick(t)
			}
			if f.count() != 0 {
				t.Fatal("bravery planning intent sent")
			}
			r224StartPick(f)
			f.tick(t)
			if f.count() != 1 || f.last().Body["championId"] != float64(-3) || f.last().Body["completed"] != (strategy == "lock-now") {
				t.Fatal(f.patches)
			}
			f.tick(t)
			if strategy == "show-then-lock" {
				if f.count() != 2 || f.last().Body["completed"] != true {
					t.Fatal(f.patches)
				}
				f.tick(t)
			}
			if strategy == "show-only" && f.count() != 1 {
				t.Fatal("show-only locked")
			}
		})
	}
}
func TestR224IntentFailureDoesNotConsumeLockBudget(t *testing.T) {
	f := r224PickFixture(t, []int64{5}, false, "lock-now")
	f.applyWrites = false
	f.tick(t)
	f.age()
	f.tick(t)
	f.age()
	f.tick(t)
	if f.count() != 2 {
		t.Fatal(f.patches)
	}
	r224StartPick(f)
	f.tick(t)
	if f.count() != 3 || f.last().Body["completed"] != true {
		t.Fatal("lock blocked by intent", f.patches)
	}
	f.age()
	f.tick(t)
	f.age()
	f.tick(t)
	if f.count() != 4 {
		t.Fatal("lock lacks independent two attempts", f.patches)
	}
	stops := 0
	for _, rec := range f.r.champSelectSnapshot().Records {
		if strings.Contains(rec.Message, "自动选用已停止") {
			stops++
		}
	}
	if stops != 1 {
		t.Fatalf("stops=%d", stops)
	}
	for i := 0; i < 4; i++ {
		f.tick(t)
	}
	stops = 0
	for _, rec := range f.r.champSelectSnapshot().Records {
		if strings.Contains(rec.Message, "自动选用已停止") {
			stops++
		}
	}
	if stops != 1 || f.r.champSelectSnapshot().PickStates["5"] != "stopped" {
		t.Fatal("stop not idempotent/visible")
	}
}
func TestR224SingleCandidateFailureExplainsStopAndResetsOnDodge(t *testing.T) {
	f := r224PickFixture(t, []int64{-3}, true, "lock-now")
	f.applyWrites = false
	r224StartPick(f)
	f.tick(t)
	f.age()
	f.tick(t)
	f.age()
	f.tick(t)
	found := false
	for _, rec := range f.r.champSelectSnapshot().Records {
		found = found || strings.Contains(rec.Message, "第 2/2 次）：已停止尝试，请手动操作")
	}
	if !found || f.count() != 2 || f.r.champSelectSnapshot().PickStates["-3"] != "stopped" {
		t.Fatal(f.r.champSelectSnapshot())
	}
	old := f.r.champSelect.runtimeID
	f.session.GameID++
	f.tick(t)
	if f.r.champSelect.runtimeID == old || f.count() != 3 || len(f.r.champSelect.pickFailures) != 0 {
		t.Fatal("new session inherited failures")
	}
}
func TestR224BraveryRandomChampionConfirmsCompletedOnly(t *testing.T) {
	for _, completed := range []bool{true, false} {
		t.Run(fmt.Sprint(completed), func(t *testing.T) {
			f := r224PickFixture(t, []int64{-3}, true, "lock-now")
			f.applyWrites = false
			r224StartPick(f)
			f.tick(t)
			f.age()
			f.session.Actions[1][0].ChampionID = 804
			f.session.Actions[1][0].Completed = completed
			f.tick(t)
			if completed {
				for _, event := range f.events {
					if event["stage"] == "postflight" && event["reason"] == "not-applied-after-2s" {
						t.Fatal("random completion reported as failed postflight")
					}
				}
			}
			if completed && f.r.champSelect.takeover["pick"] {
				t.Fatal("random completion mistaken for manual takeover")
			}
			if f.r.champSelect.submitted[5].Confirmed != completed {
				t.Fatal("random confirmation ignores completion")
			}
		})
	}
}
func TestR224RosterDropsStaleArenaTeamTwoAndDuplicateRefs(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(fmt.Sprint(missing), func(t *testing.T) {
			players, raw := r62ArenaFixturePlayers(t, 18)
			if missing {
				players = players[:17]
			}
			a := r90Fixture(t, players, raw)
			base := a.lcu.http.Transport
			a.lcu.http.Transport = gameplayRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.URL.Path == "/lol-gameflow/v1/session" {
					return response2351(map[string]any{"gameData": map[string]any{"gameId": 90001, "queue": map[string]any{"id": 1750, "gameMode": "CHERRY", "mapId": 30}, "teamOne": players, "teamTwo": []lcuLivePlayer{{PUUID: a.summoner.PUUID, ChampionID: 999}, {PUUID: "stale-2"}, {PUUID: "stale-3"}, {PUUID: "stale-4"}, {PUUID: "stale-5"}}}}), nil
				}
				return base.RoundTrip(req)
			})
			a.liveClientAllGameData = func(context.Context) ([]byte, int, error) { return []byte(`{"allPlayers":[]}`), 200, nil }
			result := a.loadGameplayLive(context.Background(), a.lcu, a.summoner, "InProgress")
			if len(result.Players) != 18 {
				t.Fatalf("players=%d", len(result.Players))
			}
			self := 0
			for _, p := range result.Players {
				if p.TeamID != 100 || p.ChampionID == 999 {
					t.Fatal("stale entry", p)
				}
				if p.IsCurrent {
					self++
				}
			}
			if self != 1 {
				t.Fatalf("self=%d", self)
			}
			events := r90Events(t, a, "stale_team_two_dropped")
			if len(events) != 1 || events[0]["count"] != float64(5) || len(events[0]) > 10 {
				t.Fatal(events)
			}
			if missing && result.MergeAppended != 1 {
				t.Fatal("playerlist did not fill missing player")
			}
		})
	}
	var session lcuGameflowSession
	for i := 0; i < 5; i++ {
		session.GameData.TeamOne = append(session.GameData.TeamOne, lcuLivePlayer{PUUID: fmt.Sprintf("one-%d", i)})
		session.GameData.TeamTwo = append(session.GameData.TeamTwo, lcuLivePlayer{PUUID: fmt.Sprintf("two-%d", i)})
	}
	session.GameData.Queue.ID = 420
	session.GameData.Queue.GameMode = "CLASSIC"
	rows, stale, duplicates := gameflowLiveRoster(session)
	if len(rows) != 10 || stale != 0 || duplicates != 0 {
		t.Fatal(rows, stale, duplicates)
	}
	session.GameData.TeamTwo[0] = session.GameData.TeamOne[0]
	rows, _, duplicates = gameflowLiveRoster(session)
	if len(rows) != 9 || duplicates != 1 || rows[0].team != 100 {
		t.Fatal(rows, duplicates)
	}
}
