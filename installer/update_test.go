package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestUpdateModeArgumentsPageAndCommand(t *testing.T) {
	options := parseInstallerOptions([]string{"--update", "--dest", `C:\游戏\Deep Legends`})
	if !options.Update || options.Destination != `C:\游戏\Deep Legends` || options.Error != nil {
		t.Fatal(options)
	}
	for _, args := range [][]string{{"--update"}, {"--update", "--dest"}, {"--dest", "--update"}} {
		if parseInstallerOptions(args).Error == nil {
			t.Fatalf("missing dest accepted %v", args)
		}
	}
	for _, shortcut := range []bool{false, true} {
		got := installerCommandLine(`C:\Temp Space\setup.exe`, options.Destination, shortcut, true)
		if got != `"C:\Temp Space\setup.exe" /S --updated /D=C:\游戏\Deep Legends` || strings.Contains(got, "--no-desktop-shortcut") {
			t.Fatal(got)
		}
	}
	html, err := renderInstallerUI("0.12.0", options)
	if err != nil || !strings.Contains(html, `<div class="page on" id="page-install">`) || strings.Contains(html, `<div class="page on" id="page-setup">`) || !strings.Contains(html, "正在升级 Deep Legends") || !strings.Contains(html, `page: "install"`) {
		t.Fatalf("initial upgrade page wrong: %v", err)
	}
	if strings.Contains(html, `setPage("setup"); refresh();`) {
		t.Fatal("update retry exposed setup page")
	}
}

func TestUpdateCompletionWordingStaysCoupledToTemplate(t *testing.T) {
	const original = "安装已完成，正在等待应用窗口…"
	const upgraded = "升级已完成，正在等待应用窗口…"
	source, err := uiFiles.ReadFile("ui/installer.html")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(source), original) != 1 {
		t.Fatal("installer template wording changed without updating the upgrade replacement")
	}
	normal, err := renderInstallerUI("test", installerOptions{})
	if err != nil || !strings.Contains(normal, original) || strings.Contains(normal, upgraded) {
		t.Fatalf("normal installation wording changed: %v", err)
	}
	updated, err := renderInstallerUI("test", installerOptions{Update: true})
	if err != nil || strings.Contains(updated, original) || !strings.Contains(updated, upgraded) {
		t.Fatalf("upgrade replacement was not applied to the actual template: %v", err)
	}
}
func TestUpdateDestinationMustExistAndBeWritable(t *testing.T) {
	root := t.TempDir()
	if validateUpgradeDestination(root) != nil {
		t.Fatal("valid destination rejected")
	}
	entries, _ := os.ReadDir(root)
	if len(entries) != 0 {
		t.Fatal("probe retained")
	}
	missing := filepath.Join(root, "missing")
	if err := validateUpgradeDestination(missing); err == nil || err.Error() != upgradeFailureMessage {
		t.Fatal("missing destination accepted")
	}
	file := filepath.Join(root, "file")
	os.WriteFile(file, []byte("x"), 0600)
	if validateUpgradeDestination(file) == nil {
		t.Fatal("file accepted as directory")
	}
	if runtime.GOOS != "windows" && os.Geteuid() != 0 {
		os.Chmod(root, 0500)
		defer os.Chmod(root, 0700)
		if validateUpgradeDestination(root) == nil {
			t.Fatal("unwritable destination accepted")
		}
	}
}
