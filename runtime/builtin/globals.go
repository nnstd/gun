package jsvalue

import (
	"encoding/base64"
	gomath "math"
	"time"
)

func newTypeErrorJSValue(message string) *JSValue {
	if ctor, ok := Globals()["TypeError"]; ok && ctor != nil {
		return ctor.Call(NewString(message))
	}
	err := NewObject()
	err.Set("name", NewString("TypeError"))
	err.Set("message", NewString(message))
	return err
}

func newRangeErrorJSValue(message string) *JSValue {
	if ctor, ok := Globals()["RangeError"]; ok && ctor != nil {
		return ctor.Call(NewString(message))
	}
	err := NewObject()
	err.Set("name", NewString("RangeError"))
	err.Set("message", NewString(message))
	return err
}

var Object *JSValue
var Array *JSValue
var String *JSValue
var Boolean *JSValue
var Number *JSValue
var BigIntCtor *JSValue
var Symbol_ *JSValue
var Reflect *JSValue
var DateCtor *JSValue
var MapCtor *JSValue
var SetCtor *JSValue
var Uint8ArrayCtor *JSValue
var Atob *JSValue
var Btoa *JSValue

// CompileFunctionFn is set by runtime/dynfunc to provide new Function() support.
// When nil, new Function() returns a no-op function.
var CompileFunctionFn func(args ...*JSValue) *JSValue

var FunctionCtor *JSValue

func init() {
	Object = NewFunction(func(args ...*JSValue) *JSValue {
		if len(args) == 0 || args[0] == nil || args[0].typ == TypeUndefined || args[0].typ == TypeNull {
			return NewObject()
		}
		if args[0].typ == TypeObject || args[0].typ == TypeFunction {
			return args[0]
		}
		return boxedPrimitiveOf(args[0])
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
			return OwnPropertyNames(args[0])
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
			if args[0] == nil || args[0].typ == TypeUndefined || args[0].typ == TypeNull {
				panic(newTypeErrorJSValue("Object.setPrototypeOf called on non-object"))
			}
			if args[0].isPrimitiveValue() {
				return args[0]
			}
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

	BigIntCtor = NewFunction(func(args ...*JSValue) *JSValue {
		return BigInt(args...)
	})
	BigIntCtor.Set("prototype", BigIntPrototype)

	String = NewFunction(func(args ...*JSValue) *JSValue {
		if len(args) == 0 {
			return NewString("")
		}
		if args[0] != nil && args[0].isBoxedPrimitive() && args[0].boxedValue != nil && args[0].boxedValue.Type() == TypeSymbol {
			panic(newTypeErrorJSValue("Cannot convert a symbol to a string"))
		}
		return NewString(args[0].String())
	})

	Boolean = NewFunction(func(args ...*JSValue) *JSValue {
		if len(args) == 0 {
			return NewBool(false)
		}
		return NewBool(args[0].Bool())
	})

	Symbol_ = NewFunction(func(args ...*JSValue) *JSValue {
		desc := ""
		if len(args) > 0 && args[0] != nil {
			desc = args[0].String()
		}
		return NewSymbol(desc)
	})
	Symbol_.Set("prototype", SymbolPrototype)
	Symbol_.Set("hasInstance", NewSymbol("Symbol.hasInstance"))
	Symbol_.Set("iterator", NewSymbol("Symbol.iterator"))
	Symbol_.Set("toPrimitive", NewSymbol("Symbol.toPrimitive"))
	Symbol_.Set("toStringTag", NewSymbol("Symbol.toStringTag"))
	Symbol_.Set("for", NewFunction(func(args ...*JSValue) *JSValue {
		desc := ""
		if len(args) > 0 && args[0] != nil {
			desc = args[0].String()
		}
		return NewSymbol(desc)
	}))

	Reflect = NewObject()
	Reflect.Set("ownKeys", NewFunction(func(args ...*JSValue) *JSValue {
		if len(args) > 0 {
			return Keys(args[0])
		}
		return NewArray()
	}))
	Reflect.Set("get", NewFunction(func(args ...*JSValue) *JSValue {
		if len(args) >= 2 {
			return args[0].Get(PropertyKey(args[1]))
		}
		return NewUndefined()
	}))
	Reflect.Set("set", NewFunction(func(args ...*JSValue) *JSValue {
		if len(args) >= 3 {
			if args[0] == nil || args[0].typ == TypeUndefined || args[0].typ == TypeNull || args[0].isPrimitiveValue() {
				panic(newTypeErrorJSValue("Reflect.set requires the first argument be an object"))
			}
			args[0].Set(PropertyKey(args[1]), args[2])
			return NewBool(true)
		}
		return NewBool(false)
	}))
	Reflect.Set("has", NewFunction(func(args ...*JSValue) *JSValue {
		if len(args) >= 2 {
			return NewBool(args[0].HasOwnProperty(PropertyKey(args[1])))
		}
		return NewBool(false)
	}))
	Reflect.Set("apply", NewFunction(func(args ...*JSValue) *JSValue {
		if len(args) >= 3 {
			callArgs := args[2].Array()
			return args[0].Call(callArgs...)
		}
		return NewUndefined()
	}))

	DateCtor = NewFunction(func(args ...*JSValue) *JSValue {
		if len(args) == 0 {
			return NewString(time.Now().Format(time.RFC1123Z))
		}
		return NewObject()
	})
	DateCtor.Set("now", NewFunction(func(args ...*JSValue) *JSValue {
		return NewNumber(float64(time.Now().UnixMilli()))
	}))

	MapCtor = NewFunction(func(args ...*JSValue) *JSValue {
		return NewMap()
	})

	SetCtor = NewFunction(func(args ...*JSValue) *JSValue {
		return NewSet()
	})

	Uint8ArrayCtor = NewFunction(func(args ...*JSValue) *JSValue {
		return NewArray()
	})

	Atob = NewFunction(func(args ...*JSValue) *JSValue {
		return AtobFunc(args...)
	})

	Btoa = NewFunction(func(args ...*JSValue) *JSValue {
		return BtoaFunc(args...)
	})

	// FunctionCtor implements new Function(paramNames..., body) via the HIR interpreter.
	// CompileFunctionFn is wired by runtime/dynfunc.
	FunctionCtor = NewFunction(func(args ...*JSValue) *JSValue {
		if CompileFunctionFn == nil {
			return NewFunction(func(args ...*JSValue) *JSValue { return NewUndefined() })
		}
		if len(args) == 0 {
			return NewFunction(func(args ...*JSValue) *JSValue { return NewUndefined() })
		}
		return CompileFunctionFn(args...)
	})

	// ArrayBuffer, SharedArrayBuffer, and all TypedArray constructors
	initTypedArrays()

	// DataView (depends on ArrayBufferCtor from initTypedArrays)
	initDataView()
}

// AtobFunc decodes a base64-encoded string.
func AtobFunc(args ...*JSValue) *JSValue {
	if len(args) > 0 && args[0] != nil {
		decoded, err := base64.StdEncoding.DecodeString(args[0].String())
		if err == nil {
			return NewString(string(decoded))
		}
	}
	return NewString("")
}

// BtoaFunc encodes a string to base64.
func BtoaFunc(args ...*JSValue) *JSValue {
	if len(args) > 0 && args[0] != nil {
		return NewString(base64.StdEncoding.EncodeToString([]byte(args[0].String())))
	}
	return NewString("")
}

// --- Global registry ---

// globalRegistry maps JS global names to their *JSValue runtime values.
// Packages register their globals via RegisterGlobal in init().
// The HIR interpreter and any runtime consumer reads via Globals().
var globalRegistry = make(map[string]*JSValue)

// RegisterGlobal adds a named global value to the registry.
func RegisterGlobal(name string, val *JSValue) {
	globalRegistry[name] = val
}

// Globals returns all registered global JS values.
func Globals() map[string]*JSValue {
	return globalRegistry
}

func init() {
	// Core constructors
	RegisterGlobal("Object", Object)
	RegisterGlobal("Array", Array)
	RegisterGlobal("Number", Number)
	RegisterGlobal("BigInt", BigIntCtor)
	RegisterGlobal("Symbol", Symbol_)
	RegisterGlobal("Reflect", Reflect)
	RegisterGlobal("Date", DateCtor)
	RegisterGlobal("Map", MapCtor)
	RegisterGlobal("Set", SetCtor)
	RegisterGlobal("Uint8Array", Uint8ArrayCtor)
	RegisterGlobal("atob", Atob)
	RegisterGlobal("btoa", Btoa)
	RegisterGlobal("RegExp", RegexpCtor)
	RegisterGlobal("Function", FunctionCtor)

	// Global functions
	RegisterGlobal("parseInt", NewFunction(func(args ...*JSValue) *JSValue {
		if len(args) == 0 {
			return NewNumber(gomath.NaN())
		}
		base := NewNumber(10)
		if len(args) > 1 {
			base = args[1]
		}
		return ParseInt(args[0], base)
	}))
	RegisterGlobal("parseFloat", NewFunction(func(args ...*JSValue) *JSValue {
		if len(args) == 0 {
			return NewNumber(gomath.NaN())
		}
		return ParseFloat(args[0])
	}))
	RegisterGlobal("isNaN", NewFunction(func(args ...*JSValue) *JSValue {
		if len(args) == 0 {
			return NewBool(true)
		}
		return NewBool(gomath.IsNaN(args[0].Number()))
	}))
	RegisterGlobal("isFinite", NewFunction(func(args ...*JSValue) *JSValue {
		if len(args) == 0 {
			return NewBool(false)
		}
		n := args[0].Number()
		return NewBool(!gomath.IsInf(n, 0) && !gomath.IsNaN(n))
	}))
	RegisterGlobal("String", String)
	RegisterGlobal("Boolean", Boolean)

	// Global constants
	RegisterGlobal("undefined", NewUndefined())
	RegisterGlobal("NaN", NewNumber(gomath.NaN()))
	RegisterGlobal("Infinity", NewNumber(gomath.Inf(1)))
	RegisterGlobal("globalThis", NewObject())
}
