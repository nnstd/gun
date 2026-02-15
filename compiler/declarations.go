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
					if assign, ok := stmt.(*ast.AssignStmt); ok && len(assign.Lhs) == 1 && len(assign.Rhs) == 1 {
						if id, ok := assign.Lhs[0].(*ast.Ident); ok {
							decls = append(decls, varDecl(id.Name, nil, assign.Rhs[0]))
						}
					}
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

		// Track package-level function variables (JS functions-as-objects pattern).
		// In JS, functions can have properties; in Go they can't, so we need to
		// skip member assignments on these vars.
		if valueNode != nil && len(t.localScopes) == 0 {
			switch valueNode.Kind() {
			case "arrow_function", "function", "function_expression":
				t.funcVarNames[name] = true
			}
		}

		// No type and no value → default to *jsvalue.JSValue
		if typ == nil && value == nil {
			typ = ptrType(selectorExpr(ident("jsvalue"), "JSValue"))
			t.addAliasedImport("github.com/nnstd/gun/runtime/jsvalue", "jsvalue")
		}

		// JS numbers are always float64. When a non-const variable is
		// initialized with an integer literal and has no type annotation,
		// explicitly set the type to float64 so it matches JS semantics
		// and is compatible with float64 return types.
		if !isConst && typ == nil && value != nil {
			if lit, ok := value.(*ast.BasicLit); ok && lit.Kind == token.INT {
				typ = ident("float64")
			}
		}

		// Register the variable in the current scope so property access
		// on it uses .Get() when appropriate.
		// Variables with explicit types, or initialized from non-JSValue
		// expressions (literals, ternaries, binary ops) are marked typed
		// so they don't get spurious JSValue coercion.
		typed := typeNode != nil || isNonJSValueInit(valueNode, t.source)
		t.addToCurrentScope(name, typed)

		// If const with simple literal value, use Go const
		if isConst && value != nil && isConstCompatible(value) && (typ == nil || isConstType(typ)) {
			decls = append(decls, constDecl(name, typ, value))
		} else {
			decls = append(decls, varDecl(name, typ, value))
		}
	}

	return decls
}

// isNonJSValueInit returns true when a tree-sitter value node clearly does not
// produce a *jsvalue.JSValue (e.g. literals, ternaries, binary/unary ops,
// calls to known string-returning methods like toLowerCase).
func isNonJSValueInit(node *sitter.Node, source []byte) bool {
	if node == nil {
		return false
	}
	switch node.Kind() {
	case "number", "string", "template_string", "true", "false",
		"ternary_expression", "binary_expression", "unary_expression":
		return true
	case "call_expression":
		fnNode := node.ChildByFieldName("function")
		if fnNode != nil && fnNode.Kind() == "member_expression" {
			propNode := fnNode.ChildByFieldName("property")
			if propNode != nil {
				switch propNode.Utf8Text(source) {
				case "toLowerCase", "toUpperCase", "trim", "trimStart", "trimEnd",
					"toString", "replace", "replaceAll", "join", "split":
					return true
				}
			}
		}
	}
	return false
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

	params, paramStmts := t.transformParams(paramsNode)
	var results *ast.FieldList
	if returnTypeNode != nil {
		retType := t.getTypeAnnotation(returnTypeNode)
		if retType != nil {
			results = fieldList(field("", retType))
		}
	}

	// Push a typed scope so isUntypedLocal/isLocalName work inside the body.
	paramInfo := extractParamInfo(paramsNode, t.source)
	t.pushTypedScope(paramInfo)
	defer t.popScope()

	var body *ast.BlockStmt
	if bodyNode != nil {
		body = t.transformBlock(bodyNode)
	} else {
		body = blockStmt()
	}

	if len(paramStmts) > 0 {
		body.List = append(paramStmts, body.List...)
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

	ensureTrailingReturn(body, results)

	return funcDecl(name, params, results, body)
}

func (t *Transformer) transformParams(node *sitter.Node) (*ast.FieldList, []ast.Stmt) {
	if node == nil {
		return fieldList(), nil
	}

	var fields []*ast.Field
	var destructureStmts []ast.Stmt
	for i := uint(0); i < node.NamedChildCount(); i++ {
		param := node.NamedChild(i)
		switch param.Kind() {
		case "required_parameter", "optional_parameter":
			nameNode := param.ChildByFieldName("pattern")
			typeNode := param.ChildByFieldName("type")
			valueNode := param.ChildByFieldName("value")

			var pType ast.Expr
			if typeNode != nil {
				mapped := t.getTypeAnnotation(typeNode)
				if mapped != nil {
					pType = mapped
				}
			}
			if pType == nil {
				pType = ptrType(selectorExpr(ident("jsvalue"), "JSValue"))
				t.addAliasedImport("github.com/nnstd/gun/runtime/jsvalue", "jsvalue")
			}

			// Optional params become pointer types
			if param.Kind() == "optional_parameter" {
				pType = ptrType(pType)
			}

			// Handle rest pattern in required_parameter (JS without type annotations)
			// tree-sitter parses `...args` as required_parameter > rest_pattern
			if nameNode != nil && nameNode.Kind() == "rest_pattern" {
				restName := "args"
				for j := uint(0); j < nameNode.NamedChildCount(); j++ {
					if child := nameNode.NamedChild(j); child.Kind() == "identifier" {
						restName = sanitizeIdent(child.Utf8Text(t.source))
						break
					}
				}
				var elemType ast.Expr = ptrType(selectorExpr(ident("jsvalue"), "JSValue"))
				t.addAliasedImport("github.com/nnstd/gun/runtime/jsvalue", "jsvalue")
				if typeNode != nil {
					if mapped := t.getTypeAnnotation(typeNode); mapped != nil {
						if at, ok := mapped.(*ast.ArrayType); ok {
							elemType = at.Elt
						} else {
							elemType = mapped
						}
					}
				}
				fields = append(fields, field(restName, &ast.Ellipsis{Elt: elemType}))
				continue
			}

			// Handle destructuring patterns in parameters
			if nameNode != nil && (nameNode.Kind() == "object_pattern" || nameNode.Kind() == "array_pattern") {
				syntheticName := fmt.Sprintf("_param%d", i)

				// Parameters with default values become variadic so callers
				// can omit them (JS allows calling with fewer args).
				if valueNode != nil {
					variadicName := fmt.Sprintf("_args%d", i)
					fields = append(fields, field(variadicName, &ast.Ellipsis{Elt: pType}))
					// var _param0 Type; if len(_args0) > 0 { _param0 = _args0[0] }
					destructureStmts = append(destructureStmts,
						&ast.DeclStmt{Decl: varDecl(syntheticName, pType, nil)},
						&ast.IfStmt{
							Cond: &ast.BinaryExpr{
								X:  callExpr(ident("len"), ident(variadicName)),
								Op: token.GTR,
								Y:  intLit("0"),
							},
							Body: blockStmt(
								&ast.AssignStmt{
									Lhs: []ast.Expr{ident(syntheticName)},
									Tok: token.ASSIGN,
									Rhs: []ast.Expr{&ast.IndexExpr{X: ident(variadicName), Index: intLit("0")}},
								},
							),
						},
					)
				} else {
					fields = append(fields, field(syntheticName, pType))
				}

				stmts := t.transformDestructuringFromExpr(nameNode, ident(syntheticName))
				destructureStmts = append(destructureStmts, stmts...)
				// Ensure the synthetic param is referenced even when all
				// destructured properties use defaults (avoids "declared and not used").
				if valueNode != nil {
					destructureStmts = append(destructureStmts,
						&ast.AssignStmt{
							Lhs: []ast.Expr{ident("_")},
							Tok: token.ASSIGN,
							Rhs: []ast.Expr{ident(syntheticName)},
						},
					)
				}
				continue
			}

			pName := "_"
			if nameNode != nil {
				pName = sanitizeIdent(nameNode.Utf8Text(t.source))
			}

			fields = append(fields, field(pName, pType))

		case "rest_parameter":
			nameNode := param.ChildByFieldName("pattern")
			typeNode := param.ChildByFieldName("type")

			pName := "args"
			if nameNode != nil {
				pName = sanitizeIdent(nameNode.Utf8Text(t.source))
			}

			var elemType ast.Expr
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
			if elemType == nil {
				elemType = ptrType(selectorExpr(ident("jsvalue"), "JSValue"))
				t.addAliasedImport("github.com/nnstd/gun/runtime/jsvalue", "jsvalue")
			}

			fields = append(fields, field(pName, &ast.Ellipsis{Elt: elemType}))
		}
	}

	return fieldList(fields...), destructureStmts
}

func (t *Transformer) transformBlock(node *sitter.Node) *ast.BlockStmt {
	if node == nil {
		return blockStmt()
	}

	var stmts []ast.Stmt
	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		// Handle variable declarations directly so destructuring (and multi-decl)
		// statements are not lost — transformStmt can only return one ast.Stmt.
		if child.Kind() == "lexical_declaration" || child.Kind() == "variable_declaration" {
			decls := t.transformVarDecl(child)
			for _, d := range decls {
				stmts = append(stmts, &ast.DeclStmt{Decl: d})
			}
			continue
		}
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
		params, _ := t.transformParams(paramsNode)

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
			var pType ast.Expr = t.jsValueType()
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
			params, _ := t.transformParams(paramsNode)
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
		typ = t.jsValueType()
	}

	return typeDecl(name, typ)
}

func (t *Transformer) transformDestructuring(pattern *sitter.Node, value *sitter.Node) []ast.Stmt {
	return t.transformDestructuringFromExpr(pattern, t.transformExpr(value))
}

func (t *Transformer) transformDestructuringFromExpr(pattern *sitter.Node, valExpr ast.Expr) []ast.Stmt {
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
			case "object_assignment_pattern":
				// { ambiguousIsNarrow = true } = options
				leftNode := child.ChildByFieldName("left")
				rightNode := child.ChildByFieldName("right")
				if leftNode != nil && rightNode != nil {
					name := leftNode.Utf8Text(t.source)
					goName := sanitizeIdent(name)
					defaultVal := t.transformExpr(rightNode)
					stmts = append(stmts, &ast.AssignStmt{
						Lhs: []ast.Expr{ident(goName)},
						Tok: token.DEFINE,
						Rhs: []ast.Expr{defaultVal},
					})
				}
			}
		}

	case "array_pattern":
		for i := uint(0); i < pattern.NamedChildCount(); i++ {
			child := pattern.NamedChild(i)

			// Rest element: const [first, ...rest] = arr → rest := arr[1:]
			if child.Kind() == "rest_pattern" {
				nameNode := child.ChildByFieldName("pattern")
				if nameNode == nil {
					// fallback: second named child is the identifier
					if child.NamedChildCount() > 0 {
						nameNode = child.NamedChild(child.NamedChildCount() - 1)
					}
				}
				if nameNode == nil {
					continue
				}
				name := nameNode.Utf8Text(t.source)
				stmts = append(stmts, &ast.AssignStmt{
					Lhs: []ast.Expr{ident(name)},
					Tok: token.DEFINE,
					Rhs: []ast.Expr{&ast.SliceExpr{
						X:   valExpr,
						Low: intLit(fmt.Sprintf("%d", i)),
					}},
				})
				continue
			}

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
