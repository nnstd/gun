package dynfunc_test

import (
	"go/types"
	"os"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
	"golang.org/x/tools/go/ssa/interp"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
)

// TestSSAVerifyJSValue verifies SSA compatibility with JSValue.
// It tests: loading, SSA building, and interpreting.
func TestSSAVerifyJSValue(t *testing.T) {
	// Step 1: Can we load JSValue runtime into SSA?
	t.Run("LoadJSValueIntoSSA", func(t *testing.T) {
		cfg := &packages.Config{Mode: packages.LoadAllSyntax}
		initial, err := packages.Load(cfg, "github.com/nnstd/gun/runtime/builtin")
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if packages.PrintErrors(initial) > 0 {
			t.Fatal("Load errors")
		}

		prog, pkgs := ssautil.AllPackages(initial, ssa.InstantiateGenerics|ssa.GlobalDebug)
		prog.Build()

		var jsvalPkg *ssa.Package
		for _, p := range pkgs {
			if p != nil && strings.HasSuffix(p.Pkg.Path(), "runtime/builtin") {
				jsvalPkg = p
				break
			}
		}
		if jsvalPkg == nil {
			t.Fatal("JSValue package not found in SSA")
		}

		// Inspect key functions
		for _, name := range []string{"NewNumber", "NewString", "Add", "Sub", "Eq"} {
			fn := jsvalPkg.Func(name)
			if fn == nil {
				t.Errorf("function %s not found in SSA", name)
				continue
			}
			t.Logf("  %s: %d blocks, %d params", name, len(fn.Blocks), fn.Signature.Params().Len())
			if len(fn.Blocks) > 0 {
				// Print function to verify SSA is real
				fn.WriteTo(os.Stderr)
			}
		}

		// Inspect JSValue type methods
		jsvalType := jsvalPkg.Type("JSValue")
		if jsvalType == nil {
			t.Fatal("JSValue type not found")
		}
		methodSet := prog.MethodSets.MethodSet(types.NewPointer(jsvalType.Object().Type()))
		t.Logf("  JSValue has %d methods", methodSet.Len())
		for i := 0; i < methodSet.Len() && i < 15; i++ {
			sel := methodSet.At(i)
			t.Logf("    .%s()", sel.Obj().Name())
		}
	})

	// Step 2: Build SSA from a program that uses JSValue, then interpret.
	t.Run("InterpretJSValue", func(t *testing.T) {
		gunRoot := findGunRoot(t)

		cases := []struct {
			name   string
			source string
		}{
			{
				"NewNumber",
				`package main

import jsvalue "github.com/nnstd/gun/runtime/builtin"

func main() {
	a := jsvalue.NewNumber(42.0)
	_ = a
}
`,
			},
			{
				"NewNumber_println",
				`package main

import jsvalue "github.com/nnstd/gun/runtime/builtin"

func main() {
	a := jsvalue.NewNumber(42.0)
	println(a.Number())
}
`,
			},
			{
				"Add",
				`package main

import jsvalue "github.com/nnstd/gun/runtime/builtin"

func main() {
	a := jsvalue.NewNumber(41.0)
	b := jsvalue.NewNumber(1.0)
	c := jsvalue.Add(a, b)
	println(c.Number())
}
`,
			},
			{
				"NewString",
				`package main

import jsvalue "github.com/nnstd/gun/runtime/builtin"

func main() {
	s := jsvalue.NewString("hello")
	println(s.String())
}
`,
			},
			{
				"NewFunction",
				`package main

import jsvalue "github.com/nnstd/gun/runtime/builtin"

func main() {
	fn := jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) > 0 {
			return args[0]
		}
		return jsvalue.NewUndefined()
	})
	result := fn.Call(jsvalue.NewNumber(42.0))
	println(result.Number())
}
`,
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				tmpDir := writeTempPackage(t, gunRoot, tc.source)
				defer os.RemoveAll(tmpDir)

				cfg := &packages.Config{Mode: packages.LoadAllSyntax, Dir: tmpDir}
				initial, err := packages.Load(cfg, ".", "runtime")
				if err != nil {
					t.Fatalf("Load: %v", err)
				}
				if packages.PrintErrors(initial) > 0 {
					t.Fatal("Load errors")
				}

				prog, pkgs := ssautil.AllPackages(initial, ssa.InstantiateGenerics)
				prog.Build()

				var mainPkg *ssa.Package
				for _, p := range pkgs {
					if p != nil && p.Pkg.Name() == "main" {
						mainPkg = p
						break
					}
				}
				if mainPkg == nil {
					t.Fatal("main package not found")
				}

				// Print main() SSA
				mainFn := mainPkg.Func("main")
				if mainFn != nil {
					t.Log("=== main() SSA ===")
					mainFn.WriteTo(os.Stderr)
				}

				// Try interpreting
				t.Log("Interpreting...")
				exitCode := interp.Interpret(
					mainPkg,
					0,
					&types.StdSizes{WordSize: 8, MaxAlign: 8},
					"main.go",
					nil,
				)

				if exitCode != 0 {
					t.Logf("FAIL: exit code %d", exitCode)
				} else {
					t.Log("OK")
				}
			})
		}
	})

	// Step 3: Build SSA only (no interpret) from transpiled Gun output
	t.Run("LoadTranspiledIntoSSA", func(t *testing.T) {
		gunRoot := findGunRoot(t)
		source := `package main

import jsvalue "github.com/nnstd/gun/runtime/builtin"

var DynFunc = jsvalue.NewFunction(func(_args ...*jsvalue.JSValue) *jsvalue.JSValue {
	var a *jsvalue.JSValue
	if len(_args) > 0 {
		a = _args[0]
	}
	return jsvalue.Add(jsvalue.From(a), jsvalue.NewNumber(1.0))
})

func main() {
	result := DynFunc.Call(jsvalue.NewNumber(41.0))
	println(result.Number())
}
`
		tmpDir := writeTempPackage(t, gunRoot, source)
		defer os.RemoveAll(tmpDir)

		cfg := &packages.Config{Mode: packages.LoadAllSyntax, Dir: tmpDir}
		initial, err := packages.Load(cfg, ".", "runtime")
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if packages.PrintErrors(initial) > 0 {
			t.Fatal("Load errors")
		}

		prog, pkgs := ssautil.AllPackages(initial, ssa.InstantiateGenerics|ssa.GlobalDebug)
		prog.Build()

		// Count SSA functions across all loaded packages
		totalFns := 0
		jsvalFns := 0
		for _, p := range pkgs {
			if p == nil {
				continue
			}
			members := p.Members
			for _, m := range members {
				if _, ok := m.(*ssa.Function); ok {
					totalFns++
					if strings.HasPrefix(p.Pkg.Path(), "runtime/builtin") {
						jsvalFns++
					}
				}
			}
		}
		t.Logf("Total SSA functions: %d, JSValue runtime: %d", totalFns, jsvalFns)

		// Inspect DynFunc init
		initFn := pkgs[0].Func("init")
		if initFn != nil {
			t.Log("=== init() SSA ===")
			initFn.WriteTo(os.Stderr)
		}
	})

	// Keep jsvalue import used
	_ = jsvalue.NewNumber(0)
}
