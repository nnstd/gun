package compiler

import "go/ast"

func transformConsoleCall(prop string, args []ast.Expr, addImport func(string)) ast.Expr {
	addImport("github.com/nnstd/gun/runtime/builtin/console")
	switch prop {
	case "log":
		return callExpr(selectorExpr(ident("console"), "Log"), args...)
	case "error":
		return callExpr(selectorExpr(ident("console"), "Error"), args...)
	case "warn":
		return callExpr(selectorExpr(ident("console"), "Warn"), args...)
	case "dir":
		return callExpr(selectorExpr(ident("console"), "Dir"), args...)
	}
	return nil
}
