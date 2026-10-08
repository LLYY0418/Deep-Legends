//go:build !license

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestR252DefaultEmbeddedAssetsHaveNoActivationContent(t *testing.T) {
	if err := fs.WalkDir(embedded, "web", func(name string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		body, err := fs.ReadFile(embedded, name)
		if err != nil {
			return err
		}
		for _, marker := range []string{"注册码", "授权到期", "DL-XXXXX", "license.yinxiaobia.net", "license-staging.yinxiaobia.net"} {
			if bytes.Contains(body, []byte(marker)) {
				t.Errorf("default embedded asset %s contains %s", name, marker)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	files, err := fs.Sub(embedded, "web")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"index.html", "license-ui.js", "gameplay.js"} {
		if _, err := fs.ReadFile(files, name); err != nil {
			t.Fatal("default frontend route missing", name, err)
		}
	}
}

func TestR248DisabledHTTPAndPrivacy(t *testing.T) {
	a := &app{}
	rec := httptest.NewRecorder()
	a.handleLicenseStatus(rec, httptest.NewRequest("GET", "/api/license/status", nil))
	if strings.TrimSpace(rec.Body.String()) != `{"state":"DISABLED"}` {
		t.Fatal("default license status", rec.Body.String())
	}
	var calls int
	handler := a.withLicenseProtection(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(204) }))
	for _, route := range []string{"/api/status", "/api/collection/rescan", "/api/gameplay/autofill"} {
		rec = httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest("POST", route, nil))
		if rec.Code != 204 {
			t.Fatal("default business gate", route, rec.Code)
		}
	}
	if calls != 3 {
		t.Fatal("business handlers did not run", calls)
	}
	rec = httptest.NewRecorder()
	a.handlePrivacy(rec, httptest.NewRequest("GET", "/api/privacy", nil))
	if strings.Contains(rec.Body.String(), "licenseDisclosure") || strings.Contains(rec.Body.String(), "注册码") || strings.Contains(rec.Body.String(), "授权凭据") {
		t.Fatal("default privacy exposes activation", rec.Body.String())
	}
}
func TestR248DisabledStartupNoAuthorizationNetwork(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1); w.WriteHeader(204) }))
	defer server.Close()
	endpoint, _ := url.Parse(server.URL)
	old := http.DefaultTransport
	http.DefaultTransport = updateRoundTrip(func(r *http.Request) (*http.Response, error) {
		if strings.HasPrefix(r.URL.Hostname(), "license") {
			copy := r.Clone(r.Context())
			target := *r.URL
			target.Scheme = endpoint.Scheme
			target.Host = endpoint.Host
			copy.URL = &target
			return old.RoundTrip(copy)
		}
		return old.RoundTrip(r)
	})
	defer func() { http.DefaultTransport = old }()
	a := &app{champions: newChampionProvider()}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	a.startApplicationBusiness(ctx)
	if requests.Load() != 0 {
		t.Fatal("default startup must issue zero authorization requests", requests.Load())
	}
}
func TestR248Existing01276ReleaseAsset(t *testing.T) {
	dir := filepath.Join("testdata", "release-0.12.76")
	assetDir := os.Getenv("DEEP_LEGENDS_UPDATE_FIXTURE_DIR")
	if assetDir == "" {
		assetDir = filepath.Join("..", "output", "update-fixtures", "release-0.12.76")
	}
	data, err := os.ReadFile(filepath.Join(dir, "latest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest updateManifest
	if err = json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	u := newUpdateManager("0.12.75", trackTestStore(t, &localStore{root: t.TempDir()}), nil)
	defer u.Close()
	u.status.Portable = false
	u.sourceDefaults = []string{""}
	u.client = &http.Client{Transport: updateRoundTrip(func(*http.Request) (*http.Response, error) { return updateResponse(200, data), nil })}
	if !u.Check(true) {
		t.Fatal("unsigned existing Latest check did not start")
	}
	waitUpdateCheck(t, u)
	if status := u.Status(); status.Latest != "0.12.76" || status.State != "available" {
		t.Fatal("existing Latest unavailable", status)
	}
	if manifest.Version != "0.12.76" {
		t.Fatal("unexpected published version", manifest.Version)
	}
	if err = verifyUpdateManifestTrust(manifest, nil); err != nil {
		t.Fatal("existing unsigned Latest rejected", err)
	}
	if err = verifyUpdateFile(context.Background(), filepath.Join(assetDir, manifest.Asset.Name), manifest.Asset); err != nil {
		t.Fatal("existing published asset rejected", err)
	}
	bad := manifest
	bad.Asset.SHA256 = "bad"
	if verifyUpdateManifestTrust(bad, nil) == nil {
		t.Fatal("invalid manifest accepted")
	}
	bad = manifest
	bad.Asset.SHA256 = strings.Repeat("0", 64)
	if verifyUpdateFile(context.Background(), filepath.Join(assetDir, manifest.Asset.Name), bad.Asset) == nil {
		t.Fatal("checksum mismatch accepted")
	}
}
