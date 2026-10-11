package main

import (
	"bufio"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"testing"
	"time"
)

func TestR272DiagnosticReplay(t *testing.T) {
	input := os.Getenv("R272_REPLAY_INPUT")
	if input == "" {
		t.Skip("user-log replay input not supplied")
	}
	file, err := os.Open(input)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()
	type row struct {
		at    time.Time
		event map[string]any
		bytes int
	}
	rows := []row{}
	runs := map[string]bool{}
	scanner := bufio.NewScanner(gz)
	scanner.Buffer(make([]byte, 65536), 16<<20)
	for scanner.Scan() {
		var e map[string]any
		if json.Unmarshal(scanner.Bytes(), &e) != nil {
			continue
		}
		var at time.Time
		switch v := e["time"].(type) {
		case string:
			at, _ = time.Parse(time.RFC3339Nano, v)
		case float64:
			at = time.UnixMilli(int64(v))
		}
		if at.IsZero() {
			continue
		}
		rows = append(rows, row{at, e, len(scanner.Bytes()) + 1})
		if e["event"] == "app_start" && e["version"] == "0.12.82" {
			runs[fmt.Sprint(e["run_id"])] = true
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	var lo, hi time.Time
	for _, r := range rows {
		if runs[fmt.Sprint(r.event["run_id"])] {
			if lo.IsZero() || r.at.Before(lo) {
				lo = r.at
			}
			if r.at.After(hi) {
				hi = r.at
			}
		}
	}
	selected := []row{}
	for _, r := range rows {
		if runs[fmt.Sprint(r.event["run_id"])] || r.event["run_id"] == nil && r.event["event"] == "riot_request" && !r.at.Before(lo) && !r.at.After(hi) {
			selected = append(selected, r)
		}
	}
	sort.SliceStable(selected, func(i, j int) bool { return selected[i].at.Before(selected[j].at) })
	a := &app{}
	dedup := map[string]int{}
	windows := map[string]string{}
	cold := map[string]bool{}
	before, after, sequence := 0, 0, 0
	lcu := map[string][]map[string]any{}
	riot := map[string]map[string]any{}
	timeline := map[string]bool{}
	by := map[string]int{}
	oldFingerprint := buildFingerprint
	buildFingerprint = "0123456789ab"
	defer func() { buildFingerprint = oldFingerprint }()
	emit := func(e map[string]any, at time.Time) {
		sequence++
		e["log_seq"] = sequence
		if e["run_id"] == nil {
			e["run_id"] = "000000000000000000000000"
		}
		raw, err := marshalDiagnosticRecord(e, at)
		if err != nil {
			t.Fatal(err)
		}
		after += len(raw) + 1
		by[fmt.Sprint(e["event"])] += len(raw) + 1
	}
	for _, r := range selected {
		before += r.bytes
		e := r.event
		name := fmt.Sprint(e["event"])
		run := fmt.Sprint(e["run_id"])
		batch := fmt.Sprint(e["run_id"]) + ":" + fmt.Sprint(r.at.Unix()/60)
		switch name {
		case "lcu_request":
			key := fmt.Sprint(e["run_id"]) + ":" + fmt.Sprint(e["window_start"])
			e["count"] = intNumber(e["count"])
			d, _ := e["duration_ms"].(map[string]any)
			e["duration_ms"] = map[string]int64{"max": int64(intNumber(d["max"]))}
			lcu[key] = append(lcu[key], e)
			continue
		case "riot_match_items", "riot_match_mode":
			b := riot[batch]
			if b == nil {
				b = map[string]any{"event": "riot_matches_summary", "counts": map[string]int{}, "max_items": 0}
				riot[batch] = b
			}
			b["counts"].(map[string]int)[name]++
			b["max_items"] = max(intNumber(b["max_items"]), intNumber(e["items"]))
			continue
		case "match_timeline_lcu_incomplete", "match_timeline_failed", "match_timeline_client":
			for _, k := range []string{"frames", "events"} {
				e[k] = intNumber(e[k])
			}
			a.recordTimelineDiagnostic(context.WithValue(context.Background(), timelineDiagnosticBatchKey{}, batch), e)
			timeline[batch] = true
			continue
		}
		for _, k := range []string{"time", "log_seq", "build_fingerprint", "run_id"} {
			delete(e, k)
		}
		if name == "client_cold_launch_timeline" {
			if cold[run] {
				continue
			}
			cold[run] = true
		}
		if name == "accept_focus_window" {
			if rows, ok := e["client_windows"].([]any); ok && len(rows) > 10 {
				e["client_windows"] = rows[:10]
			}
			shape, _ := json.Marshal(map[string]any{"client_windows": e["client_windows"], "foreground": e["foreground"], "observation_available": e["observation_available"]})
			if windows[run] == string(shape) {
				continue
			}
			windows[run] = string(shape)
		}
		if isNoisyDiagnosticEvent(name) {
			payload, _ := json.Marshal(e)
			sum := sha256.Sum256(payload)
			key := fmt.Sprintf("%s|%s|%x", run, name, sum)
			dedup[key]++
			if dedup[key] > 1 {
				if dedup[key]%100 == 0 {
					emit(map[string]any{"event": "diagnostic_dedup", "source_event": name, "repeat_count": dedup[key]}, r.at)
				}
				continue
			}
			e["repeat_count"] = 1
		}
		summaries, handled := a.successfulRequestDiagnostics(e, r.at, false)
		for _, summary := range summaries {
			emit(summary, r.at)
		}
		if handled {
			continue
		}
		if compact, keep := a.compactDiagnostic(e, r.at); keep {
			emit(compact, r.at)
		}
	}
	summaries, _ := a.successfulRequestDiagnostics(nil, hi, true)
	for _, summary := range summaries {
		emit(summary, hi)
	}
	for _, rows := range lcu {
		for _, e := range lcuRequestDiagnosticWindows(rows) {
			emit(e, hi)
		}
	}
	for _, e := range riot {
		emit(e, hi)
	}
	for batch := range timeline {
		b := a.timelineDiagnosticBatches[batch]
		if b != nil {
			emit(map[string]any{"event": "match_timeline_summary", "counts": b.Counts, "max_frames": b.MaxFrames, "max_events": b.MaxEvents, "samples": b.Samples}, hi)
		}
	}
	hours := hi.Sub(lo).Hours()
	report := map[string]any{"scope": "0.12.82 run IDs plus legacy Riot rows within UTC window; production JSONL encoder/compactor and LCU merger; missing legacy overview IDs conservatively grouped per run/minute", "estimate_only": true, "duration_hours": hours, "window_start": lo, "window_end": hi, "events": len(selected), "before_bytes": before, "after_bytes": after, "before_bytes_per_hour": float64(before) / hours, "after_bytes_per_hour": float64(after) / hours, "after_by_event": by, "additional_events_bytes_per_hour": 110000, "combined_bytes_per_hour": float64(after)/hours + 110000, "synthetic_new_diagnostic_rate": "observed49 overview requests/2.383h (~21/h), plus100 LP events/h at500bytes and2500bytes/shape, rounded up110KB/h (includes credential-source events); no new machine log"}
	raw, _ := json.MarshalIndent(report, "", "  ")
	if output := os.Getenv("R272_REPLAY_OUTPUT"); output != "" {
		if err := os.WriteFile(output, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("replay events=%d before=%d after=%d after_bytes_per_hour=%.0f", len(selected), before, after, float64(after)/hours)
	if hours <= 0 || float64(after)/hours+110000 > 1500000 {
		t.Fatal("replay log budget exceeded")
	}
}
