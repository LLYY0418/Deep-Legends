//go:build !windows

package main

func nativeAcceptWindowState() acceptWindowSnapshot {
	return acceptWindowSnapshot{category: "unsupported-platform"}
}

func nativeAcceptWindowDetails() map[string]any {
	return map[string]any{"platform": "unsupported", "observation_available": false, "window_action": "none"}
}
