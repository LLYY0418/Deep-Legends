//go:build license

package main

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

func TestR246LeaseBoundaries(t *testing.T) {
	issuer := newLicenseTestIssuer(t)
	m := issuer.Manager(&licenseTestStore{})
	testActivate(t, m, false)
	if m.lease.ExpiresAt-m.lease.IssuedAt != 7200 || licenseHeartbeat != time.Minute || licenseClockTolerance != 300*time.Second {
		t.Fatal("R246 lease/heartbeat/clock contract differs")
	}
	for _, seconds := range []int64{7200, 7201} {
		p := m.lease
		p.ExpiresAt = p.IssuedAt + seconds
		_, err := m.verify(testSignedEnvelope(issuer.key, "test-issuer", "DL-LICENSE-RESPONSE-V1", p), p.RequestID)
		if (err == nil) != (seconds == 7200) {
			t.Fatalf("signed %d-second lease acceptance: %v", seconds, err)
		}
	}
	p := m.lease
	p.LicenseExpiresAt = p.IssuedAt + 120
	p.ExpiresAt = p.LicenseExpiresAt
	if got, err := m.verify(testSignedEnvelope(issuer.key, "test-issuer", "DL-LICENSE-RESPONSE-V1", p), p.RequestID); err != nil || got.ExpiresAt != p.LicenseExpiresAt {
		t.Fatal("business expiry clipping changed", err)
	}
}

func TestR246ThirtyMinutesOfRenewTimeoutsRemainActive(t *testing.T) {
	issuer := newLicenseTestIssuer(t)
	m := issuer.Manager(&licenseTestStore{})
	testActivate(t, m, false)
	business, generation, _ := m.admission()
	deadline := m.deadline
	until := issuer.clock.Elapsed().Add(30 * time.Minute)
	var attempts int
	// The test clock accounts for each 15-second request timeout; no real 30-minute sleep.
	m.options.Client = &http.Client{Transport: updateRoundTrip(func(*http.Request) (*http.Response, error) {
		attempts++
		issuer.clock.Advance(min(licenseTimeout, until.Sub(issuer.clock.Elapsed())))
		return nil, context.DeadlineExceeded
	})}
	for issuer.clock.Elapsed().Before(until) {
		issuer.clock.Advance(min(max(time.Duration(0), m.nextRenew.Sub(issuer.clock.Elapsed())), until.Sub(issuer.clock.Elapsed())))
		if !issuer.clock.Elapsed().Before(until) {
			break
		}
		if err := m.Renew(context.Background()); err == nil {
			t.Fatal("simulated timeout succeeded")
		}
		current, currentGeneration, allowed := m.admission()
		if !allowed || m.Snapshot().State != "ACTIVE" || business.Err() != nil || current != business || currentGeneration != generation {
			t.Fatal("R246 30-minute outage must remain ACTIVE without cancelling business")
		}
		if !m.deadline.Equal(deadline) {
			t.Fatal("failed renewal extended lease")
		}
	}
	if attempts < 20 || !m.Allowed() || business.Err() != nil {
		t.Fatal("30-minute timeout simulation incomplete", attempts)
	}
}

func r246Wait(t *testing.T, predicate func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if predicate() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("automatic renew did not reach expected state")
}
func r246Run(t *testing.T, m *licenseManager) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan struct{})
	go func() { defer close(stopped); m.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-stopped:
		case <-time.After(time.Second):
			t.Error("license runner did not stop")
		}
	})
}
func r246WakeDue(m *licenseManager, clock *licenseTestClock) {
	m.mu.Lock()
	due := m.nextRenew
	m.mu.Unlock()
	clock.Advance(max(time.Duration(0), due.Sub(clock.Elapsed())))
	select {
	case m.wake <- struct{}{}:
	default:
	}
}

func TestR246OutagePastTwoHoursLocksAndAutomaticallyRecovers(t *testing.T) {
	issuer := newLicenseTestIssuer(t)
	m := issuer.Manager(&licenseTestStore{})
	testActivate(t, m, false)
	business, _, _ := m.admission()
	var offline atomic.Bool
	var timeouts atomic.Int64
	offline.Store(true)
	transport := issuer.client.Transport
	m.options.Client = &http.Client{Transport: updateRoundTrip(func(r *http.Request) (*http.Response, error) {
		if offline.Load() {
			timeouts.Add(1)
			return nil, context.DeadlineExceeded
		}
		return transport.RoundTrip(r)
	})}
	r246Run(t, m)
	r246Wait(t, func() bool { m.mu.Lock(); defer m.mu.Unlock(); return timeouts.Load() > 0 && m.failures > 0 })
	issuer.clock.Advance(2*time.Hour + time.Second)
	if m.Allowed() || m.Snapshot().State != "NETWORK_LOCKED" || business.Err() == nil {
		t.Fatal("outage past two hours failed to lock/cancel business")
	}
	offline.Store(false)
	r246WakeDue(m, issuer.clock)
	r246Wait(t, func() bool { return m.Allowed() })
	recovered, _, allowed := m.admission()
	m.mu.Lock()
	nextRenew := m.nextRenew
	m.mu.Unlock()
	if !allowed || recovered == business || recovered.Err() != nil || nextRenew.Sub(issuer.clock.Elapsed()) != time.Minute {
		t.Fatal("signed automatic recovery did not create a new usable business context")
	}
}

func TestR246FirstOnlineRenewAfterOfflineRevocationLocksImmediately(t *testing.T) {
	issuer := newLicenseTestIssuer(t)
	m := issuer.Manager(&licenseTestStore{})
	testActivate(t, m, false)
	business, _, _ := m.admission()
	var offline atomic.Bool
	offline.Store(true)
	transport := issuer.client.Transport
	m.options.Client = &http.Client{Transport: updateRoundTrip(func(r *http.Request) (*http.Response, error) {
		if offline.Load() {
			return nil, context.DeadlineExceeded
		}
		return transport.RoundTrip(r)
	})}
	r246Run(t, m)
	r246Wait(t, func() bool { m.mu.Lock(); defer m.mu.Unlock(); return m.failures > 0 })
	issuer.clock.Advance(30 * time.Minute)
	issuer.revoked.Store(true)
	if !m.Allowed() || business.Err() != nil {
		t.Fatal("offline revocation guessed without a signed response")
	}
	requests := issuer.requests.Load()
	offline.Store(false)
	r246WakeDue(m, issuer.clock)
	r246Wait(t, func() bool { return m.Snapshot().State == "REVOKED" })
	if m.Allowed() || business.Err() == nil || issuer.requests.Load() != requests+1 || m.Snapshot().Message != "注册码已停用" {
		t.Fatal("first signed REVOKED response did not immediately lock")
	}
	m.maintain(context.Background())
	if issuer.requests.Load() != requests+1 {
		t.Fatal("terminal revocation automatically renewed")
	}
}

func TestR246CachedRestartUsesSignedAndCheckpointMinimum(t *testing.T) {
	for _, row := range []struct {
		name                              string
		signed, checkpoint, elapsed, want int64
		legacy                            bool
	}{
		{"two-hour-cache-after-thirty-minutes", 7200, 7200, 1800, 5400, false},
		{"checkpoint-is-shorter", 7200, 3600, 1800, 1800, false},
		{"signed-expiry-is-shorter", 1200, 7200, 600, 600, false},
		{"oversized-checkpoint-cannot-exceed-cap", 7200, 9000, 0, 7200, false},
		{"legacy-cache", 7200, 0, 1800, 5400, true},
		{"expired-cache", 7200, 7200, 7201, 0, false},
	} {
		t.Run(row.name, func(t *testing.T) {
			issuer := newLicenseTestIssuer(t)
			store := &licenseTestStore{}
			m := issuer.Manager(store)
			testActivate(t, m, false)
			p := m.lease
			p.ExpiresAt = p.IssuedAt + row.signed
			envelope := testSignedEnvelope(issuer.key, "test-issuer", "DL-LICENSE-RESPONSE-V1", p)
			disk := m.disk
			disk.Lease = &envelope
			disk.LeaseRemaining = row.checkpoint
			if row.legacy {
				disk.LeaseWallTime = 0
			}
			if err := store.Save(disk); err != nil {
				t.Fatal(err)
			}
			issuer.clock.Advance(time.Duration(row.elapsed) * time.Second)
			restored := issuer.Manager(store)
			if row.want == 0 {
				if restored.Allowed() || restored.Snapshot().State != "NETWORK_LOCKED" {
					t.Fatal("expired cache restored without online renewal")
				}
			} else if !restored.Allowed() || restored.deadline.Sub(issuer.clock.Elapsed()) != time.Duration(row.want)*time.Second {
				t.Fatal("cached remaining time violates signed/checkpoint/cap minimum", restored.deadline.Sub(issuer.clock.Elapsed()))
			}
		})
	}
}
