package compiler

import (
	"fmt"
	"go/ast"
	"go/token"
	"strings"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

func (t *Transformer) transformExpr(node *sitter.Node) ast.Expr {
	if node == nil {
		return nil
	}

	switch node.Kind() {
	case "identifier":
		return t.resolveIdentifier(node.Utf8Text(t.source))

	case "number":
		text := node.Utf8Text(t.source)
		return basicLit(numberTokenKind(text), text)

	case "string", "string_fragment":
		text := node.Utf8Text(t.source)
		// Strip only the outer quote delimiters (not Trim, which eats multiple)
		if len(text) >= 2 && (text[0] == '\'' || text[0] == '"') {
			text = text[1 : len(text)-1]
		}
		// Escape unescaped double quotes for Go string literal
		text = strings.ReplaceAll(text, `\"`, "\x00ESCAPED_DQ\x00")
		text = strings.ReplaceAll(text, `"`, `\"`)
		text = strings.ReplaceAll(text, "\x00ESCAPED_DQ\x00", `\"`)
		return basicLit(token.STRING, `"`+text+`"`)

	case "template_string":
		return t.transformTemplateString(node)

	case "true":
		return ident("true")

	case "false":
		return ident("false")

	case "null", "undefined":
		return ident("nil")

	case "this":
		return ident("this")

	case "binary_expression":
		return t.transformBinaryExpr(node)

	case "unary_expression":
		return t.transformUnaryExpr(node)

	case "update_expression":
		return t.transformUpdateExpr(node)

	case "assignment_expression":
		return t.transformAssignmentExpr(node)

	case "augmented_assignment_expression":
		return t.transformAugmentedAssignment(node)

	case "call_expression":
		return t.transformCallExpr(node)

	case "member_expression":
		return t.transformMemberExpr(node)

	case "subscript_expression":
		return t.transformSubscriptExpr(node)

	case "array":
		return t.transformArrayLiteral(node)

	case "object":
		return t.transformObjectLiteral(node)

	case "parenthesized_expression":
		if node.NamedChildCount() > 0 {
			return t.transformExpr(node.NamedChild(0))
		}
		return nil

	case "ternary_expression":
		return t.transformTernary(node)

	case "arrow_function":
		return t.transformArrowFunc(node)

	case "function_expression", "function":
		return t.transformFuncExpr(node)

	case "new_expression":
		return t.transformNewExpr(node)

	case "as_expression", "type_assertion":
		exprNode := node.ChildByFieldName("expression")
		if exprNode == nil && node.NamedChildCount() > 0 {
			exprNode = node.NamedChild(0)
		}
		if exprNode != nil {
			return t.transformExpr(exprNode)
		}
		return nil

	case "non_null_expression":
		if node.NamedChildCount() > 0 {
			return t.transformExpr(node.NamedChild(0))
		}
		return nil

	case "await_expression":
		if node.NamedChildCount() > 0 {
			return t.transformExpr(node.NamedChild(0))
		}
		return nil

	case "spread_element":
		if node.NamedChildCount() > 0 {
			return t.transformExpr(node.NamedChild(0))
		}
		return nil

	case "type_identifier", "predefined_type", "property_identifier", "shorthand_property_identifier":
		return ident(node.Utf8Text(t.source))

	case "regex":
		t.addImport("regexp")
		pattern := node.Utf8Text(t.source)
		// Strip JS regex delimiters: /pattern/flags → pattern
		if len(pattern) >= 2 && pattern[0] == '/' {
			end := strings.LastIndex(pattern, "/")
			if end > 0 {
				pattern = pattern[1:end]
			}
		}
		// Use raw string literal (backtick) so backslashes are literal
		return callExpr(
			selectorExpr(ident("regexp"), "MustCompile"),
			basicLit(token.STRING, "`"+pattern+"`"),
		)

	case "conditional_type", "intersection_type", "union_type":
		return t.mapTypeNode(node)

	default:
		text := node.Utf8Text(t.source)
		if text != "" && isSimpleIdent(text) {
			return ident(text)
		}
		return ident("nil")
	}
}

func (t *Transformer) transformBinaryExpr(node *sitter.Node) ast.Expr {
	leftNode := node.ChildByFieldName("left")
	rightNode := node.ChildByFieldName("right")
	opNode := node.ChildByFieldName("operator")

	left := t.transformExpr(leftNode)
	right := t.transformExpr(rightNode)

	if left == nil || right == nil {
		return ident("nil")
	}

	opText := ""
	if opNode != nil {
		opText = opNode.Utf8Text(t.source)
	}

	if opText == "??" {
		return left
	}
	if opText == "instanceof" {
		return &ast.BinaryExpr{X: left, Op: token.NEQ, Y: ident("nil")}
	}

	return &ast.BinaryExpr{X: left, Op: mapBinaryOp(opText), Y: right}
}

func (t *Transformer) transformUnaryExpr(node *sitter.Node) ast.Expr {
	opNode := node.ChildByFieldName("operator")
	argNode := node.ChildByFieldName("argument")

	arg := t.transformExpr(argNode)
	if arg == nil {
		return ident("nil")
	}

	opText := ""
	if opNode != nil {
		opText = opNode.Utf8Text(t.source)
	}

	switch opText {
	case "!":
		return &ast.UnaryExpr{Op: token.NOT, X: arg}
	case "-":
		return &ast.UnaryExpr{Op: token.SUB, X: arg}
	case "+":
		return arg
	case "~":
		return &ast.UnaryExpr{Op: token.XOR, X: arg}
	case "typeof":
		t.addImport("fmt")
		return callExpr(selectorExpr(ident("fmt"), "Sprintf"), stringLit("%T"), arg)
	case "void":
		return ident("nil")
	default:
		return arg
	}
}

func (t *Transformer) transformUpdateExpr(node *sitter.Node) ast.Expr {
	argNode := node.ChildByFieldName("argument")
	opNode := node.ChildByFieldName("operator")

	arg := t.transformExpr(argNode)
	if arg == nil {
		return ident("nil")
	}

	opText := ""
	if opNode != nil {
		opText = opNode.Utf8Text(t.source)
	}

	switch opText {
	case "++":
		return &ast.BinaryExpr{X: arg, Op: token.ADD, Y: intLit("1")}
	case "--":
		return &ast.BinaryExpr{X: arg, Op: token.SUB, Y: intLit("1")}
	default:
		return arg
	}
}

func (t *Transformer) transformAssignmentExpr(node *sitter.Node) ast.Expr {
	left := t.transformExpr(node.ChildByFieldName("left"))
	right := t.transformExpr(node.ChildByFieldName("right"))
	if left == nil || right == nil {
		return ident("nil")
	}
	// In JS, assignment is an expression that returns the assigned value.
	// In Go, assignment is a statement. Wrap in an IIFE to preserve semantics.
	return &ast.CallExpr{
		Fun: &ast.FuncLit{
			Type: &ast.FuncType{
				Params:  fieldList(),
				Results: fieldList(field("", ident("any"))),
			},
			Body: blockStmt(
				&ast.AssignStmt{Lhs: []ast.Expr{left}, Tok: token.ASSIGN, Rhs: []ast.Expr{right}},
				returnStmt(right),
			),
		},
	}
}

func (t *Transformer) transformAugmentedAssignment(node *sitter.Node) ast.Expr {
	left := t.transformExpr(node.ChildByFieldName("left"))
	right := t.transformExpr(node.ChildByFieldName("right"))
	if left == nil || right == nil {
		return ident("nil")
	}
	opText := ""
	if opNode := node.ChildByFieldName("operator"); opNode != nil {
		opText = opNode.Utf8Text(t.source)
	}
	return &ast.BinaryExpr{X: left, Op: mapAugmentedOp(opText), Y: right}
}

func (t *Transformer) transformCallExpr(node *sitter.Node) ast.Expr {
	fnNode := node.ChildByFieldName("function")
	argsNode := node.ChildByFieldName("arguments")

	if fnNode == nil {
		return ident("nil")
	}

	// Check for builtin call patterns on member expressions
	if fnNode.Kind() == "member_expression" {
		objNode := fnNode.ChildByFieldName("object")
		propNode := fnNode.ChildByFieldName("property")

		if objNode != nil && propNode != nil {
			objText := objNode.Utf8Text(t.source)
			prop := propNode.Utf8Text(t.source)

			// Module-registered call transformers (e.g. hono route methods)
			// Check before transformArgs to avoid eagerly transforming callback args
			// that the module transformer will handle itself.
			if modType, ok := t.varTypes[objText]; ok {
				if fn, ok := moduleCallTransformers[modType]; ok {
					if r := fn(t, objNode, prop, argsNode); r != nil {
						return r
					}
				}
			}

			args := t.transformArgs(argsNode)

			// Known global objects (console, Math, JSON, Object)
			if r := transformBuiltinCall(objText, prop, args, t.addImport); r != nil {
				return r
			}

			// Method transforms on arbitrary receivers (string/collection methods)
			// Skip if the object is a namespace import (it's a package, not a value)
			if _, isNsImport := t.importedNames[objText]; !isNsImport {
				obj := t.transformExpr(objNode)
				if r := transformBuiltinMethod(obj, prop, args, t.addImport); r != nil {
					return r
				}
			}
		}
	}

	fun := t.transformExpr(fnNode)
	args := t.transformArgs(argsNode)
	return callExpr(fun, args...)
}

func (t *Transformer) transformArgs(node *sitter.Node) []ast.Expr {
	if node == nil {
		return nil
	}
	var args []ast.Expr
	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		if child.Kind() == "spread_element" {
			if child.NamedChildCount() > 0 {
				if inner := t.transformExpr(child.NamedChild(0)); inner != nil {
					args = append(args, inner)
				}
			}
		} else {
			if e := t.transformExpr(child); e != nil {
				args = append(args, e)
			}
		}
	}
	return args
}

func (t *Transformer) transformMemberExpr(node *sitter.Node) ast.Expr {
	objNode := node.ChildByFieldName("object")
	propNode := node.ChildByFieldName("property")

	if objNode == nil || propNode == nil {
		return ident("nil")
	}

	prop := sanitizeIdent(propNode.Utf8Text(t.source))

	// Same-package namespace import: templates.foo → Foo (direct symbol reference)
	if objNode.Kind() == "identifier" {
		name := objNode.Utf8Text(t.source)
		if imp, ok := t.importedNames[name]; ok && imp.goSymbol == "" && imp.goPkgName == "" {
			return ident(capitalize(prop))
		}
	}

	// process.X → Go equivalents
	if objNode.Kind() == "identifier" && objNode.Utf8Text(t.source) == "process" {
		if r := transformProcessMember(prop, t.addImport); r != nil {
			return r
		}
	}

	// process.env.X → os.Getenv("X")
	if objNode.Kind() == "member_expression" {
		innerObj := objNode.ChildByFieldName("object")
		innerProp := objNode.ChildByFieldName("property")
		if innerObj != nil && innerProp != nil &&
			innerObj.Utf8Text(t.source) == "process" &&
			innerProp.Utf8Text(t.source) == "env" {
			t.addImport("os")
			return callExpr(selectorExpr(ident("os"), "Getenv"), stringLit(prop))
		}
	}

	obj := t.transformExpr(objNode)

	if prop == "length" {
		return callExpr(ident("len"), obj)
	}

	return selectorExpr(obj, capitalize(prop))
}

func (t *Transformer) transformSubscriptExpr(node *sitter.Node) ast.Expr {
	obj := t.transformExpr(node.ChildByFieldName("object"))
	index := t.transformExpr(node.ChildByFieldName("index"))
	if obj == nil {
		return ident("nil")
	}
	if index == nil {
		return obj
	}
	return &ast.IndexExpr{X: obj, Index: index}
}

func (t *Transformer) transformArrayLiteral(node *sitter.Node) ast.Expr {
	var elts []ast.Expr
	for i := uint(0); i < node.NamedChildCount(); i++ {
		if e := t.transformExpr(node.NamedChild(i)); e != nil {
			elts = append(elts, e)
		}
	}

	var elemType ast.Expr = t.jsValueType()
	hasFloat, hasInt, hasString, hasBool := false, false, false, false

	for _, e := range elts {
		switch lit := e.(type) {
		case *ast.BasicLit:
			switch lit.Kind {
			case token.INT:
				hasInt = true
			case token.FLOAT:
				hasFloat = true
			case token.STRING:
				hasString = true
			}
		case *ast.Ident:
			if lit.Name == "true" || lit.Name == "false" {
				hasBool = true
			}
		}
	}

	switch {
	case hasFloat || (hasInt && !hasString && !hasBool):
		elemType = ident("float64")
		for i, e := range elts {
			if lit, ok := e.(*ast.BasicLit); ok && lit.Kind == token.INT {
				elts[i] = basicLit(token.FLOAT, lit.Value)
			}
		}
	case hasString:
		elemType = ident("string")
	case hasBool:
		elemType = ident("bool")
	}

	return compositeLit(sliceType(elemType), elts...)
}

func (t *Transformer) transformObjectLiteral(node *sitter.Node) ast.Expr {
	var elts []ast.Expr
	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		switch child.Kind() {
		case "pair":
			keyNode := child.ChildByFieldName("key")
			valNode := child.ChildByFieldName("value")
			if keyNode != nil && valNode != nil {
				key := keyNode.Utf8Text(t.source)
				if val := t.transformExpr(valNode); val != nil {
					elts = append(elts, keyValue(stringLit(key), val))
				}
			}
		case "shorthand_property_identifier":
			name := child.Utf8Text(t.source)
			elts = append(elts, keyValue(stringLit(name), t.resolveIdentifier(name)))
		}
	}
	return compositeLit(mapType(ident("string"), t.jsValueType()), elts...)
}

func (t *Transformer) transformTemplateString(node *sitter.Node) ast.Expr {
	t.addImport("fmt")

	var formatParts []string
	var args []ast.Expr

	for i := uint(0); i < node.ChildCount(); i++ {
		child := node.Child(i)
		switch child.Kind() {
		case "string_fragment", "template_chars":
			formatParts = append(formatParts, child.Utf8Text(t.source))
		case "template_substitution":
			formatParts = append(formatParts, "%v")
			if child.NamedChildCount() > 0 {
				if expr := t.transformExpr(child.NamedChild(0)); expr != nil {
					args = append(args, expr)
				}
			}
		case "`":
			// skip backticks
		default:
			if child.IsNamed() {
				formatParts = append(formatParts, "%v")
				if expr := t.transformExpr(child); expr != nil {
					args = append(args, expr)
				}
			}
		}
	}

	format := strings.Join(formatParts, "")
	// Escape double quotes so the format string is a valid Go string literal
	format = strings.ReplaceAll(format, `"`, `\"`)
	allArgs := append([]ast.Expr{stringLit(format)}, args...)
	return callExpr(selectorExpr(ident("fmt"), "Sprintf"), allArgs...)
}

func (t *Transformer) transformTernary(node *sitter.Node) ast.Expr {
	cond := t.transformExpr(node.ChildByFieldName("condition"))
	cons := t.transformExpr(node.ChildByFieldName("consequence"))
	alt := t.transformExpr(node.ChildByFieldName("alternative"))

	if cond == nil || cons == nil || alt == nil {
		return ident("nil")
	}

	return &ast.CallExpr{
		Fun: &ast.FuncLit{
			Type: &ast.FuncType{
				Params:  fieldList(),
				Results: fieldList(field("", ident("any"))),
			},
			Body: blockStmt(
				&ast.IfStmt{
					Cond: cond,
					Body: blockStmt(returnStmt(cons)),
					Else: blockStmt(returnStmt(alt)),
				},
			),
		},
	}
}

func (t *Transformer) transformArrowFunc(node *sitter.Node) ast.Expr {
	paramsNode := node.ChildByFieldName("parameters")
	bodyNode := node.ChildByFieldName("body")
	returnTypeNode := node.ChildByFieldName("return_type")
	paramNode := node.ChildByFieldName("parameter")

	var params *ast.FieldList
	var paramStmts []ast.Stmt
	if paramsNode != nil {
		params, paramStmts = t.transformParams(paramsNode)
	} else if paramNode != nil {
		params = fieldList(field(sanitizeIdent(paramNode.Utf8Text(t.source)), ptrType(selectorExpr(ident("jsvalue"), "JSValue"))))
		t.addAliasedImport("github.com/nnstd/gun/runtime/jsvalue", "jsvalue")
	} else {
		params = fieldList()
	}

	var results *ast.FieldList
	if returnTypeNode != nil {
		if retType := t.getTypeAnnotation(returnTypeNode); retType != nil {
			results = fieldList(field("", retType))
		}
	}

	var body *ast.BlockStmt
	if bodyNode != nil {
		if bodyNode.Kind() == "statement_block" {
			body = t.transformBlock(bodyNode)
		} else {
			if expr := t.transformExpr(bodyNode); expr != nil {
				body = blockStmt(returnStmt(expr))
			} else {
				body = blockStmt()
			}
		}
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

	return &ast.FuncLit{
		Type: &ast.FuncType{Params: params, Results: results},
		Body: body,
	}
}

func (t *Transformer) transformFuncExpr(node *sitter.Node) ast.Expr {
	params, paramStmts := t.transformParams(node.ChildByFieldName("parameters"))

	var results *ast.FieldList
	if rtn := node.ChildByFieldName("return_type"); rtn != nil {
		if retType := t.getTypeAnnotation(rtn); retType != nil {
			results = fieldList(field("", retType))
		}
	}

	var body *ast.BlockStmt
	if bn := node.ChildByFieldName("body"); bn != nil {
		body = t.transformBlock(bn)
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

	return &ast.FuncLit{
		Type: &ast.FuncType{Params: params, Results: results},
		Body: body,
	}
}

func (t *Transformer) transformNewExpr(node *sitter.Node) ast.Expr {
	ctorNode := node.ChildByFieldName("constructor")
	argsNode := node.ChildByFieldName("arguments")

	if ctorNode == nil {
		return ident("nil")
	}

	name := ctorNode.Utf8Text(t.source)
	args := t.transformArgs(argsNode)

	// Try builtin new expressions first
	if r := transformBuiltinNew(name, args, t); r != nil {
		return r
	}

	// Default: new Foo(args) → NewFoo(args) or &Foo{}
	if len(args) > 0 {
		return callExpr(ident(fmt.Sprintf("New%s", capitalize(name))), args...)
	}
	return addrOf(compositeLit(ident(name)))
}
