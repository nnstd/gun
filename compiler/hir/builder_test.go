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
