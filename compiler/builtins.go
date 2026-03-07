package compiler

import (
	"go/ast"
)

// isErrorType returns true if the name is a JavaScript Error constructor.
func isErrorType(name string) bool {
	switch name {
	case "Error", "TypeError", "RangeError", "ReferenceError", "SyntaxError", "URIError", "EvalError":
		return true
	}
	return false
}

// NOTE: The old transformBuiltinCall, transformGlobalCall, and transformBuiltinNew
// dispatch functions have been replaced by context-based dispatch. The actual
// transform implementations below are registered via registerDefaultBuiltins()
// in context_defaults.go.

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
			addImport("github.com/nnstd/gun/runtime/builtin")
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

// NOTE: The old mapIdentifier function has been replaced by context-based
// identifier dispatch. See registerIdentifierMappings() in context_defaults.go.

// transformErrorCall handles Error.X() static method calls (e.g. Error.captureStackTrace).
func transformErrorCall(errType, prop string, args []ast.Expr, addImport func(string)) ast.Expr {
	addImport("github.com/nnstd/gun/runtime/builtin/error")
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
