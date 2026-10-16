package main

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

// This fixture advances a synthetic transfer clock, but writes and verifies a
// real file. Join the worker, tolerating progress without hiding a stuck worker.
func r196WaitDownloadProgress(t *testing.T, u *updateManager, clock *atomic.Int64) {
	t.Helper()
	u.mu.Lock()
	done := u.downloadDone
	u.mu.Unlock()
	ticks := time.NewTicker(50 * time.Millisecond)
	defer ticks.Stop()
	progress := func() string {
		status := u.Status()
		return fmt.Sprintf("state=%s received=%d selecting=%t transferClock=%d", status.State, status.Progress.ReceivedBytes, status.Progress.SelectingSource, clock.Load())
	}
	if err := r196DownloadWait(done, progress, time.Now(), ticks.C, 15*time.Second, 60*time.Second); err != nil {
		t.Fatal(err)
	}
}

func r196DownloadWait(done <-chan struct{}, progress func() string, started time.Time, ticks <-chan time.Time, idle, hard time.Duration) error {
	lastChange, last := started, progress()
	for {
		// Completion means the actual worker exited, never merely state == ready.
		select {
		case <-done:
			return nil
		default:
		}
		select {
		case <-done:
			return nil
		case now, ok := <-ticks:
			if !ok {
				return fmt.Errorf("R196 download progress stream closed before completion")
			}
			next := progress()
			if next != last {
				last, lastChange = next, now
			}
			if now.Sub(started) >= hard {
				return fmt.Errorf("R196 download hard limit after %s; last progress %s", now.Sub(started), last)
			}
			if now.Sub(lastChange) >= idle {
				return fmt.Errorf("R196 download stalled after %s; last progress %s", now.Sub(started), last)
			}
		}
	}
}

func TestR196DownloadWaitProgressAndCompletion(t *testing.T) {
	done := make(chan struct{})
	ticks := make(chan time.Time, 4)
	started := time.Unix(0, 0)
	for _, seconds := range []int{4, 8, 12, 16} {
		ticks <- started.Add(time.Duration(seconds) * time.Second)
	}
	var polls atomic.Int32
	progress := func() string {
		count := polls.Add(1)
		if count == 5 {
			close(done)
		}
		return fmt.Sprint(count)
	}
	if err := r196DownloadWait(done, progress, started, ticks, 5*time.Second, 30*time.Second); err != nil {
		t.Fatal(err)
	}
	if polls.Load() != 5 {
		t.Fatalf("returned before worker completion: polls=%d", polls.Load())
	}
}

func TestR196DownloadWaitStallAndHardLimit(t *testing.T) {
	started := time.Unix(0, 0)
	done := make(chan struct{})
	ticks := make(chan time.Time, 2)
	ticks <- started.Add(5 * time.Second)
	err := r196DownloadWait(done, func() string { return "received=17" }, started, ticks, 5*time.Second, 20*time.Second)
	if err == nil || err.Error() != "R196 download stalled after 5s; last progress received=17" {
		t.Fatalf("stall must fail with last progress: %v", err)
	}
	ticks <- started.Add(20 * time.Second)
	close(ticks)
	var calls atomic.Int32
	err = r196DownloadWait(done, func() string { return fmt.Sprint(calls.Add(1)) }, started, ticks, 5*time.Second, 20*time.Second)
	if err == nil || err.Error() != "R196 download hard limit after 20s; last progress 2" {
		t.Fatalf("continuous progress must not hide a livelock: %v", err)
	}
}
