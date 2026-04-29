package jsvalue

import (
	"fmt"
	"sort"
	"strings"
)

// NewArray creates an array JSValue (stored as TypeObject).
func NewArray(elems ...*JSValue) *JSValue {
	v := &JSValue{
		typ:       TypeObject,
		prototype: ArrayPrototype,
		isArr:     true,
	}
	if len(elems) > smallValueListCapacity {
		copied := make([]*JSValue, len(elems))
		copy(copied, elems)
		v.arrayListOrZero().ReplaceAll(copied)
		return v
	}
	for _, e := range elems {
		v.arrayListOrZero().Push(e)
	}
	return v
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
	if v.isArr {
		return append([]*JSValue{}, v.arrayListOrZero().Slice()...)
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
		if len(args) >= 1 && args[0] != nil {
			start = normalizeIndex(int(args[0].Number()), length)
		}
		if len(args) >= 2 && args[1] != nil {
			end = normalizeIndex(int(args[1].Number()), length)
		}
		if start < 0 {
			start = 0
		}
		if start > length {
			start = length
		}
		if end < 0 {
			end = 0
		}
		if end > length {
			end = length
		}
		if end < start {
			end = start
		}
		return NewString(string(runes[start:end]))
	}
	if !arr.isArr {
		return NewArray()
	}
	list := arr.arrayListOrZero()
	length := list.Len()
	start, end := 0, length
	if len(args) >= 1 && args[0] != nil {
		start = normalizeIndex(int(args[0].Number()), length)
	}
	if len(args) >= 2 && args[1] != nil {
		end = normalizeIndex(int(args[1].Number()), length)
	}
	if start < 0 {
		start = 0
	}
	if start > length {
		start = length
	}
	if end < 0 {
		end = 0
	}
	if end > length {
		end = length
	}
	if end < start {
		end = start
	}
	// Build result slice from range.
	result := make([]*JSValue, end-start)
	for i := start; i < end; i++ {
		result[i-start] = list.Get(i)
	}
	return NewArray(result...)
}

// Concat concatenates arrays and values.
func Concat(arr *JSValue, items ...*JSValue) *JSValue {
	var result []*JSValue
	if arr != nil && arr.isArr {
		list := arr.arrayListOrZero()
		n := list.Len()
		result = make([]*JSValue, n)
		for i := range n {
			result[i] = list.Get(i)
		}
	}
	for _, item := range items {
		if item != nil && item.isArr {
			result = append(result, item.arrayListOrZero().Slice()...)
		} else {
			result = append(result, item)
		}
	}
	return NewArray(result...)
}

// Join joins array elements into a string with separator.
func Join(arr *JSValue, sep *JSValue) *JSValue {
	if arr == nil || !arr.isArr {
		return NewString("")
	}
	s := ","
	if sep != nil {
		s = sep.String()
	}
	list := arr.arrayListOrZero()
	n := list.Len()
	strs := make([]string, n)
	for i := range n {
		strs[i] = fmt.Sprint(list.Get(i))
	}
	return NewString(strings.Join(strs, s))
}

// Includes checks if array or string contains a value.
func Includes(arr *JSValue, val *JSValue) *JSValue {
	if arr == nil {
		return NewBool(false)
	}
	if arr.typ == TypeString {
		if val == nil {
			return NewBool(false)
		}
		return NewBool(strings.Contains(arr.strVal, val.String()))
	}
	if !arr.isArr {
		return NewBool(false)
	}
	list := arr.arrayListOrZero()
	n := list.Len()
	for i := range n {
		elem := list.Get(i)
		if elem == val {
			return NewBool(true)
		}
		if elem != nil && val != nil && elem.typ == val.typ {
			switch elem.typ {
			case TypeString:
				if elem.strVal == val.String() {
					return NewBool(true)
				}
			case TypeNumber:
				if elem.numVal == val.numVal {
					return NewBool(true)
				}
			case TypeBoolean:
				if elem.boolVal == val.boolVal {
					return NewBool(true)
				}
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
			if len(args) < 1 {
				return NewArray()
			}
			return Slice(args[0], args[1:]...)
		}).MarkAsMethod(),
		Writable: true, Enumerable: false, Configurable: true,
	})
	ArrayPrototype.DefineProperty("concat", &PropertyDescriptor{
		Value: NewFunction(func(args ...*JSValue) *JSValue {
			if len(args) < 1 {
				return NewArray()
			}
			result := args[0]
			for _, arg := range args[1:] {
				result = Concat(result, arg)
			}
			return result
		}).MarkAsMethod(),
		Writable: true, Enumerable: false, Configurable: true,
	})
	ArrayPrototype.DefineProperty("join", &PropertyDescriptor{
		Value: NewFunction(func(args ...*JSValue) *JSValue {
			if len(args) < 1 {
				return NewString("")
			}
			sep := NewString(",")
			if len(args) > 1 && args[1] != nil {
				sep = args[1]
			}
			return Join(args[0], sep)
		}).MarkAsMethod(),
		Writable: true, Enumerable: false, Configurable: true,
	})
	ArrayPrototype.DefineProperty("includes", &PropertyDescriptor{
		Value: NewFunction(func(args ...*JSValue) *JSValue {
			if len(args) < 2 {
				return NewBool(false)
			}
			return Includes(args[0], args[1])
		}).MarkAsMethod(),
		Writable: true, Enumerable: false, Configurable: true,
	})
	ArrayPrototype.DefineProperty("reverse", &PropertyDescriptor{
		Value: NewFunction(func(args ...*JSValue) *JSValue {
			if len(args) < 1 || args[0] == nil || !args[0].isArr {
				return NewArray()
			}
			args[0].lock()
			defer args[0].unlock()
			n := args[0].arrayListOrZero().Len()
			for i, j := 0, n-1; i < j; i, j = i+1, j-1 {
				a := args[0].arrayListOrZero().Get(i)
				b := args[0].arrayListOrZero().Get(j)
				args[0].arrayListOrZero().Set(i, b)
				args[0].arrayListOrZero().Set(j, a)
			}
			args[0].genAdd(1)
			return args[0]
		}).MarkAsMethod(),
		Writable: true, Enumerable: false, Configurable: true,
	})
	ArrayPrototype.DefineProperty("fill", &PropertyDescriptor{
		Value: NewFunction(func(args ...*JSValue) *JSValue {
			if len(args) < 2 {
				return NewArray()
			}
			this := args[0]
			if this == nil || !this.isArr {
				return NewArray()
			}
			this.lock()
			defer this.unlock()
			n := this.arrayListOrZero().Len()
			for i := range n {
				this.arrayListOrZero().Set(i, args[1])
			}
			this.genAdd(1)
			return this
		}).MarkAsMethod(),
		Writable: true, Enumerable: false, Configurable: true,
	})
	ArrayPrototype.DefineProperty("at", &PropertyDescriptor{
		Value: NewFunction(func(args ...*JSValue) *JSValue {
			if len(args) < 2 {
				return NewUndefined()
			}
			this := args[0]
			idx := int(args[1].Number())
			if this == nil || !this.isArr {
				return NewUndefined()
			}
			n := this.arrayListOrZero().Len()
			if idx < 0 {
				idx = n + idx
			}
			if idx < 0 || idx >= n {
				return NewUndefined()
			}
			return this.arrayListOrZero().Get(idx)
		}).MarkAsMethod(),
		Writable: true, Enumerable: false, Configurable: true,
	})
	ArrayPrototype.DefineProperty("sort", &PropertyDescriptor{
		Value: NewFunction(func(args ...*JSValue) *JSValue {
			if len(args) < 1 || args[0] == nil || !args[0].isArr {
				return NewArray()
			}
			this := args[0]
			this.lock()
			defer this.unlock()
			var compareFn *JSValue
			if len(args) >= 2 {
				compareFn = args[1]
			}
			// Materialise to a plain slice for sort.SliceStable, then replace.
			n := this.arrayListOrZero().Len()
			slice := make([]*JSValue, n)
			for i := range n {
				slice[i] = this.arrayListOrZero().Get(i)
			}
			sort.SliceStable(slice, func(i, j int) bool {
				a, b := slice[i], slice[j]
				if compareFn != nil && compareFn.funcVal != nil {
					return compareFn.Call(a, b).Number() < 0
				}
				return a.String() < b.String()
			})
			this.arrayListOrZero().ReplaceAll(slice)
			this.genAdd(1)
			return this
		}).MarkAsMethod(),
		Writable: true, Enumerable: false, Configurable: true,
	})
	ArrayPrototype.DefineProperty("entries", &PropertyDescriptor{
		Value: NewFunction(func(args ...*JSValue) *JSValue {
			if len(args) < 1 || args[0] == nil || !args[0].isArr {
				return NewArray()
			}
			this := args[0]
			n := this.arrayListOrZero().Len()
			entries := make([]*JSValue, n)
			for i := range n {
				entries[i] = NewArray(NewNumber(float64(i)), this.arrayListOrZero().Get(i))
			}
			return NewArray(entries...)
		}).MarkAsMethod(),
		Writable: true, Enumerable: false, Configurable: true,
	})

	defMethod(ArrayPrototype, "push", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil {
			return NewNumber(0)
		}
		this := args[0]
		this.lock()
		defer this.unlock()
		for _, a := range args[1:] {
			this.arrayListOrZero().Push(a)
		}
		this.genAdd(1)
		return NewNumber(float64(this.arrayListOrZero().Len()))
	})
	defMethod(ArrayPrototype, "pop", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil || !args[0].isArr || args[0].arrayListOrZero().Len() == 0 {
			return NewUndefined()
		}
		this := args[0]
		this.lock()
		defer this.unlock()
		last := this.arrayListOrZero().Get(this.arrayListOrZero().Len() - 1)
		this.arrayListOrZero().Truncate()
		this.genAdd(1)
		return last
	})
	defMethod(ArrayPrototype, "shift", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil || !args[0].isArr || args[0].arrayListOrZero().Len() == 0 {
			return NewUndefined()
		}
		this := args[0]
		this.lock()
		defer this.unlock()
		first := this.arrayListOrZero().Get(0)
		this.arrayListOrZero().RemoveFirst()
		this.genAdd(1)
		return first
	})
	defMethod(ArrayPrototype, "unshift", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil {
			return NewNumber(0)
		}
		this := args[0]
		this.lock()
		defer this.unlock()
		this.arrayListOrZero().Prepend(args[1:]...)
		this.genAdd(1)
		return NewNumber(float64(this.arrayListOrZero().Len()))
	})
	defMethod(ArrayPrototype, "splice", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil || !args[0].isArr {
			return NewArray()
		}
		this := args[0]
		this.lock()
		defer this.unlock()
		rest := args[1:]
		length := this.arrayListOrZero().Len()
		start := 0
		if len(rest) >= 1 && rest[0] != nil {
			start = int(rest[0].Number())
			if start < 0 {
				start = length + start
				if start < 0 {
					start = 0
				}
			}
			if start > length {
				start = length
			}
		}
		deleteCount := length - start
		if len(rest) >= 2 && rest[1] != nil {
			deleteCount = int(rest[1].Number())
			if deleteCount < 0 {
				deleteCount = 0
			}
			if start+deleteCount > length {
				deleteCount = length - start
			}
		}
		// Collect removed elements.
		removed := make([]*JSValue, deleteCount)
		for i := range deleteCount {
			removed[i] = this.arrayListOrZero().Get(start + i)
		}
		// Collect new items.
		var newItems []*JSValue
		for i := 2; i < len(rest); i++ {
			newItems = append(newItems, rest[i])
		}
		// Build result slice.
		result := make([]*JSValue, 0, length-deleteCount+len(newItems))
		for i := 0; i < start; i++ {
			result = append(result, this.arrayListOrZero().Get(i))
		}
		result = append(result, newItems...)
		for i := start + deleteCount; i < length; i++ {
			result = append(result, this.arrayListOrZero().Get(i))
		}
		this.arrayListOrZero().ReplaceAll(result)
		this.genAdd(1)
		return NewArray(removed...)
	})
	defMethod(ArrayPrototype, "indexOf", func(args ...*JSValue) *JSValue {
		if len(args) < 2 || args[0] == nil || !args[0].isArr {
			return NewNumber(-1)
		}
		n := args[0].arrayListOrZero().Len()
		for i := range n {
			if jsValueEqual(args[0].arrayListOrZero().Get(i), args[1]) {
				return NewNumber(float64(i))
			}
		}
		return NewNumber(-1)
	})
	defMethod(ArrayPrototype, "findIndex", func(args ...*JSValue) *JSValue {
		if len(args) < 2 || args[0] == nil || !args[0].isArr {
			return NewNumber(-1)
		}
		fn := args[1]
		if fn == nil || fn.funcVal == nil {
			return NewNumber(-1)
		}
		n := args[0].arrayListOrZero().Len()
		for i := range n {
			if Truthy(fn.funcVal(args[0].arrayListOrZero().Get(i), NewNumber(float64(i)))) {
				return NewNumber(float64(i))
			}
		}
		return NewNumber(-1)
	})
	defMethod(ArrayPrototype, "map", func(args ...*JSValue) *JSValue {
		if len(args) < 2 || args[0] == nil || !args[0].isArr {
			return NewArray()
		}
		this := args[0]
		fn := args[1]
		if fn == nil || fn.funcVal == nil {
			return NewArray()
		}
		list := this.arrayListOrZero()
		n := list.Len()
		results := make([]*JSValue, n)
		for i := range n {
			results[i] = fn.funcVal(list.Get(i), NewNumber(float64(i)), this)
		}
		return NewArray(results...)
	})
	defMethod(ArrayPrototype, "filter", func(args ...*JSValue) *JSValue {
		if len(args) < 2 || args[0] == nil || !args[0].isArr {
			return NewArray()
		}
		this := args[0]
		fn := args[1]
		if fn == nil || fn.funcVal == nil {
			return NewArray()
		}
		list := this.arrayListOrZero()
		n := list.Len()
		results := make([]*JSValue, 0, n)
		for i := range n {
			elem := list.Get(i)
			r := fn.funcVal(elem, NewNumber(float64(i)), this)
			if r != nil && r.Bool() {
				results = append(results, elem)
			}
		}
		return NewArray(results...)
	})
	defMethod(ArrayPrototype, "forEach", func(args ...*JSValue) *JSValue {
		if len(args) < 2 || args[0] == nil || !args[0].isArr {
			return NewUndefined()
		}
		this := args[0]
		fn := args[1]
		if fn == nil || fn.funcVal == nil {
			return NewUndefined()
		}
		list := this.arrayListOrZero()
		n := list.Len()
		for i := range n {
			fn.funcVal(list.Get(i), NewNumber(float64(i)), this)
		}
		return NewUndefined()
	})
	defMethod(ArrayPrototype, "find", func(args ...*JSValue) *JSValue {
		if len(args) < 2 || args[0] == nil || !args[0].isArr {
			return NewUndefined()
		}
		fn := args[1]
		if fn == nil || fn.funcVal == nil {
			return NewUndefined()
		}
		list := args[0].arrayListOrZero()
		n := list.Len()
		for i := range n {
			elem := list.Get(i)
			if Truthy(fn.funcVal(elem)) {
				return elem
			}
		}
		return NewUndefined()
	})
	defMethod(ArrayPrototype, "some", func(args ...*JSValue) *JSValue {
		if len(args) < 2 || args[0] == nil || !args[0].isArr {
			return NewBool(false)
		}
		fn := args[1]
		if fn == nil || fn.funcVal == nil {
			return NewBool(false)
		}
		list := args[0].arrayListOrZero()
		n := list.Len()
		for i := range n {
			if Truthy(fn.funcVal(list.Get(i))) {
				return NewBool(true)
			}
		}
		return NewBool(false)
	})
	defMethod(ArrayPrototype, "every", func(args ...*JSValue) *JSValue {
		if len(args) < 2 || args[0] == nil || !args[0].isArr {
			return NewBool(true)
		}
		fn := args[1]
		if fn == nil || fn.funcVal == nil {
			return NewBool(true)
		}
		list := args[0].arrayListOrZero()
		n := list.Len()
		for i := range n {
			if !Truthy(fn.funcVal(list.Get(i))) {
				return NewBool(false)
			}
		}
		return NewBool(true)
	})
	defMethod(ArrayPrototype, "reduce", func(args ...*JSValue) *JSValue {
		if len(args) < 2 || args[0] == nil || !args[0].isArr {
			if len(args) >= 3 {
				return args[2]
			}
			return NewUndefined()
		}
		fn := args[1]
		if fn == nil || fn.funcVal == nil {
			return NewUndefined()
		}
		list := args[0].arrayListOrZero()
		n := list.Len()
		var acc *JSValue
		startIdx := 0
		if len(args) >= 3 {
			acc = args[2]
		} else if n > 0 {
			acc = list.Get(0)
			startIdx = 1
		} else {
			return NewUndefined()
		}
		for i := startIdx; i < n; i++ {
			acc = fn.funcVal(acc, list.Get(i))
		}
		return acc
	})
	defMethod(ArrayPrototype, "flat", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil || !args[0].isArr {
			return NewArray()
		}
		list := args[0].arrayListOrZero()
		n := list.Len()
		result := make([]*JSValue, 0, n)
		for i := range n {
			elem := list.Get(i)
			if elem != nil && elem.isArr {
				result = append(result, elem.arrayListOrZero().Slice()...)
			} else {
				result = append(result, elem)
			}
		}
		return NewArray(result...)
	})
	defMethod(ArrayPrototype, "flatMap", func(args ...*JSValue) *JSValue {
		if len(args) < 2 || args[0] == nil || !args[0].isArr {
			return NewArray()
		}
		fn := args[1]
		if fn == nil || fn.funcVal == nil {
			return NewArray()
		}
		list := args[0].arrayListOrZero()
		n := list.Len()
		result := make([]*JSValue, 0, n)
		for i := range n {
			r := fn.funcVal(list.Get(i), NewNumber(float64(i)))
			if r != nil && r.isArr {
				result = append(result, r.arrayListOrZero().Slice()...)
			} else {
				result = append(result, r)
			}
		}
		return NewArray(result...)
	})
}
