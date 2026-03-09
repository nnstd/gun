package math

import (
	gomath "math"
	"math/rand"

	"github.com/nnstd/gun/runtime/builtin"
)

// Floor returns the largest integer <= x.
func Floor(x *jsvalue.JSValue) *jsvalue.JSValue {
	return jsvalue.NewNumber(gomath.Floor(x.Number()))
}

// Ceil returns the smallest integer >= x.
func Ceil(x *jsvalue.JSValue) *jsvalue.JSValue {
	return jsvalue.NewNumber(gomath.Ceil(x.Number()))
}

// Round returns the nearest integer, rounding half away from zero.
func Round(x *jsvalue.JSValue) *jsvalue.JSValue {
	return jsvalue.NewNumber(gomath.Round(x.Number()))
}

// Abs returns the absolute value of x.
func Abs(x *jsvalue.JSValue) *jsvalue.JSValue {
	return jsvalue.NewNumber(gomath.Abs(x.Number()))
}

// Max returns the larger of x or y. Supports variadic args like JS Math.max().
func Max(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	if len(args) == 0 {
		return jsvalue.NewNumber(gomath.Inf(-1))
	}
	result := args[0].Number()
	for _, a := range args[1:] {
		v := a.Number()
		if v > result {
			result = v
		}
	}
	return jsvalue.NewNumber(result)
}

// Min returns the smaller of x or y. Supports variadic args like JS Math.min().
func Min(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	if len(args) == 0 {
		return jsvalue.NewNumber(gomath.Inf(1))
	}
	result := args[0].Number()
	for _, a := range args[1:] {
		v := a.Number()
		if v < result {
			result = v
		}
	}
	return jsvalue.NewNumber(result)
}

// Sqrt returns the square root of x.
func Sqrt(x *jsvalue.JSValue) *jsvalue.JSValue {
	return jsvalue.NewNumber(gomath.Sqrt(x.Number()))
}

// Pow returns base**exp.
func Pow(base, exp *jsvalue.JSValue) *jsvalue.JSValue {
	return jsvalue.NewNumber(gomath.Pow(base.Number(), exp.Number()))
}

// Random returns a pseudo-random number in [0, 1).
func Random() *jsvalue.JSValue {
	return jsvalue.NewNumber(rand.Float64())
}

// Log returns the natural logarithm of x.
func Log(x *jsvalue.JSValue) *jsvalue.JSValue {
	return jsvalue.NewNumber(gomath.Log(x.Number()))
}

// Log2 returns the base-2 logarithm of x.
func Log2(x *jsvalue.JSValue) *jsvalue.JSValue {
	return jsvalue.NewNumber(gomath.Log2(x.Number()))
}

// Trunc returns the integer part of x, removing fractional digits.
func Trunc(x *jsvalue.JSValue) *jsvalue.JSValue {
	return jsvalue.NewNumber(gomath.Trunc(x.Number()))
}

// Sign returns -1, 0, or 1 indicating the sign of x.
func Sign(x *jsvalue.JSValue) *jsvalue.JSValue {
	v := x.Number()
	switch {
	case v > 0:
		return jsvalue.NewNumber(1)
	case v < 0:
		return jsvalue.NewNumber(-1)
	default:
		return jsvalue.NewNumber(0)
	}
}

// IsNaN returns true if x is NaN.
func IsNaN(x *jsvalue.JSValue) *jsvalue.JSValue {
	return jsvalue.NewBool(gomath.IsNaN(x.Number()))
}

// IsFinite returns true if x is not NaN, +Inf, or -Inf.
func IsFinite(x *jsvalue.JSValue) *jsvalue.JSValue {
	return jsvalue.NewBool(!gomath.IsInf(x.Number(), 0))
}

// Inf returns positive infinity as a JSValue.
func Inf() *jsvalue.JSValue {
	return jsvalue.NewNumber(gomath.Inf(1))
}

// NaN returns NaN as a JSValue.
func NaN() *jsvalue.JSValue {
	return jsvalue.NewNumber(gomath.NaN())
}
