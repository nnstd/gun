package compiler

import "go/ast"

func transformStringMethod(obj ast.Expr, prop string, args []ast.Expr, addImport func(string)) ast.Expr {
	switch prop {
	case "toString":
		addImport("fmt")
		return callExpr(selectorExpr(ident("fmt"), "Sprint"), obj)
	case "indexOf":
		addImport("strings")
		if len(args) > 0 {
			return callExpr(selectorExpr(ident("strings"), "Index"), obj, args[0])
		}
	case "split":
		addImport("strings")
		if len(args) > 0 {
			return callExpr(selectorExpr(ident("strings"), "Split"), obj, args[0])
		}
	case "join":
		addImport("strings")
		if len(args) > 0 {
			return callExpr(selectorExpr(ident("strings"), "Join"), obj, args[0])
		}
	case "trim":
		addImport("strings")
		return callExpr(selectorExpr(ident("strings"), "TrimSpace"), obj)
	case "trimStart", "trimLeft":
		addImport("strings")
		return callExpr(selectorExpr(ident("strings"), "TrimLeft"), obj, stringLit(" \\t\\n\\r"))
	case "trimEnd", "trimRight":
		addImport("strings")
		return callExpr(selectorExpr(ident("strings"), "TrimRight"), obj, stringLit(" \\t\\n\\r"))
	case "repeat":
		addImport("strings")
		if len(args) > 0 {
			return callExpr(selectorExpr(ident("strings"), "Repeat"), obj, args[0])
		}
	case "toLowerCase":
		addImport("strings")
		return callExpr(selectorExpr(ident("strings"), "ToLower"), obj)
	case "toUpperCase":
		addImport("strings")
		return callExpr(selectorExpr(ident("strings"), "ToUpper"), obj)
	case "startsWith":
		addImport("strings")
		if len(args) > 0 {
			return callExpr(selectorExpr(ident("strings"), "HasPrefix"), obj, args[0])
		}
	case "endsWith":
		addImport("strings")
		if len(args) > 0 {
			return callExpr(selectorExpr(ident("strings"), "HasSuffix"), obj, args[0])
		}
	case "replace":
		addImport("strings")
		if len(args) >= 2 {
			return callExpr(selectorExpr(ident("strings"), "Replace"), obj, args[0], args[1], intLit("1"))
		}
	case "replaceAll":
		addImport("strings")
		if len(args) >= 2 {
			return callExpr(selectorExpr(ident("strings"), "ReplaceAll"), obj, args[0], args[1])
		}
	}
	return nil
}
