package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func r264ExportFixture(t *testing.T) *localStore {
	t.Helper()
	store := newFlowDiagnosticStore(t)
	for i := 0; i < 5; i++ {
		name := "diagnostics.jsonl"
		if i > 0 {
			name = fmt.Sprintf("diagnostics.%d.jsonl", i)
		}
		line := fmt.Sprintf("{\"generation\":%d,\"payload\":\"%s\"}\n", i, strings.Repeat("x", 1000))
		data := bytes.Repeat([]byte(line), 2000)
		if err := os.WriteFile(filepath.Join(store.root, "logs", name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	return store
}

func TestR264ExportKeepsTenMBAndNewestFirst(t *testing.T) {
	a := &app{storage: r264ExportFixture(t)}
	w := httptest.NewRecorder()
	a.handleDiagnosticLog(w, httptest.NewRequest(http.MethodGet, "/api/diagnostics/log", nil))
	data := w.Body.Bytes()
	if len(data) < 10_000_000 || data[len(data)-1] != '\n' {
		t.Fatalf("incomplete: %d", len(data))
	}
	if !bytes.HasPrefix(data, []byte(`{"generation":0`)) {
		t.Fatal("oldest data precedes latest")
	}
	for i := 0; i < 5; i++ {
		marker := []byte(fmt.Sprintf(`{"generation":%d`, i))
		if bytes.Count(data, marker) != 2000 {
			t.Fatalf("generation %d lost lines", i)
		}
		if i > 0 && bytes.Index(data, marker) <= bytes.LastIndex(data, []byte(fmt.Sprintf(`{"generation":%d`, i-1))) {
			t.Fatalf("generation %d is out of order", i)
		}
	}
	if w.Header().Get("Content-Length") != fmt.Sprint(len(data)) {
		t.Fatal("missing exact download size")
	}
}

func TestR264ExportSlowThirtyFiveSecondProbeCannotConsumeWriteDeadline(t *testing.T) {
	release := make(chan struct{})
	var once sync.Once
	var probes atomic.Int32
	client := &LCUClient{baseURL: "https://fixture", token: "fixture", region: "kr", rsoPlatform: "KR", http: &http.Client{Transport: sgpRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		probes.Add(1)
		once.Do(func() {
			select {
			case <-time.After(35 * time.Second):
			case <-release:
			}
		})
		return nil, context.DeadlineExceeded
	})}}
	a := &app{storage: r264ExportFixture(t), connected: true, identityReady: true, lcu: client, summoner: Summoner{SummonerID: 1, PUUID: "fixture"}}
	server := httptest.NewUnstartedServer(http.HandlerFunc(a.handleDiagnosticLog))
	server.Config.WriteTimeout = 30 * time.Second
	server.Start()
	defer func() { close(release); server.Close() }()
	done := make(chan error, 1)
	started := time.Now()
	go func() {
		response, err := server.Client().Get(server.URL)
		if err != nil {
			done <- err
			return
		}
		defer response.Body.Close()
		data, err := io.ReadAll(response.Body)
		if err == nil && (len(data) < 10_000_000 || data[len(data)-1] != '\n') {
			err = fmt.Errorf("truncated: %d", len(data))
		}
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(7 * time.Second):
		server.CloseClientConnections()
		t.Fatal("35s probe blocked export beyond total 5s budget")
	}
	if time.Since(started) > 7*time.Second {
		t.Fatal("export exceeded budget")
	}
	if probes.Load() == 0 {
		t.Fatal("slow probe was not exercised")
	}
}
