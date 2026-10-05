//go:build windows

package main

import (
	"strings"
	"syscall"
	"unsafe"
)

// Only process names and window IDs stay in memory; never inspect account args.
func focusRunningRiotClient() (running, focused bool) {
	snapshot, err := syscall.CreateToolhelp32Snapshot(syscall.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return false, false
	}
	defer syscall.CloseHandle(snapshot)
	entry := syscall.ProcessEntry32{Size: uint32(unsafe.Sizeof(syscall.ProcessEntry32{}))}
	pids := map[uint32]bool{}
	if syscall.Process32First(snapshot, &entry) != nil {
		return false, false
	}
	for {
		name := syscall.UTF16ToString(entry.ExeFile[:])
		if strings.EqualFold(name, "RiotClientServices.exe") || strings.EqualFold(name, "RiotClientUx.exe") {
			pids[entry.ProcessID] = true
		}
		if syscall.Process32Next(snapshot, &entry) != nil {
			break
		}
	}
	if len(pids) == 0 {
		return false, false
	}
	user := syscall.NewLazyDLL("user32.dll")
	getPID := user.NewProc("GetWindowThreadProcessId")
	visible := user.NewProc("IsWindowVisible")
	show := user.NewProc("ShowWindow")
	foreground := user.NewProc("SetForegroundWindow")
	callback := syscall.NewCallback(func(hwnd, _ uintptr) uintptr {
		var pid uint32
		getPID.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
		isVisible, _, _ := visible.Call(hwnd)
		if pids[pid] && isVisible != 0 {
			show.Call(hwnd, 9)
			ok, _, _ := foreground.Call(hwnd)
			if ok != 0 {
				focused = true
				return 0
			}
		}
		return 1
	})
	user.NewProc("EnumWindows").Call(callback, 0)
	return true, focused
}
