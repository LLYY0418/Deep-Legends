//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
	"unsafe"
)

var kernel = syscall.NewLazyDLL("kernel32.dll") // Windows KnownDLL.

// Resolve non-KnownDLLs through the OS system directory, not next to a downloaded EXE.
func systemDLL(name string) *syscall.LazyDLL {
	var buf [32768]uint16
	n, _, _ := kernel.NewProc("GetSystemDirectoryW").Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if n == 0 || n >= uintptr(len(buf)) {
		panic("GetSystemDirectoryW failed")
	}
	return syscall.NewLazyDLL(filepath.Join(syscall.UTF16ToString(buf[:n]), name))
}

var (
	user             = systemDLL("user32.dll")
	dwm              = systemDLL("dwmapi.dll")
	gdi              = systemDLL("gdi32.dll")
	registerClass    = user.NewProc("RegisterClassExW")
	createWindow     = user.NewProc("CreateWindowExW")
	defaultProc      = user.NewProc("DefWindowProcW")
	destroyWindow    = user.NewProc("DestroyWindow")
	postMessage      = user.NewProc("PostMessageW")
	getMessage       = user.NewProc("GetMessageW")
	translateMessage = user.NewProc("TranslateMessage")
	dispatchMessage  = user.NewProc("DispatchMessageW")
	postQuit         = user.NewProc("PostQuitMessage")
	showWindow       = user.NewProc("ShowWindow")
	setForeground    = user.NewProc("SetForegroundWindow")
	setActive        = user.NewProc("SetActiveWindow")
	allowForeground  = user.NewProc("AllowSetForegroundWindow")
	setWindowPos     = user.NewProc("SetWindowPos")
	getForeground    = user.NewProc("GetForegroundWindow")
	getPID           = user.NewProc("GetWindowThreadProcessId")
	getWindow        = user.NewProc("GetWindow")
	getStyle         = user.NewProc("GetWindowLongW")
	isVisible        = user.NewProc("IsWindowVisible")
	isIconic         = user.NewProc("IsIconic")
	getLastInput     = user.NewProc("GetLastInputInfo")
	dwmGet           = dwm.NewProc("DwmGetWindowAttribute")
	dwmSet           = dwm.NewProc("DwmSetWindowAttribute")
	setText          = user.NewProc("SetWindowTextW")
	enableWindow     = user.NewProc("EnableWindow")
	sendMessage      = user.NewProc("SendMessageW")
)

const (
	wmDestroy          = 0x0002
	wmClose            = 0x0010
	wmCommand          = 0x0111
	wmApp              = 0x8001
	wmKeyDown          = 0x0100
	wsOverlappedWindow = 0x00cf0000
	wsVisible          = 0x10000000
	wsChild            = 0x40000000
	dwmCloak           = 13
	dwmCloaked         = 14
)

type winClass struct {
	Size, Style                                          uint32
	Proc                                                 uintptr
	ClassExtra, WindowExtra                              int32
	Instance, Icon, Cursor, Brush, Menu, Name, SmallIcon uintptr
}
type winMsg struct {
	HWND           uintptr
	Message        uint32
	WParam, LParam uintptr
	Time           uint32
	X, Y           int32
	Private        uint32
}

func wide(s string) *uint16 { p, _ := syscall.UTF16PtrFromString(s); return p }
func ownPID(hwnd uintptr) uint32 {
	var pid uint32
	getPID.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	return pid
}
func targetOK(hwnd uintptr, expected uint32) bool {
	return hwnd != 0 && expected != 0 && ownPID(hwnd) == expected
}

func cloak(hwnd uintptr, expected uint32, value bool) (bool, string) {
	if !targetOK(hwnd, expected) {
		return false, "invalid_owned_target"
	}
	if err := dwmSet.Find(); err != nil {
		return false, "0x80004001"
	}
	var v uint32
	if value {
		v = 1
	}
	r, _, _ := dwmSet.Call(hwnd, dwmCloak, uintptr(unsafe.Pointer(&v)), unsafe.Sizeof(v))
	return uint32(r) == 0, fmt.Sprintf("0x%08X", uint32(r))
}

func relativeZ(hwnd, anchor uintptr) string {
	if hwnd == 0 || anchor == 0 {
		return "unknown"
	}
	// Read only. Other windows' identities and titles are never collected.
	for _, direction := range []uintptr{3, 2} { // GW_HWNDPREV / GW_HWNDNEXT
		h := hwnd
		for i := 0; i < 256; i++ {
			h, _, _ = getWindow.Call(h, direction)
			if h == 0 || h == hwnd {
				break
			}
			if h == anchor {
				if direction == 3 {
					return "below"
				}
				return "above"
			}
		}
	}
	return "unknown"
}

func sample(hwnd uintptr, childPID, ownerPID uint32, anchor uintptr) snapshot {
	s := snapshot{Valid: targetOK(hwnd, childPID), RelativeZ: "unknown", Foreground: "none", DWMResult: "unavailable"}
	fg, _, _ := getForeground.Call()
	s.ForegroundAnchor = fg != 0 && fg == anchor
	if fg != 0 {
		switch ownPID(fg) {
		case childPID:
			s.Foreground = "probe"
		case ownerPID:
			s.Foreground = "controller"
		default:
			s.Foreground = "other"
		}
	}
	input := struct{ Size, Tick uint32 }{Size: 8}
	ok, _, _ := getLastInput.Call(uintptr(unsafe.Pointer(&input)))
	s.InputOK, s.InputTick = ok != 0, input.Tick
	if !s.Valid {
		return s
	}
	v, _, _ := isVisible.Call(hwnd)
	s.Visible = v != 0
	v, _, _ = isIconic.Call(hwnd)
	s.Minimized = v != 0
	index := int32(-20)
	v, _, _ = getStyle.Call(hwnd, uintptr(index))
	s.Topmost = v&8 != 0
	s.RelativeZ = relativeZ(hwnd, anchor)
	if dwmGet.Find() == nil {
		var bits uint32
		r, _, _ := dwmGet.Call(hwnd, dwmCloaked, uintptr(unsafe.Pointer(&bits)), unsafe.Sizeof(bits))
		s.DWMOK, s.DWMResult, s.Cloaked = uint32(r) == 0, fmt.Sprintf("0x%08X", uint32(r)), bits
	}
	return s
}

func createTopWindow(name, title string, callback uintptr, width, height, offset int) (uintptr, error) {
	instance, _, _ := kernel.NewProc("GetModuleHandleW").Call(0)
	cursor, _, _ := user.NewProc("LoadCursorW").Call(0, 32512)
	brush, _, _ := user.NewProc("GetSysColorBrush").Call(5)
	className := wide(name)
	cls := winClass{Size: uint32(unsafe.Sizeof(winClass{})), Proc: callback, Instance: instance,
		Cursor: cursor, Brush: brush, Name: uintptr(unsafe.Pointer(className))}
	if r, _, err := registerClass.Call(uintptr(unsafe.Pointer(&cls))); r == 0 {
		return 0, fmt.Errorf("RegisterClassExW: %v", err)
	}
	screenW, _, _ := user.NewProc("GetSystemMetrics").Call(0)
	screenH, _, _ := user.NewProc("GetSystemMetrics").Call(1)
	x, y := max(0, (int(screenW)-width)/2+offset), max(0, (int(screenH)-height)/2+offset)
	hwnd, _, err := createWindow.Call(0, uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(wide(title))),
		wsOverlappedWindow, uintptr(x), uintptr(y), uintptr(width), uintptr(height), 0, 0, instance, 0)
	if hwnd == 0 {
		return 0, fmt.Errorf("CreateWindowExW: %v", err)
	}
	return hwnd, nil
}

func control(parent uintptr, kind, text string, style uintptr, x, y, w, h, id int) uintptr {
	hwnd, _, _ := createWindow.Call(0, uintptr(unsafe.Pointer(wide(kind))), uintptr(unsafe.Pointer(wide(text))),
		wsChild|wsVisible|style, uintptr(x), uintptr(y), uintptr(w), uintptr(h), parent, uintptr(id), 0, 0)
	font, _, _ := gdi.NewProc("GetStockObject").Call(17)
	sendMessage.Call(hwnd, 0x0030, font, 1) // WM_SETFONT, DEFAULT_GUI_FONT.
	return hwnd
}

func textWindow(hwnd uintptr, value string) { setText.Call(hwnd, uintptr(unsafe.Pointer(wide(value)))) }

func messageLoop() error {
	var msg winMsg
	for {
		r, _, err := getMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(r) == -1 {
			return fmt.Errorf("GetMessageW: %v", err)
		}
		if r == 0 {
			return nil
		}
		// Esc works while ANY of our child controls has focus.
		if msg.Message == wmKeyDown && msg.WParam == 27 {
			root, _, _ := user.NewProc("GetAncestor").Call(msg.HWND, 2)
			postMessage.Call(root, wmClose, 0, 0)
			continue
		}
		translateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		dispatchMessage.Call(uintptr(unsafe.Pointer(&msg)))
	}
}

func windowsVersion() map[string]any {
	info := struct {
		Size, Major, Minor, Build, Platform uint32
		ServicePack                         [128]uint16
	}{Size: 276}
	r, _, _ := systemDLL("ntdll.dll").NewProc("RtlGetVersion").Call(uintptr(unsafe.Pointer(&info)))
	return map[string]any{"available": uint32(r) == 0, "major": info.Major, "minor": info.Minor, "build": info.Build,
		"controller_pid": os.Getpid(), "sample_interval_ms": 20, "api_snapshots": true}
}

func nowUTC() string { return time.Now().UTC().Format(time.RFC3339Nano) }
