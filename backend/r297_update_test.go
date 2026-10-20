package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestR297UpdateTargetsRunningInstallationDespiteTwoDeadRegistrations(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Games", "Deep Legends")
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	entries := []updateRegistryInstallation{{"Deep Legends", filepath.Join(t.TempDir(), "missing-one")}, {"Deep Legends", filepath.Join(t.TempDir(), "missing-two")}}
	for _, tc := range []struct {
		portable, uninstaller bool
		result                string
	}{{false, true, "installed"}, {true, true, "portable_env"}, {true, false, "portable_env"}, {false, false, "no_uninstaller"}} {
		got := classifyUpdateInstallationWith(root, tc.portable, tc.uninstaller, entries, "default-other", filepath.Clean)
		if got.Result != tc.result || got.RegistryEntries != 2 || got.MatchAfterNormalize || got.LocationExists {
			t.Fatal(got)
		}
		if tc.result == "installed" && got.directory != root {
			t.Fatal("normal install must remain in running directory")
		}
		if tc.result != "installed" && got.directory != "" {
			t.Fatal("unconfirmed target")
		}
		encoded, _ := json.Marshal(got.targetDiagnosticEvent())
		if strings.Contains(string(encoded), root) || strings.Contains(string(encoded), "missing-one") {
			t.Fatal("path leaked")
		}
	}
}
func TestR297UnknownTargetCannotDownloadOrApply(t *testing.T) {
	u := updateTestManager(t, []byte("setup"))
	u.status.InstallTargetBlocked = true
	var launched atomic.Bool
	u.launch = func(string, string) error { launched.Store(true); return nil }
	for _, op := range []func() error{u.Download, u.Apply} {
		if err := op(); err == nil || !strings.Contains(err.Error(), "安装位置无法确认") {
			t.Fatalf("unknown target was not rejected: %v", err)
		}
	}
	if launched.Load() {
		t.Fatal("unconfirmed target launched installer")
	}
}
