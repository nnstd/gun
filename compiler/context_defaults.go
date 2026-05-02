package compiler

import (
	"go/ast"
	"go/token"

	tcontext "github.com/nnstd/gun/compiler/context"
)

// RegisterDefaultBuiltins populates a TranspilerContext with all known JavaScript
// global objects, functions, constructors, identifiers, and modules.
//
// This centralizes builtin dispatch instead of scattering it across multiple
// lowering entrypoints.
func RegisterDefaultBuiltins(ctx *tcontext.TranspilerContext) {
	registerGlobalObjects(ctx)
	registerGlobalFunctions(ctx)
	registerConstructors(ctx)
	registerIdentifierMappings(ctx)
	registerModules(ctx)
	registerKnownGlobals(ctx)

}

// registerGlobalObjects registers objects like console, Math, JSON, Object, etc.
// These handle obj.method(args) calls.
func registerGlobalObjects(ctx *tcontext.TranspilerContext) {
	ctx.RegisterGlobal(&tcontext.GlobalObject{
		Name: "console",
		TransformCall: func(method string, args []ast.Expr, hasSpread bool, imp tcontext.Imports) ast.Expr {
			return transformConsoleCall(method, args, hasSpread, imp)
		},
	})

	ctx.RegisterGlobal(&tcontext.GlobalObject{
		Name: "Math",
		TransformCall: func(method string, args []ast.Expr, _ bool, imp tcontext.Imports) ast.Expr {
			return transformMathCall(method, args, imp.AddImport)
		},
	})

	ctx.RegisterGlobal(&tcontext.GlobalObject{
		Name: "JSON",
		TransformCall: func(method string, args []ast.Expr, _ bool, imp tcontext.Imports) ast.Expr {
			return transformJSONCall(method, args, imp)
		},
	})

	ctx.RegisterGlobal(&tcontext.GlobalObject{
		Name: "Object",
		TransformCall: func(method string, args []ast.Expr, _ bool, imp tcontext.Imports) ast.Expr {
			return transformObjectCall(method, args, imp.AddImport)
		},
	})

	ctx.RegisterGlobal(&tcontext.GlobalObject{
		Name: "Number",
		TransformCall: func(method string, args []ast.Expr, _ bool, imp tcontext.Imports) ast.Expr {
			return transformNumberCall(method, args)
		},
	})

	ctx.RegisterGlobal(&tcontext.GlobalObject{
		Name: "process",
		TransformCall: func(method string, args []ast.Expr, _ bool, imp tcontext.Imports) ast.Expr {
			return transformProcessCall(method, args, imp.AddImport)
		},
		TransformMember: func(prop string, imp tcontext.Imports) ast.Expr {
			return transformProcessMember(prop, imp.AddImport)
		},
	})

	ctx.RegisterGlobal(&tcontext.GlobalObject{
		Name: "Array",
		TransformCall: func(method string, args []ast.Expr, _ bool, imp tcontext.Imports) ast.Expr {
			return transformArrayCall(method, args, imp.AddImport)
		},
	})

	ctx.RegisterGlobal(&tcontext.GlobalObject{
		Name: "Bun",
		TransformCall: func(method string, args []ast.Expr, _ bool, imp tcontext.Imports) ast.Expr {
			imp.AddImport("github.com/nnstd/gun/runtime/bun")
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			switch method {
			case "serve":
				if len(args) > 0 {
					opts := args[0]
					// If opts is ObjectFrom("fetch", X.Get("fetch"), ...),
					return callExpr(
						selectorExpr(
							callExpr(selectorExpr(selectorExpr(ident("bun"), "AsJSValue"), "Get"), stringLit("serve")),
							"Call",
						),
						jsvalueWrapLit(opts),
					)
				}
				return callExpr(
					selectorExpr(
						callExpr(selectorExpr(selectorExpr(ident("bun"), "AsJSValue"), "Get"), stringLit("serve")),
						"Call",
					),
					callExpr(selectorExpr(ident("jsvalue"), "NewObject")),
				)
			default:
				return nil
			}
		},
		TransformMember: func(prop string, imp tcontext.Imports) ast.Expr {
			imp.AddImport("github.com/nnstd/gun/runtime/bun")
			return callExpr(selectorExpr(selectorExpr(ident("bun"), "AsJSValue"), "Get"), stringLit(prop))
		},
	})

	ctx.RegisterGlobal(&tcontext.GlobalObject{
		Name: "Promise",
		TransformCall: func(method string, args []ast.Expr, _ bool, imp tcontext.Imports) ast.Expr {
			imp.AddImport("github.com/nnstd/gun/runtime/promise")
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			for i, arg := range args {
				args[i] = jsvalueWrapLit(arg)
			}
			return callExpr(
				selectorExpr(
					callExpr(selectorExpr(selectorExpr(ident("promise"), "Promise"), "Get"), stringLit(method)),
					"Call",
				),
				args...,
			)
		},
	})

	// Error types as global objects (for static methods like Error.captureStackTrace)
	for _, errType := range []string{"Error", "TypeError", "RangeError", "ReferenceError", "SyntaxError", "URIError", "EvalError"} {
		et := errType // capture
		ctx.RegisterGlobal(&tcontext.GlobalObject{
			Name: et,
			TransformCall: func(method string, args []ast.Expr, _ bool, imp tcontext.Imports) ast.Expr {
				return transformErrorCall(et, method, args, imp.AddImport)
			},
		})
	}
}

// registerGlobalFunctions registers bare global function calls like parseInt, isNaN.
func registerGlobalFunctions(ctx *tcontext.TranspilerContext) {
	ctx.RegisterGlobalFunc(&tcontext.GlobalFunction{
		Name: "fetch",
		Transform: func(args []ast.Expr, imp tcontext.Imports) ast.Expr {
			imp.AddImport("github.com/nnstd/gun/runtime/web")
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			for i, arg := range args {
				args[i] = jsvalueWrapLit(arg)
			}
			return callExpr(selectorExpr(selectorExpr(ident("web"), "Fetch"), "Call"), args...)
		},
	})

	ctx.RegisterGlobalFunc(&tcontext.GlobalFunction{
		Name: "isNaN",
		Transform: func(args []ast.Expr, imp tcontext.Imports) ast.Expr {
			imp.AddImport("github.com/nnstd/gun/runtime/builtin/math")
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			if len(args) > 0 {
				return callExpr(selectorExpr(ident("math"), "IsNaN"), jsvalueWrapLit(args[0]))
			}
			return callExpr(selectorExpr(ident("jsvalue"), "NewBool"), ident("false"))
		},
	})

	ctx.RegisterGlobalFunc(&tcontext.GlobalFunction{
		Name: "isFinite",
		Transform: func(args []ast.Expr, imp tcontext.Imports) ast.Expr {
			imp.AddImport("github.com/nnstd/gun/runtime/builtin/math")
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			if len(args) > 0 {
				return callExpr(selectorExpr(ident("math"), "IsFinite"), jsvalueWrapLit(args[0]))
			}
			return callExpr(selectorExpr(ident("jsvalue"), "NewBool"), ident("true"))
		},
	})

	ctx.RegisterGlobalFunc(&tcontext.GlobalFunction{
		Name: "Number",
		Transform: func(args []ast.Expr, imp tcontext.Imports) ast.Expr {
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			for i, arg := range args {
				args[i] = jsvalueWrapLit(arg)
			}
			return callExpr(selectorExpr(selectorExpr(ident("jsvalue"), "Number"), "Call"), args...)
		},
	})

	ctx.RegisterGlobalFunc(&tcontext.GlobalFunction{
		Name: "Array",
		Transform: func(args []ast.Expr, imp tcontext.Imports) ast.Expr {
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			for i, arg := range args {
				args[i] = jsvalueWrapLit(arg)
			}
			return callExpr(selectorExpr(selectorExpr(ident("jsvalue"), "Array"), "Call"), args...)
		},
	})

	ctx.RegisterGlobalFunc(&tcontext.GlobalFunction{
		Name: "Symbol",
		Transform: func(args []ast.Expr, imp tcontext.Imports) ast.Expr {
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			if len(args) > 0 {
				imp.AddAliasedImport("fmt", "_gunFmt")
				return callExpr(selectorExpr(ident("jsvalue"), "NewSymbol"),
					callExpr(selectorExpr(ident("_gunFmt"), "Sprint"), args[0]))
			}
			return callExpr(selectorExpr(ident("jsvalue"), "NewSymbol"), stringLit(""))
		},
	})

	ctx.RegisterGlobalFunc(&tcontext.GlobalFunction{
		Name: "String",
		Transform: func(args []ast.Expr, imp tcontext.Imports) ast.Expr {
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			for i, arg := range args {
				args[i] = jsvalueWrapLit(arg)
			}
			return callExpr(selectorExpr(selectorExpr(ident("jsvalue"), "String"), "Call"), args...)
		},
	})

	ctx.RegisterGlobalFunc(&tcontext.GlobalFunction{
		Name: "BigInt",
		Transform: func(args []ast.Expr, imp tcontext.Imports) ast.Expr {
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			for i, arg := range args {
				args[i] = jsvalueWrapLit(arg)
			}
			return callExpr(selectorExpr(selectorExpr(ident("jsvalue"), "BigIntCtor"), "Call"), args...)
		},
	})

	ctx.RegisterGlobalFunc(&tcontext.GlobalFunction{
		Name: "parseInt",
		Transform: func(args []ast.Expr, imp tcontext.Imports) ast.Expr {
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			if len(args) >= 2 {
				return callExpr(selectorExpr(ident("jsvalue"), "ParseInt"), jsvalueWrapLit(args[0]), jsvalueWrapLit(args[1]))
			}
			if len(args) == 1 {
				return callExpr(selectorExpr(ident("jsvalue"), "ParseInt"), jsvalueWrapLit(args[0]), callExpr(selectorExpr(ident("jsvalue"), "NewNumber"), floatLit("10")))
			}
			return callExpr(selectorExpr(ident("jsvalue"), "NewNumber"), floatLit("0"))
		},
	})

	ctx.RegisterGlobalFunc(&tcontext.GlobalFunction{
		Name: "parseFloat",
		Transform: func(args []ast.Expr, imp tcontext.Imports) ast.Expr {
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			if len(args) >= 1 {
				return callExpr(selectorExpr(ident("jsvalue"), "ParseFloat"), jsvalueWrapLit(args[0]))
			}
			return callExpr(selectorExpr(ident("jsvalue"), "NewNumber"), floatLit("0"))
		},
	})

	// Timer functions: setTimeout, setInterval, setImmediate, clearTimeout, clearInterval, clearImmediate
	for _, timerFn := range []string{"setTimeout", "setInterval", "setImmediate", "clearTimeout", "clearInterval", "clearImmediate"} {
		fn := timerFn
		goFn := capitalize(fn)
		ctx.RegisterGlobalFunc(&tcontext.GlobalFunction{
			Name: fn,
			Transform: func(args []ast.Expr, imp tcontext.Imports) ast.Expr {
				imp.AddAliasedImport("github.com/nnstd/gun/runtime/eventloop", "eventloop")
				wrapped := make([]ast.Expr, len(args))
				for i, a := range args {
					wrapped[i] = jsvalueWrapLit(a)
				}
				return callExpr(selectorExpr(ident("eventloop"), goFn), wrapped...)
			},
		})
	}

	for _, fnName := range []string{"decodeURI", "decodeURIComponent", "encodeURI", "encodeURIComponent"} {
		name := fnName
		ctx.RegisterGlobalFunc(&tcontext.GlobalFunction{
			Name: name,
			Transform: func(args []ast.Expr, imp tcontext.Imports) ast.Expr {
				imp.AddImport("github.com/nnstd/gun/runtime/web")
				callName := ""
				switch name {
				case "decodeURI":
					callName = "DecodeURI"
				case "decodeURIComponent":
					callName = "DecodeURIComponent"
				case "encodeURI":
					callName = "EncodeURI"
				case "encodeURIComponent":
					callName = "EncodeURIComponent"
				}
				if len(args) > 0 {
					return callExpr(selectorExpr(ident("web"), callName), jsvalueWrapLit(args[0]))
				}
				return callExpr(selectorExpr(ident("web"), callName), callExpr(selectorExpr(ident("jsvalue"), "NewString"), stringLit("")))
			},
		})
	}

	ctx.RegisterGlobalFunc(&tcontext.GlobalFunction{
		Name: "eval",
		Transform: func(args []ast.Expr, imp tcontext.Imports) ast.Expr {
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/dynfunc", "_gunDyn")
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/jscontext", "_gunCtx")
			if len(args) > 0 {
				return callExpr(selectorExpr(ident("_gunDyn"), "EvalHIR"), callExpr(selectorExpr(ident("_gunCtx"), "Default")), args[0])
			}
			return callExpr(selectorExpr(ident("jsvalue"), "NewUndefined"))
		},
	})

	ctx.RegisterGlobalFunc(&tcontext.GlobalFunction{
		Name: "unescape",
		Transform: func(args []ast.Expr, imp tcontext.Imports) ast.Expr {
			imp.AddImport("github.com/nnstd/gun/runtime/web")
			if len(args) > 0 {
				return callExpr(selectorExpr(ident("web"), "Unescape"), jsvalueWrapLit(args[0]))
			}
			return callExpr(selectorExpr(ident("web"), "Unescape"), callExpr(selectorExpr(ident("jsvalue"), "NewString"), stringLit("")))
		},
	})

	ctx.RegisterGlobalFunc(&tcontext.GlobalFunction{
		Name: "addEventListener",
		Transform: func(args []ast.Expr, imp tcontext.Imports) ast.Expr {
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			return callExpr(selectorExpr(ident("jsvalue"), "NewUndefined"))
		},
	})

	// Error types as functions (Error() without new)
	for _, errType := range []string{"Error", "TypeError", "RangeError", "ReferenceError", "SyntaxError", "URIError", "EvalError"} {
		et := errType
		ctx.RegisterGlobalFunc(&tcontext.GlobalFunction{
			Name: et,
			Transform: func(args []ast.Expr, imp tcontext.Imports) ast.Expr {
				imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin/error", "error")
				imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
				for i, arg := range args {
					args[i] = jsvalueWrapLit(arg)
				}
				return callExpr(selectorExpr(selectorExpr(ident("error"), et), "Call"), args...)
			},
		})
	}
}

// registerConstructors registers new X() transformations.
func registerConstructors(ctx *tcontext.TranspilerContext) {
	// Error types
	for _, errType := range []string{"Error", "TypeError", "RangeError", "ReferenceError", "SyntaxError", "URIError", "EvalError"} {
		et := errType
		ctx.RegisterConstructor(&tcontext.Constructor{
			Name: et,
			Transform: func(args []ast.Expr, imp tcontext.Imports) ast.Expr {
				imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin/error", "error")
				imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
				for i, arg := range args {
					args[i] = jsvalueWrapLit(arg)
				}
				return callExpr(selectorExpr(selectorExpr(ident("error"), et), "Call"), args...)
			},
		})
	}

	ctx.RegisterConstructor(&tcontext.Constructor{
		Name: "Map",
		Transform: func(args []ast.Expr, imp tcontext.Imports) ast.Expr {
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			return callExpr(selectorExpr(ident("jsvalue"), "NewMap"))
		},
	})

	ctx.RegisterConstructor(&tcontext.Constructor{
		Name: "WeakMap",
		Transform: func(args []ast.Expr, imp tcontext.Imports) ast.Expr {
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			return callExpr(selectorExpr(ident("jsvalue"), "NewMap"))
		},
	})

	ctx.RegisterConstructor(&tcontext.Constructor{
		Name: "Set",
		Transform: func(args []ast.Expr, imp tcontext.Imports) ast.Expr {
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			return callExpr(selectorExpr(ident("jsvalue"), "NewSet"))
		},
	})

	ctx.RegisterConstructor(&tcontext.Constructor{
		Name: "WeakSet",
		Transform: func(args []ast.Expr, imp tcontext.Imports) ast.Expr {
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			return callExpr(selectorExpr(ident("jsvalue"), "NewSet"))
		},
	})

	ctx.RegisterConstructor(&tcontext.Constructor{
		Name: "Array",
		Transform: func(args []ast.Expr, imp tcontext.Imports) ast.Expr {
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			return callExpr(selectorExpr(ident("jsvalue"), "NewArray"))
		},
	})

	// Date uses the identifier mapping to jsvalue.DateCtor.
	// new Date() becomes DateCtor.New() which returns a proper JSValue.
	// No constructor transform needed — the identifier path handles it.

	ctx.RegisterConstructor(&tcontext.Constructor{
		Name: "RegExp",
		Transform: func(args []ast.Expr, imp tcontext.Imports) ast.Expr {
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			pattern := ast.Expr(stringLit(""))
			if len(args) > 0 {
				imp.AddAliasedImport("fmt", "_gunFmt")
				pattern = callExpr(selectorExpr(ident("_gunFmt"), "Sprint"), args[0])
			}
			compiled := callExpr(selectorExpr(ident("jsvalue"), "CompileRegex"), pattern)
			if len(args) > 1 {
				compiled = &ast.CallExpr{
					Fun: &ast.FuncLit{
						Type: &ast.FuncType{
							Params:  fieldList(),
							Results: fieldList(field("", selectorExpr(ident("jsvalue"), "GoRegex"))),
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
		},
	})

	ctx.RegisterConstructor(&tcontext.Constructor{
		Name: "Promise",
		Transform: func(args []ast.Expr, imp tcontext.Imports) ast.Expr {
			imp.AddImport("github.com/nnstd/gun/runtime/promise")
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			for i, arg := range args {
				args[i] = jsvalueWrapLit(arg)
			}
			return callExpr(selectorExpr(selectorExpr(ident("promise"), "Promise"), "Call"), args...)
		},
	})

	ctx.RegisterConstructor(&tcontext.Constructor{
		Name: "IntlSegmenter",
		Transform: func(args []ast.Expr, imp tcontext.Imports) ast.Expr {
			imp.AddImport("github.com/nnstd/gun/runtime/builtin/intl")
			return callExpr(selectorExpr(selectorExpr(ident("intl"), "Segmenter"), "Call"), args...)
		},
	})

	for _, ctorName := range []string{"URL", "URLSearchParams"} {
		name := ctorName
		ctx.RegisterConstructor(&tcontext.Constructor{
			Name: name,
			Transform: func(args []ast.Expr, imp tcontext.Imports) ast.Expr {
				imp.AddAliasedImport("github.com/nnstd/gun/runtime/url", "_gunUrl")
				for i, arg := range args {
					args[i] = jsvalueWrapLit(arg)
				}
				return callExpr(selectorExpr(selectorExpr(ident("_gunUrl"), name+"Constructor"), "Call"), args...)
			},
		})
	}

	for _, ctorName := range []string{"Headers", "Request", "Response", "File", "FormData"} {
		name := ctorName
		ctx.RegisterConstructor(&tcontext.Constructor{
			Name: name,
			Transform: func(args []ast.Expr, imp tcontext.Imports) ast.Expr {
				imp.AddImport("github.com/nnstd/gun/runtime/web")
				return callExpr(selectorExpr(selectorExpr(ident("web"), name), "Call"), args...)
			},
		})
	}
}

// registerIdentifierMappings registers global identifiers and their Go equivalents.
func registerIdentifierMappings(ctx *tcontext.TranspilerContext) {
	ctx.RegisterIdentifier(&tcontext.IdentifierMapping{
		Name: "undefined",
		Transform: func(imp tcontext.Imports) ast.Expr {
			return ident("nil")
		},
	})

	ctx.RegisterIdentifier(&tcontext.IdentifierMapping{
		Name: "null",
		Transform: func(imp tcontext.Imports) ast.Expr {
			return ident("nil")
		},
	})

	ctx.RegisterIdentifier(&tcontext.IdentifierMapping{
		Name: "Infinity",
		Transform: func(imp tcontext.Imports) ast.Expr {
			imp.AddImport("github.com/nnstd/gun/runtime/builtin/math")
			return callExpr(selectorExpr(ident("math"), "Inf"))
		},
	})

	ctx.RegisterIdentifier(&tcontext.IdentifierMapping{
		Name: "NaN",
		Transform: func(imp tcontext.Imports) ast.Expr {
			imp.AddImport("github.com/nnstd/gun/runtime/builtin/math")
			return callExpr(selectorExpr(ident("math"), "NaN"))
		},
	})

	ctx.RegisterIdentifier(&tcontext.IdentifierMapping{
		Name: "console",
		Transform: func(imp tcontext.Imports) ast.Expr {
			imp.AddImport("github.com/nnstd/gun/runtime/builtin/console")
			return selectorExpr(ident("console"), "AsJSValue")
		},
	})

	ctx.RegisterIdentifier(&tcontext.IdentifierMapping{
		Name: "Math",
		Transform: func(imp tcontext.Imports) ast.Expr {
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin/math", "math")
			return selectorExpr(ident("math"), "AsJSValue")
		},
	})

	ctx.RegisterIdentifier(&tcontext.IdentifierMapping{
		Name: "JSON",
		Transform: func(imp tcontext.Imports) ast.Expr {
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin/json", "_gunJSON")
			return selectorExpr(ident("_gunJSON"), "AsJSValue")
		},
	})

	for _, errType := range []string{"Error", "TypeError", "RangeError", "ReferenceError", "SyntaxError", "URIError", "EvalError"} {
		et := errType
		ctx.RegisterIdentifier(&tcontext.IdentifierMapping{
			Name: et,
			Transform: func(imp tcontext.Imports) ast.Expr {
				imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin/error", "error")
				return selectorExpr(ident("error"), et)
			},
		})
	}

	ctx.RegisterIdentifier(&tcontext.IdentifierMapping{
		Name: "Object",
		Transform: func(imp tcontext.Imports) ast.Expr {
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			return selectorExpr(ident("jsvalue"), "Object")
		},
	})

	ctx.RegisterIdentifier(&tcontext.IdentifierMapping{
		Name: "process",
		Transform: func(imp tcontext.Imports) ast.Expr {
			imp.AddImport("github.com/nnstd/gun/runtime/process")
			return callExpr(selectorExpr(ident("process"), "AsJSValue"))
		},
	})

	ctx.RegisterIdentifier(&tcontext.IdentifierMapping{
		Name: "performance",
		Transform: func(imp tcontext.Imports) ast.Expr {
			imp.AddImport("github.com/nnstd/gun/runtime/perf_hooks")
			return selectorExpr(ident("perf_hooks"), "Performance")
		},
	})

	ctx.RegisterIdentifier(&tcontext.IdentifierMapping{
		Name: "Array",
		Transform: func(imp tcontext.Imports) ast.Expr {
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			return selectorExpr(ident("jsvalue"), "Array")
		},
	})

	ctx.RegisterIdentifier(&tcontext.IdentifierMapping{
		Name: "String",
		Transform: func(imp tcontext.Imports) ast.Expr {
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			return selectorExpr(ident("jsvalue"), "String")
		},
	})

	ctx.RegisterIdentifier(&tcontext.IdentifierMapping{
		Name: "Promise",
		Transform: func(imp tcontext.Imports) ast.Expr {
			imp.AddImport("github.com/nnstd/gun/runtime/promise")
			return selectorExpr(ident("promise"), "Promise")
		},
	})

	ctx.RegisterIdentifier(&tcontext.IdentifierMapping{
		Name: "Number",
		Transform: func(imp tcontext.Imports) ast.Expr {
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			return selectorExpr(ident("jsvalue"), "Number")
		},
	})

	ctx.RegisterIdentifier(&tcontext.IdentifierMapping{
		Name: "Boolean",
		Transform: func(imp tcontext.Imports) ast.Expr {
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			return selectorExpr(ident("jsvalue"), "Boolean")
		},
	})

	ctx.RegisterIdentifier(&tcontext.IdentifierMapping{
		Name: "module",
		Transform: func(imp tcontext.Imports) ast.Expr {
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			return callExpr(selectorExpr(ident("jsvalue"), "NewObject"))
		},
	})

	ctx.RegisterIdentifier(&tcontext.IdentifierMapping{
		Name: "Intl",
		Transform: func(imp tcontext.Imports) ast.Expr {
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			return callExpr(selectorExpr(ident("jsvalue"), "NewObject"))
		},
	})

	ctx.RegisterIdentifier(&tcontext.IdentifierMapping{
		Name: "BigInt",
		Transform: func(imp tcontext.Imports) ast.Expr {
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			return selectorExpr(ident("jsvalue"), "BigIntCtor")
		},
	})

	ctx.RegisterIdentifier(&tcontext.IdentifierMapping{
		Name: "Symbol",
		Transform: func(imp tcontext.Imports) ast.Expr {
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			return selectorExpr(ident("jsvalue"), "Symbol_")
		},
	})

	ctx.RegisterIdentifier(&tcontext.IdentifierMapping{
		Name: "Reflect",
		Transform: func(imp tcontext.Imports) ast.Expr {
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			return selectorExpr(ident("jsvalue"), "Reflect")
		},
	})

	ctx.RegisterIdentifier(&tcontext.IdentifierMapping{
		Name: "Bun",
		Transform: func(imp tcontext.Imports) ast.Expr {
			imp.AddImport("github.com/nnstd/gun/runtime/bun")
			return selectorExpr(ident("bun"), "AsJSValue")
		},
	})

	for _, identName := range []string{"URL", "URLSearchParams"} {
		name := identName
		ctx.RegisterIdentifier(&tcontext.IdentifierMapping{
			Name: name,
			Transform: func(imp tcontext.Imports) ast.Expr {
				imp.AddAliasedImport("github.com/nnstd/gun/runtime/url", "_gunUrl")
				return selectorExpr(ident("_gunUrl"), name+"Constructor")
			},
		})
	}

	for _, identName := range []string{"Headers", "Request", "Response", "File", "FormData", "RegExp", "fetch"} {
		name := identName
		ctx.RegisterIdentifier(&tcontext.IdentifierMapping{
			Name: name,
			Transform: func(imp tcontext.Imports) ast.Expr {
				if name == "RegExp" {
					imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
					return selectorExpr(ident("jsvalue"), "RegexpCtor")
				}
				imp.AddImport("github.com/nnstd/gun/runtime/web")
				if name == "fetch" {
					return selectorExpr(ident("web"), "Fetch")
				}
				return selectorExpr(ident("web"), name)
			},
		})
	}

	ctx.RegisterIdentifier(&tcontext.IdentifierMapping{
		Name: "require",
		Transform: func(imp tcontext.Imports) ast.Expr {
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			imp.AddImport("github.com/nnstd/gun/runtime/module")
			imp.AddImport("github.com/nnstd/gun/runtime/process")
			return callExpr(selectorExpr(ident("module"), "CreateRequire"),
				callExpr(selectorExpr(ident("jsvalue"), "NewString"),
					callExpr(selectorExpr(ident("process"), "GetEntryScript"))))
		},
	})

	ctx.RegisterIdentifier(&tcontext.IdentifierMapping{
		Name: "__filename",
		Transform: func(imp tcontext.Imports) ast.Expr {
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			imp.AddImport("github.com/nnstd/gun/runtime/process")
			return callExpr(selectorExpr(ident("jsvalue"), "NewString"),
				callExpr(selectorExpr(ident("process"), "GetEntryScript")))
		},
	})

	ctx.RegisterIdentifier(&tcontext.IdentifierMapping{
		Name: "__dirname",
		Transform: func(imp tcontext.Imports) ast.Expr {
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			imp.AddImport("github.com/nnstd/gun/runtime/process")
			return callExpr(selectorExpr(ident("jsvalue"), "NewString"),
				callExpr(selectorExpr(ident("process"), "GetEntryDir")))
		},
	})

	ctx.RegisterIdentifier(&tcontext.IdentifierMapping{
		Name: "globalThis",
		Transform: func(imp tcontext.Imports) ast.Expr {
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/jscontext", "jscontext")
			imp.SetNeedsGlobalSync()
			return callExpr(selectorExpr(
				callExpr(selectorExpr(ident("jscontext"), "Default")),
				"Global",
			))
		},
	})

	for _, identName := range []string{"decodeURI", "decodeURIComponent", "encodeURI", "encodeURIComponent"} {
		name := identName
		ctx.RegisterIdentifier(&tcontext.IdentifierMapping{
			Name: name,
			Transform: func(imp tcontext.Imports) ast.Expr {
				imp.AddImport("github.com/nnstd/gun/runtime/web")
				imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
				callName := ""
				switch name {
				case "decodeURI":
					callName = "DecodeURI"
				case "decodeURIComponent":
					callName = "DecodeURIComponent"
				case "encodeURI":
					callName = "EncodeURI"
				}
				return callExpr(selectorExpr(ident("jsvalue"), "NewFunction"), &ast.FuncLit{
					Type: &ast.FuncType{
						Params:  fieldList(&ast.Field{Names: []*ast.Ident{ident("_a")}, Type: &ast.Ellipsis{Elt: ptrType(selectorExpr(ident("jsvalue"), "JSValue"))}}),
						Results: fieldList(field("", ptrType(selectorExpr(ident("jsvalue"), "JSValue")))),
					},
					Body: blockStmt(
						returnStmt(callExpr(selectorExpr(ident("web"), callName), &ast.IndexExpr{X: ident("_a"), Index: intLit("0")})),
					),
				})
			},
		})
	}

	ctx.RegisterIdentifier(&tcontext.IdentifierMapping{
		Name: "Date",
		Transform: func(imp tcontext.Imports) ast.Expr {
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			return selectorExpr(ident("jsvalue"), "DateCtor")
		},
	})

	ctx.RegisterIdentifier(&tcontext.IdentifierMapping{
		Name: "Map",
		Transform: func(imp tcontext.Imports) ast.Expr {
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			return selectorExpr(ident("jsvalue"), "MapCtor")
		},
	})

	ctx.RegisterIdentifier(&tcontext.IdentifierMapping{
		Name: "Set",
		Transform: func(imp tcontext.Imports) ast.Expr {
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			return selectorExpr(ident("jsvalue"), "SetCtor")
		},
	})

	ctx.RegisterIdentifier(&tcontext.IdentifierMapping{
		Name: "Uint8Array",
		Transform: func(imp tcontext.Imports) ast.Expr {
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			return selectorExpr(ident("jsvalue"), "Uint8ArrayCtor")
		},
	})

	ctx.RegisterIdentifier(&tcontext.IdentifierMapping{
		Name: "Int8Array",
		Transform: func(imp tcontext.Imports) ast.Expr {
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			return callExpr(selectorExpr(ident("jsvalue"), "GetTypedArrayCtor"), stringLit("Int8Array"))
		},
	})
	ctx.RegisterIdentifier(&tcontext.IdentifierMapping{
		Name: "Uint8ClampedArray",
		Transform: func(imp tcontext.Imports) ast.Expr {
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			return callExpr(selectorExpr(ident("jsvalue"), "GetTypedArrayCtor"), stringLit("Uint8ClampedArray"))
		},
	})
	ctx.RegisterIdentifier(&tcontext.IdentifierMapping{
		Name: "Int16Array",
		Transform: func(imp tcontext.Imports) ast.Expr {
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			return callExpr(selectorExpr(ident("jsvalue"), "GetTypedArrayCtor"), stringLit("Int16Array"))
		},
	})
	ctx.RegisterIdentifier(&tcontext.IdentifierMapping{
		Name: "Uint16Array",
		Transform: func(imp tcontext.Imports) ast.Expr {
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			return callExpr(selectorExpr(ident("jsvalue"), "GetTypedArrayCtor"), stringLit("Uint16Array"))
		},
	})
	ctx.RegisterIdentifier(&tcontext.IdentifierMapping{
		Name: "Int32Array",
		Transform: func(imp tcontext.Imports) ast.Expr {
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			return callExpr(selectorExpr(ident("jsvalue"), "GetTypedArrayCtor"), stringLit("Int32Array"))
		},
	})
	ctx.RegisterIdentifier(&tcontext.IdentifierMapping{
		Name: "Uint32Array",
		Transform: func(imp tcontext.Imports) ast.Expr {
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			return callExpr(selectorExpr(ident("jsvalue"), "GetTypedArrayCtor"), stringLit("Uint32Array"))
		},
	})
	ctx.RegisterIdentifier(&tcontext.IdentifierMapping{
		Name: "Float32Array",
		Transform: func(imp tcontext.Imports) ast.Expr {
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			return callExpr(selectorExpr(ident("jsvalue"), "GetTypedArrayCtor"), stringLit("Float32Array"))
		},
	})
	ctx.RegisterIdentifier(&tcontext.IdentifierMapping{
		Name: "Float64Array",
		Transform: func(imp tcontext.Imports) ast.Expr {
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			return callExpr(selectorExpr(ident("jsvalue"), "GetTypedArrayCtor"), stringLit("Float64Array"))
		},
	})
	ctx.RegisterIdentifier(&tcontext.IdentifierMapping{
		Name: "BigInt64Array",
		Transform: func(imp tcontext.Imports) ast.Expr {
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			return callExpr(selectorExpr(ident("jsvalue"), "GetTypedArrayCtor"), &ast.BasicLit{Kind: token.STRING, Value: "BigInt64Array"})
		},
	})
	ctx.RegisterIdentifier(&tcontext.IdentifierMapping{
		Name: "BigUint64Array",
		Transform: func(imp tcontext.Imports) ast.Expr {
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			return callExpr(selectorExpr(ident("jsvalue"), "GetTypedArrayCtor"), &ast.BasicLit{Kind: token.STRING, Value: "BigUint64Array"})
		},
	})


	ctx.RegisterIdentifier(&tcontext.IdentifierMapping{
		Name: "ArrayBuffer",
		Transform: func(imp tcontext.Imports) ast.Expr {
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			return selectorExpr(ident("jsvalue"), "ArrayBufferCtor")
		},
	})

	ctx.RegisterIdentifier(&tcontext.IdentifierMapping{
		Name: "SharedArrayBuffer",
		Transform: func(imp tcontext.Imports) ast.Expr {
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			return selectorExpr(ident("jsvalue"), "SharedArrayBufferCtor")
		},
	})

	ctx.RegisterIdentifier(&tcontext.IdentifierMapping{
		Name: "DataView",
		Transform: func(imp tcontext.Imports) ast.Expr {
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			return selectorExpr(ident("jsvalue"), "DataViewCtor")
		},
	})

	ctx.RegisterIdentifier(&tcontext.IdentifierMapping{
		Name: "Buffer",
		Transform: func(imp tcontext.Imports) ast.Expr {
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/buffer", "_gun_buffer")
			return selectorExpr(ident("_gun_buffer"), "Buffer")
		},
	})

	ctx.RegisterIdentifier(&tcontext.IdentifierMapping{
		Name: "Proxy",
		Transform: func(imp tcontext.Imports) ast.Expr {
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			return callExpr(selectorExpr(ident("jsvalue"), "NewObject"))
		},
	})

	ctx.RegisterIdentifier(&tcontext.IdentifierMapping{
		Name: "atob",
		Transform: func(imp tcontext.Imports) ast.Expr {
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			return selectorExpr(ident("jsvalue"), "Atob")
		},
	})

	ctx.RegisterIdentifier(&tcontext.IdentifierMapping{
		Name: "btoa",
		Transform: func(imp tcontext.Imports) ast.Expr {
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			return selectorExpr(ident("jsvalue"), "Btoa")
		},
	})

	ctx.RegisterIdentifier(&tcontext.IdentifierMapping{
		Name: "parseInt",
		Transform: func(imp tcontext.Imports) ast.Expr {
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			return callExpr(selectorExpr(ident("jsvalue"), "NewFunction"), &ast.FuncLit{
				Type: &ast.FuncType{
					Params:  fieldList(&ast.Field{Names: []*ast.Ident{ident("_a")}, Type: &ast.Ellipsis{Elt: ptrType(selectorExpr(ident("jsvalue"), "JSValue"))}}),
					Results: fieldList(field("", ptrType(selectorExpr(ident("jsvalue"), "JSValue")))),
				},
				Body: &ast.BlockStmt{List: []ast.Stmt{
					&ast.IfStmt{
						Cond: &ast.BinaryExpr{X: callExpr(ident("len"), ident("_a")), Op: token.GTR, Y: intLit("1")},
						Body: &ast.BlockStmt{List: []ast.Stmt{
							&ast.ReturnStmt{Results: []ast.Expr{
								callExpr(selectorExpr(ident("jsvalue"), "ParseInt"), &ast.IndexExpr{X: ident("_a"), Index: intLit("0")}, &ast.IndexExpr{X: ident("_a"), Index: intLit("1")}),
							}},
						}},
					},
					&ast.ReturnStmt{Results: []ast.Expr{
						callExpr(selectorExpr(ident("jsvalue"), "ParseInt"), &ast.IndexExpr{X: ident("_a"), Index: intLit("0")}, callExpr(selectorExpr(ident("jsvalue"), "NewNumber"), floatLit("10"))),
					}},
				}},
			})
		},
	})

	ctx.RegisterIdentifier(&tcontext.IdentifierMapping{
		Name: "unescape",
		Transform: func(imp tcontext.Imports) ast.Expr {
			imp.AddImport("github.com/nnstd/gun/runtime/web")
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			return callExpr(selectorExpr(ident("jsvalue"), "NewFunction"), &ast.FuncLit{
				Type: &ast.FuncType{
					Params:  fieldList(&ast.Field{Names: []*ast.Ident{ident("_a")}, Type: &ast.Ellipsis{Elt: ptrType(selectorExpr(ident("jsvalue"), "JSValue"))}}),
					Results: fieldList(field("", ptrType(selectorExpr(ident("jsvalue"), "JSValue")))),
				},
				Body: &ast.BlockStmt{List: []ast.Stmt{
					&ast.ReturnStmt{Results: []ast.Expr{
						callExpr(selectorExpr(ident("web"), "Unescape"), &ast.IndexExpr{X: ident("_a"), Index: intLit("0")}),
					}},
				}},
			})
		},
	})

	ctx.RegisterIdentifier(&tcontext.IdentifierMapping{
		Name: "navigator",
		Transform: func(imp tcontext.Imports) ast.Expr {
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			return callExpr(selectorExpr(ident("jsvalue"), "NewUndefined"))
		},
	})

	ctx.RegisterIdentifier(&tcontext.IdentifierMapping{
		Name: "Function",
		Transform: func(imp tcontext.Imports) ast.Expr {
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/dynfunc", "_")
			return selectorExpr(ident("jsvalue"), "FunctionCtor")
		},
	})

	ctx.RegisterGlobalFunc(&tcontext.GlobalFunction{
		Name: "Function",
		Transform: func(args []ast.Expr, imp tcontext.Imports) ast.Expr {
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			ctor := selectorExpr(selectorExpr(ident("jsvalue"), "FunctionCtor"), "New")
			return callExpr(ctor, args...)
		},
	})
	ctx.MarkKnownGlobal("Function")

	ctx.RegisterGlobalFunc(&tcontext.GlobalFunction{
		Name: "atob",
		Transform: func(args []ast.Expr, imp tcontext.Imports) ast.Expr {
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			for i, arg := range args {
				args[i] = jsvalueWrapLit(arg)
			}
			return callExpr(selectorExpr(ident("jsvalue"), "AtobFunc"), args...)
		},
	})

	ctx.RegisterGlobalFunc(&tcontext.GlobalFunction{
		Name: "btoa",
		Transform: func(args []ast.Expr, imp tcontext.Imports) ast.Expr {
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			for i, arg := range args {
				args[i] = jsvalueWrapLit(arg)
			}
			return callExpr(selectorExpr(ident("jsvalue"), "BtoaFunc"), args...)
		},
	})

	ctx.RegisterGlobalFunc(&tcontext.GlobalFunction{
		Name: "structuredClone",
		Transform: func(args []ast.Expr, imp tcontext.Imports) ast.Expr {
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			if len(args) == 0 {
				return callExpr(selectorExpr(ident("jsvalue"), "NewUndefined"))
			}
			goArgs := []ast.Expr{jsvalueWrapLit(args[0])}
			if len(args) > 1 {
				goArgs = append(goArgs, jsvalueWrapLit(args[1]))
			}
			return callExpr(selectorExpr(ident("jsvalue"), "StructuredClone"), goArgs...)
		},
	})
}

// registerModules registers TS module → Go package mappings.
func registerModules(ctx *tcontext.TranspilerContext) {
	ctx.RegisterModule("fs", &tcontext.ModuleMapping{
		GoImportPath: "github.com/nnstd/gun/runtime/fs",
		GoPkgName:    "fs",
		UseAsJSValue: true,
	})
	ctx.RegisterModule("fs/promises", &tcontext.ModuleMapping{
		GoImportPath: "github.com/nnstd/gun/runtime/fs",
		GoPkgName:    "fs",
		UseAsJSValue: true,
		SymbolOverrides: map[string]tcontext.SymbolOverride{
			"default": {GoSymbol: "PromisesAsJSValue"},
		},
	})
	ctx.RegisterModule("path", &tcontext.ModuleMapping{
		GoImportPath: "github.com/nnstd/gun/runtime/path",
		GoPkgName:    "nodepath",
		UseAsJSValue: true,
	})
	ctx.RegisterModule("os", &tcontext.ModuleMapping{
		GoImportPath: "github.com/nnstd/gun/runtime/os",
		GoPkgName:    "nodeos",
		UseAsJSValue: true,
	})
	ctx.RegisterModule("http", &tcontext.ModuleMapping{
		GoImportPath: "github.com/nnstd/gun/runtime/http",
		GoPkgName:    "nodehttp",
		UseAsJSValue: true,
	})
	ctx.RegisterModule("https", &tcontext.ModuleMapping{
		GoImportPath: "github.com/nnstd/gun/runtime/http",
		GoPkgName:    "nodehttp",
		UseAsJSValue: true,
		SymbolOverrides: map[string]tcontext.SymbolOverride{
			"default": {GoSymbol: "HTTPSAsJSValue"},
		},
	})
	ctx.RegisterModule("url", &tcontext.ModuleMapping{
		GoImportPath: "github.com/nnstd/gun/runtime/url",
		GoPkgName:    "_gunUrl",
		UseAsJSValue: true,
	})
	ctx.RegisterModule("util", &tcontext.ModuleMapping{
		GoImportPath: "github.com/nnstd/gun/runtime/util",
		GoPkgName:    "util",
		UseAsJSValue: true,
	})
	ctx.RegisterModule("events", &tcontext.ModuleMapping{
		GoImportPath: "github.com/nnstd/gun/runtime/events",
		GoPkgName:    "events",
		UseAsJSValue: true,
	})
	ctx.RegisterModule("stream", &tcontext.ModuleMapping{
		GoImportPath: "github.com/nnstd/gun/runtime/stream",
		GoPkgName:    "stream",
		UseAsJSValue: true,
	})
	ctx.RegisterModule("stream/promises", &tcontext.ModuleMapping{
		GoImportPath: "github.com/nnstd/gun/runtime/stream",
		GoPkgName:    "stream",
		UseAsJSValue: true,
		SymbolOverrides: map[string]tcontext.SymbolOverride{
			"default": {GoSymbol: "PromisesAsJSValue"},
		},
	})
	ctx.RegisterModule("buffer", &tcontext.ModuleMapping{
		GoImportPath: "github.com/nnstd/gun/runtime/buffer",
		GoPkgName:    "buffer",
		UseAsJSValue: true,
	})
	ctx.RegisterModule("string_decoder", &tcontext.ModuleMapping{
		GoImportPath: "github.com/nnstd/gun/runtime/string_decoder",
		GoPkgName:    "string_decoder",
		UseAsJSValue: true,
	})
	ctx.RegisterModule("constants", &tcontext.ModuleMapping{
		GoImportPath: "github.com/nnstd/gun/runtime/constants",
		GoPkgName:    "constants",
		UseAsJSValue: true,
	})
	ctx.RegisterModule("crypto", &tcontext.ModuleMapping{
		GoImportPath: "github.com/nnstd/gun/runtime/crypto",
		GoPkgName:    "crypto",
		UseAsJSValue: true,
	})
	ctx.RegisterModule("zlib", &tcontext.ModuleMapping{
		GoImportPath: "github.com/nnstd/gun/runtime/zlib",
		GoPkgName:    "zlib",
		UseAsJSValue: true,
	})
	ctx.RegisterModule("zlib/promises", &tcontext.ModuleMapping{
		GoImportPath: "github.com/nnstd/gun/runtime/zlib",
		GoPkgName:    "zlib",
		UseAsJSValue: true,
		SymbolOverrides: map[string]tcontext.SymbolOverride{
			"default": {GoSymbol: "PromisesAsJSValue"},
		},
	})
	ctx.RegisterModule("child_process", &tcontext.ModuleMapping{
		GoImportPath: "github.com/nnstd/gun/runtime/child_process",
		GoPkgName:    "child_process",
		UseAsJSValue: true,
	})
	ctx.RegisterModule("child_process/promises", &tcontext.ModuleMapping{
		GoImportPath: "github.com/nnstd/gun/runtime/child_process",
		GoPkgName:    "child_process",
		UseAsJSValue: true,
		SymbolOverrides: map[string]tcontext.SymbolOverride{
			"default": {GoSymbol: "PromisesAsJSValue"},
		},
	})
	ctx.RegisterModule("timers", &tcontext.ModuleMapping{
		GoImportPath: "github.com/nnstd/gun/runtime/timers",
		GoPkgName:    "timers",
		UseAsJSValue: true,
	})
	ctx.RegisterModule("timers/promises", &tcontext.ModuleMapping{
		GoImportPath: "github.com/nnstd/gun/runtime/timers",
		GoPkgName:    "timers",
		UseAsJSValue: true,
		SymbolOverrides: map[string]tcontext.SymbolOverride{
			"default": {GoSymbol: "PromisesAsJSValue"},
		},
	})
	ctx.RegisterModule("process", &tcontext.ModuleMapping{
		GoImportPath: "github.com/nnstd/gun/runtime/process",
		GoPkgName:    "process",
		UseAsJSValue: true,
		SymbolOverrides: map[string]tcontext.SymbolOverride{
			"default": {GoSymbol: "AsJSValueCached"},
		},
	})
	ctx.RegisterModule("dns", &tcontext.ModuleMapping{
		GoImportPath: "github.com/nnstd/gun/runtime/dns",
		GoPkgName:    "dns",
		UseAsJSValue: true,
	})
	ctx.RegisterModule("dgram", &tcontext.ModuleMapping{
		GoImportPath: "github.com/nnstd/gun/runtime/dgram",
		GoPkgName:    "dgram",
		UseAsJSValue: true,
	})
		ctx.RegisterModule("querystring", &tcontext.ModuleMapping{
			GoImportPath: "github.com/nnstd/gun/runtime/querystring",
			GoPkgName:    "querystring",
			UseAsJSValue: true,
		})
	ctx.RegisterModule("net", &tcontext.ModuleMapping{
		GoImportPath: "github.com/nnstd/gun/runtime/net",
		GoPkgName:    "nodenet",
		UseAsJSValue: true,
	})
	ctx.RegisterModule("tty", &tcontext.ModuleMapping{
		GoImportPath: "github.com/nnstd/gun/runtime/tty",
		GoPkgName:    "nodetty",
		UseAsJSValue: true,
	})
	ctx.RegisterModule("dns/promises", &tcontext.ModuleMapping{
		GoImportPath: "github.com/nnstd/gun/runtime/dns",
		GoPkgName:    "dns",
		UseAsJSValue: true,
		SymbolOverrides: map[string]tcontext.SymbolOverride{
			"default": {GoSymbol: "PromisesAsJSValue"},
		},
	})
	ctx.RegisterModule("assert", &tcontext.ModuleMapping{
		GoImportPath: "github.com/nnstd/gun/runtime/assert",
		GoPkgName:    "assert",
		UseAsJSValue: true,
	})
	ctx.RegisterModule("module", &tcontext.ModuleMapping{
		GoImportPath: "github.com/nnstd/gun/runtime/module",
		GoPkgName:    "module",
		UseAsJSValue: true,
	})
	ctx.RegisterModule("v8", &tcontext.ModuleMapping{
		GoImportPath: "github.com/nnstd/gun/runtime/v8",
		GoPkgName:    "v8",
		UseAsJSValue: true,
	})
	ctx.RegisterModule("perf_hooks", &tcontext.ModuleMapping{
		GoImportPath: "github.com/nnstd/gun/runtime/perf_hooks",
		GoPkgName:    "perf_hooks",
		UseAsJSValue: true,
	})
	ctx.RegisterModule("bun:ffi", &tcontext.ModuleMapping{
		GoImportPath: "github.com/nnstd/gun/runtime/ffi",
		GoPkgName:    "ffi",
		UseAsJSValue: true,
	})
}

// registerKnownGlobals marks names as known globals that should not
// be treated as JSValue property access targets (i.e., they should use
// capitalized Go selectors, not .Get("prop")).
//
// These must exactly match the old isKnownGlobalObject() list.
// Note: GlobalObject registrations (console, Math, JSON, Object, Number,
// process, Array, Error types) are already marked as known globals.
func registerKnownGlobals(ctx *tcontext.TranspilerContext) {
	// Identifiers registered via RegisterIdentifier that are typed globals
	// (not JSValue wrappers). "Promise" and "String" are intentionally
	// NOT here — they resolve to JSValue expressions and need .Get() dispatch.
	ctx.MarkKnownGlobal("undefined")
	ctx.MarkKnownGlobal("null")
	ctx.MarkKnownGlobal("NaN")
	ctx.MarkKnownGlobal("Infinity")
	ctx.MarkKnownGlobal("Boolean")
	ctx.MarkKnownGlobal("Number") // also a GlobalObject, but listed for clarity

	// Additional globals without full transform registrations
	ctx.MarkKnownGlobal("Date")
	ctx.MarkKnownGlobal("RegExp")
	ctx.MarkKnownGlobal("Bun")
	ctx.MarkKnownGlobal("Headers")
	ctx.MarkKnownGlobal("Request")
	ctx.MarkKnownGlobal("Response")
	ctx.MarkKnownGlobal("URL")
	ctx.MarkKnownGlobal("URLSearchParams")
	ctx.MarkKnownGlobal("File")
	ctx.MarkKnownGlobal("FormData")
	ctx.MarkKnownGlobal("fetch")
	ctx.MarkKnownGlobal("Symbol")
	ctx.MarkKnownGlobal("module")
	ctx.MarkKnownGlobal("require")
	ctx.MarkKnownGlobal("__filename")
	ctx.MarkKnownGlobal("__dirname")
	ctx.MarkKnownGlobal("globalThis")
	ctx.MarkKnownGlobal("performance")
	ctx.MarkKnownGlobal("decodeURI")
	ctx.MarkKnownGlobal("decodeURIComponent")
	ctx.MarkKnownGlobal("encodeURI")
	ctx.MarkKnownGlobal("encodeURIComponent")

	// Browser/Node environment globals
	// global → same as globalThis (jscontext.Default().Global())
	ctx.RegisterIdentifier(&tcontext.IdentifierMapping{
		Name: "global",
		Transform: func(imp tcontext.Imports) ast.Expr {
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/jscontext", "jscontext")
			imp.SetNeedsGlobalSync()
			return callExpr(selectorExpr(
				callExpr(selectorExpr(ident("jscontext"), "Default")),
				"Global",
			))
		},
	})
	ctx.MarkKnownGlobal("global")

	// window, self → undefined (Node.js behavior)
	for _, name := range []string{"window", "self"} {
		ctx.RegisterIdentifier(&tcontext.IdentifierMapping{
			Name: name,
			Transform: func(imp tcontext.Imports) ast.Expr {
				imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
				return callExpr(selectorExpr(ident("jsvalue"), "NewUndefined"))
			},
		})
		ctx.MarkKnownGlobal(name)
	}

	// exports, define → NewObject (module system identifiers)
	for _, name := range []string{"exports", "define"} {
		ctx.RegisterIdentifier(&tcontext.IdentifierMapping{
			Name: name,
			Transform: func(imp tcontext.Imports) ast.Expr {
				imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
				return callExpr(selectorExpr(ident("jsvalue"), "NewObject"))
			},
		})
		ctx.MarkKnownGlobal(name)
	}

	// Binary data types
	ctx.MarkKnownGlobal("Buffer")
	ctx.MarkKnownGlobal("ArrayBuffer")
	ctx.MarkKnownGlobal("SharedArrayBuffer")
	ctx.MarkKnownGlobal("DataView")
	ctx.MarkKnownGlobal("Uint8Array")
	ctx.MarkKnownGlobal("Int8Array")
	ctx.MarkKnownGlobal("Uint8ClampedArray")
	ctx.MarkKnownGlobal("Int16Array")
	ctx.MarkKnownGlobal("Uint16Array")
	ctx.MarkKnownGlobal("Int32Array")
	ctx.MarkKnownGlobal("Uint32Array")
	ctx.MarkKnownGlobal("Float32Array")
	ctx.MarkKnownGlobal("Float64Array")
	ctx.MarkKnownGlobal("BigInt64Array")
	ctx.MarkKnownGlobal("BigUint64Array")

		ctx.RegisterIdentifier(&tcontext.IdentifierMapping{
			Name: "crypto",
			Transform: func(imp tcontext.Imports) ast.Expr {
				imp.AddImport("github.com/nnstd/gun/runtime/crypto")
				return selectorExpr(ident("crypto"), "AsJSValue")
			},
		})
		ctx.MarkKnownGlobal("crypto")
}

// RegisterTest262Globals registers test262 harness functions and objects
// (assert, print, $DONE, $DONOTEVALUATE, $262) in the transpiler context.
// These generate direct Go function calls that match the harness Go source files
// in test262/harness/.
func RegisterTest262Globals(ctx *tcontext.TranspilerContext) {
	// assert(condition) → Assert(jsvalueWrapLit(condition))
	ctx.RegisterGlobalFunc(&tcontext.GlobalFunction{
		Name: "assert",
		Transform: func(args []ast.Expr, imp tcontext.Imports) ast.Expr {
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			if len(args) > 0 {
				args[0] = jsvalueWrapLit(args[0])
			}
			return callExpr(ident("Assert"), args...)
		},
	})

	// assert is also a global object with methods: sameValue, notSameValue, throws
	ctx.RegisterGlobal(&tcontext.GlobalObject{
		Name: "assert",
		TransformCall: func(method string, args []ast.Expr, _ bool, imp tcontext.Imports) ast.Expr {
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			for i, arg := range args {
				args[i] = jsvalueWrapLit(arg)
			}
			switch method {
			case "sameValue":
				return callExpr(ident("AssertSameValue"), args...)
			case "notSameValue":
				return callExpr(ident("AssertNotSameValue"), args...)
			case "throws":
				// assert.throws(ErrorType, fn) → AssertThrows("ErrorType", fn)
				var errorName ast.Expr
				if len(args) > 0 {
					// Skip jsvalue wrapping for the error type, use original expr
					origArg := args[0]
					args = args[1:]
					if id, ok := origArg.(*ast.Ident); ok {
						errorName = stringLit(id.Name)
					} else {
						errorName = stringLit("Error")
					}
				} else {
					errorName = stringLit("Error")
				}
				return callExpr(ident("AssertThrows"), append([]ast.Expr{errorName}, args...)...)
			case "deepEqual":
				return callExpr(ident("AssertSameValue"), args...)
			default:
				return nil
			}
		},
	})

	// print(...) → Print(args...)
	ctx.RegisterGlobalFunc(&tcontext.GlobalFunction{
		Name: "print",
		Transform: func(args []ast.Expr, imp tcontext.Imports) ast.Expr {
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			for i, arg := range args {
				args[i] = jsvalueWrapLit(arg)
			}
			return callExpr(ident("Print"), args...)
		},
	})

	// $DONE(err?) → Done(err?)
	ctx.RegisterGlobalFunc(&tcontext.GlobalFunction{
		Name: "$DONE",
		Transform: func(args []ast.Expr, imp tcontext.Imports) ast.Expr {
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			if len(args) > 0 {
				args[0] = jsvalueWrapLit(args[0])
			}
			return callExpr(ident("Done"), args...)
		},
	})

	// $DONOTEVALUATE() → DontEvaluate()
	ctx.RegisterGlobalFunc(&tcontext.GlobalFunction{
		Name: "$DONOTEVALUATE",
		Transform: func(args []ast.Expr, imp tcontext.Imports) ast.Expr {
			return callExpr(ident("DontEvaluate"))
		},
	})

	// $262 global object stub
	ctx.RegisterGlobal(&tcontext.GlobalObject{
		Name: "$262",
		TransformCall: func(method string, args []ast.Expr, _ bool, imp tcontext.Imports) ast.Expr {
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			return callExpr(selectorExpr(callExpr(ident("Global262")), "Get"), stringLit(method))
		},
		TransformMember: func(prop string, imp tcontext.Imports) ast.Expr {
			return callExpr(selectorExpr(callExpr(ident("Global262")), "Get"), stringLit(prop))
		},
	})

	// Test262Error constructor
	ctx.RegisterConstructor(&tcontext.Constructor{
		Name: "Test262Error",
		Transform: func(args []ast.Expr, imp tcontext.Imports) ast.Expr {
			return callExpr(ident("Test262ErrorNew"), args...)
		},
	})
}
