package compiler

import (
	"fmt"
	"go/ast"
	"go/token"
	"strings"
	"unicode"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

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

func importSpec(path string) *ast.ImportSpec {
	return &ast.ImportSpec{Path: stringLit(path)}
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

func constDecl(name string, typ ast.Expr, value ast.Expr) *ast.GenDecl {
	spec := &ast.ValueSpec{
		Names: []*ast.Ident{ident(name)},
	}
	if typ != nil {
		spec.Type = typ
	}
	if value != nil {
		spec.Values = []ast.Expr{value}
	}
	return &ast.GenDecl{Tok: token.CONST, Specs: []ast.Spec{spec}}
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

func methodDecl(recv, name string, recvType ast.Expr, params, results *ast.FieldList, body *ast.BlockStmt) *ast.FuncDecl {
	return &ast.FuncDecl{
		Recv: fieldList(field(recv, recvType)),
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

func mapType(key, value ast.Expr) *ast.MapType {
	return &ast.MapType{Key: key, Value: value}
}

func addrOf(x ast.Expr) *ast.UnaryExpr {
	return &ast.UnaryExpr{Op: token.AND, X: x}
}

func compositeLit(typ ast.Expr, elts ...ast.Expr) *ast.CompositeLit {
	return &ast.CompositeLit{Type: typ, Elts: elts}
}

func keyValue(key, value ast.Expr) *ast.KeyValueExpr {
	return &ast.KeyValueExpr{Key: key, Value: value}
}

// inferReturnType walks a Go AST block looking for return statements and
// infers the return type from the first returned expression it finds.
// Returns nil if no return with a value is found or the type can't be inferred.
func inferReturnType(body *ast.BlockStmt) *ast.FieldList {
	if body == nil {
		return nil
	}
	if typ := findReturnType(body.List); typ != nil {
		return fieldList(field("", typ))
	}
	return nil
}

func findReturnType(stmts []ast.Stmt) ast.Expr {
	for _, stmt := range stmts {
		switch s := stmt.(type) {
		case *ast.ReturnStmt:
			if len(s.Results) == 1 {
				if typ := inferExprType(s.Results[0]); typ != nil {
					return typ
				}
			}
		case *ast.IfStmt:
			if typ := findReturnType(s.Body.List); typ != nil {
				return typ
			}
			if es, ok := s.Else.(*ast.BlockStmt); ok {
				if typ := findReturnType(es.List); typ != nil {
					return typ
				}
			}
		case *ast.BlockStmt:
			if typ := findReturnType(s.List); typ != nil {
				return typ
			}
		}
	}
	return nil
}

func inferExprType(expr ast.Expr) ast.Expr {
	switch e := expr.(type) {
	case *ast.BasicLit:
		switch e.Kind {
		case token.STRING:
			return ident("string")
		case token.INT:
			return ident("float64")
		case token.FLOAT:
			return ident("float64")
		}
	case *ast.Ident:
		switch e.Name {
		case "true", "false":
			return ident("bool")
		}
	case *ast.UnaryExpr:
		if e.Op == token.NOT {
			return ident("bool")
		}
	case *ast.BinaryExpr:
		switch e.Op {
		case token.EQL, token.NEQ, token.LSS, token.LEQ, token.GTR, token.GEQ, token.LAND, token.LOR:
			return ident("bool")
		default:
			lt := inferExprType(e.X)
			rt := inferExprType(e.Y)
			if isStringType(lt) || isStringType(rt) {
				return ident("string")
			}
			if lt != nil {
				return lt
			}
			if rt != nil {
				return rt
			}
		}
	case *ast.CallExpr:
		if sel, ok := e.Fun.(*ast.SelectorExpr); ok {
			if pkg, ok := sel.X.(*ast.Ident); ok {
				switch {
				case pkg.Name == "fmt" && sel.Sel.Name == "Sprintf":
					return ident("string")
				case pkg.Name == "strconv":
					return ident("string")
				}
			}
		}
	case *ast.ParenExpr:
		return inferExprType(e.X)
	}
	return nil
}

func isStringType(expr ast.Expr) bool {
	if id, ok := expr.(*ast.Ident); ok {
		return id.Name == "string"
	}
	return false
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
			for i, r := range s.Results {
				s.Results[i] = wrapExprWithJSValue(r)
			}
		case *ast.IfStmt:
			wrapReturnStmts(s.Body.List)
			if es, ok := s.Else.(*ast.BlockStmt); ok {
				wrapReturnStmts(es.List)
			}
		case *ast.BlockStmt:
			wrapReturnStmts(s.List)
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
	// Generic: wrap with NewString(fmt.Sprintf("%v", expr)) as fallback
	return callExpr(selectorExpr(ident("jsvalue"), "NewString"),
		callExpr(selectorExpr(ident("fmt"), "Sprintf"), stringLit("%v"), expr))
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
				names = append(names, nameNode.Utf8Text(source))
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

// extractParamInfo returns a map of parameter names to whether they have
// explicit type annotations. true = has type annotation (Go native type),
// false = no annotation (defaults to *jsvalue.JSValue).
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
			typeNode := param.ChildByFieldName("type")
			if nameNode != nil && nameNode.Kind() == "identifier" {
				info[nameNode.Utf8Text(source)] = typeNode != nil && !isAnyType(typeNode, source)
			}
			// Destructured parameters generate synthetic _paramN names — track
			// them as untyped so subscript access uses .Get()/.Index().
			if nameNode != nil && (nameNode.Kind() == "object_pattern" || nameNode.Kind() == "array_pattern") {
				info[fmt.Sprintf("_param%d", i)] = typeNode != nil && !isAnyType(typeNode, source)
			}
		case "rest_parameter":
			nameNode := param.ChildByFieldName("pattern")
			typeNode := param.ChildByFieldName("type")
			if nameNode != nil {
				info[nameNode.Utf8Text(source)] = typeNode != nil && !isAnyType(typeNode, source)
			}
		case "identifier":
			info[param.Utf8Text(source)] = false
		}
	}
	return info
}

// isNilIdent returns true if the expression is the identifier "nil".
func isNilIdent(expr ast.Expr) bool {
	id, ok := expr.(*ast.Ident)
	return ok && id.Name == "nil"
}

// isAnyType returns true when a type_annotation node resolves to TS "any",
// which maps to *jsvalue.JSValue in Go and should be treated as untyped.
func isAnyType(typeNode *sitter.Node, source []byte) bool {
	if typeNode == nil {
		return false
	}
	text := strings.TrimSpace(typeNode.Utf8Text(source))
	// type_annotation nodes include the leading ":", so strip it.
	text = strings.TrimPrefix(text, ":")
	text = strings.TrimSpace(text)
	return text == "any"
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

// isMapLiteral checks if an expression is a map composite literal.
func isMapLiteral(expr ast.Expr) bool {
	cl, ok := expr.(*ast.CompositeLit)
	if !ok {
		return false
	}
	_, ok = cl.Type.(*ast.MapType)
	return ok
}

// isObjectAssignCall checks if a call expression is object.Assign(...).
func isObjectAssignCall(ce *ast.CallExpr) bool {
	sel, ok := ce.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	return ok && id.Name == "object" && sel.Sel.Name == "Assign"
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

// uncapitalize returns the string with the first letter lowercased.
func uncapitalize(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	r[0] = unicode.ToLower(r[0])
	return string(r)
}

// receiverName returns a short receiver variable name from a type name.
func receiverName(typeName string) string {
	if typeName == "" {
		return "v"
	}
	return strings.ToLower(typeName[:1])
}
