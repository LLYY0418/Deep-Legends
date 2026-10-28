package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"syscall"
	"time"
)

func main() {
	// setup, receipt, stdout, stderr, followed by the exact production arguments.
	if len(os.Args) < 6 {
		os.Exit(2)
	}
	stdout, err := os.Create(os.Args[3])
	if err != nil {
		panic(err)
	}
	defer stdout.Close()
	stderr, err := os.Create(os.Args[4])
	if err != nil {
		panic(err)
	}
	defer stderr.Close()
	cmd := exec.Command(os.Args[1], os.Args[5:]...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	cmd.Stdout, cmd.Stderr = stdout, stderr
	started := time.Now()
	err = cmd.Start()
	returned := time.Since(started).Milliseconds()
	if err != nil {
		panic(err)
	}
	raw, _ := json.Marshal(map[string]any{"pid": cmd.Process.Pid, "start_utc_ms": started.UnixMilli(), "spawn_returned_ms": returned})
	// Publish a complete receipt atomically. Test-Path must never observe an
	// empty/partial JSON file and lose the PID needed to clean up this child.
	receipt := os.Args[2] + ".tmp"
	if err := os.WriteFile(receipt, raw, 0600); err != nil {
		cmd.Process.Kill()
		panic(err)
	}
	if err := os.Rename(receipt, os.Args[2]); err != nil {
		cmd.Process.Kill()
		panic(err)
	}
	if err := cmd.Wait(); err != nil {
		os.Exit(1)
	}
}
