//go:build !windows

package main

import (
	"context"
	"errors"
)

func detectClientInstallationsWithScan() ([]clientInstallation, clientInstallationScan) {
	return nil, clientInstallationScan{}
}

func launchClientInstallation(clientInstallation) (clientLaunchResult, error) {
	return clientLaunchResult{}, errors.New("client launching is only available on Windows")
}

func launchClientInstallationContext(ctx context.Context, installation clientInstallation) (clientLaunchResult, error) {
	if err := ctx.Err(); err != nil {
		return clientLaunchResult{}, err
	}
	return launchClientInstallation(installation)
}
