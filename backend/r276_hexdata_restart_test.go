package main

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

// Restart fixtures model a completed shutdown, not a crash during pending I/O.
// Producers must finish before draining both independent persistence owners.
func r276HexdataRestartDrain(p *championProvider, clock r274Clock) error {
	p.hexdata.prefetchWait.Wait()
	p.hexdata.pruneWait.Wait()
	p.mayhemWarmWait.Wait()
	for _, cache := range []*championDataCache{p.cache, p.imageCache, p.communityImageCache, p.hexdata.stateDisk} {
		if err := r274Drain(context.Background(), cache, clock); err != nil {
			return err
		}
	}
	return nil
}

func r276HexdataRestartBarrier(t *testing.T, p *championProvider) {
	t.Helper()
	if err := r276HexdataRestartDrain(p, r274RealClock{}); err != nil {
		t.Fatal("Hexdata restart durability barrier", err)
	}
}

type r276ReleaseStateClock struct {
	r274RealClock
	gate *r274DiskGate
}

func (c r276ReleaseStateClock) Observe(state r274QueueState) {
	if state.running != 0 {
		c.gate.unblock()
	}
}

func r276HexdataRestartFixture(t *testing.T) (*championProvider, *r274DiskGate, *atomic.Int32, http.RoundTripper, string) {
	t.Helper()
	root := t.TempDir()
	data := hexdataTestPage(`<p>restart persistence fixture</p>`)
	requests := &atomic.Int32{}
	transport := championRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		requests.Add(1)
		return hexdataResponse(r, data), nil
	})
	p := newHexdataBudgetProvider(t, root, transport)
	gate := r274Gate(t, p.hexdata.stateDisk, "hexdata-state")
	p.hexdata.adoptCitation(championSourceCitation{Source: "Hexdata", Patch: "16.16", ReportDate: "2026-08-19", BuildID: hexdataTestBuild, CanonicalURL: "https://hexdata.com.cn/hero/67-vayne"})
	r274Wait(t, gate.entered, "state write gate not reached", p.hexdata.stateDisk)
	if _, err := p.hexdata.load(context.Background(), "hero", "67", "/hero/67-vayne", "text/html,application/xhtml+xml", false); err != nil {
		t.Fatal(err)
	}
	p.hexdata.pruneWait.Wait()
	r274Flush(t, p.cache)
	return p, gate, requests, transport, root
}

func TestR276HexdataCacheOnlyRestartDeterministicallyRefetches(t *testing.T) {
	p, gate, requests, transport, root := r276HexdataRestartFixture(t)
	if _, err := newHexdataBudgetProvider(t, root, transport).hexdata.load(context.Background(), "hero", "67", "/hero/67-vayne", "text/html,application/xhtml+xml", false); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 2 {
		t.Fatalf("legacy cache-only barrier did not reproduce exactly one refetch: %d", requests.Load())
	}
	gate.unblock()
	r276HexdataRestartBarrier(t, p)
}

func TestR276HexdataRestartWaitsForStateAndUsesDisk(t *testing.T) {
	p, gate, requests, transport, root := r276HexdataRestartFixture(t)
	clock := &r274FakeClock{observed: make(chan r274QueueState), closed: make(chan struct{})}
	defer close(clock.closed)
	done := make(chan error, 1)
	go func() { done <- r276HexdataRestartDrain(p, clock) }()
	select {
	case err := <-done:
		t.Fatal("restart barrier returned while state writer gate was closed", err)
	case state := <-clock.observed:
		if state.running != 1 {
			t.Fatal("restart did not observe blocked state owner", state)
		}
	case <-time.After(r274NoProgressLimit):
		t.Fatal("restart barrier never reached state owner")
	}
	gate.unblock()
	// Unblock observational callbacks; no elapsed-time sleep controls this test.
	go func() {
		for {
			select {
			case <-clock.observed:
			case <-clock.closed:
				return
			}
		}
	}()
	if err := r274Wait(t, done, "restart barrier did not complete", p.hexdata.stateDisk); err != nil {
		t.Fatal(err)
	}
	if _, err := newHexdataBudgetProvider(t, root, transport).hexdata.load(context.Background(), "hero", "67", "/hero/67-vayne", "text/html,application/xhtml+xml", false); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 1 {
		t.Fatalf("complete restart barrier refetched hero: %d", requests.Load())
	}
}
