package jsvalue

// safeArg returns args[i] or nil if out of bounds.
func safeArg(args []*JSValue, i int) *JSValue {
	if i < len(args) {
		return args[i]
	}
	return nil
}

// Prototype is an alias for ObjectPrototype — used when JS code accesses
// Object.prototype (transpiled as jsvalue.Prototype).
var Prototype *JSValue

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

// defMethod defines a prototype method using the MarkAsMethod convention.
// Package-level so initXxxPrototype functions in other files can use it.
func defMethod(proto *JSValue, name string, fn func(args ...*JSValue) *JSValue) {
	proto.DefineProperty(name, newDataDescriptor(NewFunction(fn).MarkAsMethod(), true, false, true))
}

// defGetter defines a prototype getter property.
func defGetter(proto *JSValue, name string, fn func(this *JSValue) *JSValue) {
	proto.DefineProperty(name, &PropertyDescriptor{
		Get: fn, Enumerable: false, Configurable: true,
	})
}

func init() {
	// ObjectPrototype is the root — no parent prototype.
	ObjectPrototype = &JSValue{
		typ: TypeObject,
	}
	Prototype = ObjectPrototype

	// Wire up child prototypes, all inheriting from ObjectPrototype.
	StringPrototype = &JSValue{typ: TypeObject, prototype: ObjectPrototype}
	NumberPrototype = &JSValue{typ: TypeObject, prototype: ObjectPrototype}
	BigIntPrototype = &JSValue{typ: TypeObject, prototype: ObjectPrototype}
	BooleanPrototype = &JSValue{typ: TypeObject, prototype: ObjectPrototype}
	SymbolPrototype = &JSValue{typ: TypeObject, prototype: ObjectPrototype}
	ArrayPrototype = &JSValue{typ: TypeObject, prototype: ObjectPrototype}
	FunctionPrototype = &JSValue{typ: TypeObject, prototype: ObjectPrototype}

	// --- FunctionPrototype methods ---
	initFunctionPrototype()

	// --- ObjectPrototype methods ---
	initObjectPrototype()

	// --- BooleanPrototype methods ---
	initBooleanPrototype()
	initBigIntPrototype()
	initSymbolPrototype()

	// --- Per-type prototype methods (in corresponding files) ---
	initStringPrototype()
	initNumberPrototype()
	initArrayPrototype()
	initMapPrototype()
	initSetPrototype()

	// Patch prototypes onto interned singletons. Direct field writes (not .Set())
	// are intentional: they bypass the frozen guard that protects user-facing
	// mutation paths. This is runtime-internal initialization.
	_true.prototype = BooleanPrototype
	_false.prototype = BooleanPrototype
	_emptyString.prototype = StringPrototype

	// Populate numCache now that NumberPrototype is available.
	initNumCache()

	// Global constructor objects are initialized in a separate init and may run
	// before this one. Patch their prototype properties here so constructor
	// property access is stable regardless of package init order.
	if Boolean != nil {
		Boolean.Set("prototype", BooleanPrototype)
		BooleanPrototype.DefineProperty("constructor", newDataDescriptor(Boolean, true, false, true))
	}
	if String != nil {
		String.Set("prototype", StringPrototype)
		StringPrototype.DefineProperty("constructor", newDataDescriptor(String, true, false, true))
	}
	if Object != nil {
		Object.Set("prototype", ObjectPrototype)
	}
	if Array != nil {
		Array.Set("prototype", ArrayPrototype)
		ArrayPrototype.DefineProperty("constructor", newDataDescriptor(Array, true, false, true))
	}
	if Number != nil {
		Number.Set("prototype", NumberPrototype)
		NumberPrototype.DefineProperty("constructor", newDataDescriptor(Number, true, false, true))
	}
	if BigIntCtor != nil {
		BigIntCtor.Set("prototype", BigIntPrototype)
		BigIntPrototype.DefineProperty("constructor", newDataDescriptor(BigIntCtor, true, false, true))
	}
	if Symbol_ != nil {
		Symbol_.Set("prototype", SymbolPrototype)
		SymbolPrototype.DefineProperty("constructor", newDataDescriptor(Symbol_, true, false, true))
	}
}

func initFunctionPrototype() {
	bindFn := NewFunction(func(args ...*JSValue) *JSValue {
		if len(args) < 2 {
			return NewUndefined()
		}
		origFn := args[0]
		thisArg := args[1]
		boundArgs := args[2:]
		if origFn == nil || origFn.funcVal == nil {
			return NewUndefined()
		}
		isMethod := origFn.isMethod
		return NewFunction(func(callArgs ...*JSValue) *JSValue {
			all := make([]*JSValue, 0, 1+len(boundArgs)+len(callArgs))
			if isMethod {
				all = append(all, thisArg)
			}
			all = append(all, boundArgs...)
			all = append(all, callArgs...)
			return origFn.funcVal(all...)
		})
	})
	bindFn.MarkAsMethod()
	FunctionPrototype.DefineProperty("bind", newDataDescriptor(bindFn, true, false, true))

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
	FunctionPrototype.DefineProperty("call", newDataDescriptor(callFn, true, false, true))

	applyFn := NewFunction(func(args ...*JSValue) *JSValue {
		if len(args) < 1 {
			return NewUndefined()
		}
		origFn := args[0]
		if origFn == nil || origFn.funcVal == nil {
			return NewUndefined()
		}
		callArgs := []*JSValue{}
		if origFn.isMethod && len(args) >= 2 {
			callArgs = append(callArgs, args[1])
		}
		if len(args) >= 3 && args[2] != nil && args[2].isArr {
			callArgs = append(callArgs, args[2].arrayListOrZero().Slice()...)
		}
		return origFn.funcVal(callArgs...)
	})
	applyFn.MarkAsMethod()
	FunctionPrototype.DefineProperty("apply", newDataDescriptor(applyFn, true, false, true))
}

func initObjectPrototype() {
	ObjectPrototype.DefineProperty("toString", &PropertyDescriptor{
		Value: NewFunction(func(args ...*JSValue) *JSValue {
			if len(args) > 0 && args[0] != nil {
				switch args[0].unboxed().typ {
				case TypeBoolean:
					return NewString("[object Boolean]")
				case TypeNumber:
					return NewString("[object Number]")
				case TypeString:
					return NewString("[object String]")
				case TypeBigInt:
					return NewString("[object BigInt]")
				case TypeSymbol:
					return NewString("[object Symbol]")
				}
			}
			return NewString("[object Object]")
		}).MarkAsMethod(),
		Writable: true, Enumerable: false, Configurable: true,
	})
	ObjectPrototype.DefineProperty("hasOwnProperty", &PropertyDescriptor{
		Value: NewFunction(func(args ...*JSValue) *JSValue {
			if len(args) >= 2 {
				return NewBool(args[0].HasOwnProperty(args[1].String()))
			}
			return NewBool(false)
		}).MarkAsMethod(),
		Writable: true, Enumerable: false, Configurable: true,
	})
	ObjectPrototype.DefineProperty("valueOf", &PropertyDescriptor{
		Value: NewFunction(func(args ...*JSValue) *JSValue {
			if len(args) > 0 && args[0] != nil {
				if args[0].boxedValue != nil {
					return args[0].boxedValue
				}
				return args[0]
			}
			return NewUndefined()
		}).MarkAsMethod(),
		Writable: true, Enumerable: false, Configurable: true,
	})
}

func initBooleanPrototype() {
	BooleanPrototype.DefineProperty("toString", &PropertyDescriptor{
		Value: NewFunction(func(args ...*JSValue) *JSValue {
			if len(args) > 0 && args[0] != nil {
				return NewString(args[0].unboxed().String())
			}
			return NewString("false")
		}).MarkAsMethod(),
		Writable: true, Enumerable: false, Configurable: true,
	})
	BooleanPrototype.DefineProperty("valueOf", &PropertyDescriptor{
		Value: NewFunction(func(args ...*JSValue) *JSValue {
			if len(args) > 0 && args[0] != nil {
				return NewBool(args[0].unboxed().Bool())
			}
			return NewBool(false)
		}).MarkAsMethod(),
		Writable: true, Enumerable: false, Configurable: true,
	})
}

func initBigIntPrototype() {
	BigIntPrototype.DefineProperty("toString", &PropertyDescriptor{
		Value: NewFunction(func(args ...*JSValue) *JSValue {
			if len(args) > 0 && args[0] != nil {
				return NewString(args[0].unboxed().String())
			}
			return NewString("0")
		}).MarkAsMethod(),
		Writable: true, Enumerable: false, Configurable: true,
	})
	BigIntPrototype.DefineProperty("valueOf", &PropertyDescriptor{
		Value: NewFunction(func(args ...*JSValue) *JSValue {
			if len(args) > 0 && args[0] != nil {
				return NewBigInt(args[0].unboxed().bigIntVal)
			}
			return NewBigInt(0)
		}).MarkAsMethod(),
		Writable: true, Enumerable: false, Configurable: true,
	})
}

func initSymbolPrototype() {
	SymbolPrototype.DefineProperty("toString", &PropertyDescriptor{
		Value: NewFunction(func(args ...*JSValue) *JSValue {
			if len(args) > 0 && args[0] != nil {
				return NewString(args[0].unboxed().String())
			}
			return NewString("Symbol()")
		}).MarkAsMethod(),
		Writable: true, Enumerable: false, Configurable: true,
	})
	SymbolPrototype.DefineProperty("valueOf", &PropertyDescriptor{
		Value: NewFunction(func(args ...*JSValue) *JSValue {
			if len(args) > 0 && args[0] != nil {
				return args[0].unboxed()
			}
			return NewUndefined()
		}).MarkAsMethod(),
		Writable: true, Enumerable: false, Configurable: true,
	})
}
