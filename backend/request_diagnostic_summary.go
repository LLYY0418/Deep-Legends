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
	samples                                      []int64
	local                                        bool
	riot                                         bool
	upstream                                     bool
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
				if bucket.local {
					output = append(output, map[string]any{"event": "local_request_client", "reason": "complete", "endpoint": bucket.event["endpoint"], "window_start": bucket.at.UTC().Format(time.RFC3339), "count": bucket.count, "response_bytes": bucket.bytes, "duration_ms": lcuMetricSummary(bucket.samples), "http_status": 200})
				} else if bucket.upstream {
					output = append(output, map[string]any{"event": "champion_upstream_summary", "host": bucket.event["host"], "cache": bucket.event["cache"], "attempt": bucket.event["attempt"], "window_start": bucket.at.UTC().Format(time.RFC3339), "count": bucket.count, "bytes": bucket.bytes, "duration_ms": lcuMetricSummary(bucket.samples), "status": 200})
				} else if bucket.riot {
					output = append(output, map[string]any{"event": "riot_request_summary", "route": bucket.event["route"], "category": bucket.event["category"], "window_start": bucket.at.UTC().Format(time.RFC3339), "count": bucket.count, "bytes": bucket.bytes, "duration_ms": lcuMetricSummary(bucket.samples), "status": 200})
				} else {
					output = append(output, map[string]any{"event": "sgp_request_summary", "method": bucket.event["method"], "route": bucket.event["route"], "path": bucket.event["path"], "window_start": bucket.at.UTC().Format(time.RFC3339), "count": bucket.count, "body_bytes": bucket.bytes, "duration_total_ms": bucket.duration, "duration_max_ms": bucket.maxDuration, "retried_count": bucket.retries, "http_status": 200})
				}
			}
			delete(a.successRequestDiagnostics, key)
		}
	}
	if flush {
		return output, false
	}
	local := event["event"] == "local_request_client"
	riot := event["event"] == "riot_request" && diagnosticInteger(event["status"]) == 200 && (event["error_kind"] == nil || event["error_kind"] == "none") && event["cancelled"] != true
	upstream := event["event"] == "champion_upstream" && diagnosticInteger(event["status"]) == 200
	success := local && diagnosticInteger(event["http_status"]) >= 200 && diagnosticInteger(event["http_status"]) < 300 && (event["error_kind"] == nil || event["error_kind"] == "none")
	if !success && !riot && !upstream && (event["event"] != "sgp_request" || diagnosticInteger(event["http_status"]) != 200 || event["read_failed"] == true || event["parse_failed"] == true || event["error_kind"] != nil) {
		return output, false
	}
	raw, _ := json.Marshal([]any{event["event"], event["method"], event["route"], event["path"], event["category"], event["host"], event["cache"], event["attempt"]})
	key := fmt.Sprintf("%x", sha256.Sum256(raw))
	if local {
		key = diagnosticRouteKey(event)
	}
	if a.successRequestDiagnostics == nil {
		a.successRequestDiagnostics = map[string]*successRequestDiagnostic{}
	}
	bucket := a.successRequestDiagnostics[key]
	if bucket == nil {
		// A run with many distinct paths remains bounded; unknown routes are emitted.
		if len(a.successRequestDiagnostics) >= 128 {
			return output, false
		}
		a.successRequestDiagnostics[key] = &successRequestDiagnostic{at: now, local: local, riot: riot, upstream: upstream, event: map[string]any{"method": event["method"], "route": event["route"], "path": event["path"], "endpoint": event["endpoint"], "category": event["category"], "host": event["host"], "cache": event["cache"], "attempt": event["attempt"]}}
		if !local {
			return append(output, event), true
		}
		bucket = a.successRequestDiagnostics[key]
	}
	bucket.count++
	bucket.bytes += diagnosticInteger(event["body_bytes"])
	if local {
		bucket.bytes += diagnosticInteger(event["response_bytes"])
		if len(bucket.samples) < 4096 {
			bucket.samples = append(bucket.samples, int64(diagnosticInteger(event["duration_ms"])))
		}
	}
	duration := diagnosticInteger(event["duration_ms"])
	if upstream {
		bucket.bytes += diagnosticInteger(event["bytes"])
		if len(bucket.samples) < 4096 {
			bucket.samples = append(bucket.samples, int64(duration))
		}
	}
	if riot {
		bucket.bytes += diagnosticInteger(event["bytes"])
		duration = diagnosticInteger(event["total_ms"])
		if len(bucket.samples) < 4096 {
			bucket.samples = append(bucket.samples, int64(duration))
		}
	}
	bucket.duration += duration
	bucket.maxDuration = max(bucket.maxDuration, duration)
	if event["retried"] == true {
		bucket.retries++
	}
	return output, true
}
func (a *app) flushSuccessfulRequestDiagnostics() {
	a.flushSnapshotDiagnostics()
	events, _ := a.successfulRequestDiagnostics(nil, time.Now(), true)
	for _, event := range events {
		a.appendDiagnosticEvent(event)
	}
}
