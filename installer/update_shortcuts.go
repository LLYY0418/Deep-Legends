package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const shortcutStateFilename = "update-shortcut-state.json"

// Pinned by installer-nsh.test.cjs to electron-builder's UUID.v5(appId).
const shortcutInstallRegistryKey = `Software\ae355ba0-3686-5750-a759-b20159e02f53`

type shortcutState struct {
	Exists        bool    `json:"exists"`
	CreatedTime   *string `json:"created_time"`
	ModifiedTime  *string `json:"modified_time"`
	TargetMatches *bool   `json:"target_matches"`
	Status        string  `json:"status"`
}

type shortcutUpdateReport struct {
	IconLocationStable          bool                     `json:"icon_location_stable"`
	Schema                      int                      `json:"schema"`
	Phase                       string                   `json:"phase"`
	DesktopBefore               map[string]shortcutState `json:"desktop_before"`
	DesktopAfter                map[string]shortcutState `json:"desktop_after"`
	DesktopFinal                map[string]shortcutState `json:"desktop_final"`
	StartMenuBefore             map[string]shortcutState `json:"start_menu_before"`
	StartMenuAfter              map[string]shortcutState `json:"start_menu_after"`
	CreatedTimeChanged          bool                     `json:"created_time_changed"`
	DesktopCreatedTimeChanged   bool                     `json:"desktop_created_time_changed"`
	StartMenuCreatedTimeChanged bool                     `json:"start_menu_created_time_changed"`
	KeepShortcutsBefore         *bool                    `json:"keep_shortcuts_reg_before"`
	KeepShortcutsPreNSIS        *bool                    `json:"keep_shortcuts_reg_pre_nsis"`
	KeepShortcutsReg            *bool                    `json:"keep_shortcuts_reg"`
	KeepShortcutsRepair         string                   `json:"keep_shortcuts_repair"`
	BackupResults               map[string]string        `json:"backup_results"`
	RestoreResults              map[string]string        `json:"restore_results"`
	NSISResult                  string                   `json:"nsis_result"`
}

type shortcutUpdateHooks struct {
	StabilizeIcon func() bool
	Snapshot      func() (desktop, menu map[string]shortcutState)
	ReadKeep      func() *bool
	RepairKeep    func() error
	Backup        func(scope string) error
	Restore       func(scope string) error
	TargetExists  func() bool
	Write         func(shortcutUpdateReport)
}

type shortcutUpdateGuard struct {
	hooks  shortcutUpdateHooks
	report shortcutUpdateReport
}

func shortcutCreationChanged(before, after shortcutState) bool {
	return before.Exists && after.Exists && before.CreatedTime != nil && after.CreatedTime != nil && *before.CreatedTime != *after.CreatedTime
}

// The installed target, not the .lnk name alone, must exist before restoration.
// Missing/unknown timestamps are never treated as evidence of recreation.
func beginShortcutUpdate(hooks shortcutUpdateHooks, repairRegistry bool) *shortcutUpdateGuard {
	// Migrate the icon before removing an old exe, including the first upgrade
	// from a version whose shortcuts still refer to that executable's resource.
	if hooks.StabilizeIcon != nil {
		hooks.StabilizeIcon()
	}
	desktop, menu := hooks.Snapshot()
	g := &shortcutUpdateGuard{hooks: hooks, report: shortcutUpdateReport{
		Schema: 1, Phase: "before", DesktopBefore: desktop, StartMenuBefore: menu,
		DesktopAfter: map[string]shortcutState{}, DesktopFinal: map[string]shortcutState{}, StartMenuAfter: map[string]shortcutState{},
		BackupResults: map[string]string{}, RestoreResults: map[string]string{}, NSISResult: "pending", KeepShortcutsRepair: "not_needed",
	}}
	g.report.KeepShortcutsBefore = hooks.ReadKeep()
	hasDesktop := false
	for _, scope := range []string{"current", "public"} {
		if !desktop[scope].Exists {
			g.report.BackupResults[scope] = "absent"
			continue
		}
		hasDesktop = true
		g.report.BackupResults[scope] = "ok"
		if hooks.Backup(scope) != nil {
			g.report.BackupResults[scope] = "failed"
		}
	}
	if repairRegistry && hasDesktop && (g.report.KeepShortcutsBefore == nil || !*g.report.KeepShortcutsBefore) {
		g.report.KeepShortcutsRepair = "ok"
		if hooks.RepairKeep() != nil {
			g.report.KeepShortcutsRepair = "failed"
		}
	} else if !repairRegistry {
		g.report.KeepShortcutsRepair = "skipped_portable"
	}
	g.report.KeepShortcutsPreNSIS = hooks.ReadKeep()
	hooks.Write(g.report)
	return g
}

func (g *shortcutUpdateGuard) finish(result string) {
	if g == nil {
		return
	}
	g.report.DesktopAfter, g.report.StartMenuAfter = g.hooks.Snapshot()
	g.report.Phase, g.report.NSISResult = "after", result
	g.report.KeepShortcutsReg = g.hooks.ReadKeep()
	for _, scope := range []string{"current", "public"} {
		before, after := g.report.DesktopBefore[scope], g.report.DesktopAfter[scope]
		changed := shortcutCreationChanged(before, after)
		g.report.DesktopCreatedTimeChanged = g.report.DesktopCreatedTimeChanged || changed
		g.report.StartMenuCreatedTimeChanged = g.report.StartMenuCreatedTimeChanged || shortcutCreationChanged(g.report.StartMenuBefore[scope], g.report.StartMenuAfter[scope])
		switch {
		case !before.Exists:
			g.report.RestoreResults[scope] = "absent_before"
		case g.report.BackupResults[scope] != "ok":
			g.report.RestoreResults[scope] = "no_backup"
		case after.Exists && !changed:
			g.report.RestoreResults[scope] = "unchanged"
		case !after.Exists && after.Status != "missing":
			g.report.RestoreResults[scope] = "state_unknown"
		case !g.hooks.TargetExists():
			g.report.RestoreResults[scope] = "target_missing"
		default:
			g.report.RestoreResults[scope] = "ok"
			if g.hooks.Restore(scope) != nil {
				g.report.RestoreResults[scope] = "failed"
			}
		}
	}
	g.report.CreatedTimeChanged = g.report.DesktopCreatedTimeChanged || g.report.StartMenuCreatedTimeChanged
	if result == "ok" && g.hooks.StabilizeIcon != nil {
		g.report.IconLocationStable = g.hooks.StabilizeIcon()
	}
	g.report.DesktopFinal, _ = g.hooks.Snapshot()
	g.hooks.Write(g.report)
}

// This file has no paths/accounts and is consumed once by the new backend.
// Keep it separate from R204's integer/null-only timing file.
func writeShortcutUpdateReport(root string, report shortcutUpdateReport) error {
	if err := os.MkdirAll(root, 0700); err != nil {
		return err
	}
	if info, err := os.Lstat(root); err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return os.ErrPermission
	}
	data, err := json.Marshal(report)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(root, ".update-shortcuts-*")
	if err != nil {
		return err
	}
	name := file.Name()
	defer os.Remove(name)
	_, err = file.Write(data)
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(name, filepath.Join(root, shortcutStateFilename))
}
