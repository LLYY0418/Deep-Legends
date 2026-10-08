package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func r185Chat(status string, stamp int) LCUEvent {
	return LCUEvent{URI: "/lol-chat/v1/me", Data: json.RawMessage(fmt.Sprintf(`{"icon":7,"availability":"chat","statusMessage":%q,"lol":{"rankedLeagueQueue":"RANKED_SOLO_5x5","rankedLeagueTier":"GOLD","rankedLeagueDivision":"I"},"timestamp":%d}`, status, stamp))}
}
func TestR185FacadeFingerprintAndSummary(t *testing.T) {
	a := r175App(t)
	events := make(chan string, 64)
	a.eventSubscribers = map[chan string]struct{}{events: {}}
	t.Cleanup(a.clearFacadeEventThrottle)
	a.handleFacadeLCUEvent(r185Chat("private-signature", 0))
	<-events
	a.facadeEventMu.Lock()
	generation := a.facadeEventSummaryGeneration
	a.facadeEventMu.Unlock()
	a.flushFacadeEventSourcesGeneration(generation)
	for i := 0; i < 30; i++ {
		a.handleFacadeLCUEvent(r185Chat("private-signature", i+1))
	}
	if len(events) != 0 {
		t.Fatalf("timestamp events broadcast %d", len(events))
	}
	a.facadeEventMu.Lock()
	generation = a.facadeEventSummaryGeneration
	a.facadeEventMu.Unlock()
	a.flushFacadeEventSourcesGeneration(generation)
	rows := r175Events(t, a, "facade_event_source")
	last := rows[len(rows)-1]
	if last["received"] != float64(30) || last["ignored"] != float64(30) || last["broadcast"] != float64(0) {
		t.Fatal(last)
	}
	a.facadeEventMu.Lock()
	a.facadeEventLastBroadcast = time.Now().Add(-facadeEventThrottleInterval)
	a.facadeEventMu.Unlock()
	a.handleFacadeLCUEvent(r185Chat("changed", 40))
	if len(events) != 1 {
		t.Fatal("status change did not broadcast")
	}
	if strings.Contains(fmt.Sprint(rows), "private-signature") {
		t.Fatal("chat content leaked")
	}
	for _, body := range []string{`{"availability":"away"}`, `"away"`, `invalid`} {
		if _, valid := facadeEventFingerprint(LCUEvent{URI: "/lol-chat/v1/me", Data: json.RawMessage(body)}); valid {
			t.Fatalf("partial was treated as complete: %s", body)
		}
	}
}
func TestR185FacadeFiveSecondTrailingAndProjection(t *testing.T) {
	a := r175App(t)
	events := make(chan string, 8)
	a.eventSubscribers = map[chan string]struct{}{events: {}}
	t.Cleanup(a.clearFacadeEventThrottle)
	if facadeEventThrottleInterval != 5*time.Second {
		t.Fatal(facadeEventThrottleInterval)
	}
	a.handleFacadeLCUEvent(r185Chat("a", 0))
	a.handleFacadeLCUEvent(r185Chat("b", 1))
	if len(events) != 1 {
		t.Fatal("two immediate broadcasts")
	}
	a.facadeEventMu.Lock()
	generation := a.facadeEventGeneration
	a.facadeEventTimer.Stop()
	a.facadeEventMu.Unlock()
	a.flushFacadeChangedEvent(generation)
	if len(events) != 2 {
		t.Fatal("no trailing broadcast")
	}
	a.flushFacadeChangedEvent(generation)
	if len(events) != 2 {
		t.Fatal("duplicate trailing")
	}
	for _, uri := range []string{"/lol-challenges/v1/summary-player-data/local-player", "/lol-regalia/v2/current-summoner/regalia", "/lol-collections/v1/inventories/1/backdrop"} {
		first := LCUEvent{URI: uri, Data: json.RawMessage(`{"title":"one","selectedChallengesString":"1,2,3","timestamp":1}`)}
		f, _ := facadeEventFingerprint(first)
		first.Data = json.RawMessage(`{"title":"one","selectedChallengesString":"1,2,3","timestamp":2}`)
		next, _ := facadeEventFingerprint(first)
		if (f == next) != (facadeEventSource(uri) == "challenges") {
			t.Fatal("wrong projection", uri)
		}
		first.Data = json.RawMessage(`{"title":"two","selectedChallengesString":"1,2,3"}`)
		changed, _ := facadeEventFingerprint(first)
		if changed == f {
			t.Fatal("change missed", uri)
		}
	}
	a.clearFacadeEventThrottle()
	if len(a.facadeEventFingerprints) != 0 || a.facadeEventSummaryTimer != nil {
		t.Fatal("disconnect retained timers")
	}
}
func TestR185FacadeStateWithoutSkins(t *testing.T) {
	a, client, closeServer := newR60FacadeFixture(t, `{"title":"title","selectedChallengesString":"1"}`, func(w http.ResponseWriter) { fmt.Fprint(w, `{"1":{"name":"badge"}}`) })
	defer closeServer()
	a.facadeIdentityShapeDiagnosticClient = client
	a.allSkinsWithBase = []Skin{{ID: 1000, ChampionID: 1, Name: "skin"}}
	read := func(query string) map[string]any {
		w := httptest.NewRecorder()
		a.handleFacadeState(w, httptest.NewRequest("GET", "/api/facade/state"+query, nil))
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		var value map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	full := read("?trigger=manual")
	if len(full["skins"].([]any)) != 1 {
		t.Fatal(full)
	}
	slim := read("?trigger=sse&skins=0")
	if _, exists := slim["skins"]; exists {
		t.Fatal("skins was serialized")
	}
	delete(full, "skins")
	if !reflect.DeepEqual(full, slim) {
		t.Fatalf("non-skin fields differ\nfull=%v\nslim=%v", full, slim)
	}
	if len(read("")["skins"].([]any)) != 1 {
		t.Fatal("SSE poisoned full cache")
	}
}
func TestR185BackendRuntimeMetricsAndHTTP(t *testing.T) {
	a := r175App(t)
	a.eventSubscribers = map[chan string]struct{}{make(chan string): {}, make(chan string): {}}
	ctx, cancel := context.WithCancel(context.Background())
	ticks := make(chan time.Time)
	done := make(chan struct{})
	ready := make(chan struct{})
	var reads atomic.Int32
	go func() {
		defer close(done)
		a.runBackendRuntimeMetrics(ctx, ticks, func(m *runtime.MemStats) {
			n := reads.Add(1)
			*m = runtime.MemStats{HeapAlloc: 8 << 20, HeapInuse: 12 << 20, NumGC: 3, PauseTotalNs: uint64(n) * 250000000}
			if n == 1 {
				close(ready)
			}
		})
	}()
	<-ready
	t.Cleanup(func() { cancel(); <-done })
	entered, release, requestDone := make(chan struct{}), make(chan struct{}), make(chan struct{})
	handler := a.trackLocalHTTP(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { close(entered); <-release }))
	go func() {
		defer close(requestDone)
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/api/status", nil))
	}()
	<-entered
	if len(r175Events(t, a, "backend_runtime_metrics")) != 0 {
		t.Fatal("recorded before tick")
	}
	ticks <- time.Unix(60, 0)
	rows := r175WaitEvent(t, a, "backend_runtime_metrics")
	r := rows[0]
	for key, value := range map[string]float64{"window_ms": 60000, "heap_alloc_mb": 8, "heap_inuse_mb": 12, "num_gc": 3, "gc_pause_delta_ms": 250, "sse_connections": 2, "local_http_inflight": 1} {
		if r[key] != value {
			t.Fatalf("%s=%v want %v", key, r[key], value)
		}
	}
	if r["goroutines"].(float64) < 1 {
		t.Fatal(r)
	}
	close(release)
	<-requestDone
	if a.localHTTPInFlight.Load() != 0 {
		t.Fatal("HTTP counter leaked")
	}
	ticks <- time.Unix(120, 0)
	deadline := time.Now().Add(time.Second)
	for len(r175Events(t, a, "backend_runtime_metrics")) < 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if len(r175Events(t, a, "backend_runtime_metrics")) != 2 {
		t.Fatal("not one event per tick")
	}
}
func TestR185RendererAndRequestDiagnosticSchema(t *testing.T) {
	a := r175App(t)
	for _, body := range []string{`{"event":"renderer_perf","reason":"aggregated","windowMs":60000,"longtaskCount":2,"longtaskTotalMs":180,"longtaskMaxMs":100,"timerLagCount":1,"timerLagMaxMs":500,"domNodes":500,"imgCount":50,"heapUsedMb":8,"groups":[{"section":"suite","tab":"facade","count":2,"totalMs":180,"maxMs":100},{"section":"private-account","tab":"private-player","count":1}]}`, `{"event":"local_request_client","reason":"complete","endpoint":"facade","durationMs":20,"responseBytes":1234}`} {
		w := httptest.NewRecorder()
		a.handleClientDiagnostic(w, httptest.NewRequest("POST", "/api/diagnostics/client", strings.NewReader(body)))
		if w.Code != 204 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	r := r175Events(t, a, "renderer_perf")[0]
	if r["longtask_count"] != float64(2) || r["timer_lag_count"] != float64(1) || len(r["groups"].([]any)) != 1 {
		t.Fatal(r)
	}
	if strings.Contains(fmt.Sprint(r), "private-") {
		t.Fatal("private grouping leaked")
	}
	req := r175Events(t, a, "local_request_client")[0]
	if req["response_bytes"] != float64(1234) || req["endpoint"] != "facade" {
		t.Fatal(req)
	}
}
