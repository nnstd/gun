package passes

import (
	"testing"

	"github.com/nnstd/gun/compiler/hir"
	"github.com/nnstd/gun/compiler/mir"
	"github.com/nnstd/gun/compiler/ssa"
	sitter "github.com/tree-sitter/go-tree-sitter"
	typescript "github.com/tree-sitter/tree-sitter-typescript/bindings/go"
)

func parseTS(t *testing.T, source string) *sitter.Tree {
	t.Helper()
	parser := sitter.NewParser()
	defer parser.Close()
	lang := sitter.NewLanguage(typescript.LanguageTypescript())
	if err := parser.SetLanguage(lang); err != nil {
		t.Fatal(err)
	}
	tree := parser.Parse([]byte(source), nil)
	if tree == nil {
		t.Fatal("parse returned nil")
	}
	return tree
}

func buildSSA(t *testing.T, source string) *ssa.Module {
	t.Helper()
	tree := parseTS(t, source)
	defer tree.Close()
	hirMod := hir.BuildModule(tree.RootNode(), []byte(source), "main")
	mirMod := mir.Lower(hirMod)
	return ssa.Build(mirMod)
}

// --- ConstFold tests ---

func TestConstFoldName(t *testing.T) {
	p := ConstFold{}
	if p.Name() != "const-fold" {
		t.Fatalf("expected 'const-fold', got %q", p.Name())
	}
}

func TestConstFoldRuns(t *testing.T) {
	mod := buildSSA(t, `const x = 1 + 2;`)
	err := ConstFold{}.Run(mod)
	if err != nil {
		t.Fatalf("ConstFold.Run failed: %v", err)
	}
}

func TestConstFoldNumbers(t *testing.T) {
	// Build a simple SSA module with 1 + 2
	fn := &ssa.Function{}
	b := &ssa.Block{ID: 0}
	fn.Blocks = []*ssa.Block{b}

	left := fn.NewConst(ssa.ConstNumber, "3")
	right := fn.NewConst(ssa.ConstNumber, "4")
	res := fn.NewValue(nil)
	b.Instrs = []ssa.Instr{
		&ssa.BinInstr{Res: res, Op: ssa.OpAdd, Left: left, Right: right},
	}
	b.Term = &ssa.ReturnTerm{Value: res}

	mod := &ssa.Module{Functions: []*ssa.Function{fn}}
	err := ConstFold{}.Run(mod)
	if err != nil {
		t.Fatal(err)
	}

	// Should be folded to CopyInstr with constant 7
	if len(b.Instrs) != 1 {
		t.Fatalf("expected 1 instruction, got %d", len(b.Instrs))
	}
	copy, ok := b.Instrs[0].(*ssa.CopyInstr)
	if !ok {
		t.Fatalf("expected CopyInstr, got %T", b.Instrs[0])
	}
	c, ok := copy.Src.(*ssa.Const)
	if !ok {
		t.Fatalf("expected Const source, got %T", copy.Src)
	}
	if c.Val != "7" {
		t.Fatalf("expected folded value '7', got %q", c.Val)
	}
}

func TestConstFoldStrings(t *testing.T) {
	fn := &ssa.Function{}
	b := &ssa.Block{ID: 0}
	fn.Blocks = []*ssa.Block{b}

	left := fn.NewConst(ssa.ConstString, "hello")
	right := fn.NewConst(ssa.ConstString, " world")
	res := fn.NewValue(nil)
	b.Instrs = []ssa.Instr{
		&ssa.BinInstr{Res: res, Op: ssa.OpAdd, Left: left, Right: right},
	}
	b.Term = &ssa.ReturnTerm{Value: res}

	mod := &ssa.Module{Functions: []*ssa.Function{fn}}
	ConstFold{}.Run(mod)

	copy, ok := b.Instrs[0].(*ssa.CopyInstr)
	if !ok {
		t.Fatalf("expected CopyInstr, got %T", b.Instrs[0])
	}
	c := copy.Src.(*ssa.Const)
	if c.Val != "hello world" {
		t.Fatalf("expected 'hello world', got %q", c.Val)
	}
}

func TestConstFoldNoFoldMixed(t *testing.T) {
	fn := &ssa.Function{}
	b := &ssa.Block{ID: 0}
	fn.Blocks = []*ssa.Block{b}

	left := fn.NewConst(ssa.ConstNumber, "1")
	right := fn.NewValue(nil) // non-constant
	res := fn.NewValue(nil)
	b.Instrs = []ssa.Instr{
		&ssa.BinInstr{Res: res, Op: ssa.OpAdd, Left: left, Right: right},
	}
	b.Term = &ssa.ReturnTerm{Value: res}

	mod := &ssa.Module{Functions: []*ssa.Function{fn}}
	ConstFold{}.Run(mod)

	// Should NOT be folded
	_, ok := b.Instrs[0].(*ssa.BinInstr)
	if !ok {
		t.Fatalf("expected BinInstr (not folded), got %T", b.Instrs[0])
	}
}

// --- DCE tests ---

func TestDCEName(t *testing.T) {
	p := DCE{}
	if p.Name() != "dce" {
		t.Fatalf("expected 'dce', got %q", p.Name())
	}
}

func TestDCERuns(t *testing.T) {
	mod := buildSSA(t, `const x = 42;`)
	err := DCE{}.Run(mod)
	if err != nil {
		t.Fatalf("DCE.Run failed: %v", err)
	}
}

func TestDCERemovesUnused(t *testing.T) {
	fn := &ssa.Function{}
	b := &ssa.Block{ID: 0}
	fn.Blocks = []*ssa.Block{b}

	// Two instructions: one used in return, one unused
	used := fn.NewValue(nil)
	unused := fn.NewValue(nil)
	left := fn.NewConst(ssa.ConstNumber, "1")
	right := fn.NewConst(ssa.ConstNumber, "2")

	b.Instrs = []ssa.Instr{
		&ssa.BinInstr{Res: used, Op: ssa.OpAdd, Left: left, Right: right},
		&ssa.BinInstr{Res: unused, Op: ssa.OpSub, Left: left, Right: right}, // dead
	}
	b.Term = &ssa.ReturnTerm{Value: used}

	mod := &ssa.Module{Functions: []*ssa.Function{fn}}
	DCE{}.Run(mod)

	// The unused BinInstr should be eliminated
	if len(b.Instrs) != 1 {
		t.Fatalf("expected 1 instruction after DCE, got %d", len(b.Instrs))
	}
}

func TestDCEKeepsSideEffects(t *testing.T) {
	fn := &ssa.Function{}
	b := &ssa.Block{ID: 0}
	fn.Blocks = []*ssa.Block{b}

	// A call instruction has side effects — should not be removed even if result is unused
	callRes := fn.NewValue(nil)
	callee := fn.NewConst(ssa.ConstString, "console.log")
	b.Instrs = []ssa.Instr{
		&ssa.CallInstr{Res: callRes, Func: callee, Args: nil},
	}
	b.Term = &ssa.ReturnTerm{}

	mod := &ssa.Module{Functions: []*ssa.Function{fn}}
	DCE{}.Run(mod)

	// Call should be kept (side effects)
	if len(b.Instrs) != 1 {
		t.Fatalf("expected 1 instruction (call kept), got %d", len(b.Instrs))
	}
}

// --- Pass interface tests ---

func TestPassInterface(t *testing.T) {
	passes := []Pass{ConstFold{}, DCE{}}
	mod := buildSSA(t, `function f() { return 1 + 2; }`)
	for _, p := range passes {
		if err := p.Run(mod); err != nil {
			t.Fatalf("pass %q failed: %v", p.Name(), err)
		}
	}
}
