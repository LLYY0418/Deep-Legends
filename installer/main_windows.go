package main

import (
	"errors"
	"log"
	"os"
	"runtime"
	"time"

	"deeplegends/installer/internal/webviewhost"
	"deeplegends/installer/payload"
	"golang.org/x/sys/windows"
)

func main() {
	if installerWarmup(os.Args[1:]) {
		warmInstallerWebView()
		return
	}
	mainEntered := time.Now()
	processCreatedToMain := processCreatedToMainMillis()
	defer webviewhost.StartupLog("DeepLegendsSetup")()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	enableDPIAwareness()
	options := parseInstallerOptions(os.Args[1:])
	var standbySignal *installerStandbySignal
	if options.Standby {
		if options.Error != nil {
			return
		}
		var err error
		standbySignal, err = openInstallerStandby(options)
		if err != nil {
			return
		}
		defer standbySignal.close()
	}
	var mutex windows.Handle
	first := true
	var err error
	if !options.Standby {
		mutex, first, err = acquireSingleInstance()
	}
	if err != nil {
		log.Print(err)
		return
	}
	if !first {
		log.Print("another installer instance is already running")
		return
	}
	defer func() {
		if mutex != 0 {
			windows.CloseHandle(mutex)
		}
	}()
	meta, payloadError := payload.LoadMetadata()
	if options.Standby && payloadError != nil {
		return
	}
	log.Printf("installer version=%s fingerprint=%s", meta.Version, meta.Fingerprint)
	var timing *updateInstallTiming
	var waitParent func() bool
	if options.Update {
		timing = newUpdateInstallTiming()
		timing.mark("installer_start")
		timing.duration("process_created_to_main_ms", processCreatedToMain)
		timing.duration("go_initialization_ms", time.Since(mainEntered).Milliseconds())
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

	shellStarted := time.Now()
	w, err := createShellWindow()
	if err != nil {
		if !options.Standby {
			webviewhost.ReportStartupFailure("DeepLegendsSetup", "安装", err)
		}
		return
	}
	timing.duration("shell_create_ms", time.Since(shellStarted).Milliseconds())
	w.timing = timing
	app := &installerApp{window: w, meta: meta, payloadError: payloadError, options: options, waitParent: waitParent, timing: timing, standbySignal: standbySignal}
	defer func() {
		if app.standbyMutex != 0 {
			windows.CloseHandle(app.standbyMutex)
		}
	}()

	if options.Update {
		w.starting = true
	}
	if err := webviewhost.CheckRuntime(); err != nil {
		w.starting = false
		app.recoverFailedUpdate()
		closeInstallerWindow(w)
		if options.Update {
			err = errors.New(upgradeFailureMessage)
		}
		if !options.Standby {
			webviewhost.ReportStartupFailure("DeepLegendsSetup", "安装", err)
		}
		return
	}
	path := options.Destination
	var free int64
	if !options.Update {
		path = initialInstallDir()
		free, _ = diskFreeBytes(path)
	}
	// Update validates destination and space again in install(), after visibility.
	html, err := renderInstallerUI(meta.Version, options)
	if err != nil {
		w.starting = false
		app.recoverFailedUpdate()
		closeInstallerWindow(w)
		if options.Update {
			err = errors.New(upgradeFailureMessage)
		}
		if !options.Standby {
			webviewhost.ReportStartupFailure("DeepLegendsSetup", "安装", err)
		}
		return
	}
	if err := app.embed(html, initMessage{Path: path, NeedBytes: requiredSpace(meta.InstalledBytes), FreeBytes: free, Version: meta.Version}); err != nil {
		w.starting = false
		app.recoverFailedUpdate()
		if options.Update {
			err = errors.New(upgradeFailureMessage)
		}
		closeInstallerWindow(w) // consume WM_QUIT before opening the error dialog
		if !options.Standby {
			webviewhost.ReportStartupFailure("DeepLegendsSetup", "安装", err)
		}
		return
	}
	runMessageLoop()
	if app.startupError != nil && !options.Standby {
		webviewhost.ReportStartupFailure("DeepLegendsSetup", "安装", app.startupError)
	}
}
