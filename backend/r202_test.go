package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func r202Bench(t *testing.T, subset bool) *r200Fixture {
	v := newR200Fixture(t, `[43,76,51]`)
	v.benchSetup()
	v.session.AllowSubsetChampionPicks = subset
	v.session.MyTeam[0].ChampionID = 76
	v.session.BenchChampions = []lcuChampSelectBenchChampion{{ChampionID: 57}}
	return v
}
func TestR202CardBenchWaitsForFinalization(t *testing.T) {
	v := r202Bench(t, true)
	v.session.Timer.Phase = "BAN_PICK"
	v.tick(t)
	if v.count() != 0 {
		t.Fatal("BAN_PICK sent swap", v.patches)
	}
	v.requireTrace(t, "bench-gate", "waiting-finalization")
	v.session.Timer.Phase = "FINALIZATION"
	v.tick(t)
	v.tick(t)
	if v.count() != 1 || !strings.HasSuffix(v.last().Path, "/57") {
		t.Fatal(v.patches)
	}
	v.requireTrace(t, "bench-postflight", "applied")
}
func TestR202PreFinalFailuresRefunded(t *testing.T) {
	// Ordinary ARAM still sends during BAN_PICK; unapplied writes get a fresh
	// budget at FINALIZATION, including legacy card writes already in flight.
	for _, subset := range []bool{false, true} {
		t.Run(fmt.Sprint(subset), func(t *testing.T) {
			v := r202Bench(t, false)
			v.applyWrites = false
			v.session.Timer.Phase = "BAN_PICK"
			v.tick(t)
			v.ageBench()
			v.tick(t)
			v.ageBench()
			v.tick(t)
			if v.count() != 2 {
				t.Fatal("fixture did not exhaust BAN_PICK", v.patches)
			}
			v.session.AllowSubsetChampionPicks = subset
			v.session.Timer.Phase = "FINALIZATION"
			v.tick(t)
			if v.count() != 3 {
				t.Fatal("BAN_PICK failures consumed final budget", v.patches)
			}
			v.ageBench()
			v.tick(t)
			v.ageBench()
			v.tick(t)
			v.tick(t)
			if v.count() != 4 {
				t.Fatal("final budget must remain two", v.patches)
			}
		})
	}
}
func TestR202FirstCardDoesNotYieldButPoolHeroChangeDoes(t *testing.T) {
	for _, first := range []int64{76, 57} {
		t.Run(fmt.Sprint(first), func(t *testing.T) {
			v := r202Bench(t, true)
			v.session.Timer.Phase = "BAN_PICK"
			v.session.MyTeam[0].ChampionID = 0
			v.tick(t)
			v.session.MyTeam[0].ChampionID = first
			v.session.Actions = [][]lcuChampSelectAction{{{ID: 5, Type: "pick", ActorCellID: 7, ChampionID: first, Completed: true}}}
			v.tick(t)
			if v.r.champSelectSnapshot().PickStates["57"] == "manual-takeover" {
				t.Fatal("first card yielded")
			}
			v.session.Timer.Phase = "FINALIZATION"
			v.tick(t)
			v.tick(t)
			if first == 76 && v.count() != 1 {
				t.Fatal("initial card blocked bench", v.patches)
			}
			if first == 57 && v.count() != 0 {
				t.Fatal("held pool hero swapped")
			}
			v.session.MyTeam[0].ChampionID = 51
			v.session.Actions = nil
			v.tick(t)
			if !v.r.champSelect.takeover["bench"] {
				t.Fatal("change away from pool did not yield")
			}
		})
	}
}
func TestR202BenchHolderPriority(t *testing.T) {
	pool := []int64{22, 136, 48, 57, 13}
	bench := map[int64]lcuChampSelectBenchChampion{22: {ChampionID: 22}, 57: {ChampionID: 57}, 13: {ChampionID: 13}}
	if got := champSelectBenchTarget(pool, bench, 136, false); got != 0 {
		t.Fatal("disabled preference swapped held hero", got)
	}
	if got := champSelectBenchTarget(pool, bench, 136, true); got != 22 {
		t.Fatal(got)
	}
	for _, prefer := range []bool{false, true} {
		delete(bench, 22)
		if got := champSelectBenchTarget(pool, bench, 76, prefer); got != 57 {
			t.Fatal(got)
		}
	}
}
func TestR202CameraSnapshotUsesPersistedOnly(t *testing.T) {
	v := newR198Fixture(t)
	v.apply("champselect")
	delete(v.f.lcu["General"], "CameraMode")
	s := &v.f.a.gameSettingsWatch
	s.client = v.f.client
	s.ctx = context.Background()
	s.generation = 1
	s.prev = map[string]gameSettingsFileSnapshot{}
	v.phase.Store("InProgress")
	for _, targetOK := range []bool{true, false} {
		if !targetOK {
			os.WriteFile(v.f.location.file, []byte(r198JSON), 0600)
		}
		// game.cfg disagrees deliberately; it must not be the comparison source.
		os.WriteFile(filepath.Join(v.f.location.configRoot, "game.cfg"), []byte(r198INI), 0600)
		v.f.a.recordGameSettingsWatch(context.Background(), v.f.client, gameSettingsWatchJob{stage: "in_game_60s", generation: 1})
		events := r175Events(t, v.f.a, "game_settings_watch")
		if events[len(events)-1]["camera_mode_matches_target"] != targetOK {
			t.Fatal(events)
		}
	}
}

func TestR202LiveHistoryWindowAndPages(t *testing.T) {
	for _, tc := range []struct {
		name        string
		filtered    bool
		perPage     []int
		oldPage     int
		want, pages int
		stop        string
	}{
		{"filtered", true, []int{10}, 0, 10, 1, "enough"},
		{"filtered-old", true, []int{10}, 1, 7, 1, "window"},
		{"fallback-enough", false, []int{2, 5, 4}, 0, 10, 3, "enough"},
		{"fallback-window", false, []int{2, 5, 4}, 2, 4, 2, "window"},
		{"fallback-limit", false, []int{1, 1, 1, 1}, 0, 4, 4, "page_limit"},
		{"fallback-exhausted", false, []int{2}, 0, 2, 1, "exhausted"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := r180Fixture(t)
			ref := r161Ref(1)
			calls := 0
			if !tc.filtered {
				f.a.setQueueFilterCapability("HN1", "solo:q_420", queueFilterCapabilityUnsupported)
			}
			f.a.sgp.http = &http.Client{Transport: sgpRoundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				offset, _ := strconv.Atoi(r.URL.Query().Get("startIndex"))
				page := offset / 30
				if page >= len(tc.perPage) {
					t.Fatal("extra page", r.URL)
				}
				if tc.filtered {
					if r.URL.Query().Get("tag") != "q_420" || r.URL.Query().Get("count") != "10" {
						t.Fatal(r.URL)
					}
				} else if r.URL.Query().Get("tag") != "" || r.URL.Query().Get("count") != "30" {
					t.Fatal(r.URL)
				}
				count := 30
				if tc.filtered {
					count = 10
				}
				if tc.stop == "exhausted" {
					count = 5
				}
				games := []any{}
				for i := 0; i < count; i++ {
					q := int64(440)
					if i < tc.perPage[page] {
						q = 420
					}
					g := f.sgpGame(int64(page*30+i+1), q, ref)
					g["gameCreation"] = f.now - int64(page*30+i)*60000
					if tc.oldPage == page+1 && i >= tc.perPage[page]-3 {
						g["gameCreation"] = f.now - int64(31*24*time.Hour/time.Millisecond)
					}
					games = append(games, map[string]any{"json": g})
				}
				return r178JSON(map[string]any{"games": games}, 200), nil
			})}
			result, ok := f.a.loadLiveSGPMatches(context.Background(), f.c, gameplayReference{ServerID: "HN1"}, ref, nil, 420)
			shown := recentLiveMatchesForPlayer(result.Matches, ref, 420, time.Now())
			if !ok || len(shown) != tc.want || calls != tc.pages || result.Evidence.PagesRead != tc.pages || result.Evidence.QueueFiltered != tc.filtered || result.Evidence.StopReason != tc.stop {
				t.Fatal(len(shown), calls, result.Evidence, ok)
			}
			e := liveHistoryFreshnessDiagnostic(livePlayerMatchesResult{Matches: result.Matches, Evidence: &liveHistoryEvidence{SGPRequested: true, SGPOK: true, QueueFiltered: result.Evidence.QueueFiltered, PagesRead: result.Evidence.PagesRead, StopReason: result.Evidence.StopReason}}, ref, 420, 100, 0, false, time.Now())
			if e["shown_count"] != tc.want || e["window_days"] != 30 || e["stop_reason"] != tc.stop {
				t.Fatal(e)
			}
		})
	}
}
func TestR202LiveHistoryCacheSeparatesQueue(t *testing.T) {
	f := r180Fixture(t)
	ref := r161Ref(1)
	f.a.livePlayerMatches(context.Background(), f.c, gameplayReference{ServerID: "HN1"}, ref, false, nil, 420)
	first := f.sgpCalls.Load()
	f.a.livePlayerMatches(context.Background(), f.c, gameplayReference{ServerID: "HN1"}, ref, false, nil, 2400)
	if f.sgpCalls.Load() <= first {
		t.Fatal("different queue reused cache")
	}
}

func TestR202RerollKeepsHeldPoolEvidence(t *testing.T) {
	v := r202Bench(t, true)
	v.session.MyTeam[0].ChampionID = 57
	v.tick(t)
	v.session.MyTeam[0].ChampionID = 0
	v.tick(t)
	v.session.MyTeam[0].ChampionID = 76
	v.tick(t)
	if !v.r.champSelect.takeover["bench"] {
		t.Fatal("reroll away from held pool did not yield")
	}
}
func TestR202SoftwareFirstCardThenBench(t *testing.T) {
	v := newR200Fixture(t, `[107,141,75]`)
	v.grid = append(v.grid, champSelectGridChampion{ID: 107, Owned: true})
	v.tick(t)
	v.tick(t)
	if v.count() != 1 {
		t.Fatal(v.patches)
	}
	v.session.MyTeam[0].ChampionID = 107
	v.session.BenchChampions = []lcuChampSelectBenchChampion{{ChampionID: 57}}
	v.session.Timer.Phase = "FINALIZATION"
	v.r.mu.Lock()
	v.r.champSelect.benchFirstSeen[57] = time.Now().Add(-2 * time.Second)
	v.r.mu.Unlock()
	v.tick(t)
	v.tick(t)
	if v.count() != 2 || !strings.HasSuffix(v.last().Path, "/57") {
		t.Fatal("software first card blocked bench", v.patches, v.events)
	}
	v.requireTrace(t, "bench-postflight", "applied")
}
func TestR202DelayedBenchRechecksPhase(t *testing.T) {
	v := r202Bench(t, true)
	settings := v.r.currentWatch()
	g := settings.ChampSelect.Groups["aram"]
	g.Bench.HoldMS = 1000
	settings.ChampSelect.Groups["aram"] = g
	v.r.apply(settings)
	v.r.mu.Lock()
	v.r.champSelect.benchFirstSeen[57] = time.Now().Add(-850 * time.Millisecond)
	v.r.mu.Unlock()
	v.r.evaluateChampSelect(v.c, v.r.currentWatch().ChampSelect)
	v.mu.Lock()
	v.session.Timer.Phase = "BAN_PICK"
	v.mu.Unlock()
	waitTakeoverIdle(t, v.executionFixture)
	if v.count() != 0 {
		t.Fatal("delayed bench crossed into card phase", v.patches)
	}
}
