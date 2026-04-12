package jsvalue

import "testing"

func TestObjectGlobalExports(t *testing.T) {
	if Object.TypeString() != "function" {
		t.Fatal("expected Object global to be callable")
	}
	if got := Object.Get("keys").Call(ObjectFrom("a", NewNumber(1))).Len(); got != 1 {
		t.Fatalf("Object.keys len = %d, want 1", got)
	}
}

func TestArrayGlobalExports(t *testing.T) {
	arr := Array.Call(NewNumber(1), NewNumber(2))
	if !arr.IsArray() {
		t.Fatal("expected Array() to return array")
	}
	if !Array.Get("isArray").Call(arr).Bool() {
		t.Fatal("expected Array.isArray to return true")
	}
}

func TestNumberGlobalExports(t *testing.T) {
	if got := Number.Call(NewString("42")).Number(); got != 42 {
		t.Fatalf("Number(\"42\") = %v, want 42", got)
	}
	if !Number.Get("isSafeInteger").Call(NewNumber(42)).Bool() {
		t.Fatal("expected Number.isSafeInteger(42) to be true")
	}
}
