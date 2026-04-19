package compiler

import (
	"go/ast"
)

// NOTE: The old transformBuiltinCall, transformGlobalCall, and transformBuiltinNew
// dispatch functions have been replaced by context-based dispatch. The actual
// transform implementations below are registered via RegisterDefaultBuiltins()
// in context_defaults.go.

// transformNumberCall handles Number.X() calls.
func transformNumberCall(prop string, args []ast.Expr) ast.Expr {
	for i, arg := range args {
		args[i] = jsvalueWrapLit(arg)
	}
	return callExpr(
		selectorExpr(
			callExpr(selectorExpr(selectorExpr(ident("jsvalue"), "Number"), "Get"), stringLit(prop)),
			"Call",
		),
		args...,
	)
}

// transformArrayCall handles Array.X() calls.
func transformArrayCall(prop string, args []ast.Expr, addImport func(string)) ast.Expr {
	addImport("github.com/nnstd/gun/runtime/builtin")
	for i, arg := range args {
		args[i] = jsvalueWrapLit(arg)
	}
	return callExpr(
		selectorExpr(
			callExpr(selectorExpr(selectorExpr(ident("jsvalue"), "Array"), "Get"), stringLit(prop)),
			"Call",
		),
		args...,
	)
}

// transformProcessCall handles process.X() calls.
func transformProcessCall(prop string, args []ast.Expr, addImport func(string)) ast.Expr {
	addImport("github.com/nnstd/gun/runtime/process")
	for i, arg := range args {
		args[i] = jsvalueWrapLit(arg)
	}
	return callExpr(
		selectorExpr(
			callExpr(selectorExpr(callExpr(selectorExpr(ident("process"), "AsJSValue")), "Get"), stringLit(prop)),
			"Call",
		),
		args...,
	)
}

// transformProcessMember handles process.X member access (not calls).
func transformProcessMember(prop string, addImport func(string)) ast.Expr {
	addImport("github.com/nnstd/gun/runtime/process")
	return callExpr(selectorExpr(callExpr(selectorExpr(ident("process"), "AsJSValue")), "Get"), stringLit(prop))
}

// NOTE: The old mapIdentifier function has been replaced by context-based
// identifier dispatch. See registerIdentifierMappings() in context_defaults.go.

// transformErrorCall handles Error.X() static method calls (e.g. Error.captureStackTrace).
func transformErrorCall(errType, prop string, args []ast.Expr, addImport func(string)) ast.Expr {
	addImport("github.com/nnstd/gun/runtime/builtin/error")
	// Error.captureStackTrace(obj) → error.Error.Get("captureStackTrace").Call(obj)
	// Other static methods: dispatch via .Get(prop).Call(args...)
	for i, arg := range args {
		args[i] = jsvalueWrapLit(arg)
	}
	return callExpr(
		selectorExpr(
			callExpr(selectorExpr(selectorExpr(ident("error"), errType), "Get"), stringLit(prop)),
			"Call",
		),
		args...,
	)
}
