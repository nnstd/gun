package jsvalue

// parseArrayIndex checks if a property name is a valid non-negative integer
// index (like "0", "1", "42") and returns the index. This enables JS-style
// arr["0"] === arr[0] semantics.
func parseArrayIndex(name string) (int, bool) {
	if len(name) == 0 {
		return 0, false
	}
	if name[0] == '0' {
		return 0, len(name) == 1 // "0" is valid, "01" is not
	}

	const maxInt = int(^uint(0) >> 1)
	n := 0
	for i := 0; i < len(name); i++ {
		c := name[i]
		if c < '0' || c > '9' {
			return 0, false
		}
		digit := int(c - '0')
		if n > maxInt/10 || (n == maxInt/10 && digit > maxInt%10) {
			return 0, false
		}
		n = n*10 + digit
	}
	return n, true
}

// PropertyDescriptor describes a single property on a JSValue.
type PropertyDescriptor struct {
	Value        *JSValue
	Writable     bool
	Enumerable   bool
	Configurable bool
	Get          func(*JSValue) *JSValue
	Set          func(*JSValue, *JSValue)
}

func newDataDescriptor(value *JSValue, writable, enumerable, configurable bool) *PropertyDescriptor {
	return &PropertyDescriptor{
		Value:        value,
		Writable:     writable,
		Enumerable:   enumerable,
		Configurable: configurable,
	}
}

func newWritableEnumerableDataDescriptor(value *JSValue) *PropertyDescriptor {
	return newDataDescriptor(value, true, true, true)
}

// Get retrieves a property by name. All JS code runs on the event loop
// goroutine, so no locking is needed for property access.
func (v *JSValue) Get(name string) *JSValue {
	if v == nil {
		return NewUndefined()
	}
	if v.isBoxedPrimitive() {
		if props := v.propertiesOrNil(); props != nil {
			if desc, ok := props.Get(name); ok {
				if desc.Get != nil {
					return desc.Get(v)
				}
				return desc.Value
			}
		}
	}
	if name == "__proto__" {
		return v.GetPrototype()
	}
	if !v.isArr && v.typ != TypeString && v.extOrNil() == nil && v.prototype == nil {
		return NewUndefined()
	}

	// Array/string fast paths
	if v.isArr {
		if len(name) == 1 {
			c := name[0]
			if c >= '0' && c <= '9' {
				idx := int(c - '0')
				if arr := v.arrayListOrNil(); arr != nil && idx < arr.Len() {
					return arr.Get(idx)
				}
				return NewUndefined()
			}
		}
		if idx, ok := parseArrayIndex(name); ok {
			if arr := v.arrayListOrNil(); arr != nil && idx < arr.Len() {
				return arr.Get(idx)
			}
			return NewUndefined()
		}
	}
	if v.typ == TypeString {
		if idx, ok := parseArrayIndex(name); ok {
			return v.Index(idx)
		}
	}

	ext := v.extOrNil()
	if ext == nil {
		return v.prototypeGet(name)
	}

	// Inline cache check (lock-free: event loop serializes access)
	meta := ext.meta.Load()
	if meta != nil {
		recvGen := meta.gen.Load()
		cache := &meta.cache
		for i := range cache {
			e := &cache[i]
			if e.key == name && e.source != nil && e.recvGen == recvGen && e.gen == e.source.genLoad() {
				if e.getter != nil {
					return e.getter(v)
				}
				return e.value
			}
		}
	}

	// Cache miss: walk property chain and cache result
	return v.lookupAndCache(name, ext, meta)
}

// lookupAndCache walks the prototype chain, caches the result, and returns the value.
func (v *JSValue) lookupAndCache(name string, ext *jsValueExt, meta *jsValueMeta) *JSValue {
	var next *JSValue
	for cur := v; cur != nil; cur = next {
		next = cur.prototype
		curExt := cur.extOrNil()
		if curExt == nil {
			continue
		}
		if desc, ok := curExt.properties.Get(name); ok {
			if meta != nil {
				cache := &meta.cache
				copy(cache[1:], cache[:3])
				cache[0] = cacheEntry{
					key:     name,
					value:   desc.Value,
					getter:  desc.Get,
					source:  cur,
					gen:     cur.genLoad(),
					recvGen: meta.gen.Load(),
				}
			}
			if desc.Get != nil {
				return desc.Get(v)
			}
			return desc.Value
		}
	}
	return NewUndefined()
}

// prototypeGet walks the prototype chain for an object with no own properties.
func (v *JSValue) prototypeGet(name string) *JSValue {
	var next *JSValue
	for cur := v; cur != nil; cur = next {
		curExt := cur.extOrNil()
		next = cur.prototype
		if curExt == nil {
			continue
		}
		if desc, ok := curExt.properties.Get(name); ok {
			if desc.Get != nil {
				return desc.Get(v)
			}
			return desc.Value
		}
	}
	return NewUndefined()
}

// Set sets an own property. No locking needed: event loop serializes all JS execution.
func (v *JSValue) Set(name string, value *JSValue) {
	if v == nil || v.typ == TypeUndefined || v.typ == TypeNull {
		return
	}
	if v.isPrimitiveValue() {
		panicPrimitivePropertySet()
	}
	if v.frozen {
		panicPrimitivePropertySet()
	}

	// Invalidate inline cache
	ext := v.extOrNil()
	if ext != nil {
		if meta := ext.meta.Load(); meta != nil {
			meta.gen.Add(1)
		}
	}

	// Fast path: own-property exists as data descriptor
	props := v.propertiesOrZero()
	if desc, ok := props.Get(name); ok {
		if desc.Set != nil {
			desc.Set(v, value)
			return
		}
		desc.Value = value
		return
	}
	// Walk prototype chain for inherited accessor descriptors
	for proto := v.prototype; proto != nil; proto = proto.prototype {
		protoExt := proto.extOrNil()
		if protoExt == nil {
			continue
		}
		if desc, ok := protoExt.properties.Get(name); ok && desc.Set != nil {
			desc.Set(v, value)
			return
		}
	}
	// Array index fast path
	if v.isArr {
		if idx, ok := parseArrayIndex(name); ok {
			arr := v.arrayListOrZero()
			for idx >= arr.Len() {
				arr.Push(nil)
			}
			arr.Set(idx, value)
			return
		}
	}
	// Create new data descriptor
	props.Set(name, newWritableEnumerableDataDescriptor(value))
}

// GetOwnProperty returns the property descriptor for an own property, or nil.
func (v *JSValue) GetOwnProperty(name string) *PropertyDescriptor {
	props := v.propertiesOrNil()
	if props == nil {
		return nil
	}
	desc, _ := props.Get(name)
	return desc
}

// DefineProperty sets a property descriptor directly.
// Type guard: no-op for undefined/null receivers (protects singletons).
func (v *JSValue) DefineProperty(name string, desc *PropertyDescriptor) {
	if v == nil || v.typ == TypeUndefined || v.typ == TypeNull {
		return
	}
	if v.isPrimitiveValue() {
		panicPrimitiveDefineProperty()
	}
	if v.frozen {
		panicPrimitiveDefineProperty()
	}
	// Invalidate inline cache
	ext := v.extOrNil()
	if ext != nil {
		if meta := ext.meta.Load(); meta != nil {
			meta.gen.Add(1)
		}
	}
	v.propertiesOrZero().Set(name, desc)
}

// HasOwnProperty returns true if the value has the named own property.
func (v *JSValue) HasOwnProperty(name string) bool {
	if v == nil {
		return false
	}
	if v.isBoxedPrimitive() {
		if props := v.propertiesOrNil(); props != nil {
			if _, ok := props.Get(name); ok {
				return true
			}
		}
	}
	props := v.propertiesOrNil()
	if props == nil {
		return false
	}
	return props.Has(name)
}

// OwnKeys returns the names of all own properties.
func (v *JSValue) OwnKeys() []string {
	if v == nil {
		return nil
	}
	if v.isPrimitiveValue() {
		v = boxedPrimitiveOf(v)
	}
	props := v.propertiesOrNil()
	if props == nil {
		return nil
	}
	return props.Keys()
}

// EnumerableOwnKeys returns the names of enumerable own properties only.
func (v *JSValue) EnumerableOwnKeys() []string {
	if v == nil {
		return nil
	}
	if v.isPrimitiveValue() {
		v = boxedPrimitiveOf(v)
	}
	props := v.propertiesOrNil()
	if props == nil {
		return nil
	}
	keys := make([]string, 0, props.Len())
	props.ForEach(func(k string, desc *PropertyDescriptor) {
		if desc != nil && desc.Enumerable {
			keys = append(keys, k)
		}
	})
	return keys
}
