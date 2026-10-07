//go:build license && license_staging

package main

import (
	"encoding/base64"
	"testing"
)

func TestR233StagingLicenseConfiguration(t *testing.T) {
	keys := licenseTrustKeys()
	if licenseOrigin != "https://license-staging.yinxiaobia.net" || licenseBuildLabel != "STAGING" || len(keys) != 1 || len(updateTrustKeys()) != 0 || licenseOnlineUpdates {
		t.Fatal("staging must use its single owner-supplied license key with online updates disabled")
	}
	key := keys["staging-2026-10"]
	if len(key) != 32 || base64.RawURLEncoding.EncodeToString(key) != "E8GWwjDvAE_6_6rx5CTT0XqjKrKssR3U3D2z49cGagE" {
		t.Fatal("staging key does not match the owner handoff")
	}
	if _, ok := keys["test-license-r233"]; ok {
		t.Fatal("shared vector key must never be trusted")
	}
}
