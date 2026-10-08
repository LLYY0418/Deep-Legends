package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

func TestRiotKeyDecryptionRejectsMissingAndCorruptCiphertext(t *testing.T) {
	ciphertext, err := encryptRiotKey("RGAPI-test-key")
	if err != nil {
		t.Fatalf("encryptRiotKey: %v", err)
	}
	for _, test := range []struct {
		ciphertext string
		want       bool
	}{{ciphertext, true}, {"", false}, {"not-a-valid-ciphertext", false}} {
		plain, err := decryptRiotKeyCipher(test.ciphertext)
		if valid := err == nil && strings.TrimSpace(plain) != ""; valid != test.want {
			t.Fatalf("decryption validity = %v, want %v", valid, test.want)
		}
	}
}

// Guard the entire startup routing prefix, not a helper name anywhere in the file.
// A new early route must be reviewed, even when the old guard remains as dead code.
func riotSelfCheckPrefix(source string) (string, error) {
	file, err := parser.ParseFile(token.NewFileSet(), "main.go", source, 0)
	if err != nil {
		return "", err
	}
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Name.Name != "main" {
			continue
		}
		for index, statement := range function.Body.List {
			branch, ok := statement.(*ast.IfStmt)
			if !ok {
				continue
			}
			dereference, ok := branch.Cond.(*ast.StarExpr)
			if !ok {
				continue
			}
			identifier, ok := dereference.X.(*ast.Ident)
			if !ok || identifier.Name != "selfCheckRiotKey" {
				continue
			}
			var output bytes.Buffer
			err := format.Node(&output, token.NewFileSet(), &ast.BlockStmt{List: function.Body.List[:index+1]})
			return output.String(), err
		}
	}
	return "", fmt.Errorf("missing self-check route")
}

func TestSelfCheckRiotKeyUsesEmbeddedGuard(t *testing.T) {
	source, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	expected, err := os.ReadFile("testdata/riot-self-check-prefix.go.txt")
	if err != nil {
		t.Fatal(err)
	}
	want, err := riotSelfCheckPrefix(string(expected))
	if err != nil {
		t.Fatal(err)
	}
	check := func(source string) bool {
		actual, err := riotSelfCheckPrefix(source)
		return err == nil && actual == want
	}
	if !check(string(source)) {
		t.Fatal("self-check startup route changed or bypassed; review the full AST prefix")
	}
	mutants := []string{
		strings.Replace(string(source), "if riotEmbeddedKey() == \"\" {", "return // if riotEmbeddedKey() == \"\"\n if false {", 1),
		strings.Replace(string(source), "if *selfCheckRiotKey {", "if *selfCheckRiotKey { return }\n if *selfCheckRiotKey {", 1),
		strings.Replace(string(source), "flag.Parse()", "flag.Parse(); bypassSelfCheck()", 1),
	}
	for index, mutant := range mutants {
		if !strings.Contains(mutant, "riotEmbeddedKey()") {
			t.Fatal("mutation no longer demonstrates old guard weakness")
		}
		if check(mutant) {
			t.Fatalf("bypass mutation %d escaped AST guard", index)
		}
	}
}
