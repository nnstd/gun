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
		return goIdent("this")

	case *hir.SuperExpr:
		return goIdent("super")

	case *hir.MetaPropertyExpr:
		if e.Meta == "import" && e.Property == "meta" {
			l.addImport("github.com/nnstd/gun/runtime/module")
			return selectorExpr(goIdent("module"), "ImportMeta")
		}
		return goIdent("nil")

	case *hir.TaggedTemplateLiteral:
		tag := l.lowerExpr(e.Tag)
		tmpl := l.lowerExpr(e.Template)
		return callExpr(tag, tmpl)

	default:
		return goIdent("nil")
	}
}

func (l *Lowerer) lowerIdentifier(e *hir.Identifier) ast.Expr {
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
			// Named import: pkg.Symbol
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
	return goIdent(e.Name)
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
		return callExpr(selectorExpr(goIdent("jsvalue"), "NewRegex"),
			callExpr(selectorExpr(goIdent("jsvalue"), "CompileRegex"), stringLit(e.Value)))
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
			format.WriteString("%v")
			args = append(args, l.lowerExpr(part))
		}
	}

	sprintfArgs := append([]ast.Expr{stringLit(format.String())}, args...)
	formatted := callExpr(selectorExpr(goIdent("fmt"), "Sprintf"), sprintfArgs...)
	return callExpr(selectorExpr(goIdent("jsvalue"), "NewString"), formatted)
}

func (l *Lowerer) lowerArrayLiteral(e *hir.ArrayLiteral) ast.Expr {
	l.jsvalueImport()
	var args []ast.Expr
	for _, elem := range e.Elements {
		if elem != nil {
			arg := l.lowerExpr(elem)
			args = append(args, jsvalueWrapLit(arg))
		}
	}
	return callExpr(selectorExpr(goIdent("jsvalue"), "NewArray"), args...)
}

func (l *Lowerer) lowerObjectLiteral(e *hir.ObjectLiteral) ast.Expr {
	l.jsvalueImport()
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
	left := l.lowerExpr(e.Left)
	right := l.lowerExpr(e.Right)

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
			var wrappedArgs []ast.Expr
			wrappedArgs = append(wrappedArgs, stringLit(mem.Property))
			for _, a := range e.Args {
				wrappedArgs = append(wrappedArgs, l.wrapAsJSValue(l.lowerExpr(a)))
			}
			// For complex receivers, wrap in IIFE to evaluate once
			if !l.isSimpleExpr(obj) {
				recv := goIdent("_recv")
				recvArgs := make([]ast.Expr, len(wrappedArgs))
				copy(recvArgs, wrappedArgs)
				innerCall := callExpr(selectorExpr(recv, "MethodCall"), recvArgs...)
				return callExpr(&ast.FuncLit{
					Type: &ast.FuncType{
						Params:  fieldList(goField("_recv", jsValuePtrType())),
						Results: fieldList(goField("", jsValuePtrType())),
					},
					Body: blockStmt(returnStmt(innerCall)),
				}, obj)
			}
			return callExpr(selectorExpr(obj, "MethodCall"), wrappedArgs...)
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
			} else if id.Sym.Kind == symbol.KindVariable || id.Sym.Kind == symbol.KindParameter {
				isJSValueFunc = true
			}
			if isJSValueFunc {
				l.jsvalueImport()
				fn := l.lowerIdentifier(id)
				var wrappedArgs []ast.Expr
				for _, a := range e.Args {
					wrappedArgs = append(wrappedArgs, l.wrapAsJSValue(l.lowerExpr(a)))
				}
				return callExpr(selectorExpr(fn, "Call"), wrappedArgs...)
			}
		}
	}

	fn := l.lowerExpr(e.Func)
	var args []ast.Expr
	for _, a := range e.Args {
		args = append(args, l.lowerExpr(a))
	}

	// If the lowered function expression is already JSValue (e.g. .Get() result),
	// use .Call() to invoke it
	if isAlreadyJSValue(fn) {
		l.jsvalueImport()
		for i, a := range args {
			args[i] = jsvalueWrapLit(a)
		}
		return callExpr(selectorExpr(fn, "Call"), args...)
	}

	return callExpr(fn, args...)
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

	// Known globals (console, Math, etc.) → capitalize
	if id, ok := e.Object.(*hir.Identifier); ok {
		name := id.Name
		if id.Sym != nil {
			name = id.Sym.OriginalName
		}
		if l.ctx != nil && l.ctx.IsKnownGlobal(name) {
			return selectorExpr(obj, symbol.Capitalize(e.Property))
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

// exprIsJSValue returns true if the HIR expression is known to produce *jsvalue.JSValue.
func (l *Lowerer) exprIsJSValue(e hir.Expr) bool {
	switch e := e.(type) {
	case *hir.Identifier:
		if e.Sym == nil {
			if l.ctx != nil && l.ctx.IsKnownGlobal(e.Name) {
				return false
			}
			return true
		}
		if res, ok := l.importedSyms[e.Sym]; ok {
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
		return false
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
		// Concise body: () => expr → return expr
		val := l.lowerExpr(e.ExprBody)
		val = jsvalueWrapLit(val)
		body = l.lowerFuncBody(e.Params, &hir.BlockStmt{})
		body.List = append(body.List, returnStmt(val))
	} else {
		body = blockStmt()
	}

	fnLit := l.wrapAsJSValueFunc(e.Params, body)
	return callExpr(selectorExpr(goIdent("jsvalue"), "NewFunction"), fnLit)
}

func (l *Lowerer) lowerFuncExpr(e *hir.FuncExpr) ast.Expr {
	l.jsvalueImport()

	body := l.lowerFuncBody(e.Params, e.Body)
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
