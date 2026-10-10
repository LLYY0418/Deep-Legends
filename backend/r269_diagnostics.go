package main

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"time"
)

type compactDiagnosticWindow struct {
	at        time.Time
	signature string
	count     int
	event     map[string]any
}

// Repeated status checks retain counts without filling the rolling export.
func (a *app) compactDiagnostic(event map[string]any, now time.Time) (map[string]any, bool) {
	name, _ := event["event"].(string)
	switch name {
	case "overview_art_source", "sgp_history_circuit", "gameflow_phase_client", "browser_error_client":
	default:
		return event, true
	}
	copy := make(map[string]any, len(event))
	for k, v := range event {
		copy[k] = v
	}
	key := name
	if name == "sgp_history_circuit" {
		key += "|" + fmt.Sprint(event["server_id"]) + "|" + fmt.Sprint(event["route"])
		copy = map[string]any{"event": name, "state": event["state"], "route": event["route"], "server_id": event["server_id"]}
	}

	signatureFields := make(map[string]any, len(copy))
	for k, v := range copy {
		signatureFields[k] = v
	}
	if name == "browser_error_client" {
		for _, k := range []string{"counts", "total", "deferred", "error_deferred", "repeat_count"} {
			delete(signatureFields, k)
		}
	}
	if name == "gameflow_phase_client" {
		signatureFields = map[string]any{"phase": event["phase"]}
	}
	raw, _ := json.Marshal(signatureFields)
	signature := string(raw)
	if name == "overview_art_source" || name == "browser_error_client" {
		key += "|" + riotIdentityKey(signature)
	}
	a.compactDiagnosticMu.Lock()
	defer a.compactDiagnosticMu.Unlock()
	if a.compactDiagnosticWindows == nil {
		a.compactDiagnosticWindows = map[string]compactDiagnosticWindow{}
	}
	if len(a.compactDiagnosticWindows) >= 1024 {
		var oldestKey string
		var oldest time.Time
		for k, w := range a.compactDiagnosticWindows {
			if oldest.IsZero() || w.at.Before(oldest) {
				oldestKey, oldest = k, w.at
			}
		}
		delete(a.compactDiagnosticWindows, oldestKey)
	}
	prior := a.compactDiagnosticWindows[key]
	same := prior.signature == signature
	// Circuit/gameflow snapshots are written when state changes, not when polled.
	stateOnly := name == "sgp_history_circuit" || name == "gameflow_phase_client"
	if same && (stateOnly || now.Sub(prior.at) < time.Minute) {
		prior.count++
		a.compactDiagnosticWindows[key] = prior
		return nil, false
	}
	if _, exists := copy["repeat_count"]; !exists {
		copy["repeat_count"] = 1
	}
	if prior.count > 0 {
		copy["previous_repeat_count"] = prior.count
	}
	a.compactDiagnosticWindows[key] = compactDiagnosticWindow{now, signature, 0, copy}
	return copy, true
}

var errorURLQuery = regexp.MustCompile(`(https?://[^\s?"']+)\?[^\s"']*`)
var errorUserPath = regexp.MustCompile(`(?i)(/Users/|/home/|[a-z]:\\Users\\)[^/\\\s]+`)
var errorRiotID = regexp.MustCompile(`[^\s"'<>]+#[^\s"'<>]+`)
var errorFrame = regexp.MustCompile(`([a-zA-Z0-9_-]+\.(?:js|cjs))(?:\?[^)\s]*?)?:(\d+):(\d+)`)

func safeBrowserMessage(message string) string {
	message = errorURLQuery.ReplaceAllString(message, "$1?[redacted]")
	message = errorUserPath.ReplaceAllString(message, "${1}[redacted]")
	message = errorRiotID.ReplaceAllString(message, "[player]")
	runes := []rune(message)
	if len(runes) > 160 {
		runes = runes[:160]
	}
	return string(runes)
}
func safeBrowserFrames(stack []string) []string {
	frames := []string{}
	for _, row := range stack {
		if hit := errorFrame.FindStringSubmatch(row); len(hit) > 0 {
			frames = append(frames, hit[1]+":"+hit[2]+":"+hit[3])
			if len(frames) == 3 {
				break
			}
		}
	}
	return frames
}

type timelineDiagnosticBatchKey struct{}
type timelineDiagnosticBatch struct {
	Counts               map[string]int
	MaxFrames, MaxEvents int
	Samples              []map[string]any
}

func (a *app) recordTimelineDiagnostic(ctx context.Context, event map[string]any) {
	batch, _ := ctx.Value(timelineDiagnosticBatchKey{}).(string)
	if batch == "" {
		a.recordDiagnostic(event)
		return
	}
	a.compactDiagnosticMu.Lock()
	defer a.compactDiagnosticMu.Unlock()
	if a.timelineDiagnosticBatches == nil {
		a.timelineDiagnosticBatches = map[string]*timelineDiagnosticBatch{}
	}
	b := a.timelineDiagnosticBatches[batch]
	if b == nil {
		if len(a.timelineDiagnosticBatches) >= 512 {
			return
		}
		b = &timelineDiagnosticBatch{Counts: map[string]int{}}
		a.timelineDiagnosticBatches[batch] = b
	}
	name, _ := event["event"].(string)
	b.Counts[name]++
	if n, ok := event["frames"].(int); ok {
		b.MaxFrames = max(b.MaxFrames, n)
	}
	if n, ok := event["events"].(int); ok {
		b.MaxEvents = max(b.MaxEvents, n)
	}
	if len(b.Samples) < 3 {
		b.Samples = append(b.Samples, map[string]any{"event": name, "source": event["source"], "reason": event["reason"]})
	}
}
func (a *app) flushTimelineDiagnosticBatch(batch string) {
	a.compactDiagnosticMu.Lock()
	b := a.timelineDiagnosticBatches[batch]
	delete(a.timelineDiagnosticBatches, batch)
	a.compactDiagnosticMu.Unlock()
	if b != nil {
		a.recordDiagnostic(map[string]any{"event": "match_timeline_summary", "counts": b.Counts, "max_frames": b.MaxFrames, "max_events": b.MaxEvents, "samples": b.Samples})
	}
}
