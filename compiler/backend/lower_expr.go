package backend

import (
	"fmt"
	"go/ast"
	"go/token"
	"path"
	"strconv"
	"strings"

	"github.com/nnstd/gun/compiler/context"
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
	span := hirExprSpan(e)
	var out ast.Expr
	switch e := e.(type) {
	case *hir.Identifier:
		out = l.lowerIdentifier(e)

	case *hir.Literal:
		out = l.lowerLiteral(e)

	case *hir.TemplateLiteral:
		out = l.lowerTemplateLiteral(e)

	case *hir.ArrayLiteral:
		out = l.lowerArrayLiteral(e)

	case *hir.ObjectLiteral:
		out = l.lowerObjectLiteral(e)

	case *hir.BinaryExpr:
		out = l.lowerBinaryExpr(e)

	case *hir.UnaryExpr:
		out = l.lowerUnaryExpr(e)

	case *hir.UpdateExpr:
		out = l.lowerUpdateExpr(e)

	case *hir.AssignExpr:
		out = l.lowerAssignExpr(e)

	case *hir.CallExpr:
		out = l.lowerCallExpr(e)

	case *hir.NewExpr:
		out = l.lowerNewExpr(e)

	case *hir.ClassExpr:
		out = l.lowerClassExpr(e)

	case *hir.MemberExpr:
		out = l.lowerMemberExpr(e)

	case *hir.ComputedMemberExpr:
		obj := l.lowerExpr(e.Object)
		out = callExpr(selectorExpr(obj, "Get"), l.lowerComputedPropertyKeyExpr(e.Property))

	case *hir.TernaryExpr:
		out = l.lowerTernaryExpr(e)

	case *hir.ArrowFunc:
		out = l.lowerArrowFunc(e)

	case *hir.FuncExpr:
		out = l.lowerFuncExpr(e)

	case *hir.SpreadExpr:
		out = l.lowerExpr(e.Value)

	case *hir.SequenceExpr:
		out = l.lowerSequenceExpr(e)

	case *hir.AwaitExpr:
		out = l.lowerExpr(e.Value)

	case *hir.YieldExpr:
		out = l.lowerExpr(e.Value)

	case *hir.TypeAssertExpr:
		out = l.lowerExpr(e.Expr)

	case *hir.NonNullExpr:
		out = l.lowerExpr(e.Expr)

	case *hir.ParenExpr:
		out = &ast.ParenExpr{X: l.lowerExpr(e.Expr)}

	case *hir.ThisExpr:
		if l.insideFunc == 0 {
			// Top-level this is undefined in ESM (SWC convention)
			l.jsvalueImport()
			out = callExpr(selectorExpr(goIdent("jsvalue"), "NewUndefined"))
			break
		}
		out = goIdent("this")

	case *hir.SuperExpr:
		if l.currentParentClass != "" {
			out = goIdent(l.currentParentClass)
		} else {
			out = goIdent("super")
		}

	case *hir.PrivateIdentifierExpr:
		out = l.privateKeyExpr(e.Name)

	case *hir.MetaPropertyExpr:
		if e.Meta == "import" && e.Property == "meta" {
			l.addImport("github.com/nnstd/gun/runtime/module")
			out = callExpr(selectorExpr(goIdent("module"), "ImportMetaForFile"), stringLit(l.sourcePath))
			break
		}
		out = goIdent("nil")

	case *hir.TaggedTemplateLiteral:
		out = l.lowerTaggedTemplate(e)

	default:
		out = goIdent("nil")
	}
	return setExprPos(out, span)
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
			if res.useAsJSValue {
				moduleObj := selectorExpr(goIdent(res.goPkgName), "AsJSValue")
				if res.moduleValue != "" {
					moduleObj = selectorExpr(goIdent(res.goPkgName), res.moduleValue)
				}
				if res.jsExportName == "" {
					return moduleObj
				}
				return callExpr(selectorExpr(moduleObj, "Get"), stringLit(res.jsExportName))
			}
			// Default/namespace import from Gun runtime module → pkg.AsJSValue
			if res.goSymbol == "AsJSValue" && !res.isTranspiled && isGunRuntimePkg(res.goImportPath) {
				return selectorExpr(goIdent(res.goPkgName), "AsJSValue")
			}
			// Namespace import (no goSymbol)
			if res.goSymbol == "" {
				if mapped, ok := l.importNameMap[res.modulePath+"\x00*"]; ok {
					return goIdent(mapped)
				}
				if res.goPkgName == "" {
					return goIdent(e.Sym.OriginalName)
				}
				return goIdent(res.goPkgName)
			}
			// Same-package reference
			if res.goPkgName == "" {
				if res.namespaceGet {
					return callExpr(selectorExpr(goIdent(res.goSymbol), "Get"), stringLit(res.jsExportName))
				}
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
	if res, ok := l.importedNames[e.Name]; ok {
		if res.goImportPath != "" {
			if res.goPkgName != "" && res.goPkgName != path.Base(res.goImportPath) {
				l.addAliasedImport(res.goImportPath, res.goPkgName)
			} else if res.goImportPath != "" {
				l.addImport(res.goImportPath)
			}
		}
		if res.useAsJSValue {
			moduleObj := selectorExpr(goIdent(res.goPkgName), "AsJSValue")
			if res.moduleValue != "" {
				moduleObj = selectorExpr(goIdent(res.goPkgName), res.moduleValue)
			}
			if res.jsExportName == "" {
				return moduleObj
			}
			return callExpr(selectorExpr(moduleObj, "Get"), stringLit(res.jsExportName))
		}
		if res.goSymbol == "AsJSValue" && !res.isTranspiled && isGunRuntimePkg(res.goImportPath) {
			return selectorExpr(goIdent(res.goPkgName), "AsJSValue")
		}
		if res.goSymbol == "" {
			if res.goPkgName == "" {
				return goIdent(e.Name)
			}
			return goIdent(res.goPkgName)
		}
		if res.goPkgName == "" {
			if res.namespaceGet {
				return callExpr(selectorExpr(goIdent(res.goSymbol), "Get"), stringLit(res.jsExportName))
			}
			return goIdent(res.goSymbol)
		}
		if !res.isTranspiled && res.goPkgName != "" && isGunRuntimePkg(res.goImportPath) {
			return callExpr(selectorExpr(
				selectorExpr(goIdent(res.goPkgName), "AsJSValue"),
				"Get"), stringLit(lowercaseFirst(res.goSymbol)))
		}
		return selectorExpr(goIdent(res.goPkgName), res.goSymbol)
	}
	if name, ok := l.topLevelNames[e.Name]; ok {
		return goIdent(name)
	}
	if name == "exports" && l.namespaceAlias != "" {
		l.jsvalueImport()
		return goIdent(l.namespaceAlias)
	}
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
			return l.arenaWrapNumber(floatLit(e.Value))
		}
		return l.arenaWrapNumber(callExpr(goIdent("float64"), intLit(e.Value)))
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
	l.addAliasedImport("fmt", "_gunFmt")
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
	formatted := callExpr(selectorExpr(goIdent("_gunFmt"), "Sprintf"), sprintfArgs...)
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
	hasSpread := false
	for _, elem := range e.Elements {
		if _, ok := elem.(*hir.SpreadExpr); ok {
			hasSpread = true
			break
		}
	}
	if hasSpread {
		var stmts []ast.Stmt
		stmts = append(stmts, assignDefine(
			[]ast.Expr{goIdent("_arr")},
			[]ast.Expr{callExpr(selectorExpr(goIdent("jsvalue"), "NewArray"))},
		))
		for _, elem := range e.Elements {
			if elem == nil {
				continue
			}
			if spread, ok := elem.(*hir.SpreadExpr); ok {
				val := l.lowerExpr(spread.Value)
				merged := callExpr(goIdent("append"),
					callExpr(selectorExpr(goIdent("_arr"), "Array")),
					callExpr(selectorExpr(val, "Array")),
				)
				merged.Ellipsis = 1
				newArr := callExpr(selectorExpr(goIdent("jsvalue"), "NewArray"), merged)
				newArr.Ellipsis = 1
				stmts = append(stmts, assignStmt([]ast.Expr{goIdent("_arr")}, []ast.Expr{newArr}))
				continue
			}
			val := jsvalueWrapLit(l.lowerExpr(elem))
			merged := callExpr(goIdent("append"),
				callExpr(selectorExpr(goIdent("_arr"), "Array")),
				val,
			)
			newArr := callExpr(selectorExpr(goIdent("jsvalue"), "NewArray"), merged)
			newArr.Ellipsis = 1
			stmts = append(stmts, assignStmt([]ast.Expr{goIdent("_arr")}, []ast.Expr{newArr}))
		}
		stmts = append(stmts, returnStmt(goIdent("_arr")))
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

	// Check for computed properties — need imperative construction
	hasComputed := false
	for _, prop := range e.Properties {
		if prop.Computed {
			hasComputed = true
			break
		}
	}
	if hasComputed {
		return l.lowerObjectWithComputed(e)
	}

	// Check for accessor properties (getters/setters)
	hasAccessor := false
	for _, prop := range e.Properties {
		if prop.IsGetter || prop.IsSetter {
			hasAccessor = true
			break
		}
	}
	if hasAccessor {
		return l.lowerObjectWithAccessors(e)
	}

	// jsvalue.ObjectFrom("key1", value1, "key2", value2, ...)
	var args []ast.Expr
	for _, prop := range e.Properties {
		args = append(args, stringLit(prop.KeyName), l.lowerObjectPropertyValue(prop))
	}
	return callExpr(selectorExpr(goIdent("jsvalue"), "ObjectFrom"), args...)
}

// lowerObjectWithAccessors handles object literals with getter/setter properties.
// Uses DefineProperty with get/set descriptors.
func (l *Lowerer) lowerObjectWithAccessors(e *hir.ObjectLiteral) ast.Expr {
	l.jsvalueImport()
	// Start with non-accessor properties in ObjectFrom
	var regularArgs []ast.Expr
	for _, prop := range e.Properties {
		if !prop.IsGetter && !prop.IsSetter {
			regularArgs = append(regularArgs, stringLit(prop.KeyName), l.lowerObjectPropertyValue(prop))
		}
	}
	result := callExpr(selectorExpr(goIdent("jsvalue"), "ObjectFrom"), regularArgs...)
	// Apply accessor properties via DefineProperty
	for _, prop := range e.Properties {
		if prop.IsGetter || prop.IsSetter {
			var descArgs []ast.Expr
			fnExpr := l.lowerExpr(prop.Value)
			if prop.IsGetter {
				descArgs = append(descArgs, stringLit("get"), fnExpr)
			}
			if prop.IsSetter {
				descArgs = append(descArgs, stringLit("set"), fnExpr)
			}
			result = callExpr(
				selectorExpr(goIdent("jsvalue"), "DefineProperty"),
				result,
				stringLit(prop.KeyName),
				callExpr(selectorExpr(goIdent("jsvalue"), "ObjectFrom"), descArgs...),
			)
		}
	}
	return result
}

// lowerObjectWithComputed handles object literals with computed property names
// like {[expr]: value}. Uses IIFE: func() *JSValue { obj := NewObject(); obj.Set(Sprint(expr), val); return obj }()
func (l *Lowerer) lowerObjectWithComputed(e *hir.ObjectLiteral) ast.Expr {
	l.jsvalueImport()
	l.addAliasedImport("fmt", "_gunFmt")
	var stmts []ast.Stmt
	stmts = append(stmts, assignDefine(
		[]ast.Expr{goIdent("_obj")},
		[]ast.Expr{callExpr(selectorExpr(goIdent("jsvalue"), "NewObject"))},
	))
	for _, prop := range e.Properties {
		value := l.lowerObjectPropertyValue(prop)
		var keyExpr ast.Expr
		if prop.Computed {
			keyExpr = callExpr(selectorExpr(goIdent("_gunFmt"), "Sprint"), l.lowerExpr(prop.Key))
		} else {
			keyExpr = stringLit(prop.KeyName)
		}
		stmts = append(stmts, exprStmt(
			callExpr(selectorExpr(goIdent("_obj"), "Set"), keyExpr, value),
		))
	}
	stmts = append(stmts, returnStmt(goIdent("_obj")))
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

// lowerObjectWithSpreads handles object literals with spread properties:
// {a: 1, ...obj, b: 2} → jsvalue.Assign(jsvalue.ObjectFrom("a", 1), obj, jsvalue.ObjectFrom("b", 2))
func (l *Lowerer) lowerObjectWithSpreads(e *hir.ObjectLiteral) ast.Expr {
	var args []ast.Expr
	var currentArgs []ast.Expr

	flush := func() {
		if len(currentArgs) > 0 {
			args = append(args, callExpr(selectorExpr(goIdent("jsvalue"), "ObjectFrom"), currentArgs...))
			currentArgs = nil
		}
	}

	for _, prop := range e.Properties {
		if spread, ok := prop.Key.(*hir.SpreadExpr); ok {
			flush()
			args = append(args, l.lowerExpr(spread.Value))
		} else {
			currentArgs = append(currentArgs, stringLit(prop.KeyName), l.lowerObjectPropertyValue(prop))
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
func (l *Lowerer) lowerObjectPropertyValue(prop *hir.Property) ast.Expr {
	if prop == nil || prop.Value == nil {
		return callExpr(selectorExpr(goIdent("jsvalue"), "NewUndefined"))
	}
	if !prop.Method {
		return l.lowerExpr(prop.Value)
	}
	switch fn := prop.Value.(type) {
	case *hir.ArrowFunc:
		body := fn.Body
		if body == nil {
			body = &hir.BlockStmt{Stmts: []hir.Stmt{&hir.ReturnStmt{Value: fn.ExprBody}}}
		}
		var methodBody *ast.BlockStmt
		if fn.IsAsync {
			methodBody = l.lowerAsyncFuncBody(fn.Params, body, 1, true)
		} else {
			methodBody = l.lowerMethodBody(fn.Params, body)
		}
		methodBody = l.instrumentProfiledBody(prop.KeyName, fn.Span, methodBody)
		methodLit := l.wrapAsJSValueFunc(fn.Params, methodBody)
		methodVal := l.generatedFunctionValue(prop.KeyName, fn.Span, methodLit)
		if fn.IsAsync {
			methodVal = callExpr(selectorExpr(methodVal, "MarkAsAsync"))
		}
		methodVal = callExpr(selectorExpr(methodVal, "MarkAsMethod"))
		return methodVal
	case *hir.FuncExpr:
		var methodBody *ast.BlockStmt
		if fn.IsAsync {
			methodBody = l.lowerAsyncFuncBody(fn.Params, fn.Body, 1, true)
		} else {
			methodBody = l.lowerMethodBody(fn.Params, fn.Body)
		}
		methodBody = l.instrumentProfiledBody(prop.KeyName, fn.Span, methodBody)
		methodLit := l.wrapAsJSValueFunc(fn.Params, methodBody)
		methodVal := l.generatedFunctionValue(prop.KeyName, fn.Span, methodLit)
		if fn.IsAsync {
			methodVal = callExpr(selectorExpr(methodVal, "MarkAsAsync"))
		}
		methodVal = callExpr(selectorExpr(methodVal, "MarkAsMethod"))
		return methodVal
	default:
		return l.lowerExpr(prop.Value)
	}
}
func (l *Lowerer) lowerBinaryExpr(e *hir.BinaryExpr) ast.Expr {
	l.jsvalueImport()

	// Short-circuit operators must evaluate RHS lazily when the RHS
	// has side effects. Go evaluates function arguments eagerly, so
	// `jsvalue.Nullish(a, sideEffect())` would run the side effect
	// even when `a` is defined. Emit an IIFE only in that case to
	// avoid bloating compile time on large pure expression trees
	// (e.g. unicode data tables).
	switch e.Op {
	case hir.OpOr, hir.OpAnd, hir.OpNullish:
		if hirExprHasSideEffects(e.Right) {
			return l.lowerShortCircuitBinary(e)
		}
	}

	left := l.lowerExpr(e.Left)
	right := l.lowerExpr(e.Right)

	// Special operators
	switch e.Op {
	case hir.OpIn:
		if priv, ok := e.Left.(*hir.PrivateIdentifierExpr); ok {
			return callExpr(selectorExpr(goIdent("jsvalue"), "NewBool"),
				&ast.BinaryExpr{
					X:  l.privateBrandCheck(right),
					Op: token.LAND,
					Y:  callExpr(selectorExpr(right, "HasOwnProperty"), l.privateKeyExpr(priv.Name)),
				})
		}
		// key in obj → jsvalue.NewBool(obj.HasOwnProperty(fmt.Sprint(key)))
		l.addAliasedImport("fmt", "_gunFmt")
		return callExpr(selectorExpr(goIdent("jsvalue"), "NewBool"),
			callExpr(selectorExpr(right, "HasOwnProperty"),
				callExpr(selectorExpr(goIdent("_gunFmt"), "Sprint"), left)))
	case hir.OpInstanceof:
		if id, ok := e.Right.(*hir.Identifier); ok {
			name := id.Name
			if id.Sym != nil {
				name = id.Sym.OriginalName
			}
			if name == "Promise" {
				return callExpr(selectorExpr(goIdent("jsvalue"), "NewBool"),
					&ast.BinaryExpr{
						X:  callExpr(selectorExpr(callExpr(selectorExpr(left, "Get"), stringLit("then")), "TypeString")),
						Op: token.EQL,
						Y:  stringLit("function"),
					},
				)
			}
		}
		return callExpr(selectorExpr(goIdent("jsvalue"), "InstanceOf"),
			jsvalueWrapLit(left), jsvalueWrapLit(right))
	}

	// JSValue binary operations: jsvalue.Op(left, right)
	if l.arenaEnabled && l.insideFunc > 0 && l.disableArenaCount == 0 && l.hasArenaVar > 0 {
		if arenaHelper := l.arenaBinaryHelperName(e.Op); arenaHelper != "" {
			return callExpr(selectorExpr(goIdent("jsvalue"), arenaHelper), goIdent("_arena"),
				jsvalueWrapLit(left), jsvalueWrapLit(right))
		}
	}
	helperName := mapBinaryOpToJSValue(e.Op)
	return callExpr(selectorExpr(goIdent("jsvalue"), helperName),
		jsvalueWrapLit(left), jsvalueWrapLit(right))
}

// hirExprHasSideEffects returns true when evaluating e could have an
// observable side effect (a call, assignment, new, await, etc.). Used to
// decide whether short-circuit operators need lazy IIFE lowering.
func hirExprHasSideEffects(e hir.Expr) bool {
	switch n := e.(type) {
	case nil:
		return false
	case *hir.CallExpr, *hir.NewExpr, *hir.AssignExpr, *hir.UpdateExpr,
		*hir.AwaitExpr, *hir.YieldExpr, *hir.ArrowFunc, *hir.FuncExpr, *hir.ClassExpr:
		return true
	case *hir.BinaryExpr:
		return hirExprHasSideEffects(n.Left) || hirExprHasSideEffects(n.Right)
	case *hir.UnaryExpr:
		return hirExprHasSideEffects(n.Operand)
	case *hir.TernaryExpr:
		return hirExprHasSideEffects(n.Cond) || hirExprHasSideEffects(n.Then) || hirExprHasSideEffects(n.Else)
	case *hir.MemberExpr:
		return hirExprHasSideEffects(n.Object)
	case *hir.ComputedMemberExpr:
		return hirExprHasSideEffects(n.Object) || hirExprHasSideEffects(n.Property)
	case *hir.SequenceExpr:
		for _, x := range n.Exprs {
			if hirExprHasSideEffects(x) {
				return true
			}
		}
		return false
	case *hir.ParenExpr:
		return hirExprHasSideEffects(n.Expr)
	case *hir.TypeAssertExpr:
		return hirExprHasSideEffects(n.Expr)
	case *hir.NonNullExpr:
		return hirExprHasSideEffects(n.Expr)
	case *hir.SpreadExpr:
		return hirExprHasSideEffects(n.Value)
	case *hir.TemplateLiteral:
		for _, x := range n.Parts {
			if hirExprHasSideEffects(x) {
				return true
			}
		}
		return false
	case *hir.ArrayLiteral:
		for _, el := range n.Elements {
			if hirExprHasSideEffects(el) {
				return true
			}
		}
		return false
	case *hir.ObjectLiteral:
		for _, p := range n.Properties {
			if hirExprHasSideEffects(p.Value) {
				return true
			}
			if p.Computed && hirExprHasSideEffects(p.Key) {
				return true
			}
		}
		return false
	}
	return false
}

// lowerShortCircuitBinary emits a lazy IIFE for ||, &&, ??. The RHS is only
// evaluated when the LHS cannot determine the result, matching JS semantics.
func (l *Lowerer) lowerShortCircuitBinary(e *hir.BinaryExpr) ast.Expr {
	l.jsvalueImport()

	left := jsvalueWrapLit(l.lowerExpr(e.Left))
	right := jsvalueWrapLit(l.lowerExpr(e.Right))

	// Build: if <lhsKeeps> { return _l }
	var keepLHSCond ast.Expr
	switch e.Op {
	case hir.OpOr:
		// lhs truthy → keep lhs: _l.Bool()
		keepLHSCond = callExpr(selectorExpr(goIdent("_l"), "Bool"))
	case hir.OpAnd:
		// lhs falsy → keep lhs: !_l.Bool()
		keepLHSCond = &ast.UnaryExpr{Op: token.NOT, X: callExpr(selectorExpr(goIdent("_l"), "Bool"))}
	case hir.OpNullish:
		// lhs not nullish → keep lhs: !jsvalue.IsNullish(_l)
		keepLHSCond = &ast.UnaryExpr{Op: token.NOT, X: callExpr(selectorExpr(goIdent("jsvalue"), "IsNullish"), goIdent("_l"))}
	}

	body := blockStmt(
		&ast.AssignStmt{
			Lhs: []ast.Expr{goIdent("_l")},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{left},
		},
		&ast.IfStmt{
			Cond: keepLHSCond,
			Body: blockStmt(returnStmt(goIdent("_l"))),
		},
		returnStmt(right),
	)

	return &ast.CallExpr{
		Fun: &ast.FuncLit{
			Type: &ast.FuncType{
				Params:  fieldList(),
				Results: fieldList(goField("", jsValuePtrType())),
			},
			Body: body,
		},
	}
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

	if e.Op == hir.OpNeg && l.arenaEnabled && l.insideFunc > 0 && l.disableArenaCount == 0 && l.hasArenaVar > 0 {
		return callExpr(selectorExpr(goIdent("jsvalue"), "ANeg"), goIdent("_arena"), jsvalueWrapLit(operand))
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
		if l.arenaEnabled && l.insideFunc > 0 && l.disableArenaCount == 0 && l.hasArenaVar > 0 {
			return callExpr(selectorExpr(goIdent("jsvalue"), "AInc"), goIdent("_arena"), operand)
		}
		return callExpr(selectorExpr(goIdent("jsvalue"), "Inc"), operand)
	}
	if l.arenaEnabled && l.insideFunc > 0 && l.disableArenaCount == 0 && l.hasArenaVar > 0 {
		return callExpr(selectorExpr(goIdent("jsvalue"), "ADec"), goIdent("_arena"), operand)
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

	if mem, ok := e.Left.(*hir.MemberExpr); ok && mem.Private && l.exprIsJSValue(mem.Object) {
		return l.lowerPrivateAssignExpr(mem, e.Op, e.Right, right)
	}

	// Member assignment expression: obj.prop = val / obj.prop += val / obj.#x ??= val
	if mem, ok := e.Left.(*hir.MemberExpr); ok && l.exprIsJSValue(mem.Object) {
		l.jsvalueImport()
		obj := l.lowerExpr(mem.Object)
		val := l.wrapAsJSValue(right)
		if e.Op != hir.OpAssign {
			helperName := mapAssignOpToJSValue(e.Op)
			current := callExpr(selectorExpr(obj, "Get"), l.lowerClassMemberKey(mem.Property, mem.Private, nil))
			val = callExpr(selectorExpr(goIdent("jsvalue"), helperName), current, val)
		}
		tmp := l.nextSyntheticName("_v")
		setCall := callExpr(selectorExpr(obj, "Set"), l.lowerClassMemberKey(mem.Property, mem.Private, nil), goIdent(tmp))
		return &ast.CallExpr{
			Fun: &ast.FuncLit{
				Type: &ast.FuncType{Params: fieldList(), Results: fieldList(goField("", jsValuePtrType()))},
				Body: blockStmt(
					assignDefine([]ast.Expr{goIdent(tmp)}, []ast.Expr{val}),
					exprStmt(setCall),
					returnStmt(goIdent(tmp)),
				),
			},
		}
	}

	// Computed member assignment: obj[key] = val → IIFE { tmp := val; obj.Set(key, tmp); return tmp }
	if comp, ok := e.Left.(*hir.ComputedMemberExpr); ok && l.exprIsJSValue(comp.Object) {
		l.jsvalueImport()
		obj := l.lowerExpr(comp.Object)
		key := l.lowerComputedPropertyKeyExpr(comp.Property)
		val := l.wrapAsJSValue(right)
		if e.Op != hir.OpAssign {
			helperName := mapAssignOpToJSValue(e.Op)
			current := callExpr(selectorExpr(obj, "Get"), key)
			val = callExpr(selectorExpr(goIdent("jsvalue"), helperName), current, val)
		}
		tmp := l.nextSyntheticName("_v")
		setCall := callExpr(selectorExpr(obj, "Set"), key, goIdent(tmp))
		return &ast.CallExpr{
			Fun: &ast.FuncLit{
				Type: &ast.FuncType{Params: fieldList(), Results: fieldList(goField("", jsValuePtrType()))},
				Body: blockStmt(
					assignDefine([]ast.Expr{goIdent(tmp)}, []ast.Expr{val}),
					exprStmt(setCall),
					returnStmt(goIdent(tmp)),
				),
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
		if !mem.Private {
			if id, ok := mem.Object.(*hir.Identifier); ok {
				objName := id.Name
				if id.Sym != nil {
					objName = id.Sym.OriginalName
				}
				if l.ctx != nil {
					var args []ast.Expr
					hasSpread := false
					for _, a := range e.Args {
						if _, ok := a.(*hir.SpreadExpr); ok {
							hasSpread = true
							break
						}
					}
					if hasSpread {
						args, hasSpread = l.lowerCallArgs(e.Args, true)
					} else {
						for _, a := range e.Args {
							args = append(args, l.lowerExpr(a))
						}
					}
					if objName == "Bun" && mem.Property == "serve" {
						l.needsBunWait = true
					}
					if result := l.ctx.TransformBuiltinCall(objName, mem.Property, args, hasSpread, l); result != nil {
						return result
					}
				}
			}
		}

		// Same-package namespace method call: ns.foo(args) → DirectVar.Call(args)
		// Resolve at compile time to avoid runtime dependency on namespace object.
		if id, ok := mem.Object.(*hir.Identifier); ok && id.Sym != nil {
			if res, ok := l.importedSyms[id.Sym]; ok && res.isTranspiled && res.goImportPath == "" {
				if mapped, ok := l.importNameMap[res.modulePath+"\x00"+mem.Property]; ok {
					l.jsvalueImport()
					argExprs, hasSpread := l.lowerCallArgs(e.Args, true)
					return buildCallWithSpread(selectorExpr(goIdent(mapped), "Call"), argExprs, hasSpread)
				}
			}
		}

		// JSValue method dispatch: obj.method(args) → obj.MethodCall("method", wrappedArgs...)
		if l.exprIsJSValue(mem.Object) {
			argExprs, hasSpread := l.lowerCallArgs(e.Args, true)
			if mem.Private {
				return l.lowerPrivateMethodCall(mem, argExprs, hasSpread)
			}
			l.jsvalueImport()
			obj := l.lowerExpr(mem.Object)
			wrappedArgs := append([]ast.Expr{l.lowerClassMemberKey(mem.Property, mem.Private, nil)}, argExprs...)
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

	// Computed member call: obj[key](args) → obj.MethodCall(fmt.Sprint(key), args...)
	// This ensures 'this' is passed correctly for method calls via computed keys.
	if comp, ok := e.Func.(*hir.ComputedMemberExpr); ok && l.exprIsJSValue(comp.Object) {
		l.jsvalueImport()
		obj := l.lowerExpr(comp.Object)
		argExprs, hasSpread := l.lowerCallArgs(e.Args, true)
		methodKey := l.lowerComputedPropertyKeyExpr(comp.Property)
		wrappedArgs := append([]ast.Expr{methodKey}, argExprs...)
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
			if l.ctx.IsKnownGlobal(name) && l.ctx.LookupIdentifier(name) != nil {
				fn := l.lowerIdentifier(id)
				wrappedArgs, hasSpread := l.lowerCallArgs(e.Args, true)
				return buildCallWithSpread(selectorExpr(fn, "Call"), wrappedArgs, hasSpread)
			}
		}

		// JSValue function call: fn(args) → fn.Call(wrappedArgs...)
		if id.Sym != nil {
			isJSValueFunc := false
			if res, ok := l.importedSyms[id.Sym]; ok && res.isTranspiled {
				isJSValueFunc = true
			} else if id.Sym.Kind == symbol.KindVariable || id.Sym.Kind == symbol.KindParameter || id.Sym.Kind == symbol.KindImport {
				isJSValueFunc = true
			} else if id.Sym.Kind == symbol.KindFunction {
				// In all-JSValue architecture, all functions are JSValue (except main/init at top level)
				emittedName := l.emitName(id.Sym)
				if emittedName != "main" && emittedName != "init" {
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
		limit := len(args)
		if hasSpread && limit > 0 {
			limit--
		}
		for i := 0; i < limit; i++ {
			args[i] = jsvalueWrapLit(args[i])
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
				limit := len(args)
				if hasSpread && limit > 0 {
					limit--
				}
				for i := 0; i < limit; i++ {
					args[i] = l.wrapAsJSValue(args[i])
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
	if mem, ok := e.Callee.(*hir.MemberExpr); ok {
		if obj, ok := mem.Object.(*hir.Identifier); ok {
			name := symbol.Capitalize(obj.Name) + symbol.Capitalize(mem.Property)
			if obj.Sym != nil {
				name = symbol.Capitalize(obj.Sym.OriginalName) + symbol.Capitalize(mem.Property)
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
	}

	callee := l.lowerExpr(e.Callee)
	var args []ast.Expr
	for _, a := range e.Args {
		arg := l.lowerExpr(a)
		args = append(args, jsvalueWrapLit(arg))
	}
	return callExpr(selectorExpr(callee, "New"), args...)
}
func (l *Lowerer) lowerClassExpr(e *hir.ClassExpr) ast.Expr {
	l.jsvalueImport()

	symbolicName := e.Name
	if symbolicName == "" {
		symbolicName = fmt.Sprintf("anonymousClass%d", l.syntheticCounter+1)
	}
	brandKey := l.nextSyntheticName(fmt.Sprintf("_brand_%s", symbol.Sanitize(symbolicName)))
	privateKeys := l.collectPrivateKeys(symbolicName, e.Properties, e.Methods)

	prevClassName := l.currentClassName
	prevClassBrand := l.currentClassBrand
	prevPrivateKeys := l.privateKeys
	l.privateKeys = privateKeys
	defer func() {
		l.currentClassName = prevClassName
		l.currentClassBrand = prevClassBrand
		l.privateKeys = prevPrivateKeys
	}()

	var parentExpr ast.Expr = goIdent("nil")
	if e.Parent != nil {
		parentExpr = l.lowerExpr(e.Parent)
	}

	stmts := make([]ast.Stmt, 0, len(privateKeys)+4)
	stmts = append(stmts, assignDefine([]ast.Expr{goIdent(brandKey)}, []ast.Expr{l.brandKeyValue(symbolicName)}))
	for member, goName := range privateKeys {
		desc := fmt.Sprintf("%s.#%s", symbolicName, member)
		value := callExpr(
			selectorExpr(goIdent("jsvalue"), "PropertyKey"),
			callExpr(selectorExpr(goIdent("jsvalue"), "NewSymbol"), stringLit(desc)),
		)
		stmts = append(stmts, assignDefine([]ast.Expr{goIdent(goName)}, []ast.Expr{value}))
	}

	classVarName := "_class"
	stmts = append(stmts, &ast.DeclStmt{Decl: varDecl(classVarName, jsValuePtrType(), nil)})
	if e.Name != "" {
		stmts = append(stmts, &ast.DeclStmt{Decl: varDecl(symbol.Sanitize(e.Name), jsValuePtrType(), nil)})
	}
	l.currentClassName = classVarName
	l.currentClassBrand = brandKey
	ctorLit := l.lowerClassConstructor(classVarName, e.Parent != nil, e.Constructor, e.Properties, e.Methods)
	stmts = append(stmts,
		assignStmt([]ast.Expr{goIdent(classVarName)}, []ast.Expr{
			callExpr(selectorExpr(goIdent("jsvalue"), "NewClass"), ctorLit, parentExpr),
		}),
	)
	returnVar := goIdent(classVarName)
	if e.Name != "" {
		namedVar := goIdent(symbol.Sanitize(e.Name))
		stmts = append(stmts, assignStmt([]ast.Expr{namedVar}, []ast.Expr{goIdent(classVarName)}))
		returnVar = namedVar
	}
	stmts = append(stmts, l.lowerClassSetups(goIdent(classVarName), goIdent(brandKey), e.Properties, e.Methods, e.StaticInits)...)
	stmts = append(stmts, returnStmt(returnVar))

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
func (l *Lowerer) lowerPrivateGet(mem *hir.MemberExpr) ast.Expr {
	l.jsvalueImport()
	key := l.lowerClassMemberKey(mem.Property, true, nil)
	objExpr := l.lowerExpr(mem.Object)

	// O1+: skip brand check for this.#field
	if l.optLevel >= context.O1 && l.isThisExpr(mem.Object) {
		return callExpr(selectorExpr(objExpr, "Get"), key)
	}

	l.addAliasedImport("github.com/nnstd/gun/runtime/builtin/error", "error")
	access := describeHIRExpr(mem.Object) + ".#" + mem.Property
	marker := l.lineDirectiveMarker(mem.Span)
	brandCheck := l.privateBrandCheck(goIdent("_obj"))
	hasKey := callExpr(selectorExpr(goIdent("_obj"), "HasOwnProperty"), key)
	invalidCond := &ast.BinaryExpr{
		X:  brandCheck,
		Op: token.LAND,
		Y:  hasKey,
	}
	ifBody := []ast.Stmt{}
	if marker != nil {
		ifBody = append(ifBody, marker)
	}
	ifBody = append(ifBody, exprStmt(callExpr(goIdent("panic"), callExpr(selectorExpr(goIdent("error"), "InvalidPrivateField"), stringLit(access)))))
	body := []ast.Stmt{}
	if marker != nil {
		body = append(body, marker)
	}
	body = append(body,
		&ast.IfStmt{
			Cond: &ast.UnaryExpr{Op: token.NOT, X: invalidCond},
			Body: blockStmt(ifBody...),
		},
	)
	if marker != nil {
		body = append(body, marker)
	}
	body = append(body, returnStmt(callExpr(selectorExpr(goIdent("_obj"), "Get"), key)))
	return callExpr(&ast.FuncLit{
		Type: &ast.FuncType{
			Params:  fieldList(goField("_obj", jsValuePtrType())),
			Results: fieldList(goField("", jsValuePtrType())),
		},
		Body: blockStmt(body...),
	}, objExpr)
}
func (l *Lowerer) lowerPrivateAssignExpr(mem *hir.MemberExpr, op hir.AssignOp, rhsHIR hir.Expr, rhs ast.Expr) ast.Expr {
	l.jsvalueImport()
	key := l.lowerClassMemberKey(mem.Property, true, nil)
	objExpr := l.lowerExpr(mem.Object)

	// O1+: skip brand check for this.#field = val
	if l.optLevel >= context.O1 && l.isThisExpr(mem.Object) {
		val := l.wrapAsJSValue(rhs)
		if op != hir.OpAssign {
			helperName := mapAssignOpToJSValue(op)
			current := callExpr(selectorExpr(objExpr, "Get"), key)
			val = callExpr(selectorExpr(goIdent("jsvalue"), helperName), current, val)
		}
		return callExpr(&ast.FuncLit{
			Type: &ast.FuncType{
				Params:  fieldList(),
				Results: fieldList(goField("", jsValuePtrType())),
			},
			Body: blockStmt(
				exprStmt(callExpr(selectorExpr(objExpr, "Set"), key, val)),
				returnStmt(val),
			),
		})
	}

	l.addAliasedImport("github.com/nnstd/gun/runtime/builtin/error", "error")
	access := describeHIRExpr(mem.Object) + ".#" + mem.Property + " = " + describeHIRExpr(rhsHIR)
	marker := l.lineDirectiveMarker(mem.Span)
	brandCheck := l.privateBrandCheck(goIdent("_obj"))
	hasKey := callExpr(selectorExpr(goIdent("_obj"), "HasOwnProperty"), key)
	validCond := &ast.BinaryExpr{X: brandCheck, Op: token.LAND, Y: hasKey}
	return callExpr(&ast.FuncLit{
		Type: &ast.FuncType{
			Params:  fieldList(goField("_obj", jsValuePtrType())),
			Results: fieldList(goField("", jsValuePtrType())),
		},
		Body: func() *ast.BlockStmt {
			val := l.wrapAsJSValue(rhs)
			if op != hir.OpAssign {
				helperName := mapAssignOpToJSValue(op)
				current := callExpr(selectorExpr(goIdent("_obj"), "Get"), key)
				val = callExpr(selectorExpr(goIdent("jsvalue"), helperName), current, val)
			}
			ifBody := []ast.Stmt{}
			if marker != nil {
				ifBody = append(ifBody, marker)
			}
			ifBody = append(ifBody, exprStmt(callExpr(goIdent("panic"), callExpr(selectorExpr(goIdent("error"), "InvalidPrivateField"), stringLit(access)))))
			body := []ast.Stmt{}
			if marker != nil {
				body = append(body, marker)
			}
			body = append(body, &ast.IfStmt{
				Cond: &ast.UnaryExpr{Op: token.NOT, X: validCond},
				Body: blockStmt(ifBody...),
			})
			if marker != nil {
				body = append(body, marker)
			}
			body = append(body, exprStmt(callExpr(selectorExpr(goIdent("_obj"), "Set"), key, val)))
			if marker != nil {
				body = append(body, marker)
			}
			body = append(body, returnStmt(val))
			return blockStmt(body...)
		}(),
	}, objExpr)
}
func (l *Lowerer) lowerPrivateMethodCall(mem *hir.MemberExpr, args []ast.Expr, hasSpread bool) ast.Expr {
	l.jsvalueImport()
	key := l.lowerClassMemberKey(mem.Property, true, nil)
	objExpr := l.lowerExpr(mem.Object)
	callArgs := append([]ast.Expr{key}, args...)

	// O1+: skip brand check for this.#method()
	if l.optLevel >= context.O1 && l.isThisExpr(mem.Object) {
		return buildCallWithSpread(selectorExpr(objExpr, "MethodCall"), callArgs, hasSpread)
	}

	l.addAliasedImport("github.com/nnstd/gun/runtime/builtin/error", "error")
	access := describeHIRExpr(mem.Object) + ".#" + mem.Property
	marker := l.lineDirectiveMarker(mem.Span)
	brandCheck := l.privateBrandCheck(goIdent("_obj"))
	hasKey := callExpr(selectorExpr(goIdent("_obj"), "HasOwnProperty"), key)
	validCond := &ast.BinaryExpr{X: brandCheck, Op: token.LAND, Y: hasKey}
	ifBody := []ast.Stmt{}
	if marker != nil {
		ifBody = append(ifBody, marker)
	}
	ifBody = append(ifBody, exprStmt(callExpr(goIdent("panic"), callExpr(selectorExpr(goIdent("error"), "InvalidPrivateMethodOrAccessor"), stringLit(access)))))
	body := []ast.Stmt{}
	if marker != nil {
		body = append(body, marker)
	}
	body = append(body, &ast.IfStmt{
		Cond: &ast.UnaryExpr{Op: token.NOT, X: validCond},
		Body: blockStmt(ifBody...),
	})
	if marker != nil {
		body = append(body, marker)
	}
	body = append(body, returnStmt(buildCallWithSpread(selectorExpr(goIdent("_obj"), "MethodCall"), callArgs, hasSpread)))
	return callExpr(&ast.FuncLit{
		Type: &ast.FuncType{
			Params:  fieldList(goField("_obj", jsValuePtrType())),
			Results: fieldList(goField("", jsValuePtrType())),
		},
		Body: blockStmt(body...),
	}, objExpr)
}
func (l *Lowerer) isThisExpr(e hir.Expr) bool {
	_, ok := e.(*hir.ThisExpr)
	return ok
}
func staticComputedPropertyKey(e hir.Expr) (string, bool) {
	switch e := e.(type) {
	case *hir.Literal:
		switch e.Kind {
		case hir.LitString:
			return e.Value, true
		case hir.LitNumber:
			clean := strings.ReplaceAll(e.Value, "_", "")
			if clean == "" || strings.ContainsAny(clean, ".eE") {
				return "", false
			}
			n, err := strconv.ParseInt(clean, 10, 64)
			if err != nil || n < 0 {
				return "", false
			}
			return strconv.FormatInt(n, 10), true
		}
	case *hir.ParenExpr:
		return staticComputedPropertyKey(e.Expr)
	}
	return "", false
}
func (l *Lowerer) lowerComputedPropertyKeyExpr(e hir.Expr) ast.Expr {
	if key, ok := staticComputedPropertyKey(e); ok {
		return stringLit(key)
	}
	if l.exprIsJSValue(e) {
		l.jsvalueImport()
		return callExpr(selectorExpr(goIdent("jsvalue"), "PropertyKey"), l.lowerExpr(e))
	}
	l.addAliasedImport("fmt", "_gunFmt")
	return callExpr(selectorExpr(goIdent("_gunFmt"), "Sprint"), l.lowerExpr(e))
}
func describeHIRExpr(e hir.Expr) string {
	switch e := e.(type) {
	case *hir.Identifier:
		if e.Sym != nil {
			return e.Sym.OriginalName
		}
		return e.Name
	case *hir.ThisExpr:
		return "this"
	case *hir.MemberExpr:
		if e.Private {
			return describeHIRExpr(e.Object) + ".#" + e.Property
		}
		return describeHIRExpr(e.Object) + "." + e.Property
	case *hir.ComputedMemberExpr:
		return describeHIRExpr(e.Object) + "[...]"
	case *hir.CallExpr:
		return describeHIRExpr(e.Func) + "(...)"
	case *hir.NewExpr:
		return "new " + describeHIRExpr(e.Callee)
	case *hir.Literal:
		return e.Value
	case *hir.ParenExpr:
		return "(" + describeHIRExpr(e.Expr) + ")"
	default:
		return "value"
	}
}
func (l *Lowerer) lowerMemberExpr(e *hir.MemberExpr) ast.Expr {
	// Optional chaining: a?.b → IIFE with null check
	if e.Optional {
		return l.lowerOptionalMember(e)
	}

	// Same-package namespace import: templates.foo → direct package-level var
	// This avoids going through the namespace object (which may not be
	// initialized yet during init()) and instead resolves at compile time.
	if id, ok := e.Object.(*hir.Identifier); ok && id.Sym != nil {
		if res, ok := l.importedSyms[id.Sym]; ok && res.isTranspiled && res.goImportPath == "" {
			if mapped, ok := l.importNameMap[res.modulePath+"\x00"+e.Property]; ok {
				return goIdent(mapped)
			}
			if res.goSymbol == "" {
				return goIdent(symbol.Capitalize(symbol.Sanitize(e.Property)))
			}
		}
	}

	// Check for builtin member access through context (e.g. process.env)
	if !e.Private {
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
	}

	obj := l.lowerExpr(e.Object)
	key := l.lowerClassMemberKey(e.Property, e.Private, nil)

	if e.Private {
		return l.lowerPrivateGet(e)
	}

		// .length -> .Get("length") for JSValue, len() for typed
		if !e.Private && e.Property == "length" {
			if l.exprIsJSValue(e.Object) {
				l.jsvalueImport()
				return callExpr(selectorExpr(obj, "Get"), stringLit("length"))
			}
			return callExpr(goIdent("len"), obj)
		}

	// JSValue receivers → .Get("prop")
	if l.exprIsJSValue(e.Object) {
		l.jsvalueImport()
		return callExpr(selectorExpr(obj, "Get"), key)
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
			return callExpr(selectorExpr(obj, "Get"), key)
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
	return callExpr(selectorExpr(obj, "Get"), key)
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
				returnStmt(callExpr(selectorExpr(goIdent(tmpName), "Get"), l.lowerClassMemberKey(e.Property, e.Private, nil))),
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
			if res.goSymbol == "AsJSValue" || res.useAsJSValue {
				return true
			}
			return res.isTranspiled
		}
		if e.Sym.Kind == symbol.KindParameter || e.Sym.Kind == symbol.KindVariable || e.Sym.Kind == symbol.KindImport || e.Sym.Kind == symbol.KindFunction || e.Sym.Kind == symbol.KindClass {
			return true
		}
		return false
	case *hir.ThisExpr, *hir.CallExpr, *hir.NewExpr, *hir.ClassExpr, *hir.MemberExpr,
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
		if e.IsAsync {
			body = l.lowerAsyncFuncBody(e.Params, e.Body, 0, false)
		} else {
			body = l.lowerFuncBody(e.Params, e.Body)
		}
	} else if e.ExprBody != nil {
		// Concise body: () => expr → wrap as return statement in HIR body
		val := e.ExprBody
		hirBody := &hir.BlockStmt{
			Stmts: []hir.Stmt{&hir.ReturnStmt{Value: val}},
		}
		if e.IsAsync {
			body = l.lowerAsyncFuncBody(e.Params, hirBody, 0, false)
		} else {
			body = l.lowerFuncBody(e.Params, hirBody)
		}
	} else {
		body = blockStmt()
	}
	body = l.instrumentProfiledBody("(anonymous)", e.Span, body)

	fnLit := l.wrapAsJSValueFunc(e.Params, body)
	fnVal := l.generatedFunctionValue("(anonymous)", e.Span, fnLit)
	if e.IsAsync {
		fnVal = callExpr(selectorExpr(fnVal, "MarkAsAsync"))
	}
	return fnVal
}
func (l *Lowerer) lowerFuncExpr(e *hir.FuncExpr) ast.Expr {
	l.jsvalueImport()

	// Regular function expressions get their own `this` binding (unlike arrow functions).
	// If the body references `this`, use lowerMethodBody to unpack it from _args[0]
	// and mark as method so callers (MethodCall, New) prepend `this`.
	usesThis := e.Body != nil && hirBodyUsesThis(e.Body)
	var body *ast.BlockStmt
	if e.IsAsync {
		body = l.lowerAsyncFuncBody(e.Params, e.Body, 0, usesThis)
	} else if usesThis {
		body = l.lowerMethodBody(e.Params, e.Body)
	} else {
		body = l.lowerFuncBody(e.Params, e.Body)
	}
	body = l.instrumentProfiledBody("(anonymous)", e.Span, body)
	fnLit := l.wrapAsJSValueFunc(e.Params, body)
	newFunc := l.generatedFunctionValue("(anonymous)", e.Span, fnLit)
	if e.IsAsync {
		newFunc = callExpr(selectorExpr(newFunc, "MarkAsAsync"))
	}
	if usesThis {
		newFunc = callExpr(selectorExpr(newFunc, "MarkAsMethod"))
	}

	// Named function expression: wrap in IIFE so the name is in scope for recursion.
	if e.Name != "" && e.Symbol != nil {
		nameStr := l.emitName(e.Symbol)
		declIdent := goIdent(nameStr)
		assignIdent := goIdent(nameStr)
		retIdent := goIdent(nameStr)
		return callExpr(&ast.FuncLit{
			Type: &ast.FuncType{
				Params: fieldList(),
				Results: fieldList(&ast.Field{
					Type: jsValuePtrType(),
				}),
			},
			Body: &ast.BlockStmt{
				List: []ast.Stmt{
					&ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{declIdent}, Type: jsValuePtrType()}}}},
					assignStmt([]ast.Expr{assignIdent}, []ast.Expr{newFunc}),
					&ast.ReturnStmt{Results: []ast.Expr{retIdent}},
				},
			},
		})
	}

	return newFunc
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
func (l *Lowerer) SetNeedsGlobalSync() {
	l.needsGlobalSync = true
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
func (l *Lowerer) arenaWrapNumber(expr ast.Expr) ast.Expr {
	if l.arenaEnabled && l.insideFunc > 0 && l.disableArenaCount == 0 && l.hasArenaVar > 0 {
		return callExpr(selectorExpr(goIdent("_arena"), "NewNumber"), expr)
	}
	return callExpr(selectorExpr(goIdent("jsvalue"), "NewNumber"), expr)
}
func (l *Lowerer) arenaBinaryHelperName(op hir.BinaryOp) string {
	switch op {
	case hir.OpAdd:
		return "AAdd"
	case hir.OpSub:
		return "ASub"
	case hir.OpMul:
		return "AMul"
	case hir.OpDiv:
		return "ADiv"
	case hir.OpMod:
		return "AMod"
	case hir.OpEq:
		return "AEq"
	case hir.OpNEq:
		return "ANEq"
	case hir.OpLt:
		return "ALt"
	case hir.OpGt:
		return "AGt"
	case hir.OpLtE:
		return "ALtE"
	case hir.OpGtE:
		return "AGtE"
	default:
		return ""
	}
}
