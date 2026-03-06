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

// NewMap creates an empty Map JSValue with methods on MapPrototype.
func NewMap() *JSValue {
	return &JSValue{typ: TypeMap, mapVal: &jsMap{}, properties: make(map[string]*PropertyDescriptor), prototype: MapPrototype}
}
