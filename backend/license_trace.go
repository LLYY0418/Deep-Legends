//go:build license

package main

import (
	"crypto/tls"
	"net/http/httptrace"
	"sync"
	"time"
)

// Local timings only. A nil phase did not run (for example on a reused
// connection). No address, hostname, URL, error text or request data is retained.
// TTFB is cumulative from Do; the other fields are phase durations. Incomplete
// phases report elapsed time until failure. Concurrent connect attempts use
// their outer time span, rather than double-counting overlapping dials.
type licenseNetworkTimings struct {
	mu      sync.Mutex
	started time.Time
	starts  map[string]time.Time
	ends    map[string]time.Time
	active  map[string]int
}

func newLicenseNetworkTimings() *licenseNetworkTimings {
	return &licenseNetworkTimings{started: time.Now(), starts: map[string]time.Time{}, ends: map[string]time.Time{}, active: map[string]int{}}
}
func (v *licenseNetworkTimings) begin(phase string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.active[phase]++
	delete(v.ends, phase)
	if v.starts[phase].IsZero() {
		v.starts[phase] = time.Now()
	}
}
func (v *licenseNetworkTimings) end(phase string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.active[phase] > 0 {
		v.active[phase]--
	}
	if v.active[phase] == 0 {
		v.ends[phase] = time.Now()
	}
}
func (v *licenseNetworkTimings) trace() *httptrace.ClientTrace {
	return &httptrace.ClientTrace{
		DNSStart: func(httptrace.DNSStartInfo) { v.begin("dns_ms") }, DNSDone: func(httptrace.DNSDoneInfo) { v.end("dns_ms") },
		ConnectStart: func(string, string) { v.begin("connect_ms") }, ConnectDone: func(string, string, error) { v.end("connect_ms") },
		TLSHandshakeStart: func() { v.begin("tls_ms") }, TLSHandshakeDone: func(tls.ConnectionState, error) { v.end("tls_ms") },
		GotFirstResponseByte: func() {
			v.mu.Lock()
			defer v.mu.Unlock()
			if v.ends["ttfb_ms"].IsZero() {
				v.starts["ttfb_ms"] = v.started
				v.ends["ttfb_ms"] = time.Now()
			}
		},
	}
}
func (v *licenseNetworkTimings) add(row map[string]any) {
	v.mu.Lock()
	defer v.mu.Unlock()
	for _, phase := range []string{"dns_ms", "connect_ms", "tls_ms", "ttfb_ms", "body_ms"} {
		row[phase] = nil
		start := v.starts[phase]
		if start.IsZero() {
			continue
		}
		end := v.ends[phase]
		if end.IsZero() {
			end = time.Now()
		}
		row[phase] = max(int64(0), end.Sub(start).Milliseconds())
	}
}
