package main

import (
	"context"
	"golang.org/x/sys/windows"
	"os/exec"
	"syscall"
)

func warmUpdateInstaller(ctx context.Context, path string) error {
	cmd := exec.CommandContext(ctx, path, "--warmup")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.BELOW_NORMAL_PRIORITY_CLASS | windows.CREATE_NO_WINDOW}
	return cmd.Run()
}
