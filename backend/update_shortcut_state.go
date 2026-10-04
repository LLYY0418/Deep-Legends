package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type updateShortcutSnapshot struct {
	Exists        bool    `json:"exists"`
	CreatedTime   *string `json:"created_time"`
	ModifiedTime  *string `json:"modified_time"`
	TargetMatches *bool   `json:"target_matches"`
	Status        string  `json:"status"`
}

// Independent installer module's v1 bridge. Do not log arbitrary source JSON:
// even local files must not introduce paths, account names or error messages.
type updateShortcutReport struct {
	Schema                      int                               `json:"schema"`
	Phase                       string                            `json:"phase"`
	DesktopBefore               map[string]updateShortcutSnapshot `json:"desktop_before"`
	DesktopAfter                map[string]updateShortcutSnapshot `json:"desktop_after"`
	DesktopFinal                map[string]updateShortcutSnapshot `json:"desktop_final"`
	StartMenuBefore             map[string]updateShortcutSnapshot `json:"start_menu_before"`
	StartMenuAfter              map[string]updateShortcutSnapshot `json:"start_menu_after"`
	CreatedTimeChanged          bool                              `json:"created_time_changed"`
	DesktopCreatedTimeChanged   bool                              `json:"desktop_created_time_changed"`
	StartMenuCreatedTimeChanged bool                              `json:"start_menu_created_time_changed"`
	KeepShortcutsBefore         *bool                             `json:"keep_shortcuts_reg_before"`
	KeepShortcutsPreNSIS        *bool                             `json:"keep_shortcuts_reg_pre_nsis"`
	KeepShortcutsReg            *bool                             `json:"keep_shortcuts_reg"`
	KeepShortcutsRepair         string                            `json:"keep_shortcuts_repair"`
	BackupResults               map[string]string                 `json:"backup_results"`
	RestoreResults              map[string]string                 `json:"restore_results"`
	NSISResult                  string                            `json:"nsis_result"`
}

func shortcutStateChoice(value string, choices ...string) bool {
	for _, choice := range choices {
		if value == choice {
			return true
		}
	}
	return false
}

func validShortcutSnapshotMap(rows map[string]updateShortcutSnapshot, emptyAllowed bool) bool {
	if len(rows) == 0 && emptyAllowed {
		return true
	}
	if len(rows) != 2 {
		return false
	}
	for _, scope := range []string{"current", "public"} {
		row, ok := rows[scope]
		if !ok || !shortcutStateChoice(row.Status, "ok", "missing", "read_error", "target_error", "folder_unavailable") {
			return false
		}
		for _, value := range []*string{row.CreatedTime, row.ModifiedTime} {
			if value != nil {
				if _, err := time.Parse(time.RFC3339Nano, *value); err != nil {
					return false
				}
			}
		}
		if !row.Exists && (row.CreatedTime != nil || row.ModifiedTime != nil || row.TargetMatches != nil) {
			return false
		}
	}
	return true
}

func validShortcutResults(rows map[string]string, emptyAllowed bool, choices ...string) bool {
	if len(rows) == 0 && emptyAllowed {
		return true
	}
	if len(rows) != 2 {
		return false
	}
	for _, scope := range []string{"current", "public"} {
		value, ok := rows[scope]
		if !ok || !shortcutStateChoice(value, choices...) {
			return false
		}
	}
	return true
}

func shortcutSnapshotCreationChanged(before, after updateShortcutSnapshot) bool {
	return before.Exists && after.Exists && before.CreatedTime != nil && after.CreatedTime != nil && *before.CreatedTime != *after.CreatedTime
}

func validUpdateShortcutReport(report updateShortcutReport) bool {
	pending := report.Phase == "before"
	if report.Schema != 1 || !shortcutStateChoice(report.Phase, "before", "after") ||
		!shortcutStateChoice(report.KeepShortcutsRepair, "not_needed", "ok", "failed", "skipped_portable") ||
		!shortcutStateChoice(report.NSISResult, "pending", "ok", "failed", "start_failed") ||
		pending != (report.NSISResult == "pending") {
		return false
	}
	if !validShortcutSnapshotMap(report.DesktopBefore, false) || !validShortcutSnapshotMap(report.StartMenuBefore, false) ||
		!validShortcutSnapshotMap(report.DesktopAfter, pending) || !validShortcutSnapshotMap(report.DesktopFinal, pending) || !validShortcutSnapshotMap(report.StartMenuAfter, pending) ||
		!validShortcutResults(report.BackupResults, false, "ok", "absent", "failed") ||
		!validShortcutResults(report.RestoreResults, pending, "absent_before", "no_backup", "unchanged", "state_unknown", "target_missing", "ok", "failed") {
		return false
	}
	desktop, menu := false, false
	for _, scope := range []string{"current", "public"} {
		desktop = desktop || shortcutSnapshotCreationChanged(report.DesktopBefore[scope], report.DesktopAfter[scope])
		menu = menu || shortcutSnapshotCreationChanged(report.StartMenuBefore[scope], report.StartMenuAfter[scope])
	}
	return report.DesktopCreatedTimeChanged == desktop && report.StartMenuCreatedTimeChanged == menu && report.CreatedTimeChanged == (desktop || menu)
}

func consumeUpdateShortcutState(root string, record func(map[string]any)) {
	path := filepath.Join(root, "update-shortcut-state.json")
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return
	}
	event := map[string]any{"event": "update_shortcut_state", "result": "invalid", "reason": "parse_error"}
	defer func() { _ = os.Remove(path); record(event) }()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 16384 {
		return
	}
	file, err := os.Open(path)
	if err != nil {
		return
	}
	data, err := io.ReadAll(io.LimitReader(file, 16385))
	file.Close()
	var report updateShortcutReport
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err != nil || len(data) > 16384 || decoder.Decode(&report) != nil || decoder.Decode(new(any)) != io.EOF || !validUpdateShortcutReport(report) {
		return
	}
	// Marshal only this validated schema; unknown names/values never reach logs.
	clean, _ := json.Marshal(report)
	var fields map[string]any
	_ = json.Unmarshal(clean, &fields)
	for key, value := range fields {
		event[key] = value
	}
	event["result"] = "ok"
	delete(event, "reason")
	if report.Phase == "before" {
		event["result"], event["reason"] = "partial", "nsis_not_finished"
	}
}
