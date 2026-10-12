package main

import (
	"math"
	"testing"
	"time"
)

// These tests are serial. Give the diagnostic-count fixture its own process
// summary; callbacks/timers belonging to the preceding fixture keep their owner.
func r276IsolateRelaySummary(t *testing.T) {
	t.Helper()
	previous := riotRelays
	riotRelays = &riotRelayState{}
	owned := riotRelays
	t.Cleanup(func() {
		owned.flushSummary(nil, true)
		// r276-capture-safe: serial test cleanup restores the previous relay owner after synchronous diagnostics complete
		riotRelays = previous
	})
}

func TestR276DiagnosticCountIgnoresEarlierFixtureSummary(t *testing.T) {
	r276IsolateRelaySummary(t)
	// No timer or external service: construct the preceding sample's summary.
	riotRelays.recordRequest("", 200, "match", nil)
	t.Run("next-sample", TestDiagnosticsExposeCurrentLogSizeAndEventCount)
}

// Enforce the actual age formula over the measured capture interval, with no
// added time tolerance. This is data correctness, not a latency budget.
func r276AgeWithinCapture(age float64, created int64, from, until time.Time) bool {
	rounded := func(now time.Time) float64 {
		return math.Round(math.Max(0, float64(now.UnixMilli()-created)/60000)*100) / 100
	}
	return age >= rounded(from) && age <= rounded(until)
}

func TestR276RosterAgeAccountsForElapsedCapture(t *testing.T) {
	created := time.UnixMilli(100000)
	from, until := created.Add(8*time.Minute+400*time.Millisecond), created.Add(8*time.Minute+500*time.Millisecond)
	if !r276AgeWithinCapture(8.01, created.UnixMilli(), from, until) {
		t.Fatal("real elapsed capture age rejected")
	}
	for _, wrong := range []float64{8, 8.02, 9} {
		if r276AgeWithinCapture(wrong, created.UnixMilli(), from, until) {
			t.Fatalf("wrong capture age accepted: %v", wrong)
		}
	}
}
