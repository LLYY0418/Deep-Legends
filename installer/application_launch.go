package main

import (
	"log"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Windows environment names are case-insensitive. Allocate a fresh slice:
// warmup must never contaminate the installer or the real application launch.
func applicationEnvironment(base []string) []string {
	result := make([]string, 0, len(base))
	for _, entry := range base {
		name, _, _ := strings.Cut(entry, "=")
		if !strings.EqualFold(name, "ELECTRON_RUN_AS_NODE") {
			result = append(result, entry)
		}
	}
	return result
}

func applicationCommand(exe, directory string) *exec.Cmd {
	command := exec.Command(exe)
	command.Dir = directory
	command.Env = applicationEnvironment(os.Environ())
	return command
}

func startApplication(exe, directory string) (uint32, error) {
	command := applicationCommand(exe, directory)
	if err := command.Start(); err != nil {
		return 0, err
	}
	pid := uint32(command.Process.Pid)
	go func() { _ = command.Wait() }() // release only; never terminate the real app
	return pid, nil
}

func logApplicationStart(start func() (uint32, error), started time.Time) (uint32, error) {
	pid, err := start()
	log.Printf("application launch pid=%d error=%v elapsed_ms=%d", pid, err, time.Since(started).Milliseconds())
	return pid, err
}
