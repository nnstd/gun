package jsvalue

import (
	"math"
	"testing"
)

func TestBoolInterning(t *testing.T) {
	if NewBool(true) != NewBool(true) {
		t.Fatal("NewBool(true) must return the same pointer on repeated calls")
	}
	if NewBool(false) != NewBool(false) {
		t.Fatal("NewBool(false) must return the same pointer on repeated calls")
	}
	if NewBool(true) == NewBool(false) {
		t.Fatal("NewBool(true) and NewBool(false) must be distinct")
	}
}

func TestNumberInterning(t *testing.T) {
	cases := []int{-128, -1, 0, 1, 42, 255}
	for _, n := range cases {
		a := NewNumber(float64(n))
		b := NewNumber(float64(n))
		if a != b {
			t.Fatalf("NewNumber(%d) not interned: %p vs %p", n, a, b)
		}
	}
	out := []int{-129, 256, 1000}
	for _, n := range out {
		a := NewNumber(float64(n))
		b := NewNumber(float64(n))
		if a == b {
			t.Fatalf("NewNumber(%d) should not be interned (out of range)", n)
		}
	}
	// Non-integer: must not be interned
	if NewNumber(1.5) == NewNumber(1.5) {
		t.Fatal("NewNumber(1.5) should not be interned")
	}
}

func TestEmptyStringInterning(t *testing.T) {
	if NewString("") != NewString("") {
		t.Fatal(`NewString("") must return the same pointer`)
	}
	if NewString("a") == NewString("a") {
		t.Fatal(`NewString("a") should not be interned`)
	}
}

func TestCachedValueImmutability(t *testing.T) {
	t1 := NewBool(true)
	t1.Set("foo", NewNumber(42))
	got := t1.Get("foo")
	if got != nil && got.typ == TypeNumber && got.Number() == 42 {
		t.Fatal("frozen NewBool(true) should ignore .Set(); got Number(42) back")
	}
	// Shared singleton must be unchanged
	if NewBool(true) != t1 {
		t.Fatal("singleton identity must not change after rejected mutation")
	}

	n := NewNumber(5)
	n.Set("x", NewNumber(1))
	got2 := n.Get("x")
	if got2 != nil && got2.typ == TypeNumber && got2.Number() == 1 {
		t.Fatal("frozen NewNumber(5) should ignore .Set()")
	}

	s := NewString("")
	s.Set("y", NewNumber(2))
	got3 := s.Get("y")
	if got3 != nil && got3.typ == TypeNumber && got3.Number() == 2 {
		t.Fatal("frozen NewString(\"\") should ignore .Set()")
	}
}

func TestCachedNumberArithmetic(t *testing.T) {
	sum := Add(NewNumber(1), NewNumber(2))
	if sum.Number() != 3 {
		t.Fatalf("Add(1, 2).Number() = %v, want 3", sum.Number())
	}
}

func TestCachedValuesHavePrototype(t *testing.T) {
	if NewBool(true).prototype != BooleanPrototype {
		t.Fatal("NewBool(true).prototype must be BooleanPrototype")
	}
	if NewBool(false).prototype != BooleanPrototype {
		t.Fatal("NewBool(false).prototype must be BooleanPrototype")
	}
	if NewNumber(0).prototype != NumberPrototype {
		t.Fatal("NewNumber(0).prototype must be NumberPrototype")
	}
	if NewString("").prototype != StringPrototype {
		t.Fatal(`NewString("").prototype must be StringPrototype`)
	}
	// Method on prototype chain must be reachable via Get.
	if NewBool(true).Get("toString") == nil {
		t.Fatal("NewBool(true).Get(\"toString\") must reach BooleanPrototype.toString")
	}
	if NewNumber(42).Get("toFixed") == nil {
		t.Fatal("NewNumber(42).Get(\"toFixed\") must reach NumberPrototype.toFixed")
	}
}

func TestNumberCacheSkipsNaN(t *testing.T) {
	n := NewNumber(math.NaN())
	if !math.IsNaN(n.Number()) {
		t.Fatalf("NewNumber(NaN).Number() = %v, want NaN", n.Number())
	}
	// Must not collide with any cached slot (e.g. slot 0 would be NewNumber(0)).
	zero := NewNumber(0)
	if n == zero {
		t.Fatal("NaN must not alias to the cached zero singleton")
	}
}

func TestNumberCacheSkipsNegativeZero(t *testing.T) {
	negZero := math.Copysign(0, -1)
	v := NewNumber(negZero)
	if !math.Signbit(v.Number()) {
		t.Fatal("NewNumber(-0).Number() must preserve the sign bit")
	}
	if got := 1.0 / v.Number(); got != math.Inf(-1) {
		t.Fatalf("1/NewNumber(-0) = %v, want -Inf", got)
	}
	// And -0 must not alias +0's singleton.
	if v == NewNumber(0) {
		t.Fatal("NewNumber(-0) must not alias the cached +0 singleton")
	}
}
