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
	case "max":
		return callExpr(selectorExpr(ident("math"), "Max"), args...)
	case "min":
		return callExpr(selectorExpr(ident("math"), "Min"), args...)
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
