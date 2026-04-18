package hir

// ValidateAsyncPipelinePhase1 validates the currently supported async surface
// for pipeline-backed compilation. Phase 1 supports only async function
// declarations (not main/init) with direct await statements outside protected
// regions.
func ValidateAsyncPipelinePhase1(mod *Module) []Diagnostic {
	if mod == nil {
		return nil
	}
	v := asyncPhase1Validator{sourcePath: mod.SourcePath, hasTopLevelAwait: mod.HasTopLevelAwait}
	for _, d := range mod.Declarations {
		v.walkDecl(d, phase1Context{})
	}
	return v.diags
}

// AsyncPipelinePhase1Error returns a DiagnosticError when async usage exceeds
// the currently supported pipeline phase-1 surface.
func AsyncPipelinePhase1Error(mod *Module) error {
	diags := ValidateAsyncPipelinePhase1(mod)
	if len(diags) == 0 {
		return nil
	}
	return &DiagnosticError{Diagnostics: diags}
}

type phase1Context struct {
	inAsyncDecl bool
	inProtected bool
}

type asyncPhase1Validator struct {
	sourcePath        string
	diags             []Diagnostic
	hasTopLevelAwait  bool
}

func (v *asyncPhase1Validator) add(span *SourceSpan, message string) {
	v.diags = append(v.diags, Diagnostic{
		SourcePath: v.sourcePath,
		Span:       span,
		Message:    message,
	})
}

func (v *asyncPhase1Validator) walkDecl(d Decl, ctx phase1Context) {
	switch d := d.(type) {
	case *FuncDecl:
		if d.IsAsync {
			asyncCtx := phase1Context{inAsyncDecl: true}
			v.walkParams(d.Params, ctx)
			v.walkBlock(d.Body, asyncCtx)
			return
		}
		v.walkParams(d.Params, ctx)
		v.walkBlock(d.Body, ctx)
	case *VarDecl:
		varDeclCtx := ctx
		if v.hasTopLevelAwait {
			varDeclCtx = phase1Context{inAsyncDecl: true}
		}
		for _, decl := range d.Declarators {
			v.walkPattern(decl.Pattern, varDeclCtx)
			v.walkExpr(decl.Init, varDeclCtx)
		}
	case *ClassDecl:
		v.walkExpr(d.Parent, ctx)
		if d.Constructor != nil {
			v.walkParams(d.Constructor.Params, ctx)
			v.walkBlock(d.Constructor.Body, ctx)
		}
		for _, m := range d.Methods {
			if m.IsAsync {
				asyncCtx := phase1Context{inAsyncDecl: true}
				v.walkParams(m.Params, ctx)
				v.walkBlock(m.Body, asyncCtx)
				continue
			}
			v.walkParams(m.Params, ctx)
			v.walkBlock(m.Body, ctx)
		}
		for _, p := range d.Properties {
			v.walkExpr(p.Value, ctx)
			v.walkExpr(p.Computed, ctx)
		}
		for _, expr := range d.StaticInits {
			v.walkExpr(expr, ctx)
		}
	case *ExportDecl:
		if d.Decl != nil {
			v.walkDecl(d.Decl, ctx)
		}
	case *TopLevelStmt:
		tlaCtx := ctx
		if v.hasTopLevelAwait {
			tlaCtx = phase1Context{inAsyncDecl: true}
		}
		v.walkStmt(d.Stmt, tlaCtx)
	}
}

func (v *asyncPhase1Validator) walkBlock(b *BlockStmt, ctx phase1Context) {
	if b == nil {
		return
	}
	for _, st := range b.Stmts {
		v.walkStmt(st, ctx)
	}
}

func (v *asyncPhase1Validator) walkParams(params []*Param, ctx phase1Context) {
	for _, p := range params {
		if p == nil {
			continue
		}
		v.walkPattern(p.Pattern, ctx)
		v.walkExpr(p.Default, ctx)
	}
}

func (v *asyncPhase1Validator) walkPattern(p Pattern, ctx phase1Context) {
	switch p := p.(type) {
	case *ObjectPattern:
		for _, prop := range p.Properties {
			v.walkPattern(prop.Pattern, ctx)
			v.walkExpr(prop.Default, ctx)
		}
	case *ArrayPattern:
		for _, elem := range p.Elements {
			if elem == nil {
				continue
			}
			v.walkPattern(elem.Pattern, ctx)
			v.walkExpr(elem.Default, ctx)
		}
	}
}

func (v *asyncPhase1Validator) walkStmt(s Stmt, ctx phase1Context) {
	switch s := s.(type) {
	case *BlockStmt:
		v.walkBlock(s, ctx)
	case *ExprStmt:
		if ctx.inAsyncDecl {
			if await, ok := directAwait(s.Expr); ok {
				v.walkExpr(await.Value, phase1Context{})
				return
			}
			if assign, ok := s.Expr.(*AssignExpr); ok {
				if await, ok := directAwait(assign.Right); ok {
					v.walkPattern(assign.LeftPattern, phase1Context{})
					v.walkExpr(assign.Left, phase1Context{})
					v.walkExpr(await.Value, phase1Context{})
					return
				}
			}
		}
		v.walkExpr(s.Expr, ctx)
	case *ReturnStmt:
		if ctx.inAsyncDecl {
			if await, ok := directAwait(s.Value); ok {
				v.walkExpr(await.Value, phase1Context{})
				return
			}
		}
		v.walkExpr(s.Value, ctx)
	case *IfStmt:
		v.walkExpr(s.Cond, ctx)
		v.walkBlock(s.Then, ctx)
		v.walkStmt(s.Else, ctx)
	case *ForStmt:
		v.walkStmt(s.Init, ctx)
		v.walkExpr(s.Cond, ctx)
		v.walkExpr(s.Post, ctx)
		v.walkBlock(s.Body, ctx)
	case *ForInStmt:
		v.walkExpr(s.Value, ctx)
		v.walkBlock(s.Body, ctx)
	case *ForOfStmt:
		v.walkExpr(s.Value, ctx)
		v.walkBlock(s.Body, ctx)
	case *WhileStmt:
		v.walkExpr(s.Cond, ctx)
		v.walkBlock(s.Body, ctx)
	case *DoWhileStmt:
		v.walkBlock(s.Body, ctx)
		v.walkExpr(s.Cond, ctx)
	case *SwitchStmt:
		v.walkExpr(s.Tag, ctx)
		for _, c := range s.Cases {
			v.walkExpr(c.Value, ctx)
			for _, st := range c.Body {
				v.walkStmt(st, ctx)
			}
		}
	case *TryCatchStmt:
		if ctx.inAsyncDecl {
			protected := phase1Context{inProtected: true, inAsyncDecl: true}
			v.walkBlock(s.Try, protected)
			if s.Catch != nil {
				v.walkBlock(s.Catch.Body, phase1Context{inAsyncDecl: true})
			}
			if s.Finally != nil {
				v.walkBlock(s.Finally, phase1Context{inAsyncDecl: true})
			}
			return
		}
		protected := phase1Context{inProtected: true, inAsyncDecl: ctx.inAsyncDecl}
		v.walkBlock(s.Try, protected)
		if s.Catch != nil {
			v.walkBlock(s.Catch.Body, protected)
		}
		v.walkBlock(s.Finally, protected)
	case *ThrowStmt:
		v.walkExpr(s.Value, ctx)
	case *LabeledStmt:
		v.walkStmt(s.Stmt, ctx)
	case *VarDecl:
		for _, decl := range s.Declarators {
			if ctx.inAsyncDecl {
				if await, ok := directAwait(decl.Init); ok {
					if decl.Pattern != nil {
						v.walkPattern(decl.Pattern, phase1Context{})
					}
					v.walkExpr(await.Value, phase1Context{})
					continue
				}
			}
			v.walkPattern(decl.Pattern, ctx)
			v.walkExpr(decl.Init, ctx)
		}
	}
}

func (v *asyncPhase1Validator) walkExpr(e Expr, ctx phase1Context) {
	switch e := e.(type) {
	case *AwaitExpr:
		if !ctx.inAsyncDecl {
			v.add(e.Span, "await expressions are only supported inside async function declarations")
		}
		v.walkExpr(e.Value, phase1Context{})
	case *ArrowFunc:
		if e.IsAsync {
			asyncCtx := phase1Context{inAsyncDecl: true}
			v.walkParams(e.Params, ctx)
			v.walkBlock(e.Body, asyncCtx)
			v.walkExpr(e.ExprBody, asyncCtx)
			return
		}
		v.walkParams(e.Params, phase1Context{})
		v.walkBlock(e.Body, phase1Context{})
		v.walkExpr(e.ExprBody, phase1Context{})
	case *FuncExpr:
		if e.IsAsync {
			asyncCtx := phase1Context{inAsyncDecl: true}
			v.walkParams(e.Params, ctx)
			v.walkBlock(e.Body, asyncCtx)
			return
		}
		v.walkParams(e.Params, phase1Context{})
		v.walkBlock(e.Body, phase1Context{})
	case *ArrayLiteral:
		for _, el := range e.Elements {
			v.walkExpr(el, ctx)
		}
	case *ObjectLiteral:
		for _, prop := range e.Properties {
			v.walkExpr(prop.Key, ctx)
			if prop.Method {
				if fn, ok := prop.Value.(*ArrowFunc); ok {
					if fn.IsAsync {
						asyncCtx := phase1Context{inAsyncDecl: true}
						v.walkParams(fn.Params, ctx)
						v.walkBlock(fn.Body, asyncCtx)
						v.walkExpr(fn.ExprBody, asyncCtx)
						continue
					}
					v.walkParams(fn.Params, phase1Context{})
					v.walkBlock(fn.Body, phase1Context{})
					v.walkExpr(fn.ExprBody, phase1Context{})
					continue
				}
			}
			v.walkExpr(prop.Value, ctx)
		}
	case *BinaryExpr:
		v.walkExpr(e.Left, ctx)
		v.walkExpr(e.Right, ctx)
	case *UnaryExpr:
		v.walkExpr(e.Operand, ctx)
	case *UpdateExpr:
		v.walkExpr(e.Operand, ctx)
	case *AssignExpr:
		v.walkPattern(e.LeftPattern, ctx)
		v.walkExpr(e.Left, ctx)
		v.walkExpr(e.Right, ctx)
	case *CallExpr:
		v.walkExpr(e.Func, ctx)
		for _, arg := range e.Args {
			v.walkExpr(arg, ctx)
		}
	case *NewExpr:
		v.walkExpr(e.Callee, ctx)
		for _, arg := range e.Args {
			v.walkExpr(arg, ctx)
		}
	case *ClassExpr:
		v.walkExpr(e.Parent, ctx)
		if e.Constructor != nil {
			v.walkParams(e.Constructor.Params, phase1Context{})
			v.walkBlock(e.Constructor.Body, phase1Context{})
		}
		for _, m := range e.Methods {
			if m.IsAsync {
				asyncCtx := phase1Context{inAsyncDecl: true}
				v.walkParams(m.Params, ctx)
				v.walkBlock(m.Body, asyncCtx)
				continue
			}
			v.walkParams(m.Params, phase1Context{})
			v.walkBlock(m.Body, phase1Context{})
		}
		for _, p := range e.Properties {
			v.walkExpr(p.Value, ctx)
			v.walkExpr(p.Computed, ctx)
		}
		for _, expr := range e.StaticInits {
			v.walkExpr(expr, ctx)
		}
	case *MemberExpr:
		v.walkExpr(e.Object, ctx)
	case *ComputedMemberExpr:
		v.walkExpr(e.Object, ctx)
		v.walkExpr(e.Property, ctx)
	case *TernaryExpr:
		v.walkExpr(e.Cond, ctx)
		v.walkExpr(e.Then, ctx)
		v.walkExpr(e.Else, ctx)
	case *SpreadExpr:
		v.walkExpr(e.Value, ctx)
	case *SequenceExpr:
		for _, ex := range e.Exprs {
			v.walkExpr(ex, ctx)
		}
	case *YieldExpr:
		v.walkExpr(e.Value, ctx)
	case *TypeAssertExpr:
		v.walkExpr(e.Expr, ctx)
	case *NonNullExpr:
		v.walkExpr(e.Expr, ctx)
	case *ParenExpr:
		v.walkExpr(e.Expr, ctx)
	case *TaggedTemplateLiteral:
		v.walkExpr(e.Tag, ctx)
		if e.Template != nil {
			for _, part := range e.Template.Parts {
				v.walkExpr(part, ctx)
			}
		}
	case *TemplateLiteral:
		for _, part := range e.Parts {
			v.walkExpr(part, ctx)
		}
	}
}

func directAwait(e Expr) (*AwaitExpr, bool) {
	switch e := e.(type) {
	case *AwaitExpr:
		return e, true
	case *ParenExpr:
		return directAwait(e.Expr)
	default:
		return nil, false
	}
}

func exprContainsAwait(e Expr) bool {
	switch e := e.(type) {
	case nil:
		return false
	case *AwaitExpr:
		return true
	case *BinaryExpr:
		return exprContainsAwait(e.Left) || exprContainsAwait(e.Right)
	case *UnaryExpr:
		return exprContainsAwait(e.Operand)
	case *UpdateExpr:
		return exprContainsAwait(e.Operand)
	case *AssignExpr:
		return exprContainsAwait(e.Left) || exprContainsAwait(e.Right)
	case *CallExpr:
		if exprContainsAwait(e.Func) {
			return true
		}
		for _, arg := range e.Args {
			if exprContainsAwait(arg) {
				return true
			}
		}
		return false
	case *NewExpr:
		if exprContainsAwait(e.Callee) {
			return true
		}
		for _, arg := range e.Args {
			if exprContainsAwait(arg) {
				return true
			}
		}
		return false
	case *MemberExpr:
		return exprContainsAwait(e.Object)
	case *ComputedMemberExpr:
		return exprContainsAwait(e.Object) || exprContainsAwait(e.Property)
	case *TernaryExpr:
		return exprContainsAwait(e.Cond) || exprContainsAwait(e.Then) || exprContainsAwait(e.Else)
	case *ArrayLiteral:
		for _, el := range e.Elements {
			if exprContainsAwait(el) {
				return true
			}
		}
		return false
	case *ObjectLiteral:
		for _, prop := range e.Properties {
			if exprContainsAwait(prop.Key) || exprContainsAwait(prop.Value) {
				return true
			}
		}
		return false
	case *TemplateLiteral:
		for _, part := range e.Parts {
			if exprContainsAwait(part) {
				return true
			}
		}
		return false
	case *TaggedTemplateLiteral:
		return exprContainsAwait(e.Tag) || exprContainsAwait(e.Template)
	case *SequenceExpr:
		for _, ex := range e.Exprs {
			if exprContainsAwait(ex) {
				return true
			}
		}
		return false
	case *ParenExpr:
		return exprContainsAwait(e.Expr)
	case *TypeAssertExpr:
		return exprContainsAwait(e.Expr)
	case *NonNullExpr:
		return exprContainsAwait(e.Expr)
	case *SpreadExpr:
		return exprContainsAwait(e.Value)
	case *ArrowFunc:
		return exprContainsAwait(e.ExprBody)
	case *ClassExpr:
		return exprContainsAwait(e.Parent)
	default:
		return false
	}
}

func stmtContainsAwait(s Stmt) bool {
	switch s := s.(type) {
	case nil:
		return false
	case *ExprStmt:
		return exprContainsAwait(s.Expr)
	case *ReturnStmt:
		return exprContainsAwait(s.Value)
	case *VarDecl:
		for _, d := range s.Declarators {
			if exprContainsAwait(d.Init) {
				return true
			}
		}
		return false
	case *BlockStmt:
		for _, st := range s.Stmts {
			if stmtContainsAwait(st) {
				return true
			}
		}
		return false
	case *IfStmt:
		return exprContainsAwait(s.Cond) || stmtContainsAwaitBlock(s.Then) || stmtContainsAwait(s.Else)
	case *ForStmt:
		return stmtContainsAwait(s.Init) || exprContainsAwait(s.Cond) || exprContainsAwait(s.Post) || stmtContainsAwaitBlock(s.Body)
	case *ForInStmt:
		return exprContainsAwait(s.Value) || stmtContainsAwaitBlock(s.Body)
	case *ForOfStmt:
		return exprContainsAwait(s.Value) || stmtContainsAwaitBlock(s.Body)
	case *WhileStmt:
		return exprContainsAwait(s.Cond) || stmtContainsAwaitBlock(s.Body)
	case *DoWhileStmt:
		return exprContainsAwait(s.Cond) || stmtContainsAwaitBlock(s.Body)
	case *SwitchStmt:
		if exprContainsAwait(s.Tag) {
			return true
		}
		for _, c := range s.Cases {
			if exprContainsAwait(c.Value) {
				return true
			}
			for _, st := range c.Body {
				if stmtContainsAwait(st) {
					return true
				}
			}
		}
		return false
	case *TryCatchStmt:
		if stmtContainsAwaitBlock(s.Try) {
			return true
		}
		if s.Catch != nil && stmtContainsAwaitBlock(s.Catch.Body) {
			return true
		}
		if stmtContainsAwaitBlock(s.Finally) {
			return true
		}
		return false
	case *ThrowStmt:
		return exprContainsAwait(s.Value)
	case *LabeledStmt:
		return stmtContainsAwait(s.Stmt)
	default:
		return false
	}
}

func stmtContainsAwaitBlock(b *BlockStmt) bool {
	if b == nil {
		return false
	}
	for _, st := range b.Stmts {
		if stmtContainsAwait(st) {
			return true
		}
	}
	return false
}

// scanForTopLevelAwait returns true if any TopLevelStmt or top-level VarDecl
// in the module contains an await expression (not inside a nested function).
func scanForTopLevelAwait(mod *Module) bool {
	for _, d := range mod.Declarations {
		switch d := d.(type) {
		case *TopLevelStmt:
			if stmtContainsAwait(d.Stmt) {
				return true
			}
		case *VarDecl:
			if VarDeclContainsAwait(d) {
				return true
			}
		case *ExportDecl:
			if d.Decl != nil {
				if tls, ok := d.Decl.(*TopLevelStmt); ok && stmtContainsAwait(tls.Stmt) {
					return true
				}
				if vd, ok := d.Decl.(*VarDecl); ok && VarDeclContainsAwait(vd) {
					return true
				}
			}
		}
	}
	return false
}

// VarDeclContainsAwait returns true if any declarator init contains await.
func VarDeclContainsAwait(d *VarDecl) bool {
	for _, decl := range d.Declarators {
		if exprContainsAwait(decl.Init) {
			return true
		}
	}
	return false
}
