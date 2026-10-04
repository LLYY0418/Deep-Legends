package main

import "context"

type liveProgressListenerKey struct{}
type liveProgressPublisherKey struct{}
type liveProgressListener struct{ deliver func(gameplayLiveResponse) }

func publishLiveProgress(ctx context.Context, progress gameplayLiveResponse) {
	if ctx.Err() != nil {
		return
	}
	if publish, ok := ctx.Value(liveProgressPublisherKey{}).(func(gameplayLiveResponse)); ok {
		publish(progress)
	}
}

func (f *liveSnapshotFlight) listen(ctx context.Context) func() {
	deliver, ok := ctx.Value(liveProgressListenerKey{}).(func(gameplayLiveResponse))
	if !ok {
		return func() {}
	}
	listener := &liveProgressListener{deliver: func(value gameplayLiveResponse) {
		if ctx.Err() == nil {
			deliver(value)
		}
	}}
	f.progressMu.Lock()
	if f.listeners == nil {
		f.listeners = map[*liveProgressListener]struct{}{}
	}
	f.listeners[listener] = struct{}{}
	if f.progress != nil {
		listener.deliver(*f.progress)
	}
	f.progressMu.Unlock()
	return func() { f.progressMu.Lock(); delete(f.listeners, listener); f.progressMu.Unlock() }
}
func (f *liveSnapshotFlight) publish(value gameplayLiveResponse) {
	// Each snapshot owns its roster slice; later player completions cannot race
	// with replay or serialization. Only registered anonymous refs enter it.
	value.Players = append([]gameplayLivePlayer(nil), value.Players...)
	f.progressMu.Lock()
	defer f.progressMu.Unlock()
	f.progress = &value
	for listener := range f.listeners {
		listener.deliver(value)
	}
}
