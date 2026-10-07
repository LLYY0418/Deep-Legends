//go:build license && license_staging

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestR236StagingUpdatesDisabledWithTrustedReleaseCache(t *testing.T) {
	store := trackTestStore(t, &localStore{root: t.TempDir()})
	manifest := updateTestManifest([]byte("signed release setup"))
	cache := updateCache{Current: "0.11.2", CheckedAt: time.Now(), Manifest: manifest}
	cacheData, _ := json.Marshal(cache)
	settingsData := []byte(`{"mirrors":["https://mirror.test/"],"preferredSource":"https://mirror.test/"}`)
	for name, data := range map[string][]byte{"update-manifest.json": cacheData, "update-settings.json": settingsData} {
		if err := writeLocalStoreFile(store, name, data); err != nil {
			t.Fatal(err)
		}
	}
	// Even injected valid update trust cannot override the staging build policy.
	u := newUpdateManagerWithTrust("0.11.2", store, nil, testUpdateTrust())
	defer u.Close()
	var requests, launches atomic.Int32
	u.client = &http.Client{Transport: updateRoundTrip(func(*http.Request) (*http.Response, error) {
		requests.Add(1)
		return nil, errors.New("unexpected update request")
	})}
	u.launch = func(string, string) error { launches.Add(1); return nil }
	if status := u.Status(); status.Supported || status.ReleaseURL != "" || status.Latest != "" || status.State != "idle" {
		t.Fatalf("staging exposes release updates: %+v", status)
	}
	if !u.cache.CheckedAt.IsZero() || u.directory != "" || len(u.Settings().Mirrors) != 0 {
		t.Fatal("staging read release cache/settings or prepared a download directory")
	}
	u.Start()
	if u.Check(false) || u.Check(true) || u.checkDone != nil {
		t.Fatal("staging started an automatic, cached or manual check")
	}
	if err := u.SetSettings(updateSettings{Mirrors: []string{""}}); err == nil {
		t.Fatal("staging modified update settings")
	}
	// Simulate already-ready state as well: Download/Apply must still refuse.
	u.manifest = &manifest
	u.status.Supported, u.status.State = true, "ready"
	for name, action := range map[string]func() error{
		"download": u.Download, "apply": u.Apply,
		"apply_async": func() error { return u.ApplyAsync(func() { launches.Add(1) }) },
	} {
		if err := action(); err == nil || err.Error() != "当前构建不支持更新" {
			t.Fatalf("%s did not refuse staging update: %v", name, err)
		}
	}
	if requests.Load() != 0 || launches.Load() != 0 || u.downloadDone != nil {
		t.Fatal("staging reached update transport or installer launch")
	}
	for name, want := range map[string][]byte{"update-manifest.json": cacheData, "update-settings.json": settingsData} {
		got, err := readLocalStoreFile(store, name)
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("staging touched existing %s: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(store.root, "updates")); !os.IsNotExist(err) {
		t.Fatal("staging created updates directory")
	}
}

func TestR236StagingUpdateHTTPGate(t *testing.T) {
	u := newUpdateManager("0.12.75", trackTestStore(t, &localStore{root: t.TempDir()}), nil)
	defer u.Close()
	a := &app{updates: u}
	for _, action := range []string{"check", "download", "cancel", "apply"} {
		response := httptest.NewRecorder()
		a.handleUpdateAction(response, httptest.NewRequest(http.MethodPost, "/api/update/"+action, nil))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("%s accepted staging update: %d", action, response.Code)
		}
	}
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		response := httptest.NewRecorder()
		a.handleUpdateSettings(response, httptest.NewRequest(method, "/api/update/settings", nil))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("%s accepted staging update settings: %d", method, response.Code)
		}
	}
	if (*updateManager)(nil).Status().ReleaseURL != "" {
		t.Fatal("uninitialized staging updater exposes production release page")
	}
}
