package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestR314InstallTimingExportsOnlyRelativeNewStages(t *testing.T) {
	root := t.TempDir()
	base := time.Now().Add(-time.Minute).UnixMilli()
	stages := map[string]int64{"installer_start": base, "parent_exited": base + 100, "uninstall_old_start": base + 200, "uninstall_old_done": base + 300, "extract_start": base + 400, "extract_done": base + 500, "copy_done": base + 600, "relaunch": base + 700, "process_created_to_main_ms": 4, "page_visible": base + 37, "ack_sent": base + 44, "shell_window_shown": base + 10}
	raw, _ := json.Marshal(stages)
	if err := os.WriteFile(filepath.Join(root, "update-install-timing.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	events := make(chan map[string]any, 1)
	consumeUpdateInstallTiming(root, func(event map[string]any) { events <- event })
	event := <-events
	ms := event["stages_ms"].(map[string]any)
	if ms["process_created_to_main_ms"] != int64(4) || ms["installer_start_to_page_visible"] != int64(37) || ms["installer_start_to_ack_sent"] != int64(44) {
		t.Fatal(event)
	}
	if _, old := ms["installer_start_to_shell_window_shown"]; old {
		t.Fatal("retired stage exported", event)
	}
	for _, value := range ms {
		if n, ok := value.(int64); ok && n >= base {
			t.Fatal("absolute time exported", event)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "update-install-timing.json")); !os.IsNotExist(err) {
		t.Fatal("timing must be consumed once", err)
	}
}
