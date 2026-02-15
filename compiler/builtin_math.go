package compiler

import "go/ast"

func transformMathCall(prop string, args []ast.Expr, addImport func(string)) ast.Expr {
	addImport("math")
	switch prop {
	case "floor":
		return callExpr(selectorExpr(ident("math"), "Floor"), args...)
	case "ceil":
		return callExpr(selectorExpr(ident("math"), "Ceil"), args...)
	case "round":
		return callExpr(selectorExpr(ident("math"), "Round"), args...)
	case "abs":
		return callExpr(selectorExpr(ident("math"), "Abs"), args...)
	case "max", "min":
		// min/max commonly receive mixed types (int, JSValue); coerce to float64.
		coerced := make([]ast.Expr, len(args))
		for i, a := range args {
			coerced[i] = callExpr(ident("float64"), a)
		}
		goName := "Max"
		if prop == "min" {
			goName = "Min"
		}
		return callExpr(selectorExpr(ident("math"), goName), coerced...)
	case "sqrt":
		return callExpr(selectorExpr(ident("math"), "Sqrt"), args...)
	case "pow":
		return callExpr(selectorExpr(ident("math"), "Pow"), args...)
	case "random":
		addImport("math/rand")
		return callExpr(selectorExpr(ident("rand"), "Float64"))
	}
	return nil
}
