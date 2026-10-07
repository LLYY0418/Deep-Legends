//go:build license && !license_staging

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// R237: only release builds read signed update caches. Staging's intentional
// refusal to read even a valid cache is covered by the R236 staging tests.
func TestR234SignedUpdateCacheJSONNames(t *testing.T) {
	m := updateTestManifest([]byte("setup fixture"))
	u := updateTestManager(t, []byte("setup fixture"))
	cache, _ := json.Marshal(updateCache{Current: "0.11.2", Manifest: m})
	for _, field := range []string{"manifest", "version", "signed_manifest", "kid", "sha256", ""} {
		t.Run("cache/"+field, func(t *testing.T) {
			data := cache
			if field != "" {
				data = bytes.Replace(cache, []byte(`"`+field+`"`), []byte(`"`+strings.ToUpper(field)+`"`), 1)
			}
			if err := os.WriteFile(filepath.Join(u.store.root, "update-manifest.json"), data, 0600); err != nil {
				t.Fatal(err)
			}
			restored := newUpdateManagerWithTrust("0.11.2", u.store, nil, testUpdateTrust())
			defer restored.Close()
			if (restored.cache.Manifest.Version != "") != (field == "") {
				t.Fatal("update cache exact-name policy mismatch")
			}
		})
	}
}
