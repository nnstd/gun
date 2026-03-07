package backend

import (
	"strings"
	"testing"

	"github.com/nnstd/gun/compiler/context"
	"github.com/nnstd/gun/compiler/hir"
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

// lowerTS parses TypeScript, builds HIR, lowers to Go AST, and generates Go source.
func lowerTS(t *testing.T, source string) string {
	t.Helper()
	tree := parseTS(t, source)
	defer tree.Close()

	mod := hir.BuildModule(tree.RootNode(), []byte(source), "main")
	ctx := context.New()
	file := Lower(mod, ctx, "", false)
	out, err := Generate(file)
	if err != nil {
		t.Fatalf("generate failed: %v", err)
	}
	return string(out)
}

func assertContains(t *testing.T, got, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Errorf("output does not contain %q\nGot:\n%s", want, got)
	}
}

func assertNotContains(t *testing.T, got, notWant string) {
	t.Helper()
	if strings.Contains(got, notWant) {
		t.Errorf("output should not contain %q\nGot:\n%s", notWant, got)
	}
}

// --- Unit Tests: HIR node construction → Lower → Generate ---

func TestLowerFuncDecl(t *testing.T) {
	symtab := symbol.NewTable()
	sym := symtab.Define("add", symbol.KindFunction)
	sym.FuncInfo = &symbol.FuncInfo{ParamCount: 2}

	paramA := symtab.Define("a", symbol.KindParameter)
	paramB := symtab.Define("b", symbol.KindParameter)

	mod := &hir.Module{
		Package:     "main",
		SymbolTable: symtab,
		Declarations: []hir.Decl{
			&hir.FuncDecl{
				Symbol: sym,
				Params: []*hir.Param{
					{Symbol: paramA},
					{Symbol: paramB},
				},
				Body: &hir.BlockStmt{
					Stmts: []hir.Stmt{
						&hir.ReturnStmt{
							Value: &hir.BinaryExpr{
								Op:    hir.OpAdd,
								Left:  &hir.Identifier{Sym: paramA, Name: "a"},
								Right: &hir.Identifier{Sym: paramB, Name: "b"},
							},
						},
					},
				},
			},
		},
	}

	file := Lower(mod, context.New(), "", false)
	out, err := Generate(file)
	if err != nil {
		t.Fatalf("generate failed: %v", err)
	}
	s := string(out)
	assertContains(t, s, "var add")
	assertContains(t, s, "jsvalue.NewFunction")
	assertContains(t, s, "jsvalue.Add")
}

func TestLowerVarDecl(t *testing.T) {
	symtab := symbol.NewTable()
	sym := symtab.Define("x", symbol.KindVariable)

	mod := &hir.Module{
		Package:     "main",
		SymbolTable: symtab,
		Declarations: []hir.Decl{
			&hir.VarDecl{
				Kind: hir.VarConst,
				Declarators: []*hir.Declarator{
					{Symbol: sym, Init: &hir.Literal{Kind: hir.LitNumber, Value: "42"}},
				},
			},
		},
	}

	file := Lower(mod, context.New(), "", false)
	out, err := Generate(file)
	if err != nil {
		t.Fatalf("generate failed: %v", err)
	}
	s := string(out)
	assertContains(t, s, "var x")
	assertContains(t, s, "42")
}

func TestLowerMainFunc(t *testing.T) {
	symtab := symbol.NewTable()
	sym := symtab.Define("main", symbol.KindFunction)
	sym.FuncInfo = &symbol.FuncInfo{ParamCount: 0}

	mod := &hir.Module{
		Package:     "main",
		SymbolTable: symtab,
		Declarations: []hir.Decl{
			&hir.FuncDecl{
				Symbol: sym,
				Body:   &hir.BlockStmt{},
			},
		},
	}

	file := Lower(mod, context.New(), "", false)
	out, err := Generate(file)
	if err != nil {
		t.Fatalf("generate failed: %v", err)
	}
	s := string(out)
	// main should stay as a Go func, not be wrapped in jsvalue.NewFunction
	assertContains(t, s, "func main()")
	assertNotContains(t, s, "jsvalue.NewFunction")
}

func TestLowerIfStatement(t *testing.T) {
	symtab := symbol.NewTable()
	mod := &hir.Module{
		Package:     "main",
		SymbolTable: symtab,
		Declarations: []hir.Decl{
			&hir.FuncDecl{
				Symbol: func() *symbol.Symbol {
					s := symtab.Define("main", symbol.KindFunction)
					s.FuncInfo = &symbol.FuncInfo{}
					return s
				}(),
				Body: &hir.BlockStmt{
					Stmts: []hir.Stmt{
						&hir.IfStmt{
							Cond: &hir.Literal{Kind: hir.LitBool, Value: "true"},
							Then: &hir.BlockStmt{
								Stmts: []hir.Stmt{
									&hir.ReturnStmt{},
								},
							},
						},
					},
				},
			},
		},
	}

	file := Lower(mod, context.New(), "", false)
	out, err := Generate(file)
	if err != nil {
		t.Fatalf("generate failed: %v", err)
	}
	s := string(out)
	assertContains(t, s, "if")
}

func TestLowerClassDecl(t *testing.T) {
	symtab := symbol.NewTable()
	classSym := symtab.Define("Dog", symbol.KindClass)

	mod := &hir.Module{
		Package:     "main",
		SymbolTable: symtab,
		Declarations: []hir.Decl{
			&hir.ClassDecl{
				Symbol: classSym,
				Constructor: &hir.ClassConstructor{
					Body: &hir.BlockStmt{},
				},
				Methods: []*hir.ClassMethod{
					{
						Name: "bark",
						Body: &hir.BlockStmt{
							Stmts: []hir.Stmt{
								&hir.ReturnStmt{
									Value: &hir.Literal{Kind: hir.LitString, Value: "woof"},
								},
							},
						},
					},
				},
			},
		},
	}

	file := Lower(mod, context.New(), "", false)
	out, err := Generate(file)
	if err != nil {
		t.Fatalf("generate failed: %v", err)
	}
	s := string(out)
	assertContains(t, s, "Dog")
	assertContains(t, s, "jsvalue.NewClass")
	assertContains(t, s, `"prototype"`)
	assertContains(t, s, `"bark"`)
}

func TestLowerEnumDecl(t *testing.T) {
	symtab := symbol.NewTable()
	sym := symtab.Define("Color", symbol.KindEnum)

	mod := &hir.Module{
		Package:     "main",
		SymbolTable: symtab,
		Declarations: []hir.Decl{
			&hir.EnumDecl{
				Symbol: sym,
				Members: []*hir.EnumMember{
					{Name: "Red"},
					{Name: "Green"},
					{Name: "Blue"},
				},
			},
		},
	}

	file := Lower(mod, context.New(), "", false)
	out, err := Generate(file)
	if err != nil {
		t.Fatalf("generate failed: %v", err)
	}
	s := string(out)
	assertContains(t, s, "Color")
	assertContains(t, s, "iota")
	assertContains(t, s, "ColorRed")
	assertContains(t, s, "ColorGreen")
	assertContains(t, s, "ColorBlue")
}

// --- Round-trip tests: TS → HIR → Lower → Generate ---

func TestRoundTripVariable(t *testing.T) {
	out := lowerTS(t, `const x = 42;`)
	assertContains(t, out, "var x")
	assertContains(t, out, "42")
}

func TestRoundTripFunction(t *testing.T) {
	out := lowerTS(t, `function greet(name) { return name; }`)
	assertContains(t, out, "var greet")
	assertContains(t, out, "jsvalue.NewFunction")
}

func TestRoundTripBinaryExpr(t *testing.T) {
	out := lowerTS(t, `const x = 1 + 2;`)
	assertContains(t, out, "jsvalue.Add")
}

func TestRoundTripStringLiteral(t *testing.T) {
	out := lowerTS(t, `const msg = "hello";`)
	assertContains(t, out, `jsvalue.NewString("hello")`)
}

func TestRoundTripBoolLiteral(t *testing.T) {
	out := lowerTS(t, `const flag = true;`)
	assertContains(t, out, "jsvalue.NewBool(true)")
}

func TestRoundTripIfStatement(t *testing.T) {
	out := lowerTS(t, `function f() { if (true) { return 1; } }`)
	assertContains(t, out, "if")
	assertContains(t, out, "return")
}

func TestRoundTripWhileLoop(t *testing.T) {
	out := lowerTS(t, `function f() { while (true) { break; } }`)
	assertContains(t, out, "for")
	assertContains(t, out, "break")
}

func TestRoundTripForOfLoop(t *testing.T) {
	out := lowerTS(t, `function f() { for (const x of arr) {} }`)
	assertContains(t, out, "range")
	assertContains(t, out, ".Array()")
}

func TestRoundTripTryCatch(t *testing.T) {
	out := lowerTS(t, `function f() { try { throw new Error(); } catch (e) {} }`)
	assertContains(t, out, "defer")
	assertContains(t, out, "recover")
	assertContains(t, out, "panic")
}

func TestRoundTripArrowFunction(t *testing.T) {
	out := lowerTS(t, `const f = (x) => x;`)
	assertContains(t, out, "jsvalue.NewFunction")
}

func TestRoundTripObjectLiteral(t *testing.T) {
	out := lowerTS(t, `const obj = { a: 1, b: 2 };`)
	assertContains(t, out, "jsvalue.ObjectFrom")
}

func TestRoundTripArrayLiteral(t *testing.T) {
	out := lowerTS(t, `const arr = [1, 2, 3];`)
	assertContains(t, out, "jsvalue.NewArray")
}

func TestRoundTripTernary(t *testing.T) {
	out := lowerTS(t, `const x = true ? 1 : 2;`)
	assertContains(t, out, "if")
}

func TestRoundTripExportedFunction(t *testing.T) {
	out := lowerTS(t, `export function greet() { return "hi"; }`)
	assertContains(t, out, "Greet")
	assertContains(t, out, "jsvalue.NewFunction")
}

func TestRoundTripClass(t *testing.T) {
	out := lowerTS(t, `
		class Animal {
			constructor(name) {}
			speak() { return "..."; }
		}
	`)
	assertContains(t, out, "Animal")
	assertContains(t, out, "jsvalue.NewClass")
	assertContains(t, out, `"speak"`)
}

func TestRoundTripEnum(t *testing.T) {
	out := lowerTS(t, `enum Direction { Up, Down, Left, Right }`)
	assertContains(t, out, "Direction")
	assertContains(t, out, "iota")
}

func TestGenerateProducesValidGo(t *testing.T) {
	// Verify the output is valid Go (Generate succeeds)
	snippets := []string{
		`const x = 42;`,
		`function add(a, b) { return a + b; }`,
		`const arr = [1, 2, 3];`,
		`const obj = { key: "value" };`,
		`export function greet() { return "hi"; }`,
		`const f = (x) => x + 1;`,
	}
	for _, ts := range snippets {
		tree := parseTS(t, ts)
		mod := hir.BuildModule(tree.RootNode(), []byte(ts), "main")
		file := Lower(mod, context.New(), "", false)
		_, err := Generate(file)
		tree.Close()
		if err != nil {
			t.Errorf("Generate failed for %q: %v", ts, err)
		}
	}
}
