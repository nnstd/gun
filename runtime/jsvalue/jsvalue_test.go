package jsvalue

import (
	"fmt"
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
	if v := From(nil); v.typ != TypeUndefined {
		t.Errorf("From(nil): got type=%v, want TypeUndefined", v.typ)
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

func TestParseIntDecimal(t *testing.T) {
	r := ParseInt(NewString("42"), NewNumber(10))
	if int(r.Number()) != 42 {
		t.Errorf("ParseInt(\"42\", 10): got %v, want 42", r.Number())
	}
}

func TestParseIntHex(t *testing.T) {
	r := ParseInt(NewString("0xff"), NewNumber(16))
	if int(r.Number()) != 255 {
		t.Errorf("ParseInt(\"0xff\", 16): got %v, want 255", r.Number())
	}
}

func TestParseIntStopsAtInvalidChar(t *testing.T) {
	r := ParseInt(NewString("123abc"), NewNumber(10))
	if int(r.Number()) != 123 {
		t.Errorf("ParseInt(\"123abc\", 10): got %v, want 123", r.Number())
	}
}

func TestParseIntNegative(t *testing.T) {
	r := ParseInt(NewString("-10"), NewNumber(10))
	if int(r.Number()) != -10 {
		t.Errorf("ParseInt(\"-10\", 10): got %v, want -10", r.Number())
	}
}

func TestParseIntInvalidReturnsNaN(t *testing.T) {
	r := ParseInt(NewString("abc"), NewNumber(10))
	if !math.IsNaN(r.Number()) {
		t.Errorf("ParseInt(\"abc\", 10): got %v, want NaN", r.Number())
	}
}

func TestParseFloatBasic(t *testing.T) {
	r := ParseFloat(NewString("3.14"))
	if r.Number() != 3.14 {
		t.Errorf("ParseFloat(\"3.14\"): got %v, want 3.14", r.Number())
	}
}

func TestParseFloatInvalidReturnsNaN(t *testing.T) {
	r := ParseFloat(NewString("abc"))
	if !math.IsNaN(r.Number()) {
		t.Errorf("ParseFloat(\"abc\"): got %v, want NaN", r.Number())
	}
}

// ---------------------------------------------------------------------------
// NewArray nil vs empty slice
// ---------------------------------------------------------------------------

func TestNewArrayEmptyString(t *testing.T) {
	// NewArray() with no args should produce a non-nil arrayVal so that
	// String() returns "" (array join with no elements) instead of "[object Object]".
	arr := NewArray()
	got := arr.String()
	if got != "" {
		t.Errorf("NewArray().String(): got %q, want %q", got, "")
	}
}

func TestNewArrayNonNilSlice(t *testing.T) {
	// NewArray() should produce a non-nil underlying slice so that
	// Array() returns a usable (non-nil) slice.
	arr := NewArray()
	if arr.Array() == nil {
		t.Error("NewArray().Array() should not be nil")
	}
}

// ---------------------------------------------------------------------------
// From() for arbitrary Go functions
// ---------------------------------------------------------------------------

func TestFromGoFunction(t *testing.T) {
	// From(fmt.Sprintf) should create a callable function JSValue,
	// not a string representation.
	v := From(fmt.Sprintf)
	if v.typ != TypeFunction {
		t.Fatalf("From(fmt.Sprintf): got type %v, want TypeFunction", v.typ)
	}
	if v.funcVal == nil {
		t.Fatal("From(fmt.Sprintf): funcVal should not be nil")
	}
}

func TestFromGoFunctionCall(t *testing.T) {
	// Wrapping fmt.Sprintf and calling it through JSValue should work.
	format := From(fmt.Sprintf)
	result := format.Call(NewString("hello %s"), NewString("world"))
	if result.String() != "hello world" {
		t.Errorf("From(fmt.Sprintf).Call(\"hello %%s\", \"world\"): got %q, want %q",
			result.String(), "hello world")
	}
}

// ---------------------------------------------------------------------------
// Function.prototype.apply for non-method functions
// ---------------------------------------------------------------------------

func TestFunctionApplyNonMethod(t *testing.T) {
	// A non-method Go function wrapped via From should work with .apply().
	// For non-method functions, the thisArg should be skipped.
	format := From(fmt.Sprintf)
	argsArray := NewArray(NewString("hello %s"), NewString("world"))
	// apply is called as: format.MethodCall("apply", thisArg, argsArray)
	// For non-method functions, thisArg should be ignored and only argsArray used.
	result := format.MethodCall("apply", format, argsArray)
	if result.String() != "hello world" {
		t.Errorf("apply on non-method function: got %q, want %q",
			result.String(), "hello world")
	}
}

// ---------------------------------------------------------------------------
// String.prototype.normalize
// ---------------------------------------------------------------------------

func TestStringNormalize(t *testing.T) {
	// String.prototype.normalize should return the string as-is for basic ASCII.
	v := NewString("hello")
	result := v.MethodCall("normalize")
	if result.String() != "hello" {
		t.Errorf("NewString(\"hello\").MethodCall(\"normalize\"): got %q, want %q",
			result.String(), "hello")
	}
}

func TestStringNormalizeEmpty(t *testing.T) {
	// normalize on an empty string should return "".
	v := NewString("")
	result := v.MethodCall("normalize")
	if result.String() != "" {
		t.Errorf("NewString(\"\").MethodCall(\"normalize\"): got %q, want %q",
			result.String(), "")
	}
}

func TestArrayAt(t *testing.T) {
	arr := NewArray(NewString("a"), NewString("b"), NewString("c"))
	// at(0) → "a"
	if arr.MethodCall("at", NewNumber(0)).String() != "a" {
		t.Errorf("at(0): got %q, want %q", arr.MethodCall("at", NewNumber(0)).String(), "a")
	}
	// at(-1) → "c"
	if arr.MethodCall("at", NewNumber(-1)).String() != "c" {
		t.Errorf("at(-1): got %q, want %q", arr.MethodCall("at", NewNumber(-1)).String(), "c")
	}
}

func TestArrayEntries(t *testing.T) {
	arr := NewArray(NewString("x"), NewString("y"))
	entries := arr.MethodCall("entries")
	if entries.Len() != 2 {
		t.Fatalf("entries len: got %d, want 2", entries.Len())
	}
	e0 := entries.Index(0)
	if e0.Index(0).Number() != 0 || e0.Index(1).String() != "x" {
		t.Errorf("entry[0]: got [%v, %q], want [0, \"x\"]", e0.Index(0).Number(), e0.Index(1).String())
	}
	e1 := entries.Index(1)
	if e1.Index(0).Number() != 1 || e1.Index(1).String() != "y" {
		t.Errorf("entry[1]: got [%v, %q], want [1, \"y\"]", e1.Index(0).Number(), e1.Index(1).String())
	}
}

func TestArraySetNumericKey(t *testing.T) {
	arr := NewArray(NewString("a"), NewString("b"))
	arr.Set("0", NewString("x"))
	if arr.Index(0).String() != "x" {
		t.Errorf("Set(\"0\"): got %q, want %q", arr.Index(0).String(), "x")
	}
	arr.Set("3", NewString("d"))
	if arr.Len() != 4 {
		t.Errorf("after Set(\"3\"): len=%d, want 4", arr.Len())
	}
	if arr.Index(3).String() != "d" {
		t.Errorf("Set(\"3\"): got %q, want %q", arr.Index(3).String(), "d")
	}
}

func TestIncludesString(t *testing.T) {
	s := NewString("hello world")
	if !Includes(s, NewString("world")).Bool() {
		t.Error("String.includes('world') should be true")
	}
	if Includes(s, NewString("xyz")).Bool() {
		t.Error("String.includes('xyz') should be false")
	}
}

func TestIncludesArrayValueComparison(t *testing.T) {
	// Array includes should use value comparison, not pointer comparison
	arr := NewArray(NewString("help"), NewString("version"))
	if !Includes(arr, NewString("help")).Bool() {
		t.Error("Array.includes(NewString('help')) should be true with value comparison")
	}
	if Includes(arr, NewString("missing")).Bool() {
		t.Error("Array.includes(NewString('missing')) should be false")
	}
}

func TestArraySort(t *testing.T) {
	arr := NewArray(NewString("c"), NewString("a"), NewString("b"))
	result := arr.MethodCall("sort")
	if result.Index(0).String() != "a" || result.Index(1).String() != "b" || result.Index(2).String() != "c" {
		t.Errorf("sort: got [%q, %q, %q], want [a, b, c]",
			result.Index(0).String(), result.Index(1).String(), result.Index(2).String())
	}
}

func TestSpreadIntoArray(t *testing.T) {
	// String spread: [...str] splits into characters
	elems := SpreadIntoArray(NewString("abc"))
	if len(elems) != 3 {
		t.Fatalf("SpreadIntoArray('abc'): len=%d, want 3", len(elems))
	}
	if elems[0].String() != "a" || elems[1].String() != "b" || elems[2].String() != "c" {
		t.Errorf("SpreadIntoArray('abc'): got %v", elems)
	}
	// Array spread: [...arr] returns elements
	arr := NewArray(NewNumber(1), NewNumber(2))
	aElems := SpreadIntoArray(arr)
	if len(aElems) != 2 {
		t.Fatalf("SpreadIntoArray(arr): len=%d, want 2", len(aElems))
	}
}

func TestNumberToString(t *testing.T) {
	n := NewNumber(42)
	result := n.MethodCall("toString")
	if result.String() != "42" {
		t.Errorf("(42).toString(): got %q, want %q", result.String(), "42")
	}
}

func TestStringTrimLeftRight(t *testing.T) {
	s := NewString("  hello  ")

	// trimLeft is an alias for trimStart
	r := s.MethodCall("trimLeft")
	if r.String() != "hello  " {
		t.Errorf("trimLeft: got %q, want %q", r.String(), "hello  ")
	}

	// trimRight is an alias for trimEnd
	r = s.MethodCall("trimRight")
	if r.String() != "  hello" {
		t.Errorf("trimRight: got %q, want %q", r.String(), "  hello")
	}

	// trimStart and trimEnd should produce the same results
	r = s.MethodCall("trimStart")
	if r.String() != "hello  " {
		t.Errorf("trimStart: got %q, want %q", r.String(), "hello  ")
	}
	r = s.MethodCall("trimEnd")
	if r.String() != "  hello" {
		t.Errorf("trimEnd: got %q, want %q", r.String(), "  hello")
	}
}

func TestStringTrimLeftRightTabs(t *testing.T) {
	s := NewString("\t\n hello \t\n")

	r := s.MethodCall("trimLeft")
	if r.String() != "hello \t\n" {
		t.Errorf("trimLeft with tabs/newlines: got %q, want %q", r.String(), "hello \t\n")
	}

	r = s.MethodCall("trimRight")
	if r.String() != "\t\n hello" {
		t.Errorf("trimRight with tabs/newlines: got %q, want %q", r.String(), "\t\n hello")
	}
}

func TestStringIndexAccess(t *testing.T) {
	s := NewString("hello")
	if s.Index(0).String() != "h" {
		t.Errorf("Index(0): got %q, want %q", s.Index(0).String(), "h")
	}
	if s.Index(4).String() != "o" {
		t.Errorf("Index(4): got %q, want %q", s.Index(4).String(), "o")
	}
	if TypeOf(s.Index(5)).String() != "undefined" {
		t.Errorf("Index(5): expected undefined, got %v", s.Index(5))
	}
}

func TestStringSliceMethod(t *testing.T) {
	s := NewString("-u")

	// slice(-1) should return "u"
	r := s.MethodCall("slice", NewNumber(-1))
	if r.String() != "u" {
		t.Errorf("slice(-1): got %q, want %q", r.String(), "u")
	}

	// slice(1, -1) on a 2-char string should return ""
	r = s.MethodCall("slice", NewNumber(1), NewNumber(-1))
	if r.String() != "" {
		t.Errorf("slice(1, -1): got %q, want %q", r.String(), "")
	}
}

func TestFilterPassesArrayAsThirdArg(t *testing.T) {
	arr := NewArray(NewString("a"), NewString("b"), NewString("a"), NewString("c"))
	// Dedup using self.indexOf(v) === i
	result := arr.MethodCall("filter", NewFunction(func(args ...*JSValue) *JSValue {
		v := args[0]
		i := args[1]
		self := args[2]
		return Eq(From(self.MethodCall("indexOf", From(v))), From(i))
	}))
	if result.Len() != 3 {
		t.Errorf("dedup filter: got len %d, want 3", result.Len())
	}
	if result.Index(0).String() != "a" || result.Index(1).String() != "b" || result.Index(2).String() != "c" {
		t.Errorf("dedup filter: got %v, want [a, b, c]", result)
	}
}

func TestForEachPassesArrayAsThirdArg(t *testing.T) {
	arr := NewArray(NewString("x"))
	var gotSelf *JSValue
	arr.MethodCall("forEach", NewFunction(func(args ...*JSValue) *JSValue {
		if len(args) > 2 {
			gotSelf = args[2]
		}
		return nil
	}))
	if gotSelf == nil || gotSelf.Len() != 1 {
		t.Errorf("forEach third arg: expected array, got %v", gotSelf)
	}
}

func TestMapPassesArrayAsThirdArg(t *testing.T) {
	arr := NewArray(NewNumber(1), NewNumber(2))
	var gotSelf *JSValue
	arr.MethodCall("map", NewFunction(func(args ...*JSValue) *JSValue {
		if len(args) > 2 {
			gotSelf = args[2]
		}
		return args[0]
	}))
	if gotSelf == nil || gotSelf.Len() != 2 {
		t.Errorf("map third arg: expected array, got %v", gotSelf)
	}
}
