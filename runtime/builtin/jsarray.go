package jsvalue

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

var arraySlicePool = sync.Pool{
	New: func() any {
		return make([]*JSValue, 0, 8)
	},
}

// NewArray creates an array JSValue (stored as TypeObject).
func NewArray(elems ...*JSValue) *JSValue {
	arr := elems
	if arr == nil {
		arr = arraySlicePool.Get().([]*JSValue)[:0]
	}
	return &JSValue{
		typ:        TypeObject,
		properties: propMapPool.Get().(map[string]*PropertyDescriptor),
		prototype:  ArrayPrototype,
		arrayVal:   arr,
	}
}

// ReleaseArraySlice returns an array slice to the pool for reuse.
// Slices with capacity > 16 are discarded.
func ReleaseArraySlice(s []*JSValue) {
	if cap(s) > 16 {
		return
	}
	clear(s)
	arraySlicePool.Put(s[:0])
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

func initArrayPrototype() {
	// Array.prototype.prototype points to itself so Array.prototype.slice works
	// when the transpiler generates ArrayPrototype.Get("prototype").Get("slice").
	ArrayPrototype.Set("prototype", ArrayPrototype)

	ArrayPrototype.DefineProperty("slice", &PropertyDescriptor{
		Value: NewFunction(func(args ...*JSValue) *JSValue {
			if len(args) < 1 { return NewArray() }
			return Slice(args[0], args[1:]...)
		}).MarkAsMethod(),
		Writable: true, Enumerable: false, Configurable: true,
	})
	ArrayPrototype.DefineProperty("concat", &PropertyDescriptor{
		Value: NewFunction(func(args ...*JSValue) *JSValue {
			if len(args) < 1 { return NewArray() }
			result := args[0]
			for _, arg := range args[1:] { result = Concat(result, arg) }
			return result
		}).MarkAsMethod(),
		Writable: true, Enumerable: false, Configurable: true,
	})
	ArrayPrototype.DefineProperty("join", &PropertyDescriptor{
		Value: NewFunction(func(args ...*JSValue) *JSValue {
			if len(args) < 1 { return NewString("") }
			sep := NewString(",")
			if len(args) > 1 && args[1] != nil { sep = args[1] }
			return Join(args[0], sep)
		}).MarkAsMethod(),
		Writable: true, Enumerable: false, Configurable: true,
	})
	ArrayPrototype.DefineProperty("includes", &PropertyDescriptor{
		Value: NewFunction(func(args ...*JSValue) *JSValue {
			if len(args) < 2 { return NewBool(false) }
			return Includes(args[0], args[1])
		}).MarkAsMethod(),
		Writable: true, Enumerable: false, Configurable: true,
	})
	ArrayPrototype.DefineProperty("reverse", &PropertyDescriptor{
		Value: NewFunction(func(args ...*JSValue) *JSValue {
			if len(args) < 1 || args[0] == nil || args[0].arrayVal == nil { return NewArray() }
			arr := args[0].arrayVal
			for i, j := 0, len(arr)-1; i < j; i, j = i+1, j-1 { arr[i], arr[j] = arr[j], arr[i] }
			return args[0]
		}).MarkAsMethod(),
		Writable: true, Enumerable: false, Configurable: true,
	})
	ArrayPrototype.DefineProperty("fill", &PropertyDescriptor{
		Value: NewFunction(func(args ...*JSValue) *JSValue {
			if len(args) < 2 { return NewArray() }
			this := args[0]
			if this == nil || this.arrayVal == nil { return NewArray() }
			for i := range this.arrayVal { this.arrayVal[i] = args[1] }
			return this
		}).MarkAsMethod(),
		Writable: true, Enumerable: false, Configurable: true,
	})
	ArrayPrototype.DefineProperty("at", &PropertyDescriptor{
		Value: NewFunction(func(args ...*JSValue) *JSValue {
			if len(args) < 2 { return NewUndefined() }
			this := args[0]
			idx := int(args[1].Number())
			if this == nil || this.arrayVal == nil { return NewUndefined() }
			if idx < 0 { idx = len(this.arrayVal) + idx }
			if idx < 0 || idx >= len(this.arrayVal) { return NewUndefined() }
			return this.arrayVal[idx]
		}).MarkAsMethod(),
		Writable: true, Enumerable: false, Configurable: true,
	})
	ArrayPrototype.DefineProperty("sort", &PropertyDescriptor{
		Value: NewFunction(func(args ...*JSValue) *JSValue {
			if len(args) < 1 || args[0] == nil || args[0].arrayVal == nil { return NewArray() }
			this := args[0]
			var compareFn *JSValue
			if len(args) >= 2 { compareFn = args[1] }
			sort.SliceStable(this.arrayVal, func(i, j int) bool {
				a, b := this.arrayVal[i], this.arrayVal[j]
				if compareFn != nil && compareFn.funcVal != nil {
					return compareFn.Call(a, b).Number() < 0
				}
				return a.String() < b.String()
			})
			return this
		}).MarkAsMethod(),
		Writable: true, Enumerable: false, Configurable: true,
	})
	ArrayPrototype.DefineProperty("entries", &PropertyDescriptor{
		Value: NewFunction(func(args ...*JSValue) *JSValue {
			if len(args) < 1 || args[0] == nil || args[0].arrayVal == nil { return NewArray() }
			this := args[0]
			entries := make([]*JSValue, len(this.arrayVal))
			for i, v := range this.arrayVal { entries[i] = NewArray(NewNumber(float64(i)), v) }
			return NewArray(entries...)
		}).MarkAsMethod(),
		Writable: true, Enumerable: false, Configurable: true,
	})

	defMethod(ArrayPrototype, "push", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil { return NewNumber(0) }
		this := args[0]
		this.arrayVal = append(this.arrayVal, args[1:]...)
		return NewNumber(float64(len(this.arrayVal)))
	})
	defMethod(ArrayPrototype, "pop", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil || args[0].arrayVal == nil || len(args[0].arrayVal) == 0 { return NewUndefined() }
		this := args[0]
		last := this.arrayVal[len(this.arrayVal)-1]
		this.arrayVal = this.arrayVal[:len(this.arrayVal)-1]
		return last
	})
	defMethod(ArrayPrototype, "shift", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil || args[0].arrayVal == nil || len(args[0].arrayVal) == 0 { return NewUndefined() }
		this := args[0]
		first := this.arrayVal[0]
		this.arrayVal = this.arrayVal[1:]
		return first
	})
	defMethod(ArrayPrototype, "unshift", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil { return NewNumber(0) }
		this := args[0]
		this.arrayVal = append(args[1:], this.arrayVal...)
		return NewNumber(float64(len(this.arrayVal)))
	})
	defMethod(ArrayPrototype, "splice", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil || args[0].arrayVal == nil { return NewArray() }
		this := args[0]
		rest := args[1:]
		length := len(this.arrayVal)
		start := 0
		if len(rest) >= 1 && rest[0] != nil {
			start = int(rest[0].Number())
			if start < 0 { start = length + start; if start < 0 { start = 0 } }
			if start > length { start = length }
		}
		deleteCount := length - start
		if len(rest) >= 2 && rest[1] != nil {
			deleteCount = int(rest[1].Number())
			if deleteCount < 0 { deleteCount = 0 }
			if start+deleteCount > length { deleteCount = length - start }
		}
		removed := make([]*JSValue, deleteCount)
		copy(removed, this.arrayVal[start:start+deleteCount])
		var newItems []*JSValue
		for i := 2; i < len(rest); i++ { newItems = append(newItems, rest[i]) }
		result := make([]*JSValue, 0, length-deleteCount+len(newItems))
		result = append(result, this.arrayVal[:start]...)
		result = append(result, newItems...)
		result = append(result, this.arrayVal[start+deleteCount:]...)
		this.arrayVal = result
		return NewArray(removed...)
	})
	defMethod(ArrayPrototype, "indexOf", func(args ...*JSValue) *JSValue {
		if len(args) < 2 || args[0] == nil || args[0].arrayVal == nil { return NewNumber(-1) }
		for i, elem := range args[0].arrayVal {
			if jsValueEqual(elem, args[1]) { return NewNumber(float64(i)) }
		}
		return NewNumber(-1)
	})
	defMethod(ArrayPrototype, "findIndex", func(args ...*JSValue) *JSValue {
		if len(args) < 2 || args[0] == nil || args[0].arrayVal == nil { return NewNumber(-1) }
		fn := args[1]
		if fn == nil || fn.funcVal == nil { return NewNumber(-1) }
		for i, elem := range args[0].arrayVal {
			if Truthy(fn.funcVal(elem, NewNumber(float64(i)))) { return NewNumber(float64(i)) }
		}
		return NewNumber(-1)
	})
	defMethod(ArrayPrototype, "map", func(args ...*JSValue) *JSValue {
		if len(args) < 2 || args[0] == nil || args[0].arrayVal == nil { return NewArray() }
		this := args[0]
		fn := args[1]
		if fn == nil || fn.funcVal == nil { return NewArray() }
		results := make([]*JSValue, len(this.arrayVal))
		for i, elem := range this.arrayVal { results[i] = fn.funcVal(elem, NewNumber(float64(i)), this) }
		return NewArray(results...)
	})
	defMethod(ArrayPrototype, "filter", func(args ...*JSValue) *JSValue {
		if len(args) < 2 || args[0] == nil || args[0].arrayVal == nil { return NewArray() }
		this := args[0]
		fn := args[1]
		if fn == nil || fn.funcVal == nil { return NewArray() }
		var results []*JSValue
		for i, elem := range this.arrayVal {
			r := fn.funcVal(elem, NewNumber(float64(i)), this)
			if r != nil && r.Bool() { results = append(results, elem) }
		}
		return NewArray(results...)
	})
	defMethod(ArrayPrototype, "forEach", func(args ...*JSValue) *JSValue {
		if len(args) < 2 || args[0] == nil || args[0].arrayVal == nil { return NewUndefined() }
		this := args[0]
		fn := args[1]
		if fn == nil || fn.funcVal == nil { return NewUndefined() }
		for i, elem := range this.arrayVal { fn.funcVal(elem, NewNumber(float64(i)), this) }
		return NewUndefined()
	})
	defMethod(ArrayPrototype, "find", func(args ...*JSValue) *JSValue {
		if len(args) < 2 || args[0] == nil || args[0].arrayVal == nil { return NewUndefined() }
		fn := args[1]
		if fn == nil || fn.funcVal == nil { return NewUndefined() }
		for _, elem := range args[0].arrayVal {
			if Truthy(fn.funcVal(elem)) { return elem }
		}
		return NewUndefined()
	})
	defMethod(ArrayPrototype, "some", func(args ...*JSValue) *JSValue {
		if len(args) < 2 || args[0] == nil || args[0].arrayVal == nil { return NewBool(false) }
		fn := args[1]
		if fn == nil || fn.funcVal == nil { return NewBool(false) }
		for _, elem := range args[0].arrayVal {
			if Truthy(fn.funcVal(elem)) { return NewBool(true) }
		}
		return NewBool(false)
	})
	defMethod(ArrayPrototype, "every", func(args ...*JSValue) *JSValue {
		if len(args) < 2 || args[0] == nil || args[0].arrayVal == nil { return NewBool(true) }
		fn := args[1]
		if fn == nil || fn.funcVal == nil { return NewBool(true) }
		for _, elem := range args[0].arrayVal {
			if !Truthy(fn.funcVal(elem)) { return NewBool(false) }
		}
		return NewBool(true)
	})
	defMethod(ArrayPrototype, "reduce", func(args ...*JSValue) *JSValue {
		if len(args) < 2 || args[0] == nil || args[0].arrayVal == nil {
			if len(args) >= 3 { return args[2] }
			return NewUndefined()
		}
		fn := args[1]
		if fn == nil || fn.funcVal == nil { return NewUndefined() }
		arr := args[0].arrayVal
		var acc *JSValue
		startIdx := 0
		if len(args) >= 3 { acc = args[2] } else if len(arr) > 0 { acc = arr[0]; startIdx = 1 } else { return NewUndefined() }
		for i := startIdx; i < len(arr); i++ { acc = fn.funcVal(acc, arr[i]) }
		return acc
	})
	defMethod(ArrayPrototype, "flat", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil || args[0].arrayVal == nil { return NewArray() }
		var result []*JSValue
		for _, elem := range args[0].arrayVal {
			if elem != nil && elem.arrayVal != nil { result = append(result, elem.arrayVal...) } else { result = append(result, elem) }
		}
		return NewArray(result...)
	})
	defMethod(ArrayPrototype, "flatMap", func(args ...*JSValue) *JSValue {
		if len(args) < 2 || args[0] == nil || args[0].arrayVal == nil { return NewArray() }
		fn := args[1]
		if fn == nil || fn.funcVal == nil { return NewArray() }
		var result []*JSValue
		for i, elem := range args[0].arrayVal {
			r := fn.funcVal(elem, NewNumber(float64(i)))
			if r != nil && r.arrayVal != nil { result = append(result, r.arrayVal...) } else { result = append(result, r) }
		}
		return NewArray(result...)
	})
}
