//go:build windows

// Package p11callbacktest holds the native fixture shared by both shell tests.
package p11callbacktest

import (
	"context"
	"os"
	"os/exec"
	"reflect"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/jchv/go-webview2/pkg/edge"
	"golang.org/x/sys/windows"
)

//go:noinline
func growStack(depth int) byte {
	var frame [4096]byte
	frame[0] = byte(depth)
	if depth > 0 {
		frame[1] = growStack(depth - 1)
	}
	runtime.KeepAlive(&frame)
	return frame[0] ^ frame[1]
}

// Run invokes the actual production windows.NewCallback thunk, not a Go call or
// a lookalike signature. WM_SHOWWINDOW calls our COM visibility method before
// DefWindowProc consumes LPARAM. A separate parent-window reentry probe forces
// stack adjustment while default processing keeps a local pointer temporary live;
// incoming arguments alone do not receive the runtime low-pointer fatal check.
// The subprocess contains a fatal runtime error and forces invalidptr checking
// even when the surrounding CI did not explicitly set GODEBUG.
func Run(t *testing.T, callback uintptr, configure func(*edge.Chromium)) {
	t.Helper()
	if os.Getenv("R277_P11_CALLBACK_CHILD") != "1" {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^"+t.Name()+"$", "-test.v")
		cmd.Env = append(os.Environ(), "R277_P11_CALLBACK_CHILD=1", "GODEBUG="+os.Getenv("GODEBUG")+",invalidptr=1")
		output, err := cmd.CombinedOutput()
		t.Logf("native callback child:\n%s", output)
		if err != nil || ctx.Err() != nil {
			t.Fatalf("native callback GC child failed: %v (deadline: %v)", err, ctx.Err())
		}
		if !strings.Contains(string(output), "R277_P11_NATIVE_PASS calls=1024 gc=1024 dpi=PASS default_reentry=1 default_gc=1") {
			t.Fatal("native fixture completion proof missing")
		}
		return
	}

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	user := windows.NewLazySystemDLL("user32.dll")
	hwnd, _, err := user.NewProc("CreateWindowExW").Call(0,
		uintptr(unsafe.Pointer(windows.StringToUTF16Ptr("STATIC"))),
		uintptr(unsafe.Pointer(windows.StringToUTF16Ptr("R277 P11 callback fixture"))),
		0x80000000, 10, 20, 120, 80, 0, 0, 0, 0)
	if hwnd == 0 {
		t.Fatalf("create hidden native fixture window: %v", err)
	}
	defer user.NewProc("DestroyWindow").Call(hwnd)

	// Derive the slot and controller-field offset from the installed dependency,
	// never assume a copied vtable index or Chromium memory layout.
	layout := reflect.TypeOf(edge.ICoreWebView2Controller{}).Field(0).Type.Elem()
	visible, ok := layout.FieldByName("PutIsVisible")
	if !ok || len(visible.Index) != 1 {
		t.Fatal("dependency PutIsVisible ABI changed")
	}
	table := make([]uintptr, layout.Size()/unsafe.Sizeof(uintptr(0)))
	calls := 0
	table[visible.Offset/unsafe.Sizeof(uintptr(0))] = windows.NewCallback(func(_ uintptr, value uintptr) uintptr {
		if value != 1 {
			t.Fatal("WM_SHOWWINDOW visibility behavior changed")
		}
		calls++
		growStack(32)
		runtime.GC() // Shallow GC permits stack shrinking after recursion unwinds.
		return 0
	})
	object := struct{ table *uintptr }{&table[0]}
	chromium := &edge.Chromium{}
	field, ok := reflect.TypeOf(*chromium).FieldByName("controller")
	if !ok || field.Type != reflect.TypeOf((*edge.ICoreWebView2Controller)(nil)) {
		t.Fatal("dependency Chromium controller layout changed")
	}
	*(**edge.ICoreWebView2Controller)(unsafe.Add(unsafe.Pointer(chromium), field.Offset)) = (*edge.ICoreWebView2Controller)(unsafe.Pointer(&object))
	configure(chromium)

	for round := 0; round < 256; round++ {
		for _, lParam := range []uintptr{0x60, 1, 0x7f, 0x123} {
			got, _, _ := syscall.SyscallN(callback, hwnd, 0x0018, 1, lParam)
			want, _, _ := user.NewProc("DefWindowProcW").Call(hwnd, 0x0018, 1, lParam)
			if got != want {
				t.Fatalf("default processing changed: got %d want %d", got, want)
			}
		}
	}
	if calls != 1024 {
		t.Fatalf("GC callback count %d, want exactly 1024", calls)
	}

	// A real message-owned RECT must still resize/move the window identically.
	// LocalAlloc supplies native memory, independent of Go checkptr/provenance.
	memory, err := windows.LocalAlloc(0x40, 16)
	if err != nil || memory == 0 {
		t.Fatalf("allocate native RECT: %v", err)
	}
	defer windows.LocalFree(windows.Handle(memory))
	rect := [4]int32{30, 40, 230, 140}
	var written uintptr
	if err := windows.WriteProcessMemory(windows.CurrentProcess(), memory, (*byte)(unsafe.Pointer(&rect)), 16, &written); err != nil || written != 16 {
		t.Fatalf("write native RECT: %v bytes=%d", err, written)
	}
	syscall.SyscallN(callback, hwnd, 0x02e0, 0, memory)
	var gotRect [4]int32
	if ok, _, err := user.NewProc("GetWindowRect").Call(hwnd, uintptr(unsafe.Pointer(&gotRect))); ok == 0 || gotRect != rect {
		t.Fatalf("WM_DPICHANGED behavior changed: got %v want %v error=%v", gotRect, rect, err)
	}
	syscall.SyscallN(callback, hwnd, 0x02e0, 0, 0) // Null suggested RECT remains a no-op.

	// DefWindowProc forwards WM_SETCURSOR from a child to its parent. Subclass
	// only the fixture parent: this forces actual native reentry while the
	// production shellWndProc is suspended in its default-processing Call.
	// With the old signature, uintptr(lParam) there retains a GC-scanned local
	// unsafe.Pointer temporary. Unlike an incoming argument, stack adjustment
	// validates that local and must reject the scalar 0x60. Return TRUE so the
	// deliberately scalar hit-test value does not drive further cursor work.
	child, _, err := user.NewProc("CreateWindowExW").Call(0,
		uintptr(unsafe.Pointer(windows.StringToUTF16Ptr("STATIC"))),
		uintptr(unsafe.Pointer(windows.StringToUTF16Ptr("R277 P11 child"))),
		0x40000000, 0, 0, 20, 20, hwnd, 0, 0, 0)
	if child == 0 {
		t.Fatalf("create native child fixture window: %v", err)
	}
	defer user.NewProc("DestroyWindow").Call(child)
	var previousProc uintptr
	defaultReentry := 0
	parentProc := windows.NewCallback(func(parent uintptr, message uint32, wParam uintptr, lParam uintptr) uintptr {
		if message == 0x0020 && wParam == child { // WM_SETCURSOR
			defaultReentry++
			growStack(32)
			runtime.GC() // After unwinding: exercise both growth and shrink adjustment.
			return 1
		}
		result, _, _ := user.NewProc("CallWindowProcW").Call(previousProc, parent, uintptr(message), wParam, lParam)
		return result
	})
	setProc := user.NewProc("SetWindowLongPtrW")
	previousProc, _, err = setProc.Call(hwnd, ^uintptr(3), parentProc) // GWLP_WNDPROC = -4
	if previousProc == 0 {
		t.Fatalf("subclass native parent fixture: %v", err)
	}
	defer setProc.Call(hwnd, ^uintptr(3), previousProc)
	gotDefault, _, _ := syscall.SyscallN(callback, child, 0x0020, child, 0x60)
	if defaultReentry != 1 || gotDefault != 1 {
		t.Fatalf("production default-processing reentry missing: calls=%d result=%d", defaultReentry, gotDefault)
	}
	runtime.KeepAlive(chromium)
	runtime.KeepAlive(object)
	runtime.KeepAlive(table)
	t.Log("R277_P11_NATIVE_PASS calls=1024 gc=1024 dpi=PASS default_reentry=1 default_gc=1")
}
