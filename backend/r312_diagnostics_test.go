package main

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestR312ResizeDiagnosticKeepsOnlyColorThemeAndClassFields(t *testing.T) {
	a := &app{storage: newFlowDiagnosticStore(t)}
	body := `{"event":"window_resize_trace","reason":"summary","resizeTrace":{"drag":123,"sample":60,"zoom":1,"innerWidth":1440,"nativeBackground":"#0E0C0D","theme":"crimson","resizingClass":true,"private":"unwanted-player"}}`
	call := func(value string) int {
		t.Helper()
		w := httptest.NewRecorder()
		a.handleClientDiagnostic(w, httptest.NewRequest("POST", "/api/diagnostics/client", strings.NewReader(value)))
		return w.Code
	}
	if got := call(body); got != 204 {
		t.Fatal(got)
	}
	events := r175Events(t, a, "window_resize_trace")
	if len(events) != 1 {
		t.Fatal(events)
	}
	text := stringMustR307(events)
	for _, field := range []string{`"nativeBackground":"#0E0C0D"`, `"theme":"crimson"`, `"resizingClass":true`} {
		if !strings.Contains(text, field) {
			t.Fatalf("missing %s: %s", field, text)
		}
	}
	if strings.Contains(text, "unwanted-player") {
		t.Fatal(text)
	}
	for _, theme := range []string{"auto", "light", "dark", "azure", "emerald", "violet", "crimson", "aurora", "oled"} {
		if got := call(strings.Replace(body, `"crimson"`, `"`+theme+`"`, 1)); got != 204 {
			t.Fatalf("supported theme %s rejected: %d", theme, got)
		}
	}
	for _, bad := range []string{strings.Replace(body, "#0E0C0D", "private-player", 1), strings.Replace(body, `"crimson"`, `"unknown"`, 1), strings.Replace(body, `"resizingClass":true`, `"resizingClass":"bad"`, 1)} {
		if got := call(bad); got != 400 {
			t.Fatalf("bad diagnostic accepted %d", got)
		}
	}
}
