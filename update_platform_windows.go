package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

func updateDiskFreeBytes(directory string) (int64, error) {
	path, err := windows.UTF16PtrFromString(directory)
	if err != nil {
		return 0, err
	}
	var available, total, free uint64
	err = windows.GetDiskFreeSpaceEx(path, &available, &total, &free)
	return int64(available), err
}
func installedUpdateDirectory() (string, error) {
	if os.Getenv("PORTABLE_EXECUTABLE_FILE") != "" || os.Getenv("PORTABLE_EXECUTABLE_DIR") != "" {
		return "", nil
	}
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}
	root := updateRootForExecutable(executable)
	if !regularFile(filepath.Join(root, "Uninstall Deep Legends.exe")) {
		return "", nil
	}
	for _, view := range []uint32{registry.WOW64_64KEY, registry.WOW64_32KEY} {
		parent, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Uninstall`, registry.READ|view)
		if err != nil {
			continue
		}
		names, _ := parent.ReadSubKeyNames(-1)
		for _, name := range names {
			key, err := registry.OpenKey(parent, name, registry.QUERY_VALUE|view)
			if err != nil {
				continue
			}
			display, _, _ := key.GetStringValue("DisplayName")
			location, _, _ := key.GetStringValue("InstallLocation")
			key.Close()
			if updateInstallationMatches(root, display, location, true) {
				parent.Close()
				return root, nil
			}
		}
		parent.Close()
	}
	return "", nil
}
func launchUpdateInstaller(setup, destination string) error {
	installed, err := installedUpdateDirectory()
	if err != nil {
		return err
	}
	if !updateInstallationMatches(destination, "Deep Legends", installed, regularFile(filepath.Join(destination, "Uninstall Deep Legends.exe"))) {
		return errors.New("安装位置已变化，请从发布页下载完整安装包")
	}
	command := exec.Command(setup)
	command.SysProcAttr = &syscall.SysProcAttr{CmdLine: updateCommandLine(setup, destination)}
	if err = command.Start(); err != nil {
		return err
	}
	return command.Process.Release()
}
