//go:build !windows

package main

import "os"

func gameSettingsReadOnly(_ string, info os.FileInfo) bool { return info.Mode().Perm()&0o222 == 0 }

// Capture the original permissions before temporarily unlocking CameraMode.
func cameraFilePermissions(path string, info os.FileInfo) (func(bool) error, error) {
	original := info.Mode().Perm()
	return func(locked bool) error {
		mode := original | 0200
		if locked {
			mode = original
		}
		return os.Chmod(path, mode)
	}, nil
}
