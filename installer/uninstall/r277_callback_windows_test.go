package main

import (
	"testing"

	"deeplegends/installer/internal/p11callbacktest"
	"deeplegends/installer/internal/webviewhost"
	"github.com/jchv/go-webview2/pkg/edge"
)

func TestR277ShellWndProcGCStress(t *testing.T) {
	previous := activeWindow
	defer func() { activeWindow = previous }()
	p11callbacktest.Run(t, wndProcCallback, func(chromium *edge.Chromium) {
		activeWindow = &shellWindow{surface: &webviewhost.WindowsSurface{Chromium: chromium}}
	})
}
