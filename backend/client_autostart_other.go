//go:build !windows

package main

func nativeClientAutostarts([]clientInstallation) ([]clientAutostartEntry, startupApprovalStore) {
	return nil, nil
}
