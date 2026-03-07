// Package hir defines the High-Level Intermediate Representation for the Gun
// transpiler. HIR preserves TypeScript semantics in a structured form without
// any Go-specific constructs. It serves as the first IR stage between the
// tree-sitter CST and Go code generation.
//
// Key design: all identifiers reference *symbol.Symbol for hygienic name
// generation. Go names are never computed until code emission.
package hir

import "github.com/nnstd/gun/compiler/symbol"

// --------------------------------------------------------------------
// Module (top-level container)
// --------------------------------------------------------------------

// Module represents a single TypeScript file.
type Module struct {
	Package      string
	Imports      []*ImportDecl
	Declarations []Decl
	SymbolTable  *symbol.Table
}

// --------------------------------------------------------------------
// Interfaces
// --------------------------------------------------------------------

// Node is the common interface for all HIR nodes.
type Node interface {
	hirNode()
}

// Decl is a top-level declaration.
type Decl interface {
	Node
	hirDecl()
}

// Stmt is a statement inside a function body or block.
type Stmt interface {
	Node
	hirStmt()
}

// Expr is an expression that produces a value.
type Expr interface {
	Node
	hirExpr()
}

// --------------------------------------------------------------------
// Declarations
// --------------------------------------------------------------------

// FuncDecl represents a function declaration.
type FuncDecl struct {
	Symbol   *symbol.Symbol
	Params   []*Param
	Body     *BlockStmt
	Exported bool
	IsAsync  bool
}

// VarDecl represents a variable declaration (const/let/var).
type VarDecl struct {
	Declarators []*Declarator
	Kind        VarKind // Const, Let, Var
	Exported    bool
}

// VarKind is the kind of variable declaration.
type VarKind int

const (
	VarLet VarKind = iota
	VarConst
	VarVar
)

// Declarator is a single name = value binding in a VarDecl.
type Declarator struct {
	// Exactly one of Symbol or Pattern is set.
	Symbol  *symbol.Symbol // simple name binding
	Pattern Pattern        // destructuring pattern
	Init    Expr           // initial value (may be nil)
}

// ClassDecl represents a class declaration.
type ClassDecl struct {
	Symbol      *symbol.Symbol
	Parent      Expr // extends clause (nil if none)
	Constructor *ClassConstructor
	Methods     []*ClassMethod
	Properties  []*ClassProperty
	Exported    bool
}

// ClassConstructor is the constructor of a class.
type ClassConstructor struct {
	Params []*Param
	Body   *BlockStmt
}

// ClassMethod is a method on a class.
type ClassMethod struct {
	Name     string // method name (original TS name)
	Params   []*Param
	Body     *BlockStmt
	IsStatic bool
	IsGetter bool
	IsSetter bool
	Computed Expr // non-nil if the method name is computed: [expr]()
}

// ClassProperty is a property declaration in a class body.
type ClassProperty struct {
	Name     string
	Value    Expr // initial value (may be nil)
	IsStatic bool
	Computed Expr // non-nil if computed name
}

// EnumDecl represents an enum declaration.
type EnumDecl struct {
	Symbol   *symbol.Symbol
	Members  []*EnumMember
	Exported bool
}

// EnumMember is a single member of an enum.
type EnumMember struct {
	Name  string
	Value Expr // explicit value (nil for auto-incremented)
}

// InterfaceDecl represents a TypeScript interface declaration.
type InterfaceDecl struct {
	Symbol   *symbol.Symbol
	Members  []*InterfaceMember
	Exported bool
}

// InterfaceMember is a member of an interface.
type InterfaceMember struct {
	Name       string
	IsMethod   bool
	ParamCount int    // for methods
	Type       string // TS type annotation (for properties)
}

// TypeAliasDecl represents a type alias declaration.
type TypeAliasDecl struct {
	Symbol   *symbol.Symbol
	Type     string // raw TS type expression
	Exported bool
}

// ImportDecl represents an import statement.
type ImportDecl struct {
	ModulePath string
	Default    *ImportBinding   // import X from "mod"
	Named      []*ImportBinding // import { a, b } from "mod"
	Namespace  *ImportBinding   // import * as X from "mod"
	TypeOnly   bool
}

// ImportBinding is a single imported name.
type ImportBinding struct {
	LocalName    string         // name used in this file
	OriginalName string         // name in the source module
	Symbol       *symbol.Symbol // resolved symbol
}

// ExportDecl wraps a declaration that is being exported.
type ExportDecl struct {
	Decl      Decl   // the underlying declaration (may be nil for re-exports)
	IsDefault bool
	// For export clauses: export { a, b }
	Names []ExportName
	// For re-exports: export * from "mod"
	FromModule string
}

// ExportName is a single name in an export clause.
type ExportName struct {
	LocalName    string
	ExportedName string
}

// Param represents a function parameter.
type Param struct {
	// Exactly one of Symbol or Pattern is set.
	Symbol   *symbol.Symbol // simple parameter
	Pattern  Pattern        // destructuring parameter
	Default  Expr           // default value (nil if required)
	Rest     bool           // true for ...args
	TypeAnno string         // type annotation (empty if untyped)
}

// --------------------------------------------------------------------
// Patterns (destructuring)
// --------------------------------------------------------------------

// Pattern is the interface for destructuring patterns.
type Pattern interface {
	Node
	hirPattern()
}

// ObjectPattern represents { a, b: c, d = val } destructuring.
type ObjectPattern struct {
	Properties []*ObjectPatternProp
	Rest       *symbol.Symbol // ...rest (nil if none)
}

// ObjectPatternProp is a single property in an object pattern.
type ObjectPatternProp struct {
	Key     string         // property key in source object
	Value   *symbol.Symbol // local binding (nil if shorthand = Key)
	Default Expr           // default value (nil if required)
}

// ArrayPattern represents [a, b, ...rest] destructuring.
type ArrayPattern struct {
	Elements []*ArrayPatternElem // may contain nil for holes: [,x]
	Rest     *symbol.Symbol      // ...rest (nil if none)
}

// ArrayPatternElem is a single element in an array pattern.
type ArrayPatternElem struct {
	Symbol  *symbol.Symbol // local binding
	Default Expr           // default value (nil if required)
}

// --------------------------------------------------------------------
// Statements
// --------------------------------------------------------------------

// BlockStmt is a sequence of statements in a block.
type BlockStmt struct {
	Stmts []Stmt
}

// ExprStmt is a statement that evaluates an expression for side effects.
type ExprStmt struct {
	Expr Expr
}

// ReturnStmt is a return statement.
type ReturnStmt struct {
	Value Expr // nil for bare return
}

// IfStmt is an if/else statement.
type IfStmt struct {
	Cond Expr
	Then *BlockStmt
	Else Stmt // *BlockStmt or *IfStmt (else if) or nil
}

// ForStmt is a C-style for loop.
type ForStmt struct {
	Init Stmt // VarDecl or ExprStmt or nil
	Cond Expr // nil = infinite
	Post Expr // nil = no post
	Body *BlockStmt
}

// ForInStmt is a for...in loop.
type ForInStmt struct {
	Key   *symbol.Symbol // loop variable
	Value Expr           // object to iterate
	Body  *BlockStmt
}

// ForOfStmt is a for...of loop.
type ForOfStmt struct {
	Elem    *symbol.Symbol // loop variable
	Pattern Pattern        // destructuring pattern (alternative to Elem)
	Value   Expr           // iterable
	Body    *BlockStmt
}

// WhileStmt is a while loop.
type WhileStmt struct {
	Cond Expr
	Body *BlockStmt
}

// DoWhileStmt is a do...while loop.
type DoWhileStmt struct {
	Body *BlockStmt
	Cond Expr
}

// SwitchStmt is a switch statement.
type SwitchStmt struct {
	Tag   Expr
	Cases []*CaseClause
}

// CaseClause is a single case in a switch.
type CaseClause struct {
	Value Expr   // nil for default clause
	Body  []Stmt // statements (may fallthrough)
}

// TryCatchStmt is a try/catch/finally statement.
type TryCatchStmt struct {
	Try     *BlockStmt
	Catch   *CatchClause // nil if no catch
	Finally *BlockStmt   // nil if no finally
}

// CatchClause is the catch part of try/catch.
type CatchClause struct {
	Param *symbol.Symbol // catch parameter (nil for bare catch)
	Body  *BlockStmt
}

// ThrowStmt is a throw statement.
type ThrowStmt struct {
	Value Expr
}

// BreakStmt is a break statement.
type BreakStmt struct {
	Label string // empty for unlabeled
}

// ContinueStmt is a continue statement.
type ContinueStmt struct {
	Label string // empty for unlabeled
}

// LabeledStmt is a labeled statement.
type LabeledStmt struct {
	Label string
	Stmt  Stmt
}

// EmptyStmt is an empty statement (semicolon only).
type EmptyStmt struct{}

// --------------------------------------------------------------------
// Expressions
// --------------------------------------------------------------------

// Identifier refers to a named entity.
type Identifier struct {
	Sym  *symbol.Symbol // resolved symbol (nil for unresolved globals)
	Name string         // raw name when Sym is nil
}

// Literal is a primitive literal value.
type Literal struct {
	Kind  LiteralKind
	Value string // raw text representation
}

// LiteralKind is the type of a literal.
type LiteralKind int

const (
	LitString LiteralKind = iota
	LitNumber
	LitBool
	LitNull
	LitUndefined
	LitRegex
	LitBigInt
)

// TemplateLiteral is a template string with embedded expressions.
type TemplateLiteral struct {
	Parts []Expr // alternating StringLiteral and expression parts
}

// TaggedTemplateLiteral is a tagged template expression.
type TaggedTemplateLiteral struct {
	Tag      Expr
	Template *TemplateLiteral
}

// ArrayLiteral is an array expression.
type ArrayLiteral struct {
	Elements []Expr // elements (nil entry = hole)
}

// ObjectLiteral is an object expression.
type ObjectLiteral struct {
	Properties []*Property
}

// Property is a key-value pair in an object literal.
type Property struct {
	Key      Expr   // string literal, identifier, or computed expression
	KeyName  string // convenience: key as string (empty for computed)
	Value    Expr
	Computed bool // true if key is [expr]
	Method   bool // true for method shorthand { foo() {} }
}

// SpreadExpr is a spread element: ...expr.
type SpreadExpr struct {
	Value Expr
}

// BinaryExpr is a binary operation.
type BinaryExpr struct {
	Op    BinaryOp
	Left  Expr
	Right Expr
}

// BinaryOp is a binary operator.
type BinaryOp int

const (
	// Arithmetic
	OpAdd BinaryOp = iota
	OpSub
	OpMul
	OpDiv
	OpMod
	OpExp // **

	// Comparison
	OpEq      // ===
	OpNEq     // !==
	OpEqLoose // ==
	OpNEqLoose // !=
	OpLt
	OpGt
	OpLtE
	OpGtE

	// Logical
	OpAnd // &&
	OpOr  // ||
	OpNullish // ??

	// Bitwise
	OpBitAnd
	OpBitOr
	OpBitXor
	OpShl
	OpShr
	OpUShr

	// Special
	OpIn         // in
	OpInstanceof // instanceof
)

// UnaryExpr is a unary operation.
type UnaryExpr struct {
	Op      UnaryOp
	Operand Expr
	Prefix  bool // true for prefix (!x), false for postfix (x++)
}

// UnaryOp is a unary operator.
type UnaryOp int

const (
	OpNot    UnaryOp = iota // !
	OpNeg                   // - (negate)
	OpPos                   // + (positive, identity)
	OpBitNot                // ~
	OpTypeof                // typeof
	OpVoid                  // void
	OpDelete                // delete
)

// UpdateExpr is a pre/post increment/decrement.
type UpdateExpr struct {
	Op      UpdateOp
	Operand Expr
	Prefix  bool
}

// UpdateOp is an update operator.
type UpdateOp int

const (
	OpInc UpdateOp = iota // ++
	OpDec                 // --
)

// AssignExpr is an assignment expression.
type AssignExpr struct {
	Op    AssignOp
	Left  Expr // Identifier, MemberExpr, or SubscriptExpr
	Right Expr
}

// AssignOp is an assignment operator.
type AssignOp int

const (
	OpAssign AssignOp = iota // =
	OpAddAssign              // +=
	OpSubAssign              // -=
	OpMulAssign              // *=
	OpDivAssign              // /=
	OpModAssign              // %=
	OpBitAndAssign           // &=
	OpBitOrAssign            // |=
	OpBitXorAssign           // ^=
	OpShlAssign              // <<=
	OpShrAssign              // >>=
	OpUShrAssign             // >>>=
	OpNullishAssign          // ??=
	OpAndAssign              // &&=
	OpOrAssign               // ||=
	OpExpAssign              // **=
)

// CallExpr is a function or method call.
type CallExpr struct {
	Func Expr   // function to call
	Args []Expr // arguments
}

// NewExpr is a constructor invocation: new X(args).
type NewExpr struct {
	Callee Expr
	Args   []Expr
}

// MemberExpr is a property access: obj.prop.
type MemberExpr struct {
	Object   Expr
	Property string // property name
}

// ComputedMemberExpr is a computed property access: obj[expr].
type ComputedMemberExpr struct {
	Object   Expr
	Property Expr // index or key expression
}

// TernaryExpr is a ternary/conditional expression: cond ? then : else.
type TernaryExpr struct {
	Cond Expr
	Then Expr
	Else Expr
}

// ArrowFunc is an arrow function expression.
type ArrowFunc struct {
	Params  []*Param
	Body    *BlockStmt // nil if ExprBody is set
	ExprBody Expr       // concise body: () => expr (nil if Body is set)
	IsAsync bool
}

// FuncExpr is a function expression.
type FuncExpr struct {
	Name    string // optional name (empty for anonymous)
	Params  []*Param
	Body    *BlockStmt
	IsAsync bool
}

// SequenceExpr is a comma-separated sequence of expressions.
type SequenceExpr struct {
	Exprs []Expr
}

// AwaitExpr is an await expression.
type AwaitExpr struct {
	Value Expr
}

// YieldExpr is a yield expression (generators).
type YieldExpr struct {
	Value    Expr
	Delegate bool // yield*
}

// TypeAssertExpr is a type assertion (as X) or type cast (<X>).
// Preserved in HIR to allow the backend to decide what to do.
type TypeAssertExpr struct {
	Expr Expr
	Type string // raw TS type string
}

// NonNullExpr is the non-null assertion: expr!
type NonNullExpr struct {
	Expr Expr
}

// ParenExpr is a parenthesized expression.
type ParenExpr struct {
	Expr Expr
}

// ThisExpr is the `this` keyword.
type ThisExpr struct{}

// SuperExpr is the `super` keyword.
type SuperExpr struct{}

// MetaPropertyExpr is import.meta or new.target.
type MetaPropertyExpr struct {
	Meta     string // "import" or "new"
	Property string // "meta" or "target"
}

// --------------------------------------------------------------------
// Node interface implementations (marker methods)
// --------------------------------------------------------------------

func (*FuncDecl) hirNode()      {}
func (*FuncDecl) hirDecl()      {}
func (*VarDecl) hirNode()       {}
func (*VarDecl) hirDecl()       {}
func (*ClassDecl) hirNode()     {}
func (*ClassDecl) hirDecl()     {}
func (*EnumDecl) hirNode()      {}
func (*EnumDecl) hirDecl()      {}
func (*InterfaceDecl) hirNode() {}
func (*InterfaceDecl) hirDecl() {}
func (*TypeAliasDecl) hirNode() {}
func (*TypeAliasDecl) hirDecl() {}
func (*ImportDecl) hirNode()    {}
func (*ImportDecl) hirDecl()    {}
func (*ExportDecl) hirNode()    {}
func (*ExportDecl) hirDecl()    {}

func (*BlockStmt) hirNode()    {}
func (*BlockStmt) hirStmt()    {}
func (*ExprStmt) hirNode()     {}
func (*ExprStmt) hirStmt()     {}
func (*ReturnStmt) hirNode()   {}
func (*ReturnStmt) hirStmt()   {}
func (*IfStmt) hirNode()       {}
func (*IfStmt) hirStmt()       {}
func (*ForStmt) hirNode()      {}
func (*ForStmt) hirStmt()      {}
func (*ForInStmt) hirNode()    {}
func (*ForInStmt) hirStmt()    {}
func (*ForOfStmt) hirNode()    {}
func (*ForOfStmt) hirStmt()    {}
func (*WhileStmt) hirNode()    {}
func (*WhileStmt) hirStmt()    {}
func (*DoWhileStmt) hirNode()  {}
func (*DoWhileStmt) hirStmt()  {}
func (*SwitchStmt) hirNode()   {}
func (*SwitchStmt) hirStmt()   {}
func (*TryCatchStmt) hirNode() {}
func (*TryCatchStmt) hirStmt() {}
func (*ThrowStmt) hirNode()    {}
func (*ThrowStmt) hirStmt()    {}
func (*BreakStmt) hirNode()    {}
func (*BreakStmt) hirStmt()    {}
func (*ContinueStmt) hirNode() {}
func (*ContinueStmt) hirStmt() {}
func (*LabeledStmt) hirNode()  {}
func (*LabeledStmt) hirStmt()  {}
func (*EmptyStmt) hirNode()    {}
func (*EmptyStmt) hirStmt()    {}
// VarDecl is also a valid statement (let/const in blocks)
func (*VarDecl) hirStmt() {}

func (*Identifier) hirNode()           {}
func (*Identifier) hirExpr()           {}
func (*Literal) hirNode()              {}
func (*Literal) hirExpr()              {}
func (*TemplateLiteral) hirNode()      {}
func (*TemplateLiteral) hirExpr()      {}
func (*TaggedTemplateLiteral) hirNode() {}
func (*TaggedTemplateLiteral) hirExpr() {}
func (*ArrayLiteral) hirNode()         {}
func (*ArrayLiteral) hirExpr()         {}
func (*ObjectLiteral) hirNode()        {}
func (*ObjectLiteral) hirExpr()        {}
func (*SpreadExpr) hirNode()           {}
func (*SpreadExpr) hirExpr()           {}
func (*BinaryExpr) hirNode()           {}
func (*BinaryExpr) hirExpr()           {}
func (*UnaryExpr) hirNode()            {}
func (*UnaryExpr) hirExpr()            {}
func (*UpdateExpr) hirNode()           {}
func (*UpdateExpr) hirExpr()           {}
func (*AssignExpr) hirNode()           {}
func (*AssignExpr) hirExpr()           {}
func (*CallExpr) hirNode()             {}
func (*CallExpr) hirExpr()             {}
func (*NewExpr) hirNode()              {}
func (*NewExpr) hirExpr()              {}
func (*MemberExpr) hirNode()           {}
func (*MemberExpr) hirExpr()           {}
func (*ComputedMemberExpr) hirNode()   {}
func (*ComputedMemberExpr) hirExpr()   {}
func (*TernaryExpr) hirNode()          {}
func (*TernaryExpr) hirExpr()          {}
func (*ArrowFunc) hirNode()            {}
func (*ArrowFunc) hirExpr()            {}
func (*FuncExpr) hirNode()             {}
func (*FuncExpr) hirExpr()             {}
func (*SequenceExpr) hirNode()         {}
func (*SequenceExpr) hirExpr()         {}
func (*AwaitExpr) hirNode()            {}
func (*AwaitExpr) hirExpr()            {}
func (*YieldExpr) hirNode()            {}
func (*YieldExpr) hirExpr()            {}
func (*TypeAssertExpr) hirNode()       {}
func (*TypeAssertExpr) hirExpr()       {}
func (*NonNullExpr) hirNode()          {}
func (*NonNullExpr) hirExpr()          {}
func (*ParenExpr) hirNode()            {}
func (*ParenExpr) hirExpr()            {}
func (*ThisExpr) hirNode()             {}
func (*ThisExpr) hirExpr()             {}
func (*SuperExpr) hirNode()            {}
func (*SuperExpr) hirExpr()            {}
func (*MetaPropertyExpr) hirNode()     {}
func (*MetaPropertyExpr) hirExpr()     {}

func (*ObjectPattern) hirNode()    {}
func (*ObjectPattern) hirPattern() {}
func (*ArrayPattern) hirNode()     {}
func (*ArrayPattern) hirPattern()  {}
