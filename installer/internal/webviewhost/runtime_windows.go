package webviewhost

import (
	"errors"
	"fmt"
	"log"

	"github.com/jchv/go-webview2/webviewloader"
)

// Ask the same loader that creates the controller. Registry-only probing can
// disagree with the runtime actually available to this process.
func CheckRuntime() error {
	version, err := webviewloader.GetInstalledVersion()
	if err != nil {
		return fmt.Errorf("detect WebView2 runtime: %w", err)
	}
	if version == "" {
		return errors.New("未检测到 Microsoft Edge WebView2 Runtime，请安装后重新打开安装包")
	}
	log.Print("WebView runtime version=", version)
	return nil
}
