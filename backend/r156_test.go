package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestR156InvitationQueueAndActionDiagnostics(t *testing.T) {
	tests := []struct {
		name, body, policy, result, path string
		queueID, getStatus, postStatus   int
		custom, pauseAfterGet            bool
	}{
		{name: "nested accept takes precedence", body: `[{"invitationId":"nested", "queueId":999, "gameConfig":{"queueId":420}, "summonerName":"PRIVATE_SUMMONER"}]`, policy: "accept", result: "fired", path: "/lol-lobby/v2/received-invitations/nested/accept", queueID: 420},
		{name: "flat accept fallback", body: `[{"invitationId":"flat", "queueId":420}]`, policy: "accept", result: "fired", path: "/lol-lobby/v2/received-invitations/flat/accept", queueID: 420},
		{name: "flat decline fallback", body: `[{"invitationId":"flat-decline", "queueID":440}]`, policy: "decline", result: "fired", path: "/lol-lobby/v2/received-invitations/flat-decline/decline", queueID: 440},
		{name: "nested decline", body: `[{"invitationId":"decline", "gameConfig":{"queueID":440}}]`, policy: "decline", result: "fired", path: "/lol-lobby/v2/received-invitations/decline/decline", queueID: 440},
		{name: "no policy", body: `[{"invitationId":"ignored", "gameConfig":{"queueId":2400}}]`, result: "skipped_no_policy", queueID: 2400},
		{name: "get failure", getStatus: http.StatusInternalServerError, result: "failed"},
		{name: "post failure", body: `[{"invitationId":"failed", "gameConfig":{"queueId":420}}]`, policy: "accept", result: "failed", path: "/lol-lobby/v2/received-invitations/failed/accept", queueID: 420, postStatus: http.StatusInternalServerError},
		{name: "custom paused", custom: true, result: "skipped_custom"},
		{name: "custom paused after list", body: `[{"invitationId":"paused", "gameConfig":{"queueId":420}}]`, pauseAfterGet: true, result: "skipped_custom"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			writes := make(chan string, 1)
			var events []map[string]any
			var eventsMu sync.Mutex
			runner := newWatchRunner(nil, nil)
			runner.observe = func(event map[string]any) {
				eventsMu.Lock()
				events = append(events, event)
				eventsMu.Unlock()
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				if request.Method == http.MethodGet && request.URL.Path == "/lol-lobby/v2/received-invitations" {
					if tt.getStatus != 0 {
						w.WriteHeader(tt.getStatus)
						return
					}
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(tt.body))
					if tt.pauseAfterGet {
						runner.setCustomSession(true)
					}
					return
				}
				if request.Method == http.MethodPost {
					writes <- request.URL.Path
					status := tt.postStatus
					if status == 0 {
						status = http.StatusNoContent
					}
					w.WriteHeader(status)
					return
				}
				http.NotFound(w, request)
			}))
			t.Cleanup(server.Close)
			client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
			if tt.custom {
				runner.setCustomSession(true)
			}
			runner.handleInvitations(client, watchInvitationRule{Policies: map[string]string{"420": "accept", "440": "decline"}})
			if tt.path == "" && len(writes) != 0 {
				t.Fatalf("unexpected POST path: %s", <-writes)
			}
			if tt.path != "" {
				select {
				case path := <-writes:
					if path != tt.path {
						t.Fatalf("POST path = %q, want %q", path, tt.path)
					}
				default:
					t.Fatalf("missing POST path %q", tt.path)
				}
			}
			eventsMu.Lock()
			defer eventsMu.Unlock()
			var actions, shapes []map[string]any
			for _, event := range events {
				switch event["event"] {
				case "watch_action":
					if event["action"] == "invitations" {
						actions = append(actions, event)
					}
				case "lcu_invitation_shape":
					shapes = append(shapes, event)
				}
			}
			if len(actions) != 1 || actions[0]["result"] != tt.result {
				t.Fatalf("invitation diagnostics = %#v, want %s", actions, tt.result)
			}
			if tt.getStatus != 0 && actions[0]["reason"] != "list-read-failed" {
				t.Fatalf("GET failure reason = %#v", actions[0])
			}
			if tt.queueID != 0 && actions[0]["queue_id"] != int64(tt.queueID) {
				t.Fatalf("queue ID = %#v, want %d", actions[0], tt.queueID)
			}
			if tt.policy != "" && actions[0]["policy"] != tt.policy {
				t.Fatalf("policy = %#v, want %s", actions[0], tt.policy)
			}
			if tt.body != "" && tt.getStatus == 0 && len(shapes) != 1 {
				t.Fatalf("shape diagnostics = %#v", shapes)
			}
			payload, err := json.Marshal(events)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(payload), "PRIVATE_SUMMONER") || strings.Contains(string(payload), `"nested"`) {
				t.Fatalf("personal invitation data leaked: %s", payload)
			}
		})
	}
}

func TestR156InvitationShapeRecordedOnceAndKeysOnly(t *testing.T) {
	runner := newWatchRunner(nil, nil)
	var events []map[string]any
	runner.observe = func(event map[string]any) { events = append(events, event) }
	runner.recordInvitationShape(map[string]any{"invitationId": "SECRET_ID", "gameConfig": map[string]any{"queueId": 2400, "inviterName": "SECRET_NAME"}})
	runner.recordInvitationShape(map[string]any{"other": "SECRET_OTHER"})
	if len(events) != 1 || !reflect.DeepEqual(events[0]["top_level_keys"], []string{"gameConfig", "invitationId"}) || !reflect.DeepEqual(events[0]["game_config_keys"], []string{"inviterName", "queueId"}) {
		t.Fatalf("shape diagnostics = %#v", events)
	}
	encoded, _ := json.Marshal(events)
	if strings.Contains(string(encoded), "SECRET_") {
		t.Fatalf("shape diagnostic leaked values: %s", encoded)
	}
}

func TestR156SimulatedInvitationActionReachesDiagnosticLog(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "logs"), 0o700); err != nil {
		t.Fatal(err)
	}
	store := trackTestStore(t, &localStore{root: root})
	runner := newWatchRunner(nil, nil)
	runner.observe = (&app{storage: store}).recordDiagnostic
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/lol-lobby/v2/received-invitations":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"invitationId":"PRIVATE_INVITATION", "summonerName":"PRIVATE_SUMMONER", "gameConfig":{"queueId":2400}}]`))
		case request.Method == http.MethodPost && request.URL.Path == "/lol-lobby/v2/received-invitations/PRIVATE_INVITATION/accept":
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, request)
		}
	}))
	t.Cleanup(server.Close)
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	runner.handleInvitations(client, watchInvitationRule{Policies: map[string]string{"2400": "accept"}})
	logged, err := store.readDiagnosticLog()
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{`"event":"watch_action"`, `"action":"invitations"`, `"result":"fired"`, `"queue_id":2400`, `"policy":"accept"`, `"event":"lcu_invitation_shape"`} {
		if !strings.Contains(string(logged), expected) {
			t.Fatalf("diagnostic log missing %q: %s", expected, logged)
		}
	}
	for _, private := range []string{"PRIVATE_INVITATION", "PRIVATE_SUMMONER"} {
		if strings.Contains(string(logged), private) {
			t.Fatalf("diagnostic log leaked %s: %s", private, logged)
		}
	}
}

func TestR156IdleLCURequestBucketFlushesDuringSession(t *testing.T) {
	client := &LCUClient{}
	events := make(chan map[string]any, 2)
	client.setDiagnosticObserver(func(event map[string]any) { events <- event })
	client.recordRequestDiagnostic(http.MethodGet, "/lol-lobby/v2/received-invitations", newLCURequestTrace(), http.StatusOK)
	client.diagnosticMu.Lock()
	windowStart := client.requestDiagnostics[http.MethodGet+"\x00/lol-lobby/v2/received-invitations"].windowStart
	client.diagnosticMu.Unlock()
	client.flushExpiredRequestDiagnostics(windowStart.Add(5 * time.Second))
	if len(events) != 0 {
		t.Fatal("active request bucket flushed early")
	}
	ctx, cancel := context.WithCancel(context.Background())
	ticks := make(chan time.Time)
	done := make(chan struct{})
	go func() {
		client.runRequestDiagnosticFlush(ctx, ticks)
		close(done)
	}()
	t.Cleanup(func() { cancel(); <-done })
	ticks <- windowStart.Add(lcuRequestDiagnosticWindow + time.Second)
	select {
	case event := <-events:
		if event["event"] != "lcu_request" || event["count"] != 1 || event["path"] != "/lol-lobby/v2/received-invitations" {
			t.Fatalf("idle bucket event = %#v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("idle request bucket did not flush without another request or Close")
	}
	cancel()
	<-done
	select {
	case event := <-events:
		t.Fatalf("request bucket was emitted twice: %#v", event)
	default:
	}
}

func TestR156LCURequestWindowRolloverAndCloseStillFlush(t *testing.T) {
	client := &LCUClient{http: &http.Client{}, token: "test-token"}
	var events []map[string]any
	client.setDiagnosticObserver(func(event map[string]any) { events = append(events, event) })
	trace := newLCURequestTrace()
	path := "/lol-lobby/v2/received-invitations"
	client.recordRequestDiagnostic(http.MethodGet, path, trace, http.StatusOK)
	key := http.MethodGet + "\x00" + path
	client.diagnosticMu.Lock()
	client.requestDiagnostics[key].windowStart = time.Now().Add(-2 * lcuRequestDiagnosticWindow).Truncate(lcuRequestDiagnosticWindow)
	client.diagnosticMu.Unlock()
	client.recordRequestDiagnostic(http.MethodGet, path, trace, http.StatusOK)
	if len(events) != 1 || events[0]["count"] != 1 {
		t.Fatalf("window rollover events = %#v", events)
	}
	client.Close()
	if len(events) != 2 || events[1]["count"] != 1 || client.token != "" {
		t.Fatalf("Close flush events = %#v, token cleared = %t", events, client.token == "")
	}
}

func TestR156InvitationRequestDiagnosticPathHidesInvitationID(t *testing.T) {
	for path, want := range map[string]string{
		"/lol-lobby/v2/received-invitations":                 "/lol-lobby/v2/received-invitations",
		"/lol-lobby/v2/received-invitations/PRIVATE/accept":  "/lol-lobby/v2/received-invitations/{id}/accept",
		"/lol-lobby/v2/received-invitations/PRIVATE/decline": "/lol-lobby/v2/received-invitations/{id}/decline",
	} {
		if got := lcuDiagnosticPath(path); got != want {
			t.Errorf("lcuDiagnosticPath(%q) = %q, want %q", path, got, want)
		}
	}
}
