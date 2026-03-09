package backend

import (
	"go/ast"
	"go/token"
	"path"
	"strings"

	"github.com/nnstd/gun/compiler/hir"
	"github.com/nnstd/gun/compiler/symbol"
)

// --------------------------------------------------------------------
// Expressions
// --------------------------------------------------------------------

func (l *Lowerer) lowerExpr(e hir.Expr) ast.Expr {
	if e == nil {
		return goIdent("nil")
	}
	switch e := e.(type) {
	case *hir.Identifier:
		return l.lowerIdentifier(e)

	case *hir.Literal:
		return l.lowerLiteral(e)

	case *hir.TemplateLiteral:
		return l.lowerTemplateLiteral(e)

	case *hir.ArrayLiteral:
		return l.lowerArrayLiteral(e)

	case *hir.ObjectLiteral:
		return l.lowerObjectLiteral(e)

	case *hir.BinaryExpr:
		return l.lowerBinaryExpr(e)

	case *hir.UnaryExpr:
		return l.lowerUnaryExpr(e)

	case *hir.UpdateExpr:
		return l.lowerUpdateExpr(e)

	case *hir.AssignExpr:
		return l.lowerAssignExpr(e)

	case *hir.CallExpr:
		return l.lowerCallExpr(e)

	case *hir.NewExpr:
		return l.lowerNewExpr(e)

	case *hir.MemberExpr:
		return l.lowerMemberExpr(e)

	case *hir.ComputedMemberExpr:
		obj := l.lowerExpr(e.Object)
		idx := l.lowerExpr(e.Property)
		l.addImport("fmt")
		return callExpr(selectorExpr(obj, "Get"),
			callExpr(selectorExpr(goIdent("fmt"), "Sprint"), idx))

	case *hir.TernaryExpr:
		return l.lowerTernaryExpr(e)

	case *hir.ArrowFunc:
		return l.lowerArrowFunc(e)

	case *hir.FuncExpr:
		return l.lowerFuncExpr(e)

	case *hir.SpreadExpr:
		return l.lowerExpr(e.Value)

	case *hir.SequenceExpr:
		return l.lowerSequenceExpr(e)

	case *hir.AwaitExpr:
		return l.lowerExpr(e.Value)

	case *hir.YieldExpr:
		return l.lowerExpr(e.Value)

	case *hir.TypeAssertExpr:
		return l.lowerExpr(e.Expr)

	case *hir.NonNullExpr:
		return l.lowerExpr(e.Expr)

	case *hir.ParenExpr:
		return &ast.ParenExpr{X: l.lowerExpr(e.Expr)}

	case *hir.ThisExpr:
		if l.insideFunc == 0 {
			// Top-level this is undefined in ESM (SWC convention)
			l.jsvalueImport()
			return callExpr(selectorExpr(goIdent("jsvalue"), "NewUndefined"))
		}
		return goIdent("this")

	case *hir.SuperExpr:
		return goIdent("super")

	case *hir.MetaPropertyExpr:
		if e.Meta == "import" && e.Property == "meta" {
			l.addImport("github.com/nnstd/gun/runtime/module")
			return callExpr(selectorExpr(goIdent("module"), "ImportMetaAsJSValue"))
		}
		return goIdent("nil")

	case *hir.TaggedTemplateLiteral:
		return l.lowerTaggedTemplate(e)

	default:
		return goIdent("nil")
	}
}

func (l *Lowerer) lowerIdentifier(e *hir.Identifier) ast.Expr {
	// JS `arguments` keyword → jsvalue.NewArray(_args...)
	name := e.Name
	if e.Sym != nil {
		name = e.Sym.OriginalName
	}
	if name == "arguments" {
		l.jsvalueImport()
		// In method context, _args[0] is 'this' — exclude it from arguments
		argsExpr := ast.Expr(goIdent("_args"))
		if l.insideMethod > 0 {
			argsExpr = &ast.SliceExpr{X: goIdent("_args"), Low: intLit("1")}
		}
		c := callExpr(selectorExpr(goIdent("jsvalue"), "NewArray"), argsExpr)
		c.Ellipsis = 1
		return c
	}

	// Check imported symbols first
	if e.Sym != nil {
		if res, ok := l.importedSyms[e.Sym]; ok {
			// Add the import
			if res.goImportPath != "" {
				if res.goPkgName != "" && res.goPkgName != path.Base(res.goImportPath) {
					l.addAliasedImport(res.goImportPath, res.goPkgName)
				} else if res.goImportPath != "" {
					l.addImport(res.goImportPath)
				}
			}
			// Default/namespace import from Gun runtime module → pkg.AsJSValue
			if res.goSymbol == "AsJSValue" && !res.isTranspiled && isGunRuntimePkg(res.goImportPath) {
				return selectorExpr(goIdent(res.goPkgName), "AsJSValue")
			}
			// Namespace import (no goSymbol)
			if res.goSymbol == "" {
				if res.goPkgName == "" {
					return goIdent(e.Sym.OriginalName)
				}
				return goIdent(res.goPkgName)
			}
			// Same-package reference
			if res.goPkgName == "" {
				return goIdent(res.goSymbol)
			}
			// Named import from Gun runtime module → pkg.AsJSValue.Get("jsName")
			if !res.isTranspiled && res.goPkgName != "" && isGunRuntimePkg(res.goImportPath) {
				return callExpr(selectorExpr(
					selectorExpr(goIdent(res.goPkgName), "AsJSValue"),
					"Get"), stringLit(lowercaseFirst(res.goSymbol)))
			}
			return selectorExpr(goIdent(res.goPkgName), res.goSymbol)
		}
		// Non-imported symbol — use emitName
		return goIdent(l.emitName(e.Sym))
	}
	// Unresolved — check context for known identifiers
	if l.ctx != nil {
		if expr := l.ctx.TransformIdentifier(e.Name, l); expr != nil {
			return expr
		}
	}
	return goIdent(symbol.Sanitize(e.Name))
}

func (l *Lowerer) lowerLiteral(e *hir.Literal) ast.Expr {
	l.jsvalueImport()
	switch e.Kind {
	case hir.LitString:
		return callExpr(selectorExpr(goIdent("jsvalue"), "NewString"), stringLit(e.Value))
	case hir.LitNumber:
		if strings.Contains(e.Value, ".") {
			return callExpr(selectorExpr(goIdent("jsvalue"), "NewNumber"), floatLit(e.Value))
		}
		return callExpr(selectorExpr(goIdent("jsvalue"), "NewNumber"), callExpr(goIdent("float64"), intLit(e.Value)))
	case hir.LitBool:
		return callExpr(selectorExpr(goIdent("jsvalue"), "NewBool"), goIdent(e.Value))
	case hir.LitNull:
		return callExpr(selectorExpr(goIdent("jsvalue"), "NewNull"))
	case hir.LitUndefined:
		return callExpr(selectorExpr(goIdent("jsvalue"), "NewUndefined"))
	case hir.LitRegex:
		// Parse /pattern/flags — use raw string literal for pattern to preserve backslashes
		pattern, _ := parseRegexLiteral(e.Value)
		patternLit := rawStringLit(pattern)
		compiled := callExpr(selectorExpr(goIdent("jsvalue"), "CompileRegex"), patternLit)
		// Flags are currently ignored at the Go level (Go regexp doesn't support JS flags like /g)
		return callExpr(selectorExpr(goIdent("jsvalue"), "NewRegex"), compiled)
	default:
		return goIdent(e.Value)
	}
}

func (l *Lowerer) lowerTemplateLiteral(e *hir.TemplateLiteral) ast.Expr {
	l.addImport("fmt")
	l.jsvalueImport()

	var format strings.Builder
	var args []ast.Expr

	for _, part := range e.Parts {
		if lit, ok := part.(*hir.Literal); ok && lit.Kind == hir.LitString {
			format.WriteString(lit.Value)
		} else {
			lowered := l.lowerExpr(part)
			// Safety check: if the lowered expression is just an identifier with
			// invalid Go characters, embed it back into the format string
			if id, ok := lowered.(*ast.Ident); ok && !isValidGoIdent(id.Name) {
				format.WriteString(id.Name)
			} else {
				format.WriteString("%v")
				args = append(args, lowered)
			}
		}
	}

	// Use raw string if format contains newlines or backslashes
	fmtStr := format.String()
	var fmtLit ast.Expr
	if strings.ContainsAny(fmtStr, "\n\\") {
		fmtLit = rawStringLit(fmtStr)
	} else {
		fmtLit = stringLit(fmtStr)
	}

	if len(args) == 0 {
		// No substitutions — just a string
		return callExpr(selectorExpr(goIdent("jsvalue"), "NewString"), fmtLit)
	}
	sprintfArgs := append([]ast.Expr{fmtLit}, args...)
	formatted := callExpr(selectorExpr(goIdent("fmt"), "Sprintf"), sprintfArgs...)
	return callExpr(selectorExpr(goIdent("jsvalue"), "NewString"), formatted)
}

func (l *Lowerer) lowerTaggedTemplate(e *hir.TaggedTemplateLiteral) ast.Expr {
	l.jsvalueImport()
	tag := l.lowerExpr(e.Tag)

	// Build strings array and expressions list from template parts
	var stringParts []ast.Expr
	var exprParts []ast.Expr

	if e.Template != nil {
		for _, part := range e.Template.Parts {
			if lit, ok := part.(*hir.Literal); ok && lit.Kind == hir.LitString {
				stringParts = append(stringParts, callExpr(selectorExpr(goIdent("jsvalue"), "NewString"), stringLit(lit.Value)))
			} else {
				exprParts = append(exprParts, l.lowerExpr(part))
			}
		}
	}

	// tag(jsvalue.NewArray(strings...), expr1, expr2, ...)
	stringsArr := callExpr(selectorExpr(goIdent("jsvalue"), "NewArray"), stringParts...)
	args := append([]ast.Expr{stringsArr}, exprParts...)
	return callExpr(tag, args...)
}

func (l *Lowerer) lowerArrayLiteral(e *hir.ArrayLiteral) ast.Expr {
	l.jsvalueImport()
	var args []ast.Expr
	for _, elem := range e.Elements {
		if elem == nil {
			continue
		}
		arg := l.lowerExpr(elem)
		if arg == nil {
			continue
		}
		args = append(args, jsvalueWrapLit(arg))
	}
	return callExpr(selectorExpr(goIdent("jsvalue"), "NewArray"), args...)
}

func (l *Lowerer) lowerObjectLiteral(e *hir.ObjectLiteral) ast.Expr {
	l.jsvalueImport()

	// Check if there are spread properties — if so, use jsvalue.Assign to merge
	hasSpread := false
	for _, prop := range e.Properties {
		if _, ok := prop.Key.(*hir.SpreadExpr); ok {
			hasSpread = true
			break
		}
	}
	if hasSpread {
		return l.lowerObjectWithSpreads(e)
	}

	// jsvalue.ObjectFrom(map[string]any{...})
	var elts []ast.Expr
	for _, prop := range e.Properties {
		key := stringLit(prop.KeyName)
		value := l.lowerExpr(prop.Value)
		elts = append(elts, &ast.KeyValueExpr{Key: key, Value: value})
	}
	mapType := &ast.MapType{
		Key:   goIdent("string"),
		Value: &ast.InterfaceType{Methods: &ast.FieldList{}},
	}
	mapLit := &ast.CompositeLit{Type: mapType, Elts: elts}
	return callExpr(selectorExpr(goIdent("jsvalue"), "ObjectFrom"), mapLit)
}

// lowerObjectWithSpreads handles object literals with spread properties:
// {a: 1, ...obj, b: 2} → jsvalue.Assign(jsvalue.ObjectFrom({a:1}), obj, jsvalue.ObjectFrom({b:2}))
func (l *Lowerer) lowerObjectWithSpreads(e *hir.ObjectLiteral) ast.Expr {
	var args []ast.Expr
	var currentElts []ast.Expr

	flush := func() {
		if len(currentElts) > 0 {
			mapType := &ast.MapType{
				Key:   goIdent("string"),
				Value: &ast.InterfaceType{Methods: &ast.FieldList{}},
			}
			mapLit := &ast.CompositeLit{Type: mapType, Elts: currentElts}
			args = append(args, callExpr(selectorExpr(goIdent("jsvalue"), "ObjectFrom"), mapLit))
			currentElts = nil
		}
	}

	for _, prop := range e.Properties {
		if spread, ok := prop.Key.(*hir.SpreadExpr); ok {
			flush()
			args = append(args, l.lowerExpr(spread.Value))
		} else {
			key := stringLit(prop.KeyName)
			value := l.lowerExpr(prop.Value)
			currentElts = append(currentElts, &ast.KeyValueExpr{Key: key, Value: value})
		}
	}
	flush()

	if len(args) == 0 {
		return callExpr(selectorExpr(goIdent("jsvalue"), "NewObject"))
	}
	if len(args) == 1 {
		return args[0]
	}
	return callExpr(selectorExpr(goIdent("jsvalue"), "Assign"), args...)
}

func (l *Lowerer) lowerBinaryExpr(e *hir.BinaryExpr) ast.Expr {
	l.jsvalueImport()

	left := l.lowerExpr(e.Left)
	right := l.lowerExpr(e.Right)

	// Special operators
	switch e.Op {
	case hir.OpIn:
		// key in obj → jsvalue.NewBool(obj.HasOwnProperty(fmt.Sprint(key)))
		l.addImport("fmt")
		return callExpr(selectorExpr(goIdent("jsvalue"), "NewBool"),
			callExpr(selectorExpr(right, "HasOwnProperty"),
				callExpr(selectorExpr(goIdent("fmt"), "Sprint"), left)))
	case hir.OpInstanceof:
		// a instanceof B → a != nil (simplified)
		return &ast.BinaryExpr{X: left, Op: token.NEQ, Y: goIdent("nil")}
	}

	// JSValue binary operations: jsvalue.Op(left, right)
	helperName := mapBinaryOpToJSValue(e.Op)
	return callExpr(selectorExpr(goIdent("jsvalue"), helperName),
		jsvalueWrapLit(left), jsvalueWrapLit(right))
}

func (l *Lowerer) lowerUnaryExpr(e *hir.UnaryExpr) ast.Expr {
	l.jsvalueImport()
	operand := l.lowerExpr(e.Operand)

	switch e.Op {
	case hir.OpVoid:
		return callExpr(selectorExpr(goIdent("jsvalue"), "NewUndefined"))
	case hir.OpPos:
		return operand // identity
	case hir.OpDelete:
		// delete obj.prop → handled specially, for now pass through
		return operand
	}

	helperName := mapUnaryOpToJSValue(e.Op)
	if helperName != "" {
		return callExpr(selectorExpr(goIdent("jsvalue"), helperName), jsvalueWrapLit(operand))
	}

	return operand
}

func (l *Lowerer) lowerUpdateExpr(e *hir.UpdateExpr) ast.Expr {
	l.jsvalueImport()
	operand := l.lowerExpr(e.Operand)

	if e.Op == hir.OpInc {
		return callExpr(selectorExpr(goIdent("jsvalue"), "Inc"), operand)
	}
	return callExpr(selectorExpr(goIdent("jsvalue"), "Dec"), operand)
}

func (l *Lowerer) lowerAssignExpr(e *hir.AssignExpr) ast.Expr {
	// Destructuring assignment in expression context → IIFE with destructuring stmts
	if e.LeftPattern != nil {
		right := l.lowerExpr(e.Right)
		l.jsvalueImport()
		stmts := l.lowerDestructuring(e.LeftPattern, right, false)
		stmts = append(stmts, returnStmt(goIdent("nil")))
		return &ast.CallExpr{
			Fun: &ast.FuncLit{
				Type: &ast.FuncType{Params: fieldList(), Results: fieldList(goField("", jsValuePtrType()))},
				Body: &ast.BlockStmt{List: stmts},
			},
		}
	}

	right := l.lowerExpr(e.Right)

	// Member assignment expression: obj.prop = val → IIFE { obj.Set("prop", val); return val }
	if mem, ok := e.Left.(*hir.MemberExpr); ok && e.Op == hir.OpAssign && l.exprIsJSValue(mem.Object) {
		l.jsvalueImport()
		obj := l.lowerExpr(mem.Object)
		val := l.wrapAsJSValue(right)
		setCall := callExpr(selectorExpr(obj, "Set"), stringLit(mem.Property), val)
		return &ast.CallExpr{
			Fun: &ast.FuncLit{
				Type: &ast.FuncType{Params: fieldList(), Results: fieldList(goField("", jsValuePtrType()))},
				Body: blockStmt(exprStmt(setCall), returnStmt(val)),
			},
		}
	}

	// Computed member assignment: obj[key] = val → IIFE { obj.Set(key, val); return val }
	if comp, ok := e.Left.(*hir.ComputedMemberExpr); ok && e.Op == hir.OpAssign && l.exprIsJSValue(comp.Object) {
		l.jsvalueImport()
		l.addImport("fmt")
		obj := l.lowerExpr(comp.Object)
		key := l.lowerExpr(comp.Property)
		val := l.wrapAsJSValue(right)
		setCall := callExpr(selectorExpr(obj, "Set"),
			callExpr(selectorExpr(goIdent("fmt"), "Sprint"), key), val)
		return &ast.CallExpr{
			Fun: &ast.FuncLit{
				Type: &ast.FuncType{Params: fieldList(), Results: fieldList(goField("", jsValuePtrType()))},
				Body: blockStmt(exprStmt(setCall), returnStmt(val)),
			},
		}
	}

	left := l.lowerExpr(e.Left)

	if e.Op == hir.OpAssign {
		// Simple assignment as IIFE: func() T { x = val; return val }()
		return &ast.CallExpr{
			Fun: &ast.FuncLit{
				Type: &ast.FuncType{
					Params:  fieldList(),
					Results: fieldList(goField("", jsValuePtrType())),
				},
				Body: blockStmt(
					assignStmt([]ast.Expr{left}, []ast.Expr{jsvalueWrapLit(right)}),
					returnStmt(left),
				),
			},
		}
	}

	// Augmented assignment: x += y → x = jsvalue.Add(x, y)
	l.jsvalueImport()
	helperName := mapAssignOpToJSValue(e.Op)
	computed := callExpr(selectorExpr(goIdent("jsvalue"), helperName),
		jsvalueWrapLit(left), jsvalueWrapLit(right))
	return &ast.CallExpr{
		Fun: &ast.FuncLit{
			Type: &ast.FuncType{
				Params:  fieldList(),
				Results: fieldList(goField("", jsValuePtrType())),
			},
			Body: blockStmt(
				assignStmt([]ast.Expr{left}, []ast.Expr{computed}),
				returnStmt(left),
			),
		},
	}
}

func (l *Lowerer) lowerCallExpr(e *hir.CallExpr) ast.Expr {
	// super(args) in class constructor → ClassName.CallSuper(this, args...)
	if _, ok := e.Func.(*hir.SuperExpr); ok && l.currentClassName != "" {
		l.jsvalueImport()
		args, _ := l.lowerCallArgs(e.Args, true)
		allArgs := append([]ast.Expr{goIdent("this")}, args...)
		return callExpr(selectorExpr(goIdent(l.currentClassName), "CallSuper"), allArgs...)
	}

	// Check for builtin method calls: console.log(), Math.floor(), JSON.parse(), etc.
	if mem, ok := e.Func.(*hir.MemberExpr); ok {
		if id, ok := mem.Object.(*hir.Identifier); ok {
			objName := id.Name
			if id.Sym != nil {
				objName = id.Sym.OriginalName
			}
			if l.ctx != nil {
				var args []ast.Expr
				for _, a := range e.Args {
					args = append(args, l.lowerExpr(a))
				}
				if result := l.ctx.TransformBuiltinCall(objName, mem.Property, args, l); result != nil {
					return result
				}
			}
		}

		// JSValue method dispatch: obj.method(args) → obj.MethodCall("method", wrappedArgs...)
		if l.exprIsJSValue(mem.Object) {
			l.jsvalueImport()
			obj := l.lowerExpr(mem.Object)
			argExprs, hasSpread := l.lowerCallArgs(e.Args, true)
			wrappedArgs := append([]ast.Expr{stringLit(mem.Property)}, argExprs...)
			// For complex receivers, wrap in IIFE to evaluate once
			if !l.isSimpleExpr(obj) {
				recv := goIdent("_recv")
				recvArgs := make([]ast.Expr, len(wrappedArgs))
				copy(recvArgs, wrappedArgs)
				innerCall := buildCallWithSpread(selectorExpr(recv, "MethodCall"), recvArgs, hasSpread)
				return callExpr(&ast.FuncLit{
					Type: &ast.FuncType{
						Params:  fieldList(goField("_recv", jsValuePtrType())),
						Results: fieldList(goField("", jsValuePtrType())),
					},
					Body: blockStmt(returnStmt(innerCall)),
				}, obj)
			}
			return buildCallWithSpread(selectorExpr(obj, "MethodCall"), wrappedArgs, hasSpread)
		}
	}

	// Check for bare global function calls: parseInt(), isNaN(), etc.
	if id, ok := e.Func.(*hir.Identifier); ok {
		name := id.Name
		if id.Sym != nil {
			name = id.Sym.OriginalName
		}
		if l.ctx != nil {
			var args []ast.Expr
			for _, a := range e.Args {
				args = append(args, l.lowerExpr(a))
			}
			if result := l.ctx.TransformGlobalCall(name, args, l); result != nil {
				return result
			}
		}

		// JSValue function call: fn(args) → fn.Call(wrappedArgs...)
		if id.Sym != nil {
			isJSValueFunc := false
			if res, ok := l.importedSyms[id.Sym]; ok && res.isTranspiled {
				isJSValueFunc = true
			} else if id.Sym.Kind == symbol.KindVariable || id.Sym.Kind == symbol.KindParameter || id.Sym.Kind == symbol.KindFunction {
				// In all-JSValue architecture, all functions are JSValue (except main/init)
				if id.Sym.OriginalName != "main" && id.Sym.OriginalName != "init" {
					isJSValueFunc = true
				}
			}
			if isJSValueFunc {
				l.jsvalueImport()
				fn := l.lowerIdentifier(id)
				wrappedArgs, hasSpread := l.lowerCallArgs(e.Args, true)
				return buildCallWithSpread(selectorExpr(fn, "Call"), wrappedArgs, hasSpread)
			}
		}
	}

	fn := l.lowerExpr(e.Func)
	args, hasSpread := l.lowerCallArgs(e.Args, false)

	// If the lowered function expression is already JSValue (e.g. .Get() result),
	// use .Call() to invoke it
	if isAlreadyJSValue(fn) {
		l.jsvalueImport()
		for i, a := range args {
			args[i] = jsvalueWrapLit(a)
		}
		return buildCallWithSpread(selectorExpr(fn, "Call"), args, hasSpread)
	}

	// If the function is a bare identifier (not pkg.Func), it's likely a JSValue
	// variable being called. Use .Call() to be safe.
	if id, ok := fn.(*ast.Ident); ok {
		// Don't wrap Go builtins or known function names
		if id.Name != "len" && id.Name != "cap" && id.Name != "make" &&
			id.Name != "append" && id.Name != "panic" && id.Name != "recover" &&
			id.Name != "float64" && id.Name != "int" && id.Name != "string" &&
			id.Name != "fmt" && id.Name != "nil" {
			// Check if this could be a JSValue function
			if l.exprIsJSValue(e.Func) {
				l.jsvalueImport()
				for i, a := range args {
					args[i] = l.wrapAsJSValue(a)
				}
				return buildCallWithSpread(selectorExpr(fn, "Call"), args, hasSpread)
			}
		}
	}

	return buildCallWithSpread(fn, args, hasSpread)
}

// lowerCallArgs lowers call arguments, handling spread expressions.
// Returns the lowered args and whether the last arg has spread (Ellipsis).
func (l *Lowerer) lowerCallArgs(hirArgs []hir.Expr, wrap bool) ([]ast.Expr, bool) {
	var args []ast.Expr
	hasTrailingSpread := false
	for i, a := range hirArgs {
		if spread, ok := a.(*hir.SpreadExpr); ok {
			val := l.lowerExpr(spread.Value)
			if i == len(hirArgs)-1 {
				// Trailing spread: use Ellipsis
				// Convert JSValue to slice: val.Array()
				spreadSlice := callExpr(selectorExpr(val, "Array"))
				if len(args) > 0 {
					// Go doesn't allow mixing individual args with a spread
					// for the same variadic parameter. Merge into:
					//   append([]*jsvalue.JSValue{arg1, arg2}, spreadSlice...)
					l.jsvalueImport()
					composite := &ast.CompositeLit{
						Type: &ast.ArrayType{Elt: jsValuePtrType()},
						Elts: args,
					}
					merged := callExpr(goIdent("append"), composite, spreadSlice)
					merged.Ellipsis = 1
					args = []ast.Expr{merged}
				} else {
					args = append(args, spreadSlice)
				}
				hasTrailingSpread = true
			} else {
				// Mid-position spread — just pass through (simplified)
				if wrap {
					args = append(args, l.wrapAsJSValue(val))
				} else {
					args = append(args, val)
				}
			}
		} else {
			lowered := l.lowerExpr(a)
			if lowered == nil {
				continue
			}
			if wrap {
				args = append(args, l.wrapAsJSValue(lowered))
			} else {
				args = append(args, lowered)
			}
		}
	}
	return args, hasTrailingSpread
}

// buildCallWithSpread creates a call expression, setting Ellipsis if the last arg is spread.
func buildCallWithSpread(fun ast.Expr, args []ast.Expr, hasSpread bool) *ast.CallExpr {
	c := callExpr(fun, args...)
	if hasSpread {
		c.Ellipsis = 1
	}
	return c
}

// wrapAsJSValue wraps an expression to ensure it's *jsvalue.JSValue.
func (l *Lowerer) wrapAsJSValue(expr ast.Expr) ast.Expr {
	if isAlreadyJSValue(expr) {
		return expr
	}
	return jsvalueWrapLit(expr)
}

// isSimpleExpr returns true if the expression is safe to duplicate (ident or selector).
func (l *Lowerer) isSimpleExpr(expr ast.Expr) bool {
	switch expr.(type) {
	case *ast.Ident:
		return true
	case *ast.SelectorExpr:
		return true
	}
	return false
}

func (l *Lowerer) lowerNewExpr(e *hir.NewExpr) ast.Expr {
	l.jsvalueImport()

	// Check if constructor is known through context
	if id, ok := e.Callee.(*hir.Identifier); ok {
		name := id.Name
		if id.Sym != nil {
			name = id.Sym.OriginalName
		}
		if l.ctx != nil {
			var args []ast.Expr
			for _, a := range e.Args {
				args = append(args, l.lowerExpr(a))
			}
			if expr := l.ctx.TransformBuiltinNew(name, args, l); expr != nil {
				return expr
			}
		}
	}

	callee := l.lowerExpr(e.Callee)
	var args []ast.Expr
	for _, a := range e.Args {
		arg := l.lowerExpr(a)
		args = append(args, jsvalueWrapLit(arg))
	}
	return callExpr(selectorExpr(callee, "Call"), args...)
}

func (l *Lowerer) lowerMemberExpr(e *hir.MemberExpr) ast.Expr {
	// Optional chaining: a?.b → IIFE with null check
	if e.Optional {
		return l.lowerOptionalMember(e)
	}

	// Same-package namespace import: templates.foo → Foo (capitalized package var)
	if id, ok := e.Object.(*hir.Identifier); ok && id.Sym != nil {
		if res, ok := l.importedSyms[id.Sym]; ok && res.goImportPath == "" && res.goSymbol == "" {
			return goIdent(symbol.Capitalize(e.Property))
		}
	}

	// Check for builtin member access through context (e.g. process.env)
	if id, ok := e.Object.(*hir.Identifier); ok {
		name := id.Name
		if id.Sym != nil {
			name = id.Sym.OriginalName
		}
		if l.ctx != nil {
			if expr := l.ctx.TransformBuiltinMember(name, e.Property, l); expr != nil {
				return expr
			}
		}
	}

	obj := l.lowerExpr(e.Object)

	// .length → .Len() for JSValue, len() for typed
	if e.Property == "length" {
		if l.exprIsJSValue(e.Object) {
			return callExpr(selectorExpr(obj, "Len"))
		}
		return callExpr(goIdent("len"), obj)
	}

	// JSValue receivers → .Get("prop")
	if l.exprIsJSValue(e.Object) {
		l.jsvalueImport()
		return callExpr(selectorExpr(obj, "Get"), stringLit(e.Property))
	}

	// Known globals — check if the resolved expression is a package ident (typed)
	// or a JSValue expression (needs .Get())
	if id, ok := e.Object.(*hir.Identifier); ok {
		name := id.Name
		if id.Sym != nil {
			name = id.Sym.OriginalName
		}
		if l.ctx != nil && l.ctx.IsKnownGlobal(name) {
			// If the resolved identifier is a bare Go identifier (package name),
			// use capitalized selector. If it's something else (jsvalue expression),
			// use .Get() instead.
			if _, isIdent := obj.(*ast.Ident); isIdent {
				return selectorExpr(obj, symbol.Capitalize(e.Property))
			}
			// Not a bare ident — it's a JSValue expression (e.g. error.Error)
			return callExpr(selectorExpr(obj, "Get"), stringLit(e.Property))
		}
		// Imported known-module symbols → capitalize
		if id.Sym != nil {
			if res, ok := l.importedSyms[id.Sym]; ok && !res.isTranspiled {
				return selectorExpr(obj, symbol.Capitalize(e.Property))
			}
		}
	}

	// Default for unknown: use .Get() (safer — assumes JSValue)
	l.jsvalueImport()
	return callExpr(selectorExpr(obj, "Get"), stringLit(e.Property))
}

// lowerOptionalMember handles a?.b → IIFE with jsvalue.Eq null check.
func (l *Lowerer) lowerOptionalMember(e *hir.MemberExpr) ast.Expr {
	l.jsvalueImport()
	obj := l.lowerExpr(e.Object)

	// func() *jsvalue.JSValue {
	//   _o := obj
	//   if jsvalue.Eq(_o, jsvalue.NewNull()).Bool() || _o == nil {
	//     return jsvalue.NewUndefined()
	//   }
	//   return _o.Get("prop")
	// }()
	tmpName := "_o"
	nullCheck := &ast.BinaryExpr{
		X: callExpr(selectorExpr(
			callExpr(selectorExpr(goIdent("jsvalue"), "Eq"),
				goIdent(tmpName),
				callExpr(selectorExpr(goIdent("jsvalue"), "NewNull"))),
			"Bool")),
		Op: token.LOR,
		Y:  &ast.BinaryExpr{X: goIdent(tmpName), Op: token.EQL, Y: goIdent("nil")},
	}

	return &ast.CallExpr{
		Fun: &ast.FuncLit{
			Type: &ast.FuncType{
				Params:  fieldList(),
				Results: fieldList(goField("", jsValuePtrType())),
			},
			Body: blockStmt(
				assignDefine([]ast.Expr{goIdent(tmpName)}, []ast.Expr{obj}),
				&ast.IfStmt{
					Cond: nullCheck,
					Body: blockStmt(returnStmt(callExpr(selectorExpr(goIdent("jsvalue"), "NewUndefined")))),
				},
				returnStmt(callExpr(selectorExpr(goIdent(tmpName), "Get"), stringLit(e.Property))),
			),
		},
	}
}

// exprIsJSValue returns true if the HIR expression is known to produce *jsvalue.JSValue.
func (l *Lowerer) exprIsJSValue(e hir.Expr) bool {
	switch e := e.(type) {
	case *hir.Identifier:
		if e.Sym == nil {
			if l.ctx != nil && l.ctx.IsKnownGlobal(e.Name) {
				// Known global — check if it resolves to a JSValue or a typed Go entity.
				// Bare idents like "jsvalue", "console", "math" are Go packages (not JSValue).
				// SelectorExprs like error.Error are JSValue references.
				if l.ctx.LookupIdentifier(e.Name) != nil {
					resolved := l.ctx.TransformIdentifier(e.Name, l)
					if _, isIdent := resolved.(*ast.Ident); isIdent {
						return false // Go package or type, not JSValue
					}
					return true // selector or call — it's a JSValue
				}
				return false
			}
			return true
		}
		if res, ok := l.importedSyms[e.Sym]; ok {
			// Known modules with AsJSValue resolve to a JSValue object
			if res.goSymbol == "AsJSValue" {
				return true
			}
			return res.isTranspiled
		}
		if e.Sym.Kind == symbol.KindParameter || e.Sym.Kind == symbol.KindVariable {
			return true
		}
		return false
	case *hir.ThisExpr, *hir.CallExpr, *hir.NewExpr, *hir.MemberExpr,
		*hir.ComputedMemberExpr, *hir.ArrayLiteral, *hir.ObjectLiteral,
		*hir.BinaryExpr, *hir.UnaryExpr, *hir.ArrowFunc, *hir.FuncExpr,
		*hir.TernaryExpr, *hir.TemplateLiteral:
		return true
	case *hir.Literal:
		return true // all-JSValue: literals are wrapped by lowerLiteral
	default:
		return true
	}
}

func (l *Lowerer) lowerTernaryExpr(e *hir.TernaryExpr) ast.Expr {
	cond := l.lowerExpr(e.Cond)
	cond = l.ensureBool(cond)
	then := l.lowerExpr(e.Then)
	els := l.lowerExpr(e.Else)

	l.jsvalueImport()
	// IIFE: func() *jsvalue.JSValue { if cond { return then } else { return else } }()
	return &ast.CallExpr{
		Fun: &ast.FuncLit{
			Type: &ast.FuncType{
				Params:  fieldList(),
				Results: fieldList(goField("", jsValuePtrType())),
			},
			Body: blockStmt(
				&ast.IfStmt{
					Cond: cond,
					Body: blockStmt(returnStmt(jsvalueWrapLit(then))),
					Else: blockStmt(returnStmt(jsvalueWrapLit(els))),
				},
			),
		},
	}
}

func (l *Lowerer) lowerArrowFunc(e *hir.ArrowFunc) ast.Expr {
	l.jsvalueImport()

	var body *ast.BlockStmt
	if e.Body != nil {
		body = l.lowerFuncBody(e.Params, e.Body)
	} else if e.ExprBody != nil {
		// Concise body: () => expr → wrap as return statement in HIR body
		val := e.ExprBody
		hirBody := &hir.BlockStmt{
			Stmts: []hir.Stmt{&hir.ReturnStmt{Value: val}},
		}
		body = l.lowerFuncBody(e.Params, hirBody)
	} else {
		body = blockStmt()
	}

	fnLit := l.wrapAsJSValueFunc(e.Params, body)
	return callExpr(selectorExpr(goIdent("jsvalue"), "NewFunction"), fnLit)
}

func (l *Lowerer) lowerFuncExpr(e *hir.FuncExpr) ast.Expr {
	l.jsvalueImport()

	// Regular function expressions get their own `this` binding (unlike arrow functions).
	// If the body references `this`, use lowerMethodBody to unpack it from _args[0].
	var body *ast.BlockStmt
	if e.Body != nil && hirBodyUsesThis(e.Body) {
		body = l.lowerMethodBody(e.Params, e.Body)
	} else {
		body = l.lowerFuncBody(e.Params, e.Body)
	}
	fnLit := l.wrapAsJSValueFunc(e.Params, body)
	return callExpr(selectorExpr(goIdent("jsvalue"), "NewFunction"), fnLit)
}

func (l *Lowerer) lowerSequenceExpr(e *hir.SequenceExpr) ast.Expr {
	if len(e.Exprs) == 0 {
		return goIdent("nil")
	}
	if len(e.Exprs) == 1 {
		return l.lowerExpr(e.Exprs[0])
	}
	// IIFE: func() T { expr1; expr2; return exprN }()
	l.jsvalueImport()
	var stmts []ast.Stmt
	for i, ex := range e.Exprs {
		lowered := l.lowerExpr(ex)
		if i == len(e.Exprs)-1 {
			stmts = append(stmts, returnStmt(jsvalueWrapLit(lowered)))
		} else {
			stmts = append(stmts, exprStmt(lowered))
		}
	}
	return &ast.CallExpr{
		Fun: &ast.FuncLit{
			Type: &ast.FuncType{
				Params:  fieldList(),
				Results: fieldList(goField("", jsValuePtrType())),
			},
			Body: &ast.BlockStmt{List: stmts},
		},
	}
}

// Lowerer implements context.Imports so it can be passed to TranspilerContext methods.

func (l *Lowerer) AddImport(pkg string) {
	l.addImport(pkg)
}

func (l *Lowerer) AddAliasedImport(pkg, alias string) {
	l.addAliasedImport(pkg, alias)
}

// isValidGoIdent checks if a string is a valid Go identifier.
func isValidGoIdent(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		if i == 0 {
			if r != '_' && (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') {
				return false
			}
		} else {
			if r != '_' && (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') {
				return false
			}
		}
	}
	return true
}

// lowercaseFirst returns the string with the first character lowercased.
// Used to convert Go capitalized names back to JS camelCase for .Get() lookups.
func lowercaseFirst(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	if r[0] >= 'A' && r[0] <= 'Z' {
		r[0] = r[0] + 32
	}
	return string(r)
}

// parseRegexLiteral splits /pattern/flags into pattern and flags.
func parseRegexLiteral(s string) (pattern, flags string) {
	if len(s) < 2 || s[0] != '/' {
		return s, ""
	}
	lastSlash := strings.LastIndex(s[1:], "/")
	if lastSlash < 0 {
		return s, ""
	}
	lastSlash++
	return s[1:lastSlash], s[lastSlash+1:]
}
