package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPrepareLaunchUsesResourcePayloadOnly(t *testing.T) {
	directory := t.TempDir()
	resources := filepath.Join(directory, "resources")
	if err := os.Mkdir(resources, 0700); err != nil {
		t.Fatal(err)
	}
	payload := []byte("MZ-test-uninstaller-payload")
	if err := os.WriteFile(filepath.Join(resources, corePayloadName), payload, 0600); err != nil {
		t.Fatal(err)
	}
	launch, err := prepareLaunch(filepath.Join(directory, shellFilename), uninstallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(launch.Workspace) })
	got, err := os.ReadFile(filepath.Join(launch.Workspace, coreFilename))
	if err != nil || string(got) != string(payload) {
		t.Fatalf("temporary executable not copied: %v", err)
	}
	if _, err := os.Stat(filepath.Join(directory, coreFilename)); !os.IsNotExist(err) {
		t.Fatal("second root uninstaller created")
	}
	if launch.Directory != directory {
		t.Fatal("incorrect uninstall target")
	}
}

func TestPrepareLaunchRejectsMissingOrInvalidPayload(t *testing.T) {
	for _, invalid := range []bool{false, true} {
		directory := t.TempDir()
		if err := os.Mkdir(filepath.Join(directory, "resources"), 0700); err != nil {
			t.Fatal(err)
		}
		if invalid {
			if err := os.WriteFile(filepath.Join(directory, "resources", corePayloadName), []byte("not-an-executable"), 0600); err != nil {
				t.Fatal(err)
			}
		}
		launch, err := prepareLaunch(filepath.Join(directory, shellFilename), uninstallOptions{})
		if err == nil {
			t.Fatal("invalid payload accepted")
		}
		if _, err := os.Stat(launch.Workspace); !os.IsNotExist(err) {
			t.Fatal("failed preparation leaked workspace")
		}
		if _, err := os.Stat(directory); err != nil {
			t.Fatal("install directory changed on failed preparation")
		}
	}
}
