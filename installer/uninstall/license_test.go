//go:build license

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestR232LicenseCredentialsDeletedOnlyOnSuccessfulOptIn(t *testing.T) {
	for _, tc := range []struct {
		code   int
		delete bool
	}{{0, true}, {0, false}, {7, true}} {
		root := t.TempDir()
		license := filepath.Join(root, "Deep Legends", "license")
		os.MkdirAll(license, 0700)
		os.WriteFile(filepath.Join(license, "device.key"), []byte("test protected key"), 0600)
		finishUninstall(tc.code, tc.delete, root, os.RemoveAll, func(string, any) {}, root)
		_, err := os.Stat(license)
		if os.IsNotExist(err) != (tc.code == 0 && tc.delete) {
			t.Fatal("credential cleanup disagrees with user choice", tc, err)
		}
	}
}
