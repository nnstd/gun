package jsvalue

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
			return NewString("")
		}),
		Writable:     true,
		Enumerable:   false,
		Configurable: true,
	})

	// NumberPrototype toString returns the number as string.
	NumberPrototype.DefineProperty("toString", &PropertyDescriptor{
		Value: NewFunction(func(args ...*JSValue) *JSValue {
			return NewString("0")
		}),
		Writable:     true,
		Enumerable:   false,
		Configurable: true,
	})

	// BooleanPrototype toString.
	BooleanPrototype.DefineProperty("toString", &PropertyDescriptor{
		Value: NewFunction(func(args ...*JSValue) *JSValue {
			return NewString("false")
		}),
		Writable:     true,
		Enumerable:   false,
		Configurable: true,
	})

	// ArrayPrototype fill: arr.fill(value) fills all elements with value.
	// In the transpiled context, this is called as .Get("fill").Call(this, value).
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
		}),
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
		}),
		Writable:     true,
		Enumerable:   false,
		Configurable: true,
	})
}
