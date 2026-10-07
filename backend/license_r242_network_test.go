//go:build license

package main

import (
	"net/http"
	"testing"
	"time"
)

func TestR242ShortLeaseConsumesNetworkTime(t *testing.T) {
	s, expires, _ := r242Issuer(t)
	expires.Store(s.clock.Now().Unix() + 10)
	original := s.client.Transport
	s.client.Transport = updateRoundTrip(func(r *http.Request) (*http.Response, error) {
		response, err := original.RoundTrip(r)
		s.clock.Advance(4 * time.Second)
		return response, err
	})
	m := s.Manager(&licenseTestStore{})
	testActivate(t, m, false)
	if m.deadline.Sub(s.clock.Elapsed()) != 6*time.Second || m.disk.LeaseRemaining != 6 {
		t.Fatal("network time was added to the short lease")
	}
	s.clock.Advance(6 * time.Second)
	if m.Allowed() {
		t.Fatal("short lease outlived signed expiry after network delay")
	}
}
