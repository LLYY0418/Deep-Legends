package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"time"
)

var updateInstallTimingStages = []string{"installer_start", "parent_exited", "payload_released", "nsis_start", "oninit", "check_done", "uninstall_old_start", "uninstall_old_done", "extract_start", "extract_done", "copy_done", "relaunch"}

func detailedInstallTimingStage(stage string) bool {
	return stage == "payload_released" || stage == "nsis_start" || stage == "oninit" || stage == "check_done"
}

// Consume once. Partial legacy files retain every available interval; missing
// endpoints are null, never invented or subtracted across an unknown stage.
func consumeUpdateInstallTiming(root string, record func(map[string]any)) {
	path := filepath.Join(root, "update-install-timing.json")
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return
	}
	event := map[string]any{"event": "update_install_timing", "result": "invalid", "reason": "parse_error"}
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
	if err != nil || len(data) > 4096 || json.Unmarshal(data, &stages) != nil || stages == nil {
		return
	}
	durations, missing := map[string]any{}, false
	// Complete legacy eight-stage reports remain valid; new endpoints are
	// unknown there. Once any detail is present, require all four details.
	detailed := false
	for _, stage := range updateInstallTimingStages {
		if detailedInstallTimingStage(stage) && stages[stage] > 0 {
			detailed = true
		}
	}
	previous := int64(0)
	skew := false
	for i, stage := range updateInstallTimingStages {
		value := stages[stage]
		if value <= 0 {
			if detailed || !detailedInstallTimingStage(stage) {
				missing = true
			}
		} else {
			if value < previous || value > time.Now().UnixMilli()+60000 {
				skew = true
			}
			previous = value
		}
		if i > 0 {
			prior := stages[updateInstallTimingStages[i-1]]
			name := updateInstallTimingStages[i-1] + "_to_" + stage
			durations[name] = nil
			if prior > 0 && value >= prior && value <= time.Now().UnixMilli()+60000 {
				durations[name] = value - prior
			}
		}
	}
	// Keep the aggregate gap for comparisons with pre-R212 diagnostics.
	durations["parent_exited_to_uninstall_old_start"] = nil
	if start, end := stages["parent_exited"], stages["uninstall_old_start"]; start > 0 && end >= start && end <= time.Now().UnixMilli()+60000 {
		durations["parent_exited_to_uninstall_old_start"] = end - start
	}
	event["stages_ms"] = durations
	for name, pair := range map[string][2]string{"uninstall_old_ms": {"uninstall_old_start", "uninstall_old_done"}, "extract_ms": {"extract_start", "extract_done"}, "copy_ms": {"extract_done", "copy_done"}} {
		start, end := stages[pair[0]], stages[pair[1]]
		event[name] = nil
		if start > 0 && end >= start && end <= time.Now().UnixMilli()+60000 {
			event[name] = end - start
		}
	}
	event["total_ms"] = nil
	if start, end := stages["installer_start"], stages["relaunch"]; start > 0 && end >= start && end-start <= int64(time.Hour/time.Millisecond) && end <= time.Now().UnixMilli()+60000 {
		event["total_ms"] = end - start
	}
	if skew {
		event["reason"] = "clock_skew"
		return
	}
	if missing {
		event["result"], event["reason"] = "partial", "missing_fields"
		return
	}
	if event["total_ms"] == nil {
		event["reason"] = "clock_skew"
		return
	}
	event["result"] = "ok"
	delete(event, "reason")
}
