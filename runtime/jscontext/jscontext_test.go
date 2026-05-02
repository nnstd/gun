package jscontext

import (
	"testing"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
)

func TestDefaultSingleton(t *testing.T) {
	a := Default()
	b := Default()
	if a != b {
		t.Fatal("Default() should return same pointer")
	}
	if a.Global() == nil {
		t.Fatal("Global() should not be nil")
	}
}

func TestNewIsolation(t *testing.T) {
	a := New()
	b := New()
	if a == b {
		t.Fatal("New() should return independent contexts")
	}
	a.Set("test", jsvalue.NewString("hello"))
	if got := b.Get("test"); got != nil && got.Type() != jsvalue.TypeUndefined {
		t.Fatal("Isolated context should not see other context's values")
	}
}

func TestRegisterBuiltins(t *testing.T) {
	ctx := New()
	ctx.RegisterBuiltins()
	obj := ctx.Get("Object")
	if obj == nil {
		t.Fatal("RegisterBuiltins should set Object")
	}
	parseInt := ctx.Get("parseInt")
	if parseInt == nil {
		t.Fatal("RegisterBuiltins should set parseInt")
	}
}

func TestSetGetRoundtrip(t *testing.T) {
	ctx := New()
	val := jsvalue.NewNumber(42)
	ctx.Set("x", val)
	got := ctx.Get("x")
	if got == nil {
		t.Fatal("Get should return value set by Set")
	}
	if got.Number() != 42 {
		t.Fatalf("expected 42, got %v", got.Number())
	}
}

func TestGlobalThisIdentity(t *testing.T) {
	ctx := Default()
	globalThis := ctx.Get("globalThis")
	global := ctx.Get("global")
	if globalThis == nil {
		t.Fatal("globalThis should be set on default context")
	}
	if global == nil {
		t.Fatal("global should be set on default context")
	}
	if globalThis != ctx.Global() {
		t.Fatal("globalThis should be the same object as Global()")
	}
	if global != ctx.Global() {
		t.Fatal("global should be the same object as Global()")
	}
	if globalThis != global {
		t.Fatal("globalThis === global should be true (same pointer)")
	}
}
