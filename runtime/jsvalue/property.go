package jsvalue

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
func (v *JSValue) Get(name string) *JSValue {
	// __proto__ returns the internal prototype link, not an own property.
	// This is safe from prototype pollution because Set("__proto__", x)
	// creates an own property — it never modifies the internal chain.
	if name == "__proto__" {
		return v.GetPrototype()
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

// Set sets an own property with a data descriptor (writable, enumerable, configurable).
func (v *JSValue) Set(name string, value *JSValue) {
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
