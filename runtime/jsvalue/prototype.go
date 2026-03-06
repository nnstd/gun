package jsvalue

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

// defMethod defines a prototype method using the MarkAsMethod convention.
// Package-level so initXxxPrototype functions in other files can use it.
func defMethod(proto *JSValue, name string, fn func(args ...*JSValue) *JSValue) {
	proto.DefineProperty(name, &PropertyDescriptor{
		Value: NewFunction(fn).MarkAsMethod(), Writable: true, Enumerable: false, Configurable: true,
	})
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
		typ:        TypeObject,
		properties: make(map[string]*PropertyDescriptor),
	}

	// Wire up child prototypes, all inheriting from ObjectPrototype.
	StringPrototype = &JSValue{typ: TypeObject, properties: make(map[string]*PropertyDescriptor), prototype: ObjectPrototype}
	NumberPrototype = &JSValue{typ: TypeObject, properties: make(map[string]*PropertyDescriptor), prototype: ObjectPrototype}
	BigIntPrototype = &JSValue{typ: TypeObject, properties: make(map[string]*PropertyDescriptor), prototype: ObjectPrototype}
	BooleanPrototype = &JSValue{typ: TypeObject, properties: make(map[string]*PropertyDescriptor), prototype: ObjectPrototype}
	SymbolPrototype = &JSValue{typ: TypeObject, properties: make(map[string]*PropertyDescriptor), prototype: ObjectPrototype}
	ArrayPrototype = &JSValue{typ: TypeObject, properties: make(map[string]*PropertyDescriptor), prototype: ObjectPrototype}
	FunctionPrototype = &JSValue{typ: TypeObject, properties: make(map[string]*PropertyDescriptor), prototype: ObjectPrototype}

	// --- FunctionPrototype methods ---
	initFunctionPrototype()

	// --- ObjectPrototype methods ---
	initObjectPrototype()

	// --- BooleanPrototype methods ---
	initBooleanPrototype()

	// --- Per-type prototype methods (in corresponding files) ---
	initStringPrototype()
	initNumberPrototype()
	initArrayPrototype()
	initMapPrototype()
	initSetPrototype()
}

func initFunctionPrototype() {
	bindFn := NewFunction(func(args ...*JSValue) *JSValue {
		if len(args) < 2 { return NewUndefined() }
		origFn := args[0]
		thisArg := args[1]
		boundArgs := args[2:]
		if origFn == nil || origFn.funcVal == nil { return NewUndefined() }
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
		Value: bindFn, Writable: true, Enumerable: false, Configurable: true,
	})

	callFn := NewFunction(func(args ...*JSValue) *JSValue {
		if len(args) < 1 { return NewUndefined() }
		origFn := args[0]
		if origFn == nil || origFn.funcVal == nil { return NewUndefined() }
		return origFn.funcVal(args[1:]...)
	})
	callFn.MarkAsMethod()
	FunctionPrototype.DefineProperty("call", &PropertyDescriptor{
		Value: callFn, Writable: true, Enumerable: false, Configurable: true,
	})

	applyFn := NewFunction(func(args ...*JSValue) *JSValue {
		if len(args) < 1 { return NewUndefined() }
		origFn := args[0]
		if origFn == nil || origFn.funcVal == nil { return NewUndefined() }
		callArgs := []*JSValue{}
		if origFn.isMethod && len(args) >= 2 { callArgs = append(callArgs, args[1]) }
		if len(args) >= 3 && args[2] != nil && args[2].arrayVal != nil {
			callArgs = append(callArgs, args[2].arrayVal...)
		}
		return origFn.funcVal(callArgs...)
	})
	applyFn.MarkAsMethod()
	FunctionPrototype.DefineProperty("apply", &PropertyDescriptor{
		Value: applyFn, Writable: true, Enumerable: false, Configurable: true,
	})
}

func initObjectPrototype() {
	ObjectPrototype.DefineProperty("toString", &PropertyDescriptor{
		Value: NewFunction(func(args ...*JSValue) *JSValue {
			return NewString("[object Object]")
		}),
		Writable: true, Enumerable: false, Configurable: true,
	})
	ObjectPrototype.DefineProperty("hasOwnProperty", &PropertyDescriptor{
		Value: NewFunction(func(args ...*JSValue) *JSValue {
			if len(args) >= 2 { return NewBool(args[0].HasOwnProperty(args[1].String())) }
			return NewBool(false)
		}),
		Writable: true, Enumerable: false, Configurable: true,
	})
	ObjectPrototype.DefineProperty("valueOf", &PropertyDescriptor{
		Value: NewFunction(func(args ...*JSValue) *JSValue { return NewUndefined() }),
		Writable: true, Enumerable: false, Configurable: true,
	})
}

func initBooleanPrototype() {
	BooleanPrototype.DefineProperty("toString", &PropertyDescriptor{
		Value: NewFunction(func(args ...*JSValue) *JSValue {
			if len(args) > 0 && args[0] != nil { return NewString(args[0].String()) }
			return NewString("false")
		}).MarkAsMethod(),
		Writable: true, Enumerable: false, Configurable: true,
	})
}
