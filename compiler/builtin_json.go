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
			// JSON.parse(x) → func() *jsvalue.JSValue { var v any; json.Unmarshal([]byte(x), &v); return jsvalue.From(v) }()
			addImport("github.com/nnstd/gun/runtime/jsvalue")
			return &ast.CallExpr{
				Fun: &ast.FuncLit{
					Type: &ast.FuncType{
						Params:  fieldList(),
						Results: fieldList(field("", ptrType(selectorExpr(ident("jsvalue"), "JSValue")))),
					},
					Body: blockStmt(
						&ast.DeclStmt{Decl: varDecl("v", ident("any"), nil)},
						exprStmt(callExpr(
							selectorExpr(ident("json"), "Unmarshal"),
							callExpr(ident("[]byte"), args[0]),
							addrOf(ident("v")),
						)),
						returnStmt(callExpr(selectorExpr(ident("jsvalue"), "From"), ident("v"))),
					),
				},
			}
		}
	}
	return nil
}
