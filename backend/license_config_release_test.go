//go:build license && !license_staging

package main

import (
	"crypto/ed25519"
	"testing"
)

func TestR233ReleaseLicenseConfiguration(t *testing.T) {
	if licenseOrigin != "https://license.yinxiaobia.net" || licenseBuildLabel != "release" || len(licenseTrustKeys()) != 0 || len(updateTrustKeys()) != 0 || !licenseOnlineUpdates {
		t.Fatal("release public configuration must remain unconfigured until supplied by owner")
	}
	for _, keys := range []map[string]ed25519.PublicKey{licenseTrustKeys(), updateTrustKeys()} {
		if _, ok := keys["test-license-r233"]; ok {
			t.Fatal("shared vector key must never be trusted")
		}
	}
}
