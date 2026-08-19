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
