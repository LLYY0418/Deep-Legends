//go:build windows

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// Invoked by the real-upgrade CI fixture after installing the published build.
// Uses the production registry reader, decision and native installer handoff.
func TestR297WindowsInstalledTargetHandoff(t *testing.T) {
	root, setup := os.Getenv("R297_TARGET_INSTALL"), os.Getenv("R297_TARGET_SETUP")
	if root == "" || setup == "" {
		t.Skip("requires isolated Windows real upgrade fixture")
	}
	d := detectUpdateInstallationAt(root, false)
	if d.Result != "installed" || !d.HasUninstaller || !updatePathsMatch(d.directory, root, normalizeUpdateInstallationPath) {
		t.Fatalf("running directory was not selected: %+v", d.diagnosticEvent())
	}
	if !regularFile(filepath.Join(root, "Deep Legends.exe")) || !regularFile(filepath.Join(root, "resources", "app.asar")) {
		t.Fatal("old installation is incomplete")
	}
	if err := launchDetectedUpdateInstaller(setup, d.directory, d); err != nil {
		t.Fatal(err)
	}
}
