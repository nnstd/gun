package ssa

import (
	"github.com/nnstd/gun/compiler/mir"
	"github.com/nnstd/gun/compiler/symbol"
)

// DeSSA converts an SSA module back to MIR form by eliminating phi nodes.
// Each phi is replaced with copy instructions at the end of predecessor blocks.
func DeSSA(ssaMod *Module) *mir.Module {
	mod := &mir.Module{
		Package: ssaMod.Package,
	}

	for _, g := range ssaMod.Globals {
		mod.Globals = append(mod.Globals, &mir.Global{
			Symbol: g.Symbol,
		})
	}

	for _, fn := range ssaMod.Functions {
		mod.Functions = append(mod.Functions, dessaFunction(fn))
	}

	return mod
}

func dessaFunction(fn *Function) *mir.Function {
	mirFn := &mir.Function{
		Symbol:   fn.Symbol,
		Exported: fn.Exported,
		IsMain:   fn.IsMain,
	}

	for _, p := range fn.Params {
		mirFn.Params = append(mirFn.Params, &mir.Param{
			Symbol: p.Symbol,
			Rest:   p.Rest,
		})
	}

	blockMap := make(map[int]*mir.BasicBlock)
	for _, b := range fn.Blocks {
		mb := &mir.BasicBlock{ID: b.ID}
		blockMap[b.ID] = mb
		mirFn.Blocks = append(mirFn.Blocks, mb)
	}

	// Wire edges
	for _, b := range fn.Blocks {
		mb := blockMap[b.ID]
		for _, p := range b.Preds {
			if pb, ok := blockMap[p.ID]; ok {
				mb.Preds = append(mb.Preds, pb)
			}
		}
		for _, s := range b.Succs {
			if sb, ok := blockMap[s.ID]; ok {
				mb.Succs = append(mb.Succs, sb)
			}
		}
	}

	// Convert instructions and eliminate phis
	for _, b := range fn.Blocks {
		mb := blockMap[b.ID]

		// Phi elimination: insert copies at predecessor block ends
		for _, phi := range b.Phis {
			for pred, val := range phi.Edges {
				predMB := blockMap[pred.ID]
				if predMB == nil {
					continue
				}
				sym := phi.Value.Symbol
				if sym == nil {
					sym = &symbol.Symbol{OriginalName: "_phi"}
				}
				predMB.Stmts = append(predMB.Stmts, &mir.AssignStmt{
					Target: sym,
					Value:  dessaValue(val),
				})
			}
		}

		// Convert SSA instructions to MIR statements
		for _, instr := range b.Instrs {
			if s := dessaInstr(instr); s != nil {
				mb.Stmts = append(mb.Stmts, s)
			}
		}

		// Convert terminator
		if b.Term != nil {
			mb.Term = dessaTerm(b.Term, blockMap)
		}
	}

	return mirFn
}

func dessaInstr(instr Instr) mir.Stmt {
	switch i := instr.(type) {
	case *BinInstr:
		if i.Res != nil && i.Res.Symbol != nil {
			return &mir.AssignStmt{
				Target: i.Res.Symbol,
				Value: &mir.BinExpr{
					Op:    mir.BinOp(i.Op),
					Left:  dessaValue(i.Left),
					Right: dessaValue(i.Right),
				},
			}
		}
		return &mir.ExprStmt{Expr: &mir.BinExpr{
			Op:    mir.BinOp(i.Op),
			Left:  dessaValue(i.Left),
			Right: dessaValue(i.Right),
		}}
	case *UnaryInstr:
		if i.Res != nil && i.Res.Symbol != nil {
			return &mir.AssignStmt{
				Target: i.Res.Symbol,
				Value: &mir.UnaryExpr{
					Op:      mir.UnaryOp(i.Op),
					Operand: dessaValue(i.Operand),
				},
			}
		}
		return &mir.ExprStmt{Expr: &mir.UnaryExpr{
			Op:      mir.UnaryOp(i.Op),
			Operand: dessaValue(i.Operand),
		}}
	case *CallInstr:
		var args []mir.Expr
		for _, a := range i.Args {
			args = append(args, dessaValue(a))
		}
		call := &mir.CallExpr{Func: dessaValue(i.Func), Args: args}
		if i.Res != nil && i.Res.Symbol != nil {
			return &mir.AssignStmt{Target: i.Res.Symbol, Value: call}
		}
		return &mir.ExprStmt{Expr: call}
	case *GetInstr:
		get := &mir.GetExpr{Object: dessaValue(i.Object), Key: dessaValue(i.Key)}
		if i.Res != nil && i.Res.Symbol != nil {
			return &mir.AssignStmt{Target: i.Res.Symbol, Value: get}
		}
		return &mir.ExprStmt{Expr: get}
	case *SetInstr:
		return &mir.StoreStmt{
			Object: dessaValue(i.Object),
			Key:    dessaValue(i.Key),
			Value:  dessaValue(i.Val),
		}
	case *NewInstr:
		var args []mir.Expr
		for _, a := range i.Args {
			args = append(args, dessaValue(a))
		}
		call := &mir.NewCallExpr{Callee: dessaValue(i.Callee), Args: args}
		if i.Res != nil && i.Res.Symbol != nil {
			return &mir.AssignStmt{Target: i.Res.Symbol, Value: call}
		}
		return &mir.ExprStmt{Expr: call}
	case *AllocInstr:
		if i.Kind == AllocArray {
			var elems []mir.Expr
			for _, el := range i.Elements {
				elems = append(elems, dessaValue(el))
			}
			alloc := &mir.ArrayExpr{Elements: elems}
			if i.Res != nil && i.Res.Symbol != nil {
				return &mir.AssignStmt{Target: i.Res.Symbol, Value: alloc}
			}
			return &mir.ExprStmt{Expr: alloc}
		}
		// Object
		var keys, vals []mir.Expr
		for j := 0; j+1 < len(i.Elements); j += 2 {
			keys = append(keys, dessaValue(i.Elements[j]))
			vals = append(vals, dessaValue(i.Elements[j+1]))
		}
		alloc := &mir.ObjectExpr{Keys: keys, Values: vals}
		if i.Res != nil && i.Res.Symbol != nil {
			return &mir.AssignStmt{Target: i.Res.Symbol, Value: alloc}
		}
		return &mir.ExprStmt{Expr: alloc}
	case *CopyInstr:
		if i.Res != nil && i.Res.Symbol != nil {
			return &mir.AssignStmt{Target: i.Res.Symbol, Value: dessaValue(i.Src)}
		}
		return nil
	default:
		return nil
	}
}

func dessaValue(v Value) mir.Expr {
	if v == nil {
		return &mir.NilExpr{}
	}
	switch v := v.(type) {
	case *SSAValue:
		if v.Symbol != nil {
			return &mir.IdentExpr{Symbol: v.Symbol}
		}
		return &mir.NilExpr{}
	case *Const:
		return &mir.LitExpr{Kind: mir.LitKind(v.Kind), Value: v.Val}
	default:
		return &mir.NilExpr{}
	}
}

func dessaTerm(t Terminator, blockMap map[int]*mir.BasicBlock) mir.Terminator {
	switch t := t.(type) {
	case *JumpTerm:
		if t.Target != nil {
			if mb, ok := blockMap[t.Target.ID]; ok {
				return &mir.JumpTerm{Target: mb}
			}
		}
		return &mir.ReturnTerm{}
	case *BranchTerm:
		bt := &mir.BranchTerm{Cond: dessaValue(t.Cond)}
		if t.True != nil {
			if mb, ok := blockMap[t.True.ID]; ok {
				bt.True = mb
			}
		}
		if t.False != nil {
			if mb, ok := blockMap[t.False.ID]; ok {
				bt.False = mb
			}
		}
		return bt
	case *ReturnTerm:
		if t.Value != nil {
			return &mir.ReturnTerm{Value: dessaValue(t.Value)}
		}
		return &mir.ReturnTerm{}
	case *PanicTerm:
		return &mir.PanicTerm{Value: dessaValue(t.Value)}
	case *SwitchTerm:
		st := &mir.SwitchTerm{Tag: dessaValue(t.Tag)}
		for _, c := range t.Cases {
			sc := &mir.SwitchCase{Value: dessaValue(c.Value)}
			if c.Target != nil {
				if mb, ok := blockMap[c.Target.ID]; ok {
					sc.Target = mb
				}
			}
			st.Cases = append(st.Cases, sc)
		}
		if t.Default != nil {
			if mb, ok := blockMap[t.Default.ID]; ok {
				st.Default = mb
			}
		}
		return st
	default:
		return &mir.ReturnTerm{}
	}
}
