package main

import (
	"os"
	"path/filepath"
	"testing"
)

// Only temporary destinations: never invoke installer execution, registry
// writes, or an interactive native dialog on the Windows CI worker.
func TestR86WindowsDestinationPreflightSmoke(t *testing.T) {
	root := t.TempDir()
	dest, failure := prepareDestination(filepath.Join(root, productFolder), 1)
	if failure != nil {
		t.Fatalf("preflight failed: %+v", failure)
	}
	if !filepath.IsAbs(dest) {
		t.Fatal(dest)
	}
	entries, err := os.ReadDir(dest)
	if err != nil || len(entries) != 0 {
		t.Fatalf("write probe left files: %v %v", entries, err)
	}
	blocker := filepath.Join(root, "file")
	if err := os.WriteFile(blocker, []byte("preserve"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, failure := prepareDestination(filepath.Join(blocker, productFolder), 1); failure == nil {
		t.Fatal("file accepted as directory")
	}
	data, err := os.ReadFile(blocker)
	if err != nil || string(data) != "preserve" {
		t.Fatal("preflight removed existing evidence")
	}
}

func TestR86WindowsDialogDirectoryAndDiskHelpers(t *testing.T) {
	root := t.TempDir()
	missing := filepath.Join(root, "not-created", "child")
	if got := nearestExistingDirectory(missing); got != root {
		t.Fatalf("nearest=%q want %q", got, root)
	}
	free, err := diskFreeBytes(missing)
	if err != nil || free <= 0 {
		t.Fatalf("free=%d err=%v", free, err)
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Fatal("read-only directory helper created a path")
	}
}
