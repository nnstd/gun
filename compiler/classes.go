package compiler

import (
	"fmt"
	"go/ast"
	"go/token"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

// transformClassDecl transforms a TypeScript class into JSValue-based objects.
//
// class Dog extends Animal {
//     name: string;
//     constructor(name) { this.name = name; }
//     bark() { return this.name; }
//     static create() { return new Dog("Rex"); }
// }
//
// Becomes:
//
// var Dog = jsvalue.NewClass(func(this *jsvalue.JSValue, args ...*jsvalue.JSValue) *jsvalue.JSValue {
//     this.Set("name", args[0])
//     return nil
// }, Animal)
//
// func init() {
//     Dog.Get("prototype").Set("bark", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
//         this := args[0]  // NOT YET — see note below
//         return this.Get("name")
//     }))
//     Dog.Set("create", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
//         return Dog.Call(jsvalue.NewString("Rex"))
//     }))
// }
func (t *Transformer) transformClassDecl(node *sitter.Node) []ast.Decl {
	nameNode := node.ChildByFieldName("name")
	bodyNode := node.ChildByFieldName("body")

	if nameNode == nil {
		return nil
	}

	className := t.resolveGoName(nameNode.Utf8Text(t.source))
	t.addAliasedImport("github.com/nnstd/gun/runtime/jsvalue", "jsvalue")

	// Check for extends (inside class_heritage → extends_clause)
	var parentExpr ast.Expr = ident("nil")
	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		if child.Kind() == "class_heritage" {
			for j := uint(0); j < child.NamedChildCount(); j++ {
				ext := child.NamedChild(j)
				if ext.Kind() == "extends_clause" {
					valNode := ext.ChildByFieldName("value")
					if valNode != nil {
						parentExpr = t.transformExpr(valNode)
					} else if ext.NamedChildCount() > 0 {
						parentExpr = t.transformExpr(ext.NamedChild(0))
					}
				}
			}
		}
	}

	// Collect constructor body, methods, and static methods
	var ctorStmts []ast.Stmt
	var ctorParamsNode *sitter.Node
	var methodSetups []ast.Stmt
	var staticSetups []ast.Stmt

	if bodyNode != nil {
		for i := uint(0); i < bodyNode.NamedChildCount(); i++ {
			member := bodyNode.NamedChild(i)
			switch member.Kind() {
			case "public_field_definition", "property_definition":
				// Class field: this.name = defaultValue (in constructor)
				// For now, skip — fields are set via constructor or as prototype properties

			case "method_definition":
				nameN := member.ChildByFieldName("name")
				if nameN == nil {
					continue
				}
				if nameN.Kind() == "computed_property_name" {
					// Extract side-effecting expressions from computed property names.
					// TypeScript compiles private fields to: [(_field = new WeakMap(), ...)](argv) { }
					// The assignment expressions inside the brackets init WeakMap backing stores.
					t.extractComputedPropertySideEffects(nameN, &staticSetups)

					// If the computed property is a simple identifier like [kSymbol],
					// transpile it as a method with a dynamic name using fmt.Sprint.
					// Skip the WeakMap init pattern (which has sequence/assignment expressions).
					if nameN.NamedChildCount() == 1 && nameN.NamedChild(0).Kind() == "identifier" {
						symName := nameN.NamedChild(0).Utf8Text(t.source)
						// Check if static
						isStatic := false
						for j := uint(0); j < member.ChildCount(); j++ {
							child := member.Child(j)
							if child.Kind() == "static" || (!child.IsNamed() && child.Utf8Text(t.source) == "static") {
								isStatic = true
								break
							}
						}
						bodyNode := member.ChildByFieldName("body")
						if bodyNode != nil {
							t.addImport("fmt")
							stmt := t.transformClassMethodWithDynamicName(className, member, symName, isStatic)
							if stmt != nil {
								if isStatic {
									staticSetups = append(staticSetups, stmt)
								} else {
									methodSetups = append(methodSetups, stmt)
								}
							}
						}
					}
					continue
				}
				mName := nameN.Utf8Text(t.source)

				// Check if static
				isStatic := false
				for j := uint(0); j < member.ChildCount(); j++ {
					child := member.Child(j)
					if child.Kind() == "static" || (!child.IsNamed() && child.Utf8Text(t.source) == "static") {
						isStatic = true
						break
					}
				}

				if mName == "constructor" {
					ctorParamsNode = member.ChildByFieldName("parameters")
					// Set parent for super() calls in the constructor
					prevParent := t.currentClassParent
					t.currentClassParent = parentExpr
					ctorStmts = t.transformClassConstructorBody(member)
					t.currentClassParent = prevParent
				} else if isStatic {
					stmt := t.buildMethodSetup(className, mName, member, true)
					if stmt != nil {
						staticSetups = append(staticSetups, stmt)
					}
				} else {
					stmt := t.buildMethodSetup(className, mName, member, false)
					if stmt != nil {
						methodSetups = append(methodSetups, stmt)
					}
				}
			}
		}
	}

	// Build constructor function literal
	// func(this *jsvalue.JSValue, args ...*jsvalue.JSValue) *jsvalue.JSValue { ... }
	ctorParams := fieldList(
		field("this", jsValuePtrType()),
		&ast.Field{
			Names: []*ast.Ident{ident("_args")},
			Type:  &ast.Ellipsis{Elt: jsValuePtrType()},
		},
	)

	// Unpack named params from args if the constructor had named params
	var argUnpackStmts []ast.Stmt
	if ctorParamsNode != nil {
		paramNames := extractParamNames(ctorParamsNode, t.source)
		for i, pName := range paramNames {
			if pName == "" {
				continue
			}
			pName = sanitizeIdent(pName)
			// var paramName *jsvalue.JSValue; if len(args) > i { paramName = args[i] }
			argUnpackStmts = append(argUnpackStmts,
				&ast.DeclStmt{Decl: varDecl(pName, jsValuePtrType(), nil)},
				&ast.IfStmt{
					Cond: &ast.BinaryExpr{
						X:  callExpr(ident("len"), ident("_args")),
						Op: token.GTR,
						Y:  intLit(itoa(i)),
					},
					Body: blockStmt(
						assignStmt([]ast.Expr{ident(pName)}, []ast.Expr{
							&ast.IndexExpr{X: ident("_args"), Index: intLit(itoa(i))},
						}),
					),
				},
			)
		}
	}

	allCtorStmts := append(argUnpackStmts, ctorStmts...)
	allCtorStmts = append(allCtorStmts, returnStmt(ident("nil")))

	ctorFuncLit := &ast.FuncLit{
		Type: &ast.FuncType{
			Params:  ctorParams,
			Results: fieldList(field("", jsValuePtrType())),
		},
		Body: &ast.BlockStmt{List: allCtorStmts},
	}

	// var ClassName = jsvalue.NewClass(ctorFn, parentOrNil)
	classVarDecl := varDecl(className, nil,
		callExpr(selectorExpr(ident("jsvalue"), "NewClass"), ctorFuncLit, parentExpr))

	var decls []ast.Decl
	decls = append(decls, classVarDecl)

	// Add method and static setups as init statements.
	// Static setups (including WeakMap initializations) must come first
	// since method bodies may reference the WeakMap backing variables.
	if len(methodSetups) > 0 || len(staticSetups) > 0 {
		allSetups := append(staticSetups, methodSetups...)
		initBody := &ast.BlockStmt{List: allSetups}
		initFn := funcDecl("init", fieldList(), nil, initBody)
		decls = append(decls, initFn)
	}

	// Track the class as a typed local (constructor function, not JSValue var)
	if len(t.localScopes) > 0 {
		t.addToCurrentScope(className, true)
	}
	t.pkgVarTyped[className] = false // class constructor is a JSValue function

	return decls
}

// transformClassConstructorBody transforms the constructor body,
// rewriting this.x to this.Set("x", ...) / this.Get("x").
func (t *Transformer) transformClassConstructorBody(node *sitter.Node) []ast.Stmt {
	paramsNode := node.ChildByFieldName("parameters")
	bodyNode := node.ChildByFieldName("body")

	paramInfo := extractParamInfo(paramsNode, t.source)
	t.pushTypedScope(paramInfo)
	defer t.popScope()

	// Add 'this' to scope so it's recognized as a local
	t.addToCurrentScope("this", false)

	if bodyNode == nil {
		return nil
	}

	block := t.transformBlock(bodyNode)
	return block.List
}

// buildMethodSetup creates a statement that sets a method on the class prototype
// (or the class itself for static methods).
//
// For instance methods: ClassName.Get("prototype").Set("methodName", jsvalue.NewFunction(...))
// For static methods: ClassName.Set("methodName", jsvalue.NewFunction(...))
func (t *Transformer) buildMethodSetup(className, methodName string, node *sitter.Node, isStatic bool) ast.Stmt {
	paramsNode := node.ChildByFieldName("parameters")
	bodyNode := node.ChildByFieldName("body")

	paramInfo := extractParamInfo(paramsNode, t.source)
	t.pushTypedScope(paramInfo)
	defer t.popScope()

	// Add 'this' to scope and track as JSValue local so method calls
	// on 'this' use .Get().Call() instead of capitalized selectors.
	t.addToCurrentScope("this", false)
	t.jsvalueLocals["this"] = true

	// Set class method flag for arguments keyword handling
	prevInClassMethod := t.inClassMethod
	t.inClassMethod = !isStatic
	defer func() { t.inClassMethod = prevInClassMethod }()

	// Pre-register rest params as JSValue locals BEFORE body transformation
	// so subscript access (args[0]) uses .Index() instead of Go bracket indexing.
	if paramsNode != nil {
		restFlags := extractRestFlags(paramsNode, t.source)
		paramNames := extractParamNames(paramsNode, t.source)
		for i, pName := range paramNames {
			if i < len(restFlags) && restFlags[i] {
				t.jsvalueLocals[sanitizeIdent(pName)] = true
			}
		}
	}

	// Build the function body
	var body *ast.BlockStmt
	if bodyNode != nil {
		body = t.transformBlock(bodyNode)
	} else {
		body = blockStmt()
	}

	// Methods are called as obj.Get("method").Call(args...).
	// The compiler injects 'this' as the first element of _args at call sites,
	// so we extract it here: this := _args[0], then unpack named params from _args[1:].
	var argUnpackStmts []ast.Stmt

	// Extract 'this' from _args[0]
	if !isStatic {
		argUnpackStmts = append(argUnpackStmts,
			&ast.DeclStmt{Decl: varDecl("this", jsValuePtrType(), nil)},
			&ast.IfStmt{
				Cond: &ast.BinaryExpr{
					X:  callExpr(ident("len"), ident("_args")),
					Op: token.GTR,
					Y:  intLit("0"),
				},
				Body: blockStmt(
					assignStmt([]ast.Expr{ident("this")}, []ast.Expr{
						&ast.IndexExpr{X: ident("_args"), Index: intLit("0")},
					}),
				),
			},
			// Suppress "declared and not used" if body doesn't reference this
			assignStmt([]ast.Expr{ident("_")}, []ast.Expr{ident("this")}),
		)
	}

	// Unpack named params from _args (offset by 1 for instance methods to skip 'this')
	offset := 0
	if !isStatic {
		offset = 1
	}
	if paramsNode != nil {
		paramNames := extractParamNames(paramsNode, t.source)
		isRest := extractRestFlags(paramsNode, t.source)
		for i, pName := range paramNames {
			if pName == "" {
				continue
			}
			pName = sanitizeIdent(pName)
			idx := i + offset
			if i < len(isRest) && isRest[i] {
				// Rest param: args = jsvalue.NewArray(_args[idx:]...)
				// Wrapped as JSValue array so .Get(), .Len() etc. work.
				t.jsvalueLocals[pName] = true
				newArrayCall := callExpr(selectorExpr(ident("jsvalue"), "NewArray"),
					&ast.SliceExpr{X: ident("_args"), Low: intLit(itoa(idx))})
				newArrayCall.Ellipsis = 1 // spread: NewArray(_args[idx:]...)
				argUnpackStmts = append(argUnpackStmts,
					&ast.AssignStmt{
						Lhs: []ast.Expr{ident(pName)},
						Tok: token.DEFINE,
						Rhs: []ast.Expr{newArrayCall},
					},
				)
			} else {
				argUnpackStmts = append(argUnpackStmts,
					&ast.DeclStmt{Decl: varDecl(pName, jsValuePtrType(), nil)},
					&ast.IfStmt{
						Cond: &ast.BinaryExpr{
							X:  callExpr(ident("len"), ident("_args")),
							Op: token.GTR,
							Y:  intLit(itoa(idx)),
						},
						Body: blockStmt(
							assignStmt([]ast.Expr{ident(pName)}, []ast.Expr{
								&ast.IndexExpr{X: ident("_args"), Index: intLit(itoa(idx))},
							}),
						),
					},
					// Suppress "declared and not used" for unused params
					assignStmt([]ast.Expr{ident("_")}, []ast.Expr{ident(pName)}),
				)
			}
		}
	}

	// Handle destructured params: { a, b } or [a, b] patterns
	// After extracting _param0 from _args, destructure it into individual vars
	if paramsNode != nil {
		paramIdx := 0
		for i := uint(0); i < paramsNode.NamedChildCount(); i++ {
			param := paramsNode.NamedChild(i)
			var nameNode *sitter.Node
			switch param.Kind() {
			case "required_parameter", "optional_parameter":
				nameNode = param.ChildByFieldName("pattern")
			}
			if nameNode != nil && (nameNode.Kind() == "object_pattern" || nameNode.Kind() == "array_pattern") {
				syntheticName := fmt.Sprintf("_param%d", paramIdx)
				stmts := t.transformDestructuringFromExpr(nameNode, ident(syntheticName))
				argUnpackStmts = append(argUnpackStmts, stmts...)
			}
			paramIdx++
		}
	}

	body.List = append(argUnpackStmts, body.List...)

	params := fieldList(&ast.Field{
		Names: []*ast.Ident{ident("_args")},
		Type:  &ast.Ellipsis{Elt: jsValuePtrType()},
	})

	// All methods wrapped in jsvalue.NewFunction must return *jsvalue.JSValue.
	// Always wrap returns (both value returns and bare returns → nil).
	results := fieldList(field("", jsValuePtrType()))
	t.addAliasedImport("github.com/nnstd/gun/runtime/jsvalue", "jsvalue")
	wrapReturnsWithJSValue(body)
	ensureTrailingReturn(body, results)

	fnLit := &ast.FuncLit{
		Type: &ast.FuncType{Params: params, Results: results},
		Body: body,
	}

	// Wrap in jsvalue.NewFunction
	t.addAliasedImport("github.com/nnstd/gun/runtime/jsvalue", "jsvalue")
	fnVal := callExpr(selectorExpr(ident("jsvalue"), "NewFunction"), fnLit)

	// Build the .Set() call
	var target ast.Expr
	if isStatic {
		// ClassName.Set("methodName", fn)
		target = ident(className)
	} else {
		// ClassName.Get("prototype").Set("methodName", fn)
		target = callExpr(selectorExpr(ident(className), "Get"), stringLit("prototype"))
	}

	return exprStmt(callExpr(selectorExpr(target, "Set"), stringLit(methodName), fnVal))
}

// transformClassMethodWithDynamicName transpiles a class method that has a computed
// property name like [kSymbolName]. Uses fmt.Sprint(symbolVar) as the property key.
func (t *Transformer) transformClassMethodWithDynamicName(className string, member *sitter.Node, symName string, isStatic bool) ast.Stmt {
	paramsNode := member.ChildByFieldName("parameters")
	bodyNode := member.ChildByFieldName("body")

	paramInfo := extractParamInfo(paramsNode, t.source)
	t.pushTypedScope(paramInfo)
	defer t.popScope()

	t.addToCurrentScope("this", false)
	t.jsvalueLocals["this"] = true

	prevInClassMethod := t.inClassMethod
	t.inClassMethod = !isStatic
	defer func() { t.inClassMethod = prevInClassMethod }()

	// Pre-register rest params
	if paramsNode != nil {
		restFlags := extractRestFlags(paramsNode, t.source)
		paramNames := extractParamNames(paramsNode, t.source)
		for i, pName := range paramNames {
			if i < len(restFlags) && restFlags[i] {
				t.jsvalueLocals[sanitizeIdent(pName)] = true
			}
		}
	}

	var body *ast.BlockStmt
	if bodyNode != nil {
		body = t.transformBlock(bodyNode)
	} else {
		body = blockStmt()
	}

	// Build _args variadic wrapper with 'this' extraction + param unpacking
	params := fieldList(&ast.Field{
		Names: []*ast.Ident{ident("_args")},
		Type:  &ast.Ellipsis{Elt: jsValuePtrType()},
	})

	var argUnpackStmts []ast.Stmt
	if !isStatic {
		argUnpackStmts = append(argUnpackStmts,
			&ast.DeclStmt{Decl: varDecl("this", jsValuePtrType(), nil)},
			&ast.IfStmt{
				Cond: &ast.BinaryExpr{X: callExpr(ident("len"), ident("_args")), Op: token.GTR, Y: intLit("0")},
				Body: blockStmt(assignStmt([]ast.Expr{ident("this")}, []ast.Expr{&ast.IndexExpr{X: ident("_args"), Index: intLit("0")}})),
			},
			assignStmt([]ast.Expr{ident("_")}, []ast.Expr{ident("this")}),
		)
	}
	offset := 0
	if !isStatic {
		offset = 1
	}
	if paramsNode != nil {
		paramNames := extractParamNames(paramsNode, t.source)
		for i, pName := range paramNames {
			if pName == "" {
				continue
			}
			pName = sanitizeIdent(pName)
			idx := i + offset
			argUnpackStmts = append(argUnpackStmts,
				&ast.DeclStmt{Decl: varDecl(pName, jsValuePtrType(), nil)},
				&ast.IfStmt{
					Cond: &ast.BinaryExpr{X: callExpr(ident("len"), ident("_args")), Op: token.GTR, Y: intLit(itoa(idx))},
					Body: blockStmt(assignStmt([]ast.Expr{ident(pName)}, []ast.Expr{&ast.IndexExpr{X: ident("_args"), Index: intLit(itoa(idx))}})),
				},
				assignStmt([]ast.Expr{ident("_")}, []ast.Expr{ident(pName)}),
			)
		}
	}
	body.List = append(argUnpackStmts, body.List...)

	results := fieldList(field("", jsValuePtrType()))
	t.addAliasedImport("github.com/nnstd/gun/runtime/jsvalue", "jsvalue")
	wrapReturnsWithJSValue(body)
	ensureTrailingReturn(body, results)

	fnLit := &ast.FuncLit{
		Type: &ast.FuncType{Params: params, Results: results},
		Body: body,
	}
	fnVal := callExpr(selectorExpr(ident("jsvalue"), "NewFunction"), fnLit)

	// Build the .Set(jsvalue.PropertyKey(symName), fn) call
	var target ast.Expr
	if isStatic {
		target = ident(className)
	} else {
		target = callExpr(selectorExpr(ident(className), "Get"), stringLit("prototype"))
	}
	key := callExpr(selectorExpr(ident("jsvalue"), "PropertyKey"), ident(symName))
	return exprStmt(callExpr(selectorExpr(target, "Set"), key, fnVal))
}

// extractComputedPropertySideEffects processes computed property names in class bodies
// that contain side-effecting assignment expressions. This is the TypeScript pattern for
// private class fields: [(_field1 = new WeakMap(), _field2 = new WeakMap(), kName)](argv) { }
// The assignment expressions initialize WeakMap backing stores for private fields.
func (t *Transformer) extractComputedPropertySideEffects(nameNode *sitter.Node, stmts *[]ast.Stmt) {
	// computed_property_name has one expression child inside the brackets
	for i := uint(0); i < nameNode.NamedChildCount(); i++ {
		child := nameNode.NamedChild(i)
		t.extractAssignmentExprs(child, stmts)
	}
}

// extractAssignmentExprs recursively finds assignment expressions in a tree-sitter
// node and converts them to Go assignment statements.
func (t *Transformer) extractAssignmentExprs(node *sitter.Node, stmts *[]ast.Stmt) {
	if node == nil {
		return
	}
	switch node.Kind() {
	case "assignment_expression":
		leftNode := node.ChildByFieldName("left")
		rightNode := node.ChildByFieldName("right")
		if leftNode != nil && rightNode != nil {
			lhs := t.transformExpr(leftNode)
			rhs := t.transformExpr(rightNode)
			if lhs != nil && rhs != nil {
				rhs = t.wrapAsJSValue(rhs)
				*stmts = append(*stmts, assignStmt([]ast.Expr{lhs}, []ast.Expr{rhs}))
			}
		}
	case "sequence_expression", "parenthesized_expression":
		// Recurse into comma expressions and parenthesized groups
		for i := uint(0); i < node.NamedChildCount(); i++ {
			t.extractAssignmentExprs(node.NamedChild(i), stmts)
		}
	}
}
