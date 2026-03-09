package compiler

import "go/ast"

func transformMathCall(prop string, args []ast.Expr, addImport func(string)) ast.Expr {
	addImport("github.com/nnstd/gun/runtime/builtin/math")

	// Wrap literal args since runtime/math accepts *jsvalue.JSValue
	wrappedArgs := make([]ast.Expr, len(args))
	for i, a := range args {
		wrappedArgs[i] = jsvalueWrapLit(a)
	}

	switch prop {
	case "floor":
		if len(wrappedArgs) > 0 {
			return callExpr(selectorExpr(ident("math"), "Floor"), wrappedArgs[0])
		}
	case "ceil":
		if len(wrappedArgs) > 0 {
			return callExpr(selectorExpr(ident("math"), "Ceil"), wrappedArgs[0])
		}
	case "round":
		if len(wrappedArgs) > 0 {
			return callExpr(selectorExpr(ident("math"), "Round"), wrappedArgs[0])
		}
	case "abs":
		if len(wrappedArgs) > 0 {
			return callExpr(selectorExpr(ident("math"), "Abs"), wrappedArgs[0])
		}
	case "max":
		return callExpr(selectorExpr(ident("math"), "Max"), wrappedArgs...)
	case "min":
		return callExpr(selectorExpr(ident("math"), "Min"), wrappedArgs...)
	case "sqrt":
		if len(wrappedArgs) > 0 {
			return callExpr(selectorExpr(ident("math"), "Sqrt"), wrappedArgs[0])
		}
	case "pow":
		if len(wrappedArgs) >= 2 {
			return callExpr(selectorExpr(ident("math"), "Pow"), wrappedArgs[0], wrappedArgs[1])
		}
	case "random":
		return callExpr(selectorExpr(ident("math"), "Random"))
	case "log":
		if len(wrappedArgs) > 0 {
			return callExpr(selectorExpr(ident("math"), "Log"), wrappedArgs[0])
		}
	case "log2":
		if len(wrappedArgs) > 0 {
			return callExpr(selectorExpr(ident("math"), "Log2"), wrappedArgs[0])
		}
	case "trunc":
		if len(wrappedArgs) > 0 {
			return callExpr(selectorExpr(ident("math"), "Trunc"), wrappedArgs[0])
		}
	case "sign":
		if len(wrappedArgs) > 0 {
			return callExpr(selectorExpr(ident("math"), "Sign"), wrappedArgs[0])
		}
	}
	return nil
}
