package compiler

import (
	"go/ast"

	tcontext "github.com/nnstd/gun/compiler/context"
)

func transformJSONCall(prop string, args []ast.Expr, imp tcontext.Imports) ast.Expr {
	imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin/json", "_gunJSON")
	for i, a := range args {
		args[i] = jsvalueWrapLit(a)
	}
	return callExpr(
		selectorExpr(
			callExpr(selectorExpr(selectorExpr(ident("_gunJSON"), "AsJSValue"), "Get"), stringLit(prop)),
			"Call",
		),
		args...,
	)
}
