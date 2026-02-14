package compiler

import "go/ast"

func transformConsoleCall(prop string, args []ast.Expr, addImport func(string)) ast.Expr {
	addImport("fmt")
	switch prop {
	case "log":
		return callExpr(selectorExpr(ident("fmt"), "Println"), args...)
	case "error", "warn":
		addImport("os")
		return callExpr(selectorExpr(ident("fmt"), "Fprintln"),
			append([]ast.Expr{selectorExpr(ident("os"), "Stderr")}, args...)...)
	case "dir":
		return callExpr(selectorExpr(ident("fmt"), "Printf"),
			append([]ast.Expr{stringLit("%+v\\n")}, args...)...)
	}
	return nil
}
