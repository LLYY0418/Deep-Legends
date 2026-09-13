package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"testing"
)

// Source-level guard, runnable without Windows. This verifies call order only;
// it does not claim to test native visibility, DWM permissions, or zero flashing.
func TestActorCleanupHidesBeforeUncloaking(t *testing.T) {
	f, err := parser.ParseFile(token.NewFileSet(), "actor_windows.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	wanted := map[string]bool{"wmClose": false, "reset": false, "shutdown": false}
	ast.Inspect(f, func(n ast.Node) bool {
		c, ok := n.(*ast.CaseClause)
		if !ok {
			return true
		}
		for _, label := range c.List {
			name := ""
			switch v := label.(type) {
			case *ast.Ident:
				name = v.Name
			case *ast.BasicLit:
				name, _ = strconv.Unquote(v.Value)
			}
			if _, ok := wanted[name]; !ok {
				continue
			}
			wanted[name] = true
			var hide, uncloak token.Pos
			ast.Inspect(c, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				if fn, ok := call.Fun.(*ast.Ident); ok && fn.Name == "cloak" && len(call.Args) == 3 {
					if value, ok := call.Args[2].(*ast.Ident); ok && value.Name == "false" && !uncloak.IsValid() {
						uncloak = call.Pos()
					}
				}
				if fn, ok := call.Fun.(*ast.SelectorExpr); ok && fn.Sel.Name == "Call" && len(call.Args) == 2 {
					if recv, ok := fn.X.(*ast.Ident); ok && recv.Name == "showWindow" {
						if mode, ok := call.Args[1].(*ast.BasicLit); ok && mode.Value == "0" && !hide.IsValid() {
							hide = call.Pos()
						}
					}
				}
				return true
			})
			if !hide.IsValid() || !uncloak.IsValid() || hide >= uncloak {
				t.Errorf("%s must hide its own window before removing its cloak", name)
			}
		}
		return true
	})
	for name, found := range wanted {
		if !found {
			t.Errorf("missing cleanup path %s", name)
		}
	}
}
