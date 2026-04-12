package compiler

import "go/ast"

func transformJSONCall(prop string, args []ast.Expr, addImport func(string)) ast.Expr {
	addImport("github.com/nnstd/gun/runtime/builtin/json")
	for i, a := range args {
		args[i] = jsvalueWrapLit(a)
	}
	return callExpr(
		selectorExpr(
			callExpr(selectorExpr(selectorExpr(ident("json"), "AsJSValue"), "Get"), stringLit(prop)),
			"Call",
		),
		args...,
	)
}
