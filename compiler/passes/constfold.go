package passes

import (
	"fmt"
	"strconv"

	"github.com/nnstd/gun/compiler/ssa"
)

// ConstFold performs constant folding: evaluates binary operations on
// constant operands at compile time, replacing instructions with constants.
type ConstFold struct{}

func (ConstFold) Name() string { return "const-fold" }

func (ConstFold) Run(mod *ssa.Module) error {
	for _, fn := range mod.Functions {
		for _, b := range fn.Blocks {
			b.Instrs = foldBlock(fn, b.Instrs)
		}
	}
	return nil
}

func foldBlock(fn *ssa.Function, instrs []ssa.Instr) []ssa.Instr {
	var result []ssa.Instr
	for _, instr := range instrs {
		if folded := tryFold(fn, instr); folded != nil {
			result = append(result, folded)
		} else {
			result = append(result, instr)
		}
	}
	return result
}

// tryFold attempts to fold a binary instruction with constant operands.
// Returns a CopyInstr if folded, nil otherwise.
func tryFold(fn *ssa.Function, instr ssa.Instr) ssa.Instr {
	bin, ok := instr.(*ssa.BinInstr)
	if !ok {
		return nil
	}

	lc, lok := bin.Left.(*ssa.Const)
	rc, rok := bin.Right.(*ssa.Const)
	if !lok || !rok {
		return nil
	}

	// Fold number + number
	if lc.Kind == ssa.ConstNumber && rc.Kind == ssa.ConstNumber {
		lv, le := strconv.ParseFloat(lc.Val, 64)
		rv, re := strconv.ParseFloat(rc.Val, 64)
		if le != nil || re != nil {
			return nil
		}

		var result float64
		switch bin.Op {
		case ssa.OpAdd:
			result = lv + rv
		case ssa.OpSub:
			result = lv - rv
		case ssa.OpMul:
			result = lv * rv
		case ssa.OpDiv:
			if rv == 0 {
				return nil // avoid div by zero
			}
			result = lv / rv
		case ssa.OpMod:
			if rv == 0 {
				return nil
			}
			result = float64(int64(lv) % int64(rv))
		default:
			return nil
		}

		c := fn.NewConst(ssa.ConstNumber, formatFloat(result))
		return &ssa.CopyInstr{Res: bin.Res, Src: c}
	}

	// Fold string + string (concatenation)
	if lc.Kind == ssa.ConstString && rc.Kind == ssa.ConstString && bin.Op == ssa.OpAdd {
		c := fn.NewConst(ssa.ConstString, lc.Val+rc.Val)
		return &ssa.CopyInstr{Res: bin.Res, Src: c}
	}

	return nil
}

func formatFloat(f float64) string {
	if f == float64(int64(f)) {
		return fmt.Sprintf("%d", int64(f))
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}
