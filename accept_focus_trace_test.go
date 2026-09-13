package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestAcceptFocusPreferencesNeverLogTokensOrStrings(t *testing.T) {
	raw := []byte("lol-user-experience:\n    data:\n        focusDisabled: true\n        motionEffectsDisabled: true\n        description: PrivateText\nlol-skins-viewer:\n    data:\n        lastSessionId: SECRETJWT\ninstall:\n    rso-auth:\n        password: SECRET\n    locale: zh_CN\n")
	encoded, _ := json.Marshal(focusPreferenceShape(raw))
	for _, secret := range []string{"PrivateText", "SECRET", "zh_CN", "lastSessionId", "password"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("leaked %s", secret)
		}
	}
	if !strings.Contains(string(encoded), `"value":true`) || !strings.Contains(string(encoded), "focusDisabled") {
		t.Fatal(string(encoded))
	}
	js, _ := json.Marshal(focusJSONSettingsShape([]byte(`{"data":{"focusDisabled":true,"lastSessionId":"SECRET","label":"PrivateText"}}`)))
	if strings.Contains(string(js), "SECRET") || strings.Contains(string(js), "PrivateText") || !strings.Contains(string(js), "focusDisabled") {
		t.Fatal(string(js))
	}
}

func TestAcceptFocusHelpOnlyCandidateSymbols(t *testing.T) {
	raw := []byte(`{"functions":{"GetWindowFocus":{"name":"GetWindowFocus","description":"secret prose","url":"/riotclient/focus"},"Auth":{"url":"/riotclient/auth-token"}},"title":"private"}`)
	encoded, _ := json.Marshal(focusHelpShape(raw))
	if !strings.Contains(string(encoded), "/riotclient/focus") || strings.Contains(string(encoded), "secret") || strings.Contains(string(encoded), "auth-token") || strings.Contains(string(encoded), "private") {
		t.Fatal(string(encoded))
	}
}

func TestAcceptFocusHistoryBoundedAndRedacted(t *testing.T) {
	client := &LCUClient{}
	for i := 0; i < 140; i++ {
		client.rememberAcceptFocusEvent(LCUEvent{URI: "/lol-matchmaking/v1/ready-check", Data: json.RawMessage(`{"state":"InProgress","playerResponse":"Accepted","token":"PRIVATEJWT","summonerId":123}`)})
	}
	client.rememberAcceptFocusEvent(LCUEvent{URI: "/private/SECRET", Data: json.RawMessage(`"SECRET"`)})
	snapshot := client.acceptFocusHistorySnapshot()
	if snapshot["dropped"] != 12 || len(snapshot["entries"].([]map[string]any)) != 128 {
		t.Fatal(snapshot)
	}
	raw, _ := json.Marshal(snapshot)
	for _, secret := range []string{"PRIVATEJWT", "summonerId", "SECRET"} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("private event leaked")
		}
	}
	if !strings.Contains(string(raw), "Accepted") {
		t.Fatal("missing response")
	}
}

func TestAcceptFocusExportRetainsRotationAndRejectsSymlink(t *testing.T) {
	store := &localStore{root: t.TempDir()}
	dir := filepath.Join(store.root, "logs")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	for name, text := range map[string]string{"diagnostics.1.jsonl": "{\"before\":true}\n", "diagnostics.jsonl": "{\"after\":true}\n"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	raw, err := store.readDiagnosticLogForExport()
	if err != nil || string(raw) != "{\"before\":true}\n{\"after\":true}\n" {
		t.Fatalf("%s %v", raw, err)
	}
	path := filepath.Join(dir, "diagnostics.1.jsonl")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "diagnostics.jsonl"), path); err != nil {
		t.Skip(err)
	}
	if _, err := store.readDiagnosticLogForExport(); err == nil {
		t.Fatal("accepted symlink")
	}
}

func TestAcceptFocusHelpContractHasOnlySafeMetadata(t *testing.T) {
	raw, _ := json.Marshal(focusHelpShape([]byte(`{"name":"disableFocus","type":"boolean","default":false,"description":"SECRET"}`)))
	if !strings.Contains(string(raw), `"default":false`) || strings.Contains(string(raw), "SECRET") {
		t.Fatal(string(raw))
	}
}

func TestAcceptFocusCanceledJobHasTerminalRecord(t *testing.T) {
	runner := newWatchRunner(nil, nil)
	ended := make(chan map[string]any, 1)
	runner.observe = func(event map[string]any) {
		if event["event"] == "accept_focus_action_end" {
			ended <- event
		}
	}
	client := &LCUClient{}
	if !runner.scheduleMarked(client, "accept", 5000, http.MethodPost, "/lol-matchmaking/v1/ready-check/accept", nil) {
		t.Fatal("not armed")
	}
	runner.cancelPending("accept")
	select {
	case event := <-ended:
		if event["request_attempted"] != false || event["context_canceled"] != true {
			t.Fatal(event)
		}
	case <-time.After(time.Second):
		t.Fatal("missing terminal event")
	}
}

func TestAcceptFocusRequestHasExactStatusAndSharedTrace(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/lol-matchmaking/v1/ready-check/accept" || r.Method != "POST" {
			t.Error("unexpected request")
		}
		w.WriteHeader(204)
	}))
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "test", http: server.Client()}
	events := []map[string]any{}
	trace := startAcceptFocusTrace(context.Background(), client, 0, func(e map[string]any) { events = append(events, e) })
	trace.beforeRequest()
	ctx := context.WithValue(context.Background(), acceptFocusStatusKey{}, &trace.httpStatus)
	started := time.Now()
	err := client.RequestJSON(ctx, http.MethodPost, "/lol-matchmaking/v1/ready-check/accept", nil, nil)
	trace.afterRequest(err, time.Since(started))
	last := events[len(events)-1]
	if last["status_code"] != int64(204) || last["ok"] != true {
		t.Fatal(last)
	}
	for _, event := range events {
		if event["trace_id"] != trace.id {
			t.Fatal("trace mismatch")
		}
	}
}

func TestAcceptFocusSamplerStopsOnCancellationAndSingleFlight(t *testing.T) {
	client := &LCUClient{token: "test"}
	client.setDiagnosticObserver(func(map[string]any) {})
	ctx, cancel := context.WithCancel(context.Background())
	var mu sync.Mutex
	events := []map[string]any{}
	record := func(e map[string]any) { mu.Lock(); events = append(events, e); mu.Unlock() }
	startAcceptFocusTrace(ctx, client, 0, record)
	startAcceptFocusTrace(ctx, client, 0, record)
	cancel()
	deadline := time.Now().Add(time.Second)
	for client.acceptFocusSampling.Load() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if client.acceptFocusSampling.Load() {
		t.Fatal("sampler did not stop")
	}
	mu.Lock()
	defer mu.Unlock()
	skipped, ended := false, false
	for _, event := range events {
		skipped = skipped || event["stage"] == "sampling-skipped"
		ended = ended || event["stage"] == "capture-end"
	}
	if !skipped || !ended {
		t.Fatal(events)
	}
}

func TestAcceptFocusExportInspectionReadOnlyAndRedacted(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "Config"), 0755)
	for _, name := range []string{"LCULocalPreferences.yaml", "LCUAccountPreferences.yaml", "LeagueClientSettings.yaml"} {
		os.WriteFile(filepath.Join(root, "Config", name), []byte("lol-user-experience:\n    data:\n        disableFocus: true\n        lastSessionId: PRIVATEJWT\n        label: PRIVATESTRING\n"), 0600)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("unexpected write: %s", r.Method)
		}
		switch r.URL.Path {
		case "/data-store/v1/install-dir":
			json.NewEncoder(w).Encode(root)
		case "/Help":
			w.Write([]byte(`{"functions":{"focus":{"name":"FocusWindow","url":"/riotclient/focus","description":"PRIVATESTRING"}}}`))
		default:
			w.Write([]byte(`{"data":{"disableFocus":true,"lastSessionId":"PRIVATEJWT","label":"PRIVATESTRING"}}`))
		}
	}))
	defer server.Close()
	store := &localStore{root: t.TempDir()}
	os.MkdirAll(filepath.Join(store.root, "logs"), 0755)
	a := &app{storage: store}
	client := &LCUClient{baseURL: server.URL, token: "PRIVATEAUTH", http: server.Client()}
	client.acceptFocusLastTrace.Store("test-trace")
	a.collectAcceptFocusInspection(context.Background(), client)
	raw, err := store.readDiagnosticLog()
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"PRIVATEJWT", "PRIVATESTRING", "PRIVATEAUTH", root} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("leaked private data")
		}
	}
	for _, evidence := range []string{"accept_focus_config", "accept_focus_inspection", "disableFocus", "test-trace", "export-not-at-accept"} {
		if !strings.Contains(string(raw), evidence) {
			t.Fatalf("missing %s", evidence)
		}
	}
}
