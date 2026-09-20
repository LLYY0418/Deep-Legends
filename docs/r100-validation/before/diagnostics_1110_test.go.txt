package main

import (
	"testing"
	"time"
)

func Test1110CustomDraftUsesPracticePoolAndBans(t *testing.T) {
	fixture := newR78ChampSelectFixture(t, "practice", "ban", 0, []int64{141}, map[int64]champSelectGridSelectionStatus{141: {}})
	fixture.session.QueueID = 3110
	fixture.enable("practice", "ban", []int64{141})
	fixture.runner.setCustomSession(true)
	fixture.runner.handleChampSelectPhase("ChampSelect")
	request := fixture.waitAfterEvaluate(t)
	if request.ChampionID != 141 || request.Type != "ban" {
		t.Fatalf("custom draft ban = %#v", request)
	}
}

func Test1110CustomDraftNeverBorrowsNormalPool(t *testing.T) {
	fixture := newR78ChampSelectFixture(t, "practice", "ban", 0, []int64{141}, map[int64]champSelectGridSelectionStatus{141: {}})
	fixture.session.QueueID = 3110
	fixture.enable("normal", "ban", []int64{141})
	fixture.runner.evaluateChampSelect(fixture.client, fixture.runner.currentWatch().ChampSelect)
	time.Sleep(30 * time.Millisecond)
	if fixture.runner.champSelectSnapshot().GroupID != "practice" || fixture.patches.Load() != 0 {
		t.Fatal("custom draft was unsupported or borrowed normal pool")
	}
}

func Test1110RankedPickUsesOnlyAssignedLaneNotTopFirst(t *testing.T) {
	fixture := newR78ChampSelectFixture(t, "ranked", "pick", 0, []int64{1, 2, 3}, map[int64]champSelectGridSelectionStatus{1: {}, 2: {}, 3: {}})
	fixture.enableLane("ranked", "pick", "top", []int64{1})
	fixture.enableLane("ranked", "pick", "jungle", []int64{2})
	fixture.enableLane("ranked", "pick", "middle", []int64{3})
	if request := fixture.waitAfterEvaluate(t); request.ChampionID != 3 {
		t.Fatalf("middle player used wrong pool: %#v", request)
	}
}

func Test1110ARAMDefaultsAndSavedPreferences(t *testing.T) {
	settings := defaultChampSelectSettings()
	if settings.Enabled {
		t.Fatal("master must remain off")
	}
	for _, definition := range champSelectGroupDefinitions {
		group := settings.Groups[definition.GroupID]
		aram := definition.GroupID == "aram"
		if definition.HasBench != aram || group.Bench.Enabled != aram || group.Bench.PreferFirst != aram || group.Bench.HoldMS != 1000 {
			t.Fatalf("%s defaults = %#v", definition.GroupID, group.Bench)
		}
	}
	aram := settings.Groups["aram"]
	aram.Bench.Enabled, aram.Bench.PreferFirst, aram.Bench.HoldMS = false, false, 1750
	settings.Groups["aram"] = aram
	saved := normalizeChampSelectSettings(settings).Groups["aram"].Bench
	if saved.Enabled || saved.PreferFirst || saved.HoldMS != 1750 {
		t.Fatalf("explicit preferences overwritten: %#v", saved)
	}
	if got := champSelectGroupID(3200, "", 0); got != "aram" {
		t.Fatalf("custom ARAM = %s", got)
	}
}

func Test1110DiagnosticReportsAutomationGatesNotPlayerData(t *testing.T) {
	fixture := newR78ChampSelectFixture(t, "practice", "ban", 0, nil, nil)
	fixture.session.QueueID = 3110
	var records []map[string]any
	fixture.runner.observe = func(event map[string]any) { records = append(records, event) }
	settings := fixture.runner.currentWatch().ChampSelect
	fixture.runner.recordChampSelectEvaluation(*fixture.session, "practice", settings)
	fixture.runner.recordChampSelectEvaluation(*fixture.session, "practice", settings)
	if len(records) != 1 || records[0]["group_id"] != "practice" || records[0]["ban_pool_size"] != 0 || records[0]["ban_enabled"] != false {
		t.Fatalf("diagnostics = %#v", records)
	}
	for _, key := range []string{"puuid", "summonerId", "myTeam", "gameName"} {
		if _, ok := records[0][key]; ok {
			t.Fatalf("unexpected identity field %s", key)
		}
	}
}
