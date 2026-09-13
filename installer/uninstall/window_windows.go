package main

import (
	"fmt"
	"sync"
	"time"

	"deeplegends/installer/internal/webviewhost"
	"unsafe"

	"github.com/jchv/go-webview2/pkg/edge"
	"golang.org/x/sys/windows"
)

const (
	windowClass      = "DeepLegendsUninstallShellWindow"
	WS_EX_APPWINDOW  = 0x00040000
	WS_POPUP         = 0x80000000
	WS_SYSMENU       = 0x00080000
	WS_MINIMIZEBOX   = 0x00020000
	WS_CLIPCHILDREN  = 0x02000000
	CS_HREDRAW       = 0x0002
	CS_VREDRAW       = 0x0001
	CS_DROPSHADOW    = 0x00020000
	WM_SIZE          = 0x0005
	WM_CLOSE         = 0x0010
	WM_DESTROY       = 0x0002
	WM_ERASEBKGND    = 0x0014
	WM_DPICHANGED    = 0x02E0
	WM_APP_TASK      = 0x8001
	WM_NCLBUTTONDOWN = 0x00A1
	HTCAPTION        = 2
	SW_HIDE          = 0
	SW_SHOW          = 5
	SW_MINIMIZE      = 6
	SW_RESTORE       = 9
)

var (
	user32              = windows.NewLazySystemDLL("user32.dll")
	kernel32            = windows.NewLazySystemDLL("kernel32.dll")
	showWindow          = user32.NewProc("ShowWindow")
	setForegroundWindow = user32.NewProc("SetForegroundWindow")
	postMessage         = user32.NewProc("PostMessageW")
	sendMessage         = user32.NewProc("SendMessageW")
	destroyWindow       = user32.NewProc("DestroyWindow")
	setWindowPos        = user32.NewProc("SetWindowPos")
	defWindowProc       = user32.NewProc("DefWindowProcW")
	activeWindow        *shellWindow
	wndProcCallback     = windows.NewCallback(shellWndProc)
)

type winRect struct{ Left, Top, Right, Bottom int32 }
type winPoint struct{ X, Y int32 }
type winMessage struct {
	Window  uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Point   winPoint
	Private uint32
}

type windowClassEx struct {
	Size                               uint32
	Style                              uint32
	WndProc                            uintptr
	ClassExtra, WindowExtra            int32
	Instance, Icon, Cursor, Background uintptr
	MenuName, ClassName                *uint16
	SmallIcon                          uintptr
}

type shellWindow struct {
	surface      *webviewhost.WindowsSurface
	startup      *webviewhost.Session
	startupTimer *time.Timer
	hwnd         uintptr
	chromium     *edge.Chromium
	state        uninstallState
	queueMu      sync.Mutex
	queue        []func()
	closed       bool
}

func enableDPIAwareness() {
	proc := user32.NewProc("SetProcessDpiAwarenessContext")
	if proc.Find() == nil {
		if ok, _, _ := proc.Call(^uintptr(3)); ok != 0 { // -4: PER_MONITOR_AWARE_V2
			return
		}
	}
	proc = windows.NewLazySystemDLL("shcore.dll").NewProc("SetProcessDpiAwareness")
	if proc.Find() == nil {
		if result, _, _ := proc.Call(2); result == 0 {
			return
		}
	}
	user32.NewProc("SetProcessDPIAware").Call()
}

func createShellWindow() (*shellWindow, error) {
	instance, _, _ := kernel32.NewProc("GetModuleHandleW").Call(0)
	icon, _, _ := user32.NewProc("LoadIconW").Call(instance, windowIconResourceID)
	cursor, _, _ := user32.NewProc("LoadCursorW").Call(0, 32512)
	className := windows.StringToUTF16Ptr(windowClass)
	background, _, _ := windows.NewLazySystemDLL("gdi32.dll").NewProc("CreateSolidBrush").Call(0x00140E0B)
	class := windowClassEx{
		Style:   CS_HREDRAW | CS_VREDRAW | CS_DROPSHADOW,
		WndProc: wndProcCallback, Instance: instance,
		Icon: icon, SmallIcon: icon, Cursor: cursor, ClassName: className, Background: background,
	}
	class.Size = uint32(unsafe.Sizeof(class))
	if atom, _, err := user32.NewProc("RegisterClassExW").Call(uintptr(unsafe.Pointer(&class))); atom == 0 {
		return nil, fmt.Errorf("register installer window: %w", err)
	}
	dpi := uintptr(96)
	if proc := user32.NewProc("GetDpiForSystem"); proc.Find() == nil {
		if value, _, _ := proc.Call(); value != 0 {
			dpi = value
		}
	}
	width, height := int32((700*dpi+48)/96), int32((460*dpi+48)/96)
	var work winRect
	if ok, _, _ := user32.NewProc("SystemParametersInfoW").Call(0x0030, 0, uintptr(unsafe.Pointer(&work)), 0); ok == 0 {
		work.Right, work.Bottom = width, height
	}
	x, y := work.Left+(work.Right-work.Left-width)/2, work.Top+(work.Bottom-work.Top-height)/2
	w := &shellWindow{}
	activeWindow = w
	hwnd, _, err := user32.NewProc("CreateWindowExW").Call(
		WS_EX_APPWINDOW, uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(windows.StringToUTF16Ptr("Deep Legends 卸载程序"))),
		WS_POPUP|WS_SYSMENU|WS_MINIMIZEBOX|WS_CLIPCHILDREN,
		uintptr(x), uintptr(y), uintptr(width), uintptr(height), 0, 0, instance, 0,
	)
	if hwnd == 0 {
		return nil, fmt.Errorf("create installer window: %w", err)
	}
	w.hwnd = hwnd
	if proc := windows.NewLazySystemDLL("dwmapi.dll").NewProc("DwmSetWindowAttribute"); proc.Find() == nil {
		preference := uint32(2)
		proc.Call(hwnd, 33, uintptr(unsafe.Pointer(&preference)), 4)
	}
	return w, nil
}

// A pointer-typed LPARAM preserves syscall pointer provenance for WM_DPICHANGED.
func shellWndProc(hwnd uintptr, message uint32, wParam uintptr, lParam unsafe.Pointer) uintptr {
	w := activeWindow
	if w != nil {
		switch message {
		case WM_SIZE:
			if w.surface != nil {
				if wParam == 1 {
					_ = w.surface.Visible(false)
				} else {
					_ = w.surface.Prepare()
				}
			}
		case WM_DPICHANGED:
			if lParam != nil {
				r := (*winRect)(lParam)
				setWindowPos.Call(hwnd, 0, uintptr(r.Left), uintptr(r.Top), uintptr(r.Right-r.Left), uintptr(r.Bottom-r.Top), 0x0014)
			}
			return 0
		case 0x0003: // WM_MOVE
			if w.chromium != nil {
				_ = w.chromium.NotifyParentWindowPositionChanged()
			}
		case 0x0007: // WM_SETFOCUS
			if w.chromium != nil {
				w.chromium.Focus()
			}
		case 0x0018: // WM_SHOWWINDOW: synchronize the child WebView with its parent.
			if w.surface != nil {
				_ = w.surface.Visible(wParam != 0)
			}
		case WM_CLOSE:
			if !w.state.busy() {
				destroyWindow.Call(hwnd)
			}
			return 0
		case WM_DESTROY:
			w.queueMu.Lock()
			w.closed = true
			w.queue = nil
			w.queueMu.Unlock()
			if w.startup != nil {
				w.startup.Cancel()
			}
			if w.startupTimer != nil {
				w.startupTimer.Stop()
			}
			if w.surface != nil {
				w.surface.Close()
			}
			user32.NewProc("PostQuitMessage").Call(0)
			return 0
		case WM_APP_TASK:
			w.drainUI()
			return 0
		}
	}
	result, _, _ := defWindowProc.Call(hwnd, uintptr(message), wParam, uintptr(lParam))
	return result
}

func (w *shellWindow) dispatch(fn func()) {
	w.queueMu.Lock()
	defer w.queueMu.Unlock()
	if w.closed {
		return
	}
	w.queue = append(w.queue, fn)
	postMessage.Call(w.hwnd, WM_APP_TASK, 0, 0)
}

func (w *shellWindow) drainUI() {
	w.queueMu.Lock()
	queue := w.queue
	w.queue = nil
	w.queueMu.Unlock()
	for _, fn := range queue {
		fn()
	}
}

func (w *shellWindow) drag() {
	user32.NewProc("ReleaseCapture").Call()
	sendMessage.Call(w.hwnd, WM_NCLBUTTONDOWN, HTCAPTION, 0)
}

func (w *shellWindow) show() {
	showWindow.Call(w.hwnd, SW_SHOW)
	setForegroundWindow.Call(w.hwnd)
	w.chromium.Focus()
}

func runMessageLoop() {
	var message winMessage
	getMessage := user32.NewProc("GetMessageW")
	for {
		result, _, _ := getMessage.Call(uintptr(unsafe.Pointer(&message)), 0, 0, 0)
		if int32(result) <= 0 {
			return
		}
		user32.NewProc("TranslateMessage").Call(uintptr(unsafe.Pointer(&message)))
		user32.NewProc("DispatchMessageW").Call(uintptr(unsafe.Pointer(&message)))
	}
}

func acquireSingleInstance() (windows.Handle, bool, error) {
	mutex, err := windows.CreateMutex(nil, true, windows.StringToUTF16Ptr(`Global\DeepLegendsUninstallShell`))
	if err == windows.ERROR_ALREADY_EXISTS {
		if mutex != 0 {
			windows.CloseHandle(mutex)
		}
		hwnd, _, _ := user32.NewProc("FindWindowW").Call(uintptr(unsafe.Pointer(windows.StringToUTF16Ptr(windowClass))), 0)
		if hwnd != 0 {
			showWindow.Call(hwnd, SW_RESTORE)
			setForegroundWindow.Call(hwnd)
		}
		return 0, false, nil
	}
	return mutex, err == nil, err
}
