package jsvalue

import "math"

// Arena-optimized arithmetic operations.
// These skip nil checks (compiler guarantees non-nil in arena context) and
// use inline TypeNumber fast paths to avoid method call overhead.

func AAdd(a *Arena, left, right *JSValue) *JSValue {
	if left.typ == TypeString || right.typ == TypeString {
		return NewString(left.String() + right.String())
	}
	return a.NewNumber(left.numValFast() + right.numValFast())
}

func ASub(a *Arena, left, right *JSValue) *JSValue {
	return a.NewNumber(left.numValFast() - right.numValFast())
}

func AMul(a *Arena, left, right *JSValue) *JSValue {
	return a.NewNumber(left.numValFast() * right.numValFast())
}

func ADiv(a *Arena, left, right *JSValue) *JSValue {
	bv := right.numValFast()
	if bv == 0 {
		av := left.numValFast()
		if av > 0 {
			return a.NewNumber(math.Inf(1))
		} else if av < 0 {
			return a.NewNumber(math.Inf(-1))
		}
		return a.NewNumber(math.NaN())
	}
	return a.NewNumber(left.numValFast() / bv)
}

func AMod(a *Arena, left, right *JSValue) *JSValue {
	return a.NewNumber(math.Mod(left.numValFast(), right.numValFast()))
}

func ANeg(a *Arena, v *JSValue) *JSValue {
	return a.NewNumber(-v.numValFast())
}

func AInc(a *Arena, v *JSValue) *JSValue {
	return a.NewNumber(v.numValFast() + 1)
}

func ADec(a *Arena, v *JSValue) *JSValue {
	return a.NewNumber(v.numValFast() - 1)
}

// Arena-optimized comparison operations.
// Return arena-allocated bools, skip nil checks, inline TypeNumber fast path.

func ALt(a *Arena, left, right *JSValue) *JSValue {
	if left.typ == TypeString && right.typ == TypeString {
		return a.NewBool(left.strVal < right.strVal)
	}
	return a.NewBool(left.numValFast() < right.numValFast())
}

func AGt(a *Arena, left, right *JSValue) *JSValue {
	if left.typ == TypeString && right.typ == TypeString {
		return a.NewBool(left.strVal > right.strVal)
	}
	return a.NewBool(left.numValFast() > right.numValFast())
}

func ALtE(a *Arena, left, right *JSValue) *JSValue {
	if left.typ == TypeString && right.typ == TypeString {
		return a.NewBool(left.strVal <= right.strVal)
	}
	return a.NewBool(left.numValFast() <= right.numValFast())
}

func AGtE(a *Arena, left, right *JSValue) *JSValue {
	if left.typ == TypeString && right.typ == TypeString {
		return a.NewBool(left.strVal >= right.strVal)
	}
	return a.NewBool(left.numValFast() >= right.numValFast())
}

func AEq(a *Arena, left, right *JSValue) *JSValue {
	if left.typ == TypeNumber && right.typ == TypeNumber {
		return a.NewBool(left.numVal == right.numVal)
	}
	if left.typ == TypeString && right.typ == TypeString {
		return a.NewBool(left.strVal == right.strVal)
	}
	if left.typ == TypeBoolean && right.typ == TypeBoolean {
		return a.NewBool(left.boolVal == right.boolVal)
	}
	if left.typ == TypeNull && right.typ == TypeNull {
		return a.NewBool(true)
	}
	if left.typ == TypeUndefined && right.typ == TypeUndefined {
		return a.NewBool(true)
	}
	return a.NewBool(left.numValFast() == right.numValFast())
}

func ANEq(a *Arena, left, right *JSValue) *JSValue {
	if left.typ == TypeNumber && right.typ == TypeNumber {
		return a.NewBool(left.numVal != right.numVal)
	}
	return a.NewBool(left.numValFast() != right.numValFast())
}

// numValFast is an inline fast path for Number() that avoids nil checks
// and the full type switch when the value is already TypeNumber.
//go:nosplit
func (v *JSValue) numValFast() float64 {
	if v.typ == TypeNumber {
		return v.numVal
	}
	return v.Number()
}
