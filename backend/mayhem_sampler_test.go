package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func r136SamplerApp(t *testing.T) *app {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "logs"), 0o700); err != nil {
		t.Fatal(err)
	}
	return &app{storage: trackTestStore(t, &localStore{root: root})}
}

func TestR136MayhemSamplerRetriesUntilBothShapesSucceed(t *testing.T) {
	a := r136SamplerApp(t)
	now := time.Unix(0, 0)
	a.mayhemSamplerNow = func() time.Time { return now }
	a.mayhemSamplerWait = func(ctx context.Context, duration time.Duration) bool { now = now.Add(duration); return true }
	allCalls, playerCalls := 0, 0
	a.liveClientAllGameData = func(context.Context) ([]byte, int, error) {
		allCalls++
		if allCalls < 4 {
			return nil, 0, syscall.ECONNREFUSED
		}
		return []byte(`{"allPlayers":[{"items":[{"itemID":1}]}]}`), http.StatusOK, nil
	}
	a.liveClientPlayerList = func(context.Context) ([]byte, int, error) {
		playerCalls++
		if playerCalls < 4 {
			return nil, 0, syscall.ECONNREFUSED
		}
		return []byte(`[]`), http.StatusOK, nil
	}
	a.runMayhemSampler(context.Background(), 13601, "InProgress")
	all := r90Events(t, a, "live_client_allgamedata_shape")
	players := r90Events(t, a, "live_client_playerlist_shape")
	summary := r90Events(t, a, "live_client_mayhem_sample_summary")
	if len(all) != 1 || len(players) != 1 || len(summary) != 1 {
		t.Fatalf("shape events = %d/%d, summary = %d", len(all), len(players), len(summary))
	}
	if all[0]["attempt"] != float64(4) || players[0]["attempt"] != float64(4) || summary[0]["end_reason"] != "success" {
		t.Fatalf("retry result = all:%v players:%v summary:%v", all, players, summary)
	}
	if summary[0]["allgamedata_error_kind"] != "connection-refused" || summary[0]["playerlist_error_kind"] != "connection-refused" {
		t.Fatalf("failed attempts lost classification: %v", summary[0])
	}
}

func TestR136MayhemSamplerStopsAtFiveMinutes(t *testing.T) {
	a := r136SamplerApp(t)
	now := time.Unix(0, 0)
	a.mayhemSamplerNow = func() time.Time { return now }
	a.mayhemSamplerWait = func(ctx context.Context, duration time.Duration) bool { now = now.Add(duration); return true }
	a.liveClientAllGameData = func(context.Context) ([]byte, int, error) { return nil, 0, syscall.ECONNREFUSED }
	a.liveClientPlayerList = func(context.Context) ([]byte, int, error) { return nil, 0, syscall.ECONNREFUSED }
	a.runMayhemSampler(context.Background(), 13602, "InProgress")
	summary := r90Events(t, a, "live_client_mayhem_sample_summary")
	if len(summary) != 1 || summary[0]["end_reason"] != "timeout" {
		t.Fatalf("timeout summary = %v", summary)
	}
	if summary[0]["allgamedata_attempts"].(float64) > 30 || summary[0]["playerlist_attempts"].(float64) > 30 {
		t.Fatalf("sampler exceeded bounded attempts: %v", summary[0])
	}
	if len(r90Events(t, a, "live_client_allgamedata_shape")) != 0 || len(r90Events(t, a, "live_client_playerlist_shape")) != 0 {
		t.Fatal("failed attempts must be summarized, not logged one by one")
	}
}

func TestR136MayhemSamplerStopsWhenGameflowLeaves(t *testing.T) {
	a := r136SamplerApp(t)
	client := &LCUClient{}
	a.liveClientAllGameData = func(context.Context) ([]byte, int, error) { return nil, 0, errors.New("2999 unavailable") }
	a.liveClientPlayerList = func(context.Context) ([]byte, int, error) { return nil, 0, errors.New("2999 unavailable") }
	waiting := make(chan struct{}, 1)
	a.mayhemSamplerWait = func(ctx context.Context, duration time.Duration) bool {
		waiting <- struct{}{}
		<-ctx.Done()
		return false
	}
	a.startMayhemSampler(client, 13603, "InProgress")
	select {
	case <-waiting:
	case <-time.After(2 * time.Second):
		t.Fatal("sampler did not start")
	}
	a.stopMayhemSamplerForClient("gameflow-left", client)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		summary := r90Events(t, a, "live_client_mayhem_sample_summary")
		if len(summary) == 1 {
			if summary[0]["end_reason"] != "gameflow-left" || summary[0]["allgamedata_attempts"] != float64(1) {
				t.Fatalf("leave summary = %v", summary[0])
			}
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("gameflow exit did not stop sampler")
}

func TestR136MayhemSamplerIsSingletonPerGame(t *testing.T) {
	a := r136SamplerApp(t)
	client := &LCUClient{}
	calls := make(chan struct{}, 2)
	a.liveClientAllGameData = func(context.Context) ([]byte, int, error) {
		calls <- struct{}{}
		return nil, 0, syscall.ECONNREFUSED
	}
	a.liveClientPlayerList = func(context.Context) ([]byte, int, error) { return nil, 0, syscall.ECONNREFUSED }
	a.mayhemSamplerWait = func(ctx context.Context, _ time.Duration) bool { <-ctx.Done(); return false }
	a.startMayhemSampler(client, 13604, "InProgress")
	select {
	case <-calls:
	case <-time.After(2 * time.Second):
		t.Fatal("sampler did not start")
	}
	a.startMayhemSampler(client, 13604, "Reconnect")
	if len(calls) != 0 {
		t.Fatal("a second sampler started for the same game")
	}
	a.stopMayhemSamplerForClient("gameflow-left", client)
	deadline := time.Now().Add(2 * time.Second)
	for len(r90Events(t, a, "live_client_mayhem_sample_summary")) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("singleton sampler did not stop")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestR136MayhemSamplerErrorKinds(t *testing.T) {
	for _, testCase := range []struct {
		err    error
		status int
		valid  bool
		want   string
	}{
		{syscall.ECONNREFUSED, 0, false, "connection-refused"},
		{context.DeadlineExceeded, 0, false, "timeout"},
		{nil, http.StatusTooManyRequests, false, "http-429"},
		{nil, http.StatusOK, false, "invalid-json"},
	} {
		if got := mayhemSampleErrorKind(testCase.err, testCase.status, testCase.valid); got != testCase.want {
			t.Fatalf("kind = %q, want %q", got, testCase.want)
		}
	}
}
