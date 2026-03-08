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
		TransformCall: func(method string, args []ast.Expr, imp tcontext.Imports) ast.Expr {
			return transformConsoleCall(method, args, imp.AddImport)
		},
	})

	ctx.RegisterGlobal(&tcontext.GlobalObject{
		Name: "Math",
		TransformCall: func(method string, args []ast.Expr, imp tcontext.Imports) ast.Expr {
			return transformMathCall(method, args, imp.AddImport)
		},
	})

	ctx.RegisterGlobal(&tcontext.GlobalObject{
		Name: "JSON",
		TransformCall: func(method string, args []ast.Expr, imp tcontext.Imports) ast.Expr {
			return transformJSONCall(method, args, imp.AddImport)
		},
	})

	ctx.RegisterGlobal(&tcontext.GlobalObject{
		Name: "Object",
		TransformCall: func(method string, args []ast.Expr, imp tcontext.Imports) ast.Expr {
			return transformObjectCall(method, args, imp.AddImport)
		},
	})

	ctx.RegisterGlobal(&tcontext.GlobalObject{
		Name: "Number",
		TransformCall: func(method string, args []ast.Expr, imp tcontext.Imports) ast.Expr {
			return transformNumberCall(method, args)
		},
	})

	ctx.RegisterGlobal(&tcontext.GlobalObject{
		Name: "process",
		TransformCall: func(method string, args []ast.Expr, imp tcontext.Imports) ast.Expr {
			return transformProcessCall(method, args, imp.AddImport)
		},
		TransformMember: func(prop string, imp tcontext.Imports) ast.Expr {
			return transformProcessMember(prop, imp.AddImport)
		},
	})

	ctx.RegisterGlobal(&tcontext.GlobalObject{
		Name: "Array",
		TransformCall: func(method string, args []ast.Expr, imp tcontext.Imports) ast.Expr {
			return transformArrayCall(method, args, imp.AddImport)
		},
	})

	// Error types as global objects (for static methods like Error.captureStackTrace)
	for _, errType := range []string{"Error", "TypeError", "RangeError", "ReferenceError", "SyntaxError", "URIError", "EvalError"} {
		et := errType // capture
		ctx.RegisterGlobal(&tcontext.GlobalObject{
			Name: et,
			TransformCall: func(method string, args []ast.Expr, imp tcontext.Imports) ast.Expr {
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
			imp.AddImport("math")
			if len(args) > 0 {
				return callExpr(selectorExpr(ident("math"), "IsNaN"), callExpr(selectorExpr(args[0], "Number")))
			}
			return ident("false")
		},
	})

	ctx.RegisterGlobalFunc(&tcontext.GlobalFunction{
		Name: "isFinite",
		Transform: func(args []ast.Expr, imp tcontext.Imports) ast.Expr {
			imp.AddImport("math")
			if len(args) > 0 {
				return &ast.UnaryExpr{
					Op: token.NOT,
					X:  callExpr(selectorExpr(ident("math"), "IsInf"), callExpr(selectorExpr(args[0], "Number")), intLit("0")),
				}
			}
			return ident("true")
		},
	})

	ctx.RegisterGlobalFunc(&tcontext.GlobalFunction{
		Name: "Number",
		Transform: func(args []ast.Expr, imp tcontext.Imports) ast.Expr {
			if len(args) > 0 {
				return callExpr(selectorExpr(args[0], "Number"))
			}
			return basicLit(token.FLOAT, "0")
		},
	})

	ctx.RegisterGlobalFunc(&tcontext.GlobalFunction{
		Name: "Array",
		Transform: func(args []ast.Expr, imp tcontext.Imports) ast.Expr {
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			return callExpr(selectorExpr(ident("jsvalue"), "NewArray"))
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

	// Error types as functions (Error() without new)
	for _, errType := range []string{"Error", "TypeError", "RangeError", "ReferenceError", "SyntaxError", "URIError", "EvalError"} {
		et := errType
		ctx.RegisterGlobalFunc(&tcontext.GlobalFunction{
			Name: et,
			Transform: func(args []ast.Expr, imp tcontext.Imports) ast.Expr {
				imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin/error", "jserror")
				imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
				for i, arg := range args {
					args[i] = jsvalueWrapLit(arg)
				}
				return callExpr(selectorExpr(selectorExpr(ident("jserror"), et), "Call"), args...)
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
				imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin/error", "jserror")
				imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
				for i, arg := range args {
					args[i] = jsvalueWrapLit(arg)
				}
				return callExpr(selectorExpr(selectorExpr(ident("jserror"), et), "Call"), args...)
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
		Name: "IntlSegmenter",
		Transform: func(args []ast.Expr, imp tcontext.Imports) ast.Expr {
			imp.AddImport("github.com/nnstd/gun/runtime/builtin/intl")
			return callExpr(selectorExpr(selectorExpr(ident("intl"), "Segmenter"), "Call"), args...)
		},
	})
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
			imp.AddImport("math")
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			return callExpr(selectorExpr(ident("jsvalue"), "NewNumber"), callExpr(selectorExpr(ident("math"), "Inf"), intLit("1")))
		},
	})

	ctx.RegisterIdentifier(&tcontext.IdentifierMapping{
		Name: "NaN",
		Transform: func(imp tcontext.Imports) ast.Expr {
			imp.AddImport("math")
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			return callExpr(selectorExpr(ident("jsvalue"), "NewNumber"), callExpr(selectorExpr(ident("math"), "NaN")))
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
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin/math", "jsmath")
			return ident("jsmath")
		},
	})

	ctx.RegisterIdentifier(&tcontext.IdentifierMapping{
		Name: "JSON",
		Transform: func(imp tcontext.Imports) ast.Expr {
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin/json", "json")
			return ident("json")
		},
	})

	for _, errType := range []string{"Error", "TypeError", "RangeError", "ReferenceError", "SyntaxError", "URIError", "EvalError"} {
		et := errType
		ctx.RegisterIdentifier(&tcontext.IdentifierMapping{
			Name: et,
			Transform: func(imp tcontext.Imports) ast.Expr {
				imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin/error", "jserror")
				return selectorExpr(ident("jserror"), et)
			},
		})
	}

	ctx.RegisterIdentifier(&tcontext.IdentifierMapping{
		Name: "Object",
		Transform: func(imp tcontext.Imports) ast.Expr {
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			return ident("jsvalue")
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
			return selectorExpr(ident("jsvalue"), "ArrayPrototype")
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
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			return callExpr(selectorExpr(ident("jsvalue"), "NewObject"))
		},
	})

	ctx.RegisterIdentifier(&tcontext.IdentifierMapping{
		Name: "Number",
		Transform: func(imp tcontext.Imports) ast.Expr {
			return ident("float64")
		},
	})

	ctx.RegisterIdentifier(&tcontext.IdentifierMapping{
		Name: "Boolean",
		Transform: func(imp tcontext.Imports) ast.Expr {
			imp.AddAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			return selectorExpr(ident("jsvalue"), "Truthy")
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
}

// registerModules registers TS module → Go package mappings.
func registerModules(ctx *tcontext.TranspilerContext) {
	ctx.RegisterModule("fs", &tcontext.ModuleMapping{
		GoImportPath: "github.com/nnstd/gun/runtime/fs",
		GoPkgName:    "fs",
		SymbolOverrides: map[string]tcontext.SymbolOverride{
			"promises":  {GoSymbol: ""},
			"readFile":  {GoSymbol: "ReadFileSync"},
			"writeFile": {GoSymbol: "WriteFileSync"},
		},
	})
	ctx.RegisterModule("path", &tcontext.ModuleMapping{
		GoImportPath: "github.com/nnstd/gun/runtime/path",
		GoPkgName:    "nodepath",
	})
	ctx.RegisterModule("os", &tcontext.ModuleMapping{
		GoImportPath: "github.com/nnstd/gun/runtime/os",
		GoPkgName:    "nodeos",
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
	})
	ctx.RegisterModule("util", &tcontext.ModuleMapping{
		GoImportPath: "fmt",
		GoPkgName:    "fmt",
		SymbolOverrides: map[string]tcontext.SymbolOverride{
			"format":  {GoSymbol: "Sprintf"},
			"inspect": {GoSymbol: "Sprint"},
		},
	})
	ctx.RegisterModule("events", &tcontext.ModuleMapping{
		GoImportPath: "sync",
		GoPkgName:    "sync",
	})
	ctx.RegisterModule("stream", &tcontext.ModuleMapping{
		GoImportPath: "io",
		GoPkgName:    "io",
	})
	ctx.RegisterModule("buffer", &tcontext.ModuleMapping{
		GoImportPath: "bytes",
		GoPkgName:    "bytes",
	})
	ctx.RegisterModule("crypto", &tcontext.ModuleMapping{
		GoImportPath: "crypto",
		GoPkgName:    "crypto",
	})
	ctx.RegisterModule("child_process", &tcontext.ModuleMapping{
		GoImportPath: "os/exec",
		GoPkgName:    "exec",
		SymbolOverrides: map[string]tcontext.SymbolOverride{
			"exec":     {GoSymbol: "Command"},
			"execSync": {GoSymbol: "Command"},
			"spawn":    {GoSymbol: "Command"},
		},
	})
	ctx.RegisterModule("assert", &tcontext.ModuleMapping{
		GoImportPath: "github.com/nnstd/gun/runtime/assert",
		GoPkgName:    "assert",
		SymbolOverrides: map[string]tcontext.SymbolOverride{
			"strict": {GoSymbol: ""},
		},
	})
	ctx.RegisterModule("module", &tcontext.ModuleMapping{
		GoImportPath: "github.com/nnstd/gun/runtime/module",
		GoPkgName:    "module",
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
	ctx.MarkKnownGlobal("Symbol")
	ctx.MarkKnownGlobal("module")
	ctx.MarkKnownGlobal("require")
	ctx.MarkKnownGlobal("globalThis")
}
