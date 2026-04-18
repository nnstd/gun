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
	file := Lower(mod, ctx, "", false, context.O0)
	out, err := Generate(file)
	if err != nil {
		t.Fatalf("generate failed: %v", err)
	}
	return string(out)
}

func lowerTSWithPath(t *testing.T, source, sourcePath string) string {
	t.Helper()
	tree := parseTS(t, source)
	defer tree.Close()

	mod := hir.BuildModuleWithPath(tree.RootNode(), []byte(source), "main", sourcePath)
	ctx := context.New()
	file := Lower(mod, ctx, "", false, context.O0)
	out, err := GenerateWithSource(file, mod.SourcePath, mod.SourceSize)
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

	file := Lower(mod, context.New(), "", false, context.O0)
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

	file := Lower(mod, context.New(), "", false, context.O0)
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

	file := Lower(mod, context.New(), "", false, context.O0)
	out, err := Generate(file)
	if err != nil {
		t.Fatalf("generate failed: %v", err)
	}
	s := string(out)
	// main should stay as a Go func, not be wrapped in jsvalue.NewFunction
	assertContains(t, s, "func main()")
	assertNotContains(t, s, "jsvalue.NewFunction")
}

func TestGenerateWithSourceEmitsLineDirectiveForFunction(t *testing.T) {
	out := lowerTSWithPath(t, `function add(a, b) { return a + b }`, "/tmp/example.ts")
	assertContains(t, out, "//line /tmp/example.ts:1")
}

func TestGenerateWithSourceEmitsLineDirectiveForClassMethod(t *testing.T) {
	out := lowerTSWithPath(t, "class Box {\n  open() {\n    return 1\n  }\n}\n", "/tmp/class.ts")
	assertContains(t, out, "//line /tmp/class.ts:3")
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

	file := Lower(mod, context.New(), "", false, context.O0)
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

	file := Lower(mod, context.New(), "", false, context.O0)
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

	file := Lower(mod, context.New(), "", false, context.O0)
	out, err := Generate(file)
	if err != nil {
		t.Fatalf("generate failed: %v", err)
	}
	s := string(out)
	assertContains(t, s, "Color")
	assertContains(t, s, "jsvalue.NewObject()")
	assertContains(t, s, `Color.Set("Red"`)
	assertContains(t, s, `Color.Set("Green"`)
	assertContains(t, s, `Color.Set("Blue"`)
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

func TestRoundTripPrivateClassFieldsAndMethods(t *testing.T) {
	out := lowerTS(t, `
		class Counter {
			#count = 1;
			#inc() { this.#count += 1; return this.#count; }
			value() { return this.#inc(); }
			static #label = "counter";
			static label() { return this.#label; }
		}
	`)
	assertContains(t, out, `PropertyKey(jsvalue.NewSymbol("Counter.#count"))`)
	assertContains(t, out, `PropertyKey(jsvalue.NewSymbol("Counter.#inc"))`)
	assertContains(t, out, `PropertyKey(jsvalue.NewSymbol("Counter.#label"))`)
	assertContains(t, out, `PropertyKey(jsvalue.NewSymbol("Counter.#brand"))`)
	assertNotContains(t, out, `Get("#count")`)
	assertNotContains(t, out, `Set("#count"`)
	assertNotContains(t, out, `MethodCall("#inc"`)
}

func TestRoundTripPrivateClassExpression(t *testing.T) {
	out := lowerTS(t, `
		const Box = class Box {
			#value = 1;
			get() { return this.#value; }
		};
	`)
	assertContains(t, out, `PropertyKey(jsvalue.NewSymbol("Box.#value"))`)
	assertContains(t, out, `return Box`)
	assertNotContains(t, out, `Get("#value")`)
	assertNotContains(t, out, `Set("#value"`)
}

func TestRoundTripEnum(t *testing.T) {
	out := lowerTS(t, `enum Direction { Up, Down, Left, Right }`)
	assertContains(t, out, "Direction")
	assertContains(t, out, "jsvalue.NewObject()")
	assertContains(t, out, `Direction.Set("Up"`)
}

func TestRoundTripStringEnum(t *testing.T) {
	out := lowerTS(t, `enum Dir { Up = "UP", Down = "DOWN" }`)
	assertContains(t, out, "Dir")
	assertContains(t, out, "jsvalue.NewObject()")
	assertContains(t, out, `Dir.Set("Up", jsvalue.NewString("UP"))`)
	assertContains(t, out, `Dir.Set("Down", jsvalue.NewString("DOWN"))`)
}

// --- Optional chaining tests ---

func TestRoundTripOptionalChaining(t *testing.T) {
	out := lowerTS(t, `function f() { return a?.b; }`)
	assertContains(t, out, "jsvalue.Eq")
	assertContains(t, out, "jsvalue.NewNull()")
	assertContains(t, out, "jsvalue.NewUndefined()")
	assertContains(t, out, `.Get("b")`)
}

func TestRoundTripOptionalChainingChained(t *testing.T) {
	out := lowerTS(t, `function f() { return a?.b?.c; }`)
	// Should have nested null checks
	assertContains(t, out, "jsvalue.Eq")
	assertContains(t, out, "jsvalue.NewUndefined()")
}

func TestRoundTripNonOptionalMember(t *testing.T) {
	out := lowerTS(t, `function f() { return a.b; }`)
	// Should NOT have null check
	assertNotContains(t, out, "jsvalue.Eq")
	assertContains(t, out, `.Get("b")`)
}

// --- Spread tests ---

func TestRoundTripSpreadInCall(t *testing.T) {
	out := lowerTS(t, `function f() { fn(...args); }`)
	assertContains(t, out, ".Call(")
	assertContains(t, out, "...")
}

func TestRoundTripSpreadInMethodCall(t *testing.T) {
	out := lowerTS(t, `function f() { obj.method(...args); }`)
	assertContains(t, out, "MethodCall")
	assertContains(t, out, ".Array()...")
}

func TestRoundTripSpreadArray(t *testing.T) {
	out := lowerTS(t, `const a = [...b, 1];`)
	assertContains(t, out, "jsvalue.NewArray")
}

// --- For-of destructuring tests ---

func TestRoundTripForOfArrayDestructure(t *testing.T) {
	out := lowerTS(t, `function f() { for (const [k, v] of entries) { console.log(k, v); } }`)
	assertContains(t, out, "_item")
	assertContains(t, out, ".Index(0)")
	assertContains(t, out, ".Index(1)")
	assertContains(t, out, "range")
}

func TestRoundTripForOfObjectDestructure(t *testing.T) {
	out := lowerTS(t, `function f() { for (const { name, age } of people) {} }`)
	assertContains(t, out, "_item")
	assertContains(t, out, `.Get("name")`)
	assertContains(t, out, `.Get("age")`)
}

func TestRoundTripForOfSimple(t *testing.T) {
	out := lowerTS(t, `function f() { for (const x of arr) { console.log(x); } }`)
	// Simple for-of should NOT use _item destructuring
	assertNotContains(t, out, "_item")
	assertContains(t, out, "range")
	assertContains(t, out, ".Array()")
}

// --- Regex flag tests ---

func TestRoundTripRegexNoFlags(t *testing.T) {
	out := lowerTS(t, `const re = /hello/;`)
	assertContains(t, out, "jsvalue.NewRegex")
	assertContains(t, out, "CompileRegex")
	assertNotContains(t, out, "NewRegexWithFlags")
}

func TestRoundTripRegexWithFlags(t *testing.T) {
	out := lowerTS(t, `const re = /hello/gi;`)
	// Flags are currently ignored — Go regex doesn't support JS flags like /g
	assertContains(t, out, "NewRegex")
	assertContains(t, out, "CompileRegex")
}

// --- Tagged template tests ---

func TestRoundTripTaggedTemplate(t *testing.T) {
	out := lowerTS(t, "const x = tag`hello ${name} world`;")
	// tree-sitter parses tagged templates as call_expression(tag, template_string)
	// so the output is tag(template_parts...)
	assertContains(t, out, "tag")
	assertContains(t, out, "hello")
}

// --- Destructuring tests ---

func TestRoundTripObjectDestructuring(t *testing.T) {
	out := lowerTS(t, `function f() { const { a, b } = obj; return a; }`)
	assertContains(t, out, `.Get("a")`)
	assertContains(t, out, `.Get("b")`)
}

func TestRoundTripArrayDestructuring(t *testing.T) {
	out := lowerTS(t, `function f() { const [x, y] = arr; return x; }`)
	assertContains(t, out, ".Index(0)")
	assertContains(t, out, ".Index(1)")
}

// --- Member assignment tests ---

func TestRoundTripMemberAssignment(t *testing.T) {
	out := lowerTS(t, `function f() { obj.name = "test"; }`)
	assertContains(t, out, `.Set("name"`)
}

func TestRoundTripAugmentedAssignment(t *testing.T) {
	out := lowerTS(t, `function f() { obj.count += 1; }`)
	assertContains(t, out, `.Set("count"`)
	assertContains(t, out, "jsvalue.Add")
}

func TestRoundTripUpdateStatement(t *testing.T) {
	out := lowerTS(t, `function f() { x++; }`)
	assertContains(t, out, "jsvalue.Inc")
}

// --- Nil-padding tests ---

func TestRoundTripNilPadding(t *testing.T) {
	out := lowerTS(t, `function f(a, b) { return a; }`)
	assertContains(t, out, "var a *jsvalue.JSValue")
	assertContains(t, out, "if len(_args) > 0")
	// b is unused so eliminated, but args are still unpacked
	assertContains(t, out, "if len(_args) > 1")
}

// --- Import resolution tests ---

func TestRoundTripImportDefault(t *testing.T) {
	// Note: lowerTS uses empty context, so "fs" is not a known module.
	// It gets treated as a transpiled module. Test via CLI for full resolution.
	out := lowerTS(t, `import fs from "fs"; const d = fs.readFileSync("x");`)
	assertContains(t, out, `"fs"`) // import path present
	assertContains(t, out, "fs")   // package name used
}

func TestRoundTripImportNamed(t *testing.T) {
	out := lowerTS(t, `import { readFileSync } from "fs"; const d = readFileSync("x");`)
	assertContains(t, out, "ReadFileSync") // capitalized
}

// --- Export default tests ---

func TestRoundTripExportDefaultExpr(t *testing.T) {
	out := lowerTS(t, `export default 42;`)
	assertContains(t, out, "var Default")
	assertContains(t, out, "42")
}

func TestRoundTripExportDefaultFunction(t *testing.T) {
	out := lowerTS(t, `export default function greet() { return "hi"; }`)
	assertContains(t, out, "var Greet")
	assertContains(t, out, "var Default = Greet")
}

// --- Main function creation ---

func TestRoundTripMainCreated(t *testing.T) {
	out := lowerTS(t, `const x = 42;`)
	assertContains(t, out, "func main()")
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
		file := Lower(mod, context.New(), "", false, context.O0)
		_, err := Generate(file)
		tree.Close()
		if err != nil {
			t.Errorf("Generate failed for %q: %v", ts, err)
		}
	}
}

// --- Destructuring assignment tests ---

func TestDestructuringArrayAssign(t *testing.T) {
	out := lowerTS(t, `
		let a: any, b: any;
		const arr = [1, 2];
		[a, b] = arr;
	`)
	assertContains(t, out, `_arr`)
	assertContains(t, out, `.Index(0)`)
	assertContains(t, out, `.Index(1)`)
}

func TestDestructuringObjectAssign(t *testing.T) {
	out := lowerTS(t, `
		let x: any, y: any;
		const obj = {x: 1, y: 2};
		({x, y} = obj);
	`)
	assertContains(t, out, `_obj`)
	assertContains(t, out, `.Get("x")`)
	assertContains(t, out, `.Get("y")`)
}

func TestDestructuringArrayRestAssign(t *testing.T) {
	out := lowerTS(t, `
		let first: any, rest: any;
		[first, ...rest] = [1, 2, 3];
	`)
	assertContains(t, out, `.Index(0)`)
	assertContains(t, out, `Slice`)
}

// --- Super call tests ---

func TestSuperCallInConstructor(t *testing.T) {
	out := lowerTS(t, `
		class Parent {}
		class Child extends Parent {
			constructor() { super(); }
		}
	`)
	assertContains(t, out, "CallSuper")
	assertContains(t, out, "this")
}

// --- Top-level this ---

func TestTopLevelThisIsUndefined(t *testing.T) {
	out := lowerTS(t, `const x = this;`)
	assertContains(t, out, "NewUndefined")
	assertNotContains(t, out, `goIdent("this")`)
}

func TestThisInsideFunctionIsPreserved(t *testing.T) {
	out := lowerTS(t, `
		function foo() { return this; }
	`)
	// Inside a function, this should not be NewUndefined
	assertNotContains(t, out, "NewUndefined")
}

// --- For-loop init tests ---

func TestForLoopMultiDeclaratorInit(t *testing.T) {
	out := lowerTS(t, `
		function f() {
			for (let i = 0, len = 10; i < len; i++) {}
		}
	`)
	// Should compile without "var declaration not allowed in for initializer"
	assertContains(t, out, "jsvalue.Lt")
}

func TestForLoopVarDeclInit(t *testing.T) {
	out := lowerTS(t, `
		function f() {
			for (var x: any; x; ) {}
		}
	`)
	// var decl should be hoisted before the for loop
	assertContains(t, out, "for")
}

// --- Object literal tests ---

func TestObjectLiteralSpread(t *testing.T) {
	out := lowerTS(t, `
		const a = {x: 1};
		const b = {...a, y: 2};
	`)
	assertContains(t, out, "Assign")
}

func TestObjectLiteralComputedKey(t *testing.T) {
	out := lowerTS(t, `
		const KEY = "myKey";
		const obj = { [KEY]: true };
	`)
	assertContains(t, out, "fmt.Sprint")
	assertContains(t, out, "NewObject")
	assertNotContains(t, out, `"[KEY]"`)
}

// --- Unused variable elimination tests ---

func TestUnusedVarElimination(t *testing.T) {
	out := lowerTS(t, `
		function f() {
			const x = 1;
			const y = 2;
			return y;
		}
	`)
	// x is unused, should be replaced with _
	assertContains(t, out, "_ =")
	assertContains(t, out, "return")
}

func TestUnusedVarEliminationKeepsOuterScopeAssignment(t *testing.T) {
	out := lowerTS(t, `
		let shim;
		function setShim(_shim) {
			shim = _shim;
			return shim;
		}
		function getShim() {
			return shim;
		}
	`)
	assertContains(t, out, "shim = jsvalue.From(_shim)")
	assertNotContains(t, out, "_ = jsvalue.From(_shim)")
}

func TestMethodRestParamUsesRemainingArgsSlice(t *testing.T) {
	out := lowerTS(t, `
		class Bag {
			collect(...args) {
				return args.length;
			}
		}
	`)
	assertContains(t, out, "args := jsvalue.NewArray(_args[1:]...)")
}

func TestDefaultExportObjectWithImportedDefaultDoesNotTriggerInitCycleSplit(t *testing.T) {
	out := lowerTS(t, `
		import parser from "yargs-parser";
		export default { parser };
	`)
	assertContains(t, out, "var Default =")
	assertNotContains(t, out, "func init()")
}

func TestLowerBlockPreservesNonFunctionStatementOrder(t *testing.T) {
	out := lowerTS(t, `
		function f(obj) {
			const app = obj.make();
			app.start();
			const result = app.finish();
			return result;
		}
	`)
	startIdx := strings.Index(out, `MethodCall("start")`)
	resultIdx := strings.Index(out, "result :=")
	if startIdx == -1 || resultIdx == -1 {
		t.Fatalf("expected both start call and result declaration in output:\n%s", out)
	}
	if startIdx > resultIdx {
		t.Fatalf("non-function statements were reordered:\n%s", out)
	}
}

func TestForwardDeclareBareLocalVarBeforeClosureReference(t *testing.T) {
	out := lowerTS(t, `
		function f() {
			const self = {};
			self.help = function() { return cachedHelpMessage; };
			let cachedHelpMessage;
			return self;
		}
	`)
	declIdx := strings.Index(out, "var cachedHelpMessage *jsvalue.JSValue")
	helpIdx := strings.Index(out, `Set("help"`)
	if declIdx == -1 || helpIdx == -1 {
		t.Fatalf("expected cachedHelpMessage decl and help assignment in output:\n%s", out)
	}
	if declIdx > helpIdx {
		t.Fatalf("bare local var was not hoisted before closure reference:\n%s", out)
	}
}

func TestComputedMemberAugmentedAssignUsesCurrentValue(t *testing.T) {
	out := lowerTS(t, `
		function f(rows, i, word) {
			rows[i] += word;
		}
	`)
	assertContains(t, out, `rows.Set(fmt.Sprint(i), jsvalue.Add(`)
	assertContains(t, out, `rows.Get(fmt.Sprint(i))`)
}

// --- Function hoisting tests ---

func TestFunctionHoistingInBody(t *testing.T) {
	out := lowerTS(t, `
		function outer() {
			const result = inner();
			function inner() { return 42; }
			return result;
		}
	`)
	// inner should be available before its declaration
	assertContains(t, out, "inner")
	assertContains(t, out, "NewFunction")
}

// --- FuncExpr this binding ---

func TestFuncExprThisBinding(t *testing.T) {
	out := lowerTS(t, `
		const obj = {
			method: function() { return this; }
		};
	`)
	// function expression that uses this should use lowerMethodBody
	assertContains(t, out, "this")
}

// --- Class method MarkAsMethod ---

func TestClassMethodsAreMarkedAsMethod(t *testing.T) {
	out := lowerTS(t, `
		class Foo {
			bar() { return 1; }
		}
	`)
	assertContains(t, out, "MarkAsMethod")
}

// --- Member assignment in expression context ---

func TestMemberAssignExpr(t *testing.T) {
	out := lowerTS(t, `
		function f(obj: any) {
			const x = (obj.foo = 42);
		}
	`)
	assertContains(t, out, `.Set("foo"`)
}

// --- Computed member call ---

func TestComputedMemberCall(t *testing.T) {
	out := lowerTS(t, `
		function f(obj: any, key: any) {
			obj[key]();
		}
	`)
	assertContains(t, out, "MethodCall")
	assertContains(t, out, "fmt.Sprint")
}

// --- Module.exports detection ---

func TestModuleExportsAsDefaultExport(t *testing.T) {
	tree := parseTS(t, `module.exports = function() { return 1; }`)
	mod := hir.BuildModule(tree.RootNode(), []byte(`module.exports = function() { return 1; }`), "mymod")
	tree.Close()

	hasExportDefault := false
	for _, d := range mod.Declarations {
		if ed, ok := d.(*hir.ExportDecl); ok && ed.IsDefault {
			hasExportDefault = true
		}
	}
	if !hasExportDefault {
		t.Error("module.exports = ... should produce ExportDecl with IsDefault")
	}
}

// --- Spread in call args ---

func TestSpreadWithRegularArgs(t *testing.T) {
	out := lowerTS(t, `
		function f(obj: any, arr: any) {
			obj.method("first", ...arr);
		}
	`)
	assertContains(t, out, "append")
}

func TestPrivateFieldBrandCheckOptimization(t *testing.T) {
	source := `
class A {
    #private: number;
    constructor(x: number) {
        this.#private = x;
    }
    method(other: A) {
        const a = this.#private;
        const b = other.#private;
        return a + b;
    }
}
`
	// Test O0: brand checks present for both this.#field and other.#field
	outO0 := lowerTS(t, source)
	brandCheckCountO0 := strings.Count(outO0, "HasOwnProperty")
	if brandCheckCountO0 < 4 {
		t.Errorf("O0: expected >= 4 HasOwnProperty (brand+key for read+read), got %d", brandCheckCountO0)
	}

	// Test O1: brand check eliminated for this.#field but kept for other.#field
	tree := parseTS(t, source)
	defer tree.Close()
	mod := hir.BuildModule(tree.RootNode(), []byte(source), "main")
	if err := hir.AsyncPipelinePhase1Error(mod); err != nil {
		t.Fatal(err)
	}
	fileO1 := Lower(mod, context.New(), "", false, context.O1)
	outO1Bytes, err := Generate(fileO1)
	if err != nil {
		t.Fatal(err)
	}
	outO1 := string(outO1Bytes)

	// O1 should still have brand checks for other.#field
	brandCheckCountO1 := strings.Count(outO1, "HasOwnProperty")
	if brandCheckCountO1 < 2 {
		t.Errorf("O1: expected >= 2 HasOwnProperty for other.#field (brand+key), got %d", brandCheckCountO1)
	}

	// O1 should have fewer brand checks than O0
	if brandCheckCountO1 >= brandCheckCountO0 {
		t.Errorf("O1 should have fewer HasOwnProperty checks than O0: O1=%d, O0=%d", brandCheckCountO1, brandCheckCountO0)
	}
}

func TestTopLevelAwaitGeneratesAsyncMain(t *testing.T) {
	out := lowerTS(t, `const x = await Promise.resolve(42); console.log(x);`)
	if !strings.Contains(out, "promise") {
		t.Error("async main should reference promise package")
	}
	if !strings.Contains(out, "_async_state") {
		t.Error("async main should contain async state machine")
	}
	if !strings.Contains(out, "eventloop") {
		t.Error("async main should contain eventloop run")
	}
}

func TestNoTopLevelAwaitPlainMain(t *testing.T) {
	out := lowerTS(t, `const x = 42; console.log(x);`)
	if strings.Contains(out, "_async_state") {
		t.Error("non-async module should not contain async state machine")
	}
	if strings.Contains(out, "promise.Promise") {
		t.Error("non-async module should not reference promise.Promise")
	}
}
