package main

import (
	"encoding/json"
	"log"
	"net/http"
	"regexp"
	"runtime"
	"strings"
	"sync/atomic"
)

// Non-app workers share the startup store; app-owned workers use their own
// reporter. Never retain the panic value in a diagnostic or format it for logs.
var panicDiagnosticStore atomic.Pointer[localStore]
var panicFrameName = regexp.MustCompile(`^main\.[A-Za-z0-9_.()*]+$`)

func backendPanicKind(value any) string {
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

func backendPanicEvent(site string, value any) map[string]any {
	frames := make([]string, 0, 8)
	pcs := make([]uintptr, 64)
	count := runtime.Callers(2, pcs)
	iterator := runtime.CallersFrames(pcs[:count])
	for {
		frame, more := iterator.Next()
		// go test compiles the main package under its import path.
		name := strings.Replace(frame.Function, "lol-loot-assistant/backend.", "main.", 1)
		if panicFrameName.MatchString(name) && len(frames) < 8 {
			frames = append(frames, name)
		}
		if !more || len(frames) == 8 {
			break
		}
	}
	return map[string]any{"event": "backend_panic", "site": site, "panic_kind": backendPanicKind(value), "frames": frames}
}

func reportBackendPanic(site string, value any) {
	event := backendPanicEvent(site, value)
	if store := panicDiagnosticStore.Load(); store != nil {
		_ = store.appendDiagnostic(event)
		return
	}
	// Startup/storage failure still leaves safe evidence on stderr.
	data, _ := json.Marshal(event)
	log.Print(string(data))
}

func recoverPanic(site string) {
	if value := recover(); value != nil {
		reportBackendPanic(site, value)
	}
}
func (a *app) recordRecoveredPanic(site string, value any) {
	if a != nil && a.storage != nil {
		a.recordDiagnostic(backendPanicEvent(site, value))
		return
	}
	reportBackendPanic(site, value)
}
func (a *app) recoverPanic(site string) {
	if value := recover(); value != nil {
		a.recordRecoveredPanic(site, value)
	}
}
func goSafe(site string, fn func()) {
	go func() { defer recoverPanic(site); fn() }()
}
func (a *app) goSafe(site string, fn func()) {
	go func() { defer a.recoverPanic(site); fn() }()
}

func authorizedPanicSite(r *http.Request) string {
	// ServeMux supplies the registered template, never URL/query/identity data.
	if fields := strings.Fields(r.Pattern); len(fields) > 0 {
		template := fields[len(fields)-1]
		if strings.HasPrefix(template, "/api/") {
			return template
		}
	}
	return "/api/{path}"
}
