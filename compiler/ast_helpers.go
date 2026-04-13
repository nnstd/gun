package compiler

import (
	"go/ast"
	"go/token"
	"unicode"
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

func returnStmt(results ...ast.Expr) *ast.ReturnStmt {
	return &ast.ReturnStmt{Results: results}
}

func ptrType(x ast.Expr) *ast.StarExpr {
	return &ast.StarExpr{X: x}
}

func isAlreadyJSValue(expr ast.Expr) bool {
	switch e := expr.(type) {
	case *ast.CallExpr:
		if sel, ok := e.Fun.(*ast.SelectorExpr); ok {
			if id, ok := sel.X.(*ast.Ident); ok && id.Name == "jsvalue" {
				return true
			}
			switch sel.Sel.Name {
			case "Get", "Index", "Call", "MethodCall":
				return true
			}
		}
	}
	return false
}

func jsvalueWrapLit(expr ast.Expr) ast.Expr {
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
	return callExpr(selectorExpr(ident("jsvalue"), "From"), expr)
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}
