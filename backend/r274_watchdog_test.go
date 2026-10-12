package main

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

const r274NoProgressLimit = 15 * time.Second

type r274QueueState struct {
	completed uint64
	pending   int
	running   int
	changed   []<-chan struct{}
}

func r274QueueSnapshot(caches []*championDataCache) r274QueueState {
	var state r274QueueState
	seen := map[*championDataCache]bool{}
	for _, c := range caches {
		if c == nil || seen[c] {
			continue
		}
		seen[c] = true
		q := &c.diskQueue
		q.mu.Lock()
		if q.progress == nil {
			q.progress = make(chan struct{})
		}
		state.completed += q.completed
		state.pending += len(q.tasks)
		if q.running {
			state.running++
			state.changed = append(state.changed, q.progress, q.idle)
		}
		q.mu.Unlock()
	}
	return state
}

type r274Timer interface {
	Done() <-chan time.Time
	Stop()
}
type r274Clock interface {
	NewTimer(time.Duration) r274Timer
	Observe(r274QueueState)
}
type r274RealClock struct{}
type r274RealTimer struct{ *time.Timer }

func (r274RealClock) NewTimer(d time.Duration) r274Timer { return r274RealTimer{time.NewTimer(d)} }
func (r274RealClock) Observe(r274QueueState)             {}
func (t r274RealTimer) Done() <-chan time.Time           { return t.C }
func (t r274RealTimer) Stop()                            { t.Timer.Stop() }

// A completion (including a failed/skipped task that lets the worker advance)
// renews the inactivity window. Enqueueing, touching metadata, and merely
// starting an I/O do not count as progress. There is no total drain deadline.
func r274Await[T any](ctx context.Context, done <-chan T, caches []*championDataCache, clock r274Clock) (T, error) {
	var zero T
	completed := r274QueueSnapshot(caches).completed
	initialCompleted, renewals := completed, 0
	resetOnProgress := true
	timer := clock.NewTimer(r274NoProgressLimit)
	defer func() { timer.Stop() }()
	for {
		state := r274QueueSnapshot(caches)
		if state.completed > completed {
			completed = state.completed
			if resetOnProgress {
				renewals++
				timer.Stop()
				timer = clock.NewTimer(r274NoProgressLimit)
			}
		}
		clock.Observe(state)
		cases := []reflect.SelectCase{
			{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(done)},
			{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(timer.Done())},
			{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(ctx.Done())},
		}
		for _, changed := range state.changed {
			cases = append(cases, reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(changed)})
		}
		chosen, value, ok := reflect.Select(cases)
		switch chosen {
		case 0:
			if !ok || value.Interface() == nil {
				return zero, nil
			}
			return value.Interface().(T), nil
		case 1:
			// Prefer a completion/idle that raced the watchdog timer.
			select {
			case value, ok := <-done:
				if !ok {
					return zero, nil
				}
				return value, nil
			default:
			}
			latest := r274QueueSnapshot(caches)
			if resetOnProgress && latest.completed > completed {
				continue
			}
			return zero, fmt.Errorf("disk queue stalled: no task completed for %s; completed=%d initial_completed=%d renewals=%d pending=%d running=%d", r274NoProgressLimit, latest.completed, initialCompleted, renewals, latest.pending, latest.running)
		case 2:
			return zero, ctx.Err()
		}
	}
}

func r274Drain(ctx context.Context, c *championDataCache, clock r274Clock) error {
	if c == nil {
		return nil
	}
	c.diskQueue.mu.Lock()
	idle, running := c.diskQueue.idle, c.diskQueue.running
	c.diskQueue.mu.Unlock()
	if !running {
		return nil
	}
	_, err := r274Await(ctx, idle, []*championDataCache{c}, clock)
	return err
}

// Test fixture cleanup observes initialized owners only. The production
// shutdown still supplies its original shared two-second context.
func r274CloseApp(t *testing.T, a *app, ctx context.Context) {
	t.Helper()
	var caches []*championDataCache
	if a.storage != nil {
		a.storage.mu.Lock()
		caches = append(caches, a.storage.auxiliaryDisk)
		a.storage.mu.Unlock()
	}
	a.mu.Lock()
	caches = append(caches, a.matchTierCache, a.profileIconImages)
	a.mu.Unlock()
	a.perkCatalogMu.Lock()
	caches = append(caches, a.perkCatalogDisk, a.clientCatalogCache)
	a.perkCatalogMu.Unlock()
	a.proProfiles.mu.Lock()
	caches = append(caches, a.proProfiles.disk)
	a.proProfiles.mu.Unlock()
	a.proPlayers.mu.Lock()
	caches = append(caches, a.proPlayers.disk)
	a.proPlayers.mu.Unlock()
	if a.champions != nil {
		caches = append(caches, a.champions.cache, a.champions.imageCache, a.champions.communityImageCache)
	}
	if a.mayhemRatings != nil {
		caches = append(caches, a.mayhemRatings.disk)
	}
	if a.riot != nil {
		providers, unlock := a.riot.lockCacheProviders()
		for _, p := range providers {
			caches = append(caches, p.matchDisk, p.identityDisk)
			p.specialistMu.Lock()
			caches = append(caches, p.starterCache)
			p.specialistMu.Unlock()
		}
		unlock()
	}
	seen := map[*championDataCache]bool{}
	for _, cache := range caches {
		if cache == nil || seen[cache] {
			continue
		}
		seen[cache] = true
		done := make(chan error, 1)
		go func() { done <- cache.closeDisk(ctx) }()
		if err := r274Wait(t, done, "fixture cache cleanup stalled", cache); err != nil {
			t.Fatal("fixture cache cleanup", err)
		}
	}
}

type r274FakeTimer struct {
	clock   *r274FakeClock
	at      time.Duration
	stopped bool
	fired   bool
	done    chan time.Time
}
type r274FakeClock struct {
	mu       sync.Mutex
	now      time.Duration
	timers   []*r274FakeTimer
	observed chan r274QueueState
	closed   chan struct{}
}

func (c *r274FakeClock) NewTimer(d time.Duration) r274Timer {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := &r274FakeTimer{clock: c, at: c.now + d, done: make(chan time.Time, 1)}
	c.timers = append(c.timers, t)
	return t
}
func (c *r274FakeClock) Observe(s r274QueueState) {
	select {
	case c.observed <- s:
	case <-c.closed:
	}
}
func (t *r274FakeTimer) Done() <-chan time.Time { return t.done }
func (t *r274FakeTimer) Stop() {
	t.clock.mu.Lock()
	t.stopped = true
	t.clock.mu.Unlock()
}
func (c *r274FakeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now += d
	for _, t := range c.timers {
		if !t.stopped && !t.fired && t.at <= c.now {
			t.fired = true
			t.done <- time.Unix(0, int64(c.now))
		}
	}
}

func r274WatchdogFixture(t *testing.T, count int) (*championDataCache, *r274FakeClock, []func(), <-chan error) {
	t.Helper()
	c := &championDataCache{}
	clock := &r274FakeClock{observed: make(chan r274QueueState), closed: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	var release []func()
	for i := 0; i < count; i++ {
		gate := make(chan struct{})
		var once sync.Once
		release = append(release, func() { once.Do(func() { close(gate) }) })
		key := fmt.Sprint(i)
		if !c.diskQueue.submit(cacheDiskTask{id: key, ticket: c.diskQueue.ticket(key), run: func(func() bool) { <-gate }}, false) {
			t.Fatal("synthetic write not queued")
		}
	}
	result := make(chan error, 1)
	go func() { result <- r274Drain(ctx, c, clock) }()
	t.Cleanup(func() {
		cancel()
		close(clock.closed)
		for _, open := range release {
			open()
		}
		r274Flush(t, c)
	})
	return c, clock, release, result
}

func TestR274ProgressWatchdogAllowsSlowDrain(t *testing.T) {
	c, clock, release, result := r274WatchdogFixture(t, 8)
	r274Wait(t, clock.observed, "watchdog did not start")
	// Thirty-two virtual seconds, with a real queue completion every four:
	// exceeds both old total limits, never exceeds the inactivity window.
	for i, open := range release {
		clock.advance(4 * time.Second)
		open()
		if i == len(release)-1 {
			break
		}
		for {
			select {
			case err := <-result:
				t.Fatal("slow progressing queue failed to drain", err)
			case state := <-clock.observed:
				if state.completed == uint64(i+1) {
					goto advanced
				}
			}
		}
	advanced:
	}
	for {
		select {
		case err := <-result:
			if err != nil {
				t.Fatal("slow progressing queue failed to drain", err)
			}
			if state := r274QueueSnapshot([]*championDataCache{c}); state.completed != 8 || state.running != 0 || state.pending != 0 {
				t.Fatal("drain returned before all writes completed", state)
			}
			return
		case <-clock.observed:
		}
	}
}

func TestR274ProgressWatchdogRejectsStalledDrain(t *testing.T) {
	_, clock, _, result := r274WatchdogFixture(t, 3)
	r274Wait(t, clock.observed, "watchdog did not start")
	clock.advance(r274NoProgressLimit)
	select {
	case err := <-result:
		if err == nil || !strings.Contains(err.Error(), "no task completed for 15s") || !strings.Contains(err.Error(), "completed=0") {
			t.Fatal("stalled queue did not fail with progress evidence", err)
		}
	case <-clock.observed:
		t.Fatal("stalled queue did not fail when watchdog expired")
	}
}
