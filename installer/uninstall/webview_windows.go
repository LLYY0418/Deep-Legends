package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"deeplegends/installer/internal/webviewhost"
	"github.com/jchv/go-webview2/pkg/edge"
)

type uninstallerApp struct {
	window               *shellWindow
	launch               uninstallLaunch
	initialError         error
	result               int
	additionalWorkspaces []string
	startupError         error
	pageReady            bool
}

func cacheDirectory() string {
	if root := os.Getenv("LOCALAPPDATA"); root != "" {
		return filepath.Join(root, "LOLLootAssistant")
	}
	return ""
}

func (a *uninstallerApp) embed(html string, initial initMessage) error {
	w := a.window
	var err error
	html, err = webviewhost.Document(html, initial)
	if err != nil {
		return fmt.Errorf("prepare uninstaller document: %w", err)
	}
	chromium := edge.NewChromium()
	chromium.DataPath = filepath.Join(os.TempDir(), "DeepLegendsUninstall.WebView2")
	chromium.MessageCallback = func(raw string) {
		w.dispatch(func() { a.onMessage(raw) })
	}
	w.chromium = chromium
	log.Print("WebView environment/controller initialization")
	if !chromium.Embed(w.hwnd) {
		return errors.New("WebView environment/controller initialization failed")
	}
	w.surface = &webviewhost.WindowsSurface{Chromium: chromium, Window: w.hwnd, ShowWindow: w.show}
	log.Print("WebView controller created")
	if err := configureWebView(w.surface); err != nil {
		return fmt.Errorf("configure WebView: %w", err)
	}
	w.startup = webviewhost.New(w.surface, func() {
		w.startupTimer.Stop()
		a.pageReady = true
		log.Print("uninstaller page ready; WebView visible")
		if a.initialError != nil {
			a.fail(failureMessage{Message: "卸载文件不完整，请重新安装后再卸载", Detail: a.initialError.Error()})
		}
	}, func(err error) {
		w.startupTimer.Stop()
		a.startupError = err
		log.Print("uninstaller UI startup failed: ", err)
		postMessage.Call(w.hwnd, WM_CLOSE, 0, 0)
	})
	chromium.NavigationCompletedCallback = func(_ *edge.ICoreWebView2, args *edge.ICoreWebView2NavigationCompletedEventArgs) {
		err := webviewhost.NavigationResult(args)
		w.dispatch(func() { w.startup.NavigationCompleted(err) })
	}
	w.startupTimer = time.AfterFunc(webviewhost.PageTimeout, func() { w.dispatch(w.startup.Timeout) })
	log.Print("loading uninstaller document")
	w.startup.Start(html)
	return nil
}

func (a *uninstallerApp) emit(method string, value any) {
	data, err := json.Marshal(value)
	if err == nil {
		a.window.chromium.Eval("window.host." + method + "(" + string(data) + ");")
	}
}

func (a *uninstallerApp) fail(value failureMessage) {
	a.result = 1
	a.window.state.running = false
	a.emit("failed", value)
}

func (a *uninstallerApp) onMessage(raw string) {
	if a.window.closed {
		return
	}
	if a.window.startup != nil && a.window.startup.Message(raw) {
		return
	}
	if !a.pageReady {
		return
	}
	var message uiMessage
	if len(raw) > 4096 || json.Unmarshal([]byte(raw), &message) != nil {
		return
	}
	w := a.window
	switch message.Type {
	case "drag":
		w.dispatch(w.drag)
	case "minimize":
		showWindow.Call(w.hwnd, SW_MINIMIZE)
	case "close":
		postMessage.Call(w.hwnd, WM_CLOSE, 0, 0)
	case "confirm":
		if !w.state.begin() {
			return
		}
		if a.initialError != nil {
			// Retry transient preparation failures (e.g. a locked Core file).
			executable, err := os.Executable()
			options := a.launch.Options
			options.InstallDir = a.launch.Directory
			var fresh uninstallLaunch
			if err == nil {
				fresh, err = prepareLaunch(executable, options)
			}
			if err != nil {
				a.fail(failureMessage{Message: "卸载文件不完整，请重新安装后再卸载", Detail: err.Error()})
				return
			}
			relocated, err := fresh.relocate(executable)
			if err != nil {
				_ = os.RemoveAll(fresh.Workspace)
				a.fail(failureMessage{Message: "无法启动卸载，请关闭后重试", Detail: err.Error()})
				return
			}
			if relocated {
				w.state.running = false
				postMessage.Call(w.hwnd, WM_CLOSE, 0, 0)
				return
			}
			a.additionalWorkspaces = append(a.additionalWorkspaces, fresh.Workspace)
			a.launch, a.initialError = fresh, nil
		}
		a.emit("uninstalling", struct {
			Path string `json:"path"`
		}{a.launch.Directory})
		go a.uninstall(message.DeleteData)
	}
}

func (a *uninstallerApp) uninstall(deleteData bool) {
	w := a.window
	baseline := snapshotDirectory(a.launch.Directory)
	model := uninstallProgressModel{baselineBytes: baseline.Bytes, baselineFiles: baseline.Files}
	cmd := a.launch.command(true, deleteData)
	started := time.Now()
	if err := cmd.Start(); err != nil {
		w.dispatch(func() {
			a.fail(failureMessage{Message: "无法启动卸载，请重新安装后重试", Detail: err.Error()})
		})
		return
	}
	finished := make(chan error, 1)
	go func() { finished <- cmd.Wait() }()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			update := model.update(snapshotDirectory(a.launch.Directory), time.Since(started))
			w.dispatch(func() { a.emit("progress", update) })
		case err := <-finished:
			code := exitCode(cmd, err)
			// The final cleanup step remains visible even for a tiny installation.
			if code == 0 {
				if !model.deleting {
					w.dispatch(func() { a.emit("progress", progressMessage{Percent: 94, Stage: stageDeleting}) })
					time.Sleep(200 * time.Millisecond)
				}
				w.dispatch(func() { a.emit("progress", progressMessage{Percent: 97, Stage: stageCleaning}) })
			}
			finishUninstall(code, deleteData, os.Getenv("LOCALAPPDATA"), os.RemoveAll, func(method string, value any) {
				if method == "failed" {
					w.dispatch(func() {
						a.fail(failureMessage{Message: "卸载没有完成，请关闭正在使用的程序后重试", Detail: fmt.Sprintf("退出码：%d；安装路径：%s", code, a.launch.Directory)})
					})
					return
				}
				if method == "progress" {
					w.dispatch(func() { a.emit(method, value) })
					time.Sleep(2 * time.Second) // readable, non-blocking cache warning
					return
				}
				time.Sleep(200 * time.Millisecond)
				w.dispatch(func() {
					a.result = 0
					a.emit("done", nil)
					time.AfterFunc(1200*time.Millisecond, func() {
						w.dispatch(func() {
							w.state.running = false
							postMessage.Call(w.hwnd, WM_CLOSE, 0, 0)
						})
					})
				})
			})
			return
		}
	}
}
