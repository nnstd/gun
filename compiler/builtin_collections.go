package compiler

import (
	"go/ast"
)

func transformObjectCall(prop string, args []ast.Expr, addImport func(string)) ast.Expr {
	addImport("github.com/nnstd/gun/runtime/builtin")
	for i, a := range args {
		args[i] = jsvalueWrapLit(a)
	}
	return callExpr(
		selectorExpr(
			callExpr(selectorExpr(selectorExpr(ident("jsvalue"), "Object"), "Get"), stringLit(prop)),
			"Call",
		),
		args...,
	)
}
