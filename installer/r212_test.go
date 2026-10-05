package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestR212PayloadReleaseOverlapsTwoSecondParentWait(t *testing.T) {
	finished := make(chan time.Time, 1)
	marks := map[string]time.Time{}
	parentFinished := make(chan time.Time, 1)
	released, exited := releasePayloadWhileWaiting(func() bool { time.Sleep(2 * time.Second); parentFinished <- time.Now(); return true }, func(progressMessage) {}, func() (string, string, error) {
		finished <- time.Now()
		return "fixture-setup", "", nil
	}, func(stage string) { marks[stage] = time.Now() })
	if !exited || released.err != nil || released.setup != "fixture-setup" {
		t.Fatal(released, exited)
	}
	if !((<-finished).Before(<-parentFinished)) {
		t.Fatal("payload was not released before parent_exited")
	}
	if marks["payload_released"].Before(marks["parent_exited"]) {
		t.Fatal("join milestone moved backwards", marks)
	}
}

func TestR212PayloadFailureAndFailedWaitCleanup(t *testing.T) {
	expected := errors.New("payload disk failure")
	var marks []string
	released, exited := releasePayloadWhileWaiting(func() bool { return true }, func(progressMessage) {}, func() (string, string, error) { return "", "", expected }, func(s string) { marks = append(marks, s) })
	if !exited || !errors.Is(released.err, expected) || len(marks) != 1 || marks[0] != "parent_exited" {
		t.Fatal(released, exited, marks)
	}
	directory := filepath.Join(t.TempDir(), "payload")
	os.Mkdir(directory, 0700)
	os.WriteFile(filepath.Join(directory, "setup.exe"), []byte("fixture"), 0600)
	released, exited = releasePayloadWhileWaiting(func() bool { return false }, func(progressMessage) {}, func() (string, string, error) { return filepath.Join(directory, "setup.exe"), directory, nil }, func(string) { t.Fatal("failed wait recorded success") })
	if exited || released.directory != "" {
		t.Fatal(released, exited)
	}
	if _, err := os.Stat(directory); !os.IsNotExist(err) {
		t.Fatal("failed wait leaked released payload", err)
	}
}

func TestR212NSISDetailImportAndRetry(t *testing.T) {
	root := t.TempDir()
	t.Setenv("LOL_LOOT_DATA_DIR", root)
	timing := newUpdateInstallTiming()
	timing.mark("installer_start")
	timing.mark("parent_exited")
	timing.mark("payload_released")
	timing.mark("nsis_start")
	stamp := formatTimingMS(time.Now().UnixMilli())
	if err := os.WriteFile(filepath.Join(root, "update-install-nsis-stages.txt"), []byte("oninit="+stamp+"\ncheck_done="+stamp+"\nuninstall_old_start="+stamp+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	timing.importNSIS(root)
	for _, stage := range []string{"oninit", "check_done", "uninstall_old_start"} {
		if timing.snapshot()[stage] == 0 {
			t.Fatal("missing real NSIS milestone", stage)
		}
	}
	timing.mark("parent_exited")
	for _, stage := range timingStageOrder[2:] {
		if timing.snapshot()[stage] != 0 {
			t.Fatal("retry retained stage", stage)
		}
	}
}
