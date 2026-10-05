package main

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

func r227AssertNotStopped(t *testing.T, f *executionFixture) {
	t.Helper()
	snapshot := f.r.champSelectSnapshot()
	for _, record := range snapshot.Records {
		if strings.Contains(record.Message, "自动选用已停止") {
			t.Fatalf("gate produced a failure record: %s", record.Message)
		}
	}
	if snapshot.PickStates["5"] == "stopped" {
		t.Fatal("gate marked a recoverable hero stopped")
	}
	f.r.mu.Lock()
	stopped := f.r.champSelect.exhausted["stopped:5"]
	f.r.mu.Unlock()
	if stopped {
		t.Fatal("gate left a sticky stopped marker")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, event := range f.events {
		if event["stage"] == "stopped" {
			t.Fatalf("gate emitted a stopped diagnostic: %v", event)
		}
	}
}

func r227AssertGateDiagnostic(t *testing.T, f *executionFixture, stage, reason string) {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, event := range f.events {
		if event["stage"] == stage && event["reason"] == reason {
			return
		}
	}
	t.Fatalf("missing %s/%s diagnostic", stage, reason)
}

func TestR227GateChangesDuringSessionReadDoNotStopPick(t *testing.T) {
	for _, gate := range []string{"pause", "takeover", "leave-phase", "disabled"} {
		t.Run(gate, func(t *testing.T) {
			f := r224PickFixture(t, []int64{5}, false, "lock-now")
			r224StartPick(f)
			base := f.c.http.Transport
			reading, release, evaluated := make(chan struct{}), make(chan struct{}), make(chan struct{})
			// Freeze the first session GET after evaluate's initial gates passed.
			f.c.http.Transport = gameplayRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.Method == http.MethodGet && req.URL.Path == champSelectAPI+"/session" {
					close(reading)
					select {
					case <-release:
					case <-req.Context().Done():
						return nil, req.Context().Err()
					}
				}
				return base.RoundTrip(req)
			})
			settings := f.r.currentWatch().ChampSelect
			go func() {
				defer close(evaluated)
				f.r.evaluateChampSelect(f.c, settings)
			}()
			select {
			case <-reading:
			case <-time.After(time.Second):
				close(release)
				t.Fatal("evaluation did not reach session GET")
			}
			if gate == "leave-phase" {
				f.r.handleChampSelectPhase("Lobby")
			} else {
				f.r.mu.Lock()
				switch gate {
				case "pause":
					f.r.champSelect.sessionPaused = true
				case "takeover":
					f.r.champSelect.takeover["pick"] = true
				case "disabled":
					f.r.settings.ChampSelect.Enabled = false
				}
				f.r.mu.Unlock()
			}
			close(release)
			select {
			case <-evaluated:
			case <-time.After(time.Second):
				t.Fatal("evaluation did not finish after session GET")
			}
			f.c.http.Transport = base
			if f.count() != 0 {
				t.Fatal("blocked evaluation wrote a pick")
			}
			if gate == "takeover" {
				r227AssertGateDiagnostic(t, f, "candidate-gate", "manual-takeover")
			} else {
				r227AssertGateDiagnostic(t, f, "schedule", "gate-blocked")
			}
			r227AssertNotStopped(t, f)
			if gate == "leave-phase" {
				f.r.handleChampSelectPhase("ChampSelect")
			} else {
				f.r.mu.Lock()
				f.r.champSelect.sessionPaused = false
				delete(f.r.champSelect.takeover, "pick")
				f.r.settings.ChampSelect.Enabled = true
				f.r.mu.Unlock()
			}
			f.tick(t)
			if f.count() != 1 || f.last().Body["championId"] != float64(5) || f.last().Body["completed"] != true {
				t.Fatal("resumed evaluation did not lock the candidate")
			}
			r227AssertNotStopped(t, f)
		})
	}
}

func TestR227TakeoverAfterCandidateEvaluationDoesNotStopPick(t *testing.T) {
	f := r224PickFixture(t, []int64{5}, false, "lock-now")
	r224StartPick(f)
	observe := f.r.observe
	f.r.observe = func(event map[string]any) {
		observe(event)
		// Takeover can also happen after the early candidate gate was checked.
		if event["stage"] == "candidates" && event["reason"] == "evaluated" {
			f.r.mu.Lock()
			f.r.champSelect.takeover["pick"] = true
			f.r.mu.Unlock()
		}
	}
	f.tick(t)
	f.r.observe = observe
	if f.count() != 0 {
		t.Fatal("late takeover wrote a pick")
	}
	r227AssertGateDiagnostic(t, f, "schedule", "gate-blocked")
	r227AssertNotStopped(t, f)
	f.r.mu.Lock()
	delete(f.r.champSelect.takeover, "pick")
	f.r.mu.Unlock()
	f.tick(t)
	if f.count() != 1 {
		t.Fatal("released takeover did not resume scheduling")
	}
	r227AssertNotStopped(t, f)
}
