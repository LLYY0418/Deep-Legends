package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Only elapsed milliseconds leave this process; paths and wall clocks are not
// diagnostic fields. The desktop receives the origin on the private pipe solely
// to measure the actual hide boundary, then persists only its elapsed value.
type updateApplyTiming struct {
	mu     sync.Mutex
	origin time.Time
	path   string
	stages map[string]int64
}

func (u *updateManager) beginApplyTiming(click int64) {
	now := time.Now()
	origin := now
	if click > 0 && click <= now.UnixMilli() && now.UnixMilli()-click <= 60000 {
		origin = time.UnixMilli(click)
	}
	root := filepath.Dir(u.directory)
	if u.store != nil {
		root = u.store.root
	}
	u.applyTimingMu.Lock()
	defer u.applyTimingMu.Unlock()
	u.applyTiming = &updateApplyTiming{origin: origin, path: filepath.Join(root, "update-apply-timing.json"), stages: map[string]int64{"apply_click": 0}}
	u.applyTiming.mark("apply_received")
}
func (u *updateManager) ensureApplyTiming() {
	u.applyTimingMu.Lock()
	exists := u.applyTiming != nil
	u.applyTimingMu.Unlock()
	if !exists {
		u.beginApplyTiming(0)
	}
}
func (u *updateManager) markApplyTiming(stage string) {
	u.applyTimingMu.Lock()
	t := u.applyTiming
	u.applyTimingMu.Unlock()
	if t != nil {
		t.mark(stage)
	}
}
func (t *updateApplyTiming) mark(stage string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.stages[stage] = time.Since(t.origin).Milliseconds()
	data, err := json.Marshal(t.stages)
	if err != nil {
		return
	}
	_ = writeAtomicTiming(t.path, data)
}
func writeAtomicTiming(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".update-apply-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, err = f.Write(data)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), path)
}
func consumeUpdateApplyTiming(root string, record func(map[string]any)) {
	path := filepath.Join(root, "update-apply-timing.json")
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return
	}
	defer os.Remove(path)
	event := map[string]any{"event": "update_apply_timing", "result": "invalid"}
	defer func() { record(event) }()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 4096 {
		return
	}
	data, err := os.ReadFile(path)
	var input map[string]int64
	if err != nil || json.Unmarshal(data, &input) != nil {
		return
	}
	stages := map[string]int64{}
	for _, key := range []string{"apply_click", "apply_received", "verify_done", "verify_skipped", "migrate_done", "spawn_begin", "spawn_returned", "ack_received", "windows_hidden"} {
		if value, ok := input[key]; ok && value >= 0 && value <= 3600000 {
			stages[key] = value
		}
	}
	event["stages_ms"] = stages
	event["result"] = "partial"
	if _, ok := stages["windows_hidden"]; ok {
		event["result"] = "ok"
	}
}
