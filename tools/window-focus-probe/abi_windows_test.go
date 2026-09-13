//go:build windows

package main

import (
	"testing"
	"unsafe"
)

func TestWin64ABI(t *testing.T) {
	if unsafe.Sizeof(uintptr(0)) != 8 {
		t.Skip("release targets x64")
	}
	if unsafe.Sizeof(winClass{}) != 80 || unsafe.Sizeof(winMsg{}) != 48 {
		t.Fatal("Windows ABI structure layout mismatch")
	}
	if unsafe.Offsetof(winMsg{}.WParam) != 16 || unsafe.Offsetof(winClass{}.Instance) != 24 {
		t.Fatal("Windows ABI field mismatch")
	}
}
