package compiler

import (
	"go/ast"

	tcontext "github.com/nnstd/gun/compiler/context"
)

func transformConsoleCall(prop string, args []ast.Expr, hasSpread bool, imp tcontext.Imports) ast.Expr {
	imp.AddImport("github.com/nnstd/gun/runtime/builtin/console")
	if !hasSpread {
		imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
		for i, arg := range args {
			args[i] = jsvalueWrapLit(arg)
		}
	}
	var call *ast.CallExpr
	switch prop {
	case "log":
		call = callExpr(selectorExpr(ident("console"), "Log"), args...)
	case "error":
		call = callExpr(selectorExpr(ident("console"), "Error"), args...)
	case "warn":
		call = callExpr(selectorExpr(ident("console"), "Warn"), args...)
	case "dir":
		call = callExpr(selectorExpr(ident("console"), "Dir"), args...)
	}
	if call == nil {
		return nil
	}
	if hasSpread {
		call.Ellipsis = 1
	}
	return call
}
