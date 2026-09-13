//go:build !windows

package main

import "os/exec"

func configureWarmProcess(cmd *exec.Cmd) {}
