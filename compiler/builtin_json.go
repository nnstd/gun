package compiler

import "go/ast"

func transformJSONCall(prop string, args []ast.Expr, addImport func(string)) ast.Expr {
	addImport("github.com/nnstd/gun/runtime/builtin/json")
	switch prop {
	case "stringify":
		if len(args) > 0 {
			return callExpr(selectorExpr(ident("json"), "Stringify"), args[0])
		}
	case "parse":
		if len(args) > 0 {
			return callExpr(selectorExpr(ident("json"), "Parse"), args[0])
		}
	}
	return nil
}
