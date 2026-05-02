package backend

import (
	"fmt"
	"go/ast"
	"go/token"
	"strconv"
	"strings"
)

// Go AST builder primitives.

func goIdent(name string) *ast.Ident {
	return &ast.Ident{Name: name}
}

func cloneIdent(expr ast.Expr) ast.Expr {
	if id, ok := expr.(*ast.Ident); ok {
		return &ast.Ident{Name: id.Name}
	}
	return expr
}

func basicLit(kind token.Token, value string) *ast.BasicLit {
	return &ast.BasicLit{Kind: kind, Value: value}
}

func stringLit(s string) *ast.BasicLit {
	return basicLit(token.STRING, strconv.Quote(s))
}

func intLit(s string) *ast.BasicLit {
	return basicLit(token.INT, s)
}

func floatLit(s string) *ast.BasicLit {
	return basicLit(token.FLOAT, s)
}

// rawStringLit creates a backtick-quoted raw string literal (no escape processing).
func rawStringLit(s string) *ast.BasicLit {
	// If the pattern contains backticks, fall back to double-quoted with escaping
	if strings.Contains(s, "`") {
		escaped := strings.ReplaceAll(s, `\`, `\\`)
		escaped = strings.ReplaceAll(escaped, `"`, `\"`)
		return basicLit(token.STRING, `"`+escaped+`"`)
	}
	return basicLit(token.STRING, "`"+s+"`")
}

func goField(name string, typ ast.Expr) *ast.Field {
	f := &ast.Field{Type: typ}
	if name != "" {
		f.Names = []*ast.Ident{goIdent(name)}
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
	return &ast.SelectorExpr{X: x, Sel: goIdent(sel)}
}

func assignDefine(lhs, rhs []ast.Expr) *ast.AssignStmt {
	return &ast.AssignStmt{Lhs: lhs, Rhs: rhs, Tok: token.DEFINE}
}

func assignStmt(lhs, rhs []ast.Expr) *ast.AssignStmt {
	return &ast.AssignStmt{Lhs: lhs, Rhs: rhs, Tok: token.ASSIGN}
}

func returnStmt(results ...ast.Expr) *ast.ReturnStmt {
	return &ast.ReturnStmt{Results: results}
}

func exprStmt(x ast.Expr) *ast.ExprStmt {
	return &ast.ExprStmt{X: x}
}

func varDecl(name string, typ, value ast.Expr) *ast.GenDecl {
	spec := &ast.ValueSpec{
		Names: []*ast.Ident{goIdent(name)},
	}
	if typ != nil {
		spec.Type = typ
	}
	if value != nil {
		spec.Values = []ast.Expr{value}
	}
	return &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{spec}}
}

func funcDecl(name string, params, results *ast.FieldList, body *ast.BlockStmt) *ast.FuncDecl {
	return &ast.FuncDecl{
		Name: goIdent(name),
		Type: &ast.FuncType{Params: params, Results: results},
		Body: body,
	}
}

func ptrType(x ast.Expr) *ast.StarExpr {
	return &ast.StarExpr{X: x}
}

func jsValuePtrType() ast.Expr {
	return ptrType(selectorExpr(goIdent("jsvalue"), "JSValue"))
}

func importSpecAlias(path, alias string) *ast.ImportSpec {
	spec := &ast.ImportSpec{
		Path: basicLit(token.STRING, `"`+path+`"`),
	}
	if alias != "" {
		spec.Name = goIdent(alias)
	}
	return spec
}

func itoa(i int) string {
	return fmt.Sprintf("%d", i)
}

// jsvalueWrapLit wraps a Go AST expression as *jsvalue.JSValue.
// Literals get specific constructors; already-JSValue expressions pass through.
func jsvalueWrapLit(expr ast.Expr) ast.Expr {
	if isAlreadyJSValue(expr) {
		return expr
	}
	switch e := expr.(type) {
	case *ast.BasicLit:
		switch e.Kind {
		case token.STRING:
			return callExpr(selectorExpr(goIdent("jsvalue"), "NewString"), e)
		case token.INT:
			return callExpr(selectorExpr(goIdent("jsvalue"), "NewNumber"), callExpr(goIdent("float64"), e))
		case token.FLOAT:
			return callExpr(selectorExpr(goIdent("jsvalue"), "NewNumber"), e)
		}
	case *ast.UnaryExpr:
		if e.Op == token.SUB {
			if lit, ok := e.X.(*ast.BasicLit); ok && (lit.Kind == token.INT || lit.Kind == token.FLOAT) {
				return callExpr(selectorExpr(goIdent("jsvalue"), "NewNumber"), callExpr(goIdent("float64"), e))
			}
		}
	case *ast.Ident:
		if e.Name == "true" || e.Name == "false" {
			return callExpr(selectorExpr(goIdent("jsvalue"), "NewBool"), e)
		}
		if e.Name == "nil" {
			return callExpr(selectorExpr(goIdent("jsvalue"), "NewNull"))
		}
	}
	// Safety: don't wrap nil expressions
	if expr == nil {
		return callExpr(selectorExpr(goIdent("jsvalue"), "NewNull"))
	}
	return callExpr(selectorExpr(goIdent("jsvalue"), "From"), expr)
}

// isAlreadyJSValue returns true if the expression already produces *jsvalue.JSValue.
func isAlreadyJSValue(expr ast.Expr) bool {
	switch e := expr.(type) {
	case *ast.ParenExpr:
		return isAlreadyJSValue(e.X)
	case *ast.CallExpr:
		if _, ok := e.Fun.(*ast.FuncLit); ok {
			return true
		}
		if sel, ok := e.Fun.(*ast.SelectorExpr); ok {
			if id, ok := sel.X.(*ast.Ident); ok {
				if id.Name == "jsvalue" || strings.HasPrefix(id.Name, "_gun") {
					return true
				}
			}
			switch sel.Sel.Name {
			case "Get", "Index", "Call", "MethodCall", "MarkAsAsync", "MarkAsMethod", "New":
				return true
			}
		}
	}
	return false
}
