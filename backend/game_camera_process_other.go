//go:build !windows

package main

import "errors"

func gameCameraProcessRunning() (bool, error) {
	return false, errors.New("game process status unavailable")
}
