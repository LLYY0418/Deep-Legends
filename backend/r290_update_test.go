package main

import (
	"encoding/json"
	"net/http"
	"sync/atomic"
	"testing"
)

func TestR290AutoDownloadDefaultsAndSourceRewrite(t *testing.T) {
	u := updateTestManager(t, []byte("fixture"))
	if !*u.Settings().AutoDownload {
		t.Fatal("new user default must be enabled")
	}
	if err := writeLocalStoreFile(u.store, "update-settings.json", []byte(`{"mirrors":[""]}`)); err != nil {
		t.Fatal(err)
	}
	legacy := newUpdateManagerWithTrust("0.11.2", u.store, nil, testUpdateTrust())
	defer legacy.Close()
	if !*legacy.Settings().AutoDownload {
		t.Fatal("missing legacy field must be enabled")
	}
	disabled := false
	if err := u.SetSettings(updateSettings{Mirrors: []string{""}, Automatic: &disabled, AutoDownload: &disabled}); err != nil {
		t.Fatal(err)
	}
	u.rememberUpdateSource("")
	raw, err := readLocalStoreFile(u.store, "update-settings.json")
	if err != nil {
		t.Fatal(err)
	}
	var saved updateSettings
	if json.Unmarshal(raw, &saved) != nil || saved.AutoDownload == nil || *saved.AutoDownload || saved.Automatic == nil || *saved.Automatic {
		t.Fatalf("rewrite dropped choices: %s", raw)
	}
	restarted := newUpdateManagerWithTrust("0.11.2", u.store, nil, testUpdateTrust())
	defer restarted.Close()
	if *restarted.Settings().AutoDownload || *restarted.Settings().Automatic {
		t.Fatal("disabled choices lost after restart")
	}
	if err := restarted.SetSettings(updateSettings{Mirrors: []string{""}}); err != nil {
		t.Fatal(err)
	}
	if *restarted.Settings().AutoDownload {
		t.Fatal("partial settings reset download choice")
	}
}

func TestR290AutoDownloadOnlyAfterAutomaticCheck(t *testing.T) {
	for _, mode := range []string{"disabled", "reenabled", "manual"} {
		t.Run(mode, func(t *testing.T) {
			data := []byte("verified installer")
			u := updateTestManager(t, data)
			u.autoPredownload = true
			disabled := false
			if err := u.SetSettings(updateSettings{Mirrors: []string{""}, AutoDownload: &disabled}); err != nil {
				t.Fatal(err)
			}
			if mode != "disabled" {
				enabled := true
				if err := u.SetSettings(updateSettings{Mirrors: []string{""}, AutoDownload: &enabled}); err != nil {
					t.Fatal(err)
				}
			}
			u.cache = updateCache{CheckedAt: u.now(), Current: u.status.Current, Manifest: *u.manifest}
			var downloads atomic.Int32
			u.client = &http.Client{Transport: updateRoundTrip(func(r *http.Request) (*http.Response, error) {
				if r.URL.String() == u.manifest.Asset.URL {
					downloads.Add(1)
					return updateResponse(200, data), nil
				}
				manifest, _ := json.Marshal(*u.manifest)
				return updateResponse(200, manifest), nil
			})}
			if !u.Check(mode == "manual") {
				t.Fatal("check rejected")
			}
			waitUpdateCheck(t, u)
			if mode == "reenabled" {
				waitUpdateDownload(t, u)
				if downloads.Load() != 1 || u.Status().State != "ready" {
					t.Fatal(downloads.Load(), u.Status())
				}
			} else {
				if downloads.Load() != 0 || u.Status().State != "available" {
					t.Fatal("check downloaded without permission", downloads.Load(), u.Status())
				}
				if err := u.Download(); err != nil {
					t.Fatal(err)
				}
				waitUpdateDownload(t, u)
				if downloads.Load() != 1 || u.Status().State != "ready" {
					t.Fatal("explicit download disabled", downloads.Load(), u.Status())
				}
			}
		})
	}
}
