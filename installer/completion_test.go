package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"sync/atomic"
	"testing"
	"time"
)

func TestInstallationSuccessAlwaysRunsHandoffIncludingUpdate(t *testing.T) {
	for _, update := range []bool{false, true} {
		for _, visible := range []bool{false, true} {
			t.Run(fmt.Sprintf("update=%t/visible=%t", update, visible), func(t *testing.T) {
				args := []string{}
				if update {
					args = []string{"--update", "--dest", "destination"}
				}
				options := parseInstallerOptions(args)
				deadline, ticks := make(chan time.Time, 1), make(chan time.Time, 1)
				queried, done := make(chan uint32, 2), make(chan struct{})
				var cleaned, closed, windowVisible atomic.Bool
				var handoffs atomic.Int32
				go func() {
					defer close(done)
					completeInstallation(options, installationResult{Destination: "destination"}, installationCompletionHooks{
						Failed:  func(failureMessage) { t.Error("successful installation reported failure") },
						Cleanup: func() { cleaned.Store(true) },
						Handoff: func() {
							handoffs.Add(1)
							runApplicationHandoff(handoffHooks{
								ShowStarting: func() {},
								Start: func() (uint32, error) {
									if !cleaned.Load() || closed.Load() {
										t.Error("launch must follow cleanup while installer is still open")
									}
									return 42, nil
								},
								HasWindow:   func(pid uint32) bool { queried <- pid; return windowVisible.Load() },
								LaunchError: func(error) { t.Error("unexpected launch failure") },
								CloseInstaller: func(shown bool) {
									if shown != visible {
										t.Errorf("wrong completion result: %t", shown)
									}
									closed.Store(true)
								},
							}, func(time.Duration, time.Duration) (<-chan time.Time, <-chan time.Time, func()) {
								return deadline, ticks, func() {}
							})
						},
					})
				}()
				select {
				case pid := <-queried:
					if pid != 42 || closed.Load() {
						t.Fatal("handoff did not wait for the launched application's window")
					}
				case <-done:
					t.Fatal("successful installation bypassed application handoff")
				case <-time.After(3 * time.Second):
					t.Fatal("successful installation never entered application handoff")
				}
				if visible {
					windowVisible.Store(true)
					ticks <- time.Now()
				} else {
					deadline <- time.Now()
				}
				select {
				case <-done:
					if handoffs.Load() != 1 || !closed.Load() {
						t.Fatal("handoff must finish exactly once")
					}
				case <-time.After(3 * time.Second):
					t.Fatal("handoff never released installer")
				}
			})
		}
	}
}

func TestInstallationFailureNeverLaunchesApplication(t *testing.T) {
	for _, update := range []bool{false, true} {
		for _, result := range []installationResult{{ExitCode: 2}, {WaitError: errors.New("wait failed")}} {
			failures := 0
			completeInstallation(installerOptions{Update: update}, result, installationCompletionHooks{
				Failed: func(f failureMessage) {
					failures++
					if update && f.Message != upgradeFailureMessage {
						t.Error("upgrade failure text changed")
					}
				},
				Cleanup: func() { t.Error("success cleanup on a failed installation") },
				Handoff: func() { t.Error("launched application after installation failed") },
			})
			if failures != 1 {
				t.Fatal("failure not reported exactly once")
			}
		}
	}
}

// The helper is this test executable, not the real application or an installer.
func TestInstallerExitChildProcess(t *testing.T) {
	if os.Getenv("R82_EXIT_TEST_CHILD") != "1" {
		return
	}
	fmt.Println("ready")
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		if scanner.Text() == "quit" {
			return
		}
		fmt.Println("alive")
	}
}

func TestInstallerExitOnlyClosesItsWindowAndKeepsChildAlive(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	child := exec.Command(executable, "-test.run=^TestInstallerExitChildProcess$")
	child.Env = append(os.Environ(), "R82_EXIT_TEST_CHILD=1")
	input, err := child.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	output, err := child.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = input.Close(); _ = child.Process.Kill(); _ = child.Wait() })
	lines := make(chan string, 4)
	go func() {
		scanner := bufio.NewScanner(output)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
		close(lines)
	}()
	readLine := func(want string) {
		t.Helper()
		select {
		case got := <-lines:
			if got != want {
				t.Fatalf("child process was terminated or did not respond: %q", got)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("child process did not respond")
		}
	}
	readLine("ready")
	for _, visible := range []bool{true, false} {
		var operations []string
		var dispatched func()
		const ownWindow = uintptr(123)
		finishInstallerHandoff(ownWindow, visible, installerExitHooks{
			Record: func(result bool) {
				if result != visible {
					t.Error("outcome lost")
				}
				operations = append(operations, "record")
			},
			Dispatch:  func(task func()) { operations = append(operations, "dispatch"); dispatched = task },
			MarkReady: func() { operations = append(operations, "ready") },
			CloseWindow: func(window uintptr) {
				if window != ownWindow {
					t.Errorf("cleanup targeted another window: %d", window)
				}
				operations = append(operations, "close-self")
			},
		})
		if !reflect.DeepEqual(operations, []string{"record", "dispatch"}) || dispatched == nil {
			t.Fatal("cleanup ran outside UI dispatch or before logging")
		}
		dispatched()
		if !reflect.DeepEqual(operations, []string{"record", "dispatch", "ready", "close-self"}) {
			t.Fatal(operations)
		}
		if _, err := fmt.Fprintln(input, "ping"); err != nil {
			t.Fatal("cleanup terminated child", err)
		}
		readLine("alive")
	}
}
