package compiler

import "go/ast"

// transformRegexpMethod handles method calls on regexp objects.
// The obj parameter should always be a JSValue expression (either an identifier
// or wrapped with NewRegex for typed regex values).
func transformRegexpMethod(obj ast.Expr, prop string, args []ast.Expr, addImport func(string)) ast.Expr {
	switch prop {
	case "test":
		// regex.test(str) → jsvalue.NewBool(jsvalue.MatchString(regex, str))
		// Wrapped in NewBool so the result is *jsvalue.JSValue for all-JSValue consistency.
		if len(args) > 0 {
			addImport("github.com/nnstd/gun/runtime/builtin")
			matchCall := callExpr(selectorExpr(ident("jsvalue"), "MatchString"), obj, args[0])
			return callExpr(selectorExpr(ident("jsvalue"), "NewBool"), matchCall)
		}
	case "exec":
		// regex.exec(str) → jsvalue.RegexExec(regex, str)
		if len(args) > 0 {
			addImport("github.com/nnstd/gun/runtime/builtin")
			return callExpr(selectorExpr(ident("jsvalue"), "RegexExec"), obj, args[0])
		}
	}
	return nil
}
