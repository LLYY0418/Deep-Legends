package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

var r287TimingLogMu sync.Mutex

// Only the supplied temporal observation is repeated. A failed content assertion
// calls testing.T.Fatal normally and is never converted into a retry.
func r287RecordTiming(t *testing.T, label string, limit time.Duration, attempts []float64, ok bool) {
	t.Helper()
	body, err := json.Marshal(struct {
		Label    string    `json:"label"`
		LimitMS  float64   `json:"limitMs"`
		Attempts []float64 `json:"attempts"`
		OK       bool      `json:"ok"`
	}{label, float64(limit) / float64(time.Millisecond), attempts, ok})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("TIMING %s", body)
	folder := os.Getenv("RUNNER_TEMP")
	if folder == "" {
		folder = os.TempDir()
	}
	r287TimingLogMu.Lock()
	defer r287TimingLogMu.Unlock()
	file, err := os.OpenFile(filepath.Join(folder, fmt.Sprintf("r287-go-timing-%d.jsonl", os.Getpid())), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err = file.Write(append(body, '\n')); err != nil {
		t.Fatal(err)
	}
}

func r287WaitSelect(t *testing.T, label string, limit time.Duration, observe func(<-chan time.Time) bool) bool {
	t.Helper()
	var attempts []float64
	for number := 0; number < 2; number++ {
		started := time.Now()
		timer := time.NewTimer(limit)
		ok := observe(timer.C)
		timer.Stop()
		attempts = append(attempts, float64(time.Since(started))/float64(time.Millisecond))
		r287RecordTiming(t, label, limit, attempts, ok)
		if ok {
			return true
		}
	}
	return false
}

func r287Measure(t *testing.T, label string, limit time.Duration, strict bool, measure func() time.Duration) time.Duration {
	t.Helper()
	var attempts []float64
	for number := 0; number < 2; number++ {
		elapsed := measure()
		attempts = append(attempts, float64(elapsed)/float64(time.Millisecond))
		ok := elapsed <= limit
		if strict {
			ok = elapsed < limit
		}
		r287RecordTiming(t, label, limit, attempts, ok)
		if ok {
			return elapsed
		}
	}
	t.Fatalf("%s: both measurements exceeded unchanged limit %s; attempts=%v", label, limit, attempts)
	return 0
}

type r287TimingFailure struct{ label string }
type r287TimingContext struct {
	mu       sync.Mutex
	attempts map[string][]float64
	accepted map[string]bool
}

// Collect all assertions in the measurement fixture. Logical failures still use
// testing.T normally and stop immediately; only temporal observations are retried.
func r287TimingScenario(t *testing.T, observe func(*r287TimingContext)) {
	t.Helper()
	if failure := r287RunTimingScenario(observe, t.Failed); failure != nil {
		t.Fatalf("%s: both timing measurements exceeded unchanged limit", failure.label)
	}
}
func r287RunTimingScenario(observe func(*r287TimingContext), failed func() bool) *r287TimingFailure {
	context := &r287TimingContext{attempts: make(map[string][]float64), accepted: make(map[string]bool)}
	for number := 0; number < 2; number++ {
		observe(context)
		if failed() {
			return nil
		}
		var exceeded *r287TimingFailure
		context.mu.Lock()
		for label, ok := range context.accepted {
			if !ok {
				exceeded = &r287TimingFailure{label}
				break
			}
		}
		context.mu.Unlock()
		if exceeded == nil {
			return nil
		}
		if number == 1 {
			return exceeded
		}
	}
	return nil
}
func (context *r287TimingContext) note(label string, elapsed, limit time.Duration, strict bool) ([]float64, bool) {
	context.mu.Lock()
	defer context.mu.Unlock()
	values := append(context.attempts[label], float64(elapsed)/float64(time.Millisecond))
	context.attempts[label] = values
	ok := elapsed <= limit
	if strict {
		ok = elapsed < limit
	}
	context.accepted[label] = context.accepted[label] || ok
	return append([]float64(nil), values...), ok
}
func r287TooSlow(context *r287TimingContext, t *testing.T, label string, elapsed, limit time.Duration, strict bool) bool {
	t.Helper()
	values, ok := context.note(label, elapsed, limit, strict)
	r287RecordTiming(t, label, limit, values, ok)
	// Do not abort the fixture before its remaining content/geometry assertions.
	return false
}

type r287Deadline struct {
	C      chan time.Time
	cancel chan struct{}
	done   chan struct{}
	once   sync.Once
}

func (deadline *r287Deadline) Stop() {
	deadline.once.Do(func() { close(deadline.cancel) })
	<-deadline.done
}
func r287NewDeadline(t *testing.T, label string, limit time.Duration) *r287Deadline {
	t.Helper()
	return r287StartDeadline(limit, time.Now, func(d time.Duration) (<-chan time.Time, func()) {
		timer := time.NewTimer(d)
		return timer.C, func() { timer.Stop() }
	}, func(values []float64, ok bool) { r287RecordTiming(t, label, limit, values, ok) })
}
func r287StartDeadline(limit time.Duration, now func() time.Time, timer func(time.Duration) (<-chan time.Time, func()), record func([]float64, bool)) *r287Deadline {
	deadline := &r287Deadline{C: make(chan time.Time, 1), cancel: make(chan struct{}), done: make(chan struct{})}
	go func() {
		defer close(deadline.done)
		var values []float64
		for number := 0; number < 2; number++ {
			started := now()
			tick, stop := timer(limit)
			select {
			case <-deadline.cancel:
				stop()
				values = append(values, float64(now().Sub(started))/float64(time.Millisecond))
				record(values, true)
				return
			case <-tick:
				stop()
				values = append(values, float64(now().Sub(started))/float64(time.Millisecond))
				record(append([]float64(nil), values...), false)
				if number == 1 {
					deadline.C <- now()
					return
				}
			}
		}
	}()
	return deadline
}

type r287ClockDeadline struct {
	t       *testing.T
	label   string
	limit   time.Duration
	started time.Time
	values  []float64
	stopped bool
}

func r287NewClockDeadline(t *testing.T, label string, limit time.Duration) *r287ClockDeadline {
	return &r287ClockDeadline{t: t, label: label, limit: limit, started: time.Now()}
}
func (deadline *r287ClockDeadline) Pending() bool {
	if deadline.stopped {
		return false
	}
	elapsed := time.Since(deadline.started)
	if elapsed < deadline.limit {
		return true
	}
	deadline.values = append(deadline.values, float64(elapsed)/float64(time.Millisecond))
	r287RecordTiming(deadline.t, deadline.label, deadline.limit, deadline.values, false)
	if len(deadline.values) == 2 {
		deadline.stopped = true
		return false
	}
	deadline.started = time.Now()
	return true
}
func (deadline *r287ClockDeadline) Stop() {
	if deadline.stopped {
		return
	}
	deadline.stopped = true
	deadline.values = append(deadline.values, float64(time.Since(deadline.started))/float64(time.Millisecond))
	r287RecordTiming(deadline.t, deadline.label, deadline.limit, deadline.values, true)
}
