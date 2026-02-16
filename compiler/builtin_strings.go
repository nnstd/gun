package compiler

import "go/ast"

func transformStringMethod(obj ast.Expr, prop string, args []ast.Expr, addImport func(string)) ast.Expr {
	// Check if the receiver is a JSValue method call
	wasJSValue := isJSValueMethodCall(obj)

	// Store original obj for JSValue wrapper calls
	origObj := obj

	// For non-JSValue wrapper methods, coerce to string
	needsCoercion := wasJSValue && prop != "join" &&
		prop != "toLowerCase" && prop != "toUpperCase" && prop != "trim" &&
		prop != "split" && prop != "replace" && prop != "replaceAll" &&
		prop != "charAt" && prop != "startsWith" && prop != "endsWith" && prop != "repeat"

	if needsCoercion {
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
		if len(args) > 0 {
			if wasJSValue {
				addImport("github.com/nnstd/gun/runtime/jsvalue")
				return callExpr(selectorExpr(ident("jsvalue"), "Split"), origObj, args[0])
			}
			addImport("strings")
			return callExpr(selectorExpr(ident("strings"), "Split"), obj, args[0])
		}
	case "join":
		if len(args) > 0 {
			// When receiver is a JSValue array (e.g. from .Map()), use .Join() method.
			if wasJSValue {
				return callExpr(selectorExpr(obj, "Join"), args[0])
			}
			addImport("strings")
			return callExpr(selectorExpr(ident("strings"), "Join"), obj, args[0])
		}
	case "trim":
		if wasJSValue {
			addImport("github.com/nnstd/gun/runtime/jsvalue")
			return callExpr(selectorExpr(ident("jsvalue"), "Trim"), origObj)
		}
		addImport("strings")
		return callExpr(selectorExpr(ident("strings"), "TrimSpace"), obj)
	case "trimStart", "trimLeft":
		addImport("strings")
		return callExpr(selectorExpr(ident("strings"), "TrimLeft"), obj, stringLit(" \\t\\n\\r"))
	case "trimEnd", "trimRight":
		addImport("strings")
		return callExpr(selectorExpr(ident("strings"), "TrimRight"), obj, stringLit(" \\t\\n\\r"))
	case "repeat":
		if len(args) > 0 {
			if wasJSValue {
				addImport("github.com/nnstd/gun/runtime/jsvalue")
				return callExpr(selectorExpr(ident("jsvalue"), "Repeat"), origObj, args[0])
			}
			addImport("strings")
			return callExpr(selectorExpr(ident("strings"), "Repeat"), obj, args[0])
		}
	case "toLowerCase":
		if wasJSValue {
			addImport("github.com/nnstd/gun/runtime/jsvalue")
			return callExpr(selectorExpr(ident("jsvalue"), "ToLowerCase"), origObj)
		}
		addImport("strings")
		return callExpr(selectorExpr(ident("strings"), "ToLower"), obj)
	case "toUpperCase":
		if wasJSValue {
			addImport("github.com/nnstd/gun/runtime/jsvalue")
			return callExpr(selectorExpr(ident("jsvalue"), "ToUpperCase"), origObj)
		}
		addImport("strings")
		return callExpr(selectorExpr(ident("strings"), "ToUpper"), obj)
	case "startsWith":
		if len(args) > 0 {
			if wasJSValue {
				addImport("github.com/nnstd/gun/runtime/jsvalue")
				return callExpr(selectorExpr(ident("jsvalue"), "StartsWith"), origObj, args[0])
			}
			addImport("strings")
			return callExpr(selectorExpr(ident("strings"), "HasPrefix"), obj, args[0])
		}
	case "endsWith":
		if len(args) > 0 {
			if wasJSValue {
				addImport("github.com/nnstd/gun/runtime/jsvalue")
				return callExpr(selectorExpr(ident("jsvalue"), "EndsWith"), origObj, args[0])
			}
			addImport("strings")
			return callExpr(selectorExpr(ident("strings"), "HasSuffix"), obj, args[0])
		}
	case "includes":
		addImport("strings")
		if len(args) > 0 {
			return callExpr(selectorExpr(ident("strings"), "Contains"), obj, args[0])
		}
	case "replace":
		if len(args) >= 2 {
			if wasJSValue {
				addImport("github.com/nnstd/gun/runtime/jsvalue")
				return callExpr(selectorExpr(ident("jsvalue"), "Replace"), origObj, args[0], args[1])
			}
			addImport("strings")
			return callExpr(selectorExpr(ident("strings"), "Replace"), obj, args[0], args[1], intLit("1"))
		}
	case "replaceAll":
		if len(args) >= 2 {
			if wasJSValue {
				addImport("github.com/nnstd/gun/runtime/jsvalue")
				return callExpr(selectorExpr(ident("jsvalue"), "Replace"), origObj, args[0], args[1])
			}
			addImport("strings")
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
		if wasJSValue {
			addImport("github.com/nnstd/gun/runtime/jsvalue")
			var idx ast.Expr = intLit("0")
			if len(args) > 0 {
				idx = args[0]
			}
			return callExpr(selectorExpr(ident("jsvalue"), "CharAt"), origObj, idx)
		}
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
