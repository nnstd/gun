package backend

import (
	"go/ast"
	"go/token"

	"github.com/nnstd/gun/compiler/hir"
)

// --------------------------------------------------------------------
// Statements
// --------------------------------------------------------------------

func (l *Lowerer) lowerBlock(b *hir.BlockStmt) *ast.BlockStmt {
	if b == nil {
		return blockStmt()
	}
	// Hoist function declarations to the top of the block.
	// JS hoists function declarations — they're available throughout
	// their scope regardless of where they appear in the source.
	hoisted := hoistFunctions(b.Stmts)

	var stmts []ast.Stmt
	for _, s := range hoisted {
		gs := l.lowerStmt(s)
		if gs == nil {
			continue
		}
		// Flatten BlockStmt results to avoid creating unnecessary Go scopes.
		// This covers both multi-declarator VarDecl (which produces BlockStmt)
		// and standalone HIR BlockStmts (which tree-sitter creates for
		// statement_block nodes within function bodies).
		if block, ok := gs.(*ast.BlockStmt); ok {
			stmts = append(stmts, block.List...)
			continue
		}
		stmts = append(stmts, gs)
	}
	return &ast.BlockStmt{List: stmts}
}

// hoistFunctions reorders HIR statements to match JS hoisting semantics:
// All VarDecl statements (both function-valued and regular) are moved before
// non-VarDecl statements. This ensures that all declarations are visible
// before any code that references them, matching JS's function hoisting
// and preventing "undefined" errors from forward references.
func hoistFunctions(stmts []hir.Stmt) []hir.Stmt {
	var decls []hir.Stmt
	var rest []hir.Stmt
	for _, s := range stmts {
		if _, ok := s.(*hir.VarDecl); ok {
			decls = append(decls, s)
		} else if block, ok := s.(*hir.BlockStmt); ok {
			// Recursively extract VarDecl from nested blocks
			innerDecls, innerRest := extractVarDecls(block.Stmts)
			decls = append(decls, innerDecls...)
			if len(innerRest) > 0 {
				rest = append(rest, &hir.BlockStmt{Stmts: innerRest})
			}
		} else {
			rest = append(rest, s)
		}
	}
	if len(decls) == 0 {
		return stmts
	}
	return append(decls, rest...)
}

// extractVarDecls separates VarDecl statements from other statements,
// recursing into nested BlockStmts.
func extractVarDecls(stmts []hir.Stmt) (decls, rest []hir.Stmt) {
	for _, s := range stmts {
		if _, ok := s.(*hir.VarDecl); ok {
			decls = append(decls, s)
		} else if block, ok := s.(*hir.BlockStmt); ok {
			innerDecls, innerRest := extractVarDecls(block.Stmts)
			decls = append(decls, innerDecls...)
			if len(innerRest) > 0 {
				rest = append(rest, &hir.BlockStmt{Stmts: innerRest})
			}
		} else {
			rest = append(rest, s)
		}
	}
	return
}

func isFuncDecl(s hir.Stmt) bool {
	vd, ok := s.(*hir.VarDecl)
	if !ok || len(vd.Declarators) == 0 {
		return false
	}
	for _, d := range vd.Declarators {
		if d.Init == nil {
			continue
		}
		switch d.Init.(type) {
		case *hir.ArrowFunc, *hir.FuncExpr:
			return true
		}
	}
	return false
}

func (l *Lowerer) lowerStmt(s hir.Stmt) ast.Stmt {
	if s == nil {
		return nil
	}
	switch s := s.(type) {
	case *hir.ExprStmt:
		// Check for assignment expressions that need special handling
		if assign, ok := s.Expr.(*hir.AssignExpr); ok {
			return l.lowerAssignStmt(assign)
		}
		// Check for update expressions as statements
		if update, ok := s.Expr.(*hir.UpdateExpr); ok {
			return l.lowerUpdateStmt(update)
		}
		expr := l.lowerExpr(s.Expr)
		if expr == nil {
			return nil
		}
		return exprStmt(expr)

	case *hir.ReturnStmt:
		if s.Value != nil {
			val := l.lowerExpr(s.Value)
			val = jsvalueWrapLit(val)
			return returnStmt(val)
		}
		// Bare return → return nil (all functions return *jsvalue.JSValue)
		return returnStmt(goIdent("nil"))

	case *hir.BlockStmt:
		return l.lowerBlock(s)

	case *hir.VarDecl:
		// VarDecl may produce multiple statements (multi-declarator).
		// Inline them to avoid creating a Go block scope.
		stmts := l.lowerLocalVarStmts(s)
		if len(stmts) == 1 {
			return stmts[0]
		}
		// Return a block, but the caller (lowerBlock) will inline it
		return &ast.BlockStmt{List: stmts}

	case *hir.IfStmt:
		return l.lowerIfStmt(s)

	case *hir.ForStmt:
		return l.lowerForStmt(s)

	case *hir.ForInStmt:
		return l.lowerForInStmt(s)

	case *hir.ForOfStmt:
		return l.lowerForOfStmt(s)

	case *hir.WhileStmt:
		return l.lowerWhileStmt(s)

	case *hir.DoWhileStmt:
		return l.lowerDoWhileStmt(s)

	case *hir.SwitchStmt:
		return l.lowerSwitchStmt(s)

	case *hir.TryCatchStmt:
		return l.lowerTryCatchStmt(s)

	case *hir.ThrowStmt:
		val := l.lowerExpr(s.Value)
		return exprStmt(callExpr(goIdent("panic"), val))

	case *hir.BreakStmt:
		bs := &ast.BranchStmt{Tok: token.BREAK}
		if s.Label != "" {
			bs.Label = goIdent(s.Label)
		}
		return bs

	case *hir.ContinueStmt:
		cs := &ast.BranchStmt{Tok: token.CONTINUE}
		if s.Label != "" {
			cs.Label = goIdent(s.Label)
		}
		return cs

	case *hir.LabeledStmt:
		return &ast.LabeledStmt{
			Label: goIdent(s.Label),
			Stmt:  l.lowerStmt(s.Stmt),
		}

	case *hir.EmptyStmt:
		return &ast.EmptyStmt{}

	default:
		return nil
	}
}

func (l *Lowerer) lowerLocalVarStmts(d *hir.VarDecl) []ast.Stmt {
	var stmts []ast.Stmt
	for _, decl := range d.Declarators {
		// Destructuring pattern
		if decl.Pattern != nil && decl.Init != nil {
			stmts = append(stmts, l.lowerDestructuring(decl.Pattern, l.lowerExpr(decl.Init), true)...)
			continue
		}
		if decl.Symbol == nil {
			continue
		}
		name := l.emitName(decl.Symbol)
		if decl.Init != nil {
			value := l.lowerExpr(decl.Init)
			value = jsvalueWrapLit(value)
			stmts = append(stmts, assignDefine(
				[]ast.Expr{goIdent(name)},
				[]ast.Expr{value},
			))
		} else {
			// Uninitialized local: var name *jsvalue.JSValue
			l.jsvalueImport()
			stmts = append(stmts, &ast.DeclStmt{
				Decl: varDecl(name, jsValuePtrType(), nil),
			})
		}
	}
	return stmts
}

// lowerDestructuring generates statements for a destructuring pattern.
// When define=true, uses := (declarations). When define=false, uses = (assignments).
func (l *Lowerer) lowerDestructuring(pat hir.Pattern, init ast.Expr, define bool) []ast.Stmt {
	l.jsvalueImport()
	switch p := pat.(type) {
	case *hir.ObjectPattern:
		return l.lowerObjectDestructuring(p, init, define)
	case *hir.ArrayPattern:
		return l.lowerArrayDestructuring(p, init, define)
	}
	return nil
}

func (l *Lowerer) lowerObjectDestructuring(pat *hir.ObjectPattern, init ast.Expr, define bool) []ast.Stmt {
	var stmts []ast.Stmt
	assign := assignDefine
	if !define {
		assign = assignStmt
	}
	// Assign init to a temp (always :=, it's a new variable)
	tmpName := "_obj"
	stmts = append(stmts, assignDefine(
		[]ast.Expr{goIdent(tmpName)},
		[]ast.Expr{jsvalueWrapLit(init)},
	))
	for _, prop := range pat.Properties {
		if prop.Value == nil {
			continue
		}
		name := l.emitName(prop.Value)
		getter := callExpr(selectorExpr(goIdent(tmpName), "Get"), stringLit(prop.Key))
		stmts = append(stmts, assign(
			[]ast.Expr{goIdent(name)},
			[]ast.Expr{getter},
		))
		// Default value
		if prop.Default != nil {
			defVal := l.lowerExpr(prop.Default)
			defVal = jsvalueWrapLit(defVal)
			stmts = append(stmts, &ast.IfStmt{
				Cond: &ast.BinaryExpr{
					X: goIdent(name), Op: token.EQL, Y: goIdent("nil"),
				},
				Body: blockStmt(assignStmt(
					[]ast.Expr{goIdent(name)},
					[]ast.Expr{defVal},
				)),
			})
		}
	}
	if pat.Rest != nil {
		// rest := spread remaining keys (not implemented yet — placeholder)
		name := l.emitName(pat.Rest)
		stmts = append(stmts, assign(
			[]ast.Expr{goIdent(name)},
			[]ast.Expr{goIdent(tmpName)},
		))
	}
	return stmts
}

func (l *Lowerer) lowerArrayDestructuring(pat *hir.ArrayPattern, init ast.Expr, define bool) []ast.Stmt {
	var stmts []ast.Stmt
	assign := assignDefine
	if !define {
		assign = assignStmt
	}
	tmpName := "_arr"
	stmts = append(stmts, assignDefine(
		[]ast.Expr{goIdent(tmpName)},
		[]ast.Expr{jsvalueWrapLit(init)},
	))
	for i, elem := range pat.Elements {
		if elem == nil || elem.Symbol == nil {
			continue
		}
		name := l.emitName(elem.Symbol)
		idx := callExpr(selectorExpr(goIdent(tmpName), "Index"), intLit(itoa(i)))
		stmts = append(stmts, assign(
			[]ast.Expr{goIdent(name)},
			[]ast.Expr{idx},
		))
		if elem.Default != nil {
			defVal := l.lowerExpr(elem.Default)
			defVal = jsvalueWrapLit(defVal)
			stmts = append(stmts, &ast.IfStmt{
				Cond: &ast.BinaryExpr{
					X: goIdent(name), Op: token.EQL, Y: goIdent("nil"),
				},
				Body: blockStmt(assignStmt(
					[]ast.Expr{goIdent(name)},
					[]ast.Expr{defVal},
				)),
			})
		}
	}
	if pat.Rest != nil {
		name := l.emitName(pat.Rest)
		stmts = append(stmts, assign(
			[]ast.Expr{goIdent(name)},
			[]ast.Expr{callExpr(selectorExpr(goIdent("jsvalue"), "Slice"),
				goIdent(tmpName),
				callExpr(selectorExpr(goIdent("jsvalue"), "NewNumber"),
					callExpr(goIdent("float64"), intLit(itoa(len(pat.Elements))))))},
		))
	}
	return stmts
}

func (l *Lowerer) lowerIfStmt(s *hir.IfStmt) *ast.IfStmt {
	cond := l.lowerExpr(s.Cond)
	cond = l.ensureBool(cond)

	then := l.lowerBlock(s.Then)

	var elseStmt ast.Stmt
	if s.Else != nil {
		elseStmt = l.lowerStmt(s.Else)
	}

	return &ast.IfStmt{
		Cond: cond,
		Body: then,
		Else: elseStmt,
	}
}

func (l *Lowerer) lowerForStmt(s *hir.ForStmt) ast.Stmt {
	var init ast.Stmt
	var preLoop []ast.Stmt // statements hoisted before the for loop
	if s.Init != nil {
		init = l.lowerStmt(s.Init)
		// Go for-init only accepts a single SimpleStmt. When the init is a
		// multi-declarator VarDecl it lowers to a BlockStmt — hoist all but
		// the last statement before the loop.
		if block, ok := init.(*ast.BlockStmt); ok && len(block.List) > 1 {
			preLoop = block.List[:len(block.List)-1]
			init = block.List[len(block.List)-1]
		}
		// Go for-init doesn't allow var declarations (DeclStmt) — only
		// SimpleStmts like := assignments. Hoist var decls before the loop.
		if _, ok := init.(*ast.DeclStmt); ok {
			preLoop = append(preLoop, init)
			init = nil
		}
	}

	var cond ast.Expr
	if s.Cond != nil {
		cond = l.lowerExpr(s.Cond)
		cond = l.ensureBool(cond)
	}

	var post ast.Stmt
	if s.Post != nil {
		postExpr := l.lowerExpr(s.Post)
		if postExpr != nil {
			post = exprStmt(postExpr)
		}
	}

	body := l.lowerBlock(s.Body)

	forStmt := &ast.ForStmt{
		Init: init,
		Cond: cond,
		Post: post,
		Body: body,
	}

	if len(preLoop) > 0 {
		return &ast.BlockStmt{List: append(preLoop, forStmt)}
	}
	return forStmt
}

func (l *Lowerer) lowerForInStmt(s *hir.ForInStmt) ast.Stmt {
	l.jsvalueImport()

	keyName := "_"
	if s.Key != nil {
		keyName = l.emitName(s.Key)
	}

	value := l.lowerExpr(s.Value)
	body := l.lowerBlock(s.Body)

	// for _, key := range obj.OwnKeys()
	return &ast.RangeStmt{
		Key:   goIdent("_"),
		Value: goIdent(keyName),
		Tok:   token.DEFINE,
		X:     callExpr(selectorExpr(value, "OwnKeys")),
		Body:  body,
	}
}

func (l *Lowerer) lowerForOfStmt(s *hir.ForOfStmt) ast.Stmt {
	l.jsvalueImport()

	value := l.lowerExpr(s.Value)
	body := l.lowerBlock(s.Body)

	if s.Pattern != nil {
		// for (const {k, v} of entries) → iterate + destructure each element
		tmpName := "_item"
		destructStmts := l.lowerDestructuring(s.Pattern, goIdent(tmpName), true)
		body.List = append(destructStmts, body.List...)
		return &ast.RangeStmt{
			Key:   goIdent("_"),
			Value: goIdent(tmpName),
			Tok:   token.DEFINE,
			X:     callExpr(selectorExpr(value, "Array")),
			Body:  body,
		}
	}

	elemName := "_"
	if s.Elem != nil {
		elemName = l.emitName(s.Elem)
	}

	// for _, elem := range arr.Array()
	return &ast.RangeStmt{
		Key:   goIdent("_"),
		Value: goIdent(elemName),
		Tok:   token.DEFINE,
		X:     callExpr(selectorExpr(value, "Array")),
		Body:  body,
	}
}

func (l *Lowerer) lowerWhileStmt(s *hir.WhileStmt) *ast.ForStmt {
	cond := l.lowerExpr(s.Cond)
	cond = l.ensureBool(cond)
	body := l.lowerBlock(s.Body)

	return &ast.ForStmt{Cond: cond, Body: body}
}

func (l *Lowerer) lowerDoWhileStmt(s *hir.DoWhileStmt) *ast.ForStmt {
	body := l.lowerBlock(s.Body)
	cond := l.lowerExpr(s.Cond)
	cond = l.ensureBool(cond)

	// for { body; if !cond { break } }
	body.List = append(body.List, &ast.IfStmt{
		Cond: &ast.UnaryExpr{Op: token.NOT, X: cond},
		Body: blockStmt(&ast.BranchStmt{Tok: token.BREAK}),
	})

	return &ast.ForStmt{Body: body}
}

func (l *Lowerer) lowerSwitchStmt(s *hir.SwitchStmt) *ast.SwitchStmt {
	tag := l.lowerExpr(s.Tag)

	var cases []ast.Stmt
	for _, c := range s.Cases {
		cc := &ast.CaseClause{}
		if c.Value != nil {
			cc.List = []ast.Expr{l.lowerExpr(c.Value)}
		}
		for _, st := range c.Body {
			if gs := l.lowerStmt(st); gs != nil {
				cc.Body = append(cc.Body, gs)
			}
		}
		cases = append(cases, cc)
	}

	return &ast.SwitchStmt{
		Tag:  tag,
		Body: &ast.BlockStmt{List: cases},
	}
}

func (l *Lowerer) lowerTryCatchStmt(s *hir.TryCatchStmt) ast.Stmt {
	l.jsvalueImport()

	var stmts []ast.Stmt

	// Build defer+recover for catch
	if s.Catch != nil {
		catchBody := l.lowerBlock(s.Catch.Body)

		paramName := "_"
		if s.Catch.Param != nil {
			paramName = l.emitName(s.Catch.Param)
		}

		// Strip return values from catch body (defer closures can't return values).
		// Only strip at the direct level — don't recurse into nested FuncLit.
		stripTopLevelReturns(catchBody)
		// Prepend: paramName := jsvalue.From(r); _ = paramName
		catchBody.List = append([]ast.Stmt{
			assignDefine(
				[]ast.Expr{goIdent(paramName)},
				[]ast.Expr{callExpr(selectorExpr(goIdent("jsvalue"), "From"), goIdent("r"))},
			),
			assignStmt([]ast.Expr{goIdent("_")}, []ast.Expr{goIdent(paramName)}),
		}, catchBody.List...)

		recoverBlock := blockStmt(
			&ast.IfStmt{
				Init: assignDefine(
					[]ast.Expr{goIdent("r")},
					[]ast.Expr{callExpr(goIdent("recover"))},
				),
				Cond: &ast.BinaryExpr{
					X: goIdent("r"), Op: token.NEQ, Y: goIdent("nil"),
				},
				Body: catchBody,
			},
		)

		deferStmt := &ast.DeferStmt{
			Call: &ast.CallExpr{
				Fun: &ast.FuncLit{
					Type: &ast.FuncType{Params: fieldList()},
					Body: recoverBlock,
				},
			},
		}
		stmts = append(stmts, deferStmt)
	}

	// Try body
	if s.Try != nil {
		for _, st := range s.Try.Stmts {
			if gs := l.lowerStmt(st); gs != nil {
				stmts = append(stmts, gs)
			}
		}
	}

	// Finally is lowered as a separate defer
	if s.Finally != nil {
		finallyBody := l.lowerBlock(s.Finally)
		finallyDefer := &ast.DeferStmt{
			Call: &ast.CallExpr{
				Fun: &ast.FuncLit{
					Type: &ast.FuncType{Params: fieldList()},
					Body: finallyBody,
				},
			},
		}
		// Finally defer should be first (executes last)
		stmts = append([]ast.Stmt{finallyDefer}, stmts...)
	}

	return &ast.BlockStmt{List: stmts}
}

// lowerAssignStmt handles assignment expressions as statements.
// Member assignments: obj.prop = val → obj.Set("prop", val)
// Subscript assignments: obj[key] = val → obj.Set(key, val)
// Destructuring assignments: [a, b] = val / {x, y} = val → expanded
// Simple assignments: x = val → x = val
func (l *Lowerer) lowerAssignStmt(assign *hir.AssignExpr) ast.Stmt {
	// Destructuring assignment: [a, ...b] = expr or {x, y} = expr
	if assign.LeftPattern != nil {
		right := l.lowerExpr(assign.Right)
		stmts := l.lowerDestructuring(assign.LeftPattern, right, false)
		return &ast.BlockStmt{List: stmts}
	}

	right := l.lowerExpr(assign.Right)

	// Member assignment: obj.prop = val → obj.Set("prop", wrappedVal)
	if mem, ok := assign.Left.(*hir.MemberExpr); ok {
		if l.exprIsJSValue(mem.Object) {
			l.jsvalueImport()
			obj := l.lowerExpr(mem.Object)
			val := l.wrapAsJSValue(right)
			if assign.Op != hir.OpAssign {
				// Augmented: obj.prop += val → obj.Set("prop", jsvalue.Add(obj.Get("prop"), val))
				helperName := mapAssignOpToJSValue(assign.Op)
				current := callExpr(selectorExpr(obj, "Get"), stringLit(mem.Property))
				val = callExpr(selectorExpr(goIdent("jsvalue"), helperName), current, val)
			}
			return exprStmt(callExpr(selectorExpr(obj, "Set"), stringLit(mem.Property), val))
		}
	}

	// Subscript assignment: obj[key] = val → obj.Set(fmt.Sprint(key), wrappedVal)
	if comp, ok := assign.Left.(*hir.ComputedMemberExpr); ok {
		if l.exprIsJSValue(comp.Object) {
			l.jsvalueImport()
			l.addImport("fmt")
			obj := l.lowerExpr(comp.Object)
			key := l.lowerExpr(comp.Property)
			val := l.wrapAsJSValue(right)
			return exprStmt(callExpr(selectorExpr(obj, "Set"),
				callExpr(selectorExpr(goIdent("fmt"), "Sprint"), key), val))
		}
	}

	// Simple variable assignment
	left := l.lowerExpr(assign.Left)
	if assign.Op == hir.OpAssign {
		return assignStmt([]ast.Expr{left}, []ast.Expr{jsvalueWrapLit(right)})
	}
	// Augmented assignment: x += val → x = jsvalue.Add(x, val)
	l.jsvalueImport()
	helperName := mapAssignOpToJSValue(assign.Op)
	computed := callExpr(selectorExpr(goIdent("jsvalue"), helperName),
		jsvalueWrapLit(left), jsvalueWrapLit(right))
	return assignStmt([]ast.Expr{left}, []ast.Expr{computed})
}

// lowerUpdateStmt handles update expressions (x++, x--) as statements.
func (l *Lowerer) lowerUpdateStmt(update *hir.UpdateExpr) ast.Stmt {
	l.jsvalueImport()
	operand := l.lowerExpr(update.Operand)
	if update.Op == hir.OpInc {
		return assignStmt([]ast.Expr{operand},
			[]ast.Expr{callExpr(selectorExpr(goIdent("jsvalue"), "Inc"), operand)})
	}
	return assignStmt([]ast.Expr{operand},
		[]ast.Expr{callExpr(selectorExpr(goIdent("jsvalue"), "Dec"), operand)})
}

// stripTopLevelReturns removes return values from return statements at the
// direct level of a block. Does NOT recurse into nested FuncLit bodies.
func stripTopLevelReturns(block *ast.BlockStmt) {
	for i, s := range block.List {
		if ret, ok := s.(*ast.ReturnStmt); ok {
			// Convert return with value to expression statement + bare return
			if len(ret.Results) > 0 {
				var stmts []ast.Stmt
				for _, r := range ret.Results {
					stmts = append(stmts, exprStmt(r))
				}
				stmts = append(stmts, &ast.ReturnStmt{})
				// Replace the return with the expanded statements
				block.List = append(block.List[:i], append(stmts, block.List[i+1:]...)...)
			}
		}
		if ifStmt, ok := s.(*ast.IfStmt); ok {
			stripTopLevelReturns(ifStmt.Body)
			if elseBlock, ok := ifStmt.Else.(*ast.BlockStmt); ok {
				stripTopLevelReturns(elseBlock)
			}
		}
	}
}

// ensureBool wraps a JSValue expression with .Bool() for use in Go conditions.
func (l *Lowerer) ensureBool(expr ast.Expr) ast.Expr {
	if expr == nil {
		return goIdent("true")
	}

	// Already a native bool expression
	if l.isNativeBool(expr) {
		return expr
	}

	// Check if expression produces int (Len(), len()) — unwrap parens first
	unwrapped := expr
	if paren, ok := unwrapped.(*ast.ParenExpr); ok {
		unwrapped = paren.X
	}
	if call, ok := unwrapped.(*ast.CallExpr); ok {
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Len" {
			return &ast.BinaryExpr{X: expr, Op: token.GTR, Y: intLit("0")}
		}
		if id, ok := call.Fun.(*ast.Ident); ok && id.Name == "len" {
			return &ast.BinaryExpr{X: expr, Op: token.GTR, Y: intLit("0")}
		}
	}

	// JSValue expression → .Bool()
	return callExpr(selectorExpr(expr, "Bool"))
}

// isNativeBool returns true if the expression produces a native Go bool.
func (l *Lowerer) isNativeBool(expr ast.Expr) bool {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name == "true" || e.Name == "false"
	case *ast.BinaryExpr:
		switch e.Op {
		case token.EQL, token.NEQ, token.LSS, token.GTR, token.LEQ, token.GEQ,
			token.LAND, token.LOR:
			return true
		}
	case *ast.UnaryExpr:
		if e.Op == token.NOT {
			return true
		}
	case *ast.ParenExpr:
		return l.isNativeBool(e.X)
	case *ast.CallExpr:
		if sel, ok := e.Fun.(*ast.SelectorExpr); ok {
			// .Bool() itself returns bool
			if sel.Sel.Name == "Bool" {
				return true
			}
		}
	}
	return false
}
