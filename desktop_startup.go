package main

import (
	"encoding/json"
	"io"
	"net/http"
)

// Startup timing lives entirely in the desktop shell: by the time this Go
// process can log anything, Electron has already booted, the splash window has
// already been created, and most of the user-visible wait is over. The
// diagnostics stream therefore has no way to explain "I clicked Finish and
// stared at nothing for five seconds" -- app_start is emitted near the *end* of
// that window, not the start.
//
// This endpoint closes that blind spot. The shell measures each hop with its
// own clock and posts one summary after the main window is actually on screen.
// Fixed numeric fields (not a free-form map) keep the payload bounded and make
// the accepted shape reviewable, matching how client diagnostics are handled.

// R82 cold-install sample 7: comment-only identity change; runtime stays identical.
const desktopStartupPhaseLimitMs = 600000

type desktopStartupRequest struct {
	// Windows created the process -> main.cjs executed its first statement.
	// This is Electron/Chromium cold boot plus any antivirus scan of the
	// freshly written executable; none of our code runs during it.
	ProcessToJS int `json:"processToJs"`
	// First statement -> app ready (Electron finishes internal init).
	JSToReady int `json:"jsToReady"`
	// App ready -> splash BrowserWindow constructed (still hidden).
	ReadyToSplash int `json:"readyToSplash"`
	// Splash constructed -> did-finish-load (the historical splash_paint metric).
	SplashPaint int `json:"splashPaint"`
	// Process creation -> ready-to-show actually revealed the splash window.
	SplashWindowShown int `json:"splashWindowShown"`
	// Backend spawn -> LOOT_READY line received on stdout.
	SpawnToReady int `json:"spawnToReady"`
	// LOOT_READY -> main window reports ready-to-show.
	ReadyToWindow int `json:"readyToWindow"`
	// Process creation -> main window shown. The number the user perceives.
	Total int `json:"total"`
}

func clampDesktopStartupPhase(value int) int {
	if value < 0 {
		return 0
	}
	if value > desktopStartupPhaseLimitMs {
		return desktopStartupPhaseLimitMs
	}
	return value
}

func (a *app) handleDesktopStartup(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 2<<10)
	defer r.Body.Close()
	var request desktopStartupRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		http.Error(w, "invalid startup diagnostic", http.StatusBadRequest)
		return
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		http.Error(w, "invalid startup diagnostic", http.StatusBadRequest)
		return
	}
	a.recordDiagnostic(map[string]any{
		"event": "desktop_startup_phases_ms",
		"phases_ms": map[string]any{
			"process_to_js":       clampDesktopStartupPhase(request.ProcessToJS),
			"js_to_ready":         clampDesktopStartupPhase(request.JSToReady),
			"ready_to_splash":     clampDesktopStartupPhase(request.ReadyToSplash),
			"splash_paint":        clampDesktopStartupPhase(request.SplashPaint),
			"splash_window_shown": clampDesktopStartupPhase(request.SplashWindowShown),
			"spawn_to_ready":      clampDesktopStartupPhase(request.SpawnToReady),
			"ready_to_window":     clampDesktopStartupPhase(request.ReadyToWindow),
			"total":               clampDesktopStartupPhase(request.Total),
		},
	})
	w.WriteHeader(http.StatusNoContent)
}
