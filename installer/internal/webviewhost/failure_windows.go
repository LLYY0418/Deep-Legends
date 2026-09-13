package webviewhost

import (
	"fmt"
	"log"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Report the failed custom UI without silently switching to another installer.
func ReportStartupFailure(name, operation string, err error) {
	log.Printf("%s界面启动失败: %v", operation, err)
	message := fmt.Sprintf("%s界面未能启动。请将下面的错误和日志发送给开发者。\n\n错误：%v\n\n日志：%s", operation, err, StartupLogPath(name))
	text, _ := windows.UTF16PtrFromString(message)
	title, _ := windows.UTF16PtrFromString("Deep Legends")
	windows.NewLazySystemDLL("user32.dll").NewProc("MessageBoxW").Call(0, uintptr(unsafe.Pointer(text)), uintptr(unsafe.Pointer(title)), 0x10)
}
