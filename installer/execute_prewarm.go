package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"time"
)

// R83 addendum 3: execution warmup is experimental and disabled by default.
// The worklist reports that after installation at 2026-09-12 20:22, fingerprint
// d2212835fcb1, the first launch never showed its main window and left the
// splash visible indefinitely. Execution warmup is not a proven cause, but it
// can kill a just-started Deep Legends.exe. Until its benefit is established,
// require DEEP_LEGENDS_STARTUP_EXECUTE_WARM=1 to opt in to that experiment.
// Preserve the final-file execution implementation and its nonblocking Finish;
// both final-file reads remain independent of execution and enabled by default.
const executableStableInterval = 400 * time.Millisecond

type warmFileStamp struct{ info os.FileInfo }

func (s warmFileStamp) same(other warmFileStamp) bool {
	return s.info != nil && other.info != nil && s.info.Size() == other.info.Size() &&
		s.info.ModTime().Equal(other.info.ModTime()) && os.SameFile(s.info, other.info)
}

// Check only metadata and read access; do not read a still-growing executable.
func inspectWarmExecutable(path string) (warmFileStamp, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return warmFileStamp{}, err
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 {
		return warmFileStamp{}, errors.New("not a nonempty regular executable")
	}
	file, err := os.Open(path)
	if err != nil {
		return warmFileStamp{}, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return warmFileStamp{}, err
	}
	stamp := warmFileStamp{info}
	if !stamp.same(warmFileStamp{opened}) {
		return warmFileStamp{}, errors.New("file changed while opening")
	}
	return stamp, nil
}

type warmTarget struct {
	Name, Path string
	NodeMode   bool
}
type warmExit struct {
	Code      int
	Err       error
	ElapsedMS int64
}
type executeWarmHooks struct {
	Inspect func(string) (warmFileStamp, error)
	Run     func(context.Context, warmTarget) warmExit
	Log     func(string, ...any)
}
type executableWarmState struct {
	target                       warmTarget
	baseline, observed, executed warmFileStamp
	stableSince, launchedAt      time.Time
	readyMS                      int64
	attempted, voided            bool
	result                       chan warmExit
	cancel                       context.CancelFunc
}

// Diagnostic counts only. Paths lists executions not classified as complete
// and valid; it must never choose which files receive the unconditional reads.
type warmFallback struct {
	Paths        []string
	ExecuteValid int
}

// The install worker owns all state. Execution goroutines communicate only via
// buffered result channels; Finish never waits for CreateProcess/Wait to return.
type executionWarmup struct {
	enabled   bool
	started   time.Time
	budget    time.Duration
	hooks     executeWarmHooks
	states    []executableWarmState
	ctx       context.Context
	cancel    context.CancelFunc
	finalized bool
	fallback  warmFallback
}

func startupExecuteWarmEnabled(value string) bool {
	return value == "1"
}

func newConfiguredExecutionWarmup(directory string, started time.Time, hooks executeWarmHooks) *executionWarmup {
	enabled := startupPrewarmEnabled(os.Getenv("DEEP_LEGENDS_STARTUP_PREWARM")) &&
		startupExecuteWarmEnabled(os.Getenv("DEEP_LEGENDS_STARTUP_EXECUTE_WARM"))
	return newExecutionWarmup(directory, started, enabled, startupPrewarmBudget, hooks)
}

func newExecutionWarmup(directory string, started time.Time, enabled bool, budget time.Duration, hooks executeWarmHooks) *executionWarmup {
	if hooks.Inspect == nil {
		hooks.Inspect = inspectWarmExecutable
	}
	if hooks.Run == nil {
		hooks.Run = runExecutePrewarm
	}
	if hooks.Log == nil {
		hooks.Log = log.Printf
	}
	w := &executionWarmup{enabled: enabled, started: started, budget: budget, hooks: hooks}
	for index, path := range startupPrewarmPaths(directory) {
		name := "后端"
		if index == 0 {
			name = "主程序"
		}
		state := executableWarmState{target: warmTarget{Name: name, Path: path, NodeMode: index == 0}, readyMS: -1}
		if enabled {
			state.baseline, _ = hooks.Inspect(path)
		}
		w.states = append(w.states, state)
	}
	return w
}

func installationStillRunning(finished <-chan struct{}) bool {
	select {
	case <-finished:
		return false
	default:
		return true
	}
}

func (w *executionWarmup) Poll(now time.Time, finished <-chan struct{}) {
	if w == nil || !w.enabled || w.finalized || !installationStillRunning(finished) {
		return
	}
	for index := range w.states {
		state := &w.states[index]
		stamp, err := w.hooks.Inspect(state.target.Path)
		if state.attempted {
			if err != nil || !stamp.same(state.executed) {
				state.voided = true
				state.cancel()
			}
			continue
		}
		if err != nil || stamp.same(state.baseline) {
			// An unchanged executable from the old installation is not new output.
			state.observed, state.stableSince = warmFileStamp{}, time.Time{}
			continue
		}
		if !stamp.same(state.observed) {
			state.observed, state.stableSince = stamp, now
			continue
		}
		if now.Sub(state.stableSince) < executableStableInterval {
			continue
		}
		if !installationStillRunning(finished) {
			return
		}
		// Start the shared 8s execution budget when the first file is ready,
		// not before the ~20s NSIS extraction has produced any executable.
		if w.ctx == nil {
			w.ctx, w.cancel = context.WithTimeout(context.Background(), w.budget)
		}
		if w.ctx.Err() != nil {
			continue
		}
		ctx, cancel := context.WithCancel(w.ctx)
		state.cancel, state.attempted, state.executed = cancel, true, stamp
		state.readyMS, state.launchedAt = now.Sub(w.started).Milliseconds(), time.Now()
		state.result = make(chan warmExit, 1)
		target, result, run := state.target, state.result, w.hooks.Run
		go func() {
			defer cancel()
			if ctx.Err() != nil || !installationStillRunning(finished) {
				result <- warmExit{Code: -1, Err: context.Canceled}
				return
			}
			result <- run(ctx, target)
		}()
	}
}

func (w *executionWarmup) Cancel() {
	if w != nil && w.cancel != nil {
		w.cancel()
	}
}

// NSIS has exited. Record completion and final-file identity without waiting
// for CreateProcess/Wait. The result is diagnostic: completeStartupWarmup reads
// both final files for every outcome, including ok and still_running.
func (w *executionWarmup) Finish() warmFallback {
	if w == nil {
		return warmFallback{}
	}
	if w.finalized {
		return w.fallback
	}
	w.finalized = true
	expired := w.ctx != nil && errors.Is(w.ctx.Err(), context.DeadlineExceeded)
	w.Cancel()
	if !w.enabled {
		for _, state := range w.states {
			w.fallback.Paths = append(w.fallback.Paths, state.target.Path)
		}
		w.hooks.Log("startup warm mode=execute enabled=false")
		return w.fallback
	}
	for index := range w.states {
		state := &w.states[index]
		outcome := warmExit{Code: -1}
		reason, done := "not_ready", false
		if state.attempted {
			select {
			case outcome = <-state.result:
				done = true
			default:
			}
			current, err := w.hooks.Inspect(state.target.Path)
			state.voided = state.voided || err != nil || !current.same(state.executed)
			switch {
			case state.voided:
				reason = "file_changed"
			case !done && expired:
				reason = "timeout"
			case !done:
				reason = "still_running"
			case outcome.Err != nil || outcome.Code != 0:
				reason = "execute_failed"
			default:
				reason = "ok"
			}
			if !done {
				outcome.ElapsedMS = time.Since(state.launchedAt).Milliseconds()
			}
		} else if expired {
			reason = "budget_exhausted"
		}
		if reason == "ok" {
			w.fallback.ExecuteValid++
		} else {
			w.fallback.Paths = append(w.fallback.Paths, state.target.Path)
		}
		w.hooks.Log("startup warm mode=execute file=%s ready_ms=%d exit=%d elapsed_ms=%d voided=%t attempted=%t reason=%s error=%q",
			state.target.Name, state.readyMS, outcome.Code, outcome.ElapsedMS, state.voided, state.attempted, reason, fmt.Sprint(outcome.Err))
	}
	return w.fallback
}
