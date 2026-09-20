package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// This guard discovers emitters instead of trusting a historical filename list.
// It checks direct map literals and constant concatenation, not arbitrary data
// flow or identifier values inside free-form strings. Runtime privacy tests and
// schema review remain mandatory for any newly introduced diagnostic emitter.
func forbiddenDiagnosticKeys(source string) ([]string, error) {
	file, err := parser.ParseFile(token.NewFileSet(), "emitter.go", source, 0)
	if err != nil {
		return nil, err
	}
	var stringValue func(ast.Expr) string
	stringValue = func(expr ast.Expr) string {
		switch value := expr.(type) {
		case *ast.BasicLit:
			if value.Kind == token.STRING {
				s, _ := strconv.Unquote(value.Value)
				return s
			}
		case *ast.BinaryExpr:
			if value.Op == token.ADD {
				return stringValue(value.X) + stringValue(value.Y)
			}
		case *ast.ParenExpr:
			return stringValue(value.X)
		}
		return ""
	}
	var forbidden []string
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || (selector.Sel.Name != "recordDiagnostic" && selector.Sel.Name != "appendDiagnostic" && selector.Sel.Name != "appendDiagnosticEvent") {
			return true
		}
		for _, arg := range call.Args {
			ast.Inspect(arg, func(n ast.Node) bool {
				pair, ok := n.(*ast.KeyValueExpr)
				if !ok {
					return true
				}
				key := strings.ToLower(strings.ReplaceAll(stringValue(pair.Key), "_", ""))
				switch key {
				case "puuid", "accounthash", "accountid", "summonerid", "requestedparticipantid", "eventparticipantids":
					forbidden = append(forbidden, key)
				}
				return true
			})
		}
		return true
	})
	return forbidden, nil
}

func TestR86DiagnosticEmittersAreDiscoveredAcrossSourceFiles(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		fields, err := forbiddenDiagnosticKeys(string(data))
		if err != nil {
			t.Fatal(err)
		}
		if len(fields) > 0 {
			t.Errorf("%s emits stable identifiers: %v", file, fields)
		}
	}
	// A new file was outside the old four-file guard. Keeping the old sources
	// unchanged cannot conceal either a direct or independently concatenated key.
	for _, key := range []string{`"puuid"`, `"pu"+"uid"`} {
		fixture := `package main; func newEmitter(a *app) { a.recordDiagnostic(map[string]any{` + key + `: "private"}) }`
		fields, err := forbiddenDiagnosticKeys(fixture)
		if err != nil || len(fields) != 1 {
			t.Fatalf("new-file bypass survived: %v %v", fields, err)
		}
	}
}
