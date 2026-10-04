package main

import (
	"os"
	"path/filepath"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestR206IconOnlyChangesIconAndPreservesCreation(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "Deep Legends.lnk")
	target := filepath.Join(root, "target.exe")
	icon := filepath.Join(root, "app.ico")
	os.WriteFile(target, []byte("fixture"), 0600)
	data, _ := uiFiles.ReadFile("ui/app.ico")
	writeStableIcon(icon, data)
	r205MakeLink(t, path, target)
	before := snapshotWindowsShortcut(path, target)
	if err := setShortcutIcon(path, icon); err != nil {
		t.Fatal(err)
	}
	after := snapshotWindowsShortcut(path, target)
	if shortcutCreationChanged(before, after) || after.TargetMatches == nil || !*after.TargetMatches {
		t.Fatal("icon update recreated or retargeted link")
	}
	actual, index, err := shortcutIcon(path)
	if err != nil || actual != icon || index != 0 {
		t.Fatal(actual, index, err)
	}
	if err := withShortcutObject(path, func(link, _ *shortcutCOM) error {
		appID, err := r205AppID(link, "")
		if err != nil {
			return err
		}
		if appID != "DeepLegends.R205.Fixture" {
			t.Fatal("AppUserModelID changed")
		}
		for slot, want := range map[int]string{6: "fixture description", 10: "--fixture-argument"} {
			var buffer [260]uint16
			if err := link.call(slot, uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer))); err != nil {
				return err
			}
			if windows.UTF16ToString(buffer[:]) != want {
				t.Fatal("non-icon field changed", slot)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(path)
	if err := setShortcutIcon(path, icon); err != nil {
		t.Fatal(err)
	}
	next, _ := os.Stat(path)
	if !info.ModTime().Equal(next.ModTime()) {
		t.Fatal("same icon caused COM save")
	}
}
