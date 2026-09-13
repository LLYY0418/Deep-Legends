package main

import (
	"bytes"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"testing"
)

// Go skips *_windows.go on Unix. Parse the OS adapter boundaries explicitly.
// message_wiring_test.go also guards the callers: checking only completion and
// exit bodies cannot detect a newly added path that never calls them. These
// source contracts complement behavior tests; they are not a whole-program proof.
func completionFunction(t *testing.T, name, symbol string) *ast.FuncDecl {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), name, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, declaration := range file.Decls {
		if function, ok := declaration.(*ast.FuncDecl); ok && function.Name.Name == symbol {
			return function
		}
	}
	t.Fatalf("missing %s in %s", symbol, name)
	return nil
}

func assertCompletionBlock(t *testing.T, statements []ast.Stmt, want string) {
	t.Helper()
	expected, err := parser.ParseFile(token.NewFileSet(), "expected.go", "package main; func expected() {"+want+"}", 0)
	if err != nil {
		t.Fatal(err)
	}
	print := func(node ast.Node) string {
		var result bytes.Buffer
		if err := format.Node(&result, token.NewFileSet(), node); err != nil {
			t.Fatal(err)
		}
		return result.String()
	}
	got, expectedBody := print(&ast.BlockStmt{List: statements}), print(expected.Decls[0].(*ast.FuncDecl).Body)
	if got != expectedBody {
		t.Fatalf("Windows adapter bypasses portable completion/exit policy:\ngot %s\nwant %s", got, expectedBody)
	}
}

func TestWindowsInstallCompletionCannotBypassPortableUpdateHandoff(t *testing.T) {
	function := completionFunction(t, "install_windows.go", "install")
	found := 0
	ast.Inspect(function.Body, func(node ast.Node) bool {
		clause, ok := node.(*ast.CommClause)
		if !ok {
			return true
		}
		expression, ok := clause.Comm.(*ast.ExprStmt)
		if !ok {
			return true
		}
		receive, ok := expression.X.(*ast.UnaryExpr)
		if !ok || receive.Op != token.ARROW {
			return true
		}
		channel, ok := receive.X.(*ast.Ident)
		if !ok || channel.Name != "finished" {
			return true
		}
		found++
		assertCompletionBlock(t, clause.Body, `
completeInstallation(a.options, installationResult{ExitCode: cmd.ProcessState.ExitCode(), WaitError: waitErr, Destination: dest}, installationCompletionHooks{
    Failed: reportFailure,
    Cleanup: func() { _ = os.RemoveAll(temporaryDir) },
    Handoff: func() { a.handoffApplication(filepath.Join(dest, a.meta.ExeName), dest) },
})
return`)
		return false
	})
	if found != 1 {
		t.Fatalf("expected one portable completion boundary, found %d", found)
	}
}

func TestWindowsHandoffExitAdapterCannotTerminateApplication(t *testing.T) {
	function := completionFunction(t, "handoff_windows.go", "handoffApplication")
	found := 0
	ast.Inspect(function.Body, func(node ast.Node) bool {
		field, ok := node.(*ast.KeyValueExpr)
		if !ok {
			return true
		}
		key, ok := field.Key.(*ast.Ident)
		if !ok || key.Name != "CloseInstaller" {
			return true
		}
		callback, ok := field.Value.(*ast.FuncLit)
		if !ok {
			t.Fatal("exit adapter must be explicit")
		}
		found++
		assertCompletionBlock(t, callback.Body.List, `
finishInstallerHandoff(w.hwnd, visible, installerExitHooks{
    Record: func(visible bool) { log.Printf("application handoff visible=%t elapsed_ms=%d", visible, time.Since(started).Milliseconds()) },
    Dispatch: w.dispatch,
    MarkReady: func() { w.state.phase = phaseReady },
    CloseWindow: func(hwnd uintptr) { postMessage.Call(hwnd, WM_CLOSE, 0, 0) },
})`)
		return false
	})
	if found != 1 {
		t.Fatalf("expected one self-only exit adapter, found %d", found)
	}
}
