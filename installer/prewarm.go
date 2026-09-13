package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"time"
)

const startupPrewarmBudget = 8 * time.Second

// R83's 2026-09-12 Windows cold-install report: six runs, median total
// 3849→3471ms, installed_to_visible 5401→4948ms; reads took 46ms with three
// failure-free prewarm runs. Gains: spawn_to_ready -507ms, splash_window_shown
// -530ms. Keep an explicit control switch; unknown values use the default.
func startupPrewarmEnabled(value string) bool { return value != "0" }

func startupPrewarmPaths(directory string) []string {
	return []string{filepath.Join(directory, "Deep Legends.exe"), filepath.Join(directory, "resources", "app.asar.unpacked", "backend", "loot-service.exe")}
}

type prewarmResult struct {
	Files, Failures int
	TimedOut        bool
	ElapsedMS       int64
}

func readPrewarmFile(ctx context.Context, path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	buffer := make([]byte, 128*1024)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		_, err := file.Read(buffer)
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func runStartupPrewarm(paths []string, read func(context.Context, string) error, budget time.Duration, progress func(int)) prewarmResult {
	started := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), budget)
	defer cancel()
	completed := make(chan error, len(paths))
	go func() {
		for _, path := range paths {
			if ctx.Err() != nil {
				return
			}
			completed <- read(ctx, path)
		}
	}()
	var result prewarmResult
	for result.Files < len(paths) {
		select {
		case <-ctx.Done():
			result.TimedOut = true
			result.ElapsedMS = time.Since(started).Milliseconds()
			return result
		case err := <-completed:
			result.Files++
			if err != nil {
				result.Failures++
			}
			progress(97 + 3*result.Files/len(paths))
		}
	}
	result.ElapsedMS = time.Since(started).Milliseconds()
	return result
}
