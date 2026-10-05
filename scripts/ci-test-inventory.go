// R225: parse test declarations without depending on the host's GOOS.
package main

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type testDeclaration struct {
	Name string `json:"name"`
	File string `json:"file"`
}

func main() {
	var tests []testDeclaration
	for _, root := range os.Args[1:] {
		err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if path != root && entry.IsDir() && (strings.HasPrefix(entry.Name(), ".") || strings.HasPrefix(entry.Name(), "_") || entry.Name() == "node_modules" || entry.Name() == "vendor" || entry.Name() == "testdata") {
				return filepath.SkipDir
			}
			if path != root && entry.IsDir() {
				if _, err := os.Stat(filepath.Join(path, "go.mod")); err == nil {
					return filepath.SkipDir
				}
			}
			if !strings.HasSuffix(path, "_test.go") {
				return nil
			}
			file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
			if err != nil {
				return err
			}
			aliases := map[string]bool{}
			for _, imp := range file.Imports {
				value, _ := strconv.Unquote(imp.Path.Value)
				if value == "testing" {
					alias := "testing"
					if imp.Name != nil {
						alias = imp.Name.Name
					}
					aliases[alias] = true
				}
			}
			for _, declaration := range file.Decls {
				fn, ok := declaration.(*ast.FuncDecl)
				if !ok || fn.Recv != nil || !strings.HasPrefix(fn.Name.Name, "Test") || fn.Type.Params.NumFields() != 1 {
					continue
				}
				pointer, ok := fn.Type.Params.List[0].Type.(*ast.StarExpr)
				if !ok {
					continue
				}
				selector, ok := pointer.X.(*ast.SelectorExpr)
				if !ok || selector.Sel.Name != "T" {
					continue
				}
				alias, ok := selector.X.(*ast.Ident)
				if ok && aliases[alias.Name] {
					tests = append(tests, testDeclaration{fn.Name.Name, filepath.ToSlash(path)})
				}
			}
			return nil
		})
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	if err := json.NewEncoder(os.Stdout).Encode(tests); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
