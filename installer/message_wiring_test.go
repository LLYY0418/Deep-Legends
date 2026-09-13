package main

import (
	"go/ast"
	"go/token"
	"strconv"
	"testing"
)

// Protect the routing boundary, not a blacklist of helper names. A new function
// in any Windows file must not let install messages avoid the tested completion
// path. Unrelated message handlers and unused helpers remain free to change.
func TestWindowsInstallMessageCannotBypassCompletion(t *testing.T) {
	function := completionFunction(t, "webview_windows.go", "onMessage")
	statements := function.Body.List
	if len(statements) == 0 {
		t.Fatal("missing message dispatch")
	}
	dispatch, ok := statements[len(statements)-1].(*ast.SwitchStmt)
	if !ok {
		t.Fatal("message dispatch must be the final statement")
	}
	// Include everything before the switch and its selector. Otherwise a shortcut
	// before case "install", or a rewritten message type, evades the case guard.
	prefix := append([]ast.Stmt(nil), statements[:len(statements)-1]...)
	prefix = append(prefix, &ast.SwitchStmt{Init: dispatch.Init, Tag: dispatch.Tag, Body: &ast.BlockStmt{}})
	assertCompletionBlock(t, prefix, `
if a.window.closed { return }
if a.window.startup != nil && a.window.startup.Message(raw) { return }
if !a.pageReady { return }
if len(raw) > 4096 { return }
var message uiMessage
if json.Unmarshal([]byte(raw), &message) != nil { return }
w := a.window
switch message.Type {}`)

	found := 0
	for _, statement := range dispatch.Body.List {
		clause, ok := statement.(*ast.CaseClause)
		if !ok {
			t.Fatal("invalid message case")
		}
		for _, label := range clause.List {
			literal, ok := label.(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				t.Fatal("message cases must use explicit string labels")
			}
			name, err := strconv.Unquote(literal.Value)
			if err != nil {
				t.Fatal(err)
			}
			if name != "install" {
				continue
			}
			found++
			if len(clause.List) != 1 {
				t.Fatal("install must have its own case")
			}
			// Validation may reject a request. Every accepted request, normal or
			// update, must reach the same single, top-level go a.install(message).
			assertCompletionBlock(t, clause.Body, `
if a.options.Update {
    message.Path = a.options.Destination
    if a.options.Error != nil {
        a.fail(failureMessage{Message: upgradeFailureMessage})
        return
    }
}
if w.browsing || !w.state.begin() { return }
w.pathRevision++
if a.payloadError != nil {
    a.fail(failureMessage{Message: "这个安装包不完整，请重新下载"})
    return
}
a.emit("installing", struct { Path string `+"`json:\"path\"`"+` }{message.Path})
go a.install(message)`)
		}
	}
	if found != 1 {
		t.Fatalf("expected one shared install message case, found %d", found)
	}
}

func TestWindowsInstallMessageCallbackUsesSharedDispatcher(t *testing.T) {
	function := completionFunction(t, "webview_windows.go", "embed")
	found := 0
	ast.Inspect(function.Body, func(node ast.Node) bool {
		assignment, ok := node.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for _, target := range assignment.Lhs {
			selector, ok := target.(*ast.SelectorExpr)
			if !ok || selector.Sel.Name != "MessageCallback" {
				continue
			}
			found++
			assertCompletionBlock(t, []ast.Stmt{assignment}, `
chromium.MessageCallback = func(raw string) { w.dispatch(func() { a.onMessage(raw) }) }`)
		}
		return true
	})
	if found != 1 {
		t.Fatalf("expected one shared WebView message callback, found %d", found)
	}
}

func TestWindowsInstallAutoUpdateUsesSharedDispatcher(t *testing.T) {
	function := completionFunction(t, "webview_windows.go", "embed")
	found := 0
	ast.Inspect(function.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != "New" {
			return true
		}
		owner, ok := selector.X.(*ast.Ident)
		if !ok || owner.Name != "webviewhost" {
			return true
		}
		found++
		if len(call.Args) != 3 {
			t.Fatal("startup must supply an explicit ready callback")
		}
		ready, ok := call.Args[1].(*ast.FuncLit)
		if !ok {
			t.Fatal("startup ready adapter must be explicit")
		}
		assertCompletionBlock(t, ready.Body.List, `
w.startupTimer.Stop()
a.pageReady = true
log.Print("installer page ready; WebView visible")
if a.options.Update {
    a.onMessage(`+"`{\"type\":\"install\"}`"+`)
} else if a.payloadError != nil {
    a.fail(failureMessage{Message: "这个安装包不完整，请重新下载"})
} else {
    a.validatePath(initial.Path, false)
}`)
		return false
	})
	if found != 1 {
		t.Fatalf("expected one startup ready adapter, found %d", found)
	}
}
