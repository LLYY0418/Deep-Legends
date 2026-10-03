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

// Preserve every Windows attribute, not just the read-only bit, across rename.
func cameraFilePermissions(path string, _ os.FileInfo) (func(bool) error, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	original, err := windows.GetFileAttributes(name)
	if err != nil {
		return nil, err
	}
	return func(locked bool) error {
		attributes := original &^ windows.FILE_ATTRIBUTE_READONLY
		if locked {
			attributes = original
		}
		return windows.SetFileAttributes(name, attributes)
	}, nil
}
