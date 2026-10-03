package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestR201UpdateSkipsCRCManualKeepsCRC(t *testing.T) {
	for _, command := range []string{upgradeSetupCommandLine("setup.exe", "dest"), portableSetupCommandLine("setup.exe", "dest")} {
		if !strings.Contains(command, " /NCRC ") {
			t.Fatal(command)
		}
	}
	if strings.Contains(setupCommandLine("setup.exe", "dest", true), "/NCRC") {
		t.Fatal("manual install CRC disabled")
	}
	options := parseInstallerOptions([]string{"--update", "--parent-first", "--parent-pid", "42", "--dest", "dest"})
	if options.Error != nil || options.ParentPID != 42 || !options.ParentFirst {
		t.Fatal(options)
	}
	if parseInstallerOptions([]string{"--update", "--parent-first", "--dest", "dest"}).Error == nil {
		t.Fatal("new handshake missing PID accepted")
	}
	// .63 remains parseable without the new protocol flag.
	legacy := parseInstallerOptions([]string{"--update", "--fresh-install", "--parent-pid", "42", "--dest", "dest"})
	if legacy.Error != nil || legacy.ParentFirst {
		t.Fatal(legacy)
	}
}
func TestR201NSISTimingRecordsRealStages(t *testing.T) {
	root := t.TempDir()
	t.Setenv("LOL_LOOT_DATA_DIR", root)
	timing := newUpdateInstallTiming()
	timing.mark("installer_start")
	timing.mark("parent_exited")
	now := time.Now().UnixMilli()
	raw := strings.Join([]string{"extract_start=" + formatTimingMS(now), "extract_done=" + formatTimingMS(now), "copy_done=" + formatTimingMS(now)}, "\n")
	os.WriteFile(filepath.Join(root, "update-install-stages.txt"), []byte(raw), 0600)
	timing.importNSIS(root)
	timing.mark("relaunch")
	data, err := os.ReadFile(filepath.Join(root, "update-install-timing.json"))
	if err != nil {
		t.Fatal(err)
	}
	var stages map[string]int64
	if json.Unmarshal(data, &stages) != nil || len(stages) != 6 {
		t.Fatal(string(data))
	}
	if strings.Contains(string(data), root) {
		t.Fatal("path recorded")
	}
	timing.mark("parent_exited")
	if timing.stages["extract_start"] != 0 {
		t.Fatal("retry retained stale boundaries")
	}
}
func formatTimingMS(value int64) string { data, _ := json.Marshal(value); return string(data) }

func TestR201FailureReopensOriginalPortableOnlyAfterParentExit(t *testing.T) {
	root := t.TempDir()
	original := filepath.Join(root, "portable.exe")
	os.WriteFile(original, []byte("fixture"), 0600)
	options := installerOptions{Update: true, FreshInstall: true, Destination: filepath.Join(root, "new"), RecoveryExe: original}
	calls := 0
	start := func(exe, directory string) (uint32, error) {
		calls++
		if exe != original || directory != root {
			t.Fatal(exe, directory)
		}
		return 42, nil
	}
	if recoverUpdateApplication(options, "Deep Legends.exe", false, start) || calls != 0 {
		t.Fatal("running parent duplicated")
	}
	if !recoverUpdateApplication(options, "Deep Legends.exe", true, start) || calls != 1 {
		t.Fatal("original launcher not recovered")
	}
	options.RecoveryExe = filepath.Join(root, "missing.exe")
	if recoverUpdateApplication(options, "Deep Legends.exe", true, start) || calls != 1 {
		t.Fatal("missing launcher started")
	}
}
