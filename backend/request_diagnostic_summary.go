package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"
)

type successRequestDiagnostic struct {
	at                                           time.Time
	event                                        map[string]any
	count, bytes, duration, maxDuration, retries int
}

func diagnosticInteger(value any) int {
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	}
	return 0
}

// Keep each route's first success and all failures. Repeated successes retain
// request counts, bytes and timing totals in a bounded one-minute summary.
func (a *app) successfulRequestDiagnostics(event map[string]any, now time.Time, flush bool) ([]map[string]any, bool) {
	output := []map[string]any{}
	a.successRequestDiagnosticMu.Lock()
	defer a.successRequestDiagnosticMu.Unlock()
	for key, bucket := range a.successRequestDiagnostics {
		if flush || now.Sub(bucket.at) >= time.Minute {
			if bucket.count > 0 {
				output = append(output, map[string]any{"event": "sgp_request_summary", "method": bucket.event["method"], "route": bucket.event["route"], "path": bucket.event["path"], "window_start": bucket.at.UTC().Format(time.RFC3339), "count": bucket.count, "body_bytes": bucket.bytes, "duration_total_ms": bucket.duration, "duration_max_ms": bucket.maxDuration, "retried_count": bucket.retries, "http_status": 200})
			}
			delete(a.successRequestDiagnostics, key)
		}
	}
	if flush {
		return output, false
	}
	if event["event"] != "sgp_request" || diagnosticInteger(event["http_status"]) != 200 || event["read_failed"] == true || event["parse_failed"] == true || event["error_kind"] != nil {
		return output, false
	}
	raw, _ := json.Marshal([]any{event["method"], event["route"], event["path"]})
	key := fmt.Sprintf("%x", sha256.Sum256(raw))
	if a.successRequestDiagnostics == nil {
		a.successRequestDiagnostics = map[string]*successRequestDiagnostic{}
	}
	bucket := a.successRequestDiagnostics[key]
	if bucket == nil {
		// A run with many distinct paths remains bounded; unknown routes are emitted.
		if len(a.successRequestDiagnostics) >= 128 {
			return output, false
		}
		a.successRequestDiagnostics[key] = &successRequestDiagnostic{at: now, event: map[string]any{"method": event["method"], "route": event["route"], "path": event["path"]}}
		return append(output, event), true
	}
	bucket.count++
	bucket.bytes += diagnosticInteger(event["body_bytes"])
	duration := diagnosticInteger(event["duration_ms"])
	bucket.duration += duration
	bucket.maxDuration = max(bucket.maxDuration, duration)
	if event["retried"] == true {
		bucket.retries++
	}
	return output, true
}
func (a *app) flushSuccessfulRequestDiagnostics() {
	events, _ := a.successfulRequestDiagnostics(nil, time.Now(), true)
	for _, event := range events {
		a.appendDiagnosticEvent(event)
	}
}
