//go:build !windows

package main

import (
	"errors"
	"syscall"
)

func updateLongPathName(value string) (string, error) { return value, nil }

func detectUpdateInstallation() (updateInstallDetection, error) {
	return updateInstallDetection{Result: "unsupported_platform"}, nil
}
func installedUpdateDirectory() (string, error) { return "", nil }
func launchUpdateInstaller(string, string) error {
	return errors.New("应用内升级仅支持 Windows")
}
func launchUpdateInstallerTimed(setup, dest string, mark func(string)) error {
	return launchUpdateInstaller(setup, dest)
}
func updateDiskFreeBytes(directory string) (int64, error) {
	var stat syscall.Statfs_t
	err := syscall.Statfs(directory, &stat)
	return int64(stat.Bavail) * int64(stat.Bsize), err
}

func updateFileBusy(error) bool { return false }
