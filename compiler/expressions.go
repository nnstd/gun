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

	case "null", "undefined":
		return ident("nil")

	case "meta_property":
		// import.meta → module.ImportMeta
		t.addImport("github.com/nnstd/gun/runtime/module")
		return selectorExpr(ident("module"), "ImportMeta")

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

	op := mapBinaryOp(opText)

	// Check if either operand is a JSValue expression.
	leftIsJSValue := t.nodeReturnsJSValue(leftNode)
	rightIsJSValue := t.nodeReturnsJSValue(rightNode)
	eitherJSValue := leftIsJSValue || rightIsJSValue || t.isPkgLevelVar(leftNode) || t.isPkgLevelVar(rightNode)

	// ?? (nullish coalescing) → jsvalue.Nullish(left, right)
	if opText == "??" {
		if eitherJSValue {
			t.addAliasedImport("github.com/nnstd/gun/runtime/jsvalue", "jsvalue")
			return callExpr(selectorExpr(ident("jsvalue"), "Nullish"),
				jsvalueWrapLit(left), jsvalueWrapLit(right))
		}
		return left
	}

	// instanceof → != nil
	if opText == "instanceof" {
		return &ast.BinaryExpr{X: left, Op: token.NEQ, Y: ident("nil")}
	}

	// When either operand is JSValue, use jsvalue operation helpers.
	// This eliminates complex type coercion — the helpers handle type semantics internally.
	if eitherJSValue {
		t.addAliasedImport("github.com/nnstd/gun/runtime/jsvalue", "jsvalue")
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
		// !str.match(regex) / !str.exec(regex) → FindStringSubmatch(...) == nil
		// These return []string in Go, not bool, so ! must become == nil.
		if argNode != nil && argNode.Kind() == "call_expression" {
			fnNode := argNode.ChildByFieldName("function")
			if fnNode != nil && fnNode.Kind() == "member_expression" {
				propNode := fnNode.ChildByFieldName("property")
				if propNode != nil {
					prop := propNode.Utf8Text(t.source)
					if prop == "match" || prop == "exec" {
						return &ast.BinaryExpr{X: arg, Op: token.EQL, Y: ident("nil")}
					}
				}
			}
		}
		// !jsValue → jsvalue.Not(jsValue) — returns *JSValue boolean
		if argNode != nil && t.nodeReturnsJSValue(argNode) {
			t.addAliasedImport("github.com/nnstd/gun/runtime/jsvalue", "jsvalue")
			return callExpr(selectorExpr(ident("jsvalue"), "Not"), arg)
		}
		return &ast.UnaryExpr{Op: token.NOT, X: arg}
	case "-":
		// -jsValue → jsvalue.Neg(jsValue)
		if argNode != nil && t.nodeReturnsJSValue(argNode) {
			t.addAliasedImport("github.com/nnstd/gun/runtime/jsvalue", "jsvalue")
			return callExpr(selectorExpr(ident("jsvalue"), "Neg"), arg)
		}
		return &ast.UnaryExpr{Op: token.SUB, X: arg}
	case "+":
		return arg
	case "~":
		// ~jsValue → jsvalue.BitNot(jsValue)
		if argNode != nil && t.nodeReturnsJSValue(argNode) {
			t.addAliasedImport("github.com/nnstd/gun/runtime/jsvalue", "jsvalue")
			return callExpr(selectorExpr(ident("jsvalue"), "BitNot"), arg)
		}
		return &ast.UnaryExpr{Op: token.XOR, X: arg}
	case "typeof":
		// typeof jsValue → jsvalue.TypeOf(jsValue)
		if argNode != nil && t.nodeReturnsJSValue(argNode) {
			t.addAliasedImport("github.com/nnstd/gun/runtime/jsvalue", "jsvalue")
			return callExpr(selectorExpr(ident("jsvalue"), "TypeOf"), arg)
		}
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
	// Use *jsvalue.JSValue return type when the target is an untyped variable
	// (package-level var or untyped param), since those default to *jsvalue.JSValue.
	var retType ast.Expr = ident("any")
	assignRHS := right
	retExpr := right
	if leftNode := node.ChildByFieldName("left"); leftNode != nil && leftNode.Kind() == "identifier" {
		name := leftNode.Utf8Text(t.source)
		if t.isUntypedLocal(name) || !t.isLocalName(name) {
			retType = jsValuePtrType()
			t.addAliasedImport("github.com/nnstd/gun/runtime/jsvalue", "jsvalue")
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
					t.addAliasedImport("github.com/nnstd/gun/runtime/jsvalue", "jsvalue")
					return callExpr(selectorExpr(ident("jsvalue"), "NewBool"),
						callExpr(selectorExpr(args[0], "HasOwnProperty"), callExpr(selectorExpr(ident("fmt"), "Sprint"), args[1])))
				}
				// Generic .call(): fn.call(thisArg, ...args) → fn.Call(thisArg, ...args)
				obj := t.transformExpr(objNode)
				for i, a := range args {
					args[i] = jsvalueWrapLit(a)
				}
				t.addAliasedImport("github.com/nnstd/gun/runtime/jsvalue", "jsvalue")
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
			if r := transformBuiltinCall(objText, prop, args, t.addImport); r != nil {
				return r
			}

			// Method transforms on arbitrary receivers (string/collection methods)
			// Skip if the object is a namespace import (it's a package, not a value)
			isUntypedLocal := objNode.Kind() == "identifier" && t.isUntypedLocal(objText)
			if _, isNsImport := t.importedNames[objText]; !isNsImport {
				obj := t.transformExpr(objNode)

				// When the receiver returns JSValue (untyped local or JSValue-returning expression) and the method is a regex method,
				// call the package-level function (e.g., pattern.test(s) → jsvalue.MatchString(pattern, s))
				if (isUntypedLocal || t.nodeReturnsJSValue(objNode)) && t.builtins.IsRegexMethod(prop) {
					if r := transformRegexpMethod(obj, prop, args, t.addImport); r != nil {
						return r
					}
				}

				// Map/Set method dispatch for known Map/Set locals
				if objNode.Kind() == "identifier" {
					if msType, ok := t.mapSetLocals[objText]; ok {
						if msType == "map" {
							if r := transformMapMethod(obj, prop, args, t.addImport); r != nil {
								return r
							}
						} else if msType == "set" {
							if r := transformSetMethod(obj, prop, args, t.addImport); r != nil {
								return r
							}
						}
					}
				}

				// For JSValue receivers, try JSValue string wrapper methods first (before fmt.Sprint coercion).
				// Only for methods that have dedicated jsvalue.* wrappers.
				if (isUntypedLocal || t.nodeReturnsJSValue(objNode)) && hasJSValueStringWrapper(prop) {
					if r := transformStringMethod(obj, prop, args, t.addImport, true); r != nil {
						return r
					}
				}

				// When the receiver returns JSValue (untyped local or JSValue-returning expression),
				// coerce it to string for string methods, but not for array methods.
				// Wrap the result in JSValue to maintain type consistency.
				if (isUntypedLocal || t.nodeReturnsJSValue(objNode)) && !t.builtins.IsArrayMethod(prop) && !t.builtins.IsRegexMethod(prop) {
					t.addImport("fmt")
					t.addAliasedImport("github.com/nnstd/gun/runtime/jsvalue", "jsvalue")
					obj = callExpr(selectorExpr(ident("fmt"), "Sprint"), obj)
					coercedArgs := make([]ast.Expr, len(args))
					copy(coercedArgs, args)
					for i, arg := range coercedArgs {
						// Skip coercion for arguments that should remain as their original types
						if shouldCoerceArg(prop, i, arg) {
							coercedArgs[i] = callExpr(selectorExpr(ident("fmt"), "Sprint"), arg)
						}
					}
					if r := transformBuiltinMethod(obj, prop, coercedArgs, t.addImport); r != nil {
						// Wrap result in JSValue based on return type
						switch prop {
						case "split":
							// split returns []string → wrap with FromStrings
							return callExpr(selectorExpr(ident("jsvalue"), "FromStrings"), r)
						case "indexOf", "lastIndexOf", "search":
							// These return int → wrap with NewNumber
							return callExpr(selectorExpr(ident("jsvalue"), "NewNumber"), callExpr(ident("float64"), r))
						case "startsWith", "endsWith", "includes":
							// These return bool → wrap with NewBool
							return callExpr(selectorExpr(ident("jsvalue"), "NewBool"), r)
						case "match":
							// match returns special handling - might already be wrapped
							return r
						default:
							// String methods (charAt, toLowerCase, toUpperCase, trim, replace, etc.) → wrap with NewString
							return callExpr(selectorExpr(ident("jsvalue"), "NewString"), r)
						}
					}
				} else if (isUntypedLocal || t.nodeReturnsJSValue(objNode)) && t.builtins.IsArrayMethod(prop) {
					// For array methods on JSValue receivers (untyped locals or JSValue-returning expressions), apply JSValue coercion
					t.addAliasedImport("github.com/nnstd/gun/runtime/jsvalue", "jsvalue")

					// Handle map/filter/forEach with package-level functions
					if prop == "map" || prop == "filter" || prop == "forEach" ||
						prop == "find" || prop == "some" || prop == "every" || prop == "reduce" {
						funcName := capitalize(prop)
						return callExpr(selectorExpr(ident("jsvalue"), funcName), append([]ast.Expr{obj}, args...)...)
					}

					// Handle length with array coercion
					if prop == "length" {
						return callExpr(ident("len"), callExpr(selectorExpr(obj, "Array")))
					}

					// For other array methods, call transformCollectionMethod with isJSValueReceiver=true
					if r := transformCollectionMethod(obj, prop, args, t.addImport, true); r != nil {
						return r
					}
				} else {
					// [].concat(x) → just x; skip coercion so JSValue args stay as-is.
					if prop == "concat" {
						if cl, ok := obj.(*ast.CompositeLit); ok && len(cl.Elts) == 0 && len(args) > 0 {
							return args[0]
						}
					}
					coercedArgs := t.coerceJSValueArgs(args, argsNode)
					// []*jsvalue.JSValue slice locals dispatch through package-level functions.
					// Runtime accepts any for array param, handling []*JSValue internally.
					if objNode.Kind() == "identifier" && t.jsvalueSliceLocals[objText] && t.builtins.IsArrayMethod(prop) {
						t.addAliasedImport("github.com/nnstd/gun/runtime/jsvalue", "jsvalue")
						for k, a := range coercedArgs {
							coercedArgs[k] = jsvalueWrapLit(a)
						}
						funcName := capitalize(prop)
						return callExpr(selectorExpr(ident("jsvalue"), funcName), append([]ast.Expr{obj}, coercedArgs...)...)
					}
					if r := transformBuiltinMethod(obj, prop, coercedArgs, t.addImport); r != nil {
						return r
					}
				}
			}

			// Method call on a local scope variable (JSValue parameter):
			// use selector form so it compiles as obj.Method(args)
			if objNode.Kind() == "identifier" && t.isLocalName(objText) {
				obj := t.transformExpr(objNode)
				// JSValue slice locals ([]*jsvalue.JSValue) don't have JSValue methods;
				// wrap with jsvalue.NewArray() to convert to *jsvalue.JSValue first.

				return callExpr(selectorExpr(obj, capitalize(prop)), args...)
			}

			// Method call on a package-level untyped variable (JSValue):
			// use .Get("method").Call(args...) for dynamic dispatch.
			if objNode.Kind() == "identifier" {
				if typed, ok := t.pkgVarTyped[objText]; ok && !typed {
					t.addAliasedImport("github.com/nnstd/gun/runtime/jsvalue", "jsvalue")
					obj := t.transformExpr(objNode)
					for i, arg := range args {
						args[i] = t.wrapAsJSValue(arg)
					}
					return callExpr(selectorExpr(callExpr(selectorExpr(obj, "Get"), stringLit(prop)), "Call"), args...)
				}
			}
		}
	}

	// Bare global function calls: isNaN(), Error(), parseInt(), etc.
	if fnNode.Kind() == "identifier" {
		name := fnNode.Utf8Text(t.source)
		args := t.transformArgs(argsNode)
		if r := transformGlobalCall(name, args, t.addImport); r != nil {
			t.addAliasedImport("github.com/nnstd/gun/runtime/jsvalue", "jsvalue")
			return r
		}
	}

	// When calling a function imported from a transpiled (non-runtime) module,
	// wrap each argument with jsvalue.From() since all params are *jsvalue.JSValue.
	if fnNode.Kind() == "identifier" {
		name := fnNode.Utf8Text(t.source)
		if imp, ok := t.importedNames[name]; ok && imp.isTranspiled && imp.goSymbol != "" {
			fun := t.transformExpr(fnNode)
			args := t.transformArgs(argsNode)
			for i, arg := range args {
				args[i] = t.wrapAsJSValue(arg)
			}
			return callExpr(fun, args...)
		}
	}

	// When calling a local function variable whose params are all *jsvalue.JSValue
	// (untyped hoisted function), wrap non-JSValue arguments with jsvalue.From().
	// Pad with nil if fewer args than params (JS allows omitting trailing args).
	if fnNode.Kind() == "identifier" {
		fnName := fnNode.Utf8Text(t.source)
		_, inParamCounts := t.funcParamCounts[fnName]
		if inParamCounts {
			fun := t.transformExpr(fnNode)
			args := t.transformArgs(argsNode)
			for i, arg := range args {
				args[i] = t.wrapAsJSValue(arg)
			}
			if expected, ok := t.funcParamCounts[fnName]; ok && len(args) < expected {
				for len(args) < expected {
					args = append(args, ident("nil"))
				}
			}
			return callExpr(fun, args...)
		}
		// Untyped locals/pkg vars that are NOT hoisted functions hold *jsvalue.JSValue
		// which may be a function reference — use .Call() to invoke.
		isPkgUntyped := false
		if typed, ok := t.pkgVarTyped[fnName]; ok && !typed {
			isPkgUntyped = true
		}
		if t.isUntypedLocal(fnName) || isPkgUntyped {
			t.addAliasedImport("github.com/nnstd/gun/runtime/jsvalue", "jsvalue")
			fun := t.transformExpr(fnNode)
			args := t.transformArgs(argsNode)
			for i, arg := range args {
				args[i] = t.wrapAsJSValue(arg)
			}
			return callExpr(selectorExpr(fun, "Call"), args...)
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
				t.addAliasedImport("github.com/nnstd/gun/runtime/jsvalue", "jsvalue")
				for i, arg := range args {
					args[i] = jsvalueWrapLit(arg)
				}
			}
		}
	}

	// If the function expression is a JSValue (from .Get() chain, etc.),
	// use .Call() to invoke it instead of Go function call syntax.
	if isAlreadyJSValue(fun) {
		t.addAliasedImport("github.com/nnstd/gun/runtime/jsvalue", "jsvalue")
		for i, a := range args {
			args[i] = jsvalueWrapLit(a)
		}
		return callExpr(selectorExpr(fun, "Call"), args...)
	}

	return callExpr(fun, args...)
}

// isKnownGlobalObject returns true if the name is a known JS/Go global object
// (console, Math, JSON, Object, process, etc.) that should NOT get .Get() dispatch.
func isKnownGlobalObject(name string) bool {
	switch name {
	case "console", "Math", "JSON", "Object", "Array", "Number", "Boolean",
		"Error", "TypeError", "RangeError", "Date", "RegExp", "Symbol",
		"process", "module", "require", "globalThis",
		"undefined", "null", "NaN", "Infinity":
		return true
	}
	return false
}

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
	case "fs", "nodepath", "json", "process", "module":
		return true
	}
	return false
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

	prop := propNode.Utf8Text(t.source)

	// Same-package namespace import: use .Get() for JSValue object property access.
	// This handles cross-file variable references like DefaultValuesForTypeKey.BOOLEAN.
	if objNode.Kind() == "identifier" {
		name := objNode.Utf8Text(t.source)
		if imp, ok := t.importedNames[name]; ok && imp.goSymbol == "" && imp.goPkgName == "" {
			obj := t.transformExpr(objNode)
			t.addAliasedImport("github.com/nnstd/gun/runtime/jsvalue", "jsvalue")
			return callExpr(selectorExpr(obj, "Get"), stringLit(prop))
		}
	}

	// process.X → Go equivalents
	if objNode.Kind() == "identifier" && objNode.Utf8Text(t.source) == "process" {
		if r := transformProcessMember(prop, t.addImport); r != nil {
			return r
		}
	}

	// process.env.X → process.Env["X"]
	if objNode.Kind() == "member_expression" {
		innerObj := objNode.ChildByFieldName("object")
		innerProp := objNode.ChildByFieldName("property")
		if innerObj != nil && innerProp != nil &&
			innerObj.Utf8Text(t.source) == "process" &&
			innerProp.Utf8Text(t.source) == "env" {
			t.addImport("github.com/nnstd/gun/runtime/process")
			return &ast.IndexExpr{X: selectorExpr(ident("process"), "Env"), Index: stringLit(prop)}
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

	// Map/Set .size property
	if prop == "size" && objNode.Kind() == "identifier" {
		if msType, ok := t.mapSetLocals[objNode.Utf8Text(t.source)]; ok {
			t.addAliasedImport("github.com/nnstd/gun/runtime/jsvalue", "jsvalue")
			if msType == "map" {
				return callExpr(selectorExpr(ident("jsvalue"), "MapSize"), obj)
			}
			return callExpr(selectorExpr(ident("jsvalue"), "SetSize"), obj)
		}
	}

	// For local scope variables (function parameters typed as *jsvalue.JSValue),
	// use dynamic property access via Get() instead of Go selector expressions.
	if objNode.Kind() == "identifier" && t.isLocalName(objNode.Utf8Text(t.source)) {
		t.addAliasedImport("github.com/nnstd/gun/runtime/jsvalue", "jsvalue")
		return callExpr(selectorExpr(obj, "Get"), stringLit(prop))
	}

	// Package-level untyped variables default to *jsvalue.JSValue —
	// use .Get() for property access just like local JSValue vars.
	if objNode.Kind() == "identifier" {
		name := objNode.Utf8Text(t.source)
		if typed, ok := t.pkgVarTyped[name]; ok && !typed {
			t.addAliasedImport("github.com/nnstd/gun/runtime/jsvalue", "jsvalue")
			return callExpr(selectorExpr(obj, "Get"), stringLit(prop))
		}
	}

	// If the object is a JSValue expression (from Get(), method call, etc.),
	// use .Get() for property access.
	if t.nodeReturnsJSValue(objNode) {
		t.addAliasedImport("github.com/nnstd/gun/runtime/jsvalue", "jsvalue")
		return callExpr(selectorExpr(obj, "Get"), stringLit(prop))
	}

	// Unknown identifiers from OTHER transpiled files in the same package.
	// These are cross-file package-level variables, all *jsvalue.JSValue.
	if objNode.Kind() == "identifier" {
		name := objNode.Utf8Text(t.source)
		_, isImported := t.importedNames[name]
		_, isOwnPkgVar := t.pkgVarTyped[name]
		isKnownGlobal := isKnownGlobalObject(name)
		if !isImported && !isKnownGlobal && !t.isLocalName(name) && !isOwnPkgVar {
			t.addAliasedImport("github.com/nnstd/gun/runtime/jsvalue", "jsvalue")
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
	isJSValueObj := objNode != nil && objNode.Kind() == "identifier" && t.isUntypedLocal(objNode.Utf8Text(t.source))
	// Also catch call expressions that return JSValue (e.g. arg.Slice(-1)[0]).
	if !isJSValueObj && objNode != nil && t.nodeReturnsJSValue(objNode) {
		isJSValueObj = true
	}
	if isJSValueObj {
		indexNode := node.ChildByFieldName("index")
		if indexNode != nil && (indexNode.Kind() == "string" || indexNode.Kind() == "string_literal") {
			return callExpr(selectorExpr(obj, "Get"), index)
		}
		// If the index is itself a JSValue (untyped local), use .Get() with string coercion
		// since .Index() expects int.
		if indexNode != nil && indexNode.Kind() == "identifier" && t.isUntypedLocal(indexNode.Utf8Text(t.source)) {
			t.addImport("fmt")
			return callExpr(selectorExpr(obj, "Get"), callExpr(selectorExpr(ident("fmt"), "Sprint"), index))
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
		// Everything else (call expressions, etc.) — use .Get() with fmt.Sprint
		// since the result may be a string, not an int.
		t.addImport("fmt")
		return callExpr(selectorExpr(obj, "Get"), callExpr(selectorExpr(ident("fmt"), "Sprint"), index))
	}
	// Go slice indices must be integers; wrap float64 vars with int().
	indexNode := node.ChildByFieldName("index")
	if indexNode != nil && indexNode.Kind() == "identifier" {
		index = callExpr(ident("int"), index)
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
	var args []ast.Expr
	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		switch child.Kind() {
		case "pair":
			keyNode := child.ChildByFieldName("key")
			valNode := child.ChildByFieldName("value")
			if keyNode != nil && valNode != nil {
				key := keyNode.Utf8Text(t.source)
				if val := t.transformExpr(valNode); val != nil {
					args = append(args, stringLit(key), t.wrapAsJSValue(val))
				}
			}
		case "shorthand_property_identifier":
			name := child.Utf8Text(t.source)
			args = append(args, stringLit(name), t.wrapAsJSValue(t.resolveIdentifier(name)))
		}
	}
	t.addAliasedImport("github.com/nnstd/gun/runtime/jsvalue", "jsvalue")
	return callExpr(selectorExpr(ident("jsvalue"), "ObjectFrom"), args...)
}

// isJSValueGet returns true if the expression is a JSValue .Get() call.
func isNumericLit(expr ast.Expr) bool {
	lit, ok := expr.(*ast.BasicLit)
	if !ok {
		return false
	}
	return lit.Kind == token.INT || lit.Kind == token.FLOAT
}

// isNumericExpr returns true if the expression is a numeric value (literal or unary expression like -1).
func isNumericExpr(expr ast.Expr) bool {
	// Check for basic numeric literal
	if isNumericLit(expr) {
		return true
	}
	// Check for unary expression (e.g., -1, +5)
	unary, ok := expr.(*ast.UnaryExpr)
	if !ok {
		return false
	}
	if unary.Op != token.SUB && unary.Op != token.ADD {
		return false
	}
	return isNumericLit(unary.X)
}

// shouldCoerceArg returns true if the argument should be coerced to string for the given method.
// Some methods expect specific argument types (regex, integer) that should not be coerced.
func shouldCoerceArg(method string, argIndex int, arg ast.Expr) bool {
	// Skip coercion for literal arguments
	if _, isLit := arg.(*ast.BasicLit); isLit {
		return false
	}

	// Methods that expect regex arguments (don't coerce)
	if method == "match" && argIndex == 0 {
		return false
	}

	// Methods that expect integer index arguments (don't coerce)
	if (method == "charAt" || method == "charCodeAt" || method == "codePointAt" ||
		method == "substring" || method == "substr") && argIndex == 0 {
		return false
	}

	// Methods that expect integer arguments for both start and end positions
	if method == "substring" && argIndex == 1 {
		return false
	}

	// Default: coerce to string
	return true
}

func isBoolLit(expr ast.Expr) bool {
	id, ok := expr.(*ast.Ident)
	return ok && (id.Name == "true" || id.Name == "false")
}

func isNilNode(node *sitter.Node) bool {
	if node == nil {
		return false
	}
	return node.Kind() == "null" || node.Kind() == "undefined"
}

// hasJSValueStringWrapper reports whether a string method has a dedicated jsvalue.* wrapper.
func hasJSValueStringWrapper(prop string) bool {
	switch prop {
	case "substring", "lastIndexOf", "indexOf", "split", "trim", "repeat",
		"toLowerCase", "toUpperCase", "startsWith", "endsWith",
		"charAt", "replace", "replaceAll", "match",
		"codePointAt", "charCodeAt", "toString":
		return true
	}
	return false
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

// isGoBuiltin returns true if the name is a Go built-in function.
func isGoBuiltin(name string) bool {
	switch name {
	case "append", "cap", "close", "complex", "copy", "delete", "imag",
		"len", "make", "new", "panic", "print", "println", "real", "recover":
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

	// Check for other JSValue methods (Get, Index, etc.)
	switch sel.Sel.Name {
	case "Get", "Index", "Call", "ToSlice":
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

	// Pointers/interfaces that could be nil → != nil
	switch expr.(type) {
	case *ast.Ident, *ast.SelectorExpr, *ast.IndexExpr:
		return &ast.BinaryExpr{X: expr, Op: token.NEQ, Y: ident("nil")}
	case *ast.CallExpr:
		return &ast.BinaryExpr{X: expr, Op: token.NEQ, Y: ident("nil")}
	}

	return expr
}

// isJSValueExpr returns true if the Go AST expression is known to produce *jsvalue.JSValue.
func isJSValueExpr(expr ast.Expr) bool {
	if isJSValueMethodCall(expr) {
		return true
	}
	if isJSValueGet(expr) {
		return true
	}
	// Identifiers that look like JSValue (checked by caller context)
	return false
}


// coerceJSValueArgs wraps JSValue identifier args with fmt.Sprint() so they
// can be passed to functions expecting string (e.g. regexp.MatchString).
func (t *Transformer) coerceJSValueArgs(args []ast.Expr, argsNode *sitter.Node) []ast.Expr {
	if argsNode == nil {
		return args
	}
	out := make([]ast.Expr, len(args))
	copy(out, args)
	for i := uint(0); i < argsNode.NamedChildCount() && int(i) < len(out); i++ {
		argNode := argsNode.NamedChild(i)
		if argNode != nil && argNode.Kind() == "identifier" && t.isUntypedLocal(argNode.Utf8Text(t.source)) {
			t.addImport("fmt")
			out[i] = callExpr(selectorExpr(ident("fmt"), "Sprint"), out[i])
		}
	}
	return out
}

// wrapAsJSValue wraps an expression with jsvalue.From() so it can be used
// as a *jsvalue.JSValue value in map literals. Expressions that are already
// jsvalue constructor calls are returned as-is.
func (t *Transformer) wrapAsJSValue(expr ast.Expr) ast.Expr {
	if isJSValueExpr(expr) {
		return expr
	}
	t.addAliasedImport("github.com/nnstd/gun/runtime/jsvalue", "jsvalue")
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
		t.addAliasedImport("github.com/nnstd/gun/runtime/jsvalue", "jsvalue")
		cons = jsvalueWrapLit(cons)
		alt = jsvalueWrapLit(alt)
	} else if id, ok := resultType.(*ast.Ident); ok {
		switch id.Name {
		case "string":
			if t.inferNodeResultType(consNode) == "" {
				t.addImport("fmt")
				cons = callExpr(selectorExpr(ident("fmt"), "Sprint"), cons)
			}
			if t.inferNodeResultType(altNode) == "" {
				t.addImport("fmt")
				alt = callExpr(selectorExpr(ident("fmt"), "Sprint"), alt)
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
		t.addAliasedImport("github.com/nnstd/gun/runtime/jsvalue", "jsvalue")
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
		t.addAliasedImport("github.com/nnstd/gun/runtime/jsvalue", "jsvalue")
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
	var results *ast.FieldList
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

	return &ast.FuncLit{
		Type: &ast.FuncType{Params: params, Results: results},
		Body: body,
	}
}

func (t *Transformer) transformFuncExpr(node *sitter.Node) ast.Expr {
	paramsNode := node.ChildByFieldName("parameters")
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

	ensureTrailingReturn(body, results)

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
	if r := transformBuiltinNew(name, args, t); r != nil {
		return r
	}

	// Default: new Foo(args) → Foo.Call(args...)
	// Classes are JSValue constructor functions; .Call() creates an instance.
	t.addAliasedImport("github.com/nnstd/gun/runtime/jsvalue", "jsvalue")
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
