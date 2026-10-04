package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func r205ShortcutReport() updateShortcutReport {
	stamp, matches, kept := "2026-10-01T01:00:00Z", true, true
	present := updateShortcutSnapshot{Exists: true, CreatedTime: &stamp, ModifiedTime: &stamp, TargetMatches: &matches, Status: "ok"}
	rows := func() map[string]updateShortcutSnapshot {
		return map[string]updateShortcutSnapshot{"current": present, "public": {Status: "missing"}}
	}
	return updateShortcutReport{Schema: 1, Phase: "after", DesktopBefore: rows(), DesktopAfter: rows(), DesktopFinal: rows(), StartMenuBefore: rows(), StartMenuAfter: rows(), KeepShortcutsBefore: &kept, KeepShortcutsPreNSIS: &kept, KeepShortcutsReg: &kept, KeepShortcutsRepair: "not_needed", BackupResults: map[string]string{"current": "ok", "public": "absent"}, RestoreResults: map[string]string{"current": "unchanged", "public": "absent_before"}, NSISResult: "ok"}
}

func TestR205ShortcutStateConsumedOnceAndPrivate(t *testing.T) {
	for _, mode := range []string{"unchanged", "recreated", "deleted_restored", "pending", "malformed", "unknown_field", "private_status", "private_scope", "inaccurate_change", "oversize"} {
		t.Run(mode, func(t *testing.T) {
			report := r205ShortcutReport()
			switch mode {
			case "recreated":
				stamp := "2026-10-04T01:00:00Z"
				row := report.DesktopAfter["current"]
				row.CreatedTime = &stamp
				report.DesktopAfter["current"] = row
				report.CreatedTimeChanged, report.DesktopCreatedTimeChanged = true, true
				report.RestoreResults["current"] = "ok"
			case "deleted_restored":
				report.DesktopAfter["current"] = updateShortcutSnapshot{Status: "missing"}
				report.RestoreResults["current"] = "ok"
			case "pending":
				report.Phase, report.NSISResult = "before", "pending"
				report.DesktopAfter, report.DesktopFinal, report.StartMenuAfter = nil, nil, nil
				report.RestoreResults = nil
			case "private_status":
				row := report.DesktopBefore["current"]
				row.Status = `C:\Users\fixture-account\Desktop`
				report.DesktopBefore["current"] = row
			case "private_scope":
				report.DesktopBefore["fixture-account"] = report.DesktopBefore["current"]
			case "inaccurate_change":
				report.CreatedTimeChanged = true
			}
			raw, _ := json.Marshal(report)
			if mode == "malformed" {
				raw = []byte("{")
			}
			if mode == "unknown_field" {
				raw = bytes.Replace(raw, []byte(`"schema":1`), []byte(`"schema":1,"path":"C:\\Users\\fixture-account"`), 1)
			}
			if mode == "oversize" {
				raw = []byte(strings.Repeat("x", 16385))
			}
			root := t.TempDir()
			path := filepath.Join(root, "update-shortcut-state.json")
			if err := os.WriteFile(path, raw, 0600); err != nil {
				t.Fatal(err)
			}
			var rows []map[string]any
			consumeUpdateShortcutState(root, func(row map[string]any) { rows = append(rows, row) })
			consumeUpdateShortcutState(root, func(row map[string]any) { rows = append(rows, row) })
			if len(rows) != 1 {
				t.Fatal(rows)
			}
			want := "invalid"
			if mode == "unchanged" || mode == "recreated" || mode == "deleted_restored" {
				want = "ok"
			}
			if mode == "pending" {
				want = "partial"
			}
			if rows[0]["event"] != "update_shortcut_state" || rows[0]["result"] != want {
				t.Fatal(rows)
			}
			if want == "ok" && rows[0]["created_time_changed"] != (mode == "recreated") {
				t.Fatal(rows)
			}
			encoded, _ := json.Marshal(rows)
			if bytes.Contains(encoded, []byte(root)) || bytes.Contains(encoded, []byte("fixture-account")) || bytes.Contains(encoded, []byte("Users")) {
				t.Fatal("source data leaked", string(encoded))
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatal("report not consumed")
			}
		})
	}
}

func TestR205ShortcutStateRejectsLinkAndPreservesTiming(t *testing.T) {
	root := t.TempDir()
	external := filepath.Join(t.TempDir(), "fixture.json")
	raw, _ := json.Marshal(r205ShortcutReport())
	os.WriteFile(external, raw, 0600)
	if err := os.Symlink(external, filepath.Join(root, "update-shortcut-state.json")); err != nil {
		t.Skip("symlink unavailable")
	}
	var row map[string]any
	consumeUpdateShortcutState(root, func(event map[string]any) { row = event })
	if row["result"] != "invalid" {
		t.Fatal(row)
	}
	if _, err := os.Stat(external); err != nil {
		t.Fatal("symlink target touched", err)
	}
	os.WriteFile(filepath.Join(root, "update-install-timing.json"), []byte(`{"installer_start":1000}`), 0600)
	os.WriteFile(filepath.Join(root, "update-shortcut-state.json"), raw, 0600)
	consumeUpdateShortcutState(root, func(event map[string]any) { row = event })
	if row["result"] != "ok" {
		t.Fatal(row)
	}
	consumeUpdateInstallTiming(root, func(event map[string]any) { row = event })
	if row["event"] != "update_install_timing" || row["result"] != "partial" {
		t.Fatal("timing schema changed", row)
	}
}
