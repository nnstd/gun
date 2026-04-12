package jsvalue

import gomath "math"

var Object *JSValue
var Array *JSValue
var Number *JSValue

func init() {
	Object = NewFunction(func(args ...*JSValue) *JSValue {
		if len(args) == 0 || args[0] == nil || args[0].typ == TypeUndefined || args[0].typ == TypeNull {
			return NewObject()
		}
		return args[0]
	})
	Object.Set("prototype", ObjectPrototype)
	Object.Set("create", NewFunction(func(args ...*JSValue) *JSValue {
		if len(args) > 0 {
			return Create(args[0])
		}
		return Create(nil)
	}))
	Object.Set("keys", NewFunction(func(args ...*JSValue) *JSValue {
		if len(args) > 0 {
			return Keys(args[0])
		}
		return NewArray()
	}))
	Object.Set("values", NewFunction(func(args ...*JSValue) *JSValue {
		if len(args) > 0 {
			return Values(args[0])
		}
		return NewArray()
	}))
	Object.Set("entries", NewFunction(func(args ...*JSValue) *JSValue {
		if len(args) > 0 {
			return Entries(args[0])
		}
		return NewArray()
	}))
	Object.Set("fromEntries", NewFunction(func(args ...*JSValue) *JSValue {
		if len(args) > 0 {
			return FromEntries(args[0])
		}
		return NewObject()
	}))
	Object.Set("assign", NewFunction(func(args ...*JSValue) *JSValue {
		if len(args) == 0 {
			return NewObject()
		}
		sources := make([]any, 0, len(args)-1)
		for _, arg := range args[1:] {
			sources = append(sources, arg)
		}
		return Assign(args[0], sources...)
	}))
	Object.Set("freeze", NewFunction(func(args ...*JSValue) *JSValue {
		if len(args) > 0 {
			return args[0]
		}
		return NewUndefined()
	}))
	Object.Set("defineProperty", NewFunction(func(args ...*JSValue) *JSValue {
		if len(args) >= 3 {
			return DefineProperty(args[0], args[1], args[2])
		}
		return NewUndefined()
	}))
	Object.Set("getOwnPropertyNames", NewFunction(func(args ...*JSValue) *JSValue {
		if len(args) > 0 {
			return Keys(args[0])
		}
		return NewArray()
	}))
	Object.Set("getPrototypeOf", NewFunction(func(args ...*JSValue) *JSValue {
		if len(args) > 0 {
			return args[0].GetPrototype()
		}
		return NewNull()
	}))
	Object.Set("setPrototypeOf", NewFunction(func(args ...*JSValue) *JSValue {
		if len(args) >= 2 {
			args[0].SetPrototype(args[1])
			return args[0]
		}
		return NewUndefined()
	}))
	Object.Set("hasOwn", NewFunction(func(args ...*JSValue) *JSValue {
		if len(args) >= 2 {
			return NewBool(args[0].HasOwnProperty(PropertyKey(args[1])))
		}
		return NewBool(false)
	}))

	Array = NewFunction(func(args ...*JSValue) *JSValue {
		return NewArray(args...)
	})
	Array.Set("prototype", ArrayPrototype)
	Array.Set("isArray", NewFunction(func(args ...*JSValue) *JSValue {
		if len(args) > 0 {
			return IsArrayValue(args[0])
		}
		return NewBool(false)
	}))

	Number = NewFunction(func(args ...*JSValue) *JSValue {
		if len(args) > 0 {
			return NewNumber(args[0].Number())
		}
		return NewNumber(0)
	})
	Number.Set("prototype", NumberPrototype)
	Number.Set("isNaN", NewFunction(func(args ...*JSValue) *JSValue {
		if len(args) > 0 {
			return NewBool(args[0].TypeString() == "number" && args[0].Number() != args[0].Number())
		}
		return NewBool(false)
	}))
	Number.Set("isFinite", NewFunction(func(args ...*JSValue) *JSValue {
		if len(args) > 0 {
			n := args[0].Number()
			return NewBool(!gomath.IsNaN(n) && !gomath.IsInf(n, 0))
		}
		return NewBool(false)
	}))
	Number.Set("isInteger", NewFunction(func(args ...*JSValue) *JSValue {
		if len(args) > 0 {
			n := args[0].Number()
			return NewBool(n == float64(int64(n)))
		}
		return NewBool(false)
	}))
	Number.Set("isSafeInteger", Number.Get("isInteger"))
	Number.Set("parseInt", NewFunction(func(args ...*JSValue) *JSValue {
		if len(args) >= 2 {
			return ParseInt(args[0], args[1])
		}
		if len(args) == 1 {
			return ParseInt(args[0], NewNumber(10))
		}
		return NewNumber(0)
	}))
	Number.Set("parseFloat", NewFunction(func(args ...*JSValue) *JSValue {
		if len(args) > 0 {
			return ParseFloat(args[0])
		}
		return NewNumber(0)
	}))
}
