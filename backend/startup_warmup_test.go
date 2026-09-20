package main

import (
	"context"
	"flag"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStartupWarmupHelper(t *testing.T) {
	if os.Getenv("R83_WARMUP_HELPER") != "1" {
		return
	}
	flag.CommandLine = flag.NewFlagSet("warmup-child", flag.ExitOnError)
	// If main reaches net.Listen it must fail, not briefly listen and then exit.
	os.Args = []string{os.Args[0], "--startup-warmup", "--listen", "r83-invalid-listen-address"}
	main()
	os.Exit(0)
}

func TestStartupWarmupProcessDoesNotInitializeServicesOrUserFiles(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable, "-test.run=^TestStartupWarmupHelper$")
	cmd.Env = append(os.Environ(), "R83_WARMUP_HELPER=1", "LOL_LOOT_DATA_DIR="+filepath.Join(root, "user-data"),
		"XDG_CACHE_HOME="+filepath.Join(root, "cache"), "APPDATA="+filepath.Join(root, "roaming"),
		"LOCALAPPDATA="+filepath.Join(root, "local"), "DEEP_LEGENDS_FEATURE_GATES_URL=")
	output, err := cmd.CombinedOutput()
	if err != nil || len(output) != 0 {
		t.Fatalf("warmup did not exit cleanly before listener initialization: %v %s", err, output)
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatalf("warmup created user data: %v %v", entries, err)
	}
}

func TestStartupWarmupReturnsImmediatelyAfterParsingBeforeAnyInitializer(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "main.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, declaration := range file.Decls {
		main, ok := declaration.(*ast.FuncDecl)
		if !ok || main.Name.Name != "main" {
			continue
		}
		for index, statement := range main.Body.List {
			call, ok := statement.(*ast.ExprStmt)
			if !ok {
				continue
			}
			expr, ok := call.X.(*ast.CallExpr)
			if !ok {
				continue
			}
			selector, ok := expr.Fun.(*ast.SelectorExpr)
			if !ok || selector.Sel.Name != "Parse" {
				continue
			}
			branch, ok := main.Body.List[index+1].(*ast.IfStmt)
			if !ok || branch.Else != nil || len(branch.Body.List) != 1 {
				t.Fatal("warmup must be first branch after flag.Parse")
			}
			condition, ok := branch.Cond.(*ast.StarExpr)
			if !ok {
				t.Fatal("warmup flag is ignored")
			}
			name, ok := condition.X.(*ast.Ident)
			if !ok || name.Name != "startupWarmup" {
				t.Fatal("wrong warmup flag")
			}
			if _, ok := branch.Body.List[0].(*ast.ReturnStmt); !ok {
				t.Fatal("warmup ran an initializer instead of returning")
			}
			return
		}
	}
	t.Fatal("missing early warmup return")
}

func TestStartupWarmupHasNoNewPackageInitializers(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, declaration := range file.Decls {
			if function, ok := declaration.(*ast.FuncDecl); ok && function.Name.Name == "init" {
				t.Fatalf("new package initializer must be audited for warmup side effects: %s", name)
			}
		}
	}
}
