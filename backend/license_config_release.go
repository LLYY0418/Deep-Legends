//go:build license && !license_staging

package main

import "crypto/ed25519"

const licenseOrigin = "https://license.yinxiaobia.net"
const licenseBuildLabel = "release"
const licenseOnlineUpdates = true

// Public configuration is supplied by the owner, never generated here.
// Empty trust roots block release authorization and signed updates.
func licenseTrustKeys() map[string]ed25519.PublicKey { return map[string]ed25519.PublicKey{} }
func updateTrustKeys() map[string]ed25519.PublicKey  { return map[string]ed25519.PublicKey{} }
