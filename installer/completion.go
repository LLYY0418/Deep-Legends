package main

import "fmt"

type installationResult struct {
	ExitCode    int
	WaitError   error
	Destination string
}

type installationCompletionHooks struct {
	Failed  func(failureMessage)
	Cleanup func()
	Handoff func()
}

// Both normal installation and --update finish here. The platform adapter
// supplies OS operations; it must not decide whether a successful update waits
// for the application window or exits immediately.
func completeInstallation(options installerOptions, result installationResult, hooks installationCompletionHooks) {
	if result.WaitError != nil || result.ExitCode != 0 {
		message := "安装没有完成，请检查安装位置后重试"
		if options.Update {
			message = upgradeFailureMessage
		}
		hooks.Failed(failureMessage{Message: message, Detail: fmt.Sprintf("退出码：%d；目标路径：%s", result.ExitCode, result.Destination)})
		return
	}
	hooks.Cleanup()
	hooks.Handoff()
}

type installerExitHooks struct {
	Record      func(bool)
	Dispatch    func(func())
	MarkReady   func()
	CloseWindow func(uintptr)
}

// Cleanup owns only the installer HWND. There is deliberately no application
// process/termination capability here: success and timeout both leave it alive.
func finishInstallerHandoff(installerWindow uintptr, visible bool, hooks installerExitHooks) {
	// Record before dispatch: the UI may exit the process immediately on close.
	hooks.Record(visible)
	hooks.Dispatch(func() {
		hooks.MarkReady()
		hooks.CloseWindow(installerWindow)
	})
}
