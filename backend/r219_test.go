package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestR219StatusClientDiscovery(t *testing.T) {
	for _, tc := range []struct {
		result, want string
		connected    bool
	}{
		{"process-not-found", "process-not-found", false},
		{"process-query-failed", "process-query-failed", false},
		{"credentials-unreadable", "credentials-unreadable", false},
		{"probe-failed", "probe-failed", false},
		{"searching", "", false},
		{"", "", false},
		{"PRIVATE-PATH", "", false},
		{"process-not-found", "connected", true},
	} {
		t.Run(tc.result+tc.want, func(t *testing.T) {
			a := &app{connected: tc.connected, connectionState: "connecting", discovery: LCUDiscoveryStatus{Result: tc.result, Detail: "PRIVATE-PATH"}}
			response := httptest.NewRecorder()
			a.handleStatus(response, httptest.NewRequest(http.MethodGet, "/api/status", nil))
			var status statusResponse
			if err := json.Unmarshal(response.Body.Bytes(), &status); err != nil {
				t.Fatal(err)
			}
			if status.ClientDiscovery != tc.want || strings.Contains(response.Body.String(), "PRIVATE-PATH") {
				t.Fatal(status.ClientDiscovery, response.Body.String())
			}
		})
	}
}

func TestR219RenderFailureAndHideDiagnosticFields(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		check      func(*testing.T, map[string]any)
	}{
		{"known", `{"event":"status_render_failed","reason":"failed","errorType":"TypeError","functionName":"renderLaunchpad","requestId":123}`, func(t *testing.T, e map[string]any) {
			if e["error_type"] != "TypeError" || e["function_name"] != "renderLaunchpad" {
				t.Fatal(e)
			}
			if _, ok := e["request_id"]; ok {
				t.Fatal("extra render failure metadata", e)
			}
		}},
		{"unapproved", `{"event":"status_render_failed","reason":"failed","errorType":"PRIVATE-PATH","functionName":"/PRIVATE-PATH"}`, func(t *testing.T, e map[string]any) {
			if e["error_type"] != "Error" || e["function_name"] != "other" {
				t.Fatal(e)
			}
		}},
		{"hide", `{"event":"blocking_state_client","reason":"hide","source":"startup","hide_reason":"no-client-process","durationMs":99999999}`, func(t *testing.T, e map[string]any) {
			if e["hide_reason"] != "no-client-process" || e["duration_ms"] != float64(3600000) {
				t.Fatal(e)
			}
		}},
		{"hide-private", `{"event":"blocking_state_client","reason":"hide","source":"startup","hide_reason":"PRIVATE-PATH"}`, func(t *testing.T, e map[string]any) {
			if _, ok := e["hide_reason"]; ok {
				t.Fatal(e)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := r175App(t)
			response := httptest.NewRecorder()
			a.handleClientDiagnostic(response, httptest.NewRequest(http.MethodPost, "/api/diagnostics/client", strings.NewReader(tc.body)))
			if response.Code != http.StatusNoContent {
				t.Fatal(response.Code, response.Body.String())
			}
			raw, err := a.storage.readDiagnosticLog()
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(raw), "PRIVATE-PATH") {
				t.Fatal("private diagnostic metadata retained")
			}
			var event map[string]any
			if err := json.Unmarshal(raw, &event); err != nil {
				t.Fatal(err)
			}
			tc.check(t, event)
		})
	}
}
