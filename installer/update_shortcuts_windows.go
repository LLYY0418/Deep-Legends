package main

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

type shortcutCOM struct{ table *[21]uintptr }

//go:uintptrescapes
func (p *shortcutCOM) call(slot int, args ...uintptr) error {
	args = append([]uintptr{uintptr(unsafe.Pointer(p))}, args...)
	hr, _, _ := syscall.SyscallN(p.table[slot], args...)
	if int32(hr) < 0 {
		return errors.New("shortcut COM operation failed")
	}
	return nil
}

// Load the original object and edit only SetPath. No Resolve, tracking search,
// network lookup, or reconstruction of its AppUserModelID/arguments/icon.
func withShortcutObject(path string, action func(link, persist *shortcutCOM) error) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	// S_FALSE also initialized this thread successfully and owns an uninit.
	if err := windows.CoInitializeEx(0, 2); err != nil && err != syscall.Errno(1) {
		return err
	}
	defer windows.NewLazySystemDLL("ole32.dll").NewProc("CoUninitialize").Call()
	class, _ := windows.GUIDFromString("{00021401-0000-0000-C000-000000000046}")
	linkID, _ := windows.GUIDFromString("{000214F9-0000-0000-C000-000000000046}")
	persistID, _ := windows.GUIDFromString("{0000010B-0000-0000-C000-000000000046}")
	var link, persist *shortcutCOM
	hr, _, _ := windows.NewLazySystemDLL("ole32.dll").NewProc("CoCreateInstance").Call(
		uintptr(unsafe.Pointer(&class)), 0, 1, uintptr(unsafe.Pointer(&linkID)), uintptr(unsafe.Pointer(&link)))
	if int32(hr) < 0 || link == nil {
		return errors.New("shortcut COM unavailable")
	}
	defer link.call(2)
	if err := link.call(0, uintptr(unsafe.Pointer(&persistID)), uintptr(unsafe.Pointer(&persist))); err != nil || persist == nil {
		return errors.New("shortcut persistence unavailable")
	}
	defer persist.call(2)
	if path != "" {
		name, err := windows.UTF16PtrFromString(path)
		if err != nil {
			return err
		}
		if err := persist.call(5, uintptr(unsafe.Pointer(name)), 0); err != nil {
			return err
		}
		runtime.KeepAlive(name)
	}
	return action(link, persist)
}

func shortcutTarget(path string) (string, error) {
	var target string
	err := withShortcutObject(path, func(link, _ *shortcutCOM) error {
		var buffer [windows.MAX_PATH]uint16
		if err := link.call(3, uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer)), 0, 0); err != nil {
			return err
		}
		target = windows.UTF16ToString(buffer[:])
		if target == "" {
			return errors.New("shortcut target unavailable")
		}
		return nil
	})
	return target, err
}

func retargetShortcut(path, target string) error {
	return withShortcutObject(path, func(link, persist *shortcutCOM) error {
		name, err := windows.UTF16PtrFromString(target)
		if err != nil {
			return err
		}
		if err := link.call(20, uintptr(unsafe.Pointer(name))); err != nil {
			return err
		}
		runtime.KeepAlive(name)
		file, err := windows.UTF16PtrFromString(path)
		if err != nil {
			return err
		}
		err = persist.call(6, uintptr(unsafe.Pointer(file)), 1)
		runtime.KeepAlive(file)
		return err
	})
}

func snapshotWindowsShortcut(path, expectedTarget string) shortcutState {
	if path == "" {
		return shortcutState{Status: "folder_unavailable"}
	}
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return shortcutState{Status: "missing"}
	}
	if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
		return shortcutState{Status: "read_error"}
	}
	state := shortcutState{Exists: true, Status: "ok"}
	modified := info.ModTime().UTC().Format(time.RFC3339Nano)
	state.ModifiedTime = &modified
	if data, ok := info.Sys().(*syscall.Win32FileAttributeData); ok && (data.CreationTime.HighDateTime != 0 || data.CreationTime.LowDateTime != 0) {
		created := time.Unix(0, data.CreationTime.Nanoseconds()).UTC().Format(time.RFC3339Nano)
		state.CreatedTime = &created
	}
	target, err := shortcutTarget(path)
	if err != nil {
		state.Status = "target_error"
	} else {
		matches := strings.EqualFold(filepath.Clean(target), filepath.Clean(expectedTarget))
		state.TargetMatches = &matches
	}
	return state
}

func copyShortcutFile(source, destination string) error {
	info, err := os.Lstat(source)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
		return errors.New("shortcut backup unavailable")
	}
	data, err := os.ReadFile(source)
	if err != nil || len(data) > 1<<20 {
		return errors.New("shortcut backup read failed")
	}
	// Overwrite the same directory entry rather than delete/rename the icon.
	if current, err := os.Lstat(destination); err == nil && !current.Mode().IsRegular() {
		return errors.New("shortcut destination unavailable")
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.WriteFile(destination, data, 0600)
}

func restoreShortcutFile(backup, destination, target string, before shortcutState) error {
	if info, err := os.Stat(target); err != nil || !info.Mode().IsRegular() {
		return errors.New("shortcut target missing")
	}
	oldTarget, err := shortcutTarget(backup)
	if err != nil {
		return err
	}
	// Work on the private backup first. A COM failure cannot damage a working
	// shortcut just installed by NSIS. All original properties are loaded.
	if !strings.EqualFold(filepath.Clean(oldTarget), filepath.Clean(target)) {
		if err := retargetShortcut(backup, target); err != nil {
			return err
		}
	}
	if err := copyShortcutFile(backup, destination); err != nil {
		return err
	}
	name, err := windows.UTF16PtrFromString(destination)
	if err != nil {
		return err
	}
	handle, err := windows.CreateFile(name, windows.FILE_WRITE_ATTRIBUTES, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(handle)
	var created, modified *windows.Filetime
	if before.CreatedTime != nil {
		stamp, err := time.Parse(time.RFC3339Nano, *before.CreatedTime)
		if err != nil {
			return err
		}
		value := windows.NsecToFiletime(stamp.UnixNano())
		created = &value
	}
	if before.ModifiedTime != nil {
		stamp, err := time.Parse(time.RFC3339Nano, *before.ModifiedTime)
		if err != nil {
			return err
		}
		value := windows.NsecToFiletime(stamp.UnixNano())
		modified = &value
	}
	return windows.SetFileTime(handle, created, nil, modified)
}

func keepShortcutRegistryRoots() ([]registry.Key, error) {
	var roots []registry.Key
	for _, root := range []registry.Key{registry.CURRENT_USER, registry.LOCAL_MACHINE} {
		key, err := registry.OpenKey(root, shortcutInstallRegistryKey, registry.QUERY_VALUE|registry.WOW64_64KEY)
		if errors.Is(err, registry.ErrNotExist) {
			continue
		}
		if err != nil {
			return roots, err
		}
		key.Close()
		roots = append(roots, root)
	}
	return roots, nil
}

func readKeepShortcutsRegistry() *bool {
	roots, err := keepShortcutRegistryRoots()
	if err != nil {
		return nil
	}
	all := len(roots) > 0
	for _, root := range roots {
		key, err := registry.OpenKey(root, shortcutInstallRegistryKey, registry.QUERY_VALUE|registry.WOW64_64KEY)
		if err != nil {
			return nil
		}
		value, _, err := key.GetStringValue("KeepShortcuts")
		key.Close()
		if err != nil && !errors.Is(err, registry.ErrNotExist) && !errors.Is(err, registry.ErrUnexpectedType) {
			return nil
		}
		all = all && err == nil && value == "true"
	}
	return &all
}

func repairKeepShortcutsRegistry() error {
	roots, err := keepShortcutRegistryRoots()
	if err != nil {
		return err
	}
	if len(roots) == 0 {
		roots = []registry.Key{registry.CURRENT_USER}
	}
	var result error
	for _, root := range roots {
		_, err := ensureKeepShortcutsValue(root, shortcutInstallRegistryKey)
		result = errors.Join(result, err)
	}
	return result
}

func ensureKeepShortcutsValue(root registry.Key, path string) (bool, error) {
	if existing, err := registry.OpenKey(root, path, registry.QUERY_VALUE|registry.WOW64_64KEY); err == nil {
		value, _, valueErr := existing.GetStringValue("KeepShortcuts")
		existing.Close()
		if valueErr == nil && value == "true" {
			return false, nil
		}
	}
	key, _, err := registry.CreateKey(root, path, registry.SET_VALUE|registry.WOW64_64KEY)
	if err != nil {
		return false, err
	}
	defer key.Close()
	return true, key.SetStringValue("KeepShortcuts", "true")
}

func newWindowsShortcutUpdate(dest, temporaryDir, exeName string, repairRegistry bool) *shortcutUpdateGuard {
	target := filepath.Join(dest, exeName)
	desktop, menu := map[string]string{}, map[string]string{}
	for scope, id := range map[string]*windows.KNOWNFOLDERID{"current": windows.FOLDERID_Desktop, "public": windows.FOLDERID_PublicDesktop} {
		if directory, err := windows.KnownFolderPath(id, windows.KF_FLAG_DONT_VERIFY); err == nil {
			desktop[scope] = filepath.Join(directory, "Deep Legends.lnk")
		}
	}
	for scope, id := range map[string]*windows.KNOWNFOLDERID{"current": windows.FOLDERID_Programs, "public": windows.FOLDERID_CommonPrograms} {
		if directory, err := windows.KnownFolderPath(id, windows.KF_FLAG_DONT_VERIFY); err == nil {
			menu[scope] = filepath.Join(directory, "Deep Legends.lnk")
		}
	}
	before := map[string]shortcutState{}
	hooks := shortcutUpdateHooks{
		Snapshot: func() (map[string]shortcutState, map[string]shortcutState) {
			d, m := map[string]shortcutState{}, map[string]shortcutState{}
			for _, scope := range []string{"current", "public"} {
				d[scope] = snapshotWindowsShortcut(desktop[scope], target)
				m[scope] = snapshotWindowsShortcut(menu[scope], target)
			}
			return d, m
		},
		ReadKeep: readKeepShortcutsRegistry, RepairKeep: repairKeepShortcutsRegistry,
		Backup: func(scope string) error {
			before[scope] = snapshotWindowsShortcut(desktop[scope], target)
			return copyShortcutFile(desktop[scope], filepath.Join(temporaryDir, "shortcut-"+scope+".lnk"))
		},
		Restore: func(scope string) error {
			return restoreShortcutFile(filepath.Join(temporaryDir, "shortcut-"+scope+".lnk"), desktop[scope], target, before[scope])
		},
		TargetExists: func() bool { info, err := os.Stat(target); return err == nil && info.Mode().IsRegular() },
		Write: func(report shortcutUpdateReport) {
			if timing := newUpdateInstallTiming(); timing != nil {
				_ = writeShortcutUpdateReport(filepath.Dir(timing.path), report)
			}
		},
	}
	return beginShortcutUpdate(hooks, repairRegistry)
}
