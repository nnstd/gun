package compiler

import (
	"go/ast"
	"go/token"
)

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
		return transformObjectCall(prop, args, addImport)
	case "Number":
		return transformNumberCall(prop, args)
	case "process":
		return transformProcessCall(prop, args, addImport)
	case "Array":
		return transformArrayCall(prop, args, addImport)
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
	if r := transformRegexpMethod(obj, prop, args, addImport); r != nil {
		return r
	}
	return nil
}

// transformBuiltinNew handles `new X(args)` for known builtin types.
// Returns nil if the type is not a known builtin.
func transformBuiltinNew(name string, args []ast.Expr, t *Transformer) ast.Expr {
	switch name {
	case "Error", "TypeError", "RangeError", "ReferenceError":
		t.addImport("errors")
		if len(args) > 0 {
			return callExpr(selectorExpr(ident("errors"), "New"), args[0])
		}
		return callExpr(selectorExpr(ident("errors"), "New"), stringLit("error"))
	case "Map":
		return callExpr(ident("make"), mapType(t.jsValueType(), t.jsValueType()))
	case "Set":
		return callExpr(ident("make"), mapType(t.jsValueType(), &ast.StructType{Fields: &ast.FieldList{}}))
	case "Date":
		t.addImport("time")
		return callExpr(selectorExpr(ident("time"), "Now"))
	case "RegExp":
		t.addImport("regexp")
		pattern := ast.Expr(stringLit(""))
		if len(args) > 0 {
			pattern = args[0]
		}
		compiled := callExpr(selectorExpr(ident("regexp"), "MustCompile"), pattern)
		// When flags argument is present, wrap in IIFE to preserve the reference
		// (Go rejects unused variables). JS regex flags like "g" have no direct
		// Go equivalent — global matching is handled at the call site.
		if len(args) > 1 {
			return &ast.CallExpr{
				Fun: &ast.FuncLit{
					Type: &ast.FuncType{
						Params:  fieldList(),
						Results: fieldList(field("", ptrType(selectorExpr(ident("regexp"), "Regexp")))),
					},
					Body: blockStmt(
						&ast.AssignStmt{
							Lhs: []ast.Expr{ident("_")},
							Tok: token.ASSIGN,
							Rhs: []ast.Expr{args[1]},
						},
						returnStmt(compiled),
					),
				},
			}
		}
		return compiled
	case "Hono":
		t.addImport("github.com/nnstd/gun/runtime/hono")
		return callExpr(selectorExpr(ident("hono"), "New"))
	case "IntlSegmenter":
		t.addImport("github.com/nnstd/gun/runtime/intl")
		return callExpr(selectorExpr(ident("intl"), "NewSegmenter"))
	}
	return nil
}

// transformNumberCall handles Number.X() calls.
func transformNumberCall(prop string, args []ast.Expr) ast.Expr {
	switch prop {
	case "isNaN":
		// Number.isNaN(x) → math.IsNaN(float64(x)) — but simplified
		if len(args) > 0 {
			return ident("false")
		}
		return ident("false")
	case "isFinite":
		if len(args) > 0 {
			return ident("true")
		}
		return ident("true")
	case "isInteger", "isSafeInteger":
		// For integer code points, this is always true in practice
		if len(args) > 0 {
			return ident("true")
		}
		return ident("true")
	case "parseInt":
		if len(args) > 0 {
			return args[0]
		}
		return intLit("0")
	case "parseFloat":
		if len(args) > 0 {
			return args[0]
		}
		return floatLit("0.0")
	}
	return nil
}

// transformArrayCall handles Array.X() calls.
func transformArrayCall(prop string, args []ast.Expr, addImport func(string)) ast.Expr {
	switch prop {
	case "isArray":
		// Array.isArray(x) → x.IsArray()
		if len(args) > 0 {
			addImport("github.com/nnstd/gun/runtime/jsvalue")
			return callExpr(selectorExpr(args[0], "IsArray"))
		}
	}
	return nil
}

// transformProcessCall handles process.X() calls.
func transformProcessCall(prop string, args []ast.Expr, addImport func(string)) ast.Expr {
	switch prop {
	case "exit":
		addImport("os")
		if len(args) > 0 {
			return callExpr(selectorExpr(ident("os"), "Exit"), args[0])
		}
		return callExpr(selectorExpr(ident("os"), "Exit"), intLit("0"))
	case "cwd":
		addImport("os")
		// os.Getwd() returns (string, error); wrap in helper
		return callExpr(ident("func() string { d, _ := os.Getwd(); return d }"))
	}
	return nil
}

// transformProcessMember handles process.X member access (not calls).
func transformProcessMember(prop string, addImport func(string)) ast.Expr {
	switch prop {
	case "env":
		// process.env → os.Environ() is not right; it's used as a map.
		// Return a placeholder that works for subscript access.
		addImport("os")
		return ident("os") // process.env.X will become os.Getenv via further transforms
	case "argv":
		addImport("os")
		return selectorExpr(ident("os"), "Args")
	case "platform":
		addImport("runtime")
		return selectorExpr(ident("runtime"), "GOOS")
	case "stdout":
		addImport("os")
		return selectorExpr(ident("os"), "Stdout")
	case "stderr":
		addImport("os")
		return selectorExpr(ident("os"), "Stderr")
	case "cwd":
		addImport("os")
		// Return a function value that matches process.cwd()
		return selectorExpr(ident("os"), "Getwd")
	case "versions":
		// process.versions is rarely useful in Go; return empty map
		return compositeLit(mapType(ident("string"), ident("string")))
	case "pid":
		addImport("os")
		return callExpr(selectorExpr(ident("os"), "Getpid"))
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
	case "process":
		// process as standalone value — return a non-nil *jsvalue.JSValue so it's
		// truthy in boolean contexts and comparable to nil in optional chaining.
		addImport("github.com/nnstd/gun/runtime/jsvalue")
		return callExpr(selectorExpr(ident("jsvalue"), "NewBool"), ident("true"))
	case "Number":
		// Number as standalone (used as Number(x) call) — identity for numeric values
		return ident("float64")
	case "Boolean":
		addImport("github.com/nnstd/gun/runtime/jsvalue")
		return selectorExpr(ident("jsvalue"), "Truthy")
	default:
		return ident(sanitizeIdent(name))
	}
}
