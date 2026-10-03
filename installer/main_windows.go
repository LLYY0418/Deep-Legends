package main

import (
	"log"
	"os"
	"runtime"

	"deeplegends/installer/internal/webviewhost"
	"deeplegends/installer/payload"
	"golang.org/x/sys/windows"
)

func main() {
	defer webviewhost.StartupLog("DeepLegendsSetup")()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	enableDPIAwareness()
	options := parseInstallerOptions(os.Args[1:])
	mutex, first, err := acquireSingleInstance()
	if err != nil {
		log.Print(err)
		return
	}
	if !first {
		log.Print("another installer instance is already running")
		return
	}
	defer windows.CloseHandle(mutex)
	meta, payloadError := payload.LoadMetadata()
	log.Printf("installer version=%s fingerprint=%s", meta.Version, meta.Fingerprint)
	var timing *updateInstallTiming
	var waitParent func() bool
	if options.Update {
		timing = newUpdateInstallTiming()
		timing.mark("installer_start")
		if options.Error != nil {
			return
		}
		if options.ParentPID == 0 {
			options.ParentPID = legacyUpgradeParent(options.Destination, meta.ExeName)
		}
		if options.ParentPID != 0 {
			parent, err := windows.OpenProcess(windows.SYNCHRONIZE, false, options.ParentPID)
			if err != nil && err != windows.ERROR_INVALID_PARAMETER {
				return
			}
			if err == nil {
				defer windows.CloseHandle(parent)
				waitParent = func() bool {
					result, err := windows.WaitForSingleObject(parent, 15000)
					return err == nil && result == windows.WAIT_OBJECT_0
				}
			}
		}
		if waitParent == nil {
			waitParent = func() bool { return true }
		}
	}

	if err := webviewhost.CheckRuntime(); err != nil {
		webviewhost.ReportStartupFailure("DeepLegendsSetup", "安装", err)
		return
	}
	path := initialInstallDir()
	if options.Update {
		path = options.Destination
	}
	free, _ := diskFreeBytes(path)
	html, err := renderInstallerUI(meta.Version, options)
	if err != nil {
		webviewhost.ReportStartupFailure("DeepLegendsSetup", "安装", err)
		return
	}
	w, err := createShellWindow()
	if err != nil {
		webviewhost.ReportStartupFailure("DeepLegendsSetup", "安装", err)
		return
	}
	app := &installerApp{window: w, meta: meta, payloadError: payloadError, options: options, waitParent: waitParent, timing: timing}
	if err := app.embed(html, initMessage{Path: path, NeedBytes: requiredSpace(meta.InstalledBytes), FreeBytes: free, Version: meta.Version}); err != nil {
		destroyWindow.Call(w.hwnd)
		runMessageLoop() // consume WM_QUIT before opening the error dialog
		webviewhost.ReportStartupFailure("DeepLegendsSetup", "安装", err)
		return
	}
	runMessageLoop()
	if app.startupError != nil {
		webviewhost.ReportStartupFailure("DeepLegendsSetup", "安装", app.startupError)
	}
}
