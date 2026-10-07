//go:build license && license_staging

package main

import (
	"crypto/ed25519"
	"encoding/base64"
)

const licenseOrigin = "https://license-staging.yinxiaobia.net"
const licenseBuildLabel = "STAGING"
const licenseOnlineUpdates = false

// R236: owner-supplied staging public key, validated as canonical unpadded
// base64url and exactly 32 bytes before handoff. No private key is needed here.
func licenseTrustKeys() map[string]ed25519.PublicKey {
	key, err := base64.RawURLEncoding.Strict().DecodeString("E8GWwjDvAE_6_6rx5CTT0XqjKrKssR3U3D2z49cGagE")
	if err != nil || len(key) != ed25519.PublicKeySize {
		return map[string]ed25519.PublicKey{}
	}
	return map[string]ed25519.PublicKey{"staging-2026-10": ed25519.PublicKey(key)}
}
func updateTrustKeys() map[string]ed25519.PublicKey { return map[string]ed25519.PublicKey{} }
