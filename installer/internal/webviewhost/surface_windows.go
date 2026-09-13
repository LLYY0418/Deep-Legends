package webviewhost

import (
	"errors"
	"fmt"
	"runtime"
	"unsafe"

	"github.com/jchv/go-webview2/pkg/edge"
	"golang.org/x/sys/windows"
)

// COM reports HRESULT, not the thread's Win32 GetLastError value.
//
//go:uintptrescapes
func comCall(object unsafe.Pointer, slot int, args ...uintptr) error {
	if object == nil {
		return errors.New("WebView COM interface is unavailable")
	}
	vtable := *(*unsafe.Pointer)(object)
	method := *(*edge.ComProc)(unsafe.Add(vtable, uintptr(slot)*unsafe.Sizeof(uintptr(0))))
	result, _, _ := method.Call(append([]uintptr{uintptr(object)}, args...)...)
	runtime.KeepAlive(object)
	if int32(result) < 0 {
		return fmt.Errorf("HRESULT 0x%08X", uint32(result))
	}
	return nil
}

type rect struct{ Left, Top, Right, Bottom int32 }

type WindowsSurface struct {
	Chromium   *edge.Chromium
	Window     uintptr
	ShowWindow func()
}

func (s *WindowsSurface) Resize() error {
	var bounds rect
	if ok, _, err := windows.NewLazySystemDLL("user32.dll").NewProc("GetClientRect").Call(s.Window, uintptr(unsafe.Pointer(&bounds))); ok == 0 {
		return err
	}
	if bounds.Right <= bounds.Left || bounds.Bottom <= bounds.Top {
		return errors.New("WebView client area is empty")
	}
	return comCall(unsafe.Pointer(s.Chromium.GetController()), controllerPutBounds, uintptr(unsafe.Pointer(&bounds)))
}
func (s *WindowsSurface) Visible(visible bool) error {
	value := uintptr(0)
	if visible {
		value = 1
	}
	return comCall(unsafe.Pointer(s.Chromium.GetController()), controllerPutVisible, value)
}
func (s *WindowsSurface) Prepare() error {
	if err := s.Resize(); err != nil {
		return err
	}
	return s.Visible(true)
}

func coreFromController(controller unsafe.Pointer) (unsafe.Pointer, error) {
	var core unsafe.Pointer
	if err := comCall(controller, controllerGetCore, uintptr(unsafe.Pointer(&core))); err != nil {
		return nil, fmt.Errorf("get_CoreWebView2: %w", err)
	}
	if core == nil {
		return nil, errors.New("get_CoreWebView2 returned success without an interface")
	}
	return core, nil
}

func (s *WindowsSurface) Navigate(html string) error {
	core, err := coreFromController(unsafe.Pointer(s.Chromium.GetController()))
	if err != nil {
		return err
	}
	defer comCall(core, unknownRelease) // Release the GetCoreWebView2 reference.
	text, err := windows.UTF16PtrFromString(html)
	if err != nil {
		return err
	}
	err = comCall(core, coreNavigateToString, uintptr(unsafe.Pointer(text)))
	runtime.KeepAlive(text)
	return err
}
func (s *WindowsSurface) Show() error {
	s.ShowWindow()
	// Creating the controller under a hidden parent and revealing only the
	// parent is insufficient. Synchronize both its bounds and its visibility.
	if err := s.Prepare(); err != nil {
		return err
	}
	var visible int32
	if err := comCall(unsafe.Pointer(s.Chromium.GetController()), controllerGetVisible, uintptr(unsafe.Pointer(&visible))); err != nil {
		return err
	}
	if visible == 0 {
		return errors.New("WebView controller remained hidden")
	}
	return nil
}
func (s *WindowsSurface) Close() {
	if s.Chromium.GetController() != nil {
		_ = comCall(unsafe.Pointer(s.Chromium.GetController()), controllerClose)
	}
}
func NavigationResult(args *edge.ICoreWebView2NavigationCompletedEventArgs) error {
	var success int32
	if err := comCall(unsafe.Pointer(args), navigationGetSuccess, uintptr(unsafe.Pointer(&success))); err != nil {
		return err
	}
	if success != 0 {
		return nil
	}
	var status int32
	if err := comCall(unsafe.Pointer(args), navigationGetError, uintptr(unsafe.Pointer(&status))); err != nil {
		return err
	}
	return fmt.Errorf("WebView navigation failed (WebErrorStatus=%d)", status)
}
