package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"time"
)

var updateInstallTimingStages = []string{"installer_start", "parent_exited", "extract_start", "extract_done", "copy_done", "relaunch"}

// Consume once, including malformed data, so a stale installation cannot be
// attributed to later starts. Log durations only, never local paths.
func consumeUpdateInstallTiming(root string, record func(map[string]any)) {
	path := filepath.Join(root, "update-install-timing.json")
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return
	}
	event := map[string]any{"event": "update_install_timing", "result": "invalid"}
	defer func() { _ = os.Remove(path); record(event) }()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 4096 {
		return
	}
	file, err := os.Open(path)
	if err != nil {
		return
	}
	data, err := io.ReadAll(io.LimitReader(file, 4097))
	file.Close()
	var stages map[string]int64
	if err != nil || len(data) > 4096 || json.Unmarshal(data, &stages) != nil {
		return
	}
	previous := int64(0)
	durations := map[string]int64{}
	for i, stage := range updateInstallTimingStages {
		value := stages[stage]
		if value <= 0 || value < previous || value > time.Now().UnixMilli()+60000 {
			return
		}
		if i > 0 {
			durations[updateInstallTimingStages[i-1]+"_to_"+stage] = value - previous
		}
		previous = value
	}
	total := previous - stages["installer_start"]
	if total > int64(time.Hour/time.Millisecond) {
		return
	}
	event["result"] = "ok"
	event["stages_ms"] = durations
	event["total_ms"] = total
}
