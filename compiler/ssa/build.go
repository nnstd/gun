package ssa

import (
	"github.com/nnstd/gun/compiler/mir"
	"github.com/nnstd/gun/compiler/symbol"
)

// Build converts a MIR module into SSA form.
func Build(mirMod *mir.Module) *Module {
	mod := &Module{
		Package: mirMod.Package,
	}

	// Convert globals
	for _, g := range mirMod.Globals {
		mod.Globals = append(mod.Globals, &Global{
			Symbol: g.Symbol,
		})
	}

	// Convert each function
	for _, mirFn := range mirMod.Functions {
		ssaFn := buildFunction(mirFn)
		mod.Functions = append(mod.Functions, ssaFn)
	}

	return mod
}

func buildFunction(mirFn *mir.Function) *Function {
	fn := &Function{
		Symbol:   mirFn.Symbol,
		Exported: mirFn.Exported,
		IsMain:   mirFn.IsMain,
	}

	// Create SSA blocks from MIR blocks
	blockMap := make(map[int]*Block)
	for _, mb := range mirFn.Blocks {
		b := &Block{ID: mb.ID}
		blockMap[mb.ID] = b
		fn.Blocks = append(fn.Blocks, b)
	}

	// Wire CFG edges
	for _, mb := range mirFn.Blocks {
		b := blockMap[mb.ID]
		for _, pred := range mb.Preds {
			if pb, ok := blockMap[pred.ID]; ok {
				b.Preds = append(b.Preds, pb)
			}
		}
		for _, succ := range mb.Succs {
			if sb, ok := blockMap[succ.ID]; ok {
				b.Succs = append(b.Succs, sb)
			}
		}
	}

	// Create params
	for _, mp := range mirFn.Params {
		val := fn.NewValue(mp.Symbol)
		fn.Params = append(fn.Params, &Param{
			Symbol: mp.Symbol,
			Val:    val,
			Rest:   mp.Rest,
		})
	}

	// Compute dominator tree and dominance frontiers
	ComputeDominators(fn)
	ComputeDominanceFrontiers(fn)

	// Convert MIR statements to SSA instructions
	builder := &ssaBuilder{
		fn:       fn,
		blockMap: blockMap,
		defs:     make(map[*symbol.Symbol]map[*Block]Value),
		stacks:   make(map[*symbol.Symbol][]Value),
	}

	// Record param definitions in entry block
	if len(fn.Blocks) > 0 {
		for _, p := range fn.Params {
			builder.writeVar(p.Symbol, fn.Blocks[0], p.Val)
		}
	}

	// Convert instructions block by block
	for i, mb := range mirFn.Blocks {
		b := fn.Blocks[i]
		builder.buildBlock(b, mb)
	}

	// Insert phi nodes
	builder.insertPhis()

	return fn
}

// ssaBuilder holds state during SSA construction.
type ssaBuilder struct {
	fn       *Function
	blockMap map[int]*Block

	// Variable definitions: symbol → block → current SSA value
	defs   map[*symbol.Symbol]map[*Block]Value
	stacks map[*symbol.Symbol][]Value

	// Blocks where each variable is defined (for phi insertion)
	defBlocks map[*symbol.Symbol][]*Block
}

func (sb *ssaBuilder) writeVar(sym *symbol.Symbol, block *Block, val Value) {
	if sym == nil {
		return
	}
	if sb.defs[sym] == nil {
		sb.defs[sym] = make(map[*Block]Value)
	}
	sb.defs[sym][block] = val

	// Track definition sites
	if sb.defBlocks == nil {
		sb.defBlocks = make(map[*symbol.Symbol][]*Block)
	}
	sb.defBlocks[sym] = append(sb.defBlocks[sym], block)
}

func (sb *ssaBuilder) readVar(sym *symbol.Symbol, block *Block) Value {
	if sym == nil {
		return sb.fn.NewConst(ConstNull, "nil")
	}
	if defs, ok := sb.defs[sym]; ok {
		if val, ok := defs[block]; ok {
			return val
		}
	}
	// Variable not defined in this block — search up dominator tree
	if block.IDom != nil && block.IDom != block {
		return sb.readVar(sym, block.IDom)
	}
	// Undefined — return nil constant
	return sb.fn.NewConst(ConstNull, "nil")
}

func (sb *ssaBuilder) buildBlock(b *Block, mb *mir.BasicBlock) {
	for _, s := range mb.Stmts {
		sb.buildStmt(b, s)
	}
	if mb.Term != nil {
		b.Term = sb.buildTerm(b, mb.Term)
	}
}

func (sb *ssaBuilder) buildStmt(b *Block, s mir.Stmt) {
	switch s := s.(type) {
	case *mir.DeclStmt:
		if s.Value != nil {
			val := sb.buildExpr(b, s.Value)
			sb.writeVar(s.Symbol, b, val)
		} else {
			sb.writeVar(s.Symbol, b, sb.fn.NewConst(ConstNull, "nil"))
		}
	case *mir.AssignStmt:
		val := sb.buildExpr(b, s.Value)
		sb.writeVar(s.Target, b, val)
	case *mir.ExprStmt:
		sb.buildExpr(b, s.Expr)
	case *mir.StoreStmt:
		obj := sb.buildExpr(b, s.Object)
		key := sb.buildExpr(b, s.Key)
		val := sb.buildExpr(b, s.Value)
		b.Instrs = append(b.Instrs, &SetInstr{Object: obj, Key: key, Val: val})
	case *mir.DeferStmt:
		sb.buildExpr(b, s.Call)
	}
}

func (sb *ssaBuilder) buildExpr(b *Block, e mir.Expr) Value {
	if e == nil {
		return sb.fn.NewConst(ConstNull, "nil")
	}
	switch e := e.(type) {
	case *mir.IdentExpr:
		if e.Symbol != nil {
			return sb.readVar(e.Symbol, b)
		}
		return sb.fn.NewConst(ConstString, e.Name)
	case *mir.LitExpr:
		return sb.fn.NewConst(mirLitToSSA(e.Kind), e.Value)
	case *mir.BinExpr:
		left := sb.buildExpr(b, e.Left)
		right := sb.buildExpr(b, e.Right)
		res := sb.fn.NewValue(nil)
		instr := &BinInstr{Res: res, Op: BinOp(e.Op), Left: left, Right: right}
		res.Def = instr
		b.Instrs = append(b.Instrs, instr)
		return res
	case *mir.UnaryExpr:
		operand := sb.buildExpr(b, e.Operand)
		res := sb.fn.NewValue(nil)
		instr := &UnaryInstr{Res: res, Op: UnaryOp(e.Op), Operand: operand}
		res.Def = instr
		b.Instrs = append(b.Instrs, instr)
		return res
	case *mir.CallExpr:
		fn := sb.buildExpr(b, e.Func)
		var args []Value
		for _, a := range e.Args {
			args = append(args, sb.buildExpr(b, a))
		}
		res := sb.fn.NewValue(nil)
		instr := &CallInstr{Res: res, Func: fn, Args: args}
		res.Def = instr
		b.Instrs = append(b.Instrs, instr)
		return res
	case *mir.NewCallExpr:
		callee := sb.buildExpr(b, e.Callee)
		var args []Value
		for _, a := range e.Args {
			args = append(args, sb.buildExpr(b, a))
		}
		res := sb.fn.NewValue(nil)
		instr := &NewInstr{Res: res, Callee: callee, Args: args}
		res.Def = instr
		b.Instrs = append(b.Instrs, instr)
		return res
	case *mir.GetExpr:
		obj := sb.buildExpr(b, e.Object)
		key := sb.buildExpr(b, e.Key)
		res := sb.fn.NewValue(nil)
		instr := &GetInstr{Res: res, Object: obj, Key: key}
		res.Def = instr
		b.Instrs = append(b.Instrs, instr)
		return res
	case *mir.IndexExpr:
		obj := sb.buildExpr(b, e.Object)
		idx := sb.buildExpr(b, e.Index)
		res := sb.fn.NewValue(nil)
		instr := &GetInstr{Res: res, Object: obj, Key: idx}
		res.Def = instr
		b.Instrs = append(b.Instrs, instr)
		return res
	case *mir.ArrayExpr:
		var elems []Value
		for _, el := range e.Elements {
			elems = append(elems, sb.buildExpr(b, el))
		}
		res := sb.fn.NewValue(nil)
		instr := &AllocInstr{Res: res, Kind: AllocArray, Elements: elems}
		res.Def = instr
		b.Instrs = append(b.Instrs, instr)
		return res
	case *mir.ObjectExpr:
		var kv []Value
		for i, k := range e.Keys {
			kv = append(kv, sb.buildExpr(b, k))
			if i < len(e.Values) {
				kv = append(kv, sb.buildExpr(b, e.Values[i]))
			}
		}
		res := sb.fn.NewValue(nil)
		instr := &AllocInstr{Res: res, Kind: AllocObject, Elements: kv}
		res.Def = instr
		b.Instrs = append(b.Instrs, instr)
		return res
	case *mir.ThisExpr:
		return sb.fn.NewConst(ConstString, "this")
	case *mir.NilExpr:
		return sb.fn.NewConst(ConstNull, "nil")
	case *mir.FuncExpr:
		return sb.fn.NewConst(ConstString, "(func)")
	case *mir.SpreadExpr:
		return sb.buildExpr(b, e.Value)
	case *mir.TemplateExpr:
		// Template strings as a call to sprintf (simplified)
		var args []Value
		for _, p := range e.Parts {
			args = append(args, sb.buildExpr(b, p))
		}
		res := sb.fn.NewValue(nil)
		instr := &CallInstr{
			Res:  res,
			Func: sb.fn.NewConst(ConstString, "__template"),
			Args: args,
		}
		res.Def = instr
		b.Instrs = append(b.Instrs, instr)
		return res
	default:
		return sb.fn.NewConst(ConstNull, "nil")
	}
}

func (sb *ssaBuilder) buildTerm(b *Block, t mir.Terminator) Terminator {
	switch t := t.(type) {
	case *mir.JumpTerm:
		if target, ok := sb.blockMap[t.Target.ID]; ok {
			return &JumpTerm{Target: target}
		}
		return &ReturnTerm{}
	case *mir.BranchTerm:
		cond := sb.buildExpr(b, t.Cond)
		term := &BranchTerm{Cond: cond}
		if t.True != nil {
			if tb, ok := sb.blockMap[t.True.ID]; ok {
				term.True = tb
			}
		}
		if t.False != nil {
			if fb, ok := sb.blockMap[t.False.ID]; ok {
				term.False = fb
			}
		}
		return term
	case *mir.ReturnTerm:
		if t.Value != nil {
			return &ReturnTerm{Value: sb.buildExpr(b, t.Value)}
		}
		return &ReturnTerm{}
	case *mir.PanicTerm:
		return &PanicTerm{Value: sb.buildExpr(b, t.Value)}
	case *mir.SwitchTerm:
		tag := sb.buildExpr(b, t.Tag)
		term := &SwitchTerm{Tag: tag}
		for _, c := range t.Cases {
			sc := &SwitchCase{Value: sb.buildExpr(b, c.Value)}
			if c.Target != nil {
				if tb, ok := sb.blockMap[c.Target.ID]; ok {
					sc.Target = tb
				}
			}
			term.Cases = append(term.Cases, sc)
		}
		if t.Default != nil {
			if db, ok := sb.blockMap[t.Default.ID]; ok {
				term.Default = db
			}
		}
		return term
	default:
		return &ReturnTerm{}
	}
}

// insertPhis inserts phi nodes at the iterated dominance frontier of each variable.
func (sb *ssaBuilder) insertPhis() {
	if sb.defBlocks == nil {
		return
	}

	for sym, defSites := range sb.defBlocks {
		// Compute iterated dominance frontier
		var worklist []*Block
		placed := make(map[*Block]bool)
		inWorklist := make(map[*Block]bool)

		for _, b := range defSites {
			worklist = append(worklist, b)
			inWorklist[b] = true
		}

		for len(worklist) > 0 {
			b := worklist[0]
			worklist = worklist[1:]

			for _, df := range b.DomFront {
				if placed[df] {
					continue
				}
				placed[df] = true

				// Insert phi
				phiVal := sb.fn.NewValue(sym)
				phi := &Phi{
					Value: phiVal,
					Edges: make(map[*Block]Value),
				}
				// Fill edges from predecessors
				for _, pred := range df.Preds {
					phi.Edges[pred] = sb.readVar(sym, pred)
				}
				df.Phis = append(df.Phis, phi)

				// Record the phi as a new definition
				sb.writeVar(sym, df, phiVal)

				if !inWorklist[df] {
					worklist = append(worklist, df)
					inWorklist[df] = true
				}
			}
		}
	}
}

func mirLitToSSA(k mir.LitKind) ConstKind {
	switch k {
	case mir.LitString:
		return ConstString
	case mir.LitNumber:
		return ConstNumber
	case mir.LitBool:
		return ConstBool
	case mir.LitNull:
		return ConstNull
	case mir.LitUndefined:
		return ConstUndefined
	default:
		return ConstString
	}
}
