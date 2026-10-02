package main

import (
	"bufio"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

func updateLongPathName(value string) (string, error) {
	input, err := windows.UTF16PtrFromString(value)
	if err != nil {
		return "", err
	}
	buffer := make([]uint16, 260)
	for {
		n, err := windows.GetLongPathName(input, &buffer[0], uint32(len(buffer)))
		if err != nil {
			return "", err
		}
		if n < uint32(len(buffer)) {
			return windows.UTF16ToString(buffer[:n]), nil
		}
		buffer = make([]uint16, int(n)+1)
	}
}

func updateDiskFreeBytes(directory string) (int64, error) {
	path, err := windows.UTF16PtrFromString(directory)
	if err != nil {
		return 0, err
	}
	var available, total, free uint64
	err = windows.GetDiskFreeSpaceEx(path, &available, &total, &free)
	return int64(available), err
}
func detectUpdateInstallation() (updateInstallDetection, error) {
	executable, err := os.Executable()
	if err != nil {
		return updateInstallDetection{Result: "no_uninstaller"}, err
	}
	root := updateRootForExecutable(executable)
	entries := []updateRegistryInstallation{}
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
			entries = append(entries, updateRegistryInstallation{display, location})
		}
		parent.Close()
	}
	return classifyUpdateInstallation(root, os.Getenv("PORTABLE_EXECUTABLE_FILE") != "" || os.Getenv("PORTABLE_EXECUTABLE_DIR") != "", regularFile(filepath.Join(root, "Uninstall Deep Legends.exe")), entries), nil
}
func installedUpdateDirectory() (string, error) {
	d, err := detectUpdateInstallation()
	return d.directory, err
}
func launchUpdateInstaller(setup, destination string) error {
	installed, err := installedUpdateDirectory()
	if err == nil && installed == "" {
		return launchPortableUpdateInstaller(setup, destination)
	}
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

func launchPortableUpdateInstaller(setup, dest string) error {
	expected, err := defaultPortableInstallDirectory()
	if err != nil {
		return err
	}
	if !strings.EqualFold(filepath.Clean(dest), filepath.Clean(expected)) {
		return errors.New("升级安装目录不正确")
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	if strings.EqualFold(filepath.Clean(dest), filepath.Clean(updateRootForExecutable(executable))) {
		return errors.New("当前安装目录正在使用，请从发布页安装")
	}
	parent := os.Getpid()
	if value, err := strconv.Atoi(os.Getenv("LOOT_DESKTOP_PID")); err == nil && value > 0 {
		parent = value
	}
	cmd := exec.Command(setup)
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: portableUpdateCommandLine(setup, dest, parent), HideWindow: true}
	cmd.Env = portableUpdateEnvironment(os.Environ())
	output, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err = cmd.Start(); err != nil {
		return err
	}
	result := make(chan error, 1)
	goSafe("update-installer-result", func() {
		scanner := bufio.NewScanner(output)
		scanner.Buffer(make([]byte, 256), 4096)
		for scanner.Scan() {
			if scanner.Text() == "DEEP_LEGENDS_UPDATE_INSTALLED" {
				result <- nil
				_ = cmd.Wait()
				return
			}
			if scanner.Text() == "DEEP_LEGENDS_UPDATE_FAILED" {
				break
			}
		}
		result <- errors.New("安装未完成，请重试或打开发布页")
		_ = cmd.Wait()
	})
	select {
	case err := <-result:
		return err
	case <-time.After(5 * time.Minute):
		return errors.New("安装等待超时，请重试或打开发布页")
	}
}
