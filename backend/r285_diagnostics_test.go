package main

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func TestR285AutoloadGuardDiagnosticSurvivesStorage(t *testing.T) {
	a := r175App(t)
	for i, guard := range []string{"near", "requested", "frame-pending", "container", "service-down"} {
		raw, _ := json.Marshal(map[string]any{"bound": true, "near": true, "requested": true, "started": false, "blocked_reason": guard, "generation": i + 1})
		w := httptest.NewRecorder()
		a.recordR278Diagnostic(w, clientDiagnosticRequest{Event: "match_autoload_client", Reason: "guard", R278: raw})
		if w.Code != 204 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	events := readDiagnosticEvents(t, a.storage, "match_autoload_client")
	if len(events) != 5 {
		t.Fatalf("events=%v", events)
	}
	for i, e := range events {
		if e["generation"] != float64(i+1) {
			t.Fatal(e)
		}
		if e["blocked_reason"] != []string{"near", "requested", "frame-pending", "container", "service-down"}[i] {
			t.Fatal(e)
		}
	}
}
