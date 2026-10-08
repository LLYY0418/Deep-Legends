package main

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestR216PythonV2MigrationConsistency260(t *testing.T) {
	r216AssertGolden(t, "testdata/r216/python-v2-golden.json", v2ScoreParams, false)
}
func TestR216PythonV21GoldenConsistency260(t *testing.T) {
	r216AssertGolden(t, "testdata/r216/python-v21-golden.json", currentScoreParams, true)
}
func r216AssertGolden(t *testing.T, path string, params scoreParams, production bool) {
	t.Helper()
	var fixtures []struct {
		Path   string
		Scores []struct {
			ParticipantID int64 `json:"participantId"`
			RawScore      float64
			Rank          int
			Badge         string
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &fixtures); err != nil {
		t.Fatal(err)
	}
	if len(fixtures) != 260 {
		t.Fatal(len(fixtures))
	}
	maxDiff := 0.
	missingSamples := 0
	for _, f := range fixtures {
		raw, err := os.ReadFile(filepath.Join("..", f.Path))
		if os.IsNotExist(err) {
			missingSamples++
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		var r riotMatch
		if err = json.Unmarshal(raw, &r); err != nil {
			t.Fatal(err)
		}
		m := convertRiotMatchInfo(&r.Info, r.Info.Participants[0].PUUID, nil, nil, "KR", "")
		if !production {
			scores := computeMatchScoresGo(m, params)
			for i := range m.Participants {
				m.Participants[i].Score = scores[m.Participants[i].ParticipantID]
			}
		}
		for _, want := range f.Scores {
			s := findR216Score(m, want.ParticipantID)
			if s == nil {
				t.Fatal(f.Path, want.ParticipantID)
			}
			diff := math.Abs(s.RawScore - want.RawScore)
			maxDiff = math.Max(maxDiff, diff)
			if diff >= 1e-9 || s.Rank != want.Rank || s.Badge != want.Badge {
				t.Fatalf("%s pid%d got%+v want%+v diff%g", f.Path, want.ParticipantID, s, want, diff)
			}
		}
	}
	if missingSamples > 0 {
		t.Logf("missing %d of %d raw samples; checked every remaining sample", missingSamples, len(fixtures))
		t.Skip("校准原始样本已清理，见 R220")
	}
	t.Logf("2600 participants, maximum absolute difference %.17g", maxDiff)
}
func findR216Score(m gameplayMatch, id int64) *matchScore {
	for _, p := range m.Participants {
		if p.ParticipantID == id {
			return p.Score
		}
	}
	return nil
}
func TestR216LegacyV2JSGolden20(t *testing.T) {
	r216AssertGameplayGolden(t, "testdata/r216/js-v2-golden.json", v2ScoreParams)
}
func TestR216V21GameplayGolden20(t *testing.T) {
	r216AssertGameplayGolden(t, "testdata/r216/gameplay-v21-golden.json", currentScoreParams)
}
func r216AssertGameplayGolden(t *testing.T, path string, params scoreParams) {
	t.Helper()
	var fs []struct {
		Match  gameplayMatch
		Scores map[int64]struct {
			Score      float64
			RawScore   float64
			Rank       int
			Total      int
			Badge      string
			Components []matchScorePart
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &fs); err != nil {
		t.Fatal(err)
	}
	if len(fs) != 20 {
		t.Fatal(len(fs))
	}
	for _, f := range fs {
		got := computeMatchScoresGo(f.Match, params)
		for id, w := range f.Scores {
			g := got[id]
			if g == nil || math.Abs(g.RawScore-w.RawScore) >= 1e-9 || g.Value != w.Score || g.Rank != w.Rank || g.Total != w.Total || g.Badge != w.Badge {
				t.Fatalf("pid%d got%+v want%+v", id, g, w)
			}
			for j, p := range g.Parts {
				q := w.Components[j]
				if p.Key != q.Key || math.Abs(p.Norm-q.Norm) > 1e-12 || math.Abs(p.Reference-q.Reference) > 1e-9 {
					t.Fatal(p, q)
				}
			}
		}
	}
}
func r216Person(id, team int64) gameplayParticipant {
	return gameplayParticipant{ParticipantID: id, TeamID: team, Win: team == 100, Kills: 3, Assists: 6, Deaths: 3, Damage: 10000, Gold: 9000, CS: 100, VisionScore: 10}
}
func r216Game(ps ...gameplayParticipant) gameplayMatch {
	return gameplayMatch{GameID: 216, Duration: 1200, Result: "win", Participants: ps}
}
func TestR216LegacyScoringInvariants(t *testing.T) {
	ps := []gameplayParticipant{r216Person(4, 200), r216Person(1, 100), r216Person(3, 200), r216Person(2, 100)}
	m := r216Game(ps...)
	before := computeMatchScoresGo(m, v2ScoreParams)
	for i, j := 0, len(ps)-1; i < j; i, j = i+1, j-1 {
		ps[i], ps[j] = ps[j], ps[i]
	}
	if !reflect.DeepEqual(before, computeMatchScoresGo(m, v2ScoreParams)) {
		t.Fatal("order changed scores")
	}
	for _, kind := range []string{"missing", "zero", "negative"} {
		m := r216Game(r216Person(1, 100), r216Person(2, 200))
		if kind == "missing" {
			m.Participants[0].scoreMissing = map[string]bool{"vision": true}
		} else if kind == "zero" {
			m.Participants[0].VisionScore = 0
			m.Participants[1].VisionScore = 0
		} else {
			m.Participants[0].VisionScore = -1
		}
		for _, s := range computeMatchScoresGo(m, v2ScoreParams) {
			for _, p := range s.Parts {
				if p.Key == "vision" {
					t.Fatal(kind, p)
				}
			}
			if !scoreValid(s.Value) {
				t.Fatal(s)
			}
		}
	}
	for _, result := range []string{"remake", "unknown"} {
		m.Result = result
		if len(computeMatchScoresGo(m, v2ScoreParams)) != 0 {
			t.Fatal(result)
		}
	}
	m = r216Game(r216Person(1, 100), r216Person(1, 200))
	if len(computeMatchScoresGo(m, v2ScoreParams)) != 0 {
		t.Fatal("duplicate")
	}
	m = r216Game(r216Person(1, 100), r216Person(2, 200), r216Person(3, 200))
	m.Participants[0].Kills = 10
	m.Participants[0].Assists = 0
	m.Participants[0].Deaths = 0
	m.Participants[1].Kills = 0
	m.Participants[1].Assists = 10
	m.Participants[1].Deaths = 0
	m.Participants[2].Kills = 10
	m.Participants[2].Assists = 0
	m.Participants[2].Deaths = 0
	s := computeMatchScoresGo(m, v2ScoreParams)
	if s[1].Value != s[2].Value {
		t.Fatal("assist credit", s)
	}
	m.Participants[0].Win = false
	if computeMatchScoresGo(m, v2ScoreParams)[1].Value != s[1].Value {
		t.Fatal("win changed score")
	}
	m.Participants[1].scoreMissing = map[string]bool{"win": true}
	for _, r := range computeMatchScoresGo(m, v2ScoreParams) {
		if r.Badge != "" || r.Value < 2 || r.Value > 10 {
			t.Fatal(r)
		}
		sum := 2.
		for _, p := range r.Parts {
			sum += p.Contribution
			if math.Abs(p.Norm-p.Contribution/(8*p.Weight)) > 1e-10 {
				t.Fatal(p)
			}
		}
		if math.Abs(sum-r.RawScore) > 1e-10 {
			t.Fatal(r)
		}
	}
	m.QueueID = 1700
	for _, s := range computeMatchScoresGo(m, v2ScoreParams) {
		if s.Badge != "" || s.Rank < 1 {
			t.Fatal("Arena", s)
		}
	}
}
func TestR216RoleReferencesAndOutliers(t *testing.T) {
	ps := []gameplayParticipant{}
	roles := []string{"top", "jungle", "middle", "bottom", "utility"}
	for ti, team := range []int64{100, 200} {
		for i, role := range roles {
			p := r216Person(int64(ti*5+i+1), team)
			p.Position = role
			p.Damage = (i + 1) * 1000
			p.Gold = (i + 1) * 1000
			p.CS = (i + 1) * 20
			p.VisionScore = 50 - i*10
			ps = append(ps, p)
		}
	}
	m := r216Game(ps...)
	m.QueueID = 420
	s := computeMatchScoresGo(m, v2ScoreParams)
	for _, r := range s {
		if r.Value != 6 || !r.RoleComplete {
			t.Fatal(r)
		}
	}
	ps[0].Damage = 1000000000
	if computeMatchScoresGo(m, v2ScoreParams)[5].Value != s[5].Value {
		t.Fatal("top suppresses support")
	}
	for _, q := range []int64{0, 450, 1700, 9999} {
		m.QueueID = q
		for _, r := range computeMatchScoresGo(m, v2ScoreParams) {
			if r.RoleComplete {
				t.Fatal(q)
			}
		}
	}
	m = r216Game(r216Person(1, 100), r216Person(2, 100), r216Person(3, 200), r216Person(4, 200))
	before := computeMatchScoresGo(m, v2ScoreParams)[2].Value
	m.Participants[0].Damage = 1000000000
	if computeMatchScoresGo(m, v2ScoreParams)[2].Value != before {
		t.Fatal("median outlier")
	}
}
func TestR216PresenceRoundTripAndExtraFields(t *testing.T) {
	var raw riotParticipant
	if err := json.Unmarshal([]byte(`{"participantId":1,"totalDamageTaken":0,"damageSelfMitigated":0,"doubleKills":2,"challenges":{"dragonTakedowns":0}}`), &raw); err != nil {
		t.Fatal(err)
	}
	if raw.TotalDamageTaken == nil || *historyIntValue(raw.TotalDamageTaken) != 0 || raw.DamageSelfMitigated == nil || raw.TotalHealsOnTeammates != nil || raw.Challenges.DragonTakedowns == nil {
		t.Fatal(raw)
	}
	b, _ := json.Marshal(raw)
	var again riotParticipant
	_ = json.Unmarshal(b, &again)
	if !again.scoreMissing["vision"] || !again.scoreMissing["combat"] {
		t.Fatal(string(b))
	}
}
func TestR216V21ScoreDiagnosticDedupPrivacy(t *testing.T) {
	a := &app{storage: newFlowDiagnosticStore(t)}
	m := r216Game(r216Person(1, 100), r216Person(2, 200))
	m.Participants[0].DisplayName = "DO_NOT_LOG_NAME"
	m.Participants[0].PlayerRef = "DO_NOT_LOG_PUUID"
	applyMatchScores(&m)
	a.recordMatchScores("riot", m)
	a.recordMatchScores("sgp", m)
	raw, err := a.storage.readDiagnosticLog()
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	if strings.Count(s, "match_score_computed") != 1 || strings.Contains(s, "DO_NOT_LOG") || !strings.Contains(s, `"paramVersion":"v2.1"`) || !strings.Contains(s, "reference") {
		t.Fatal(s)
	}
}

// Offline score bridge used by Claude's P4 evaluator. Not an evaluation: it
// outputs the same production Go scorer for supplied matches and frozen params.
func TestR216ScoreDump(t *testing.T) {
	input := os.Getenv("R216_SCORE_DUMP_INPUT")
	if input == "" {
		t.Skip("offline bridge not requested")
	}
	if os.Getenv("R216_SCORE_DUMP_OUTPUT") == "" {
		t.Fatal("R216_SCORE_DUMP_OUTPUT is required")
	}
	var req struct {
		Params        scoreParams
		Matches       []riotMatch
		Gameplay      []gameplayMatch
		Timelines     []lcuGameTimeline
		KeywordParams *matchKeywordParams
	}
	raw, err := os.ReadFile(input)
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &req); err != nil {
		t.Fatal(err)
	}
	if len(req.Timelines) > 0 && len(req.Timelines) != len(req.Matches) {
		t.Fatal("timeline count must match match count")
	}
	out := []map[int64]*matchScore{}
	tags := [][]participantMatchTags{}
	for index, r := range req.Matches {
		subject := ""
		if len(r.Info.Participants) > 0 {
			subject = r.Info.Participants[0].PUUID
		}
		m := convertRiotMatchInfo(&r.Info, subject, nil, nil, "KR", "")
		scores := computeMatchScoresGo(m, req.Params)
		out = append(out, scores)
		for i := range m.Participants {
			m.Participants[i].Score = scores[m.Participants[i].ParticipantID]
		}
		if index < len(req.Timelines) {
			params := currentMatchKeywordParams
			if req.KeywordParams != nil {
				params = *req.KeywordParams
			}
			tags = append(tags, computeMatchTagsWithScore(m, req.Timelines[index].frames(), params, req.Params))
		}
	}
	for _, m := range req.Gameplay {
		out = append(out, computeMatchScoresGo(m, req.Params))
	}
	if path := os.Getenv("R216_TAG_DUMP_OUTPUT"); path != "" {
		data, err := json.Marshal(tags)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	data, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(os.Getenv("R216_SCORE_DUMP_OUTPUT"), data, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestR216LegacyRankBadgesAndHiddenPrecision(t *testing.T) {
	ps := []gameplayParticipant{}
	for i, d := range []int{10000, 8000, 11000, 9000} {
		team := int64(100)
		if i > 1 {
			team = 200
		}
		p := r216Person(int64(i+1), team)
		p.Damage = d
		p.Kills = d / 1000
		p.Assists = 0
		p.Deaths = 1
		p.Gold = d
		p.CS = d / 100
		p.VisionScore = d / 2000
		ps = append(ps, p)
	}
	s := computeMatchScoresGo(r216Game(ps...), v2ScoreParams)
	for i, w := range []struct {
		rank  int
		badge string
	}{{2, "MVP"}, {4, ""}, {1, "SVP"}, {3, ""}} {
		if s[int64(i+1)].Rank != w.rank || s[int64(i+1)].Badge != w.badge || s[int64(i+1)].Total != 4 {
			t.Fatal(s, w)
		}
	}
	ps = []gameplayParticipant{r216Person(1, 100), r216Person(2, 100), r216Person(3, 200), r216Person(4, 200)}
	ps[2].Damage = 9999
	ps[3].Damage = 9998
	s = computeMatchScoresGo(r216Game(ps...), v2ScoreParams)
	if s[3].Value != s[4].Value || s[3].RawScore <= s[4].RawScore {
		t.Fatal("hidden precision", s)
	}
	for i := int64(1); i <= 4; i++ {
		if s[i].Rank != int(i) {
			t.Fatal("tie id order", s)
		}
	}
}

func TestR216V21ProductionFrozenParams(t *testing.T) {
	var frozen struct{ Params scoreParams }
	raw, err := os.ReadFile("testdata/r216/v21-candidate.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &frozen); err != nil {
		t.Fatal(err)
	}
	if matchScoreParamVersion != "v2.1" || currentScoreParams.AssistFactor != frozen.Params.AssistFactor {
		t.Fatal(currentScoreParams, matchScoreParamVersion)
	}
	for i, w := range currentScoreParams.Weights {
		if math.Abs(w-frozen.Params.Weights[i]) > 1e-15 {
			t.Fatal(currentScoreParams, frozen.Params)
		}
	}
}
