package webviewhost

import (
	"fmt"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"unsafe"

	"github.com/jchv/go-webview2/pkg/edge"
	"golang.org/x/sys/windows"
)

type fakeCOMObject struct {
	table  []uintptr
	header struct{ table *uintptr }
	layout reflect.Type
}

func newFakeCOMObject(objectType any) *fakeCOMObject {
	layout := reflect.TypeOf(objectType).Field(0).Type.Elem()
	object := &fakeCOMObject{layout: layout, table: make([]uintptr, layout.Size()/unsafe.Sizeof(uintptr(0)))}
	object.header.table = &object.table[0]
	return object
}

func (object *fakeCOMObject) bind(t *testing.T, method string, callback any) {
	t.Helper()
	field, ok := object.layout.FieldByName(method)
	if !ok {
		t.Fatalf("missing actual dependency method %s", method)
	}
	var offset uintptr
	layout := object.layout
	for _, index := range field.Index {
		member := layout.Field(index)
		offset += member.Offset
		layout = member.Type
	}
	object.table[offset/unsafe.Sizeof(uintptr(0))] = windows.NewCallback(callback)
}

func (object *fakeCOMObject) pointer() unsafe.Pointer { return unsafe.Pointer(&object.header) }

// Exercise the complete settings call chain with native COM callbacks. Success
// leaves a nonzero Win32 last-error; failure deliberately leaves last-error zero.
func TestSettingsUseHRESULTAcrossEntireConfiguration(t *testing.T) {
	for _, failed := range []string{"", "get_CoreWebView2", "get_Settings", "put_IsScriptEnabled", "put_IsWebMessageEnabled", "put_AreDefaultScriptDialogsEnabled", "put_IsStatusBarEnabled", "put_AreDevToolsEnabled", "put_AreDefaultContextMenusEnabled", "put_IsZoomControlEnabled", "put_IsBuiltInErrorPageEnabled"} {
		t.Run(fmt.Sprintf("failure=%s", failed), func(t *testing.T) {
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			setLastError := windows.NewLazySystemDLL("kernel32.dll").NewProc("SetLastError")
			result := func(name string) uintptr {
				if failed == name {
					setLastError.Call(0)
					return 0x80070005 // E_ACCESSDENIED
				}
				setLastError.Call(5)
				return 0 // S_OK
			}
			// Derive every callback address from the actual dependency vtable.
			// Handwritten slot arrays previously copied the production ABI bug.
			controller := newFakeCOMObject(edge.ICoreWebView2Controller{})
			core := newFakeCOMObject(edge.ICoreWebView2{})
			settings := newFakeCOMObject(edge.ICoreWebViewSettings{})
			coreReleases, settingsReleases := 0, 0
			core.bind(t, "Release", func(uintptr) uintptr { coreReleases++; return 1 })
			settings.bind(t, "Release", func(uintptr) uintptr { settingsReleases++; return 1 })
			controller.bind(t, "NotifyParentWindowPositionChanged", func(uintptr) uintptr { return 0 })
			controller.bind(t, "GetCoreWebView2", func(_ uintptr, out *unsafe.Pointer) uintptr {
				if failed != "get_CoreWebView2" {
					*out = core.pointer()
				}
				return result("get_CoreWebView2")
			})
			core.bind(t, "GetSettings", func(_ uintptr, out *unsafe.Pointer) uintptr {
				if failed != "get_Settings" {
					*out = settings.pointer()
				}
				return result("get_Settings")
			})
			calls := map[string]uintptr{}
			for _, name := range []string{
				"put_IsScriptEnabled", "put_IsWebMessageEnabled", "put_AreDefaultScriptDialogsEnabled", "put_IsStatusBarEnabled", "put_AreDevToolsEnabled", "put_AreDefaultContextMenusEnabled", "put_IsZoomControlEnabled", "put_IsBuiltInErrorPageEnabled",
			} {
				settings.bind(t, "Put"+strings.TrimPrefix(name, "put_"), func(_ uintptr, value uintptr) uintptr {
					calls[name] = value
					return result(name)
				})
			}
			err := configureController(controller.pointer())
			runtime.KeepAlive(controller)
			runtime.KeepAlive(core)
			runtime.KeepAlive(settings)
			if failed == "" {
				if err != nil {
					t.Fatalf("successful COM call rejected because of stale last-error: %v", err)
				}
				if len(calls) != 8 {
					t.Fatalf("only configured %d settings", len(calls))
				}
				for name, value := range calls {
					want := uintptr(0)
					if name == "put_IsScriptEnabled" || name == "put_IsWebMessageEnabled" {
						want = 1
					}
					if value != want {
						t.Fatalf("%s=%d, want %d", name, value, want)
					}
				}
			} else if err == nil || !strings.Contains(err.Error(), failed) || !strings.Contains(err.Error(), "0x80070005") {
				t.Fatalf("failed HRESULT lost its operation/code: %v", err)
			}
			wantCore, wantSettings := 1, 1
			if failed == "get_CoreWebView2" {
				wantCore, wantSettings = 0, 0
			} else if failed == "get_Settings" {
				wantSettings = 0
			}
			if coreReleases != wantCore || settingsReleases != wantSettings {
				t.Fatalf("COM references released %d/%d, want %d/%d", coreReleases, settingsReleases, wantCore, wantSettings)
			}
		})
	}
}
