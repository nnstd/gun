package ssa

import (
	"testing"

	"github.com/nnstd/gun/compiler/hir"
	"github.com/nnstd/gun/compiler/mir"
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

func buildSSA(t *testing.T, source string) *Module {
	t.Helper()
	tree := parseTS(t, source)
	defer tree.Close()
	hirMod := hir.BuildModule(tree.RootNode(), []byte(source), "main")
	mirMod := mir.Lower(hirMod)
	return Build(mirMod)
}

// --- SSA Build Tests ---

func TestBuildSimpleFunction(t *testing.T) {
	mod := buildSSA(t, `function add(a, b) { return a + b; }`)

	if len(mod.Functions) == 0 {
		t.Fatal("expected at least one function")
	}
	fn := mod.Functions[0]
	if fn.Symbol.OriginalName != "add" {
		t.Fatalf("expected 'add', got %q", fn.Symbol.OriginalName)
	}
	if len(fn.Params) != 2 {
		t.Fatalf("expected 2 params, got %d", len(fn.Params))
	}
	if len(fn.Blocks) == 0 {
		t.Fatal("expected blocks")
	}
}

func TestBuildVariable(t *testing.T) {
	mod := buildSSA(t, `const x = 42;`)
	if len(mod.Globals) == 0 {
		t.Fatal("expected globals")
	}
}

func TestBuildIfBranch(t *testing.T) {
	mod := buildSSA(t, `
		function f() {
			if (true) { return 1; } else { return 2; }
		}
	`)
	fn := mod.Functions[0]
	// Should have multiple blocks with BranchTerm
	hasBranch := false
	for _, b := range fn.Blocks {
		if _, ok := b.Term.(*BranchTerm); ok {
			hasBranch = true
		}
	}
	if !hasBranch {
		t.Fatal("expected a BranchTerm")
	}
}

func TestBuildWhileLoop(t *testing.T) {
	mod := buildSSA(t, `
		function f() {
			while (true) { break; }
		}
	`)
	fn := mod.Functions[0]
	if len(fn.Blocks) < 3 {
		t.Fatalf("expected at least 3 blocks, got %d", len(fn.Blocks))
	}
}

func TestSSAValueIDs(t *testing.T) {
	mod := buildSSA(t, `function f(x) { return x + 1; }`)
	fn := mod.Functions[0]
	// All values should have unique IDs
	seen := make(map[int]bool)
	for _, b := range fn.Blocks {
		for _, instr := range b.Instrs {
			if res := instr.Result(); res != nil {
				if seen[res.ValueID] {
					t.Fatalf("duplicate value ID: %d", res.ValueID)
				}
				seen[res.ValueID] = true
			}
		}
	}
}

// --- Dominator Tree Tests ---

func TestDominatorTree(t *testing.T) {
	mod := buildSSA(t, `
		function f(x) {
			if (x) { return 1; }
			return 2;
		}
	`)
	fn := mod.Functions[0]
	entry := fn.Blocks[0]

	// Entry should dominate itself
	if entry.IDom != entry {
		t.Fatal("entry should be its own immediate dominator")
	}

	// Reachable blocks (with predecessors or entry) should have dominators
	for _, b := range fn.Blocks[1:] {
		if len(b.Preds) == 0 {
			continue // unreachable blocks may not have IDom
		}
		if b.IDom == nil {
			t.Fatalf("reachable block %d has no immediate dominator", b.ID)
		}
		// Walk up dominator tree — should reach entry
		runner := b
		reachedEntry := false
		for i := 0; i < len(fn.Blocks)+1; i++ {
			if runner == entry {
				reachedEntry = true
				break
			}
			runner = runner.IDom
			if runner == nil {
				break
			}
		}
		if !reachedEntry {
			t.Fatalf("block %d not dominated by entry", b.ID)
		}
	}
}

// --- De-SSA Tests ---

func TestDeSSARoundTrip(t *testing.T) {
	tree := parseTS(t, `function add(a, b) { return a + b; }`)
	defer tree.Close()

	hirMod := hir.BuildModule(tree.RootNode(), []byte(`function add(a, b) { return a + b; }`), "main")
	mirMod := mir.Lower(hirMod)
	ssaMod := Build(mirMod)
	mirOut := DeSSA(ssaMod)

	if mirOut.Package != "main" {
		t.Fatalf("expected package 'main', got %q", mirOut.Package)
	}
	if len(mirOut.Functions) == 0 {
		t.Fatal("expected functions after de-SSA")
	}
	fn := mirOut.Functions[0]
	if fn.Symbol.OriginalName != "add" {
		t.Fatalf("expected 'add', got %q", fn.Symbol.OriginalName)
	}
	if len(fn.Blocks) == 0 {
		t.Fatal("expected blocks after de-SSA")
	}
}

func TestDeSSAPreservesStructure(t *testing.T) {
	tree := parseTS(t, `
		function f() {
			if (true) { return 1; }
			return 2;
		}
	`)
	defer tree.Close()

	hirMod := hir.BuildModule(tree.RootNode(), []byte(`
		function f() {
			if (true) { return 1; }
			return 2;
		}
	`), "main")
	mirMod := mir.Lower(hirMod)
	ssaMod := Build(mirMod)
	mirOut := DeSSA(ssaMod)

	fn := mirOut.Functions[0]
	// Should still have multiple blocks
	if len(fn.Blocks) < 3 {
		t.Fatalf("expected at least 3 blocks after de-SSA, got %d", len(fn.Blocks))
	}
	// Should have BranchTerm preserved
	hasBranch := false
	for _, b := range fn.Blocks {
		if _, ok := b.Term.(*mir.BranchTerm); ok {
			hasBranch = true
		}
	}
	if !hasBranch {
		t.Fatal("expected BranchTerm preserved after de-SSA")
	}
}

func TestFullPipelineRoundTrip(t *testing.T) {
	// TS → HIR → MIR → SSA → De-SSA → MIR
	// Verify the MIR output is structurally valid
	snippets := []string{
		`function f() { return 42; }`,
		`function f(x) { if (x) { return 1; } return 0; }`,
		`function f() { while (true) { break; } }`,
		`const x = 1 + 2;`,
	}
	for _, ts := range snippets {
		tree := parseTS(t, ts)
		hirMod := hir.BuildModule(tree.RootNode(), []byte(ts), "main")
		mirMod := mir.Lower(hirMod)
		ssaMod := Build(mirMod)
		mirOut := DeSSA(ssaMod)
		tree.Close()

		if mirOut == nil {
			t.Fatalf("de-SSA returned nil for %q", ts)
		}
		if mirOut.Package != "main" {
			t.Fatalf("wrong package for %q", ts)
		}
	}
}
