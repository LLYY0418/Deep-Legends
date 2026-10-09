package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestR264CollectionIdentityAndStreamUseSameConnectedSession(t *testing.T) {
	client := &LCUClient{region: "kr", rsoPlatform: "KR"}
	a := &app{lcu: client, identityReady: true, eventStream: true, summoner: Summoner{SummonerID: 1, PUUID: "fixture"}, storage: newFlowDiagnosticStore(t)}
	for cycle := 0; cycle < 2; cycle++ {
		w := httptest.NewRecorder()
		a.handleCollectionEnsure(w, httptest.NewRequest(http.MethodPost, "/api/collection/ensure", nil))
		if w.Code != http.StatusAccepted {
			t.Fatalf("cycle %d: identity and stream ready but ensure=%d", cycle, w.Code)
		}
		if view := a.currentClientView(); view.State != "ready" {
			t.Fatalf("ensure accepted but view=%+v", view)
		}
		w = httptest.NewRecorder()
		a.handleStatus(w, httptest.NewRequest(http.MethodGet, "/api/status", nil))
		var status statusResponse
		if err := json.Unmarshal(w.Body.Bytes(), &status); err != nil || !status.Connected {
			t.Fatalf("status mismatch: %v %+v", err, status)
		}
		a.mu.Lock()
		a.clearSnapshotLocked("")
		a.mu.Unlock()
		w = httptest.NewRecorder()
		a.handleCollectionEnsure(w, httptest.NewRequest(http.MethodPost, "/api/collection/ensure", nil))
		if w.Code != http.StatusConflict {
			t.Fatal("disconnected collection accepted")
		}
		a.mu.Lock()
		a.lcu = client
		a.identityReady = true
		a.eventStream = true
		a.summoner = Summoner{SummonerID: 1, PUUID: "fixture"}
		a.mu.Unlock()
	}
	data, _ := a.storage.readDiagnosticLog()
	var found bool
	for _, line := range splitJSONLines(data) {
		var event map[string]any
		if json.Unmarshal(line, &event) == nil && event["event"] == "collection_ensure" {
			found = true
			if _, ok := event["client_view_state"]; !ok {
				t.Fatal("missing view diagnostic")
			}
		}
	}
	if !found {
		t.Fatal("missing collection_ensure diagnostic")
	}
}

func splitJSONLines(data []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i, b := range data {
		if b == '\n' {
			lines = append(lines, data[start:i])
			start = i + 1
		}
	}
	return lines
}

func TestR264LateIdentityCannotResurrectClosedSession(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	client := &LCUClient{baseURL: "https://fixture", token: "fixture", region: "kr", rsoPlatform: "KR", platformProbe: true}
	client.http = &http.Client{Transport: gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		body := `{}`
		if r.URL.Path == "/lol-summoner/v1/current-summoner" {
			close(entered)
			<-release
			body = `{"summonerId":1,"puuid":"fixture","gameName":"Fixture"}`
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})}
	a := &app{lcu: client, connected: true}
	done := make(chan bool, 1)
	go func() { done <- a.refreshIdentityWithClient(client) }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("identity request did not start")
	}
	a.mu.Lock()
	a.clearSnapshotLocked("")
	a.mu.Unlock()
	close(release)
	select {
	case accepted := <-done:
		if accepted {
			t.Fatal("old identity resurrected the closed session")
		}
	case <-time.After(time.Second):
		t.Fatal("identity completion stalled")
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.clientSessionConnectedLocked() || a.identityReady || a.lcu != nil || a.summoner.SummonerID != 0 {
		t.Fatal("stale session published")
	}
}
