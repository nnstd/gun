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
