package compiler

import "go/ast"

func transformJSONCall(prop string, args []ast.Expr, addImport func(string)) ast.Expr {
	addImport("encoding/json")
	switch prop {
	case "stringify":
		if len(args) > 0 {
			return callExpr(selectorExpr(ident("json"), "Marshal"), args[0])
		}
	case "parse":
		if len(args) > 0 {
			// JSON.parse(x) → func() any { var v any; json.Unmarshal([]byte(x), &v); return v }()
			return &ast.CallExpr{
				Fun: &ast.FuncLit{
					Type: &ast.FuncType{
						Params:  fieldList(),
						Results: fieldList(field("", ident("any"))),
					},
					Body: blockStmt(
						&ast.DeclStmt{Decl: varDecl("v", ident("any"), nil)},
						exprStmt(callExpr(
							selectorExpr(ident("json"), "Unmarshal"),
							callExpr(ident("[]byte"), args[0]),
							addrOf(ident("v")),
						)),
						returnStmt(ident("v")),
					),
				},
			}
		}
	}
	return nil
}
