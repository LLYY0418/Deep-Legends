package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestR209CardImageDiagnosticsPersistBoundedCompleteFields(t *testing.T) {
	a := r175App(t)
	cases := []struct {
		event, body string
		fields      map[string]any
	}{
		{"collection_card_image_state", `{"event":"collection_card_image_state","reason":"waiting","activeCount":8,"activeJobs":-1,"queued":4,"pendingObserved":2,"visiblePending":1,"observerRootOk":false,"oldestActiveAgeMs":99999999}`, map[string]any{"active_count": float64(8), "active_jobs": float64(0), "queued": float64(4), "pending_observed": float64(2), "visible_pending": float64(1), "observer_root_ok": false, "oldest_active_age_ms": float64(1000000)}},
		{"card_image_slot_reconciled", `{"event":"card_image_slot_reconciled","reason":"reconciled","beforeCount":8,"afterCount":0,"beforeRemoteCount":2,"afterRemoteCount":0}`, map[string]any{"before_count": float64(8), "after_count": float64(0), "before_remote_count": float64(2), "after_remote_count": float64(0)}},
		{"card_image_observer_fallback", `{"event":"card_image_observer_fallback","reason":"visible-pending","count":99999999}`, map[string]any{"count": float64(100000)}},
	}
	for _, tc := range cases {
		w := httptest.NewRecorder()
		a.handleClientDiagnostic(w, httptest.NewRequest(http.MethodPost, "/api/diagnostics/client", strings.NewReader(tc.body)))
		if w.Code != http.StatusNoContent {
			t.Fatalf("%s status=%d body=%s", tc.event, w.Code, w.Body.String())
		}
		events := r175Events(t, a, tc.event)
		if len(events) != 1 {
			t.Fatalf("%s events=%v", tc.event, events)
		}
		for field, want := range tc.fields {
			if got := events[0][field]; got != want {
				t.Fatalf("%s.%s=%v want=%v", tc.event, field, got, want)
			}
		}
	}
	for _, body := range []string{
		`{"event":"collection_card_image_state","reason":"waiting","path":"PRIVATE"}`,
		`{"event":"card_image_slot_reconciled","reason":"reconciled","puuid":"PRIVATE"}`,
		`{"event":"card_image_observer_fallback","reason":"unknown","count":1}`,
	} {
		w := httptest.NewRecorder()
		a.handleClientDiagnostic(w, httptest.NewRequest(http.MethodPost, "/api/diagnostics/client", strings.NewReader(body)))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("invalid diagnostic status=%d", w.Code)
		}
	}
}
