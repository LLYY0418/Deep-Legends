//go:build !license

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// The candidate is served over real loopback HTTP before it becomes Latest.
// URL validation remains on the exact public GitHub URLs; only this test's
// transport maps those URLs to the fixture. Windows then installs this verified
// download in r206-real-upgrade-windows.ps1. This is not an anonymous Latest run.
func TestR238DefaultOnline076To077(t *testing.T) {
	data := []byte("R238 synthetic public installer")
	if file := os.Getenv("R238_UPGRADE_SETUP"); file != "" {
		var err error
		data, err = os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
	}
	digest := sha256.Sum256(data)
	candidate := r238CandidateVersion(t)
	name := "Deep-Legends-Setup-" + candidate + "-public.exe"
	manifest := updateManifest{Schema: 1, Version: candidate, Fingerprint: "a1b2c3d4e5f6", PublishedAt: time.Date(2026, 10, 7, 0, 0, 0, 0, time.UTC), MinSupported: "0.9.0", Notes: "R238 candidate fixture", Asset: updateAsset{Name: name, Size: int64(len(data)), SHA256: hex.EncodeToString(digest[:]), URL: "https://github.com/" + updateRepo + "/releases/download/v" + candidate + "/" + name}}
	encoded, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	var checks, downloads atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/manifest":
			checks.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(encoded)
		case "/setup":
			downloads.Add(1)
			http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(data))
		default:
			t.Errorf("unexpected fixture path: %s", r.URL)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	target, _ := url.Parse(server.URL)
	storeRoot := t.TempDir()
	if os.Getenv("R238_UPGRADE_SETUP") != "" {
		// Exercise the same isolated data directory as the installed application,
		// rather than a second unrelated Windows temporary volume.
		storeRoot = os.Getenv("R238_UPGRADE_DATA")
		if info, err := os.Stat(storeRoot); err != nil || !info.IsDir() {
			t.Fatalf("real upgrade data directory unavailable: %q %v", storeRoot, err)
		}
	}
	store := trackTestStore(t, &localStore{root: storeRoot})
	u := newUpdateManager("0.12.76", store, nil)
	defer u.Close()
	// Capture the live download state before Close cancels it. The existing
	// five-second wait and every success assertion remain unchanged.
	defer func() {
		if !t.Failed() {
			return
		}
		files := []map[string]any{}
		entries, readErr := os.ReadDir(u.directory)
		for _, entry := range entries {
			if info, err := entry.Info(); err == nil {
				files = append(files, map[string]any{"name": entry.Name(), "size": info.Size()})
			}
		}
		row := map[string]any{"status": u.Status(), "directory": u.directory, "files": files, "setup_size": len(data), "checks": checks.Load(), "downloads": downloads.Load()}
		if readErr != nil {
			row["directory_error"] = readErr.Error()
		}
		raw, _ := json.MarshalIndent(row, "", "  ")
		t.Logf("R238 online failure state: %s", raw)
		if evidence := os.Getenv("R238_UPGRADE_EVIDENCE"); evidence != "" {
			if err := os.WriteFile(filepath.Join(evidence, "online-076-077-failure.json"), raw, 0600); err != nil {
				t.Logf("failure evidence write: %v", err)
			}
		}
	}()
	u.status.Portable = false
	u.installDir = filepath.Join(store.root, "installed")
	if dir := os.Getenv("R238_UPGRADE_INSTALL"); dir != "" {
		u.installDir = dir
	}
	u.sourceDefaults, u.mirrors = []string{}, []string{""}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	defer transport.CloseIdleConnections()
	u.client = &http.Client{Transport: updateRoundTrip(func(r *http.Request) (*http.Response, error) {
		copy := r.Clone(r.Context())
		address := *target
		switch r.URL.String() {
		case updateManifestPath:
			address.Path = "/manifest"
		case manifest.Asset.URL:
			address.Path = "/setup"
		default:
			return nil, fmt.Errorf("unexpected update URL: %s", r.URL)
		}
		copy.URL = &address
		return transport.RoundTrip(copy)
	})}
	if !u.Check(true) {
		t.Fatal("0.12.76 check did not start")
	}
	waitUpdateCheck(t, u)
	if status := u.Status(); status.State != "available" || status.Latest != candidate {
		t.Fatal("candidate not available", status)
	}
	if err = u.Download(); err != nil {
		t.Fatal(err)
	}
	waitUpdateDownload(t, u)
	if status := u.Status(); status.State != "ready" || status.Error != "" {
		t.Fatal("verified download not ready", status)
	}
	file := filepath.Join(u.directory, name)
	if err = verifyUpdateFile(context.Background(), file, manifest.Asset); err != nil {
		t.Fatal(err)
	}
	var launches int
	u.launch = func(setup, dest string) error {
		if setup != file || dest != u.installDir {
			t.Fatalf("install handoff mismatch: %s %s", setup, dest)
		}
		launches++
		return nil
	}
	if err = u.Apply(); err != nil {
		t.Fatal(err)
	}
	if launches != 1 || checks.Load() != 1 || downloads.Load() == 0 {
		t.Fatal("online chain incomplete", launches, checks.Load(), downloads.Load())
	}
	if evidence := os.Getenv("R238_UPGRADE_EVIDENCE"); evidence != "" {
		// The ordinary test temp directory is cleaned on exit. Preserve only the
		// verified public candidate for the subsequent real Windows installer.
		download := filepath.Join(evidence, name)
		downloaded, readErr := os.ReadFile(file)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if err = os.WriteFile(download, downloaded, 0600); err != nil {
			t.Fatal(err)
		}
		row, _ := json.MarshalIndent(map[string]any{"current": "0.12.76", "latest": candidate, "download": download, "sha256": manifest.Asset.SHA256, "size": len(data), "checks": checks.Load(), "downloads": downloads.Load(), "unsigned_validation": true, "transport": "real loopback HTTP with exact GitHub URL mapping; not anonymous Latest", "install_handoff": true}, "", "  ")
		if err = os.WriteFile(filepath.Join(evidence, "online-076-077.json"), row, 0600); err != nil {
			t.Fatal(err)
		}
	}
}

// r238CandidateVersion is the version being released, read from the desktop
// package so that a version bump needs no edits to this upgrade fixture.
func r238CandidateVersion(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "desktop", "package.json"))
	if err != nil {
		t.Fatal(err)
	}
	var pkg struct {
		Version string `json:"version"`
	}
	if err = json.Unmarshal(raw, &pkg); err != nil {
		t.Fatal(err)
	}
	if compareVersions(pkg.Version, "0.12.76") <= 0 {
		t.Fatalf("candidate %q must be newer than the published 0.12.76 it upgrades", pkg.Version)
	}
	return pkg.Version
}
