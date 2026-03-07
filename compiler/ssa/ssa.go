// Package ssa provides Static Single Assignment form for the Gun transpiler.
//
// SSA converts MIR functions into a form where every variable is assigned
// exactly once, with phi nodes at control flow merge points. This form
// enables optimization passes like constant folding and dead code elimination.
//
// The SSA construction algorithm uses the standard approach:
//  1. Compute the dominator tree
//  2. Compute dominance frontiers
//  3. Insert phi nodes at iterated dominance frontiers
//  4. Rename variables using a stack-based algorithm
package ssa

import "github.com/nnstd/gun/compiler/symbol"

// --------------------------------------------------------------------
// Module & Function
// --------------------------------------------------------------------

// Module is a collection of SSA functions and globals.
type Module struct {
	Package   string
	Functions []*Function
	Globals   []*Global
}

// Global is a package-level variable (not in SSA form).
type Global struct {
	Symbol *symbol.Symbol
	Init   Value // initial value
}

// Function is an SSA function with a CFG of blocks.
type Function struct {
	Symbol *symbol.Symbol
	Params []*Param
	Blocks []*Block // Blocks[0] is the entry block

	nextValueID int
	Exported    bool
	IsMain      bool
}

// Param is a function parameter (each is an SSA value).
type Param struct {
	Symbol *symbol.Symbol
	Val    *SSAValue // the SSA value representing this param
	Rest   bool
}

// --------------------------------------------------------------------
// Blocks
// --------------------------------------------------------------------

// Block is a basic block in SSA form.
type Block struct {
	ID    int
	Phis  []*Phi
	Instrs []Instr
	Term  Terminator

	// CFG
	Preds []*Block
	Succs []*Block

	// Dominator tree
	IDom     *Block   // immediate dominator
	Children []*Block // blocks this block dominates
	DomFront []*Block // dominance frontier
}

// --------------------------------------------------------------------
// Values
// --------------------------------------------------------------------

// Value is the interface for all SSA values (results of instructions, phi nodes, constants).
type Value interface {
	ssaValue()
	ID() int
}

// SSAValue is a concrete SSA value with a unique ID.
type SSAValue struct {
	ValueID int
	Symbol  *symbol.Symbol // original variable (nil for temporaries)
	Def     Instr          // the instruction that defines this value (nil for params)
}

func (v *SSAValue) ssaValue() {}
func (v *SSAValue) ID() int   { return v.ValueID }

// Const is a constant value.
type Const struct {
	ValueID int
	Kind    ConstKind
	Val     string
}

// ConstKind is the type of constant.
type ConstKind int

const (
	ConstString ConstKind = iota
	ConstNumber
	ConstBool
	ConstNull
	ConstUndefined
)

func (c *Const) ssaValue() {}
func (c *Const) ID() int   { return c.ValueID }

// --------------------------------------------------------------------
// Phi nodes
// --------------------------------------------------------------------

// Phi is a phi instruction at a block entry, merging values from predecessors.
type Phi struct {
	Value *SSAValue              // the result value
	Edges map[*Block]Value       // predecessor block → incoming value
}

// --------------------------------------------------------------------
// Instructions
// --------------------------------------------------------------------

// Instr is an SSA instruction.
type Instr interface {
	instrNode()
	Result() *SSAValue // the value produced (nil if void)
}

// BinInstr is a binary operation.
type BinInstr struct {
	Res   *SSAValue
	Op    BinOp
	Left  Value
	Right Value
}

// BinOp is a binary operator (mirrors mir.BinOp).
type BinOp int

const (
	OpAdd BinOp = iota
	OpSub
	OpMul
	OpDiv
	OpMod
	OpEq
	OpNEq
	OpLt
	OpGt
	OpLtE
	OpGtE
	OpAnd
	OpOr
	OpBitAnd
	OpBitOr
	OpBitXor
	OpShl
	OpShr
)

// UnaryInstr is a unary operation.
type UnaryInstr struct {
	Res     *SSAValue
	Op      UnaryOp
	Operand Value
}

// UnaryOp is a unary operator.
type UnaryOp int

const (
	OpNot    UnaryOp = iota
	OpNeg
	OpBitNot
	OpTypeof
)

// CallInstr calls a function with arguments.
type CallInstr struct {
	Res  *SSAValue
	Func Value
	Args []Value
}

// GetInstr reads a property from an object.
type GetInstr struct {
	Res    *SSAValue
	Object Value
	Key    Value
}

// SetInstr writes a property on an object (no result value).
type SetInstr struct {
	Object Value
	Key    Value
	Val    Value
}

// NewInstr calls a constructor.
type NewInstr struct {
	Res    *SSAValue
	Callee Value
	Args   []Value
}

// AllocInstr creates a new array or object.
type AllocInstr struct {
	Res      *SSAValue
	Kind     AllocKind
	Elements []Value // for arrays: elements; for objects: alternating key/value
}

// AllocKind is the type of allocation.
type AllocKind int

const (
	AllocArray AllocKind = iota
	AllocObject
)

// CopyInstr copies a value (used during de-SSA to replace phi nodes).
type CopyInstr struct {
	Res *SSAValue
	Src Value
}

func (i *BinInstr) instrNode()   {}
func (i *BinInstr) Result() *SSAValue   { return i.Res }
func (i *UnaryInstr) instrNode() {}
func (i *UnaryInstr) Result() *SSAValue { return i.Res }
func (i *CallInstr) instrNode()  {}
func (i *CallInstr) Result() *SSAValue  { return i.Res }
func (i *GetInstr) instrNode()   {}
func (i *GetInstr) Result() *SSAValue   { return i.Res }
func (i *SetInstr) instrNode()   {}
func (i *SetInstr) Result() *SSAValue   { return nil }
func (i *NewInstr) instrNode()   {}
func (i *NewInstr) Result() *SSAValue   { return i.Res }
func (i *AllocInstr) instrNode() {}
func (i *AllocInstr) Result() *SSAValue { return i.Res }
func (i *CopyInstr) instrNode()  {}
func (i *CopyInstr) Result() *SSAValue  { return i.Res }

// --------------------------------------------------------------------
// Terminators
// --------------------------------------------------------------------

// Terminator is the final instruction of a block.
type Terminator interface {
	termNode()
}

// JumpTerm is an unconditional jump.
type JumpTerm struct {
	Target *Block
}

// BranchTerm is a conditional branch.
type BranchTerm struct {
	Cond  Value
	True  *Block
	False *Block
}

// ReturnTerm returns from the function.
type ReturnTerm struct {
	Value Value // nil for void return
}

// PanicTerm terminates with a panic.
type PanicTerm struct {
	Value Value
}

// SwitchTerm is a multi-way branch.
type SwitchTerm struct {
	Tag     Value
	Cases   []*SwitchCase
	Default *Block
}

// SwitchCase is one arm of a switch.
type SwitchCase struct {
	Value  Value
	Target *Block
}

func (*JumpTerm) termNode()   {}
func (*BranchTerm) termNode() {}
func (*ReturnTerm) termNode() {}
func (*PanicTerm) termNode()  {}
func (*SwitchTerm) termNode() {}

// --------------------------------------------------------------------
// Helpers
// --------------------------------------------------------------------

// NewValue allocates a new SSA value in the function.
func (f *Function) NewValue(sym *symbol.Symbol) *SSAValue {
	v := &SSAValue{
		ValueID: f.nextValueID,
		Symbol:  sym,
	}
	f.nextValueID++
	return v
}

// NewConst creates a constant value.
func (f *Function) NewConst(kind ConstKind, val string) *Const {
	c := &Const{
		ValueID: f.nextValueID,
		Kind:    kind,
		Val:     val,
	}
	f.nextValueID++
	return c
}
