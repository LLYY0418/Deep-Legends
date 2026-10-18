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
	case "build_spells_client", "augment_description_client", "overview_art_client", "career_cards_render", "match_virtual_client", "pool_render_client", "history_outage_retry", "client_follow_client", "history_outage_ui", "overview_reference_fallback", "match_autoload_client", "collection_new_marker", "overview_reload_reason", "announcement_state":
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
		case "mode":
			s, ok := value.(string)
			if !ok || (s != "arena" && s != "mayhem" && s != "rift" && s != "aram") {
				http.Error(w, "invalid mode", 400)
				return
			}
			event[key] = s
		case "error_kind":
			s, ok := value.(string)
			if !ok || (s != "none" && s != "timeout" && s != "backoff_skipped" && s != "no_response" && s != "not_found") {
				http.Error(w, "invalid error kind", 400)
				return
			}
			event[key] = s
		case "card":
			s, ok := value.(string)
			if !ok || (s != "recent-ranked" && s != "ability" && s != "champions") {
				http.Error(w, "invalid card", 400)
				return
			}
			event[key] = s
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
			if ok && (s == "complete" || s == "cards" || s == "progress" || s == "retained" || s == "event" || s == "status" || s == "circuit" || s == "fresh-503" || s == "bundled" || s == "remote" || s == "cache" || s == "stale_cache" || s == "lcu" || s == "cdragon" || s == "hexdata" || s == "none") {
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
			if ok && (s == "" || s == "loading" || s == "quota" || s == "paused" || s == "not-ready" || s == "no-more" || s == "service-down" || s == "near" || s == "requested" || s == "frame-pending" || s == "container") {
				event[key] = s
			}
		case "gave_up", "video_path_present", "reduced_motion", "video_rendered", "src_set", "playing", "applied", "bound", "near", "requested", "started":
			v, ok := value.(bool)
			if !ok {
				http.Error(w, "invalid flag", 400)
				return
			}
			event[key] = v
		case "error_code", "media_http_status", "rerender_count":
			v, ok := value.(float64)
			maximum := map[string]float64{"error_code": 4, "media_http_status": 599, "rerender_count": 100000}[key]
			if !ok || v < 0 || v > maximum || v != float64(int(v)) {
				http.Error(w, "invalid media metric", 400)
				return
			}
			event[key] = int(v)
		case "spell1Id_missing", "spell1Id_zero", "spell1Id_value", "spell2Id_missing", "spell2Id_zero", "spell2Id_value", "augment_id", "retry_count", "games", "generation", "collapsed", "hydrated", "item_count", "rendered_cards", "duration_ms", "count", "latency_ms", "matches_before", "matches_after", "durationMs":
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
