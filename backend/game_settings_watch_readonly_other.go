//go:build !windows

package main

import "os"

func gameSettingsReadOnly(_ string, info os.FileInfo) bool { return info.Mode().Perm()&0o222 == 0 }
