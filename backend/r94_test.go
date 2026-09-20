package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"

	"github.com/gorilla/websocket"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func r94Client(read func(*http.Request) (string, error)) *LCUClient {
	return &LCUClient{baseURL: "https://lcu.invalid", token: "test", http: &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		body, err := read(r)
		if err != nil {
			return nil, err
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})}}
}

func TestR94PhaseEndpointFindsSamePhaseIdentityJumpsWithoutRosterReads(t *testing.T) {
	var paths []string
	gameID := int64(8980574903)
	client := r94Client(func(r *http.Request) (string, error) {
		paths = append(paths, r.URL.Path)
		switch r.URL.Path {
		case "/lol-gameflow/v1/gameflow-phase":
			return `"InProgress"`, nil
		case "/lol-gameflow/v1/session":
			return fmt.Sprintf(`{"phase":"InProgress","gameData":{"gameId":%d}}`, gameID), nil
		default:
			t.Fatalf("identity probe read full live data: %s", r.URL.Path)
			return "", nil
		}
	})
	a := &app{connected: true, lcu: client}
	for _, id := range []int64{8980574903, 8980593274, 8980593274} {
		gameID = id
		recorder := httptest.NewRecorder()
		a.handleGameplayPhase(recorder, httptest.NewRequest(http.MethodGet, "/api/gameplay/phase", nil))
		var got gameplayPhaseIdentity
		if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if got.Phase != "InProgress" || got.GameID != id || !got.Connected {
			t.Fatalf("identity = %+v", got)
		}
	}
	if !reflect.DeepEqual(paths, []string{"/lol-gameflow/v1/gameflow-phase", "/lol-gameflow/v1/session", "/lol-gameflow/v1/gameflow-phase", "/lol-gameflow/v1/session", "/lol-gameflow/v1/gameflow-phase", "/lol-gameflow/v1/session"}) {
		t.Fatal(paths)
	}
}

func TestR94IdentityProbePhaseFallbackAndCancellation(t *testing.T) {
	for _, tc := range []struct {
		name, phase, session string
		fail                 bool
		wantPhase            string
		wantID               int64
		probes               int
	}{
		{"selection ignores previous game", "ChampSelect", `{"phase":"ChampSelect","gameData":{"gameId":90}}`, false, "ChampSelect", 0, 0},
		{"inactive has no identity", "Lobby", ``, false, "Lobby", 0, 0},
		{"session failure retains phase", "InProgress", ``, true, "InProgress", 0, 1},
		{"session sees newer phase", "InProgress", `{"phase":"ChampSelect","gameData":{"gameId":90}}`, false, "ChampSelect", 0, 1},
		{"reconnect identity", "Reconnect", `{"phase":"Reconnect","gameData":{"gameId":91}}`, false, "Reconnect", 91, 1},
		{"missing identity", "GameStart", `{"phase":"GameStart","gameData":{}}`, false, "GameStart", 0, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			probes := 0
			client := r94Client(func(r *http.Request) (string, error) {
				if strings.HasSuffix(r.URL.Path, "gameflow-phase") {
					return `"` + tc.phase + `"`, nil
				}
				probes++
				deadline, ok := r.Context().Deadline()
				if !ok || time.Until(deadline) > 1500*time.Millisecond {
					t.Fatal("identity probe must be bounded to 1.5s")
				}
				if tc.fail {
					return "", errors.New("session unavailable")
				}
				return tc.session, nil
			})
			got := readGameplayPhaseIdentity(context.Background(), client)
			if got.Phase != tc.wantPhase || got.GameID != tc.wantID || probes != tc.probes {
				t.Fatalf("identity=%+v probes=%d", got, probes)
			}
		})
	}
	t.Run("request cancellation reaches session probe", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		client := r94Client(func(r *http.Request) (string, error) {
			if strings.HasSuffix(r.URL.Path, "gameflow-phase") {
				return `"InProgress"`, nil
			}
			cancel()
			<-r.Context().Done()
			return "", r.Context().Err()
		})
		got := readGameplayPhaseIdentity(ctx, client)
		if got.Phase != "InProgress" || got.GameID != 0 {
			t.Fatal(got)
		}
	})
}

func TestR94SessionSSECarriesIdentityAndRejectsOldSelectionData(t *testing.T) {
	updates := make(chan string, 32)
	a := &app{eventSubscribers: map[chan string]struct{}{updates: {}}}
	for _, id := range []int64{8980574903, 8980593274} {
		a.broadcastGameplayIdentity(LCUEvent{URI: "/lol-gameflow/v1/session", Data: json.RawMessage(fmt.Sprintf(`{"phase":"InProgress","gameData":{"gameId":%d}}`, id))})
		select {
		case message := <-updates:
			if !strings.HasPrefix(message, "gameflow:") {
				t.Fatal(message)
			}
			var got gameplayPhaseIdentity
			if err := json.Unmarshal([]byte(strings.TrimPrefix(message, "gameflow:")), &got); err != nil {
				t.Fatal(err)
			}
			if got.Phase != "InProgress" || got.GameID != id {
				t.Fatal(got)
			}
		default:
			t.Fatal("identity event missing")
		}
	}
	for _, data := range []string{`{"phase":"ChampSelect","gameData":{"gameId":90}}`, `{"gameData":{"gameId":90}}`, `{"phase":"InProgress"}`, `malformed`} {
		a.broadcastGameplayIdentity(LCUEvent{URI: "/lol-gameflow/v1/session", Data: json.RawMessage(data)})
	}
	a.broadcastGameplayIdentity(LCUEvent{URI: "/unrelated", Data: json.RawMessage(`{"phase":"InProgress","gameData":{"gameId":90}}`)})
	if len(updates) != 0 {
		t.Fatal("untrusted session identity forwarded")
	}
	a.broadcastEvent("gameflow:ChampSelect")
	if got := <-updates; got != "gameflow:ChampSelect" {
		t.Fatal(got)
	}
}

func TestR94DiagnosticBatchPersistsRawPhaseSourceAndIdentityComparisons(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "logs"), 0700); err != nil {
		t.Fatal(err)
	}
	a := &app{storage: trackTestStore(t, &localStore{root: root})}
	body := clientDiagnosticRequest{Event: "gameflow_phase_client", Reason: "batch", TransportDropped: 3, TransportFailed: 1}
	for _, id := range []int64{8980574903, 8980593274, 0} {
		body.Observations = append(body.Observations, gameflowClientObservation{Reason: "received", Phase: "InProgress", PreviousPhase: "InProgress", Source: "poll", GameID: id, CachedGameID: 8980574903, GameIDComparison: "deliberately incorrect", ReceivedAt: 1789370200123})
	}
	body.Observations = append(body.Observations, gameflowClientObservation{Reason: "invalidate", Phase: "Reconnect", PreviousPhase: "InProgress", Source: "sse", GameID: 8980593274, Invalidated: true, PhaseChanged: true})
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	a.handleClientDiagnostic(recorder, httptest.NewRequest(http.MethodPost, "/api/diagnostics/client", strings.NewReader(string(encoded))))
	if recorder.Code != 204 {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	log, err := a.storage.readDiagnosticLog()
	if err != nil {
		t.Fatal(err)
	}
	var rows []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(string(log)), "\n") {
		var row map[string]any
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			t.Fatal(err)
		}
		if row["event"] == "gameflow_phase_client" {
			rows = append(rows, row)
		}
	}
	if len(rows) != 4 {
		t.Fatalf("rows=%s", log)
	}
	for i, comparison := range []string{"same", "different", "unavailable", "first"} {
		if rows[i]["game_id_comparison"] != comparison {
			t.Fatal(rows[i])
		}
	}
	if rows[1]["phase"] != "InProgress" || rows[1]["source"] != "poll" || rows[1]["game_id"] != float64(8980593274) || rows[1]["cached_game_id"] != float64(8980574903) || rows[1]["received_at"] != float64(1789370200123) || rows[1]["transport_dropped"] != float64(3) {
		t.Fatal(rows[1])
	}
	if rows[3]["source"] != "sse" || rows[3]["phase"] != "Reconnect" || rows[3]["invalidated"] != true {
		t.Fatal(rows[3])
	}
}

func TestR94DiagnosticBatchRejectsMalformedObservations(t *testing.T) {
	valid := gameflowClientObservation{Reason: "received", Phase: "InProgress", Source: "poll", GameID: 8980574903}
	for name, observations := range map[string][]gameflowClientObservation{
		"empty": {}, "too many": {valid, valid, valid, valid, valid, valid, valid, valid, valid},
		"invalid phase":  {{Reason: "received", Phase: "private player name", Source: "poll"}},
		"invalid source": {{Reason: "received", Phase: "InProgress", Source: "private"}},
		"invalid ID":     {{Reason: "received", Phase: "InProgress", Source: "poll", GameID: 9007199254740992}},
	} {
		t.Run(name, func(t *testing.T) {
			body, err := json.Marshal(clientDiagnosticRequest{Event: "gameflow_phase_client", Reason: "batch", Observations: observations})
			if err != nil {
				t.Fatal(err)
			}
			recorder := httptest.NewRecorder()
			(&app{}).handleClientDiagnostic(recorder, httptest.NewRequest(http.MethodPost, "/api/diagnostics/client", strings.NewReader(string(body))))
			if recorder.Code != 400 {
				t.Fatalf("status=%d", recorder.Code)
			}
		})
	}
}

func TestR94ConnectedSessionForwardsIdentityThroughLCUEventCallback(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !websocket.IsWebSocketUpgrade(r) {
			http.Error(w, "fixture endpoint unavailable", http.StatusNotFound)
			return
		}
		connection, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer connection.Close()
		if _, _, err := connection.ReadMessage(); err != nil {
			return
		}
		for _, id := range []int64{8980574903, 8980593274} {
			event := map[string]any{"uri": "/lol-gameflow/v1/session", "eventType": "Update", "data": map[string]any{"phase": "InProgress", "gameData": map[string]any{"gameId": id}}}
			if err := connection.WriteJSON([]any{8, "OnJsonApiEvent", event}); err != nil {
				return
			}
		}
		event := map[string]any{"uri": "/lol-gameflow/v1/gameflow-phase", "eventType": "Update", "data": "InProgress"}
		if err := connection.WriteJSON([]any{8, "OnJsonApiEvent", event}); err != nil {
			return
		}
		_, _, _ = connection.ReadMessage()
	}))
	defer server.Close()
	address, _ := url.Parse(server.URL)
	port, _ := strconv.Atoi(address.Port())
	client := newLCUClient(port, "fixture")
	defer client.Close()
	updates := make(chan string, 32)
	a := &app{lcu: client, connected: true, refreshRequests: make(chan struct{}, 1), eventSubscribers: map[chan string]struct{}{updates: {}}}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- a.runConnectedSession(ctx, client) }()
	defer func() {
		cancel()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Error("session failed to stop")
		}
	}()
	var got []string
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	for len(got) < 3 {
		select {
		case message := <-updates:
			if strings.HasPrefix(message, "gameflow:") {
				got = append(got, message)
			}
		case <-deadline.C:
			t.Fatalf("missing actual callback broadcasts: %v", got)
		}
	}
	if !strings.Contains(got[0], `"gameId":8980574903`) || !strings.Contains(got[1], `"gameId":8980593274`) || got[2] != "gameflow:InProgress" {
		t.Fatal(got)
	}
}
