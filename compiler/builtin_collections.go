package compiler

import (
	"go/ast"
	"go/token"
)

func transformCollectionMethod(obj ast.Expr, prop string, args []ast.Expr, addImport func(string)) ast.Expr {
	switch prop {
	case "concat":
		if len(args) > 0 {
			// [].concat(x) → just x (empty receiver is a no-op)
			if cl, ok := obj.(*ast.CompositeLit); ok && len(cl.Elts) == 0 {
				return args[0]
			}
			// JSValue receiver: coerce to array, append, wrap result
			if isJSValueMethodCall(obj) {
				addImport("github.com/nnstd/gun/runtime/jsvalue")
				wrapped := make([]ast.Expr, len(args))
				for i, a := range args {
					wrapped[i] = callExpr(selectorExpr(ident("jsvalue"), "From"), a)
				}
				// Create append call: append(obj.Array(), wrappedArgs...)
				appendArgs := append([]ast.Expr{callExpr(selectorExpr(obj, "Array"))}, wrapped...)
				appendCall := &ast.CallExpr{
					Fun:  ident("append"),
					Args: appendArgs,
				}
				// Wrap in NewArray with spread: jsvalue.NewArray(appendCall...)
				return &ast.CallExpr{
					Fun:      selectorExpr(ident("jsvalue"), "NewArray"),
					Args:     []ast.Expr{appendCall},
					Ellipsis: token.Pos(1), // indicates spread operator
				}
			}
			return callExpr(ident("append"), append([]ast.Expr{obj}, args...)...)
		}
	case "push":
		if len(args) > 0 {
			// JSValue receiver: coerce to array, append, wrap result
			if isJSValueMethodCall(obj) {
				addImport("github.com/nnstd/gun/runtime/jsvalue")
				wrapped := make([]ast.Expr, len(args))
				for i, a := range args {
					wrapped[i] = callExpr(selectorExpr(ident("jsvalue"), "From"), a)
				}
				// Create append call: append(obj.Array(), wrappedArgs...)
				appendArgs := append([]ast.Expr{callExpr(selectorExpr(obj, "Array"))}, wrapped...)
				appendCall := &ast.CallExpr{
					Fun:  ident("append"),
					Args: appendArgs,
				}
				// Wrap in NewArray with spread: jsvalue.NewArray(appendCall...)
				return &ast.CallExpr{
					Fun:      selectorExpr(ident("jsvalue"), "NewArray"),
					Args:     []ast.Expr{appendCall},
					Ellipsis: token.Pos(1), // indicates spread operator
				}
			}
			return callExpr(ident("append"), append([]ast.Expr{obj}, args...)...)
		}
	case "length":
		// JSValue receiver: use len(obj.Array()) for arrays or len(obj.String()) for strings
		if isJSValueMethodCall(obj) {
			// For now, assume array - string length should be handled by string methods
			return callExpr(ident("len"), callExpr(selectorExpr(obj, "Array")))
		}
		return callExpr(ident("len"), obj)
	case "includes":
		if len(args) > 0 {
			return callExpr(ident("contains"), append([]ast.Expr{obj}, args...)...)
		}
	case "slice":
		// JSValue receiver: coerce to array, slice, wrap result
		if isJSValueMethodCall(obj) {
			addImport("github.com/nnstd/gun/runtime/jsvalue")
			sliceExpr := &ast.SliceExpr{X: callExpr(selectorExpr(obj, "Array"))}
			if len(args) >= 1 {
				sliceExpr.Low = args[0]
			}
			if len(args) >= 2 {
				sliceExpr.High = args[1]
			}
			// Wrap in NewArray with spread: jsvalue.NewArray(sliceExpr...)
			return &ast.CallExpr{
				Fun:      selectorExpr(ident("jsvalue"), "NewArray"),
				Args:     []ast.Expr{sliceExpr},
				Ellipsis: token.Pos(1), // indicates spread operator
			}
		}
		if len(args) >= 2 {
			return &ast.SliceExpr{X: obj, Low: args[0], High: args[1]}
		}
		if len(args) == 1 {
			return &ast.SliceExpr{X: obj, Low: args[0]}
		}
	case "map", "filter", "forEach":
		// Transform to package-level function calls: arr.map(fn) → jsvalue.Map(arr, fn)
		if len(args) > 0 {
			addImport("github.com/nnstd/gun/runtime/jsvalue")
			funcName := capitalize(prop) // Map, Filter, ForEach
			return callExpr(selectorExpr(ident("jsvalue"), funcName), append([]ast.Expr{obj}, args...)...)
		}
	case "reduce", "find", "some", "every":
		// These need more complex transforms; pass through for now
	}
	return nil
}

func transformObjectCall(prop string, args []ast.Expr, addImport func(string)) ast.Expr {
	switch prop {
	case "keys", "values", "entries", "assign":
		if len(args) > 0 {
			return args[0]
		}
	case "create":
		// Object.create(null) → jsvalue.NewObject()
		addImport("github.com/nnstd/gun/runtime/jsvalue")
		return callExpr(selectorExpr(ident("jsvalue"), "NewObject"))
	}
	return nil
}
