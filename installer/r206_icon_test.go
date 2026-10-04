package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestR206StableIconUnchangedDoesNotWrite(t *testing.T) {
	data, err := uiFiles.ReadFile("ui/app.ico")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "deep-legends", "app.ico")
	wrote, err := writeStableIcon(path, data)
	if err != nil || !wrote {
		t.Fatal(wrote, err)
	}
	stamp := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if os.Chtimes(path, stamp, stamp) != nil {
		t.Fatal("timestamp fixture")
	}
	wrote, err = writeStableIcon(path, data)
	if err != nil || wrote {
		t.Fatal("identical icon rewritten", wrote, err)
	}
	info, _ := os.Stat(path)
	if !info.ModTime().Equal(stamp) {
		t.Fatal("no-op touched icon")
	}
}
