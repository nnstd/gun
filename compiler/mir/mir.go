// Package mir defines the Mid-Level Intermediate Representation for the Gun
// transpiler. MIR normalizes HIR into a Go-friendly computational model:
//
//   - JS-only constructs are desugared (optional chaining, destructuring, etc.)
//   - Control flow is made explicit via basic blocks with terminators
//   - Complex expressions (ternary, sequence, assignment) are lowered to statements
//   - Each function body is a CFG (control flow graph) of basic blocks
//
// MIR is language-neutral but Go-friendly — it can be directly lowered to Go AST
// by the backend, or further transformed into SSA form for optimization.
package mir

import (
	"github.com/nnstd/gun/compiler/hir"
	"github.com/nnstd/gun/compiler/symbol"
)

// --------------------------------------------------------------------
// Module
// --------------------------------------------------------------------

// Module represents a single compiled file.
type Module struct {
	Package   string
	Functions []*Function
	Globals   []*Global
	Imports   []*Import
	Async     AsyncIndex
}

// Import is a Go import declaration.
type Import struct {
	Path  string // Go import path
	Alias string // alias (empty = default)
}

// Global is a package-level variable.
type Global struct {
	Symbol *symbol.Symbol
	Init   Expr // initial value (may be nil)
}

// --------------------------------------------------------------------
// Function
// --------------------------------------------------------------------

// Function is a MIR function with a CFG body.
type Function struct {
	Symbol *symbol.Symbol
	Params []*Param
	Blocks []*BasicBlock // Blocks[0] is the entry block
	Locals []*Variable   // all local variables (including params)

	// Metadata
	Exported bool
	IsMain   bool // true for main/init (Go func, not JSValue)
	Async    AsyncFuncInfo
}

// AsyncIndex summarizes async-related facts gathered during MIR lowering.
type AsyncIndex struct {
	BySymbol map[symbol.ID]AsyncFuncInfo
}

// AsyncFuncInfo records minimal async metadata for a lowered function.
type AsyncFuncInfo struct {
	Declared   bool
	AwaitCount int
}

// Param is a function parameter.
type Param struct {
	Symbol  *symbol.Symbol
	Rest    bool // ...args
	Default Expr // default value (nil if required)
}

// Variable is a local variable.
type Variable struct {
	Symbol *symbol.Symbol
}

// --------------------------------------------------------------------
// Basic Blocks & Terminators
// --------------------------------------------------------------------

// BasicBlock is a straight-line sequence of statements ending with a terminator.
type BasicBlock struct {
	ID    int
	Stmts []Stmt
	Term  Terminator

	// CFG edges (populated during construction)
	Preds []*BasicBlock
	Succs []*BasicBlock
}

// Terminator is the final instruction of a basic block that transfers control.
type Terminator interface {
	termNode()
}

// JumpTerm is an unconditional jump to another block.
type JumpTerm struct {
	Target *BasicBlock
}

// BranchTerm is a conditional branch (if/else).
type BranchTerm struct {
	Cond  Expr
	True  *BasicBlock
	False *BasicBlock
}

// ReturnTerm exits the function, optionally with a value.
type ReturnTerm struct {
	Value Expr // nil for bare return
}

// SwitchTerm is a multi-way branch.
type SwitchTerm struct {
	Tag     Expr
	Cases   []*SwitchCase
	Default *BasicBlock // nil if no default
}

// SwitchCase is one arm of a SwitchTerm.
type SwitchCase struct {
	Value  Expr
	Target *BasicBlock
}

// PanicTerm terminates a block with a panic (throw).
type PanicTerm struct {
	Value Expr
}

func (*JumpTerm) termNode()   {}
func (*BranchTerm) termNode() {}
func (*ReturnTerm) termNode() {}
func (*SwitchTerm) termNode() {}
func (*PanicTerm) termNode()  {}

// --------------------------------------------------------------------
// Statements
// --------------------------------------------------------------------

// Stmt is a MIR statement — a single side-effecting operation.
type Stmt interface {
	mirStmt()
}

// AssignStmt assigns a value to a variable.
type AssignStmt struct {
	Target *symbol.Symbol
	Value  Expr
}

// StoreStmt sets a property on an object: obj.Set(key, value).
type StoreStmt struct {
	Object Expr
	Key    Expr // string expression for property name
	Value  Expr
}

// ExprStmt evaluates an expression for its side effects.
type ExprStmt struct {
	Expr Expr
}

// DeclStmt declares a new local variable with optional initial value.
type DeclStmt struct {
	Symbol *symbol.Symbol
	Value  Expr // nil = uninitialized
}

// DeferStmt represents a defer call (lowered from try/finally).
type DeferStmt struct {
	Call Expr // the function call to defer
}

// ProtectedTryCatchStmt preserves try/catch/finally structure for async
// functions until async-aware lowering can consume it.
type ProtectedTryCatchStmt struct {
	Node *hir.TryCatchStmt
}

func (*AssignStmt) mirStmt()            {}
func (*StoreStmt) mirStmt()             {}
func (*ExprStmt) mirStmt()              {}
func (*DeclStmt) mirStmt()              {}
func (*DeferStmt) mirStmt()             {}
func (*ProtectedTryCatchStmt) mirStmt() {}

// --------------------------------------------------------------------
// Expressions
// --------------------------------------------------------------------

// Expr is a MIR expression — a pure value computation (no control flow).
type Expr interface {
	mirExpr()
}

// IdentExpr references a variable by symbol.
type IdentExpr struct {
	Symbol *symbol.Symbol
	Name   string // fallback for unresolved names
}

// LitExpr is a literal value.
type LitExpr struct {
	Kind  LitKind
	Value string
}

// LitKind is the type of a literal.
type LitKind int

const (
	LitString LitKind = iota
	LitNumber
	LitBool
	LitNull
	LitUndefined
	LitRegex
)

// BinExpr is a binary operation.
type BinExpr struct {
	Op    BinOp
	Left  Expr
	Right Expr
}

// BinOp is a binary operator (same values as hir.BinaryOp).
type BinOp int

const (
	OpAdd BinOp = iota
	OpSub
	OpMul
	OpDiv
	OpMod
	OpExp
	OpEq
	OpNEq
	OpEqLoose
	OpNEqLoose
	OpLt
	OpGt
	OpLtE
	OpGtE
	OpAnd
	OpOr
	OpNullish
	OpBitAnd
	OpBitOr
	OpBitXor
	OpShl
	OpShr
	OpUShr
	OpIn
	OpInstanceof
)

// UnaryExpr is a unary operation.
type UnaryExpr struct {
	Op      UnaryOp
	Operand Expr
}

// UnaryOp is a unary operator.
type UnaryOp int

const (
	OpNot UnaryOp = iota
	OpNeg
	OpPos
	OpBitNot
	OpTypeof
	OpVoid
)

// CallExpr is a function call.
type CallExpr struct {
	Func Expr
	Args []Expr
}

// NewCallExpr is a constructor call (new X(args)).
type NewCallExpr struct {
	Callee Expr
	Args   []Expr
}

// GetExpr reads a property: obj.Get("prop").
type GetExpr struct {
	Object Expr
	Key    Expr // string expression
}

// IndexExpr reads an element by index: arr.Index(i).
type IndexExpr struct {
	Object Expr
	Index  Expr
}

// FuncExpr is an inline function value (arrow function or function expression).
type FuncExpr struct {
	Func *Function // points to a MIR function
}

// ArrayExpr creates an array from elements.
type ArrayExpr struct {
	Elements []Expr
}

// ObjectExpr creates an object from key-value pairs.
type ObjectExpr struct {
	Keys   []Expr // string expressions
	Values []Expr
}

// SpreadExpr spreads an iterable.
type SpreadExpr struct {
	Value Expr
}

// TemplateExpr is a template string.
type TemplateExpr struct {
	Parts []Expr // alternating string literals and expressions
}

// ThisExpr is the `this` reference.
type ThisExpr struct{}

// NilExpr is the nil/undefined value.
type NilExpr struct{}

func (*IdentExpr) mirExpr()    {}
func (*LitExpr) mirExpr()      {}
func (*BinExpr) mirExpr()      {}
func (*UnaryExpr) mirExpr()    {}
func (*CallExpr) mirExpr()     {}
func (*NewCallExpr) mirExpr()  {}
func (*GetExpr) mirExpr()      {}
func (*IndexExpr) mirExpr()    {}
func (*FuncExpr) mirExpr()     {}
func (*ArrayExpr) mirExpr()    {}
func (*ObjectExpr) mirExpr()   {}
func (*SpreadExpr) mirExpr()   {}
func (*TemplateExpr) mirExpr() {}
func (*ThisExpr) mirExpr()     {}
func (*NilExpr) mirExpr()      {}
