package passes

import "github.com/nnstd/gun/compiler/ssa"

// DCE (Dead Code Elimination) removes instructions whose results are unused.
type DCE struct{}

func (DCE) Name() string { return "dce" }

func (DCE) Run(mod *ssa.Module) error {
	for _, fn := range mod.Functions {
		eliminateDeadCode(fn)
	}
	return nil
}

func eliminateDeadCode(fn *ssa.Function) {
	// Build use count: how many times each value ID is referenced
	uses := make(map[int]int)
	for _, b := range fn.Blocks {
		for _, phi := range b.Phis {
			for _, v := range phi.Edges {
				if v != nil {
					uses[v.ID()]++
				}
			}
		}
		for _, instr := range b.Instrs {
			countUses(instr, uses)
		}
		if b.Term != nil {
			countTermUses(b.Term, uses)
		}
	}

	// Mark param values as used
	for _, p := range fn.Params {
		if p.Val != nil {
			uses[p.Val.ID()]++
		}
	}

	// Remove instructions with zero uses (if they have a result and no side effects)
	for _, b := range fn.Blocks {
		var kept []ssa.Instr
		for _, instr := range b.Instrs {
			res := instr.Result()
			if res != nil && uses[res.ValueID] == 0 && !hasSideEffects(instr) {
				continue // dead
			}
			kept = append(kept, instr)
		}
		b.Instrs = kept
	}
}

func countUses(instr ssa.Instr, uses map[int]int) {
	switch i := instr.(type) {
	case *ssa.BinInstr:
		if i.Left != nil {
			uses[i.Left.ID()]++
		}
		if i.Right != nil {
			uses[i.Right.ID()]++
		}
	case *ssa.UnaryInstr:
		if i.Operand != nil {
			uses[i.Operand.ID()]++
		}
	case *ssa.CallInstr:
		if i.Func != nil {
			uses[i.Func.ID()]++
		}
		for _, a := range i.Args {
			if a != nil {
				uses[a.ID()]++
			}
		}
	case *ssa.GetInstr:
		if i.Object != nil {
			uses[i.Object.ID()]++
		}
		if i.Key != nil {
			uses[i.Key.ID()]++
		}
	case *ssa.SetInstr:
		if i.Object != nil {
			uses[i.Object.ID()]++
		}
		if i.Key != nil {
			uses[i.Key.ID()]++
		}
		if i.Val != nil {
			uses[i.Val.ID()]++
		}
	case *ssa.NewInstr:
		if i.Callee != nil {
			uses[i.Callee.ID()]++
		}
		for _, a := range i.Args {
			if a != nil {
				uses[a.ID()]++
			}
		}
	case *ssa.AllocInstr:
		for _, el := range i.Elements {
			if el != nil {
				uses[el.ID()]++
			}
		}
	case *ssa.CopyInstr:
		if i.Src != nil {
			uses[i.Src.ID()]++
		}
	}
}

func countTermUses(t ssa.Terminator, uses map[int]int) {
	switch t := t.(type) {
	case *ssa.BranchTerm:
		if t.Cond != nil {
			uses[t.Cond.ID()]++
		}
	case *ssa.ReturnTerm:
		if t.Value != nil {
			uses[t.Value.ID()]++
		}
	case *ssa.PanicTerm:
		if t.Value != nil {
			uses[t.Value.ID()]++
		}
	case *ssa.SwitchTerm:
		if t.Tag != nil {
			uses[t.Tag.ID()]++
		}
		for _, c := range t.Cases {
			if c.Value != nil {
				uses[c.Value.ID()]++
			}
		}
	}
}

// hasSideEffects returns true if the instruction has side effects beyond computing a value.
func hasSideEffects(instr ssa.Instr) bool {
	switch instr.(type) {
	case *ssa.CallInstr, *ssa.SetInstr, *ssa.NewInstr:
		return true
	}
	return false
}
