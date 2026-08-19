package main

import (
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
	store := &localStore{root: root}
	want := convenienceSettings{AutoAccept: true, AutoReconnect: true}
	if err := saveConvenienceSettings(store, want); err != nil {
		t.Fatal(err)
	}
	got := loadConvenienceSettings(store)
	if !got.AutoAccept || got.AutoPlayAgain || !got.AutoReconnect {
		t.Fatalf("loaded %+v, want %+v", got, want)
	}
	if _, err := os.Stat(filepath.Join(root, convenienceSettingsFile)); err != nil {
		t.Fatalf("settings file missing: %v", err)
	}
}

func TestHandleGameplayConvenienceRoundTrip(t *testing.T) {
	root := t.TempDir()
	store := &localStore{root: root}
	a := &app{storage: store, convenience: newConvenienceRunner(store, nil)}

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
	saved := loadConvenienceSettings(store)
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
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	runner := newConvenienceRunner(nil, nil)
	runner.apply(convenienceSettings{AutoAccept: true})
	runner.handlePhase(client, "ReadyCheck")
	runner.handlePhase(client, "ReadyCheck")
	select {
	case got := <-requests:
		if got != "POST /lol-matchmaking/v1/ready-check/accept" {
			t.Fatalf("got %q", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("accept was not posted")
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
	runner := newConvenienceRunner(nil, nil)
	runner.handlePhase(client, "ReadyCheck")
	runner.handlePhase(client, "EndOfGame")
	runner.handlePhase(client, "Reconnect")
	time.Sleep(150 * time.Millisecond)
}

func TestConvenienceReconnectPostsAndNotifies(t *testing.T) {
	done := make(chan string, 1)
	events := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		done <- r.Method + " " + r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)
	client := &LCUClient{baseURL: server.URL, token: "test-token", http: server.Client()}
	runner := newConvenienceRunner(nil, func(event string) { events <- event })
	runner.apply(convenienceSettings{AutoReconnect: true})
	runner.handlePhase(client, "Reconnect")
	select {
	case got := <-done:
		if got != "POST /lol-gameflow/v1/reconnect" {
			t.Fatalf("got %q", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("reconnect was not posted")
	}
	select {
	case event := <-events:
		if event != "convenience:reconnect" {
			t.Fatalf("event = %q", event)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("reconnect event was not broadcast")
	}
}
