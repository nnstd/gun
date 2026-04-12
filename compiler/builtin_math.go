package compiler

import "go/ast"

func transformMathCall(prop string, args []ast.Expr, addImport func(string)) ast.Expr {
	addImport("github.com/nnstd/gun/runtime/builtin/math")
	for i, a := range args {
		args[i] = jsvalueWrapLit(a)
	}
	return callExpr(
		selectorExpr(
			callExpr(selectorExpr(selectorExpr(ident("math"), "AsJSValue"), "Get"), stringLit(prop)),
			"Call",
		),
		args...,
	)
}
