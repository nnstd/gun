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

// Get retrieves a property by name, walking the prototype chain.
// Uses double-checked locking: RLock for cache hits, Lock for cache misses.
// Prototype chain walk uses RLock per object with next-pointer pattern to prevent races.
// Special: "__proto__" returns the internal prototype (not an own property).
// Nil-safe: returns undefined for nil receiver.
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

	// Phase 1: RLock receiver — check cache + array/string fast paths
	v.rlock()
	if v.isArr {
		if idx, ok := parseArrayIndex(name); ok {
			if arr := v.arrayListOrNil(); arr != nil && idx < arr.Len() {
				val := arr.Get(idx)
				v.runlock()
				return val
			}
			v.runlock()
			return NewUndefined()
		}
	}
	if v.typ == TypeString {
		if idx, ok := parseArrayIndex(name); ok {
			v.runlock()
			return v.Index(idx)
		}
	}

	ext := v.extOrNil()
	if ext == nil {
		v.runlock()
		var next *JSValue
		for cur := v; cur != nil; cur = next {
			curExt := cur.extOrNil()
			next = cur.prototype
			if curExt == nil {
				continue
			}
			meta := curExt.meta
			if meta == nil {
				meta = cur.ensureMeta()
			}
			meta.mu.RLock()
			desc, ok := curExt.properties.Get(name)
			meta.mu.RUnlock()
			if ok {
				if desc.Get != nil {
					return desc.Get(v)
				}
				return desc.Value
			}
		}
		return NewUndefined()
	}

	meta := ext.meta
	if meta == nil {
		meta = v.ensureMeta()
	}
	recvGen := meta.gen.Load()
	cache := &meta.cache
	for i := range cache {
		e := &cache[i]
		if e.key == name && e.source != nil && e.recvGen == recvGen && e.gen == e.source.genLoad() {
			val := e.value
			getter := e.getter
			v.runlock()
			if getter != nil {
				return getter(v)
			}
			return val
		}
	}
	v.runlock()

	// Phase 2: Cache miss — Lock receiver, recheck, walk prototype chain
	v.lock()
	recvGen = v.genLoad()
	cache = v.cacheEntries()
	for i := range cache {
		e := &cache[i]
		if e.key == name && e.source != nil && e.recvGen == recvGen && e.gen == e.source.genLoad() {
			val := e.value
			getter := e.getter
			v.unlock()
			if getter != nil {
				return getter(v)
			}
			return val
		}
	}
	var next *JSValue
	for cur := v; cur != nil; cur = next {
		if cur != v {
			cur.rlock()
		}
		next = cur.prototype
		props := cur.propertiesOrNil()
		if props == nil {
			if cur != v {
				cur.runlock()
			}
			continue
		}
		if desc, ok := props.Get(name); ok {
			copy(cache[1:], cache[:3])
			cache[0] = cacheEntry{
				key:     name,
				value:   desc.Value,
				getter:  desc.Get,
				source:  cur,
				gen:     cur.genLoad(),
				recvGen: recvGen,
			}
			val := desc.Value
			if cur != v {
				cur.runlock()
			}
			v.unlock()
			if desc.Get != nil {
				return desc.Get(v)
			}
			return val
		}
		if cur != v {
			cur.runlock()
		}
	}
	v.unlock()
	return NewUndefined()
}

// Set sets an own property with Lock protection. If an accessor (getter/setter) descriptor
// already exists on this object or its prototype chain, the setter is invoked instead.
// Prototype chain walk uses RLock with next-pointer pattern to prevent races with SetPrototype().
// Nil-safe: does nothing for nil, undefined, or null receivers.
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
	v.lock()
	v.genAdd(1)
	// Fast path: own-property exists as data descriptor
	if desc, ok := v.propertiesOrZero().Get(name); ok {
		if desc.Set != nil {
			desc.Set(v, value)
			v.unlock()
			return
		}
		desc.Value = value
		v.unlock()
		return
	}
	// Walk prototype chain for inherited accessor descriptors
	var next *JSValue
	for proto := v.prototype; proto != nil; proto = next {
		proto.rlock()
		next = proto.prototype
		if desc, ok := proto.propertiesOrZero().Get(name); ok && desc.Set != nil {
			proto.runlock()
			v.unlock()
			desc.Set(v, value)
			return
		}
		proto.runlock()
	}
	// Array index fast path
	if v.isArr {
		if idx, ok := parseArrayIndex(name); ok {
			arr := v.arrayListOrZero()
			for idx >= arr.Len() {
				arr.Push(nil)
			}
			arr.Set(idx, value)
			v.unlock()
			return
		}
	}
	// Create new data descriptor
	v.propertiesOrZero().Set(name, newWritableEnumerableDataDescriptor(value))
	v.unlock()
}

// GetOwnProperty returns the property descriptor for an own property, or nil.
func (v *JSValue) GetOwnProperty(name string) *PropertyDescriptor {
	props := v.propertiesOrNil()
	if props == nil {
		return nil
	}
	v.rlock()
	desc, _ := props.Get(name)
	v.runlock()
	return desc
}

// DefineProperty sets a property descriptor directly with Lock protection.
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
	v.lock()
	defer v.unlock()
	v.genAdd(1)
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
	v.rlock()
	ok := props.Has(name)
	v.runlock()
	return ok
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
	v.rlock()
	keys := props.Keys()
	v.runlock()
	return keys
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
	v.rlock()
	keys := make([]string, 0, props.Len())
	props.ForEach(func(k string, desc *PropertyDescriptor) {
		if desc != nil && desc.Enumerable {
			keys = append(keys, k)
		}
	})
	v.runlock()
	return keys
}
