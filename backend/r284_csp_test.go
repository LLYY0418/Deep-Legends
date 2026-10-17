package main

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestR284CSPDiagnosticOriginPrivacy(t *testing.T) {
	for _, tc := range []struct{ raw, want string }{
		{"https://user:password@game.gtimg.cn:8443/private/player.png?token=secret#player", "https://game.gtimg.cn:8443"},
		{"http://127.0.0.1:2999/private", "http://127.0.0.1:2999"},
		{"data:image/svg+xml,private", "data:"}, {"data", "data:"}, {"blob:https://fixture/private", "blob:"}, {"file:///Users/private/player.png", "file:"}, {"inline", ""},
	} {
		t.Run(tc.want+tc.raw[:min(4, len(tc.raw))], func(t *testing.T) {
			a := r175App(t)
			raw, _ := json.Marshal(map[string]any{"event": "browser_error_client", "reason": "csp", "cspDirective": "img-src", "blockedOrigin": tc.raw})
			w := httptest.NewRecorder()
			a.handleClientDiagnostic(w, httptest.NewRequest("POST", "/api/diagnostics/client", strings.NewReader(string(raw))))
			if w.Code != 204 {
				t.Fatal(w.Code, w.Body.String())
			}
			rows := r175Events(t, a, "browser_error_client")
			if len(rows) != 1 {
				t.Fatal(rows)
			}
			got, _ := rows[0]["blocked_origin"].(string)
			if got != tc.want {
				t.Fatalf("blocked_origin=%q want=%q", got, tc.want)
			}
			stored, _ := json.Marshal(rows)
			for _, private := range []string{"private", "password", "secret"} {
				if strings.Contains(string(stored), private) {
					t.Fatalf("private CSP data persisted: %s", stored)
				}
			}
		})
	}
}
