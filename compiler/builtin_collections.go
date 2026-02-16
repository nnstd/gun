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
			// JSValue receiver: coerce to array, append, wrap result
			if isJSValueReceiver || isJSValueMethodCall(obj) {
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
			if isJSValueReceiver || isJSValueMethodCall(obj) {
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
	case "pop":
		// JSValue receiver: get last element from array
		if isJSValueReceiver || isJSValueMethodCall(obj) {
			addImport("github.com/nnstd/gun/runtime/jsvalue")
			// Create IIFE: func() *jsvalue.JSValue { arr := obj.Array(); if len(arr) > 0 { return arr[len(arr)-1] }; return jsvalue.NewUndefined() }()
			arrIdent := ident("_arr")
			arrDecl := &ast.AssignStmt{
				Lhs: []ast.Expr{arrIdent},
				Tok: token.DEFINE,
				Rhs: []ast.Expr{callExpr(selectorExpr(obj, "Array"))},
			}
			lenCheck := &ast.BinaryExpr{
				X:  callExpr(ident("len"), arrIdent),
				Op: token.GTR,
				Y:  &ast.BasicLit{Kind: token.INT, Value: "0"},
			}
			lastIndex := &ast.BinaryExpr{
				X:  callExpr(ident("len"), arrIdent),
				Op: token.SUB,
				Y:  &ast.BasicLit{Kind: token.INT, Value: "1"},
			}
			returnLast := &ast.ReturnStmt{
				Results: []ast.Expr{&ast.IndexExpr{X: arrIdent, Index: lastIndex}},
			}
			ifStmt := &ast.IfStmt{
				Cond: lenCheck,
				Body: &ast.BlockStmt{List: []ast.Stmt{returnLast}},
			}
			returnUndef := &ast.ReturnStmt{
				Results: []ast.Expr{callExpr(selectorExpr(ident("jsvalue"), "NewUndefined"))},
			}
			iife := &ast.CallExpr{
				Fun: &ast.FuncLit{
					Type: &ast.FuncType{
						Results: &ast.FieldList{
							List: []*ast.Field{{Type: &ast.StarExpr{X: selectorExpr(ident("jsvalue"), "JSValue")}}},
						},
					},
					Body: &ast.BlockStmt{
						List: []ast.Stmt{arrDecl, ifStmt, returnUndef},
					},
				},
			}
			return iife
		}
		// For typed arrays: arr[len(arr)-1]
		lenCall := callExpr(ident("len"), obj)
		lastIndex := &ast.BinaryExpr{
			X:  lenCall,
			Op: token.SUB,
			Y:  &ast.BasicLit{Kind: token.INT, Value: "1"},
		}
		return &ast.IndexExpr{X: obj, Index: lastIndex}
	case "length":
		// JSValue receiver: use len(obj.Array()) for arrays or len(obj.String()) for strings
		if isJSValueReceiver || isJSValueMethodCall(obj) {
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
		if isJSValueReceiver || isJSValueMethodCall(obj) {
			addImport("github.com/nnstd/gun/runtime/jsvalue")
			sliceExpr := &ast.SliceExpr{X: callExpr(selectorExpr(obj, "Array"))}
			if len(args) >= 1 {
				sliceExpr.Low = normalizeSliceIndex(args[0], callExpr(selectorExpr(obj, "Array")))
			}
			if len(args) >= 2 {
				sliceExpr.High = normalizeSliceIndex(args[1], callExpr(selectorExpr(obj, "Array")))
			}
			// Wrap in NewArray with spread: jsvalue.NewArray(sliceExpr...)
			return &ast.CallExpr{
				Fun:      selectorExpr(ident("jsvalue"), "NewArray"),
				Args:     []ast.Expr{sliceExpr},
				Ellipsis: token.Pos(1), // indicates spread operator
			}
		}
		if len(args) >= 2 {
			return &ast.SliceExpr{X: obj, Low: normalizeSliceIndex(args[0], obj), High: normalizeSliceIndex(args[1], obj)}
		}
		if len(args) == 1 {
			return &ast.SliceExpr{X: obj, Low: normalizeSliceIndex(args[0], obj)}
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
