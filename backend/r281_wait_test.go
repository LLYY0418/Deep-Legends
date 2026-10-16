package main

import (
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"
)

// Test-only convergence wait. Progress is supplied by completed work/state,
// never by elapsed time or poll counts; the hard ceiling also bounds livelock.
// read must only take a bounded state snapshot: no channel receive, network I/O,
// or waiting for another task. All call sites keep waiting in this helper.
type r281WaitOptions struct {
	idle, hard, interval time.Duration
	now                  func() time.Time
	sleep                func(time.Duration)
}

func r281AwaitProgress(label string, read func() (bool, int, string), options r281WaitOptions) error {
	if options.idle == 0 {
		options.idle = 15 * time.Second
	}
	if options.hard == 0 {
		options.hard = time.Minute
	}
	if options.interval == 0 {
		options.interval = 10 * time.Millisecond
	}
	if options.now == nil {
		options.now = time.Now
	}
	if options.sleep == nil {
		options.sleep = time.Sleep
	}
	started := options.now()
	changed := started
	previous := ""
	for {
		ready, remaining, progress := read()
		if ready {
			return nil
		}
		now := options.now()
		if progress != previous {
			previous, changed = progress, now
		}
		if now.Sub(started) >= options.hard {
			return fmt.Errorf("%s: hard limit; %d tasks remaining", label, remaining)
		}
		if now.Sub(changed) >= options.idle {
			return fmt.Errorf("%s: no progress; %d tasks remaining", label, remaining)
		}
		options.sleep(options.interval)
	}
}

func r281SeasonSnapshot(a *app) (int, string) {
	a.seasonBackfillMu.Lock()
	defer a.seasonBackfillMu.Unlock()
	keys := make([]string, 0, len(a.seasonBackfills))
	for key := range a.seasonBackfills {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return len(keys), strings.Join(keys, "\x00")
}

func r281AwaitSeasonJobs(a *app, label string, quietSamples int) error {
	quiet := 0
	return r281AwaitProgress(label, func() (bool, int, string) {
		running, progress := r281SeasonSnapshot(a)
		if running == 0 {
			// r276-capture-safe: r281AwaitProgress calls read serially and joins it before returning.
			quiet++
		} else {
			// r276-capture-safe: quiet is private to this synchronous convergence wait.
			quiet = 0
		}
		return quiet >= quietSamples, running, progress
	}, r281WaitOptions{})
}

func r281RequireProgress(t *testing.T, label string, read func() (bool, int, string)) {
	t.Helper()
	if err := r281AwaitProgress(label, read, r281WaitOptions{}); err != nil {
		t.Fatal(err)
	}
}

func r281WaitUntil(t *testing.T, label string, predicate func() bool) {
	t.Helper()
	r281RequireProgress(t, label, func() (bool, int, string) {
		ready := predicate()
		remaining := 1
		if ready {
			remaining = 0
		}
		return ready, remaining, fmt.Sprint(ready)
	})
}

func TestR281BackgroundConvergenceWait(t *testing.T) {
	clock := time.Unix(1800000000, 0)
	start := clock
	// r276-capture-safe: the fake clock is advanced by synchronous options.sleep; no goroutine is started.
	options := r281WaitOptions{now: func() time.Time { return clock }, sleep: func(d time.Duration) { clock = clock.Add(d) }, interval: time.Second}
	// The original fixed five-second deadline cannot observe a sixth-second completion.
	legacyReady := false
	deadline := clock.Add(5 * time.Second)
	for clock.Before(deadline) {
		if clock.Sub(start) >= 6*time.Second {
			legacyReady = true
			break
		}
		clock = clock.Add(time.Second)
	}
	if legacyReady {
		t.Fatal("old fixed deadline unexpectedly accepted the delayed job")
	}
	clock = start
	if err := r281AwaitProgress("six-second season cleanup", func() (bool, int, string) {
		return clock.Sub(start) >= 6*time.Second, 1, "one pending task"
	}, options); err != nil {
		t.Fatal(err)
	}
	if clock.Sub(start) != 6*time.Second {
		t.Fatal("completion was accepted before the job finished")
	}
	clock = start
	err := r281AwaitProgress("never converges season cleanup", func() (bool, int, string) { return false, 7, "unchanged" }, options)
	if err == nil || !strings.Contains(err.Error(), "never converges season cleanup: no progress; 7 tasks remaining") {
		t.Fatalf("missing named failure: %v", err)
	}
	clock = start
	options.idle, options.hard = 3*time.Second, 8*time.Second
	err = r281AwaitProgress("changing tasks", func() (bool, int, string) { return false, 2, clock.String() }, options)
	if err == nil || !strings.Contains(err.Error(), "hard limit; 2 tasks remaining") {
		t.Fatalf("progress bypassed hard ceiling: %v", err)
	}
	clock = start
	err = r281AwaitProgress("finishing tasks", func() (bool, int, string) {
		remaining := 4 - int(clock.Sub(start)/(2*time.Second))
		return remaining == 0, remaining, fmt.Sprint(remaining)
	}, options)
	if err != nil {
		t.Fatal(err)
	}
}
