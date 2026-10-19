//go:build windows

package main

import (
	"errors"
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"unsafe"
)

type shellCOMObject struct{ vtable *[8]uintptr }

func (o *shellCOMObject) call(slot int, args ...uintptr) uintptr {
	r, _, _ := syscall.SyscallN(o.vtable[slot], append([]uintptr{uintptr(unsafe.Pointer(o))}, args...)...)
	return r
}
func shellHRESULTFailed(r uintptr) bool { return int32(r) < 0 }

// IShellLinkW::GetPath after IPersistFile::Load. Do not execute or resolve the
// shortcut, and never infer its target from the shortcut's display name.
func shellLinkTarget(path string) (string, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	ole := windows.NewLazySystemDLL("ole32.dll")
	hr, _, _ := ole.NewProc("CoInitializeEx").Call(0, 2)
	if shellHRESULTFailed(hr) {
		return "", errors.New("shell-link COM unavailable")
	}
	defer ole.NewProc("CoUninitialize").Call()
	clsid := windows.GUID{Data1: 0x00021401, Data4: [8]byte{0xc0, 0, 0, 0, 0, 0, 0, 0x46}}
	linkID := windows.GUID{Data1: 0x000214f9, Data4: [8]byte{0xc0, 0, 0, 0, 0, 0, 0, 0x46}}
	persistID := windows.GUID{Data1: 0x0000010b, Data4: [8]byte{0xc0, 0, 0, 0, 0, 0, 0, 0x46}}
	var link, persist *shellCOMObject
	hr, _, _ = ole.NewProc("CoCreateInstance").Call(uintptr(unsafe.Pointer(&clsid)), 0, 1, uintptr(unsafe.Pointer(&linkID)), uintptr(unsafe.Pointer(&link)))
	if shellHRESULTFailed(hr) || link == nil {
		return "", errors.New("shell-link create failed")
	}
	defer link.call(2)
	if shellHRESULTFailed(link.call(0, uintptr(unsafe.Pointer(&persistID)), uintptr(unsafe.Pointer(&persist)))) || persist == nil {
		return "", errors.New("shell-link persist unavailable")
	}
	defer persist.call(2)
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", err
	}
	if shellHRESULTFailed(persist.call(5, uintptr(unsafe.Pointer(name)), 0)) {
		return "", errors.New("shell-link load failed")
	}
	buffer := make([]uint16, 32768)
	if shellHRESULTFailed(link.call(3, uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer)), 0, 4)) {
		return "", errors.New("shell-link path unavailable")
	}
	target := windows.UTF16ToString(buffer)
	if !filepath.IsAbs(target) {
		return "", errors.New("shell-link target unresolved")
	}
	return target, nil
}

func inspectWindowsFileVersion(path string) clientFileVersion {
	info, err := os.Stat(path)
	v := clientFileVersion{}
	if err != nil || !info.Mode().IsRegular() {
		return v
	}
	v.Exists, v.Modified = true, info.ModTime()
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return v
	}
	dll := windows.NewLazySystemDLL("version.dll")
	var ignored uint32
	size, _, _ := dll.NewProc("GetFileVersionInfoSizeW").Call(uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(&ignored)))
	if size < 52 || size > 16<<20 {
		return v
	}
	buffer := make([]byte, int(size))
	ok, _, _ := dll.NewProc("GetFileVersionInfoW").Call(uintptr(unsafe.Pointer(name)), 0, size, uintptr(unsafe.Pointer(&buffer[0])))
	if ok == 0 {
		return v
	}
	root, _ := windows.UTF16PtrFromString(`\`)
	var fixed *[13]uint32
	var length uint32
	ok, _, _ = dll.NewProc("VerQueryValueW").Call(uintptr(unsafe.Pointer(&buffer[0])), uintptr(unsafe.Pointer(root)), uintptr(unsafe.Pointer(&fixed)), uintptr(unsafe.Pointer(&length)))
	if ok == 0 || fixed == nil || length < 52 || fixed[0] != 0xFEEF04BD {
		return v
	}
	v.Parts = [4]uint16{uint16(fixed[2] >> 16), uint16(fixed[2]), uint16(fixed[3] >> 16), uint16(fixed[3])}
	v.Known = true
	runtime.KeepAlive(buffer)
	return v
}
