package main

import "deeplegends/installer/internal/webviewhost"

func configureWebView(surface *webviewhost.WindowsSurface) error {
	if err := surface.Configure(); err != nil {
		return err
	}
	chromium := surface.Chromium
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
	return nil
}
