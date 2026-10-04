package main

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

func shortcutIcon(path string) (string, int32, error) {
	var icon string
	var index int32
	err := withShortcutObject(path, func(link, _ *shortcutCOM) error {
		var buffer [windows.MAX_PATH]uint16
		err := link.call(16, uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer)), uintptr(unsafe.Pointer(&index)))
		icon = windows.UTF16ToString(buffer[:])
		return err
	})
	return icon, index, err
}

func setShortcutIcon(path, icon string) error {
	current, index, err := shortcutIcon(path)
	if err != nil {
		return err
	}
	if shortcutTargetsMatch(current, icon) && index == 0 {
		return nil
	}
	before, err := os.Stat(path)
	if err != nil {
		return err
	}
	attrs, ok := before.Sys().(*syscall.Win32FileAttributeData)
	if !ok {
		return errors.New("shortcut timestamps unavailable")
	}
	err = withShortcutObject(path, func(link, persist *shortcutCOM) error {
		name, err := windows.UTF16PtrFromString(icon)
		if err != nil {
			return err
		}
		err = link.call(17, uintptr(unsafe.Pointer(name)), 0)
		runtime.KeepAlive(name)
		if err != nil {
			return err
		}
		file, err := windows.UTF16PtrFromString(path)
		if err != nil {
			return err
		}
		err = persist.call(6, uintptr(unsafe.Pointer(file)), 1)
		runtime.KeepAlive(file)
		return err
	})
	if err != nil {
		return err
	}
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	handle, err := windows.CreateFile(name, windows.FILE_WRITE_ATTRIBUTES, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return err
	}
	created := windows.Filetime{LowDateTime: attrs.CreationTime.LowDateTime, HighDateTime: attrs.CreationTime.HighDateTime}
	err = windows.SetFileTime(handle, &created, nil, nil)
	windows.CloseHandle(handle)
	if err != nil {
		return err
	}
	// SHCNE_UPDATEITEM | SHCNF_PATHW | SHCNF_FLUSH: refresh this exact entry.
	windows.NewLazySystemDLL("shell32.dll").NewProc("SHChangeNotify").Call(0x2000, 0x1005, uintptr(unsafe.Pointer(name)), 0)
	runtime.KeepAlive(name)
	return nil
}

func stabilizeWindowsShortcutIcons(dest, exeName string) bool {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		return false
	}
	icon := filepath.Join(base, "deep-legends", "app.ico")
	data, err := uiFiles.ReadFile("ui/app.ico")
	if err != nil {
		return false
	}
	if _, err = writeStableIcon(icon, data); err != nil {
		return false
	}
	stable := true
	for _, id := range []*windows.KNOWNFOLDERID{windows.FOLDERID_Desktop, windows.FOLDERID_Programs} {
		directory, err := windows.KnownFolderPath(id, windows.KF_FLAG_DONT_VERIFY)
		if err != nil {
			stable = false
			continue
		}
		path := filepath.Join(directory, "Deep Legends.lnk")
		if _, err := os.Lstat(path); os.IsNotExist(err) {
			continue
		}
		target, err := shortcutTarget(path)
		// Only edit this installed application's links, never a same-name link to
		// another checkout. SetIconLocation preserves all loaded ShellLink fields.
		if err != nil || !shortcutTargetsMatch(target, filepath.Join(dest, exeName)) {
			stable = false
			continue
		}
		if setShortcutIcon(path, icon) != nil {
			stable = false
		}
	}
	return stable
}
