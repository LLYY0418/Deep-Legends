package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func executeWarmEnvironment(base []string, nodeMode bool) []string {
	var env []string
	for _, entry := range applicationEnvironment(base) {
		name, _, _ := strings.Cut(entry, "=")
		switch strings.ToUpper(name) {
		// A caller's Node preload/inspector/coverage flags would turn a no-op
		// warmup into code execution, listening sockets or persistent output.
		case "NODE_OPTIONS", "NODE_V8_COVERAGE", "NODE_REDIRECT_WARNINGS", "NODE_CHANNEL_FD",
			"ELECTRON_ENABLE_LOGGING", "ELECTRON_LOG_FILE", "ELECTRON_LOG_ASAR_READS",
			"ELECTRON_ENABLE_STACK_DUMPING", "ELECTRON_DEFAULT_ERROR_MODE", "ELECTRON_NO_ATTACH_CONSOLE":
			continue
		}
		env = append(env, entry)
	}
	if nodeMode {
		env = append(env, "ELECTRON_RUN_AS_NODE=1", "ELECTRON_NO_ATTACH_CONSOLE=1")
	}
	return env
}

func executeWarmCommand(ctx context.Context, target warmTarget) *exec.Cmd {
	args := []string{"--startup-warmup"}
	if target.NodeMode {
		args = []string{"-e", "process.exit(0)"}
	}
	cmd := exec.CommandContext(ctx, target.Path, args...)
	cmd.Dir = filepath.Dir(target.Path)
	cmd.Env = executeWarmEnvironment(os.Environ(), target.NodeMode)
	configureWarmProcess(cmd)
	return cmd
}

func runExecutePrewarm(ctx context.Context, target warmTarget) warmExit {
	started := time.Now()
	cmd := executeWarmCommand(ctx, target)
	err := cmd.Run() // only called in an execution goroutine, never by the UI
	code := -1
	if cmd.ProcessState != nil {
		code = cmd.ProcessState.ExitCode()
	}
	if ctx.Err() != nil {
		err = ctx.Err()
	}
	return warmExit{Code: code, Err: err, ElapsedMS: time.Since(started).Milliseconds()}
}
