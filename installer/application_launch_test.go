package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestLaunchEnvironmentChild(t *testing.T) {
	if os.Getenv("R83_LAUNCH_CHILD") != "1" {
		return
	}
	if os.Getenv("R83_LAUNCH_BLOCK") == "1" {
		for {
			time.Sleep(time.Hour)
		}
	}
	var present []string
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if strings.EqualFold(name, "ELECTRON_RUN_AS_NODE") {
			present = append(present, entry)
		}
	}
	if present == nil {
		present = []string{}
	}
	_ = json.NewEncoder(os.Stdout).Encode(present)
	os.Exit(0)
}

func TestExecuteWarmEnvironmentCannotLeakIntoRealApplication(t *testing.T) {
	// This guard must exercise the real launch even with execution warmup off.
	unsetWarmEnvironment(t, "DEEP_LEGENDS_STARTUP_EXECUTE_WARM")
	unsetWarmEnvironment(t, "DEEP_LEGENDS_STARTUP_PREWARM")
	base := []string{"KEEP=value", "ELECTRON_RUN_AS_NODE=old", "electron_run_as_node=lower", "NODE_OPTIONS=--inspect", "NODE_V8_COVERAGE=bad"}
	original := append([]string(nil), base...)
	warm := executeWarmEnvironment(base, true)
	if !reflect.DeepEqual(base, original) {
		t.Fatal("warmup mutated parent environment")
	}
	if !reflect.DeepEqual(warm, []string{"KEEP=value", "ELECTRON_RUN_AS_NODE=1", "ELECTRON_NO_ATTACH_CONSOLE=1"}) {
		t.Fatal(warm)
	}
	backend := executeWarmEnvironment(base, false)
	if !reflect.DeepEqual(backend, []string{"KEEP=value"}) {
		t.Fatal(backend)
	}
	if !reflect.DeepEqual(applicationEnvironment(base), []string{"KEEP=value", "NODE_OPTIONS=--inspect", "NODE_V8_COVERAGE=bad"}) {
		t.Fatal("normal launch still has Node-only mode")
	}

	t.Setenv("ELECTRON_RUN_AS_NODE", "external")
	t.Setenv("R83_LAUNCH_CHILD", "1")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	run := func(cmd *exec.Cmd) []string {
		cmd.Args = []string{executable, "-test.run=^TestLaunchEnvironmentChild$"}
		bytes, err := cmd.Output()
		if err != nil {
			t.Fatal(err)
		}
		var values []string
		if err := json.Unmarshal(bytes, &values); err != nil {
			t.Fatal(err, string(bytes))
		}
		return values
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if got := run(executeWarmCommand(ctx, warmTarget{Path: executable, NodeMode: true})); !reflect.DeepEqual(got, []string{"ELECTRON_RUN_AS_NODE=1"}) {
		t.Fatal(got)
	}
	if os.Getenv("ELECTRON_RUN_AS_NODE") != "external" {
		t.Fatal("warmup changed installer process environment")
	}
	if got := run(applicationCommand(executable, t.TempDir())); len(got) != 0 {
		t.Fatal("real child inherited Node-only mode", got)
	}
}

func TestExecuteWarmCommandsUseOnlyImmediateExitEntrypoints(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	node := executeWarmCommand(ctx, warmTarget{Path: "app.exe", NodeMode: true})
	backend := executeWarmCommand(ctx, warmTarget{Path: "backend.exe"})
	if !reflect.DeepEqual(node.Args, []string{"app.exe", "-e", "process.exit(0)"}) || !reflect.DeepEqual(backend.Args, []string{"backend.exe", "--startup-warmup"}) {
		t.Fatalf("unsafe warmup entrypoint: %v / %v", node.Args, backend.Args)
	}
	if node.Cancel == nil || backend.Cancel == nil {
		t.Fatal("warmup must use CommandContext cancellation")
	}
}

func TestExecuteWarmCommandCancellationReapsOnlyItsChild(t *testing.T) {
	t.Setenv("R83_LAUNCH_CHILD", "1")
	t.Setenv("R83_LAUNCH_BLOCK", "1")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := executeWarmCommand(ctx, warmTarget{Path: executable})
	cmd.Args = []string{executable, "-test.run=^TestLaunchEnvironmentChild$"}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Process.Kill()
	cancel()
	waited := make(chan error, 1)
	go func() { waited <- cmd.Wait() }()
	select {
	case err := <-waited:
		if err == nil {
			t.Fatal("cancelled warm process returned success")
		}
	case <-time.After(time.Second):
		_ = cmd.Process.Kill()
		<-waited
		t.Fatal("warmup cancellation did not stop its child")
	}
	// Wait populates ProcessState for both a killed Unix child and a Windows child.
	if cmd.ProcessState == nil {
		t.Fatal("warm process was not reaped")
	}
}
