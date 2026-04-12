package pipeline

import (
	"strings"
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

func compile(t *testing.T, source string, level OptLevel) string {
	t.Helper()
	tree := parseTS(t, source)
	defer tree.Close()
	p := New(level)
	out, err := p.CompileTree(tree.RootNode(), []byte(source), "main", "", false)
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}
	return string(out)
}

func assertContains(t *testing.T, got, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Errorf("output does not contain %q\nGot:\n%s", want, got)
	}
}

// --- Pipeline creation ---

func TestNewO0(t *testing.T) {
	p := New(O0)
	if p.OptLevel != O0 {
		t.Fatal("expected O0")
	}
	if len(p.Passes) != 0 {
		t.Fatalf("O0 should have no passes, got %d", len(p.Passes))
	}
}

func TestNewO1(t *testing.T) {
	p := New(O1)
	if len(p.Passes) != 1 {
		t.Fatalf("O1 should have 1 pass, got %d", len(p.Passes))
	}
	if p.Passes[0].Name() != "const-fold" {
		t.Fatalf("expected const-fold, got %q", p.Passes[0].Name())
	}
}

func TestNewO2(t *testing.T) {
	p := New(O2)
	if len(p.Passes) != 2 {
		t.Fatalf("O2 should have 2 passes, got %d", len(p.Passes))
	}
}

// --- Full pipeline tests ---

func TestCompileO0(t *testing.T) {
	out := compile(t, `const x = 42;`, O0)
	assertContains(t, out, "package main")
	assertContains(t, out, "42")
}

func TestCompileO1(t *testing.T) {
	out := compile(t, `function f() { return 1 + 2; }`, O1)
	assertContains(t, out, "package main")
	assertContains(t, out, "jsvalue.NewFunction")
}

func TestCompileO2(t *testing.T) {
	out := compile(t, `const x = "hello";`, O2)
	assertContains(t, out, "package main")
	assertContains(t, out, "hello")
}

func TestCompileExportedFunction(t *testing.T) {
	out := compile(t, `export function greet() { return "hi"; }`, O0)
	assertContains(t, out, "Greet")
}

func TestCompileClass(t *testing.T) {
	out := compile(t, `class Dog { bark() { return "woof"; } }`, O0)
	assertContains(t, out, "Dog")
	assertContains(t, out, "jsvalue.NewClass")
}

// --- Hook tests ---

func TestHooks(t *testing.T) {
	tree := parseTS(t, `const x = 42;`)
	defer tree.Close()

	p := New(O2)
	hirCalled := false
	mirCalled := false
	ssaCalled := false

	p.OnHIR = func(mod *hir.Module) { hirCalled = true }
	p.OnMIR = func(mod *mir.Module) { mirCalled = true }
	p.OnSSA = func(mod *ssa.Module) { ssaCalled = true }

	_, err := p.CompileTree(tree.RootNode(), []byte(`const x = 42;`), "main", "", false)
	if err != nil {
		t.Fatal(err)
	}

	if !hirCalled {
		t.Error("OnHIR hook not called")
	}
	if !mirCalled {
		t.Error("OnMIR hook not called")
	}
	if !ssaCalled {
		t.Error("OnSSA hook not called")
	}
}

// --- CompileHIR tests ---

func TestCompileHIR(t *testing.T) {
	tree := parseTS(t, `const x = 42;`)
	defer tree.Close()

	hirMod := hir.BuildModule(tree.RootNode(), []byte(`const x = 42;`), "main")
	p := New(O0)
	out, err := p.CompileHIR(hirMod, "", false)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	assertContains(t, s, "package main")
	assertContains(t, s, "42")
}

func TestCompileTreeSupportsAsyncFunctionDeclarationPhase1(t *testing.T) {
	tree := parseTS(t, `async function load() { return await fetch(); }`)
	defer tree.Close()

	p := New(O0)
	out, err := p.CompileTree(tree.RootNode(), []byte(`async function load() { return await fetch(); }`), "main", "", false)
	if err != nil {
		t.Fatalf("expected async compile success, got %v", err)
	}
	assertContains(t, string(out), "promise.Promise.Call")
}

func TestCompileHIRSupportsAsyncFunctionDeclarationPhase1(t *testing.T) {
	tree := parseTS(t, `async function load() { return await fetch(); }`)
	defer tree.Close()

	hirMod := hir.BuildModule(tree.RootNode(), []byte(`async function load() { return await fetch(); }`), "main")
	p := New(O0)
	out, err := p.CompileHIR(hirMod, "", false)
	if err != nil {
		t.Fatalf("expected async compile success, got %v", err)
	}
	assertContains(t, string(out), "promise.Promise.Call")
}

func TestCompilePackageSupportsAsyncFunctionDeclarationPhase1(t *testing.T) {
	p := New(O0)
	out, err := p.CompilePackage(map[string][]byte{
		"entry.ts": []byte(`import { load } from "./util"; export function main() { return load(); }`),
		"util.ts":  []byte(`export async function load() { return await Promise.resolve(1); }`),
	}, "main", "", "entry.ts")
	if err != nil {
		t.Fatalf("expected async package compile success, got %v", err)
	}
	assertContains(t, string(out["util.ts"]), "promise.Promise.Call")
}

func TestCompileTreeRejectsAwaitInParameterDefaultPhase0(t *testing.T) {
	tree := parseTS(t, `function load(value = await fetch()) { return value; }`)
	defer tree.Close()

	p := New(O0)
	_, err := p.CompileTree(tree.RootNode(), []byte(`function load(value = await fetch()) { return value; }`), "main", "", false)
	if err == nil {
		t.Fatal("expected async diagnostic error")
	}
	assertContains(t, err.Error(), "await expressions are only supported inside async function declarations")
}

func TestCompileTreeSupportsAsyncArrowPhase1(t *testing.T) {
	tree := parseTS(t, `const load = async () => { await Promise.resolve(1); return 2; }`)
	defer tree.Close()

	p := New(O0)
	out, err := p.CompileTree(tree.RootNode(), []byte(`const load = async () => { await Promise.resolve(1); return 2; }`), "main", "", false)
	if err != nil {
		t.Fatalf("expected async arrow compile success, got %v", err)
	}
	assertContains(t, string(out), "promise.Promise.Call")
}

func TestCompileTreeSupportsAsyncFunctionExpressionPhase1(t *testing.T) {
	tree := parseTS(t, `const load = async function() { await Promise.resolve(1); return 2; }`)
	defer tree.Close()

	p := New(O0)
	out, err := p.CompileTree(tree.RootNode(), []byte(`const load = async function() { await Promise.resolve(1); return 2; }`), "main", "", false)
	if err != nil {
		t.Fatalf("expected async function expression compile success, got %v", err)
	}
	assertContains(t, string(out), "promise.Promise.Call")
}

// --- Various TypeScript snippets ---

func TestCompileVariousSnippets(t *testing.T) {
	snippets := []struct {
		name string
		ts   string
	}{
		{"variable", `const x = 42;`},
		{"function", `function add(a, b) { return a + b; }`},
		{"arrow", `const f = (x) => x + 1;`},
		{"class", `class Dog { bark() { return "woof"; } }`},
		{"if-else", `function f(x) { if (x) { return 1; } else { return 2; } }`},
		{"while", `function f() { while (true) { break; } }`},
		{"array", `const arr = [1, 2, 3];`},
		{"object", `const obj = { a: 1, b: 2 };`},
		{"enum", `enum Color { Red, Green, Blue }`},
		{"export", `export function greet() { return "hi"; }`},
	}

	for _, s := range snippets {
		t.Run(s.name, func(t *testing.T) {
			for _, level := range []OptLevel{O0, O1, O2} {
				out := compile(t, s.ts, level)
				if !strings.Contains(out, "package main") {
					t.Errorf("O%d: missing package declaration for %q", level, s.name)
				}
			}
		})
	}
}
