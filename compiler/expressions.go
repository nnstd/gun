package compiler

import (
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

	case "null":
		t.addAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
		return callExpr(selectorExpr(ident("jsvalue"), "NewNull"))

	case "undefined":
		t.addAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
		return callExpr(selectorExpr(ident("jsvalue"), "NewUndefined"))

	case "meta_property":
		// import.meta → module.ImportMeta
		t.addImport("github.com/nnstd/gun/runtime/module")
		return selectorExpr(ident("module"), "ImportMeta")

	case "this":
		// At package level (no scopes), this is undefined (ES modules)
		if len(t.localScopes) == 0 {
			t.addAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			return callExpr(selectorExpr(ident("jsvalue"), "NewUndefined"))
		}
		return ident("this")

	case "binary_expression":
		return t.transformBinaryExpr(node)

	case "unary_expression":
		return t.transformUnaryExpr(node)

	case "update_expression":
		return t.transformUpdateExpr(node)

	case "assignment_expression":
		return t.transformAssignmentExpr(node)

	case "sequence_expression":
		// JS comma operator: (expr1, expr2, ..., exprN) — evaluates all, returns last.
		// Wrap in IIFE to preserve side effects of all expressions.
		return t.transformSequenceExpr(node)

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
		t.addAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
		pattern := node.Utf8Text(t.source)
		// Strip JS regex delimiters: /pattern/flags → pattern
		if len(pattern) >= 2 && pattern[0] == '/' {
			end := strings.LastIndex(pattern, "/")
			if end > 0 {
				pattern = pattern[1:end]
			}
		}
		// Wrap in jsvalue.NewRegex for all-JSValue consistency
		// Use jsvalue.CompileRegex to handle JS-style \uNNNN escapes
		compiled := callExpr(
			selectorExpr(ident("jsvalue"), "CompileRegex"),
			basicLit(token.STRING, "`"+pattern+"`"),
		)
		return callExpr(selectorExpr(ident("jsvalue"), "NewRegex"), compiled)

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

	op := mapBinaryOp(opText)

	// Check if either operand is a JSValue expression.
	leftIsJSValue := t.nodeReturnsJSValue(leftNode)
	rightIsJSValue := t.nodeReturnsJSValue(rightNode)
	eitherJSValue := leftIsJSValue || rightIsJSValue || t.isPkgLevelVar(leftNode) || t.isPkgLevelVar(rightNode)

	// ?? (nullish coalescing) → jsvalue.Nullish(left, right)
	if opText == "??" {
		if eitherJSValue {
			t.addAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			return callExpr(selectorExpr(ident("jsvalue"), "Nullish"),
				jsvalueWrapLit(left), jsvalueWrapLit(right))
		}
		return left
	}

	// instanceof → != nil
	if opText == "instanceof" {
		return &ast.BinaryExpr{X: left, Op: token.NEQ, Y: ident("nil")}
	}

	// in → obj.HasOwnProperty(key) wrapped as JSValue bool
	if opText == "in" {
		t.addAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
		t.addImport("fmt")
		return callExpr(selectorExpr(ident("jsvalue"), "NewBool"),
			callExpr(selectorExpr(right, "HasOwnProperty"), callExpr(selectorExpr(ident("fmt"), "Sprint"), left)))
	}

	// When either operand is JSValue, use jsvalue operation helpers.
	// This eliminates complex type coercion — the helpers handle type semantics internally.
	if eitherJSValue {
		t.addAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
		wrappedLeft := jsvalueWrapLit(left)
		wrappedRight := jsvalueWrapLit(right)

		// Logical || → jsvalue.Or(left, right) — short-circuit, returns first truthy
		if op == token.LOR {
			return callExpr(selectorExpr(ident("jsvalue"), "Or"), wrappedLeft, wrappedRight)
		}
		// Logical && → jsvalue.And(left, right) — short-circuit, returns first falsy
		if op == token.LAND {
			return callExpr(selectorExpr(ident("jsvalue"), "And"), wrappedLeft, wrappedRight)
		}

		// Loose equality (== / !=) → EqLoose/NEqLoose (null == undefined is true)
		// Strict equality (=== / !==) → Eq/NEq (null === undefined is false)
		if opText == "==" {
			return callExpr(selectorExpr(ident("jsvalue"), "EqLoose"), wrappedLeft, wrappedRight)
		}
		if opText == "!=" {
			return callExpr(selectorExpr(ident("jsvalue"), "NEqLoose"), wrappedLeft, wrappedRight)
		}

		// Arithmetic and comparison → jsvalue.Add/Sub/Eq/Lt etc.
		if fnName := jsvalueOpName(op); fnName != "" {
			return callExpr(selectorExpr(ident("jsvalue"), fnName), wrappedLeft, wrappedRight)
		}
	}

	// Non-JSValue paths: pure native Go operations (typed variables, Go stdlib, etc.)

	// Logical AND/OR require both operands to be bool in Go.
	if op == token.LAND || op == token.LOR {
		left = t.ensureBool(left)
		right = t.ensureBool(right)
	}

	return &ast.BinaryExpr{X: left, Op: op, Y: right}
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
		// !jsValue → jsvalue.Not(jsValue) — returns *JSValue boolean
		if argNode != nil && t.nodeReturnsJSValue(argNode) {
			t.addAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			return callExpr(selectorExpr(ident("jsvalue"), "Not"), arg)
		}
		// !x.length → x.Len() == 0 (length returns int, can't use ! on int)
		if argNode != nil && argNode.Kind() == "member_expression" {
			propNode := argNode.ChildByFieldName("property")
			if propNode != nil && propNode.Utf8Text(t.source) == "length" {
				return &ast.BinaryExpr{X: arg, Op: token.EQL, Y: intLit("0")}
			}
		}
		return &ast.UnaryExpr{Op: token.NOT, X: arg}
	case "-":
		// -jsValue → jsvalue.Neg(jsValue)
		if argNode != nil && t.nodeReturnsJSValue(argNode) {
			t.addAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			return callExpr(selectorExpr(ident("jsvalue"), "Neg"), arg)
		}
		return &ast.UnaryExpr{Op: token.SUB, X: arg}
	case "+":
		return arg
	case "~":
		// ~jsValue → jsvalue.BitNot(jsValue)
		if argNode != nil && t.nodeReturnsJSValue(argNode) {
			t.addAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			return callExpr(selectorExpr(ident("jsvalue"), "BitNot"), arg)
		}
		return &ast.UnaryExpr{Op: token.XOR, X: arg}
	case "typeof":
		// typeof jsValue → jsvalue.TypeOf(jsValue)
		if argNode != nil && t.nodeReturnsJSValue(argNode) {
			t.addAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			return callExpr(selectorExpr(ident("jsvalue"), "TypeOf"), arg)
		}
		t.addImport("fmt")
		return callExpr(selectorExpr(ident("fmt"), "Sprintf"), stringLit("%T"), arg)
	case "void":
		// void X in JS always evaluates to undefined
		t.addAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
		return callExpr(selectorExpr(ident("jsvalue"), "NewUndefined"))
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

func (t *Transformer) transformSequenceExpr(node *sitter.Node) ast.Expr {
	// JS comma operator: (a, b, c) evaluates all expressions, returns last.
	// Wrap in IIFE: func() T { a; b; return c }()
	count := node.NamedChildCount()
	if count == 0 {
		return ident("nil")
	}
	if count == 1 {
		return t.transformExpr(node.NamedChild(0))
	}

	var stmts []ast.Stmt
	for i := uint(0); i < count-1; i++ {
		child := node.NamedChild(i)
		if child.Kind() == "assignment_expression" {
			// Side-effecting assignment: transform as statement
			lhs := t.transformExpr(child.ChildByFieldName("left"))
			rhs := t.transformExpr(child.ChildByFieldName("right"))
			// Skip invalid assignments like nil = value (from unsupported destructuring)
			if lhs != nil && rhs != nil {
				if id, ok := lhs.(*ast.Ident); ok && id.Name == "nil" {
					continue
				}
				stmts = append(stmts, assignStmt([]ast.Expr{lhs}, []ast.Expr{rhs}))
			}
		} else if e := t.transformExpr(child); e != nil {
			stmts = append(stmts, exprStmt(e))
		}
	}

	lastChild := node.NamedChild(count - 1)
	lastExpr := t.transformExpr(lastChild)
	if lastExpr == nil {
		lastExpr = ident("nil")
	}

	// Determine return type
	var retType ast.Expr = ident("any")
	if t.nodeReturnsJSValue(lastChild) {
		t.addAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
		retType = jsValuePtrType()
	}

	stmts = append(stmts, returnStmt(lastExpr))
	return &ast.CallExpr{
		Fun: &ast.FuncLit{
			Type: &ast.FuncType{
				Params:  fieldList(),
				Results: fieldList(field("", retType)),
			},
			Body: &ast.BlockStmt{List: stmts},
		},
	}
}

func (t *Transformer) transformAssignmentExpr(node *sitter.Node) ast.Expr {
	leftNode := node.ChildByFieldName("left")
	rightNode := node.ChildByFieldName("right")

	// Handle member assignment on JSValue: obj.prop = value → obj.Set("prop", value)
	if leftNode != nil && leftNode.Kind() == "member_expression" {
		memObj := leftNode.ChildByFieldName("object")
		memProp := leftNode.ChildByFieldName("property")
		if memObj != nil && memProp != nil {
			isJSV := memObj.Kind() == "this" || (memObj.Kind() == "identifier" && t.isUntypedLocal(memObj.Utf8Text(t.source))) || t.nodeReturnsJSValue(memObj)
			if isJSV {
				obj := t.transformExpr(memObj)
				rhs := t.transformExpr(rightNode)
				if obj != nil && rhs != nil {
					rhs = t.wrapAsJSValue(rhs)
					t.addAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
					propName := memProp.Utf8Text(t.source)
					return &ast.CallExpr{
						Fun: &ast.FuncLit{
							Type: &ast.FuncType{
								Params:  fieldList(),
								Results: fieldList(field("", jsValuePtrType())),
							},
							Body: blockStmt(
								exprStmt(callExpr(selectorExpr(obj, "Set"), stringLit(propName), rhs)),
								returnStmt(rhs),
							),
						},
					}
				}
			}
		}
	}

	// Handle subscript assignment on JSValue: seen[key] = value → seen.Set(key, value)
	if leftNode != nil && leftNode.Kind() == "subscript_expression" {
		subObj := leftNode.ChildByFieldName("object")
		subIdx := leftNode.ChildByFieldName("index")
		isJSValueObj := subObj != nil && subObj.Kind() == "identifier" && (t.isUntypedLocal(subObj.Utf8Text(t.source)) || t.isUntypedLocal(sanitizeIdent(subObj.Utf8Text(t.source))))
		if !isJSValueObj && subObj != nil && t.nodeReturnsJSValue(subObj) {
			isJSValueObj = true
		}
		if isJSValueObj && subIdx != nil {
			obj := t.transformExpr(subObj)
			key := t.transformExpr(subIdx)
			rhs := t.transformExpr(rightNode)
			if obj != nil && key != nil && rhs != nil {
				if subIdx.Kind() != "string" && subIdx.Kind() != "string_literal" {
					t.addImport("fmt")
					key = callExpr(selectorExpr(ident("fmt"), "Sprint"), key)
				}
				rhs = t.wrapAsJSValue(rhs)
				t.addAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
				// IIFE: func() *jsvalue.JSValue { obj.Set(key, rhs); return rhs }()
				return &ast.CallExpr{
					Fun: &ast.FuncLit{
						Type: &ast.FuncType{
							Params:  fieldList(),
							Results: fieldList(field("", jsValuePtrType())),
						},
						Body: blockStmt(
							exprStmt(callExpr(selectorExpr(obj, "Set"), key, rhs)),
							returnStmt(rhs),
						),
					},
				}
			}
		}
	}

	left := t.transformExpr(leftNode)
	right := t.transformExpr(rightNode)
	if left == nil || right == nil {
		return ident("nil")
	}
	// Skip invalid assignments where LHS is nil (from unsupported destructuring patterns)
	if isNilIdent(left) {
		return right
	}
	// In JS, assignment is an expression that returns the assigned value.
	// In Go, assignment is a statement. Wrap in an IIFE to preserve semantics.
	// Use *jsvalue.JSValue return type when the target is an untyped variable
	// (package-level var or untyped param), since those default to *jsvalue.JSValue.
	var retType ast.Expr = ident("any")
	assignRHS := right
	retExpr := right
	if leftNode != nil && leftNode.Kind() == "identifier" {
		name := leftNode.Utf8Text(t.source)
		if t.isUntypedLocal(name) || t.isUntypedLocal(sanitizeIdent(name)) || !t.isLocalName(name) {
			retType = jsValuePtrType()
			t.addAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			// Wrap RHS with jsvalue.From() so any-typed expressions
			// (e.g. conditional IIFEs) convert to *jsvalue.JSValue.
			assignRHS = t.wrapAsJSValue(right)
			retExpr = left
		}
	}
	return &ast.CallExpr{
		Fun: &ast.FuncLit{
			Type: &ast.FuncType{
				Params:  fieldList(),
				Results: fieldList(field("", retType)),
			},
			Body: blockStmt(
				&ast.AssignStmt{Lhs: []ast.Expr{left}, Tok: token.ASSIGN, Rhs: []ast.Expr{assignRHS}},
				returnStmt(retExpr),
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

	// super(args) in class constructor → call parent constructor on this
	if fnNode.Kind() == "super" && t.currentClassParent != nil {
		args := t.transformArgs(argsNode)
		for i, arg := range args {
			args[i] = jsvalueWrapLit(arg)
		}
		// Parent.Call(this, args...) — NewClass constructors receive (this, args...)
		// But parent is a JSValue function, so we use parent.funcVal(this, args...)
		// which is what NewClass's inner constructor does.
		// Simplest: just call parent as a function that initializes this.
		allArgs := append([]ast.Expr{ident("this")}, args...)
		t.addAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
		// Call parent constructor on the child's this: Parent.CallSuper(this, args...)
		return callExpr(selectorExpr(t.currentClassParent, "CallSuper"), allArgs...)
	}

	// Check for builtin call patterns on member expressions
	if fnNode.Kind() == "member_expression" {
		objNode := fnNode.ChildByFieldName("object")
		propNode := fnNode.ChildByFieldName("property")

		if objNode != nil && propNode != nil {
			objText := objNode.Utf8Text(t.source)
			prop := propNode.Utf8Text(t.source)

			// Module-registered call transformers (e.g. hono route methods, yargs commands)
			// Check before transformArgs to avoid eagerly transforming callback args
			// that the module transformer will handle itself.
			modType := ""
			if mt, ok := t.varTypes[objText]; ok {
				modType = mt
			} else {
				modType = t.inferModuleType(objNode)
			}
			if modType != "" {
				if fn, ok := moduleCallTransformers[modType]; ok {
					if r := fn(t, objNode, prop, argsNode); r != nil {
						return r
					}
				}
			}

			args := t.transformArgs(argsNode)

			// Handle .call() — fn.call(thisArg, arg1, arg2) → fn.Call(thisArg, arg1, arg2)
			// Special case: Object.prototype.hasOwnProperty.call(obj, prop)
			// → jsvalue.NewBool(obj.HasOwnProperty(prop.String()))
			if prop == "call" {
				if isObjectPrototypeHasOwnProperty(objNode, t.source) && len(args) >= 2 {
					t.addImport("fmt")
					t.addAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
					return callExpr(selectorExpr(ident("jsvalue"), "NewBool"),
						callExpr(selectorExpr(args[0], "HasOwnProperty"), callExpr(selectorExpr(ident("fmt"), "Sprint"), args[1])))
				}
				// Generic .call(): fn.call(thisArg, ...args) → fn.Call(thisArg, ...args)
				obj := t.transformExpr(objNode)
				for i, a := range args {
					args[i] = jsvalueWrapLit(a)
				}
				t.addAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
				return callExpr(selectorExpr(obj, "Call"), args...)
			}


			// For Math calls, pre-coerce JSValue args to .Number() so that
			// math.Min/Max/Floor/etc. receive float64 values.
			if objText == "Math" && argsNode != nil {
				for i, arg := range args {
					if uint(i) < argsNode.NamedChildCount() {
						argNode := argsNode.NamedChild(uint(i))
						if argNode != nil && argNode.Kind() == "identifier" && t.isUntypedLocal(argNode.Utf8Text(t.source)) {
							args[i] = callExpr(selectorExpr(arg, "Number"))
						}
					}
				}
			}

			// Known global objects (console, Math, JSON, Object)
			if r := t.ctx.TransformBuiltinCall(objText, prop, args, t); r != nil {
				return r
			}

			// Regex method dispatch (pattern.test(s), pattern.exec(s))
			if (objNode.Kind() == "identifier" && t.isUntypedLocal(objText)) || t.nodeReturnsJSValue(objNode) {
				if t.builtins.IsRegexMethod(prop) {
					obj := t.transformExpr(objNode)
					if r := transformRegexpMethod(obj, prop, args, t.addImport); r != nil {
						return r
					}
				}
			}

			// Method call on a local scope variable or 'this':
			// use .MethodCall("method", args...) which auto-prepends receiver as 'this'.
			if (objNode.Kind() == "identifier" && t.isLocalName(objText)) || objNode.Kind() == "this" {
				if argsNodeHasSpread(argsNode) {
					obj := t.transformExpr(objNode)
					return t.generateMethodCallWithSpread(obj, prop, argsNode, func(e ast.Expr) ast.Expr { return t.wrapAsJSValue(e) })
				}
				t.addAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
				obj := t.transformExpr(objNode)
				for i, arg := range args {
					args[i] = t.wrapAsJSValue(arg)
				}
				allArgs := append([]ast.Expr{stringLit(prop)}, args...)
				return callExpr(selectorExpr(obj, "MethodCall"), allArgs...)
			}

			// Method call on a package-level untyped variable (JSValue):
			if objNode.Kind() == "identifier" {
				if typed, ok := t.pkgVarTyped[objText]; ok && !typed {
					if argsNodeHasSpread(argsNode) {
						obj := t.transformExpr(objNode)
						return t.generateMethodCallWithSpread(obj, prop, argsNode, func(e ast.Expr) ast.Expr { return t.wrapAsJSValue(e) })
					}
					t.addAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
					obj := t.transformExpr(objNode)
					for i, arg := range args {
						args[i] = t.wrapAsJSValue(arg)
					}
					allArgs := append([]ast.Expr{stringLit(prop)}, args...)
					return callExpr(selectorExpr(obj, "MethodCall"), allArgs...)
				}
			}

			// String/number/boolean literal as method receiver:
			// "str".method() → jsvalue.NewString("str").MethodCall("method", ...)
			if objNode.Kind() == "string" || objNode.Kind() == "template_string" ||
				objNode.Kind() == "number" || objNode.Kind() == "true" || objNode.Kind() == "false" {
				t.addAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
				obj := jsvalueWrapLit(t.transformExpr(objNode))
				for i, arg := range args {
					args[i] = t.wrapAsJSValue(arg)
				}
				allArgs := append([]ast.Expr{stringLit(prop)}, args...)
				return callExpr(selectorExpr(obj, "MethodCall"), allArgs...)
			}

			// Catch-all: method call on any JSValue-returning expression
			// (call results, new expressions, etc.) uses .MethodCall("method", args...).
			// For complex receivers (calls, chains), wrap in IIFE to evaluate once.
			if t.nodeReturnsJSValue(objNode) {
				if argsNodeHasSpread(argsNode) {
					obj := t.transformExpr(objNode)
					return t.generateMethodCallWithSpread(obj, prop, argsNode, func(e ast.Expr) ast.Expr { return t.wrapAsJSValue(e) })
				}
				t.addAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
				obj := t.transformExpr(objNode)
				for i, arg := range args {
					args[i] = t.wrapAsJSValue(arg)
				}
				allArgs := append([]ast.Expr{stringLit(prop)}, args...)
				if !isSimpleExpr(obj) {
					recv := ident("_recv")
					recvArgs := append([]ast.Expr{stringLit(prop)}, args...)
					innerCall := callExpr(selectorExpr(recv, "MethodCall"), recvArgs...)
					return callExpr(&ast.FuncLit{
						Type: &ast.FuncType{
							Params:  fieldList(field("_recv", jsValuePtrType())),
							Results: fieldList(field("", jsValuePtrType())),
						},
						Body: blockStmt(returnStmt(innerCall)),
					}, obj)
				}
				return callExpr(selectorExpr(obj, "MethodCall"), allArgs...)
			}
		}
	}

	// Bare global function calls: isNaN(), Error(), parseInt(), etc.
	if fnNode.Kind() == "identifier" {
		name := fnNode.Utf8Text(t.source)
		args := t.transformArgs(argsNode)
		if r := t.ctx.TransformGlobalCall(name, args, t); r != nil {
			t.addAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			return r
		}
	}

	// When calling a function imported from a transpiled (non-runtime) module,
	// use .Call() since all transpiled exports are *jsvalue.JSValue (NewFunction).
	if fnNode.Kind() == "identifier" {
		name := fnNode.Utf8Text(t.source)
		if imp, ok := t.importedNames[name]; ok && imp.isTranspiled && imp.goSymbol != "" {
			fun := t.transformExpr(fnNode)
			if argsNodeHasSpread(argsNode) {
				return t.generateCallWithSpread(fun, argsNode, func(e ast.Expr) ast.Expr { return t.wrapAsJSValue(e) })
			}
			t.addAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			args := t.transformArgs(argsNode)
			for i, arg := range args {
				args[i] = t.wrapAsJSValue(arg)
			}
			return callExpr(selectorExpr(fun, "Call"), args...)
		}
	}

	// All function variables (hoisted, package-level, locals) are *jsvalue.JSValue
	// in the all-JSValue architecture — use .Call() to invoke.
	if fnNode.Kind() == "identifier" {
		fnName := fnNode.Utf8Text(t.source)
		isPkgUntyped := false
		if typed, ok := t.pkgVarTyped[fnName]; ok && !typed {
			isPkgUntyped = true
		}
		if t.isUntypedLocal(fnName) || isPkgUntyped {
			fun := t.transformExpr(fnNode)
			if argsNodeHasSpread(argsNode) {
				return t.generateCallWithSpread(fun, argsNode, func(e ast.Expr) ast.Expr { return t.wrapAsJSValue(e) })
			}
			t.addAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			args := t.transformArgs(argsNode)
			for i, arg := range args {
				args[i] = t.wrapAsJSValue(arg)
			}
			return callExpr(selectorExpr(fun, "Call"), args...)
		}
	}

	// Subscript method call: this[kSym]() or obj[key]()
	// The receiver (this/obj) must be passed as the first argument.
	if fnNode.Kind() == "subscript_expression" {
		subObj := fnNode.ChildByFieldName("object")
		if subObj != nil && (subObj.Kind() == "this" || t.nodeReturnsJSValue(subObj)) {
			t.addAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			obj := t.transformExpr(subObj)
			fnExpr := t.transformExpr(fnNode) // this.Get(fmt.Sprint(key))
			args := t.transformArgs(argsNode)
			for i, arg := range args {
				args[i] = t.wrapAsJSValue(arg)
			}
			allArgs := append([]ast.Expr{obj}, args...)
			return callExpr(selectorExpr(fnExpr, "Call"), allArgs...)
		}
	}

	fun := t.transformExpr(fnNode)
	args := t.transformArgs(argsNode)

	// When calling runtime package functions (fs.ReadFileSync, path.Join, etc.),
	// wrap literal arguments with jsvalueWrapLit since all runtime functions
	// now accept *jsvalue.JSValue.
	if sel, ok := fun.(*ast.SelectorExpr); ok {
		if pkgIdent, ok := sel.X.(*ast.Ident); ok {
			if isRuntimePackage(pkgIdent.Name) {
				t.addAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
				for i, arg := range args {
					args[i] = jsvalueWrapLit(arg)
				}
			}
		}
	}

	// If the function expression is a JSValue (from .Get() chain, etc.),
	// use .Call() to invoke it instead of Go function call syntax.
	if isAlreadyJSValue(fun) {
		if argsNodeHasSpread(argsNode) {
			return t.generateCallWithSpread(fun, argsNode, jsvalueWrapLit)
		}
		t.addAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
		for i, a := range args {
			args[i] = jsvalueWrapLit(a)
		}
		return callExpr(selectorExpr(fun, "Call"), args...)
	}

	return callExpr(fun, args...)
}

// NOTE: isKnownGlobalObject has been replaced by t.ctx.IsKnownGlobal().
// All known globals are registered via registerDefaultBuiltins() in context_defaults.go.

// isObjectPrototypeHasOwnProperty checks if a tree-sitter node represents
// the pattern Object.prototype.hasOwnProperty (a 3-level member chain).
func isObjectPrototypeHasOwnProperty(node *sitter.Node, source []byte) bool {
	if node == nil || node.Kind() != "member_expression" {
		return false
	}
	// node = Object.prototype.hasOwnProperty
	prop := node.ChildByFieldName("property")
	if prop == nil || prop.Utf8Text(source) != "hasOwnProperty" {
		return false
	}
	// inner = Object.prototype
	inner := node.ChildByFieldName("object")
	if inner == nil || inner.Kind() != "member_expression" {
		return false
	}
	innerProp := inner.ChildByFieldName("property")
	if innerProp == nil || innerProp.Utf8Text(source) != "prototype" {
		return false
	}
	innerObj := inner.ChildByFieldName("object")
	if innerObj == nil {
		return false
	}
	return innerObj.Utf8Text(source) == "Object"
}

// isRuntimePackage returns true if the name is a known Gun runtime package alias.
func isRuntimePackage(name string) bool {
	switch name {
	case "fs", "nodepath", "json", "process", "module", "jserror", "jsmath":
		return true
	}
	return false
}

// spreadArgInfo tracks whether an argument came from a spread element.
type spreadArgInfo struct {
	expr       ast.Expr
	isSpread   bool
	isSlice    bool // true if expr is already []*JSValue (rest param), skip .Array()
}

// spreadToSlice converts a spread expression to []*JSValue for variadic calls.
// For *JSValue: calls .Array(). For []*JSValue (rest params): uses directly.
func spreadToSlice(info spreadArgInfo) ast.Expr {
	if info.isSlice {
		return info.expr
	}
	return callExpr(selectorExpr(info.expr, "Array"))
}

func (t *Transformer) transformArgsWithSpread(node *sitter.Node) []spreadArgInfo {
	if node == nil {
		return nil
	}
	var args []spreadArgInfo
	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		if child.Kind() == "spread_element" {
			if child.NamedChildCount() > 0 {
				innerNode := child.NamedChild(0)
				if inner := t.transformExpr(innerNode); inner != nil {
					args = append(args, spreadArgInfo{expr: inner, isSpread: true, isSlice: false})
				}
			}
		} else {
			if e := t.transformExpr(child); e != nil {
				args = append(args, spreadArgInfo{expr: e, isSpread: false})
			}
		}
	}
	return args
}

func (t *Transformer) transformArgs(node *sitter.Node) []ast.Expr {
	infos := t.transformArgsWithSpread(node)
	args := make([]ast.Expr, len(infos))
	for i, info := range infos {
		args[i] = info.expr
	}
	return args
}

// argsNodeHasSpread checks if any argument in the arguments node is a spread_element.
func argsNodeHasSpread(node *sitter.Node) bool {
	if node == nil {
		return false
	}
	for i := uint(0); i < node.NamedChildCount(); i++ {
		if node.NamedChild(i).Kind() == "spread_element" {
			return true
		}
	}
	return false
}

// generateMethodCallWithSpread generates a MethodCall/Call where spread args
// are properly expanded using .Array(). wrapFn wraps non-spread args.
func (t *Transformer) generateMethodCallWithSpread(obj ast.Expr, method string, argsNode *sitter.Node, wrapFn func(ast.Expr) ast.Expr) ast.Expr {
	t.addAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
	infos := t.transformArgsWithSpread(argsNode)

	// Build the arguments for MethodCall: first arg is always the method name string
	// For a simple case where all args are a single spread, optimize:
	// obj.MethodCall("method", arr.Array()...)
	if len(infos) == 1 && infos[0].isSpread {
		return &ast.CallExpr{
			Fun:      selectorExpr(obj, "MethodCall"),
			Args:     []ast.Expr{stringLit(method), spreadToSlice(infos[0])},
			Ellipsis: 1,
		}
	}

	// Mixed case: build a []*jsvalue.JSValue slice and pass with ...
	// append([]*jsvalue.JSValue{a, b}, spread.Array()...)
	var parts []ast.Expr
	var currentLiterals []ast.Expr

	flushLiterals := func() {
		if len(currentLiterals) > 0 {
			parts = append(parts, &ast.CompositeLit{
				Type: &ast.ArrayType{Elt: jsValuePtrType()},
				Elts: currentLiterals,
			})
			currentLiterals = nil
		}
	}

	for _, info := range infos {
		if info.isSpread {
			flushLiterals()
			parts = append(parts, spreadToSlice(info))
		} else {
			wrapped := info.expr
			if wrapFn != nil {
				wrapped = wrapFn(wrapped)
			}
			currentLiterals = append(currentLiterals, wrapped)
		}
	}
	flushLiterals()

	// Build the slice using append chains
	var sliceExpr ast.Expr
	if len(parts) == 1 {
		sliceExpr = parts[0]
	} else {
		sliceExpr = parts[0]
		for _, p := range parts[1:] {
			sliceExpr = &ast.CallExpr{
				Fun:  ident("append"),
				Args: []ast.Expr{sliceExpr, p},
				Ellipsis: 1,
			}
		}
	}

	return &ast.CallExpr{
		Fun:      selectorExpr(obj, "MethodCall"),
		Args:     []ast.Expr{stringLit(method), sliceExpr},
		Ellipsis: 1,
	}
}

// generateCallWithSpread generates a .Call() where spread args are expanded.
func (t *Transformer) generateCallWithSpread(fn ast.Expr, argsNode *sitter.Node, wrapFn func(ast.Expr) ast.Expr) ast.Expr {
	t.addAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
	infos := t.transformArgsWithSpread(argsNode)

	if len(infos) == 1 && infos[0].isSpread {
		return &ast.CallExpr{
			Fun:      selectorExpr(fn, "Call"),
			Args:     []ast.Expr{spreadToSlice(infos[0])},
			Ellipsis: 1,
		}
	}

	// Mixed case: build a []*jsvalue.JSValue slice
	var parts []ast.Expr
	var currentLiterals []ast.Expr

	flushLiterals := func() {
		if len(currentLiterals) > 0 {
			parts = append(parts, &ast.CompositeLit{
				Type: &ast.ArrayType{Elt: jsValuePtrType()},
				Elts: currentLiterals,
			})
			currentLiterals = nil
		}
	}

	for _, info := range infos {
		if info.isSpread {
			flushLiterals()
			parts = append(parts, spreadToSlice(info))
		} else {
			wrapped := info.expr
			if wrapFn != nil {
				wrapped = wrapFn(wrapped)
			}
			currentLiterals = append(currentLiterals, wrapped)
		}
	}
	flushLiterals()

	var sliceExpr ast.Expr
	if len(parts) == 1 {
		sliceExpr = parts[0]
	} else {
		sliceExpr = parts[0]
		for _, p := range parts[1:] {
			sliceExpr = &ast.CallExpr{
				Fun:  ident("append"),
				Args: []ast.Expr{sliceExpr, p},
				Ellipsis: 1,
			}
		}
	}

	return &ast.CallExpr{
		Fun:      selectorExpr(fn, "Call"),
		Args:     []ast.Expr{sliceExpr},
		Ellipsis: 1,
	}
}

func (t *Transformer) transformMemberExpr(node *sitter.Node) ast.Expr {
	objNode := node.ChildByFieldName("object")
	propNode := node.ChildByFieldName("property")

	if objNode == nil || propNode == nil {
		return ident("nil")
	}

	prop := propNode.Utf8Text(t.source)

	// Same-package namespace import: templates.foo → Foo (direct reference)
	if objNode.Kind() == "identifier" {
		name := objNode.Utf8Text(t.source)
		if imp, ok := t.importedNames[name]; ok && imp.goSymbol == "" && imp.goPkgName == "" {
			return ident(capitalize(prop))
		}
	}

	// Global object member access (e.g. process.env, process.argv)
	if objNode.Kind() == "identifier" {
		if r := t.ctx.TransformBuiltinMember(objNode.Utf8Text(t.source), prop, t); r != nil {
			return r
		}
	}

	// Error.X → jserror.Error.Get("X") for static properties
	if objNode.Kind() == "identifier" && isErrorType(objNode.Utf8Text(t.source)) {
		t.addAliasedImport("github.com/nnstd/gun/runtime/builtin/error", "jserror")
		errName := objNode.Utf8Text(t.source)
		return callExpr(selectorExpr(selectorExpr(ident("jserror"), errName), "Get"), stringLit(prop))
	}

	// process.env.X → process.Env.Get("X")
	if objNode.Kind() == "member_expression" {
		innerObj := objNode.ChildByFieldName("object")
		innerProp := objNode.ChildByFieldName("property")
		if innerObj != nil && innerProp != nil &&
			innerObj.Utf8Text(t.source) == "process" &&
			innerProp.Utf8Text(t.source) == "env" {
			t.addImport("github.com/nnstd/gun/runtime/process")
			return callExpr(selectorExpr(selectorExpr(ident("process"), "Env"), "Get"), stringLit(prop))
		}
	}

	obj := t.transformExpr(objNode)

	if prop == "length" {
		// For JSValue expressions, use Len() which returns int
		if t.nodeReturnsJSValue(objNode) {
			return callExpr(selectorExpr(obj, "Len"))
		}
		return callExpr(ident("len"), obj)
	}



	// For local scope variables and 'this' (typed as *jsvalue.JSValue),
	// use dynamic property access via Get() instead of Go selector expressions.
	if (objNode.Kind() == "identifier" && t.isLocalName(objNode.Utf8Text(t.source))) || objNode.Kind() == "this" {
		t.addAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
		return callExpr(selectorExpr(obj, "Get"), stringLit(prop))
	}

	// Package-level untyped variables default to *jsvalue.JSValue —
	// use .Get() for property access just like local JSValue vars.
	if objNode.Kind() == "identifier" {
		name := objNode.Utf8Text(t.source)
		if typed, ok := t.pkgVarTyped[name]; ok && !typed {
			t.addAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			return callExpr(selectorExpr(obj, "Get"), stringLit(prop))
		}
	}

	// If the object is a JSValue expression (from Get(), method call, etc.),
	// use .Get() for property access.
	if t.nodeReturnsJSValue(objNode) {
		t.addAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
		return callExpr(selectorExpr(obj, "Get"), stringLit(prop))
	}

	// Unknown identifiers from OTHER transpiled files in the same package.
	// These are cross-file package-level variables, all *jsvalue.JSValue.
	if objNode.Kind() == "identifier" {
		name := objNode.Utf8Text(t.source)
		_, isImported := t.importedNames[name]
		_, isOwnPkgVar := t.pkgVarTyped[name]
		isKnownGlobal := t.ctx.IsKnownGlobal(name)
		if !isImported && !isKnownGlobal && !t.isLocalName(name) && !isOwnPkgVar {
			t.addAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			return callExpr(selectorExpr(obj, "Get"), stringLit(prop))
		}
	}

	return selectorExpr(obj, capitalize(prop))
}

func (t *Transformer) transformSubscriptExpr(node *sitter.Node) ast.Expr {
	objNode := node.ChildByFieldName("object")
	obj := t.transformExpr(objNode)
	index := t.transformExpr(node.ChildByFieldName("index"))
	if obj == nil {
		return ident("nil")
	}
	if index == nil {
		return obj
	}
	// JSValue arrays can't be indexed directly; use .Index() for numeric
	// keys and .Get() for string keys.
	isJSValueObj := objNode != nil && objNode.Kind() == "identifier" && (t.isUntypedLocal(objNode.Utf8Text(t.source)) || t.jsvalueLocals[objNode.Utf8Text(t.source)])
	// Also catch call expressions that return JSValue (e.g. arg.Slice(-1)[0]).
	if !isJSValueObj && objNode != nil && t.nodeReturnsJSValue(objNode) {
		isJSValueObj = true
	}
	if isJSValueObj {
		indexNode := node.ChildByFieldName("index")
		if indexNode != nil && (indexNode.Kind() == "string" || indexNode.Kind() == "string_literal") {
			return callExpr(selectorExpr(obj, "Get"), index)
		}
		// If the index is itself a JSValue (untyped local), use .Get() with PropertyKey
		// to correctly handle Symbol keys.
		if indexNode != nil && indexNode.Kind() == "identifier" && t.isUntypedLocal(indexNode.Utf8Text(t.source)) {
			t.addAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			return callExpr(selectorExpr(obj, "Get"), callExpr(selectorExpr(ident("jsvalue"), "PropertyKey"), index))
		}
		// Typed locals could be strings (e.g. notFlagsArgv); use .Get() with fmt.Sprint.
		if indexNode != nil && indexNode.Kind() == "identifier" && t.isTypedLocal(indexNode.Utf8Text(t.source)) {
			t.addImport("fmt")
			return callExpr(selectorExpr(obj, "Get"), callExpr(selectorExpr(ident("fmt"), "Sprint"), index))
		}
		// Number literals use .Index() directly.
		if indexNode != nil && indexNode.Kind() == "number" {
			return callExpr(selectorExpr(obj, "Index"), index)
		}
		// Everything else (call expressions, etc.) — use .Get() with PropertyKey
		// in case the value is a Symbol.
		t.addAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
		return callExpr(selectorExpr(obj, "Get"), callExpr(selectorExpr(ident("jsvalue"), "PropertyKey"), index))
	}
	// Go slice indices must be integers; wrap float64 vars with int().
	indexNode := node.ChildByFieldName("index")
	if indexNode != nil && indexNode.Kind() == "identifier" {
		index = callExpr(ident("int"), index)
	}
	return &ast.IndexExpr{X: obj, Index: index}
}

func (t *Transformer) transformArrayLiteral(node *sitter.Node) ast.Expr {
	t.addAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")

	// Check if any element is a spread_element
	hasSpread := false
	for i := uint(0); i < node.NamedChildCount(); i++ {
		if node.NamedChild(i).Kind() == "spread_element" {
			hasSpread = true
			break
		}
	}

	if !hasSpread {
		// Simple case: no spread, just collect elements
		var elts []ast.Expr
		for i := uint(0); i < node.NamedChildCount(); i++ {
			if e := t.transformExpr(node.NamedChild(i)); e != nil {
				elts = append(elts, jsvalueWrapLit(e))
			}
		}
		return callExpr(selectorExpr(ident("jsvalue"), "NewArray"), elts...)
	}

	// Has spread elements: build using append + SpreadIntoArray
	// [...arr] or [...str] or [a, ...b, c]
	var parts []ast.Expr
	var currentElts []ast.Expr

	flushElts := func() {
		if len(currentElts) > 0 {
			parts = append(parts, &ast.CompositeLit{
				Type: &ast.ArrayType{Elt: jsValuePtrType()},
				Elts: currentElts,
			})
			currentElts = nil
		}
	}

	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		if child.Kind() == "spread_element" {
			flushElts()
			if child.NamedChildCount() > 0 {
				inner := t.transformExpr(child.NamedChild(0))
				if inner != nil {
					parts = append(parts, callExpr(selectorExpr(ident("jsvalue"), "SpreadIntoArray"), inner))
				}
			}
		} else {
			if e := t.transformExpr(child); e != nil {
				currentElts = append(currentElts, jsvalueWrapLit(e))
			}
		}
	}
	flushElts()

	if len(parts) == 1 {
		// Single spread: NewArray(SpreadIntoArray(x)...)
		return &ast.CallExpr{
			Fun:      selectorExpr(ident("jsvalue"), "NewArray"),
			Args:     []ast.Expr{parts[0]},
			Ellipsis: 1,
		}
	}

	// Multiple parts: append chains then NewArray(result...)
	result := parts[0]
	for _, p := range parts[1:] {
		result = &ast.CallExpr{
			Fun:      ident("append"),
			Args:     []ast.Expr{result, p},
			Ellipsis: 1,
		}
	}
	return &ast.CallExpr{
		Fun:      selectorExpr(ident("jsvalue"), "NewArray"),
		Args:     []ast.Expr{result},
		Ellipsis: 1,
	}
}

func (t *Transformer) transformObjectLiteral(node *sitter.Node) ast.Expr {
	var args []ast.Expr
	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		switch child.Kind() {
		case "pair":
			keyNode := child.ChildByFieldName("key")
			valNode := child.ChildByFieldName("value")
			if keyNode != nil && valNode != nil {
				var keyExpr ast.Expr
				if keyNode.Kind() == "computed_property_name" {
					// Computed property: { [expr]: value } → key is evaluated at runtime
					inner := keyNode.NamedChild(0)
					if inner != nil {
						keyExpr = callExpr(selectorExpr(ident("jsvalue"), "PropertyKey"), t.transformExpr(inner))
					}
				} else {
					key := keyNode.Utf8Text(t.source)
					// Strip quote characters from string-literal keys.
					// JS: { 'short-option-groups': true } — the key is short-option-groups, not 'short-option-groups'.
					if keyNode.Kind() == "string" || keyNode.Kind() == "string_fragment" {
						if len(key) >= 2 && (key[0] == '\'' || key[0] == '"') {
							key = key[1 : len(key)-1]
						}
					}
					keyExpr = stringLit(key)
				}
				if val := t.transformExpr(valNode); val != nil && keyExpr != nil {
					args = append(args, keyExpr, t.wrapAsJSValue(val))
				}
			}
		case "shorthand_property_identifier":
			name := child.Utf8Text(t.source)
			args = append(args, stringLit(name), t.wrapAsJSValue(t.resolveIdentifier(name)))
		}
	}
	t.addAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
	return callExpr(selectorExpr(ident("jsvalue"), "ObjectFrom"), args...)
}






func isStringLitNode(node *sitter.Node) bool {
	if node == nil {
		return false
	}
	if node.Kind() == "string" || node.Kind() == "template_string" {
		return true
	}
	// Chained string concat: ("a" + b) + c — binary_expression containing a string
	if node.Kind() == "binary_expression" {
		return isStringLitNode(node.ChildByFieldName("left")) || isStringLitNode(node.ChildByFieldName("right"))
	}
	return false
}

func isBoolReturningMethod(name string) bool {
	switch name {
	case "MatchString", "HasPrefix", "HasSuffix", "Contains", "ContainsAny",
		"EqualFold", "IsNaN", "IsInf":
		return true
	}
	return false
}


func isJSValueGet(expr ast.Expr) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	return sel.Sel.Name == "Get"
}

// isJSValueMethodCall returns true if the expression is a method call that
// likely returns *jsvalue.JSValue (e.g. obj.Get(), obj.Slice(), obj.Index()).
func isJSValueMethodCall(expr ast.Expr) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}

	// Check if it's a jsvalue package function
	if ident, ok := sel.X.(*ast.Ident); ok && ident.Name == "jsvalue" {
		return globalBuiltins.IsJSValuePackageFunction(sel.Sel.Name)
	}

	// Check for other JSValue methods (Get, Index, Call, MethodCall, etc.)
	switch sel.Sel.Name {
	case "Get", "Index", "Call", "MethodCall", "ToSlice":
		return true
	}
	return false
}

// ensureBool wraps a non-boolean expression so it can be used in an if/for condition.
// JSValue expressions are wrapped with .Bool() for truthiness.
// Native Go bool expressions pass through unchanged.
func (t *Transformer) ensureBool(expr ast.Expr) ast.Expr {
	if expr == nil {
		return ident("false")
	}
	// Already a native boolean expression — pass through
	switch e := expr.(type) {
	case *ast.BinaryExpr:
		switch e.Op {
		case token.EQL, token.NEQ, token.LSS, token.GTR, token.LEQ, token.GEQ, token.LAND, token.LOR:
			return expr
		}
	case *ast.UnaryExpr:
		if e.Op == token.NOT {
			return expr
		}
		if e.Op == token.XOR {
			return &ast.BinaryExpr{X: expr, Op: token.NEQ, Y: intLit("0")}
		}
	case *ast.Ident:
		if e.Name == "true" || e.Name == "false" {
			return expr
		}
		if t.isTypedLocal(e.Name) {
			if typeName, ok := t.typedLocalTypes[e.Name]; ok && typeName == "bool" {
				return expr
			}
		}
	}

	// len(x) returns int; convert to len(x) > 0.
	if call, ok := expr.(*ast.CallExpr); ok {
		if id, ok := call.Fun.(*ast.Ident); ok && id.Name == "len" {
			return &ast.BinaryExpr{X: expr, Op: token.GTR, Y: intLit("0")}
		}
	}

	// JSValue expressions → .Bool() for truthiness
	if isJSValueExpr(expr) {
		return callExpr(selectorExpr(expr, "Bool"))
	}

	// .Len() returns int, convert to > 0
	if call, ok := expr.(*ast.CallExpr); ok {
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Len" {
			return &ast.BinaryExpr{X: expr, Op: token.GTR, Y: intLit("0")}
		}
	}

	// Native Go bool-returning methods
	if call, ok := expr.(*ast.CallExpr); ok {
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
			if isBoolReturningMethod(sel.Sel.Name) {
				return expr
			}
		}
		if id, ok := call.Fun.(*ast.Ident); ok {
			if retType, ok := t.funcReturnTypes[id.Name]; ok && retType == "bool" {
				return expr
			}
		}
	}

	// For JSValue expressions, use .Bool() for proper JS truthiness.
	// nil and undefined are both falsy. Non-nil JSValue wrapping
	// false/0/"" should also be falsy.
	switch e := expr.(type) {
	case *ast.Ident:
		if e.Name == "nil" {
			return expr
		}
		if t.isTypedLocal(e.Name) {
			return &ast.BinaryExpr{X: expr, Op: token.NEQ, Y: ident("nil")}
		}
		// JSValue variable: nil check + .Bool()
		return &ast.BinaryExpr{
			X:  &ast.BinaryExpr{X: expr, Op: token.NEQ, Y: ident("nil")},
			Op: token.LAND,
			Y:  callExpr(selectorExpr(expr, "Bool")),
		}
	case *ast.SelectorExpr:
		// Property access: .Get("x") on JSValue returns *JSValue, use .Bool().
		// But package.Function (Go selector) is not JSValue.
		if sel := e.Sel.Name; sel == "Get" || sel == "Bool" || sel == "Len" {
			return callExpr(selectorExpr(expr, "Bool"))
		}
		return &ast.BinaryExpr{X: expr, Op: token.NEQ, Y: ident("nil")}
	case *ast.IndexExpr:
		// Subscript access — result could be JSValue
		return callExpr(selectorExpr(expr, "Bool"))
	case *ast.CallExpr:
		// Function/method call result — check if it returns JSValue
		if isJSValueCallExpr(e) {
			return callExpr(selectorExpr(expr, "Bool"))
		}
		return &ast.BinaryExpr{X: expr, Op: token.NEQ, Y: ident("nil")}
	}

	return expr
}

// isJSValueCallExpr checks if a Go AST call expression returns *jsvalue.JSValue.
// Used to decide whether .Bool() is safe to call on the result.
func isJSValueCallExpr(call *ast.CallExpr) bool {
	// obj.Get("x"), obj.MethodCall("x"), obj.Call(...), obj.Index(n)
	if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
		switch sel.Sel.Name {
		case "Get", "MethodCall", "Call", "Index", "GetPrototype":
			return true
		}
		// jsvalue.* package functions
		if id, ok := sel.X.(*ast.Ident); ok && id.Name == "jsvalue" {
			return true
		}
	}
	// IIFE: func(...) *jsvalue.JSValue { ... }(args)
	if fnLit, ok := call.Fun.(*ast.FuncLit); ok {
		if fnLit.Type.Results != nil && len(fnLit.Type.Results.List) > 0 {
			if isJSValuePtrType(fnLit.Type.Results.List[0].Type) {
				return true
			}
		}
	}
	return false
}

// isJSValueExpr returns true if the Go AST expression is known to produce *jsvalue.JSValue.
func isJSValueExpr(expr ast.Expr) bool {
	if isJSValueMethodCall(expr) {
		return true
	}
	if isJSValueGet(expr) {
		return true
	}
	// IIFE (immediately invoked function expression) that returns *jsvalue.JSValue
	if call, ok := expr.(*ast.CallExpr); ok {
		if fnLit, ok := call.Fun.(*ast.FuncLit); ok {
			if fnLit.Type.Results != nil && len(fnLit.Type.Results.List) == 1 {
				if isJSValuePtrType(fnLit.Type.Results.List[0].Type) {
					return true
				}
			}
		}
	}
	// Identifiers that look like JSValue (checked by caller context)
	return false
}



// wrapAsJSValue wraps an expression with jsvalue.From() so it can be used
// as a *jsvalue.JSValue value in map literals. Expressions that are already
// jsvalue constructor calls are returned as-is.
func (t *Transformer) wrapAsJSValue(expr ast.Expr) ast.Expr {
	if isJSValueExpr(expr) {
		return expr
	}
	t.addAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
	return callExpr(selectorExpr(ident("jsvalue"), "From"), expr)
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
		case "escape_sequence":
			// Template string escape sequences → their literal values
			text := child.Utf8Text(t.source)
			switch text {
			case `\$`:
				formatParts = append(formatParts, "$")
			case `\n`:
				formatParts = append(formatParts, "\n")
			case `\t`:
				formatParts = append(formatParts, "\t")
			case `\\`:
				formatParts = append(formatParts, `\`)
			case `\"`:
				formatParts = append(formatParts, `"`)
			case `\'`:
				formatParts = append(formatParts, `'`)
			case "\\`":
				formatParts = append(formatParts, "`")
			default:
				// Strip the leading backslash for unknown escapes
				if len(text) > 1 && text[0] == '\\' {
					formatParts = append(formatParts, text[1:])
				} else {
					formatParts = append(formatParts, text)
				}
			}
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

	// No interpolations — return plain string literal
	if len(args) == 0 {
		// Use backtick raw string if it contains newlines (Go "" can't have newlines)
		if strings.Contains(format, "\n") && !strings.Contains(format, "`") {
			// Undo the \" escaping for raw strings (backtick strings are raw)
			raw := strings.ReplaceAll(format, `\"`, `"`)
			return basicLit(token.STRING, "`"+raw+"`")
		}
		return basicLit(token.STRING, `"`+format+`"`)
	}

	t.addImport("fmt")
	// Escape real newlines/tabs as Go escape sequences.
	// Also escape literal backslashes that aren't already part of Go escapes.
	var escaped strings.Builder
	for i := 0; i < len(format); i++ {
		ch := format[i]
		switch ch {
		case '\n':
			escaped.WriteString(`\n`)
		case '\t':
			escaped.WriteString(`\t`)
		case '\r':
			escaped.WriteString(`\r`)
		case '\\':
			// Check if already a valid Go escape sequence
			if i+1 < len(format) {
				next := format[i+1]
				if next == 'n' || next == 't' || next == 'r' || next == '"' || next == '\\' || next == 'u' || next == 'x' {
					escaped.WriteByte(ch)
					continue
				}
			}
			escaped.WriteString(`\\`)
		default:
			escaped.WriteByte(ch)
		}
	}
	format = escaped.String()
	// Escape literal % signs so they don't get interpreted by Sprintf
	format = strings.ReplaceAll(format, "%", "%%")
	// Restore %v placeholders (they were added as "%v" but we escaped all %)
	for range args {
		format = strings.Replace(format, "%%v", "%v", 1)
	}
	allArgs := append([]ast.Expr{stringLit(format)}, args...)
	return callExpr(selectorExpr(ident("fmt"), "Sprintf"), allArgs...)
}

func (t *Transformer) transformTernary(node *sitter.Node) ast.Expr {
	consNode := node.ChildByFieldName("consequence")
	altNode := node.ChildByFieldName("alternative")
	cond := t.ensureBool(t.transformExpr(node.ChildByFieldName("condition")))
	cons := t.transformExpr(consNode)
	alt := t.transformExpr(altNode)

	if cond == nil || cons == nil || alt == nil {
		return ident("nil")
	}

	// Try to infer a concrete return type so the IIFE doesn't return `any`.
	resultType := t.inferTernaryResultType(consNode, altNode)

	// Coerce branches whose type doesn't match the inferred result type.
	if isJSValuePtrType(resultType) {
		// JSValue result: wrap both branches with jsvalueWrapLit
		t.addAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
		cons = jsvalueWrapLit(cons)
		alt = jsvalueWrapLit(alt)
	} else if id, ok := resultType.(*ast.Ident); ok {
		switch id.Name {
		case "string":
			if t.inferNodeResultType(consNode) == "" {
				t.addImport("fmt")
				if id, ok := cons.(*ast.Ident); ok && id.Name == "nil" {
					cons = stringLit("")
				} else {
					cons = callExpr(selectorExpr(ident("fmt"), "Sprint"), cons)
				}
			}
			if t.inferNodeResultType(altNode) == "" {
				t.addImport("fmt")
				if id, ok := alt.(*ast.Ident); ok && id.Name == "nil" {
					alt = stringLit("")
				} else {
					alt = callExpr(selectorExpr(ident("fmt"), "Sprint"), alt)
				}
			}
		case "float64":
			if t.inferNodeResultType(consNode) == "" {
				cons = callExpr(selectorExpr(cons, "Number"))
			}
			if t.inferNodeResultType(altNode) == "" {
				alt = callExpr(selectorExpr(alt, "Number"))
			}
		case "int":
			if t.inferNodeResultType(consNode) == "" {
				cons = callExpr(ident("int"), callExpr(selectorExpr(cons, "Number")))
			}
			if t.inferNodeResultType(altNode) == "" {
				alt = callExpr(ident("int"), callExpr(selectorExpr(alt, "Number")))
			}
		}
	}

	return &ast.CallExpr{
		Fun: &ast.FuncLit{
			Type: &ast.FuncType{
				Params:  fieldList(),
				Results: fieldList(field("", resultType)),
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

// inferTernaryResultType returns a Go type expression for the IIFE wrapping a
// ternary. When either branch involves JSValue, returns *jsvalue.JSValue.
// When both branches clearly produce the same native type, that type is used;
// otherwise falls back to `any`.
func (t *Transformer) inferTernaryResultType(cons, alt *sitter.Node) ast.Expr {
	// If either branch returns JSValue, the whole ternary should return JSValue.
	if t.nodeReturnsJSValue(cons) || t.nodeReturnsJSValue(alt) {
		t.addAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
		return jsValuePtrType()
	}
	a := t.inferNodeResultType(cons)
	b := t.inferNodeResultType(alt)
	if a != "" && a == b {
		return ident(a)
	}
	// int and float64 are compatible numeric types; prefer float64.
	if (a == "int" && b == "float64") || (a == "float64" && b == "int") {
		return ident("float64")
	}
	if a != "" && b == "" && isGoTypeName(a) {
		return ident(a)
	}
	if b != "" && a == "" && isGoTypeName(b) {
		return ident(b)
	}
	return ident("any")
}

func (t *Transformer) inferNodeResultType(node *sitter.Node) string {
	if node == nil {
		return ""
	}
	switch node.Kind() {
	case "number":
		return "int"
	case "call_expression":
		fn := node.ChildByFieldName("function")
		if fn != nil && fn.Kind() == "identifier" && fn.Utf8Text(t.source) == "Number" {
			return "float64"
		}
	case "string", "template_string":
		return "string"
	case "true", "false":
		return "bool"
	case "array":
		return "array"
	case "unary_expression":
		// -1, +1 etc. are numeric
		op := node.ChildByFieldName("operator")
		arg := node.ChildByFieldName("argument")
		if op != nil && arg != nil {
			opText := op.Utf8Text(t.source)
			if (opText == "-" || opText == "+") && arg.Kind() == "number" {
				return "int"
			}
		}
	case "ternary_expression":
		// Infer from branches of nested ternary
		cons := node.ChildByFieldName("consequence")
		alt := node.ChildByFieldName("alternative")
		a := t.inferNodeResultType(cons)
		b := t.inferNodeResultType(alt)
		if a != "" && a == b {
			return a
		}
		if a != "" {
			return a
		}
		if b != "" {
			return b
		}
	case "parenthesized_expression":
		if node.NamedChildCount() > 0 {
			return t.inferNodeResultType(node.NamedChild(0))
		}
	case "binary_expression":
		if isStringLitNode(node) {
			return "string"
		}
	case "member_expression":
		prop := node.ChildByFieldName("property")
		if prop != nil && prop.Utf8Text(t.source) == "length" {
			return "int"
		}
	}
	return ""
}

func (t *Transformer) transformArrowFunc(node *sitter.Node) ast.Expr {
	paramsNode := node.ChildByFieldName("parameters")
	bodyNode := node.ChildByFieldName("body")
	returnTypeNode := node.ChildByFieldName("return_type")
	paramNode := node.ChildByFieldName("parameter")

	var params *ast.FieldList
	var paramStmts []ast.Stmt
	var paramNames []string
	if paramsNode != nil {
		params, paramStmts = t.transformParams(paramsNode)
		paramNames = extractParamNames(paramsNode, t.source)
	} else if paramNode != nil {
		pName := sanitizeIdent(paramNode.Utf8Text(t.source))
		params = fieldList(field(pName, ptrType(selectorExpr(ident("jsvalue"), "JSValue"))))
		t.addAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
		paramNames = []string{pName}
	} else {
		params = fieldList()
	}

	// Push a typed scope so isUntypedLocal works correctly inside the body.
	if paramsNode != nil {
		t.pushTypedScope(extractParamInfo(paramsNode, t.source))
	} else {
		t.pushScope(paramNames)
	}
	defer t.popScope()

	// All-JSValue: ignore return type annotations, determined by body content
	_ = returnTypeNode

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

	var results *ast.FieldList
	if hasReturnValue(body) {
		results = fieldList(field("", ptrType(selectorExpr(ident("jsvalue"), "JSValue"))))
		t.addAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
		wrapReturnsWithJSValue(body)
	}

	ensureTrailingReturn(body, results)

	fnLit := &ast.FuncLit{
		Type: &ast.FuncType{Params: params, Results: results},
		Body: body,
	}

	// Wrap in jsvalue.NewFunction for all-JSValue consistency.
	// Convert to variadic form: func(_args ...*jsvalue.JSValue) *jsvalue.JSValue
	return t.wrapFuncLitAsJSValue(fnLit, paramNames)
}

func (t *Transformer) transformFuncExpr(node *sitter.Node) ast.Expr {
	paramsNode := node.ChildByFieldName("parameters")
	bodyNode := node.ChildByFieldName("body")

	// Check if the function body uses 'this' (not arrow functions — they don't bind 'this')
	usesThis := nodeUsesThis(bodyNode)
	if usesThis {
		t.addToCurrentScope("this", false)
		t.jsvalueLocals["this"] = true
	}

	params, paramStmts := t.transformParams(paramsNode)

	// Push a typed scope so isUntypedLocal works correctly inside the body.
	t.pushTypedScope(extractParamInfo(paramsNode, t.source))
	defer t.popScope()

	var results *ast.FieldList
	if rtn := node.ChildByFieldName("return_type"); rtn != nil {
		if retType := t.getTypeAnnotation(rtn); retType != nil {
			results = fieldList(field("", retType))
		}
	}

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
		if hasReturnValue(body) {
			results = fieldList(field("", ptrType(selectorExpr(ident("jsvalue"), "JSValue"))))
			t.addAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
			wrapReturnsWithJSValue(body)
		}
	}

	ensureTrailingReturn(body, results)

	// If the function uses 'this', add it as the first parameter so the
	// wrapper extracts it from _args[0] (matching .Call(receiver, args...) convention)
	if usesThis {
		thisField := field("this", ptrType(selectorExpr(ident("jsvalue"), "JSValue")))
		if params == nil {
			params = fieldList(thisField)
		} else {
			params.List = append([]*ast.Field{thisField}, params.List...)
		}
	}

	fnLit := &ast.FuncLit{
		Type: &ast.FuncType{Params: params, Results: results},
		Body: body,
	}

	paramNames := extractParamNames(paramsNode, t.source)
	if usesThis {
		paramNames = append([]string{"this"}, paramNames...)
	}
	return t.wrapFuncLitAsJSValue(fnLit, paramNames)
}

func (t *Transformer) transformNewExpr(node *sitter.Node) ast.Expr {
	ctorNode := node.ChildByFieldName("constructor")
	argsNode := node.ChildByFieldName("arguments")

	if ctorNode == nil {
		return ident("nil")
	}

	name := ctorNode.Utf8Text(t.source)

	// Handle member_expression constructors like new Intl.Segmenter()
	// Flatten "Intl.Segmenter" → "IntlSegmenter" for builtin lookup and fallback
	if ctorNode.Kind() == "member_expression" {
		objNode := ctorNode.ChildByFieldName("object")
		propNode := ctorNode.ChildByFieldName("property")
		if objNode != nil && propNode != nil {
			name = capitalize(objNode.Utf8Text(t.source)) + capitalize(propNode.Utf8Text(t.source))
		}
	}

	args := t.transformArgs(argsNode)

	// Try builtin new expressions first
	if r := t.ctx.TransformBuiltinNew(name, args, t); r != nil {
		return r
	}

	// Default: new Foo(args) → Foo.Call(args...)
	// Classes are JSValue constructor functions; .Call() creates an instance.
	t.addAliasedImport("github.com/nnstd/gun/runtime/builtin", "jsvalue")
	for i, arg := range args {
		args[i] = t.wrapAsJSValue(arg)
	}
	ctor := t.transformExpr(ctorNode)
	return callExpr(selectorExpr(ctor, "Call"), args...)
}

// inferModuleType walks a tree-sitter node to determine if it originates from
// a known module package. This handles chained calls like yargs.Default(args).command(...)
// where the object of .command() is a call_expression rather than a simple identifier.
func (t *Transformer) inferModuleType(node *sitter.Node) string {
	if node == nil {
		return ""
	}
	switch node.Kind() {
	case "identifier":
		name := node.Utf8Text(t.source)
		if modType, ok := t.varTypes[name]; ok {
			return modType
		}
		if imp, ok := t.importedNames[name]; ok {
			return imp.goPkgName
		}
	case "call_expression":
		fnNode := node.ChildByFieldName("function")
		return t.inferModuleType(fnNode)
	case "member_expression":
		objNode := node.ChildByFieldName("object")
		if objNode == nil {
			return ""
		}
		if objNode.Kind() == "identifier" {
			name := objNode.Utf8Text(t.source)
			if modType, ok := t.varTypes[name]; ok {
				return modType
			}
			if imp, ok := t.importedNames[name]; ok {
				return imp.goPkgName
			}
		}
		return t.inferModuleType(objNode)
	}
	return ""
}
