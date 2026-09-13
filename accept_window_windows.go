//go:build windows

package main

import (
	"golang.org/x/sys/windows"
	"path/filepath"
	"strings"
	"unsafe"
)

var acceptUser32 = windows.NewLazySystemDLL("user32.dll")
var acceptGetForeground = acceptUser32.NewProc("GetForegroundWindow")
var acceptGetPID = acceptUser32.NewProc("GetWindowThreadProcessId")

func nativeAcceptWindowState() acceptWindowSnapshot {
	hwnd, _, _ := acceptGetForeground.Call()
	state := acceptWindowSnapshot{window: hwnd, category: "unknown"}
	if hwnd == 0 {
		return state
	}
	state.category = acceptWindowCategory(hwnd)
	return state
}

func acceptWindowCategory(hwnd uintptr) string {
	var pid uint32
	acceptGetPID.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	process, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return "unknown"
	}
	defer windows.CloseHandle(process)
	buffer := make([]uint16, 1024)
	size := uint32(len(buffer))
	if windows.QueryFullProcessImageName(process, 0, &buffer[0], &size) != nil {
		return "unknown"
	}
	switch strings.ToLower(filepath.Base(windows.UTF16ToString(buffer[:size]))) {
	case "leagueclientux.exe", "leagueclient.exe":
		return "league-client"
	case "riotclientservices.exe", "riotclientux.exe":
		return "riot-client"
	case "league of legends.exe":
		return "league-game"
	default:
		return "other"
	}
}

var acceptTopWindow = acceptUser32.NewProc("GetTopWindow")
var acceptNextWindow = acceptUser32.NewProc("GetWindow")
var acceptVisible = acceptUser32.NewProc("IsWindowVisible")
var acceptIconic = acceptUser32.NewProc("IsIconic")
var acceptRect = acceptUser32.NewProc("GetWindowRect")
var acceptStyle = acceptUser32.NewProc("GetWindowLongW")
var acceptLastInput = acceptUser32.NewProc("GetLastInputInfo")
var acceptTicks = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetTickCount")

// Read-only bounded top-level traversal, top to bottom. Never query titles,
// command lines, text, screenshots, keyboard contents or authentication data.
func nativeAcceptWindowDetails() map[string]any {
	fg := nativeAcceptWindowState()
	result := map[string]any{"platform": "windows", "foreground": fg.category, "observation_available": fg.window != 0, "window_action": "none"}
	info := struct{ Size, Tick uint32 }{Size: 8}
	result["last_input_available"] = false
	if ok, _, _ := acceptLastInput.Call(uintptr(unsafe.Pointer(&info))); ok != 0 {
		result["last_input_available"] = true
		now, _, _ := acceptTicks.Call()
		result["last_input_age_ms"] = uint32(now) - info.Tick
	}
	rows := []map[string]any{}
	hwnd, _, _ := acceptTopWindow.Call(0)
	visited := map[uintptr]bool{}
	unclassified := 0
	for rank := 0; hwnd != 0 && rank < 256; rank++ {
		if visited[hwnd] {
			result["traversal_changed"] = true
			break
		}
		visited[hwnd] = true
		category := acceptWindowCategory(hwnd)
		if category == "unknown" {
			unclassified++
		}
		if category == "league-client" || category == "riot-client" || category == "league-game" {
			visible, _, _ := acceptVisible.Call(hwnd)
			iconic, _, _ := acceptIconic.Call(hwnd)
			index := int32(-20)
			style, _, _ := acceptStyle.Call(hwnd, uintptr(index))
			row := map[string]any{"category": category, "z_rank": rank, "foreground": hwnd == fg.window, "visible": visible != 0, "minimized": iconic != 0, "topmost": style&8 != 0}
			rect := struct{ Left, Top, Right, Bottom int32 }{}
			row["rect_available"] = false
			if ok, _, _ := acceptRect.Call(hwnd, uintptr(unsafe.Pointer(&rect))); ok != 0 {
				row["rect_available"] = true
				row["rect"] = []int32{rect.Left, rect.Top, rect.Right, rect.Bottom}
			}
			rows = append(rows, row)
			if len(rows) >= 16 {
				result["window_limit_reached"] = true
				break
			}
		}
		hwnd, _, _ = acceptNextWindow.Call(hwnd, 2) // GW_HWNDNEXT
	}
	result["traversed_windows"] = len(visited)
	result["unclassified_windows"] = unclassified
	result["traversal_limit_reached"] = len(visited) >= 256
	result["client_windows"] = rows
	return result
}
