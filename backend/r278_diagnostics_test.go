package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestR278P14LCUQuantilesSurvivePacking(t *testing.T) {
	rows := []map[string]any{}
	for _, path := range []string{"/one", "/two"} {
		rows = append(rows, lcuRequestDiagnosticEvent(&lcuRequestDiagnosticBucket{windowStart: time.Unix(100, 0), method: "GET", path: path, samples: []lcuRequestDiagnosticSample{{durationMS: 10, httpStatus: 200}, {durationMS: 100, httpStatus: 200}}}))
	}
	out := lcuRequestDiagnosticWindows(rows)
	if len(out) != 2 {
		t.Fatalf("route summaries lost: %#v", out)
	}
	for _, row := range out {
		m, ok := row["duration_ms"].(map[string]int64)
		if !ok || m["p50"] == 0 || m["p95"] != 100 {
			t.Fatalf("percentiles lost: %#v", row)
		}
	}
}
func TestR278P14SnapshotRetainsChanges(t *testing.T) {
	a := &app{}
	at := time.Unix(100, 0)
	for i := 0; i < 10; i++ {
		out, handled := a.snapshotDiagnostics(map[string]any{"event": "overview_art_source", "skin_id": 123, "poster_available": true}, at, false)
		if !handled || len(out) != map[bool]int{true: 1, false: 0}[i == 0] {
			t.Fatalf("duplicate not compacted: %#v", out)
		}
	}
	out, handled := a.snapshotDiagnostics(map[string]any{"event": "overview_art_source", "skin_id": 124, "poster_available": true}, at, false)
	if !handled || len(out) != 1 {
		t.Fatal("semantic change suppressed")
	}
	sums, _ := a.snapshotDiagnostics(nil, at.Add(time.Hour), true)
	if len(sums) != 1 || sums[0]["count"] != 9 {
		t.Fatalf("repeat count lost %#v", sums)
	}
}

func TestR278P14ShapePerPlayerTenMinuteWindow(t *testing.T) {
	a := r175App(t)
	at := time.Unix(100, 0)
	s := newOverviewResponseShape()
	s.account = "private-player:HN10"
	s.Bytes = 100
	s.Top["matches"] = 80
	a.recordOverviewShape(s, at)
	s.Bytes = 101
	a.recordOverviewShape(s, at.Add(time.Minute))
	s.account = "different-player:HN10"
	a.recordOverviewShape(s, at.Add(time.Minute))
	s.account = "private-player:HN10"
	a.recordOverviewShape(s, at.Add(10*time.Minute))
	rows := r175Events(t, a, "overview_response_shape")
	if len(rows) != 4 {
		t.Fatalf("shape records lost %#v", rows)
	}
	if rows[1]["top_level_bytes"] != nil || rows[1]["response_bytes"] != float64(101) {
		t.Fatal("repeat shape must be bytes only")
	}
	if rows[2]["top_level_bytes"] == nil || rows[3]["top_level_bytes"] == nil {
		t.Fatal("new player/window lacks full shape")
	}
	for _, row := range rows {
		raw, _ := json.Marshal(row)
		if strings.Contains(string(raw), "private-player") || strings.Contains(string(raw), "different-player") {
			t.Fatal("player key leaked")
		}
	}
}
