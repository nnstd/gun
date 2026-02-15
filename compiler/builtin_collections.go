package compiler

import "go/ast"

func transformCollectionMethod(obj ast.Expr, prop string, args []ast.Expr, addImport func(string)) ast.Expr {
	switch prop {
	case "concat":
		if len(args) > 0 {
			// [].concat(x) → just x (empty receiver is a no-op)
			if cl, ok := obj.(*ast.CompositeLit); ok && len(cl.Elts) == 0 {
				return args[0]
			}
			return callExpr(ident("append"), append([]ast.Expr{obj}, args...)...)
		}
	case "push":
		if len(args) > 0 {
			// When the receiver is a JSValue (e.g. obj.Get("key")), use .Push() method
			// and wrap args with jsvalue.From().
			if isJSValueMethodCall(obj) {
				wrapped := make([]ast.Expr, len(args))
				for i, a := range args {
					wrapped[i] = callExpr(selectorExpr(ident("jsvalue"), "From"), a)
				}
				return callExpr(selectorExpr(obj, "Push"), wrapped...)
			}
			return callExpr(ident("append"), append([]ast.Expr{obj}, args...)...)
		}
	case "length":
		return callExpr(ident("len"), obj)
	case "includes":
		if len(args) > 0 {
			return callExpr(ident("contains"), append([]ast.Expr{obj}, args...)...)
		}
	case "slice":
		if len(args) >= 2 {
			return &ast.SliceExpr{X: obj, Low: args[0], High: args[1]}
		}
		if len(args) == 1 {
			return &ast.SliceExpr{X: obj, Low: args[0]}
		}
	case "map", "filter", "forEach", "reduce", "find", "some", "every":
		// These need more complex transforms; pass through for now
	}
	return nil
}

func transformObjectCall(prop string, args []ast.Expr, addImport func(string)) ast.Expr {
	switch prop {
	case "keys", "values", "entries", "assign":
		if len(args) > 0 {
			return args[0]
		}
	case "create":
		// Object.create(null) → jsvalue.NewObject()
		addImport("github.com/nnstd/gun/runtime/jsvalue")
		return callExpr(selectorExpr(ident("jsvalue"), "NewObject"))
	}
	return nil
}
