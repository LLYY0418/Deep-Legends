package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func r205State(stamp string) shortcutState {
	match := true
	return shortcutState{Exists: true, CreatedTime: &stamp, ModifiedTime: &stamp, TargetMatches: &match, Status: "ok"}
}

func TestR205ManualInstallHasNoShortcutGuard(t *testing.T) {
	var guard *shortcutUpdateGuard
	for _, result := range []string{"start_failed", "failed", "ok"} {
		guard.finish(result)
	}
}

func TestR205ShortcutCreationChanged(t *testing.T) {
	old, newer, missing := r205State("2026-10-01T01:00:00Z"), r205State("2026-10-04T01:00:00Z"), shortcutState{Status: "missing"}
	for _, item := range []struct {
		before, after shortcutState
		changed       bool
	}{{old, old, false}, {old, newer, true}, {old, missing, false}, {missing, newer, false}, {missing, missing, false}, {old, shortcutState{Exists: true, Status: "target_error"}, false}} {
		if shortcutCreationChanged(item.before, item.after) != item.changed {
			t.Fatal(item)
		}
	}
}

func TestR205ShortcutRestoreCases(t *testing.T) {
	for _, mode := range []string{"deleted", "recreated", "unchanged", "target_missing", "backup_failed", "restore_failed", "state_unknown"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			path, backup, target := filepath.Join(root, "Deep Legends.lnk"), filepath.Join(root, "backup.lnk"), filepath.Join(root, "Deep Legends.exe")
			original := []byte("fixture original AUMI, arguments and icon")
			if err := os.WriteFile(path, original, 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(target, []byte("fixture executable"), 0600); err != nil {
				t.Fatal(err)
			}
			stamp := "2026-10-01T01:00:00Z"
			kept := true
			var reports []shortcutUpdateReport
			restores := 0
			hooks := shortcutUpdateHooks{
				Snapshot: func() (map[string]shortcutState, map[string]shortcutState) {
					current := shortcutState{Status: "missing"}
					if _, err := os.Stat(path); err == nil {
						current = r205State(stamp)
					}
					if mode == "state_unknown" && len(reports) > 0 {
						current = shortcutState{Status: "read_error"}
					}
					return map[string]shortcutState{"current": current, "public": {Status: "missing"}}, map[string]shortcutState{"current": {Status: "missing"}, "public": {Status: "missing"}}
				},
				ReadKeep: func() *bool { value := kept; return &value }, RepairKeep: func() error { t.Fatal("unexpected registry write"); return nil },
				Backup: func(string) error {
					if mode == "backup_failed" {
						return errors.New("fixture")
					}
					data, err := os.ReadFile(path)
					if err != nil {
						return err
					}
					return os.WriteFile(backup, data, 0600)
				},
				Restore: func(string) error {
					restores++
					if mode == "restore_failed" {
						return errors.New("fixture")
					}
					data, err := os.ReadFile(backup)
					if err != nil {
						return err
					}
					stamp = "2026-10-01T01:00:00Z"
					return os.WriteFile(path, data, 0600)
				},
				TargetExists: func() bool { _, err := os.Stat(target); return err == nil },
				Write: func(report shortcutUpdateReport) {
					reports = append(reports, report)
					if err := writeShortcutUpdateReport(root, report); err != nil {
						t.Fatal(err)
					}
				},
			}
			guard := beginShortcutUpdate(hooks, true)
			switch mode {
			case "deleted", "backup_failed", "restore_failed":
				os.Remove(path)
			case "recreated", "target_missing":
				stamp = "2026-10-04T01:00:00Z"
				os.WriteFile(path, []byte("NSIS replacement"), 0600)
			}
			if mode == "target_missing" {
				os.Remove(target)
			}
			guard.finish("ok")
			want := map[string]string{"deleted": "ok", "recreated": "ok", "unchanged": "unchanged", "target_missing": "target_missing", "backup_failed": "no_backup", "restore_failed": "failed", "state_unknown": "state_unknown"}[mode]
			if guard.report.RestoreResults["current"] != want {
				t.Fatal(guard.report)
			}
			if mode == "deleted" || mode == "recreated" || mode == "unchanged" {
				data, err := os.ReadFile(path)
				if err != nil || !bytes.Equal(data, original) {
					t.Fatalf("original shortcut not retained: %q %v", data, err)
				}
			}
			if (mode == "deleted" || mode == "recreated" || mode == "restore_failed") != (restores == 1) {
				t.Fatalf("restore calls=%d", restores)
			}
			if guard.report.CreatedTimeChanged != (mode == "recreated" || mode == "target_missing") {
				t.Fatal("recreation evidence overwritten", guard.report)
			}
			if len(reports) != 2 || reports[0].Phase != "before" || reports[1].Phase != "after" {
				t.Fatal(reports)
			}
			data, err := os.ReadFile(filepath.Join(root, shortcutStateFilename))
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Contains(data, []byte(root)) || bytes.Contains(data, original) {
				t.Fatal("shortcut contents/path leaked")
			}
			var saved shortcutUpdateReport
			if json.Unmarshal(data, &saved) != nil || saved.Phase != "after" {
				t.Fatal(string(data))
			}
		})
	}
}

func TestR205KeepShortcutsRepair(t *testing.T) {
	for _, tc := range []struct {
		name                                             string
		existing, hasDesktop, portable, unknown, failure bool
		writes                                           int
	}{
		{"missing_with_desktop", false, true, false, false, false, 1},
		{"already_true", true, true, false, false, false, 0},
		{"no_desktop", false, false, false, false, false, 0},
		{"portable", false, true, true, false, false, 0},
		{"unknown", false, true, false, true, false, 1},
		{"permission", false, true, false, false, true, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			value, writes := tc.existing, 0
			g := beginShortcutUpdate(shortcutUpdateHooks{
				Snapshot: func() (map[string]shortcutState, map[string]shortcutState) {
					row := shortcutState{Status: "missing"}
					if tc.hasDesktop {
						row = r205State("2026-10-01T01:00:00Z")
					}
					return map[string]shortcutState{"current": row, "public": {Status: "missing"}}, nil
				},
				ReadKeep: func() *bool {
					if tc.unknown && writes == 0 {
						return nil
					}
					v := value
					return &v
				},
				RepairKeep: func() error {
					writes++
					if tc.failure {
						return errors.New("fixture permission")
					}
					value = true
					return nil
				},
				Backup: func(string) error { return nil }, Write: func(shortcutUpdateReport) {},
			}, !tc.portable)
			if writes != tc.writes {
				t.Fatalf("writes=%d want=%d", writes, tc.writes)
			}
			if tc.writes == 1 && !tc.failure && (g.report.KeepShortcutsPreNSIS == nil || !*g.report.KeepShortcutsPreNSIS) {
				t.Fatal("KeepShortcuts not repaired")
			}
			if tc.failure && g.report.KeepShortcutsRepair != "failed" {
				t.Fatal(g.report)
			}
		})
	}
}

func TestR205MenuRecreationDiagnostic(t *testing.T) {
	reads := 0
	missing := map[string]shortcutState{"current": {Status: "missing"}, "public": {Status: "missing"}}
	g := beginShortcutUpdate(shortcutUpdateHooks{
		Snapshot: func() (map[string]shortcutState, map[string]shortcutState) {
			reads++
			stamp := "2026-10-01T01:00:00Z"
			if reads > 1 {
				stamp = "2026-10-04T01:00:00Z"
			}
			return missing, map[string]shortcutState{"current": r205State(stamp), "public": {Status: "missing"}}
		},
		ReadKeep: func() *bool { return nil }, Write: func(shortcutUpdateReport) {},
	}, true)
	g.finish("failed")
	if !g.report.CreatedTimeChanged || !g.report.StartMenuCreatedTimeChanged || g.report.DesktopCreatedTimeChanged {
		t.Fatal(g.report)
	}
}
