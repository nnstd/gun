package compiler

import (
	"go/ast"
	"go/token"
	"strings"
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
