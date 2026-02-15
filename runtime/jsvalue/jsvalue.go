package jsvalue

import (
	"fmt"
	"regexp"
	"sync/atomic"
)

// ValueType represents the JavaScript type tag.
type ValueType int

const (
	TypeUndefined ValueType = iota
	TypeNull
	TypeBoolean
	TypeNumber
	TypeBigInt
	TypeString
	TypeSymbol
	TypeObject
	TypeFunction
)

// JSValue models a JavaScript value with typed storage and prototype chain.
type JSValue struct {
	typ        ValueType
	boolVal    bool
	numVal     float64
	strVal     string
	intVal     int
	bigIntVal  int64
	symbolDesc string
	symbolID   uint64
	properties map[string]*PropertyDescriptor
	prototype  *JSValue
	funcVal    func(...*JSValue) *JSValue
	arrayVal   []*JSValue
}

var symbolCounter uint64

// NewString creates a string JSValue.
func NewString(s string) *JSValue {
	return &JSValue{typ: TypeString, strVal: s, prototype: StringPrototype}
}

// NewNumber creates a number JSValue.
func NewNumber(f float64) *JSValue {
	return &JSValue{typ: TypeNumber, numVal: f, prototype: NumberPrototype}
}

// NewInt creates a number JSValue from an int.
func NewInt(i int) *JSValue {
	return &JSValue{typ: TypeNumber, numVal: float64(i), intVal: i, prototype: NumberPrototype}
}

// NewBigInt creates a bigint JSValue.
func NewBigInt(i int64) *JSValue {
	return &JSValue{typ: TypeBigInt, bigIntVal: i, prototype: BigIntPrototype}
}

// NewBool creates a boolean JSValue.
func NewBool(b bool) *JSValue {
	return &JSValue{typ: TypeBoolean, boolVal: b, prototype: BooleanPrototype}
}

// NewSymbol creates a symbol JSValue with a unique ID.
func NewSymbol(description string) *JSValue {
	id := atomic.AddUint64(&symbolCounter, 1)
	return &JSValue{typ: TypeSymbol, symbolDesc: description, symbolID: id, prototype: SymbolPrototype}
}

// NewNull creates a null JSValue.
func NewNull() *JSValue {
	return &JSValue{typ: TypeNull}
}

// NewUndefined creates an undefined JSValue.
func NewUndefined() *JSValue {
	return &JSValue{typ: TypeUndefined}
}

// NewObject creates an empty object JSValue.
func NewObject() *JSValue {
	return &JSValue{
		typ:        TypeObject,
		properties: make(map[string]*PropertyDescriptor),
		prototype:  ObjectPrototype,
	}
}

// NewArray creates an array JSValue (stored as TypeObject).
func NewArray(elems ...*JSValue) *JSValue {
	return &JSValue{
		typ:        TypeObject,
		properties: make(map[string]*PropertyDescriptor),
		prototype:  ArrayPrototype,
		arrayVal:   elems,
	}
}

// NewFunction creates a function JSValue.
func NewFunction(fn func(...*JSValue) *JSValue) *JSValue {
	return &JSValue{
		typ:        TypeFunction,
		properties: make(map[string]*PropertyDescriptor),
		prototype:  FunctionPrototype,
		funcVal:    fn,
	}
}

// Type returns the pre-computed type tag.
func (v *JSValue) Type() ValueType {
	return v.typ
}

// TypeString returns the JavaScript typeof string.
func (v *JSValue) TypeString() string {
	switch v.typ {
	case TypeUndefined:
		return "undefined"
	case TypeNull:
		return "object"
	case TypeBoolean:
		return "boolean"
	case TypeNumber:
		return "number"
	case TypeBigInt:
		return "bigint"
	case TypeString:
		return "string"
	case TypeSymbol:
		return "symbol"
	case TypeObject:
		return "object"
	case TypeFunction:
		return "function"
	default:
		return "undefined"
	}
}

// String returns the string value (or a string representation).
func (v *JSValue) String() string {
	switch v.typ {
	case TypeString:
		return v.strVal
	case TypeNumber:
		if v.numVal == float64(int64(v.numVal)) {
			return fmt.Sprintf("%d", int64(v.numVal))
		}
		return fmt.Sprintf("%g", v.numVal)
	case TypeBigInt:
		return fmt.Sprintf("%d", v.bigIntVal)
	case TypeBoolean:
		if v.boolVal {
			return "true"
		}
		return "false"
	case TypeNull:
		return "null"
	case TypeUndefined:
		return "undefined"
	case TypeSymbol:
		return fmt.Sprintf("Symbol(%s)", v.symbolDesc)
	case TypeObject:
		return "[object Object]"
	case TypeFunction:
		return "function"
	default:
		return "undefined"
	}
}

// Number returns the numeric value.
func (v *JSValue) Number() float64 {
	return v.numVal
}

// Bool returns the boolean value.
func (v *JSValue) Bool() bool {
	return v.boolVal
}

// Int returns the int value.
func (v *JSValue) Int() int {
	return v.intVal
}

// BigInt returns the bigint value.
func (v *JSValue) BigInt() int64 {
	return v.bigIntVal
}

// SymbolDesc returns the symbol description.
func (v *JSValue) SymbolDesc() string {
	return v.symbolDesc
}

// Array returns the underlying array elements, or nil if not an array.
func (v *JSValue) Array() []*JSValue {
	return v.arrayVal
}

// Index returns the element at position i in an array JSValue.
// Returns undefined if out of bounds or not an array.
func (v *JSValue) Index(i int) *JSValue {
	if v.arrayVal != nil && i >= 0 && i < len(v.arrayVal) {
		return v.arrayVal[i]
	}
	return NewUndefined()
}

// Len returns the length of the value as an int.
// For strings, returns the character count; for arrays, the element count;
// otherwise checks the "length" property.
func (v *JSValue) Len() int {
	switch v.typ {
	case TypeString:
		return len(v.strVal)
	case TypeObject:
		if v.arrayVal != nil {
			return len(v.arrayVal)
		}
	}
	if prop := v.Get("length"); prop.typ == TypeNumber {
		return int(prop.numVal)
	}
	return 0
}

// MatchString treats the JSValue as a regex pattern and tests it against s.
// If the value is a string, it compiles it as a regexp and matches.
func (v *JSValue) MatchString(s string) bool {
	if v.typ == TypeString && v.strVal != "" {
		re, err := regexp.Compile(v.strVal)
		if err != nil {
			return false
		}
		return re.MatchString(s)
	}
	return false
}

// Match implements JS String.prototype.match(regexp).
// Returns an array JSValue of match strings, or null if no match.
func (v *JSValue) Match(re *regexp.Regexp) *JSValue {
	if v.typ != TypeString {
		return NewNull()
	}
	matches := re.FindAllString(v.strVal, -1)
	if matches == nil {
		return NewNull()
	}
	elems := make([]*JSValue, len(matches))
	for i, m := range matches {
		elems[i] = NewString(m)
	}
	return NewArray(elems...)
}

// CharAt implements JS String.prototype.charAt(index).
// Returns the character at the given position as a single-character string JSValue.
func (v *JSValue) CharAt(pos int) *JSValue {
	if v.typ != TypeString {
		return NewString("")
	}
	runes := []rune(v.strVal)
	if pos < 0 || pos >= len(runes) {
		return NewString("")
	}
	return NewString(string(runes[pos]))
}

// CodePointAt returns the Unicode code point at the given position.
func (v *JSValue) CodePointAt(pos int) int {
	if v.typ != TypeString {
		return 0
	}
	runes := []rune(v.strVal)
	if pos < 0 || pos >= len(runes) {
		return 0
	}
	return int(runes[pos])
}

// Slice implements JS Array.prototype.slice(start, end).
// Returns a new array JSValue with elements from start to end.
func (v *JSValue) Slice(args ...int) *JSValue {
	if v.arrayVal == nil {
		return NewArray()
	}
	n := len(v.arrayVal)
	start := 0
	end := n
	if len(args) > 0 {
		start = args[0]
		if start < 0 {
			start = n + start
			if start < 0 {
				start = 0
			}
		}
		if start > n {
			start = n
		}
	}
	if len(args) > 1 {
		end = args[1]
		if end < 0 {
			end = n + end
			if end < 0 {
				end = 0
			}
		}
		if end > n {
			end = n
		}
	}
	if start >= end {
		return NewArray()
	}
	elems := make([]*JSValue, end-start)
	copy(elems, v.arrayVal[start:end])
	return NewArray(elems...)
}

// IsArray returns true if the JSValue holds an array.
func (v *JSValue) IsArray() bool {
	return v != nil && v.arrayVal != nil
}

// Map applies a callback to each element and returns a new array JSValue.
func (v *JSValue) Map(fn func(*JSValue) *JSValue) *JSValue {
	if v.arrayVal == nil {
		return NewArray()
	}
	results := make([]*JSValue, len(v.arrayVal))
	for i, elem := range v.arrayVal {
		results[i] = fn(elem)
	}
	return NewArray(results...)
}

// From wraps an arbitrary Go value as a *JSValue.
// If the value is already a *JSValue, it is returned as-is.
func From(v any) *JSValue {
	if v == nil {
		return NewNull()
	}
	switch val := v.(type) {
	case *JSValue:
		return val
	case string:
		return NewString(val)
	case int:
		return NewInt(val)
	case float64:
		return NewNumber(val)
	case bool:
		return NewBool(val)
	case func(...*JSValue) *JSValue:
		return NewFunction(val)
	case map[string]*JSValue:
		obj := NewObject()
		for k, v := range val {
			obj.Set(k, v)
		}
		return obj
	default:
		return NewString(fmt.Sprint(val))
	}
}

// Truthy implements JavaScript truthiness semantics.
// Returns false for nil, undefined, null, false, 0, NaN, and "".
func Truthy(v *JSValue) bool {
	if v == nil {
		return false
	}
	switch v.typ {
	case TypeUndefined, TypeNull:
		return false
	case TypeBoolean:
		return v.boolVal
	case TypeNumber:
		return v.numVal != 0 && v.numVal == v.numVal // NaN != NaN
	case TypeString:
		return v.strVal != ""
	default:
		return true
	}
}
