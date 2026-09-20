//go:build !windows

package main

import "errors"

func detectClientInstallationsWithScan() ([]clientInstallation, clientInstallationScan) {
	return nil, clientInstallationScan{}
}

func launchClientInstallation(clientInstallation) (clientLaunchResult, error) {
	return clientLaunchResult{}, errors.New("client launching is only available on Windows")
}
