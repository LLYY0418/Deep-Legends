package main

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func r313Order(events chan string) []string {
	var out []string
	for len(events) > 0 {
		out = append(out, <-events)
	}
	return out
}
func TestR313ShellAcknowledgementRequiresVisibleWindowAndPreservesProtocol(t *testing.T) {
	opts := installerOptions{Update: true, ParentFirst: true}
	order := make(chan string, 8)
	report := func(line string) error { order <- line; return nil }
	mark := func(s string) { order <- s }
	if ok, err := acknowledgeUpdateShell(opts, false, report, mark); ok || err == nil || len(order) > 0 {
		t.Fatal(ok, err, len(order))
	}
	order <- "page_visible"
	ok, err := acknowledgeUpdateShell(opts, true, report, mark)
	got := r313Order(order)
	if !ok || err != nil || !reflect.DeepEqual(got, []string{"page_visible", "DEEP_LEGENDS_UPDATE_STARTED", "ack_sent"}) {
		t.Fatal(ok, err, got)
	}
	for _, legacy := range []installerOptions{{Update: true}, {}} {
		if ok, err := acknowledgeUpdateShell(legacy, true, func(string) error { t.Fatal("legacy timing changed"); return nil }, mark); ok || err != nil {
			t.Fatal(ok, err)
		}
	}
	if ok, err := acknowledgeUpdateShell(opts, true, func(string) error { return errors.New("closed pipe") }, mark); ok || err == nil {
		t.Fatal("failed ack accepted")
	}
}
func TestR313EveryEarlyAcknowledgedFailureWaitsThenRecoversOriginalApplication(t *testing.T) {
	for _, failure := range []string{"runtime", "webview-embed", "webview-controller-timeout", "webview-timeout", "payloadError", "destination"} {
		t.Run(failure, func(t *testing.T) {
			root := t.TempDir()
			exe := filepath.Join(root, "original.exe")
			os.WriteFile(exe, []byte("old application"), 0600)
			opts := installerOptions{Update: true, ParentFirst: true, Destination: root, RecoveryExe: exe}
			order := make(chan string, 8)
			exited, recovered := recoverAcknowledgedUpdate(opts, "Deep Legends.exe", true, false, func() bool { order <- "parent_exited"; return true }, func(got, dir string) (uint32, error) {
				if got != exe || dir != root {
					t.Fatal(got, dir)
				}
				order <- "recovered"
				return 1, nil
			})
			got := r313Order(order)
			if !exited || !recovered || !reflect.DeepEqual(got, []string{"parent_exited", "recovered"}) {
				t.Fatal(exited, recovered, got)
			}
			_, recovered = recoverAcknowledgedUpdate(opts, "Deep Legends.exe", false, false, func() bool { t.Fatal("unacknowledged parent must keep running"); return true }, func(string, string) (uint32, error) { t.Fatal("duplicate application"); return 1, nil })
			if recovered {
				t.Fatal("recovered live parent")
			}
			_, recovered = recoverAcknowledgedUpdate(opts, "Deep Legends.exe", true, false, func() bool { return false }, func(string, string) (uint32, error) { t.Fatal("live parent"); return 1, nil })
			if recovered {
				t.Fatal("failed wait accepted")
			}
		})
	}
}
func TestR313NativeStartupAndFailureAdaptersKeepRecoveryWired(t *testing.T) {
	main, err := os.ReadFile("main_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(main)
	if strings.Contains(source, "w.show()") || strings.Contains(source, "acknowledgeUpdateShell(") {
		t.Fatal("main must not reveal an unpainted progress page or ACK")
	}
	if strings.Index(source, "webviewhost.CheckRuntime()") > strings.Index(source, "app.embed(") {
		t.Fatal("runtime must be checked before embed")
	}

	if strings.Count(source, "app.recoverFailedUpdate()") != 3 {
		t.Fatal("runtime/document/embed failure adapters must all recover")
	}
	view, err := os.ReadFile("webview_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(view)
	watchdog, factory := strings.Index(s, "watchdog = newStartupWatchdog("), strings.Index(s, "embedded := chromium.Embed(")
	if watchdog < 0 || factory <= watchdog || !strings.Contains(s[watchdog:factory], "recoverAcknowledgedUpdate(options, exeName, acknowledged, false, waitParent, startApplication)") || !strings.Contains(s[watchdog:factory], "time.AfterFunc(webviewhost.PageTimeout, fn)") || !strings.Contains(s[watchdog:factory], "os.Exit(1)") {
		t.Fatal("controller creation must have an independent timeout that recovers before exiting")
	}
	if !strings.Contains(s[factory:], "if watchdog != nil && !watchdog.stop() {") || !strings.Contains(s[factory:], "<-watchdog.done") {
		t.Fatal("expired controller factory must not continue while recovery is in progress")
	}
	start := strings.Index(s, "func (a *installerApp) fail(")
	end := strings.Index(s, "func (a *installerApp) validatePath(")
	if start < 0 || end < start {
		t.Fatal("failure adapter missing")
	}
	failure := s[start:end]
	if strings.Index(failure, "a.recoverFailedUpdate()") < 0 || strings.Index(failure, "a.recoverFailedUpdate()") > strings.Index(failure, `a.emit("failed", failure)`) {
		t.Fatal("payload and destination failures must recover before failure UI")
	}
	if !strings.Contains(s, "w.starting = false\n\t\ta.recoverFailedUpdate()\n\t\ta.startupError = err") {
		t.Fatal("WebView startup failure must recover before closing shell")
	}
	ready, ack := strings.Index(s, "a.pageReady = true"), strings.Index(s, "a.ackSent, err = acknowledgeUpdateShell(")
	if ready < 0 || ack < ready || !strings.Contains(s[ack:], "w.shown && a.pageReady") {
		t.Fatal("ACK must require the visible ready progress page")
	}

	if strings.Contains(s, "fmt.Fprintln(os.Stdout, \"DEEP_LEGENDS_UPDATE_STARTED\")") {
		t.Fatal("ACK returned to page-ready callback")
	}
}
