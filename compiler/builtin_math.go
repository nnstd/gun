package compiler

import "go/ast"

func transformMathCall(prop string, args []ast.Expr, addImport func(string)) ast.Expr {
	addImport("github.com/nnstd/gun/runtime/jsmath")

	// Wrap literal args since runtime/math accepts *jsvalue.JSValue
	wrappedArgs := make([]ast.Expr, len(args))
	for i, a := range args {
		wrappedArgs[i] = jsvalueWrapLit(a)
	}

	switch prop {
	case "floor":
		if len(wrappedArgs) > 0 {
			return callExpr(selectorExpr(ident("jsmath"), "Floor"), wrappedArgs[0])
		}
	case "ceil":
		if len(wrappedArgs) > 0 {
			return callExpr(selectorExpr(ident("jsmath"), "Ceil"), wrappedArgs[0])
		}
	case "round":
		if len(wrappedArgs) > 0 {
			return callExpr(selectorExpr(ident("jsmath"), "Round"), wrappedArgs[0])
		}
	case "abs":
		if len(wrappedArgs) > 0 {
			return callExpr(selectorExpr(ident("jsmath"), "Abs"), wrappedArgs[0])
		}
	case "max":
		return callExpr(selectorExpr(ident("jsmath"), "Max"), wrappedArgs...)
	case "min":
		return callExpr(selectorExpr(ident("jsmath"), "Min"), wrappedArgs...)
	case "sqrt":
		if len(wrappedArgs) > 0 {
			return callExpr(selectorExpr(ident("jsmath"), "Sqrt"), wrappedArgs[0])
		}
	case "pow":
		if len(wrappedArgs) >= 2 {
			return callExpr(selectorExpr(ident("jsmath"), "Pow"), wrappedArgs[0], wrappedArgs[1])
		}
	case "random":
		return callExpr(selectorExpr(ident("jsmath"), "Random"))
	case "log":
		if len(wrappedArgs) > 0 {
			return callExpr(selectorExpr(ident("jsmath"), "Log"), wrappedArgs[0])
		}
	case "log2":
		if len(wrappedArgs) > 0 {
			return callExpr(selectorExpr(ident("jsmath"), "Log2"), wrappedArgs[0])
		}
	case "trunc":
		if len(wrappedArgs) > 0 {
			return callExpr(selectorExpr(ident("jsmath"), "Trunc"), wrappedArgs[0])
		}
	case "sign":
		if len(wrappedArgs) > 0 {
			return callExpr(selectorExpr(ident("jsmath"), "Sign"), wrappedArgs[0])
		}
	}
	return nil
}
