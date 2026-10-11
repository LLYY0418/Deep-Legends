package main

import (
	"testing"
	"time"
)

func TestR272SuccessRequestSummaryRetainsFailuresAndTotals(t *testing.T) {
	a := r175App(t)
	now := time.Now()
	input := func(duration int) map[string]any {
		return map[string]any{"event": "sgp_request", "method": "GET", "route": "match-history", "path": "/matches/{puuid}", "http_status": 200, "body_bytes": 1000, "duration_ms": duration}
	}
	for _, duration := range []int{10, 20, 30} {
		rows, handled := a.successfulRequestDiagnostics(input(duration), now, false)
		if !handled {
			t.Fatal("success not aggregated")
		}
		for _, row := range rows {
			a.appendDiagnosticEvent(row)
		}
	}
	failure := input(40)
	failure["http_status"] = 401
	rows, handled := a.successfulRequestDiagnostics(failure, now, false)
	if handled || len(rows) != 0 {
		t.Fatal("failure suppressed")
	}
	a.recordDiagnostic(failure)
	a.flushSuccessfulRequestDiagnostics()
	summaries := r175Events(t, a, "sgp_request_summary")
	if len(summaries) != 1 || summaries[0]["count"] != float64(2) || summaries[0]["body_bytes"] != float64(2000) || summaries[0]["duration_total_ms"] != float64(50) {
		t.Fatal("lost summary totals", summaries)
	}
	if len(r175Events(t, a, "sgp_request")) != 2 {
		t.Fatal("first success/failure lost")
	}
}
func TestR272ShapeAndLPDiagnosticHourProjection(t *testing.T) {
	a := r175App(t)
	shape := newOverviewResponseShape()
	shape.observe([]byte(`{"matches":[{"participants":[{"gameName":"Anonymous","score":{"parts":[]}}]}]}`))
	for i := 0; i < 21; i++ {
		a.recordDiagnostic(shape.event())
		a.recordDiagnostic(map[string]any{"event": "lp_annotate", "path": "cache", "is_current": true, "ranked_matches": 4, "records_available": 4, "matched": 4, "skipped_reason": ""})
		a.recordDiagnostic(map[string]any{"event": "lp_chip_render", "reason": "render", "ranked_matches": 4, "with_delta": 4, "chips": 4, "revision_skipped": 0})
	}
	raw, err := a.storage.readDiagnosticLog()
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) > 100000 {
		t.Fatal("new diagnostics exceed100KB/h allocation", len(raw))
	}
	if len(r175Events(t, a, "overview_response_shape")) != 21 || len(r175Events(t, a, "lp_annotate")) != 1 || len(r175Events(t, a, "lp_chip_render")) != 1 {
		t.Fatal("shape requests/dedup events lost")
	}
}
