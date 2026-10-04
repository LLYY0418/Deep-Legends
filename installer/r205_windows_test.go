package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

type r205PropertyKey struct {
	Format windows.GUID
	ID     uint32
}
type r205Variant struct {
	Type     uint16
	Reserved [3]uint16
	Pointer  *uint16
	Extra    uintptr
}

func r205AppID(link *shortcutCOM, set string) (string, error) {
	id, _ := windows.GUIDFromString("{886D8EEB-8CF2-4446-8D02-CDBA1DBDCF99}")
	format, _ := windows.GUIDFromString("{9F4C2855-9F79-4B39-A8D0-E1D42DE1D5F3}")
	key := r205PropertyKey{Format: format, ID: 5}
	var store *shortcutCOM
	if err := link.call(0, uintptr(unsafe.Pointer(&id)), uintptr(unsafe.Pointer(&store))); err != nil {
		return "", err
	}
	defer store.call(2)
	if set != "" {
		value, err := windows.UTF16PtrFromString(set)
		if err != nil {
			return "", err
		}
		variant := r205Variant{Type: 31, Pointer: value}
		err = store.call(6, uintptr(unsafe.Pointer(&key)), uintptr(unsafe.Pointer(&variant)))
		runtime.KeepAlive(value)
		if err != nil {
			return "", err
		}
		return set, store.call(7)
	}
	var value r205Variant
	if err := store.call(5, uintptr(unsafe.Pointer(&key)), uintptr(unsafe.Pointer(&value))); err != nil {
		return "", err
	}
	defer windows.NewLazySystemDLL("ole32.dll").NewProc("PropVariantClear").Call(uintptr(unsafe.Pointer(&value)))
	if value.Type != 31 || value.Pointer == nil {
		return "", nil
	}
	return windows.UTF16PtrToString(value.Pointer), nil
}

func r205MakeLink(t *testing.T, path, target string) {
	t.Helper()
	if err := withShortcutObject("", func(link, persist *shortcutCOM) error {
		for slot, text := range map[int]string{20: target, 7: "fixture description", 11: "--fixture-argument", 17: target} {
			value, err := windows.UTF16PtrFromString(text)
			if err != nil {
				return err
			}
			var callErr error
			if slot == 17 {
				callErr = link.call(slot, uintptr(unsafe.Pointer(value)), 3)
			} else {
				callErr = link.call(slot, uintptr(unsafe.Pointer(value)))
			}
			runtime.KeepAlive(value)
			if callErr != nil {
				return callErr
			}
		}
		if _, err := r205AppID(link, "DeepLegends.R205.Fixture"); err != nil {
			return err
		}
		name, err := windows.UTF16PtrFromString(path)
		if err != nil {
			return err
		}
		err = persist.call(6, uintptr(unsafe.Pointer(name)), 1)
		runtime.KeepAlive(name)
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func TestR205WindowsShortcutRoundTrip(t *testing.T) {
	for _, changedTarget := range []bool{false, true} {
		t.Run(map[bool]string{false: "same_target", true: "changed_target"}[changedTarget], func(t *testing.T) {
			root := t.TempDir()
			path, backup := filepath.Join(root, "Deep Legends.lnk"), filepath.Join(root, "backup.lnk")
			oldTarget, target := filepath.Join(root, "old.exe"), filepath.Join(root, "new.exe")
			os.WriteFile(oldTarget, []byte("fixture"), 0600)
			os.WriteFile(target, []byte("fixture"), 0600)
			if !changedTarget {
				target = oldTarget
			}
			r205MakeLink(t, path, oldTarget)
			before := snapshotWindowsShortcut(path, target)
			if !before.Exists || before.CreatedTime == nil || before.ModifiedTime == nil || before.TargetMatches == nil || *before.TargetMatches == changedTarget {
				data, _ := json.Marshal(before)
				actual, err := shortcutTarget(path)
				t.Fatalf("before=%s fixture_target=%q actual=%q err=%v", data, target, actual, err)
			}
			original, _ := os.ReadFile(path)
			if err := copyShortcutFile(path, backup); err != nil {
				t.Fatal(err)
			}
			os.Remove(path)
			if err := restoreShortcutFile(backup, path, target, before); err != nil {
				t.Fatal(err)
			}
			after := snapshotWindowsShortcut(path, target)
			if !after.Exists || after.TargetMatches == nil || !*after.TargetMatches || shortcutCreationChanged(before, after) || *after.ModifiedTime != *before.ModifiedTime {
				data, _ := json.Marshal([]shortcutState{before, after})
				actual, err := shortcutTarget(path)
				t.Fatalf("before/after=%s fixture_target=%q actual=%q err=%v", data, target, actual, err)
			}
			if !changedTarget {
				data, _ := os.ReadFile(path)
				if !bytes.Equal(data, original) {
					t.Fatal("original properties reconstructed")
				}
			}
			if err := withShortcutObject(path, func(link, _ *shortcutCOM) error {
				id, err := r205AppID(link, "")
				if err != nil {
					return err
				}
				if id != "DeepLegends.R205.Fixture" {
					t.Fatalf("AppUserModelID changed: %q", id)
				}
				for slot, expected := range map[int]string{6: "fixture description", 10: "--fixture-argument"} {
					var value [260]uint16
					if err := link.call(slot, uintptr(unsafe.Pointer(&value[0])), uintptr(len(value))); err != nil {
						return err
					}
					if windows.UTF16ToString(value[:]) != expected {
						t.Fatal("link property changed", slot)
					}
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			os.Remove(target)
			os.Remove(path)
			if restoreShortcutFile(backup, path, target, before) == nil {
				t.Fatal("missing target accepted")
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatal("link restored without target")
			}
		})
	}
}

func TestR205WindowsShortcutTargetAlias(t *testing.T) {
	root := t.TempDir()
	target, alias, other := filepath.Join(root, "target.exe"), filepath.Join(root, "alias.exe"), filepath.Join(root, "other.exe")
	for _, path := range []string{target, other} {
		if err := os.WriteFile(path, []byte("fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Link(target, alias); err != nil {
		t.Fatal(err)
	}
	if !shortcutTargetsMatch(alias, target) || shortcutTargetsMatch(other, target) {
		t.Fatal("target file identity not respected")
	}
}

func TestR205WindowsKeepShortcutsStringValue(t *testing.T) {
	id, err := windows.GenerateGUID()
	if err != nil {
		t.Fatal(err)
	}
	path := `Software\DeepLegendsR205Test-` + strings.Trim(id.String(), "{}")
	defer registry.DeleteKey(registry.CURRENT_USER, path)
	wrote, err := ensureKeepShortcutsValue(registry.CURRENT_USER, path)
	if err != nil || !wrote {
		t.Fatal(wrote, err)
	}
	key, err := registry.OpenKey(registry.CURRENT_USER, path, registry.QUERY_VALUE|registry.WOW64_64KEY)
	if err != nil {
		t.Fatal(err)
	}
	defer key.Close()
	value, kind, err := key.GetStringValue("KeepShortcuts")
	if err != nil || value != "true" || kind != registry.SZ {
		t.Fatal(value, kind, err)
	}
	wrote, err = ensureKeepShortcutsValue(registry.CURRENT_USER, path)
	if err != nil || wrote {
		t.Fatal("existing value rewritten", wrote, err)
	}
}

func TestR205WindowsCOMAlreadyInitialized(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := windows.CoInitializeEx(0, 2); err != nil && err != syscall.Errno(1) {
		t.Fatal(err)
	}
	defer windows.NewLazySystemDLL("ole32.dll").NewProc("CoUninitialize").Call()
	if err := withShortcutObject("", func(_, _ *shortcutCOM) error { return nil }); err != nil {
		t.Fatal("S_FALSE rejected", err)
	}
}
