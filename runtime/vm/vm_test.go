package vm

import (
	"testing"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
)

func TestConstants(t *testing.T) {
	consts := AsJSValue.Get("constants")
	if consts == nil {
		t.Fatal("vm.constants is nil")
	}
	if consts.Get("USE_MAIN_CONTEXT_DEFAULT_LOADER").Number() != 1 {
		t.Fatal("vm.constants.USE_MAIN_CONTEXT_DEFAULT_LOADER != 1")
	}
	if consts.Get("DONT_CONTEXTIFY").Number() != 2 {
		t.Fatal("vm.constants.DONT_CONTEXTIFY != 2")
	}
}

func TestCreateContext(t *testing.T) {
	ctx := AsJSValue.Get("createContext").Call()
	if ctx == nil {
		t.Fatal("createContext() returned nil")
	}
	if !AsJSValue.Get("isContext").Call(ctx).Bool() {
		t.Fatal("isContext(createContext()) = false, want true")
	}
}

func TestCreateContextWithSandbox(t *testing.T) {
	sandbox := jsvalue.NewObject()
	sandbox.Set("x", jsvalue.NewNumber(42))
	ctx := AsJSValue.Get("createContext").Call(sandbox)
	if ctx == nil {
		t.Fatal("createContext(sandbox) returned nil")
	}
	if !AsJSValue.Get("isContext").Call(ctx).Bool() {
		t.Fatal("isContext(createContext(sandbox)) = false, want true")
	}
	if ctx.Get("x").Number() != 42 {
		t.Fatalf("sandbox x = %v, want 42", ctx.Get("x").Number())
	}
}

func TestIsContext(t *testing.T) {
	plain := jsvalue.NewObject()
	if AsJSValue.Get("isContext").Call(plain).Bool() {
		t.Fatal("isContext(plain object) = true, want false")
	}
	ctx := AsJSValue.Get("createContext").Call()
	if !AsJSValue.Get("isContext").Call(ctx).Bool() {
		t.Fatal("isContext(context) = false, want true")
	}
}

func TestIsContextGoRegistry(t *testing.T) {
	ctx := AsJSValue.Get("createContext").Call()
	keys := ctx.OwnKeys()
	for _, k := range keys {
		if k == "_isVMContext" {
			t.Fatal("context has JS-visible _isVMContext property; isContext should use Go-side registry")
		}
	}
}

func TestRunInThisContext(t *testing.T) {
	result := AsJSValue.Get("runInThisContext").Call(jsvalue.NewString("1 + 2"))
	if result.Number() != 3 {
		t.Fatalf("runInThisContext('1 + 2') = %v, want 3", result.Number())
	}
}

func TestRunInThisContextStatements(t *testing.T) {
	result := AsJSValue.Get("runInThisContext").Call(jsvalue.NewString("var x = 10; var y = 20;"))
	if result == nil {
		t.Fatal("runInThisContext(statements) returned nil")
	}
}

func TestRunInNewContext(t *testing.T) {
	result := AsJSValue.Get("runInNewContext").Call(jsvalue.NewString("5 + 5"))
	if result.Number() != 10 {
		t.Fatalf("runInNewContext('5 + 5') = %v, want 10", result.Number())
	}
}

func TestRunInContext(t *testing.T) {
	ctx := AsJSValue.Get("createContext").Call()
	result := AsJSValue.Get("runInContext").Call(jsvalue.NewString("3 * 7"), ctx)
	if result.Number() != 21 {
		t.Fatalf("runInContext('3 * 7') = %v, want 21", result.Number())
	}
}

func TestRunInContextNotAContext(t *testing.T) {
	plain := jsvalue.NewObject()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("runInContext with non-context should panic")
		}
	}()
	AsJSValue.Get("runInContext").Call(jsvalue.NewString("1"), plain)
}

func TestScriptRunInThisContext(t *testing.T) {
	Script := AsJSValue.Get("Script")
	script := Script.Call(jsvalue.NewString("100 - 50"))
	result := script.MethodCall("runInThisContext")
	if result.Number() != 50 {
		t.Fatalf("Script.runInThisContext() = %v, want 50", result.Number())
	}
}

func TestScriptRunInContext(t *testing.T) {
	Script := AsJSValue.Get("Script")
	script := Script.Call(jsvalue.NewString("a + b"))
	ctx := AsJSValue.Get("createContext").Call()
	ctx.Set("a", jsvalue.NewNumber(3))
	ctx.Set("b", jsvalue.NewNumber(4))
	result := script.MethodCall("runInContext", ctx)
	if result.Number() != 7 {
		t.Fatalf("Script.runInContext() = %v, want 7", result.Number())
	}
}

func TestScriptRunInNewContext(t *testing.T) {
	Script := AsJSValue.Get("Script")
	script := Script.Call(jsvalue.NewString("40 + 2"))
	result := script.MethodCall("runInNewContext")
	if result.Number() != 42 {
		t.Fatalf("Script.runInNewContext() = %v, want 42", result.Number())
	}
}

func TestScriptCreateCachedData(t *testing.T) {
	Script := AsJSValue.Get("Script")
	script := Script.Call(jsvalue.NewString("1"))
	cached := script.MethodCall("createCachedData")
	if cached == nil {
		t.Fatal("createCachedData() returned nil")
	}
}

func TestCompileFunction(t *testing.T) {
	fn := AsJSValue.Get("compileFunction").Call(
		jsvalue.NewString("return a + b"),
		jsvalue.NewArray(jsvalue.NewString("a"), jsvalue.NewString("b")),
	)
	if fn == nil {
		t.Fatal("compileFunction() returned nil")
	}
	result := fn.Call(jsvalue.NewNumber(10), jsvalue.NewNumber(20))
	if result.Number() != 30 {
		t.Fatalf("compiled function(10, 20) = %v, want 30", result.Number())
	}
}

func TestContextIsolation(t *testing.T) {
	ctx1 := AsJSValue.Get("createContext").Call()
	ctx2 := AsJSValue.Get("createContext").Call()

	AsJSValue.Get("runInContext").Call(jsvalue.NewString("var x = 100"), ctx1)
	AsJSValue.Get("runInContext").Call(jsvalue.NewString("var x = 200"), ctx2)

	v1 := AsJSValue.Get("runInContext").Call(jsvalue.NewString("x"), ctx1)
	v2 := AsJSValue.Get("runInContext").Call(jsvalue.NewString("x"), ctx2)

	if v1.Number() != 100 {
		t.Fatalf("ctx1.x = %v, want 100", v1.Number())
	}
	if v2.Number() != 200 {
		t.Fatalf("ctx2.x = %v, want 200", v2.Number())
	}
}

func TestScriptIsUnique(t *testing.T) {
	Script := AsJSValue.Get("Script")
	s1 := Script.Call(jsvalue.NewString("1"))
	s2 := Script.Call(jsvalue.NewString("2"))
	scriptMu.RLock()
	_, ok1 := scriptRegistry[s1]
	_, ok2 := scriptRegistry[s2]
	scriptMu.RUnlock()
	if !ok1 || !ok2 {
		t.Fatal("each Script should have its own registry entry")
	}
}

func TestScriptNoArgs(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("new Script() without args should panic")
		}
	}()
	Script := AsJSValue.Get("Script")
	Script.Call()
}
