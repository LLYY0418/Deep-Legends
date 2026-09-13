package main

import (
	"context"
	"time"
)

// Execution is an additional attempt at warming the final file's scan state.
// Its completion status never substitutes for the proven two-file read warmup.
func completeStartupWarmup(directory string, execution *executionWarmup, read func(context.Context, string) error, budget time.Duration, progress func(int)) (prewarmResult, warmFallback) {
	status := warmFallback{Paths: startupPrewarmPaths(directory)}
	if execution != nil {
		status = execution.Finish()
	}
	result := runStartupPrewarm(startupPrewarmPaths(directory), read, budget, progress)
	return result, status
}
