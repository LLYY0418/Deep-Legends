package main

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var shell32 = windows.NewLazySystemDLL("shell32.dll")

func initialInstallDir() string {
	for _, view := range []uint32{registry.WOW64_64KEY, registry.WOW64_32KEY} {
		key, err := registry.OpenKey(registry.CURRENT_USER, `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`, registry.READ|view)
		if err != nil {
			continue
		}
		names, _ := key.ReadSubKeyNames(-1)
		for _, name := range names {
			entry, err := registry.OpenKey(key, name, registry.QUERY_VALUE|view)
			if err != nil {
				continue
			}
			displayName, _, _ := entry.GetStringValue("DisplayName")
			location, _, _ := entry.GetStringValue("InstallLocation")
			entry.Close()
			if displayName == productFolder {
				if normalized, err := normalizeInstallDir(location); err == nil {
					key.Close()
					return normalized
				}
			}
		}
		key.Close()
	}
	return defaultInstallDir(os.Getenv("LOCALAPPDATA"))
}

type browseInfo struct {
	Owner, Root        uintptr
	DisplayName, Title *uint16
	Flags              uint32
	Callback           uintptr
	Param              *uint16
	Image              int32
}

var browseCallback = windows.NewCallback(func(hwnd uintptr, message uint32, lParam uintptr, initial *uint16) uintptr {
	if message == 1 && initial != nil { // BFFM_INITIALIZED / BFFM_SETSELECTIONW
		sendMessage.Call(hwnd, 0x467, 1, uintptr(unsafe.Pointer(initial)))
	}
	return 0
})

// Called from a posted UI task, after the WebView message callback has returned.
// SHBrowseForFolder owns its modal message pump and is the only system dialog.
func chooseDirectory(hwnd uintptr, current string) (string, bool) {
	var display [windows.MAX_PATH]uint16
	current = nearestExistingDirectory(cleanPathInput(current))
	initial, _ := windows.UTF16PtrFromString(current)
	info := browseInfo{
		Owner: hwnd, DisplayName: &display[0], Title: windows.StringToUTF16Ptr("选择 Deep Legends 的安装位置"),
		Flags: 0x0001 | 0x0010 | 0x0040, Callback: browseCallback, Param: initial,
	}
	pidl, _, _ := shell32.NewProc("SHBrowseForFolderW").Call(uintptr(unsafe.Pointer(&info)))
	runtime.KeepAlive(initial)
	if pidl == 0 {
		return "", false
	}
	defer windows.NewLazySystemDLL("ole32.dll").NewProc("CoTaskMemFree").Call(pidl)
	var selected [windows.MAX_PATH]uint16
	if ok, _, _ := shell32.NewProc("SHGetPathFromIDListW").Call(pidl, uintptr(unsafe.Pointer(&selected[0]))); ok == 0 {
		return "", false
	}
	return appendProductFolder(windows.UTF16ToString(selected[:])), true
}

func nearestExistingDirectory(path string) string {
	for path != "" && path != "." {
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			return path
		}
		parent := filepath.Dir(path)
		if parent == path {
			break
		}
		path = parent
	}
	return ""
}

func diskFreeBytes(path string) (int64, error) {
	parent := nearestExistingDirectory(path)
	if parent == "" {
		return -1, errors.New("disk unavailable")
	}
	name, err := windows.UTF16PtrFromString(parent)
	if err != nil {
		return -1, err
	}
	var free uint64
	if err := windows.GetDiskFreeSpaceEx(name, &free, nil, nil); err != nil {
		return -1, err
	}
	return int64(min(free, 1<<63-1)), nil
}

func checkPath(input string, installedBytes int64) pathMessage {
	result := pathMessage{FreeBytes: -1}
	dest, err := normalizeInstallDir(input)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	free, err := diskFreeBytes(dest)
	if err != nil {
		result.Error = "无法读取磁盘空间，请选择可用的本机磁盘"
		return result
	}
	result.FreeBytes = free
	if free < requiredSpace(installedBytes) {
		result.Error = "磁盘空间不足，请换一个安装位置"
		return result
	}
	result.OK = true
	return result
}
