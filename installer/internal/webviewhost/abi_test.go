package webviewhost

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// This runs on macOS/Linux too. Derive the oracle from actual dependency
// declarations, not another handwritten array that repeats production mistakes.
func dependencyCOMSlots(t *testing.T) map[string]map[string]int {
	t.Helper()
	command := exec.Command("go", "list", "-m", "-f", "{{.Dir}}", "github.com/jchv/go-webview2")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("locate the pinned WebView2 dependency: %v: %s", err, output)
	}
	directory := filepath.Join(strings.TrimSpace(string(output)), "pkg", "edge")
	structs := map[string]*ast.StructType{}
	for _, name := range []string{"corewebview2.go", "ICoreWebView2Controller.go", "ICoreWebView2Controller2.go", "ICoreWebViewSettings.go", "ICoreWebView2NavigationCompletedEventArgs.go"} {
		file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(directory, name), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			if spec, ok := node.(*ast.TypeSpec); ok {
				if structure, ok := spec.Type.(*ast.StructType); ok {
					structs[spec.Name.Name] = structure
				}
			}
			return true
		})
	}
	var expand func(string) []string
	expand = func(name string) []string {
		structure := structs[name]
		if structure == nil {
			t.Fatalf("missing COM declaration %s", name)
		}
		var names []string
		for _, field := range structure.Fields.List {
			if len(field.Names) == 0 {
				embedded, ok := field.Type.(*ast.Ident)
				if !ok {
					t.Fatalf("unexpected embedded COM field in %s", name)
				}
				names = append(names, expand(embedded.Name)...)
			} else {
				for _, method := range field.Names {
					names = append(names, method.Name)
				}
			}
		}
		return names
	}
	layouts := map[string]map[string]int{}
	for _, name := range []string{"_IUnknownVtbl", "_ICoreWebView2ControllerVtbl", "_ICoreWebView2Controller2Vtbl", "iCoreWebView2Vtbl", "_ICoreWebViewSettingsVtbl", "_ICoreWebView2NavigationCompletedEventArgsVtbl"} {
		slots := map[string]int{}
		for index, method := range expand(name) {
			slots[method] = index
		}
		layouts[name] = slots
	}
	return layouts
}

func TestCOMSlotsMatchCompleteDependencyInterfaces(t *testing.T) {
	layouts := dependencyCOMSlots(t)
	for _, item := range []struct {
		iface, method string
		slot          int
	}{
		{"_IUnknownVtbl", "QueryInterface", unknownQueryInterface},
		{"_IUnknownVtbl", "Release", unknownRelease},
		{"_ICoreWebView2ControllerVtbl", "GetIsVisible", controllerGetVisible},
		{"_ICoreWebView2ControllerVtbl", "PutIsVisible", controllerPutVisible},
		{"_ICoreWebView2ControllerVtbl", "PutBounds", controllerPutBounds},
		{"_ICoreWebView2ControllerVtbl", "Close", controllerClose},
		{"_ICoreWebView2ControllerVtbl", "GetCoreWebView2", controllerGetCore},
		{"_ICoreWebView2Controller2Vtbl", "PutDefaultBackgroundColor", controller2PutBackground},
		{"iCoreWebView2Vtbl", "GetSettings", coreGetSettings},
		{"iCoreWebView2Vtbl", "NavigateToString", coreNavigateToString},
		{"_ICoreWebViewSettingsVtbl", "PutIsScriptEnabled", settingsPutScriptEnabled},
		{"_ICoreWebViewSettingsVtbl", "PutIsWebMessageEnabled", settingsPutWebMessageEnabled},
		{"_ICoreWebViewSettingsVtbl", "PutAreDefaultScriptDialogsEnabled", settingsPutDefaultScriptDialogs},
		{"_ICoreWebViewSettingsVtbl", "PutIsStatusBarEnabled", settingsPutStatusBar},
		{"_ICoreWebViewSettingsVtbl", "PutAreDevToolsEnabled", settingsPutDevTools},
		{"_ICoreWebViewSettingsVtbl", "PutAreDefaultContextMenusEnabled", settingsPutContextMenus},
		{"_ICoreWebViewSettingsVtbl", "PutIsZoomControlEnabled", settingsPutZoomControl},
		{"_ICoreWebViewSettingsVtbl", "PutIsBuiltInErrorPageEnabled", settingsPutBuiltInErrorPage},
		{"_ICoreWebView2NavigationCompletedEventArgsVtbl", "GetIsSuccess", navigationGetSuccess},
		{"_ICoreWebView2NavigationCompletedEventArgsVtbl", "GetWebErrorStatus", navigationGetError},
	} {
		t.Run(item.method, func(t *testing.T) {
			want, exists := layouts[item.iface][item.method]
			if !exists {
				t.Fatalf("%s.%s missing from dependency", item.iface, item.method)
			}
			if item.slot != want {
				t.Fatalf("%s: configured slot %d, actual COM slot %d", item.method, item.slot, want)
			}
		})
	}
}
