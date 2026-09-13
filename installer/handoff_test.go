package main

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestApplicationWindowRequiresPIDVisibilityAndUsefulSize(t *testing.T) {
	for _, item := range []struct {
		name   string
		pid    uint32
		window applicationWindow
		want   bool
	}{
		{"splash", 42, applicationWindow{42, true, 460, 292}, true},
		{"main", 42, applicationWindow{42, true, 1280, 800}, true},
		{"other process", 42, applicationWindow{99, true, 1280, 800}, false},
		{"invisible", 42, applicationWindow{42, false, 460, 292}, false},
		{"small helper", 42, applicationWindow{42, true, 1, 1}, false},
		{"no process", 0, applicationWindow{0, true, 460, 292}, false},
	} {
		t.Run(item.name, func(t *testing.T) {
			if matchesApplicationWindow(item.pid, item.window) != item.want {
				t.Fatal(item)
			}
		})
	}
}

func TestApplicationHandoffKeepsInstallerUntilItsWindow(t *testing.T) {
	deadline, ticks := make(chan time.Time, 1), make(chan time.Time, 1)
	queries, started := make(chan uint32, 4), make(chan struct{})
	var shown, closed, ready atomic.Bool
	var stops atomic.Int32
	done := make(chan bool, 1)
	go func() {
		done <- runApplicationHandoff(handoffHooks{
			ShowStarting: func() { shown.Store(true) },
			Start: func() (uint32, error) {
				if !shown.Load() || closed.Load() {
					t.Error("installer hidden before launch")
				}
				close(started)
				return 42, nil
			},
			HasWindow:   func(pid uint32) bool { queries <- pid; return ready.Load() },
			LaunchError: func(error) { t.Error("unexpected launch error") },
			CloseInstaller: func(visible bool) {
				if !visible {
					t.Error("successful handoff reported as a timeout")
				}
				closed.Store(true)
			},
		}, func(timeout, interval time.Duration) (<-chan time.Time, <-chan time.Time, func()) {
			if timeout != 20*time.Second || interval != 100*time.Millisecond {
				t.Errorf("wrong handoff timing %v/%v", timeout, interval)
			}
			return deadline, ticks, func() { stops.Add(1) }
		})
	}()
	<-started
	if pid := <-queries; pid != 42 {
		t.Fatalf("checked wrong PID %d", pid)
	}
	if closed.Load() {
		t.Fatal("closed before a visible application window")
	}
	ready.Store(true)
	ticks <- time.Now()
	select {
	case success := <-done:
		if !success || !closed.Load() || stops.Load() != 1 {
			t.Fatal("handoff did not close exactly after success")
		}
	case <-time.After(time.Second):
		t.Fatal("visible application never released installer")
	}
}

func TestApplicationHandoffTimeoutAlsoBoundsBlockedOrFailedLaunch(t *testing.T) {
	for _, blocked := range []bool{false, true} {
		t.Run(map[bool]string{false: "failed launch", true: "blocked CreateProcess"}[blocked], func(t *testing.T) {
			deadline, ticks := make(chan time.Time, 1), make(chan time.Time)
			startEntered, startGate := make(chan struct{}), make(chan struct{})
			defer close(startGate)
			var closed atomic.Int32
			done := make(chan bool, 1)
			go func() {
				done <- runApplicationHandoff(handoffHooks{
					ShowStarting: func() {}, Start: func() (uint32, error) {
						close(startEntered)
						if blocked {
							<-startGate
						}
						return 0, errors.New("missing exe")
					},
					HasWindow: func(uint32) bool { t.Error("queried windows without a PID"); return false }, LaunchError: func(error) {},
					CloseInstaller: func(visible bool) {
						if visible {
							t.Error("timeout reported as a successful handoff")
						}
						closed.Add(1)
					},
				}, func(timeout, _ time.Duration) (<-chan time.Time, <-chan time.Time, func()) {
					if timeout != 20*time.Second {
						t.Errorf("timeout removed: %v", timeout)
					}
					return deadline, ticks, func() {}
				})
			}()
			<-startEntered
			if closed.Load() != 0 {
				t.Fatal("closed before launch/timeout")
			}
			deadline <- time.Now()
			select {
			case success := <-done:
				if success || closed.Load() != 1 {
					t.Fatal("timeout must only close installer once")
				}
			case <-time.After(250 * time.Millisecond):
				t.Fatal("20-second deadline no longer releases installer")
			}
		})
	}
}
