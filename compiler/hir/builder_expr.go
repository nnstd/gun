package hir

import (
	"strconv"
	"strings"

	"github.com/nnstd/gun/compiler/symbol"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// --------------------------------------------------------------------
// Expressions
// --------------------------------------------------------------------

func (b *Builder) buildExpr(node *sitter.Node) Expr {
	if node == nil {
		return nil
	}
	switch node.Kind() {
	case "identifier":
		name := b.nodeText(node)
		if sym := b.symtab.Lookup(name); sym != nil {
			return &Identifier{Sym: sym, Name: name}
		}
		return &Identifier{Name: name}

	case "number":
		return &Literal{Kind: LitNumber, Value: b.nodeText(node)}

	case "string", "string_fragment":
		raw := b.nodeText(node)
		// Strip surrounding quotes if present
		if len(raw) >= 2 && (raw[0] == '\'' || raw[0] == '"' || raw[0] == '`') {
			quoted := raw
			if unquoted, err := strconv.Unquote(quoted); err == nil {
				raw = unquoted
			} else {
				raw = raw[1 : len(raw)-1]
			}
		} else {
			raw = decodeJSStringFragment(raw)
		}
		return &Literal{Kind: LitString, Value: raw}

	case "true":
		return &Literal{Kind: LitBool, Value: "true"}

	case "false":
		return &Literal{Kind: LitBool, Value: "false"}

	case "null":
		return &Literal{Kind: LitNull, Value: "null"}

	case "undefined":
		return &Literal{Kind: LitUndefined, Value: "undefined"}

	case "regex":
		return &Literal{Kind: LitRegex, Value: b.nodeText(node)}

	case "template_string":
		return b.buildTemplateLiteral(node)

	case "tagged_template_expression":
		tag := b.buildExpr(node.ChildByFieldName("function"))
		tmpl := node.ChildByFieldName("arguments")
		var template *TemplateLiteral
		if tmpl != nil {
			if tl, ok := b.buildExpr(tmpl).(*TemplateLiteral); ok {
				template = tl
			}
		}
		return &TaggedTemplateLiteral{Tag: tag, Template: template}

	case "array":
		return b.buildArrayLiteral(node)

	case "object":
		return b.buildObjectLiteral(node)

	case "binary_expression":
		return b.buildBinaryExpr(node)

	case "unary_expression":
		return b.buildUnaryExpr(node)

	case "update_expression":
		return b.buildUpdateExpr(node)

	case "assignment_expression":
		// Member-target destructuring: ({k: this.k} = obj) → SequenceExpr
		leftNode := node.ChildByFieldName("left")
		if leftNode != nil && leftNode.Kind() == "object_pattern" && b.hasNonIdentValues(leftNode) {
			return b.buildMemberDestructuringSeq(node)
		}
		return b.buildAssignExpr(node)

	case "augmented_assignment_expression":
		return b.buildAugmentedAssignExpr(node)

	case "ternary_expression":
		return b.buildTernaryExpr(node)

	case "call_expression":
		return b.buildCallExpr(node)

	case "new_expression":
		return b.buildNewExpr(node)

	case "class", "class_expression", "class_declaration", "abstract_class_declaration":
		return b.buildClassExpr(node)

	case "member_expression":
		return b.buildMemberExpr(node)

	case "subscript_expression":
		return b.buildComputedMemberExpr(node)

	case "arrow_function":
		return b.buildArrowFunc(node)

	case "function_expression", "function":
		return b.buildFuncExpr(node)

	case "parenthesized_expression":
		if node.NamedChildCount() > 0 {
			inner := b.buildExpr(node.NamedChild(0))
			return &ParenExpr{Expr: inner}
		}
		return nil

	case "spread_element":
		if node.NamedChildCount() > 0 {
			return &SpreadExpr{Value: b.buildExpr(node.NamedChild(0))}
		}
		return nil

	case "sequence_expression":
		var exprs []Expr
		for i := uint(0); i < node.NamedChildCount(); i++ {
			if e := b.buildExpr(node.NamedChild(i)); e != nil {
				exprs = append(exprs, e)
			}
		}
		return &SequenceExpr{Exprs: exprs, Span: b.span(node)}

	case "await_expression":
		if node.NamedChildCount() > 0 {
			return &AwaitExpr{Value: b.buildExpr(node.NamedChild(0))}
		}
		return nil

	case "yield_expression":
		delegate := false
		var value Expr
		for i := uint(0); i < node.ChildCount(); i++ {
			if node.Child(i).Kind() == "*" {
				delegate = true
			}
		}
		if node.NamedChildCount() > 0 {
			value = b.buildExpr(node.NamedChild(0))
		}
		return &YieldExpr{Value: value, Delegate: delegate}

	case "as_expression", "type_assertion":
		// Type assertions: preserve the expression, note the type
		exprNode := node.ChildByFieldName("expression")
		if exprNode == nil && node.NamedChildCount() > 0 {
			exprNode = node.NamedChild(0)
		}
		typeStr := ""
		if t := node.ChildByFieldName("type"); t != nil {
			typeStr = b.nodeText(t)
		}
		if exprNode != nil {
			return &TypeAssertExpr{
				Expr: b.buildExpr(exprNode),
				Type: typeStr,
			}
		}
		return nil

	case "non_null_expression":
		if node.NamedChildCount() > 0 {
			return &NonNullExpr{Expr: b.buildExpr(node.NamedChild(0))}
		}
		return nil

	case "this":
		return &ThisExpr{}

	case "super":
		return &SuperExpr{}

	case "private_property_identifier":
		return &PrivateIdentifierExpr{Name: strings.TrimPrefix(b.nodeText(node), "#")}

	case "meta_property":
		meta := ""
		prop := ""
		if node.ChildCount() >= 2 {
			meta = b.nodeText(node.Child(0))
			prop = b.nodeText(node.Child(node.ChildCount() - 1))
		}
		return &MetaPropertyExpr{Meta: meta, Property: prop}

	case "type_identifier", "predefined_type", "generic_type":
		// Type references in expression context — just use as identifier
		return &Identifier{Name: b.nodeText(node)}

	case "comment", "line_comment", "block_comment":
		return nil

	default:
		// Fallback: return as identifier with raw text
		text := b.nodeText(node)
		if text != "" {
			return &Identifier{Name: text}
		}
		return nil
	}
}

func (b *Builder) buildTemplateLiteral(node *sitter.Node) *TemplateLiteral {
	tl := &TemplateLiteral{}
	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		switch child.Kind() {
		case "string_fragment", "template_content":
			tl.Parts = append(tl.Parts, &Literal{Kind: LitString, Value: decodeJSStringFragment(b.nodeText(child))})
		case "escape_sequence":
			tl.Parts = append(tl.Parts, &Literal{Kind: LitString, Value: decodeJSStringFragment(b.nodeText(child))})
		case "template_substitution":
			if child.NamedChildCount() > 0 {
				tl.Parts = append(tl.Parts, b.buildExpr(child.NamedChild(0)))
			}
		default:
			tl.Parts = append(tl.Parts, b.buildExpr(child))
		}
	}
	return tl
}

func decodeJSStringFragment(raw string) string {
	if raw == "" {
		return raw
	}
	quoted := `"` + strings.ReplaceAll(raw, `"`, `\"`) + `"`
	if unquoted, err := strconv.Unquote(quoted); err == nil {
		return unquoted
	}
	return raw
}

func (b *Builder) buildArrayLiteral(node *sitter.Node) *ArrayLiteral {
	al := &ArrayLiteral{}
	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		al.Elements = append(al.Elements, b.buildExpr(child))
	}
	return al
}

func (b *Builder) buildObjectLiteral(node *sitter.Node) *ObjectLiteral {
	ol := &ObjectLiteral{}
	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		switch child.Kind() {
		case "pair":
			keyNode := child.ChildByFieldName("key")
			valueNode := child.ChildByFieldName("value")
			if keyNode != nil && valueNode != nil {
				prop := &Property{
					KeyName: b.nodeText(keyNode),
					Key:     b.buildExpr(keyNode),
					Value:   b.buildExpr(valueNode),
				}
				// Computed property name: [expr]
				if keyNode.Kind() == "computed_property_name" {
					prop.Computed = true
					// Build the inner expression (unwrap the brackets)
					if keyNode.NamedChildCount() > 0 {
						prop.Key = b.buildExpr(keyNode.NamedChild(0))
					}
				}
				ol.Properties = append(ol.Properties, prop)
			}
		case "shorthand_property_identifier", "shorthand_property_identifier_pattern":
			name := b.nodeText(child)
			ol.Properties = append(ol.Properties, &Property{
				KeyName: name,
				Key:     &Literal{Kind: LitString, Value: name},
				Value:   &Identifier{Name: name, Sym: b.symtab.Lookup(name)},
			})
		case "method_definition":
			nameNode := child.ChildByFieldName("name")
			if nameNode == nil {
				continue
			}
			name := b.nodeText(nameNode)
			paramsNode := child.ChildByFieldName("parameters")
			bodyNode := child.ChildByFieldName("body")
			params := b.buildParams(paramsNode)
			var body *BlockStmt
			if bodyNode != nil {
				body = b.buildBlock(bodyNode)
			}
			ol.Properties = append(ol.Properties, &Property{
				KeyName: name,
				Key:     &Literal{Kind: LitString, Value: name},
				Value:   &ArrowFunc{Params: params, Body: body},
				Method:  true,
			})
		case "spread_element":
			if child.NamedChildCount() > 0 {
				ol.Properties = append(ol.Properties, &Property{
					Key:   &SpreadExpr{Value: b.buildExpr(child.NamedChild(0))},
					Value: b.buildExpr(child.NamedChild(0)),
				})
			}
		}
	}
	return ol
}

func (b *Builder) buildBinaryExpr(node *sitter.Node) *BinaryExpr {
	leftNode := node.ChildByFieldName("left")
	rightNode := node.ChildByFieldName("right")
	opNode := node.ChildByFieldName("operator")

	op := OpAdd
	if opNode != nil {
		op = parseBinaryOp(b.nodeText(opNode))
	}

	return &BinaryExpr{
		Op:    op,
		Left:  b.buildExpr(leftNode),
		Right: b.buildExpr(rightNode),
	}
}

func parseBinaryOp(s string) BinaryOp {
	switch s {
	case "+":
		return OpAdd
	case "-":
		return OpSub
	case "*":
		return OpMul
	case "/":
		return OpDiv
	case "%":
		return OpMod
	case "**":
		return OpExp
	case "===":
		return OpEq
	case "!==":
		return OpNEq
	case "==":
		return OpEqLoose
	case "!=":
		return OpNEqLoose
	case "<":
		return OpLt
	case ">":
		return OpGt
	case "<=":
		return OpLtE
	case ">=":
		return OpGtE
	case "&&":
		return OpAnd
	case "||":
		return OpOr
	case "??":
		return OpNullish
	case "&":
		return OpBitAnd
	case "|":
		return OpBitOr
	case "^":
		return OpBitXor
	case "<<":
		return OpShl
	case ">>":
		return OpShr
	case ">>>":
		return OpUShr
	case "in":
		return OpIn
	case "instanceof":
		return OpInstanceof
	default:
		return OpAdd
	}
}

func (b *Builder) buildUnaryExpr(node *sitter.Node) *UnaryExpr {
	opNode := node.ChildByFieldName("operator")
	argNode := node.ChildByFieldName("argument")

	op := OpNot
	if opNode != nil {
		switch b.nodeText(opNode) {
		case "!":
			op = OpNot
		case "-":
			op = OpNeg
		case "+":
			op = OpPos
		case "~":
			op = OpBitNot
		case "typeof":
			op = OpTypeof
		case "void":
			op = OpVoid
		case "delete":
			op = OpDelete
		}
	}

	return &UnaryExpr{
		Op:      op,
		Operand: b.buildExpr(argNode),
		Prefix:  true,
	}
}

func (b *Builder) buildUpdateExpr(node *sitter.Node) *UpdateExpr {
	argNode := node.ChildByFieldName("argument")
	opNode := node.ChildByFieldName("operator")

	op := OpInc
	if opNode != nil && b.nodeText(opNode) == "--" {
		op = OpDec
	}

	// Determine prefix vs postfix by position
	prefix := false
	if opNode != nil && argNode != nil {
		prefix = opNode.StartByte() < argNode.StartByte()
	}

	return &UpdateExpr{
		Op:      op,
		Operand: b.buildExpr(argNode),
		Prefix:  prefix,
	}
}

func (b *Builder) buildAssignExpr(node *sitter.Node) *AssignExpr {
	leftNode := node.ChildByFieldName("left")
	rightNode := node.ChildByFieldName("right")

	// Destructuring assignment: [a, b] = expr or {x, y} = expr
	// Uses Lookup (not Define) since variables already exist in assignment context.
	if leftNode != nil {
		switch leftNode.Kind() {
		case "array_pattern":
			return &AssignExpr{
				Op:          OpAssign,
				LeftPattern: b.buildArrayPatternLookup(leftNode),
				Right:       b.buildExpr(rightNode),
				Span:        b.span(node),
			}
		case "object_pattern":
			return &AssignExpr{
				Op:          OpAssign,
				LeftPattern: b.buildObjectPatternLookup(leftNode),
				Right:       b.buildExpr(rightNode),
				Span:        b.span(node),
			}
		}
	}

	return &AssignExpr{
		Op:    OpAssign,
		Left:  b.buildExpr(leftNode),
		Right: b.buildExpr(rightNode),
		Span:  b.span(node),
	}
}

func (b *Builder) buildAugmentedAssignExpr(node *sitter.Node) *AssignExpr {
	leftNode := node.ChildByFieldName("left")
	rightNode := node.ChildByFieldName("right")
	opNode := node.ChildByFieldName("operator")

	op := OpAddAssign
	if opNode != nil {
		op = parseAssignOp(b.nodeText(opNode))
	}

	return &AssignExpr{
		Op:    op,
		Left:  b.buildExpr(leftNode),
		Right: b.buildExpr(rightNode),
		Span:  b.span(node),
	}
}

func parseAssignOp(s string) AssignOp {
	switch s {
	case "=":
		return OpAssign
	case "+=":
		return OpAddAssign
	case "-=":
		return OpSubAssign
	case "*=":
		return OpMulAssign
	case "/=":
		return OpDivAssign
	case "%=":
		return OpModAssign
	case "&=":
		return OpBitAndAssign
	case "|=":
		return OpBitOrAssign
	case "^=":
		return OpBitXorAssign
	case "<<=":
		return OpShlAssign
	case ">>=":
		return OpShrAssign
	case ">>>=":
		return OpUShrAssign
	case "??=":
		return OpNullishAssign
	case "&&=":
		return OpAndAssign
	case "||=":
		return OpOrAssign
	case "**=":
		return OpExpAssign
	default:
		return OpAssign
	}
}

func (b *Builder) buildTernaryExpr(node *sitter.Node) *TernaryExpr {
	condNode := node.ChildByFieldName("condition")
	consNode := node.ChildByFieldName("consequence")
	altNode := node.ChildByFieldName("alternative")

	return &TernaryExpr{
		Cond: b.buildExpr(condNode),
		Then: b.buildExpr(consNode),
		Else: b.buildExpr(altNode),
	}
}

func (b *Builder) buildCallExpr(node *sitter.Node) *CallExpr {
	fnNode := node.ChildByFieldName("function")
	argsNode := node.ChildByFieldName("arguments")

	fn := b.buildExpr(fnNode)

	var args []Expr
	if argsNode != nil {
		for i := uint(0); i < argsNode.NamedChildCount(); i++ {
			child := argsNode.NamedChild(i)
			if e := b.buildExpr(child); e != nil {
				args = append(args, e)
			}
		}
	}

	return &CallExpr{Func: fn, Args: args, Span: b.span(node)}
}

func (b *Builder) buildNewExpr(node *sitter.Node) *NewExpr {
	var callee Expr
	var args []Expr

	// First named child is the constructor
	if node.NamedChildCount() > 0 {
		callee = b.buildExpr(node.NamedChild(0))
	}

	// Arguments node
	argsNode := node.ChildByFieldName("arguments")
	if argsNode != nil {
		for i := uint(0); i < argsNode.NamedChildCount(); i++ {
			if e := b.buildExpr(argsNode.NamedChild(i)); e != nil {
				args = append(args, e)
			}
		}
	}

	return &NewExpr{Callee: callee, Args: args, Span: b.span(node)}
}

func (b *Builder) buildClassExpr(node *sitter.Node) Expr {
	parts := b.buildClassParts(node)
	name := ""
	if nameNode := node.ChildByFieldName("name"); nameNode != nil {
		name = b.nodeText(nameNode)
	}
	return &ClassExpr{
		Name:        name,
		Parent:      parts.parent,
		Constructor: parts.constructor,
		Methods:     parts.methods,
		Properties:  parts.properties,
		StaticInits: parts.staticInits,
		Span:        b.span(node),
	}
}

func (b *Builder) buildMemberExpr(node *sitter.Node) Expr {
	objNode := node.ChildByFieldName("object")
	propNode := node.ChildByFieldName("property")

	if objNode == nil || propNode == nil {
		return nil
	}

	// Check for optional chaining: obj?.prop
	isOptional := false
	if node.ChildByFieldName("optional_chain") != nil {
		isOptional = true
	}
	if !isOptional {
		for i := uint(0); i < node.ChildCount(); i++ {
			if node.Child(i).Kind() == "?." || node.Child(i).Kind() == "optional_chain" {
				isOptional = true
				break
			}
		}
	}
	obj := b.buildExpr(objNode)
	prop := b.nodeText(propNode)
	isPrivate := propNode.Kind() == "private_property_identifier"
	if isPrivate {
		prop = strings.TrimPrefix(prop, "#")
	}

	return &MemberExpr{Object: obj, Property: prop, Private: isPrivate, Optional: isOptional, Span: b.span(node)}
}

func (b *Builder) buildComputedMemberExpr(node *sitter.Node) Expr {
	objNode := node.ChildByFieldName("object")
	indexNode := node.ChildByFieldName("index")

	if objNode == nil || indexNode == nil {
		return nil
	}

	return &ComputedMemberExpr{
		Object:   b.buildExpr(objNode),
		Property: b.buildExpr(indexNode),
		Span:     b.span(node),
	}
}

func (b *Builder) buildArrowFunc(node *sitter.Node) *ArrowFunc {
	paramsNode := node.ChildByFieldName("parameters")
	if paramsNode == nil {
		// Single-param arrow: code => ... uses "parameter" field (singular)
		paramsNode = node.ChildByFieldName("parameter")
	}
	bodyNode := node.ChildByFieldName("body")

	// Check for async
	isAsync := false
	for i := uint(0); i < node.ChildCount(); i++ {
		if node.Child(i).Kind() == "async" {
			isAsync = true
			break
		}
	}

	b.symtab.PushScope()
	defer b.symtab.PopScope()

	var params []*Param
	if paramsNode != nil {
		if paramsNode.Kind() == "identifier" {
			// Single param arrow: x => ...
			name := b.nodeText(paramsNode)
			sym := b.symtab.Define(name, symbol.KindParameter)
			params = []*Param{{Symbol: sym}}
		} else {
			params = b.buildParams(paramsNode)
		}
	}

	af := &ArrowFunc{
		Params:  params,
		IsAsync: isAsync,
		Span:    b.span(node),
	}

	if bodyNode != nil {
		if bodyNode.Kind() == "statement_block" {
			af.Body = b.buildBlock(bodyNode)
		} else {
			// Concise body: () => expr
			af.ExprBody = b.buildExpr(bodyNode)
		}
	}

	return af
}

func (b *Builder) buildFuncExpr(node *sitter.Node) *FuncExpr {
	nameNode := node.ChildByFieldName("name")
	paramsNode := node.ChildByFieldName("parameters")
	bodyNode := node.ChildByFieldName("body")

	name := ""
	if nameNode != nil {
		name = b.nodeText(nameNode)
	}

	isAsync := false
	for i := uint(0); i < node.ChildCount(); i++ {
		if node.Child(i).Kind() == "async" {
			isAsync = true
			break
		}
	}

	b.symtab.PushScope()
	defer b.symtab.PopScope()

	params := b.buildParams(paramsNode)

	var body *BlockStmt
	if bodyNode != nil {
		body = b.buildBlock(bodyNode)
	}

	return &FuncExpr{
		Name:    name,
		Params:  params,
		Body:    body,
		IsAsync: isAsync,
		Span:    b.span(node),
	}
}

// stripQuotes removes surrounding quotes from a string.
func stripQuotes(s string) string {
	if len(s) >= 2 {
		if (s[0] == '\'' || s[0] == '"') && s[len(s)-1] == s[0] {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// unused but kept for reference
var _ = strings.TrimPrefix
var _ = stripQuotes

// hasNonIdentValues checks if an object_pattern has any pair_pattern
// whose value is not a simple identifier (e.g. member expressions like this.prop).
func (b *Builder) hasNonIdentValues(node *sitter.Node) bool {
	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		if child.Kind() == "pair_pattern" {
			valueNode := child.ChildByFieldName("value")
			if valueNode != nil && valueNode.Kind() != "identifier" {
				return true
			}
		}
	}
	return false
}

// buildMemberDestructuringSeq handles object destructuring where targets
// are member expressions: ({handlers: this.handlers} = obj).
// Returns a SequenceExpr of individual assignments.
func (b *Builder) buildMemberDestructuringSeq(node *sitter.Node) *SequenceExpr {
	patNode := node.ChildByFieldName("left")
	rightNode := node.ChildByFieldName("right")
	right := b.buildExpr(rightNode)
	var exprs []Expr
	for i := uint(0); i < patNode.NamedChildCount(); i++ {
		child := patNode.NamedChild(i)
		if child.Kind() == "pair_pattern" {
			keyNode := child.ChildByFieldName("key")
			valueNode := child.ChildByFieldName("value")
			if keyNode != nil && valueNode != nil {
				key := b.nodeText(keyNode)
				target := b.buildExpr(valueNode)
				getter := &MemberExpr{
					Object:   right,
					Property: key,
				}
				exprs = append(exprs, &AssignExpr{
					Op:    OpAssign,
					Left:  target,
					Right: getter,
				})
			}
		}
	}
	return &SequenceExpr{Exprs: exprs, Span: b.span(node)}
}
