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
	app := &installerApp{window: w, meta: meta, payloadError: payloadError, options: options}
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
