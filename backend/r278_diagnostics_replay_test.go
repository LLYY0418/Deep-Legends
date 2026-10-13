package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"testing"
	"time"
)

func TestR278P14RealReplay(t *testing.T) {
	input := os.Getenv("R278_REPLAY_INPUT")
	if input == "" {
		t.Skip("private real trace not supplied")
	}
	f, err := os.Open(input)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	type record struct {
		at   time.Time
		row  map[string]any
		size int
	}
	records := []record{}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 65536), 16<<20)
	lo, _ := time.Parse(time.RFC3339, "2026-10-13T14:59:15Z")
	hi, _ := time.Parse(time.RFC3339, "2026-10-13T15:59:01Z")
	for scanner.Scan() {
		var row map[string]any
		if json.Unmarshal(scanner.Bytes(), &row) != nil {
			continue
		}
		stamp, _ := row["time"].(string)
		at, _ := time.Parse(time.RFC3339Nano, stamp)
		if !at.Before(lo) && !at.After(hi) {
			records = append(records, record{at, row, len(scanner.Bytes()) + 1})
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	sort.SliceStable(records, func(i, j int) bool { return records[i].at.Before(records[j].at) })
	a := &app{}
	before, after, sequence := 0, 0, 0
	by := map[string]int{}
	lcu := map[string][]map[string]any{}
	saved := buildFingerprint
	buildFingerprint = "0123456789ab"
	defer func() { buildFingerprint = saved }()
	emit := func(row map[string]any, at time.Time) {
		copy := map[string]any{}
		for k, v := range row {
			copy[k] = v
		}
		sequence++
		copy["log_seq"] = sequence
		copy["run_id"] = "000000000000000000000000"
		raw, err := marshalDiagnosticRecord(copy, at)
		if err != nil {
			t.Fatal(err)
		}
		after += len(raw) + 1
		by[fmt.Sprint(row["event"])] += len(raw) + 1
	}
	for _, rec := range records {
		before += rec.size
		row := rec.row
		name := fmt.Sprint(row["event"])
		for _, k := range []string{"time", "log_seq", "build_fingerprint", "run_id"} {
			delete(row, k)
		}
		if name == "lcu_request" { // Legacy ten-second summaries are not raw requests: preserve every failure/slow group and omitted count.
			window := rec.at.Truncate(time.Minute).Format(time.RFC3339)
			paths, _ := row["paths"].([]any)
			if len(paths) == 0 {
				emit(row, rec.at)
				continue
			}
			for _, item := range paths {
				p := item.(map[string]any)
				statuses, _ := p["http_status"].(map[string]any)
				safe := intNumber(p["max_ms"]) <= 1000
				for code := range statuses {
					safe = safe && code == "200" || false
				}
				path := map[string]any{"event": "lcu_request", "window_start": window, "path": p["path"], "method": p["method"], "count": intNumber(p["count"]), "duration_ms": map[string]int64{"max": int64(intNumber(p["max_ms"]))}, "http_status": p["http_status"]}
				if safe {
					lcu[window] = append(lcu[window], path)
				} else {
					emit(path, rec.at)
				}
			}
			if intNumber(row["omitted_paths"]) > 0 {
				emit(map[string]any{"event": "lcu_request_legacy_omitted", "count": row["omitted_paths"], "window_start": row["window_start"]}, rec.at)
			}
			continue
		}
		sums, handled := a.successfulRequestDiagnostics(row, rec.at, false)
		for _, s := range sums {
			emit(s, rec.at)
		}
		if handled {
			continue
		}
		sums, handled = a.snapshotDiagnostics(row, rec.at, false)
		for _, s := range sums {
			emit(s, rec.at)
		}
		if handled {
			continue
		}
		if compact, keep := a.compactDiagnostic(row, rec.at); keep {
			emit(compact, rec.at)
		}
	}
	for _, rows := range lcu {
		for _, row := range lcuRequestDiagnosticWindows(rows) {
			emit(row, hi)
		}
	}
	for _, flush := range []func() []map[string]any{func() []map[string]any { v, _ := a.successfulRequestDiagnostics(nil, hi, true); return v }, func() []map[string]any { v, _ := a.snapshotDiagnostics(nil, hi, true); return v }} {
		for _, row := range flush() {
			emit(row, hi)
		}
	}
	// The source cannot associate legacy response shapes to players: keep all full structures.
	// New-event replay uses the observed UI actions and keeps all event envelopes in the byte total.
	added := 0
	for _, rec := range records {
		var event map[string]any
		switch rec.row["event"] {
		case "client_view_client":
			event = map[string]any{"event": "client_follow_client", "trigger": "login", "applied": true}
		case "overview_rerender_client":
			event = map[string]any{"event": "overview_reload_reason", "reason": "section-return"}
		case "sgp_history_circuit":
			event = map[string]any{"event": "history_outage_ui", "state": "shown", "source": "circuit"}
		case "overview_response_shape":
			event = map[string]any{"event": "overview_reference_fallback", "reason": "empty-revision", "count": 1}
		case "riot_local_rate_limited":
			event = map[string]any{"event": "match_autoload_client", "bound": true, "near": true, "requested": true, "started": false, "blocked_reason": "quota"}
		case "app_start":
			event = map[string]any{"event": "announcement_state", "id": "cn-history-outage", "notice_revision": "1", "source": "bundled", "action": "shown"}
		case "collection_dirty_marked":
			event = map[string]any{"event": "collection_new_marker", "kind": "skin", "source": "event", "latency_ms": 100}
		default:
			continue
		}
		if compact, keep := a.compactDiagnostic(event, rec.at); keep {
			prior := after
			emit(compact, rec.at)
			added += after - prior
		}
	}
	hours := 59.75 / 60
	report := map[string]any{"records": len(records), "before_bytes": before, "after_bytes": after, "before_bytes_per_hour": float64(before) / hours, "after_bytes_per_hour": float64(after) / hours, "added_bytes_per_hour": float64(added) / hours, "after_by_event": by, "scope": "same 0.12.84 trace segment, production encoder/compactors; full legacy shapes retained; legacy LCU grouped failure/slow evidence retained (individual raw samples unavailable), unknown omitted routes counted separately; new events mapped to observed UI actions", "duration_minutes": 59.75}
	raw, _ := json.MarshalIndent(report, "", "  ")
	if output := os.Getenv("R278_REPLAY_OUTPUT"); output != "" {
		if err := os.WriteFile(output, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("trace=%d before=%d after=%d per_hour=%.0f increment=%.0f", len(records), before, after, float64(after)/hours, float64(added)/hours)
	if float64(after)/hours > 1000000 || float64(added)/hours > 50000 {
		t.Fatal("R278 log budget exceeded")
	}
}
