package main

import "time"

const applicationWindowTimeout = 20 * time.Second
const applicationWindowPoll = 100 * time.Millisecond

type applicationWindow struct {
	PID           uint32
	Visible       bool
	Width, Height int32
}

func matchesApplicationWindow(pid uint32, window applicationWindow) bool {
	return pid != 0 && window.PID == pid && window.Visible && window.Width >= 160 && window.Height >= 100
}

type handoffHooks struct {
	ShowStarting   func()
	Start          func() (uint32, error)
	HasWindow      func(uint32) bool
	LaunchError    func(error)
	CloseInstaller func(bool)
}

type handoffClock func(time.Duration, time.Duration) (<-chan time.Time, <-chan time.Time, func())

func realHandoffClock(timeout, interval time.Duration) (<-chan time.Time, <-chan time.Time, func()) {
	deadline, ticker := time.NewTimer(timeout), time.NewTicker(interval)
	return deadline.C, ticker.C, func() { deadline.Stop(); ticker.Stop() }
}

// The installer remains visible throughout this function. Even CreateProcess
// can block on a cold Windows launch, so it must not own the deadline or UI thread.
func runApplicationHandoff(h handoffHooks, clock handoffClock) (visible bool) {
	h.ShowStarting()
	defer func() { h.CloseInstaller(visible) }()
	deadline, ticks, stop := clock(applicationWindowTimeout, applicationWindowPoll)
	defer stop()
	type launchResult struct {
		pid uint32
		err error
	}
	launched := make(chan launchResult, 1)
	go func() { pid, err := h.Start(); launched <- launchResult{pid, err} }()
	var pid uint32
	for {
		select {
		case <-deadline:
			return false
		case result := <-launched:
			pid = result.pid
			if result.err != nil {
				h.LaunchError(result.err)
			}
			if pid != 0 && h.HasWindow(pid) {
				return true
			}
		case <-ticks:
			if pid != 0 && h.HasWindow(pid) {
				return true
			}
		}
	}
}
