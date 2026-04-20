package jsvalue

import "testing"

func expectTypeError(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic")
		}
		err, ok := r.(*JSValue)
		if !ok {
			t.Fatalf("expected *JSValue panic, got %T", r)
		}
		if got := err.Get("name").String(); got != "TypeError" {
			t.Fatalf("panic name = %q, want TypeError", got)
		}
	}()
	fn()
}

func TestObjectBoxesPrimitives(t *testing.T) {
	bool1 := Object.Call(NewBool(true))
	bool2 := Object.Call(NewBool(true))
	if bool1 == bool2 {
		t.Fatal("Object(true) should return a fresh wrapper each time")
	}
	if got := bool1.MethodCall("valueOf"); got != NewBool(true) {
		t.Fatalf("Object(true).valueOf() = %v, want true singleton", got)
	}
	if bool1.Get("constructor") != Globals()["Boolean"] {
		t.Fatal("Object(true).constructor must be Boolean")
	}

	num1 := Object.Call(NewNumber(42))
	num2 := Object.Call(NewNumber(42))
	if num1 == num2 {
		t.Fatal("Object(42) should return a fresh wrapper each time")
	}
	if got := num1.MethodCall("valueOf").Number(); got != 42 {
		t.Fatalf("Object(42).valueOf() = %v, want 42", got)
	}
	if num1.Get("constructor") != Number {
		t.Fatal("Object(42).constructor must be Number")
	}

	str1 := Object.Call(NewString("ab"))
	str2 := Object.Call(NewString("ab"))
	if str1 == str2 {
		t.Fatal(`Object("ab") should return a fresh wrapper each time`)
	}
	if got := str1.MethodCall("valueOf").String(); got != "ab" {
		t.Fatalf(`Object("ab").valueOf() = %q, want "ab"`, got)
	}
	if str1.Get("constructor") != Globals()["String"] {
		t.Fatal(`Object("ab").constructor must be String`)
	}
	keys := Keys(str1).Array()
	if len(keys) != 2 || keys[0].String() != "0" || keys[1].String() != "1" {
		t.Fatalf(`Object.keys(Object("ab")) = %#v, want ["0", "1"]`, keys)
	}
}

func TestPrimitiveMethodCallsStillWork(t *testing.T) {
	if got := NewBool(true).MethodCall("toString").String(); got != "true" {
		t.Fatalf("true.toString() = %q, want true", got)
	}
	if got := NewNumber(42).MethodCall("toFixed", NewNumber(1)).String(); got != "42.0" {
		t.Fatalf("(42).toFixed(1) = %q, want 42.0", got)
	}
	if got := NewString("ab").MethodCall("valueOf").String(); got != "ab" {
		t.Fatalf(`"ab".valueOf() = %q, want "ab"`, got)
	}
}

func TestPrimitiveMutationThrowsTypeError(t *testing.T) {
	expectTypeError(t, func() { NewBool(true).Set("x", NewNumber(1)) })
	expectTypeError(t, func() { NewNumber(42).Set("x", NewNumber(1)) })
	expectTypeError(t, func() { NewString("").Set("x", NewNumber(1)) })
	expectTypeError(t, func() {
		DefineProperty(NewBool(true), NewString("x"), ObjectFrom("value", NewNumber(1)))
	})
	expectTypeError(t, func() {
		Reflect.Get("set").Call(NewBool(true), NewString("x"), NewNumber(1))
	})
	if got := Object.Get("setPrototypeOf").Call(NewBool(true), NewObject()); got != NewBool(true) {
		t.Fatal("Object.setPrototypeOf(true, {}) must return the original primitive")
	}
}

func TestObjectKeysOnPrimitiveInputs(t *testing.T) {
	strKeys := Object.Get("keys").Call(NewString("ab")).Array()
	if len(strKeys) != 2 || strKeys[0].String() != "0" || strKeys[1].String() != "1" {
		t.Fatalf(`Object.keys("ab") = %#v, want ["0", "1"]`, strKeys)
	}
	if got := len(Object.Get("keys").Call(NewNumber(42)).Array()); got != 0 {
		t.Fatalf("Object.keys(42) len = %d, want 0", got)
	}
	if got := len(Object.Get("keys").Call(NewBool(true)).Array()); got != 0 {
		t.Fatalf("Object.keys(true) len = %d, want 0", got)
	}
}
