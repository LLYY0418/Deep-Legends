package main

import (
	"go/ast"
	"go/constant"
	"go/parser"
	"go/token"
	"go/types"
	"strconv"
	"strings"
	"testing"
)

// This is a conservative, single-file syntax/data-flow guard, not a proof of
// semantic equivalence. Covered: comparisons/switches; queue and constant alias
// chains; numeric casts and arithmetic; numeric strings and stringification;
// call/receiver wrappers; standard equality/comparison helpers (also aliased).
// R90 also covers literal map/array/slice indexing and local collection aliases,
// their range keys/values, type assertions (including comma-ok/type switches),
// address/dereference aliases, captured closure variables, local const/iota
// groups, and slices.Contains/Index/BinarySearch (including generic aliases).
// Fields and bare identifiers named QueueID/queueID are conservative roots;
// bindings distinguish aliases and their local shadows. An unrelated variable
// literally named queueID is also treated as a root (no type/body proof).
//
// R90B additionally models sends/receives on the same local channel binding,
// locally resolved struct-literal fields (keyed or positional), the first result
// of multi-value calls, standard endian PutUint16/32/64 writes to named buffers,
// and same-file package-level func[T comparable](T, T) bool calls/aliases.
//
// Limits: map/slice IndexExpr and RangeStmt tracking require a literal source
// in this file, possibly through identifier aliases, slices or unary wrappers.
// General index/field/pointer writes, append/copy, function-returned collection
// contents, channel range and range-function sources are not modeled. Channel
// sends propagate to the same identifier and its forward aliases; writes through
// an alias do not flow back to the original binding, and goroutine ordering or
// channels passed between functions are not analyzed.
// Struct provenance is keyed by locally resolved type and field, not instance;
// different fields/types stay separate, but mixed instances merge facts. This
// may over-report unrelated instances or hide a queue-vs-constant comparison
// when both instances' same field is conservatively treated as queue-sourced.
// Nested fields work only where go/types can resolve the local selections;
// opaque/imported types and arbitrary heap aliases remain outside coverage.
// Previously bindValues dropped non-comma-ok multi-return calls such as
// b, err := json.Marshal(q). It now propagates argument provenance to the FIRST
// result only (including 3+ results), never the error/status or later results.
// A locally resolved struct return is metadata, not a scalar queue alias; its
// fields rely on the separate literal/selection model. Callee-populated fields
// without a locally visible source are not recovered from arguments alone.
// A queue returned in a later slot, or from a callee without queue arguments,
// is still invisible: no callee return/body analysis is performed.
// Parameter-side-effect writes are modeled ONLY for encoding/binary's
// BigEndian/LittleEndian/NativeEndian.PutUint16/32/64(buffer, value), with a
// named buffer and known value provenance. Custom encoder functions, writes
// through slice expressions/fields, AppendUint/Write, and arbitrary buffer
// mutation or binary decoding are not modeled. bytes.Equal/Compare reuse the
// resulting queue/numeric provenance; encoding equivalence itself is not proven.
// The generic signature rule accepts exactly one explicit comparable parameter,
// two parameters of that type and one bool result declared in this file. It
// flags queue arguments conservatively without inspecting the function body or
// requiring a literal counterpart; unrelated generic identities may need review.
// Constraint aliases, helpers declared in other files/packages, method wrappers,
// reflection-built calls and custom membership helpers are not analyzed.
// TypeAssertExpr preserves a known source, not one lost at the above boundaries.
// Direct pointer aliases and closure captures do not constitute general heap or
// interprocedural analysis. Local const/iota and field resolution use go/types
// on resolvable parts of this file, with no package imports; missing sibling
// declarations/imported constants keep the limited syntactic fallback.
// Unknown table indexing does not propagate its index's queue taint:
// queue_groups.go owns registered queue classification outside scanned files.
// Wrapper calls conservatively propagate their arguments; assignments are
// flow-insensitive, so ambiguous reassignments may require a manual review.
func arenaLiteralComparisons(name string, source any) []token.Pos {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, name, source, 0)
	if err != nil {
		panic(err)
	}
	// Partial checking resolves local const groups using Go's own iota and
	// inheritance semantics. Missing sibling declarations/imports are expected:
	// their errors must not prevent the independent syntactic checks below.
	typeInfo := &types.Info{Types: map[ast.Expr]types.TypeAndValue{}, Selections: map[*ast.SelectorExpr]*types.Selection{}}
	config := types.Config{Error: func(error) {}}
	_, _ = config.Check("queueguard", fset, []*ast.File{f}, typeInfo)
	aliases := map[*ast.Object]bool{}
	numericAliases := map[*ast.Object]bool{}
	queueFields := map[types.Object]bool{}
	numericFields := map[types.Object]bool{}
	collections := map[*ast.Object]queueGuardCollection{}
	values := map[*ast.Object]constant.Value{}
	functions := map[*ast.Object]string{}
	imports := map[string]string{}
	for _, declaration := range f.Decls {
		if fn, ok := declaration.(*ast.FuncDecl); ok && queueGuardGenericComparator(fn) {
			functions[fn.Name.Obj] = "queueguard.generic-comparison"
		}
	}
	for _, spec := range f.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			continue
		}
		alias := path[strings.LastIndex(path, "/")+1:]
		if spec.Name != nil {
			alias = spec.Name.Name
		}
		imports[alias] = path
	}
	var functionName func(ast.Expr) string
	functionName = func(e ast.Expr) string {
		switch e := e.(type) {
		case *ast.ParenExpr:
			return functionName(e.X)
		case *ast.IndexExpr:
			return functionName(e.X) // Explicit single-argument instantiation.
		case *ast.IndexListExpr:
			return functionName(e.X) // e.g. slices.Contains[[]int64, int64].
		case *ast.Ident:
			return functions[e.Obj]
		case *ast.SelectorExpr:
			if pkg, ok := e.X.(*ast.Ident); ok && pkg.Obj == nil && imports[pkg.Name] != "" {
				return imports[pkg.Name] + "." + e.Sel.Name
			}
			if receiver := functionName(e.X); receiver != "" {
				return receiver + "." + e.Sel.Name
			}
		}
		return ""
	}
	// Formatting metadata (base 10, format strings) is not a numeric source.
	// Both constant evaluation and range/alias propagation follow the value arg.
	valueArgument := func(call *ast.CallExpr) ast.Expr {
		switch functionName(call.Fun) {
		case "strconv.FormatInt", "strconv.FormatUint":
			if len(call.Args) == 2 {
				return call.Args[0]
			}
		case "fmt.Sprintf":
			if len(call.Args) == 2 {
				return call.Args[1]
			}
		default:
			if len(call.Args) == 1 {
				return call.Args[0]
			}
		}
		return nil
	}
	var isQueue, isInteger func(ast.Expr) bool
	var collectionOf func(ast.Expr) queueGuardCollection
	isQueue = func(e ast.Expr) bool {
		switch e := e.(type) {
		case *ast.ParenExpr:
			return isQueue(e.X)
		case *ast.CallExpr:
			// A classifier's returned metadata is not the original numeric ID.
			// Track fields separately instead of tainting the whole struct and
			// every downstream pool/length/candidate derived from that object.
			result := typeInfo.Types[e].Type
			if tuple, ok := result.(*types.Tuple); ok && tuple.Len() > 0 {
				result = tuple.At(0).Type()
			}
			if result != nil {
				if _, metadata := result.Underlying().(*types.Struct); metadata {
					return false
				}
			}
			for _, arg := range e.Args {
				if isQueue(arg) {
					return true
				}
			}
			if method, ok := e.Fun.(*ast.SelectorExpr); ok {
				return isQueue(method.X)
			}
		case *ast.UnaryExpr:
			return isQueue(e.X)
		case *ast.StarExpr:
			return isQueue(e.X)
		case *ast.TypeAssertExpr:
			return isQueue(e.X)
		case *ast.IndexExpr:
			return collectionOf(e.X)&queueGuardQueueValues != 0
		case *ast.BinaryExpr:
			if queueGuardArithmetic(e.Op) {
				return isQueue(e.X) || isQueue(e.Y)
			}
		case *ast.CompositeLit:
			for _, element := range e.Elts {
				if pair, ok := element.(*ast.KeyValueExpr); ok {
					element = pair.Value
				}
				if isQueue(element) {
					return true
				}
			}
		case *ast.SelectorExpr:
			if strings.EqualFold(e.Sel.Name, "queueID") {
				return true
			}
			if selection := typeInfo.Selections[e]; selection != nil {
				return queueFields[selection.Obj()]
			}
		case *ast.Ident:
			return strings.EqualFold(e.Name, "queueID") || e.Obj != nil && aliases[e.Obj]
		}
		return false
	}
	var constantValue func(ast.Expr) constant.Value
	constantValue = func(e ast.Expr) (value constant.Value) {
		// Invalid or unresolved constant arithmetic must not crash the guard.
		value = constant.MakeUnknown()
		defer func() {
			if recover() != nil {
				value = constant.MakeUnknown()
			}
		}()
		if typed, ok := typeInfo.Types[e]; ok && typed.Value != nil {
			return typed.Value
		}
		switch e := e.(type) {
		case *ast.ParenExpr:
			return constantValue(e.X)
		case *ast.Ident:
			if known := values[e.Obj]; known != nil {
				return known
			}
		case *ast.StarExpr:
			return constantValue(e.X)
		case *ast.TypeAssertExpr:
			return constantValue(e.X)
		case *ast.UnaryExpr:
			if e.Op == token.AND {
				return constantValue(e.X)
			}
			if e.Op == token.ADD || e.Op == token.SUB || e.Op == token.XOR {
				return constant.UnaryOp(e.Op, constantValue(e.X), 0)
			}
		case *ast.BinaryExpr:
			if queueGuardArithmetic(e.Op) {
				left, right := constantValue(e.X), constantValue(e.Y)
				if e.Op == token.SHL || e.Op == token.SHR {
					if shift, ok := constant.Uint64Val(right); ok && shift <= 64 {
						return constant.Shift(left, e.Op, uint(shift))
					}
				} else {
					return constant.BinaryOp(left, e.Op, right)
				}
			}
		case *ast.CallExpr:
			return constantValue(valueArgument(e))
		case *ast.BasicLit:
			return constant.MakeFromLiteral(e.Value, e.Kind, 0)
		}
		return value
	}
	isInteger = func(e ast.Expr) bool {
		if isQueue(e) {
			return false
		}
		if id, ok := e.(*ast.Ident); ok && numericAliases[id.Obj] {
			return true
		}
		switch e := e.(type) {
		case *ast.SelectorExpr:
			if selection := typeInfo.Selections[e]; selection != nil {
				return numericFields[selection.Obj()]
			}
		case *ast.ParenExpr:
			return isInteger(e.X)
		case *ast.StarExpr:
			return isInteger(e.X)
		case *ast.TypeAssertExpr:
			return isInteger(e.X)
		case *ast.CallExpr:
			return isInteger(valueArgument(e))
		case *ast.UnaryExpr:
			if e.Op == token.ARROW || e.Op == token.AND || e.Op == token.ADD || e.Op == token.SUB || e.Op == token.XOR {
				return isInteger(e.X)
			}
		case *ast.BinaryExpr:
			if queueGuardArithmetic(e.Op) && isInteger(e.X) && isInteger(e.Y) {
				return true
			}
		case *ast.IndexExpr:
			return collectionOf(e.X)&queueGuardNumericValues != 0
		}
		return queueGuardNumericConstant(constantValue(e))
	}
	collectionOf = func(e ast.Expr) queueGuardCollection {
		switch e := e.(type) {
		case *ast.Ident:
			return collections[e.Obj]
		case *ast.ParenExpr:
			return collectionOf(e.X)
		case *ast.SliceExpr:
			return collectionOf(e.X)
		case *ast.UnaryExpr:
			return collectionOf(e.X)
		case *ast.StarExpr:
			return collectionOf(e.X)
		case *ast.TypeAssertExpr:
			return collectionOf(e.X)
		case *ast.CompositeLit:
			kind := queueGuardCollectionType(e.Type)
			if kind == token.ILLEGAL {
				return 0
			}
			facts := queueGuardKnownCollection
			for _, element := range e.Elts {
				value := element
				if pair, ok := element.(*ast.KeyValueExpr); ok {
					value = pair.Value
					if isInteger(pair.Key) {
						facts |= queueGuardNumericKeys
					}
					if isQueue(pair.Key) {
						facts |= queueGuardQueueKeys
					}
				} else if kind == token.LBRACK {
					// Implicit array/slice indices are integer selectors too.
					facts |= queueGuardNumericKeys
				}
				if isInteger(value) {
					facts |= queueGuardNumericValues
				}
				if isQueue(value) {
					facts |= queueGuardQueueValues
				}
			}
			return facts
		}
		return 0
	}
	for changed := true; changed; {
		changed = false
		bindFacts := func(lhs ast.Expr, numeric, queue bool) {
			if id, ok := lhs.(*ast.Ident); ok && id.Obj != nil && id.Name != "_" {
				if numeric && !numericAliases[id.Obj] {
					numericAliases[id.Obj], changed = true, true
				}
				if queue && !aliases[id.Obj] {
					aliases[id.Obj], changed = true, true
				}
			}
		}
		bind := func(lhs ast.Expr, rhs ast.Expr) {
			bindFacts(lhs, isInteger(rhs), isQueue(rhs))
			if id, ok := lhs.(*ast.Ident); ok && id.Obj != nil && id.Name != "_" {
				if facts := collections[id.Obj] | collectionOf(rhs); facts != collections[id.Obj] {
					collections[id.Obj], changed = facts, true
				}
				if values[id.Obj] == nil {
					if value := constantValue(rhs); value.Kind() != constant.Unknown {
						values[id.Obj] = value
						changed = true
					}
				}
				if functions[id.Obj] == "" {
					if name := functionName(rhs); name != "" {
						functions[id.Obj] = name
						changed = true
					}
				}
			}
		}
		bindValues := func(lhs, rhs []ast.Expr) {
			if len(lhs) == len(rhs) {
				for i := range lhs {
					bind(lhs[i], rhs[i])
				}
			} else if len(lhs) == 2 && len(rhs) == 1 {
				// Only the first result is the asserted/indexed value; ok is a
				// boolean and must never inherit either provenance.
				switch rhs[0].(type) {
				case *ast.TypeAssertExpr, *ast.IndexExpr:
					bind(lhs[0], rhs[0])
				case *ast.UnaryExpr:
					if rhs[0].(*ast.UnaryExpr).Op == token.ARROW {
						bind(lhs[0], rhs[0])
					}
				case *ast.CallExpr:
					bind(lhs[0], rhs[0]) // First returned value, never the error/status.
				}
			} else if len(lhs) > 2 && len(rhs) == 1 {
				if _, ok := rhs[0].(*ast.CallExpr); ok {
					bind(lhs[0], rhs[0])
				}
			}
		}
		ast.Inspect(f, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.SendStmt:
				// Same channel binding, flow-insensitive: receiving by assignment
				// or select follows the existing UnaryExpr queue propagation.
				bindFacts(n.Chan, isInteger(n.Value), isQueue(n.Value))
			case *ast.CompositeLit:
				typed := typeInfo.Types[n].Type
				if typed == nil {
					break
				}
				fields, ok := typed.Underlying().(*types.Struct)
				if !ok {
					break
				}
				for index, element := range n.Elts {
					var field *types.Var
					if pair, ok := element.(*ast.KeyValueExpr); ok {
						if key, ok := pair.Key.(*ast.Ident); ok {
							for i := 0; i < fields.NumFields(); i++ {
								if candidate := fields.Field(i); candidate.Name() == key.Name {
									field = candidate
									break
								}
							}
						}
						element = pair.Value
					} else if index < fields.NumFields() {
						field = fields.Field(index)
					}
					if field == nil {
						continue
					}
					if queue := isQueue(element); queue && !queueFields[field] {
						queueFields[field], changed = true, true
					}
					if numeric := isInteger(element); numeric && !numericFields[field] {
						numericFields[field], changed = true, true
					}
				}
			case *ast.CallExpr:
				if len(n.Args) == 2 && queueGuardBinaryPut(functionName(n.Fun)) {
					bindFacts(n.Args[0], isInteger(n.Args[1]), isQueue(n.Args[1]))
				}
			case *ast.AssignStmt:
				bindValues(n.Lhs, n.Rhs)
			case *ast.ValueSpec:
				lhs := make([]ast.Expr, len(n.Names))
				for i := range n.Names {
					lhs[i] = n.Names[i]
				}
				bindValues(lhs, n.Values)
			case *ast.RangeStmt:
				facts := collectionOf(n.X)
				bindFacts(n.Key, facts&queueGuardNumericKeys != 0, facts&queueGuardQueueKeys != 0)
				bindFacts(n.Value, facts&queueGuardNumericValues != 0, facts&queueGuardQueueValues != 0)
			}
			return true
		})
	}
	var found []token.Pos
	ast.Inspect(f, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.IndexExpr:
			if isQueue(n.Index) && collectionOf(n.X)&queueGuardNumericKeys != 0 {
				found = append(found, n.Pos())
			}
		case *ast.BinaryExpr:
			switch n.Op {
			case token.EQL, token.NEQ, token.LSS, token.LEQ, token.GTR, token.GEQ:
				if isQueue(n.X) && isInteger(n.Y) || isQueue(n.Y) && isInteger(n.X) {
					found = append(found, n.Pos())
				}
			}
		case *ast.SwitchStmt:
			if isQueue(n.Tag) {
				for _, stmt := range n.Body.List {
					for _, value := range stmt.(*ast.CaseClause).List {
						if isInteger(value) {
							found = append(found, value.Pos())
						}
					}
				}
			}
		case *ast.CallExpr:
			switch functionName(n.Fun) {
			case "queueguard.generic-comparison":
				for _, argument := range n.Args {
					if isQueue(argument) {
						found = append(found, n.Pos())
						break
					}
				}
			case "slices.Contains", "slices.Index", "slices.BinarySearch":
				if len(n.Args) == 2 {
					facts := collectionOf(n.Args[0])
					if facts&queueGuardNumericValues != 0 && isQueue(n.Args[1]) || facts&queueGuardQueueValues != 0 && isInteger(n.Args[1]) {
						found = append(found, n.Pos())
					}
				}
			case "reflect.DeepEqual", "strings.EqualFold", "strings.Compare", "bytes.Equal", "bytes.Compare", "cmp.Compare":
				if len(n.Args) == 2 && (isQueue(n.Args[0]) && isInteger(n.Args[1]) || isQueue(n.Args[1]) && isInteger(n.Args[0])) {
					found = append(found, n.Pos())
				}
			}
		}
		return true
	})
	return found
}

// Bound the side-effect model to standard endian writes with a named buffer.
func queueGuardBinaryPut(name string) bool {
	for _, endian := range []string{"BigEndian", "LittleEndian", "NativeEndian"} {
		for _, width := range []string{"16", "32", "64"} {
			if name == "encoding/binary."+endian+".PutUint"+width {
				return true
			}
		}
	}
	return false
}

// Recognize a local package-level func[T comparable](T, T) bool, independent
// of its name/body; this is a conservative signature rule, not call-graph analysis.
func queueGuardGenericComparator(fn *ast.FuncDecl) bool {
	if fn.Recv != nil || fn.Type.TypeParams == nil || len(fn.Type.TypeParams.List) != 1 || fn.Type.Results == nil || len(fn.Type.Results.List) != 1 {
		return false
	}
	parameter := fn.Type.TypeParams.List[0]
	constraint, ok := parameter.Type.(*ast.Ident)
	if !ok || constraint.Name != "comparable" || len(parameter.Names) != 1 {
		return false
	}
	result := fn.Type.Results.List[0]
	resultType, ok := result.Type.(*ast.Ident)
	if !ok || resultType.Name != "bool" || len(result.Names) > 1 {
		return false
	}
	count := 0
	for _, field := range fn.Type.Params.List {
		kind, ok := field.Type.(*ast.Ident)
		if !ok || kind.Name != parameter.Names[0].Name {
			return false
		}
		if len(field.Names) == 0 {
			count++
		} else {
			count += len(field.Names)
		}
	}
	return count == 2
}

type queueGuardCollection uint8

const (
	queueGuardKnownCollection queueGuardCollection = 1 << iota
	queueGuardNumericKeys
	queueGuardNumericValues
	queueGuardQueueKeys
	queueGuardQueueValues
)

// Resolve local named/aliased collection types without following fields or
// imported declarations. Cyclic aliases in an invalid specimen cannot recurse.
func queueGuardCollectionType(expr ast.Expr) token.Token {
	seen := map[*ast.Object]bool{}
	for {
		switch e := expr.(type) {
		case *ast.MapType:
			return token.MAP
		case *ast.ArrayType:
			return token.LBRACK
		case *ast.ParenExpr:
			expr = e.X
		case *ast.Ident:
			if e.Obj == nil || seen[e.Obj] {
				return token.ILLEGAL
			}
			seen[e.Obj] = true
			decl, ok := e.Obj.Decl.(*ast.TypeSpec)
			if !ok {
				return token.ILLEGAL
			}
			expr = decl.Type
		default:
			return token.ILLEGAL
		}
	}
}

func queueGuardArithmetic(op token.Token) bool {
	switch op {
	case token.ADD, token.SUB, token.MUL, token.QUO, token.REM, token.SHL, token.SHR, token.AND, token.OR, token.XOR, token.AND_NOT:
		return true
	}
	return false
}

func queueGuardNumericConstant(value constant.Value) bool {
	if value.Kind() == constant.String {
		_, err := strconv.ParseInt(constant.StringVal(value), 10, 64)
		return err == nil
	}
	return constant.ToInt(value).Kind() == constant.Int
}

func TestR88QueueGuardMutations(t *testing.T) {
	for _, body := range []string{
		"return session.QueueID == 3110", "q := session.QueueID; return q == 1700",
		"switch session.QueueID { case 1700,1710: return true }; return false",
		"var q = session.QueueID; alias := q; return 0 >= alias",
	} {
		if len(arenaLiteralComparisons("mutant.go", "package main; func f(session struct{QueueID int})bool{"+body+"}")) == 0 {
			t.Fatal("missed mutation:", body)
		}
	}
	if len(arenaLiteralComparisons("valid.go", "package main; func f(session struct{QueueID int})bool{q := session.QueueID; _ = q; {q := 1; return q == 1}}")) != 0 {
		t.Fatal("shadowed unrelated variable rejected")
	}
}
