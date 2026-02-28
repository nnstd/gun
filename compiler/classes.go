package compiler

import (
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

	className := capitalize(nameNode.Utf8Text(t.source))
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
					continue
				}
				mName := nameN.Utf8Text(t.source)

				// Check if static
				isStatic := false
				for j := uint(0); j < member.ChildCount(); j++ {
					child := member.Child(j)
					if child.Kind() == "static" || (child.IsNamed() == false && child.Utf8Text(t.source) == "static") {
						isStatic = true
						break
					}
				}

				if mName == "constructor" {
					ctorParamsNode = member.ChildByFieldName("parameters")
					ctorStmts = t.transformClassConstructorBody(member)
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

	// Add method and static setups as init statements
	if len(methodSetups) > 0 || len(staticSetups) > 0 {
		allSetups := append(methodSetups, staticSetups...)
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

	// Add 'this' to scope
	t.addToCurrentScope("this", false)

	// Build the function body
	var body *ast.BlockStmt
	if bodyNode != nil {
		body = t.transformBlock(bodyNode)
	} else {
		body = blockStmt()
	}

	// For instance methods, 'this' needs to be the first arg
	// The NewClass helper doesn't pass 'this' to prototype methods automatically,
	// so methods are regular functions. 'this' is accessed via closure or args.
	// For now, methods don't receive 'this' — they're called as obj.Get("method").Call()
	// where 'this' binding would need explicit support.

	// Build variadic function params: func(args ...*jsvalue.JSValue) *jsvalue.JSValue
	// Unpack named params from args
	var argUnpackStmts []ast.Stmt
	if paramsNode != nil {
		paramNames := extractParamNames(paramsNode, t.source)
		for i, pName := range paramNames {
			if pName == "" {
				continue
			}
			pName = sanitizeIdent(pName)
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
	body.List = append(argUnpackStmts, body.List...)

	params := fieldList(&ast.Field{
		Names: []*ast.Ident{ident("_args")},
		Type:  &ast.Ellipsis{Elt: jsValuePtrType()},
	})

	// Determine return type
	var results *ast.FieldList
	if hasReturnValue(body) {
		results = fieldList(field("", jsValuePtrType()))
		t.addAliasedImport("github.com/nnstd/gun/runtime/jsvalue", "jsvalue")
		wrapReturnsWithJSValue(body)
	}
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
