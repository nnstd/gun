package compiler

import (
	"go/ast"
)

func transformObjectCall(prop string, args []ast.Expr, addImport func(string)) ast.Expr {
	addImport("github.com/nnstd/gun/runtime/builtin")
	switch prop {
	case "keys":
		if len(args) > 0 {
			return callExpr(selectorExpr(ident("jsvalue"), "Keys"), args[0])
		}
	case "values":
		if len(args) > 0 {
			return callExpr(selectorExpr(ident("jsvalue"), "Values"), args[0])
		}
	case "entries":
		if len(args) > 0 {
			return callExpr(selectorExpr(ident("jsvalue"), "Entries"), args[0])
		}
	case "fromEntries":
		if len(args) > 0 {
			return callExpr(selectorExpr(ident("jsvalue"), "FromEntries"), args[0])
		}
	case "assign":
		if len(args) > 0 {
			return callExpr(selectorExpr(ident("jsvalue"), "Assign"), args...)
		}
	case "create":
		if len(args) > 0 {
			return callExpr(selectorExpr(ident("jsvalue"), "Create"), args[0])
		}
	case "freeze":
		if len(args) > 0 {
			return args[0] // No-op in Go — objects are mutable
		}
	case "defineProperty":
		if len(args) >= 3 {
			return callExpr(selectorExpr(ident("jsvalue"), "DefineProperty"), args...)
		}
	case "getOwnPropertyNames":
		if len(args) > 0 {
			return callExpr(selectorExpr(ident("jsvalue"), "Keys"), args[0])
		}
	case "getPrototypeOf":
		if len(args) > 0 {
			return callExpr(selectorExpr(args[0], "GetPrototype"))
		}
	case "setPrototypeOf":
		if len(args) >= 2 {
			return callExpr(selectorExpr(args[0], "SetPrototype"), args[1])
		}
	case "hasOwn":
		addImport("fmt")
		if len(args) >= 2 {
			return callExpr(selectorExpr(ident("jsvalue"), "NewBool"),
				callExpr(selectorExpr(args[0], "HasOwnProperty"),
					callExpr(selectorExpr(ident("fmt"), "Sprint"), args[1])))
		}
	}
	return nil
}
