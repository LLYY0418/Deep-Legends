//go:build windows

package main

import (
	"deeplegends/installer/internal/webviewhost"
	"github.com/jchv/go-webview2/pkg/edge"
	"runtime"
	"time"
)

// No payload, installer mutex, application data, ACK or installation writes.
// The host remains hidden; only the shared WebView profile/cache is prepared.
func warmInstallerWebView() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	enableDPIAwareness()
	w, err := createShellWindow()
	if err != nil {
		return
	}
	w.starting = true
	chromium := newInstallerChromium()
	w.chromium = chromium
	if !chromium.Embed(w.hwnd) {
		destroyWindow.Call(w.hwnd)
		runMessageLoop()
		return
	}
	surface := &webviewhost.WindowsSurface{Chromium: chromium, Window: w.hwnd, ShowWindow: func() {}, DeferVisible: true}
	w.surface = surface
	if surface.Configure() != nil {
		destroyWindow.Call(w.hwnd)
		runMessageLoop()
		return
	}
	html, err := renderInstallerUI("", installerOptions{Update: true})
	if err != nil {
		destroyWindow.Call(w.hwnd)
		runMessageLoop()
		return
	}
	html, err = webviewhost.Document(html, initMessage{})
	if err != nil {
		destroyWindow.Call(w.hwnd)
		runMessageLoop()
		return
	}
	closeWarm := func() { destroyWindow.Call(w.hwnd) }
	session := webviewhost.New(surface, closeWarm, func(error) { closeWarm() })
	chromium.MessageCallback = func(raw string) { w.dispatch(func() { session.Message(raw) }) }
	chromium.NavigationCompletedCallback = func(_ *edge.ICoreWebView2, args *edge.ICoreWebView2NavigationCompletedEventArgs) {
		err := webviewhost.NavigationResult(args)
		w.dispatch(func() { session.NavigationCompleted(err) })
	}
	timer := time.AfterFunc(webviewhost.PageTimeout, func() { w.dispatch(session.Timeout) })
	defer timer.Stop()
	session.Start(html)
	runMessageLoop()
}
