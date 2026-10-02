package main

import (
	"golang.org/x/sys/windows"
	"os"
)

func gameSettingsReadOnly(path string, info os.FileInfo) bool {
	name, err := windows.UTF16PtrFromString(path)
	if err == nil {
		if attributes, err := windows.GetFileAttributes(name); err == nil {
			return attributes&windows.FILE_ATTRIBUTE_READONLY != 0
		}
	}
	return info.Mode().Perm()&0o222 == 0
}
