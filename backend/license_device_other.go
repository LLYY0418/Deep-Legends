//go:build license && !windows

package main

import "errors"

// The distributed client targets Windows. Other systems use explicit stores
// in tests; production never silently stores a plaintext device private key.
func licensePlatformDirectory() (string, error) {
	return "", errors.New("device key protection requires Windows")
}
func licenseProtect([]byte) ([]byte, error) {
	return nil, errors.New("device key protection requires Windows")
}
func licenseUnprotect([]byte) ([]byte, error) {
	return nil, errors.New("device key protection requires Windows")
}
