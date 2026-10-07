//go:build license

package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"
)

var testUpdatePub, testUpdatePrivate, _ = ed25519.GenerateKey(rand.Reader)

func testUpdateTrust() map[string]ed25519.PublicKey {
	return map[string]ed25519.PublicKey{"test-update": testUpdatePub}
}
func signUpdateTestManifest(m updateManifest) updateManifest {
	m.SignedManifest = nil
	signed := testSignedEnvelope(testUpdatePrivate, "test-update", "DL-UPDATE-MANIFEST-V1", m)
	m.SignedManifest = &signed
	return m
}
func TestR232UpdateSignatureTrust(t *testing.T) {
	m := updateTestManifest([]byte("test setup"))
	if err := verifyUpdateManifestTrust(m, testUpdateTrust()); err != nil {
		t.Fatal(err)
	}
	if verifyUpdateManifestTrust(m, updateTrustKeys()) == nil {
		t.Fatal("test key trusted by production")
	}
	for _, mutate := range []func(*updateManifest){func(m *updateManifest) { m.SignedManifest = nil }, func(m *updateManifest) { m.Asset.SHA256 = "tampered" }, func(m *updateManifest) { m.Version = "0.99.0" }, func(m *updateManifest) { copy := *m.SignedManifest; copy.Kid = "unknown"; m.SignedManifest = &copy }} {
		bad := m
		mutate(&bad)
		if verifyUpdateManifestTrust(bad, testUpdateTrust()) == nil {
			t.Fatal("unsafe update accepted")
		}
	}
}
