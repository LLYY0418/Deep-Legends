package webviewhost

import (
	"runtime"
	"testing"
	"unsafe"

	"github.com/jchv/go-webview2/pkg/edge"
	"golang.org/x/sys/windows"
)

// These ABI checks run with the installer tests on Windows, without installing
// anything or requiring WebView2. Cross compilation alone cannot execute them.
func TestCOMUsesHRESULTInsteadOfThreadLastError(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	var vtable [7]uintptr
	object := struct{ vtable *[7]uintptr }{&vtable}
	vtable[6] = windows.NewCallback(func(uintptr) uintptr {
		windows.NewLazySystemDLL("kernel32.dll").NewProc("SetLastError").Call(5)
		return 0 // S_OK, despite stale Win32 last-error.
	})
	if err := comCall(unsafe.Pointer(&object), 6); err != nil {
		t.Fatal(err)
	}
	vtable[6] = windows.NewCallback(func(uintptr) uintptr { return 0x80004005 })
	if err := comCall(unsafe.Pointer(&object), 6); err == nil {
		t.Fatal("failed HRESULT ignored")
	}
}

func TestFailedNavigationEventIsNotReadiness(t *testing.T) {
	var vtable [6]uintptr
	object := struct{ vtable *[6]uintptr }{&vtable}
	var success int32
	vtable[3] = windows.NewCallback(func(_ uintptr, value *int32) uintptr { *value = success; return 0 })
	vtable[4] = windows.NewCallback(func(_ uintptr, value *int32) uintptr { *value = 13; return 0 })
	args := (*edge.ICoreWebView2NavigationCompletedEventArgs)(unsafe.Pointer(&object))
	if NavigationResult(args) == nil {
		t.Fatal("failed navigation accepted")
	}
	success = 1
	if err := NavigationResult(args); err != nil {
		t.Fatal(err)
	}
}
