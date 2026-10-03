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
	if stage == "parent_exited" {
		for _, name := range []string{"extract_start", "extract_done", "copy_done", "relaunch"} {
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
	data, err := json.Marshal(t.stages)
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
	}
}

// NSIS writes real stage boundaries, not estimated progress counters. Its
// child TEMP is private to this installation; no application paths are logged.
func (t *updateInstallTiming) importNSIS(directory string) {
	if t == nil {
		return
	}
	file, err := os.Open(filepath.Join(directory, "update-install-stages.txt"))
	if err != nil {
		return
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 128), 1024)
	t.mu.Lock()
	defer t.mu.Unlock()
	for scanner.Scan() {
		stage, raw, ok := strings.Cut(scanner.Text(), "=")
		if !ok || (stage != "extract_start" && stage != "extract_done" && stage != "copy_done") {
			continue
		}
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || value < t.stages["parent_exited"] || value > time.Now().UnixMilli()+1000 {
			continue
		}
		if stage == "extract_start" && t.stages[stage] != 0 {
			continue
		}
		t.stages[stage] = value
	}
	t.writeLocked()
}
