package main

// Observe only. Never use foreground locks, window minimization, process hooks,
// or synthetic input to disguise a client-initiated focus change.
type acceptWindowSnapshot struct {
	window   uintptr // memory-only comparison; never serialized
	category string
}

func acceptWindowDiagnostic(stage string, before, current acceptWindowSnapshot) map[string]any {
	return map[string]any{"event": "watch_accept_window", "stage": stage, "foreground": current.category, "foreground_changed": before.window != 0 && current.window != 0 && before.window != current.window, "observation_available": current.window != 0, "window_action": "none", "scope": "foreground-only"}
}

func observeAcceptRequest(record func(map[string]any), read func() acceptWindowSnapshot) func() {
	before := read()
	record(acceptWindowDiagnostic("before-http", before, before))
	return func() { record(acceptWindowDiagnostic("after-http", before, read())) }
}
