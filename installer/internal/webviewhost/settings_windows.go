package webviewhost

import (
	"errors"
	"fmt"
	"log"
	"unsafe"

	"github.com/jchv/go-webview2/pkg/edge"
)

// Use the base ICoreWebView2Settings ABI. The dependency's GetSettings/Put*
// wrappers read GetLastError instead of HRESULT and can reject successful calls.
func configureController(controller unsafe.Pointer) error {
	core, err := coreFromController(controller)
	if err != nil {
		return err
	}
	defer comCall(core, unknownRelease)
	var settings unsafe.Pointer
	if err := comCall(core, coreGetSettings, uintptr(unsafe.Pointer(&settings))); err != nil {
		return fmt.Errorf("get_Settings: %w", err)
	}
	if settings == nil {
		return errors.New("get_Settings returned success without an interface")
	}
	defer comCall(settings, unknownRelease)
	for _, option := range []struct {
		name  string
		slot  int
		value uintptr
	}{
		{"IsScriptEnabled", settingsPutScriptEnabled, 1},
		{"IsWebMessageEnabled", settingsPutWebMessageEnabled, 1},
		{"AreDefaultScriptDialogsEnabled", settingsPutDefaultScriptDialogs, 0},
		{"IsStatusBarEnabled", settingsPutStatusBar, 0},
		{"AreDevToolsEnabled", settingsPutDevTools, 0},
		{"AreDefaultContextMenusEnabled", settingsPutContextMenus, 0},
		{"IsZoomControlEnabled", settingsPutZoomControl, 0},
		{"IsBuiltInErrorPageEnabled", settingsPutBuiltInErrorPage, 0},
	} {
		if err := comCall(settings, option.slot, option.value); err != nil {
			return fmt.Errorf("put_%s: %w", option.name, err)
		}
	}
	return nil
}

func (s *WindowsSurface) Configure() error {
	controller := unsafe.Pointer(s.Chromium.GetController())
	if err := configureController(controller); err != nil {
		return err
	}
	s.Chromium.SetGlobalPermission(edge.CoreWebView2PermissionStateDeny)
	// Controller2 is optional; preserve the dark native background on older
	// runtimes. A successful QueryInterface returns a reference we must release.
	var controller2 unsafe.Pointer
	iid := edge.NewGUID("{c979903e-d4ca-4228-92eb-47ee3fa96eab}")
	if err := comCall(controller, unknownQueryInterface, uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&controller2))); err == nil && controller2 != nil {
		defer comCall(controller2, unknownRelease)
		// COREWEBVIEW2_COLOR is passed by value: A=255, R=11, G=14, B=20.
		if err := comCall(controller2, controller2PutBackground, 0x140E0BFF); err != nil {
			log.Print("WebView background: ", err)
		}
	}
	log.Print("WebView settings configured")
	return nil
}
