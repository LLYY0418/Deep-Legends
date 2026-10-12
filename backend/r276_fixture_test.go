package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Count callback invocations atomically, then signal completed callbacks under
// a separate lock. A handler blocked by a gate has not completed a request.
type r276RequestCounter struct {
	calls     atomic.Int64
	mu        sync.Mutex
	completed int
	changed   chan struct{}
	waiting   chan int
}

func newR276RequestCounter() *r276RequestCounter {
	return &r276RequestCounter{changed: make(chan struct{}), waiting: make(chan int, 1)}
}

func (c *r276RequestCounter) handler(next func(*http.Request) (*http.Response, error)) func(*http.Request) (*http.Response, error) {
	return func(r *http.Request) (*http.Response, error) {
		response, err := next(r)
		c.calls.Add(1)
		c.mu.Lock()
		c.completed++
		close(c.changed)
		c.changed = make(chan struct{})
		c.mu.Unlock()
		return response, err
	}
}

func (c *r276RequestCounter) load() int64 { return c.calls.Load() }

func (c *r276RequestCounter) wait(ctx context.Context, count int, settled func() bool) error {
	timer := time.NewTimer(r274NoProgressLimit)
	defer timer.Stop()
	previous := 0
	for {
		c.mu.Lock()
		completed, changed := c.completed, c.changed
		c.mu.Unlock()
		if completed >= count && (settled == nil || settled()) {
			return nil
		}
		if completed > previous {
			previous = completed
			timer.Stop()
			timer.Reset(r274NoProgressLimit)
		}
		select {
		case c.waiting <- completed:
		default:
		}
		// After decoding the final response, the real worker publishes its last
		// account-cache update. Observe that protected state, without sleeping or
		// guessing a delay; its disk queue is drained by fixture cleanup.
		if completed >= count {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-timer.C:
				return fmt.Errorf("fixture refresh stalled after %d completed requests", completed)
			default:
				runtime.Gosched()
			}
			continue
		}
		select {
		case <-changed:
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			c.mu.Lock()
			latest := c.completed
			c.mu.Unlock()
			if latest > completed {
				continue
			}
			return fmt.Errorf("fixture requests stalled: completed=%d want=%d", completed, count)
		}
	}
}

func TestR276ParallelFixtureCounter(t *testing.T) {
	counter := newR276RequestCounter()
	started, release := make(chan struct{}, 2), make(chan struct{})
	handler := counter.handler(func(r *http.Request) (*http.Response, error) {
		started <- struct{}{}
		<-release
		return r102AccountResponse(r), nil
	})
	var workers sync.WaitGroup
	workers.Add(2)
	for i := 0; i < 2; i++ {
		go func() {
			defer workers.Done()
			req, _ := http.NewRequest("GET", "https://fixture.invalid/fixture/KR1", nil)
			response, _ := handler(req)
			response.Body.Close()
		}()
	}
	r274Wait(t, started, "first counter worker did not enter gate")
	r274Wait(t, started, "second counter worker did not enter gate")
	close(release)
	workers.Wait()
	if got := counter.load(); got != 2 {
		t.Fatalf("concurrent fixture requests = %d, want 2", got)
	}
}

func TestR276RefreshCompletionBarrierGated(t *testing.T) {
	seed, counter := r102FixtureSeed(2), newR276RequestCounter()
	root := t.TempDir()
	started := make(chan string, 2)
	gates := map[string]chan struct{}{}
	for _, ref := range seed.Accounts {
		gates[ref.GameName] = make(chan struct{})
	}
	var releaseOnce sync.Once
	release := func() {
		releaseOnce.Do(func() {
			for _, gate := range gates {
				close(gate)
			}
		})
	}
	refreshing := false
	handler := counter.handler(func(r *http.Request) (*http.Response, error) {
		name := path.Base(path.Dir(r.URL.Path))
		if !refreshing {
			return r102AccountResponse(r), nil
		}
		started <- name
		<-gates[name]
		body, _ := json.Marshal(riotAccount{PUUID: "id-" + name, GameName: name + "-refreshed", TagLine: "KR1"})
		return r99Response(string(body)), nil
	})
	p := r99SeedProvider(t, root, handler)
	for _, ref := range seed.Accounts {
		if _, err := p.accountByRiotID(context.Background(), ref.GameName, ref.TagLine); err != nil {
			t.Fatal(err)
		}
	}
	r274Flush(t, p.identityDisk)
	refreshing = true // Priming is synchronous; no refresh exists before restart.
	p = r99SeedProvider(t, root, handler)
	settled := func() bool {
		p.accountMu.Lock()
		defer p.accountMu.Unlock()
		for _, ref := range seed.Accounts {
			found := false
			for _, entry := range p.accountCache {
				if entry.account.PUUID == "id-"+ref.GameName && entry.account.GameName == ref.GameName+"-refreshed" {
					found = true
				}
			}
			if !found {
				return false
			}
		}
		return true // The final shared-state update of both real refresh workers.
	}
	t.Cleanup(func() {
		release()
		if err := counter.wait(context.Background(), 4, settled); err != nil {
			t.Error(err)
		}
		r274Flush(t, p.identityDisk)
	})
	for i, ref := range seed.Accounts {
		account, err := p.resolveProSeed(context.Background(), seed, i)
		if err != nil || account.PUUID != "id-"+ref.GameName {
			t.Fatalf("foreground disk identity %d: %+v %v", i, account, err)
		}
	}
	for range seed.Accounts {
		r274Wait(t, started, "background refresh did not enter controlled gate")
	}
	result := make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		if err := counter.wait(ctx, 4, settled); err != nil {
			result <- err
			return
		}
		if got := counter.load(); got != 4 {
			result <- fmt.Errorf("read before refresh completion: calls=%d, want 4", got)
			return
		}
		result <- nil
	}()
	select {
	case count := <-counter.waiting:
		if count != 2 {
			t.Fatalf("blocked fixture completed %d requests, want 2", count)
		}
	case err := <-result:
		t.Fatalf("completion barrier returned while both refresh gates were closed: %v", err)
	case <-time.After(r274NoProgressLimit):
		t.Fatal("completion waiter did not observe the controlled pending work")
	}
	if got := counter.load(); got != 2 {
		t.Fatalf("closed gates allowed request completion: %d", got)
	}
	release()
	if err := r274Wait(t, result, "background completion barrier stalled"); err != nil {
		t.Fatal(err)
	}
	if !settled() {
		t.Fatal("barrier returned before real background cache updates")
	}
}
