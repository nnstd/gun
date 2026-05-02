package pipeline

import (
	"go/ast"
	"strings"
	"testing"

	"github.com/nnstd/gun/compiler/context"
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

func compile(t *testing.T, source string, level context.OptLevel) string {
	t.Helper()
	tree := parseTS(t, source)
	defer tree.Close()
	ctx := context.New()
	registerPipelineTestDefaults(ctx)
	p := NewWithContext(level, ctx)
	out, err := p.CompileTree(tree.RootNode(), []byte(source), "main", "", false)
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}
	return string(out)
}

func registerPipelineTestDefaults(ctx *context.TranspilerContext) {
	for _, name := range []string{"globalThis", "global"} {
		ctx.RegisterIdentifier(&context.IdentifierMapping{
			Name: name,
			Transform: func(imp context.Imports) ast.Expr {
				imp.AddAliasedImport("github.com/nnstd/gun/runtime/jscontext", "jscontext")
				imp.SetNeedsGlobalSync()
				return &ast.CallExpr{
					Fun: &ast.SelectorExpr{
						X: &ast.CallExpr{
							Fun: &ast.SelectorExpr{
								X:   &ast.Ident{Name: "jscontext"},
								Sel: &ast.Ident{Name: "Default"},
							},
						},
						Sel: &ast.Ident{Name: "Global"},
					},
				}
			},
		})
	}
}

func assertContains(t *testing.T, got, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Errorf("output does not contain %q\nGot:\n%s", want, got)
	}
}

// --- Pipeline creation ---

func TestNewO0(t *testing.T) {
	p := New(context.O0)
	if p.OptLevel != context.O0 {
		t.Fatal("expected context.O0")
	}
	if len(p.Passes) != 0 {
		t.Fatalf("context.O0 should have no passes, got %d", len(p.Passes))
	}
}

func TestNewO1(t *testing.T) {
	p := New(context.O1)
	if len(p.Passes) != 1 {
		t.Fatalf("context.O1 should have 1 pass, got %d", len(p.Passes))
	}
	if p.Passes[0].Name() != "const-fold" {
		t.Fatalf("expected const-fold, got %q", p.Passes[0].Name())
	}
}

func TestNewO2(t *testing.T) {
	p := New(context.O2)
	if len(p.Passes) != 2 {
		t.Fatalf("O2 should have 2 passes, got %d", len(p.Passes))
	}
}

// --- Full pipeline tests ---

func TestCompileO0(t *testing.T) {
	out := compile(t, `const x = 42;`, context.O0)
	assertContains(t, out, "package main")
	assertContains(t, out, "42")
}

func TestCompileO1(t *testing.T) {
	out := compile(t, `function f() { return 1 + 2; }`, context.O1)
	assertContains(t, out, "package main")
	assertContains(t, out, "jsvalue.NewFunction")
}

func TestCompileO2(t *testing.T) {
	out := compile(t, `const x = "hello";`, context.O2)
	assertContains(t, out, "package main")
	assertContains(t, out, "hello")
}

func TestCompileExportedFunction(t *testing.T) {
	out := compile(t, `export function greet() { return "hi"; }`, context.O0)
	assertContains(t, out, "Greet")
}

func TestCompileClass(t *testing.T) {
	out := compile(t, `class Dog { bark() { return "woof"; } }`, context.O0)
	assertContains(t, out, "Dog")
	assertContains(t, out, "jsvalue.NewClass")
}

// --- Hook tests ---

func TestHooks(t *testing.T) {
	tree := parseTS(t, `const x = 42;`)
	defer tree.Close()

	p := New(context.O2)
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
	p := New(context.O0)
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

	p := New(context.O0)
	out, err := p.CompileTree(tree.RootNode(), []byte(`async function load() { return await fetch(); }`), "main", "", false)
	if err != nil {
		t.Fatalf("expected async compile success, got %v", err)
	}
	assertContains(t, string(out), "_gunPromise.Promise.Call")
}

func TestCompileHIRSupportsAsyncFunctionDeclarationPhase1(t *testing.T) {
	tree := parseTS(t, `async function load() { return await fetch(); }`)
	defer tree.Close()

	hirMod := hir.BuildModule(tree.RootNode(), []byte(`async function load() { return await fetch(); }`), "main")
	p := New(context.O0)
	out, err := p.CompileHIR(hirMod, "", false)
	if err != nil {
		t.Fatalf("expected async compile success, got %v", err)
	}
	assertContains(t, string(out), "_gunPromise.Promise.Call")
}

func TestCompilePackageSupportsAsyncFunctionDeclarationPhase1(t *testing.T) {
	p := New(context.O0)
	out, err := p.CompilePackage(map[string][]byte{
		"entry.ts": []byte(`import { load } from "./util"; export function main() { return load(); }`),
		"util.ts":  []byte(`export async function load() { return await Promise.resolve(1); }`),
	}, "main", "", "entry.ts")
	if err != nil {
		t.Fatalf("expected async package compile success, got %v", err)
	}
	assertContains(t, string(out["util.ts"]), "_gunPromise.Promise.Call")
}

func TestCompileTreeRejectsAwaitInParameterDefaultPhase0(t *testing.T) {
	tree := parseTS(t, `function load(value = await fetch()) { return value; }`)
	defer tree.Close()

	p := New(context.O0)
	_, err := p.CompileTree(tree.RootNode(), []byte(`function load(value = await fetch()) { return value; }`), "main", "", false)
	if err == nil {
		t.Fatal("expected async diagnostic error")
	}
	assertContains(t, err.Error(), "await expressions are only supported inside async function declarations")
}

func TestCompileTreeSupportsAsyncArrowPhase1(t *testing.T) {
	tree := parseTS(t, `const load = async () => { await Promise.resolve(1); return 2; }`)
	defer tree.Close()

	p := New(context.O0)
	out, err := p.CompileTree(tree.RootNode(), []byte(`const load = async () => { await Promise.resolve(1); return 2; }`), "main", "", false)
	if err != nil {
		t.Fatalf("expected async arrow compile success, got %v", err)
	}
	assertContains(t, string(out), "_gunPromise.Promise.Call")
}

func TestCompilePackageThreadsOptLevelToJSONModule(t *testing.T) {
	var json strings.Builder
	json.WriteString(`{"items":[`)
	for i := 0; i < 32*1024; i++ {
		if i > 0 {
			json.WriteByte(',')
		}
		json.WriteString(`{"name":"stone","id":1}`)
	}
	json.WriteString(`]}`)

	files := map[string][]byte{
		"entry.ts":  []byte(`import data from "./data.json"; console.log(data.items.length);`),
		"data.json": []byte(json.String()),
	}
	o1, err := New(context.O1).CompilePackage(files, "main", "", "entry.ts")
	if err != nil {
		t.Fatal(err)
	}
	if source := string(packageOutput(t, o1, "data.json", "data")); !strings.Contains(source, "module.DataToJSValue(data)") || strings.Contains(source, "type jsonRoot struct") {
		t.Fatalf("O1 JSON module should use untyped runtime parse:\n%s", source)
	}

	o2, err := New(context.O2).CompilePackage(files, "main", "", "entry.ts")
	if err != nil {
		t.Fatal(err)
	}
	if source := string(packageOutput(t, o2, "data.json", "data")); !strings.Contains(source, "type jsonRoot struct") || strings.Contains(source, "module.DataToJSValue(data)") {
		t.Fatalf("O2 JSON module should use typed schema conversion:\n%s", source)
	}
}

func packageOutput(t *testing.T, outputs map[string][]byte, names ...string) []byte {
	t.Helper()
	for _, name := range names {
		if out := outputs[name]; len(out) > 0 {
			return out
		}
	}
	keys := make([]string, 0, len(outputs))
	for key := range outputs {
		keys = append(keys, key)
	}
	t.Fatalf("missing package output %v; got keys %v", names, keys)
	return nil
}

func TestCompileTreeSupportsAsyncFunctionExpressionPhase1(t *testing.T) {
	tree := parseTS(t, `const load = async function() { await Promise.resolve(1); return 2; }`)
	defer tree.Close()

	p := New(context.O0)
	out, err := p.CompileTree(tree.RootNode(), []byte(`const load = async function() { await Promise.resolve(1); return 2; }`), "main", "", false)
	if err != nil {
		t.Fatalf("expected async function expression compile success, got %v", err)
	}
	assertContains(t, string(out), "_gunPromise.Promise.Call")
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
			for _, level := range []context.OptLevel{context.O0, context.O1, context.O2} {
				out := compile(t, s.ts, level)
				if !strings.Contains(out, "package main") {
					t.Errorf("O%d: missing package declaration for %q", level, s.name)
				}
			}
		})
	}
}

// --- JSContext pipeline tests ---

func TestPipelineGlobalThisAndGlobalSameObject(t *testing.T) {
	out := compile(t, "var x = globalThis === global;", context.O0)
	assertContains(t, out, "jscontext")
}

func TestPipelineVarOnGlobalThis(t *testing.T) {
	out := compile(t, "var x = 1; var g = globalThis.x;", context.O0)
	assertContains(t, out, `.Set("x",`)
}
