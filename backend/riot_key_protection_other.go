//go:build !windows

package main

// Development fallback only; writeLocalStoreFile enforces owner-only 0600.
func protectRiotUserKey(data []byte) ([]byte, error)   { return append([]byte(nil), data...), nil }
func unprotectRiotUserKey(data []byte) ([]byte, error) { return append([]byte(nil), data...), nil }
