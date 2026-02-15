package compiler

import "go/ast"

// transformRegexpMethod handles method calls on regexp objects.
// The obj parameter should always be a JSValue expression (either an identifier
// or wrapped with NewRegex for typed regex values).
func transformRegexpMethod(obj ast.Expr, prop string, args []ast.Expr, addImport func(string)) ast.Expr {
	switch prop {
	case "test":
		// regex.test(str) → jsvalue.MatchString(regex, str)
		if len(args) > 0 {
			addImport("github.com/nnstd/gun/runtime/jsvalue")
			return callExpr(selectorExpr(ident("jsvalue"), "MatchString"), obj, args[0])
		}
	case "exec":
		// For now, exec is not supported on JSValue regex
		// Could be added as a package-level function if needed
		return nil
	}
	return nil
}
