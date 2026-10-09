package main

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestR264HistoryCircuitStops503ButKeepsRankedAndSummoner(t *testing.T) {
	var history, other atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "history") {
			history.Add(1)
			http.Error(w, "outage", 503)
			return
		}
		other.Add(1)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()
	client := &LCUClient{}
	p := newSGPProvider()
	p.http = server.Client()
	p.token, p.tokenAt, p.tokenClient = "fixture", time.Now(), client
	p.retryWait = func(context.Context, time.Duration) error { return nil }
	for i := 0; i < 6; i++ {
		route := "SUMMARY"
		if i%2 == 1 {
			route = "DETAILS"
		}
		var out map[string]any
		_ = p.getJSON(context.Background(), client, "HN1", route, "history", server.URL+"/history", &out)
	}
	if history.Load() != 3 {
		t.Fatalf("cooldown did not stop history requests: %d", history.Load())
	}
	for _, route := range []string{"RANKED", "SUMMONER"} {
		var out map[string]any
		if err := p.getJSON(context.Background(), client, "HN1", route, "other", server.URL+"/other", &out); err != nil {
			t.Fatal(err)
		}
	}
	if other.Load() != 2 {
		t.Fatal("healthy endpoints affected")
	}
}

func TestR264HistoryCircuitSingleProbeRecoveryAndServerIsolation(t *testing.T) {
	var requests atomic.Int32
	var healthy atomic.Bool
	var clock atomic.Int64
	clock.Store(time.Now().UnixNano())
	gate := make(chan struct{})
	entered := make(chan struct{}, 1)
	var block atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if block.Load() {
			select {
			case entered <- struct{}{}:
			default:
			}
			<-gate
		}
		if !healthy.Load() {
			http.Error(w, "outage", 503)
			return
		}
		w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()
	client := &LCUClient{}
	p := newSGPProvider()
	p.http = server.Client()
	p.token, p.tokenAt, p.tokenClient = "fixture", time.Now(), client
	p.retryWait = func(context.Context, time.Duration) error { return nil }
	p.historyClock = func() time.Time { return time.Unix(0, clock.Load()) }
	read := func(serverID string) error {
		var out map[string]any
		return p.getJSON(context.Background(), client, serverID, "SUMMARY", "history", server.URL, &out)
	}
	_ = read("HN1")
	if requests.Load() != 3 {
		t.Fatal(requests.Load())
	}
	healthy.Store(true)
	if err := read("HN2"); err != nil {
		t.Fatal("other server was blocked", err)
	}
	clock.Add(int64(61 * time.Second))
	block.Store(true)
	done := make(chan error, 1)
	go func() { done <- read("HN1") }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("missing probe")
	}
	for i := 0; i < 10; i++ {
		if err := read("HN1"); err == nil {
			t.Fatal("concurrent probe accepted")
		}
	}
	if requests.Load() != 5 {
		t.Fatalf("more than one half-open request: %d", requests.Load())
	}
	close(gate)
	block.Store(false)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := read("HN1"); err != nil {
		t.Fatal("successful probe did not recover", err)
	}
}

func TestR264OverviewNonHistoryCardsArriveBeforeUnavailableHistory(t *testing.T) {
	a, publicRef, _ := newGameplayOverviewSGPFixture(t, true)
	old := a.sgp.http.Transport
	a.sgp.retryWait = func(context.Context, time.Duration) error { return nil }
	a.sgp.http.Transport = sgpRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if strings.Contains(r.URL.Path, "match-history-query") {
			select {
			case <-time.After(3 * time.Second):
			case <-r.Context().Done():
				return nil, r.Context().Err()
			}
			return &http.Response{StatusCode: 503, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("outage")), Request: r}, nil
		}
		return old.RoundTrip(r)
	})
	oldLCU := a.lcu.http.Transport
	a.lcu.http.Transport = gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if strings.Contains(r.URL.Path, "lol-match-history") {
			return &http.Response{StatusCode: 503, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("outage")), Request: r}, nil
		}
		return oldLCU.RoundTrip(r)
	})
	server := httptest.NewServer(http.HandlerFunc(a.handleGameplayOverview))
	defer server.Close()
	request, _ := http.NewRequest(http.MethodPost, server.URL, strings.NewReader(`{"playerRef":"`+publicRef+`","serverId":"HN1","count":20}`))
	request.Header.Set("Accept", "application/x-ndjson")
	request.Header.Set("X-Overview-Cards", "1")
	client := server.Client()
	client.Timeout = 2 * time.Second
	started := time.Now()
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("non-history cards waited for history: %v", err)
	}
	defer response.Body.Close()
	line, err := bufio.NewReader(response.Body).ReadBytes('\n')
	if err != nil {
		t.Fatal(err)
	}
	var frame struct {
		Type     string           `json:"type"`
		Overview gameplayOverview `json:"overview"`
	}
	if err = json.Unmarshal(line, &frame); err != nil || frame.Type != "cards" || len(frame.Overview.Capabilities) == 0 || time.Since(started) > 2*time.Second {
		t.Fatalf("early cards missing: %s, %v", line, err)
	}
}

func TestR264LCU503HistoryAndTimelineUseOneShortOutageMessage(t *testing.T) {
	a, _, _ := newGameplayOverviewSGPFixture(t, false)
	a.sgp = nil
	a.lcu.http.Transport = gameplayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 503, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("raw fixture outage")), Request: r}, nil
	})
	_, caps, _ := a.loadDetailedMatches(context.Background(), a.lcu, gameplayReference{ServerID: "HN1"}, a.summoner.PUUID, true, 0, 20, "all", nil, nil)
	found := false
	for _, cap := range caps {
		if cap.Name == "match-history" {
			found = true
			if cap.Detail != historyServerUnavailableMessage {
				t.Fatal(cap)
			}
		}
	}
	if !found {
		t.Fatal("missing history capability")
	}
	w := httptest.NewRecorder()
	a.handleGameplayMatchTimeline(w, httptest.NewRequest(http.MethodPost, "/api/gameplay/match-timeline", strings.NewReader(`{"gameId":264,"participantId":1,"serverId":"HN1"}`)))
	var timeline matchTimelineResponse
	if err := json.Unmarshal(w.Body.Bytes(), &timeline); err != nil || timeline.Available || timeline.Detail != historyServerUnavailableMessage {
		t.Fatalf("timeline %v %s", err, w.Body.String())
	}
}
