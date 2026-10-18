package main

import (
	"strings"
	"testing"
)

func TestR286RealExitAndTaskkillFixtures(t *testing.T) {
	found := 0
	for _, sample := range r285GameLogFixtures(t) {
		for _, line := range strings.Split(sample.Net, "\n") {
			if (strings.Contains(line, "Memory Allocations") || strings.Contains(line, "logfile stopped")) && strings.Contains(line, ",X.X.X.X") {
				t.Fatal("incorrect sanitized suffix", line)
			}
		}
		if sample.Directory != "2026-10-18T22-54-39" && sample.Directory != "2026-10-18T22-58-32" {
			continue
		}
		found++
		s := r285FixtureSignal(t, sample)
		want, marker := "canceled-voluntary", "game-exit-early"
		if strings.Contains(sample.Directory, "22-58-32") {
			want, marker = "fired", "game-truncated"
			if strings.Contains(sample.R3D, "Quit Game Confirmed") {
				t.Fatal("kill fixture contains invented voluntary marker")
			}
		} else if !strings.Contains(sample.R3D, "Quit Game Confirmed") {
			t.Fatal("missing real voluntary marker")
		}
		runner := r284Reconnect(t, s)
		runner.start(t)
		runner.now.Store(15000)
		runner.release <- struct{}{}
		runner.decision(t, want)
		if s.MatchedMarker != marker {
			t.Fatalf("%s: %+v", sample.Directory, s)
		}
	}
	if found != 2 {
		t.Fatal("missing R286 real fixtures", found)
	}
}
