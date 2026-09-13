#!/usr/bin/env python3
"""R83 mutation proof in disposable copies/Go overlays; never build a Setup."""
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[1]
CASES = [
    ('execute opt-in: defaults on', 'installer/execute_prewarm.go', 'return value == "1"', 'return value != "0"', 'TestExecuteWarmConfigDefaultsOff'),
    ('execute opt-in: explicit one disabled', 'installer/execute_prewarm.go', 'return value == "1"', 'return false', 'TestExecuteWarmConfigExplicitOne'),
    ('execute opt-in: adapter bypasses gate', 'installer/execute_prewarm.go', 'startupExecuteWarmEnabled(os.Getenv("DEEP_LEGENDS_STARTUP_EXECUTE_WARM"))', 'true', 'TestExecuteWarmConfigDefaultsOff'),
    ('execute opt-in: missing disabled log', 'installer/execute_prewarm.go', 'w.hooks.Log("startup warm mode=execute enabled=false")', '// disabled log removed', 'TestExecuteWarmConfigDefaultsOff'),
    ('execute opt-in: read warmup depends on execute switch', 'installer/prewarm_completion.go', 'result := runStartupPrewarm(startupPrewarmPaths(directory), read, budget, progress)', 'result := prewarmResult{}; if execution != nil && execution.enabled { result = runStartupPrewarm(startupPrewarmPaths(directory), read, budget, progress) }', 'TestExecuteWarmConfigDefaultsOff'),
    ('A: default remains off', 'installer/prewarm.go', 'return value != "0"', 'return value == "1"', 'TestPrewarmDefaultsOn'),
    ('A: cannot disable', 'installer/prewarm.go', 'return value != "0"', 'return true', 'TestPrewarmDefaultsOn'),
    ('A: unknown values disable', 'installer/prewarm.go', 'return value != "0"', 'return value == "" || value == "1"', 'TestPrewarmDefaultsOn'),
    ('A: explicit enable disabled', 'installer/prewarm.go', 'return value != "0"', 'return value != "0" && value != "1"', 'TestPrewarmDefaultsOn'),
    ('B: real launch inherits Node mode', 'installer/application_launch.go', 'if !strings.EqualFold(name, "ELECTRON_RUN_AS_NODE") {', 'if !strings.EqualFold(name, "unused") {', 'TestExecuteWarmEnvironment'),
    ('B: execute after NSIS exit', 'installer/execute_prewarm.go', 'w.finalized || !installationStillRunning(finished)', 'w.finalized || installationStillRunning(finished)', 'TestExecuteWarmupStartsBoth'),
    ('B: skip stable interval', 'installer/execute_prewarm.go', 'const executableStableInterval = 400 * time.Millisecond', 'const executableStableInterval = 0 * time.Millisecond', 'TestExecuteWarmupStartsBoth'),
    ('B: ignore growing file', 'installer/execute_prewarm.go', 'if !stamp.same(state.observed) {', 'if state.observed.info == nil {', 'TestExecuteWarmupRequiresStable'),
    ('B: ignore changed mtime', 'installer/execute_prewarm.go', 's.info.ModTime().Equal(other.info.ModTime()) && ', '', 'TestExecuteWarmupVoidsSameSize'),
    ('B: ignore replaced file', 'installer/execute_prewarm.go', ' && os.SameFile(s.info, other.info)', '', 'TestExecuteWarmupVoidsSameSize'),
    ('B: rewritten warmup still accepted', 'installer/execute_prewarm.go', 'state.voided = state.voided || err != nil || !current.same(state.executed)', '_ = current; _ = err; state.voided = false', 'TestExecuteWarmupRequiresStable'),
    ('B: accept failed process', 'installer/execute_prewarm.go', 'case outcome.Err != nil || outcome.Code != 0:', 'case false:', 'TestExecuteWarmupFailures'),
    ('B: missing files do not fall back', 'installer/execute_prewarm.go', '} else {\n\t\t\tw.fallback.Paths = append(w.fallback.Paths, state.target.Path)', '} else if state.attempted {\n\t\t\tw.fallback.Paths = append(w.fallback.Paths, state.target.Path)', 'TestExecuteWarmupFailures'),
    ('B: finish waits for blocked process', 'installer/execute_prewarm.go', 'case outcome = <-state.result:\n\t\t\t\tdone = true\n\t\t\tdefault:', 'case outcome = <-state.result:\n\t\t\t\tdone = true', 'TestExecuteWarmupFailures'),
    ('B: real process cannot be cancelled', 'installer/execute_command.go', 'exec.CommandContext(ctx, target.Path, args...)', 'exec.Command(target.Path, args...)', 'TestExecuteWarmCommandsUseOnly|TestExecuteWarmCommandCancellation'),
    ('B: backend uses side-effecting self-test', 'installer/execute_command.go', '[]string{"--startup-warmup"}', '[]string{"--self-test"}', 'TestExecuteWarmCommandsUseOnly'),
    ('B: Windows polling omits execution', 'installer/install_windows.go', '\t\t\ta.startupWarm.Poll(time.Now(), finished)', '\t\t\t// execution moved out of live polling', 'TestWindowsPollStartsExecution'),
    ('B: logger omits microseconds', 'installer/internal/webviewhost/startup.go', 'log.SetFlags(log.LstdFlags | log.Lmicroseconds)', 'log.SetFlags(log.LstdFlags)', 'TestStartupLogAndRealCollector'),
    ('B: PID no longer at line start', 'installer/internal/webviewhost/startup.go', 'log.SetFlags(log.LstdFlags | log.Lmicroseconds)', 'log.SetFlags(log.LstdFlags | log.Lmicroseconds | log.Lmsgprefix)', 'TestStartupLogAndRealCollector'),
    ('B: handoff format drifts', 'installer/handoff_windows.go', 'application handoff visible=%t elapsed_ms=%d', 'application handoff visible=%t extra=field elapsed_ms=%d', 'TestStartupLogAndRealCollector'),
    ('B: collector filters wrong prefix', 'scripts/r82-startup-ab-report.cjs', 'line.startsWith(`[pid=${pid}] `)', 'line.startsWith(`[process=${pid}] `)', 'TestStartupLogAndRealCollector'),
    ('B: collector regex drifts', 'scripts/r82-startup-ab-report.cjs', 'application handoff visible=(true|false) elapsed_ms=', 'application handoff visible=(true|false) milliseconds=', 'TestStartupLogAndRealCollector'),
    ('B: launch failure loses elapsed', 'installer/application_launch.go', 'log.Printf("application launch pid=%d error=%v elapsed_ms=%d", pid, err, time.Since(started).Milliseconds())', 'if err == nil { log.Printf("application launch pid=%d error=%v elapsed_ms=%d", pid, err, time.Since(started).Milliseconds()) }', 'TestStartupLogAndRealCollector'),
    ('final: successful execution skips reads', 'installer/prewarm_completion.go', 'runStartupPrewarm(startupPrewarmPaths(directory), read, budget, progress)', 'runStartupPrewarm(status.Paths, read, budget, progress)', 'TestFinalDirectoryReadsBoth'),
    ('final: remove all read warmup', 'installer/prewarm_completion.go', 'result := runStartupPrewarm(startupPrewarmPaths(directory), read, budget, progress)', 'result := prewarmResult{}', 'TestFinalDirectoryReadsBoth'),
    ('final: Windows handoff omits warmup', 'installer/handoff_windows.go', 'result, executionStatus := completeStartupWarmup(directory, a.startupWarm, readPrewarmFile, startupPrewarmBudget, progress)', 'result, executionStatus := prewarmResult{}, warmFallback{}', 'TestFinalDirectoryWindowsAdapter'),
    ('final: adapter points at temporary directory', 'installer/install_windows.go', 'newConfiguredExecutionWarmup(dest, installationStarted,', 'newConfiguredExecutionWarmup(temporaryDir, installationStarted,', 'TestFinalDirectoryWindowsAdapter'),
    ('final: manager redirects target paths', 'installer/execute_prewarm.go', 'range startupPrewarmPaths(directory) {', 'range startupPrewarmPaths(os.TempDir()) {', 'TestFinalDirectoryExecutionPaths'),
    ('final: old installed executables count as new', 'installer/execute_prewarm.go', 'state.baseline, _ = hooks.Inspect(path)', 'state.baseline = warmFileStamp{}', 'TestFinalDirectoryUpgrade'),
    ('final: execution budget starts before readiness', 'installer/execute_prewarm.go', 'context.WithTimeout(context.Background(), w.budget)', 'context.WithDeadline(context.Background(), w.started.Add(w.budget))', 'TestFinalDirectoryExecutionBudget'),
    ('final: read count includes execution successes', 'installer/handoff_windows.go', 'result.Files, result.Failures, result.TimedOut', 'result.Files+executionStatus.ExecuteValid, result.Failures, result.TimedOut', 'TestFinalDirectoryWindowsAdapter'),
    ('final: still_running triggers compensating work', 'installer/execute_prewarm.go', 'reason = "still_running"', 'reason = "still_running"; _ = readPrewarmFile(context.Background(), state.target.Path)', 'TestFinalDirectoryStillRunning'),
]


def main():
    env = dict(os.environ, GOCACHE=str(ROOT / ".gocache"), GOMODCACHE=str(ROOT / ".gomodcache"), GOTMPDIR=tempfile.gettempdir(), GOPROXY="off")
    env.pop("GOOS", None)
    env.pop("GOARCH", None)
    evidence = []
    with tempfile.TemporaryDirectory(prefix="r83-mutations-") as directory:
        work = Path(directory)
        shutil.copytree(ROOT / "installer", work / "installer", ignore=shutil.ignore_patterns("*.exe", "node_modules"))
        (work / "scripts").mkdir()
        shutil.copyfile(ROOT / "scripts/r82-startup-ab-report.cjs", work / "scripts/r82-startup-ab-report.cjs")

        def run(pattern):
            return subprocess.run(["go", "test", "-count=1", "-timeout=15s", "-run", "^(" + pattern + ")", "."],
                                  cwd=work / "installer", env=env, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, timeout=45)

        baseline = run("TestPrewarm|TestExecuteWarm|TestStartupLogAndRealCollector|TestWindowsPollStartsExecution|TestFinalDirectory")
        if baseline.returncode:
            raise SystemExit("BASELINE FAILED:\n" + baseline.stdout)
        print("PASS isolated baseline", flush=True)
        for name, relative, before, after, pattern in CASES:
            file = work / relative
            original = file.read_text()
            if original.count(before) != 1:
                raise SystemExit(f"Mutation target changed: {name}")
            file.write_text(original.replace(before, after))
            try:
                result = run(pattern)
            finally:
                file.write_text(original)
            failures = [line.strip() for line in result.stdout.splitlines() if "--- FAIL:" in line]
            if not result.returncode or not failures:
                raise SystemExit(f"SURVIVED or invalid mutation: {name}\n{result.stdout}")
            evidence.append({"mutation": name, "failed_tests": failures})
            print("KILLED " + name + ": " + "; ".join(failures), flush=True)

        # Mutate the real main entry with Go's overlay support, without editing
        # source or copying embedded assets/caches into the mutation workspace.
        original = (ROOT / "main.go").read_text()
        before = "if *startupWarmup {\n\t\treturn\n\t}"
        if original.count(before) != 1:
            raise SystemExit("Root warmup mutation target changed")
        changed = work / "main.go"
        changed.write_text(original.replace(before, "if *startupWarmup { /* wrongly continue into storage/services */ }"))
        overlay = work / "overlay.json"
        overlay.write_text(json.dumps({"Replace": {str(ROOT / "main.go"): str(changed)}}))
        result = subprocess.run(["go", "test", "-overlay", str(overlay), "-count=1", "-timeout=15s", "-run", "^TestStartupWarmupProcess", "."],
                                cwd=ROOT, env=env, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, timeout=45)
        failures = [line.strip() for line in result.stdout.splitlines() if "--- FAIL:" in line]
        if not result.returncode or not failures:
            raise SystemExit("Backend side-effect mutation survived/invalid:\n" + result.stdout)
        evidence.append({"mutation": "B: backend warmup initializes storage/services", "failed_tests": failures})
        print("KILLED backend side-effect mutation: " + "; ".join(failures), flush=True)
    print(json.dumps({"killed": len(evidence), "mutations": evidence}, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    main()
