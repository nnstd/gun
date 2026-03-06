package jsvalue

// NewObject creates an empty object JSValue.
func NewObject() *JSValue {
	return &JSValue{
		typ:        TypeObject,
		properties: make(map[string]*PropertyDescriptor),
		prototype:  ObjectPrototype,
	}
}

// ObjectFrom creates an object from alternating key (string) and value (*JSValue) pairs.
func ObjectFrom(pairs ...any) *JSValue {
	obj := NewObject()
	for i := 0; i+1 < len(pairs); i += 2 {
		key, _ := pairs[i].(string)
		val, _ := pairs[i+1].(*JSValue)
		if val == nil {
			val = NewUndefined()
		}
		obj.Set(key, val)
	}
	return obj
}

// Keys returns the keys of an object as a JSValue array.
// Implements Object.keys() semantics.
func Keys(obj *JSValue) *JSValue {
	if obj == nil {
		return NewArray()
	}
	keys := obj.OwnKeys()
	result := make([]*JSValue, len(keys))
	for i, key := range keys {
		result[i] = NewString(key)
	}
	return NewArray(result...)
}

// NewClass creates a JS class: a constructor function with a prototype object.
// The constructor's "prototype" property holds the prototype that instances inherit from.
// parent is the parent class (or nil for no inheritance).
func NewClass(constructor func(this *JSValue, args ...*JSValue) *JSValue, parent *JSValue) *JSValue {
	proto := NewObject()
	if parent != nil {
		// Inheritance: Child.prototype.__proto__ = Parent.prototype
		parentProto := parent.Get("prototype")
		if parentProto != nil && parentProto.typ != TypeUndefined {
			proto.SetPrototype(parentProto)
		}
	}

	ctor := NewFunction(func(args ...*JSValue) *JSValue {
		// new Class(args): create instance with prototype chain
		instance := NewObject()
		instance.SetPrototype(proto)
		result := constructor(instance, args...)
		// If constructor explicitly returns an object, use it; otherwise use instance
		if result != nil && result.typ == TypeObject {
			return result
		}
		return instance
	})

	// Store the raw constructor for super() calls via CallSuper
	ctor.classInit = constructor

	// Set up Class.prototype and Class.prototype.constructor
	ctor.Set("prototype", proto)
	proto.Set("constructor", ctor)

	// Inherit static methods from parent
	if parent != nil {
		ctor.SetPrototype(parent)
	}

	return ctor
}
