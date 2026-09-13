package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The apply diagnostic used to set phase = "ChampSelect" as a constant right
// after the guard passed, so the field could never show what phase a rejected
// apply was actually in. Prove the recorded value now comes from the client.
func TestR79ItemSetContextReportsObservedGameflowPhase(t *testing.T) {
	for _, testCase := range []struct {
		name      string
		phaseBody string
		status    int
		want      string
		wantError bool
	}{
		{name: "champ-select", phaseBody: `"ChampSelect"`, status: http.StatusOK, want: "ChampSelect"},
		{name: "in-progress", phaseBody: `"InProgress"`, status: http.StatusOK, want: "InProgress", wantError: true},
		{name: "lobby", phaseBody: `"Lobby"`, status: http.StatusOK, want: "Lobby", wantError: true},
		{name: "unreadable", phaseBody: `nope`, status: http.StatusInternalServerError, want: "", wantError: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/lol-gameflow/v1/gameflow-phase":
					w.WriteHeader(testCase.status)
					_, _ = w.Write([]byte(testCase.phaseBody))
				case "/lol-champ-select/v1/session":
					_, _ = w.Write([]byte(`{"localPlayerCellId":0,"myTeam":[{"cellId":0,"puuid":"","summonerId":0,"championId":64}]}`))
				default:
					http.Error(w, "unexpected endpoint", http.StatusNotFound)
				}
			}))
			defer server.Close()

			client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
			phase, err := validateGameplayItemSetContextPhase(context.Background(), client, Summoner{}, 64)
			if phase != testCase.want {
				t.Fatalf("observed phase = %q, want %q", phase, testCase.want)
			}
			if (err != nil) != testCase.wantError {
				t.Fatalf("error = %v, wantError = %v", err, testCase.wantError)
			}
		})
	}
}

// End-to-end: a rejected apply must leave the real phase in the diagnostic so a
// future log can distinguish "user clicked with a stale page" from "guard bug".
func TestR79ItemSetApplyDiagnosticRecordsRejectedPhase(t *testing.T) {
	playerRef := strings.Repeat("p", 48)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /lol-gameflow/v1/gameflow-phase":
			_, _ = w.Write([]byte(`"InProgress"`))
		default:
			http.Error(w, "unexpected endpoint", http.StatusNotFound)
		}
	}))
	defer server.Close()

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "logs"), 0o700); err != nil {
		t.Fatal(err)
	}
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	a := &app{
		connected: true,
		lcu:       client,
		summoner:  Summoner{SummonerID: 123, AccountID: 456, PUUID: playerRef},
		storage:   &localStore{root: root},
	}
	body := `{"title":"瑞兹 · 中路","championId":13,"mapId":11,"position":"middle","storage":"recommended","blocks":[{"type":"核心装","items":[{"id":3003,"count":1}]}]}`
	recorder := httptest.NewRecorder()
	a.handleGameplayItemSetApply(recorder, httptest.NewRequest(http.MethodPost, "/api/gameplay/item-sets/apply", strings.NewReader(body)))
	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), "只能在英雄选择阶段应用装备方案") {
		t.Fatalf("status/body = %d %q", recorder.Code, recorder.Body.String())
	}

	data, err := a.storage.readDiagnosticLog()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var event map[string]any
		if json.Unmarshal([]byte(line), &event) != nil || event["event"] != "item_set_apply" {
			continue
		}
		found = true
		if event["phase"] != "InProgress" {
			t.Fatalf("recorded phase = %v, want InProgress (a constant would read ChampSelect or empty)", event["phase"])
		}
		if event["apply_stage"] != "champ-select-guard" {
			t.Fatalf("apply_stage = %v, want champ-select-guard", event["apply_stage"])
		}
		if stored, _ := event["stored"].(bool); stored {
			t.Fatal("a rejected apply must not report stored=true")
		}
	}
	if !found {
		t.Fatalf("no item_set_apply diagnostic was written: %s", data)
	}
}
