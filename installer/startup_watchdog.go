package main

import "sync/atomic"

// The WebView factory pumps its own UI loop before the document Session exists.
// Keep that earlier stage bounded too; only one side may finish startup.
type startupWatchdog struct {
	state  atomic.Int32 // 0 pending, 1 completed, 2 expired
	cancel func() bool
	done   chan struct{}
}

func newStartupWatchdog(after func(func()) func() bool, expired func()) *startupWatchdog {
	w := &startupWatchdog{done: make(chan struct{})}
	w.cancel = after(func() {
		if w.state.CompareAndSwap(0, 2) {
			defer close(w.done)
			expired()
		}
	})
	return w
}

func (w *startupWatchdog) stop() bool {
	if w.state.CompareAndSwap(0, 1) {
		w.cancel()
		close(w.done)
		return true
	}
	return w.state.Load() == 1
}
