//go:build !windows

package main

import "errors"

func detectClientInstallations() []clientInstallation { return nil }

func launchClientInstallation(clientInstallation) error {
	return errors.New("client launching is only available on Windows")
}
