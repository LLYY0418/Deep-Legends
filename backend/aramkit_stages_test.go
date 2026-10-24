package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func r116bAugmentRow(t *testing.T, rows []championMetricRow, id int) championMetricRow {
	t.Helper()
	for _, r := range rows {
		if len(r.Assets) > 0 && r.Assets[0].ID == id {
			return r
		}
	}
	t.Fatalf("missing augment %d", id)
	return championMetricRow{}
}
func TestLoadMayhemDetailEmitsAugmentStagesInParentUnits(t *testing.T) {
	r := r307Resources(t)
	var raw aramkitRawDetail
	b := r307Fixture(t, "champion-157", &raw)
	detail, _, e := parseARAMKitDetail(b, 157, r)
	if e != nil {
		t.Fatal(e)
	}
	row := r116bAugmentRow(t, detail.RecommendedAugments, 3078)
	if len(row.Stages) != 4 {
		t.Fatal("stages missing")
	}
	for _, s := range row.Stages {
		var found *aramkitStat
		for _, upstream := range raw.Augments.Stages[string(rune('0'+s.Stage))] {
			if upstream.ID == 3078 {
				copy := upstream
				found = &copy
				break
			}
		}
		if found == nil || s.WinRate != found.WinRate*100 || s.PickRate != found.PickRate*100 || s.Games != found.SampleCount || s.Grade != found.Tier {
			t.Fatal("stage units changed", s, found)
		}
	}
}
func TestLoadMayhemDetailKeepsMissingAugmentStagesAbsent(t *testing.T) {
	r := r307Resources(t)
	var raw aramkitRawDetail
	r307Fixture(t, "champion-157", &raw)
	delete(raw.Augments.Stages, "2")
	b, _ := json.Marshal(raw)
	detail, _, e := parseARAMKitDetail(b, 157, r)
	if e != nil {
		t.Fatal(e)
	}
	for _, s := range r116bAugmentRow(t, detail.RecommendedAugments, 3078).Stages {
		if s.Stage == 2 {
			t.Fatal("missing stage zero filled")
		}
	}
}
func TestLoadMayhemDetailWithoutAnyStageRowsOmitsStagesKey(t *testing.T) {
	r := r307Resources(t)
	var raw aramkitRawDetail
	r307Fixture(t, "champion-157", &raw)
	raw.Augments.Stages = nil
	b, _ := json.Marshal(raw)
	detail, _, e := parseARAMKitDetail(b, 157, r)
	if e != nil {
		t.Fatal(e)
	}
	b, _ = json.Marshal(detail.RecommendedAugments)
	if strings.Contains(string(b), `"stages"`) {
		t.Fatal("missing stages emitted")
	}
}
func TestMayhemDetailResponseSerializesAugmentStagesForFrontend(t *testing.T) {
	p, _ := newARAMKitFixtureProvider(t, "")
	detail, e := p.loadMayhemDetail(t.Context(), "157")
	if e != nil {
		t.Fatal(e)
	}
	var wire map[string]json.RawMessage
	b, _ := json.Marshal(detail)
	if json.Unmarshal(b, &wire) != nil || !strings.Contains(string(wire["recommendedAugments"]), `"stages"`) {
		t.Fatal("wire stages missing")
	}
	for _, key := range []string{"itemRanking", "build"} {
		if strings.Contains(string(wire[key]), `"stages"`) {
			t.Fatal("unrelated stages", key)
		}
	}
}
