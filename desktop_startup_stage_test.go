package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const startupStageTestToken = "r84-synthetic-local-token-0123456789"

func startupStageServer(t *testing.T, store *localStore) *httptest.Server {
	t.Helper()
	a := &app{storage: store, token: startupStageTestToken}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/diagnostics/startup-stage", a.authorized(a.handleDesktopStartupStage))
	mux.HandleFunc("POST /api/diagnostics/startup", a.authorized(a.handleDesktopStartup))
	mux.HandleFunc("GET /api/diagnostics/log", a.authorized(a.handleDiagnosticLog))
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

func TestDesktopStartupStageHTTPValidationAndDiskWrite(t *testing.T) {
	for _, tc := range []struct {
		name, body, token string
		status            int
		elapsed           float64
	}{
		{"backend", `{"stage":"backend_ready","elapsedMs":10}`, startupStageTestToken, 204, 10},
		{"created", `{"stage":"main_window_created","elapsedMs":20}`, startupStageTestToken, 204, 20},
		{"loaded", `{"stage":"main_window_did_finish_load","elapsedMs":30}`, startupStageTestToken, 204, 30},
		{"shown", `{"stage":"main_window_ready_to_show","elapsedMs":40}`, startupStageTestToken, 204, 40},
		{"negative", `{"stage":"backend_ready","elapsedMs":-1}`, startupStageTestToken, 204, 0},
		{"oversized time", `{"stage":"backend_ready","elapsedMs":9999999}`, startupStageTestToken, 204, 600000},
		{"unknown field", `{"stage":"backend_ready","token":"must-not-leak"}`, startupStageTestToken, 400, 0},
		{"unknown stage", `{"stage":"arbitrary text"}`, startupStageTestToken, 400, 0},
		{"wrong type", `{"stage":"backend_ready","elapsedMs":"slow"}`, startupStageTestToken, 400, 0},
		{"trailing", `{"stage":"backend_ready"}{}`, startupStageTestToken, 400, 0},
		{"empty", ``, startupStageTestToken, 400, 0},
		{"oversized body", `{"stage":"` + strings.Repeat("x", 2048) + `"}`, startupStageTestToken, 400, 0},
		{"unauthorized", `{"stage":"backend_ready"}`, "wrong-token", 401, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := newStartupTestStore(t)
			server := startupStageServer(t, store)
			request, err := http.NewRequest(http.MethodPost, server.URL+"/api/diagnostics/startup-stage", strings.NewReader(tc.body))
			if err != nil {
				t.Fatal(err)
			}
			request.Header.Set("X-Local-Token", tc.token)
			response, err := server.Client().Do(request)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			if response.StatusCode != tc.status {
				t.Fatalf("status %d, want %d", response.StatusCode, tc.status)
			}
			events := readDiagnosticEvents(t, store, "desktop_startup_stage")
			if tc.status == 204 {
				if len(events) != 1 || events[0]["elapsed_ms"] != tc.elapsed || events[0]["run_id"] == "" || events[0]["log_seq"] != float64(1) {
					t.Fatalf("successful HTTP did not persist the bounded event: %#v", events)
				}
			} else if len(events) != 0 {
				t.Fatal("rejected input reached disk", events)
			}
		})
	}
}

func TestDesktopStartupStageMissingLogDirectoryCannotAcknowledgeSuccess(t *testing.T) {
	server := startupStageServer(t, trackTestStore(t, &localStore{root: t.TempDir()})) // Intentionally no logs directory.
	request, _ := http.NewRequest(http.MethodPost, server.URL+"/api/diagnostics/startup-stage", strings.NewReader(`{"stage":"backend_ready"}`))
	request.Header.Set("X-Local-Token", startupStageTestToken)
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusServiceUnavailable {
		t.Fatal("missing disk write looked successful", response.StatusCode)
	}
}

func runDesktopStageShell(t *testing.T, mode, source string) (string, error, *localStore) {
	t.Helper()
	store := newStartupTestStore(t)
	if err := store.appendDiagnostic(map[string]any{"event": "app_start", "build_fingerprint": "a84b00000001"}); err != nil {
		t.Fatal(err)
	}
	server := startupStageServer(t, store)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "node", "desktop/test-fixtures/startup-stages.cjs", server.URL, startupStageTestToken, t.TempDir(), mode, source)
	output, err := command.CombinedOutput()
	return string(output), err, store
}

func TestDesktopStartupStageShellAndExportEndToEnd(t *testing.T) {
	for _, mode := range []string{"normal", "stuck-created", "stuck-loaded"} {
		t.Run(mode, func(t *testing.T) {
			output, err, store := runDesktopStageShell(t, mode, os.Getenv("R84_STAGE_SOURCE"))
			if err != nil {
				t.Fatalf("real shell/HTTP/disk/export failed: %v\n%s", err, output)
			}
			// Reopening the app uses the same directory and a new run_id. Export
			// must still include the previous incomplete startup after a restart.
			reopened := trackTestStore(t, &localStore{root: store.root})
			if err := reopened.appendDiagnostic(map[string]any{"event": "app_start", "build_fingerprint": "a84b00000001"}); err != nil {
				t.Fatal(err)
			}
			server := startupStageServer(t, reopened)
			request, _ := http.NewRequest(http.MethodGet, server.URL+"/api/diagnostics/log", nil)
			request.Header.Set("X-Local-Token", startupStageTestToken)
			response, err := server.Client().Do(request)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			data, err := io.ReadAll(response.Body)
			if err != nil || response.StatusCode != 200 || !strings.Contains(string(data), "main_window_created") || !strings.Contains(string(data), store.diagnosticRunID) {
				t.Fatalf("restart export lost prior startup: %v, %s", err, data)
			}
		})
	}
}

func TestDesktopStartupStageShellMutationsFailRealExport(t *testing.T) {
	data, err := os.ReadFile("desktop/main.cjs")
	if err != nil {
		t.Fatal(err)
	}
	original := string(data)
	for _, tc := range []struct{ name, before, after, failure string }{
		{"delay until window shows", "if (!backendReady || reportedStartupStages.has(stage)) return;", "if (!startupMarks.mainShown || !backendReady || reportedStartupStages.has(stage)) return;", "export must contain stages before main window visibility"},
		{"unknown payload field", "JSON.stringify({ stage, elapsedMs:", "JSON.stringify({ unknownField: true, stage, elapsedMs:", "diagnostic POST rejected"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if strings.Count(original, tc.before) != 1 {
				t.Fatal("mutation target changed")
			}
			file := filepath.Join(t.TempDir(), "main.cjs")
			if err := os.WriteFile(file, []byte(strings.Replace(original, tc.before, tc.after, 1)), 0600); err != nil {
				t.Fatal(err)
			}
			output, err, _ := runDesktopStageShell(t, "stuck-loaded", file)
			if err == nil || !strings.Contains(output, tc.failure) {
				t.Fatalf("mutation survived or failed for wrong reason: %v\n%s", err, output)
			}
		})
	}
}

func TestDesktopStartupStageProductionRouteRemainsAuthenticated(t *testing.T) {
	data, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `mux.HandleFunc("POST /api/diagnostics/startup-stage", a.authorized(a.handleDesktopStartupStage))`) {
		t.Fatal("production startup stage route missing or unauthenticated")
	}
}
