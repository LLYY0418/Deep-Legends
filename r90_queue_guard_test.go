package main

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"testing"
)

// Unlike syntax-only mutations, every R90 specimen must also compile. The
// registry lives in a second file: the production guard scans champselect files,
// while queue_groups.go owns the numeric registration and is outside that scan.
func r90QueueGuardSource(t *testing.T, imports, body string) string {
	t.Helper()
	source := "package fixture\n" + imports + `
func check(session struct{QueueID, ChampionID int64}) bool {
` + body + "\n}"
	registry := `package fixture
var registeredQueueModeGroups = map[int64]string{1750: "arena"}
var registeredQueueProperties = map[int64]struct{Size int}{1750: {Size: 3}}
`
	fset := token.NewFileSet()
	var files []*ast.File
	for i, text := range []string{source, registry} {
		name := []string{"champselect_fixture.go", "queue_groups.go"}[i]
		file, err := parser.ParseFile(fset, name, text, 0)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, file)
	}
	config := types.Config{Importer: importer.Default()}
	if _, err := config.Check("fixture", fset, files, nil); err != nil {
		t.Fatalf("specimen must compile before checking the guard: %v\n%s", err, source)
	}
	return source
}

func TestR90QueueGuardRequired(t *testing.T) {
	for name, body := range map[string]string{
		"literal-map": `return map[int64]bool{1750:true, 1760:true}[session.QueueID]`,
		"handler-map": `handlers := map[int64]func() bool{1750:func()bool{return true}}; return handlers[session.QueueID]()`,
		"range-slice": `for _, v := range []int64{1700,1710} {if session.QueueID==v{return true}};return false`,
		"type-assert": `var v interface{}=session.QueueID; n:=v.(int64);return n==3110`,
	} {
		t.Run(name, func(t *testing.T) {
			source := r90QueueGuardSource(t, "", body)
			if len(arenaLiteralComparisons("champselect_fixture.go", source)) == 0 {
				t.Fatal("missed R90 queue literal:", name)
			}
		})
	}
}

func TestR90QueueGuardExtra(t *testing.T) {
	for _, tc := range []struct{ name, imports, body string }{
		{"pointer", "", `p:=&session.QueueID;return *p==3110`},
		{"closure-capture", "", `q:=session.QueueID; check:=func()bool{return q==1750};return check()`},
		{"type-switch", "", `var boxed any=session.QueueID;switch n:=boxed.(type){case int64:return n==1750};return false`},
		{"iota-inheritance", "", `const(base=iota+1700; next);return session.QueueID==next`},
		{"slice-membership", `import "slices"`, `return slices.Contains([]int64{1700,1710,1750},session.QueueID)`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := r90QueueGuardSource(t, tc.imports, tc.body)
			if len(arenaLiteralComparisons("champselect_fixture.go", source)) == 0 {
				t.Fatal("missed additional queue literal:", tc.name)
			}
		})
	}
}

func TestR90QueueGuardCombinations(t *testing.T) {
	for _, tc := range []struct{ name, imports, body string }{
		{"map-alias", "", `var codes=map[int64]bool{1750:true};alias:=codes;return alias[session.QueueID]`},
		{"map-comma-ok", "", `codes:=map[int64]struct{}{1750:{}};_,ok:=codes[session.QueueID];return ok`},
		{"map-named-type", "", `type targets map[int64]bool; codes:=targets{1750:true};return codes[session.QueueID]`},
		{"string-map", `import "strconv"`, `codes:=map[string]bool{"1750":true};return codes[strconv.FormatInt(session.QueueID,10)]`},
		{"literal-slice-index", "", `return []int64{1750,1760}[session.QueueID]>0`},
		{"sparse-array-index", "", `return [...]bool{1750:true}[session.QueueID]`},
		{"slice-alias-index", "", `codes:=[]int64{1750};alias:=codes;return alias[session.QueueID]>0`},
		{"slice-element-comparison", "", `codes:=[]int64{1750};return session.QueueID==codes[0]`},
		{"range-alias", "", `codes:=[]int64{1750};alias:=codes;for _,v:=range alias{if session.QueueID==v{return true}};return false`},
		{"range-assignment", "", `var v int64;for _,v=range []int64{1750}{if session.QueueID==v{return true}};return false`},
		{"range-map-key", "", `codes:=map[int64]bool{1750:true};for v:=range codes{if session.QueueID==v{return true}};return false`},
		{"range-map-value", "", `codes:=map[string]int64{"arena":1750};for _,v:=range codes{if session.QueueID==v{return true}};return false`},
		{"range-queue-element", "", `for _,v:=range []int64{session.QueueID}{if v==1750{return true}};return false`},
		{"range-cast-target", "", `for _,v:=range []int64{1750}{if session.QueueID==int64(v){return true}};return false`},
		{"range-string-target", `import "strconv"`, `for _,v:=range []int64{1750}{if strconv.FormatInt(session.QueueID,10)==strconv.FormatInt(v,10){return true}};return false`},
		{"range-arithmetic-target", "", `for _,v:=range []int64{1750}{if session.QueueID==v+0{return true}};return false`},
		{"boxed-slice-range", "", `var boxed any=[]int64{1750};for _,v:=range boxed.([]int64){if session.QueueID==v{return true}};return false`},
		{"queue-element-index", "", `codes:=[]int64{session.QueueID};return codes[0]==1750`},
		{"assert-comma-ok", "", `var boxed any=session.QueueID;n,ok:=boxed.(int64);return ok&&n==1750`},
		{"assert-var-comma-ok", "", `var boxed any=session.QueueID;var n,ok=boxed.(int64);return ok&&n==1750`},
		{"boxed-numeric", "", `var boxed any=int64(1750);n:=boxed.(int64);return session.QueueID==n`},
		{"numeric-pointer", "", `n:=int64(1750);p:=&n;return session.QueueID==*p`},
		{"inherited-constant", "", `const(base=1750; inherited);return session.QueueID==inherited`},
		{"iota-arithmetic", "", `const(base=1700+iota*50;next);return session.QueueID==next`},
		{"membership-alias", `import s "slices"`, `contains:=s.Contains[[]int64,int64];codes:=[]int64{1750};return contains(codes,session.QueueID)`},
		{"membership-index", `import "slices"`, `return slices.Index([]int64{1750},session.QueueID)>=0`},
		{"membership-search", `import "slices"`, `_,ok:=slices.BinarySearch([]int64{1750},session.QueueID);return ok`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := r90QueueGuardSource(t, tc.imports, tc.body)
			if len(arenaLiteralComparisons("champselect_fixture.go", source)) == 0 {
				t.Fatal("missed combined queue literal:", tc.name)
			}
		})
	}
}

func TestR90QueueGuardAllowsRegisteredAndUnrelated(t *testing.T) {
	for name, body := range map[string]string{
		"registry":               `return registeredQueueModeGroups[session.QueueID]=="arena"`,
		"registry-alias":         `table:=registeredQueueModeGroups;return table[session.QueueID]=="arena"`,
		"registry-result-number": `return registeredQueueProperties[session.QueueID].Size==3`,
		"registry-comma-ok":      `group,ok:=registeredQueueModeGroups[session.QueueID];return ok&&group=="arena"`,
		"unrelated-comparison":   `n:=int64(1750);return session.ChampionID==n`,
		"unrelated-map":          `champions:=map[int64]bool{1750:true};return champions[session.ChampionID]`,
		"unrelated-handler-map":  `handlers:=map[int64]func()bool{1750:func()bool{return true}};return handlers[session.ChampionID]()`,
		"unrelated-range":        `for _,v:=range []int64{1750}{if session.ChampionID==v{return true}};return false`,
		"range-shadow":           `q:=session.QueueID;_=q;for _,q:=range []int64{1750}{if session.ChampionID==q{return true}};return false`,
		"collection-shadow":      `codes:=map[int64]bool{1750:true};_=codes;{codes:=registeredQueueModeGroups;return codes[session.QueueID]=="arena"}`,
		"unrelated-assert":       `var boxed any=session.ChampionID;n:=boxed.(int64);return n==1750`,
		"unrelated-pointer":      `p:=&session.ChampionID;return *p==1750`,
		"map-nonnumeric-key":     `m:=map[string]int64{"arena":1750};return m["arena"]==1750`,
	} {
		t.Run(name, func(t *testing.T) {
			source := r90QueueGuardSource(t, "", body)
			if got := arenaLiteralComparisons("champselect_fixture.go", source); len(got) != 0 {
				t.Fatalf("legal code rejected: %s: %v", name, got)
			}
		})
	}
}
