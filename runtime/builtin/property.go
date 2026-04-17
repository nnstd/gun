package jsvalue

import "strconv"

// parseArrayIndex checks if a property name is a valid non-negative integer
// index (like "0", "1", "42") and returns the index. This enables JS-style
// arr["0"] === arr[0] semantics.
func parseArrayIndex(name string) (int, bool) {
	if len(name) == 0 || (name[0] == '0' && len(name) > 1) {
		return 0, name == "0" // "0" is valid, "01" is not
	}
	n, err := strconv.Atoi(name)
	if err != nil || n < 0 {
		return 0, false
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

// Get retrieves a property by name, walking the prototype chain.
// Uses double-checked locking: RLock for cache hits, Lock for cache misses.
// Prototype chain walk uses RLock per object with next-pointer pattern to prevent races.
// Special: "__proto__" returns the internal prototype (not an own property).
// Nil-safe: returns undefined for nil receiver.
func (v *JSValue) Get(name string) *JSValue {
	if v == nil {
		return NewUndefined()
	}
	if name == "__proto__" {
		return v.GetPrototype()
	}

	// Phase 1: RLock receiver — check cache + array/string fast paths
	v.rlock()
	if v.arrayVal != nil {
		if idx, ok := parseArrayIndex(name); ok {
			if idx < len(v.arrayVal) {
				val := v.arrayVal[idx]
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
	recvGen := v.gen.Load()
	for i := range v.cache {
		e := &v.cache[i]
		if e.key == name && e.source != nil && e.recvGen == recvGen && e.gen == e.source.gen.Load() {
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
	recvGen = v.gen.Load()
	for i := range v.cache {
		e := &v.cache[i]
		if e.key == name && e.source != nil && e.recvGen == recvGen && e.gen == e.source.gen.Load() {
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
		if cur.properties != nil {
			if desc, ok := cur.properties[name]; ok {
				copy(v.cache[1:], v.cache[:3])
				v.cache[0] = cacheEntry{
					key:     name,
					value:   desc.Value,
					getter:  desc.Get,
					source:  cur,
					gen:     cur.gen.Load(),
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
	v.lock()
	v.gen.Add(1)
	// Fast path: own-property exists as data descriptor
	if v.properties != nil {
		if desc, ok := v.properties[name]; ok {
			if desc.Set != nil {
				desc.Set(v, value)
				v.unlock()
				return
			}
			desc.Value = value
			v.unlock()
			return
		}
	}
	// Walk prototype chain for inherited accessor descriptors
	var next *JSValue
	for proto := v.prototype; proto != nil; proto = next {
		proto.rlock()
		next = proto.prototype
		if proto.properties != nil {
			if desc, ok := proto.properties[name]; ok && desc.Set != nil {
				proto.runlock()
				v.unlock()
				desc.Set(v, value)
				return
			}
		}
		proto.runlock()
	}
	// Array index fast path
	if v.arrayVal != nil {
		if idx, ok := parseArrayIndex(name); ok {
			for idx >= len(v.arrayVal) {
				v.arrayVal = append(v.arrayVal, nil)
			}
			v.arrayVal[idx] = value
			v.unlock()
			return
		}
	}
	// Create new data descriptor
	if v.properties == nil {
		v.properties = make(map[string]*PropertyDescriptor)
	}
	v.properties[name] = &PropertyDescriptor{
		Value:        value,
		Writable:     true,
		Enumerable:   true,
		Configurable: true,
	}
	v.unlock()
}

// GetOwnProperty returns the property descriptor for an own property, or nil.
func (v *JSValue) GetOwnProperty(name string) *PropertyDescriptor {
	if v.properties == nil {
		return nil
	}
	v.rlock()
	desc := v.properties[name]
	v.runlock()
	return desc
}

// DefineProperty sets a property descriptor directly with Lock protection.
// Type guard: no-op for undefined/null receivers (protects singletons).
func (v *JSValue) DefineProperty(name string, desc *PropertyDescriptor) {
	if v == nil || v.typ == TypeUndefined || v.typ == TypeNull {
		return
	}
	v.lock()
	defer v.unlock()
	v.gen.Add(1)
	if v.properties == nil {
		v.properties = make(map[string]*PropertyDescriptor)
	}
	v.properties[name] = desc
}

// HasOwnProperty returns true if the value has the named own property.
func (v *JSValue) HasOwnProperty(name string) bool {
	if v.properties == nil {
		return false
	}
	v.rlock()
	_, ok := v.properties[name]
	v.runlock()
	return ok
}

// OwnKeys returns the names of all own properties.
func (v *JSValue) OwnKeys() []string {
	if v.properties == nil {
		return nil
	}
	v.rlock()
	keys := make([]string, 0, len(v.properties))
	for k := range v.properties {
		keys = append(keys, k)
	}
	v.runlock()
	return keys
}

// EnumerableOwnKeys returns the names of enumerable own properties only.
func (v *JSValue) EnumerableOwnKeys() []string {
	if v.properties == nil {
		return nil
	}
	v.rlock()
	keys := make([]string, 0, len(v.properties))
	for k, desc := range v.properties {
		if desc != nil && desc.Enumerable {
			keys = append(keys, k)
		}
	}
	v.runlock()
	return keys
}
