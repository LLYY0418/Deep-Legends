//go:build !license

package main

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"net/http"
)

const licenseBuildEnabled = false
const licenseOnlineUpdates = true

// The disabled build has no identity, store, issuer, renewal loop or trust root.
// These adapters retain the common business call sites without authorization.
type licenseManager struct{}

var errLicenseLocked = errors.New("business context unavailable")

func productionLicenseManager(func(map[string]any)) *licenseManager { return nil }
func (a *app) startApplicationBusiness(ctx context.Context) {
	a.mu.Lock()
	a.proRefreshContext = ctx
	a.mu.Unlock()
	a.goSafe("connection_manager", func() { a.runConnectionManager(ctx) })
	a.warmProPlayersCaches()
	a.refreshFeatureGates(ctx)
}
func (a *app) withLicenseProtection(next http.Handler) http.Handler { return next }
func licenseContextValid(ctx context.Context) bool                  { return ctx.Err() == nil }
func (c *LCUClient) licenseRequestContext(ctx context.Context) (context.Context, func(), error) {
	return ctx, func() {}, ctx.Err()
}
func (c *LCUClient) licenseResult(ctx context.Context) error              { return ctx.Err() }
func (a *app) licensedDiscovery() (*LCUClient, LCUDiscoveryStatus, error) { return a.discoverColdLCU() }
func (a *app) licenseSideEffect(ctx context.Context) error                { return ctx.Err() }
func (a *app) licenseBusinessContext() context.Context {
	a.mu.RLock()
	ctx := a.proRefreshContext
	a.mu.RUnlock()
	if ctx != nil {
		return ctx
	}
	return context.Background()
}
func (a *app) proBusinessContext() context.Context {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.proRefreshContext
}
func (a *app) handleLicenseStatus(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	respondJSON(w, map[string]string{"state": "DISABLED"})
}
func (a *app) handleLicenseActivate(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) }
func (a *app) recordLicenseWindowDiagnostic(json.RawMessage) bool           { return true }
func appendLicensePrivacy(map[string]any)                                   {}
func updateTrustKeys() map[string]ed25519.PublicKey                         { return nil }

// The optional envelope remains a data field so both build modes share the
// updater DTO; the default build does not verify or require this field.
type signedLicenseEnvelope struct {
	Algorithm string `json:"algorithm"`
	Kid       string `json:"kid"`
	Payload   string `json:"payload"`
	Signature string `json:"signature"`
}

func strictLicenseJSON(data []byte, target any) error { return json.Unmarshal(data, target) }
func verifyUpdateManifestTrust(manifest updateManifest, _ map[string]ed25519.PublicKey) error {
	return validateUpdateManifest(manifest)
}

func frontendBuildAsset(_ string, body []byte) []byte { return body }
