//go:build license

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Re-sign the local issuer's response with its test-generated in-memory key.
// No external service, real code or private-key file is involved.
func r242Issuer(t *testing.T) (*licenseTestIssuer, *atomic.Int64, *atomic.Bool) {
	t.Helper()
	s := newLicenseTestIssuer(t)
	expires, expired := &atomic.Int64{}, &atomic.Bool{}
	original := s.client.Transport
	s.client.Transport = updateRoundTrip(func(r *http.Request) (*http.Response, error) {
		response, err := original.RoundTrip(r)
		if err != nil {
			return response, err
		}
		defer response.Body.Close()
		var envelope signedLicenseEnvelope
		if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
			return nil, err
		}
		data, _ := rawURLDecode(envelope.Payload, -1)
		var p licensePayload
		if err := json.Unmarshal(data, &p); err != nil {
			return nil, err
		}
		if expired.Load() {
			p.Status, p.Error = "ERROR", "EXPIRED"
		} else if p.Status == "ACTIVE" && expires.Load() > 0 {
			p.LicenseExpiresAt = expires.Load()
			p.ExpiresAt = min(p.ExpiresAt, p.LicenseExpiresAt)
		}
		w := httptest.NewRecorder()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(testSignedEnvelope(s.key, "test-issuer", "DL-LICENSE-RESPONSE-V1", p))
		return w.Result(), nil
	})
	return s, expires, expired
}

func TestR242ExpiredTerminalPersistsUntilExplicitReentry(t *testing.T) {
	for _, stage := range []string{"activate", "renew", "terminal-write-retry"} {
		t.Run(stage, func(t *testing.T) {
			s, _, expired := r242Issuer(t)
			store := &licenseTestStore{}
			m := s.Manager(store)
			if stage != "activate" {
				testActivate(t, m, false)
			}
			business, _, _ := m.admission()
			if stage == "terminal-write-retry" {
				store.failAt = map[int]bool{store.saves + 2: true}
			}
			expired.Store(true)
			var err error
			if stage == "activate" {
				err = m.Activate(context.Background(), strings.Repeat("0", 20))
			} else {
				err = m.Renew(context.Background())
			}
			if err == nil || err.Error() != "EXPIRED" || m.Allowed() || business.Err() == nil || m.Snapshot().State != "REVOKED" || m.Snapshot().Message != "注册码已过期" || m.disk.Lease != nil {
				t.Fatal("EXPIRED did not terminate and cancel business", err, m.Snapshot())
			}
			count := s.requests.Load()
			expired.Store(false) // Server has been extended; network alone cannot revive it.
			s.clock.Advance(time.Minute)
			m.maintain(context.Background())
			if s.requests.Load() != count || m.Allowed() || m.storeDirty {
				t.Fatal("terminal renewal or persistence retry is incorrect")
			}
			disk, _ := store.Load()
			if disk.Terminal != "REVOKED" || disk.TerminalReason != "EXPIRED" || disk.Lease != nil {
				t.Fatal("expiry reason was not persisted")
			}
			restored := s.Manager(store)
			restored.maintain(context.Background())
			_ = restored.Renew(context.Background())
			if restored.Snapshot().Message != "注册码已过期" || restored.Allowed() || s.requests.Load() != count {
				t.Fatal("restart lost reason or automatically recovered", restored.Snapshot())
			}
			testActivate(t, restored, false)
			if restored.disk.TerminalReason != "" || restored.disk.Terminal != "" {
				t.Fatal("successful explicit reentry retained terminal reason")
			}
			s.revoked.Store(true)
			_ = restored.Renew(context.Background())
			if s.Manager(store).Snapshot().Message != "注册码已停用" {
				t.Fatal("subsequent revocation incorrectly retained expiry reason")
			}
		})
	}
}

func TestR242ExpiredActivationHTTPAndLegacyRevoked(t *testing.T) {
	s, _, expired := r242Issuer(t)
	store := &licenseTestStore{}
	m := s.Manager(store)
	expired.Store(true)
	w := httptest.NewRecorder()
	(&app{license: m}).handleLicenseActivate(w, httptest.NewRequest("POST", "/api/license/activate", strings.NewReader(`{"code":"00000000000000000000"}`)))
	if w.Code != http.StatusForbidden || strings.TrimSpace(w.Body.String()) != "注册码已过期" {
		t.Fatal("expiry activation HTTP message differs", w.Code, w.Body.String())
	}
	disk, _ := store.Load()
	disk.TerminalReason = ""
	_ = store.Save(disk)
	if s.Manager(store).Snapshot().Message != "注册码已停用" {
		t.Fatal("legacy REVOKED cache changed meaning")
	}
}

func TestR242LicenseExpiryStrictSignedField(t *testing.T) {
	s := newLicenseTestIssuer(t)
	m := s.Manager(&licenseTestStore{})
	testActivate(t, m, false)
	data, _ := rawURLDecode(m.disk.Lease.Payload, -1)
	var base map[string]any
	_ = json.Unmarshal(data, &base)
	issued := m.lease.IssuedAt
	for _, row := range []struct {
		name  string
		value any
	}{
		{"zero", int64(0)}, {"negative", int64(-1)}, {"null", nil},
		{"before-issued", issued - 1}, {"string", "1791244800"},
		{"fraction", float64(issued) + 0.5}, {"exponent", json.Number("1e12")},
		{"overflow", json.Number("9223372036854775808")}, {"boolean", true},
		{"lease-after-expiry", issued + 20},
	} {
		t.Run(row.name, func(t *testing.T) {
			base["license_expires_at"] = row.value
			if _, err := m.verify(testSignedEnvelope(s.key, "test-issuer", "DL-LICENSE-RESPONSE-V1", base), ""); err == nil {
				t.Fatal("invalid signed optional expiry accepted", row.name)
			}
		})
	}
	base["license_expires_at"] = issued + 900
	if p, err := m.verify(testSignedEnvelope(s.key, "test-issuer", "DL-LICENSE-RESPONSE-V1", base), ""); err != nil || p.LicenseExpiresAt != issued+900 {
		t.Fatal("valid signed expiry rejected", err)
	}
	delete(base, "license_expires_at")
	if p, err := m.verify(testSignedEnvelope(s.key, "test-issuer", "DL-LICENSE-RESPONSE-V1", base), ""); err != nil || p.LicenseExpiresAt != 0 {
		t.Fatal("permanent legacy lease rejected", err)
	}
}

func TestR242ShortLeaseRenewalAndRestart(t *testing.T) {
	s, expires, _ := r242Issuer(t)
	store := &licenseTestStore{}
	m := s.Manager(store)
	testActivate(t, m, false)
	expires.Store(s.clock.Now().Unix() + 40)
	if err := m.Renew(context.Background()); err != nil || m.deadline.Sub(s.clock.Elapsed()) != 40*time.Second {
		t.Fatal("signed renewal did not shorten the previous lease", err)
	}
	w := httptest.NewRecorder()
	(&app{license: m}).handleLicenseStatus(w, httptest.NewRequest("GET", "/api/license/status", nil))
	var snapshot licenseSnapshot
	_ = json.Unmarshal(w.Body.Bytes(), &snapshot)
	if snapshot.LicenseExpiresAt != expires.Load() || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("status does not expose signed business expiry", w.Body.String())
	}
	s.clock.Advance(25 * time.Second)
	restored := s.Manager(store)
	if !restored.Allowed() || restored.deadline.Sub(s.clock.Elapsed()) != 15*time.Second || restored.Snapshot().LicenseExpiresAt != expires.Load() {
		t.Fatal("short cached lease restarted at the full lease cap")
	}
	business, _, _ := restored.admission()
	s.clock.Advance(15 * time.Second)
	if restored.Allowed() || business.Err() == nil || m.Allowed() {
		t.Fatal("short lease boundary did not lock/cancel business")
	}
	store = &licenseTestStore{}
	expires.Store(s.clock.Now().Unix() + 12)
	fresh := s.Manager(store)
	testActivate(t, fresh, false)
	if fresh.deadline.Sub(s.clock.Elapsed()) != 12*time.Second {
		t.Fatal("short activation lease was extended")
	}
	s.clock.Advance(12 * time.Second)
	if fresh.Allowed() {
		t.Fatal("short activation did not expire")
	}
}
