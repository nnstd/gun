package compiler

import (
	"fmt"
	"go/ast"
	"go/token"
	"strings"
	"unicode"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

func itoa(i int) string {
	return fmt.Sprintf("%d", i)
}

func ident(name string) *ast.Ident {
	return &ast.Ident{Name: name}
}

func basicLit(kind token.Token, value string) *ast.BasicLit {
	return &ast.BasicLit{Kind: kind, Value: value}
}

func stringLit(s string) *ast.BasicLit {
	return basicLit(token.STRING, `"`+s+`"`)
}

func intLit(s string) *ast.BasicLit {
	return basicLit(token.INT, s)
}

func floatLit(s string) *ast.BasicLit {
	return basicLit(token.FLOAT, s)
}

// jsvalueWrapLit wraps a Go AST expression as a *jsvalue.JSValue value.
// Literals get specific constructors (NewString, NewNumber, NewBool).
// Expressions already returning *JSValue (jsvalue.* calls, .Get(), etc.) pass through.
// Everything else is wrapped with jsvalue.From() as a safe fallback.
func jsvalueWrapLit(expr ast.Expr) ast.Expr {
	// Already a JSValue expression — pass through
	if isAlreadyJSValue(expr) {
		return expr
	}
	switch e := expr.(type) {
	case *ast.BasicLit:
		switch e.Kind {
		case token.STRING:
			return callExpr(selectorExpr(ident("jsvalue"), "NewString"), e)
		case token.INT:
			return callExpr(selectorExpr(ident("jsvalue"), "NewNumber"), callExpr(ident("float64"), e))
		case token.FLOAT:
			return callExpr(selectorExpr(ident("jsvalue"), "NewNumber"), e)
		}
	case *ast.UnaryExpr:
		// Handle negative literals: -2 → jsvalue.NewNumber(float64(-2))
		if e.Op == token.SUB {
			if lit, ok := e.X.(*ast.BasicLit); ok && (lit.Kind == token.INT || lit.Kind == token.FLOAT) {
				return callExpr(selectorExpr(ident("jsvalue"), "NewNumber"), callExpr(ident("float64"), e))
			}
		}
	case *ast.Ident:
		if e.Name == "true" || e.Name == "false" {
			return callExpr(selectorExpr(ident("jsvalue"), "NewBool"), e)
		}
		if e.Name == "nil" {
			return callExpr(selectorExpr(ident("jsvalue"), "NewNull"))
		}
	}
	// Unknown expression type — wrap with jsvalue.From() as safe fallback
	return callExpr(selectorExpr(ident("jsvalue"), "From"), expr)
}

// isAlreadyJSValue returns true if the expression is known to produce *jsvalue.JSValue.
func isAlreadyJSValue(expr ast.Expr) bool {
	switch e := expr.(type) {
	case *ast.CallExpr:
		// jsvalue.NewString(...), jsvalue.From(...), jsvalue.Slice(...), etc.
		if sel, ok := e.Fun.(*ast.SelectorExpr); ok {
			if id, ok := sel.X.(*ast.Ident); ok && id.Name == "jsvalue" {
				return true
			}
			// .Get(), .Index(), .Call() on JSValue receivers
			switch sel.Sel.Name {
			case "Get", "Index", "Call":
				return true
			}
		}
	case *ast.SelectorExpr:
		// obj.Field — could be JSValue property access
		return false
	}
	return false
}

func field(name string, typ ast.Expr) *ast.Field {
	f := &ast.Field{Type: typ}
	if name != "" {
		f.Names = []*ast.Ident{ident(name)}
	}
	return f
}

func fieldList(fields ...*ast.Field) *ast.FieldList {
	return &ast.FieldList{List: fields}
}

func blockStmt(stmts ...ast.Stmt) *ast.BlockStmt {
	return &ast.BlockStmt{List: stmts}
}

func callExpr(fun ast.Expr, args ...ast.Expr) *ast.CallExpr {
	return &ast.CallExpr{Fun: fun, Args: args}
}

func selectorExpr(x ast.Expr, sel string) *ast.SelectorExpr {
	return &ast.SelectorExpr{X: x, Sel: ident(sel)}
}

func assignDefine(lhs []ast.Expr, rhs []ast.Expr) *ast.AssignStmt {
	return &ast.AssignStmt{Lhs: lhs, Tok: token.DEFINE, Rhs: rhs}
}

func assignStmt(lhs []ast.Expr, rhs []ast.Expr) *ast.AssignStmt {
	return &ast.AssignStmt{Lhs: lhs, Tok: token.ASSIGN, Rhs: rhs}
}

func returnStmt(results ...ast.Expr) *ast.ReturnStmt {
	return &ast.ReturnStmt{Results: results}
}

func exprStmt(x ast.Expr) *ast.ExprStmt {
	return &ast.ExprStmt{X: x}
}


func importSpecAlias(path, alias string) *ast.ImportSpec {
	spec := &ast.ImportSpec{Path: stringLit(path)}
	if alias != "" {
		spec.Name = ident(alias)
	}
	return spec
}

func varDecl(name string, typ ast.Expr, value ast.Expr) *ast.GenDecl {
	spec := &ast.ValueSpec{
		Names: []*ast.Ident{ident(name)},
	}
	if typ != nil {
		spec.Type = typ
	}
	if value != nil {
		spec.Values = []ast.Expr{value}
	}
	return &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{spec}}
}

func typeDecl(name string, typ ast.Expr) *ast.GenDecl {
	return &ast.GenDecl{
		Tok: token.TYPE,
		Specs: []ast.Spec{
			&ast.TypeSpec{Name: ident(name), Type: typ},
		},
	}
}

func funcDecl(name string, params, results *ast.FieldList, body *ast.BlockStmt) *ast.FuncDecl {
	return &ast.FuncDecl{
		Name: ident(name),
		Type: &ast.FuncType{Params: params, Results: results},
		Body: body,
	}
}


func ptrType(x ast.Expr) *ast.StarExpr {
	return &ast.StarExpr{X: x}
}

// jsValuePtrType returns the AST expression for *jsvalue.JSValue.
func jsValuePtrType() ast.Expr {
	return ptrType(selectorExpr(ident("jsvalue"), "JSValue"))
}

// isJSValuePtrType reports whether an AST expression represents *jsvalue.JSValue.
func isJSValuePtrType(expr ast.Expr) bool {
	star, ok := expr.(*ast.StarExpr)
	if !ok {
		return false
	}
	sel, ok := star.X.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == "jsvalue" && sel.Sel.Name == "JSValue"
}

func sliceType(elt ast.Expr) *ast.ArrayType {
	return &ast.ArrayType{Elt: elt}
}


func compositeLit(typ ast.Expr, elts ...ast.Expr) *ast.CompositeLit {
	return &ast.CompositeLit{Type: typ, Elts: elts}
}

// hasReturnValue reports whether the block contains at least one return
// statement that carries a value.
// nodeHasReturnValue checks a tree-sitter node for return_statement nodes
// that have a value child (i.e. return something, not bare return).
// Only scans the immediate function body — does not descend into nested functions.
func nodeHasReturnValue(node *sitter.Node) bool {
	if node == nil {
		return false
	}
	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		kind := child.Kind()
		if kind == "return_statement" && child.NamedChildCount() > 0 {
			return true
		}
		// Don't descend into nested function bodies.
		if kind == "function_declaration" || kind == "arrow_function" || kind == "function_expression" || kind == "function" {
			continue
		}
		if nodeHasReturnValue(child) {
			return true
		}
	}
	return false
}

func hasReturnValue(body *ast.BlockStmt) bool {
	if body == nil {
		return false
	}
	return stmtsHaveReturn(body.List)
}

func stmtsHaveReturn(stmts []ast.Stmt) bool {
	for _, stmt := range stmts {
		switch s := stmt.(type) {
		case *ast.ReturnStmt:
			if len(s.Results) > 0 {
				return true
			}
		case *ast.IfStmt:
			if stmtsHaveReturn(s.Body.List) {
				return true
			}
			if es, ok := s.Else.(*ast.BlockStmt); ok {
				if stmtsHaveReturn(es.List) {
					return true
				}
			}
		case *ast.BlockStmt:
			if stmtsHaveReturn(s.List) {
				return true
			}
		case *ast.ForStmt:
			if s.Body != nil && stmtsHaveReturn(s.Body.List) {
				return true
			}
		case *ast.RangeStmt:
			if s.Body != nil && stmtsHaveReturn(s.Body.List) {
				return true
			}
		case *ast.SwitchStmt:
			if s.Body != nil && stmtsHaveReturn(s.Body.List) {
				return true
			}
		case *ast.CaseClause:
			if stmtsHaveReturn(s.Body) {
				return true
			}
		}
	}
	return false
}

// wrapReturnsWithJSValue rewrites every `return expr` inside body to
// `return jsvalue.NewString(expr)` for string literals,
// `return jsvalue.NewNumber(expr)` for numeric literals,
// `return jsvalue.NewBool(expr)` for boolean idents,
// and a generic call expression for everything else.
func wrapReturnsWithJSValue(body *ast.BlockStmt) {
	if body == nil {
		return
	}
	wrapReturnStmts(body.List)
}

// ensureTrailingReturn appends a zero-value return to the body if the last
// statement is not already a return. This prevents Go "missing return" errors
// for functions where not all code paths explicitly return.
func ensureTrailingReturn(body *ast.BlockStmt, results *ast.FieldList) {
	if body == nil || results == nil || len(results.List) == 0 {
		return
	}
	if len(body.List) > 0 {
		if _, ok := body.List[len(body.List)-1].(*ast.ReturnStmt); ok {
			return
		}
	}
	var zeros []ast.Expr
	for _, f := range results.List {
		zeros = append(zeros, zeroValueFor(f.Type))
	}
	body.List = append(body.List, returnStmt(zeros...))
}

// zeroValueFor returns the zero-value expression for a Go type expression.
func zeroValueFor(typ ast.Expr) ast.Expr {
	switch t := typ.(type) {
	case *ast.Ident:
		switch t.Name {
		case "string":
			return &ast.BasicLit{Kind: token.STRING, Value: `""`}
		case "bool":
			return ident("false")
		case "int", "float64", "int64", "int32", "byte", "rune":
			return &ast.BasicLit{Kind: token.INT, Value: "0"}
		}
	case *ast.StarExpr:
		return ident("nil")
	case *ast.ArrayType:
		return ident("nil")
	case *ast.MapType:
		return ident("nil")
	}
	return ident("nil")
}

func wrapReturnStmts(stmts []ast.Stmt) {
	for _, stmt := range stmts {
		switch s := stmt.(type) {
		case *ast.ReturnStmt:
			if len(s.Results) == 0 {
				// Bare return → return nil (for *jsvalue.JSValue return type)
				s.Results = []ast.Expr{ident("nil")}
			}
			for i, r := range s.Results {
				s.Results[i] = wrapExprWithJSValue(r)
			}
		case *ast.IfStmt:
			wrapReturnStmts(s.Body.List)
			if es, ok := s.Else.(*ast.BlockStmt); ok {
				wrapReturnStmts(es.List)
			} else if ei, ok := s.Else.(*ast.IfStmt); ok {
				wrapReturnStmts([]ast.Stmt{ei})
			}
		case *ast.BlockStmt:
			wrapReturnStmts(s.List)
		case *ast.ForStmt:
			if s.Body != nil {
				wrapReturnStmts(s.Body.List)
			}
		case *ast.RangeStmt:
			if s.Body != nil {
				wrapReturnStmts(s.Body.List)
			}
		case *ast.SwitchStmt:
			if s.Body != nil {
				wrapReturnStmts(s.Body.List)
			}
		case *ast.TypeSwitchStmt:
			if s.Body != nil {
				wrapReturnStmts(s.Body.List)
			}
		case *ast.CaseClause:
			wrapReturnStmts(s.Body)
		// Note: do NOT descend into DeferStmt — defer funcs have their own return types
		}
	}
}

func wrapExprWithJSValue(expr ast.Expr) ast.Expr {
	switch e := expr.(type) {
	case *ast.BasicLit:
		switch e.Kind {
		case token.STRING:
			return callExpr(selectorExpr(ident("jsvalue"), "NewString"), e)
		case token.INT, token.FLOAT:
			return callExpr(selectorExpr(ident("jsvalue"), "NewNumber"), callExpr(ident("float64"), e))
		}
	case *ast.Ident:
		switch e.Name {
		case "true", "false":
			return callExpr(selectorExpr(ident("jsvalue"), "NewBool"), e)
		case "nil":
			return callExpr(selectorExpr(ident("jsvalue"), "NewNull"))
		}
	}
	// Already a JSValue expression — pass through
	if isAlreadyJSValue(expr) {
		return expr
	}
	// Generic: wrap with jsvalue.From(expr) as fallback
	return callExpr(selectorExpr(ident("jsvalue"), "From"), expr)
}

// exprReferencesName checks if a Go AST expression references a standalone
// identifier with the given name (not as a selector field like pkg.Name).
// Used to detect self-referencing variables that would cause initialization cycles.
func exprReferencesName(expr ast.Expr, name string) bool {
	// Collect all idents that are selector Sel fields — these are NOT references
	selectorFields := make(map[*ast.Ident]bool)
	ast.Inspect(expr, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok {
			selectorFields[sel.Sel] = true
		}
		return true
	})

	found := false
	ast.Inspect(expr, func(n ast.Node) bool {
		if found {
			return false
		}
		if id, ok := n.(*ast.Ident); ok && id.Name == name && !selectorFields[id] {
			found = true
			return false
		}
		return true
	})
	return found
}

// fixInitCycles splits self-referencing package-level var declarations into
// forward declarations + init() assignments to avoid Go initialization cycles.
// e.g. var f = jsvalue.NewFunction(... f.Call() ...) → var f *jsvalue.JSValue + init() { f = ... }
func (t *Transformer) fixInitCycles(decls []ast.Decl) []ast.Decl {
	// Build reference graph: for each var, what other vars does it reference?
	type varInfo struct {
		name  string
		value ast.Expr
		decl  *ast.GenDecl
		spec  *ast.ValueSpec
	}
	var vars []varInfo
	allVarNames := make(map[string]bool)
	for _, d := range decls {
		if gd, ok := d.(*ast.GenDecl); ok && gd.Tok == token.VAR {
			for _, spec := range gd.Specs {
				if vs, ok := spec.(*ast.ValueSpec); ok && len(vs.Names) > 0 {
					allVarNames[vs.Names[0].Name] = true
					if len(vs.Values) > 0 {
						vars = append(vars, varInfo{vs.Names[0].Name, vs.Values[0], gd, vs})
					}
				}
			}
		}
	}

	// Detect vars in cycles: self-ref or mutual ref
	refs := make(map[string]map[string]bool) // name → set of referenced var names
	for _, v := range vars {
		r := make(map[string]bool)
		for otherName := range allVarNames {
			if exprReferencesName(v.value, otherName) {
				r[otherName] = true
			}
		}
		refs[v.name] = r
	}

	cyclicVars := make(map[string]bool)
	for _, v := range vars {
		// Self-reference
		if refs[v.name][v.name] {
			cyclicVars[v.name] = true
		}
		// Mutual reference: A→B and B→A
		for dep := range refs[v.name] {
			if dep != v.name && refs[dep][v.name] {
				cyclicVars[v.name] = true
				cyclicVars[dep] = true
			}
		}
	}

	var result []ast.Decl
	var initStmts []ast.Stmt

	for _, d := range decls {
		gd, ok := d.(*ast.GenDecl)
		if !ok || gd.Tok != token.VAR {
			result = append(result, d)
			continue
		}
		hasCycle := false
		for _, spec := range gd.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok || len(vs.Names) == 0 || len(vs.Values) == 0 {
				continue
			}
			if cyclicVars[vs.Names[0].Name] {
				hasCycle = true
				break
			}
		}
		if !hasCycle {
			result = append(result, d)
			continue
		}
		// Split: forward declare with type, assign in init()
		for _, spec := range gd.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok || len(vs.Names) == 0 {
				continue
			}
			name := vs.Names[0].Name
			if cyclicVars[name] && len(vs.Values) > 0 {
				// Forward declare with explicit type
				typ := vs.Type
				if typ == nil {
					typ = jsValuePtrType()
					t.addAliasedImport("github.com/nnstd/gun/runtime/jsvalue", "jsvalue")
				}
				result = append(result, varDecl(name, typ, nil))
				initStmts = append(initStmts, assignStmt(
					[]ast.Expr{ident(name)}, []ast.Expr{vs.Values[0]},
				))
			} else {
				// No cycle — keep as-is
				result = append(result, &ast.GenDecl{
					Tok:   token.VAR,
					Specs: []ast.Spec{vs},
				})
			}
		}
	}

	if len(initStmts) > 0 {
		initFn := funcDecl("init", fieldList(), nil, &ast.BlockStmt{List: initStmts})
		result = append(result, initFn)
	}

	return result
}

// funcDeclToJSValueVar converts a Go func declaration into a var declaration
// with jsvalue.NewFunction wrapping: func Foo(a, b) R { body } →
// var Foo = jsvalue.NewFunction(func(_args ...*jsvalue.JSValue) *jsvalue.JSValue { ... })
func (t *Transformer) funcDeclToJSValueVar(d *ast.FuncDecl) ast.Decl {
	// Extract param names from the func declaration
	var paramNames []string
	if d.Type.Params != nil {
		for _, f := range d.Type.Params.List {
			for _, n := range f.Names {
				paramNames = append(paramNames, n.Name)
			}
		}
	}

	fnLit := &ast.FuncLit{
		Type: d.Type,
		Body: d.Body,
	}

	jsVal := t.wrapFuncLitAsJSValue(fnLit, paramNames)
	return varDecl(d.Name.Name, nil, jsVal)
}

// wrapFuncLitAsJSValue converts a Go function literal into a jsvalue.NewFunction call.
// It wraps the original function in a variadic JSValue dispatcher that unpacks
// _args and calls the original, preserving its param handling (including
// destructuring params with defaults and variadic params).
func (t *Transformer) wrapFuncLitAsJSValue(fnLit *ast.FuncLit, paramNames []string) ast.Expr {
	t.addAliasedImport("github.com/nnstd/gun/runtime/jsvalue", "jsvalue")

	// Ensure return type is *jsvalue.JSValue and all returns are wrapped
	wrapReturnsWithJSValue(fnLit.Body)
	results := fieldList(field("", jsValuePtrType()))
	fnLit.Type.Results = results
	ensureTrailingReturn(fnLit.Body, results)

	// Build args to pass to the original function by unpacking _args.
	// Each original param gets _args[i]; variadic params get _args[i:]...
	var callArgs []ast.Expr
	paramIdx := 0
	if fnLit.Type.Params != nil {
		for _, f := range fnLit.Type.Params.List {
			_, isVariadic := f.Type.(*ast.Ellipsis)
			for range f.Names {
				if isVariadic {
					// Variadic param: pass remaining _args as spread slice
					callArgs = append(callArgs, &ast.SliceExpr{
						X:      ident("_args"),
						Low:    intLit(itoa(paramIdx)),
						Slice3: false,
					})
				} else {
					// Regular param: pass _args[i] or nil if missing
					callArgs = append(callArgs, &ast.CallExpr{
						Fun: &ast.FuncLit{
							Type: &ast.FuncType{
								Results: fieldList(field("", f.Type)),
							},
							Body: blockStmt(
								&ast.IfStmt{
									Cond: &ast.BinaryExpr{
										X:  callExpr(ident("len"), ident("_args")),
										Op: token.GTR,
										Y:  intLit(itoa(paramIdx)),
									},
									Body: blockStmt(returnStmt(&ast.IndexExpr{
										X: ident("_args"), Index: intLit(itoa(paramIdx)),
									})),
								},
								returnStmt(ident("nil")),
							),
						},
					})
				}
				paramIdx++
			}
		}
	}

	// Build wrapper: func(_args ...*jsvalue.JSValue) *jsvalue.JSValue { return inner(args...) }
	// If the last arg is a variadic spread, set Ellipsis on the CallExpr.
	hasVariadicSpread := false
	if fnLit.Type.Params != nil {
		for _, f := range fnLit.Type.Params.List {
			if _, ok := f.Type.(*ast.Ellipsis); ok {
				hasVariadicSpread = true
			}
		}
	}

	innerCall := callExpr(fnLit, callArgs...)
	if hasVariadicSpread {
		innerCall.Ellipsis = 1 // non-zero triggers "args..." syntax
	}
	wrapperBody := blockStmt(returnStmt(innerCall))

	variadicFn := &ast.FuncLit{
		Type: &ast.FuncType{
			Params: fieldList(&ast.Field{
				Names: []*ast.Ident{ident("_args")},
				Type:  &ast.Ellipsis{Elt: jsValuePtrType()},
			}),
			Results: results,
		},
		Body: wrapperBody,
	}

	return callExpr(selectorExpr(ident("jsvalue"), "NewFunction"), variadicFn)
}

// collectUsedIdents walks the Go AST declarations and returns the set of
// all identifier names that appear as the X in selector expressions (pkg.Symbol)
// or as standalone identifiers. Used to prune unused imports.
func collectUsedIdents(decls []ast.Decl) map[string]bool {
	used := make(map[string]bool)
	for _, d := range decls {
		ast.Inspect(d, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.SelectorExpr:
				if id, ok := node.X.(*ast.Ident); ok {
					used[id.Name] = true
				}
			case *ast.Ident:
				used[node.Name] = true
			}
			return true
		})
	}
	return used
}

// goKeywords is the set of Go reserved words that cannot be used as identifiers.
var goKeywords = map[string]bool{
	"break": true, "case": true, "chan": true, "const": true, "continue": true,
	"default": true, "defer": true, "else": true, "fallthrough": true, "for": true,
	"func": true, "go": true, "goto": true, "if": true, "import": true,
	"interface": true, "map": true, "package": true, "range": true, "return": true,
	"select": true, "struct": true, "switch": true, "type": true, "var": true,
}

// goBuiltins is the set of Go predeclared identifiers (types, constants, functions)
// that should not be shadowed by user variables.
var goBuiltins = map[string]bool{
	"string": true, "int": true, "int8": true, "int16": true, "int32": true, "int64": true,
	"uint": true, "uint8": true, "uint16": true, "uint32": true, "uint64": true,
	"float32": true, "float64": true, "complex64": true, "complex128": true,
	"bool": true, "byte": true, "rune": true, "error": true, "any": true, "comparable": true,
	"len": true, "cap": true, "make": true, "new": true, "append": true, "copy": true,
	"delete": true, "panic": true, "recover": true, "print": true, "println": true,
	"true": true, "false": true, "nil": true, "iota": true,
}

// extractParamNames extracts parameter names from a tree-sitter parameters node.
func extractParamNames(node *sitter.Node, source []byte) []string {
	if node == nil {
		return nil
	}
	var names []string
	for i := uint(0); i < node.NamedChildCount(); i++ {
		param := node.NamedChild(i)
		switch param.Kind() {
		case "required_parameter", "optional_parameter":
			nameNode := param.ChildByFieldName("pattern")
			if nameNode != nil && nameNode.Kind() == "identifier" {
				names = append(names, sanitizeIdent(nameNode.Utf8Text(source)))
			} else if nameNode != nil && nameNode.Kind() == "rest_pattern" {
				// ...args inside required_parameter
				for j := uint(0); j < nameNode.NamedChildCount(); j++ {
					if child := nameNode.NamedChild(j); child.Kind() == "identifier" {
						names = append(names, child.Utf8Text(source))
						break
					}
				}
			} else if nameNode != nil && (nameNode.Kind() == "object_pattern" || nameNode.Kind() == "array_pattern") {
				// Destructured param: {a, b} or [a, b] → synthetic _param{i}
				names = append(names, fmt.Sprintf("_param%d", i))
			}
		case "rest_parameter":
			nameNode := param.ChildByFieldName("pattern")
			if nameNode != nil {
				names = append(names, nameNode.Utf8Text(source))
			}
		case "identifier":
			names = append(names, param.Utf8Text(source))
		}
	}
	return names
}

// extractRestFlags returns a bool slice parallel to extractParamNames indicating
// which parameters are rest parameters (...args).
func extractRestFlags(node *sitter.Node, source []byte) []bool {
	if node == nil {
		return nil
	}
	var flags []bool
	for i := uint(0); i < node.NamedChildCount(); i++ {
		param := node.NamedChild(i)
		switch param.Kind() {
		case "required_parameter", "optional_parameter":
			nameNode := param.ChildByFieldName("pattern")
			if nameNode != nil && nameNode.Kind() == "rest_pattern" {
				flags = append(flags, true)
			} else {
				flags = append(flags, false)
			}
		case "rest_parameter":
			flags = append(flags, true)
		case "identifier":
			flags = append(flags, false)
		}
	}
	return flags
}

// extractParamInfo returns a map of parameter names to whether they have
// explicit type annotations. In all-JSValue mode, all params are untyped (false).
func extractParamInfo(node *sitter.Node, source []byte) map[string]bool {
	if node == nil {
		return nil
	}
	info := make(map[string]bool)
	for i := uint(0); i < node.NamedChildCount(); i++ {
		param := node.NamedChild(i)
		switch param.Kind() {
		case "required_parameter", "optional_parameter":
			nameNode := param.ChildByFieldName("pattern")
			if nameNode != nil && nameNode.Kind() == "identifier" {
				info[sanitizeIdent(nameNode.Utf8Text(source))] = false // all-JSValue: always untyped
			}
			if nameNode != nil && (nameNode.Kind() == "object_pattern" || nameNode.Kind() == "array_pattern") {
				info[fmt.Sprintf("_param%d", i)] = false
			}
		case "rest_parameter":
			nameNode := param.ChildByFieldName("pattern")
			if nameNode != nil {
				info[sanitizeIdent(nameNode.Utf8Text(source))] = false
			}
		case "identifier":
			info[param.Utf8Text(source)] = false
		}
	}
	return info
}

// nodeUsesThis checks if a tree-sitter node or any of its descendants reference 'this'.
// It stops at nested function boundaries (function/arrow_function) since those have their own 'this'.
func nodeUsesThis(node *sitter.Node) bool {
	if node == nil {
		return false
	}
	if node.Kind() == "this" {
		return true
	}
	// Don't descend into nested arrow functions or function expressions
	// (they have their own 'this' binding)
	if node.Kind() == "arrow_function" || node.Kind() == "function" || node.Kind() == "function_expression" {
		return false
	}
	for i := uint(0); i < node.ChildCount(); i++ {
		if nodeUsesThis(node.Child(i)) {
			return true
		}
	}
	return false
}

// isBoolExpr reports whether a Go AST expression is known to produce a bool.
func isBoolExpr(expr ast.Expr) bool {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name == "true" || e.Name == "false"
	case *ast.UnaryExpr:
		return e.Op == token.NOT
	case *ast.BinaryExpr:
		switch e.Op {
		case token.EQL, token.NEQ, token.LSS, token.GTR, token.LEQ, token.GEQ, token.LAND, token.LOR:
			return true
		}
	}
	return false
}

// isGoTypeName reports whether s is a valid Go type name for ternary IIFE return types.
func isGoTypeName(s string) bool {
	switch s {
	case "string", "bool", "int", "float64", "byte", "rune":
		return true
	}
	return false
}

// isNilIdent returns true if the expression is the identifier "nil".
func isNilIdent(expr ast.Expr) bool {
	id, ok := expr.(*ast.Ident)
	return ok && id.Name == "nil"
}


// sanitizeIdent makes a JS identifier safe for use as a Go identifier by
// replacing illegal characters and escaping reserved keywords.
func sanitizeIdent(name string) string {
	if strings.ContainsRune(name, '$') {
		name = strings.ReplaceAll(name, "$", "_")
	}
	if goKeywords[name] || goBuiltins[name] {
		return name + "_"
	}
	return name
}

// capitalize returns the string with the first letter uppercased (Go export convention).
func capitalize(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}


