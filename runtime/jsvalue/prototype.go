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
		if len(args) >= 2 {
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
