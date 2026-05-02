package dynfunc_test

import (
	"testing"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
	"github.com/nnstd/gun/runtime/dynfunc"
	"github.com/nnstd/gun/runtime/jscontext"
)

// --- eval() reads from default context ---

func TestEvalHIR_ReadsGlobalFromDefaultContext(t *testing.T) {
	ctx := jscontext.Default()
	ctx.Set("myGlobalVar", jsvalue.NewNumber(42))

	result := dynfunc.EvalHIR(nil, jsvalue.NewString("myGlobalVar"))
	if result.String() != "42" {
		t.Fatalf("expected 42, got %s", result.String())
	}
}

func TestEvalHIR_ReadsBuiltinGlobal(t *testing.T) {
	result := dynfunc.EvalHIR(nil, jsvalue.NewString("Math.floor(3.7)"))
	if result.String() != "3" {
		t.Fatalf("expected 3, got %s", result.String())
	}
}

func TestEvalHIR_ReadsObjectPropertyFromContext(t *testing.T) {
	ctx := jscontext.Default()

	obj := jsvalue.NewObject()
	obj.Set("name", jsvalue.NewString("gun"))
	obj.Set("version", jsvalue.NewNumber(1))
	ctx.Set("config", obj)

	result := dynfunc.EvalHIR(nil, jsvalue.NewString("config.name"))
	if result.String() != "gun" {
		t.Fatalf("expected 'gun', got %s", result.String())
	}
}

// --- eval() modifies default context via globalThis ---

func TestEvalHIR_WritesToDefaultContextViaGlobalThis(t *testing.T) {
	ctx := jscontext.Default()
	defer ctx.Set("evalWriteTest", jsvalue.NewUndefined())

	dynfunc.EvalHIR(nil, jsvalue.NewString("globalThis.evalWriteTest = 99"))

	got := ctx.Get("evalWriteTest")
	if got == nil || got.String() != "99" {
		t.Fatalf("expected 99 in default context, got %s", safeString(got))
	}
}

func TestEvalHIR_UpdatesExistingGlobalViaGlobalThis(t *testing.T) {
	ctx := jscontext.Default()
	defer ctx.Set("evalMutTest", jsvalue.NewUndefined())

	ctx.Set("evalMutTest", jsvalue.NewNumber(10))
	dynfunc.EvalHIR(nil, jsvalue.NewString("globalThis.evalMutTest = globalThis.evalMutTest + 5"))

	got := ctx.Get("evalMutTest")
	if got == nil || got.String() != "15" {
		t.Fatalf("expected 15, got %s", safeString(got))
	}
}

// --- new Function() reads from default context ---

func TestCompileFunctionHIR_ReadsGlobalVarFromDefaultContext(t *testing.T) {
	ctx := jscontext.Default()
	ctx.Set("sharedCounter", jsvalue.NewNumber(100))

	fn := dynfunc.CompileFunctionHIR(
		jsvalue.NewString(""),
		jsvalue.NewString("return sharedCounter"),
	)
	result := fn.Call()
	if result.String() != "100" {
		t.Fatalf("expected 100, got %s", result.String())
	}
}

func TestCompileFunctionHIR_ReadsObjectFromDefaultContext(t *testing.T) {
	ctx := jscontext.Default()

	obj := jsvalue.NewObject()
	obj.Set("name", jsvalue.NewString("gun"))
	obj.Set("version", jsvalue.NewNumber(1))
	ctx.Set("config", obj)

	fn := dynfunc.CompileFunctionHIR(
		jsvalue.NewString(""),
		jsvalue.NewString("return config.name"),
	)
	result := fn.Call()
	if result.String() != "gun" {
		t.Fatalf("expected 'gun', got %s", result.String())
	}
}

func TestCompileFunctionHIR_ReadsFunctionFromDefaultContext(t *testing.T) {
	ctx := jscontext.Default()

	doubler := jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		return jsvalue.NewNumber(args[0].Number() * 2)
	})
	ctx.Set("doubler", doubler)

	fn := dynfunc.CompileFunctionHIR(
		jsvalue.NewString("x"),
		jsvalue.NewString("return doubler(x)"),
	)
	result := fn.Call(jsvalue.NewNumber(21))
	if result.String() != "42" {
		t.Fatalf("expected 42, got %s", result.String())
	}
}

// --- new Function() modifies default context via globalThis ---

func TestCompileFunctionHIR_WritesToDefaultContextViaGlobalThis(t *testing.T) {
	ctx := jscontext.Default()
	defer ctx.Set("funcWriteTest", jsvalue.NewUndefined())

	fn := dynfunc.CompileFunctionHIR(
		jsvalue.NewString("x"),
		jsvalue.NewString("globalThis.funcWriteTest = x"),
	)
	fn.Call(jsvalue.NewNumber(77))

	got := ctx.Get("funcWriteTest")
	if got == nil || got.String() != "77" {
		t.Fatalf("expected 77 in default context, got %s", safeString(got))
	}
}

func TestCompileFunctionHIR_UpdatesExistingGlobalViaGlobalThis(t *testing.T) {
	ctx := jscontext.Default()
	defer ctx.Set("funcMutTest", jsvalue.NewUndefined())

	ctx.Set("funcMutTest", jsvalue.NewNumber(10))

	fn := dynfunc.CompileFunctionHIR(
		jsvalue.NewString("delta"),
		jsvalue.NewString("globalThis.funcMutTest = globalThis.funcMutTest + delta"),
	)
	fn.Call(jsvalue.NewNumber(5))

	got := ctx.Get("funcMutTest")
	if got == nil || got.String() != "15" {
		t.Fatalf("expected 15, got %s", safeString(got))
	}
}

// --- eval and new Function share same context ---

func TestEvalAndFunctionShareContext(t *testing.T) {
	ctx := jscontext.Default()
	defer ctx.Set("sharedVar", jsvalue.NewUndefined())

	// Function writes to global via globalThis
	writeFn := dynfunc.CompileFunctionHIR(
		jsvalue.NewString("val"),
		jsvalue.NewString("globalThis.sharedVar = val"),
	)
	writeFn.Call(jsvalue.NewString("hello"))

	// Eval reads what Function wrote
	result := dynfunc.EvalHIR(nil, jsvalue.NewString("sharedVar"))
	if result.String() != "hello" {
		t.Fatalf("expected 'hello', got %s", result.String())
	}
}

func TestFunctionWrites_EvalWrites_FunctionReads(t *testing.T) {
	ctx := jscontext.Default()
	defer ctx.Set("pingPong", jsvalue.NewUndefined())

	// Step 1: Function writes via globalThis
	writeFn := dynfunc.CompileFunctionHIR(
		jsvalue.NewString(""),
		jsvalue.NewString("globalThis.pingPong = 1"),
	)
	writeFn.Call()

	// Step 2: Eval writes via globalThis
	dynfunc.EvalHIR(nil, jsvalue.NewString("globalThis.pingPong = globalThis.pingPong + 1"))

	// Step 3: Another Function reads
	readFn := dynfunc.CompileFunctionHIR(
		jsvalue.NewString(""),
		jsvalue.NewString("return pingPong"),
	)
	result := readFn.Call()
	if result.String() != "2" {
		t.Fatalf("expected 2, got %s", result.String())
	}
}

// --- helper ---

func safeString(v *jsvalue.JSValue) string {
	if v == nil {
		return "<nil>"
	}
	return v.String()
}
