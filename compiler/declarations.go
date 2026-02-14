package compiler

import (
	"fmt"
	"go/ast"
	"go/token"
	"strings"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

func (t *Transformer) transformVarDecl(node *sitter.Node) []ast.Decl {
	var decls []ast.Decl
	isConst := false

	// Check if this is a const declaration
	for i := uint(0); i < node.ChildCount(); i++ {
		child := node.Child(i)
		if child.Kind() == "const" {
			isConst = true
			break
		}
	}

	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		if child.Kind() != "variable_declarator" {
			continue
		}

		nameNode := child.ChildByFieldName("name")
		typeNode := child.ChildByFieldName("type")
		valueNode := child.ChildByFieldName("value")

		if nameNode == nil {
			continue
		}

		name := sanitizeIdent(nameNode.Utf8Text(t.source))

		// Handle destructuring patterns
		if nameNode.Kind() == "object_pattern" || nameNode.Kind() == "array_pattern" {
			if valueNode != nil {
				stmts := t.transformDestructuring(nameNode, valueNode)
				for _, stmt := range stmts {
					decls = append(decls, &ast.FuncDecl{
						Name: ident("_destructure_placeholder"),
						Type: &ast.FuncType{Params: fieldList()},
						Body: blockStmt(stmt),
					})
				}
			}
			continue
		}

		var typ ast.Expr
		if typeNode != nil {
			typ = t.getTypeAnnotation(typeNode)
		}

		var value ast.Expr
		if valueNode != nil {
			value = t.transformExpr(valueNode)
		}

		// Track variable types for known constructors (e.g. new Hono() → "hono")
		if valueNode != nil && valueNode.Kind() == "new_expression" {
			ctorNode := valueNode.ChildByFieldName("constructor")
			if ctorNode != nil {
				ctorName := ctorNode.Utf8Text(t.source)
				if imp, ok := t.importedNames[ctorName]; ok {
					t.varTypes[name] = imp.goPkgName
				}
			}
		}

		// No type and no value → default to *jsvalue.JSValue
		if typ == nil && value == nil {
			typ = ptrType(selectorExpr(ident("jsvalue"), "JSValue"))
			t.addAliasedImport("github.com/nnstd/gun/runtime/jsvalue", "jsvalue")
		}

		// If const with simple literal value, use Go const
		if isConst && value != nil && isConstCompatible(value) && (typ == nil || isConstType(typ)) {
			decls = append(decls, constDecl(name, typ, value))
		} else {
			decls = append(decls, varDecl(name, typ, value))
		}
	}

	return decls
}

func isConstCompatible(expr ast.Expr) bool {
	switch expr.(type) {
	case *ast.BasicLit:
		return true
	case *ast.UnaryExpr:
		return true
	default:
		return false
	}
}

func isConstType(expr ast.Expr) bool {
	id, ok := expr.(*ast.Ident)
	if !ok {
		return false
	}
	switch id.Name {
	case "string", "float64", "int", "int64", "bool":
		return true
	default:
		return false
	}
}

func (t *Transformer) transformFuncDecl(node *sitter.Node, exported bool) *ast.FuncDecl {
	nameNode := node.ChildByFieldName("name")
	paramsNode := node.ChildByFieldName("parameters")
	returnTypeNode := node.ChildByFieldName("return_type")
	bodyNode := node.ChildByFieldName("body")

	if nameNode == nil {
		return nil
	}

	name := nameNode.Utf8Text(t.source)
	if exported {
		name = capitalize(name)
	}

	params := t.transformParams(paramsNode)
	var results *ast.FieldList
	if returnTypeNode != nil {
		retType := t.getTypeAnnotation(returnTypeNode)
		if retType != nil {
			results = fieldList(field("", retType))
		}
	}

	var body *ast.BlockStmt
	if bodyNode != nil {
		body = t.transformBlock(bodyNode)
	} else {
		body = blockStmt()
	}

	if results == nil {
		if inferred := inferReturnType(body); inferred != nil {
			results = inferred
		} else if hasReturnValue(body) {
			results = fieldList(field("", ptrType(selectorExpr(ident("jsvalue"), "JSValue"))))
			t.addAliasedImport("github.com/nnstd/gun/runtime/jsvalue", "jsvalue")
			t.addImport("fmt")
			wrapReturnsWithJSValue(body)
		}
	}

	return funcDecl(name, params, results, body)
}

func (t *Transformer) transformParams(node *sitter.Node) *ast.FieldList {
	if node == nil {
		return fieldList()
	}

	var fields []*ast.Field
	for i := uint(0); i < node.NamedChildCount(); i++ {
		param := node.NamedChild(i)
		switch param.Kind() {
		case "required_parameter", "optional_parameter":
			nameNode := param.ChildByFieldName("pattern")
			typeNode := param.ChildByFieldName("type")

			pName := "_"
			if nameNode != nil {
				pName = sanitizeIdent(nameNode.Utf8Text(t.source))
			}

			var pType ast.Expr = ident("any")
			if typeNode != nil {
				mapped := t.getTypeAnnotation(typeNode)
				if mapped != nil {
					pType = mapped
				}
			}

			// Optional params become pointer types
			if param.Kind() == "optional_parameter" {
				pType = ptrType(pType)
			}

			fields = append(fields, field(pName, pType))

		case "rest_parameter":
			nameNode := param.ChildByFieldName("pattern")
			typeNode := param.ChildByFieldName("type")

			pName := "args"
			if nameNode != nil {
				pName = sanitizeIdent(nameNode.Utf8Text(t.source))
			}

			var elemType ast.Expr = ident("any")
			if typeNode != nil {
				mapped := t.getTypeAnnotation(typeNode)
				if mapped != nil {
					// If it's already a slice type, use the element type
					if at, ok := mapped.(*ast.ArrayType); ok {
						elemType = at.Elt
					} else {
						elemType = mapped
					}
				}
			}

			fields = append(fields, field(pName, &ast.Ellipsis{Elt: elemType}))
		}
	}

	return fieldList(fields...)
}

func (t *Transformer) transformBlock(node *sitter.Node) *ast.BlockStmt {
	if node == nil {
		return blockStmt()
	}

	var stmts []ast.Stmt
	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		if s := t.transformStmt(child); s != nil {
			stmts = append(stmts, s)
		}
	}
	return &ast.BlockStmt{List: stmts}
}

func (t *Transformer) transformInterfaceDecl(node *sitter.Node) ast.Decl {
	nameNode := node.ChildByFieldName("name")
	bodyNode := node.ChildByFieldName("body")

	if nameNode == nil {
		return nil
	}

	name := capitalize(nameNode.Utf8Text(t.source))

	if bodyNode == nil {
		return typeDecl(name, &ast.InterfaceType{Methods: fieldList()})
	}

	// Determine if all members are method signatures
	allMethods := true
	count := bodyNode.NamedChildCount()
	for i := uint(0); i < count; i++ {
		member := bodyNode.NamedChild(i)
		if member.Kind() != "method_signature" {
			allMethods = false
			break
		}
	}

	if allMethods {
		return t.transformInterfaceAsGoInterface(name, bodyNode)
	}
	return t.transformInterfaceAsStruct(name, bodyNode)
}

func (t *Transformer) transformInterfaceAsGoInterface(name string, body *sitter.Node) ast.Decl {
	var methods []*ast.Field
	for i := uint(0); i < body.NamedChildCount(); i++ {
		member := body.NamedChild(i)
		if member.Kind() != "method_signature" {
			continue
		}

		mNameNode := member.ChildByFieldName("name")
		paramsNode := member.ChildByFieldName("parameters")
		returnNode := member.ChildByFieldName("return_type")

		if mNameNode == nil {
			continue
		}

		mName := capitalize(mNameNode.Utf8Text(t.source))
		params := t.transformParams(paramsNode)

		var results *ast.FieldList
		if returnNode != nil {
			retType := t.getTypeAnnotation(returnNode)
			if retType != nil {
				results = fieldList(field("", retType))
			}
		}

		methods = append(methods, &ast.Field{
			Names: []*ast.Ident{ident(mName)},
			Type:  &ast.FuncType{Params: params, Results: results},
		})
	}

	return typeDecl(name, &ast.InterfaceType{Methods: fieldList(methods...)})
}

func (t *Transformer) transformInterfaceAsStruct(name string, body *sitter.Node) ast.Decl {
	var fields []*ast.Field
	for i := uint(0); i < body.NamedChildCount(); i++ {
		member := body.NamedChild(i)
		switch member.Kind() {
		case "property_signature":
			pNameNode := member.ChildByFieldName("name")
			pTypeNode := member.ChildByFieldName("type")
			if pNameNode == nil {
				continue
			}
			pName := capitalize(pNameNode.Utf8Text(t.source))
			var pType ast.Expr = ident("any")
			if pTypeNode != nil {
				mapped := t.getTypeAnnotation(pTypeNode)
				if mapped != nil {
					pType = mapped
				}
			}

			// Add json tag
			origName := pNameNode.Utf8Text(t.source)
			tag := "`json:\"" + origName + "\"`"
			fields = append(fields, &ast.Field{
				Names: []*ast.Ident{ident(pName)},
				Type:  pType,
				Tag:   basicLit(token.STRING, tag),
			})

		case "method_signature":
			// Include method signatures as function-typed fields
			mNameNode := member.ChildByFieldName("name")
			paramsNode := member.ChildByFieldName("parameters")
			returnNode := member.ChildByFieldName("return_type")
			if mNameNode == nil {
				continue
			}
			mName := capitalize(mNameNode.Utf8Text(t.source))
			params := t.transformParams(paramsNode)
			var results *ast.FieldList
			if returnNode != nil {
				retType := t.getTypeAnnotation(returnNode)
				if retType != nil {
					results = fieldList(field("", retType))
				}
			}
			fields = append(fields, &ast.Field{
				Names: []*ast.Ident{ident(mName)},
				Type:  &ast.FuncType{Params: params, Results: results},
			})
		}
	}

	return typeDecl(name, &ast.StructType{Fields: fieldList(fields...)})
}

func (t *Transformer) transformEnumDecl(node *sitter.Node) []ast.Decl {
	nameNode := node.ChildByFieldName("name")
	bodyNode := node.ChildByFieldName("body")

	if nameNode == nil || bodyNode == nil {
		return nil
	}

	enumName := capitalize(nameNode.Utf8Text(t.source))

	// Type declaration
	typeD := typeDecl(enumName, ident("int"))

	// Collect members — tree-sitter uses:
	//   property_identifier for simple members (Red, Green, Blue)
	//   enum_assignment for valued members (Up = "UP")
	var specs []ast.Spec
	isStringEnum := false
	first := true

	for i := uint(0); i < bodyNode.NamedChildCount(); i++ {
		member := bodyNode.NamedChild(i)

		var mName string
		var mValue *sitter.Node

		switch member.Kind() {
		case "property_identifier":
			// Simple enum member: Red, Green, Blue
			mName = enumName + capitalize(member.Utf8Text(t.source))
		case "enum_assignment":
			// Valued enum member: Up = "UP"
			mNameNode := member.ChildByFieldName("name")
			mValue = member.ChildByFieldName("value")
			if mNameNode == nil {
				continue
			}
			mName = enumName + capitalize(mNameNode.Utf8Text(t.source))
		default:
			continue
		}

		if mValue != nil {
			valText := mValue.Utf8Text(t.source)
			if strings.HasPrefix(valText, "\"") || strings.HasPrefix(valText, "'") {
				isStringEnum = true
			}
		}

		spec := &ast.ValueSpec{
			Names: []*ast.Ident{ident(mName)},
		}

		if isStringEnum && mValue != nil {
			valText := mValue.Utf8Text(t.source)
			valText = strings.Trim(valText, "'\"")
			spec.Values = []ast.Expr{stringLit(valText)}
		} else if !isStringEnum && first {
			spec.Type = ident(enumName)
			spec.Values = []ast.Expr{ident("iota")}
			first = false
		}

		specs = append(specs, spec)
	}

	if isStringEnum {
		typeD = typeDecl(enumName, ident("string"))
	}

	constD := &ast.GenDecl{
		Tok:    token.CONST,
		Lparen: 1,
		Specs:  specs,
	}

	return []ast.Decl{typeD, constD}
}

func (t *Transformer) transformTypeAlias(node *sitter.Node) ast.Decl {
	nameNode := node.ChildByFieldName("name")
	typeNode := node.ChildByFieldName("value")

	if nameNode == nil || typeNode == nil {
		return nil
	}

	name := capitalize(nameNode.Utf8Text(t.source))
	typ := t.mapTypeNode(typeNode)
	if typ == nil {
		typ = ident("any")
	}

	return typeDecl(name, typ)
}

func (t *Transformer) transformDestructuring(pattern *sitter.Node, value *sitter.Node) []ast.Stmt {
	valExpr := t.transformExpr(value)
	var stmts []ast.Stmt

	switch pattern.Kind() {
	case "object_pattern":
		for i := uint(0); i < pattern.NamedChildCount(); i++ {
			child := pattern.NamedChild(i)
			switch child.Kind() {
			case "shorthand_property_identifier_pattern":
				name := child.Utf8Text(t.source)
				stmts = append(stmts, &ast.AssignStmt{
					Lhs: []ast.Expr{ident(name)},
					Tok: token.DEFINE,
					Rhs: []ast.Expr{selectorExpr(valExpr, capitalize(name))},
				})
			case "pair_pattern":
				keyNode := child.ChildByFieldName("key")
				valNode := child.ChildByFieldName("value")
				if keyNode != nil && valNode != nil {
					name := valNode.Utf8Text(t.source)
					key := keyNode.Utf8Text(t.source)
					stmts = append(stmts, &ast.AssignStmt{
						Lhs: []ast.Expr{ident(name)},
						Tok: token.DEFINE,
						Rhs: []ast.Expr{selectorExpr(valExpr, capitalize(key))},
					})
				}
			}
		}

	case "array_pattern":
		for i := uint(0); i < pattern.NamedChildCount(); i++ {
			child := pattern.NamedChild(i)
			name := child.Utf8Text(t.source)
			if name == "" {
				continue
			}
			stmts = append(stmts, &ast.AssignStmt{
				Lhs: []ast.Expr{ident(name)},
				Tok: token.DEFINE,
				Rhs: []ast.Expr{&ast.IndexExpr{
					X:     valExpr,
					Index: intLit(fmt.Sprintf("%d", i)),
				}},
			})
		}
	}

	return stmts
}
