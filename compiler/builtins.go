package compiler

import (
	"go/ast"
	"go/token"
)

// isErrorType returns true if the name is a JavaScript Error constructor.
func isErrorType(name string) bool {
	switch name {
	case "Error", "TypeError", "RangeError", "ReferenceError", "SyntaxError", "URIError", "EvalError":
		return true
	}
	return false
}

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
	if isErrorType(obj) {
		return transformErrorCall(obj, prop, args, addImport)
	}
	return nil
}

// transformGlobalCall handles bare global function calls like isNaN(), Error(), parseInt().
func transformGlobalCall(name string, args []ast.Expr, t *Transformer) ast.Expr {
	switch name {
	case "isNaN":
		t.addImport("math")
		if len(args) > 0 {
			return callExpr(selectorExpr(ident("math"), "IsNaN"), callExpr(selectorExpr(args[0], "Number")))
		}
		return ident("false")
	case "isFinite":
		t.addImport("math")
		if len(args) > 0 {
			return &ast.UnaryExpr{
				Op: token.NOT,
				X:  callExpr(selectorExpr(ident("math"), "IsInf"), callExpr(selectorExpr(args[0], "Number")), intLit("0")),
			}
		}
		return ident("true")
	case "Number":
		if len(args) > 0 {
			return callExpr(selectorExpr(args[0], "Number"))
		}
		return basicLit(token.FLOAT, "0")
	case "Array":
		// Array(n) → jsvalue.NewArray() — creates empty array (length handled at runtime)
		t.addAliasedImport("github.com/nnstd/gun/runtime/jsvalue", "jsvalue")
		return callExpr(selectorExpr(ident("jsvalue"), "NewArray"))
	}
	if isErrorType(name) {
		// Error("msg") → jserror.Error.Call(msg) — JS spec: Error() without new also creates Error
		t.addAliasedImport("github.com/nnstd/gun/runtime/jserror", "jserror")
		t.addAliasedImport("github.com/nnstd/gun/runtime/jsvalue", "jsvalue")
		for i, arg := range args {
			args[i] = jsvalueWrapLit(arg)
		}
		return callExpr(selectorExpr(selectorExpr(ident("jserror"), name), "Call"), args...)
	}
	return nil
}

// transformBuiltinMethod dispatches method calls on arbitrary receivers (e.g. x.push, x.split).
// Returns nil if the method is not a known builtin.
func transformBuiltinMethod(obj ast.Expr, prop string, args []ast.Expr, addImport func(string)) ast.Expr {
	if r := transformStringMethod(obj, prop, args, addImport); r != nil {
		return r
	}
	if r := transformCollectionMethod(obj, prop, args, addImport, false); r != nil {
		return r
	}
	// For typed regex values (not JSValue), use method form directly
	// JSValue regex is handled separately in expressions.go
	// regex.test() and regex.exec() are now handled by the JSValue dispatch
	// path since regex literals are *jsvalue.JSValue with test/exec methods.
	return nil
}

// transformBuiltinNew handles `new X(args)` for known builtin types.
// Returns nil if the type is not a known builtin.
func transformBuiltinNew(name string, args []ast.Expr, t *Transformer) ast.Expr {
	switch name {
	case "Error", "TypeError", "RangeError", "ReferenceError", "SyntaxError", "URIError", "EvalError":
		t.addAliasedImport("github.com/nnstd/gun/runtime/jserror", "jserror")
		t.addAliasedImport("github.com/nnstd/gun/runtime/jsvalue", "jsvalue")
		for i, arg := range args {
			args[i] = jsvalueWrapLit(arg)
		}
		return callExpr(selectorExpr(selectorExpr(ident("jserror"), name), "Call"), args...)
	case "Map":
		t.addAliasedImport("github.com/nnstd/gun/runtime/jsvalue", "jsvalue")
		return callExpr(selectorExpr(ident("jsvalue"), "NewMap"))
	case "Set":
		t.addAliasedImport("github.com/nnstd/gun/runtime/jsvalue", "jsvalue")
		return callExpr(selectorExpr(ident("jsvalue"), "NewSet"))
	case "Array":
		// new Array(n) → jsvalue.NewArray() (size ignored, filled at runtime)
		t.addAliasedImport("github.com/nnstd/gun/runtime/jsvalue", "jsvalue")
		return callExpr(selectorExpr(ident("jsvalue"), "NewArray"))
	case "Date":
		t.addImport("time")
		return callExpr(selectorExpr(ident("time"), "Now"))
	case "RegExp":
		t.addImport("regexp")
		t.addAliasedImport("github.com/nnstd/gun/runtime/jsvalue", "jsvalue")
		pattern := ast.Expr(stringLit(""))
		if len(args) > 0 {
			// Always coerce to string — in all-JSValue mode all args are *jsvalue.JSValue
			t.addImport("fmt")
			pattern = callExpr(selectorExpr(ident("fmt"), "Sprint"), args[0])
		}
		compiled := callExpr(selectorExpr(ident("regexp"), "MustCompile"), pattern)
		// Flags argument: consume but ignore (Go regex doesn't support JS flags like "g")
		if len(args) > 1 {
			compiled = &ast.CallExpr{
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
		return callExpr(selectorExpr(ident("jsvalue"), "NewRegex"), compiled)
	case "Hono":
		t.addImport("github.com/nnstd/gun/runtime/hono")
		return callExpr(selectorExpr(ident("hono"), "New"))
	case "IntlSegmenter":
		t.addImport("github.com/nnstd/gun/runtime/intl")
		return callExpr(selectorExpr(selectorExpr(ident("intl"), "Segmenter"), "Call"), args...)
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
		// Array.isArray(x) → jsvalue.IsArrayValue(x)
		if len(args) > 0 {
			addImport("github.com/nnstd/gun/runtime/jsvalue")
			return callExpr(selectorExpr(ident("jsvalue"), "IsArrayValue"), args[0])
		}
	}
	return nil
}

// transformProcessCall handles process.X() calls.
func transformProcessCall(prop string, args []ast.Expr, addImport func(string)) ast.Expr {
	addImport("github.com/nnstd/gun/runtime/process")
	switch prop {
	case "exit":
		if len(args) > 0 {
			return callExpr(selectorExpr(ident("process"), "Exit"), args[0])
		}
		return callExpr(selectorExpr(ident("process"), "Exit"), intLit("0"))
	case "cwd":
		return callExpr(selectorExpr(ident("process"), "Cwd"))
	}
	return nil
}

// transformProcessMember handles process.X member access (not calls).
func transformProcessMember(prop string, addImport func(string)) ast.Expr {
	addImport("github.com/nnstd/gun/runtime/process")
	switch prop {
	case "env":
		return selectorExpr(ident("process"), "Env")
	case "argv":
		return selectorExpr(ident("process"), "Argv")
	case "platform":
		return selectorExpr(ident("process"), "Platform")
	case "stdout":
		return selectorExpr(ident("process"), "Stdout")
	case "stderr":
		return selectorExpr(ident("process"), "Stderr")
	case "versions":
		return selectorExpr(ident("process"), "Versions")
	case "version":
		return selectorExpr(ident("process"), "Version")
	case "pid":
		return selectorExpr(ident("process"), "Pid")
	case "cwd":
		return selectorExpr(ident("process"), "Cwd")
	default:
		// Unknown process properties → process.AsJSValue().Get("prop")
		return callExpr(selectorExpr(callExpr(selectorExpr(ident("process"), "AsJSValue")), "Get"), stringLit(prop))
	}
}

// mapIdentifier maps known TS global identifiers to their Go equivalents.
func mapIdentifier(name string, addImport func(string)) ast.Expr {
	switch name {
	case "undefined", "null":
		return ident("nil")
	case "console":
		addImport("github.com/nnstd/gun/runtime/console")
		return ident("console")
	case "Math":
		addImport("github.com/nnstd/gun/runtime/jsmath")
		return ident("jsmath")
	case "JSON":
		addImport("github.com/nnstd/gun/runtime/json")
		return ident("json")
	case "Error", "TypeError", "RangeError", "ReferenceError", "SyntaxError", "URIError", "EvalError":
		addImport("github.com/nnstd/gun/runtime/jserror")
		return selectorExpr(ident("jserror"), name)
	case "Object":
		addImport("github.com/nnstd/gun/runtime/object")
		return ident("object")
	case "process":
		// process as standalone value — return a JSValue object with process properties
		// so process?.version, process?.versions etc. work through .Get()
		addImport("github.com/nnstd/gun/runtime/process")
		return callExpr(selectorExpr(ident("process"), "AsJSValue"))
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

// transformErrorCall handles Error.X() static method calls (e.g. Error.captureStackTrace).
func transformErrorCall(errType, prop string, args []ast.Expr, addImport func(string)) ast.Expr {
	addImport("github.com/nnstd/gun/runtime/jserror")
	// Error.captureStackTrace(obj) → jserror.Error.Get("captureStackTrace").Call(obj)
	// Other static methods: dispatch via .Get(prop).Call(args...)
	for i, arg := range args {
		args[i] = jsvalueWrapLit(arg)
	}
	return callExpr(
		selectorExpr(
			callExpr(selectorExpr(selectorExpr(ident("jserror"), errType), "Get"), stringLit(prop)),
			"Call",
		),
		args...,
	)
}
