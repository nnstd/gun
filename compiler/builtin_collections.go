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

func transformCollectionMethod(obj ast.Expr, prop string, args []ast.Expr, addImport func(string), isJSValueReceiver bool) ast.Expr {
	switch prop {
	case "concat":
		if len(args) > 0 {
			// [].concat(x) → just x (empty receiver is a no-op)
			if cl, ok := obj.(*ast.CompositeLit); ok && len(cl.Elts) == 0 {
				return args[0]
			}
			// Check if receiver is array literal with JSValue elements
			isJSValueArrayLit := false
			if cl, ok := obj.(*ast.CompositeLit); ok {
				if at, ok := cl.Type.(*ast.ArrayType); ok {
					if isJSValuePtrType(at.Elt) {
						isJSValueArrayLit = true
					}
				}
			}
			// JSValue receiver: use jsvalue.Concat wrapper
			if isJSValueReceiver || isJSValueMethodCall(obj) || isJSValueArrayLit {
				addImport("github.com/nnstd/gun/runtime/jsvalue")
				wrapped := make([]ast.Expr, len(args))
				for i, a := range args {
					wrapped[i] = callExpr(selectorExpr(ident("jsvalue"), "From"), a)
				}
				// Wrap array literal in jsvalue.NewArray if needed
				receiver := obj
				if isJSValueArrayLit {
					if cl, ok := obj.(*ast.CompositeLit); ok {
						receiver = callExpr(selectorExpr(ident("jsvalue"), "NewArray"), cl.Elts...)
					}
				}
				concatArgs := append([]ast.Expr{receiver}, wrapped...)
				return callExpr(selectorExpr(ident("jsvalue"), "Concat"), concatArgs...)
			}
			return callExpr(ident("append"), append([]ast.Expr{obj}, args...)...)
		}
	case "push":
		if len(args) > 0 {
			// JSValue receiver: use jsvalue.Push wrapper
			if isJSValueReceiver || isJSValueMethodCall(obj) {
				addImport("github.com/nnstd/gun/runtime/jsvalue")
				wrapped := make([]ast.Expr, len(args))
				for i, a := range args {
					wrapped[i] = callExpr(selectorExpr(ident("jsvalue"), "From"), a)
				}
				pushArgs := append([]ast.Expr{obj}, wrapped...)
				return callExpr(selectorExpr(ident("jsvalue"), "Push"), pushArgs...)
			}
			return callExpr(ident("append"), append([]ast.Expr{obj}, args...)...)
		}
	case "pop":
		// JSValue receiver: use jsvalue.Pop wrapper
		if isJSValueReceiver || isJSValueMethodCall(obj) {
			addImport("github.com/nnstd/gun/runtime/jsvalue")
			return callExpr(selectorExpr(ident("jsvalue"), "Pop"), obj)
		}
		// For typed arrays: arr[len(arr)-1]
		lenCall := callExpr(ident("len"), obj)
		lastIndex := &ast.BinaryExpr{
			X:  lenCall,
			Op: token.SUB,
			Y:  &ast.BasicLit{Kind: token.INT, Value: "1"},
		}
		return &ast.IndexExpr{X: obj, Index: lastIndex}
	case "shift":
		// JSValue receiver: use jsvalue.Shift wrapper
		if isJSValueReceiver || isJSValueMethodCall(obj) {
			addImport("github.com/nnstd/gun/runtime/jsvalue")
			return callExpr(selectorExpr(ident("jsvalue"), "Shift"), obj)
		}
		// For typed arrays: arr[0]
		return &ast.IndexExpr{X: obj, Index: &ast.BasicLit{Kind: token.INT, Value: "0"}}
	case "unshift":
		if len(args) > 0 {
			// JSValue receiver: use jsvalue.Unshift wrapper
			if isJSValueReceiver || isJSValueMethodCall(obj) {
				addImport("github.com/nnstd/gun/runtime/jsvalue")
				wrapped := make([]ast.Expr, len(args))
				for i, a := range args {
					wrapped[i] = callExpr(selectorExpr(ident("jsvalue"), "From"), a)
				}
				unshiftArgs := append([]ast.Expr{obj}, wrapped...)
				return callExpr(selectorExpr(ident("jsvalue"), "Unshift"), unshiftArgs...)
			}
		}
	case "length":
		// JSValue receiver: use len(obj.Array()) for arrays or len(obj.String()) for strings
		if isJSValueReceiver || isJSValueMethodCall(obj) {
			// For now, assume array - string length should be handled by string methods
			return callExpr(ident("len"), callExpr(selectorExpr(obj, "Array")))
		}
		return callExpr(ident("len"), obj)
	case "includes":
		if len(args) > 0 {
			// For JSValue arrays, use jsvalue.Includes wrapper
			if isJSValueReceiver || isJSValueMethodCall(obj) {
				addImport("github.com/nnstd/gun/runtime/jsvalue")
				// Wrap the argument with jsvalue.From to ensure it's a JSValue
				wrappedArg := callExpr(selectorExpr(ident("jsvalue"), "From"), args[0])
				return callExpr(selectorExpr(ident("jsvalue"), "Includes"), obj, wrappedArg)
			}
			// For typed arrays, use slices.Contains
			addImport("slices")
			return callExpr(selectorExpr(ident("slices"), "Contains"), obj, args[0])
		}
	case "splice":
		// arr.splice(start, deleteCount, ...items) → jsvalue.Splice(arr, args...)
		if isJSValueReceiver || isJSValueMethodCall(obj) {
			addImport("github.com/nnstd/gun/runtime/jsvalue")
			spliceArgs := []ast.Expr{obj}
			for _, arg := range args {
				spliceArgs = append(spliceArgs, jsvalueWrapLit(arg))
			}
			return callExpr(selectorExpr(ident("jsvalue"), "Splice"), spliceArgs...)
		}
	case "slice":
		// JSValue receiver: use jsvalue.Slice wrapper
		if isJSValueReceiver || isJSValueMethodCall(obj) {
			addImport("github.com/nnstd/gun/runtime/jsvalue")
			// Build args: wrap each slice argument as JSValue
			sliceArgs := []ast.Expr{obj}
			for _, arg := range args {
				sliceArgs = append(sliceArgs, jsvalueWrapLit(arg))
			}
			return callExpr(selectorExpr(ident("jsvalue"), "Slice"), sliceArgs...)
		}
		if len(args) >= 2 {
			return &ast.SliceExpr{X: obj, Low: normalizeSliceIndex(args[0], obj), High: normalizeSliceIndex(args[1], obj)}
		}
		if len(args) == 1 {
			return &ast.SliceExpr{X: obj, Low: normalizeSliceIndex(args[0], obj)}
		}
	case "join":
		// JSValue receiver: use jsvalue.Join wrapper
		if isJSValueReceiver || isJSValueMethodCall(obj) {
			addImport("github.com/nnstd/gun/runtime/jsvalue")
			// Determine separator (default to ",") — wrap as JSValue
			var sep ast.Expr = callExpr(selectorExpr(ident("jsvalue"), "NewString"), stringLit(","))
			if len(args) >= 1 {
				sep = jsvalueWrapLit(args[0])
			}
			return callExpr(selectorExpr(ident("jsvalue"), "Join"), obj, sep)
		}
	case "map", "filter", "forEach":
		// Transform to package-level function calls: arr.map(fn) → jsvalue.Map(arr, fn)
		// Only for JSValue receivers - typed arrays are handled elsewhere
		if len(args) > 0 && (isJSValueReceiver || isJSValueMethodCall(obj)) {
			addImport("github.com/nnstd/gun/runtime/jsvalue")
			funcName := capitalize(prop) // Map, Filter, ForEach
			return callExpr(selectorExpr(ident("jsvalue"), funcName), append([]ast.Expr{obj}, args...)...)
		}
	case "find", "some", "every", "reduce":
		// Transform to package-level function calls like map/filter/forEach
		if len(args) > 0 && (isJSValueReceiver || isJSValueMethodCall(obj)) {
			addImport("github.com/nnstd/gun/runtime/jsvalue")
			funcName := capitalize(prop) // Find, Some, Every, Reduce
			return callExpr(selectorExpr(ident("jsvalue"), funcName), append([]ast.Expr{obj}, args...)...)
		}
	}
	return nil
}

func transformMapMethod(obj ast.Expr, prop string, args []ast.Expr, addImport func(string)) ast.Expr {
	addImport("github.com/nnstd/gun/runtime/jsvalue")
	switch prop {
	case "get":
		if len(args) > 0 {
			return callExpr(selectorExpr(ident("jsvalue"), "MapGet"), obj, args[0])
		}
	case "set":
		if len(args) >= 2 {
			return callExpr(selectorExpr(ident("jsvalue"), "MapSet"), obj, args[0], args[1])
		}
	case "has":
		if len(args) > 0 {
			return callExpr(selectorExpr(ident("jsvalue"), "MapHas"), obj, args[0])
		}
	case "delete":
		if len(args) > 0 {
			return callExpr(selectorExpr(ident("jsvalue"), "MapDelete"), obj, args[0])
		}
	case "clear":
		return callExpr(selectorExpr(ident("jsvalue"), "MapClear"), obj)
	case "keys":
		return callExpr(selectorExpr(ident("jsvalue"), "MapKeys"), obj)
	case "values":
		return callExpr(selectorExpr(ident("jsvalue"), "MapValues"), obj)
	case "entries":
		return callExpr(selectorExpr(ident("jsvalue"), "MapEntries"), obj)
	case "forEach":
		if len(args) > 0 {
			return callExpr(selectorExpr(ident("jsvalue"), "MapForEach"), obj, args[0])
		}
	}
	return nil
}

func transformSetMethod(obj ast.Expr, prop string, args []ast.Expr, addImport func(string)) ast.Expr {
	addImport("github.com/nnstd/gun/runtime/jsvalue")
	switch prop {
	case "add":
		if len(args) > 0 {
			return callExpr(selectorExpr(ident("jsvalue"), "SetAdd"), obj, args[0])
		}
	case "has":
		if len(args) > 0 {
			return callExpr(selectorExpr(ident("jsvalue"), "SetHas"), obj, args[0])
		}
	case "delete":
		if len(args) > 0 {
			return callExpr(selectorExpr(ident("jsvalue"), "SetDelete"), obj, args[0])
		}
	case "clear":
		return callExpr(selectorExpr(ident("jsvalue"), "SetClear"), obj)
	case "keys":
		return callExpr(selectorExpr(ident("jsvalue"), "SetKeys"), obj)
	case "values":
		return callExpr(selectorExpr(ident("jsvalue"), "SetValues"), obj)
	case "entries":
		return callExpr(selectorExpr(ident("jsvalue"), "SetEntries"), obj)
	case "forEach":
		if len(args) > 0 {
			return callExpr(selectorExpr(ident("jsvalue"), "SetForEach"), obj, args[0])
		}
	}
	return nil
}

func transformObjectCall(prop string, args []ast.Expr, addImport func(string)) ast.Expr {
	addImport("github.com/nnstd/gun/runtime/object")
	switch prop {
	case "keys":
		if len(args) > 0 {
			return callExpr(selectorExpr(ident("object"), "Keys"), args[0])
		}
	case "values":
		if len(args) > 0 {
			return callExpr(selectorExpr(ident("object"), "Values"), args[0])
		}
	case "entries":
		if len(args) > 0 {
			return callExpr(selectorExpr(ident("object"), "Entries"), args[0])
		}
	case "assign":
		if len(args) > 0 {
			return callExpr(selectorExpr(ident("object"), "Assign"), args...)
		}
	case "create":
		if len(args) > 0 {
			return callExpr(selectorExpr(ident("object"), "Create"), args[0])
		}
		return callExpr(selectorExpr(ident("object"), "Create"), ident("nil"))
	case "defineProperty":
		if len(args) >= 3 {
			return callExpr(selectorExpr(ident("object"), "DefineProperty"), args[0], args[1], args[2])
		}
		return ident("nil")
	}
	return nil
}
