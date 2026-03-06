package jsvalue

import (
	"fmt"
	"strings"
)

// NewArray creates an array JSValue (stored as TypeObject).
func NewArray(elems ...*JSValue) *JSValue {
	arr := elems
	if arr == nil {
		arr = []*JSValue{}
	}
	return &JSValue{
		typ:        TypeObject,
		properties: make(map[string]*PropertyDescriptor),
		prototype:  ArrayPrototype,
		arrayVal:   arr,
	}
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

// FromStrings converts a []string into an array JSValue.
func FromStrings(ss []string) *JSValue {
	elems := make([]*JSValue, len(ss))
	for i, s := range ss {
		elems[i] = NewString(s)
	}
	return NewArray(elems...)
}

// SpreadIntoArray spreads a JSValue into array elements.
// For arrays, returns the elements as-is (like [...arr]).
// For strings, splits into individual characters (like [...str]).
// For other types, returns a single-element slice.
func SpreadIntoArray(v *JSValue) []*JSValue {
	if v == nil {
		return nil
	}
	if v.arrayVal != nil {
		return v.arrayVal
	}
	if v.typ == TypeString {
		runes := []rune(v.strVal)
		elems := make([]*JSValue, len(runes))
		for i, r := range runes {
			elems[i] = NewString(string(r))
		}
		return elems
	}
	return []*JSValue{v}
}

// normalizeIndex converts negative indices to positive ones.
// In JavaScript, arr[-1] means arr[len(arr)-1].
func normalizeIndex(idx int, length int) int {
	if idx < 0 {
		return length + idx
	}
	return idx
}

// Slice creates a slice of an array or string.
func Slice(arr *JSValue, args ...*JSValue) *JSValue {
	if arr == nil {
		return NewArray()
	}
	if arr.typ == TypeString {
		runes := []rune(arr.strVal)
		length := len(runes)
		start, end := 0, length
		if len(args) >= 1 && args[0] != nil { start = normalizeIndex(int(args[0].Number()), length) }
		if len(args) >= 2 && args[1] != nil { end = normalizeIndex(int(args[1].Number()), length) }
		if start < 0 { start = 0 }
		if start > length { start = length }
		if end < 0 { end = 0 }
		if end > length { end = length }
		if end < start { end = start }
		return NewString(string(runes[start:end]))
	}
	if arr.arrayVal == nil {
		return NewArray()
	}
	length := len(arr.arrayVal)
	start, end := 0, length
	if len(args) >= 1 && args[0] != nil { start = normalizeIndex(int(args[0].Number()), length) }
	if len(args) >= 2 && args[1] != nil { end = normalizeIndex(int(args[1].Number()), length) }
	if start < 0 { start = 0 }
	if start > length { start = length }
	if end < 0 { end = 0 }
	if end > length { end = length }
	if end < start { end = start }
	return NewArray(arr.arrayVal[start:end]...)
}

// Concat concatenates arrays and values.
func Concat(arr *JSValue, items ...*JSValue) *JSValue {
	var result []*JSValue
	if arr != nil && arr.arrayVal != nil {
		result = make([]*JSValue, len(arr.arrayVal))
		copy(result, arr.arrayVal)
	}
	for _, item := range items {
		if item != nil && item.arrayVal != nil {
			result = append(result, item.arrayVal...)
		} else {
			result = append(result, item)
		}
	}
	return NewArray(result...)
}

// Join joins array elements into a string with separator.
func Join(arr *JSValue, sep *JSValue) *JSValue {
	if arr == nil || arr.arrayVal == nil {
		return NewString("")
	}
	s := ","
	if sep != nil { s = sep.String() }
	strs := make([]string, len(arr.arrayVal))
	for i, elem := range arr.arrayVal {
		strs[i] = fmt.Sprint(elem)
	}
	return NewString(strings.Join(strs, s))
}

// Includes checks if array or string contains a value.
func Includes(arr *JSValue, val *JSValue) *JSValue {
	if arr == nil { return NewBool(false) }
	if arr.typ == TypeString {
		if val == nil { return NewBool(false) }
		return NewBool(strings.Contains(arr.strVal, val.String()))
	}
	if arr.arrayVal == nil { return NewBool(false) }
	for _, elem := range arr.arrayVal {
		if elem == val { return NewBool(true) }
		if elem != nil && val != nil && elem.typ == val.typ {
			switch elem.typ {
			case TypeString:
				if elem.strVal == val.String() { return NewBool(true) }
			case TypeNumber:
				if elem.numVal == val.numVal { return NewBool(true) }
			case TypeBoolean:
				if elem.boolVal == val.boolVal { return NewBool(true) }
			}
		}
	}
	return NewBool(false)
}

// IsArrayValue returns a *JSValue boolean indicating whether v is an array.
// This is the JSValue-returning wrapper for Array.isArray().
func IsArrayValue(v *JSValue) *JSValue {
	if v == nil {
		return NewBool(false)
	}
	return NewBool(v.IsArray())
}
