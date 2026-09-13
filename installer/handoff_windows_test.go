package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Run on an interactive Windows desktop. Unlike the portable timing tests,
// this exercises the actual EnumWindows callback ABI and native visibility/PID.
func TestWindowsApplicationWindowDetection(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	hwnd, _, err := user32.NewProc("CreateWindowExW").Call(0,
		uintptr(unsafe.Pointer(windows.StringToUTF16Ptr("STATIC"))),
		uintptr(unsafe.Pointer(windows.StringToUTF16Ptr("R82 window detection test"))),
		WS_POPUP, 0, 0, 460, 292, 0, 0, 0, 0)
	if hwnd == 0 {
		t.Fatalf("create test window: %v", err)
	}
	defer destroyWindow.Call(hwnd)
	pid := uint32(os.Getpid())
	search := windowSearch{pid: pid}
	inspectApplicationWindow(hwnd, &search)
	if search.found {
		t.Fatal("hidden native window counted as visible")
	}
	showWindow.Call(hwnd, SW_SHOW)
	search = windowSearch{pid: pid + 1}
	inspectApplicationWindow(hwnd, &search)
	if search.found {
		t.Fatal("native callback accepted a different PID")
	}
	if !hasApplicationWindow(pid) {
		t.Fatal("EnumWindows did not find the visible splash-sized window")
	}
	if ok, _, err := user32.NewProc("MoveWindow").Call(hwnd, 0, 0, 1, 1, 1); ok == 0 {
		t.Fatalf("resize test window: %v", err)
	}
	search = windowSearch{pid: pid}
	inspectApplicationWindow(hwnd, &search)
	if search.found {
		t.Fatal("native callback accepted a tiny helper window")
	}
}

func TestWindowsMissingApplicationDoesNotYieldAPID(t *testing.T) {
	directory := t.TempDir()
	pid, err := startApplication(filepath.Join(directory, "missing.exe"), directory)
	if err == nil || pid != 0 {
		t.Fatalf("missing application: pid=%d err=%v", pid, err)
	}
}
