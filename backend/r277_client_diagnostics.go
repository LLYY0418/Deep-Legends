package main

import (
	"encoding/json"
	"net/http"
)

func (a *app) recordR277ClientDiagnostic(w http.ResponseWriter, r clientDiagnosticRequest) {
	event := map[string]any{"event": r.Event, "reason": r.Reason}
	if r.Event == "client_view_client" {
		switch r.ClientViewState {
		case "ready", "no-client", "exiting", "invalid":
		default:
			http.Error(w, "invalid client view state", 400)
			return
		}

		action := r.ClientViewAction
		if action == "" {
			action = r.Reason
		}
		switch action {
		case "received", "applied", "rejected":
		default:
			http.Error(w, "invalid client view action", 400)
			return
		}
		if r.RejectionReason == "" {
			r.RejectionReason = r.Reason
		}
		event["action"] = action
		switch r.RejectionReason {
		case "frame", "new-generation", "generation-rollback", "duplicate-generation", "invalid-frame":
		default:
			http.Error(w, "invalid client view reason", 400)
			return
		}
		event["state"], event["generation"], event["reason"] = r.ClientViewState, r.ClientViewGeneration, r.RejectionReason
	} else {
		delete(event, "reason") // Zoom configuration itself is the deduplication key.
		if r.LaneMatchupMode != "auto" && r.LaneMatchupMode != "fixed" || r.ZoomMultiplier < 0.25 || r.ZoomMultiplier > 4 || r.DevicePixelRatio <= 0 || r.DevicePixelRatio > 8 {
			http.Error(w, "invalid zoom state", 400)
			return
		}
		event["mode"], event["multiplier"], event["devicePixelRatio"] = r.LaneMatchupMode, r.ZoomMultiplier, r.DevicePixelRatio
		for key, value := range map[string]int{"windowWidth": r.WindowWidth, "windowHeight": r.WindowHeight, "screenWidth": r.ScreenWidth, "screenHeight": r.ScreenHeight} {
			if value <= 0 || value > 32768 {
				http.Error(w, "invalid zoom dimensions", 400)
				return
			}
			event[key] = value
		}
	}
	signature, _ := json.Marshal(event)
	a.mu.Lock()
	if a.r277ClientDiagnosticSeen == nil {
		a.r277ClientDiagnosticSeen = map[string]bool{}
	}
	duplicate := a.r277ClientDiagnosticSeen[string(signature)]
	if !duplicate {
		if len(a.r277ClientDiagnosticSeen) >= 128 {
			a.r277ClientDiagnosticSeen = map[string]bool{}
		}
		a.r277ClientDiagnosticSeen[string(signature)] = true
	}
	a.mu.Unlock()
	if !duplicate {
		a.recordDiagnostic(event)
	}
	w.WriteHeader(http.StatusNoContent)
}
