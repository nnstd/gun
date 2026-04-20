package jsvalue

import "math"

func AAdd(a *Arena, left, right *JSValue) *JSValue {
	if left == nil {
		left = NewUndefined()
	}
	if right == nil {
		right = NewUndefined()
	}
	if left.typ == TypeString || right.typ == TypeString {
		return NewString(left.String() + right.String())
	}
	return a.NewNumber(left.Number() + right.Number())
}

func ASub(a *Arena, left, right *JSValue) *JSValue {
	if left == nil {
		left = NewUndefined()
	}
	if right == nil {
		right = NewUndefined()
	}
	return a.NewNumber(left.Number() - right.Number())
}

func AMul(a *Arena, left, right *JSValue) *JSValue {
	if left == nil {
		left = NewUndefined()
	}
	if right == nil {
		right = NewUndefined()
	}
	return a.NewNumber(left.Number() * right.Number())
}

func ADiv(a *Arena, left, right *JSValue) *JSValue {
	if left == nil {
		left = NewUndefined()
	}
	if right == nil {
		right = NewUndefined()
	}
	bv := right.Number()
	if bv == 0 {
		av := left.Number()
		if av > 0 {
			return a.NewNumber(math.Inf(1))
		} else if av < 0 {
			return a.NewNumber(math.Inf(-1))
		}
		return a.NewNumber(math.NaN())
	}
	return a.NewNumber(left.Number() / bv)
}

func AMod(a *Arena, left, right *JSValue) *JSValue {
	if left == nil {
		left = NewUndefined()
	}
	if right == nil {
		right = NewUndefined()
	}
	return a.NewNumber(math.Mod(left.Number(), right.Number()))
}

func ANeg(a *Arena, v *JSValue) *JSValue {
	if v == nil {
		return a.NewNumber(math.NaN())
	}
	return a.NewNumber(-v.Number())
}

func AInc(a *Arena, v *JSValue) *JSValue {
	if v == nil {
		return a.NewNumber(1)
	}
	return a.NewNumber(v.Number() + 1)
}

func ADec(a *Arena, v *JSValue) *JSValue {
	if v == nil {
		return a.NewNumber(-1)
	}
	return a.NewNumber(v.Number() - 1)
}
