package main

import (
	"encoding/json"
	"os"
	"regexp"
	"runtime"
	"strings"
)

// This diagnostic tool is a separate process; safe panic evidence stays on its
// stderr pipe. No panic value, identity, arguments, or path is serialized.
var panicFrameName = regexp.MustCompile(`^main\.[A-Za-z0-9_.()*]+$`)

func probePanicKind(value any) string {
	if err, ok := value.(runtime.Error); ok {
		text := err.Error()
		for _, kind := range []struct{ prefix, name string }{
			{"runtime error: index out of range", "index-out-of-range"},
			{"runtime error: invalid memory address or nil pointer dereference", "nil-pointer"},
			{"runtime error: slice bounds out of range", "slice-bounds"},
			{"interface conversion:", "type-assertion"},
			{"send on closed channel", "closed-channel"},
			{"close of closed channel", "closed-channel"},
		} {
			if strings.HasPrefix(text, kind.prefix) {
				return kind.name
			}
		}
	}
	return "other"
}

func probePanicEvent(site string, value any) map[string]any {
	frames := make([]string, 0, 8)
	pcs := make([]uintptr, 64)
	count := runtime.Callers(2, pcs)
	iterator := runtime.CallersFrames(pcs[:count])
	for {
		frame, more := iterator.Next()
		// go test compiles the main package under its import path.
		name := strings.Replace(frame.Function, "deep-legends-window-focus-probe.", "main.", 1)
		if panicFrameName.MatchString(name) && len(frames) < 8 {
			frames = append(frames, name)
		}
		if !more || len(frames) == 8 {
			break
		}
	}
	return map[string]any{"event": "backend_panic", "site": site, "panic_kind": probePanicKind(value), "frames": frames}
}

func goSafe(site string, fn func()) {
	go func() {
		defer func() {
			if value := recover(); value != nil {
				_ = json.NewEncoder(os.Stderr).Encode(probePanicEvent(site, value))
			}
		}()
		fn()
	}()
}
