//go:build windows

package main

func gameCameraProcessRunning() (bool, error) {
	result, err := nativeProcessCommands("League of Legends.exe")
	return result.ProcessCount > 0, err
}
