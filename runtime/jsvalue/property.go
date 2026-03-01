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
	Get          func() *JSValue
	Set          func(*JSValue)
}

// Get retrieves a property by name, walking the prototype chain.
// Special: "__proto__" returns the internal prototype (not an own property).
// Nil-safe: returns undefined for nil receiver.
func (v *JSValue) Get(name string) *JSValue {
	if v == nil {
		return NewUndefined()
	}
	// __proto__ returns the internal prototype link, not an own property.
	// This is safe from prototype pollution because Set("__proto__", x)
	// creates an own property — it never modifies the internal chain.
	if name == "__proto__" {
		return v.GetPrototype()
	}
	// For arrays, numeric string keys access arrayVal (JS: arr["0"] === arr[0])
	if v.arrayVal != nil {
		if idx, ok := parseArrayIndex(name); ok && idx < len(v.arrayVal) {
			return v.arrayVal[idx]
		}
	}
	if v.properties != nil {
		if desc, ok := v.properties[name]; ok {
			if desc.Get != nil {
				return desc.Get()
			}
			return desc.Value
		}
	}
	if v.prototype != nil {
		return v.prototype.Get(name)
	}
	return NewUndefined()
}

// Set sets an own property. If an accessor (getter/setter) descriptor already exists
// on this object or its prototype chain, the setter is invoked instead of overwriting.
// Nil-safe: does nothing for nil receiver.
func (v *JSValue) Set(name string, value *JSValue) {
	if v == nil {
		return
	}
	// Check own properties first for an accessor descriptor
	if v.properties != nil {
		if desc, ok := v.properties[name]; ok && desc.Set != nil {
			desc.Set(value)
			return
		}
	}
	// Walk prototype chain for inherited accessor descriptors
	for proto := v.prototype; proto != nil; proto = proto.prototype {
		if proto.properties != nil {
			if desc, ok := proto.properties[name]; ok && desc.Set != nil {
				desc.Set(value)
				return
			}
		}
	}
	// No accessor found — create a data descriptor
	if v.properties == nil {
		v.properties = make(map[string]*PropertyDescriptor)
	}
	v.properties[name] = &PropertyDescriptor{
		Value:        value,
		Writable:     true,
		Enumerable:   true,
		Configurable: true,
	}
}

// GetOwnProperty returns the property descriptor for an own property, or nil.
func (v *JSValue) GetOwnProperty(name string) *PropertyDescriptor {
	if v.properties == nil {
		return nil
	}
	return v.properties[name]
}

// DefineProperty sets a property descriptor directly.
func (v *JSValue) DefineProperty(name string, desc *PropertyDescriptor) {
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
	_, ok := v.properties[name]
	return ok
}

// OwnKeys returns the names of all own properties.
func (v *JSValue) OwnKeys() []string {
	if v.properties == nil {
		return nil
	}
	keys := make([]string, 0, len(v.properties))
	for k := range v.properties {
		keys = append(keys, k)
	}
	return keys
}
