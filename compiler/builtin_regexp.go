package compiler

import "go/ast"

// transformRegexpMethod handles method calls on regexp objects.
func transformRegexpMethod(obj ast.Expr, prop string, args []ast.Expr, addImport func(string)) ast.Expr {
	switch prop {
	case "test":
		// regex.test(str) → regex.MatchString(str)
		if len(args) > 0 {
			arg := args[0]
			if isJSValueMethodCall(arg) {
				addImport("fmt")
				arg = callExpr(selectorExpr(ident("fmt"), "Sprint"), arg)
			}
			return callExpr(selectorExpr(obj, "MatchString"), arg)
		}
	case "exec":
		// regex.exec(str) → regex.FindStringSubmatch(str)
		if len(args) > 0 {
			arg := args[0]
			if isJSValueMethodCall(arg) {
				addImport("fmt")
				arg = callExpr(selectorExpr(ident("fmt"), "Sprint"), arg)
			}
			return callExpr(selectorExpr(obj, "FindStringSubmatch"), arg)
		}
	}
	return nil
}
