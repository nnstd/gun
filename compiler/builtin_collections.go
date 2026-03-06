package compiler

import (
	"go/ast"
	"go/token"
)

// normalizeSliceIndex converts negative slice indices to len(arr) - abs(index).
// In JavaScript, arr.slice(0, -1) means "from start to one before the end".
// In Go, we need to convert -1 to len(arr)-1.
func normalizeSliceIndex(index ast.Expr, arr ast.Expr) ast.Expr {
	// Check if index is a negative literal
	if unary, ok := index.(*ast.UnaryExpr); ok && unary.Op == token.SUB {
		if lit, ok := unary.X.(*ast.BasicLit); ok && lit.Kind == token.INT {
			// Negative literal: convert -N to len(arr)-N
			return &ast.BinaryExpr{
				X:  callExpr(ident("len"), arr),
				Op: token.SUB,
				Y:  lit,
			}
		}
	}
	// Non-negative or non-literal: use as-is
	return index
}

// transformCollectionMethod handles typed Go array methods (non-JSValue).
// JSValue arrays use prototype methods via .MethodCall() instead.
func transformCollectionMethod(obj ast.Expr, prop string, args []ast.Expr, addImport func(string), isJSValueReceiver bool) ast.Expr {
	if isJSValueReceiver {
		return nil // JSValue receivers use prototype methods
	}
	switch prop {
	case "concat":
		if len(args) > 0 {
			return callExpr(ident("append"), append([]ast.Expr{obj}, args...)...)
		}
	case "push":
		if len(args) > 0 {
			return callExpr(ident("append"), append([]ast.Expr{obj}, args...)...)
		}
	case "pop":
		// For typed arrays: arr[len(arr)-1]
		lenCall := callExpr(ident("len"), obj)
		lastIndex := &ast.BinaryExpr{
			X:  lenCall,
			Op: token.SUB,
			Y:  &ast.BasicLit{Kind: token.INT, Value: "1"},
		}
		return &ast.IndexExpr{X: obj, Index: lastIndex}
	case "shift":
		// For typed arrays: arr[0]
		return &ast.IndexExpr{X: obj, Index: &ast.BasicLit{Kind: token.INT, Value: "0"}}
	case "length":
		return callExpr(ident("len"), obj)
	case "includes":
		if len(args) > 0 {
			addImport("slices")
			return callExpr(selectorExpr(ident("slices"), "Contains"), obj, args[0])
		}
	case "slice":
		if len(args) >= 2 {
			return &ast.SliceExpr{X: obj, Low: normalizeSliceIndex(args[0], obj), High: normalizeSliceIndex(args[1], obj)}
		}
		if len(args) == 1 {
			return &ast.SliceExpr{X: obj, Low: normalizeSliceIndex(args[0], obj)}
		}
	case "join":
		// For typed string arrays: strings.Join(arr, sep)
		addImport("strings")
		var sep ast.Expr = stringLit(",")
		if len(args) >= 1 {
			sep = args[0]
		}
		return callExpr(selectorExpr(ident("strings"), "Join"), obj, sep)
	}
	return nil
}

func transformObjectCall(prop string, args []ast.Expr, addImport func(string)) ast.Expr {
	addImport("github.com/nnstd/gun/runtime/object")
	switch prop {
	case "keys":
		if len(args) > 0 {
			return callExpr(selectorExpr(ident("jsobject"), "Keys"), args[0])
		}
	case "values":
		if len(args) > 0 {
			return callExpr(selectorExpr(ident("jsobject"), "Values"), args[0])
		}
	case "entries":
		if len(args) > 0 {
			return callExpr(selectorExpr(ident("jsobject"), "Entries"), args[0])
		}
	case "assign":
		if len(args) > 0 {
			return callExpr(selectorExpr(ident("jsobject"), "Assign"), args...)
		}
	case "create":
		if len(args) > 0 {
			return callExpr(selectorExpr(ident("jsobject"), "Create"), args[0])
		}
	case "freeze":
		if len(args) > 0 {
			return args[0] // No-op in Go — objects are mutable
		}
	case "defineProperty":
		if len(args) >= 3 {
			return callExpr(selectorExpr(ident("jsobject"), "DefineProperty"), args...)
		}
	case "getOwnPropertyNames":
		if len(args) > 0 {
			return callExpr(selectorExpr(ident("jsobject"), "Keys"), args[0])
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
