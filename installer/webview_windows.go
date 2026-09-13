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
	"deeplegends/installer/payload"
	"github.com/jchv/go-webview2/pkg/edge"
)

type installerApp struct {
	options      installerOptions
	window       *shellWindow
	meta         payload.Metadata
	payloadError error
	startupError error
	pageReady    bool
	startupWarm  *executionWarmup
}

func (a *installerApp) embed(html string, initial initMessage) error {
	w := a.window
	var err error
	html, err = webviewhost.Document(html, initial)
	if err != nil {
		return fmt.Errorf("prepare installer document: %w", err)
	}
	chromium := edge.NewChromium()
	chromium.DataPath = filepath.Join(os.Getenv("LOCALAPPDATA"), "Temp", "DeepLegendsSetup.WebView2")
	chromium.MessageCallback = func(raw string) { w.dispatch(func() { a.onMessage(raw) }) }
	w.chromium = chromium
	log.Print("WebView environment/controller initialization")
	if !chromium.Embed(w.hwnd) {
		return errors.New("WebView environment/controller initialization failed")
	}
	w.surface = &webviewhost.WindowsSurface{Chromium: chromium, Window: w.hwnd, ShowWindow: w.show}
	log.Print("WebView controller created")
	if err := w.surface.Configure(); err != nil {
		return fmt.Errorf("configure WebView: %w", err)
	}
	// Suppress reload/navigation accelerators, which would reset the JS state
	// while NSIS is still running. Alt-F4 continues through WM_CLOSE.
	chromium.AcceleratorKeyCallback = func(key uint) bool {
		control, _, _ := user32.NewProc("GetKeyState").Call(0x11)
		alt, _, _ := user32.NewProc("GetKeyState").Call(0x12)
		if control&0x8000 != 0 {
			// Keep normal text editing; suppress browser reload, print, save,
			// find and navigation commands that could open extra UI.
			return key != 'A' && key != 'C' && key != 'V' && key != 'X' && key != 'Z' && key != 'Y'
		}
		if alt&0x8000 != 0 && (key == 0x25 || key == 0x27 || key == 0x24) {
			return true
		}
		return key >= 0x70 && key <= 0x7B && key != 0x73 // function keys except Alt-F4
	}
	w.startup = webviewhost.New(w.surface, func() {
		w.startupTimer.Stop()
		a.pageReady = true
		log.Print("installer page ready; WebView visible")
		if a.options.Update {
			a.onMessage(`{"type":"install"}`)
		} else if a.payloadError != nil {
			a.fail(failureMessage{Message: "这个安装包不完整，请重新下载"})
		} else {
			a.validatePath(initial.Path, false)
		}
	}, func(err error) {
		w.startupTimer.Stop()
		a.startupError = err
		log.Print("installer UI startup failed: ", err)
		postMessage.Call(w.hwnd, WM_CLOSE, 0, 0)
	})
	chromium.NavigationCompletedCallback = func(_ *edge.ICoreWebView2, args *edge.ICoreWebView2NavigationCompletedEventArgs) {
		err := webviewhost.NavigationResult(args)
		w.dispatch(func() { w.startup.NavigationCompleted(err) })
	}
	w.startupTimer = time.AfterFunc(webviewhost.PageTimeout, func() { w.dispatch(w.startup.Timeout) })
	log.Print("loading installer document")
	w.startup.Start(html)
	return nil
}

// All calls to this method run on the locked UI thread.
func (a *installerApp) emit(method string, value any) {
	data, err := json.Marshal(value)
	if err != nil {
		return
	}
	a.window.chromium.Eval("window.host." + method + "(" + string(data) + ");")
}

func (a *installerApp) fail(failure failureMessage) {
	if a.options.Update {
		failure.Message = upgradeFailureMessage
	}
	a.window.state.phase = phaseFailed
	a.emit("failed", failure)
}

func (a *installerApp) validatePath(path string, replace bool) {
	w := a.window
	w.pathRevision++
	revision := w.pathRevision
	go func() {
		result := checkPath(path, a.meta.InstalledBytes)
		if replace {
			result.Path = &path
		}
		w.dispatch(func() {
			if revision == w.pathRevision && !w.state.busy() {
				a.emit("path", result)
			}
		})
	}()
}

func (a *installerApp) onMessage(raw string) {
	if a.window.closed {
		return
	}
	if a.window.startup != nil && a.window.startup.Message(raw) {
		return
	}
	if !a.pageReady {
		return
	}
	if len(raw) > 4096 {
		return
	}
	var message uiMessage
	if json.Unmarshal([]byte(raw), &message) != nil {
		return
	}
	w := a.window
	switch message.Type {
	case "drag":
		// A move loop is modal; never enter it inside a WebView callback.
		w.dispatch(w.drag)
	case "minimize":
		showWindow.Call(w.hwnd, SW_MINIMIZE)
	case "close":
		postMessage.Call(w.hwnd, WM_CLOSE, 0, 0)
	case "path":
		if a.options.Update {
			return
		}
		if !w.state.busy() && !w.browsing {
			a.validatePath(message.Path, false)
		}
	case "browse":
		if a.options.Update {
			return
		}
		if w.state.busy() || w.browsing {
			return
		}
		w.browsing = true
		w.pathRevision++
		w.dispatch(func() {
			selected, ok := chooseDirectory(w.hwnd, message.Path)
			w.browsing = false
			if ok {
				a.validatePath(selected, true)
			}
		})
	case "install":
		if a.options.Update {
			message.Path = a.options.Destination
			if a.options.Error != nil {
				a.fail(failureMessage{Message: upgradeFailureMessage})
				return
			}
		}
		if w.browsing || !w.state.begin() {
			return
		}
		w.pathRevision++
		if a.payloadError != nil {
			a.fail(failureMessage{Message: "这个安装包不完整，请重新下载"})
			return
		}
		a.emit("installing", struct {
			Path string `json:"path"`
		}{message.Path})
		go a.install(message)
	}
}
