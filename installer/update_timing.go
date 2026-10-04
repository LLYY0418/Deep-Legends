package main

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type updateInstallTiming struct {
	mu     sync.Mutex
	path   string
	stages map[string]int64
}

var timingStageOrder = []string{"installer_start", "parent_exited", "uninstall_old_start", "uninstall_old_done", "extract_start", "extract_done", "copy_done", "relaunch"}

func newUpdateInstallTiming() *updateInstallTiming {
	root := strings.TrimSpace(os.Getenv("LOL_LOOT_DATA_DIR"))
	if root == "" {
		base, err := os.UserCacheDir()
		if err != nil {
			return nil
		}
		root = filepath.Join(base, "LOLLootAssistant")
	}
	return &updateInstallTiming{path: filepath.Join(root, "update-install-timing.json"), stages: map[string]int64{}}
}
func (t *updateInstallTiming) mark(stage string) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if stage == "installer_start" {
		_ = os.Remove(filepath.Join(filepath.Dir(t.path), "update-install-nsis-stages.txt"))
	}
	if stage == "parent_exited" {
		for _, name := range timingStageOrder[2:] {
			delete(t.stages, name)
		}
	}
	t.stages[stage] = time.Now().UnixMilli()
	t.writeLocked()
}
func (t *updateInstallTiming) writeLocked() {
	if os.MkdirAll(filepath.Dir(t.path), 0700) != nil {
		return
	}
	stages := make(map[string]any, len(timingStageOrder))
	for _, stage := range timingStageOrder {
		if value := t.stages[stage]; value > 0 {
			stages[stage] = value
		} else {
			stages[stage] = nil
		}
	}
	data, err := json.Marshal(stages)
	if err != nil {
		return
	}
	file, err := os.CreateTemp(filepath.Dir(t.path), ".update-timing-*")
	if err != nil {
		return
	}
	name := file.Name()
	defer os.Remove(name)
	_, err = file.Write(data)
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err == nil && closeErr == nil {
		_ = os.Rename(name, t.path)
		var lines strings.Builder
		for _, stage := range timingStageOrder {
			if value := t.stages[stage]; value > 0 {
				lines.WriteString(stage + "=" + strconv.FormatInt(value, 10) + "\n")
			}
		}
		_ = os.WriteFile(filepath.Join(filepath.Dir(t.path), "update-install-stages.txt"), []byte(lines.String()), 0600)
	}
}

// NSIS writes real stage boundaries, not estimated progress counters. Its
// child TEMP is private to this installation; no application paths are logged.
func (t *updateInstallTiming) importNSIS(directory string) {
	if t == nil {
		return
	}
	file, err := os.Open(filepath.Join(filepath.Dir(t.path), "update-install-nsis-stages.txt"))
	if os.IsNotExist(err) {
		file, err = os.Open(filepath.Join(directory, "update-install-stages.txt"))
	}
	if err != nil {
		return
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 128), 1024)
	t.mu.Lock()
	defer t.mu.Unlock()
	changed := false
	for scanner.Scan() {
		stage, raw, ok := strings.Cut(scanner.Text(), "=")
		if !ok || (stage != "uninstall_old_start" && stage != "uninstall_old_done" && stage != "extract_start" && stage != "extract_done" && stage != "copy_done") {
			continue
		}
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || value < t.stages["parent_exited"] || value > time.Now().UnixMilli()+1000 {
			continue
		}
		if t.stages[stage] != 0 {
			continue
		}
		t.stages[stage] = value
		changed = true
	}
	if changed {
		t.writeLocked()
	}
}

func (t *updateInstallTiming) snapshot() map[string]int64 {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make(map[string]int64, len(t.stages))
	for key, value := range t.stages {
		out[key] = value
	}
	return out
}
