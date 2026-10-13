package main

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestR277NewClientDiagnosticsAcceptedAndDeduplicated(t *testing.T) {
	for _, body := range []string{
		`{"event":"client_view_client","reason":"applied","state":"ready","generation":2,"rejectionReason":"new-generation"}`,
		`{"event":"ui_zoom_state","reason":"applied","mode":"fixed","multiplier":1.25,"windowWidth":1280,"windowHeight":900,"screenWidth":1920,"screenHeight":1080,"devicePixelRatio":2}`,
	} {
		t.Run(body, func(t *testing.T) {
			a := &app{storage: newFlowDiagnosticStore(t)}
			for i := 0; i < 2; i++ {
				w := httptest.NewRecorder()
				a.handleClientDiagnostic(w, httptest.NewRequest("POST", "/api/client-diagnostic", strings.NewReader(body)))
				if w.Code != 204 {
					t.Fatalf("diagnostic rejected status=%d body=%s", w.Code, w.Body.String())
				}
			}
			event := "client_view_client"
			if strings.Contains(body, "ui_zoom_state") {
				event = "ui_zoom_state"
			}
			if events := r175Events(t, a, event); len(events) != 1 {
				t.Fatalf("diagnostic not deduplicated: %v", events)
			}
		})
	}
}

func TestR277ClientDiagnosticActualRendererTransport(t *testing.T) {
	a := &app{storage: newFlowDiagnosticStore(t)}
	for _, body := range []string{
		`{"event":"client_view_client","action":"applied","reason":"new-generation","state":"ready","generation":3}`,
		`{"event":"client_view_client","action":"rejected","reason":"generation-rollback","state":"no-client","generation":2}`,
	} {
		w := httptest.NewRecorder()
		a.handleClientDiagnostic(w, httptest.NewRequest("POST", "/api/diagnostics/client", strings.NewReader(body)))
		if w.Code != 204 {
			t.Fatalf("actual renderer packet rejected: %d %s", w.Code, w.Body.String())
		}
	}
}
