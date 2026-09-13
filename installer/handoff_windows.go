package main

import (
	"log"
	"os"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	enumApplicationWindows    = user32.NewProc("EnumWindows")
	windowProcessID           = user32.NewProc("GetWindowThreadProcessId")
	windowIsVisible           = user32.NewProc("IsWindowVisible")
	windowRectangle           = user32.NewProc("GetWindowRect")
	applicationWindowCallback = windows.NewCallback(inspectApplicationWindow)
)

type windowSearch struct {
	pid   uint32
	found bool
}

func inspectApplicationWindow(hwnd uintptr, search *windowSearch) uintptr {
	var pid uint32
	if thread, _, _ := windowProcessID.Call(hwnd, uintptr(unsafe.Pointer(&pid))); thread == 0 || pid != search.pid {
		return 1
	}
	visible, _, _ := windowIsVisible.Call(hwnd)
	var rect winRect
	if ok, _, _ := windowRectangle.Call(hwnd, uintptr(unsafe.Pointer(&rect))); ok == 0 {
		return 1
	}
	if matchesApplicationWindow(search.pid, applicationWindow{PID: pid, Visible: visible != 0, Width: rect.Right - rect.Left, Height: rect.Bottom - rect.Top}) {
		search.found = true
		return 0
	}
	return 1
}

func hasApplicationWindow(pid uint32) bool {
	search := windowSearch{pid: pid}
	enumApplicationWindows.Call(applicationWindowCallback, uintptr(unsafe.Pointer(&search)))
	return search.found
}

func (a *installerApp) handoffApplication(exe, directory string) {
	w := a.window
	if startupPrewarmEnabled(os.Getenv("DEEP_LEGENDS_STARTUP_PREWARM")) {
		progress := func(percent int) {
			w.dispatch(func() {
				w.state.phase = phaseFinishing
				a.emit("progress", progressMessage{Percent: percent, Stage: "正在优化首次启动…"})
			})
		}
		progress(97)
		result, executionStatus := completeStartupWarmup(directory, a.startupWarm, readPrewarmFile, startupPrewarmBudget, progress)
		log.Printf("startup prewarm enabled=true files=%d failures=%d timed_out=%t elapsed_ms=%d execute_valid=%d fallback_files=%d", result.Files, result.Failures, result.TimedOut, result.ElapsedMS, executionStatus.ExecuteValid, len(executionStatus.Paths))
	} else {
		log.Print("startup prewarm enabled=false elapsed_ms=0")
	}
	started := time.Now()
	runApplicationHandoff(handoffHooks{
		ShowStarting: func() { w.dispatch(func() { w.state.phase = phaseFinishing; a.emit("done", nil) }) },
		Start: func() (uint32, error) {
			return logApplicationStart(func() (uint32, error) { return startApplication(exe, directory) }, started)
		},
		HasWindow: hasApplicationWindow,
		LaunchError: func(err error) {
			w.dispatch(func() {
				a.emit("progress", progressMessage{Percent: 100, Stage: "程序未能启动，请稍后从开始菜单重试"})
			})
		},
		CloseInstaller: func(visible bool) {
			finishInstallerHandoff(w.hwnd, visible, installerExitHooks{
				Record: func(visible bool) {
					log.Printf("application handoff visible=%t elapsed_ms=%d", visible, time.Since(started).Milliseconds())
				},
				Dispatch:    w.dispatch,
				MarkReady:   func() { w.state.phase = phaseReady },
				CloseWindow: func(hwnd uintptr) { postMessage.Call(hwnd, WM_CLOSE, 0, 0) },
			})
		},
	}, realHandoffClock)
}
