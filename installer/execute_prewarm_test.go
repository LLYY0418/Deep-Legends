package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func warmFixture(t *testing.T, budget time.Duration, run func(context.Context, warmTarget) warmExit) (*executionWarmup, string, chan struct{}, *[]string) {
	t.Helper()
	directory := t.TempDir()
	lines := []string{}
	w := newExecutionWarmup(directory, time.Now(), true, budget, executeWarmHooks{
		Run: run, Log: func(format string, args ...any) { lines = append(lines, fmt.Sprintf(format, args...)) },
	})
	t.Cleanup(w.Cancel)
	return w, directory, make(chan struct{}), &lines
}
func writeWarmFile(t *testing.T, path, value string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(value), 0600); err != nil {
		t.Fatal(err)
	}
}
func awaitWarmResult(t *testing.T, state *executableWarmState) {
	t.Helper()
	select {
	case result := <-state.result:
		state.result <- result
	case <-time.After(time.Second):
		t.Fatal("execution did not complete")
	}
}

func TestExecuteWarmupStartsBothProcessesWhileFakeNSISStillRunning(t *testing.T) {
	entered := make(chan string, 2)
	release := make(chan struct{})
	w, directory, finished, _ := warmFixture(t, time.Second, func(ctx context.Context, target warmTarget) warmExit {
		entered <- target.Name
		select {
		case <-release:
			return warmExit{Code: 0}
		case <-ctx.Done():
			return warmExit{Code: -1, Err: ctx.Err()}
		}
	})
	for _, path := range startupPrewarmPaths(directory) {
		writeWarmFile(t, path, "new complete executable")
	}
	w.Poll(w.started, finished)
	w.Poll(w.started.Add(399*time.Millisecond), finished)
	for _, state := range w.states {
		if state.attempted {
			t.Fatal("scheduled execution before 400ms stability")
		}
	}
	select {
	case <-entered:
		t.Fatal("executed before 400ms stability")
	default:
	}
	w.Poll(w.started.Add(400*time.Millisecond), finished)
	// Both must enter without waiting for the other process or for NSIS exit.
	for range 2 {
		select {
		case <-entered:
		case <-time.After(time.Second):
			t.Fatal("warmup did not overlap running NSIS")
		}
	}
	if !installationStillRunning(finished) {
		t.Fatal("fake NSIS already exited")
	}
	close(release)
	for index := range w.states {
		awaitWarmResult(t, &w.states[index])
	}
	close(finished)
	fallback := w.Finish()
	if fallback.ExecuteValid != 2 || len(fallback.Paths) != 0 {
		t.Fatal(fallback)
	}
}

func TestExecuteWarmupRequiresStableFinalFileAndVoidsRewrittenExecutable(t *testing.T) {
	w, directory, finished, lines := warmFixture(t, time.Second, func(context.Context, warmTarget) warmExit { return warmExit{Code: 0} })
	file := startupPrewarmPaths(directory)[0]
	for index := range 5 {
		writeWarmFile(t, file, strings.Repeat("x", index+1))
		w.Poll(w.started.Add(time.Duration(index)*500*time.Millisecond), finished)
	}
	if w.states[0].attempted {
		t.Fatal("executed while file size was changing")
	}
	w.Poll(w.started.Add(2400*time.Millisecond), finished)
	awaitWarmResult(t, &w.states[0])
	writeWarmFile(t, file, "rewritten after warmup")
	close(finished)
	fallback := w.Finish()
	if fallback.ExecuteValid != 0 || !reflect.DeepEqual(fallback.Paths, startupPrewarmPaths(directory)) {
		t.Fatal(fallback)
	}
	if !strings.Contains(strings.Join(*lines, "\n"), "voided=true") {
		t.Fatal(*lines)
	}
	reads := 0
	result, _ := completeStartupWarmup(directory, w, func(context.Context, string) error { reads++; return nil }, time.Second, func(int) {})
	if reads != 2 || result.Failures != 0 {
		t.Fatal("invalidated execution lost the unconditional reads", reads, result)
	}
}

func TestExecuteWarmupRejectsUnchangedUpgradeFilesAndUnreadableFiles(t *testing.T) {
	directory := t.TempDir()
	file := startupPrewarmPaths(directory)[0]
	writeWarmFile(t, file, "previously installed executable")
	w := newExecutionWarmup(directory, time.Now(), true, time.Second, executeWarmHooks{Run: func(context.Context, warmTarget) warmExit {
		t.Error("pre-existing executable must not be mistaken for new installation")
		return warmExit{Code: 0}
	}, Log: func(string, ...any) {}})
	defer w.Cancel()
	finished := make(chan struct{})
	w.Poll(w.started, finished)
	w.Poll(w.started.Add(time.Second), finished)
	if w.states[0].attempted {
		t.Fatal("executed old upgrade file")
	}
	if len(w.Finish().Paths) != 2 {
		t.Fatal("old files must fall back to read warmup")
	}
	w = newExecutionWarmup(directory, time.Now(), true, time.Second, executeWarmHooks{
		Inspect: func(string) (warmFileStamp, error) { return warmFileStamp{}, os.ErrPermission },
		Run:     func(context.Context, warmTarget) warmExit { t.Error("executed unreadable file"); return warmExit{} }, Log: func(string, ...any) {},
	})
	w.Poll(w.started, finished)
	w.Poll(w.started.Add(time.Second), finished)
	if len(w.Finish().Paths) != 2 {
		t.Fatal("unreadable files must fall back")
	}
}

func TestExecuteWarmupCannotBeginAfterNSISExit(t *testing.T) {
	w, directory, finished, _ := warmFixture(t, time.Second, func(context.Context, warmTarget) warmExit { t.Error("warmup began after NSIS exit"); return warmExit{} })
	writeWarmFile(t, startupPrewarmPaths(directory)[0], "complete")
	w.Poll(w.started, finished)
	close(finished)
	w.Poll(w.started.Add(time.Second), finished)
	if w.states[0].attempted {
		t.Fatal("execution scheduled after NSIS exit")
	}
	if len(w.Finish().Paths) != 2 {
		t.Fatal("late files must use read fallback")
	}
}

func TestExecuteWarmupFailuresAndBlockedProcessNeverHoldInstallationHandoff(t *testing.T) {
	for _, failure := range []string{"nonzero", "blocked", "missing"} {
		t.Run(failure, func(t *testing.T) {
			release := make(chan struct{})
			releaseWarm := sync.OnceFunc(func() { close(release) })
			defer releaseWarm()
			entered := make(chan struct{}, 2)
			w, directory, finished, _ := warmFixture(t, 250*time.Millisecond, func(ctx context.Context, target warmTarget) warmExit {
				entered <- struct{}{}
				if failure == "blocked" {
					<-release
				} // even a runner ignoring cancellation cannot block Finish
				return warmExit{Code: 7, Err: errors.New("synthetic failure")}
			})
			if failure != "missing" {
				for _, file := range startupPrewarmPaths(directory) {
					writeWarmFile(t, file, "complete")
				}
				w.Poll(w.started, finished)
				w.Poll(w.started.Add(time.Second), finished)
				for range 2 {
					select {
					case <-entered:
					case <-time.After(time.Second):
						t.Fatal("missing execution attempt")
					}
				}
				if failure == "blocked" {
					select {
					case <-w.ctx.Done():
					case <-time.After(time.Second):
						t.Fatal("8s execution budget was ignored")
					}
				} else {
					for i := range w.states {
						awaitWarmResult(t, &w.states[i])
					}
				}
			}
			close(finished)
			started := time.Now()
			planned := make(chan warmFallback, 1)
			go func() { planned <- w.Finish() }()
			var fallback warmFallback
			select {
			case fallback = <-planned:
			case <-time.After(500 * time.Millisecond):
				releaseWarm()
				<-planned
				t.Fatal("Finish waited for a blocked warm process")
			}
			if len(fallback.Paths) != 2 || fallback.ExecuteValid != 0 {
				t.Fatal(fallback)
			}
			// Exercise the unchanged real completion and handoff policies too.
			launched, closed := false, false
			completeInstallation(installerOptions{}, installationResult{}, installationCompletionHooks{
				Failed: func(failureMessage) { t.Error("warmup failed installation") }, Cleanup: func() {},
				Handoff: func() {
					completeStartupWarmup(directory, w, func(context.Context, string) error {
						if failure == "missing" {
							return os.ErrNotExist
						}
						return nil
					}, time.Second, func(int) {})
					runApplicationHandoff(handoffHooks{ShowStarting: func() {}, Start: func() (uint32, error) { return 42, nil },
						HasWindow: func(pid uint32) bool { launched = pid == 42; return launched }, LaunchError: func(error) { t.Error("launch failed") },
						CloseInstaller: func(visible bool) { closed = visible }}, realHandoffClock)
				},
			})
			if !launched || !closed || time.Since(started) > 500*time.Millisecond {
				t.Fatal("warmup blocked completion or real launch")
			}
		})
	}
}

func TestExecuteWarmupVoidsSameSizeMtimeChangeAndReplacedFile(t *testing.T) {
	for _, change := range []string{"mtime", "replacement"} {
		t.Run(change, func(t *testing.T) {
			w, directory, finished, lines := warmFixture(t, time.Second, func(context.Context, warmTarget) warmExit { return warmExit{Code: 0} })
			file := startupPrewarmPaths(directory)[0]
			writeWarmFile(t, file, "original")
			w.Poll(w.started, finished)
			w.Poll(w.started.Add(time.Second), finished)
			awaitWarmResult(t, &w.states[0])
			w.Cancel()
			info, err := os.Stat(file)
			if err != nil {
				t.Fatal(err)
			}
			if change == "mtime" {
				changed := info.ModTime().Add(time.Second)
				if err := os.Chtimes(file, changed, changed); err != nil {
					t.Fatal(err)
				}
			} else {
				replacement := filepath.Join(directory, "replacement.exe")
				writeWarmFile(t, replacement, "replaced") // same length and timestamp, different file identity
				if err := os.Chtimes(replacement, info.ModTime(), info.ModTime()); err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(file); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(replacement, file); err != nil {
					t.Fatal(err)
				}
			}
			close(finished)
			if result := w.Finish(); result.ExecuteValid != 0 || len(result.Paths) != 2 {
				t.Fatal(result)
			}
			if !strings.Contains(strings.Join(*lines, "\n"), "voided=true") {
				t.Fatal(*lines)
			}
		})
	}
}

func TestExecuteWarmupExplicitControlDoesNotInspectOrExecute(t *testing.T) {
	forbidden := func(string) (warmFileStamp, error) {
		t.Error("control touched executable")
		return warmFileStamp{}, nil
	}
	w := newExecutionWarmup(t.TempDir(), time.Now(), false, time.Second, executeWarmHooks{Inspect: forbidden})
	w.Poll(time.Now(), make(chan struct{}))
	if result := w.Finish(); len(result.Paths) != 2 || result.ExecuteValid != 0 {
		t.Fatal(result)
	}
}
