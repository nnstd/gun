package jsvalue

import (
	"math"
	"testing"
)

func TestBoolInterning(t *testing.T) {
	t1 := NewBool(true)
	t2 := NewBool(true)
	if t1 != t2 {
		t.Fatal("NewBool(true) must return the same pointer on repeated calls")
	}
	f1 := NewBool(false)
	f2 := NewBool(false)
	if f1 != f2 {
		t.Fatal("NewBool(false) must return the same pointer on repeated calls")
	}
	if t1 == f1 {
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
	n1 := NewNumber(1.5)
	n2 := NewNumber(1.5)
	if n1 == n2 {
		t.Fatal("NewNumber(1.5) should not be interned")
	}
}

func TestEmptyStringInterning(t *testing.T) {
	s1 := NewString("")
	s2 := NewString("")
	if s1 != s2 {
		t.Fatal(`NewString("") must return the same pointer`)
	}
	a1 := NewString("a")
	a2 := NewString("a")
	if a1 == a2 {
		t.Fatal(`NewString("a") should not be interned`)
	}
}

func TestCachedValueImmutability(t *testing.T) {
	assertTypeError := func(name string, fn func()) {
		t.Helper()
		defer func() {
			r := recover()
			if r == nil {
				t.Fatalf("%s: expected panic", name)
			}
			err, ok := r.(*JSValue)
			if !ok {
				t.Fatalf("%s: expected *JSValue panic, got %T", name, r)
			}
			if got := err.Get("name").String(); got != "TypeError" {
				t.Fatalf("%s: panic name = %q, want TypeError", name, got)
			}
		}()
		fn()
	}

	t1 := NewBool(true)
	assertTypeError("bool", func() { t1.Set("foo", NewNumber(42)) })
	if NewBool(true) != t1 {
		t.Fatal("singleton identity must not change after rejected mutation")
	}

	n := NewNumber(5)
	assertTypeError("number", func() { n.Set("x", NewNumber(1)) })

	s := NewString("")
	assertTypeError("string", func() { s.Set("y", NewNumber(2)) })
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
