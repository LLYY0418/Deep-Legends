package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestConvenienceSettingsPersist(t *testing.T) {
	root := t.TempDir()
	store := trackTestStore(t, &localStore{root: root})
	want := convenienceSettings{AutoAccept: true, AutoReconnect: true}
	settings := loadWatchSettings(store)
	settings.Rules.AutoAccept.Enabled = want.AutoAccept
	settings.Rules.AutoPlayAgain.Enabled = want.AutoPlayAgain
	settings.Rules.AutoReconnect.Enabled = want.AutoReconnect
	if err := saveWatchSettings(store, settings); err != nil {
		t.Fatal(err)
	}
	got := legacyConvenienceSettings(loadWatchSettings(store))
	if !got.AutoAccept || got.AutoPlayAgain || !got.AutoReconnect {
		t.Fatalf("loaded %+v, want %+v", got, want)
	}
	if _, err := os.Stat(filepath.Join(root, convenienceSettingsFile)); err != nil {
		t.Fatalf("settings file missing: %v", err)
	}
}

func TestHandleGameplayConvenienceRoundTrip(t *testing.T) {
	root := t.TempDir()
	store := trackTestStore(t, &localStore{root: root})
	a := &app{storage: store, convenience: newWatchRunner(store, nil)}

	get := httptest.NewRequest(http.MethodGet, "/api/gameplay/convenience", nil)
	recorder := httptest.NewRecorder()
	a.handleGameplayConvenience(recorder, get)
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var initial convenienceSettings
	if err := json.Unmarshal(recorder.Body.Bytes(), &initial); err != nil {
		t.Fatal(err)
	}
	if initial.AutoAccept || initial.AutoPlayAgain || initial.AutoReconnect {
		t.Fatalf("defaults should be off: %+v", initial)
	}

	body := `{"autoAccept":true,"autoPlayAgain":true,"autoReconnect":false}`
	post := httptest.NewRequest(http.MethodPost, "/api/gameplay/convenience", strings.NewReader(body))
	post.Header.Set("Content-Type", "application/json")
	recorder = httptest.NewRecorder()
	a.handleGameplayConvenience(recorder, post)
	if recorder.Code != http.StatusOK {
		t.Fatalf("POST status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	current := a.convenience.current()
	if !current.AutoAccept || !current.AutoPlayAgain || current.AutoReconnect {
		t.Fatalf("in-memory settings = %+v", current)
	}
	saved := legacyConvenienceSettings(loadWatchSettings(store))
	if saved != current {
		t.Fatalf("disk %+v != memory %+v", saved, current)
	}
}

func TestHandleGameplayConvenienceNilRunner(t *testing.T) {
	a := &app{}
	recorder := httptest.NewRecorder()
	a.handleGameplayConvenience(recorder, httptest.NewRequest(http.MethodGet, "/api/gameplay/convenience", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET status = %d", recorder.Code)
	}
	post := httptest.NewRequest(http.MethodPost, "/api/gameplay/convenience", strings.NewReader(`{"autoAccept":true}`))
	post.Header.Set("Content-Type", "application/json")
	recorder = httptest.NewRecorder()
	a.handleGameplayConvenience(recorder, post)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("POST status = %d, want 503", recorder.Code)
	}
}

func TestConvenienceHandlePhaseIsNilSafe(t *testing.T) {
	var runner *convenienceRunner
	runner.handlePhase(nil, "ReadyCheck")
}

func TestConvenienceReadyCheckPostsAcceptOnce(t *testing.T) {
	requests := make(chan string, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		requests <- r.Method + " " + r.URL.Path
		if r.Method == http.MethodGet && r.URL.Path == "/lol-gameflow/v1/session" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"phase":"ReadyCheck","gameData":{"queue":{"id":420,"gameMode":"CLASSIC","mapId":11,"type":"MATCHED_GAME"}}}`)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	runner := newWatchRunner(nil, nil)
	settings := defaultWatchSettings()
	settings.Rules.AutoAccept.Enabled = true
	settings.Rules.AutoAccept.DelayMS = 0
	runner.apply(settings)
	runner.handlePhase(client, "ReadyCheck")
	runner.handlePhase(client, "ReadyCheck")
	if !r287WaitSelect(t, "backend/convenience_test.go:115 wait", 2*time.Second, func(r287Deadline <-chan time.Time) bool {
		select {
		case got := <-requests:
			if got != "GET /lol-gameflow/v1/session" {
				t.Fatalf("got %q", got)
			}
			return true
		case <-r287Deadline:
			return false
		}
	}) {
		t.Fatal("ready-check preflight was not requested")
	}
	if !r287WaitSelect(t, "backend/convenience_test.go:123 wait", 2*time.Second, func(r287Deadline <-chan time.Time) bool {
		select {
		case got := <-requests:
			if got != "POST /lol-matchmaking/v1/ready-check/accept" {
				t.Fatalf("got %q", got)
			}
			return true
		case <-r287Deadline:
			return false
		}
	}) {
		t.Fatal("accept was not posted after the non-custom preflight")
	}
	select {
	case extra := <-requests:
		t.Fatalf("debounced accept still posted %q", extra)
	case <-time.After(200 * time.Millisecond):
	}
}

func TestConvenienceDisabledPhaseDoesNotCallClient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected LCU call %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	runner := newWatchRunner(nil, nil)
	runner.handlePhase(client, "ReadyCheck")
	runner.handlePhase(client, "EndOfGame")
	runner.handlePhase(client, "Reconnect")
	time.Sleep(150 * time.Millisecond)
}

func TestConvenienceReconnectPostsAndNotifies(t *testing.T) {
	done := make(chan string, 1)
	events := make(chan string, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/lol-gameflow/v1/session" {
			_, _ = w.Write([]byte(`{"gameData":{"gameId":284}}`))
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == "/lol-gameflow/v1/gameflow-phase" {
			_, _ = w.Write([]byte(`"Reconnect"`))
			return
		}
		done <- r.Method + " " + r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	runner := newWatchRunner(nil, func(event string) { events <- event })
	runner.reconnectSignals = func(context.Context, *LCUClient) watchReconnectSignal { return r285CrashSignal() }
	fire := make(chan struct{})
	waiting := make(chan time.Duration, 1)
	runner.wait = func(ctx context.Context, d time.Duration) error {
		waiting <- d
		select {
		case <-fire:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	settings := defaultWatchSettings()
	settings.Rules.AutoReconnect.Enabled = true
	settings.Rules.AutoReconnect.DelayMS = 3000
	runner.apply(settings)
	runner.handlePhase(client, "Reconnect")
	if !r287WaitSelect(t, "backend/convenience_test.go:187 wait", time.Second, func(r287Deadline <-chan time.Time) bool {
		select {
		case event := <-events:
			if event != "watch:armed:reconnect:5000" {
				t.Fatalf("armed event = %q", event)
			}
			return true
		case <-r287Deadline:
			return false
		}
	}) {
		t.Fatal("reconnect was not armed")
	}
	if delay := <-waiting; delay != 5*time.Second {
		t.Fatalf("delay=%v", delay)
	}
	select {
	case <-done:
		t.Fatal("POST before timer fired")
	default:
	}
	close(fire)
	if !r287WaitSelect(t, "backend/convenience_test.go:204 wait", 4*time.Second, func(r287Deadline <-chan time.Time) bool {
		select {
		case got := <-done:
			if got != "POST /lol-gameflow/v1/reconnect" {
				t.Fatalf("got %q", got)
			}
			return true
		case <-r287Deadline:
			return false
		}
	}) {
		t.Fatal("reconnect was not posted")
	}
	if !r287WaitSelect(t, "backend/convenience_test.go:212 wait", 2*time.Second, func(r287Deadline <-chan time.Time) bool {
		select {
		case event := <-events:
			if event != "watch:fired:reconnect" {
				t.Fatalf("event = %q", event)
			}
			return true
		case <-r287Deadline:
			return false
		}
	}) {
		t.Fatal("reconnect event was not broadcast")
	}
}
