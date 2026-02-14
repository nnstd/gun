package compiler

import "go/ast"

// transformBuiltinCall dispatches calls on known global objects (console, Math, JSON, Object).
// Returns nil if the call is not a known builtin.
func transformBuiltinCall(obj, prop string, args []ast.Expr, addImport func(string)) ast.Expr {
	switch obj {
	case "console":
		return transformConsoleCall(prop, args, addImport)
	case "Math":
		return transformMathCall(prop, args, addImport)
	case "JSON":
		return transformJSONCall(prop, args, addImport)
	case "Object":
		return transformObjectCall(prop, args)
	}
	return nil
}

// transformBuiltinMethod dispatches method calls on arbitrary receivers (e.g. x.push, x.split).
// Returns nil if the method is not a known builtin.
func transformBuiltinMethod(obj ast.Expr, prop string, args []ast.Expr, addImport func(string)) ast.Expr {
	if r := transformStringMethod(obj, prop, args, addImport); r != nil {
		return r
	}
	if r := transformCollectionMethod(obj, prop, args, addImport); r != nil {
		return r
	}
	return nil
}

// transformBuiltinNew handles `new X(args)` for known builtin types.
// Returns nil if the type is not a known builtin.
func transformBuiltinNew(name string, args []ast.Expr, addImport func(string)) ast.Expr {
	switch name {
	case "Error":
		addImport("errors")
		if len(args) > 0 {
			return callExpr(selectorExpr(ident("errors"), "New"), args[0])
		}
		return callExpr(selectorExpr(ident("errors"), "New"), stringLit("error"))
	case "Map":
		return callExpr(ident("make"), mapType(ident("any"), ident("any")))
	case "Set":
		return callExpr(ident("make"), mapType(ident("any"), &ast.StructType{Fields: &ast.FieldList{}}))
	case "Date":
		addImport("time")
		return callExpr(selectorExpr(ident("time"), "Now"))
	case "RegExp":
		addImport("regexp")
		if len(args) > 0 {
			return callExpr(selectorExpr(ident("regexp"), "MustCompile"), args[0])
		}
		return callExpr(selectorExpr(ident("regexp"), "MustCompile"), stringLit(""))
	case "Hono":
		addImport("gun/runtime/hono")
		return callExpr(selectorExpr(ident("hono"), "New"))
	}
	return nil
}

// mapIdentifier maps known TS global identifiers to their Go equivalents.
func mapIdentifier(name string, addImport func(string)) ast.Expr {
	switch name {
	case "undefined", "null":
		return ident("nil")
	case "console":
		return ident("fmt")
	case "Math":
		addImport("math")
		return ident("math")
	case "JSON":
		addImport("encoding/json")
		return ident("json")
	case "Error":
		addImport("errors")
		return ident("errors")
	default:
		return ident(name)
	}
}
