package mir

import (
	"github.com/nnstd/gun/compiler/hir"
	"github.com/nnstd/gun/compiler/symbol"
)

// Lower converts an HIR module into a MIR module, building CFGs for each function.
func Lower(hirMod *hir.Module) *Module {
	l := &lowerer{
		hirMod: hirMod,
		symtab: hirMod.SymbolTable,
	}
	return l.lower()
}

type lowerer struct {
	hirMod  *hir.Module
	symtab  *symbol.Table
	module  *Module
	blockID int
}

func (l *lowerer) lower() *Module {
	l.module = &Module{
		Package: l.hirMod.Package,
	}

	for _, imp := range l.hirMod.Imports {
		l.module.Imports = append(l.module.Imports, &Import{
			Path: imp.ModulePath,
		})
	}

	for _, d := range l.hirMod.Declarations {
		l.lowerDecl(d)
	}

	return l.module
}

func (l *lowerer) newBlock() *BasicBlock {
	b := &BasicBlock{ID: l.blockID}
	l.blockID++
	return b
}

func (l *lowerer) addEdge(from, to *BasicBlock) {
	from.Succs = append(from.Succs, to)
	to.Preds = append(to.Preds, from)
}

// --------------------------------------------------------------------
// Declarations
// --------------------------------------------------------------------

func (l *lowerer) lowerDecl(d hir.Decl) {
	switch d := d.(type) {
	case *hir.FuncDecl:
		l.lowerFuncDecl(d)
	case *hir.VarDecl:
		l.lowerVarDecl(d)
	case *hir.ClassDecl:
		l.lowerClassDecl(d)
	case *hir.ExportDecl:
		l.lowerExportDecl(d)
	case *hir.EnumDecl:
		l.lowerEnumDecl(d)
	case *hir.InterfaceDecl, *hir.TypeAliasDecl, *hir.ImportDecl:
		// Type-level declarations don't produce MIR functions or globals
	}
}

func (l *lowerer) lowerFuncDecl(d *hir.FuncDecl) {
	fn := &Function{
		Symbol:   d.Symbol,
		Exported: d.Exported,
		IsMain:   d.Symbol != nil && (d.Symbol.OriginalName == "main" || d.Symbol.OriginalName == "init"),
	}

	// Parameters
	for _, p := range d.Params {
		param := &Param{Symbol: p.Symbol, Rest: p.Rest}
		if p.Default != nil {
			param.Default = l.lowerExpr(p.Default)
		}
		fn.Params = append(fn.Params, param)
	}

	// Build CFG from body
	fb := &funcBuilder{lowerer: l, fn: fn}
	fb.buildBody(d.Body)

	l.module.Functions = append(l.module.Functions, fn)
}

func (l *lowerer) lowerVarDecl(d *hir.VarDecl) {
	for _, decl := range d.Declarators {
		if decl.Symbol == nil {
			continue
		}
		var init Expr
		if decl.Init != nil {
			init = l.lowerExpr(decl.Init)
		}
		l.module.Globals = append(l.module.Globals, &Global{
			Symbol: decl.Symbol,
			Init:   init,
		})
	}
}

func (l *lowerer) lowerClassDecl(d *hir.ClassDecl) {
	// Classes are represented as globals in MIR
	// The class construction logic is lowered to an initializer expression
	l.module.Globals = append(l.module.Globals, &Global{
		Symbol: d.Symbol,
		Init:   &LitExpr{Kind: LitNull, Value: "null"}, // placeholder
	})

	// Constructor becomes a function
	if d.Constructor != nil && d.Symbol != nil {
		ctorSym := &symbol.Symbol{
			ID:           d.Symbol.ID + 10000, // offset to avoid collision
			OriginalName: d.Symbol.OriginalName + "_ctor",
			Kind:         symbol.KindFunction,
		}
		ctorDecl := &hir.FuncDecl{
			Symbol: ctorSym,
			Params: d.Constructor.Params,
			Body:   d.Constructor.Body,
		}
		l.lowerFuncDecl(ctorDecl)
	}

	// Methods become functions
	for _, m := range d.Methods {
		mSym := &symbol.Symbol{
			ID:           d.Symbol.ID + 20000,
			OriginalName: d.Symbol.OriginalName + "_" + m.Name,
			Kind:         symbol.KindFunction,
		}
		mDecl := &hir.FuncDecl{
			Symbol: mSym,
			Params: m.Params,
			Body:   m.Body,
		}
		l.lowerFuncDecl(mDecl)
	}
}

func (l *lowerer) lowerEnumDecl(d *hir.EnumDecl) {
	// Enums become globals — one per member
	for _, m := range d.Members {
		sym := &symbol.Symbol{
			ID:           d.Symbol.ID + 30000,
			OriginalName: d.Symbol.OriginalName + "_" + m.Name,
			Kind:         symbol.KindVariable,
		}
		var init Expr
		if m.Value != nil {
			init = l.lowerExpr(m.Value)
		}
		l.module.Globals = append(l.module.Globals, &Global{
			Symbol: sym,
			Init:   init,
		})
	}
}

func (l *lowerer) lowerExportDecl(d *hir.ExportDecl) {
	if d.Decl != nil {
		l.lowerDecl(d.Decl)
	}
}

// --------------------------------------------------------------------
// Function body → CFG builder
// --------------------------------------------------------------------

type funcBuilder struct {
	*lowerer
	fn       *Function
	current  *BasicBlock
	breakTo  *BasicBlock // target for break statements
	contTo   *BasicBlock // target for continue statements
}

func (fb *funcBuilder) buildBody(body *hir.BlockStmt) {
	entry := fb.newBlock()
	fb.fn.Blocks = append(fb.fn.Blocks, entry)
	fb.current = entry

	if body != nil {
		fb.lowerStmts(body.Stmts)
	}

	// Ensure the last block has a terminator
	if fb.current != nil && fb.current.Term == nil {
		fb.current.Term = &ReturnTerm{}
	}
}

func (fb *funcBuilder) emit(s Stmt) {
	if fb.current == nil {
		return
	}
	fb.current.Stmts = append(fb.current.Stmts, s)
}

func (fb *funcBuilder) startBlock(b *BasicBlock) {
	fb.fn.Blocks = append(fb.fn.Blocks, b)
	fb.current = b
}

func (fb *funcBuilder) lowerStmts(stmts []hir.Stmt) {
	for _, s := range stmts {
		fb.lowerStmt(s)
		// If current block is terminated, subsequent stmts are dead code
		if fb.current != nil && fb.current.Term != nil {
			return
		}
	}
}

func (fb *funcBuilder) lowerStmt(s hir.Stmt) {
	if fb.current == nil {
		return
	}
	switch s := s.(type) {
	case *hir.ExprStmt:
		fb.emit(&ExprStmt{Expr: fb.lowerExpr(s.Expr)})

	case *hir.ReturnStmt:
		var val Expr
		if s.Value != nil {
			val = fb.lowerExpr(s.Value)
		}
		fb.current.Term = &ReturnTerm{Value: val}

	case *hir.VarDecl:
		for _, decl := range s.Declarators {
			if decl.Symbol == nil {
				continue
			}
			var init Expr
			if decl.Init != nil {
				init = fb.lowerExpr(decl.Init)
			}
			fb.emit(&DeclStmt{Symbol: decl.Symbol, Value: init})
		}

	case *hir.IfStmt:
		fb.lowerIf(s)

	case *hir.ForStmt:
		fb.lowerFor(s)

	case *hir.ForInStmt:
		fb.lowerForIn(s)

	case *hir.ForOfStmt:
		fb.lowerForOf(s)

	case *hir.WhileStmt:
		fb.lowerWhile(s)

	case *hir.DoWhileStmt:
		fb.lowerDoWhile(s)

	case *hir.SwitchStmt:
		fb.lowerSwitch(s)

	case *hir.TryCatchStmt:
		fb.lowerTryCatch(s)

	case *hir.ThrowStmt:
		val := fb.lowerExpr(s.Value)
		fb.current.Term = &PanicTerm{Value: val}

	case *hir.BreakStmt:
		if fb.breakTo != nil {
			fb.current.Term = &JumpTerm{Target: fb.breakTo}
			fb.addEdge(fb.current, fb.breakTo)
		}

	case *hir.ContinueStmt:
		if fb.contTo != nil {
			fb.current.Term = &JumpTerm{Target: fb.contTo}
			fb.addEdge(fb.current, fb.contTo)
		}

	case *hir.BlockStmt:
		fb.lowerStmts(s.Stmts)

	case *hir.LabeledStmt:
		fb.lowerStmt(s.Stmt)

	case *hir.EmptyStmt:
		// nothing
	}
}

// --------------------------------------------------------------------
// Control flow lowering
// --------------------------------------------------------------------

func (fb *funcBuilder) lowerIf(s *hir.IfStmt) {
	cond := fb.lowerExpr(s.Cond)
	thenBlock := fb.newBlock()
	elseBlock := fb.newBlock()
	joinBlock := fb.newBlock()

	fb.current.Term = &BranchTerm{Cond: cond, True: thenBlock, False: elseBlock}
	fb.addEdge(fb.current, thenBlock)
	fb.addEdge(fb.current, elseBlock)

	// Then
	fb.startBlock(thenBlock)
	if s.Then != nil {
		fb.lowerStmts(s.Then.Stmts)
	}
	if fb.current.Term == nil {
		fb.current.Term = &JumpTerm{Target: joinBlock}
		fb.addEdge(fb.current, joinBlock)
	}

	// Else
	fb.startBlock(elseBlock)
	if s.Else != nil {
		fb.lowerStmt(s.Else)
	}
	if fb.current.Term == nil {
		fb.current.Term = &JumpTerm{Target: joinBlock}
		fb.addEdge(fb.current, joinBlock)
	}

	// Join
	fb.startBlock(joinBlock)
}

func (fb *funcBuilder) lowerFor(s *hir.ForStmt) {
	// Init
	if s.Init != nil {
		fb.lowerStmt(s.Init)
	}

	condBlock := fb.newBlock()
	bodyBlock := fb.newBlock()
	postBlock := fb.newBlock()
	exitBlock := fb.newBlock()

	// Jump to cond
	fb.current.Term = &JumpTerm{Target: condBlock}
	fb.addEdge(fb.current, condBlock)

	// Condition
	fb.startBlock(condBlock)
	if s.Cond != nil {
		cond := fb.lowerExpr(s.Cond)
		condBlock.Term = &BranchTerm{Cond: cond, True: bodyBlock, False: exitBlock}
		fb.addEdge(condBlock, bodyBlock)
		fb.addEdge(condBlock, exitBlock)
	} else {
		condBlock.Term = &JumpTerm{Target: bodyBlock}
		fb.addEdge(condBlock, bodyBlock)
	}

	// Body
	fb.startBlock(bodyBlock)
	savedBreak := fb.breakTo
	savedCont := fb.contTo
	fb.breakTo = exitBlock
	fb.contTo = postBlock
	if s.Body != nil {
		fb.lowerStmts(s.Body.Stmts)
	}
	if fb.current.Term == nil {
		fb.current.Term = &JumpTerm{Target: postBlock}
		fb.addEdge(fb.current, postBlock)
	}
	fb.breakTo = savedBreak
	fb.contTo = savedCont

	// Post
	fb.startBlock(postBlock)
	if s.Post != nil {
		fb.emit(&ExprStmt{Expr: fb.lowerExpr(s.Post)})
	}
	postBlock.Term = &JumpTerm{Target: condBlock}
	fb.addEdge(postBlock, condBlock)

	// Exit
	fb.startBlock(exitBlock)
}

func (fb *funcBuilder) lowerForIn(s *hir.ForInStmt) {
	// for key in obj → desugar to iteration over OwnKeys
	iterBlock := fb.newBlock()
	bodyBlock := fb.newBlock()
	exitBlock := fb.newBlock()

	val := fb.lowerExpr(s.Value)
	// Declare iteration var
	fb.emit(&ExprStmt{Expr: &CallExpr{
		Func: &GetExpr{Object: val, Key: &LitExpr{Kind: LitString, Value: "OwnKeys"}},
	}})

	fb.current.Term = &JumpTerm{Target: iterBlock}
	fb.addEdge(fb.current, iterBlock)

	fb.startBlock(iterBlock)
	iterBlock.Term = &BranchTerm{
		Cond:  &LitExpr{Kind: LitBool, Value: "true"}, // simplified
		True:  bodyBlock,
		False: exitBlock,
	}
	fb.addEdge(iterBlock, bodyBlock)
	fb.addEdge(iterBlock, exitBlock)

	fb.startBlock(bodyBlock)
	savedBreak := fb.breakTo
	savedCont := fb.contTo
	fb.breakTo = exitBlock
	fb.contTo = iterBlock
	if s.Body != nil {
		fb.lowerStmts(s.Body.Stmts)
	}
	if fb.current.Term == nil {
		fb.current.Term = &JumpTerm{Target: iterBlock}
		fb.addEdge(fb.current, iterBlock)
	}
	fb.breakTo = savedBreak
	fb.contTo = savedCont

	fb.startBlock(exitBlock)
}

func (fb *funcBuilder) lowerForOf(s *hir.ForOfStmt) {
	// Same structure as forIn but iterates over .Array()
	iterBlock := fb.newBlock()
	bodyBlock := fb.newBlock()
	exitBlock := fb.newBlock()

	val := fb.lowerExpr(s.Value)
	fb.emit(&ExprStmt{Expr: &CallExpr{
		Func: &GetExpr{Object: val, Key: &LitExpr{Kind: LitString, Value: "Array"}},
	}})

	fb.current.Term = &JumpTerm{Target: iterBlock}
	fb.addEdge(fb.current, iterBlock)

	fb.startBlock(iterBlock)
	iterBlock.Term = &BranchTerm{
		Cond:  &LitExpr{Kind: LitBool, Value: "true"},
		True:  bodyBlock,
		False: exitBlock,
	}
	fb.addEdge(iterBlock, bodyBlock)
	fb.addEdge(iterBlock, exitBlock)

	fb.startBlock(bodyBlock)
	savedBreak := fb.breakTo
	savedCont := fb.contTo
	fb.breakTo = exitBlock
	fb.contTo = iterBlock
	if s.Body != nil {
		fb.lowerStmts(s.Body.Stmts)
	}
	if fb.current.Term == nil {
		fb.current.Term = &JumpTerm{Target: iterBlock}
		fb.addEdge(fb.current, iterBlock)
	}
	fb.breakTo = savedBreak
	fb.contTo = savedCont

	fb.startBlock(exitBlock)
}

func (fb *funcBuilder) lowerWhile(s *hir.WhileStmt) {
	condBlock := fb.newBlock()
	bodyBlock := fb.newBlock()
	exitBlock := fb.newBlock()

	fb.current.Term = &JumpTerm{Target: condBlock}
	fb.addEdge(fb.current, condBlock)

	fb.startBlock(condBlock)
	cond := fb.lowerExpr(s.Cond)
	condBlock.Term = &BranchTerm{Cond: cond, True: bodyBlock, False: exitBlock}
	fb.addEdge(condBlock, bodyBlock)
	fb.addEdge(condBlock, exitBlock)

	fb.startBlock(bodyBlock)
	savedBreak := fb.breakTo
	savedCont := fb.contTo
	fb.breakTo = exitBlock
	fb.contTo = condBlock
	if s.Body != nil {
		fb.lowerStmts(s.Body.Stmts)
	}
	if fb.current.Term == nil {
		fb.current.Term = &JumpTerm{Target: condBlock}
		fb.addEdge(fb.current, condBlock)
	}
	fb.breakTo = savedBreak
	fb.contTo = savedCont

	fb.startBlock(exitBlock)
}

func (fb *funcBuilder) lowerDoWhile(s *hir.DoWhileStmt) {
	bodyBlock := fb.newBlock()
	condBlock := fb.newBlock()
	exitBlock := fb.newBlock()

	fb.current.Term = &JumpTerm{Target: bodyBlock}
	fb.addEdge(fb.current, bodyBlock)

	fb.startBlock(bodyBlock)
	savedBreak := fb.breakTo
	savedCont := fb.contTo
	fb.breakTo = exitBlock
	fb.contTo = condBlock
	if s.Body != nil {
		fb.lowerStmts(s.Body.Stmts)
	}
	if fb.current.Term == nil {
		fb.current.Term = &JumpTerm{Target: condBlock}
		fb.addEdge(fb.current, condBlock)
	}
	fb.breakTo = savedBreak
	fb.contTo = savedCont

	fb.startBlock(condBlock)
	cond := fb.lowerExpr(s.Cond)
	condBlock.Term = &BranchTerm{Cond: cond, True: bodyBlock, False: exitBlock}
	fb.addEdge(condBlock, bodyBlock)
	fb.addEdge(condBlock, exitBlock)

	fb.startBlock(exitBlock)
}

func (fb *funcBuilder) lowerSwitch(s *hir.SwitchStmt) {
	tag := fb.lowerExpr(s.Tag)
	exitBlock := fb.newBlock()

	var cases []*SwitchCase
	var defaultBlock *BasicBlock

	for _, c := range s.Cases {
		caseBlock := fb.newBlock()
		if c.Value == nil {
			defaultBlock = caseBlock
		} else {
			cases = append(cases, &SwitchCase{
				Value:  fb.lowerExpr(c.Value),
				Target: caseBlock,
			})
		}
		fb.addEdge(fb.current, caseBlock)
	}
	if defaultBlock == nil {
		defaultBlock = exitBlock
	}

	fb.current.Term = &SwitchTerm{Tag: tag, Cases: cases, Default: defaultBlock}

	// Lower each case body
	savedBreak := fb.breakTo
	fb.breakTo = exitBlock
	for i, c := range s.Cases {
		var caseBlock *BasicBlock
		if c.Value == nil {
			caseBlock = defaultBlock
		} else {
			caseBlock = cases[i].Target
			if c.Value == nil {
				continue
			}
		}
		fb.startBlock(caseBlock)
		for _, st := range c.Body {
			fb.lowerStmt(st)
		}
		if fb.current.Term == nil {
			fb.current.Term = &JumpTerm{Target: exitBlock}
			fb.addEdge(fb.current, exitBlock)
		}
	}
	fb.breakTo = savedBreak

	fb.startBlock(exitBlock)
}

func (fb *funcBuilder) lowerTryCatch(s *hir.TryCatchStmt) {
	// try/catch is desugared:
	// - finally → DeferStmt
	// - catch → DeferStmt with recover
	// - try body → inline statements

	if s.Finally != nil {
		// defer func() { finally-body }()
		finallyFn := &Function{Blocks: []*BasicBlock{}}
		innerFB := &funcBuilder{lowerer: fb.lowerer, fn: finallyFn}
		innerFB.buildBody(s.Finally)
		fb.emit(&DeferStmt{Call: &FuncExpr{Func: finallyFn}})
	}

	if s.Catch != nil {
		// defer func() { if r := recover(); r != nil { catch-body } }()
		catchFn := &Function{Blocks: []*BasicBlock{}}
		innerFB := &funcBuilder{lowerer: fb.lowerer, fn: catchFn}
		innerFB.buildBody(s.Catch.Body)
		fb.emit(&DeferStmt{Call: &FuncExpr{Func: catchFn}})
	}

	// Try body — inline
	if s.Try != nil {
		fb.lowerStmts(s.Try.Stmts)
	}
}

// --------------------------------------------------------------------
// Expression lowering
// --------------------------------------------------------------------

func (l *lowerer) lowerExpr(e hir.Expr) Expr {
	if e == nil {
		return &NilExpr{}
	}
	switch e := e.(type) {
	case *hir.Identifier:
		return &IdentExpr{Symbol: e.Sym, Name: e.Name}
	case *hir.Literal:
		return l.lowerLiteral(e)
	case *hir.TemplateLiteral:
		var parts []Expr
		for _, p := range e.Parts {
			parts = append(parts, l.lowerExpr(p))
		}
		return &TemplateExpr{Parts: parts}
	case *hir.ArrayLiteral:
		var elems []Expr
		for _, el := range e.Elements {
			elems = append(elems, l.lowerExpr(el))
		}
		return &ArrayExpr{Elements: elems}
	case *hir.ObjectLiteral:
		var keys, vals []Expr
		for _, p := range e.Properties {
			keys = append(keys, &LitExpr{Kind: LitString, Value: p.KeyName})
			vals = append(vals, l.lowerExpr(p.Value))
		}
		return &ObjectExpr{Keys: keys, Values: vals}
	case *hir.BinaryExpr:
		return &BinExpr{
			Op:    BinOp(e.Op),
			Left:  l.lowerExpr(e.Left),
			Right: l.lowerExpr(e.Right),
		}
	case *hir.UnaryExpr:
		return &UnaryExpr{
			Op:      UnaryOp(e.Op),
			Operand: l.lowerExpr(e.Operand),
		}
	case *hir.UpdateExpr:
		// x++ → x + 1, x-- → x - 1
		op := OpAdd
		if e.Op == hir.OpDec {
			op = OpSub
		}
		return &BinExpr{
			Op:    op,
			Left:  l.lowerExpr(e.Operand),
			Right: &LitExpr{Kind: LitNumber, Value: "1"},
		}
	case *hir.AssignExpr:
		// Assignment expressions are lowered to the right-hand side value.
		// The actual assignment is handled by the statement context.
		return l.lowerExpr(e.Right)
	case *hir.CallExpr:
		var args []Expr
		for _, a := range e.Args {
			args = append(args, l.lowerExpr(a))
		}
		return &CallExpr{Func: l.lowerExpr(e.Func), Args: args}
	case *hir.NewExpr:
		var args []Expr
		for _, a := range e.Args {
			args = append(args, l.lowerExpr(a))
		}
		return &NewCallExpr{Callee: l.lowerExpr(e.Callee), Args: args}
	case *hir.MemberExpr:
		return &GetExpr{
			Object: l.lowerExpr(e.Object),
			Key:    &LitExpr{Kind: LitString, Value: e.Property},
		}
	case *hir.ComputedMemberExpr:
		return &IndexExpr{
			Object: l.lowerExpr(e.Object),
			Index:  l.lowerExpr(e.Property),
		}
	case *hir.TernaryExpr:
		// Ternary desugared: represented as a conditional expression
		// (the backend will lower this to an IIFE or if/else)
		return &CallExpr{
			Func: &IdentExpr{Name: "__ternary"},
			Args: []Expr{
				l.lowerExpr(e.Cond),
				l.lowerExpr(e.Then),
				l.lowerExpr(e.Else),
			},
		}
	case *hir.ArrowFunc, *hir.FuncExpr:
		// Inline functions stay as function expressions
		fn := &Function{Blocks: []*BasicBlock{}}
		innerFB := &funcBuilder{lowerer: l, fn: fn}

		switch f := e.(type) {
		case *hir.ArrowFunc:
			for _, p := range f.Params {
				fn.Params = append(fn.Params, &Param{Symbol: p.Symbol, Rest: p.Rest})
			}
			if f.Body != nil {
				innerFB.buildBody(f.Body)
			} else if f.ExprBody != nil {
				entry := l.newBlock()
				fn.Blocks = append(fn.Blocks, entry)
				innerFB.current = entry
				entry.Term = &ReturnTerm{Value: l.lowerExpr(f.ExprBody)}
			}
		case *hir.FuncExpr:
			for _, p := range f.Params {
				fn.Params = append(fn.Params, &Param{Symbol: p.Symbol, Rest: p.Rest})
			}
			innerFB.buildBody(f.Body)
		}
		return &FuncExpr{Func: fn}
	case *hir.SpreadExpr:
		return &SpreadExpr{Value: l.lowerExpr(e.Value)}
	case *hir.SequenceExpr:
		// Only the last expression matters for value; earlier ones are side effects
		if len(e.Exprs) == 0 {
			return &NilExpr{}
		}
		return l.lowerExpr(e.Exprs[len(e.Exprs)-1])
	case *hir.AwaitExpr:
		return l.lowerExpr(e.Value)
	case *hir.YieldExpr:
		return l.lowerExpr(e.Value)
	case *hir.TypeAssertExpr:
		return l.lowerExpr(e.Expr)
	case *hir.NonNullExpr:
		return l.lowerExpr(e.Expr)
	case *hir.ParenExpr:
		return l.lowerExpr(e.Expr)
	case *hir.ThisExpr:
		return &ThisExpr{}
	case *hir.SuperExpr:
		return &IdentExpr{Name: "super"}
	case *hir.MetaPropertyExpr:
		return &IdentExpr{Name: e.Meta + "." + e.Property}
	default:
		return &NilExpr{}
	}
}

func (l *lowerer) lowerLiteral(e *hir.Literal) Expr {
	switch e.Kind {
	case hir.LitString:
		return &LitExpr{Kind: LitString, Value: e.Value}
	case hir.LitNumber:
		return &LitExpr{Kind: LitNumber, Value: e.Value}
	case hir.LitBool:
		return &LitExpr{Kind: LitBool, Value: e.Value}
	case hir.LitNull:
		return &LitExpr{Kind: LitNull, Value: "null"}
	case hir.LitUndefined:
		return &LitExpr{Kind: LitUndefined, Value: "undefined"}
	case hir.LitRegex:
		return &LitExpr{Kind: LitRegex, Value: e.Value}
	default:
		return &LitExpr{Kind: LitString, Value: e.Value}
	}
}
