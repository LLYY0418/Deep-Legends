package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"testing"
)

// This runs on every host; the native companion exercises the registered ABI.
func TestR277ShellCallbackScalarLPARAM(t *testing.T) {
	for _, path := range []string{"window_windows.go", filepath.Join("uninstall", "window_windows.go")} {
		t.Run(path, func(t *testing.T) {
			file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Name.Name != "shellWndProc" {
					continue
				}
				found = true
				want := []string{"uintptr", "uint32", "uintptr", "uintptr"}
				if len(fn.Type.Params.List) != len(want) {
					t.Fatal("shellWndProc ABI parameter count changed")
				}
				for i, field := range fn.Type.Params.List {
					kind, ok := field.Type.(*ast.Ident)
					if !ok || kind.Name != want[i] {
						t.Errorf("shellWndProc parameter %d must be %s, not a GC-scanned pointer", i, want[i])
					}
				}
				reads := 0
				ast.Inspect(fn.Body, func(node ast.Node) bool {
					clause, ok := node.(*ast.CaseClause)
					if !ok {
						return true
					}
					isDPI := len(clause.List) == 1
					if isDPI {
						id, ok := clause.List[0].(*ast.Ident)
						isDPI = ok && id.Name == "WM_DPICHANGED"
					}
					ast.Inspect(clause, func(n ast.Node) bool {
						call, ok := n.(*ast.CallExpr)
						if !ok {
							return true
						}
						sel, ok := call.Fun.(*ast.SelectorExpr)
						if ok && sel.Sel.Name == "ReadProcessMemory" {
							reads++
							if !isDPI {
								t.Error("LPARAM pointer read outside WM_DPICHANGED")
							}
						}
						return true
					})
					return false
				})
				if reads != 1 {
					t.Errorf("want one message-scoped RECT read, got %d", reads)
				}
			}
			if !found {
				t.Fatal("shellWndProc missing")
			}
		})
	}
}
