package main

import (
	"context"
	"sync"
	"time"
)

// Optional test observer: no production caller installs it and no identifiers
// or request contents are passed to it. Spans overlap for concurrent workers.
type riotTimingProbeKey struct{}
type riotTimingProbe func(string, time.Time, time.Time)

func startRiotTimingProbe(ctx context.Context, phase string) func() {
	probe, _ := ctx.Value(riotTimingProbeKey{}).(riotTimingProbe)
	if probe == nil {
		return func() {}
	}
	started := time.Now()
	probe(phase, started, time.Time{}) // Capture waits that are still in flight at failure.
	return func() { probe(phase, started, time.Now()) }
}

func waitRiotProfile(ctx context.Context, wait *sync.WaitGroup) {
	defer startRiotTimingProbe(ctx, "profile_wait")()
	wait.Wait()
}

// Test-only foreground-window waiter; production contexts use waitRiotDelay.
type riotForegroundWaitKey struct{}
