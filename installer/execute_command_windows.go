package main

import (
	"golang.org/x/sys/windows"
	"os/exec"
	"syscall"
)

func configureWarmProcess(cmd *exec.Cmd) {
	// Go's Windows runtime disables critical-error/crash dialogs. Preserve the
	// inherited error mode (no CREATE_DEFAULT_ERROR_MODE) and hide console UI.
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
}
