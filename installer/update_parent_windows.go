package main

import (
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// 0.12.63 installed updates do not supply a PID. Match the exact destination
// executable and a visible application window, never a process name alone.
func legacyUpgradeParent(destination, exeName string) uint32 {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return 0
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	for err = windows.Process32First(snapshot, &entry); err == nil; err = windows.Process32Next(snapshot, &entry) {
		if !strings.EqualFold(windows.UTF16ToString(entry.ExeFile[:]), exeName) {
			continue
		}
		process, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, entry.ProcessID)
		if err != nil {
			continue
		}
		buffer := make([]uint16, 32768)
		size := uint32(len(buffer))
		err = windows.QueryFullProcessImageName(process, 0, &buffer[0], &size)
		windows.CloseHandle(process)
		if err == nil && strings.EqualFold(filepath.Clean(windows.UTF16ToString(buffer[:size])), filepath.Join(destination, exeName)) && hasApplicationWindow(entry.ProcessID) {
			return entry.ProcessID
		}
	}
	return 0
}

type legacyWindowOperation struct {
	pid         uint32
	show, close bool
}

var legacyUpdateWindowCallback = windows.NewCallback(func(hwnd uintptr, operation *legacyWindowOperation) uintptr {
	var pid uint32
	windowProcessID.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	if pid == operation.pid {
		mode := uintptr(0)
		if operation.show {
			mode = SW_RESTORE
		}
		showWindow.Call(hwnd, mode)
		if operation.close {
			postMessage.Call(hwnd, WM_CLOSE, 0, 0)
		}
	}
	return 1
})

func legacyUpdateWindows(pid uint32, show, close bool) {
	if pid != 0 {
		operation := legacyWindowOperation{pid, show, close}
		enumApplicationWindows.Call(legacyUpdateWindowCallback, uintptr(unsafe.Pointer(&operation)))
	}
}
