package main

import (
	"log"
	"os"
	"runtime"

	"deeplegends/installer/internal/webviewhost"
	uiassets "deeplegends/installer/ui"
	"golang.org/x/sys/windows"
)

func main() { os.Exit(run()) }

func run() int {
	defer webviewhost.StartupLog("DeepLegendsUninstall")()
	log.Print("uninstaller version=", version)
	executable, err := os.Executable()
	if err != nil {
		return 1
	}
	var launch uninstallLaunch
	if len(os.Args) == 2 && os.Args[1] == "--dl-uninstall-worker" {
		launch, err = loadWorker(executable)
	} else {
		var options uninstallOptions
		options, err = parseOptions(commandParameters())
		if err == nil {
			launch, err = prepareLaunch(executable, options)
		}
		launch.Options = options
		if err == nil {
			var relocated bool
			relocated, err = launch.relocate(executable)
			if relocated {
				return 0 // the temporary worker owns cleanup and the operation
			}
		}
	}
	if launch.Workspace != "" {
		defer launch.cleanup(executable)
	}
	if err != nil {
		log.Print(err)
		if launch.Options.silent() {
			return 1
		}
		return launch.showUI(err)
	}
	return dispatchMode(launch.Options, func() int { return launch.runWithoutUI(true) }, func() int { return launch.showUI(nil) })
}

func (launch uninstallLaunch) showUI(launchError error) int {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := webviewhost.CheckRuntime(); err != nil {
		webviewhost.ReportStartupFailure("DeepLegendsUninstall", "卸载", err)
		return 1
	}
	enableDPIAwareness()
	mutex, first, err := acquireSingleInstance()
	if err != nil {
		return 1
	}
	if !first {
		log.Print("another uninstaller instance is already running")
		return 0
	}
	defer windows.CloseHandle(mutex)
	html, err := uiassets.RenderUninstaller()
	if err != nil {
		webviewhost.ReportStartupFailure("DeepLegendsUninstall", "卸载", err)
		return 1
	}
	w, err := createShellWindow()
	if err != nil {
		webviewhost.ReportStartupFailure("DeepLegendsUninstall", "卸载", err)
		return 1
	}
	baseline := snapshotDirectory(launch.Directory)
	cache := snapshotDirectory(cacheDirectory())
	app := &uninstallerApp{window: w, launch: launch, initialError: launchError}
	if err := app.embed(html, initMessage{Path: launch.Directory, SizeBytes: baseline.Bytes, CacheBytes: cache.Bytes, Version: version}); err != nil {
		destroyWindow.Call(w.hwnd)
		runMessageLoop() // consume WM_QUIT before opening the error dialog
		webviewhost.ReportStartupFailure("DeepLegendsUninstall", "卸载", err)
		return 1
	}
	runMessageLoop()
	for _, workspace := range app.additionalWorkspaces {
		_ = os.RemoveAll(workspace)
	}
	if app.startupError != nil {
		webviewhost.ReportStartupFailure("DeepLegendsUninstall", "卸载", app.startupError)
		return 1
	}
	return app.result
}
