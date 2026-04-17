package jsvalue

type jsSet struct {
	items []*JSValue
}

func (s *jsSet) find(val *JSValue) int {
	for i, item := range s.items {
		if jsValueEqual(item, val) {
			return i
		}
	}
	return -1
}

// NewSet creates an empty Set JSValue with methods on SetPrototype.
func NewSet() *JSValue {
	return &JSValue{typ: TypeSet, setVal: &jsSet{}, prototype: SetPrototype}
}

func initSetPrototype() {
	SetPrototype = &JSValue{typ: TypeObject, prototype: ObjectPrototype}

	defGetter(SetPrototype, "size", func(this *JSValue) *JSValue {
		if this == nil || this.setVal == nil { return NewNumber(0) }
		return NewNumber(float64(len(this.setVal.items)))
	})
	defMethod(SetPrototype, "add", func(args ...*JSValue) *JSValue {
		if len(args) < 2 || args[0] == nil || args[0].setVal == nil {
			if len(args) > 0 { return args[0] }
			return NewUndefined()
		}
		this := args[0]
		if this.setVal.find(args[1]) < 0 { this.setVal.items = append(this.setVal.items, args[1]) }
		return this
	})
	defMethod(SetPrototype, "has", func(args ...*JSValue) *JSValue {
		if len(args) < 2 || args[0] == nil || args[0].setVal == nil { return NewBool(false) }
		return NewBool(args[0].setVal.find(args[1]) >= 0)
	})
	defMethod(SetPrototype, "delete", func(args ...*JSValue) *JSValue {
		if len(args) < 2 || args[0] == nil || args[0].setVal == nil { return NewBool(false) }
		this := args[0]
		if i := this.setVal.find(args[1]); i >= 0 {
			this.setVal.items = append(this.setVal.items[:i], this.setVal.items[i+1:]...)
			return NewBool(true)
		}
		return NewBool(false)
	})
	defMethod(SetPrototype, "clear", func(args ...*JSValue) *JSValue {
		if len(args) >= 1 && args[0] != nil && args[0].setVal != nil { args[0].setVal.items = nil }
		return NewUndefined()
	})
	defMethod(SetPrototype, "values", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil || args[0].setVal == nil { return NewArray() }
		elems := make([]*JSValue, len(args[0].setVal.items))
		copy(elems, args[0].setVal.items)
		return NewArray(elems...)
	})
	defMethod(SetPrototype, "keys", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil || args[0].setVal == nil { return NewArray() }
		elems := make([]*JSValue, len(args[0].setVal.items))
		copy(elems, args[0].setVal.items)
		return NewArray(elems...)
	})
	defMethod(SetPrototype, "entries", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil || args[0].setVal == nil { return NewArray() }
		pairs := make([]*JSValue, len(args[0].setVal.items))
		for i, item := range args[0].setVal.items { pairs[i] = NewArray(item, item) }
		return NewArray(pairs...)
	})
	defMethod(SetPrototype, "forEach", func(args ...*JSValue) *JSValue {
		if len(args) < 2 || args[0] == nil || args[0].setVal == nil { return NewUndefined() }
		fn := args[1]
		if fn == nil || fn.funcVal == nil { return NewUndefined() }
		for _, item := range args[0].setVal.items { fn.funcVal(item) }
		return NewUndefined()
	})
}
