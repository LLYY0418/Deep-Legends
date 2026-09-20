//go:build !windows

package main

import (
	"errors"
	"syscall"
)

func installedUpdateDirectory() (string, error) { return "", nil }
func launchUpdateInstaller(string, string) error {
	return errors.New("应用内升级仅支持 Windows 安装版")
}
func updateDiskFreeBytes(directory string) (int64, error) {
	var stat syscall.Statfs_t
	err := syscall.Statfs(directory, &stat)
	return int64(stat.Bavail) * int64(stat.Bsize), err
}
