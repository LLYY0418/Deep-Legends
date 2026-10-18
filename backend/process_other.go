//go:build !windows

package main

import (
	"errors"
	"os/exec"
)

func hideCommandWindow(_ *exec.Cmd) {}

func nativeLeagueProcessCommands() (processQueryResult, error) {
	return processQueryResult{Method: "native"}, errors.New("native process discovery is only available on Windows")
}

func nativeRiotClientProcessCommands() (processQueryResult, error) {
	return processQueryResult{Method: "native"}, errors.New("native process discovery is only available on Windows")
}

func nativeLeagueProcessSnapshot() (processQueryResult, error) {
	return processQueryResult{Method: "native"}, errors.New("native process discovery is only available on Windows")
}

func nativeLeagueCrashHandlerSeen() bool { return false }

func nativeLeagueGameProcessRunning() (bool, error) {
	return false, errors.New("game process snapshot is only available on Windows")
}
