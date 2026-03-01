package compiler

import (
	"fmt"
	"go/ast"
	"go/token"
	"strings"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

// getTypeString converts a Go AST type expression to a string representation.
func getTypeString(typ ast.Expr) string {
	if typ == nil {
		return ""
	}
	switch t := typ.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + getTypeString(t.X)
	case *ast.ArrayType:
		if t.Len == nil {
			return "[]" + getTypeString(t.Elt)
		}
		return "[...]" + getTypeString(t.Elt)
	case *ast.MapType:
		return "map[" + getTypeString(t.Key) + "]" + getTypeString(t.Value)
	case *ast.SelectorExpr:
		return getTypeString(t.X) + "." + t.Sel.Name
	default:
		return ""
	}
}

// wrapInJSValue wraps a Go expression in the appropriate JSValue constructor
// based on the original tree-sitter node type.
func (t *Transformer) wrapInJSValue(node *sitter.Node, expr ast.Expr) ast.Expr {
	if node == nil || expr == nil {
		return expr
	}

	t.addAliasedImport("github.com/nnstd/gun/runtime/jsvalue", "jsvalue")

	switch node.Kind() {
	case "true", "false":
		// Boolean literals: wrap in jsvalue.NewBool()
		return callExpr(selectorExpr(ident("jsvalue"), "NewBool"), expr)
	case "number":
		// Number literals: wrap in jsvalue.NewNumber()
		return callExpr(selectorExpr(ident("jsvalue"), "NewNumber"), expr)
	case "string", "template_string":
		// String literals: wrap in jsvalue.NewString()
		return callExpr(selectorExpr(ident("jsvalue"), "NewString"), expr)
	default:
		// Other expressions: wrap in jsvalue.From()
		return callExpr(selectorExpr(ident("jsvalue"), "From"), expr)
	}
}

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
		// Also applies when value is nil literal (null/undefined).
		if typ == nil && (value == nil || isNilIdent(value)) {
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
		typed := typeNode != nil || t.isNonJSValueInit(valueNode)
		t.addToCurrentScope(name, typed)

		// Track the actual Go type of typed locals for proper boolean conversion.
		// For boolean literals, infer the type even if there's no explicit annotation.
		if typed && len(t.localScopes) > 0 {
			if typ != nil {
				t.typedLocalTypes[name] = getTypeString(typ)
			} else if value != nil {
				if isBoolExpr(value) {
					t.typedLocalTypes[name] = "bool"
				}
			}
		}

		// Track package-level variable types so property access on untyped
		// package vars uses .Get() instead of capitalized selectors.
		if len(t.localScopes) == 0 {
			t.pkgVarTyped[name] = typed
		}

		// Track typed locals whose elements are *jsvalue.JSValue (array literals
		// with JSValue elements) so subscript access is recognized as JSValue.
		if typed && valueNode != nil && valueNode.Kind() == "array" {
			if cl, ok := value.(*ast.CompositeLit); ok {
				if at, ok := cl.Type.(*ast.ArrayType); ok {
					if isJSValuePtrType(at.Elt) {
						t.jsvalueSliceLocals[name] = true
					}
				}
			}
		}

		// Track locals assigned from jsvalueSliceLocals (e.g. var toCheck = patterns)
		if valueNode != nil && valueNode.Kind() == "identifier" {
			rhsName := valueNode.Utf8Text(t.source)
			if t.jsvalueSliceLocals[rhsName] {
				t.jsvalueSliceLocals[name] = true
			}
		}

		// Track locals that hold *jsvalue.JSValue (not slices or maps)
		// so method calls on them are properly coerced.
		if !typed && valueNode != nil && t.nodeReturnsJSValue(valueNode) {
			t.jsvalueLocals[name] = true
		}

		// Track locals initialized from new Map() or new Set()
		if valueNode != nil && valueNode.Kind() == "new_expression" {
			ctorNode := valueNode.ChildByFieldName("constructor")
			if ctorNode != nil {
				switch ctorNode.Utf8Text(t.source) {
				case "Map":
					t.mapSetLocals[name] = "map"
				case "Set":
					t.mapSetLocals[name] = "set"
				}
			}
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

// isNonJSValueInit returns true when a tree-sitter value node clearly does not
// produce a *jsvalue.JSValue (e.g. literals, ternaries, binary/unary ops,
// calls to known string-returning methods like toLowerCase).
func (t *Transformer) isNonJSValueInit(node *sitter.Node) bool {
	if node == nil {
		return false
	}
	switch node.Kind() {
	case "number", "string", "template_string", "true", "false",
		"array":
		return true
	case "ternary_expression":
		// If either branch returns JSValue, the ternary is JSValue
		consNode := node.ChildByFieldName("consequence")
		altNode := node.ChildByFieldName("alternative")
		if (consNode != nil && t.nodeReturnsJSValue(consNode)) ||
			(altNode != nil && t.nodeReturnsJSValue(altNode)) {
			return false
		}
		return true
	case "new_expression":
		// All new expressions produce *jsvalue.JSValue (classes are JSValue constructors)
		return false
	case "new_expression_UNUSED":
		ctorNode := node.ChildByFieldName("constructor")
		if ctorNode != nil {
			name := ctorNode.Utf8Text(t.source)
			if name == "Map" || name == "Set" {
				return false
			}
		}
		return true
	case "binary_expression":
		// When either operand returns JSValue, the binary expression now
		// produces a JSValue result (via jsvalue.Add, jsvalue.Eq, jsvalue.And, etc.)
		left := node.ChildByFieldName("left")
		right := node.ChildByFieldName("right")
		if (left != nil && t.nodeReturnsJSValue(left)) ||
			(right != nil && t.nodeReturnsJSValue(right)) {
			return false
		}
		return true
	case "unary_expression":
		// Unary operations on JSValue (!, -, ~, typeof) now return JSValue
		argNode := node.ChildByFieldName("argument")
		if argNode != nil && t.nodeReturnsJSValue(argNode) {
			return false
		}
		return true
	case "call_expression":
		fnNode := node.ChildByFieldName("function")
		if fnNode != nil && fnNode.Kind() == "member_expression" {
			objNode := fnNode.ChildByFieldName("object")
			propNode := fnNode.ChildByFieldName("property")
			// Math.xxx() calls return native float64.
			if objNode != nil && objNode.Kind() == "identifier" && objNode.Utf8Text(t.source) == "Math" {
				return true
			}
			if propNode != nil {
				prop := propNode.Utf8Text(t.source)
				// String methods: on typed receivers (native strings) they return native Go types;
				// on JSValue receivers (untyped parameters or JSValue locals) they return *jsvalue.JSValue.
				switch prop {
				case "toLowerCase", "toUpperCase", "trim", "trimStart", "trimEnd",
					"toString", "replace", "replaceAll", "join",
					"codePointAt", "charCodeAt", "indexOf", "split", "charAt":
					if objNode != nil {
						// If receiver returns JSValue, the method call also returns JSValue
						return !t.nodeReturnsJSValue(objNode)
					}
					return true
				case "match", "exec":
					// match/exec always return native Go types ([]string) regardless of receiver,
					// because the transpiler transforms them to FindStringSubmatch which returns []string
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

	// Push a typed scope so isUntypedLocal/isLocalName work inside the body
	// AND during parameter destructuring (which may register typed locals).
	paramInfo := extractParamInfo(paramsNode, t.source)
	t.pushTypedScope(paramInfo)
	defer t.popScope()

	params, paramStmts := t.transformParams(paramsNode)
	var results *ast.FieldList
	// In the all-JSValue world, explicit return type annotations are ignored —
	// all functions return *jsvalue.JSValue (handled below in the hasReturnValue check).
	_ = returnTypeNode

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
		// Always use *jsvalue.JSValue for functions with return values
		// This ensures consistency across all transpiled functions
		if hasReturnValue(body) {
			results = fieldList(field("", ptrType(selectorExpr(ident("jsvalue"), "JSValue"))))
			t.addAliasedImport("github.com/nnstd/gun/runtime/jsvalue", "jsvalue")
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
			_ = param.ChildByFieldName("type") // type annotations ignored in all-JSValue mode
			valueNode := param.ChildByFieldName("value")

			// All-JSValue: all params are *jsvalue.JSValue
			var pType ast.Expr = ptrType(selectorExpr(ident("jsvalue"), "JSValue"))
			t.addAliasedImport("github.com/nnstd/gun/runtime/jsvalue", "jsvalue")

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
				// Track rest params as JSValue slice locals so collection methods
				// wrap them with jsvalue.NewArray(args...) before calling
				t.jsvalueSliceLocals[restName] = true
				// All-JSValue: rest params are always ...*jsvalue.JSValue
				var elemType ast.Expr = ptrType(selectorExpr(ident("jsvalue"), "JSValue"))
				t.addAliasedImport("github.com/nnstd/gun/runtime/jsvalue", "jsvalue")
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

			pName := "args"
			if nameNode != nil {
				pName = sanitizeIdent(nameNode.Utf8Text(t.source))
			}

			// All-JSValue: rest params are always ...*jsvalue.JSValue
			var elemType ast.Expr = ptrType(selectorExpr(ident("jsvalue"), "JSValue"))
			t.addAliasedImport("github.com/nnstd/gun/runtime/jsvalue", "jsvalue")
			// Track rest params as JSValue slice locals
			t.jsvalueSliceLocals[pName] = true

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

	// First pass: pre-scan function declarations for hoisting.
	// Extract signatures and register names in scope/funcParamCounts,
	// but do NOT transform bodies yet — surrounding variables and sibling
	// hoisted functions must be registered first so that cross-references
	// between hoisted functions (and references to outer variables like argv)
	// resolve correctly when bodies are transformed in the second pass.
	type hoistedInfo struct {
		name string
		typ  *ast.FuncType
	}
	var hoisted []hoistedInfo
	hoistedSet := map[string]bool{}
	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		if child.Kind() == "function_declaration" {
			nameNode := child.ChildByFieldName("name")
			if nameNode == nil {
				continue
			}
			name := nameNode.Utf8Text(t.source)

			// Extract Go function type (params + return type) without transforming body.
			paramsNode := child.ChildByFieldName("parameters")
			returnTypeNode := child.ChildByFieldName("return_type")
			paramInfo := extractParamInfo(paramsNode, t.source)
			t.pushTypedScope(paramInfo)
			params, _ := t.transformParams(paramsNode)
			var results *ast.FieldList
			if returnTypeNode != nil {
				retType := t.getTypeAnnotation(returnTypeNode)
				if retType != nil {
					results = fieldList(field("", retType))
				}
			}
			// If no explicit return type but the body has return statements
			// with values, default to *jsvalue.JSValue.
			bodyNode := child.ChildByFieldName("body")
			if results == nil && bodyNode != nil && nodeHasReturnValue(bodyNode) {
				results = fieldList(field("", jsValuePtrType()))
			}
			t.popScope()

			funcType := &ast.FuncType{Params: params, Results: results}
			hoisted = append(hoisted, hoistedInfo{name: name, typ: funcType})
			hoistedSet[name] = true

			// All hoisted functions are JSValue in the all-JSValue architecture
			t.addToCurrentScope(name, false)
			if params != nil {
				t.funcParamCounts[name] = len(params.List)
			}
		}
	}

	// Emit forward declarations for hoisted functions as *jsvalue.JSValue.
	for _, h := range hoisted {
		t.addAliasedImport("github.com/nnstd/gun/runtime/jsvalue", "jsvalue")
		stmts = append(stmts, &ast.DeclStmt{
			Decl: &ast.GenDecl{
				Tok: token.VAR,
				Specs: []ast.Spec{
					&ast.ValueSpec{
						Names: []*ast.Ident{ident(h.name)},
						Type:  jsValuePtrType(),
					},
				},
			},
		})
	}

	// Second pass: process all statements in order.
	// Hoisted function bodies are transformed HERE so that all surrounding
	// variables and sibling hoisted functions are available in scope.
	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		if child.Kind() == "function_declaration" {
			nameNode := child.ChildByFieldName("name")
			if nameNode != nil {
				name := nameNode.Utf8Text(t.source)
				if hoistedSet[name] {
					if d := t.transformFuncDecl(child, false); d != nil {
						paramNames := extractParamNames(child.ChildByFieldName("parameters"), t.source)
						fnLit := &ast.FuncLit{Type: d.Type, Body: d.Body}
						jsVal := t.wrapFuncLitAsJSValue(fnLit, paramNames)
						stmts = append(stmts, assignStmt(
							[]ast.Expr{ident(d.Name.Name)},
							[]ast.Expr{jsVal},
						))
					}
				}
			}
			continue
		}
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

		// All-JSValue: interface methods return *jsvalue.JSValue
		var results *ast.FieldList
		if returnNode != nil {
			t.addAliasedImport("github.com/nnstd/gun/runtime/jsvalue", "jsvalue")
			results = fieldList(field("", jsValuePtrType()))
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
			// All-JSValue: interface methods return *jsvalue.JSValue
			var results *ast.FieldList
			if returnNode != nil {
				t.addAliasedImport("github.com/nnstd/gun/runtime/jsvalue", "jsvalue")
				results = fieldList(field("", jsValuePtrType()))
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

// preRegisterDestructureNames registers destructured variable names in scope
// BEFORE the body is transformed, so they're recognized as JSValue locals.
func (t *Transformer) preRegisterDestructureNames(pattern *sitter.Node) {
	if pattern == nil {
		return
	}
	switch pattern.Kind() {
	case "object_pattern":
		for i := uint(0); i < pattern.NamedChildCount(); i++ {
			child := pattern.NamedChild(i)
			switch child.Kind() {
			case "shorthand_property_identifier_pattern":
				t.addToCurrentScope(child.Utf8Text(t.source), false)
				t.jsvalueLocals[child.Utf8Text(t.source)] = true
			case "pair_pattern":
				valNode := child.ChildByFieldName("value")
				if valNode != nil {
					t.addToCurrentScope(valNode.Utf8Text(t.source), false)
					t.jsvalueLocals[valNode.Utf8Text(t.source)] = true
				}
			}
		}
	case "array_pattern":
		for i := uint(0); i < pattern.NamedChildCount(); i++ {
			child := pattern.NamedChild(i)
			if child.Kind() == "identifier" {
				t.addToCurrentScope(child.Utf8Text(t.source), false)
				t.jsvalueLocals[child.Utf8Text(t.source)] = true
			}
		}
	}
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
					Rhs: []ast.Expr{callExpr(selectorExpr(valExpr, "Get"), stringLit(name))},
				})
				// All destructured fields are JSValue (properties from objects)
				t.addToCurrentScope(name, false)
				t.jsvalueLocals[name] = true
			case "pair_pattern":
				keyNode := child.ChildByFieldName("key")
				valNode := child.ChildByFieldName("value")
				if keyNode != nil && valNode != nil {
					name := valNode.Utf8Text(t.source)
					key := keyNode.Utf8Text(t.source)
					stmts = append(stmts, &ast.AssignStmt{
						Lhs: []ast.Expr{ident(name)},
						Tok: token.DEFINE,
						Rhs: []ast.Expr{callExpr(selectorExpr(valExpr, "Get"), stringLit(key))},
					})
					// All destructured fields are JSValue (properties from objects)
					t.addToCurrentScope(name, false)
					t.jsvalueLocals[name] = true
				}
			case "object_assignment_pattern":
				// { ambiguousIsNarrow = true } = options
				leftNode := child.ChildByFieldName("left")
				rightNode := child.ChildByFieldName("right")
				if leftNode != nil && rightNode != nil {
					name := leftNode.Utf8Text(t.source)
					goName := sanitizeIdent(name)
					defaultVal := t.transformExpr(rightNode)
					// Wrap default value in JSValue since destructured fields are JSValue
					defaultVal = t.wrapInJSValue(rightNode, defaultVal)
					stmts = append(stmts, &ast.AssignStmt{
						Lhs: []ast.Expr{ident(goName)},
						Tok: token.DEFINE,
						Rhs: []ast.Expr{defaultVal},
					})
					// All destructured fields are JSValue (properties from objects)
					t.addToCurrentScope(goName, false)
					t.jsvalueLocals[goName] = true
				}
			}
		}

	case "array_pattern":
		// Check if the source value is a *jsvalue.JSValue — if so, use
		// .Index() instead of Go's [] operator.
		useJSIndex := false
		if id, ok := valExpr.(*ast.Ident); ok {
			useJSIndex = t.isUntypedLocal(id.Name) || strings.HasPrefix(id.Name, "_param") || strings.HasPrefix(id.Name, "_item")
		}

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
				var rhs ast.Expr
				if useJSIndex {
					rhs = callExpr(selectorExpr(valExpr, "Slice"), intLit(fmt.Sprintf("%d", i)))
				} else {
					rhs = &ast.SliceExpr{
						X:   valExpr,
						Low: intLit(fmt.Sprintf("%d", i)),
					}
				}
				stmts = append(stmts, &ast.AssignStmt{
					Lhs: []ast.Expr{ident(name)},
					Tok: token.DEFINE,
					Rhs: []ast.Expr{rhs},
				})
				// All destructured array elements are JSValue
				t.addToCurrentScope(name, false)
				t.jsvalueLocals[name] = true
				continue
			}

			name := child.Utf8Text(t.source)
			if name == "" {
				continue
			}
			var rhs ast.Expr
			if useJSIndex {
				rhs = callExpr(selectorExpr(valExpr, "Index"), intLit(fmt.Sprintf("%d", i)))
			} else {
				rhs = &ast.IndexExpr{
					X:     valExpr,
					Index: intLit(fmt.Sprintf("%d", i)),
				}
			}
			stmts = append(stmts, &ast.AssignStmt{
				Lhs: []ast.Expr{ident(name)},
				Tok: token.DEFINE,
				Rhs: []ast.Expr{rhs},
			})
			// All destructured array elements are JSValue
			t.addToCurrentScope(name, false)
			t.jsvalueLocals[name] = true
		}
	}

	return stmts
}
