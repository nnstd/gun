package compiler

import "go/ast"

func transformStringMethod(obj ast.Expr, prop string, args []ast.Expr, addImport func(string)) ast.Expr {
	// If the receiver is a JSValue method call (Get, Slice, Index, etc.),
	// coerce to string for string methods.
	if isJSValueMethodCall(obj) {
		addImport("fmt")
		obj = callExpr(selectorExpr(ident("fmt"), "Sprint"), obj)
	}

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
	case "includes":
		addImport("strings")
		if len(args) > 0 {
			return callExpr(selectorExpr(ident("strings"), "Contains"), obj, args[0])
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
	case "codePointAt":
		// str.codePointAt(pos) → int([]rune(str)[pos])
		var idx ast.Expr = intLit("0")
		if len(args) > 0 {
			idx = args[0]
		}
		return callExpr(ident("int"), &ast.IndexExpr{
			X:     callExpr(ident("[]rune"), obj),
			Index: idx,
		})
	case "charAt":
		// str.charAt(i) → string([]rune(str)[i])
		var idx ast.Expr = intLit("0")
		if len(args) > 0 {
			idx = args[0]
		}
		return callExpr(ident("string"), &ast.IndexExpr{
			X:     callExpr(ident("[]rune"), obj),
			Index: idx,
		})
	case "charCodeAt":
		// str.charCodeAt(pos) → int(str[pos])
		var idx ast.Expr = intLit("0")
		if len(args) > 0 {
			idx = args[0]
		}
		return callExpr(ident("int"), &ast.IndexExpr{
			X:     callExpr(ident("[]rune"), obj),
			Index: idx,
		})
	case "match":
		// str.match(regex) → regex.FindStringSubmatch(str)
		if len(args) > 0 {
			return callExpr(selectorExpr(args[0], "FindStringSubmatch"), obj)
		}
	}
	return nil
}
