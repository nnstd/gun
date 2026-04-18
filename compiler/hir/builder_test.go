package hir

import (
	"strings"
	"testing"

	"github.com/nnstd/gun/compiler/symbol"
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

func buildHIR(t *testing.T, source string) *Module {
	t.Helper()
	tree := parseTS(t, source)
	defer tree.Close()
	return BuildModule(tree.RootNode(), []byte(source), "main")
}

func buildHIRWithPath(t *testing.T, source, sourcePath string) *Module {
	t.Helper()
	tree := parseTS(t, source)
	defer tree.Close()
	return BuildModuleWithPath(tree.RootNode(), []byte(source), "main", sourcePath)
}

func hirString(t *testing.T, source string) string {
	t.Helper()
	mod := buildHIR(t, source)
	return Sprint(mod)
}

func assertHIRContains(t *testing.T, hir, want string) {
	t.Helper()
	if !strings.Contains(hir, want) {
		t.Errorf("HIR does not contain %q\nGot:\n%s", want, hir)
	}
}

func assertHIRNotContains(t *testing.T, hir, notWant string) {
	t.Helper()
	if strings.Contains(hir, notWant) {
		t.Errorf("HIR should not contain %q\nGot:\n%s", notWant, hir)
	}
}

// --- Declaration Tests ---

func TestFunctionDeclaration(t *testing.T) {
	out := hirString(t, `function add(x, y) { return x + y; }`)
	assertHIRContains(t, out, "Function add#")
	assertHIRContains(t, out, "Return")
}

func TestAsyncFunctionDeclaration(t *testing.T) {
	out := hirString(t, `async function fetchData() { return 42; }`)
	assertHIRContains(t, out, "async Function fetchData#")
}

func TestVariableDeclaration(t *testing.T) {
	out := hirString(t, `const x = 42;`)
	assertHIRContains(t, out, "const x#")
	assertHIRContains(t, out, "42")
}

func TestLetDeclaration(t *testing.T) {
	out := hirString(t, `let y = "hello";`)
	assertHIRContains(t, out, "let y#")
}

func TestVarDeclaration(t *testing.T) {
	out := hirString(t, `var z = true;`)
	assertHIRContains(t, out, "var z#")
}

func TestClassDeclaration(t *testing.T) {
	out := hirString(t, `
		class Dog {
			constructor(name) { this.name = name; }
			bark() { return "woof"; }
		}
	`)
	assertHIRContains(t, out, "Class Dog#")
	assertHIRContains(t, out, "Constructor")
	assertHIRContains(t, out, "Method bark")
}

func TestClassExtends(t *testing.T) {
	out := hirString(t, `
		class Animal {}
		class Dog extends Animal {}
	`)
	assertHIRContains(t, out, "Class Dog#")
	assertHIRContains(t, out, "extends")
}

func TestEnumDeclaration(t *testing.T) {
	out := hirString(t, `enum Color { Red, Green, Blue }`)
	assertHIRContains(t, out, "Enum Color#")
	assertHIRContains(t, out, "Red")
	assertHIRContains(t, out, "Green")
	assertHIRContains(t, out, "Blue")
}

func TestInterfaceDeclaration(t *testing.T) {
	out := hirString(t, `
		interface Shape {
			area(): number;
			name: string;
		}
	`)
	assertHIRContains(t, out, "Interface Shape#")
	assertHIRContains(t, out, "method area")
	assertHIRContains(t, out, "prop name")
}

func TestTypeAlias(t *testing.T) {
	out := hirString(t, `type ID = string | number;`)
	assertHIRContains(t, out, "TypeAlias ID#")
}

func TestFunctionDeclarationSpan(t *testing.T) {
	mod := buildHIRWithPath(t, `function add(x, y) { return x + y }`, "/tmp/example.ts")
	fd, ok := mod.Declarations[0].(*FuncDecl)
	if !ok {
		t.Fatalf("decl = %T, want *FuncDecl", mod.Declarations[0])
	}
	if fd.Span == nil || fd.Span.StartLine != 1 || fd.Span.EndByte <= fd.Span.StartByte {
		t.Fatalf("unexpected function span: %#v", fd.Span)
	}
	if mod.SourcePath != "/tmp/example.ts" {
		t.Fatalf("module source path = %q", mod.SourcePath)
	}
}

func TestArrowFunctionSpan(t *testing.T) {
	mod := buildHIRWithPath(t, `const fn = (value) => value + 1`, "/tmp/arrow.ts")
	vd, ok := mod.Declarations[0].(*VarDecl)
	if !ok {
		t.Fatalf("decl = %T, want *VarDecl", mod.Declarations[0])
	}
	af, ok := vd.Declarators[0].Init.(*ArrowFunc)
	if !ok {
		t.Fatalf("init = %T, want *ArrowFunc", vd.Declarators[0].Init)
	}
	if af.Span == nil || af.Span.StartLine != 1 || af.Span.EndByte <= af.Span.StartByte {
		t.Fatalf("unexpected arrow span: %#v", af.Span)
	}
}

func TestClassMethodSpan(t *testing.T) {
	mod := buildHIRWithPath(t, `class Box { open() { return 1 } }`, "/tmp/class.ts")
	cd, ok := mod.Declarations[0].(*ClassDecl)
	if !ok {
		t.Fatalf("decl = %T, want *ClassDecl", mod.Declarations[0])
	}
	if len(cd.Methods) != 1 {
		t.Fatalf("expected 1 method, got %d", len(cd.Methods))
	}
	if cd.Methods[0].Span == nil || cd.Methods[0].Span.StartLine != 1 || cd.Methods[0].Span.EndByte <= cd.Methods[0].Span.StartByte {
		t.Fatalf("unexpected method span: %#v", cd.Methods[0].Span)
	}
}

// --- Import Tests ---

func TestImportDefault(t *testing.T) {
	out := hirString(t, `import fs from "fs";`)
	assertHIRContains(t, out, `Import { fs } from "fs"`)
}

func TestImportNamed(t *testing.T) {
	out := hirString(t, `import { readFile, writeFile } from "fs";`)
	assertHIRContains(t, out, "readFile")
	assertHIRContains(t, out, "writeFile")
	assertHIRContains(t, out, `from "fs"`)
}

func TestImportNamespace(t *testing.T) {
	out := hirString(t, `import * as path from "path";`)
	assertHIRContains(t, out, "* as path")
}

func TestImportAlias(t *testing.T) {
	out := hirString(t, `import { readFile as read } from "fs";`)
	assertHIRContains(t, out, "readFile as read")
}

// --- Export Tests ---

func TestExportFunction(t *testing.T) {
	out := hirString(t, `export function greet() { return "hi"; }`)
	assertHIRContains(t, out, "Export")
	assertHIRContains(t, out, "Function greet#")
}

func TestExportDefault(t *testing.T) {
	out := hirString(t, `export default 42;`)
	assertHIRContains(t, out, "ExportDefault")
}

func TestExportClause(t *testing.T) {
	out := hirString(t, `
		const a = 1;
		const b = 2;
		export { a, b };
	`)
	assertHIRContains(t, out, "Export { a, b }")
}

// --- Statement Tests ---

func TestIfStatement(t *testing.T) {
	out := hirString(t, `function f() { if (x > 0) { return 1; } else { return 0; } }`)
	assertHIRContains(t, out, "If")
	assertHIRContains(t, out, "Then:")
	assertHIRContains(t, out, "Else:")
}

func TestForStatement(t *testing.T) {
	out := hirString(t, `function f() { for (let i = 0; i < 10; i++) { console.log(i); } }`)
	assertHIRContains(t, out, "For cond=")
}

func TestWhileStatement(t *testing.T) {
	out := hirString(t, `function f() { while (true) { break; } }`)
	assertHIRContains(t, out, "While")
	assertHIRContains(t, out, "Break")
}

func TestDoWhileStatement(t *testing.T) {
	out := hirString(t, `function f() { do { x++; } while (x < 10); }`)
	assertHIRContains(t, out, "DoWhile")
}

func TestSwitchStatement(t *testing.T) {
	out := hirString(t, `
		function f(x) {
			switch (x) {
				case 1: return "one";
				case 2: return "two";
				default: return "other";
			}
		}
	`)
	assertHIRContains(t, out, "Switch")
	assertHIRContains(t, out, "Case")
	assertHIRContains(t, out, "Default:")
}

func TestTryCatch(t *testing.T) {
	out := hirString(t, `
		function f() {
			try {
				throw new Error("fail");
			} catch (e) {
				console.log(e);
			} finally {
				cleanup();
			}
		}
	`)
	assertHIRContains(t, out, "Try")
	assertHIRContains(t, out, "Catch")
	assertHIRContains(t, out, "Finally")
	assertHIRContains(t, out, "Throw")
}

func TestReturnStatement(t *testing.T) {
	out := hirString(t, `function f() { return 42; }`)
	assertHIRContains(t, out, "Return 42")
}

func TestForOfStatement(t *testing.T) {
	out := hirString(t, `function f() { for (const x of arr) { console.log(x); } }`)
	assertHIRContains(t, out, "ForOf")
}

func TestForInStatement(t *testing.T) {
	out := hirString(t, `function f() { for (const k in obj) { console.log(k); } }`)
	assertHIRContains(t, out, "ForIn")
}

// --- Expression Tests ---

func TestBinaryExpression(t *testing.T) {
	out := hirString(t, `const x = 1 + 2;`)
	assertHIRContains(t, out, "+")
}

func TestUnaryExpression(t *testing.T) {
	out := hirString(t, `const x = !true;`)
	assertHIRContains(t, out, "!")
}

func TestTernaryExpression(t *testing.T) {
	out := hirString(t, `const x = a ? 1 : 2;`)
	assertHIRContains(t, out, "?")
}

func TestCallExpression(t *testing.T) {
	out := hirString(t, `function f() { console.log("hello"); }`)
	assertHIRContains(t, out, "console")
	assertHIRContains(t, out, "log")
}

func TestNewExpression(t *testing.T) {
	out := hirString(t, `const x = new Map();`)
	assertHIRContains(t, out, "new")
}

func TestArrowFunction(t *testing.T) {
	out := hirString(t, `const f = (x) => x + 1;`)
	assertHIRContains(t, out, "=>")
}

func TestObjectLiteral(t *testing.T) {
	out := hirString(t, `const obj = { a: 1, b: 2 };`)
	assertHIRContains(t, out, "props")
}

func TestArrayLiteral(t *testing.T) {
	out := hirString(t, `const arr = [1, 2, 3];`)
	assertHIRContains(t, out, "elems")
}

func TestTemplateLiteral(t *testing.T) {
	out := hirString(t, "const s = `hello ${name}`;")
	assertHIRContains(t, out, "`...`")
}

func TestSpreadExpression(t *testing.T) {
	mod := buildHIR(t, `const arr = [...other, 1];`)
	if len(mod.Declarations) == 0 {
		t.Fatal("expected declarations")
	}
	vd, ok := mod.Declarations[0].(*VarDecl)
	if !ok {
		t.Fatalf("expected VarDecl, got %T", mod.Declarations[0])
	}
	al, ok := vd.Declarators[0].Init.(*ArrayLiteral)
	if !ok {
		t.Fatalf("expected ArrayLiteral, got %T", vd.Declarators[0].Init)
	}
	if len(al.Elements) != 2 {
		t.Fatalf("expected 2 elements, got %d", len(al.Elements))
	}
	_, isSpread := al.Elements[0].(*SpreadExpr)
	if !isSpread {
		t.Fatalf("expected first element to be SpreadExpr, got %T", al.Elements[0])
	}
}

func TestMemberExpression(t *testing.T) {
	out := hirString(t, `const x = obj.foo;`)
	assertHIRContains(t, out, ".foo")
}

func TestComputedMember(t *testing.T) {
	out := hirString(t, `const x = arr[0];`)
	assertHIRContains(t, out, "[")
}

func TestThisExpression(t *testing.T) {
	out := hirString(t, `
		class Foo {
			method() { return this; }
		}
	`)
	assertHIRContains(t, out, "this")
}

// --- Pattern Tests ---

func TestObjectDestructuring(t *testing.T) {
	mod := buildHIR(t, `const { a, b } = obj;`)
	if len(mod.Declarations) == 0 {
		t.Fatal("expected at least one declaration")
	}
	// The declaration should have a pattern
	vd, ok := mod.Declarations[0].(*VarDecl)
	if !ok {
		t.Fatalf("expected VarDecl, got %T", mod.Declarations[0])
	}
	if len(vd.Declarators) == 0 {
		t.Fatal("expected declarators")
	}
	if vd.Declarators[0].Pattern == nil {
		t.Fatal("expected destructuring pattern")
	}
	pat, ok := vd.Declarators[0].Pattern.(*ObjectPattern)
	if !ok {
		t.Fatalf("expected ObjectPattern, got %T", vd.Declarators[0].Pattern)
	}
	if len(pat.Properties) != 2 {
		t.Fatalf("expected 2 properties, got %d", len(pat.Properties))
	}
}

func TestArrayDestructuring(t *testing.T) {
	mod := buildHIR(t, `const [x, y] = arr;`)
	if len(mod.Declarations) == 0 {
		t.Fatal("expected at least one declaration")
	}
	vd, ok := mod.Declarations[0].(*VarDecl)
	if !ok {
		t.Fatalf("expected VarDecl, got %T", mod.Declarations[0])
	}
	if len(vd.Declarators) == 0 {
		t.Fatal("expected declarators")
	}
	if vd.Declarators[0].Pattern == nil {
		t.Fatal("expected destructuring pattern")
	}
	pat, ok := vd.Declarators[0].Pattern.(*ArrayPattern)
	if !ok {
		t.Fatalf("expected ArrayPattern, got %T", vd.Declarators[0].Pattern)
	}
	if len(pat.Elements) != 2 {
		t.Fatalf("expected 2 elements, got %d", len(pat.Elements))
	}
}

// --- Symbol Table Tests ---

func TestSymbolsAreUnique(t *testing.T) {
	mod := buildHIR(t, `
		const x = 1;
		const y = 2;
		function add(a, b) { return a + b; }
	`)

	ids := make(map[int]bool)
	mod.SymbolTable.ForEach(func(sym *symbol.Symbol) {
		if ids[int(sym.ID)] {
			t.Fatalf("duplicate symbol ID: %d", sym.ID)
		}
		ids[int(sym.ID)] = true
	})
}

func TestFunctionParamCountTracked(t *testing.T) {
	mod := buildHIR(t, `function greet(name, greeting) { return greeting + name; }`)
	var found bool
	for _, d := range mod.Declarations {
		fd, ok := d.(*FuncDecl)
		if !ok {
			continue
		}
		if fd.Symbol.OriginalName == "greet" {
			found = true
			if fd.Symbol.FuncInfo == nil {
				t.Fatal("expected FuncInfo")
			}
			if fd.Symbol.FuncInfo.ParamCount != 2 {
				t.Fatalf("expected 2 params, got %d", fd.Symbol.FuncInfo.ParamCount)
			}
		}
	}
	if !found {
		t.Fatal("function 'greet' not found")
	}
}

// --- Printer Tests ---

func TestPrinterModule(t *testing.T) {
	out := hirString(t, `
		import fs from "fs";
		const data = fs.readFileSync("file.txt");
		export function process(x) { return x; }
	`)
	assertHIRContains(t, out, `Module "main"`)
	assertHIRContains(t, out, "Import")
	assertHIRContains(t, out, "Export")
	assertHIRContains(t, out, "Function process#")
}

// --- Optional chaining ---

func TestOptionalChainingFlag(t *testing.T) {
	mod := buildHIR(t, `function f() { return a?.b; }`)
	// Find the MemberExpr with Optional=true
	found := false
	for _, d := range mod.Declarations {
		fd, ok := d.(*FuncDecl)
		if !ok || fd.Body == nil {
			continue
		}
		for _, s := range fd.Body.Stmts {
			rs, ok := s.(*ReturnStmt)
			if !ok || rs.Value == nil {
				continue
			}
			if me, ok := rs.Value.(*MemberExpr); ok {
				if me.Optional {
					found = true
				}
			}
		}
	}
	if !found {
		t.Fatal("expected MemberExpr with Optional=true for a?.b")
	}
}

func TestNonOptionalMemberFlag(t *testing.T) {
	mod := buildHIR(t, `function f() { return a.b; }`)
	for _, d := range mod.Declarations {
		fd, ok := d.(*FuncDecl)
		if !ok || fd.Body == nil {
			continue
		}
		for _, s := range fd.Body.Stmts {
			rs, ok := s.(*ReturnStmt)
			if !ok || rs.Value == nil {
				continue
			}
			if me, ok := rs.Value.(*MemberExpr); ok {
				if me.Optional {
					t.Fatal("a.b should NOT have Optional=true")
				}
			}
		}
	}
}

// --- For-of destructuring ---

func TestForOfArrayDestructuring(t *testing.T) {
	mod := buildHIR(t, `function f() { for (const [k, v] of entries) {} }`)
	for _, d := range mod.Declarations {
		fd, ok := d.(*FuncDecl)
		if !ok || fd.Body == nil {
			continue
		}
		for _, s := range fd.Body.Stmts {
			fof, ok := s.(*ForOfStmt)
			if !ok {
				continue
			}
			if fof.Pattern == nil {
				t.Fatal("expected Pattern on ForOfStmt for [k, v] destructuring")
			}
			ap, ok := fof.Pattern.(*ArrayPattern)
			if !ok {
				t.Fatalf("expected ArrayPattern, got %T", fof.Pattern)
			}
			if len(ap.Elements) != 2 {
				t.Fatalf("expected 2 elements, got %d", len(ap.Elements))
			}
			return
		}
	}
	t.Fatal("did not find ForOfStmt")
}

func TestForOfObjectDestructuring(t *testing.T) {
	mod := buildHIR(t, `function f() { for (const { name, age } of people) {} }`)
	for _, d := range mod.Declarations {
		fd, ok := d.(*FuncDecl)
		if !ok || fd.Body == nil {
			continue
		}
		for _, s := range fd.Body.Stmts {
			fof, ok := s.(*ForOfStmt)
			if !ok {
				continue
			}
			if fof.Pattern == nil {
				t.Fatal("expected Pattern on ForOfStmt for { name, age } destructuring")
			}
			op, ok := fof.Pattern.(*ObjectPattern)
			if !ok {
				t.Fatalf("expected ObjectPattern, got %T", fof.Pattern)
			}
			if len(op.Properties) != 2 {
				t.Fatalf("expected 2 properties, got %d", len(op.Properties))
			}
			return
		}
	}
	t.Fatal("did not find ForOfStmt")
}

// --- Top-level statement ---

func TestTopLevelStatement(t *testing.T) {
	mod := buildHIR(t, `console.log("hello");`)
	found := false
	for _, d := range mod.Declarations {
		if _, ok := d.(*TopLevelStmt); ok {
			found = true
		}
	}
	if !found {
		t.Fatal("expected TopLevelStmt for console.log at top level")
	}
}

func TestPrinterNested(t *testing.T) {
	out := hirString(t, `
		function outer() {
			if (true) {
				while (x) {
					return 0;
				}
			}
		}
	`)
	// Just verify it doesn't crash and contains structure
	assertHIRContains(t, out, "Function outer#")
	assertHIRContains(t, out, "If")
	assertHIRContains(t, out, "While")
	assertHIRContains(t, out, "Return")
}

func TestTopLevelAwaitDetection(t *testing.T) {
	mod := buildHIR(t, `const x = await Promise.resolve(42);`)
	if !mod.HasTopLevelAwait {
		t.Error("HasTopLevelAwait should be true for top-level await")
	}
}

func TestTopLevelAwaitExpression(t *testing.T) {
	mod := buildHIR(t, `await Promise.resolve(1);`)
	if !mod.HasTopLevelAwait {
		t.Error("HasTopLevelAwait should be true for top-level await expression")
	}
}

func TestNoTopLevelAwait(t *testing.T) {
	mod := buildHIR(t, `const x = 42; console.log(x);`)
	if mod.HasTopLevelAwait {
		t.Error("HasTopLevelAwait should be false when no await at top level")
	}
}

func TestAwaitInsideAsyncFunction(t *testing.T) {
	mod := buildHIR(t, `
		async function foo() {
			const x = await Promise.resolve(42);
			return x;
		}
		const y = 1;
	`)
	if mod.HasTopLevelAwait {
		t.Error("HasTopLevelAwait should be false — await is inside async function, not at top level")
	}
}
