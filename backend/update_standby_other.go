//go:build !windows

package main

import (
	"context"
	"errors"
)

func startUpdateStandby(context.Context, string, string) (updateStandbyProcess, error) {
	return nil, errors.New("standby requires Windows")
}
