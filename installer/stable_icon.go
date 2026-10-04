package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
)

func writeStableIcon(path string, data []byte) (bool, error) {
	if len(data) < 4 || !bytes.Equal(data[:4], []byte{0, 0, 1, 0}) {
		return false, errors.New("invalid icon resource")
	}
	if existing, err := os.ReadFile(path); err == nil && bytes.Equal(existing, data) {
		return false, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return false, err
	}
	if info, err := os.Lstat(path); err == nil && !info.Mode().IsRegular() {
		return false, os.ErrPermission
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return false, err
	}
	return true, nil
}
