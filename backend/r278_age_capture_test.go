package main

import (
	"encoding/json"
	"math"
	"testing"
	"time"
)

// Independently reproduce the diagnostic's millisecond timestamp/0.01 minute
// contract. The measured call interval is the only range; no slack is added.
func r278CaptureAgeBounds(created int64, from, until time.Time) (float64, float64) {
	rounded := func(at time.Time) float64 {
		return math.Round(math.Max(0, float64(at.UnixMilli()-created)/60000)*100) / 100
	}
	return rounded(from), rounded(until)
}

func r278CaptureAgesValid(event map[string]any, created int64, from, until time.Time) bool {
	lower, upper := r278CaptureAgeBounds(created, from, until)
	shown, shownOK := event["shown_newest_age_min"].(float64)
	sgp, sgpOK := event["sgp_newest_queue_age_min"].(float64)
	return !until.Before(from) && lower >= 8 && shownOK && sgpOK && shown == sgp &&
		math.Round(shown*100)/100 == shown && shown >= lower && shown <= upper
}

func r278AssertCaptureAges(t *testing.T, event map[string]any, created int64, from, until time.Time) {
	t.Helper()
	lower, upper := r278CaptureAgeBounds(created, from, until)
	if !r278CaptureAgesValid(event, created, from, until) {
		t.Fatalf("timestamp-derived age contract: shown=%v sgp=%v expected rounded interval [%v,%v], lower >= 8 and equal fields", event["shown_newest_age_min"], event["sgp_newest_queue_age_min"], lower, upper)
	}
}

func TestR278AgeCaptureProductTimestampContract(t *testing.T) {
	created := time.UnixMilli(1800000000000)
	ref := r161Ref(1)
	matches := []gameplayMatch{{GameID: 112, CreatedAt: created.UnixMilli(), QueueID: 440, Result: "win", SubjectParticipantID: 1, Participants: []gameplayParticipant{{ParticipantID: 1, PlayerRef: ref}}}}
	for _, elapsed := range []time.Duration{400 * time.Millisecond, 900 * time.Millisecond, 1570 * time.Millisecond} {
		t.Run(elapsed.String(), func(t *testing.T) {
			sampled := created.Add(8*time.Minute + elapsed)
			result := livePlayerMatchesResult{Matches: matches, Evidence: &liveHistoryEvidence{SGPRequested: true, SGPOK: true, SGP: matches}}
			raw := liveHistoryFreshnessDiagnostic(result, ref, 440, 100, 0, true, sampled)
			encoded, err := json.Marshal(raw)
			if err != nil {
				t.Fatal(err)
			}
			var event map[string]any
			if err := json.Unmarshal(encoded, &event); err != nil {
				t.Fatal(err)
			}
			if event["is_current"] != true {
				t.Fatal("is_current contract", event)
			}
			r278AssertCaptureAges(t, event, created.UnixMilli(), sampled, sampled)
		})
	}
}

func TestR278AgeCaptureRejectsWrongValues(t *testing.T) {
	created := time.UnixMilli(1800000000000)
	from, until := created.Add(8*time.Minute+400*time.Millisecond), created.Add(8*time.Minute+500*time.Millisecond)
	event := func(a, b float64) map[string]any {
		return map[string]any{"shown_newest_age_min": a, "sgp_newest_queue_age_min": b}
	}
	if !r278CaptureAgesValid(event(8.01, 8.01), created.UnixMilli(), from, until) {
		t.Fatal("elapsed timestamp age rejected")
	}
	for _, wrong := range []float64{9, 7, 8, 8 + 400.0/60000} {
		if r278CaptureAgesValid(event(wrong, wrong), created.UnixMilli(), from, until) {
			t.Fatalf("wrong age accepted: %v", wrong)
		}
	}
	if r278CaptureAgesValid(event(8.01, 8.02), created.UnixMilli(), from, until) {
		t.Fatal("different fields accepted")
	}
	// Even a wide real interval cannot admit an unrounded diagnostic.
	if r278CaptureAgesValid(event(8.011, 8.011), created.UnixMilli(), from, created.Add(8*time.Minute+2*time.Second)) {
		t.Fatal("unrounded age accepted inside interval")
	}
	if r278CaptureAgesValid(event(7.99, 7.99), created.UnixMilli(), created.Add(7*time.Minute+59900*time.Millisecond), created.Add(8*time.Minute)) {
		t.Fatal("lower bound below eight accepted")
	}
}
