package backend

import (
	"go/ast"
	"go/token"

	"github.com/nnstd/gun/compiler/hir"
	"github.com/nnstd/gun/compiler/symbol"
)

type asyncLoopLabels struct {
	breakLabel    int
	continueLabel int
	valid         bool
	namedBreak    map[string]int
	namedContinue map[string]int
}

type asyncProtected struct {
	catchLabel int
	catchName  string
	valid      bool
}

type asyncFinalizer struct {
	label int
	valid bool
}

type asyncStateCase struct {
	label int
	body  []ast.Stmt
}

type asyncFuncBuilder struct {
	l              *Lowerer
	stateName      string
	resolveName    string
	rejectName     string
	stepName       string
	awaitValueName string
	completeKind   string
	completeTarget string
	completeValue  string
	cases          []asyncStateCase
	nextLabel      int
}

func (l *Lowerer) lowerAsyncFuncBody(params []*hir.Param, body *hir.BlockStmt, argOffset int, bindThis bool) *ast.BlockStmt {
	l.disableArenaCount++
	defer func() { l.disableArenaCount-- }()
	l.jsvalueImport()
	l.addImport("github.com/nnstd/gun/runtime/promise")
	l.insideFunc++
	if bindThis {
		l.insideMethod++
	}
	defer func() {
		l.insideFunc--
		if bindThis {
			l.insideMethod--
		}
	}()
	prevAsyncTemps := l.asyncTempSymbols
	l.asyncTempSymbols = nil
	defer func() { l.asyncTempSymbols = prevAsyncTemps }()

	stateName := l.nextSyntheticName("_async_state")
	resolveName := l.nextSyntheticName("_async_resolve")
	rejectName := l.nextSyntheticName("_async_reject")
	stepName := l.nextSyntheticName("_async_step")
	awaitValueName := l.nextSyntheticName("_async_await_value")
	completeKind := l.nextSyntheticName("_async_complete_kind")
	completeTarget := l.nextSyntheticName("_async_complete_target")
	completeValue := l.nextSyntheticName("_async_complete_value")

	builder := &asyncFuncBuilder{
		l:              l,
		stateName:      stateName,
		resolveName:    resolveName,
		rejectName:     rejectName,
		stepName:       stepName,
		awaitValueName: awaitValueName,
		completeKind:   completeKind,
		completeTarget: completeTarget,
		completeValue:  completeValue,
	}

	endLabel := builder.newLabel()
	builder.addCase(endLabel, []ast.Stmt{
		exprStmt(callExpr(selectorExpr(goIdent(resolveName), "Call"), callExpr(selectorExpr(goIdent("jsvalue"), "NewUndefined")))),
		returnStmt(callExpr(selectorExpr(goIdent("jsvalue"), "NewUndefined"))),
	})
	normalizedBody := l.normalizeAsyncBlock(body)
	var bodyStmts []hir.Stmt
	if normalizedBody != nil {
		bodyStmts = normalizedBody.Stmts
	}
	entryLabel := builder.compileSeq(bodyStmts, endLabel, asyncLoopLabels{}, asyncProtected{}, asyncFinalizer{})

	var stmts []ast.Stmt
	thisUsed := bindThis && hirBodyUsesThis(body)
	if thisUsed {
		stmts = append(stmts, l.lowerAsyncThisSetup()...)
		argOffset++ // params start after 'this' in _args
	}
	stmts = append(stmts, l.lowerAsyncParamSetup(params, argOffset)...)
	stmts = append(stmts, l.lowerAsyncLocalDecls(normalizedBody)...)
	stmts = append(stmts,
		&ast.DeclStmt{Decl: varDecl(stateName, goIdent("int"), nil)},
		&ast.DeclStmt{Decl: varDecl(resolveName, jsValuePtrType(), nil)},
		&ast.DeclStmt{Decl: varDecl(rejectName, jsValuePtrType(), nil)},
		&ast.DeclStmt{Decl: varDecl(stepName, jsValuePtrType(), nil)},
		&ast.DeclStmt{Decl: varDecl(awaitValueName, jsValuePtrType(), nil)},
		&ast.DeclStmt{Decl: varDecl(completeKind, goIdent("int"), nil)},
		&ast.DeclStmt{Decl: varDecl(completeTarget, goIdent("int"), nil)},
		&ast.DeclStmt{Decl: varDecl(completeValue, jsValuePtrType(), nil)},
	)

	executorBody := []ast.Stmt{
		assignStmt([]ast.Expr{goIdent(resolveName)}, []ast.Expr{callExpr(selectorExpr(goIdent("jsvalue"), "NewUndefined"))}),
		&ast.IfStmt{
			Cond: &ast.BinaryExpr{
				X:  callExpr(goIdent("len"), goIdent("_args")),
				Op: token.GTR,
				Y:  intLit("0"),
			},
			Body: blockStmt(assignStmt(
				[]ast.Expr{goIdent(resolveName)},
				[]ast.Expr{&ast.IndexExpr{X: goIdent("_args"), Index: intLit("0")}},
			)),
		},
		assignStmt([]ast.Expr{goIdent(rejectName)}, []ast.Expr{callExpr(selectorExpr(goIdent("jsvalue"), "NewUndefined"))}),
		&ast.IfStmt{
			Cond: &ast.BinaryExpr{
				X:  callExpr(goIdent("len"), goIdent("_args")),
				Op: token.GTR,
				Y:  intLit("1"),
			},
			Body: blockStmt(assignStmt(
				[]ast.Expr{goIdent(rejectName)},
				[]ast.Expr{&ast.IndexExpr{X: goIdent("_args"), Index: intLit("1")}},
			)),
		},
		assignStmt([]ast.Expr{goIdent(stateName)}, []ast.Expr{intLit(itoa(entryLabel))}),
		assignStmt([]ast.Expr{goIdent(stepName)}, []ast.Expr{callExpr(selectorExpr(goIdent("jsvalue"), "NewFunction"), builder.stepFuncLit())}),
		exprStmt(callExpr(selectorExpr(goIdent(stepName), "Call"))),
		returnStmt(callExpr(selectorExpr(goIdent("jsvalue"), "NewUndefined"))),
	}

	stmts = append(stmts, returnStmt(callExpr(
		selectorExpr(selectorExpr(goIdent("promise"), "Promise"), "Call"),
		callExpr(selectorExpr(goIdent("jsvalue"), "NewFunction"), builder.callbackFuncLit(executorBody)),
	)))

	stmts = eliminateUnusedVars(stmts)
	return &ast.BlockStmt{List: stmts}
}

func (l *Lowerer) lowerAsyncThisSetup() []ast.Stmt {
	l.jsvalueImport()
	return []ast.Stmt{
		&ast.DeclStmt{Decl: varDecl("this", jsValuePtrType(), nil)},
		&ast.IfStmt{
			Cond: &ast.BinaryExpr{
				X:  callExpr(goIdent("len"), goIdent("_args")),
				Op: token.GTR,
				Y:  intLit("0"),
			},
			Body: blockStmt(assignStmt(
				[]ast.Expr{goIdent("this")},
				[]ast.Expr{&ast.IndexExpr{X: goIdent("_args"), Index: intLit("0")}},
			)),
		},
	}
}

func (l *Lowerer) lowerAsyncParamSetup(params []*hir.Param, argOffset int) []ast.Stmt {
	var stmts []ast.Stmt
	for i, p := range params {
		if p == nil {
			continue
		}
		l.jsvalueImport()
		argIndex := i + argOffset
		if p.Pattern != nil {
			tmpName := l.nextSyntheticName("_async_param")
			stmts = append(stmts, &ast.DeclStmt{Decl: varDecl(tmpName, jsValuePtrType(), nil)})
			stmts = append(stmts, &ast.IfStmt{
				Cond: &ast.BinaryExpr{
					X:  callExpr(goIdent("len"), goIdent("_args")),
					Op: token.GTR,
					Y:  intLit(itoa(argIndex)),
				},
				Body: blockStmt(assignStmt(
					[]ast.Expr{goIdent(tmpName)},
					[]ast.Expr{&ast.IndexExpr{X: goIdent("_args"), Index: intLit(itoa(argIndex))}},
				)),
			})
			if p.Default != nil {
				defVal := l.wrapAsJSValue(l.lowerExpr(p.Default))
				stmts = append(stmts, &ast.IfStmt{
					Cond: l.isNilOrUndefined(goIdent(tmpName)),
					Body: blockStmt(assignStmt([]ast.Expr{goIdent(tmpName)}, []ast.Expr{defVal})),
				})
			}
			stmts = append(stmts, l.lowerDestructuring(p.Pattern, goIdent(tmpName), true)...)
			continue
		}
		if p.Symbol == nil {
			continue
		}
		name := l.emitName(p.Symbol)
		if p.Rest {
			restCall := callExpr(selectorExpr(goIdent("jsvalue"), "NewArray"),
				&ast.SliceExpr{X: goIdent("_args"), Low: intLit(itoa(argIndex))})
			restCall.Ellipsis = 1
			stmts = append(stmts, &ast.DeclStmt{Decl: varDecl(name, jsValuePtrType(), restCall)})
			continue
		}
		stmts = append(stmts, &ast.DeclStmt{Decl: varDecl(name, jsValuePtrType(), nil)})
		stmts = append(stmts, &ast.IfStmt{
			Cond: &ast.BinaryExpr{
				X:  callExpr(goIdent("len"), goIdent("_args")),
				Op: token.GTR,
				Y:  intLit(itoa(argIndex)),
			},
			Body: blockStmt(assignStmt(
				[]ast.Expr{goIdent(name)},
				[]ast.Expr{&ast.IndexExpr{X: goIdent("_args"), Index: intLit(itoa(argIndex))}},
			)),
		})
		if p.Default != nil {
			defVal := l.wrapAsJSValue(l.lowerExpr(p.Default))
			stmts = append(stmts, &ast.IfStmt{
				Cond: l.isNilOrUndefined(goIdent(name)),
				Body: blockStmt(assignStmt([]ast.Expr{goIdent(name)}, []ast.Expr{defVal})),
			})
		}
	}
	return stmts
}

func (l *Lowerer) lowerAsyncLocalDecls(body *hir.BlockStmt) []ast.Stmt {
	seen := map[symbol.ID]bool{}
	var locals []*symbol.Symbol
	var collectPattern func(hir.Pattern)
	collectPattern = func(p hir.Pattern) {
		switch p := p.(type) {
		case *hir.ObjectPattern:
			for _, prop := range p.Properties {
				if prop.Value != nil && !seen[prop.Value.ID] {
					seen[prop.Value.ID] = true
					locals = append(locals, prop.Value)
				}
				collectPattern(prop.Pattern)
			}
			if p.Rest != nil && !seen[p.Rest.ID] {
				seen[p.Rest.ID] = true
				locals = append(locals, p.Rest)
			}
		case *hir.ArrayPattern:
			for _, elem := range p.Elements {
				if elem == nil {
					continue
				}
				if elem.Symbol != nil && !seen[elem.Symbol.ID] {
					seen[elem.Symbol.ID] = true
					locals = append(locals, elem.Symbol)
				}
				collectPattern(elem.Pattern)
			}
			if p.Rest != nil && !seen[p.Rest.ID] {
				seen[p.Rest.ID] = true
				locals = append(locals, p.Rest)
			}
		}
	}
	var walkBlock func(*hir.BlockStmt)
	var walkStmt func(hir.Stmt)
	walkBlock = func(b *hir.BlockStmt) {
		if b == nil {
			return
		}
		for _, st := range b.Stmts {
			walkStmt(st)
		}
	}
	walkStmt = func(s hir.Stmt) {
		switch s := s.(type) {
		case *hir.BlockStmt:
			walkBlock(s)
		case *hir.VarDecl:
			for _, d := range s.Declarators {
				if d.Symbol != nil && !seen[d.Symbol.ID] {
					seen[d.Symbol.ID] = true
					locals = append(locals, d.Symbol)
				}
				collectPattern(d.Pattern)
			}
		case *hir.IfStmt:
			walkBlock(s.Then)
			walkStmt(s.Else)
		case *hir.ForStmt:
			walkStmt(s.Init)
			walkBlock(s.Body)
		case *hir.ForInStmt:
			if s.Key != nil && !seen[s.Key.ID] {
				seen[s.Key.ID] = true
				locals = append(locals, s.Key)
			}
			walkBlock(s.Body)
		case *hir.ForOfStmt:
			if s.Elem != nil && !seen[s.Elem.ID] {
				seen[s.Elem.ID] = true
				locals = append(locals, s.Elem)
			}
			collectPattern(s.Pattern)
			walkBlock(s.Body)
		case *hir.WhileStmt:
			walkBlock(s.Body)
		case *hir.DoWhileStmt:
			walkBlock(s.Body)
		case *hir.SwitchStmt:
			for _, c := range s.Cases {
				for _, st := range c.Body {
					walkStmt(st)
				}
			}
		case *hir.LabeledStmt:
			walkStmt(s.Stmt)
		case *hir.TryCatchStmt:
			if s.Catch != nil && s.Catch.Param != nil && !seen[s.Catch.Param.ID] {
				seen[s.Catch.Param.ID] = true
				locals = append(locals, s.Catch.Param)
			}
			walkBlock(s.Try)
			if s.Catch != nil {
				walkBlock(s.Catch.Body)
			}
		}
	}
	walkBlock(body)

	var stmts []ast.Stmt
	for _, sym := range locals {
		stmts = append(stmts, &ast.DeclStmt{Decl: varDecl(l.emitName(sym), jsValuePtrType(), nil)})
	}
	for _, sym := range l.asyncTempSymbols {
		if seen[sym.ID] {
			continue
		}
		seen[sym.ID] = true
		stmts = append(stmts, &ast.DeclStmt{Decl: varDecl(l.emitName(sym), jsValuePtrType(), nil)})
	}
	return stmts
}

func (b *asyncFuncBuilder) newLabel() int {
	label := b.nextLabel
	b.nextLabel++
	return label
}

func (b *asyncFuncBuilder) addCase(label int, body []ast.Stmt) {
	b.cases = append(b.cases, asyncStateCase{label: label, body: body})
}

func (b *asyncFuncBuilder) compileSeq(stmts []hir.Stmt, next int, loop asyncLoopLabels, protected asyncProtected, finalizer asyncFinalizer) int {
	entry := next
	for i := len(stmts) - 1; i >= 0; i-- {
		entry = b.compileStmt(stmts[i], entry, loop, protected, finalizer)
	}
	return entry
}

func (b *asyncFuncBuilder) compileStmt(stmt hir.Stmt, next int, loop asyncLoopLabels, protected asyncProtected, finalizer asyncFinalizer) int {
	switch s := stmt.(type) {
	case nil:
		return next
	case *hir.BlockStmt:
		return b.compileSeq(s.Stmts, next, loop, protected, finalizer)
	case *hir.EmptyStmt:
		return next
	case *hir.ExprStmt:
		if await, ok := asyncDirectAwait(s.Expr); ok {
			label := b.newLabel()
			b.addCase(label, b.awaitCase(await.Value, next, nil, nil, protected, finalizer))
			return label
		}
		if assign, ok := s.Expr.(*hir.AssignExpr); ok {
			if await, ok := asyncDirectAwait(assign.Right); ok {
				label := b.newLabel()
				assignExpr := &hir.AssignExpr{
					Op:          assign.Op,
					Left:        assign.Left,
					LeftPattern: assign.LeftPattern,
					Right: &hir.Identifier{
						Name: b.awaitValueName,
					},
				}
				assignStmt := b.l.lowerStmt(&hir.ExprStmt{Expr: assignExpr})
				var onFulfilled []ast.Stmt
				if assignStmt != nil {
					if block, ok := assignStmt.(*ast.BlockStmt); ok {
						onFulfilled = append(onFulfilled, block.List...)
					} else {
						onFulfilled = append(onFulfilled, assignStmt)
					}
				}
				b.addCase(label, b.awaitCase(await.Value, next, onFulfilled, nil, protected, finalizer))
				return label
			}
		}
		label := b.newLabel()
		lowered := b.l.lowerStmt(s)
		body := []ast.Stmt{}
		if lowered != nil {
			body = append(body, lowered)
		}
		body = append(body, b.transition(next)...)
		b.addCase(label, body)
		return label
	case *hir.VarDecl:
		entry := next
		for i := len(s.Declarators) - 1; i >= 0; i-- {
			decl := s.Declarators[i]
			if decl.Symbol == nil && decl.Pattern == nil {
				continue
			}
			if await, ok := asyncDirectAwait(decl.Init); ok {
				label := b.newLabel()
				var onFulfilled []ast.Stmt
				if decl.Pattern != nil {
					onFulfilled = append(onFulfilled, b.l.lowerDestructuring(decl.Pattern, goIdent(b.awaitValueName), false)...)
				} else if decl.Symbol != nil {
					onFulfilled = append(onFulfilled, assignStmt([]ast.Expr{goIdent(b.l.emitName(decl.Symbol))}, []ast.Expr{goIdent(b.awaitValueName)}))
				}
				b.addCase(label, b.awaitCase(await.Value, entry, onFulfilled, nil, protected, finalizer))
				entry = label
				continue
			}
			if decl.Init == nil {
				continue
			}
			label := b.newLabel()
			var body []ast.Stmt
			if decl.Pattern != nil {
				body = append(body, b.l.lowerDestructuring(decl.Pattern, b.l.lowerExpr(decl.Init), false)...)
			} else if decl.Symbol != nil {
				body = append(body, assignStmt([]ast.Expr{goIdent(b.l.emitName(decl.Symbol))}, []ast.Expr{b.l.wrapAsJSValue(b.l.lowerExpr(decl.Init))}))
			}
			body = append(body, b.transition(entry)...)
			b.addCase(label, body)
			entry = label
		}
		return entry
	case *hir.ReturnStmt:
		if await, ok := asyncDirectAwait(s.Value); ok {
			label := b.newLabel()
			var onFulfilled []ast.Stmt
			if finalizer.valid {
				onFulfilled = b.finalizerSetReturn(goIdent(b.awaitValueName), true, finalizer)
			} else {
				onFulfilled = []ast.Stmt{
					exprStmt(callExpr(selectorExpr(goIdent(b.resolveName), "Call"), goIdent(b.awaitValueName))),
					returnStmt(callExpr(selectorExpr(goIdent("jsvalue"), "NewUndefined"))),
				}
			}
			b.addCase(label, b.awaitCase(await.Value, -1, onFulfilled, nil, protected, finalizer))
			return label
		}
		label := b.newLabel()
		var result ast.Expr = callExpr(selectorExpr(goIdent("jsvalue"), "NewUndefined"))
		if s.Value != nil {
			result = b.l.wrapAsJSValue(b.l.lowerExpr(s.Value))
		}
		if finalizer.valid {
			b.addCase(label, b.finalizerSetReturn(result, false, finalizer))
		} else {
			b.addCase(label, []ast.Stmt{
				exprStmt(callExpr(selectorExpr(goIdent(b.resolveName), "Call"), result)),
				returnStmt(callExpr(selectorExpr(goIdent("jsvalue"), "NewUndefined"))),
			})
		}
		return label
	case *hir.ThrowStmt:
		label := b.newLabel()
		var reason ast.Expr = callExpr(selectorExpr(goIdent("jsvalue"), "NewUndefined"))
		if s.Value != nil {
			reason = b.l.wrapAsJSValue(b.l.lowerExpr(s.Value))
		}
		if protected.valid {
			body := b.catchTransition(reason, protected)
			b.addCase(label, body)
			return label
		}
		if finalizer.valid {
			b.addCase(label, b.finalizerSetReject(reason, false, finalizer))
			return label
		}
		b.addCase(label, []ast.Stmt{
			exprStmt(callExpr(selectorExpr(goIdent(b.rejectName), "Call"), reason)),
			returnStmt(callExpr(selectorExpr(goIdent("jsvalue"), "NewUndefined"))),
		})
		return label
	case *hir.IfStmt:
		thenEntry := b.compileSeq(s.Then.Stmts, next, loop, protected, finalizer)
		elseEntry := next
		if s.Else != nil {
			elseEntry = b.compileStmt(s.Else, next, loop, protected, finalizer)
		}
		label := b.newLabel()
		cond := b.l.ensureBool(b.l.lowerExpr(s.Cond))
		b.addCase(label, []ast.Stmt{
			&ast.IfStmt{
				Cond: cond,
				Body: blockStmt(b.transition(thenEntry)...),
				Else: blockStmt(b.transition(elseEntry)...),
			},
		})
		return label
	case *hir.WhileStmt:
		condEntryLabel := b.newLabel()
		bodyEntry := b.compileSeq(s.Body.Stmts, condEntryLabel, asyncLoopLabels{breakLabel: next, continueLabel: condEntryLabel, valid: true}, protected, finalizer)
		branchEntry := b.compileConditionExpr(s.Cond, bodyEntry, next, loop, protected, finalizer)
		b.addCase(condEntryLabel, b.transition(branchEntry))
		return condEntryLabel
	case *hir.DoWhileStmt:
		condEntryLabel := b.newLabel()
		bodyEntry := b.compileSeq(s.Body.Stmts, condEntryLabel, asyncLoopLabels{breakLabel: next, continueLabel: condEntryLabel, valid: true}, protected, finalizer)
		branchEntry := b.compileConditionExpr(s.Cond, bodyEntry, next, loop, protected, finalizer)
		b.addCase(condEntryLabel, b.transition(branchEntry))
		return bodyEntry
	case *hir.ForStmt:
		condEntryLabel := b.newLabel()
		postLabel := condEntryLabel
		if s.Post != nil {
			postLabel = b.compileExprLoopStep(s.Post, condEntryLabel, loop, protected, finalizer)
		}
		bodyEntry := b.compileSeq(s.Body.Stmts, postLabel, asyncLoopLabels{breakLabel: next, continueLabel: postLabel, valid: true}, protected, finalizer)
		branchEntry := bodyEntry
		if s.Cond != nil {
			branchEntry = b.compileConditionExpr(s.Cond, bodyEntry, next, loop, protected, finalizer)
		}
		b.addCase(condEntryLabel, b.transition(branchEntry))
		if s.Init != nil {
			return b.compileStmt(s.Init, condEntryLabel, loop, protected, finalizer)
		}
		return condEntryLabel
	case *hir.ForInStmt:
		itemsName := b.l.emitName(b.l.newAsyncTempSymbol())
		indexName := b.l.emitName(b.l.newAsyncTempSymbol())
		postLabel := b.newLabel()
		prepLabel := b.newLabel()
		bodyEntry := b.compileSeq(s.Body.Stmts, postLabel, asyncLoopLabels{breakLabel: next, continueLabel: postLabel, valid: true}, protected, finalizer)
		itemExpr := callExpr(selectorExpr(goIdent(itemsName), "Get"), callExpr(selectorExpr(goIdent("fmt"), "Sprint"), goIdent(indexName)))
		prepBody := []ast.Stmt{}
		if s.Key != nil {
			prepBody = append(prepBody, assignStmt([]ast.Expr{goIdent(b.l.emitName(s.Key))}, []ast.Expr{itemExpr}))
		}
		prepBody = append(prepBody, b.transition(bodyEntry)...)
		b.addCase(prepLabel, prepBody)
		condLabel := b.newLabel()
		cond := b.l.ensureBool(callExpr(
			selectorExpr(goIdent("jsvalue"), "Lt"),
			goIdent(indexName),
			callExpr(selectorExpr(goIdent("jsvalue"), "NewNumber"),
				callExpr(goIdent("float64"), callExpr(selectorExpr(goIdent(itemsName), "Len")))),
		))
		b.addCase(condLabel, []ast.Stmt{
			&ast.IfStmt{
				Cond: cond,
				Body: blockStmt(b.transition(prepLabel)...),
				Else: blockStmt(b.transition(next)...),
			},
		})
		initLabel := b.newLabel()
		b.l.jsvalueImport()
		b.l.addImport("fmt")
		b.addCase(initLabel, []ast.Stmt{
			assignStmt([]ast.Expr{goIdent(itemsName)}, []ast.Expr{callExpr(selectorExpr(goIdent("jsvalue"), "Keys"), b.l.wrapAsJSValue(b.l.lowerExpr(s.Value)))}),
			assignStmt([]ast.Expr{goIdent(indexName)}, []ast.Expr{callExpr(selectorExpr(goIdent("jsvalue"), "NewNumber"), intLit("0"))}),
			assignStmt([]ast.Expr{goIdent(b.stateName)}, []ast.Expr{intLit(itoa(condLabel))}),
			&ast.BranchStmt{Tok: token.CONTINUE},
		})
		b.addCase(postLabel, []ast.Stmt{
			assignStmt([]ast.Expr{goIdent(indexName)}, []ast.Expr{callExpr(selectorExpr(goIdent("jsvalue"), "Add"), goIdent(indexName), callExpr(selectorExpr(goIdent("jsvalue"), "NewNumber"), intLit("1")))}),
			assignStmt([]ast.Expr{goIdent(b.stateName)}, []ast.Expr{intLit(itoa(condLabel))}),
			&ast.BranchStmt{Tok: token.CONTINUE},
		})
		return initLabel
	case *hir.ForOfStmt:
		itemsName := b.l.emitName(b.l.newAsyncTempSymbol())
		indexName := b.l.emitName(b.l.newAsyncTempSymbol())
		postLabel := b.newLabel()
		prepLabel := b.newLabel()
		bodyEntry := b.compileSeq(s.Body.Stmts, postLabel, asyncLoopLabels{breakLabel: next, continueLabel: postLabel, valid: true}, protected, finalizer)
		itemExpr := callExpr(selectorExpr(goIdent(itemsName), "Get"), callExpr(selectorExpr(goIdent("fmt"), "Sprint"), goIdent(indexName)))
		prepBody := []ast.Stmt{}
		if s.Pattern != nil {
			prepBody = append(prepBody, b.l.lowerDestructuring(s.Pattern, itemExpr, false)...)
		} else if s.Elem != nil {
			prepBody = append(prepBody, assignStmt([]ast.Expr{goIdent(b.l.emitName(s.Elem))}, []ast.Expr{itemExpr}))
		}
		prepBody = append(prepBody, b.transition(bodyEntry)...)
		b.addCase(prepLabel, prepBody)
		condLabel := b.newLabel()
		cond := b.l.ensureBool(callExpr(
			selectorExpr(goIdent("jsvalue"), "Lt"),
			goIdent(indexName),
			callExpr(selectorExpr(goIdent("jsvalue"), "NewNumber"),
				callExpr(goIdent("float64"), callExpr(selectorExpr(goIdent(itemsName), "Len")))),
		))
		b.addCase(condLabel, []ast.Stmt{
			&ast.IfStmt{
				Cond: cond,
				Body: blockStmt(b.transition(prepLabel)...),
				Else: blockStmt(b.transition(next)...),
			},
		})
		initLabel := b.newLabel()
		b.l.jsvalueImport()
		b.l.addImport("fmt")
		b.addCase(initLabel, []ast.Stmt{
			assignStmt([]ast.Expr{goIdent(itemsName)}, []ast.Expr{b.l.wrapAsJSValue(callExpr(selectorExpr(b.l.lowerExpr(s.Value), "Array")))}),
			assignStmt([]ast.Expr{goIdent(indexName)}, []ast.Expr{callExpr(selectorExpr(goIdent("jsvalue"), "NewNumber"), intLit("0"))}),
			assignStmt([]ast.Expr{goIdent(b.stateName)}, []ast.Expr{intLit(itoa(condLabel))}),
			&ast.BranchStmt{Tok: token.CONTINUE},
		})
		b.addCase(postLabel, []ast.Stmt{
			assignStmt([]ast.Expr{goIdent(indexName)}, []ast.Expr{callExpr(selectorExpr(goIdent("jsvalue"), "Add"), goIdent(indexName), callExpr(selectorExpr(goIdent("jsvalue"), "NewNumber"), intLit("1")))}),
			assignStmt([]ast.Expr{goIdent(b.stateName)}, []ast.Expr{intLit(itoa(condLabel))}),
			&ast.BranchStmt{Tok: token.CONTINUE},
		})
		return initLabel
	case *hir.SwitchStmt:
		exitTarget := next
		switchLoop := asyncLoopLabels{breakLabel: exitTarget, continueLabel: loop.continueLabel, valid: true, namedBreak: loop.namedBreak, namedContinue: loop.namedContinue}
		clauseEntries := make([]int, len(s.Cases))
		nextEntry := exitTarget
		for i := len(s.Cases) - 1; i >= 0; i-- {
			clauseEntries[i] = b.compileSeq(s.Cases[i].Body, nextEntry, switchLoop, protected, finalizer)
			nextEntry = clauseEntries[i]
		}
		dispatchTarget := exitTarget
		for i := len(s.Cases) - 1; i >= 0; i-- {
			if s.Cases[i].Value == nil {
				dispatchTarget = clauseEntries[i]
				continue
			}
			label := b.newLabel()
			cond := b.l.ensureBool(callExpr(selectorExpr(goIdent("jsvalue"), "Eq"), b.l.wrapAsJSValue(b.l.lowerExpr(s.Tag)), b.l.wrapAsJSValue(b.l.lowerExpr(s.Cases[i].Value))))
			b.addCase(label, []ast.Stmt{
				&ast.IfStmt{
					Cond: cond,
					Body: blockStmt(b.transition(clauseEntries[i])...),
					Else: blockStmt(b.transition(dispatchTarget)...),
				},
			})
			dispatchTarget = label
		}
		return dispatchTarget
	case *hir.LabeledStmt:
		if s.Stmt == nil {
			return next
		}
		labeledLoop := asyncLoopLabels{
			breakLabel:    loop.breakLabel,
			continueLabel: loop.continueLabel,
			valid:         loop.valid,
			namedBreak:    cloneIntMap(loop.namedBreak),
			namedContinue: cloneIntMap(loop.namedContinue),
		}
		switch inner := s.Stmt.(type) {
		case *hir.ForStmt:
			exitTarget := next
			condEntryLabel := b.newLabel()
			postLabel := condEntryLabel
			if inner.Post != nil {
				postLabel = b.compileExprLoopStep(inner.Post, condEntryLabel, labeledLoop, protected, finalizer)
			}
			bodyLoop := asyncLoopLabels{
				breakLabel:    exitTarget,
				continueLabel: postLabel,
				valid:         true,
				namedBreak:    cloneIntMap(loop.namedBreak),
				namedContinue: cloneIntMap(loop.namedContinue),
			}
			if bodyLoop.namedBreak == nil {
				bodyLoop.namedBreak = map[string]int{}
			}
			if bodyLoop.namedContinue == nil {
				bodyLoop.namedContinue = map[string]int{}
			}
			bodyLoop.namedBreak[s.Label] = exitTarget
			bodyLoop.namedContinue[s.Label] = postLabel
			bodyEntry := b.compileSeq(inner.Body.Stmts, postLabel, bodyLoop, protected, finalizer)
			branchEntry := bodyEntry
			if inner.Cond != nil {
				branchEntry = b.compileConditionExpr(inner.Cond, bodyEntry, exitTarget, bodyLoop, protected, finalizer)
			}
			b.addCase(condEntryLabel, b.transition(branchEntry))
			if inner.Init != nil {
				return b.compileStmt(inner.Init, condEntryLabel, labeledLoop, protected, finalizer)
			}
			return condEntryLabel
		case *hir.WhileStmt:
			exitTarget := next
			condEntryLabel := b.newLabel()
			bodyLoop := asyncLoopLabels{
				breakLabel:    exitTarget,
				continueLabel: condEntryLabel,
				valid:         true,
				namedBreak:    cloneIntMap(loop.namedBreak),
				namedContinue: cloneIntMap(loop.namedContinue),
			}
			if bodyLoop.namedBreak == nil {
				bodyLoop.namedBreak = map[string]int{}
			}
			if bodyLoop.namedContinue == nil {
				bodyLoop.namedContinue = map[string]int{}
			}
			bodyLoop.namedBreak[s.Label] = exitTarget
			bodyLoop.namedContinue[s.Label] = condEntryLabel
			bodyEntry := b.compileSeq(inner.Body.Stmts, condEntryLabel, bodyLoop, protected, finalizer)
			branchEntry := b.compileConditionExpr(inner.Cond, bodyEntry, exitTarget, bodyLoop, protected, finalizer)
			b.addCase(condEntryLabel, b.transition(branchEntry))
			return condEntryLabel
		case *hir.DoWhileStmt:
			exitTarget := next
			condEntryLabel := b.newLabel()
			bodyLoop := asyncLoopLabels{
				breakLabel:    exitTarget,
				continueLabel: condEntryLabel,
				valid:         true,
				namedBreak:    cloneIntMap(loop.namedBreak),
				namedContinue: cloneIntMap(loop.namedContinue),
			}
			if bodyLoop.namedBreak == nil {
				bodyLoop.namedBreak = map[string]int{}
			}
			if bodyLoop.namedContinue == nil {
				bodyLoop.namedContinue = map[string]int{}
			}
			bodyLoop.namedBreak[s.Label] = exitTarget
			bodyLoop.namedContinue[s.Label] = condEntryLabel
			bodyEntry := b.compileSeq(inner.Body.Stmts, condEntryLabel, bodyLoop, protected, finalizer)
			branchEntry := b.compileConditionExpr(inner.Cond, bodyEntry, exitTarget, bodyLoop, protected, finalizer)
			b.addCase(condEntryLabel, b.transition(branchEntry))
			return bodyEntry
		case *hir.ForInStmt:
			itemsName := b.l.emitName(b.l.newAsyncTempSymbol())
			indexName := b.l.emitName(b.l.newAsyncTempSymbol())
			postLabel := b.newLabel()
			prepLabel := b.newLabel()
			bodyLoop := asyncLoopLabels{
				breakLabel:    next,
				continueLabel: postLabel,
				valid:         true,
				namedBreak:    cloneIntMap(loop.namedBreak),
				namedContinue: cloneIntMap(loop.namedContinue),
			}
			if bodyLoop.namedBreak == nil {
				bodyLoop.namedBreak = map[string]int{}
			}
			if bodyLoop.namedContinue == nil {
				bodyLoop.namedContinue = map[string]int{}
			}
			bodyLoop.namedBreak[s.Label] = next
			bodyLoop.namedContinue[s.Label] = postLabel
			bodyEntry := b.compileSeq(inner.Body.Stmts, postLabel, bodyLoop, protected, finalizer)
			itemExpr := callExpr(selectorExpr(goIdent(itemsName), "Get"), callExpr(selectorExpr(goIdent("fmt"), "Sprint"), goIdent(indexName)))
			prepBody := []ast.Stmt{}
			if inner.Key != nil {
				prepBody = append(prepBody, assignStmt([]ast.Expr{goIdent(b.l.emitName(inner.Key))}, []ast.Expr{itemExpr}))
			}
			prepBody = append(prepBody, b.transition(bodyEntry)...)
			b.addCase(prepLabel, prepBody)
			condLabel := b.newLabel()
			cond := b.l.ensureBool(callExpr(
				selectorExpr(goIdent("jsvalue"), "Lt"),
				goIdent(indexName),
				callExpr(selectorExpr(goIdent("jsvalue"), "NewNumber"),
					callExpr(goIdent("float64"), callExpr(selectorExpr(goIdent(itemsName), "Len")))),
			))
			b.addCase(condLabel, []ast.Stmt{
				&ast.IfStmt{Cond: cond, Body: blockStmt(b.transition(prepLabel)...), Else: blockStmt(b.transition(next)...)},
			})
			initLabel := b.newLabel()
			b.l.jsvalueImport()
			b.l.addImport("fmt")
			b.addCase(initLabel, []ast.Stmt{
				assignStmt([]ast.Expr{goIdent(itemsName)}, []ast.Expr{callExpr(selectorExpr(goIdent("jsvalue"), "Keys"), b.l.wrapAsJSValue(b.l.lowerExpr(inner.Value)))}),
				assignStmt([]ast.Expr{goIdent(indexName)}, []ast.Expr{callExpr(selectorExpr(goIdent("jsvalue"), "NewNumber"), intLit("0"))}),
				assignStmt([]ast.Expr{goIdent(b.stateName)}, []ast.Expr{intLit(itoa(condLabel))}),
				&ast.BranchStmt{Tok: token.CONTINUE},
			})
			b.addCase(postLabel, []ast.Stmt{
				assignStmt([]ast.Expr{goIdent(indexName)}, []ast.Expr{callExpr(selectorExpr(goIdent("jsvalue"), "Add"), goIdent(indexName), callExpr(selectorExpr(goIdent("jsvalue"), "NewNumber"), intLit("1")))}),
				assignStmt([]ast.Expr{goIdent(b.stateName)}, []ast.Expr{intLit(itoa(condLabel))}),
				&ast.BranchStmt{Tok: token.CONTINUE},
			})
			return initLabel
		case *hir.ForOfStmt:
			itemsName := b.l.emitName(b.l.newAsyncTempSymbol())
			indexName := b.l.emitName(b.l.newAsyncTempSymbol())
			postLabel := b.newLabel()
			prepLabel := b.newLabel()
			bodyLoop := asyncLoopLabels{
				breakLabel:    next,
				continueLabel: postLabel,
				valid:         true,
				namedBreak:    cloneIntMap(loop.namedBreak),
				namedContinue: cloneIntMap(loop.namedContinue),
			}
			if bodyLoop.namedBreak == nil {
				bodyLoop.namedBreak = map[string]int{}
			}
			if bodyLoop.namedContinue == nil {
				bodyLoop.namedContinue = map[string]int{}
			}
			bodyLoop.namedBreak[s.Label] = next
			bodyLoop.namedContinue[s.Label] = postLabel
			bodyEntry := b.compileSeq(inner.Body.Stmts, postLabel, bodyLoop, protected, finalizer)
			itemExpr := callExpr(selectorExpr(goIdent(itemsName), "Get"), callExpr(selectorExpr(goIdent("fmt"), "Sprint"), goIdent(indexName)))
			prepBody := []ast.Stmt{}
			if inner.Pattern != nil {
				prepBody = append(prepBody, b.l.lowerDestructuring(inner.Pattern, itemExpr, false)...)
			} else if inner.Elem != nil {
				prepBody = append(prepBody, assignStmt([]ast.Expr{goIdent(b.l.emitName(inner.Elem))}, []ast.Expr{itemExpr}))
			}
			prepBody = append(prepBody, b.transition(bodyEntry)...)
			b.addCase(prepLabel, prepBody)
			condLabel := b.newLabel()
			cond := b.l.ensureBool(callExpr(
				selectorExpr(goIdent("jsvalue"), "Lt"),
				goIdent(indexName),
				callExpr(selectorExpr(goIdent("jsvalue"), "NewNumber"),
					callExpr(goIdent("float64"), callExpr(selectorExpr(goIdent(itemsName), "Len")))),
			))
			b.addCase(condLabel, []ast.Stmt{
				&ast.IfStmt{Cond: cond, Body: blockStmt(b.transition(prepLabel)...), Else: blockStmt(b.transition(next)...)},
			})
			initLabel := b.newLabel()
			b.l.jsvalueImport()
			b.l.addImport("fmt")
			b.addCase(initLabel, []ast.Stmt{
				assignStmt([]ast.Expr{goIdent(itemsName)}, []ast.Expr{b.l.wrapAsJSValue(callExpr(selectorExpr(b.l.lowerExpr(inner.Value), "Array")))}),
				assignStmt([]ast.Expr{goIdent(indexName)}, []ast.Expr{callExpr(selectorExpr(goIdent("jsvalue"), "NewNumber"), intLit("0"))}),
				assignStmt([]ast.Expr{goIdent(b.stateName)}, []ast.Expr{intLit(itoa(condLabel))}),
				&ast.BranchStmt{Tok: token.CONTINUE},
			})
			b.addCase(postLabel, []ast.Stmt{
				assignStmt([]ast.Expr{goIdent(indexName)}, []ast.Expr{callExpr(selectorExpr(goIdent("jsvalue"), "Add"), goIdent(indexName), callExpr(selectorExpr(goIdent("jsvalue"), "NewNumber"), intLit("1")))}),
				assignStmt([]ast.Expr{goIdent(b.stateName)}, []ast.Expr{intLit(itoa(condLabel))}),
				&ast.BranchStmt{Tok: token.CONTINUE},
			})
			return initLabel
		case *hir.SwitchStmt:
			exitTarget := next
			switchLoop := asyncLoopLabels{
				breakLabel:    exitTarget,
				continueLabel: loop.continueLabel,
				valid:         true,
				namedBreak:    cloneIntMap(loop.namedBreak),
				namedContinue: cloneIntMap(loop.namedContinue),
			}
			if switchLoop.namedBreak == nil {
				switchLoop.namedBreak = map[string]int{}
			}
			switchLoop.namedBreak[s.Label] = exitTarget
			clauseEntries := make([]int, len(inner.Cases))
			nextEntry := exitTarget
			for i := len(inner.Cases) - 1; i >= 0; i-- {
				clauseEntries[i] = b.compileSeq(inner.Cases[i].Body, nextEntry, switchLoop, protected, finalizer)
				nextEntry = clauseEntries[i]
			}
			dispatchTarget := exitTarget
			for i := len(inner.Cases) - 1; i >= 0; i-- {
				if inner.Cases[i].Value == nil {
					dispatchTarget = clauseEntries[i]
					continue
				}
				label := b.newLabel()
				cond := b.l.ensureBool(callExpr(selectorExpr(goIdent("jsvalue"), "Eq"), b.l.wrapAsJSValue(b.l.lowerExpr(inner.Tag)), b.l.wrapAsJSValue(b.l.lowerExpr(inner.Cases[i].Value))))
				b.addCase(label, []ast.Stmt{
					&ast.IfStmt{Cond: cond, Body: blockStmt(b.transition(clauseEntries[i])...), Else: blockStmt(b.transition(dispatchTarget)...)},
				})
				dispatchTarget = label
			}
			return dispatchTarget
		default:
			return b.compileStmt(inner, next, labeledLoop, protected, finalizer)
		}
	case *hir.TryCatchStmt:
		if s.Catch == nil && s.Finally == nil {
			label := b.newLabel()
			b.addCase(label, []ast.Stmt{
				exprStmt(callExpr(selectorExpr(goIdent(b.rejectName), "Call"),
					callExpr(selectorExpr(goIdent("jsvalue"), "NewString"),
						stringLit("unsupported async protected statement shape")))),
				returnStmt(callExpr(selectorExpr(goIdent("jsvalue"), "NewUndefined"))),
			})
			return label
		}
		if s.Finally != nil {
			dispatchLabel := b.newLabel()
			b.addCase(dispatchLabel, b.finalizerDispatch())
			finallyEntry := b.compileSeq(s.Finally.Stmts, dispatchLabel, loop, asyncProtected{}, asyncFinalizer{})
			normalToFinally := b.newLabel()
			b.addCase(normalToFinally, b.finalizerSetJump(next, false, asyncFinalizer{label: finallyEntry, valid: true}))
			catchLabel := normalToFinally
			catchName := ""
			if s.Catch != nil {
				catchName = b.l.emitName(s.Catch.Param)
				catchLabel = b.compileSeq(s.Catch.Body.Stmts, normalToFinally, loop, asyncProtected{}, asyncFinalizer{label: finallyEntry, valid: true})
			}
			return b.compileSeq(s.Try.Stmts, normalToFinally, loop, asyncProtected{
				catchLabel: catchLabel,
				catchName:  catchName,
				valid:      s.Catch != nil,
			}, asyncFinalizer{label: finallyEntry, valid: true})
		}
		catchLabel := b.compileSeq(s.Catch.Body.Stmts, next, loop, asyncProtected{}, asyncFinalizer{})
		catchName := ""
		if s.Catch.Param != nil {
			catchName = b.l.emitName(s.Catch.Param)
		}
		return b.compileSeq(s.Try.Stmts, next, loop, asyncProtected{
			catchLabel: catchLabel,
			catchName:  catchName,
			valid:      true,
		}, asyncFinalizer{})
	case *hir.BreakStmt:
		label := b.newLabel()
		target := next
		if s.Label != "" && loop.namedBreak != nil {
			if named, ok := loop.namedBreak[s.Label]; ok {
				target = named
			}
		} else if loop.valid {
			target = loop.breakLabel
		}
		if finalizer.valid {
			b.addCase(label, b.finalizerSetJump(target, false, finalizer))
		} else {
			b.addCase(label, b.transition(target))
		}
		return label
	case *hir.ContinueStmt:
		label := b.newLabel()
		target := next
		if s.Label != "" && loop.namedContinue != nil {
			if named, ok := loop.namedContinue[s.Label]; ok {
				target = named
			}
		} else if loop.valid {
			target = loop.continueLabel
		}
		if finalizer.valid {
			b.addCase(label, b.finalizerSetJump(target, false, finalizer))
		} else {
			b.addCase(label, b.transition(target))
		}
		return label
	default:
		label := b.newLabel()
		b.addCase(label, []ast.Stmt{
			exprStmt(callExpr(selectorExpr(goIdent(b.rejectName), "Call"),
				callExpr(selectorExpr(goIdent("jsvalue"), "NewString"),
					stringLit("unsupported async statement shape")))),
			returnStmt(callExpr(selectorExpr(goIdent("jsvalue"), "NewUndefined"))),
		})
		return label
	}
}

func (b *asyncFuncBuilder) transition(next int) []ast.Stmt {
	return []ast.Stmt{
		assignStmt([]ast.Expr{goIdent(b.stateName)}, []ast.Expr{intLit(itoa(next))}),
		&ast.BranchStmt{Tok: token.CONTINUE},
	}
}

func (b *asyncFuncBuilder) callbackFuncLit(body []ast.Stmt) *ast.FuncLit {
	return &ast.FuncLit{
		Type: &ast.FuncType{
			Params: fieldList(&ast.Field{
				Names: []*ast.Ident{goIdent("_args")},
				Type:  &ast.Ellipsis{Elt: jsValuePtrType()},
			}),
			Results: fieldList(goField("", jsValuePtrType())),
		},
		Body: &ast.BlockStmt{List: body},
	}
}

func (b *asyncFuncBuilder) stepFuncLit() *ast.FuncLit {
	var clauses []ast.Stmt
	for _, c := range b.cases {
		clauses = append(clauses, &ast.CaseClause{
			List: []ast.Expr{intLit(itoa(c.label))},
			Body: c.body,
		})
	}
	clauses = append(clauses, &ast.CaseClause{
		Body: []ast.Stmt{returnStmt(callExpr(selectorExpr(goIdent("jsvalue"), "NewUndefined")))},
	})
	return b.callbackFuncLit([]ast.Stmt{
		&ast.ForStmt{
			Body: blockStmt(&ast.SwitchStmt{
				Tag:  goIdent(b.stateName),
				Body: &ast.BlockStmt{List: clauses},
			}),
		},
		returnStmt(callExpr(selectorExpr(goIdent("jsvalue"), "NewUndefined"))),
	})
}

func (b *asyncFuncBuilder) awaitCase(value hir.Expr, next int, onFulfilled []ast.Stmt, onRejected []ast.Stmt, protected asyncProtected, finalizer asyncFinalizer) []ast.Stmt {
	if next >= 0 {
		onFulfilled = append(onFulfilled, exprStmt(callExpr(selectorExpr(goIdent(b.stepName), "Call"))))
	}
	onFulfilled = append(onFulfilled, returnStmt(callExpr(selectorExpr(goIdent("jsvalue"), "NewUndefined"))))
	if onRejected == nil {
		if protected.valid {
			onRejected = b.catchCallbackTransition(goIdent(b.awaitValueName), protected)
		} else if finalizer.valid {
			onRejected = b.finalizerSetReject(goIdent(b.awaitValueName), true, finalizer)
		} else {
			onRejected = []ast.Stmt{
				exprStmt(callExpr(selectorExpr(goIdent(b.rejectName), "Call"), goIdent(b.awaitValueName))),
				returnStmt(callExpr(selectorExpr(goIdent("jsvalue"), "NewUndefined"))),
			}
		}
	}
	body := []ast.Stmt{}
	if next >= 0 {
		body = append(body, assignStmt([]ast.Expr{goIdent(b.stateName)}, []ast.Expr{intLit(itoa(next))}))
	}
	fulfilledFn := callExpr(selectorExpr(goIdent("jsvalue"), "NewFunction"), b.callbackFuncLit(append([]ast.Stmt{
		&ast.IfStmt{
			Cond: &ast.BinaryExpr{
				X:  callExpr(goIdent("len"), goIdent("_args")),
				Op: token.GTR,
				Y:  intLit("0"),
			},
			Body: blockStmt(assignStmt(
				[]ast.Expr{goIdent(b.awaitValueName)},
				[]ast.Expr{&ast.IndexExpr{X: goIdent("_args"), Index: intLit("0")}},
			)),
		},
	}, onFulfilled...)))
	rejectedFn := callExpr(selectorExpr(goIdent("jsvalue"), "NewFunction"), b.callbackFuncLit(append([]ast.Stmt{
		assignStmt([]ast.Expr{goIdent(b.awaitValueName)}, []ast.Expr{callExpr(selectorExpr(goIdent("jsvalue"), "NewUndefined"))}),
		&ast.IfStmt{
			Cond: &ast.BinaryExpr{
				X:  callExpr(goIdent("len"), goIdent("_args")),
				Op: token.GTR,
				Y:  intLit("0"),
			},
			Body: blockStmt(assignStmt(
				[]ast.Expr{goIdent(b.awaitValueName)},
				[]ast.Expr{&ast.IndexExpr{X: goIdent("_args"), Index: intLit("0")}},
			)),
		},
	}, onRejected...)))
	resolvePromise := callExpr(
		selectorExpr(
			callExpr(selectorExpr(selectorExpr(goIdent("promise"), "Promise"), "Get"), stringLit("resolve")),
			"Call",
		),
		b.l.wrapAsJSValue(b.l.lowerExpr(value)),
	)
	resolvePromiseName := b.l.nextSyntheticName("_async_resolved")
	panicName := b.l.nextSyntheticName("_async_panic")
	thenCall := callExpr(
		selectorExpr(goIdent(resolvePromiseName), "MethodCall"),
		stringLit("then"),
		fulfilledFn,
		rejectedFn,
	)
	body = append(body,
		assignStmt([]ast.Expr{goIdent(b.awaitValueName)}, []ast.Expr{callExpr(selectorExpr(goIdent("jsvalue"), "NewUndefined"))}),
		&ast.DeclStmt{Decl: varDecl(resolvePromiseName, jsValuePtrType(), nil)},
		&ast.DeclStmt{Decl: varDecl(panicName, jsValuePtrType(), nil)},
		exprStmt(callExpr(&ast.FuncLit{
			Type: &ast.FuncType{Params: fieldList()},
			Body: blockStmt(
				&ast.DeferStmt{
					Call: callExpr(&ast.FuncLit{
						Type: &ast.FuncType{Params: fieldList()},
						Body: blockStmt(
							&ast.IfStmt{
								Init: assignDefine(
									[]ast.Expr{goIdent("r")},
									[]ast.Expr{callExpr(goIdent("recover"))},
								),
								Cond: &ast.BinaryExpr{X: goIdent("r"), Op: token.NEQ, Y: goIdent("nil")},
								Body: blockStmt(assignStmt(
									[]ast.Expr{goIdent(panicName)},
									[]ast.Expr{callExpr(selectorExpr(goIdent("jsvalue"), "From"), goIdent("r"))},
								)),
							},
						),
					}),
				},
				assignStmt([]ast.Expr{goIdent(resolvePromiseName)}, []ast.Expr{resolvePromise}),
			),
		})),
		&ast.IfStmt{
			Cond: &ast.BinaryExpr{X: goIdent(panicName), Op: token.NEQ, Y: goIdent("nil")},
			Body: blockStmt(b.asyncPanicTransition(goIdent(panicName), protected, finalizer)...),
		},
		exprStmt(thenCall),
		returnStmt(callExpr(selectorExpr(goIdent("jsvalue"), "NewUndefined"))),
	)
	return body
}

func (b *asyncFuncBuilder) asyncPanicTransition(errExpr ast.Expr, protected asyncProtected, finalizer asyncFinalizer) []ast.Stmt {
	if protected.valid {
		return b.catchTransition(errExpr, protected)
	}
	if finalizer.valid {
		return b.finalizerSetReject(errExpr, false, finalizer)
	}
	return []ast.Stmt{
		exprStmt(callExpr(selectorExpr(goIdent(b.rejectName), "Call"), errExpr)),
		returnStmt(callExpr(selectorExpr(goIdent("jsvalue"), "NewUndefined"))),
	}
}

func (b *asyncFuncBuilder) catchTransition(errExpr ast.Expr, protected asyncProtected) []ast.Stmt {
	if !protected.valid {
		return []ast.Stmt{
			exprStmt(callExpr(selectorExpr(goIdent(b.rejectName), "Call"), errExpr)),
			returnStmt(callExpr(selectorExpr(goIdent("jsvalue"), "NewUndefined"))),
		}
	}
	var body []ast.Stmt
	if protected.catchName != "" {
		body = append(body, assignStmt([]ast.Expr{goIdent(protected.catchName)}, []ast.Expr{errExpr}))
	}
	body = append(body,
		assignStmt([]ast.Expr{goIdent(b.stateName)}, []ast.Expr{intLit(itoa(protected.catchLabel))}),
		&ast.BranchStmt{Tok: token.CONTINUE},
	)
	return body
}

func (b *asyncFuncBuilder) catchCallbackTransition(errExpr ast.Expr, protected asyncProtected) []ast.Stmt {
	if !protected.valid {
		return []ast.Stmt{
			exprStmt(callExpr(selectorExpr(goIdent(b.rejectName), "Call"), errExpr)),
			returnStmt(callExpr(selectorExpr(goIdent("jsvalue"), "NewUndefined"))),
		}
	}
	var body []ast.Stmt
	if protected.catchName != "" {
		body = append(body, assignStmt([]ast.Expr{goIdent(protected.catchName)}, []ast.Expr{errExpr}))
	}
	body = append(body,
		assignStmt([]ast.Expr{goIdent(b.stateName)}, []ast.Expr{intLit(itoa(protected.catchLabel))}),
		exprStmt(callExpr(selectorExpr(goIdent(b.stepName), "Call"))),
		returnStmt(callExpr(selectorExpr(goIdent("jsvalue"), "NewUndefined"))),
	)
	return body
}

func (b *asyncFuncBuilder) finalizerSetJump(target int, callback bool, finalizer asyncFinalizer) []ast.Stmt {
	body := []ast.Stmt{
		assignStmt([]ast.Expr{goIdent(b.completeKind)}, []ast.Expr{intLit("1")}),
		assignStmt([]ast.Expr{goIdent(b.completeTarget)}, []ast.Expr{intLit(itoa(target))}),
		assignStmt([]ast.Expr{goIdent(b.completeValue)}, []ast.Expr{callExpr(selectorExpr(goIdent("jsvalue"), "NewUndefined"))}),
		assignStmt([]ast.Expr{goIdent(b.stateName)}, []ast.Expr{intLit(itoa(finalizer.label))}),
	}
	if callback {
		body = append(body,
			exprStmt(callExpr(selectorExpr(goIdent(b.stepName), "Call"))),
			returnStmt(callExpr(selectorExpr(goIdent("jsvalue"), "NewUndefined"))),
		)
	} else {
		body = append(body, &ast.BranchStmt{Tok: token.CONTINUE})
	}
	return body
}

func (b *asyncFuncBuilder) finalizerSetReturn(value ast.Expr, callback bool, finalizer asyncFinalizer) []ast.Stmt {
	body := []ast.Stmt{
		assignStmt([]ast.Expr{goIdent(b.completeKind)}, []ast.Expr{intLit("2")}),
		assignStmt([]ast.Expr{goIdent(b.completeValue)}, []ast.Expr{value}),
		assignStmt([]ast.Expr{goIdent(b.stateName)}, []ast.Expr{intLit(itoa(finalizer.label))}),
	}
	if callback {
		body = append(body,
			exprStmt(callExpr(selectorExpr(goIdent(b.stepName), "Call"))),
			returnStmt(callExpr(selectorExpr(goIdent("jsvalue"), "NewUndefined"))),
		)
	} else {
		body = append(body, &ast.BranchStmt{Tok: token.CONTINUE})
	}
	return body
}

func (b *asyncFuncBuilder) finalizerSetReject(value ast.Expr, callback bool, finalizer asyncFinalizer) []ast.Stmt {
	body := []ast.Stmt{
		assignStmt([]ast.Expr{goIdent(b.completeKind)}, []ast.Expr{intLit("3")}),
		assignStmt([]ast.Expr{goIdent(b.completeValue)}, []ast.Expr{value}),
		assignStmt([]ast.Expr{goIdent(b.stateName)}, []ast.Expr{intLit(itoa(finalizer.label))}),
	}
	if callback {
		body = append(body,
			exprStmt(callExpr(selectorExpr(goIdent(b.stepName), "Call"))),
			returnStmt(callExpr(selectorExpr(goIdent("jsvalue"), "NewUndefined"))),
		)
	} else {
		body = append(body, &ast.BranchStmt{Tok: token.CONTINUE})
	}
	return body
}

func (b *asyncFuncBuilder) finalizerDispatch() []ast.Stmt {
	return []ast.Stmt{
		&ast.IfStmt{
			Cond: &ast.BinaryExpr{X: goIdent(b.completeKind), Op: token.EQL, Y: intLit("2")},
			Body: blockStmt(
				exprStmt(callExpr(selectorExpr(goIdent(b.resolveName), "Call"), goIdent(b.completeValue))),
				returnStmt(callExpr(selectorExpr(goIdent("jsvalue"), "NewUndefined"))),
			),
		},
		&ast.IfStmt{
			Cond: &ast.BinaryExpr{X: goIdent(b.completeKind), Op: token.EQL, Y: intLit("3")},
			Body: blockStmt(
				exprStmt(callExpr(selectorExpr(goIdent(b.rejectName), "Call"), goIdent(b.completeValue))),
				returnStmt(callExpr(selectorExpr(goIdent("jsvalue"), "NewUndefined"))),
			),
		},
		assignStmt([]ast.Expr{goIdent(b.stateName)}, []ast.Expr{goIdent(b.completeTarget)}),
		&ast.BranchStmt{Tok: token.CONTINUE},
	}
}

func cloneIntMap(src map[string]int) map[string]int {
	if src == nil {
		return nil
	}
	dst := make(map[string]int, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func (b *asyncFuncBuilder) compileConditionExpr(expr hir.Expr, trueTarget, falseTarget int, loop asyncLoopLabels, protected asyncProtected, finalizer asyncFinalizer) int {
	prefix, normalized := b.l.normalizeAsyncExpr(expr, false)
	label := b.newLabel()
	cond := b.l.ensureBool(b.l.lowerExpr(normalized))
	b.addCase(label, []ast.Stmt{
		&ast.IfStmt{
			Cond: cond,
			Body: blockStmt(b.transition(trueTarget)...),
			Else: blockStmt(b.transition(falseTarget)...),
		},
	})
	if len(prefix) == 0 {
		return label
	}
	return b.compileSeq(prefix, label, loop, protected, finalizer)
}

func (b *asyncFuncBuilder) compileExprLoopStep(expr hir.Expr, next int, loop asyncLoopLabels, protected asyncProtected, finalizer asyncFinalizer) int {
	prefix, normalized := b.l.normalizeAsyncExpr(expr, true)
	stmts := append(prefix, &hir.ExprStmt{Expr: normalized})
	return b.compileSeq(stmts, next, loop, protected, finalizer)
}

func asyncDirectAwait(e hir.Expr) (*hir.AwaitExpr, bool) {
	switch e := e.(type) {
	case *hir.AwaitExpr:
		return e, true
	case *hir.ParenExpr:
		return asyncDirectAwait(e.Expr)
	default:
		return nil, false
	}
}

func (l *Lowerer) normalizeAsyncBlock(body *hir.BlockStmt) *hir.BlockStmt {
	if body == nil {
		return &hir.BlockStmt{}
	}
	var out []hir.Stmt
	for _, st := range body.Stmts {
		out = append(out, l.normalizeAsyncStmt(st)...)
	}
	return &hir.BlockStmt{Stmts: out, Span: body.Span}
}

func (l *Lowerer) normalizeAsyncStmt(stmt hir.Stmt) []hir.Stmt {
	switch s := stmt.(type) {
	case nil:
		return nil
	case *hir.BlockStmt:
		return []hir.Stmt{l.normalizeAsyncBlock(s)}
	case *hir.ExprStmt:
		prefix, expr := l.normalizeAsyncExpr(s.Expr, true)
		return append(prefix, &hir.ExprStmt{Expr: expr, Span: s.Span})
	case *hir.ReturnStmt:
		prefix, expr := l.normalizeAsyncExpr(s.Value, true)
		return append(prefix, &hir.ReturnStmt{Value: expr, Span: s.Span})
	case *hir.ThrowStmt:
		prefix, expr := l.normalizeAsyncExpr(s.Value, true)
		return append(prefix, &hir.ThrowStmt{Value: expr, Span: s.Span})
	case *hir.VarDecl:
		var out []hir.Stmt
		decl := &hir.VarDecl{Kind: s.Kind}
		for _, d := range s.Declarators {
			prefix, init := l.normalizeAsyncExpr(d.Init, true)
			out = append(out, prefix...)
			decl.Declarators = append(decl.Declarators, &hir.Declarator{
				Symbol:  d.Symbol,
				Pattern: d.Pattern,
				Init:    init,
			})
		}
		return append(out, decl)
	case *hir.IfStmt:
		prefix, cond := l.normalizeAsyncExpr(s.Cond, false)
		var elseStmt hir.Stmt
		if s.Else != nil {
			elseNormalized := l.normalizeAsyncStmt(s.Else)
			if len(elseNormalized) == 1 {
				elseStmt = elseNormalized[0]
			} else {
				elseStmt = &hir.BlockStmt{Stmts: elseNormalized}
			}
		}
		return append(prefix, &hir.IfStmt{
			Cond: cond,
			Then: l.normalizeAsyncBlock(s.Then),
			Else: elseStmt,
			Span: s.Span,
		})
	case *hir.WhileStmt:
		return []hir.Stmt{&hir.WhileStmt{Cond: s.Cond, Body: l.normalizeAsyncBlock(s.Body), Span: s.Span}}
	case *hir.DoWhileStmt:
		return []hir.Stmt{&hir.DoWhileStmt{Body: l.normalizeAsyncBlock(s.Body), Cond: s.Cond, Span: s.Span}}
	case *hir.ForStmt:
		var init hir.Stmt
		if s.Init != nil {
			initStmts := l.normalizeAsyncStmt(s.Init)
			if len(initStmts) == 1 {
				init = initStmts[0]
			} else if len(initStmts) > 1 {
				init = &hir.BlockStmt{Stmts: initStmts}
			}
		}
		return []hir.Stmt{&hir.ForStmt{
			Init: init,
			Cond: s.Cond,
			Post: s.Post,
			Body: l.normalizeAsyncBlock(s.Body),
			Span: s.Span,
		}}
	case *hir.ForInStmt:
		prefix, value := l.normalizeAsyncExpr(s.Value, false)
		return append(prefix, &hir.ForInStmt{
			Key:   s.Key,
			Value: value,
			Body:  l.normalizeAsyncBlock(s.Body),
			Span:  s.Span,
		})
	case *hir.ForOfStmt:
		prefix, value := l.normalizeAsyncExpr(s.Value, false)
		return append(prefix, &hir.ForOfStmt{
			Elem:    s.Elem,
			Pattern: s.Pattern,
			Value:   value,
			Body:    l.normalizeAsyncBlock(s.Body),
			Span:    s.Span,
		})
	case *hir.TryCatchStmt:
		var catchClause *hir.CatchClause
		if s.Catch != nil {
			catchClause = &hir.CatchClause{Param: s.Catch.Param, Body: l.normalizeAsyncBlock(s.Catch.Body)}
		}
		var finallyBlock *hir.BlockStmt
		if s.Finally != nil {
			finallyBlock = l.normalizeAsyncBlock(s.Finally)
		}
		return []hir.Stmt{&hir.TryCatchStmt{
			Try:     l.normalizeAsyncBlock(s.Try),
			Catch:   catchClause,
			Finally: finallyBlock,
			Span:    s.Span,
		}}
	case *hir.SwitchStmt:
		prefix, tag := l.normalizeAsyncExpr(s.Tag, false)
		cases := make([]*hir.CaseClause, 0, len(s.Cases))
		for _, c := range s.Cases {
			casePrefix, caseValue := l.normalizeAsyncExpr(c.Value, false)
			prefix = append(prefix, casePrefix...)
			cases = append(cases, &hir.CaseClause{
				Value: caseValue,
				Body:  l.normalizeAsyncBlock(&hir.BlockStmt{Stmts: c.Body}).Stmts,
			})
		}
		return append(prefix, &hir.SwitchStmt{Tag: tag, Cases: cases, Span: s.Span})
	default:
		return []hir.Stmt{stmt}
	}
}

func (l *Lowerer) normalizeAsyncExpr(expr hir.Expr, allowDirectAwait bool) ([]hir.Stmt, hir.Expr) {
	switch e := expr.(type) {
	case nil:
		return nil, nil
	case *hir.AwaitExpr:
		prefix, value := l.normalizeAsyncExpr(e.Value, false)
		awaitExpr := &hir.AwaitExpr{Value: value, Span: e.Span}
		if allowDirectAwait {
			return prefix, awaitExpr
		}
		tmp := l.newAsyncTempSymbol()
		prefix = append(prefix, &hir.VarDecl{
			Kind: hir.VarLet,
			Declarators: []*hir.Declarator{{
				Symbol: tmp,
				Init:   awaitExpr,
			}},
		})
		return prefix, &hir.Identifier{Sym: tmp, Name: tmp.OriginalName}
	case *hir.BinaryExpr:
		prefix, left := l.normalizeAsyncExpr(e.Left, false)
		rightPrefix, right := l.normalizeAsyncExpr(e.Right, false)
		prefix = append(prefix, rightPrefix...)
		return prefix, &hir.BinaryExpr{Op: e.Op, Left: left, Right: right}
	case *hir.UnaryExpr:
		prefix, operand := l.normalizeAsyncExpr(e.Operand, false)
		return prefix, &hir.UnaryExpr{Op: e.Op, Operand: operand, Prefix: e.Prefix}
	case *hir.UpdateExpr:
		prefix, operand := l.normalizeAsyncExpr(e.Operand, false)
		return prefix, &hir.UpdateExpr{Op: e.Op, Operand: operand, Prefix: e.Prefix}
	case *hir.AssignExpr:
		prefix, left := l.normalizeAsyncExpr(e.Left, false)
		rightPrefix, right := l.normalizeAsyncExpr(e.Right, allowDirectAwait)
		prefix = append(prefix, rightPrefix...)
		return prefix, &hir.AssignExpr{Op: e.Op, Left: left, LeftPattern: e.LeftPattern, Right: right, Span: e.Span}
	case *hir.CallExpr:
		prefix, fn := l.normalizeAsyncExpr(e.Func, false)
		var args []hir.Expr
		for _, arg := range e.Args {
			argPrefix, argExpr := l.normalizeAsyncExpr(arg, false)
			prefix = append(prefix, argPrefix...)
			args = append(args, argExpr)
		}
		return prefix, &hir.CallExpr{Func: fn, Args: args, Span: e.Span}
	case *hir.NewExpr:
		prefix, callee := l.normalizeAsyncExpr(e.Callee, false)
		var args []hir.Expr
		for _, arg := range e.Args {
			argPrefix, argExpr := l.normalizeAsyncExpr(arg, false)
			prefix = append(prefix, argPrefix...)
			args = append(args, argExpr)
		}
		return prefix, &hir.NewExpr{Callee: callee, Args: args, Span: e.Span}
	case *hir.MemberExpr:
		prefix, object := l.normalizeAsyncExpr(e.Object, false)
		return prefix, &hir.MemberExpr{Object: object, Property: e.Property, Private: e.Private, Optional: e.Optional, Span: e.Span}
	case *hir.ComputedMemberExpr:
		prefix, object := l.normalizeAsyncExpr(e.Object, false)
		propPrefix, prop := l.normalizeAsyncExpr(e.Property, false)
		prefix = append(prefix, propPrefix...)
		return prefix, &hir.ComputedMemberExpr{Object: object, Property: prop, Span: e.Span}
	case *hir.TernaryExpr:
		prefix, cond := l.normalizeAsyncExpr(e.Cond, false)
		thenPrefix, thenExpr := l.normalizeAsyncExpr(e.Then, false)
		elsePrefix, elseExpr := l.normalizeAsyncExpr(e.Else, false)
		prefix = append(prefix, thenPrefix...)
		prefix = append(prefix, elsePrefix...)
		return prefix, &hir.TernaryExpr{Cond: cond, Then: thenExpr, Else: elseExpr}
	case *hir.ArrayLiteral:
		var prefix []hir.Stmt
		var elems []hir.Expr
		for _, el := range e.Elements {
			elPrefix, elExpr := l.normalizeAsyncExpr(el, false)
			prefix = append(prefix, elPrefix...)
			elems = append(elems, elExpr)
		}
		return prefix, &hir.ArrayLiteral{Elements: elems}
	case *hir.ObjectLiteral:
		var prefix []hir.Stmt
		props := make([]*hir.Property, 0, len(e.Properties))
		for _, prop := range e.Properties {
			keyPrefix, key := l.normalizeAsyncExpr(prop.Key, false)
			valPrefix, val := l.normalizeAsyncExpr(prop.Value, false)
			prefix = append(prefix, keyPrefix...)
			prefix = append(prefix, valPrefix...)
			props = append(props, &hir.Property{
				Key:      key,
				KeyName:  prop.KeyName,
				Value:    val,
				Computed: prop.Computed,
				Method:   prop.Method,
			})
		}
		return prefix, &hir.ObjectLiteral{Properties: props}
	case *hir.TemplateLiteral:
		var prefix []hir.Stmt
		parts := make([]hir.Expr, 0, len(e.Parts))
		for _, part := range e.Parts {
			partPrefix, partExpr := l.normalizeAsyncExpr(part, false)
			prefix = append(prefix, partPrefix...)
			parts = append(parts, partExpr)
		}
		return prefix, &hir.TemplateLiteral{Parts: parts}
	case *hir.TaggedTemplateLiteral:
		prefix, tag := l.normalizeAsyncExpr(e.Tag, false)
		tplPrefix, tpl := l.normalizeAsyncExpr(e.Template, false)
		prefix = append(prefix, tplPrefix...)
		if template, ok := tpl.(*hir.TemplateLiteral); ok {
			return prefix, &hir.TaggedTemplateLiteral{Tag: tag, Template: template}
		}
		return prefix, e
	case *hir.SequenceExpr:
		var prefix []hir.Stmt
		exprs := make([]hir.Expr, 0, len(e.Exprs))
		for i, ex := range e.Exprs {
			exPrefix, exExpr := l.normalizeAsyncExpr(ex, i == len(e.Exprs)-1 && allowDirectAwait)
			prefix = append(prefix, exPrefix...)
			exprs = append(exprs, exExpr)
		}
		return prefix, &hir.SequenceExpr{Exprs: exprs, Span: e.Span}
	case *hir.ParenExpr:
		prefix, inner := l.normalizeAsyncExpr(e.Expr, allowDirectAwait)
		return prefix, &hir.ParenExpr{Expr: inner}
	case *hir.TypeAssertExpr:
		prefix, inner := l.normalizeAsyncExpr(e.Expr, allowDirectAwait)
		return prefix, &hir.TypeAssertExpr{Expr: inner, Type: e.Type}
	case *hir.NonNullExpr:
		prefix, inner := l.normalizeAsyncExpr(e.Expr, allowDirectAwait)
		return prefix, &hir.NonNullExpr{Expr: inner}
	case *hir.SpreadExpr:
		prefix, inner := l.normalizeAsyncExpr(e.Value, false)
		return prefix, &hir.SpreadExpr{Value: inner}
	default:
		return nil, expr
	}
}

func (l *Lowerer) newAsyncTempSymbol() *symbol.Symbol {
	name := l.nextSyntheticName("_await_tmp")
	sym := l.symtab.Define(name, symbol.KindVariable)
	l.asyncTempSymbols = append(l.asyncTempSymbols, sym)
	return sym
}
