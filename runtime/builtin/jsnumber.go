package jsvalue

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Number interning cache for integers in [numCacheMin, numCacheMax].
// Populated by initNumCache(), invoked from prototype.init() so NumberPrototype
// is available when slots are created.
const (
	numCacheMin = -128
	numCacheMax = 255
)

var numCache [numCacheMax - numCacheMin + 1]*JSValue

// initNumCache populates numCache with frozen singletons. Idempotent: slot 0
// holds the singleton for numCacheMin (-128), so a non-nil check guards reentry.
func initNumCache() {
	if numCache[0] != nil {
		return
	}
	for i := numCacheMin; i <= numCacheMax; i++ {
		numCache[i-numCacheMin] = &JSValue{
			typ:       TypeNumber,
			numVal:    float64(i),
			intVal:    i,
			prototype: NumberPrototype,
			frozen:    true,
		}
	}
}

// NewNumber creates a number JSValue. Returns a cached singleton for integer
// values in [-128, 255]. NaN and -0 are explicitly excluded: NaN's int
// conversion is implementation-defined in Go, and -0 must preserve its sign
// bit to produce -Infinity on reciprocation (1/-0 === -Infinity in JS).
func NewNumber(f float64) *JSValue {
	if !math.IsNaN(f) && !(f == 0 && math.Signbit(f)) {
		if i := int(f); float64(i) == f && i >= numCacheMin && i <= numCacheMax {
			return numCache[i-numCacheMin]
		}
	}
	return &JSValue{typ: TypeNumber, numVal: f, prototype: NumberPrototype}
}

// NewInt creates a number JSValue from an int. Returns a cached singleton for
// values in [-128, 255].
func NewInt(i int) *JSValue {
	if i >= numCacheMin && i <= numCacheMax {
		return numCache[i-numCacheMin]
	}
	return &JSValue{typ: TypeNumber, numVal: float64(i), intVal: i, prototype: NumberPrototype}
}

// NewBigInt creates a bigint JSValue.
func NewBigInt(i int64) *JSValue {
	return &JSValue{typ: TypeBigInt, bigIntVal: i, prototype: BigIntPrototype}
}

// BigInt implements the JavaScript BigInt() coercion for basic number/string/bigint inputs.
func BigInt(args ...*JSValue) *JSValue {
	if len(args) == 0 || args[0] == nil {
		return NewBigInt(0)
	}
	v := args[0]
	switch v.typ {
	case TypeBigInt:
		return v
	case TypeNumber:
		return NewBigInt(int64(v.Number()))
	case TypeString:
		if i, err := strconv.ParseInt(strings.TrimSpace(v.String()), 10, 64); err == nil {
			return NewBigInt(i)
		}
		return NewBigInt(0)
	default:
		if i, err := strconv.ParseInt(strings.TrimSpace(v.String()), 10, 64); err == nil {
			return NewBigInt(i)
		}
		return NewBigInt(int64(v.Number()))
	}
}

// ---------------------------------------------------------------------------
// Arithmetic operations
// ---------------------------------------------------------------------------

// Add implements the JavaScript + operator.
// If either operand is a string, concatenates; otherwise numeric addition.
func Add(a, b *JSValue) *JSValue {
	if a == nil {
		a = NewUndefined()
	}
	if b == nil {
		b = NewUndefined()
	}
	if a.typ == TypeString || b.typ == TypeString {
		return NewString(a.String() + b.String())
	}
	return NewNumber(a.Number() + b.Number())
}

// Sub implements the JavaScript - operator.
func Sub(a, b *JSValue) *JSValue {
	if a == nil {
		a = NewUndefined()
	}
	if b == nil {
		b = NewUndefined()
	}
	return NewNumber(a.Number() - b.Number())
}

// Mul implements the JavaScript * operator.
func Mul(a, b *JSValue) *JSValue {
	if a == nil {
		a = NewUndefined()
	}
	if b == nil {
		b = NewUndefined()
	}
	return NewNumber(a.Number() * b.Number())
}

// Div implements the JavaScript / operator.
func Div(a, b *JSValue) *JSValue {
	if a == nil {
		a = NewUndefined()
	}
	if b == nil {
		b = NewUndefined()
	}
	bv := b.Number()
	if bv == 0 {
		av := a.Number()
		if av > 0 {
			return NewNumber(math.Inf(1))
		} else if av < 0 {
			return NewNumber(math.Inf(-1))
		}
		return NewNumber(math.NaN())
	}
	return NewNumber(a.Number() / bv)
}

// Mod implements the JavaScript % operator.
func Mod(a, b *JSValue) *JSValue {
	if a == nil {
		a = NewUndefined()
	}
	if b == nil {
		b = NewUndefined()
	}
	return NewNumber(math.Mod(a.Number(), b.Number()))
}

// Neg implements the JavaScript unary - operator.
func Neg(a *JSValue) *JSValue {
	if a == nil {
		return NewNumber(math.NaN())
	}
	return NewNumber(-a.Number())
}

// ---------------------------------------------------------------------------
// Bitwise operations
// ---------------------------------------------------------------------------

// BitNot implements the JavaScript ~ operator.
func BitNot(a *JSValue) *JSValue {
	if a == nil {
		return NewInt(-1)
	}
	return NewInt(^int(a.Number()))
}

// BitAnd implements the JavaScript & operator.
func BitAnd(a, b *JSValue) *JSValue {
	return NewInt(int(a.Number()) & int(b.Number()))
}

// BitOr implements the JavaScript | operator.
func BitOr(a, b *JSValue) *JSValue {
	return NewInt(int(a.Number()) | int(b.Number()))
}

// BitXor implements the JavaScript ^ operator.
func BitXor(a, b *JSValue) *JSValue {
	return NewInt(int(a.Number()) ^ int(b.Number()))
}

// Shl implements the JavaScript << operator.
func Shl(a, b *JSValue) *JSValue {
	return NewInt(int(a.Number()) << uint(b.Number()))
}

// Shr implements the JavaScript >> operator.
func Shr(a, b *JSValue) *JSValue {
	return NewInt(int(a.Number()) >> uint(b.Number()))
}

// UShr implements the JavaScript >>> operator (unsigned right shift).
func UShr(a, b *JSValue) *JSValue {
	return NewInt(int(uint32(a.Number()) >> uint(b.Number())))
}

// ---------------------------------------------------------------------------
// Comparison operations (all return *JSValue boolean)
// ---------------------------------------------------------------------------

// Eq implements JavaScript === (strict equality).
func Eq(a, b *JSValue) *JSValue {
	if a == nil && b == nil {
		return NewBool(true)
	}
	if a == nil || b == nil {
		return NewBool(false)
	}
	if a.typ != b.typ {
		return NewBool(false)
	}
	switch a.typ {
	case TypeUndefined, TypeNull:
		return NewBool(true)
	case TypeBoolean:
		return NewBool(a.boolVal == b.boolVal)
	case TypeNumber:
		return NewBool(a.numVal == b.numVal)
	case TypeString:
		return NewBool(a.strVal == b.strVal)
	case TypeSymbol:
		return NewBool(a.symbolID == b.symbolID)
	default:
		return NewBool(a == b) // reference equality for objects
	}
}

// NEq implements JavaScript !== (strict inequality).
func NEq(a, b *JSValue) *JSValue {
	return NewBool(!Eq(a, b).boolVal)
}

// EqLoose implements JavaScript == (abstract/loose equality).
// Key difference from Eq (===): null == undefined is true.
func EqLoose(a, b *JSValue) *JSValue {
	if a == nil {
		a = NewUndefined()
	}
	if b == nil {
		b = NewUndefined()
	}
	// null == undefined (and vice versa) is true
	if (a.typ == TypeNull || a.typ == TypeUndefined) && (b.typ == TypeNull || b.typ == TypeUndefined) {
		return NewBool(true)
	}
	// For other types, fall back to strict equality
	return Eq(a, b)
}

// NEqLoose implements JavaScript != (abstract/loose inequality).
func NEqLoose(a, b *JSValue) *JSValue {
	return NewBool(!EqLoose(a, b).boolVal)
}

// Lt implements JavaScript < operator.
func Lt(a, b *JSValue) *JSValue {
	if a == nil {
		a = NewUndefined()
	}
	if b == nil {
		b = NewUndefined()
	}
	if a.typ == TypeString && b.typ == TypeString {
		return NewBool(a.strVal < b.strVal)
	}
	return NewBool(a.Number() < b.Number())
}

// Gt implements JavaScript > operator.
func Gt(a, b *JSValue) *JSValue {
	if a == nil {
		a = NewUndefined()
	}
	if b == nil {
		b = NewUndefined()
	}
	if a.typ == TypeString && b.typ == TypeString {
		return NewBool(a.strVal > b.strVal)
	}
	return NewBool(a.Number() > b.Number())
}

// LtE implements JavaScript <= operator.
func LtE(a, b *JSValue) *JSValue {
	if a == nil {
		a = NewUndefined()
	}
	if b == nil {
		b = NewUndefined()
	}
	if a.typ == TypeString && b.typ == TypeString {
		return NewBool(a.strVal <= b.strVal)
	}
	return NewBool(a.Number() <= b.Number())
}

// GtE implements JavaScript >= operator.
func GtE(a, b *JSValue) *JSValue {
	if a == nil {
		a = NewUndefined()
	}
	if b == nil {
		b = NewUndefined()
	}
	if a.typ == TypeString && b.typ == TypeString {
		return NewBool(a.strVal >= b.strVal)
	}
	return NewBool(a.Number() >= b.Number())
}

// ---------------------------------------------------------------------------
// Increment/Decrement
// ---------------------------------------------------------------------------

// Inc implements JavaScript ++ (prefix/postfix increment).
// Returns a new JSValue to avoid aliasing bugs when the operand is shared
// with another variable (e.g. start := ...; i := From(start)).
func Inc(a *JSValue) *JSValue {
	if a == nil {
		return NewNumber(1)
	}
	return NewNumber(a.Number() + 1)
}

// Dec implements JavaScript -- (prefix/postfix decrement).
// Returns a new JSValue to avoid aliasing bugs.
func Dec(a *JSValue) *JSValue {
	if a == nil {
		return NewNumber(-1)
	}
	return NewNumber(a.Number() - 1)
}

// ---------------------------------------------------------------------------
// Parsing
// ---------------------------------------------------------------------------

// ParseInt parses a string as an integer with the given radix, matching JS parseInt().
func ParseInt(s, radix *JSValue) *JSValue {
	str := strings.TrimSpace(fmt.Sprint(s))
	base := int(radix.Number())
	if base == 0 {
		base = 10
	}
	// Handle 0x prefix for hex
	if base == 16 && len(str) > 2 && str[0] == '0' && (str[1] == 'x' || str[1] == 'X') {
		str = str[2:]
	}
	// Parse digit by digit (JS parseInt stops at first invalid char)
	result := 0
	found := false
	neg := false
	i := 0
	if i < len(str) && (str[i] == '-' || str[i] == '+') {
		if str[i] == '-' {
			neg = true
		}
		i++
	}
	for ; i < len(str); i++ {
		c := str[i]
		digit := -1
		switch {
		case c >= '0' && c <= '9':
			digit = int(c - '0')
		case c >= 'a' && c <= 'z':
			digit = int(c-'a') + 10
		case c >= 'A' && c <= 'Z':
			digit = int(c-'A') + 10
		}
		if digit < 0 || digit >= base {
			break
		}
		result = result*base + digit
		found = true
	}
	if !found {
		return NewNumber(math.NaN())
	}
	if neg {
		result = -result
	}
	return NewNumber(float64(result))
}

// ParseFloat parses a string as a floating-point number, matching JS parseFloat().
func ParseFloat(s *JSValue) *JSValue {
	str := strings.TrimSpace(fmt.Sprint(s))
	// Parse manually to handle JS-style stopping at invalid chars
	var num float64
	n, _ := fmt.Sscanf(str, "%f", &num)
	if n == 0 {
		return NewNumber(math.NaN())
	}
	return NewNumber(num)
}

func initNumberPrototype() {
	defMethod(NumberPrototype, "toString", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil {
			return NewString("0")
		}
		recv := args[0].unboxed()
		n := recv.numVal
		if math.IsNaN(n) {
			return NewString("NaN")
		}
		if math.IsInf(n, 1) {
			return NewString("Infinity")
		}
		if math.IsInf(n, -1) {
			return NewString("-Infinity")
		}
		radix := 10
		if len(args) > 1 && args[1] != nil {
			radix = int(args[1].numVal)
		}
		if radix == 10 {
			return NewString(recv.String())
		}
		neg := n < 0
		if neg {
			n = -n
		}
		intPart := int64(n)
		s := strconv.FormatInt(intPart, radix)
		if neg {
			s = "-" + s
		}
		return NewString(s)
	})
	defMethod(NumberPrototype, "valueOf", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil {
			return NewNumber(0)
		}
		return NewNumber(args[0].unboxed().numVal)
	})
	defMethod(NumberPrototype, "toFixed", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil {
			return NewString("0")
		}
		n := args[0].unboxed().numVal
		if math.IsNaN(n) {
			return NewString("NaN")
		}
		digits := 0
		if len(args) > 1 && args[1] != nil {
			digits = int(args[1].numVal)
		}
		return NewString(strconv.FormatFloat(n, 'f', digits, 64))
	})
	defMethod(NumberPrototype, "toPrecision", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil {
			return NewString("0")
		}
		recv := args[0].unboxed()
		n := recv.numVal
		if math.IsNaN(n) {
			return NewString("NaN")
		}
		if math.IsInf(n, 0) {
			return NewString(args[0].String())
		}
		if len(args) < 2 || args[1] == nil {
			return NewString(recv.String())
		}
		prec := int(args[1].numVal)
		return NewString(strconv.FormatFloat(n, 'g', prec, 64))
	})
	defMethod(NumberPrototype, "toExponential", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil {
			return NewString("0")
		}
		recv := args[0].unboxed()
		n := recv.numVal
		if math.IsNaN(n) {
			return NewString("NaN")
		}
		if math.IsInf(n, 0) {
			return NewString(recv.String())
		}
		digits := -1
		if len(args) > 1 && args[1] != nil {
			digits = int(args[1].numVal)
		}
		if digits < 0 {
			return NewString(strconv.FormatFloat(n, 'e', -1, 64))
		}
		return NewString(strconv.FormatFloat(n, 'e', digits, 64))
	})
	defMethod(NumberPrototype, "toLocaleString", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil {
			return NewString("0")
		}
		return NewString(args[0].unboxed().String())
	})
}
