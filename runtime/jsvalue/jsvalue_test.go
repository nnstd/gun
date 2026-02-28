package jsvalue

import (
	"math"
	"testing"
)

func TestFromMap(t *testing.T) {
	m := map[string]*JSValue{
		"name": NewString("go"),
		"ver":  NewInt(1),
	}
	v := From(m)
	if v.typ != TypeObject {
		t.Fatalf("expected TypeObject, got %v", v.typ)
	}
	if got := v.Get("name").String(); got != "go" {
		t.Errorf("name: got %q, want %q", got, "go")
	}
	if got := int(v.Get("ver").Number()); got != 1 {
		t.Errorf("ver: got %d, want 1", got)
	}
}

func TestFromPassthrough(t *testing.T) {
	orig := NewString("hello")
	v := From(orig)
	if v != orig {
		t.Error("From(*JSValue) should return the same pointer")
	}
}

func TestFromPrimitives(t *testing.T) {
	if v := From(42); v.typ != TypeNumber || int(v.Number()) != 42 {
		t.Errorf("From(int): got type=%v num=%v", v.typ, v.Number())
	}
	if v := From(3.14); v.typ != TypeNumber || v.Number() != 3.14 {
		t.Errorf("From(float64): got type=%v num=%v", v.typ, v.Number())
	}
	if v := From(true); v.typ != TypeBoolean || !v.Bool() {
		t.Errorf("From(bool): got type=%v bool=%v", v.typ, v.Bool())
	}
	if v := From("hi"); v.typ != TypeString || v.String() != "hi" {
		t.Errorf("From(string): got type=%v str=%v", v.typ, v.String())
	}
	if v := From(nil); v.typ != TypeNull {
		t.Errorf("From(nil): got type=%v, want TypeNull", v.typ)
	}
}

// ---------------------------------------------------------------------------
// Arithmetic operations
// ---------------------------------------------------------------------------

func TestAdd(t *testing.T) {
	// Numeric addition
	r := Add(NewNumber(2), NewNumber(3))
	if r.Number() != 5 {
		t.Errorf("Add(2,3): got %v, want 5", r.Number())
	}
	// String concatenation
	r = Add(NewString("hello"), NewString(" world"))
	if r.String() != "hello world" {
		t.Errorf("Add strings: got %q", r.String())
	}
	// Mixed: string + number → string
	r = Add(NewString("n="), NewNumber(42))
	if r.String() != "n=42" {
		t.Errorf("Add string+num: got %q", r.String())
	}
	// Nil safety
	r = Add(nil, NewNumber(5))
	if math.IsNaN(r.Number()) {
		// undefined + 5 = NaN in JS
	}
}

func TestSub(t *testing.T) {
	r := Sub(NewNumber(10), NewNumber(3))
	if r.Number() != 7 {
		t.Errorf("Sub(10,3): got %v, want 7", r.Number())
	}
}

func TestMul(t *testing.T) {
	r := Mul(NewNumber(4), NewNumber(5))
	if r.Number() != 20 {
		t.Errorf("Mul(4,5): got %v, want 20", r.Number())
	}
}

func TestDiv(t *testing.T) {
	r := Div(NewNumber(10), NewNumber(3))
	if math.Abs(r.Number()-10.0/3.0) > 1e-10 {
		t.Errorf("Div(10,3): got %v", r.Number())
	}
	// Division by zero
	r = Div(NewNumber(1), NewNumber(0))
	if !math.IsInf(r.Number(), 1) {
		t.Errorf("Div(1,0): got %v, want +Inf", r.Number())
	}
	r = Div(NewNumber(-1), NewNumber(0))
	if !math.IsInf(r.Number(), -1) {
		t.Errorf("Div(-1,0): got %v, want -Inf", r.Number())
	}
	r = Div(NewNumber(0), NewNumber(0))
	if !math.IsNaN(r.Number()) {
		t.Errorf("Div(0,0): got %v, want NaN", r.Number())
	}
}

func TestMod(t *testing.T) {
	r := Mod(NewNumber(10), NewNumber(3))
	if r.Number() != 1 {
		t.Errorf("Mod(10,3): got %v, want 1", r.Number())
	}
}

func TestBitNot(t *testing.T) {
	r := BitNot(NewNumber(0))
	if r.Int() != -1 {
		t.Errorf("BitNot(0): got %v, want -1", r.Int())
	}
	r = BitNot(NewNumber(-1))
	if r.Int() != 0 {
		t.Errorf("BitNot(-1): got %v, want 0", r.Int())
	}
}

func TestBitwise(t *testing.T) {
	if BitAnd(NewNumber(5), NewNumber(3)).Int() != 1 {
		t.Error("BitAnd(5,3) should be 1")
	}
	if BitOr(NewNumber(5), NewNumber(3)).Int() != 7 {
		t.Error("BitOr(5,3) should be 7")
	}
	if BitXor(NewNumber(5), NewNumber(3)).Int() != 6 {
		t.Error("BitXor(5,3) should be 6")
	}
	if Shl(NewNumber(1), NewNumber(3)).Int() != 8 {
		t.Error("Shl(1,3) should be 8")
	}
	if Shr(NewNumber(8), NewNumber(2)).Int() != 2 {
		t.Error("Shr(8,2) should be 2")
	}
}

// ---------------------------------------------------------------------------
// Comparison operations
// ---------------------------------------------------------------------------

func TestEq(t *testing.T) {
	if !Eq(NewNumber(1), NewNumber(1)).Bool() {
		t.Error("Eq(1,1) should be true")
	}
	if Eq(NewNumber(1), NewNumber(2)).Bool() {
		t.Error("Eq(1,2) should be false")
	}
	if Eq(NewNumber(1), NewString("1")).Bool() {
		t.Error("Eq(1,'1') should be false (strict equality)")
	}
	if !Eq(NewString("a"), NewString("a")).Bool() {
		t.Error("Eq('a','a') should be true")
	}
	if !Eq(NewBool(true), NewBool(true)).Bool() {
		t.Error("Eq(true,true) should be true")
	}
	if !Eq(NewNull(), NewNull()).Bool() {
		t.Error("Eq(null,null) should be true")
	}
	if !Eq(NewUndefined(), NewUndefined()).Bool() {
		t.Error("Eq(undefined,undefined) should be true")
	}
	if Eq(NewNull(), NewUndefined()).Bool() {
		t.Error("Eq(null,undefined) should be false (strict)")
	}
}

func TestNEq(t *testing.T) {
	if !NEq(NewNumber(1), NewNumber(2)).Bool() {
		t.Error("NEq(1,2) should be true")
	}
	if NEq(NewNumber(1), NewNumber(1)).Bool() {
		t.Error("NEq(1,1) should be false")
	}
}

func TestLtGt(t *testing.T) {
	if !Lt(NewNumber(1), NewNumber(2)).Bool() {
		t.Error("Lt(1,2) should be true")
	}
	if !Gt(NewNumber(2), NewNumber(1)).Bool() {
		t.Error("Gt(2,1) should be true")
	}
	if !LtE(NewNumber(1), NewNumber(1)).Bool() {
		t.Error("LtE(1,1) should be true")
	}
	if !GtE(NewNumber(2), NewNumber(2)).Bool() {
		t.Error("GtE(2,2) should be true")
	}
	// String comparisons
	if !Lt(NewString("a"), NewString("b")).Bool() {
		t.Error("Lt('a','b') should be true")
	}
}

// ---------------------------------------------------------------------------
// Logical operations
// ---------------------------------------------------------------------------

func TestNot(t *testing.T) {
	if !Not(NewBool(false)).Bool() {
		t.Error("Not(false) should be true")
	}
	if Not(NewBool(true)).Bool() {
		t.Error("Not(true) should be false")
	}
	if !Not(NewNumber(0)).Bool() {
		t.Error("Not(0) should be true")
	}
	if !Not(NewString("")).Bool() {
		t.Error("Not('') should be true")
	}
}

func TestAndOr(t *testing.T) {
	// And returns first falsy or last truthy
	r := And(NewNumber(1), NewString("yes"))
	if r.String() != "yes" {
		t.Errorf("And(1,'yes'): got %q", r.String())
	}
	r = And(NewNumber(0), NewString("yes"))
	if r.Number() != 0 {
		t.Errorf("And(0,'yes'): got %v", r.Number())
	}
	// Or returns first truthy or last falsy
	r = Or(NewNumber(0), NewString("fallback"))
	if r.String() != "fallback" {
		t.Errorf("Or(0,'fallback'): got %q", r.String())
	}
	r = Or(NewNumber(1), NewString("fallback"))
	if r.Number() != 1 {
		t.Errorf("Or(1,'fallback'): got %v", r.Number())
	}
}

func TestNullish(t *testing.T) {
	r := Nullish(NewNull(), NewString("default"))
	if r.String() != "default" {
		t.Errorf("Nullish(null,'default'): got %q", r.String())
	}
	r = Nullish(NewUndefined(), NewString("default"))
	if r.String() != "default" {
		t.Errorf("Nullish(undefined,'default'): got %q", r.String())
	}
	r = Nullish(NewNumber(0), NewString("default"))
	if r.Number() != 0 {
		t.Error("Nullish(0,'default') should return 0, not default")
	}
}

func TestIncDec(t *testing.T) {
	if Inc(NewNumber(5)).Number() != 6 {
		t.Error("Inc(5) should be 6")
	}
	if Dec(NewNumber(5)).Number() != 4 {
		t.Error("Dec(5) should be 4")
	}
}

func TestTypeOf(t *testing.T) {
	if TypeOf(NewNumber(1)).String() != "number" {
		t.Error("TypeOf(1) should be 'number'")
	}
	if TypeOf(NewString("")).String() != "string" {
		t.Error("TypeOf('') should be 'string'")
	}
	if TypeOf(NewBool(true)).String() != "boolean" {
		t.Error("TypeOf(true) should be 'boolean'")
	}
	if TypeOf(nil).String() != "undefined" {
		t.Error("TypeOf(nil) should be 'undefined'")
	}
	if TypeOf(NewNull()).String() != "object" {
		t.Error("TypeOf(null) should be 'object'")
	}
}

// ---------------------------------------------------------------------------
// Updated signature tests
// ---------------------------------------------------------------------------

func TestReplaceWithRegex(t *testing.T) {
	// Replace with regex JSValue
	r := Replace(NewString("abc123def"), NewString("123"), NewString("XXX"))
	if r.String() != "abcXXXdef" {
		t.Errorf("Replace string: got %q", r.String())
	}
}

func TestSliceString(t *testing.T) {
	r := Slice(NewString("hello"), NewNumber(1))
	if r.String() != "ello" {
		t.Errorf("Slice('hello',1): got %q", r.String())
	}
	r = Slice(NewString("hello"), NewNumber(1), NewNumber(3))
	if r.String() != "el" {
		t.Errorf("Slice('hello',1,3): got %q", r.String())
	}
}

func TestSliceArray(t *testing.T) {
	arr := NewArray(NewNumber(1), NewNumber(2), NewNumber(3), NewNumber(4))
	r := Slice(arr, NewNumber(1), NewNumber(3))
	if r.Len() != 2 {
		t.Errorf("Slice array: got len %d, want 2", r.Len())
	}
}

func TestSplitJSValue(t *testing.T) {
	r := Split(NewString("a.b.c"), NewString("."))
	if r.Len() != 3 {
		t.Errorf("Split: got len %d, want 3", r.Len())
	}
}

func TestJoinJSValue(t *testing.T) {
	arr := NewArray(NewString("a"), NewString("b"), NewString("c"))
	r := Join(arr, NewString(","))
	if r.String() != "a,b,c" {
		t.Errorf("Join: got %q", r.String())
	}
}

func TestSubstringJSValue(t *testing.T) {
	r := Substring(NewString("hello"), NewNumber(1), NewNumber(3))
	if r.String() != "el" {
		t.Errorf("Substring: got %q", r.String())
	}
}

func TestCharAtJSValue(t *testing.T) {
	r := CharAt(NewString("hello"), NewNumber(1))
	if r.String() != "e" {
		t.Errorf("CharAt: got %q", r.String())
	}
}

func TestLastIndexOfJSValue(t *testing.T) {
	r := LastIndexOf(NewString("hello world hello"), NewString("hello"))
	if int(r.Number()) != 12 {
		t.Errorf("LastIndexOf: got %v", r.Number())
	}
}
