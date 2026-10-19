//go:build !windows

package main

func nativeClientLauncherProcesses([]clientInstallation) clientLauncherProcesses {
	return clientLauncherProcesses{Launchers: map[string]bool{}}
}
