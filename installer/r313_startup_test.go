package main

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestR313ShellAcknowledgementRequiresVisibleWindowAndPreservesProtocol(t *testing.T) {
	opts := installerOptions{Update: true, ParentFirst: true}
	var order []string
	report := func(line string) error { order = append(order, line); return nil }
	mark := func(s string) { order = append(order, s) }
	if ok, err := acknowledgeUpdateShell(opts, false, report, mark); ok || err == nil || len(order) > 0 {
		t.Fatal(ok, err, order)
	}
	order = append(order, "shell_window_shown")
	ok, err := acknowledgeUpdateShell(opts, true, report, mark)
	if !ok || err != nil || !reflect.DeepEqual(order, []string{"shell_window_shown", "DEEP_LEGENDS_UPDATE_STARTED", "ack_sent"}) {
		t.Fatal(ok, err, order)
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
	for _, failure := range []string{"runtime", "webview-embed", "webview-timeout", "payloadError", "destination"} {
		t.Run(failure, func(t *testing.T) {
			root := t.TempDir()
			exe := filepath.Join(root, "original.exe")
			os.WriteFile(exe, []byte("old application"), 0600)
			opts := installerOptions{Update: true, ParentFirst: true, Destination: root, RecoveryExe: exe}
			var order []string
			exited, recovered := recoverAcknowledgedUpdate(opts, "Deep Legends.exe", true, false, func() bool { order = append(order, "parent_exited"); return true }, func(got, dir string) (uint32, error) {
				if got != exe || dir != root {
					t.Fatal(got, dir)
				}
				order = append(order, "recovered")
				return 1, nil
			})
			if !exited || !recovered || !reflect.DeepEqual(order, []string{"parent_exited", "recovered"}) {
				t.Fatal(exited, recovered, order)
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
