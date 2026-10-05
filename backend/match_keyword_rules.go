package main

import (
	_ "embed"
	"encoding/json"
	"math"
)

type matchKeywordRuleParams struct {
	LeftThreshold  float64 `json:"leftThreshold"`
	RightThreshold float64 `json:"rightThreshold"`
	EndingBaseline float64 `json:"endingBaseline"`
	EndingGap      float64 `json:"endingGap"`
	EndingWindow   int     `json:"endingWindow"`
	PeakGap        float64 `json:"peakGap"`
}
type matchTimelineScoreParams struct {
	MissingPolicy    string      `json:"missingPolicy"`
	SmoothingMinutes float64     `json:"smoothingMinutes"`
	Score            scoreParams `json:"score"`
}
type matchKeywordLookupRow struct {
	Win     bool   `json:"win"`
	TeamMax bool   `json:"teamMax"`
	Left    string `json:"left"`
	Right   string `json:"right"`
	Last    string `json:"last"`
	Keyword string `json:"keyword"`
}
type matchKeywordModel struct {
	Rules    matchKeywordRuleParams   `json:"rules"`
	Timeline matchTimelineScoreParams `json:"timeline"`
	Table    []matchKeywordLookupRow  `json:"table"`
}
type matchTimelineAnalysis struct {
	Left  string `json:"left"`
	Right string `json:"right"`
	Last  string `json:"last"`
}

// R218 authorizes R217's fixed rule shape. Only its numerical scale and the
// timeline's missing-dimension/smoothing policy are selected by grouped CV.
// Settlement scores and display suppression remain frozen R216 v2.1 behavior.
//
//go:embed testdata/r217/keyword-candidate.json
var matchKeywordCandidateJSON []byte
var currentMatchKeywordModel = func() matchKeywordModel {
	var model matchKeywordModel
	if err := json.Unmarshal(matchKeywordCandidateJSON, &model); err != nil {
		panic(err)
	}
	if model.Rules.EndingWindow != 7 || len(model.Table) != 42 {
		panic("invalid frozen keyword rule shape")
	}
	return model
}()

func analyzeMatchKeywordCurve(scores []float64, p matchKeywordRuleParams) matchTimelineAnalysis {
	if len(scores) < 2 {
		return matchTimelineAnalysis{}
	}
	for _, s := range scores {
		if !scoreValid(s) {
			return matchTimelineAnalysis{}
		}
	}
	k := len(scores) / 2
	a := matchTimelineAnalysis{Left: "DOWN", Right: "DOWN", Last: "FAIR"}
	if scoreMedian(scores[:k]) >= p.LeftThreshold {
		a.Left = "UP"
	}
	if scoreMedian(scores[k:]) >= p.RightThreshold {
		a.Right = "UP"
	}
	final := scores[len(scores)-1]
	switch {
	case a.Right == "UP" && final < p.EndingBaseline:
		a.Last = "POOR"
	case a.Right == "DOWN" && final >= p.EndingBaseline:
		a.Last = "EXCELLENT"
	default:
		start := max(0, len(scores)-1-p.EndingWindow)
		peak := scores[start]
		for _, s := range scores[start : len(scores)-1] {
			peak = math.Max(peak, s)
		}
		if final-peak >= p.EndingGap {
			a.Last = "GOOD"
		}
	}
	return a
}

func lookupMatchKeyword(scores []float64, win, teamMax bool, model matchKeywordModel) (string, matchTimelineAnalysis) {
	a := analyzeMatchKeywordCurve(scores, model.Rules)
	for _, row := range model.Table {
		if row.Win != win || row.TeamMax != teamMax || row.Left != a.Left || row.Right != a.Right || row.Last != a.Last {
			continue
		}
		if win && a.Left == "UP" && a.Right == "UP" && a.Last == "GOOD" {
			peak := scores[0]
			for _, s := range scores {
				peak = math.Max(peak, s)
			}
			if scores[len(scores)-1]-peak >= model.Rules.PeakGap {
				return "unstoppable", a
			}
			return "leader", a
		}
		return row.Keyword, a
	}
	return "", a // An unseen combination never falls through to a guessed label.
}

func smoothTimelineScore(s *matchScore, at int64, params matchTimelineScoreParams) float64 {
	value := s.RawScore
	if params.MissingPolicy == "neutral-baseline" {
		observed, total := 0., 0.
		for _, weight := range params.Score.Weights {
			total += weight
		}
		for _, part := range s.Parts {
			for i, key := range matchScoreKeys {
				if key == part.Key {
					observed += params.Score.Weights[i]
				}
			}
		}
		if total > 0 {
			value = 6 + (value-6)*observed/total
		}
	}
	if params.SmoothingMinutes > 0 {
		minutes := float64(at) / 60
		value = 6 + (value-6)*minutes/(minutes+params.SmoothingMinutes)
	}
	return value
}
