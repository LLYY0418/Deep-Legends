package main

import (
	"errors"
	"os"
	"strconv"
	"strings"
)

const upgradeFailureMessage = "升级失败，请从发布页下载完整安装包重新安装"

type installerOptions struct {
	Update       bool
	FreshInstall bool
	ParentPID    uint32
	Destination  string
	Error        error
}

func parseInstallerOptions(args []string) installerOptions {
	var options installerOptions
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--update":
			options.Update = true
		case "--fresh-install":
			options.FreshInstall = true
		case "--parent-pid":
			if i+1 >= len(args) {
				options.Error = errors.New(upgradeFailureMessage)
				continue
			}
			i++
			value, err := strconv.ParseUint(args[i], 10, 32)
			if err != nil || value == 0 {
				options.Error = errors.New(upgradeFailureMessage)
			} else {
				options.ParentPID = uint32(value)
			}
		case "--dest":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				options.Error = errors.New(upgradeFailureMessage)
				continue
			}
			i++
			options.Destination = args[i]
		}
	}
	if options.Update && options.Destination == "" {
		options.Error = errors.New(upgradeFailureMessage)
	}
	if options.FreshInstall && (!options.Update || options.ParentPID == 0) {
		options.Error = errors.New(upgradeFailureMessage)
	}
	return options
}
func validateUpgradeDestination(dest string) error {
	info, err := os.Stat(dest)
	if err != nil || !info.IsDir() {
		return errors.New(upgradeFailureMessage)
	}
	probe, err := os.CreateTemp(dest, ".deeplegends-upgrade-*")
	if err != nil {
		return errors.New(upgradeFailureMessage)
	}
	_, writeErr := probe.Write([]byte("Deep Legends upgrade write probe"))
	closeErr := probe.Close()
	removeErr := os.Remove(probe.Name())
	if writeErr != nil || closeErr != nil || removeErr != nil {
		return errors.New(upgradeFailureMessage)
	}
	return nil
}
func upgradeSetupCommandLine(setup, dest string) string {
	return `"` + setup + `" /S --updated /D=` + strings.TrimRight(dest, `\`)
}
func installerCommandLine(setup, dest string, shortcut, update bool) string {
	if update {
		return upgradeSetupCommandLine(setup, dest)
	}
	return setupCommandLine(setup, dest, shortcut)
}
func renderInstallerUI(version string, options installerOptions) (string, error) {
	rendered, err := renderUI(version)
	if err != nil || !options.Update {
		return rendered, err
	}
	// Keep the R80 source artwork immutable. The upgrade shell has its progress
	// page selected before WebView's first paint and never exposes setup controls.
	return strings.NewReplacer(
		`<div class="page on" id="page-setup">`, `<div class="page" id="page-setup">`,
		`<div class="page" id="page-install">`, `<div class="page on" id="page-install">`,
		`page: "setup"`, `page: "install"`,
		`正在安装 Deep Legends`, `正在升级 Deep Legends`,
		`正在快速安装`, `正在升级`,
		`安装已完成，正在等待应用窗口…`, `升级已完成，正在等待应用窗口…`,
		`$("btn-retry").addEventListener("click", () => { setPage("setup"); refresh(); });`, `$("btn-retry").addEventListener("click", () => send("install", { path: pathInput.value }));`,
		`    refresh();`+"\n  },\n  path(s)", `    refresh();`+"\n    window.host.installing({ path: s.path });\n  },\n  path(s)",
	).Replace(rendered), nil
}

// The parent stays alive until the NSIS child has exited successfully. Report
// installation success before waiting for its normal quit, then reuse handoff.
func portableUpdateHandoff(report func(string) error, waitParent func() bool, start func()) bool {
	if report("DEEP_LEGENDS_UPDATE_INSTALLED") != nil || waitParent == nil || !waitParent() {
		return false
	}
	start()
	return true
}
func portableSetupCommandLine(setup, dest string) string {
	return `"` + setup + `" /S /currentuser --portable-upgrade --updated /D=` + strings.TrimRight(dest, `\`)
}
