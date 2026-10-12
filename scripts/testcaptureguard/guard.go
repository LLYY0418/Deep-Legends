// Package testcaptureguard conservatively audits writes captured by test callbacks.
// It checks syntax and synchronization shape, not arbitrary callee behavior;
// race sweeps remain required for dynamic ownership and indirect method effects.
package testcaptureguard

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
)

type Finding struct {
	File   string `json:"file"`
	Line   int    `json:"line"`
	Target string `json:"target"`
	Kind   string `json:"kind"`
	Reason string `json:"reason,omitempty"`
}
type Report struct {
	Files      int       `json:"files"`
	Callbacks  int       `json:"callbacks"`
	Issues     []Finding `json:"issues"`
	Protected  []Finding `json:"protected"`
	Whitelists []Finding `json:"whitelists"`
}
type fileAudit struct {
	fset    *token.FileSet
	file    *ast.File
	parents map[ast.Node]ast.Node
	imports map[string]string
	types   map[string]*ast.StructType
	funcs   map[*ast.Object]*ast.FuncLit
	async   map[*ast.FuncLit]bool
	seen    map[string]bool
	report  *Report
}

func Analyze(name string, source []byte) (Report, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, name, source, parser.ParseComments)
	if err != nil {
		return Report{}, err
	}
	r := Report{Files: 1, Issues: []Finding{}, Protected: []Finding{}, Whitelists: []Finding{}}
	a := &fileAudit{fset: fset, file: file, parents: map[ast.Node]ast.Node{}, imports: map[string]string{}, types: map[string]*ast.StructType{}, funcs: map[*ast.Object]*ast.FuncLit{}, async: map[*ast.FuncLit]bool{}, seen: map[string]bool{}, report: &r}
	for _, i := range file.Imports {
		p, _ := strconv.Unquote(i.Path.Value)
		name := filepath.Base(p)
		if i.Name != nil {
			name = i.Name.Name
		}
		a.imports[name] = p
	}
	stack := []ast.Node{}
	ast.Inspect(file, func(n ast.Node) bool {
		if n == nil {
			stack = stack[:len(stack)-1]
			return true
		}
		if len(stack) > 0 {
			a.parents[n] = stack[len(stack)-1]
		}
		stack = append(stack, n)
		if t, ok := n.(*ast.TypeSpec); ok {
			if s, ok := t.Type.(*ast.StructType); ok {
				a.types[t.Name.Name] = s
			}
		}
		return true
	})
	ast.Inspect(file, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.AssignStmt:
			for i, rhs := range n.Rhs {
				if f, ok := rhs.(*ast.FuncLit); ok && i < len(n.Lhs) {
					if id, ok := n.Lhs[i].(*ast.Ident); ok && id.Obj != nil {
						a.funcs[id.Obj] = f
					} else {
						a.async[f] = true
					}
				}
			}
		case *ast.ValueSpec:
			for i, v := range n.Values {
				if f, ok := v.(*ast.FuncLit); ok && i < len(n.Names) {
					a.funcs[n.Names[i].Obj] = f
				}
			}
		case *ast.ReturnStmt:
			for _, v := range n.Results {
				if f, ok := v.(*ast.FuncLit); ok {
					a.async[f] = true
				}
			}
		case *ast.KeyValueExpr:
			if f, ok := n.Value.(*ast.FuncLit); ok {
				a.async[f] = true
			}
		}
		return true
	})
	ast.Inspect(file, func(n ast.Node) bool {
		if g, ok := n.(*ast.GoStmt); ok {
			a.mark(g.Call.Fun)
		}
		if call, ok := n.(*ast.CallExpr); ok && !a.synchronous(call) {
			for _, arg := range call.Args {
				a.mark(arg)
			}
		}
		return true
	})
	ast.Inspect(file, func(n ast.Node) bool {
		f, ok := n.(*ast.FuncLit)
		if !ok {
			return true
		}
		if !a.async[f] {
			for parent := a.parents[f]; parent != nil; parent = a.parents[parent] {
				if outer, ok := parent.(*ast.FuncLit); ok && a.async[outer] {
					a.async[f] = true
					break
				}
			}
		}
		if a.async[f] {
			r.Callbacks++
			a.check(f)
		}
		return true
	})
	return r, nil
}

func (a *fileAudit) mark(e ast.Expr) {
	switch e := e.(type) {
	case *ast.FuncLit:
		a.async[e] = true
	case *ast.Ident:
		if f := a.funcs[e.Obj]; f != nil {
			a.async[f] = true
		}
	case *ast.ParenExpr:
		a.mark(e.X)
	}
}
func (a *fileAudit) synchronous(call *ast.CallExpr) bool {
	s, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	if id, ok := s.X.(*ast.Ident); ok {
		p := a.imports[id.Name]
		if p == "go/ast" && (s.Sel.Name == "Inspect" || s.Sel.Name == "Walk") {
			return true
		}
		if p == "path/filepath" && (s.Sel.Name == "Walk" || s.Sel.Name == "WalkDir") {
			return true
		}
		if p == "sort" && (s.Sel.Name == "Slice" || s.Sel.Name == "SliceStable" || s.Sel.Name == "Search") {
			return true
		}
		if s.Sel.Name == "Run" && a.typeName(a.objectType(id.Obj)) == "testing.T" {
			parallel := false
			for _, arg := range call.Args {
				if f, ok := arg.(*ast.FuncLit); ok {
					ast.Inspect(f.Body, func(n ast.Node) bool {
						if c, ok := n.(*ast.CallExpr); ok {
							if s, ok := c.Fun.(*ast.SelectorExpr); ok && s.Sel.Name == "Parallel" {
								parallel = true
							}
						}
						return true
					})
				}
			}
			return !parallel
		}
	}
	return false
}
func (a *fileAudit) objectType(o *ast.Object) ast.Expr {
	if o == nil {
		return nil
	}
	switch d := o.Decl.(type) {
	case *ast.Field:
		return d.Type
	case *ast.ValueSpec:
		if d.Type != nil {
			return d.Type
		}
		for i, n := range d.Names {
			if n.Obj == o && i < len(d.Values) {
				return a.valueType(d.Values[i])
			}
		}
	case *ast.AssignStmt:
		for i, n := range d.Lhs {
			if id, ok := n.(*ast.Ident); ok && id.Obj == o && i < len(d.Rhs) {
				return a.valueType(d.Rhs[i])
			}
		}
	}
	return nil
}
func (a *fileAudit) valueType(e ast.Expr) ast.Expr {
	switch e := e.(type) {
	case *ast.CompositeLit:
		return e.Type
	case *ast.UnaryExpr:
		return a.valueType(e.X)
	case *ast.CallExpr:
		if id, ok := e.Fun.(*ast.Ident); ok && id.Name == "new" && len(e.Args) == 1 {
			return e.Args[0]
		}
	case *ast.Ident:
		return a.objectType(e.Obj)
	}
	return nil
}
func (a *fileAudit) typeName(e ast.Expr) string {
	switch e := e.(type) {
	case *ast.StarExpr:
		return a.typeName(e.X)
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		if id, ok := e.X.(*ast.Ident); ok {
			return a.imports[id.Name] + "." + e.Sel.Name
		}
	}
	return ""
}
func (a *fileAudit) expressionType(e ast.Expr) ast.Expr {
	switch e := e.(type) {
	case *ast.Ident:
		return a.objectType(e.Obj)
	case *ast.SelectorExpr:
		if s := a.types[a.typeName(a.expressionType(e.X))]; s != nil {
			for _, f := range s.Fields.List {
				for _, n := range f.Names {
					if n.Name == e.Sel.Name {
						return f.Type
					}
				}
			}
		}
	}
	return nil
}
func (a *fileAudit) mutex(e ast.Expr) bool {
	t := a.typeName(a.expressionType(e))
	return t == "sync.Mutex" || t == "sync.RWMutex"
}
func (a *fileAudit) text(n ast.Node) string {
	var b bytes.Buffer
	printer.Fprint(&b, a.fset, n)
	return b.String()
}
func root(e ast.Expr) *ast.Ident {
	switch e := e.(type) {
	case *ast.Ident:
		return e
	case *ast.SelectorExpr:
		return root(e.X)
	case *ast.IndexExpr:
		return root(e.X)
	case *ast.StarExpr:
		return root(e.X)
	case *ast.ParenExpr:
		return root(e.X)
	}
	return nil
}
func (a *fileAudit) captured(e ast.Expr, f *ast.FuncLit) bool {
	id := root(e)
	if id == nil || id.Name == "_" || id.Obj == nil {
		return false
	}
	if id.Obj.Kind != ast.Var {
		return false
	}
	p := id.Obj.Pos()
	return p < f.Pos() || p > f.End()
}
func (a *fileAudit) lockHeld(n ast.Node, f *ast.FuncLit) bool {
	held := map[string]bool{}
	var blocks []*ast.BlockStmt
	for p := a.parents[n]; p != nil; p = a.parents[p] {
		if b, ok := p.(*ast.BlockStmt); ok {
			blocks = append(blocks, b)
		}
		if p == f {
			break
		}
	}
	for i := len(blocks) - 1; i >= 0; i-- {
		for _, s := range blocks[i].List {
			if s.Pos() >= n.Pos() || s.End() >= n.End() {
				break
			}
			if e, ok := s.(*ast.ExprStmt); ok {
				if c, ok := e.X.(*ast.CallExpr); ok {
					if method, ok := c.Fun.(*ast.SelectorExpr); ok && a.mutex(method.X) {
						k := a.text(method.X)
						if method.Sel.Name == "Lock" {
							held[k] = true
						}
						if method.Sel.Name == "Unlock" {
							delete(held, k)
						}
						continue
					}
				}
			}
			// A conditional unlock can invalidate an outer lock. Acquisition
			// inside an earlier branch never proves a lock on every later path.
			ast.Inspect(s, func(n ast.Node) bool {
				switch n.(type) {
				case *ast.FuncLit, *ast.DeferStmt, *ast.GoStmt:
					return false
				}
				if c, ok := n.(*ast.CallExpr); ok {
					if method, ok := c.Fun.(*ast.SelectorExpr); ok && method.Sel.Name == "Unlock" {
						delete(held, a.text(method.X))
					}
				}
				return true
			})
		}
	}
	return len(held) > 0
}
func (a *fileAudit) exemption(line int) string {
	for _, g := range a.file.Comments {
		for _, c := range g.List {
			l := a.fset.Position(c.Pos()).Line
			if l != line && l != line-1 {
				continue
			}
			if i := strings.Index(c.Text, "r276-capture-safe:"); i >= 0 {
				reason := strings.TrimSpace(strings.TrimSuffix(c.Text[i+len("r276-capture-safe:"):], "*/"))
				if len(reason) >= 16 {
					return reason
				}
			}
		}
	}
	return ""
}
func (a *fileAudit) finding(n ast.Node, e ast.Expr, f *ast.FuncLit, kind string) {
	if !a.captured(e, f) {
		return
	}
	p := a.fset.Position(n.Pos())
	target := a.text(e)
	key := fmt.Sprintf("%s:%d:%s", p.Filename, p.Line, target)
	if a.seen[key] {
		return
	}
	a.seen[key] = true
	x := Finding{File: p.Filename, Line: p.Line, Target: target, Kind: kind}
	if reason := a.exemption(p.Line); reason != "" {
		x.Reason = reason
		a.report.Whitelists = append(a.report.Whitelists, x)
		return
	}
	if a.lockHeld(n, f) {
		x.Reason = "write occurs while an identified sync.Mutex/RWMutex exclusive lock is held"
		a.report.Protected = append(a.report.Protected, x)
		return
	}
	a.report.Issues = append(a.report.Issues, x)
}
func (a *fileAudit) check(f *ast.FuncLit) {
	ast.Inspect(f.Body, func(n ast.Node) bool {
		if _, ok := n.(*ast.FuncLit); ok {
			return false
		}
		switch n := n.(type) {
		case *ast.AssignStmt:
			for _, lhs := range n.Lhs {
				a.finding(n, lhs, f, "assignment")
			}
		case *ast.IncDecStmt:
			a.finding(n, n.X, f, "increment")
		case *ast.CallExpr:
			if id, ok := n.Fun.(*ast.Ident); ok && len(n.Args) > 0 && (id.Name == "append" || id.Name == "copy" || id.Name == "delete" || id.Name == "clear") {
				a.finding(n, n.Args[0], f, "builtin-"+id.Name)
			}
		}
		return true
	})
}
