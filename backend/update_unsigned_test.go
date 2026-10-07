//go:build !license

package main

import "crypto/ed25519"

func testUpdateTrust() map[string]ed25519.PublicKey          { return nil }
func signUpdateTestManifest(m updateManifest) updateManifest { return m }
