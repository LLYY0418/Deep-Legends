package main

import (
	"encoding/json"
	"errors"
	"go/ast"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"deeplegends/installer/internal/webviewhost"
)

// Read the production Windows adapter's format, not a second hand-written copy
// of it. Its existing AST guard separately fixes the production argument wiring.
func productionHandoffLogFormat(t *testing.T, prefix string) string {
	t.Helper()
	function := completionFunction(t, "handoff_windows.go", "handoffApplication")
	var formats []string
	ast.Inspect(function.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || len(call.Args) == 0 {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != "Printf" {
			return true
		}
		owner, ok := selector.X.(*ast.Ident)
		if !ok || owner.Name != "log" {
			return true
		}
		literal, ok := call.Args[0].(*ast.BasicLit)
		if !ok {
			return true
		}
		value, err := strconv.Unquote(literal.Value)
		if err == nil && strings.HasPrefix(value, prefix) {
			formats = append(formats, value)
		}
		return true
	})
	if len(formats) != 1 {
		t.Fatalf("expected one production %q formatter, got %v", prefix, formats)
	}
	return formats[0]
}

func TestStartupLogAndRealCollectorStayCoupledForSuccessAndLaunchFailure(t *testing.T) {
	root := t.TempDir()
	for _, key := range []string{"TMPDIR", "TEMP", "TMP"} {
		t.Setenv(key, root)
	}
	previousFlags := log.Flags()
	t.Cleanup(func() { log.SetFlags(previousFlags) })
	log.SetFlags(log.LstdFlags | log.Lmsgprefix) // StartupLog must override unsafe inherited flags
	closeLog := webviewhost.StartupLog("DeepLegendsSetup")
	log.Printf(productionHandoffLogFormat(t, "startup prewarm enabled=true"), 2, 0, false, 46, 0, 2)
	pid, err := logApplicationStart(func() (uint32, error) { return 77, nil }, time.Now().Add(-50*time.Millisecond))
	if pid != 77 || err != nil {
		t.Fatal(pid, err)
	}
	failure := errors.New("synthetic CreateProcess failure")
	pid, err = logApplicationStart(func() (uint32, error) { return 0, failure }, time.Now().Add(-50*time.Millisecond))
	if pid != 0 || !errors.Is(err, failure) {
		t.Fatal(pid, err)
	}
	log.Printf(productionHandoffLogFormat(t, "application handoff"), true, 1234)
	closeLog()
	if log.Flags() != log.LstdFlags|log.Lmsgprefix {
		t.Fatal("StartupLog did not restore flags")
	}
	bytes, err := os.ReadFile(webviewhost.StartupLogPath("DeepLegendsSetup"))
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(map[string]any{"log": string(bytes), "pid": os.Getpid()})
	fixture := filepath.Join(root, "fixture.json")
	if err := os.WriteFile(fixture, payload, 0600); err != nil {
		t.Fatal(err)
	}
	report, err := filepath.Abs("../scripts/r82-startup-ab-report.cjs")
	if err != nil {
		t.Fatal(err)
	}
	// Execute the actual JS parser and collector against actual Go logger bytes.
	script := `
const assert=require('node:assert/strict'),fs=require('node:fs');
const {parseInstallerStartupLog,collectRecord}=require(process.argv[1]);
const fixture=JSON.parse(fs.readFileSync(process.argv[2]));
const lines=fixture.log.trim().split(/\r?\n/);
assert.ok(lines.every(line => new RegExp('^\\[pid='+fixture.pid+'\\] \\d{4}/\\d{2}/\\d{2} \\d{2}:\\d{2}:\\d{2}\\.\\d{6} ').test(line)),fixture.log);
const parsed=parseInstallerStartupLog(fixture.log,fixture.pid);
assert.equal(parsed.launches.length,2, 'both success and failure must carry elapsed_ms');
assert.equal(parsed.launches[0].pid,77);assert.equal(parsed.launches[0].error,'<nil>');
assert.equal(parsed.launches[1].pid,0);assert.equal(parsed.launches[1].error,'synthetic CreateProcess failure');
assert.ok(parsed.launches.every(row => row.elapsed_ms>=50));
const time='2026-09-12T00:00:00Z',fingerprint='abcdef012345',run_id='real-go-logger';
const input={records:[],group:'prewarm',fingerprint,pid:fixture.pid,since:Date.parse(time),startupLog:fixture.log,diagnostics:[
 {event:'app_start',build_fingerprint:fingerprint,run_id,time},
 {event:'desktop_startup_phases_ms',run_id,time,log_seq:2,phases_ms:{process_to_js:100,spawn_to_ready:200,total:300,splash_window_shown:250}}]};
const result=collectRecord(input);
assert.ok(result,'real StartupLog output did not reach the actual collector');
assert.equal(result.prewarm_ms,46);assert.equal(result.handoff_ms,1234);
assert.equal(collectRecord({...input,pid:fixture.pid+1}),null,'collector ignored PID attribution');
`
	output, err := exec.Command("node", "-e", script, report, fixture).CombinedOutput()
	if err != nil {
		t.Fatalf("real StartupLog/collector contract failed: %v\n%s", err, output)
	}
}

func TestWindowsRealLaunchMustUseSanitizedCommandAndTimedLogger(t *testing.T) {
	function := completionFunction(t, "handoff_windows.go", "handoffApplication")
	found := 0
	ast.Inspect(function.Body, func(node ast.Node) bool {
		field, ok := node.(*ast.KeyValueExpr)
		if !ok {
			return true
		}
		key, ok := field.Key.(*ast.Ident)
		if !ok || key.Name != "Start" {
			return true
		}
		callback, ok := field.Value.(*ast.FuncLit)
		if !ok {
			t.Fatal("explicit launch adapter required")
		}
		found++
		assertCompletionBlock(t, callback.Body.List, `return logApplicationStart(func() (uint32,error) { return startApplication(exe,directory) },started)`)
		return false
	})
	if found != 1 {
		t.Fatal("missing timed application launch adapter")
	}
	start := completionFunction(t, "application_launch.go", "startApplication")
	assertCompletionBlock(t, start.Body.List[:1], `command := applicationCommand(exe,directory)`)
}

func TestWindowsPollStartsExecutionBeforeCompletionWithoutChangingCompletionPolicy(t *testing.T) {
	install := completionFunction(t, "install_windows.go", "install")
	found := 0
	ast.Inspect(install.Body, func(node ast.Node) bool {
		clause, ok := node.(*ast.CommClause)
		if !ok {
			return true
		}
		expr, ok := clause.Comm.(*ast.ExprStmt)
		if !ok {
			return true
		}
		receive, ok := expr.X.(*ast.UnaryExpr)
		if !ok {
			return true
		}
		selector, ok := receive.X.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != "C" {
			return true
		}
		owner, ok := selector.X.(*ast.Ident)
		if !ok || owner.Name != "ticker" {
			return true
		}
		found++
		assertCompletionBlock(t, clause.Body[:1], `a.startupWarm.Poll(time.Now(),finished)`)
		return false
	})
	if found != 1 {
		t.Fatal("execution warmup is not wired into active NSIS polling")
	}
	function := completionFunction(t, "execute_command_windows.go", "configureWarmProcess")
	assertCompletionBlock(t, function.Body.List, `cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow:true, CreationFlags:windows.CREATE_NO_WINDOW}`)
}
