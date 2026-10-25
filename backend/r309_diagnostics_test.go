package main

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestR309DiagnosticSanitization(t *testing.T) {
	a := r175App(t)
	for _, event := range []string{"filter_action_client", "mayhem_augment_detail_client"} {
		if clientDiagnosticEvents[event] == nil {
			t.Fatal("event missing")
		}
	}
	raw := json.RawMessage(`{"action":"clear","rows_loaded":1000,"rows_rendered":20,"click_to_paint_ms":16.5,"longtask_ms":0,"puuid":"SECRET"}`)
	w := httptest.NewRecorder()
	a.recordR309Diagnostic(w, clientDiagnosticRequest{Event: "filter_action_client", Reason: "clear", R278: raw})
	if w.Code != 204 {
		t.Fatal(w.Code, w.Body.String())
	}
	events := readDiagnosticEvents(t, a.storage, "filter_action_client")
	if len(events) != 1 {
		t.Fatal(events)
	}
	out, _ := json.Marshal(events)
	if strings.Contains(string(out), "SECRET") || strings.Contains(string(out), "puuid") {
		t.Fatal(string(out))
	}
	for _, bad := range []string{`{"action":"oops"}`, `{"action":"clear","rows_loaded":-1}`} {
		w = httptest.NewRecorder()
		a.recordR309Diagnostic(w, clientDiagnosticRequest{Event: "filter_action_client", Reason: "clear", R278: json.RawMessage(bad)})
		if w.Code != 400 {
			t.Fatal(w.Code)
		}
	}
}
