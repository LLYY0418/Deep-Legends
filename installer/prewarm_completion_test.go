package main

import (
	"context"
	"go/ast"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestFinalDirectoryReadsBothFilesForEveryExecutionOutcome(t *testing.T) {
	for _, outcome := range []string{"ok", "mixed", "failed", "still_running", "timeout", "voided", "missing", "not_started", "no_manager", "read_error"} {
		t.Run(outcome, func(t *testing.T) {
			entered, release := make(chan struct{}, 2), make(chan struct{})
			releaseOnce := sync.OnceFunc(func() { close(release) })
			defer releaseOnce()
			budget := time.Second
			if outcome == "timeout" {
				budget = 20 * time.Millisecond
			}
			w, directory, finished, lines := warmFixture(t, budget, func(_ context.Context, target warmTarget) warmExit {
				entered <- struct{}{}
				if outcome == "still_running" || outcome == "timeout" {
					<-release
				}
				if outcome == "failed" || (outcome == "mixed" && !target.NodeMode) {
					return warmExit{Code: 7, Err: os.ErrPermission}
				}
				return warmExit{Code: 0}
			})
			paths := startupPrewarmPaths(directory)
			for index, path := range paths {
				if outcome == "missing" && index == 0 {
					continue
				}
				writeWarmFile(t, path, "complete final executable")
			}
			if outcome != "not_started" && outcome != "no_manager" {
				w.Poll(w.started, finished)
				w.Poll(w.started.Add(400*time.Millisecond), finished)
				attempts := 2
				if outcome == "missing" {
					attempts = 1
				}
				for range attempts {
					select {
					case <-entered:
					case <-time.After(time.Second):
						t.Fatal("execution did not start")
					}
				}
				if outcome == "timeout" {
					select {
					case <-w.ctx.Done():
					case <-time.After(time.Second):
						t.Fatal("execution budget ignored")
					}
				}
				if outcome != "still_running" && outcome != "timeout" {
					for index := range w.states {
						if w.states[index].attempted {
							awaitWarmResult(t, &w.states[index])
						}
					}
				}
				if outcome == "voided" {
					writeWarmFile(t, paths[0], "changed final executable after execution")
				}
			}
			close(finished)
			execution := w
			if outcome == "no_manager" {
				execution = nil
			}
			var reads []string
			var progress []int
			type completed struct {
				read      prewarmResult
				execution warmFallback
			}
			done := make(chan completed, 1)
			go func() {
				result, status := completeStartupWarmup(directory, execution, func(ctx context.Context, path string) error {
					reads = append(reads, path)
					if outcome == "read_error" && path == paths[0] {
						return os.ErrPermission
					}
					return readPrewarmFile(ctx, path)
				}, time.Second, func(percent int) { progress = append(progress, percent) })
				done <- completed{result, status}
			}()
			var got completed
			select {
			case got = <-done:
			case <-time.After(500 * time.Millisecond):
				releaseOnce()
				<-done
				t.Fatal("reads or Finish waited for execution")
			}
			if !reflect.DeepEqual(reads, paths) || got.read.Files != 2 || !reflect.DeepEqual(progress, []int{98, 100}) {
				t.Fatal("execution outcome changed unconditional final-file reads", reads, got, progress)
			}
			failures := 0
			if outcome == "missing" || outcome == "read_error" {
				failures = 1
			}
			if got.read.Failures != failures || got.read.TimedOut {
				t.Fatal(got)
			}
			valid := 0
			switch outcome {
			case "ok", "read_error":
				valid = 2
			case "mixed", "voided", "missing":
				valid = 1
			}
			if got.execution.ExecuteValid != valid || len(got.execution.Paths) != 2-valid {
				t.Fatal("diagnostic counts lost", got.execution)
			}
			if outcome == "still_running" && strings.Count(strings.Join(*lines, "\n"), "reason=still_running") != 2 {
				t.Fatal(*lines)
			}
		})
	}
}

func TestFinalDirectoryExecutionPathsAreExactlyInstalledPaths(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "最终目录 Deep Legends")
	paths := startupPrewarmPaths(directory)
	started := make(chan warmTarget, 2)
	w := newExecutionWarmup(directory, time.Now(), true, time.Second, executeWarmHooks{
		Run: func(_ context.Context, target warmTarget) warmExit { started <- target; return warmExit{Code: 0} }, Log: func(string, ...any) {},
	})
	defer w.Cancel()
	for index, state := range w.states {
		if state.target.Path != paths[index] {
			t.Fatal("execution points outside final directory", state.target.Path, paths[index])
		}
		writeWarmFile(t, paths[index], "final executable")
	}
	finished := make(chan struct{})
	w.Poll(w.started, finished)
	w.Poll(w.started.Add(400*time.Millisecond), finished)
	got := map[string]bool{}
	for range 2 {
		select {
		case target := <-started:
			got[target.Path] = true
			ctx, cancel := context.WithCancel(context.Background())
			command := executeWarmCommand(ctx, target)
			cancel()
			if command.Path != target.Path || command.Dir != filepath.Dir(target.Path) {
				t.Fatal("command redirected execution", command.Path, command.Dir)
			}
		case <-time.After(time.Second):
			t.Fatal("final execution did not start")
		}
	}
	for _, path := range paths {
		if !got[path] {
			t.Fatal(got)
		}
	}
}

func TestFinalDirectoryUpgradeExecutesOnlyChangedFiles(t *testing.T) {
	directory := t.TempDir()
	paths := startupPrewarmPaths(directory)
	for _, path := range paths {
		writeWarmFile(t, path, "old executable")
	}
	entered := make(chan string, 2)
	w := newExecutionWarmup(directory, time.Now(), true, time.Second, executeWarmHooks{
		Run: func(_ context.Context, target warmTarget) warmExit { entered <- target.Path; return warmExit{Code: 0} }, Log: func(string, ...any) {},
	})
	defer w.Cancel()
	finished := make(chan struct{})
	w.Poll(w.started, finished)
	w.Poll(w.started.Add(time.Second), finished)
	for _, state := range w.states {
		if state.attempted {
			t.Fatal("old executable treated as new")
		}
	}
	writeWarmFile(t, paths[0], "new installed executable")
	w.Poll(w.started.Add(2*time.Second), finished)
	w.Poll(w.started.Add(2400*time.Millisecond), finished)
	select {
	case path := <-entered:
		if path != paths[0] {
			t.Fatal(path)
		}
	case <-time.After(time.Second):
		t.Fatal("changed executable did not start")
	}
	awaitWarmResult(t, &w.states[0])
	if w.states[1].attempted {
		t.Fatal("unchanged backend was executed")
	}
}

func TestFinalDirectoryExecutionBudgetStartsAtStableFile(t *testing.T) {
	w, directory, finished, _ := warmFixture(t, 250*time.Millisecond, func(context.Context, warmTarget) warmExit { return warmExit{Code: 0} })
	w.started = time.Now().Add(-time.Hour)
	w.Poll(time.Now(), finished)
	if w.ctx != nil {
		t.Fatal("budget started before final files were ready")
	}
	for _, path := range startupPrewarmPaths(directory) {
		writeWarmFile(t, path, "complete")
	}
	now := time.Now()
	w.Poll(now, finished)
	w.Poll(now.Add(400*time.Millisecond), finished)
	deadline, ok := w.ctx.Deadline()
	if !ok || time.Until(deadline) < 150*time.Millisecond || time.Until(deadline) > 250*time.Millisecond {
		t.Fatal("execution budget changed", deadline)
	}
}

func TestFinalDirectoryWindowsAdapterUsesFinalPathAndUnconditionalReads(t *testing.T) {
	install := completionFunction(t, "install_windows.go", "install")
	found := 0
	ast.Inspect(install.Body, func(node ast.Node) bool {
		statement, ok := node.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for _, expression := range statement.Rhs {
			call, ok := expression.(*ast.CallExpr)
			if !ok {
				continue
			}
			name, ok := call.Fun.(*ast.Ident)
			if !ok || name.Name != "newConfiguredExecutionWarmup" {
				continue
			}
			found++
			assertCompletionBlock(t, []ast.Stmt{statement}, `a.startupWarm = newConfiguredExecutionWarmup(dest, installationStarted, executeWarmHooks{})`)
		}
		return true
	})
	if found != 1 {
		t.Fatal("final execution adapter missing", found)
	}
	handoff := completionFunction(t, "handoff_windows.go", "handoffApplication")
	branch, ok := handoff.Body.List[1].(*ast.IfStmt)
	if !ok {
		t.Fatal("prewarm enable switch missing")
	}
	assertCompletionBlock(t, []ast.Stmt{&ast.ExprStmt{X: branch.Cond}}, `startupPrewarmEnabled(os.Getenv("DEEP_LEGENDS_STARTUP_PREWARM"))`)
	assertCompletionBlock(t, branch.Body.List[1:], `
progress(97)
result, executionStatus := completeStartupWarmup(directory, a.startupWarm, readPrewarmFile, startupPrewarmBudget, progress)
log.Printf("startup prewarm enabled=true files=%d failures=%d timed_out=%t elapsed_ms=%d execute_valid=%d fallback_files=%d", result.Files, result.Failures, result.TimedOut, result.ElapsedMS, executionStatus.ExecuteValid, len(executionStatus.Paths))`)
}

func TestFinalDirectoryHasNoStagingExecutionBranchOrLogFields(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, obsolete := range []string{"source=staging", "final_verified", "source_removed", "discovered_ms", "nsisExtractionDirectory", "executeVerifiedWarmup", "verifyWarmDestination", "warmDigest", "retryWarmCleanup"} {
			if strings.Contains(string(data), obsolete) {
				t.Errorf("obsolete execution branch/field %q in %s", obsolete, name)
			}
		}
	}
}

func TestFinalDirectoryStillRunningOnlyRecordsStatus(t *testing.T) {
	function := completionFunction(t, "execute_prewarm.go", "Finish")
	found := 0
	ast.Inspect(function.Body, func(node ast.Node) bool {
		clause, ok := node.(*ast.CaseClause)
		if !ok || len(clause.Body) == 0 {
			return true
		}
		assignment, ok := clause.Body[0].(*ast.AssignStmt)
		if !ok || len(assignment.Rhs) != 1 {
			return true
		}
		literal, ok := assignment.Rhs[0].(*ast.BasicLit)
		if !ok || literal.Value != `"still_running"` {
			return true
		}
		found++
		assertCompletionBlock(t, clause.Body, `reason = "still_running"`)
		return false
	})
	if found != 1 {
		t.Fatal("still_running was removed or reclassified", found)
	}
}
