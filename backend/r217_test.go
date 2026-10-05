package main

import (
	"encoding/json"
	"os"
	"testing"
)

// This bridge runs production code offline. It is not a holdout evaluator;
// only Claude's guarded once-only script supplies the new holdout paths.
func TestR217KeywordDump(t *testing.T) {
	input := os.Getenv("R217_KEYWORD_DUMP_INPUT")
	if input == "" {
		t.Skip("offline bridge not requested")
	}
	output := os.Getenv("R217_KEYWORD_DUMP_OUTPUT")
	if output == "" {
		t.Fatal("output required")
	}
	var req struct {
		Matches []struct {
			MatchID      string `json:"matchId"`
			MatchPath    string `json:"matchPath"`
			TimelinePath string `json:"timelinePath"`
		}
		Models map[string]matchKeywordModel `json:"models"`
	}
	raw, err := os.ReadFile(input)
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &req); err != nil {
		t.Fatal(err)
	}
	if len(req.Models) == 0 {
		req.Models = map[string]matchKeywordModel{"frozen": currentMatchKeywordModel}
	}
	out := map[string][][]map[string]any{}
	for _, fixture := range req.Matches {
		raw, err = os.ReadFile(fixture.MatchPath)
		if err != nil {
			t.Fatal(err)
		}
		var r riotMatch
		if err = json.Unmarshal(raw, &r); err != nil {
			t.Fatal(err)
		}
		raw, err = os.ReadFile(fixture.TimelinePath)
		if err != nil {
			t.Fatal(err)
		}
		var tl lcuGameTimeline
		if err = json.Unmarshal(raw, &tl); err != nil {
			t.Fatal(err)
		}
		if len(r.Info.Participants) != 10 {
			t.Fatal("expected ten participants", fixture.MatchID)
		}
		m := convertRiotMatchInfo(&r.Info, r.Info.Participants[0].PUUID, nil, nil, "KR", "")
		applyMatchScores(&m)
		people := map[int64]gameplayParticipant{}
		for _, p := range m.Participants {
			people[p.ParticipantID] = p
		}
		for name, model := range req.Models {
			rows := []map[string]any{}
			for _, tag := range computeMatchTagsWithRules(m, tl.frames(), model) {
				p := people[tag.ParticipantID]
				rawKeyword, analysis := lookupMatchKeyword(tag.Checkpoints, p.Win, p.Score.Badge != "", model)
				rows = append(rows, map[string]any{"matchId": fixture.MatchID, "participantId": p.ParticipantID, "win": p.Win, "teamMax": p.Score.Badge != "", "seconds": tag.Seconds, "checkpoints": tag.Checkpoints, "rawKeyword": rawKeyword, "keyword": tag.Keyword, "analysis": analysis})
			}
			out[name] = append(out[name], rows)
		}
	}
	raw, err = json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(output, raw, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestR217CurveDirectionsAndFourEndingClasses(t *testing.T) {
	params := matchKeywordRuleParams{6, 6, 6, -.3, 7, -.01}
	for _, tc := range []struct {
		scores            []float64
		left, right, last string
	}{
		{[]float64{7, 7, 7, 5}, "UP", "UP", "POOR"},
		{[]float64{5, 5, 5, 6}, "DOWN", "DOWN", "EXCELLENT"},
		{[]float64{7, 7, 7, 7}, "UP", "UP", "GOOD"},
		{[]float64{7, 7, 7, 6.5}, "UP", "UP", "FAIR"},
		{[]float64{6.1, 6.1, 5.8, 5.9}, "UP", "DOWN", "GOOD"},
		{[]float64{5, 5, 7, 7}, "DOWN", "UP", "GOOD"},
	} {
		got := analyzeMatchKeywordCurve(tc.scores, params)
		if got != (matchTimelineAnalysis{tc.left, tc.right, tc.last}) {
			t.Fatal(tc, got)
		}
	}
	if got := analyzeMatchKeywordCurve([]float64{7}, params); got != (matchTimelineAnalysis{}) {
		t.Fatal(got)
	}
}

func TestR217LookupCoversAllFrozenCombinationsAndRejectsUnseen(t *testing.T) {
	raw, err := os.ReadFile("../docs/history/reports/r217/rule-freeze.json")
	if err != nil {
		t.Fatal(err)
	}
	var frozen struct {
		Table []matchKeywordLookupRow `json:"table"`
	}
	if err = json.Unmarshal(raw, &frozen); err != nil {
		t.Fatal(err)
	}
	if len(frozen.Table) != 42 || len(currentMatchKeywordModel.Table) != 42 {
		t.Fatal("lookup coverage")
	}
	for i, row := range frozen.Table {
		if currentMatchKeywordModel.Table[i] != row {
			t.Fatal("changed P1 table", row)
		}
	}
	model := currentMatchKeywordModel
	model.Rules = matchKeywordRuleParams{6, 6, 6, -.3, 7, -.01}
	// WIN / team maximum / DOWN DOWN GOOD was never observed in training.
	if key, _ := lookupMatchKeyword([]float64{5, 5, 5, 5}, true, true, model); key != "" {
		t.Fatal("guessed unseen combination", key)
	}
	if key, _ := lookupMatchKeyword([]float64{7, 7, 7, 7}, true, true, model); key != "unstoppable" {
		t.Fatal(key)
	}
	if key, _ := lookupMatchKeyword([]float64{7, 7, 7.2, 7.1}, true, true, model); key != "leader" {
		t.Fatal(key)
	}
}

func TestR217MinuteCheckpointsAndUnknownFrames(t *testing.T) {
	for _, duration := range []int64{60, 61, 120, 1800, 1801} {
		m := r216Game(r216Person(1, 100), r216Person(2, 200))
		m.Duration = duration
		applyMatchScores(&m)
		pf := map[string]json.RawMessage{"1": json.RawMessage(`{"totalGold":500,"minionsKilled":0,"jungleMinionsKilled":0,"damageStats":{"totalDamageDoneToChampions":0}}`), "2": json.RawMessage(`{"totalGold":500,"minionsKilled":0,"jungleMinionsKilled":0,"damageStats":{"totalDamageDoneToChampions":0}}`)}
		rows := computeMatchTags(m, []timelineFrame{{Timestamp: 0, ParticipantFrames: pf}}, currentMatchKeywordParams)
		if len(rows) != 2 {
			t.Fatal(rows)
		}
		for _, row := range rows {
			if len(row.Checkpoints) != int((duration-1)/60)+1 || len(row.Seconds) != len(row.Checkpoints) || row.Seconds[len(row.Seconds)-1] != duration {
				t.Fatal(duration, row)
			}
		}
		if got := computeMatchTags(m, []timelineFrame{{Timestamp: duration * 1000, ParticipantFrames: pf}}, currentMatchKeywordParams); duration > 60 && len(got) != 0 {
			t.Fatal("filled unknown past from future", got)
		}
	}
}
