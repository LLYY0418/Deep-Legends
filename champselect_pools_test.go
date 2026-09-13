package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func Test1129RankedBanMigrationPreservesLegacyAndPickPools(t *testing.T) {
	store := &localStore{root: t.TempDir()}
	s := defaultWatchSettings()
	s.ChampSelect.Enabled = true
	g := s.ChampSelect.Groups["ranked"]
	g.Ban.Enabled, g.Ban.DelayMS = true, 1750
	g.Ban.Champions = map[string][]int64{"default": {141, 56}, "top": {56, 11}, "jungle": {121, 104}, "middle": {5}, "bottom": {804}, "utility": {99}, "invalid": {666}}
	g.Pick.Champions["middle"], g.Pick.Champions["default"] = []int64{804, 5}, []int64{99}
	s.ChampSelect.Groups["ranked"] = g
	data, _ := json.Marshal(s)
	if err := os.WriteFile(filepath.Join(store.root, convenienceSettingsFile), data, 0600); err != nil {
		t.Fatal(err)
	}
	loaded := loadWatchSettings(store)
	got := loaded.ChampSelect.Groups["ranked"]
	if !slices.Equal(got.Ban.Champions["default"], []int64{141, 56, 11, 121, 104}) || len(got.Ban.Champions) != 1 {
		t.Fatalf("shared ban=%v", got.Ban.Champions)
	}
	if !reflect.DeepEqual(got.Pick, g.Pick) || !got.Ban.Enabled || got.Ban.DelayMS != 1750 {
		t.Fatalf("unrelated settings changed: %+v", got)
	}
	if len(got.Ban.LegacyLaneChampions) != 6 || !slices.Equal(got.Ban.LegacyLaneChampions["middle"], []int64{5}) {
		t.Fatalf("backup=%v", got.Ban.LegacyLaneChampions)
	}
	for i := 0; i < 3; i++ {
		if err := saveWatchSettings(store, loaded); err != nil {
			t.Fatal(err)
		}
		roundtrip := loadWatchSettings(store)
		if !reflect.DeepEqual(roundtrip, loaded) {
			t.Fatal("load/save migration was not idempotent")
		}
		loaded = roundtrip
	}
	clone := cloneChampSelectSettings(loaded.ChampSelect)
	clone.Groups["ranked"].Ban.LegacyLaneChampions["top"][0] = 999
	if loaded.ChampSelect.Groups["ranked"].Ban.LegacyLaneChampions["top"][0] != 56 {
		t.Fatal("backup was shallow cloned")
	}
	got.Ban.Champions["default"] = []int64{}
	loaded.ChampSelect.Groups["ranked"] = got
	if err := saveWatchSettings(store, loaded); err != nil {
		t.Fatal(err)
	}
	cleared := loadWatchSettings(store).ChampSelect.Groups["ranked"]
	if len(cleared.Ban.Champions["default"]) != 0 || len(cleared.Ban.LegacyLaneChampions) != 6 {
		t.Fatal("cleared shared pool refilled or backup lost")
	}
}

func Test1129RankedSharedBanAcrossAllPositionsAndQueues(t *testing.T) {
	for _, queue := range []int64{420, 440} {
		for _, lane := range []string{"top", "jungle", "middle", "bottom", "utility", "", "unknown"} {
			t.Run(fmt.Sprintf("%d/%s", queue, lane), func(t *testing.T) {
				f := newExecutionFixture(t)
				f.session.QueueID, f.session.MyTeam[0].AssignedPosition = queue, lane
				f.session.Timer.Phase, f.bans = "BAN_PICK", []int64{141, 104}
				s := f.r.currentWatch()
				g := s.ChampSelect.Groups["ranked"]
				g.Ban.Enabled, g.Ban.DelayMS, g.Ban.Strategy = true, 0, "show-only"
				g.Ban.Champions["default"] = []int64{141, 104}
				s.ChampSelect.Groups["ranked"] = g
				f.r.apply(s)
				f.tick(t)
				if p := f.last(); p.Body["championId"] != float64(141) || p.Body["type"] != "ban" {
					t.Fatalf("shared ban=%+v", p)
				}
				// Both execution and the last-moment preflight must use shared,
				// even if the assigned position changes before the write.
				f.r.mu.Lock()
				d := f.r.champSelect.submitted[0].Decision
				f.r.mu.Unlock()
				f.session.MyTeam[0].AssignedPosition = "utility"
				if !f.r.champSelectRequestStillCurrent(context.Background(), f.c, d) {
					t.Fatal("shared ban rejected after position changed")
				}
			})
		}
	}
}

func Test1129RankedDiagnosticsReportSeparatePoolPositions(t *testing.T) {
	f := newExecutionFixture(t)
	f.session.QueueID, f.session.MyTeam[0].AssignedPosition = 420, "middle"
	s := f.r.currentWatch()
	g := s.ChampSelect.Groups["ranked"]
	g.Ban.Champions["default"], g.Pick.Champions["middle"] = []int64{141, 104}, []int64{804}
	s.ChampSelect.Groups["ranked"] = g
	f.r.recordChampSelectEvaluation(f.session, "ranked", s.ChampSelect)
	for _, e := range f.events {
		if e["event"] == "champselect_evaluation" {
			if e["ban_pool_position"] != "default" || e["pick_pool_position"] != "middle" || e["ban_pool_size"] != 2 || e["pick_pool_size"] != 1 {
				t.Fatalf("pool diagnostics=%v", e)
			}
			return
		}
	}
	t.Fatal("missing pool diagnostics")
}

func Test1129SkipLogsDeduplicateWithoutHidingChangedReasons(t *testing.T) {
	f := newExecutionFixture(t)
	session := f.session
	pool := []int64{5, 804}
	skipped := map[int64]string{5: "teammate-picked"}
	for i := 0; i < 4; i++ {
		f.r.recordChampSelectCandidates(session, "pick", pool, 804, skipped, 4, 4)
	}
	count := func() int {
		n := 0
		for _, record := range f.r.champSelectSnapshot().Records {
			if strings.Contains(record.Message, "已顺延前") {
				n++
			}
		}
		return n
	}
	if count() != 1 {
		t.Fatalf("skip summaries=%d", count())
	}
	skipped[5] = "intent"
	f.r.recordChampSelectCandidates(session, "pick", pool, 804, skipped, 4, 4)
	if count() != 2 {
		t.Fatal("changed conflict reason was hidden")
	}
}

func Test1129SourceLogIgnoresOnlyRawOrderChanges(t *testing.T) {
	f := newExecutionFixture(t)
	count := func() int {
		n := 0
		for _, e := range f.events {
			if e["event"] == "champselect_source" {
				n++
			}
		}
		return n
	}
	raw := []int64{141, 104, -1}
	f.r.recordChampSelectSource(champSelectAPI, "test", f.session, f.picks, raw)
	f.r.recordChampSelectSource(champSelectAPI, "test", f.session, f.picks, []int64{-1, 104, 141})
	if count() != 1 || !slices.Equal(raw, []int64{141, 104, -1}) {
		t.Fatal("raw order caused duplicate log or was mutated")
	}
	f.r.recordChampSelectSource(champSelectAPI, "test", f.session, f.picks, []int64{-1, 804, 141})
	if count() != 2 {
		t.Fatal("same-count membership change was hidden")
	}
	f.session.Actions[0][0].ChampionID = 141
	f.r.recordChampSelectSource(champSelectAPI, "test", f.session, f.picks, []int64{-1, 804, 141})
	if count() != 3 {
		t.Fatal("action change was hidden")
	}
}

func Test1129WriteLogNamesSeparateIntentHoverAndLock(t *testing.T) {
	for _, tc := range []struct {
		d          champSelectDecision
		verb, step string
	}{
		{champSelectDecision{Action: champSelectActionPick, Intent: true}, "提前预选", "intent"},
		{champSelectDecision{Action: champSelectActionBan}, "亮出禁用", "hover"},
		{champSelectDecision{Action: champSelectActionBan, Completed: true}, "禁用并锁定", "lock"},
		{champSelectDecision{Action: champSelectActionPick, Completed: true}, "选用并锁定", "lock"},
	} {
		if champSelectDecisionVerb(tc.d) != tc.verb || champSelectDecisionStep(tc.d) != tc.step {
			t.Fatalf("decision=%+v", tc.d)
		}
	}
}
