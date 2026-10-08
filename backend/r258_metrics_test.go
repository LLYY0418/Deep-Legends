package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestR258ColdRequestMetricsWindowEpochAndLateFrames(t *testing.T) {
	a := r175App(t)
	client := &LCUClient{}
	a.lcu = client
	a.connected = true
	start := time.Now()
	a.recordColdLCURequest(start.Add(-time.Millisecond))
	a.recordColdLCURequest(start.Add(time.Millisecond))
	a.observeColdLaunchDiscovery(LCUDiscoveryStatus{ProcessCount: 1, ProcessAt: start, AttemptAt: start}, start)
	a.recordColdLCURequest(start.Add(2999 * time.Millisecond))
	a.recordColdLCURequest(start.Add(3 * time.Second))
	a.recordColdSGPBytes(&LCUClient{}, 999)
	a.recordColdSGPBytes(client, 120)
	a.observeFirstMatchesCard("network", time.Now())
	a.observeColdLaunchMilestone("self_tab_header_ms", time.Now())
	a.recordColdSGPBytes(client, 999)
	events := r175Events(t, a, "client_cold_launch_timeline")
	last := events[len(events)-1]
	if last["lcu_requests_first_3s"] != float64(2) || last["sgp_bytes_first_screen"] != float64(120) || last["browser_queued_requests_first_3s"] != float64(-1) {
		t.Fatal(last)
	}
	if a.recordBrowserColdRequests(start.Add(time.Second).UnixMilli(), 10, 10, true) {
		t.Fatal("foreign epoch accepted")
	}
	if !a.recordBrowserColdRequests(start.UnixMilli(), 2, 6, false) || !a.recordBrowserColdRequests(start.UnixMilli(), 3, 7, true) {
		t.Fatal("late completed resources rejected")
	}
	if a.recordBrowserColdRequests(start.UnixMilli(), 1, 5, true) {
		t.Fatal("stale frame replaced newer counts")
	}
	last = r175Events(t, a, "client_cold_launch_timeline")[len(r175Events(t, a, "client_cold_launch_timeline"))-1]
	if last["browser_queued_requests_first_3s"] != float64(3) || last["browser_requests_window_elapsed"] != true {
		t.Fatal(last)
	}
	a.observeColdLaunchDiscovery(LCUDiscoveryStatus{Result: "process-not-found"}, time.Now())
	if a.coldRequestWindow() != nil || a.recordBrowserColdRequests(start.UnixMilli(), 99, 99, true) {
		t.Fatal("closed window retained")
	}
}
func TestR258ColdLCUCountsJSONAndByteRequests(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{"ok":true}`)) }))
	defer server.Close()
	a := r175App(t)
	client := &LCUClient{http: server.Client(), baseURL: server.URL, token: "fixture"}
	start := time.Now()
	a.observeColdLaunchDiscovery(LCUDiscoveryStatus{ProcessCount: 1, ProcessAt: start, AttemptAt: start}, start)
	a.bindColdRequestCounters(client)
	var out map[string]bool
	if err := client.RequestJSON(context.Background(), http.MethodGet, "/json", nil, &out); err != nil {
		t.Fatal(err)
	}
	if _, err := client.GetBytesContext(context.Background(), "/image"); err != nil {
		t.Fatal(err)
	}
	a.coldLaunch.mu.Lock()
	count := len(a.coldLaunch.lcuStarts)
	a.coldLaunch.mu.Unlock()
	if count != 2 {
		t.Fatalf("starts=%d", count)
	}
}
func TestR258DOMEpochExcludesDeliveryLatencyAndRejectsOldWindow(t *testing.T) {
	now := time.Now()
	at, valid := clientDOMEventTime(now.Add(-time.Second).UnixMilli(), now)
	if !valid || now.Sub(at) < time.Second {
		t.Fatal(at, valid)
	}
	if _, valid = clientDOMEventTime(now.Add(time.Minute).UnixMilli(), now); valid {
		t.Fatal("future epoch accepted")
	}
	a := r175App(t)
	a.observeColdLaunchDiscovery(LCUDiscoveryStatus{ProcessCount: 1, ProcessAt: now, AttemptAt: now}, now)
	a.observeFirstMatchesCard("network", now.Add(-time.Second))
	a.observeColdLaunchMilestone("self_tab_header_ms", now.Add(-time.Second))
	if len(a.coldLaunch.milestones) != 0 || a.coldLaunch.matchesSource != "" {
		t.Fatal("old DOM wrote current launch")
	}
	a.observeOverviewSource("snapshot")
	a.observeFirstMatchesCard("network", now.Add(time.Millisecond))
	if a.coldLaunch.matchesSource != "snapshot" {
		t.Fatal("snapshot mislabeled network")
	}
}
func TestR258NewMetricDiagnosticsAreStrictAndStripPrivateFields(t *testing.T) {
	a := r175App(t)
	now := time.Now()
	a.observeColdLaunchDiscovery(LCUDiscoveryStatus{ProcessCount: 1, ProcessAt: now, AttemptAt: now}, now)
	for _, body := range []string{`{"event":"browser_cold_requests_client","reason":"sample","startedAt":1,"count":-1,"resourceCount":0,"timingAvailable":false,"windowElapsed":true}`, `{"event":"champselect_request_client","reason":"complete","endpoint":"champselect","requestId":5,"responseBytes":124,"startedAt":1,"completedAt":2,"shellNode":"PRIVATE-PATH"}`} {
		recorder := httptest.NewRecorder()
		a.handleClientDiagnostic(recorder, httptest.NewRequest(http.MethodPost, "/api/diagnostics/client", strings.NewReader(body)))
		if recorder.Code != http.StatusNoContent {
			t.Fatal(recorder.Code, recorder.Body.String())
		}
	}
	raw, err := a.storage.readDiagnosticLog()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "PRIVATE-") {
		t.Fatal("raw fields leaked")
	}
	rows := r175Events(t, a, "champselect_request_client")
	if len(rows) != 1 || rows[0]["response_bytes"] != float64(124) || rows[0]["request_id"] != float64(5) {
		t.Fatal(rows)
	}
	recorder := httptest.NewRecorder()
	a.handleClientDiagnostic(recorder, httptest.NewRequest(http.MethodPost, "/api/diagnostics/client", strings.NewReader(`{"event":"browser_cold_requests_client","reason":"sample","startedAt":1,"count":2,"resourceCount":1,"timingAvailable":true}`)))
	if recorder.Code != http.StatusBadRequest {
		t.Fatal("impossible count accepted")
	}
}

func TestR258ColdSGPBytesIncludeGetAndSummonerPost(t *testing.T) {
	getBody := `{"value":"中"}`
	postBody := `[{"name":"public"}]`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			_, _ = w.Write([]byte(postBody))
		} else {
			_, _ = w.Write([]byte(getBody))
		}
	}))
	defer server.Close()
	a := r175App(t)
	client := &LCUClient{}
	a.lcu = client
	a.connected = true
	start := time.Now()
	a.observeColdLaunchDiscovery(LCUDiscoveryStatus{ProcessCount: 1, ProcessAt: start, AttemptAt: start}, start)
	provider := newSGPProvider()
	provider.http = server.Client()
	provider.serverBases["HN1"] = server.URL
	provider.sessionToken = "fixture"
	provider.sessionAt = time.Now()
	provider.sessionOwner = client
	provider.requestBytes = a.recordColdSGPBytes
	var output map[string]string
	if err := provider.getJSONWithToken(context.Background(), client, sgpTokenLeagueSession, "HN1", "RANKED", "/public", server.URL+"/public", &output); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.summonerByPUUIDOn(context.Background(), client, "HN1", strings.Repeat("q", 48)); err != nil {
		t.Fatal(err)
	}
	if a.coldLaunch.sgpFirstScreenBytes != int64(len(getBody)+len(postBody)) {
		t.Fatal("POST bytes omitted", a.coldLaunch.sgpFirstScreenBytes)
	}
}
