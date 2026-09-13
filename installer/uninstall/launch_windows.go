package main

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"unicode/utf16"

	"golang.org/x/sys/windows"
)

func commandParameters() string {
	raw := windows.UTF16PtrToString(windows.GetCommandLine())
	_, end := nextToken(raw, 0)
	return strings.TrimSpace(raw[end:])
}

// Like NSIS's normal uninstaller bootstrap, an executable in $INSTDIR must
// leave before removal. The temporary worker owns UI, completion and errors.
// During electron-builder upgrades our executable is already outside $INSTDIR;
// run inline in that case so ExecWait receives the actual Core exit code.
func (launch uninstallLaunch) relocate(executable string) (bool, error) {
	if !strings.EqualFold(filepath.Clean(filepath.Dir(executable)), launch.Directory) {
		return false, nil
	}
	worker := filepath.Join(launch.Workspace, shellFilename)
	if err := copyExecutable(executable, worker); err != nil {
		return false, err
	}
	launch.ParentPID = uint32(os.Getpid())
	data, err := json.Marshal(launch)
	if err != nil {
		return false, err
	}
	if err := os.WriteFile(filepath.Join(launch.Workspace, "launch.json"), data, 0600); err != nil {
		return false, err
	}
	cmd := exec.Command(worker, "--dl-uninstall-worker")
	cmd.Dir = launch.Workspace
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	if err := cmd.Start(); err != nil {
		return false, err
	}
	_ = cmd.Process.Release()
	return true, nil
}

func loadWorker(executable string) (uninstallLaunch, error) {
	var launch uninstallLaunch
	data, err := os.ReadFile(filepath.Join(filepath.Dir(executable), "launch.json"))
	if err != nil {
		return launch, err
	}
	if err := json.Unmarshal(data, &launch); err != nil {
		return launch, err
	}
	if !strings.EqualFold(filepath.Clean(launch.Workspace), filepath.Dir(executable)) ||
		!strings.HasPrefix(filepath.Base(launch.Workspace), "DeepLegendsUninstall-") ||
		launch.ParentPID == 0 || launch.ParentPID == uint32(os.Getpid()) ||
		!filepath.IsAbs(launch.Directory) || filepath.Dir(filepath.Clean(launch.Directory)) == filepath.Clean(launch.Directory) ||
		strings.ContainsAny(launch.Directory, "\"\x00\r\n") {
		return uninstallLaunch{}, fmt.Errorf("invalid uninstaller handoff")
	}
	parent, err := windows.OpenProcess(windows.SYNCHRONIZE, false, launch.ParentPID)
	if err == nil {
		defer windows.CloseHandle(parent)
		if _, err := windows.WaitForSingleObject(parent, windows.INFINITE); err != nil {
			return launch, err
		}
	} else if err != windows.ERROR_INVALID_PARAMETER {
		return launch, err
	}
	return launch, nil
}

func (launch uninstallLaunch) command(silent, deleteData bool) *exec.Cmd {
	core := filepath.Join(launch.Workspace, coreFilename)
	cmd := exec.Command(core)
	cmd.Dir = launch.Workspace
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow: silent,
		CmdLine:    launch.Options.coreCommandLine(core, launch.Directory, silent, deleteData),
	}
	return cmd
}

func exitCode(cmd *exec.Cmd, err error) int {
	if cmd.ProcessState != nil {
		return cmd.ProcessState.ExitCode()
	}
	if err != nil {
		return 1
	}
	return 0
}

func (launch uninstallLaunch) runWithoutUI(silent bool) int {
	deleteData := launch.Options.hasFlag("--delete-app-data")
	cmd := launch.command(silent, deleteData)
	err := cmd.Run()
	code := exitCode(cmd, err)
	finishUninstall(code, deleteData, os.Getenv("LOCALAPPDATA"), os.RemoveAll, func(string, any) {})
	return code
}

// The running temporary image cannot delete itself on Windows. A hidden,
// bounded cleanup waits for this PID; it only removes our random workspace.
// Use an encoded script with literal quoting, never interpolate into cmd.exe.
func (launch uninstallLaunch) cleanup(executable string) {
	if !strings.EqualFold(filepath.Dir(executable), launch.Workspace) {
		_ = os.RemoveAll(launch.Workspace)
		return
	}
	quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }
	script := "$ErrorActionPreference='SilentlyContinue'; Wait-Process -Id " + strconv.Itoa(os.Getpid()) + "; " +
		"for($i=0;$i -lt 20;$i++){Remove-Item -LiteralPath " + quote(launch.Workspace) +
		" -Recurse -Force; if(!(Test-Path -LiteralPath " + quote(launch.Workspace) + ")){break}; Start-Sleep -Milliseconds 500}"
	units := utf16.Encode([]rune(script))
	encoded := make([]byte, len(units)*2)
	for i, unit := range units {
		binary.LittleEndian.PutUint16(encoded[2*i:], unit)
	}
	system, err := windows.GetSystemDirectory()
	if err != nil {
		return
	}
	cmd := exec.Command(filepath.Join(system, `WindowsPowerShell\v1.0\powershell.exe`), "-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden", "-EncodedCommand", base64.StdEncoding.EncodeToString(encoded))
	cmd.Dir = os.TempDir()
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	if cmd.Start() == nil {
		_ = cmd.Process.Release()
	}
}
