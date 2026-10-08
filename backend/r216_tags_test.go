package main

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestR216ArenaHasNoMatchKeywords(t *testing.T) {
	m := r216Game(r216Person(1, 100), r216Person(2, 200))
	applyMatchScores(&m)
	m.QueueID = 1700
	if len(computeMatchTags(m, []timelineFrame{{Timestamp: 60000}}, initialMatchKeywordParams)) != 0 {
		t.Fatal("arena tags")
	}
}
func TestR216MatchTagsPersistAndPrivacy(t *testing.T) {
	a := &app{storage: newFlowDiagnosticStore(t)}
	m := r216Game(r216Person(1, 100), r216Person(2, 200))
	applyMatchScores(&m)
	m.Participants[0].DisplayName = "SECRET_NAME"
	a.cacheMatchTagInput(m)
	frames := []timelineFrame{}
	for _, at := range []int64{0, 240000, 540000, 840000} {
		pf := map[string]json.RawMessage{}
		for _, id := range []string{"1", "2"} {
			pf[id] = json.RawMessage(`{"totalGold":2000,"minionsKilled":30,"jungleMinionsKilled":0,"damageStats":{"totalDamageDoneToChampions":1000}}`)
		}
		frames = append(frames, timelineFrame{Timestamp: at, ParticipantFrames: pf})
	}
	first := a.tagsFromTimeline(dataSourceSGP, "cn", m.GameID, frames)
	if len(first) != 2 || len(first[0].Checkpoints) < 3 {
		t.Fatal(first)
	}
	a.matchTagInputs = nil
	again := a.tagsFromTimeline(dataSourceSGP, "cn", m.GameID, nil)
	if len(again) != 2 {
		t.Fatal("disk miss", again)
	}
	raw, err := os.ReadFile(filepath.Join(a.storage.root, matchTagsFile(dataSourceSGP, "cn", m.GameID)))
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) == "" {
		t.Fatal("empty")
	}
	var disk map[string]any
	_ = json.Unmarshal(raw, &disk)
	if len(disk) != 2 {
		t.Fatal("unexpected stored identities", disk)
	}
	// Seed an obsolete-but-current-schema disabled value to verify read filtering.
	var cached matchTagDisk
	if err := json.Unmarshal(raw, &cached); err != nil {
		t.Fatal(err)
	}
	cached.Participants[0].Keyword = "leader"
	updated, _ := json.Marshal(cached)
	if err := os.WriteFile(filepath.Join(a.storage.root, matchTagsFile(dataSourceSGP, "cn", m.GameID)), updated, 0600); err != nil {
		t.Fatal(err)
	}
	filtered := a.readMatchTags(dataSourceSGP, "cn", m.GameID)
	if filtered[0].Keyword != "" {
		t.Fatal("disabled keyword")
	}
}
func TestR216HistoricalKeywordDump(t *testing.T) {
	output := os.Getenv("R216_HISTORICAL_KEYWORD_DUMP")
	if output == "" {
		t.Skip("historical export not requested")
	}
	var fixtures []struct {
		Path    string
		MatchID string `json:"match_id"`
	}
	raw, err := os.ReadFile("testdata/r216/python-v2-golden.json")
	if err != nil {
		t.Fatal(err)
	}
	_ = json.Unmarshal(raw, &fixtures)
	out := []map[string]any{}
	for _, f := range fixtures {
		raw, err = os.ReadFile(filepath.Join("..", f.Path))
		if err != nil {
			t.Fatal(err)
		}
		var r riotMatch
		_ = json.Unmarshal(raw, &r)
		m := convertRiotMatchInfo(&r.Info, r.Info.Participants[0].PUUID, nil, nil, "KR", "")
		timeline := f.Path[:len(f.Path)-len("-match.json")] + "-timeline.json"
		raw, err = os.ReadFile(filepath.Join("..", timeline))
		if err != nil {
			t.Fatal(err)
		}
		var tl lcuGameTimeline
		_ = json.Unmarshal(raw, &tl)
		tags := computeMatchTags(m, tl.frames(), initialMatchKeywordParams)
		for _, tag := range tags {
			for _, p := range m.Participants {
				if p.ParticipantID == tag.ParticipantID {
					out = append(out, map[string]any{"match_id": f.MatchID, "participantId": p.ParticipantID, "win": p.Win, "badge": p.Score.Badge, "checkpoints": tag.Checkpoints, "keyword": tag.Keyword})
				}
			}
		}
	}
	data, _ := json.Marshal(out)
	if err = os.WriteFile(output, data, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestR216TimelineKeywordCheckpointUsesPastEventsAndOptionalFields(t *testing.T) {
	m := r216Game(r216Person(1, 100), r216Person(2, 200))
	m.Duration = 900
	applyMatchScores(&m)
	pf := map[string]json.RawMessage{"1": json.RawMessage(`{"totalGold":3000,"minionsKilled":40,"jungleMinionsKilled":0}`), "2": json.RawMessage(`{"totalGold":3000,"minionsKilled":40,"jungleMinionsKilled":0}`)}
	frames := []timelineFrame{{Timestamp: 0, ParticipantFrames: pf}, {Timestamp: 600000, ParticipantFrames: pf, Events: []timelineEvent{{EventType: "CHAMPION_KILL", Timestamp: 200000, KillerID: 1, VictimID: 2}, {Type: "CHAMPION_KILL", Timestamp: 700000, KillerID: 2, VictimID: 1}}}}
	model := currentMatchKeywordModel
	model.Timeline.MissingPolicy = "omit-and-renormalize"
	model.Timeline.SmoothingMinutes = 0
	tags := computeMatchTagsWithRules(m, frames, model)
	if len(tags) != 2 || len(tags[0].Checkpoints) != 15 {
		t.Fatal(tags)
	}
	// The 5-minute frame's KDA must include eventType's early kill, but not a
	// future kill located in the same frame. Gold/CS are observed; damage absent.
	cp := m
	cp.Duration = 300
	cp.Participants = append([]gameplayParticipant(nil), m.Participants...)
	for i := range cp.Participants {
		p := &cp.Participants[i]
		p.Kills = 0
		p.Deaths = 0
		p.Assists = 0
		p.Gold = 3000
		p.CS = 40
		p.scoreMissing = map[string]bool{"vision": true, "damage": true}
	}
	cp.Participants[0].Kills = 1
	cp.Participants[1].Deaths = 1
	want := computeMatchScoresGo(cp, currentScoreParams)
	for _, tag := range tags {
		if math.Abs(tag.Checkpoints[4]-want[tag.ParticipantID].RawScore) > 1e-9 {
			t.Fatal("future event leaked or eventType ignored", tag, want[tag.ParticipantID])
		}
	}
}

func TestR217SixKeywordsKeepCacheVersionAndReadOnlyFiltering(t *testing.T) {
	if matchTagCacheSchemaVersion != 3 || matchTagModelVersion() != "tags-v3-v2.1-50bbd1866cee8969" {
		t.Fatal(matchTagModelVersion())
	}
	a := &app{storage: newFlowDiagnosticStore(t)}
	old := matchTagDisk{Version: "tags-v2-v2.1-r216-model", Participants: []participantMatchTags{{ParticipantID: 1, Keyword: "unstoppable", Checkpoints: []float64{7, 7, 7}}}}
	raw, _ := json.Marshal(old)
	if err := writeLocalStoreFile(a.storage, matchTagsFile(dataSourceSGP, "cn", 216), raw); err != nil {
		t.Fatal(err)
	}
	if got := a.readMatchTags(dataSourceSGP, "cn", 216); got != nil {
		t.Fatal("accepted old score cache", got)
	}
	if len(disabledMatchKeywords) != 8 {
		t.Fatal(disabledMatchKeywords)
	}
	for _, key := range []string{"unstoppable", "latebloomer", "resilience", "unlucky", "unyielding", "struggling"} {
		if disabledMatchKeywords[key] {
			t.Fatal("retained keyword disabled", key)
		}
	}
	// Existing raw classifications remain unchanged; display suppression is final.
	old.Version = matchTagModelVersion()
	old.Participants = nil
	for i, key := range matchKeywordKeys {
		old.Participants = append(old.Participants, participantMatchTags{ParticipantID: int64(i + 1), Keyword: key, Checkpoints: []float64{5, 6, 7}})
	}
	raw, _ = json.Marshal(old)
	if err := writeLocalStoreFile(a.storage, matchTagsFile(dataSourceSGP, "cn", 216), raw); err != nil {
		t.Fatal(err)
	}
	// No registered inputs or frames: a cache hit must not recompute or lose curves.
	rows := a.tagsFromTimeline(dataSourceSGP, "cn", 216, nil)
	if len(rows) != len(matchKeywordKeys) {
		t.Fatal(rows)
	}
	for i, row := range rows {
		want := matchKeywordKeys[i]
		if disabledMatchKeywords[want] {
			want = ""
		}
		if row.Keyword != want || len(row.Checkpoints) != 3 || row.Checkpoints[0] != 5 || row.Checkpoints[1] != 6 || row.Checkpoints[2] != 7 {
			t.Fatal("cache filtering changed classification or curve", matchKeywordKeys[i], row)
		}
	}
	after, err := readLocalStoreFile(a.storage, matchTagsFile(dataSourceSGP, "cn", 216))
	if err != nil || string(after) != string(raw) {
		t.Fatal("display filtering rewrote persisted tags", err)
	}
}

func TestR217ComputedKeywordsFollowHoldoutDisplayWithoutFallback(t *testing.T) {
	m := r216Game(r216Person(1, 100), r216Person(2, 200))
	m.Duration = 900
	applyMatchScores(&m)
	for i := range m.Participants {
		m.Participants[i].Score.RawScore = 6.2
	}
	pf := map[string]json.RawMessage{"1": json.RawMessage(`{"totalGold":2000,"minionsKilled":30,"jungleMinionsKilled":0,"damageStats":{"totalDamageDoneToChampions":1000}}`), "2": json.RawMessage(`{"totalGold":2000,"minionsKilled":30,"jungleMinionsKilled":0,"damageStats":{"totalDamageDoneToChampions":1000}}`)}
	model := currentMatchKeywordModel
	model.Rules = matchKeywordRuleParams{6, 6, 6, -.3, 7, -.01}
	model.Timeline.SmoothingMinutes = 0
	model.Timeline.MissingPolicy = "omit-and-renormalize"
	rows := computeMatchTagsWithRules(m, []timelineFrame{{Timestamp: 0, ParticipantFrames: pf, Events: []timelineEvent{{Type: "CHAMPION_KILL", Timestamp: 60000, KillerID: 1, VictimID: 2}}}}, model)
	if len(rows) != 2 {
		t.Fatal(rows)
	}
	closed, open := 0, 0
	for _, row := range rows {
		p := m.Participants[int(row.ParticipantID)-1]
		key, _ := lookupMatchKeyword(row.Checkpoints, p.Win, p.Score.Badge != "", model)
		want := key
		if disabledMatchKeywords[key] {
			closed++
			want = ""
		} else {
			open++
		}
		if row.Keyword != want {
			t.Fatal("display changed raw prediction or fell through", key, row)
		}
	}
	if closed != 1 || open != 1 {
		t.Fatal("fixture must cover both open and closed predictions", closed, open)
	}
}
