package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"sort"
	"strings"
)

// R216: the shipped v2.1 scorer. Additional combat fields are never score inputs.
// Keep this implementation shared by history, timeline checkpoints and season scans.
type scoreParams struct {
	AssistFactor float64    `json:"assistFactor"`
	Weights      [6]float64 `json:"weights"`
}

var v2ScoreParams = scoreParams{1, [6]float64{.30, .20, .22, .08, .12, .08}}
var v21ScoreParams = scoreParams{.4, [6]float64{.36, .26, .16, .02, .18, .02}}
var currentScoreParams = v21ScoreParams

const matchScoreParamVersion = "v2.1"

var matchScoreKeys = [6]string{"kda", "kp", "damage", "gold", "cs", "vision"}

type matchScorePart struct {
	Key          string  `json:"key"`
	Value        float64 `json:"value"`
	Reference    float64 `json:"reference"`
	Norm         float64 `json:"norm"`
	Weight       float64 `json:"weight"`
	Contribution float64 `json:"contribution"`
}
type matchScore struct {
	Value        float64          `json:"value"`
	RawScore     float64          `json:"rawScore"`
	Rank         int              `json:"rank"`
	Total        int              `json:"total"`
	Badge        string           `json:"badge"`
	Parts        []matchScorePart `json:"parts"`
	RoleComplete bool             `json:"-"`
}

func scoreGroup(p gameplayParticipant) int64 {
	if p.SubteamID > 0 {
		return -p.SubteamID
	}
	return p.TeamID
}
func scorePosition(p gameplayParticipant) string {
	return strings.ToLower(strings.TrimSpace(p.Position))
}
func scoreMedian(values []float64) float64 {
	v := append([]float64(nil), values...)
	sort.Float64s(v)
	n := len(v)
	if n%2 == 1 {
		return v[n/2]
	}
	return (v[n/2-1] + v[n/2]) / 2
}
func scoreValid(v float64) bool { return v >= 0 && !math.IsNaN(v) && !math.IsInf(v, 0) }
func computeMatchScoresGo(m gameplayMatch, params scoreParams) map[int64]*matchScore {
	out := map[int64]*matchScore{}
	ps := m.Participants
	if len(ps) < 2 || m.Result == "remake" || m.Result == "unknown" {
		return out
	}
	groups := map[int64][]int{}
	ids := map[int64]bool{}
	for i, p := range ps {
		if p.ParticipantID <= 0 || p.ParticipantID > 9007199254740991 || ids[p.ParticipantID] {
			return out
		}
		ids[p.ParticipantID] = true
		groups[scoreGroup(p)] = append(groups[scoreGroup(p)], i)
	}
	role := len(ps) == 10 && len(groups) == 2 && (m.QueueID == 400 || m.QueueID == 420 || m.QueueID == 430 || m.QueueID == 440 || m.QueueID == 490)
	for _, group := range groups {
		seen := map[string]bool{}
		for _, i := range group {
			seen[scorePosition(ps[i])] = true
		}
		for _, r := range []string{"top", "jungle", "middle", "bottom", "utility"} {
			role = role && seen[r]
		}
		role = role && len(group) == 5
	}
	values := make([][6]float64, len(ps))
	active := [6]bool{}
	total := 0.0
	for i, p := range ps {
		tk := 0.
		combat := !p.scoreMissing["combat"]
		for _, j := range groups[scoreGroup(p)] {
			if ps[j].scoreMissing["combat"] || ps[j].Kills < 0 {
				combat = false
			}
			tk += float64(ps[j].Kills)
		}
		v := [6]float64{math.NaN(), math.NaN(), float64(p.Damage), float64(p.Gold), math.NaN(), float64(p.VisionScore)}
		if combat && p.Kills >= 0 && p.Assists >= 0 && p.Deaths >= 0 {
			v[0] = (float64(p.Kills) + params.AssistFactor*float64(p.Assists)) / math.Max(1, float64(p.Deaths))
			if tk > 0 {
				v[1] = math.Min(1, float64(p.Kills+p.Assists)/tk)
			}
		}
		if m.Duration > 0 {
			v[4] = float64(p.CS) / (float64(m.Duration) / 60)
		}
		for j, key := range matchScoreKeys {
			if p.scoreMissing[key] {
				v[j] = math.NaN()
			}
		}
		values[i] = v
	}
	for j := 0; j < 6; j++ {
		valid, positive := true, false
		for _, v := range values {
			valid = valid && scoreValid(v[j])
			positive = positive || v[j] > 0
		}
		active[j] = valid && positive && params.Weights[j] > 0
		if active[j] {
			total += params.Weights[j]
		}
	}
	if total <= 0 {
		return out
	}
	for i, p := range ps {
		s := &matchScore{RawScore: 2, Total: len(ps), RoleComplete: role, Parts: []matchScorePart{}}
		for j, key := range matchScoreKeys {
			if !active[j] {
				continue
			}
			peers := []float64{}
			for k, q := range ps {
				if !role || j < 2 || scorePosition(q) == scorePosition(p) {
					peers = append(peers, values[k][j])
				}
			}
			ref := scoreMedian(peers)
			if ref == 0 {
				for _, v := range peers {
					ref += v
				}
				ref /= float64(len(peers))
			}
			x, b := values[i][j], ref
			if j == 0 {
				x, b = math.Sqrt(x), math.Sqrt(b)
			}
			norm := .5
			if ref > 0 {
				norm = x / (x + b)
			}
			weight := params.Weights[j] / total
			part := matchScorePart{key, values[i][j], ref, norm, weight, 8 * norm * weight}
			s.Parts = append(s.Parts, part)
		}
		// JS computes 2 + the sum, rather than incrementing an initial 2.
		sum := 0.
		for _, part := range s.Parts {
			sum += part.Contribution
		}
		s.RawScore = 2 + sum
		s.Value = math.Floor(s.RawScore*10+.5) / 10
		out[p.ParticipantID] = s
	}
	ranked := append([]gameplayParticipant(nil), ps...)
	sort.Slice(ranked, func(i, j int) bool {
		a, b := out[ranked[i].ParticipantID], out[ranked[j].ParticipantID]
		if a.RawScore == b.RawScore {
			return ranked[i].ParticipantID < ranked[j].ParticipantID
		}
		return a.RawScore > b.RawScore
	})
	known, w, l := true, false, false
	for _, p := range ps {
		known = known && !p.scoreMissing["win"]
		w = w || p.Win
		l = l || !p.Win
	}
	seen := map[bool]bool{}
	for i, p := range ranked {
		s := out[p.ParticipantID]
		s.Rank = i + 1
		if m.QueueID != 1700 && m.QueueID != 1710 && known && w && l && !seen[p.Win] {
			if p.Win {
				s.Badge = "MVP"
			} else {
				s.Badge = "SVP"
			}
			seen[p.Win] = true
		}
	}
	return out
}
func applyMatchScores(m *gameplayMatch) {
	scores := computeMatchScoresGo(*m, currentScoreParams)
	for i := range m.Participants {
		m.Participants[i].Score = scores[m.Participants[i].ParticipantID]
	}
	m.Revision = ""
	if data, err := json.Marshal(m); err == nil {
		sum := sha256.Sum256(data)
		m.Revision = hex.EncodeToString(sum[:8])
	}
}
func (a *app) recordMatchScores(source string, m gameplayMatch, contexts ...context.Context) {
	if a == nil || m.GameID <= 0 {
		return
	}
	a.cacheMatchTagInput(m)
	if len(contexts) == 0 {
		return
	}
	batch, _ := contexts[0].Value(matchScoreBatchKey{}).(*matchScoreBatch)
	if batch == nil {
		return
	}
	batch.Lock()
	defer batch.Unlock()
	if batch.games[m.GameID] {
		return
	}
	batch.games[m.GameID] = true
	valid := false
	for _, p := range m.Participants {
		if p.Score != nil {
			valid = true
			if p.Score.Badge != "" {
				batch.badges[p.Score.Badge]++
			}
		}
	}
	if !valid {
		batch.failed++
		if batch.failed <= 3 {
			a.recordDiagnostic(map[string]any{"event": "match_score_failed", "source": source, "reason": "missing_score", "version": matchScoreParamVersion})
		}
	}
}

// JSON presence must survive conversion and disk round trips. A missing dimension
// is omitted for the whole match; zero is a valid observed value.
func scoreMissingFields(raw map[string]json.RawMessage, riot bool) map[string]bool {
	fields := map[string][]string{"combat": {"kills", "deaths", "assists"}, "damage": {"damage"}, "gold": {"gold"}, "cs": {"cs"}, "vision": {"visionScore"}, "win": {"win"}, "wardsPlaced": {"wardsPlaced"}, "wardsKilled": {"wardsKilled"}}
	if riot {
		fields["damage"] = []string{"totalDamageDealtToChampions"}
		fields["gold"] = []string{"goldEarned"}
		fields["cs"] = []string{"totalMinionsKilled", "neutralMinionsKilled"}
	}
	missing := map[string]bool{}
	for key, names := range fields {
		for _, name := range names {
			v, ok := raw[name]
			if !ok || string(v) == "null" {
				missing[key] = true
			}
		}
	}
	return missing
}
func (p *riotParticipant) UnmarshalJSON(data []byte) error {
	type plain riotParticipant
	var v plain
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	*p = riotParticipant(v)
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	p.scoreMissing = scoreMissingFields(raw, true)
	return nil
}
func (p *gameplayParticipant) UnmarshalJSON(data []byte) error {
	type plain gameplayParticipant
	var v plain
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	*p = gameplayParticipant(v)
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	p.scoreMissing = scoreMissingFields(raw, false)
	return nil
}
func (p *lcuParticipant) UnmarshalJSON(data []byte) error {
	type plain lcuParticipant
	var v plain
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	*p = lcuParticipant(v)
	var raw struct {
		Stats map[string]json.RawMessage `json:"stats"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	p.scoreMissing = scoreMissingFields(raw.Stats, true)
	return nil
}

func marshalScorePresence(value any, missing map[string]bool, riot bool) ([]byte, error) {
	data, err := json.Marshal(value)
	if err != nil || len(missing) == 0 {
		return data, err
	}
	var raw map[string]json.RawMessage
	if err = json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	fields := map[string][]string{"combat": {"kills", "deaths", "assists"}, "damage": {"damage"}, "gold": {"gold"}, "cs": {"cs"}, "vision": {"visionScore"}, "win": {"win"}, "wardsPlaced": {"wardsPlaced"}, "wardsKilled": {"wardsKilled"}}
	if riot {
		fields["damage"] = []string{"totalDamageDealtToChampions"}
		fields["gold"] = []string{"goldEarned"}
		fields["cs"] = []string{"totalMinionsKilled", "neutralMinionsKilled"}
	}
	for key, names := range fields {
		if missing[key] {
			for _, name := range names {
				delete(raw, name)
			}
		}
	}
	return json.Marshal(raw)
}
func (p riotParticipant) MarshalJSON() ([]byte, error) {
	type plain riotParticipant
	return marshalScorePresence(plain(p), p.scoreMissing, true)
}
func (p gameplayParticipant) MarshalJSON() ([]byte, error) {
	type plain gameplayParticipant
	return marshalScorePresence(plain(p), p.scoreMissing, false)
}
