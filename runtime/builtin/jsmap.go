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
	return &JSValue{typ: TypeMap, mapVal: &jsMap{}, prototype: MapPrototype}
}

func initMapPrototype() {
	MapPrototype = &JSValue{typ: TypeObject, prototype: ObjectPrototype}

	defGetter(MapPrototype, "size", func(this *JSValue) *JSValue {
		if this == nil || this.mapVal == nil { return NewNumber(0) }
		return NewNumber(float64(len(this.mapVal.entries)))
	})
	defMethod(MapPrototype, "get", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil || args[0].mapVal == nil { return NewUndefined() }
		if len(args) < 2 { return NewUndefined() }
		if i := args[0].mapVal.find(args[1]); i >= 0 { return args[0].mapVal.entries[i].value }
		return NewUndefined()
	})
	defMethod(MapPrototype, "set", func(args ...*JSValue) *JSValue {
		if len(args) < 3 || args[0] == nil || args[0].mapVal == nil {
			if len(args) > 0 { return args[0] }
			return NewUndefined()
		}
		this := args[0]
		key, value := args[1], args[2]
		if i := this.mapVal.find(key); i >= 0 { this.mapVal.entries[i].value = value } else { this.mapVal.entries = append(this.mapVal.entries, &jsMapEntry{key, value}) }
		return this
	})
	defMethod(MapPrototype, "has", func(args ...*JSValue) *JSValue {
		if len(args) < 2 || args[0] == nil || args[0].mapVal == nil { return NewBool(false) }
		return NewBool(args[0].mapVal.find(args[1]) >= 0)
	})
	defMethod(MapPrototype, "delete", func(args ...*JSValue) *JSValue {
		if len(args) < 2 || args[0] == nil || args[0].mapVal == nil { return NewBool(false) }
		this := args[0]
		if i := this.mapVal.find(args[1]); i >= 0 {
			this.mapVal.entries = append(this.mapVal.entries[:i], this.mapVal.entries[i+1:]...)
			return NewBool(true)
		}
		return NewBool(false)
	})
	defMethod(MapPrototype, "clear", func(args ...*JSValue) *JSValue {
		if len(args) >= 1 && args[0] != nil && args[0].mapVal != nil { args[0].mapVal.entries = nil }
		return NewUndefined()
	})
	defMethod(MapPrototype, "keys", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil || args[0].mapVal == nil { return NewArray() }
		keys := make([]*JSValue, len(args[0].mapVal.entries))
		for i, e := range args[0].mapVal.entries { keys[i] = e.key }
		return NewArray(keys...)
	})
	defMethod(MapPrototype, "values", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil || args[0].mapVal == nil { return NewArray() }
		vals := make([]*JSValue, len(args[0].mapVal.entries))
		for i, e := range args[0].mapVal.entries { vals[i] = e.value }
		return NewArray(vals...)
	})
	defMethod(MapPrototype, "entries", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil || args[0].mapVal == nil { return NewArray() }
		pairs := make([]*JSValue, len(args[0].mapVal.entries))
		for i, e := range args[0].mapVal.entries { pairs[i] = NewArray(e.key, e.value) }
		return NewArray(pairs...)
	})
	defMethod(MapPrototype, "forEach", func(args ...*JSValue) *JSValue {
		if len(args) < 2 || args[0] == nil || args[0].mapVal == nil { return NewUndefined() }
		fn := args[1]
		if fn == nil || fn.funcVal == nil { return NewUndefined() }
		for _, e := range args[0].mapVal.entries { fn.funcVal(e.value, e.key) }
		return NewUndefined()
	})
}
