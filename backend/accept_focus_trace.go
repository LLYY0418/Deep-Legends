package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

var acceptTraceSequence atomic.Uint64

type acceptFocusTrace struct {
	id         string
	started    time.Time
	record     func(map[string]any)
	before     acceptWindowSnapshot
	httpStatus atomic.Int64
}

type acceptFocusStatusKey struct{}

func (t *acceptFocusTrace) emit(event map[string]any) {
	event["trace_id"] = t.id
	event["elapsed_ms"] = time.Since(t.started).Milliseconds()
	event["diagnostic_schema"] = 2
	t.record(event)
}
func startAcceptFocusTrace(ctx context.Context, client *LCUClient, delayMS int, record func(map[string]any)) *acceptFocusTrace {
	t := &acceptFocusTrace{id: fmt.Sprintf("accept-%d-%d", time.Now().UnixMilli(), acceptTraceSequence.Add(1)), started: time.Now(), record: record}
	client.acceptFocusLastTrace.Store(t.id)
	t.emit(client.acceptFocusHistorySnapshot())
	t.emit(map[string]any{"event": "accept_focus_trace", "stage": "ready-check", "configured_delay_ms": delayMS, "read_only": true, "sampling_ms": 200, "capture_seconds": 20, "scope": "sampled-window-state-not-call-stack"})
	client.diagnosticMu.Lock()
	enabled := client.diagnosticObserve != nil
	client.diagnosticMu.Unlock()
	if !enabled {
		t.emit(map[string]any{"event": "accept_focus_trace", "stage": "sampling-skipped", "reason": "diagnostic-observer-unavailable"})
		return t
	}
	original := nativeAcceptWindowState()
	// One short-lived sampler per client. It never blocks the accept HTTP request.
	if !client.acceptFocusSampling.CompareAndSwap(false, true) {
		owner, _ := client.acceptFocusOwner.Load().(string)
		t.emit(map[string]any{"event": "accept_focus_trace", "stage": "sampling-skipped", "reason": "capture-already-active", "owner_trace_id": owner})
		return t
	}
	client.acceptFocusOwner.Store(t.id)
	go func() {
		defer client.acceptFocusSampling.Store(false)
		sampleCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		phaseDone := make(chan struct{})
		go func() { defer close(phaseDone); t.samplePhases(sampleCtx, client) }()
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()
		previous := ""
		previousWindow := original.window
		samples := 0
		changes := 0
		unavailable := 0
		var lastSample time.Time
		var maxGap time.Duration
		endReason := "time-limit"
		defer func() {
			cancel()
			<-phaseDone
			t.emit(map[string]any{"event": "accept_focus_trace", "stage": "capture-end", "samples": samples, "changes": changes, "unavailable_samples": unavailable, "max_sample_gap_ms": maxGap.Milliseconds(), "actual_duration_ms": time.Since(t.started).Milliseconds(), "reason": endReason})
		}()
		// Queue an immediate baseline rather than waiting for the first tick.
		first := make(chan time.Time, 1)
		first <- time.Now()
		for {
			select {
			case <-sampleCtx.Done():
				if ctx.Err() != nil {
					endReason = "canceled"
				}
				return
			case <-first:
			case <-ticker.C:
			}
			if _, ok := client.credentials(); !ok {
				endReason = "credentials-unavailable"
				return
			}
			state := nativeAcceptWindowDetails()
			now := time.Now()
			if !lastSample.IsZero() && now.Sub(lastSample) > maxGap {
				maxGap = now.Sub(lastSample)
			}
			lastSample = now
			if available, _ := state["observation_available"].(bool); !available {
				unavailable++
			}
			foreground := nativeAcceptWindowState()
			state["foreground_changed"] = previousWindow != 0 && foreground.window != 0 && previousWindow != foreground.window
			state["foreground_is_original"] = original.window != 0 && foreground.window == original.window
			previousWindow = foreground.window
			samples++
			raw, _ := json.Marshal(state)
			if string(raw) != previous || samples%10 == 0 {
				if string(raw) != previous {
					changes++
				}
				state["event"] = "accept_focus_window"
				state["sample"] = samples
				t.emit(state)
				previous = string(raw)
			}
		}
	}()
	return t
}

func (t *acceptFocusTrace) samplePhases(ctx context.Context, client *LCUClient) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			callCtx, cancel := context.WithTimeout(ctx, 600*time.Millisecond)
			var phase string
			err := client.GetJSONContext(callCtx, "/lol-gameflow/v1/gameflow-phase", &phase)
			cancel()
			switch phase {
			case "None", "Lobby", "Matchmaking", "ReadyCheck", "ChampSelect", "InProgress", "Reconnect", "WaitingForStats", "PreEndOfGame", "EndOfGame":
			default:
				phase = "unknown"
			}
			event := map[string]any{"event": "accept_focus_phase", "phase": phase, "ok": err == nil}
			if err != nil {
				event["error_class"] = focusErrorClass(err)
			}
			var httpErr *LCUHTTPError
			if errors.As(err, &httpErr) {
				event["status_code"] = httpErr.StatusCode
			}
			t.emit(event)
		}
	}
}
func (t *acceptFocusTrace) beforeRequest() {
	t.before = nativeAcceptWindowState()
	t.emit(acceptWindowDiagnostic("before-http", t.before, t.before))
}
func (t *acceptFocusTrace) afterRequest(err error, duration time.Duration) {
	t.emit(acceptWindowDiagnostic("after-http", t.before, nativeAcceptWindowState()))
	event := map[string]any{"event": "accept_focus_request", "method": http.MethodPost, "path": "/lol-matchmaking/v1/ready-check/accept", "ok": err == nil, "duration_ms": duration.Milliseconds()}
	event["status_code"] = t.httpStatus.Load()
	if err != nil {
		switch {
		case errors.Is(err, context.Canceled):
			event["error_class"] = "canceled"
		case errors.Is(err, context.DeadlineExceeded):
			event["error_class"] = "timeout"
		default:
			event["error_class"] = "request-error"
		}
	}
	var e *LCUHTTPError
	if errors.As(err, &e) {
		event["status_code"] = e.StatusCode
	}
	t.emit(event)
}

// Export-side inspection uses a separate lock; never delay automatic acceptance.
type acceptFocusInspection struct{ mu sync.Mutex }
