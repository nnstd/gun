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
		name := node.Utf8Text(t.source)
		return t.mapIdentifier(name)

	case "number":
		text := node.Utf8Text(t.source)
		return basicLit(numberTokenKind(text), text)

	case "string", "string_fragment":
		text := node.Utf8Text(t.source)
		// Remove TS quotes and use Go quotes
		text = strings.Trim(text, "'\"")
		return stringLit(text)

	case "template_string":
		return t.transformTemplateString(node)

	case "true":
		return ident("true")

	case "false":
		return ident("false")

	case "null", "undefined":
		return ident("nil")

	case "this":
		// Will be replaced with receiver name in class context
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
		// x as T → x.(T) or just x for simplicity
		exprNode := node.ChildByFieldName("expression")
		if exprNode == nil && node.NamedChildCount() > 0 {
			exprNode = node.NamedChild(0)
		}
		if exprNode != nil {
			return t.transformExpr(exprNode)
		}
		return nil

	case "non_null_expression":
		// x! → just x
		if node.NamedChildCount() > 0 {
			return t.transformExpr(node.NamedChild(0))
		}
		return nil

	case "await_expression":
		// Strip await, use inner expression
		if node.NamedChildCount() > 0 {
			return t.transformExpr(node.NamedChild(0))
		}
		return nil

	case "spread_element":
		if node.NamedChildCount() > 0 {
			inner := t.transformExpr(node.NamedChild(0))
			// Return as-is; the caller (e.g. call args) handles the ellipsis
			return inner
		}
		return nil

	case "type_identifier", "predefined_type":
		return ident(node.Utf8Text(t.source))

	case "property_identifier":
		return ident(node.Utf8Text(t.source))

	case "shorthand_property_identifier":
		return ident(node.Utf8Text(t.source))

	case "regex":
		t.addImport("regexp")
		text := node.Utf8Text(t.source)
		return callExpr(
			selectorExpr(ident("regexp"), "MustCompile"),
			stringLit(text),
		)

	case "conditional_type", "intersection_type", "union_type":
		return t.mapTypeNode(node)

	default:
		// Last resort: use the raw text as an identifier
		text := node.Utf8Text(t.source)
		if text != "" && isSimpleIdent(text) {
			return ident(text)
		}
		return ident("nil")
	}
}

func (t *Transformer) mapIdentifier(name string) ast.Expr {
	switch name {
	case "undefined":
		return ident("nil")
	case "null":
		return ident("nil")
	case "console":
		return ident("fmt")
	case "Math":
		t.addImport("math")
		return ident("math")
	case "JSON":
		t.addImport("encoding/json")
		return ident("json")
	case "Error":
		t.addImport("errors")
		return ident("errors")
	default:
		return ident(name)
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

	op := mapBinaryOp(opText)

	// Special case: ?? (nullish coalescing)
	if opText == "??" {
		// Convert to: func() T { if left != nil { return left }; return right }()
		// Simplified: just use left for now
		return left
	}

	// Special case: instanceof
	if opText == "instanceof" {
		// No direct Go equivalent; return a comment-like expression
		return &ast.BinaryExpr{X: left, Op: token.NEQ, Y: ident("nil")}
	}

	return &ast.BinaryExpr{X: left, Op: op, Y: right}
}

func mapBinaryOp(op string) token.Token {
	switch op {
	case "+":
		return token.ADD
	case "-":
		return token.SUB
	case "*":
		return token.MUL
	case "/":
		return token.QUO
	case "%":
		return token.REM
	case "==", "===":
		return token.EQL
	case "!=", "!==":
		return token.NEQ
	case "<":
		return token.LSS
	case ">":
		return token.GTR
	case "<=":
		return token.LEQ
	case ">=":
		return token.GEQ
	case "&&":
		return token.LAND
	case "||":
		return token.LOR
	case "&":
		return token.AND
	case "|":
		return token.OR
	case "^":
		return token.XOR
	case "<<":
		return token.SHL
	case ">>", ">>>":
		return token.SHR
	default:
		return token.ADD
	}
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
	leftNode := node.ChildByFieldName("left")
	rightNode := node.ChildByFieldName("right")

	left := t.transformExpr(leftNode)
	right := t.transformExpr(rightNode)

	if left == nil || right == nil {
		return ident("nil")
	}

	// Return as a pseudo-expression; the statement layer will handle it
	return &ast.BinaryExpr{X: left, Op: token.ASSIGN, Y: right}
}

func (t *Transformer) transformAugmentedAssignment(node *sitter.Node) ast.Expr {
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

	op := mapAugmentedOp(opText)
	return &ast.BinaryExpr{X: left, Op: op, Y: right}
}

func mapAugmentedOp(op string) token.Token {
	switch op {
	case "+=":
		return token.ADD_ASSIGN
	case "-=":
		return token.SUB_ASSIGN
	case "*=":
		return token.MUL_ASSIGN
	case "/=":
		return token.QUO_ASSIGN
	case "%=":
		return token.REM_ASSIGN
	case "&=":
		return token.AND_ASSIGN
	case "|=":
		return token.OR_ASSIGN
	case "^=":
		return token.XOR_ASSIGN
	case "<<=":
		return token.SHL_ASSIGN
	case ">>=":
		return token.SHR_ASSIGN
	default:
		return token.ADD_ASSIGN
	}
}

func (t *Transformer) transformCallExpr(node *sitter.Node) ast.Expr {
	fnNode := node.ChildByFieldName("function")
	argsNode := node.ChildByFieldName("arguments")

	if fnNode == nil {
		return ident("nil")
	}

	// Check for special call patterns
	if fnNode.Kind() == "member_expression" {
		objNode := fnNode.ChildByFieldName("object")
		propNode := fnNode.ChildByFieldName("property")

		if objNode != nil && propNode != nil {
			obj := objNode.Utf8Text(t.source)
			prop := propNode.Utf8Text(t.source)

			result := t.transformSpecialCall(obj, prop, argsNode)
			if result != nil {
				return result
			}
		}
	}

	fun := t.transformExpr(fnNode)
	args := t.transformArgs(argsNode)

	return callExpr(fun, args...)
}

func (t *Transformer) transformSpecialCall(obj, prop string, argsNode *sitter.Node) ast.Expr {
	args := t.transformArgs(argsNode)

	switch obj {
	case "console":
		t.addImport("fmt")
		switch prop {
		case "log":
			return callExpr(selectorExpr(ident("fmt"), "Println"), args...)
		case "error", "warn":
			t.addImport("os")
			return callExpr(selectorExpr(ident("fmt"), "Fprintln"),
				append([]ast.Expr{selectorExpr(ident("os"), "Stderr")}, args...)...)
		case "dir":
			return callExpr(selectorExpr(ident("fmt"), "Printf"),
				append([]ast.Expr{stringLit("%+v\\n")}, args...)...)
		}

	case "Math":
		t.addImport("math")
		switch prop {
		case "floor":
			return callExpr(selectorExpr(ident("math"), "Floor"), args...)
		case "ceil":
			return callExpr(selectorExpr(ident("math"), "Ceil"), args...)
		case "round":
			return callExpr(selectorExpr(ident("math"), "Round"), args...)
		case "abs":
			return callExpr(selectorExpr(ident("math"), "Abs"), args...)
		case "max":
			return callExpr(selectorExpr(ident("math"), "Max"), args...)
		case "min":
			return callExpr(selectorExpr(ident("math"), "Min"), args...)
		case "sqrt":
			return callExpr(selectorExpr(ident("math"), "Sqrt"), args...)
		case "pow":
			return callExpr(selectorExpr(ident("math"), "Pow"), args...)
		case "random":
			t.addImport("math/rand")
			return callExpr(selectorExpr(ident("rand"), "Float64"))
		}

	case "JSON":
		t.addImport("encoding/json")
		switch prop {
		case "stringify":
			if len(args) > 0 {
				return callExpr(selectorExpr(ident("json"), "Marshal"), args[0])
			}
		case "parse":
			if len(args) > 0 {
				// JSON.parse(x) → func() any { var v any; json.Unmarshal([]byte(x), &v); return v }()
				return &ast.CallExpr{
					Fun: &ast.FuncLit{
						Type: &ast.FuncType{
							Params:  fieldList(),
							Results: fieldList(field("", ident("any"))),
						},
						Body: blockStmt(
							&ast.DeclStmt{Decl: varDecl("v", ident("any"), nil)},
							exprStmt(callExpr(
								selectorExpr(ident("json"), "Unmarshal"),
								callExpr(ident("[]byte"), args[0]),
								addrOf(ident("v")),
							)),
							returnStmt(ident("v")),
						),
					},
				}
			}
		}

	case "Object":
		switch prop {
		case "keys":
			// No direct equivalent; return a placeholder
			if len(args) > 0 {
				return args[0]
			}
		case "values":
			if len(args) > 0 {
				return args[0]
			}
		case "assign":
			if len(args) > 0 {
				return args[0]
			}
		}
	}

	// Check for array method patterns on any object
	switch prop {
	case "push":
		objExpr := ident(obj)
		if len(args) > 0 {
			return callExpr(ident("append"), append([]ast.Expr{objExpr}, args...)...)
		}
	case "length":
		return callExpr(ident("len"), ident(obj))
	case "toString":
		t.addImport("fmt")
		return callExpr(selectorExpr(ident("fmt"), "Sprint"), ident(obj))
	case "includes":
		// No direct Go equivalent; return a placeholder comment
		if len(args) > 0 {
			return callExpr(ident("contains"), append([]ast.Expr{ident(obj)}, args...)...)
		}
	case "indexOf":
		t.addImport("strings")
		if len(args) > 0 {
			return callExpr(selectorExpr(ident("strings"), "Index"),
				ident(obj), args[0])
		}
	case "split":
		t.addImport("strings")
		if len(args) > 0 {
			return callExpr(selectorExpr(ident("strings"), "Split"),
				ident(obj), args[0])
		}
	case "join":
		t.addImport("strings")
		if len(args) > 0 {
			return callExpr(selectorExpr(ident("strings"), "Join"),
				ident(obj), args[0])
		}
	case "trim":
		t.addImport("strings")
		return callExpr(selectorExpr(ident("strings"), "TrimSpace"), ident(obj))
	case "toLowerCase":
		t.addImport("strings")
		return callExpr(selectorExpr(ident("strings"), "ToLower"), ident(obj))
	case "toUpperCase":
		t.addImport("strings")
		return callExpr(selectorExpr(ident("strings"), "ToUpper"), ident(obj))
	case "startsWith":
		t.addImport("strings")
		if len(args) > 0 {
			return callExpr(selectorExpr(ident("strings"), "HasPrefix"),
				ident(obj), args[0])
		}
	case "endsWith":
		t.addImport("strings")
		if len(args) > 0 {
			return callExpr(selectorExpr(ident("strings"), "HasSuffix"),
				ident(obj), args[0])
		}
	case "replace":
		t.addImport("strings")
		if len(args) >= 2 {
			return callExpr(selectorExpr(ident("strings"), "Replace"),
				ident(obj), args[0], args[1], intLit("1"))
		}
	case "replaceAll":
		t.addImport("strings")
		if len(args) >= 2 {
			return callExpr(selectorExpr(ident("strings"), "ReplaceAll"),
				ident(obj), args[0], args[1])
		}
	case "slice":
		if len(args) >= 2 {
			return &ast.SliceExpr{X: ident(obj), Low: args[0], High: args[1]}
		}
		if len(args) == 1 {
			return &ast.SliceExpr{X: ident(obj), Low: args[0]}
		}
	case "map", "filter", "forEach", "reduce", "find", "some", "every":
		// These need more complex transforms; for now pass through
	}

	return nil
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
				inner := t.transformExpr(child.NamedChild(0))
				if inner != nil {
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

	obj := t.transformExpr(objNode)
	prop := propNode.Utf8Text(t.source)

	// Special property transforms
	if prop == "length" {
		return callExpr(ident("len"), obj)
	}

	// Check for optional chaining (the ?. operator)
	// In tree-sitter this shows up as optional_chain
	optionalChain := false
	for i := uint(0); i < node.ChildCount(); i++ {
		child := node.Child(i)
		if child.Kind() == "?." || child.Kind() == "optional_chain" {
			optionalChain = true
			break
		}
	}

	if optionalChain {
		// For now, just do regular member access
		return selectorExpr(obj, capitalize(prop))
	}

	return selectorExpr(obj, capitalize(prop))
}

func (t *Transformer) transformSubscriptExpr(node *sitter.Node) ast.Expr {
	objNode := node.ChildByFieldName("object")
	indexNode := node.ChildByFieldName("index")

	obj := t.transformExpr(objNode)
	index := t.transformExpr(indexNode)

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
		child := node.NamedChild(i)
		if e := t.transformExpr(child); e != nil {
			elts = append(elts, e)
		}
	}

	// Infer element type from first element
	var elemType ast.Expr = ident("any")
	hasFloat := false
	hasInt := false
	hasString := false
	hasBool := false

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
		// Use float64 for numeric arrays (TS number = float64)
		elemType = ident("float64")
		// Convert int literals to float literals for consistency
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
				val := t.transformExpr(valNode)
				if val != nil {
					elts = append(elts, keyValue(stringLit(key), val))
				}
			}
		case "shorthand_property_identifier":
			name := child.Utf8Text(t.source)
			elts = append(elts, keyValue(stringLit(name), ident(name)))
		case "spread_element":
			// Can't directly spread in Go map literal; skip
		}
	}

	return compositeLit(mapType(ident("string"), ident("any")), elts...)
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
				expr := t.transformExpr(child.NamedChild(0))
				if expr != nil {
					args = append(args, expr)
				}
			}
		case "`":
			// Skip backticks
		default:
			if child.IsNamed() {
				formatParts = append(formatParts, "%v")
				expr := t.transformExpr(child)
				if expr != nil {
					args = append(args, expr)
				}
			}
		}
	}

	format := strings.Join(formatParts, "")
	allArgs := append([]ast.Expr{stringLit(format)}, args...)
	return callExpr(selectorExpr(ident("fmt"), "Sprintf"), allArgs...)
}

func (t *Transformer) transformTernary(node *sitter.Node) ast.Expr {
	condNode := node.ChildByFieldName("condition")
	consNode := node.ChildByFieldName("consequence")
	altNode := node.ChildByFieldName("alternative")

	cond := t.transformExpr(condNode)
	cons := t.transformExpr(consNode)
	alt := t.transformExpr(altNode)

	if cond == nil || cons == nil || alt == nil {
		return ident("nil")
	}

	// Go doesn't have ternary; use immediately-invoked func
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

	// Single parameter without parens
	paramNode := node.ChildByFieldName("parameter")

	var params *ast.FieldList
	if paramsNode != nil {
		params = t.transformParams(paramsNode)
	} else if paramNode != nil {
		name := paramNode.Utf8Text(t.source)
		params = fieldList(field(name, ident("any")))
	} else {
		params = fieldList()
	}

	var results *ast.FieldList
	if returnTypeNode != nil {
		retType := t.getTypeAnnotation(returnTypeNode)
		if retType != nil {
			results = fieldList(field("", retType))
		}
	}

	var body *ast.BlockStmt
	if bodyNode != nil {
		if bodyNode.Kind() == "statement_block" {
			body = t.transformBlock(bodyNode)
		} else {
			// Expression body → return expr
			expr := t.transformExpr(bodyNode)
			if expr != nil {
				body = blockStmt(returnStmt(expr))
			} else {
				body = blockStmt()
			}
		}
	} else {
		body = blockStmt()
	}

	return &ast.FuncLit{
		Type: &ast.FuncType{Params: params, Results: results},
		Body: body,
	}
}

func (t *Transformer) transformFuncExpr(node *sitter.Node) ast.Expr {
	paramsNode := node.ChildByFieldName("parameters")
	bodyNode := node.ChildByFieldName("body")
	returnTypeNode := node.ChildByFieldName("return_type")

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

	// new ClassName(args) → NewClassName(args) or &ClassName{} depending on context
	// For known patterns, use factory function
	args := t.transformArgs(argsNode)

	switch name {
	case "Error":
		t.addImport("errors")
		if len(args) > 0 {
			return callExpr(selectorExpr(ident("errors"), "New"), args[0])
		}
		return callExpr(selectorExpr(ident("errors"), "New"), stringLit("error"))
	case "Map":
		return callExpr(ident("make"), mapType(ident("any"), ident("any")))
	case "Set":
		return callExpr(ident("make"), mapType(ident("any"), &ast.StructType{Fields: &ast.FieldList{}}))
	case "Date":
		t.addImport("time")
		return callExpr(selectorExpr(ident("time"), "Now"))
	case "RegExp":
		t.addImport("regexp")
		if len(args) > 0 {
			return callExpr(selectorExpr(ident("regexp"), "MustCompile"), args[0])
		}
	default:
		// new Foo(args) → NewFoo(args)
		factoryName := fmt.Sprintf("New%s", capitalize(name))
		if len(args) > 0 {
			return callExpr(ident(factoryName), args...)
		}
		// No args → &Foo{}
		return addrOf(compositeLit(ident(name)))
	}

	return ident("nil")
}
