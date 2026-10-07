//go:build license

package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"
)

// Local diagnostics only: never retain error text, bodies or device identities.
type licenseExchangeFailure struct {
	retryable bool
	cause     error
}

func (e *licenseExchangeFailure) Error() string { return e.cause.Error() }
func (e *licenseExchangeFailure) Unwrap() error { return e.cause }
func retryLicenseActivation(err error) bool {
	var failure *licenseExchangeFailure
	return errors.As(err, &failure) && failure.retryable
}
func licenseNetworkFailure(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	var dns *net.DNSError
	if errors.As(err, &dns) {
		return "dns"
	}
	var certificate *tls.CertificateVerificationError
	var unknown x509.UnknownAuthorityError
	var invalid x509.CertificateInvalidError
	var host x509.HostnameError
	var record tls.RecordHeaderError
	if errors.As(err, &certificate) || errors.As(err, &unknown) || errors.As(err, &invalid) || errors.As(err, &host) || errors.As(err, &record) {
		return "tls"
	}
	var network net.Error
	if errors.As(err, &network) && network.Timeout() {
		return "timeout"
	}
	return "connect"
}
func licenseUnsignedError(data []byte) (string, bool) {
	var value map[string]json.RawMessage
	if json.Unmarshal(data, &value) != nil {
		return "", false
	}
	raw, present := value["error"]
	if !present {
		return "", false
	}
	var code string
	if json.Unmarshal(raw, &code) != nil {
		return "other", true
	}
	switch code {
	case "INVALID_CODE", "REPLACED", "REVOKED", "EXPIRED", "REPLAY", "TIMESTAMP_INVALID", "RATE_LIMITED", "SERVICE_UNAVAILABLE", "UNSUPPORTED_VERSION":
		return code, true
	}
	return "other", true
}
func licenseCFRay(value string) string {
	parts := strings.Split(value, "-")
	if len(parts) > 2 || len(parts[0]) == 0 || len(parts[0]) > 32 {
		return ""
	}
	for _, c := range parts[0] {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F') {
			return ""
		}
	}
	if len(parts) == 2 {
		if len(parts[1]) != 3 {
			return ""
		}
		for _, c := range parts[1] {
			if c < 'A' || c > 'Z' {
				return ""
			}
		}
	}
	return value
}

func (m *licenseManager) observeExchangeFailure(kind, failure, reason string, status int, serverError, ray string, started time.Time, timings *licenseNetworkTimings) {
	if m.options.Observe == nil {
		return
	}
	row := map[string]any{"event": "license_renew", "result": "failed", "reason": reason, "kind": kind, "failure": failure, "elapsed_ms": max(int64(0), m.options.Elapsed().Sub(started).Milliseconds())}
	if status > 0 {
		row["http_status"] = status
	}
	if serverError != "" {
		row["server_error"] = serverError
	}
	if ray != "" {
		row["cf_ray"] = ray
	}
	timings.add(row)
	m.options.Observe(row)
}
func activationKind(activate bool) string {
	if activate {
		return "activate"
	}
	return "renew"
}
func transientLicenseStatus(status int) bool {
	return status >= http.StatusInternalServerError && status <= 599
}

// Admission to the serialized exchange consumes the same activation budget.
// A renew already in flight must not make an activation exceed 35 seconds.
func (m *licenseManager) lockOperation(ctx context.Context) bool {
	for {
		if ctx.Err() != nil {
			return false
		}
		if m.opMu.TryLock() {
			return true
		}
		timer := time.NewTimer(10 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return false
		case <-timer.C:
		}
	}
}
