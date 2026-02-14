package compiler

import "go/ast"

func transformCollectionMethod(obj ast.Expr, prop string, args []ast.Expr, addImport func(string)) ast.Expr {
	switch prop {
	case "push":
		if len(args) > 0 {
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

func transformObjectCall(prop string, args []ast.Expr) ast.Expr {
	switch prop {
	case "keys", "values", "assign":
		if len(args) > 0 {
			return args[0]
		}
	}
	return nil
}
