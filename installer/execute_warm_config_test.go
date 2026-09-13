package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func unsetWarmEnvironment(t *testing.T, name string) {
	t.Helper()
	t.Setenv(name, "") // Register restoration of the caller's original value.
	if err := os.Unsetenv(name); err != nil {
		t.Fatal(err)
	}
}

func TestExecuteWarmConfigDefaultsOffAndReadsBothFiles(t *testing.T) {
	for _, value := range []string{"unset", "", "0", "true", "yes", " 1", "1 ", "01"} {
		t.Run(fmt.Sprintf("value=%q", value), func(t *testing.T) {
			unsetWarmEnvironment(t, "DEEP_LEGENDS_STARTUP_PREWARM")
			unsetWarmEnvironment(t, "DEEP_LEGENDS_STARTUP_EXECUTE_WARM")
			if value != "unset" {
				t.Setenv("DEEP_LEGENDS_STARTUP_EXECUTE_WARM", value)
			}
			directory := t.TempDir()
			var lines []string
			w := newConfiguredExecutionWarmup(directory, time.Now(), executeWarmHooks{
				Inspect: func(string) (warmFileStamp, error) {
					t.Error("disabled execution inspected an executable")
					return warmFileStamp{}, os.ErrNotExist
				},
				Run: func(context.Context, warmTarget) warmExit {
					t.Error("disabled execution launched a warmup process")
					return warmExit{}
				},
				Log: func(format string, args ...any) { lines = append(lines, fmt.Sprintf(format, args...)) },
			})
			t.Cleanup(w.Cancel)
			paths := startupPrewarmPaths(directory)
			for _, path := range paths {
				writeWarmFile(t, path, "installed executable")
			}
			finished := make(chan struct{})
			w.Poll(w.started, finished)
			w.Poll(w.started.Add(time.Second), finished)
			close(finished)
			w.Poll(w.started.Add(2*time.Second), finished)
			if w.enabled || w.ctx != nil || w.cancel != nil {
				t.Fatal("disabled execution created a process cancellation context")
			}
			for _, state := range w.states {
				if state.attempted || state.result != nil || state.cancel != nil {
					t.Fatal("disabled execution scheduled a runner", state.target.Path)
				}
			}
			var reads []string
			result, status := completeStartupWarmup(directory, w, func(ctx context.Context, path string) error {
				reads = append(reads, path)
				return readPrewarmFile(ctx, path)
			}, startupPrewarmBudget, func(int) {})
			w.Finish() // The install worker also defers Finish; log only once.
			if !reflect.DeepEqual(reads, paths) || result.Files != 2 || result.Failures != 0 || result.TimedOut {
				t.Fatal("disabled execution changed the read warmup", reads, result)
			}
			if status.ExecuteValid != 0 || !reflect.DeepEqual(status.Paths, paths) {
				t.Fatal("disabled execution diagnostics lost the two final files", status)
			}
			if !reflect.DeepEqual(lines, []string{"startup warm mode=execute enabled=false"}) {
				t.Fatal("missing/duplicated disabled status or unexpected execution attempt", lines)
			}
		})
	}
}

func TestExecuteWarmConfigExplicitOneRetainsExecutionAndBothReads(t *testing.T) {
	for _, outcome := range []string{"ok", "failed", "still_running"} {
		t.Run(outcome, func(t *testing.T) {
			unsetWarmEnvironment(t, "DEEP_LEGENDS_STARTUP_PREWARM")
			t.Setenv("DEEP_LEGENDS_STARTUP_EXECUTE_WARM", "1")
			directory := t.TempDir()
			entered := make(chan warmTarget, 2)
			release := make(chan struct{})
			releaseWarm := sync.OnceFunc(func() { close(release) })
			defer releaseWarm()
			var lines []string
			w := newConfiguredExecutionWarmup(directory, time.Now(), executeWarmHooks{
				Run: func(ctx context.Context, target warmTarget) warmExit {
					entered <- target
					if outcome == "still_running" {
						<-release // Even a runner ignoring cancellation cannot delay Finish.
					}
					if outcome == "failed" {
						return warmExit{Code: 7, Err: errors.New("synthetic execution failure")}
					}
					return warmExit{Code: 0}
				},
				Log: func(format string, args ...any) { lines = append(lines, fmt.Sprintf(format, args...)) },
			})
			t.Cleanup(w.Cancel)
			if !w.enabled || w.budget != 8*time.Second {
				t.Fatal("explicit execution opt-in or budget changed")
			}
			paths := startupPrewarmPaths(directory)
			for _, path := range paths {
				writeWarmFile(t, path, "newly installed executable")
			}
			finished := make(chan struct{})
			w.Poll(w.started, finished)
			w.Poll(w.started.Add(399*time.Millisecond), finished)
			for _, state := range w.states {
				if state.attempted {
					t.Fatal("opt-in bypassed final-file stability")
				}
			}
			w.Poll(w.started.Add(400*time.Millisecond), finished)
			seen := make(map[string]bool)
			for range 2 {
				select {
				case target := <-entered:
					seen[target.Path] = target.NodeMode
				case <-time.After(time.Second):
					t.Fatal("explicit opt-in did not execute both files during NSIS")
				}
			}
			if !reflect.DeepEqual(seen, map[string]bool{paths[0]: true, paths[1]: false}) {
				t.Fatal("opt-in changed execution targets", seen)
			}
			if outcome != "still_running" {
				for i := range w.states {
					awaitWarmResult(t, &w.states[i])
				}
			}
			close(finished)
			var reads []string
			type completion struct {
				result prewarmResult
				status warmFallback
			}
			done := make(chan completion, 1)
			go func() {
				result, status := completeStartupWarmup(directory, w, func(ctx context.Context, path string) error {
					reads = append(reads, path)
					return readPrewarmFile(ctx, path)
				}, startupPrewarmBudget, func(int) {})
				done <- completion{result, status}
			}()
			select {
			case got := <-done:
				valid := 0
				if outcome == "ok" {
					valid = 2
				}
				if got.status.ExecuteValid != valid || len(got.status.Paths) != 2-valid {
					t.Fatal("opt-in changed execution classification", got.status)
				}
				if !reflect.DeepEqual(reads, paths) || got.result.Files != 2 || got.result.Failures != 0 || got.result.TimedOut {
					t.Fatal("execution outcome changed the two-file read warmup", reads, got.result)
				}
			case <-time.After(500 * time.Millisecond):
				releaseWarm()
				<-done
				t.Fatal("opt-in blocked read warmup on an execution process")
			}
			if len(lines) != 2 || strings.Contains(strings.Join(lines, "\n"), "enabled=false") || strings.Count(strings.Join(lines, "\n"), "attempted=true") != 2 {
				t.Fatal("explicit opt-in lost execution logs", lines)
			}
		})
	}
}

func TestExecuteWarmConfigMasterDisableOverridesOptIn(t *testing.T) {
	t.Setenv("DEEP_LEGENDS_STARTUP_PREWARM", "0")
	t.Setenv("DEEP_LEGENDS_STARTUP_EXECUTE_WARM", "1")
	w := newConfiguredExecutionWarmup(t.TempDir(), time.Now(), executeWarmHooks{
		Inspect: func(string) (warmFileStamp, error) {
			t.Error("master disable inspected executable")
			return warmFileStamp{}, nil
		},
	})
	defer w.Cancel()
	if w.enabled {
		t.Fatal("execution opt-in bypassed the existing all-prewarm disable switch")
	}
}
