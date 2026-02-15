package jsvalue

import (
	"fmt"
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
	TypeRegex
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
	regexVal   interface{} // stores *regexp.Regexp to avoid import cycle
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

// NewRegex creates a regex JSValue from a compiled regexp.
// The regex parameter should be *regexp.Regexp but is typed as interface{}
// to avoid import cycles.
func NewRegex(regex interface{}) *JSValue {
	return &JSValue{
		typ:        TypeRegex,
		properties: make(map[string]*PropertyDescriptor),
		regexVal:   regex,
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
	case TypeRegex:
		return "object" // In JavaScript, typeof /regex/ === "object"
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


// IsArray returns true if the JSValue holds an array.
func (v *JSValue) IsArray() bool {
	return v != nil && v.arrayVal != nil
}

// Len returns the length of the JSValue, matching JavaScript semantics.
// For strings: returns character count (not byte count).
// For arrays: returns element count.
// For objects with a length property: returns that property's numeric value.
// Otherwise: returns 0.
func (v *JSValue) Len() int {
	if v == nil {
		return 0
	}
	switch v.typ {
	case TypeString:
		// JavaScript .length on strings returns character count (UTF-16 code units)
		// In Go, we approximate this with rune count
		return len([]rune(v.strVal))
	case TypeObject:
		if v.arrayVal != nil {
			return len(v.arrayVal)
		}
	}
	// Check for length property on objects
	if prop := v.Get("length"); prop != nil && prop.typ == TypeNumber {
		return int(prop.numVal)
	}
	return 0
}

// MatchString tests whether the JSValue (as a regex) matches the string s.
// This implements JavaScript's regex.test(s) method.
// Returns false if the JSValue is not a regex.
func (v *JSValue) MatchString(s string) bool {
	if v == nil || v.typ != TypeRegex || v.regexVal == nil {
		return false
	}
	// Type assert to *regexp.Regexp
	// We use interface{} in the struct to avoid import cycles
	if re, ok := v.regexVal.(interface{ MatchString(string) bool }); ok {
		return re.MatchString(s)
	}
	return false
}


// Call invokes the JSValue as a function with the given arguments.
// Returns undefined if the value is not a function.
func (v *JSValue) Call(args ...*JSValue) *JSValue {
	if v.funcVal != nil {
		return v.funcVal(args...)
	}
	return NewUndefined()
}

// ToSlice converts an any value to []*JSValue. Handles []*JSValue passthrough
// and *JSValue (via .Array()). Used when an IIFE returns any but the target is []*JSValue.
func ToSlice(v any) []*JSValue {
	switch val := v.(type) {
	case []*JSValue:
		return val
	case *JSValue:
		return val.Array()
	default:
		return nil
	}
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
		// Check if it's a regex (has MatchString method)
		if re, ok := v.(interface{ MatchString(string) bool }); ok {
			return NewRegex(re)
		}
		return NewString(fmt.Sprint(val))
	}
}

// FromStrings converts a []string into an array JSValue.
func FromStrings(ss []string) *JSValue {
	elems := make([]*JSValue, len(ss))
	for i, s := range ss {
		elems[i] = NewString(s)
	}
	return NewArray(elems...)
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

// Map applies fn to each element and returns a new array.
func Map(arr *JSValue, fn func(*JSValue) *JSValue) *JSValue {
	if arr == nil || arr.arrayVal == nil {
		return NewArray()
	}
	results := make([]*JSValue, len(arr.arrayVal))
	for i, elem := range arr.arrayVal {
		results[i] = fn(elem)
	}
	return NewArray(results...)
}

// Filter returns a new array containing elements for which fn returns true.
func Filter(arr *JSValue, fn func(*JSValue) bool) *JSValue {
	if arr == nil || arr.arrayVal == nil {
		return NewArray()
	}
	var results []*JSValue
	for _, elem := range arr.arrayVal {
		if fn(elem) {
			results = append(results, elem)
		}
	}
	return NewArray(results...)
}

// ForEach calls fn for each element in the array.
func ForEach(arr *JSValue, fn func(*JSValue)) {
	if arr == nil || arr.arrayVal == nil {
		return
	}
	for _, elem := range arr.arrayVal {
		fn(elem)
	}
}
