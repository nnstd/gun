package mir

import (
	"strings"
	"testing"

	"github.com/nnstd/gun/compiler/hir"
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

func lowerToMIR(t *testing.T, source string) *Module {
	t.Helper()
	tree := parseTS(t, source)
	defer tree.Close()
	hirMod := hir.BuildModule(tree.RootNode(), []byte(source), "main")
	return Lower(hirMod)
}

func mirString(t *testing.T, source string) string {
	t.Helper()
	mod := lowerToMIR(t, source)
	return Sprint(mod)
}

func assertContains(t *testing.T, got, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Errorf("MIR does not contain %q\nGot:\n%s", want, got)
	}
}

// --- Basic tests ---

func TestLowerSimpleFunction(t *testing.T) {
	mod := lowerToMIR(t, `function add(a, b) { return a + b; }`)

	if len(mod.Functions) == 0 {
		t.Fatal("expected at least one function")
	}
	fn := mod.Functions[0]
	if fn.Symbol.OriginalName != "add" {
		t.Fatalf("expected function 'add', got %q", fn.Symbol.OriginalName)
	}
	if len(fn.Params) != 2 {
		t.Fatalf("expected 2 params, got %d", len(fn.Params))
	}
	if len(fn.Blocks) == 0 {
		t.Fatal("expected at least one block")
	}
	// Entry block should have a ReturnTerm
	hasReturn := false
	for _, b := range fn.Blocks {
		if _, ok := b.Term.(*ReturnTerm); ok {
			hasReturn = true
		}
	}
	if !hasReturn {
		t.Fatal("expected a return terminator")
	}
}

func TestLowerVariable(t *testing.T) {
	mod := lowerToMIR(t, `const x = 42;`)

	if len(mod.Globals) == 0 {
		t.Fatal("expected at least one global")
	}
	g := mod.Globals[0]
	if g.Symbol.OriginalName != "x" {
		t.Fatalf("expected global 'x', got %q", g.Symbol.OriginalName)
	}
	lit, ok := g.Init.(*LitExpr)
	if !ok {
		t.Fatalf("expected LitExpr, got %T", g.Init)
	}
	if lit.Value != "42" {
		t.Fatalf("expected value '42', got %q", lit.Value)
	}
}

func TestLowerAsyncFunctionMetadata(t *testing.T) {
	mod := lowerToMIR(t, `async function load() { return await fetch(); }`)
	if len(mod.Functions) == 0 {
		t.Fatal("expected at least one function")
	}
	fn := mod.Functions[0]
	if !fn.Async.Declared {
		t.Fatal("expected async metadata on function")
	}
	if fn.Async.AwaitCount != 1 {
		t.Fatalf("await count = %d, want 1", fn.Async.AwaitCount)
	}
	if got := mod.Async.BySymbol[fn.Symbol.ID].AwaitCount; got != 1 {
		t.Fatalf("module async index await count = %d, want 1", got)
	}
}

func TestLowerAsyncTryCatchPreservesProtectedRegion(t *testing.T) {
	mod := lowerToMIR(t, `
		async function load() {
			try {
				return await fetch();
			} catch (err) {
				return err;
			}
		}
	`)
	fn := mod.Functions[0]
	found := false
	for _, b := range fn.Blocks {
		for _, st := range b.Stmts {
			if _, ok := st.(*ProtectedTryCatchStmt); ok {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("expected protected try/catch stmt in async MIR")
	}
}

func TestLowerIfCFG(t *testing.T) {
	mod := lowerToMIR(t, `
		function f() {
			if (true) {
				return 1;
			} else {
				return 2;
			}
		}
	`)

	fn := mod.Functions[0]
	// Should have at least 4 blocks: entry, then, else, join
	if len(fn.Blocks) < 4 {
		t.Fatalf("expected at least 4 blocks for if/else, got %d", len(fn.Blocks))
	}

	// Entry block should have a BranchTerm
	entry := fn.Blocks[0]
	if _, ok := entry.Term.(*BranchTerm); !ok {
		t.Fatalf("entry block should have BranchTerm, got %T", entry.Term)
	}
}

func TestLowerWhileCFG(t *testing.T) {
	mod := lowerToMIR(t, `
		function f() {
			while (true) {
				break;
			}
		}
	`)

	fn := mod.Functions[0]
	// Should have blocks: entry, cond, body, exit
	if len(fn.Blocks) < 4 {
		t.Fatalf("expected at least 4 blocks for while, got %d", len(fn.Blocks))
	}

	// Find the cond block (should have BranchTerm)
	hasBranch := false
	for _, b := range fn.Blocks {
		if _, ok := b.Term.(*BranchTerm); ok {
			hasBranch = true
		}
	}
	if !hasBranch {
		t.Fatal("expected a BranchTerm in while loop")
	}
}

func TestLowerForCFG(t *testing.T) {
	mod := lowerToMIR(t, `
		function f() {
			for (let i = 0; i < 10; i++) {
				// body
			}
		}
	`)

	fn := mod.Functions[0]
	// for loop: entry, cond, body, post, exit
	if len(fn.Blocks) < 5 {
		t.Fatalf("expected at least 5 blocks for for loop, got %d", len(fn.Blocks))
	}
}

func TestLowerDoWhileCFG(t *testing.T) {
	mod := lowerToMIR(t, `
		function f() {
			do {
				// body
			} while (false);
		}
	`)

	fn := mod.Functions[0]
	// do-while: entry, body, cond, exit
	if len(fn.Blocks) < 4 {
		t.Fatalf("expected at least 4 blocks for do-while, got %d", len(fn.Blocks))
	}
}

func TestLowerSwitch(t *testing.T) {
	mod := lowerToMIR(t, `
		function f(x) {
			switch (x) {
				case 1: return "one";
				case 2: return "two";
				default: return "other";
			}
		}
	`)

	fn := mod.Functions[0]
	// Find SwitchTerm
	hasSwitch := false
	for _, b := range fn.Blocks {
		if _, ok := b.Term.(*SwitchTerm); ok {
			hasSwitch = true
		}
	}
	if !hasSwitch {
		t.Fatal("expected a SwitchTerm")
	}
}

func TestLowerThrow(t *testing.T) {
	mod := lowerToMIR(t, `function f() { throw new Error("fail"); }`)

	fn := mod.Functions[0]
	hasPanic := false
	for _, b := range fn.Blocks {
		if _, ok := b.Term.(*PanicTerm); ok {
			hasPanic = true
		}
	}
	if !hasPanic {
		t.Fatal("expected a PanicTerm for throw")
	}
}

func TestLowerBreakContinue(t *testing.T) {
	mod := lowerToMIR(t, `
		function f() {
			while (true) {
				if (true) break;
				continue;
			}
		}
	`)

	fn := mod.Functions[0]
	// break and continue should produce JumpTerms
	jumpCount := 0
	for _, b := range fn.Blocks {
		if _, ok := b.Term.(*JumpTerm); ok {
			jumpCount++
		}
	}
	if jumpCount < 2 {
		t.Fatalf("expected at least 2 JumpTerms (break+continue), got %d", jumpCount)
	}
}

// --- Printer tests ---

func TestPrinterOutput(t *testing.T) {
	out := mirString(t, `
		function greet(name) {
			if (name) {
				return name;
			}
			return "world";
		}
	`)
	assertContains(t, out, "func greet")
	assertContains(t, out, "bb0:")
	assertContains(t, out, "branch")
	assertContains(t, out, "return")
}

func TestPrinterGlobals(t *testing.T) {
	out := mirString(t, `const x = 42;`)
	assertContains(t, out, "global x")
	assertContains(t, out, "42")
}

func TestPrinterSwitch(t *testing.T) {
	out := mirString(t, `
		function f(x) {
			switch (x) {
				case 1: return 1;
				default: return 0;
			}
		}
	`)
	assertContains(t, out, "switch")
	assertContains(t, out, "case")
}

// --- CFG edge tests ---

func TestCFGEdges(t *testing.T) {
	mod := lowerToMIR(t, `
		function f() {
			if (true) {
				return 1;
			}
			return 2;
		}
	`)

	fn := mod.Functions[0]
	entry := fn.Blocks[0]

	// Entry should have 2 successors (then, else)
	if len(entry.Succs) != 2 {
		t.Fatalf("entry block should have 2 successors, got %d", len(entry.Succs))
	}

	// Then and else blocks should have entry as predecessor
	for _, succ := range entry.Succs {
		found := false
		for _, pred := range succ.Preds {
			if pred == entry {
				found = true
			}
		}
		if !found {
			t.Fatalf("successor bb%d should have entry as predecessor", succ.ID)
		}
	}
}

func TestCFGWhileBackedge(t *testing.T) {
	mod := lowerToMIR(t, `
		function f() {
			while (true) {}
		}
	`)

	fn := mod.Functions[0]

	// Find the cond block — it should have a backedge (pred from body/post)
	var condBlock *BasicBlock
	for _, b := range fn.Blocks {
		if _, ok := b.Term.(*BranchTerm); ok {
			condBlock = b
			break
		}
	}
	if condBlock == nil {
		t.Fatal("expected a cond block with BranchTerm")
	}
	// Cond block should have at least 2 predecessors (entry jump + backedge)
	if len(condBlock.Preds) < 2 {
		t.Fatalf("cond block should have ≥2 predecessors (entry + backedge), got %d", len(condBlock.Preds))
	}
}

// --- Expression lowering tests ---

func TestLowerBinaryExpr(t *testing.T) {
	mod := lowerToMIR(t, `const x = 1 + 2;`)
	g := mod.Globals[0]
	bin, ok := g.Init.(*BinExpr)
	if !ok {
		t.Fatalf("expected BinExpr, got %T", g.Init)
	}
	if bin.Op != OpAdd {
		t.Fatalf("expected OpAdd, got %d", bin.Op)
	}
}

func TestLowerArrayExpr(t *testing.T) {
	mod := lowerToMIR(t, `const arr = [1, 2, 3];`)
	g := mod.Globals[0]
	arr, ok := g.Init.(*ArrayExpr)
	if !ok {
		t.Fatalf("expected ArrayExpr, got %T", g.Init)
	}
	if len(arr.Elements) != 3 {
		t.Fatalf("expected 3 elements, got %d", len(arr.Elements))
	}
}

func TestLowerObjectExpr(t *testing.T) {
	mod := lowerToMIR(t, `const obj = { a: 1, b: 2 };`)
	g := mod.Globals[0]
	obj, ok := g.Init.(*ObjectExpr)
	if !ok {
		t.Fatalf("expected ObjectExpr, got %T", g.Init)
	}
	if len(obj.Keys) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(obj.Keys))
	}
}

func TestLowerMemberExpr(t *testing.T) {
	mod := lowerToMIR(t, `const x = obj.foo;`)
	g := mod.Globals[0]
	get, ok := g.Init.(*GetExpr)
	if !ok {
		t.Fatalf("expected GetExpr, got %T", g.Init)
	}
	key, ok := get.Key.(*LitExpr)
	if !ok || key.Value != "foo" {
		t.Fatalf("expected key 'foo', got %v", get.Key)
	}
}
