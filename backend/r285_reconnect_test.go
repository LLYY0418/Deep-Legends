package main

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"
)

func TestR285ReconnectHTTPAndPhaseConfirmation(t *testing.T) {
	for _, kind := range []string{"sse-before-http", "transport-phase", "http500", "different-game"} {
		t.Run(kind, func(t *testing.T) {
			f := r284Reconnect(t, r285CrashSignal())
			original := f.client.http.Transport
			f.client.http.Transport = gameplayRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.URL.Path != "/lol-gameflow/v1/reconnect" {
					return original.RoundTrip(req)
				}
				f.posts.Add(1)
				if kind == "http500" {
					res := response2351(nil)
					res.StatusCode = 500
					return res, nil
				}
				f.phase.Store("GameStart")
				f.runner.handlePhase(f.client, "GameStart")
				if kind == "different-game" {
					f.game.Store(999)
				}
				if kind == "sse-before-http" {
					select {
					case <-req.Context().Done():
						return nil, req.Context().Err()
					case <-time.After(20 * time.Millisecond):
						return response2351(nil), nil
					}
				}
				return nil, errors.New("fixture transport interrupted")
			})
			f.start(t)
			f.now.Store(15000)
			f.release <- struct{}{}
			want, by, status := "fired", "http", 200
			if kind == "transport-phase" {
				by, status = "phase", 0
			}
			if kind == "http500" || kind == "different-game" {
				want, by = "failed", "none"
				if kind == "http500" {
					status = 500
				} else {
					status = 0
				}
			}
			if !r287WaitSelect(t, "backend/r285_reconnect_test.go:56 wait", 7*time.Second, func(r287Deadline <-chan time.Time) bool {
				select {
				case d := <-f.decisions:
					if d["outcome"] != want || d["confirmed_by"] != by || d["http_status"] != status {
						t.Fatalf("%s decision=%v", kind, d)
					}
					return true
				case <-r287Deadline:
					return false
				}
			}) {
				t.Fatal("missing bounded decision")
			}
			if f.posts.Load() != 1 {
				t.Fatalf("posts=%d", f.posts.Load())
			}
		})
	}
}

func TestR285ReconnectDelayChoicesAndSettlement(t *testing.T) {
	for _, tc := range []struct{ in, want int }{{0, 5000}, {3000, 5000}, {5000, 5000}, {10000, 10000}, {12000, 10000}, {15000, 15000}, {20000, 20000}, {25000, 20000}, {30000, 30000}, {40000, 30000}} {
		s := defaultWatchSettings()
		s.Rules.AutoReconnect.DelayMS = tc.in
		if got := normalizeWatchSettings(s).Rules.AutoReconnect.DelayMS; got != tc.want {
			t.Fatalf("%d => %d want %d", tc.in, got, tc.want)
		}
	}
	if reconnectSettlementDelayMS != 5000 {
		t.Fatalf("minimum=%d", reconnectSettlementDelayMS)
	}
	f := r284Reconnect(t, watchReconnectSignal{})
	s := f.runner.currentWatch()
	s.Rules.AutoReconnect.DelayMS = 5000
	f.runner.apply(s)
	f.runner.handlePhase(f.client, "Reconnect")
	if d := <-f.waiting; d != 5*time.Second {
		t.Fatalf("timer=%s", d)
	}
	<-f.sampled
	f.now.Store(2800)
	f.phase.Store("WaitingForStats")
	f.runner.handlePhase(f.client, "WaitingForStats")
	f.decision(t, "canceled-settle")
	if f.posts.Load() != 0 {
		t.Fatal("settlement reconnected")
	}
}

func TestR285ReconnectSessionCancellationStillStopsPOST(t *testing.T) {
	f := r284Reconnect(t, r285CrashSignal())
	session, cancel := context.WithCancel(context.Background())
	defer cancel()
	f.client.sessionContext = session
	original := f.client.http.Transport
	f.client.http.Transport = gameplayRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/lol-gameflow/v1/reconnect" {
			return original.RoundTrip(req)
		}
		f.posts.Add(1)
		cancel()
		<-req.Context().Done()
		return nil, req.Context().Err()
	})
	f.start(t)
	f.release <- struct{}{}
	d := f.decision(t, "failed")
	if d["confirmed_by"] != "none" || d["http_status"] != 0 {
		t.Fatal(d)
	}
}

func r285CrashSignal() watchReconnectSignal {
	return watchReconnectSignal{MatchedMarker: "crash-handler", CrashHandlerSeen: true, ProcessKnown: true, ProcessGone: true}
}
