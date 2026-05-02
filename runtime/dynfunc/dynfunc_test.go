package dynfunc_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"plugin"
	"testing"

	sitter "github.com/tree-sitter/go-tree-sitter"
	typescript "github.com/tree-sitter/tree-sitter-typescript/bindings/go"
	"github.com/nnstd/gun/compiler/context"
	"github.com/nnstd/gun/compiler/pipeline"
	jsvalue "github.com/nnstd/gun/runtime/builtin"
	"github.com/nnstd/gun/runtime/dynfunc"
)

// TestBuildAndRun tests the simplest approach: transpile → go build → run as subprocess.
func TestBuildAndRun(t *testing.T) {
	source := `package main

import jsvalue "github.com/nnstd/gun/runtime/builtin"

var DynFunc = jsvalue.NewFunction(func(_args ...*jsvalue.JSValue) *jsvalue.JSValue {
	var a *jsvalue.JSValue
	if len(_args) > 0 {
		a = _args[0]
	}
	return jsvalue.Add(jsvalue.From(a), jsvalue.NewNumber(float64(1)))
})

func main() {
	result := DynFunc.Call(jsvalue.NewNumber(float64(41)))
	println(result.String())
}
`
	gunRoot := findGunRoot(t)
	tmpDir := writeTempPackage(t, gunRoot, source)
	defer os.RemoveAll(tmpDir)

	binPath := filepath.Join(tmpDir, "dynfunc_bin")
	cmd := exec.Command("go", "build", "-o", binPath, ".")
	cmd.Dir = tmpDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	cmd = exec.Command(binPath)
	out, err := cmd.CombinedOutput()
	t.Logf("Output: %s", out)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if string(out) != "42\n" {
		t.Fatalf("expected 42, got %q", string(out))
	}
}

// TestBuildPlugin tests loading transpiled code as a Go plugin.
func TestBuildPlugin(t *testing.T) {
	source := `package main

import jsvalue "github.com/nnstd/gun/runtime/builtin"

var DynFunc = jsvalue.NewFunction(func(_args ...*jsvalue.JSValue) *jsvalue.JSValue {
	var a *jsvalue.JSValue
	if len(_args) > 0 {
		a = _args[0]
	}
	return jsvalue.Add(jsvalue.From(a), jsvalue.NewNumber(float64(1)))
})
`
	gunRoot := findGunRoot(t)
	tmpDir := writeTempPackage(t, gunRoot, source)
	defer os.RemoveAll(tmpDir)

	soPath := filepath.Join(tmpDir, filepath.Base(tmpDir)+".so")
	cmd := exec.Command("go", "build", "-buildmode=plugin", "-o", soPath, ".")
	cmd.Dir = tmpDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build -buildmode=plugin: %v\n%s", err, out)
	}

	plug, err := plugin.Open(soPath)
	if err != nil {
		t.Fatalf("plugin.Open: %v", err)
	}

	sym, err := plug.Lookup("DynFunc")
	if err != nil {
		t.Fatalf("Lookup DynFunc: %v", err)
	}

	dynFunc, ok := sym.(**jsvalue.JSValue)
	if !ok {
		t.Fatalf("expected **jsvalue.JSValue, got %T", sym)
	}

	result := (*dynFunc).Call(jsvalue.NewNumber(float64(41)))
	t.Logf("Result: %s", result.String())
	if result.String() != "42" {
		t.Fatalf("expected 42, got %s", result.String())
	}
}

// TestTranspilePlugin tests the full flow: JS source → Gun pipeline → Go source → plugin → call.
func TestTranspilePlugin(t *testing.T) {
	jsSource := []byte("function DynFunc(a) { return a + 1 }")

	parser := sitter.NewParser()
	defer parser.Close()
	lang := sitter.NewLanguage(typescript.LanguageTypescript())
	if err := parser.SetLanguage(lang); err != nil {
		t.Fatal(err)
	}

	tree := parser.Parse(jsSource, nil)
	defer tree.Close()

	pipe := pipeline.New(context.O0)
	goSource, err := pipe.CompileTree(tree.RootNode(), jsSource, "main", "github.com/nnstd/gun", false)
	if err != nil {
		t.Fatalf("transpile: %v", err)
	}

	t.Logf("Transpiled:\n%s", string(goSource))

	gunRoot := findGunRoot(t)
	tmpDir := writeTempPackage(t, gunRoot, string(goSource))
	defer os.RemoveAll(tmpDir)

	soPath := filepath.Join(tmpDir, filepath.Base(tmpDir)+".so")
	cmd := exec.Command("go", "build", "-buildmode=plugin", "-o", soPath, ".")
	cmd.Dir = tmpDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build plugin: %v\n%s", err, out)
	}

	plug, err := plugin.Open(soPath)
	if err != nil {
		t.Fatalf("plugin.Open: %v", err)
	}

	sym, err := plug.Lookup("DynFunc")
	if err != nil {
		t.Fatalf("Lookup DynFunc: %v", err)
	}

	dynFunc, ok := sym.(**jsvalue.JSValue)
	if !ok {
		t.Fatalf("expected **jsvalue.JSValue, got %T", sym)
	}

	result := (*dynFunc).Call(jsvalue.NewNumber(float64(41)))
	t.Logf("Result: %s", result.String())
	if result.String() != "42" {
		t.Fatalf("expected 42, got %s", result.String())
	}
}

// TestCompileFunctionAPI tests the public CompileFunction API
// (new Function("a", "return a + 1") equivalent).
func TestCompileFunctionAPI(t *testing.T) {
	fn := dynfunc.CompileFunction(
		jsvalue.NewString("a"),
		jsvalue.NewString("return a + 1"),
	)

	result := fn.Call(jsvalue.NewNumber(float64(41)))
	t.Logf("CompileFunction result: %s", result.String())
	if result.String() != "42" {
		t.Fatalf("expected 42, got %s", result.String())
	}
}

// TestCompileFunctionHIR tests the toolchain-free HIR interpreter.
func TestCompileFunctionHIR(t *testing.T) {
	t.Run("simple_add", func(t *testing.T) {
		fn := dynfunc.CompileFunctionHIR(
			jsvalue.NewString("a"),
			jsvalue.NewString("return a + 1"),
		)

		result := fn.Call(jsvalue.NewNumber(float64(41)))
		t.Logf("HIR result: %s", result.String())
		if result.String() != "42" {
			t.Fatalf("expected 42, got %s", result.String())
		}
	})

	t.Run("multi_param", func(t *testing.T) {
		fn := dynfunc.CompileFunctionHIR(
			jsvalue.NewString("a, b"),
			jsvalue.NewString("return a + b"),
		)

		result := fn.Call(jsvalue.NewNumber(float64(20)), jsvalue.NewNumber(float64(22)))
		t.Logf("HIR multi-param result: %s", result.String())
		if result.String() != "42" {
			t.Fatalf("expected 42, got %s", result.String())
		}
	})

	t.Run("if_else", func(t *testing.T) {
		fn := dynfunc.CompileFunctionHIR(
			jsvalue.NewString("x"),
			jsvalue.NewString("if (x > 10) { return x * 2 } else { return x }"),
		)

		r1 := fn.Call(jsvalue.NewNumber(float64(5)))
		r2 := fn.Call(jsvalue.NewNumber(float64(20)))
		t.Logf("HIR if_else: %s, %s", r1.String(), r2.String())
		if r1.String() != "5" {
			t.Fatalf("expected 5, got %s", r1.String())
		}
		if r2.String() != "40" {
			t.Fatalf("expected 40, got %s", r2.String())
		}
	})

	t.Run("string_concat", func(t *testing.T) {
		fn := dynfunc.CompileFunctionHIR(
			jsvalue.NewString("a, b"),
			jsvalue.NewString("return a + b"),
		)

		result := fn.Call(jsvalue.NewString("hello "), jsvalue.NewString("world"))
		t.Logf("HIR string concat: %s", result.String())
		if result.String() != "hello world" {
			t.Fatalf("expected 'hello world', got %s", result.String())
		}
	})

	t.Run("closure", func(t *testing.T) {
		fn := dynfunc.CompileFunctionHIR(
			jsvalue.NewString("n"),
			jsvalue.NewString("const add = (a, b) => a + b; return add(n, 8)"),
		)

		result := fn.Call(jsvalue.NewNumber(float64(34)))
		t.Logf("HIR closure: %s", result.String())
		if result.String() != "42" {
			t.Fatalf("expected 42, got %s", result.String())
		}
	})

	t.Run("for_loop", func(t *testing.T) {
		fn := dynfunc.CompileFunctionHIR(
			jsvalue.NewString("n"),
			jsvalue.NewString("let sum = 0; for (let i = 0; i < n; i = i + 1) { sum = sum + i } return sum"),
		)

		result := fn.Call(jsvalue.NewNumber(float64(5)))
		t.Logf("HIR for_loop: %s", result.String())
		if result.String() != "10" {
			t.Fatalf("expected 10, got %s", result.String())
		}
	})

	// --- New feature tests ---

	t.Run("spread_in_call", func(t *testing.T) {
		fn := dynfunc.CompileFunctionHIR(
			jsvalue.NewString("a"),
			jsvalue.NewString("const arr = [2, 3]; return Math.max(a, ...arr)"),
		)

		result := fn.Call(jsvalue.NewNumber(float64(1)))
		t.Logf("HIR spread_in_call: %s", result.String())
		if result.String() != "3" {
			t.Fatalf("expected 3, got %s", result.String())
		}
	})

	t.Run("spread_in_array", func(t *testing.T) {
		fn := dynfunc.CompileFunctionHIR(
			jsvalue.NewString(""),
			jsvalue.NewString("const a = [1, 2]; const b = [...a, 3]; return b.length"),
		)

		result := fn.Call()
		t.Logf("HIR spread_in_array: %s", result.String())
		if result.String() != "3" {
			t.Fatalf("expected 3, got %s", result.String())
		}
	})

	t.Run("object_destructure", func(t *testing.T) {
		fn := dynfunc.CompileFunctionHIR(
			jsvalue.NewString("obj"),
			jsvalue.NewString("const { a, b } = obj; return a + b"),
		)

		obj := jsvalue.NewObject()
		obj.Set("a", jsvalue.NewNumber(float64(20)))
		obj.Set("b", jsvalue.NewNumber(float64(22)))
		result := fn.Call(obj)
		t.Logf("HIR object_destructure: %s", result.String())
		if result.String() != "42" {
			t.Fatalf("expected 42, got %s", result.String())
		}
	})

	t.Run("object_destructure_default", func(t *testing.T) {
		fn := dynfunc.CompileFunctionHIR(
			jsvalue.NewString("obj"),
			jsvalue.NewString("const { x = 10 } = obj; return x"),
		)

		result := fn.Call(jsvalue.NewObject())
		t.Logf("HIR destructure_default: %s", result.String())
		if result.String() != "10" {
			t.Fatalf("expected 10, got %s", result.String())
		}
	})

	t.Run("array_destructure", func(t *testing.T) {
		fn := dynfunc.CompileFunctionHIR(
			jsvalue.NewString("arr"),
			jsvalue.NewString("const [a, b] = arr; return a + b"),
		)

		arr := jsvalue.NewArray(
			jsvalue.NewNumber(float64(20)),
			jsvalue.NewNumber(float64(22)),
		)
		result := fn.Call(arr)
		t.Logf("HIR array_destructure: %s", result.String())
		if result.String() != "42" {
			t.Fatalf("expected 42, got %s", result.String())
		}
	})

	t.Run("destructure_param", func(t *testing.T) {
		fn := dynfunc.CompileFunctionHIR(
			jsvalue.NewString("{ a, b }"),
			jsvalue.NewString("return a * b"),
		)

		obj := jsvalue.NewObject()
		obj.Set("a", jsvalue.NewNumber(float64(6)))
		obj.Set("b", jsvalue.NewNumber(float64(7)))
		result := fn.Call(obj)
		t.Logf("HIR destructure_param: %s", result.String())
		if result.String() != "42" {
			t.Fatalf("expected 42, got %s", result.String())
		}
	})

	t.Run("rest_param", func(t *testing.T) {
		fn := dynfunc.CompileFunctionHIR(
			jsvalue.NewString("first, ...rest"),
			jsvalue.NewString("return rest.length"),
		)

		result := fn.Call(jsvalue.NewNumber(1), jsvalue.NewNumber(2), jsvalue.NewNumber(3))
		t.Logf("HIR rest_param: %s", result.String())
		if result.String() != "2" {
			t.Fatalf("expected 2, got %s", result.String())
		}
	})

	t.Run("global_Math", func(t *testing.T) {
		fn := dynfunc.CompileFunctionHIR(
			jsvalue.NewString(""),
			jsvalue.NewString("return Math.floor(3.7)"),
		)

		result := fn.Call()
		t.Logf("HIR Math.floor: %s", result.String())
		if result.String() != "3" {
			t.Fatalf("expected 3, got %s", result.String())
		}
	})

	t.Run("context_isolation", func(t *testing.T) {
		// new Function() CANNOT see outer scope — only globals + params
		// This matches Bun/Node/browser behavior
		fn := dynfunc.CompileFunctionHIR(
			jsvalue.NewString(""),
			jsvalue.NewString("return typeof closureVar"),
		)

		result := fn.Call()
		t.Logf("HIR context_isolation: %s", result.String())
		if result.String() != "undefined" {
			t.Fatalf("expected 'undefined' (isolated scope), got %s", result.String())
		}
	})

	t.Run("global_access", func(t *testing.T) {
		// new Function() CAN see global objects
		fn := dynfunc.CompileFunctionHIR(
			jsvalue.NewString(""),
			jsvalue.NewString("return typeof Math"),
		)

		result := fn.Call()
		t.Logf("HIR global_access: %s", result.String())
		if result.String() != "object" {
			t.Fatalf("expected 'object', got %s", result.String())
		}
	})

	t.Run("try_catch", func(t *testing.T) {
		fn := dynfunc.CompileFunctionHIR(
			jsvalue.NewString(""),
			jsvalue.NewString("try { throw new Error('test') } catch(e) { return e.message }"),
		)

		result := fn.Call()
		t.Logf("HIR try_catch: %s", result.String())
		if result.String() != "test" {
			t.Fatalf("expected 'test', got %s", result.String())
		}
	})

	t.Run("ternary", func(t *testing.T) {
		fn := dynfunc.CompileFunctionHIR(
			jsvalue.NewString("x"),
			jsvalue.NewString("return x > 0 ? x * 2 : x * -1"),
		)

		r1 := fn.Call(jsvalue.NewNumber(float64(5)))
		r2 := fn.Call(jsvalue.NewNumber(float64(-3)))
		t.Logf("HIR ternary: %s, %s", r1.String(), r2.String())
		if r1.String() != "10" {
			t.Fatalf("expected 10, got %s", r1.String())
		}
		if r2.String() != "3" {
			t.Fatalf("expected 3, got %s", r2.String())
		}
	})

	t.Run("while_loop", func(t *testing.T) {
		fn := dynfunc.CompileFunctionHIR(
			jsvalue.NewString("n"),
			jsvalue.NewString("let i = 0, sum = 0; while (i < n) { sum = sum + i; i = i + 1 } return sum"),
		)

		result := fn.Call(jsvalue.NewNumber(float64(4)))
		t.Logf("HIR while: %s", result.String())
		if result.String() != "6" {
			t.Fatalf("expected 6, got %s", result.String())
		}
	})

	t.Run("object_literal", func(t *testing.T) {
		fn := dynfunc.CompileFunctionHIR(
			jsvalue.NewString(""),
			jsvalue.NewString("const obj = { x: 1, y: 2 }; return obj.x + obj.y"),
		)

		result := fn.Call()
		t.Logf("HIR object_literal: %s", result.String())
		if result.String() != "3" {
			t.Fatalf("expected 3, got %s", result.String())
		}
	})

	t.Run("typeof", func(t *testing.T) {
		fn := dynfunc.CompileFunctionHIR(
			jsvalue.NewString("x"),
			jsvalue.NewString("return typeof x"),
		)

		r1 := fn.Call(jsvalue.NewNumber(1))
		r2 := fn.Call(jsvalue.NewString("hi"))
		r3 := fn.Call(jsvalue.NewUndefined())
		t.Logf("HIR typeof: %s, %s, %s", r1.String(), r2.String(), r3.String())
		if r1.String() != "number" {
			t.Fatalf("expected 'number', got %s", r1.String())
		}
		if r2.String() != "string" {
			t.Fatalf("expected 'string', got %s", r2.String())
		}
		if r3.String() != "undefined" {
			t.Fatalf("expected 'undefined', got %s", r3.String())
		}
	})

	t.Run("nullish_coalescing", func(t *testing.T) {
		fn := dynfunc.CompileFunctionHIR(
			jsvalue.NewString("x"),
			jsvalue.NewString("return x ?? 42"),
		)

		r1 := fn.Call(jsvalue.NewNull())
		r2 := fn.Call(jsvalue.NewNumber(5))
		t.Logf("HIR nullish: %s, %s", r1.String(), r2.String())
		if r1.String() != "42" {
			t.Fatalf("expected 42, got %s", r1.String())
		}
		if r2.String() != "5" {
			t.Fatalf("expected 5, got %s", r2.String())
		}
	})

	t.Run("switch", func(t *testing.T) {
		fn := dynfunc.CompileFunctionHIR(
			jsvalue.NewString("x"),
			jsvalue.NewString(`switch(x) { case 1: return "one"; case 2: return "two"; default: return "other" }`),
		)

		r1 := fn.Call(jsvalue.NewNumber(1))
		r2 := fn.Call(jsvalue.NewNumber(2))
		r3 := fn.Call(jsvalue.NewNumber(99))
		if r1.String() != "one" || r2.String() != "two" || r3.String() != "other" {
			t.Fatalf("switch: %s, %s, %s", r1.String(), r2.String(), r3.String())
		}
	})

	t.Run("EvalHIR", func(t *testing.T) {
		result := dynfunc.EvalHIR(nil, jsvalue.NewString("2 + 2 * 10"))
		t.Logf("EvalHIR: %s", result.String())
		if result.String() != "22" {
			t.Fatalf("expected 22, got %s", result.String())
		}
	})
}

// --- helpers ---

func writeTempPackage(t *testing.T, gunRoot, source string) string {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "gun-dynfunc-*")
	if err != nil {
		t.Fatal(err)
	}

	// Use unique module name from temp dir to avoid "plugin already loaded"
	modName := filepath.Base(tmpDir)
	gomod := fmt.Sprintf(`module %s

go 1.25.0

require github.com/nnstd/gun v0.0.0

replace github.com/nnstd/gun => %s
`, modName, gunRoot)

	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte(gomod), 0644); err != nil {
		os.RemoveAll(tmpDir)
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte(source), 0644); err != nil {
		os.RemoveAll(tmpDir)
		t.Fatal(err)
	}

	cmd := exec.Command("go", "mod", "tidy")
	cmd.Dir = tmpDir
	if out, err := cmd.CombinedOutput(); err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("go mod tidy: %v\n%s", err, out)
	}

	return tmpDir
}

func findGunRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			if _, err := os.Stat(filepath.Join(dir, "compiler")); err == nil {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find gun repo root")
		}
		dir = parent
	}
}
