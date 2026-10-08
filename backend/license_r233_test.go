//go:build license

package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

type licenseR233Events struct {
	mu   sync.Mutex
	rows []map[string]any
}

func (e *licenseR233Events) add(row map[string]any) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.rows = append(e.rows, row)
}
func (e *licenseR233Events) has(result, stage string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, row := range e.rows {
		if row["event"] == "license_store_write" && row["result"] == result && row["stage"] == stage {
			return true
		}
	}
	return false
}
func (e *licenseR233Events) renewals() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	count := 0
	for _, row := range e.rows {
		if row["event"] == "license_renew" && row["result"] == "ok" {
			count++
		}
	}
	return count
}
func r233Wait(t *testing.T, predicate func() bool) {
	t.Helper()
	end := time.Now().Add(3 * time.Second)
	for time.Now().Before(end) {
		if predicate() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("license maintenance did not make expected progress")
}
func r233Run(t *testing.T, m *licenseManager) {
	t.Helper()
	terminal := m.Snapshot().State == "REPLACED" || m.Snapshot().State == "REVOKED"
	initial := make(chan struct{}, 1)
	observe := m.options.Observe
	m.options.Observe = func(row map[string]any) {
		if observe != nil {
			observe(row)
		}
		if row["event"] == "license_renew" && row["result"] == "ok" {
			select {
			case initial <- struct{}{}:
			default:
			}
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); m.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Error("manager did not stop")
		}
	})
	if !terminal {
		select {
		case <-initial:
		case <-time.After(3 * time.Second):
			t.Fatal("initial heartbeat did not finish")
		}
	}
}
func r233Advance(m *licenseManager, clock *licenseTestClock, d time.Duration) {
	clock.Advance(d)
	select {
	case m.wake <- struct{}{}:
	default:
	}
}
func r233SaveCount(store *licenseTestStore) int {
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.saves
}

func TestR233SlowLocalClockCachedRestart(t *testing.T) {
	s := newLicenseTestIssuer(t)
	store := &licenseTestStore{}
	local := &licenseTestClock{now: s.clock.Elapsed(), wallShift: -60 * time.Second}
	m := s.Manager(store)
	m.options.Now, m.options.Elapsed = local.Now, local.Elapsed
	testActivate(t, m, false)
	if err := m.Renew(context.Background()); err != nil {
		t.Fatal(err)
	}
	s.offline.Store(true)
	restored := newLicenseManager(m.options)
	if !restored.Allowed() || restored.deadline.Sub(local.Elapsed()) > licenseLifetime {
		t.Fatal("60-second slow clock rejected or lengthened cached lease")
	}
	local.Advance(licenseLifetime)
	if restored.Allowed() {
		t.Fatal("cached restart exceeded the lease cap")
	}
}
func TestR233SmallRuntimeStepPreservesContextAndDeadline(t *testing.T) {
	s := newLicenseTestIssuer(t)
	m := s.Manager(&licenseTestStore{})
	testActivate(t, m, false)
	ctx, generation, _ := m.admission()
	s.clock.Advance(time.Minute)
	if err := m.Renew(context.Background()); err != nil {
		t.Fatal(err)
	}
	original := m.deadline
	s.clock.StepWall(-5 * time.Second)
	if !m.Allowed() || ctx.Err() != nil || !m.deadline.Equal(original) {
		t.Fatal("small correction canceled business or changed deadline")
	}
	_, current, _ := m.admission()
	if generation != current {
		t.Fatal("small correction changed admission generation")
	}
	s.clock.Advance(licenseLifetime)
	if m.Allowed() || ctx.Err() == nil {
		t.Fatal("wall rollback extended runtime past monotonic expiry")
	}
}
func TestR233LargeRuntimeStepLocksAndImmediatelyRenews(t *testing.T) {
	s := newLicenseTestIssuer(t)
	m := s.Manager(&licenseTestStore{})
	local := &licenseTestClock{now: s.clock.Elapsed()}
	m.options.Now, m.options.Elapsed = local.Now, local.Elapsed
	testActivate(t, m, false)
	r233Run(t, m)
	r233Wait(t, func() bool { return s.requests.Load() >= 2 })
	m.Snapshot()
	old, _, _ := m.admission()
	before := s.requests.Load()
	local.StepWall(-10 * time.Minute)
	if m.Allowed() || old.Err() == nil {
		t.Fatal("large rollback did not lock/cancel old business")
	}
	// Elapsed time stays still: the 60-second scheduled heartbeat cannot fire.
	r233Wait(t, func() bool { return s.requests.Load() > before && m.Allowed() })
	if old.Err() == nil {
		t.Fatal("renewal revived canceled context")
	}
}
func TestR233FailedCounterSaveDoesNotRefreshCachedLifetime(t *testing.T) {
	s := newLicenseTestIssuer(t)
	store := &licenseTestStore{}
	m := s.Manager(store)
	testActivate(t, m, false)
	elapsed := 10 * time.Minute
	s.clock.Advance(elapsed)
	s.offline.Store(true)
	_ = m.Renew(context.Background()) // counter persists a newer high-water mark
	restored := s.Manager(store)
	remaining := licenseLifetime - elapsed
	if !restored.Allowed() || restored.deadline.Sub(s.clock.Elapsed()) > remaining {
		t.Fatal("failed renew refreshed the cached lease anchor")
	}
	s.clock.Advance(remaining)
	if restored.Allowed() {
		t.Fatal("offline requests kept expired disk lease alive")
	}
}
func TestR233TransientCounterFailuresContinueHeartbeats(t *testing.T) {
	s := newLicenseTestIssuer(t)
	store := &licenseTestStore{}
	m := s.Manager(store)
	events := &licenseR233Events{}
	m.options.Observe = events.add
	testActivate(t, m, false)
	r233Run(t, m)
	r233Wait(t, func() bool { return s.requests.Load() >= 2 })
	m.Snapshot()
	ctx, _, _ := m.admission()
	count := s.requests.Load()
	store.mu.Lock()
	store.failNext = 2
	baseline := store.saves
	store.mu.Unlock()
	r233Advance(m, s.clock, time.Minute)
	r233Wait(t, func() bool { return r233SaveCount(store) >= baseline+1 })
	m.Snapshot()
	if !m.Allowed() || ctx.Err() != nil || s.requests.Load() != count || !events.has("failed", "counter") {
		t.Fatal("first persistence failure damaged active session or sent a request")
	}
	r233Advance(m, s.clock, 15*time.Second)
	r233Wait(t, func() bool { return r233SaveCount(store) >= baseline+2 })
	m.Snapshot()
	if !m.Allowed() || ctx.Err() != nil || s.requests.Load() != count {
		t.Fatal("second persistence failure damaged session")
	}
	r233Advance(m, s.clock, 30*time.Second)
	r233Wait(t, func() bool { return s.requests.Load() > count && events.has("recovered", "counter") })
	m.Snapshot()
	if !m.Allowed() || ctx.Err() != nil {
		t.Fatal("heartbeat recovery required reentry")
	}
}
func TestR233LeaseSaveFailureKeepsVerifiedMemoryLease(t *testing.T) {
	s := newLicenseTestIssuer(t)
	store := &licenseTestStore{}
	m := s.Manager(store)
	events := &licenseR233Events{}
	m.options.Observe = events.add
	testActivate(t, m, false)
	r233Run(t, m)
	r233Wait(t, func() bool { return s.requests.Load() >= 2 })
	m.Snapshot()
	ctx, _, _ := m.admission()
	oldDeadline := m.deadline
	count := s.requests.Load()
	renewals := events.renewals()
	store.mu.Lock()
	store.failAt = map[int]bool{store.saves + 2: true}
	store.mu.Unlock()
	r233Advance(m, s.clock, time.Minute)
	r233Wait(t, func() bool { return events.has("failed", "lease") })
	m.Snapshot()
	if !m.Allowed() || ctx.Err() != nil || !m.deadline.After(oldDeadline) || !events.has("failed", "lease") {
		t.Fatal("verified lease lost because save failed")
	}
	disk, _ := store.Load()
	if disk.LeaseWallTime == m.disk.LeaseWallTime {
		t.Fatal("fault injection did not leave older disk lease")
	}
	r233Advance(m, s.clock, time.Minute)
	r233Wait(t, func() bool {
		return s.requests.Load() >= count+2 && events.has("recovered", "counter") && events.renewals() >= renewals+2
	})
	m.Snapshot()
	disk, _ = store.Load()
	if disk.LeaseWallTime != m.disk.LeaseWallTime {
		t.Fatal("current lease not persisted on recovery")
	}
}
func TestR233PersistentSaveFailureExpiresThenAutoRecovers(t *testing.T) {
	s := newLicenseTestIssuer(t)
	store := &licenseTestStore{}
	m := s.Manager(store)
	events := &licenseR233Events{}
	m.options.Observe = events.add
	testActivate(t, m, false)
	r233Run(t, m)
	r233Wait(t, func() bool { return s.requests.Load() >= 2 })
	m.Snapshot()
	ctx, _, _ := m.admission()
	count := s.requests.Load()
	store.mu.Lock()
	store.fail = true
	baseline := store.saves
	store.mu.Unlock()
	r233Advance(m, s.clock, time.Minute)
	r233Wait(t, func() bool { return r233SaveCount(store) > baseline })
	m.Snapshot()
	if !m.Allowed() || ctx.Err() != nil || s.requests.Load() != count || !events.has("failed", "counter") {
		t.Fatal("failed counter was sent or session locked early")
	}
	r233Advance(m, s.clock, licenseLifetime)
	r233Wait(t, func() bool { return m.Snapshot().State == "NETWORK_LOCKED" })
	if ctx.Err() == nil || m.Snapshot().State == "DEVICE_ERROR" {
		t.Fatal("persistent failure has wrong terminal behavior")
	}
	store.mu.Lock()
	store.fail = false
	store.mu.Unlock()
	r233Advance(m, s.clock, time.Minute)
	r233Wait(t, func() bool { return m.Allowed() && s.requests.Load() > count && events.has("recovered", "counter") })
}
func TestR233TerminalSaveFailureRetriesWithoutRenew(t *testing.T) {
	for _, terminal := range []string{"REPLACED", "REVOKED"} {
		t.Run(terminal, func(t *testing.T) {
			s := newLicenseTestIssuer(t)
			store := &licenseTestStore{}
			m := s.Manager(store)
			events := &licenseR233Events{}
			m.options.Observe = events.add
			testActivate(t, m, false)
			old, _, _ := m.admission()
			if terminal == "REVOKED" {
				s.revoked.Store(true)
			} else {
				testActivate(t, s.Manager(&licenseTestStore{}), false)
			}
			store.mu.Lock()
			store.failAt = map[int]bool{store.saves + 2: true, store.saves + 3: true}
			store.mu.Unlock()
			if m.Renew(context.Background()) == nil || m.Snapshot().State != terminal || old.Err() == nil || !events.has("failed", "terminal") {
				t.Fatal("terminal persistence failure did not close gate immediately")
			}
			count := s.requests.Load()
			r233Run(t, m)
			r233Advance(m, s.clock, 15*time.Second)
			r233Wait(t, func() bool { return events.has("failed", "terminal_retry") })
			if s.requests.Load() != count || m.Allowed() || old.Err() == nil {
				t.Fatal("repeated terminal write failure revived business or sent renew")
			}
			r233Advance(m, s.clock, 30*time.Second)
			r233Wait(t, func() bool { return events.has("recovered", "terminal_retry") })
			if s.requests.Load() != count || m.Allowed() || s.Manager(store).Snapshot().State != terminal {
				t.Fatal("terminal retry renewed or failed to persist terminal state")
			}
		})
	}
}
func TestR233WriteRetriesAndCleansTemporaryFiles(t *testing.T) {
	for _, failures := range []int{1, 2, 4} {
		t.Run(time.Duration(failures).String(), func(t *testing.T) {
			root := t.TempDir()
			target := filepath.Join(root, "lease.json")
			attempts := 0
			var delays []time.Duration
			err := retryLicenseFileWrite(func() error {
				return writeLicenseFileOnce(target, []byte("protected fixture"), func(a, b string) error {
					attempts++
					if attempts <= failures {
						return errors.New("test transient rename")
					}
					return os.Rename(a, b)
				})
			}, func(d time.Duration) { delays = append(delays, d) })
			if (err != nil) != (failures == 4) || attempts != min(failures+1, 4) || len(delays) != attempts-1 {
				t.Fatal("retry budget mismatch", err, attempts, delays)
			}
			for i, d := range delays {
				if d != []time.Duration{50 * time.Millisecond, 200 * time.Millisecond, 800 * time.Millisecond}[i] {
					t.Fatal("wrong retry delay")
				}
			}
			files, _ := filepath.Glob(filepath.Join(root, ".license-*"))
			if len(files) > 0 {
				t.Fatal("failed writes left temporary files")
			}
			if err == nil {
				var saved []byte
				saved, _ = os.ReadFile(target)
				if string(saved) != "protected fixture" {
					t.Fatal("wrong persisted bytes")
				}
			}
		})
	}
	// Exercise the same retry wrapper for a write failure, not just Rename.
	attempts := 0
	if retryLicenseFileWrite(func() error { attempts++; return errors.New("test write error") }, func(time.Duration) {}) == nil || attempts != 4 {
		t.Fatal("write errors not bounded/retried")
	}
}
