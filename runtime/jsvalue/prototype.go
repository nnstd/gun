package jsvalue

import (
	"sort"
	"strings"
)

// safeArg returns args[i] or nil if out of bounds.
func safeArg(args []*JSValue, i int) *JSValue {
	if i < len(args) { return args[i] }
	return nil
}

// Global prototype singletons.
var (
	ObjectPrototype   *JSValue
	StringPrototype   *JSValue
	NumberPrototype   *JSValue
	BigIntPrototype   *JSValue
	BooleanPrototype  *JSValue
	SymbolPrototype   *JSValue
	ArrayPrototype    *JSValue
	FunctionPrototype *JSValue
	MapPrototype      *JSValue
	SetPrototype      *JSValue
)

func init() {
	// ObjectPrototype is the root — no parent prototype.
	ObjectPrototype = &JSValue{
		typ:        TypeObject,
		properties: make(map[string]*PropertyDescriptor),
	}

	// Wire up child prototypes, all inheriting from ObjectPrototype.
	StringPrototype = &JSValue{
		typ:        TypeObject,
		properties: make(map[string]*PropertyDescriptor),
		prototype:  ObjectPrototype,
	}
	NumberPrototype = &JSValue{
		typ:        TypeObject,
		properties: make(map[string]*PropertyDescriptor),
		prototype:  ObjectPrototype,
	}
	BigIntPrototype = &JSValue{
		typ:        TypeObject,
		properties: make(map[string]*PropertyDescriptor),
		prototype:  ObjectPrototype,
	}
	BooleanPrototype = &JSValue{
		typ:        TypeObject,
		properties: make(map[string]*PropertyDescriptor),
		prototype:  ObjectPrototype,
	}
	SymbolPrototype = &JSValue{
		typ:        TypeObject,
		properties: make(map[string]*PropertyDescriptor),
		prototype:  ObjectPrototype,
	}
	ArrayPrototype = &JSValue{
		typ:        TypeObject,
		properties: make(map[string]*PropertyDescriptor),
		prototype:  ObjectPrototype,
	}
	FunctionPrototype = &JSValue{
		typ:        TypeObject,
		properties: make(map[string]*PropertyDescriptor),
		prototype:  ObjectPrototype,
	}

	// Built-in methods on FunctionPrototype.
	// Marked as methods so MethodCall passes the function as 'this' (args[0]).
	bindFn := NewFunction(func(args ...*JSValue) *JSValue {
		// bind(thisArg, ...boundArgs) → returns a new function
		// args[0] = the original function (this), args[1] = thisArg, args[2:] = bound args
		if len(args) < 2 {
			return NewUndefined()
		}
		origFn := args[0]
		thisArg := args[1]
		boundArgs := args[2:]
		if origFn == nil || origFn.funcVal == nil {
			return NewUndefined()
		}
		return NewFunction(func(callArgs ...*JSValue) *JSValue {
			all := make([]*JSValue, 0, 1+len(boundArgs)+len(callArgs))
			all = append(all, thisArg)
			all = append(all, boundArgs...)
			all = append(all, callArgs...)
			return origFn.funcVal(all...)
		})
	})
	bindFn.MarkAsMethod()
	FunctionPrototype.DefineProperty("bind", &PropertyDescriptor{
		Value:        bindFn,
		Writable:     true,
		Enumerable:   false,
		Configurable: true,
	})
	callFn := NewFunction(func(args ...*JSValue) *JSValue {
		if len(args) < 1 {
			return NewUndefined()
		}
		origFn := args[0]
		if origFn == nil || origFn.funcVal == nil {
			return NewUndefined()
		}
		return origFn.funcVal(args[1:]...)
	})
	callFn.MarkAsMethod()
	FunctionPrototype.DefineProperty("call", &PropertyDescriptor{
		Value:        callFn,
		Writable:     true,
		Enumerable:   false,
		Configurable: true,
	})
	applyFn := NewFunction(func(args ...*JSValue) *JSValue {
		if len(args) < 1 {
			return NewUndefined()
		}
		origFn := args[0]
		if origFn == nil || origFn.funcVal == nil {
			return NewUndefined()
		}
		callArgs := []*JSValue{}
		// Only prepend thisArg for method functions (which expect 'this' as first arg)
		if origFn.isMethod && len(args) >= 2 {
			callArgs = append(callArgs, args[1])
		}
		if len(args) >= 3 && args[2] != nil && args[2].arrayVal != nil {
			callArgs = append(callArgs, args[2].arrayVal...)
		}
		return origFn.funcVal(callArgs...)
	})
	applyFn.MarkAsMethod()
	FunctionPrototype.DefineProperty("apply", &PropertyDescriptor{
		Value:        applyFn,
		Writable:     true,
		Enumerable:   false,
		Configurable: true,
	})

	// Built-in methods on ObjectPrototype.
	ObjectPrototype.DefineProperty("toString", &PropertyDescriptor{
		Value: NewFunction(func(args ...*JSValue) *JSValue {
			return NewString("[object Object]")
		}),
		Writable:     true,
		Enumerable:   false,
		Configurable: true,
	})
	ObjectPrototype.DefineProperty("hasOwnProperty", &PropertyDescriptor{
		Value: NewFunction(func(args ...*JSValue) *JSValue {
			// Two calling patterns:
			// 1. obj.hasOwnProperty("key") — not supported without 'this' binding
			// 2. Object.prototype.hasOwnProperty.call(obj, "key") — args[0]=obj, args[1]=key
			if len(args) >= 2 {
				return NewBool(args[0].HasOwnProperty(args[1].String()))
			}
			return NewBool(false)
		}),
		Writable:     true,
		Enumerable:   false,
		Configurable: true,
	})
	ObjectPrototype.DefineProperty("valueOf", &PropertyDescriptor{
		Value: NewFunction(func(args ...*JSValue) *JSValue {
			return NewUndefined()
		}),
		Writable:     true,
		Enumerable:   false,
		Configurable: true,
	})

	// StringPrototype toString returns the string value.
	StringPrototype.DefineProperty("toString", &PropertyDescriptor{
		Value: NewFunction(func(args ...*JSValue) *JSValue {
			if len(args) > 0 && args[0] != nil {
				return NewString(args[0].String())
			}
			return NewString("")
		}).MarkAsMethod(),
		Writable:     true,
		Enumerable:   false,
		Configurable: true,
	})

	// StringPrototype normalize returns the string as-is (basic NFC is a no-op for ASCII).
	StringPrototype.DefineProperty("normalize", &PropertyDescriptor{
		Value: NewFunction(func(args ...*JSValue) *JSValue {
			if len(args) > 0 && args[0] != nil {
				return args[0]
			}
			return NewString("")
		}).MarkAsMethod(),
		Writable:     true,
		Enumerable:   false,
		Configurable: true,
	})

	// Helper for defining prototype methods.
	defMethod := func(proto *JSValue, name string, fn func(args ...*JSValue) *JSValue) {
		proto.DefineProperty(name, &PropertyDescriptor{
			Value: NewFunction(fn).MarkAsMethod(), Writable: true, Enumerable: false, Configurable: true,
		})
	}
	defGetter := func(proto *JSValue, name string, fn func(this *JSValue) *JSValue) {
		proto.DefineProperty(name, &PropertyDescriptor{
			Get: fn, Enumerable: false, Configurable: true,
		})
	}

	// --- StringPrototype methods ---

	defMethod(StringPrototype, "toLowerCase", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil { return NewString("") }
		return NewString(strings.ToLower(args[0].String()))
	})
	defMethod(StringPrototype, "toUpperCase", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil { return NewString("") }
		return NewString(strings.ToUpper(args[0].String()))
	})
	defMethod(StringPrototype, "trim", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil { return NewString("") }
		return NewString(strings.TrimSpace(args[0].String()))
	})
	defMethod(StringPrototype, "trimStart", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil { return NewString("") }
		return NewString(strings.TrimLeft(args[0].String(), " \t\n\r"))
	})
	defMethod(StringPrototype, "trimEnd", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil { return NewString("") }
		return NewString(strings.TrimRight(args[0].String(), " \t\n\r"))
	})
	defMethod(StringPrototype, "split", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil { return NewArray() }
		return Split(args[0], safeArg(args, 1))
	})
	defMethod(StringPrototype, "replace", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil { return NewString("") }
		return Replace(args[0], safeArg(args, 1), safeArg(args, 2))
	})
	defMethod(StringPrototype, "replaceAll", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil { return NewString("") }
		return Replace(args[0], safeArg(args, 1), safeArg(args, 2))
	})
	defMethod(StringPrototype, "charAt", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil { return NewString("") }
		return CharAt(args[0], safeArg(args, 1))
	})
	defMethod(StringPrototype, "indexOf", func(args ...*JSValue) *JSValue {
		if len(args) < 2 || args[0] == nil { return NewNumber(-1) }
		s := args[0].String()
		sub := args[1].String()
		return NewNumber(float64(strings.Index(s, sub)))
	})
	defMethod(StringPrototype, "lastIndexOf", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil { return NewNumber(-1) }
		return LastIndexOf(args[0], safeArg(args, 1))
	})
	defMethod(StringPrototype, "substring", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil { return NewString("") }
		extras := args[2:]
		return Substring(args[0], safeArg(args, 1), extras...)
	})
	defMethod(StringPrototype, "startsWith", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil { return NewBool(false) }
		return StartsWith(args[0], safeArg(args, 1))
	})
	defMethod(StringPrototype, "endsWith", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil { return NewBool(false) }
		return EndsWith(args[0], safeArg(args, 1))
	})
	defMethod(StringPrototype, "includes", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil { return NewBool(false) }
		return Includes(args[0], safeArg(args, 1))
	})
	defMethod(StringPrototype, "repeat", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil { return NewString("") }
		return Repeat(args[0], safeArg(args, 1))
	})
	defMethod(StringPrototype, "match", func(args ...*JSValue) *JSValue {
		if len(args) < 2 || args[0] == nil { return NewUndefined() }
		return RegexExec(safeArg(args, 1), args[0])
	})
	defMethod(StringPrototype, "search", func(args ...*JSValue) *JSValue {
		if len(args) < 2 || args[0] == nil { return NewNumber(-1) }
		pattern := safeArg(args, 1)
		if pattern != nil && pattern.typ == TypeRegex && pattern.regexVal != nil {
			if re, ok := pattern.regexVal.(interface{ FindStringIndex(string) []int }); ok {
				loc := re.FindStringIndex(args[0].String())
				if loc != nil { return NewNumber(float64(loc[0])) }
			}
		}
		if pattern != nil {
			return NewNumber(float64(strings.Index(args[0].String(), pattern.String())))
		}
		return NewNumber(-1)
	})
	defMethod(StringPrototype, "padStart", func(args ...*JSValue) *JSValue {
		if len(args) < 2 || args[0] == nil { return args[0] }
		s := args[0].String()
		targetLen := int(args[1].Number())
		pad := " "
		if len(args) > 2 && args[2] != nil { pad = args[2].String() }
		for len([]rune(s)) < targetLen { s = pad + s }
		return NewString(string([]rune(s)[:targetLen]))
	})
	defMethod(StringPrototype, "padEnd", func(args ...*JSValue) *JSValue {
		if len(args) < 2 || args[0] == nil { return args[0] }
		s := args[0].String()
		targetLen := int(args[1].Number())
		pad := " "
		if len(args) > 2 && args[2] != nil { pad = args[2].String() }
		for len([]rune(s)) < targetLen { s = s + pad }
		return NewString(string([]rune(s)[:targetLen]))
	})
	defMethod(StringPrototype, "codePointAt", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil { return NewUndefined() }
		runes := []rune(args[0].String())
		idx := 0
		if len(args) > 1 && args[1] != nil { idx = int(args[1].Number()) }
		if idx < 0 || idx >= len(runes) { return NewUndefined() }
		return NewNumber(float64(runes[idx]))
	})
	defMethod(StringPrototype, "charCodeAt", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil { return NewNumber(0) }
		runes := []rune(args[0].String())
		idx := 0
		if len(args) > 1 && args[1] != nil { idx = int(args[1].Number()) }
		if idx < 0 || idx >= len(runes) { return NewNumber(0) }
		return NewNumber(float64(runes[idx]))
	})

	// NumberPrototype toString returns the number as string.
	NumberPrototype.DefineProperty("toString", &PropertyDescriptor{
		Value: NewFunction(func(args ...*JSValue) *JSValue {
			if len(args) > 0 && args[0] != nil {
				return NewString(args[0].String())
			}
			return NewString("0")
		}).MarkAsMethod(),
		Writable:     true,
		Enumerable:   false,
		Configurable: true,
	})

	// BooleanPrototype toString.
	BooleanPrototype.DefineProperty("toString", &PropertyDescriptor{
		Value: NewFunction(func(args ...*JSValue) *JSValue {
			if len(args) > 0 && args[0] != nil {
				return NewString(args[0].String())
			}
			return NewString("false")
		}).MarkAsMethod(),
		Writable:     true,
		Enumerable:   false,
		Configurable: true,
	})

	// Array.prototype.prototype points to itself so Array.prototype.slice works
	// when the transpiler generates ArrayPrototype.Get("prototype").Get("slice").
	ArrayPrototype.Set("prototype", ArrayPrototype)

	// ArrayPrototype slice: arr.slice(start, end) or [].slice.call(target).
	// args[0] = this (the array), args[1:] = start, end
	ArrayPrototype.DefineProperty("slice", &PropertyDescriptor{
		Value: NewFunction(func(args ...*JSValue) *JSValue {
			if len(args) < 1 {
				return NewArray()
			}
			return Slice(args[0], args[1:]...)
		}).MarkAsMethod(),
		Writable:     true,
		Enumerable:   false,
		Configurable: true,
	})

	// ArrayPrototype concat: arr.concat(other)
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
		Writable:     true,
		Enumerable:   false,
		Configurable: true,
	})

	// ArrayPrototype join: arr.join(sep)
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
		Writable:     true,
		Enumerable:   false,
		Configurable: true,
	})

	// ArrayPrototype includes: arr.includes(val)
	ArrayPrototype.DefineProperty("includes", &PropertyDescriptor{
		Value: NewFunction(func(args ...*JSValue) *JSValue {
			if len(args) < 2 {
				return NewBool(false)
			}
			return Includes(args[0], args[1])
		}).MarkAsMethod(),
		Writable:     true,
		Enumerable:   false,
		Configurable: true,
	})

	// ArrayPrototype reverse: arr.reverse()
	ArrayPrototype.DefineProperty("reverse", &PropertyDescriptor{
		Value: NewFunction(func(args ...*JSValue) *JSValue {
			if len(args) < 1 || args[0] == nil || args[0].arrayVal == nil {
				return NewArray()
			}
			arr := args[0].arrayVal
			for i, j := 0, len(arr)-1; i < j; i, j = i+1, j-1 {
				arr[i], arr[j] = arr[j], arr[i]
			}
			return args[0]
		}).MarkAsMethod(),
		Writable:     true,
		Enumerable:   false,
		Configurable: true,
	})

	// ArrayPrototype fill: arr.fill(value) fills all elements with value.
	ArrayPrototype.DefineProperty("fill", &PropertyDescriptor{
		Value: NewFunction(func(args ...*JSValue) *JSValue {
			if len(args) < 2 {
				return NewArray()
			}
			this := args[0]
			val := args[1]
			if this == nil || this.arrayVal == nil {
				return NewArray()
			}
			for i := range this.arrayVal {
				this.arrayVal[i] = val
			}
			return this
		}).MarkAsMethod(),
		Writable:     true,
		Enumerable:   false,
		Configurable: true,
	})

	// ArrayPrototype at: arr.at(index) returns element at index (supports negative).
	ArrayPrototype.DefineProperty("at", &PropertyDescriptor{
		Value: NewFunction(func(args ...*JSValue) *JSValue {
			if len(args) < 2 {
				return NewUndefined()
			}
			this := args[0]
			idx := int(args[1].Number())
			if this == nil || this.arrayVal == nil {
				return NewUndefined()
			}
			if idx < 0 {
				idx = len(this.arrayVal) + idx
			}
			if idx < 0 || idx >= len(this.arrayVal) {
				return NewUndefined()
			}
			return this.arrayVal[idx]
		}).MarkAsMethod(),
		Writable:     true,
		Enumerable:   false,
		Configurable: true,
	})

	// ArrayPrototype sort: arr.sort(compareFn?) sorts in place and returns the array.
	ArrayPrototype.DefineProperty("sort", &PropertyDescriptor{
		Value: NewFunction(func(args ...*JSValue) *JSValue {
			if len(args) < 1 || args[0] == nil || args[0].arrayVal == nil {
				return NewArray()
			}
			this := args[0]
			var compareFn *JSValue
			if len(args) >= 2 {
				compareFn = args[1]
			}
			sort.SliceStable(this.arrayVal, func(i, j int) bool {
				a, b := this.arrayVal[i], this.arrayVal[j]
				if compareFn != nil && compareFn.funcVal != nil {
					result := compareFn.Call(a, b)
					return result.Number() < 0
				}
				// Default: string comparison
				return a.String() < b.String()
			})
			return this
		}).MarkAsMethod(),
		Writable:     true,
		Enumerable:   false,
		Configurable: true,
	})

	// ArrayPrototype entries: arr.entries() returns an array of [index, value] pairs.
	ArrayPrototype.DefineProperty("entries", &PropertyDescriptor{
		Value: NewFunction(func(args ...*JSValue) *JSValue {
			if len(args) < 1 || args[0] == nil || args[0].arrayVal == nil {
				return NewArray()
			}
			this := args[0]
			entries := make([]*JSValue, len(this.arrayVal))
			for i, v := range this.arrayVal {
				entries[i] = NewArray(NewNumber(float64(i)), v)
			}
			return NewArray(entries...)
		}).MarkAsMethod(),
		Writable:     true,
		Enumerable:   false,
		Configurable: true,
	})

	// --- Additional ArrayPrototype methods ---

	// arr.push(...items) — mutates, returns new length
	defMethod(ArrayPrototype, "push", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil {
			return NewNumber(0)
		}
		this := args[0]
		this.arrayVal = append(this.arrayVal, args[1:]...)
		return NewNumber(float64(len(this.arrayVal)))
	})

	// arr.pop() — mutates, returns removed element
	defMethod(ArrayPrototype, "pop", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil || args[0].arrayVal == nil || len(args[0].arrayVal) == 0 {
			return NewUndefined()
		}
		this := args[0]
		last := this.arrayVal[len(this.arrayVal)-1]
		this.arrayVal = this.arrayVal[:len(this.arrayVal)-1]
		return last
	})

	// arr.shift() — mutates, returns removed first element
	defMethod(ArrayPrototype, "shift", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil || args[0].arrayVal == nil || len(args[0].arrayVal) == 0 {
			return NewUndefined()
		}
		this := args[0]
		first := this.arrayVal[0]
		this.arrayVal = this.arrayVal[1:]
		return first
	})

	// arr.unshift(...items) — mutates, returns new length
	defMethod(ArrayPrototype, "unshift", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil {
			return NewNumber(0)
		}
		this := args[0]
		this.arrayVal = append(args[1:], this.arrayVal...)
		return NewNumber(float64(len(this.arrayVal)))
	})

	// arr.splice(start, deleteCount, ...items) — mutates, returns removed elements
	defMethod(ArrayPrototype, "splice", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil || args[0].arrayVal == nil {
			return NewArray()
		}
		this := args[0]
		rest := args[1:]
		length := len(this.arrayVal)
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
		removed := make([]*JSValue, deleteCount)
		copy(removed, this.arrayVal[start:start+deleteCount])
		var newItems []*JSValue
		for i := 2; i < len(rest); i++ {
			newItems = append(newItems, rest[i])
		}
		result := make([]*JSValue, 0, length-deleteCount+len(newItems))
		result = append(result, this.arrayVal[:start]...)
		result = append(result, newItems...)
		result = append(result, this.arrayVal[start+deleteCount:]...)
		this.arrayVal = result
		return NewArray(removed...)
	})

	// arr.indexOf(value) — returns index or -1
	defMethod(ArrayPrototype, "indexOf", func(args ...*JSValue) *JSValue {
		if len(args) < 2 || args[0] == nil || args[0].arrayVal == nil {
			return NewNumber(-1)
		}
		for i, elem := range args[0].arrayVal {
			if jsValueEqual(elem, args[1]) {
				return NewNumber(float64(i))
			}
		}
		return NewNumber(-1)
	})

	// arr.findIndex(fn) — returns index or -1
	defMethod(ArrayPrototype, "findIndex", func(args ...*JSValue) *JSValue {
		if len(args) < 2 || args[0] == nil || args[0].arrayVal == nil {
			return NewNumber(-1)
		}
		fn := args[1]
		if fn == nil || fn.funcVal == nil {
			return NewNumber(-1)
		}
		for i, elem := range args[0].arrayVal {
			if Truthy(fn.funcVal(elem, NewNumber(float64(i)))) {
				return NewNumber(float64(i))
			}
		}
		return NewNumber(-1)
	})

	// arr.map(fn) — returns new array
	defMethod(ArrayPrototype, "map", func(args ...*JSValue) *JSValue {
		if len(args) < 2 || args[0] == nil || args[0].arrayVal == nil {
			return NewArray()
		}
		fn := args[1]
		if fn == nil || fn.funcVal == nil {
			return NewArray()
		}
		results := make([]*JSValue, len(args[0].arrayVal))
		for i, elem := range args[0].arrayVal {
			results[i] = fn.funcVal(elem, NewNumber(float64(i)))
		}
		return NewArray(results...)
	})

	// arr.filter(fn) — returns new array
	defMethod(ArrayPrototype, "filter", func(args ...*JSValue) *JSValue {
		if len(args) < 2 || args[0] == nil || args[0].arrayVal == nil {
			return NewArray()
		}
		fn := args[1]
		if fn == nil || fn.funcVal == nil {
			return NewArray()
		}
		var results []*JSValue
		for i, elem := range args[0].arrayVal {
			r := fn.funcVal(elem, NewNumber(float64(i)))
			if r != nil && r.Bool() {
				results = append(results, elem)
			}
		}
		return NewArray(results...)
	})

	// arr.forEach(fn)
	defMethod(ArrayPrototype, "forEach", func(args ...*JSValue) *JSValue {
		if len(args) < 2 || args[0] == nil || args[0].arrayVal == nil {
			return NewUndefined()
		}
		fn := args[1]
		if fn == nil || fn.funcVal == nil {
			return NewUndefined()
		}
		for i, elem := range args[0].arrayVal {
			fn.funcVal(elem, NewNumber(float64(i)))
		}
		return NewUndefined()
	})

	// arr.find(fn) — returns first matching element or undefined
	defMethod(ArrayPrototype, "find", func(args ...*JSValue) *JSValue {
		if len(args) < 2 || args[0] == nil || args[0].arrayVal == nil {
			return NewUndefined()
		}
		fn := args[1]
		if fn == nil || fn.funcVal == nil {
			return NewUndefined()
		}
		for _, elem := range args[0].arrayVal {
			if Truthy(fn.funcVal(elem)) {
				return elem
			}
		}
		return NewUndefined()
	})

	// arr.some(fn) — returns true if any element matches
	defMethod(ArrayPrototype, "some", func(args ...*JSValue) *JSValue {
		if len(args) < 2 || args[0] == nil || args[0].arrayVal == nil {
			return NewBool(false)
		}
		fn := args[1]
		if fn == nil || fn.funcVal == nil {
			return NewBool(false)
		}
		for _, elem := range args[0].arrayVal {
			if Truthy(fn.funcVal(elem)) {
				return NewBool(true)
			}
		}
		return NewBool(false)
	})

	// arr.every(fn) — returns true if all elements match
	defMethod(ArrayPrototype, "every", func(args ...*JSValue) *JSValue {
		if len(args) < 2 || args[0] == nil || args[0].arrayVal == nil {
			return NewBool(true)
		}
		fn := args[1]
		if fn == nil || fn.funcVal == nil {
			return NewBool(true)
		}
		for _, elem := range args[0].arrayVal {
			if !Truthy(fn.funcVal(elem)) {
				return NewBool(false)
			}
		}
		return NewBool(true)
	})

	// arr.reduce(fn, initial?) — returns accumulated value
	defMethod(ArrayPrototype, "reduce", func(args ...*JSValue) *JSValue {
		if len(args) < 2 || args[0] == nil || args[0].arrayVal == nil {
			if len(args) >= 3 {
				return args[2]
			}
			return NewUndefined()
		}
		fn := args[1]
		if fn == nil || fn.funcVal == nil {
			return NewUndefined()
		}
		arr := args[0].arrayVal
		var acc *JSValue
		startIdx := 0
		if len(args) >= 3 {
			acc = args[2]
		} else if len(arr) > 0 {
			acc = arr[0]
			startIdx = 1
		} else {
			return NewUndefined()
		}
		for i := startIdx; i < len(arr); i++ {
			acc = fn.funcVal(acc, arr[i])
		}
		return acc
	})

	// arr.flat() — flattens one level
	defMethod(ArrayPrototype, "flat", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil || args[0].arrayVal == nil {
			return NewArray()
		}
		var result []*JSValue
		for _, elem := range args[0].arrayVal {
			if elem != nil && elem.arrayVal != nil {
				result = append(result, elem.arrayVal...)
			} else {
				result = append(result, elem)
			}
		}
		return NewArray(result...)
	})

	// arr.flatMap(fn) — map then flatten one level
	defMethod(ArrayPrototype, "flatMap", func(args ...*JSValue) *JSValue {
		if len(args) < 2 || args[0] == nil || args[0].arrayVal == nil {
			return NewArray()
		}
		fn := args[1]
		if fn == nil || fn.funcVal == nil {
			return NewArray()
		}
		var result []*JSValue
		for i, elem := range args[0].arrayVal {
			r := fn.funcVal(elem, NewNumber(float64(i)))
			if r != nil && r.arrayVal != nil {
				result = append(result, r.arrayVal...)
			} else {
				result = append(result, r)
			}
		}
		return NewArray(result...)
	})

	// --- MapPrototype ---

	MapPrototype = &JSValue{
		typ:        TypeObject,
		properties: make(map[string]*PropertyDescriptor),
		prototype:  ObjectPrototype,
	}

	defGetter(MapPrototype, "size", func(this *JSValue) *JSValue {
		if this == nil || this.mapVal == nil {
			return NewNumber(0)
		}
		return NewNumber(float64(len(this.mapVal.entries)))
	})

	defMethod(MapPrototype, "get", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil || args[0].mapVal == nil {
			return NewUndefined()
		}
		this := args[0]
		if len(args) < 2 {
			return NewUndefined()
		}
		if i := this.mapVal.find(args[1]); i >= 0 {
			return this.mapVal.entries[i].value
		}
		return NewUndefined()
	})

	defMethod(MapPrototype, "set", func(args ...*JSValue) *JSValue {
		if len(args) < 3 || args[0] == nil || args[0].mapVal == nil {
			if len(args) > 0 {
				return args[0]
			}
			return NewUndefined()
		}
		this := args[0]
		key, value := args[1], args[2]
		if i := this.mapVal.find(key); i >= 0 {
			this.mapVal.entries[i].value = value
		} else {
			this.mapVal.entries = append(this.mapVal.entries, &jsMapEntry{key, value})
		}
		return this
	})

	defMethod(MapPrototype, "has", func(args ...*JSValue) *JSValue {
		if len(args) < 2 || args[0] == nil || args[0].mapVal == nil {
			return NewBool(false)
		}
		return NewBool(args[0].mapVal.find(args[1]) >= 0)
	})

	defMethod(MapPrototype, "delete", func(args ...*JSValue) *JSValue {
		if len(args) < 2 || args[0] == nil || args[0].mapVal == nil {
			return NewBool(false)
		}
		this := args[0]
		if i := this.mapVal.find(args[1]); i >= 0 {
			this.mapVal.entries = append(this.mapVal.entries[:i], this.mapVal.entries[i+1:]...)
			return NewBool(true)
		}
		return NewBool(false)
	})

	defMethod(MapPrototype, "clear", func(args ...*JSValue) *JSValue {
		if len(args) >= 1 && args[0] != nil && args[0].mapVal != nil {
			args[0].mapVal.entries = nil
		}
		return NewUndefined()
	})

	defMethod(MapPrototype, "keys", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil || args[0].mapVal == nil {
			return NewArray()
		}
		keys := make([]*JSValue, len(args[0].mapVal.entries))
		for i, e := range args[0].mapVal.entries {
			keys[i] = e.key
		}
		return NewArray(keys...)
	})

	defMethod(MapPrototype, "values", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil || args[0].mapVal == nil {
			return NewArray()
		}
		vals := make([]*JSValue, len(args[0].mapVal.entries))
		for i, e := range args[0].mapVal.entries {
			vals[i] = e.value
		}
		return NewArray(vals...)
	})

	defMethod(MapPrototype, "entries", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil || args[0].mapVal == nil {
			return NewArray()
		}
		pairs := make([]*JSValue, len(args[0].mapVal.entries))
		for i, e := range args[0].mapVal.entries {
			pairs[i] = NewArray(e.key, e.value)
		}
		return NewArray(pairs...)
	})

	defMethod(MapPrototype, "forEach", func(args ...*JSValue) *JSValue {
		if len(args) < 2 || args[0] == nil || args[0].mapVal == nil {
			return NewUndefined()
		}
		fn := args[1]
		if fn == nil || fn.funcVal == nil {
			return NewUndefined()
		}
		for _, e := range args[0].mapVal.entries {
			fn.funcVal(e.value, e.key)
		}
		return NewUndefined()
	})

	// --- SetPrototype ---

	SetPrototype = &JSValue{
		typ:        TypeObject,
		properties: make(map[string]*PropertyDescriptor),
		prototype:  ObjectPrototype,
	}

	defGetter(SetPrototype, "size", func(this *JSValue) *JSValue {
		if this == nil || this.setVal == nil {
			return NewNumber(0)
		}
		return NewNumber(float64(len(this.setVal.items)))
	})

	defMethod(SetPrototype, "add", func(args ...*JSValue) *JSValue {
		if len(args) < 2 || args[0] == nil || args[0].setVal == nil {
			if len(args) > 0 {
				return args[0]
			}
			return NewUndefined()
		}
		this := args[0]
		if this.setVal.find(args[1]) < 0 {
			this.setVal.items = append(this.setVal.items, args[1])
		}
		return this
	})

	defMethod(SetPrototype, "has", func(args ...*JSValue) *JSValue {
		if len(args) < 2 || args[0] == nil || args[0].setVal == nil {
			return NewBool(false)
		}
		return NewBool(args[0].setVal.find(args[1]) >= 0)
	})

	defMethod(SetPrototype, "delete", func(args ...*JSValue) *JSValue {
		if len(args) < 2 || args[0] == nil || args[0].setVal == nil {
			return NewBool(false)
		}
		this := args[0]
		if i := this.setVal.find(args[1]); i >= 0 {
			this.setVal.items = append(this.setVal.items[:i], this.setVal.items[i+1:]...)
			return NewBool(true)
		}
		return NewBool(false)
	})

	defMethod(SetPrototype, "clear", func(args ...*JSValue) *JSValue {
		if len(args) >= 1 && args[0] != nil && args[0].setVal != nil {
			args[0].setVal.items = nil
		}
		return NewUndefined()
	})

	defMethod(SetPrototype, "values", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil || args[0].setVal == nil {
			return NewArray()
		}
		elems := make([]*JSValue, len(args[0].setVal.items))
		copy(elems, args[0].setVal.items)
		return NewArray(elems...)
	})

	defMethod(SetPrototype, "keys", func(args ...*JSValue) *JSValue {
		// Set.keys() === Set.values() per JS spec
		if len(args) < 1 || args[0] == nil || args[0].setVal == nil {
			return NewArray()
		}
		elems := make([]*JSValue, len(args[0].setVal.items))
		copy(elems, args[0].setVal.items)
		return NewArray(elems...)
	})

	defMethod(SetPrototype, "entries", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil || args[0].setVal == nil {
			return NewArray()
		}
		pairs := make([]*JSValue, len(args[0].setVal.items))
		for i, item := range args[0].setVal.items {
			pairs[i] = NewArray(item, item) // [value, value] per JS spec
		}
		return NewArray(pairs...)
	})

	defMethod(SetPrototype, "forEach", func(args ...*JSValue) *JSValue {
		if len(args) < 2 || args[0] == nil || args[0].setVal == nil {
			return NewUndefined()
		}
		fn := args[1]
		if fn == nil || fn.funcVal == nil {
			return NewUndefined()
		}
		for _, item := range args[0].setVal.items {
			fn.funcVal(item)
		}
		return NewUndefined()
	})
}
