package compiler

import "go/ast"

// transformRegexpMethod handles method calls on regexp objects.
func transformRegexpMethod(obj ast.Expr, prop string, args []ast.Expr, addImport func(string)) ast.Expr {
	switch prop {
	case "test":
		// regex.test(str) → regex.MatchString(str)
		if len(args) > 0 {
			return callExpr(selectorExpr(obj, "MatchString"), args[0])
		}
	case "exec":
		// regex.exec(str) → regex.FindStringSubmatch(str)
		if len(args) > 0 {
			return callExpr(selectorExpr(obj, "FindStringSubmatch"), args[0])
		}
	}
	return nil
}
