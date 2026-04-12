package compiler

import (
	"go/ast"
	"go/token"

	tcontext "github.com/nnstd/gun/compiler/context"
)

// RegisterDefaultBuiltins populates a TranspilerContext with all known JavaScript
// global objects, functions, constructors, identifiers, and modules.
//
// This replaces the previously hardcoded switch/case dispatchers scattered across
// builtins.go, imports.go, and expressions.go with a unified registration.
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
			return transformJSONCall(method, args, imp.AddImport)
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
					return callExpr(
						selectorExpr(
							callExpr(selectorExpr(selectorExpr(ident("bun"), "AsJSValue"), "Get"), stringLit("serve")),
							"Call",
						),
						jsvalueWrapLit(args[0]),
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
				imp.AddImport("fmt")
				return callExpr(selectorExpr(ident("jsvalue"), "NewSymbol"),
					callExpr(selectorExpr(ident("fmt"), "Sprint"), args[0]))
			}
			return callExpr(selectorExpr(ident("jsvalue"), "NewSymbol"), stringLit(""))
		},
	})

	ctx.RegisterGlobalFunc(&tcontext.GlobalFunction{
		Name: "String",
		Transform: func(args []ast.Expr, imp tcontext.Imports) ast.Expr {
			imp.AddImport("fmt")
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			if len(args) > 0 {
				return callExpr(selectorExpr(ident("jsvalue"), "NewString"),
					callExpr(selectorExpr(ident("fmt"), "Sprint"), args[0]))
			}
			return callExpr(selectorExpr(ident("jsvalue"), "NewString"), stringLit(""))
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

	for _, fnName := range []string{"decodeURI", "decodeURIComponent", "encodeURI"} {
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
				}
				if len(args) > 0 {
					return callExpr(selectorExpr(ident("web"), callName), jsvalueWrapLit(args[0]))
				}
				return callExpr(selectorExpr(ident("web"), callName), callExpr(selectorExpr(ident("jsvalue"), "NewString"), stringLit("")))
			},
		})
	}

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

	ctx.RegisterConstructor(&tcontext.Constructor{
		Name: "Date",
		Transform: func(args []ast.Expr, imp tcontext.Imports) ast.Expr {
			imp.AddImport("time")
			return callExpr(selectorExpr(ident("time"), "Now"))
		},
	})

	ctx.RegisterConstructor(&tcontext.Constructor{
		Name: "RegExp",
		Transform: func(args []ast.Expr, imp tcontext.Imports) ast.Expr {
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			pattern := ast.Expr(stringLit(""))
			if len(args) > 0 {
				imp.AddImport("fmt")
				pattern = callExpr(selectorExpr(ident("fmt"), "Sprint"), args[0])
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

	for _, ctorName := range []string{"Headers", "Request", "Response", "URL", "File"} {
		name := ctorName
		ctx.RegisterConstructor(&tcontext.Constructor{
			Name: name,
			Transform: func(args []ast.Expr, imp tcontext.Imports) ast.Expr {
				imp.AddImport("github.com/nnstd/gun/runtime/web")
				if name == "URL" {
					if len(args) > 0 {
						return callExpr(selectorExpr(ident("web"), "ParseURL"), jsvalueWrapLit(args[0]))
					}
					return callExpr(selectorExpr(ident("web"), "ParseURL"), callExpr(selectorExpr(ident("jsvalue"), "NewString"), stringLit("")))
				}
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
			return ident("console")
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
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin/json", "json")
			return selectorExpr(ident("json"), "AsJSValue")
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
			imp.AddImport("fmt")
			return callExpr(selectorExpr(ident("jsvalue"), "NewFunction"), &ast.FuncLit{
				Type: &ast.FuncType{
					Params:  fieldList(&ast.Field{Names: []*ast.Ident{ident("_a")}, Type: &ast.Ellipsis{Elt: ptrType(selectorExpr(ident("jsvalue"), "JSValue"))}}),
					Results: fieldList(field("", ptrType(selectorExpr(ident("jsvalue"), "JSValue")))),
				},
				Body: blockStmt(returnStmt(callExpr(selectorExpr(ident("jsvalue"), "NewString"), callExpr(selectorExpr(ident("fmt"), "Sprint"), &ast.IndexExpr{X: ident("_a"), Index: intLit("0")})))),
			})
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
			// Boolean(x) → jsvalue.NewBool(jsvalue.Truthy(x))
			// Wrap as a NewFunction so it's callable as JSValue
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			return callExpr(selectorExpr(ident("jsvalue"), "NewFunction"),
				&ast.FuncLit{
					Type: &ast.FuncType{
						Params:  fieldList(&ast.Field{Names: []*ast.Ident{ident("_a")}, Type: &ast.Ellipsis{Elt: ptrType(selectorExpr(ident("jsvalue"), "JSValue"))}}),
						Results: fieldList(field("", ptrType(selectorExpr(ident("jsvalue"), "JSValue")))),
					},
					Body: blockStmt(returnStmt(callExpr(selectorExpr(ident("jsvalue"), "NewBool"),
						callExpr(selectorExpr(ident("jsvalue"), "Truthy"), &ast.IndexExpr{X: ident("_a"), Index: intLit("0")})))),
				})
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
		Name: "Bun",
		Transform: func(imp tcontext.Imports) ast.Expr {
			imp.AddImport("github.com/nnstd/gun/runtime/bun")
			return selectorExpr(ident("bun"), "AsJSValue")
		},
	})

	for _, identName := range []string{"Headers", "Request", "Response", "URL", "File", "RegExp"} {
		name := identName
		ctx.RegisterIdentifier(&tcontext.IdentifierMapping{
			Name: name,
			Transform: func(imp tcontext.Imports) ast.Expr {
				if name == "RegExp" {
					imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
					return selectorExpr(ident("jsvalue"), "RegexpCtor")
				}
				imp.AddImport("github.com/nnstd/gun/runtime/web")
				if name == "URL" {
					return selectorExpr(ident("web"), "URL")
				}
				return selectorExpr(ident("web"), name)
			},
		})
	}

	ctx.RegisterIdentifier(&tcontext.IdentifierMapping{
		Name: "require",
		Transform: func(imp tcontext.Imports) ast.Expr {
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			return callExpr(selectorExpr(ident("jsvalue"), "NewObject"))
		},
	})

	ctx.RegisterIdentifier(&tcontext.IdentifierMapping{
		Name: "globalThis",
		Transform: func(imp tcontext.Imports) ast.Expr {
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			return callExpr(selectorExpr(ident("jsvalue"), "NewObject"))
		},
	})

	for _, identName := range []string{"decodeURI", "decodeURIComponent", "encodeURI"} {
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
}

// registerModules registers TS module → Go package mappings.
func registerModules(ctx *tcontext.TranspilerContext) {
	ctx.RegisterModule("fs", &tcontext.ModuleMapping{
		GoImportPath: "github.com/nnstd/gun/runtime/fs",
		GoPkgName:    "fs",
		UseAsJSValue: true,
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
		GoImportPath: "net/http",
		GoPkgName:    "http",
	})
	ctx.RegisterModule("https", &tcontext.ModuleMapping{
		GoImportPath: "net/http",
		GoPkgName:    "http",
	})
	ctx.RegisterModule("url", &tcontext.ModuleMapping{
		GoImportPath: "github.com/nnstd/gun/runtime/url",
		GoPkgName:    "url",
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
	ctx.RegisterModule("buffer", &tcontext.ModuleMapping{
		GoImportPath: "github.com/nnstd/gun/runtime/buffer",
		GoPkgName:    "buffer",
		UseAsJSValue: true,
	})
	ctx.RegisterModule("crypto", &tcontext.ModuleMapping{
		GoImportPath: "github.com/nnstd/gun/runtime/crypto",
		GoPkgName:    "crypto",
		UseAsJSValue: true,
	})
	ctx.RegisterModule("child_process", &tcontext.ModuleMapping{
		GoImportPath: "github.com/nnstd/gun/runtime/child_process",
		GoPkgName:    "child_process",
		UseAsJSValue: true,
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
	ctx.MarkKnownGlobal("File")
	ctx.MarkKnownGlobal("Symbol")
	ctx.MarkKnownGlobal("module")
	ctx.MarkKnownGlobal("require")
	ctx.MarkKnownGlobal("globalThis")
	ctx.MarkKnownGlobal("decodeURI")
	ctx.MarkKnownGlobal("decodeURIComponent")
	ctx.MarkKnownGlobal("encodeURI")
}
