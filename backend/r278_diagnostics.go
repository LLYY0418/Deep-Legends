package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

func isR278Diagnostic(event string) bool {
	switch event {
	case "match_virtual_client", "pool_render_client", "history_outage_retry", "client_follow_client", "history_outage_ui", "overview_reference_fallback", "match_autoload_client", "collection_new_marker", "overview_reload_reason", "announcement_state":
		return true
	}
	return false
}
func (a *app) recordR278Diagnostic(w http.ResponseWriter, r clientDiagnosticRequest) {
	if len(r.R278) == 0 {
		r.R278 = json.RawMessage(`{}`)
	}
	var fields map[string]any
	if len(r.R278) > 2048 || json.Unmarshal(r.R278, &fields) != nil {
		http.Error(w, "invalid diagnostic fields", 400)
		return
	}
	event := map[string]any{"event": r.Event, "reason": r.Reason}
	for key, value := range fields {
		switch key {
		case "view", "result":
			v, ok := value.(string)
			if !ok || !(v == "owned" || v == "remaining" || v == "unowned" || v == "recovered" || v == "still_down") {
				http.Error(w, "invalid state", 400)
				return
			}
			event[key] = v
		case "zoom", "height_mismatch_max_px":
			v, ok := value.(float64)
			if !ok || v < 0 || v > 100000 {
				http.Error(w, "invalid geometry", 400)
				return
			}
			event[key] = v
		case "from_group", "to_group":
			if s, ok := value.(string); ok && (s == "players" || s == "kr" || s == "pro") {
				event[key] = s
			} else {
				http.Error(w, "invalid group", 400)
				return
			}
		case "search_from", "search_to":
			s, ok := value.(string)
			if !ok {
				http.Error(w, "invalid region", 400)
				return
			}
			if s == "" || isRiotRegion(s) || tencentSGPServers[strings.ToUpper(s)] != "" {
				event[key] = s
			}
		case "source":
			s, ok := value.(string)
			if ok && (s == "event" || s == "status" || s == "circuit" || s == "fresh-503" || s == "bundled" || s == "remote" || s == "cache") {
				event[key] = s
			}
		case "id", "notice_revision":
			s, ok := value.(string)
			if !ok || s != "" && !noticeIdentifier.MatchString(s) {
				http.Error(w, "invalid notice identity", 400)
				return
			}
			if key == "notice_revision" {
				key = "revision"
			}
			event[key] = s
		case "blocked_reason":
			s, ok := value.(string)
			if ok && (s == "" || s == "loading" || s == "quota" || s == "paused" || s == "not-ready" || s == "no-more") {
				event[key] = s
			}
		case "applied", "bound", "near", "requested", "started":
			v, ok := value.(bool)
			if !ok {
				http.Error(w, "invalid flag", 400)
				return
			}
			event[key] = v
		case "collapsed", "hydrated", "item_count", "rendered_cards", "duration_ms", "count", "latency_ms", "matches_before", "matches_after", "durationMs":
			v, ok := value.(float64)
			if !ok || v < 0 || v > 3600000 {
				http.Error(w, "invalid metric", 400)
				return
			}
			event[key] = int(v)
		default:
			http.Error(w, "unknown diagnostic field", 400)
			return
		}
	}
	if r.Event == "client_follow_client" {
		event["trigger"] = r.Reason
	}
	if r.Event == "history_outage_ui" {
		event["state"] = r.Reason
	}
	if r.Event == "collection_new_marker" {
		event["kind"] = "skin"
	}
	if r.Event == "announcement_state" {
		event["action"] = r.Reason
	}
	a.recordDiagnostic(event)
	w.WriteHeader(204)
}
func (a *app) recordOverviewShape(s *overviewResponseShape, now time.Time) {
	if s.EmptyRevision {
		a.recordDiagnostic(map[string]any{"event": "overview_reference_fallback", "reason": "empty-revision", "count": 1})
	}
	event := s.event()
	if s.account != "" {
		a.shapeDiagnosticMu.Lock()
		if a.shapeDiagnosticWindows == nil {
			a.shapeDiagnosticWindows = map[string]time.Time{}
		}
		key := riotIdentityKey(s.account)
		last := a.shapeDiagnosticWindows[key]
		if !last.IsZero() && now.Sub(last) < 10*time.Minute {
			event = map[string]any{"event": "overview_response_shape", "response_bytes": s.Bytes}
		} else {
			if len(a.shapeDiagnosticWindows) >= 256 {
				for k, at := range a.shapeDiagnosticWindows {
					if now.Sub(at) >= 10*time.Minute {
						delete(a.shapeDiagnosticWindows, k)
					}
				}
				if len(a.shapeDiagnosticWindows) >= 256 {
					for k := range a.shapeDiagnosticWindows {
						delete(a.shapeDiagnosticWindows, k)
						break
					}
				}
			}
			a.shapeDiagnosticWindows[key] = now
		}
		a.shapeDiagnosticMu.Unlock()
	}
	a.recordDiagnostic(event)
}
func diagnosticRouteKey(event map[string]any) string {
	return fmt.Sprint(event["event"], "|", event["endpoint"], "|", event["method"], "|", event["route"], "|", event["path"])
}
