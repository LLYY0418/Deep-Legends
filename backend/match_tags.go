package main

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type matchKeywordParams struct {
	High      float64 `json:"high"`
	Low       float64 `json:"low"`
	Rise      float64 `json:"rise"`
	Delta     float64 `json:"delta"`
	Turn      float64 `json:"turn"`
	Amplitude float64 `json:"amplitude"`
	Weak      float64 `json:"weak"`
	Good      float64 `json:"good"`
	Excellent float64 `json:"excellent"`
	Struggle  float64 `json:"struggle"`
}

var initialMatchKeywordParams = matchKeywordParams{6.5, 4.5, 2, 1.5, .8, 3, 5, 6, 7, 4.5}

//go:embed testdata/r216/keyword-thresholds.json
var calibratedKeywordJSON []byte
var currentMatchKeywordParams = func() matchKeywordParams {
	var params matchKeywordParams
	if err := json.Unmarshal(calibratedKeywordJSON, &params); err != nil {
		panic(err)
	}
	return params
}()

// R217 holdout: suppress the eight closed predictions; never fall through to another keyword.
var disabledMatchKeywords = map[string]bool{
	"leader": true, "victorious": true, "dedication": true, "average": true, "rollercoaster": true, "decline": true, "innocent": true, "slowstarter": true,
}

const matchTagCacheSchemaVersion = 3

var matchKeywordKeys = []string{"unstoppable", "leader", "victorious", "latebloomer", "resilience", "dedication", "average", "rollercoaster", "decline", "innocent", "unlucky", "slowstarter", "unyielding", "struggling"}

type matchCurveFeatures struct {
	Early     float64 `json:"early"`
	Late      float64 `json:"late"`
	Delta     float64 `json:"delta"`
	Low       float64 `json:"low"`
	Amplitude float64 `json:"amplitude"`
	Turns     int     `json:"turns"`
	Final     float64 `json:"final"`
	Minimum   float64 `json:"minimum"`
}

func matchCurve(scores []float64, turn float64) matchCurveFeatures {
	n := len(scores)
	if n == 0 {
		return matchCurveFeatures{}
	}
	third := max(1, n/3)
	f := matchCurveFeatures{Low: scores[0], Final: scores[n-1], Minimum: scores[0]}
	maximum := scores[0]
	for i, s := range scores {
		if i < third {
			f.Early += s / float64(third)
		}
		if i >= n-third {
			f.Late += s / float64(third)
		}
		f.Minimum = math.Min(f.Minimum, s)
		maximum = math.Max(maximum, s)
	}
	f.Low = f.Minimum
	if n > 2 {
		f.Low = scores[1]
		for _, s := range scores[1 : n-1] {
			f.Low = math.Min(f.Low, s)
		}
	}
	f.Amplitude = maximum - f.Minimum
	f.Delta = f.Late - f.Early
	direction := 0
	for i := 1; i < n; i++ {
		d := scores[i] - scores[i-1]
		if math.Abs(d) < turn {
			continue
		}
		next := 1
		if d < 0 {
			next = -1
		}
		if direction != 0 && next != direction {
			f.Turns++
		}
		direction = next
	}
	return f
}
func classifyMatchKeyword(scores []float64, win bool, badge string, params matchKeywordParams) string {
	if len(scores) < 3 {
		return ""
	}
	f := matchCurve(scores, params.Turn)
	key := "average"
	if win {
		switch {
		case badge == "MVP" && f.Minimum >= params.High:
			key = "unstoppable"
		case f.Low <= params.Low && f.Final-f.Low >= params.Rise:
			key = "resilience"
		case f.Delta >= params.Delta:
			key = "latebloomer"
		case f.Early >= params.High && f.Delta > -params.Delta:
			key = "victorious"
		case badge == "MVP":
			key = "leader"
		case f.Delta <= -params.Delta:
			key = "decline"
		case f.Turns >= 2 && f.Amplitude >= params.Amplitude:
			key = "rollercoaster"
		case f.Final < params.Weak:
			key = "dedication"
		}
	}
	if !win {
		switch {
		case badge == "SVP" && f.Final >= params.Excellent:
			key = "innocent"
		case f.Low <= params.Low && f.Final-f.Low >= params.Rise:
			key = "unyielding"
		case f.Delta >= params.Delta:
			key = "slowstarter"
		case f.Final >= params.Good:
			key = "unlucky"
		case f.Delta <= -params.Delta:
			key = "decline"
		case f.Turns >= 2 && f.Amplitude >= params.Amplitude:
			key = "rollercoaster"
		case f.Final < params.Struggle && f.Late < params.Struggle:
			key = "struggling"
		}
	}
	return key
}

type participantMatchTags struct {
	ParticipantID int64     `json:"participantId"`
	Keyword       string    `json:"keyword,omitempty"`
	Checkpoints   []float64 `json:"checkpoints"`
	Seconds       []int64   `json:"seconds"`
}
type timelineScoreFrame struct {
	TotalGold           *int `json:"totalGold"`
	MinionsKilled       *int `json:"minionsKilled"`
	JungleMinionsKilled *int `json:"jungleMinionsKilled"`
	DamageStats         struct {
		Damage *int `json:"totalDamageDoneToChampions"`
	} `json:"damageStats"`
}

func computeMatchTags(m gameplayMatch, frames []timelineFrame, params matchKeywordParams) []participantMatchTags {
	return computeMatchTagsWithScore(m, frames, params, currentScoreParams)
}
func computeMatchTagsWithScore(m gameplayMatch, frames []timelineFrame, params matchKeywordParams, scoreModel scoreParams) []participantMatchTags {
	model := currentMatchKeywordModel
	model.Timeline.Score = scoreModel
	return computeMatchTagsWithRules(m, frames, model)
}

func computeMatchTagsWithRules(m gameplayMatch, frames []timelineFrame, model matchKeywordModel) []participantMatchTags {
	rows := computeMatchTagCurves(m, frames, model.Timeline)
	people := map[int64]gameplayParticipant{}
	for _, p := range m.Participants {
		people[p.ParticipantID] = p
	}
	for i := range rows {
		p := people[rows[i].ParticipantID]
		keyword, _ := lookupMatchKeyword(rows[i].Checkpoints, p.Win, p.Score.Badge != "", model)
		if !disabledMatchKeywords[keyword] {
			rows[i].Keyword = keyword
		}
	}
	return rows
}

func computeMatchTagCurves(m gameplayMatch, frames []timelineFrame, timelineParams matchTimelineScoreParams) []participantMatchTags {
	out := []participantMatchTags{}
	if m.QueueID == 1700 || m.QueueID == 1710 || strings.EqualFold(m.GameMode, "CHERRY") || m.Result == "unknown" || m.Result == "remake" || len(frames) == 0 {
		return out
	}
	// KR solo OP.GG timestamps are minute-spaced. Other modes retain their
	// R216 schedule: mode-specific OP.GG intervals remain unverified.
	interval := int64(60)
	if m.MapID == 12 || strings.EqualFold(m.GameMode, "ARAM") || seasonMayhemQueue(m.QueueID) {
		interval = 180
	}
	checkpoints := []int64{}
	for at := interval; at < m.Duration; at += interval {
		checkpoints = append(checkpoints, at)
	}
	curves := map[int64][]float64{}
	frames = append([]timelineFrame(nil), frames...)
	sort.SliceStable(frames, func(i, j int) bool { return frames[i].Timestamp < frames[j].Timestamp })
	for _, at := range checkpoints {
		checkpoint := m
		checkpoint.Duration = at
		checkpoint.Participants = append([]gameplayParticipant(nil), m.Participants...)
		counts := map[int64][3]int{}
		combatKnown := true
		var latest *timelineFrame
		for i := range frames {
			f := &frames[i]
			if f.Timestamp <= at*1000 {
				latest = f
			}
			for _, e := range f.Events {
				eventType := strings.TrimSpace(e.Type)
				if eventType == "" {
					eventType = e.EventType
				}
				if e.Timestamp > at*1000 || strings.ToUpper(eventType) != "CHAMPION_KILL" {
					continue
				}
				if e.VictimID <= 0 {
					combatKnown = false
				}
				if e.KillerID > 0 {
					v := counts[e.KillerID]
					v[0]++
					counts[e.KillerID] = v
				}
				if e.VictimID > 0 {
					v := counts[e.VictimID]
					v[1]++
					counts[e.VictimID] = v
				}
				for _, id := range e.AssistingParticipantIDs {
					v := counts[id]
					v[2]++
					counts[id] = v
				}
			}
		}
		for i := range checkpoint.Participants {
			p := &checkpoint.Participants[i]
			v := counts[p.ParticipantID]
			p.Kills, p.Deaths, p.Assists = v[0], v[1], v[2]
			p.scoreMissing = map[string]bool{"vision": true, "combat": !combatKnown}
			var data timelineScoreFrame
			if latest == nil {
				p.scoreMissing["gold"] = true
				p.scoreMissing["cs"] = true
				p.scoreMissing["damage"] = true
			} else {
				raw := latest.ParticipantFrames[strconv.FormatInt(p.ParticipantID, 10)]
				_ = json.Unmarshal(raw, &data)
				if data.TotalGold == nil {
					p.scoreMissing["gold"] = true
				} else {
					p.Gold = *data.TotalGold
				}
				if data.MinionsKilled == nil || data.JungleMinionsKilled == nil {
					p.scoreMissing["cs"] = true
				} else {
					p.CS = *data.MinionsKilled + *data.JungleMinionsKilled
				}
				if data.DamageStats.Damage == nil {
					p.scoreMissing["damage"] = true
				} else {
					p.Damage = *data.DamageStats.Damage
				}
			}
		}
		scores := computeMatchScoresGo(checkpoint, timelineParams.Score)
		for _, p := range m.Participants {
			if s := scores[p.ParticipantID]; s != nil {
				curves[p.ParticipantID] = append(curves[p.ParticipantID], smoothTimelineScore(s, at, timelineParams))
			}
		}
	}
	for _, p := range m.Participants {
		if p.Score == nil {
			continue
		}
		curve := append(curves[p.ParticipantID], p.Score.RawScore)
		// A missing minute is unknown, never filled from future settlement data.
		if len(curves[p.ParticipantID]) != len(checkpoints) {
			continue
		}
		seconds := append(append([]int64(nil), checkpoints...), m.Duration)
		out = append(out, participantMatchTags{ParticipantID: p.ParticipantID, Checkpoints: curve, Seconds: seconds})
	}
	return out
}

type matchTagInput struct {
	Match gameplayMatch
	At    time.Time
}
type matchTagDisk struct {
	Version      string                 `json:"version"`
	Participants []participantMatchTags `json:"participants"`
}

func matchTagModelVersion() string {
	sum := sha256.Sum256(matchKeywordCandidateJSON)
	return fmt.Sprintf("tags-v%d-%s-%s", matchTagCacheSchemaVersion, matchScoreParamVersion, hex.EncodeToString(sum[:8]))
}
func matchTagScope(m gameplayMatch) string {
	for _, p := range m.Participants {
		if isRiotRegion(p.reference.Region) {
			return riotPlatform(p.reference.Region)
		}
		if p.reference.ServerID != "" {
			return "cn:" + strings.ToUpper(p.reference.ServerID)
		}
	}
	return "cn"
}
func (a *app) cacheMatchTagInput(m gameplayMatch) {
	if a == nil || m.GameID <= 0 {
		return
	}
	clone := m
	clone.Participants = append([]gameplayParticipant(nil), m.Participants...)
	for i := range clone.Participants {
		p := &clone.Participants[i]
		p.DisplayName = ""
		p.GameName = ""
		p.TagLine = ""
		p.PlayerRef = ""
		p.reference = gameplayReference{}
		p.ProPlayer = nil
	}
	key := matchTagScope(m) + "|" + strconv.FormatInt(m.GameID, 10)
	a.matchTagsMu.Lock()
	defer a.matchTagsMu.Unlock()
	if a.matchTagInputs == nil {
		a.matchTagInputs = map[string]matchTagInput{}
	}
	a.matchTagInputs[key] = matchTagInput{clone, time.Now()}
	if len(a.matchTagInputs) > 5000 {
		oldest := ""
		var at time.Time
		for k, v := range a.matchTagInputs {
			if oldest == "" || v.At.Before(at) {
				oldest = k
				at = v.At
			}
		}
		delete(a.matchTagInputs, oldest)
	}
}
func matchTagsFile(source, scope string, id int64) string {
	return filepath.Join("match-tags", source, strings.ReplaceAll(scope, ":", "-"), strconv.FormatInt(id, 10)+".json")
}
func (a *app) readMatchTags(source, scope string, id int64) []participantMatchTags {
	if a.storage == nil {
		return nil
	}
	raw, err := readLocalStoreFile(a.storage, matchTagsFile(source, scope, id))
	if err != nil {
		return nil
	}
	var disk matchTagDisk
	if json.Unmarshal(raw, &disk) != nil || disk.Version != matchTagModelVersion() {
		return nil
	}
	for i := range disk.Participants {
		if disabledMatchKeywords[disk.Participants[i].Keyword] {
			disk.Participants[i].Keyword = ""
		}
	}
	_ = os.Chtimes(filepath.Join(a.storage.root, matchTagsFile(source, scope, id)), time.Now(), time.Now())
	return disk.Participants
}
func (a *app) tagsFromTimeline(source, scope string, id int64, frames []timelineFrame) []participantMatchTags {
	if rows := a.readMatchTags(source, scope, id); rows != nil {
		return rows
	}
	a.matchTagsMu.Lock()
	input, ok := a.matchTagInputs[scope+"|"+strconv.FormatInt(id, 10)]
	if !ok && strings.HasPrefix(scope, "cn:") {
		input, ok = a.matchTagInputs["cn|"+strconv.FormatInt(id, 10)]
	}
	a.matchTagsMu.Unlock()
	if !ok {
		return nil
	}
	rows := computeMatchTags(input.Match, frames, currentMatchKeywordParams)
	if len(rows) == 0 {
		return nil
	}
	if a.storage != nil {
		data, _ := json.Marshal(matchTagDisk{matchTagModelVersion(), rows})
		if err := writeLocalStoreFile(a.storage, matchTagsFile(source, scope, id), data); err != nil {
			a.recordDiagnostic(map[string]any{"event": "match_tags_save_failed", "game_id": id, "source": source, "reason": safeDiagnosticReason(err)})
		} else {
			a.pruneMatchTags()
		}
	}
	return rows
}
func (a *app) pruneMatchTags() {
	a.matchTagsMu.Lock()
	defer a.matchTagsMu.Unlock()
	if a.storage == nil {
		return
	}
	files := []string{}
	_ = filepath.WalkDir(filepath.Join(a.storage.root, "match-tags"), func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(path, ".json") {
			files = append(files, path)
		}
		return nil
	})
	if len(files) <= 5000 {
		return
	}
	sort.Slice(files, func(i, j int) bool {
		a, e := os.Stat(files[i])
		b, f := os.Stat(files[j])
		return e == nil && (f != nil || a.ModTime().Before(b.ModTime()))
	})
	for _, f := range files[:len(files)-5000] {
		_ = os.Remove(f)
	}
}
func (a *app) attachStoredMatchTags(m *gameplayMatch) {
	scope := matchTagScope(*m)
	sources := []string{dataSourceSGP, dataSourceLCU}
	if isRiotRegion(scope) {
		sources = []string{dataSourceRiot}
	}
	for _, source := range sources {
		rows := a.readMatchTags(source, scope, m.GameID)
		for _, tag := range rows {
			for i := range m.Participants {
				if m.Participants[i].ParticipantID == tag.ParticipantID {
					m.Participants[i].Keyword = tag.Keyword
				}
			}
		}
		if len(rows) > 0 {
			m.TagsAvailable = true
			return
		}
	}
}
func (a *app) matchTagsDiagnostic(source string, id int64, rows []participantMatchTags) {
	a.recordDiagnostic(map[string]any{"event": "match_tags_computed", "source": source, "game_id": id, "participants": len(rows), "param_version": fmt.Sprint(matchTagModelVersion())})
}
