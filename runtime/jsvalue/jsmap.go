package jsvalue

type jsMapEntry struct {
	key   *JSValue
	value *JSValue
}

type jsMap struct {
	entries []*jsMapEntry
}

func (m *jsMap) find(key *JSValue) int {
	for i, e := range m.entries {
		if jsValueEqual(e.key, key) {
			return i
		}
	}
	return -1
}

// jsValueEqual compares two JSValues by type and value (not pointer identity).
func jsValueEqual(a, b *JSValue) bool {
	if a == b {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if a.typ != b.typ {
		return false
	}
	switch a.typ {
	case TypeNumber:
		return a.numVal == b.numVal
	case TypeString:
		return a.strVal == b.strVal
	case TypeBoolean:
		return a.boolVal == b.boolVal
	case TypeNull, TypeUndefined:
		return true
	case TypeBigInt:
		return a.bigIntVal == b.bigIntVal
	case TypeSymbol:
		return a.symbolID == b.symbolID
	default:
		return a == b // reference equality for objects/functions
	}
}

// NewMap creates an empty Map JSValue.
func NewMap() *JSValue {
	return &JSValue{typ: TypeMap, mapVal: &jsMap{}}
}

// MapGet returns the value for key, or undefined.
func MapGet(m *JSValue, key *JSValue) *JSValue {
	if m == nil || m.mapVal == nil {
		return NewUndefined()
	}
	if i := m.mapVal.find(key); i >= 0 {
		return m.mapVal.entries[i].value
	}
	return NewUndefined()
}

// MapSet sets key to value and returns the map.
func MapSet(m *JSValue, key *JSValue, value *JSValue) *JSValue {
	if m == nil || m.mapVal == nil {
		return m
	}
	if i := m.mapVal.find(key); i >= 0 {
		m.mapVal.entries[i].value = value
	} else {
		m.mapVal.entries = append(m.mapVal.entries, &jsMapEntry{key, value})
	}
	return m
}

// MapHas returns whether the map contains key.
func MapHas(m *JSValue, key *JSValue) *JSValue {
	if m == nil || m.mapVal == nil {
		return NewBool(false)
	}
	return NewBool(m.mapVal.find(key) >= 0)
}

// MapDelete removes key from the map.
func MapDelete(m *JSValue, key *JSValue) *JSValue {
	if m == nil || m.mapVal == nil {
		return NewBool(false)
	}
	if i := m.mapVal.find(key); i >= 0 {
		m.mapVal.entries = append(m.mapVal.entries[:i], m.mapVal.entries[i+1:]...)
		return NewBool(true)
	}
	return NewBool(false)
}

// MapClear removes all entries.
func MapClear(m *JSValue) {
	if m != nil && m.mapVal != nil {
		m.mapVal.entries = nil
	}
}

// MapSize returns the number of entries.
func MapSize(m *JSValue) *JSValue {
	if m == nil || m.mapVal == nil {
		return NewNumber(0)
	}
	return NewNumber(float64(len(m.mapVal.entries)))
}

// MapKeys returns an array of keys in insertion order.
func MapKeys(m *JSValue) *JSValue {
	if m == nil || m.mapVal == nil {
		return NewArray()
	}
	keys := make([]*JSValue, len(m.mapVal.entries))
	for i, e := range m.mapVal.entries {
		keys[i] = e.key
	}
	return NewArray(keys...)
}

// MapValues returns an array of values in insertion order.
func MapValues(m *JSValue) *JSValue {
	if m == nil || m.mapVal == nil {
		return NewArray()
	}
	vals := make([]*JSValue, len(m.mapVal.entries))
	for i, e := range m.mapVal.entries {
		vals[i] = e.value
	}
	return NewArray(vals...)
}

// MapEntries returns an array of [key, value] pairs.
func MapEntries(m *JSValue) *JSValue {
	if m == nil || m.mapVal == nil {
		return NewArray()
	}
	pairs := make([]*JSValue, len(m.mapVal.entries))
	for i, e := range m.mapVal.entries {
		pairs[i] = NewArray(e.key, e.value)
	}
	return NewArray(pairs...)
}

// MapForEach calls fn(value, key) for each entry.
func MapForEach(m *JSValue, fn *JSValue) {
	if m == nil || m.mapVal == nil || fn == nil || fn.funcVal == nil {
		return
	}
	for _, e := range m.mapVal.entries {
		fn.funcVal(e.value, e.key)
	}
}
