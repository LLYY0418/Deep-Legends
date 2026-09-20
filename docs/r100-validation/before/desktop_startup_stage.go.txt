package main

import (
	"encoding/json"
	"io"
	"net/http"
)

// Success-only startup timings cannot explain a window that never appears.
// Accept only fixed milestones; no paths, tokens or arbitrary renderer text.
type desktopStartupStageRequest struct {
	Stage     string `json:"stage"`
	ElapsedMS int    `json:"elapsedMs"`
}

func (a *app) handleDesktopStartupStage(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<10)
	defer r.Body.Close()
	var request desktopStartupStageRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		http.Error(w, "invalid startup stage", http.StatusBadRequest)
		return
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		http.Error(w, "invalid startup stage", http.StatusBadRequest)
		return
	}
	switch request.Stage {
	case "backend_ready", "main_window_created", "main_window_did_finish_load", "main_window_ready_to_show":
	default:
		http.Error(w, "invalid startup stage", http.StatusBadRequest)
		return
	}
	// Acknowledge only an actual disk write, including run_id and log_seq. A
	// missing/unwritable logs directory must not look like successful telemetry.
	if err := a.storage.appendDiagnostic(map[string]any{
		"event": "desktop_startup_stage", "stage": request.Stage,
		"elapsed_ms": clampDesktopStartupPhase(request.ElapsedMS),
	}); err != nil {
		http.Error(w, "startup stage log unavailable", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
