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
	v := &JSValue{typ: TypeSet, prototype: SetPrototype}
	v.setSetState(&jsSet{})
	return v
}

func initSetPrototype() {
	SetPrototype = &JSValue{typ: TypeObject, prototype: ObjectPrototype}

	defGetter(SetPrototype, "size", func(this *JSValue) *JSValue {
		if this == nil || this.setState() == nil {
			return NewNumber(0)
		}
		return NewNumber(float64(len(this.setState().items)))
	})
	defMethod(SetPrototype, "add", func(args ...*JSValue) *JSValue {
		if len(args) < 2 || args[0] == nil || args[0].setState() == nil {
			if len(args) > 0 {
				return args[0]
			}
			return NewUndefined()
		}
		this := args[0]
		if this.setState().find(args[1]) < 0 {
			this.setState().items = append(this.setState().items, args[1])
		}
		return this
	})
	defMethod(SetPrototype, "has", func(args ...*JSValue) *JSValue {
		if len(args) < 2 || args[0] == nil || args[0].setState() == nil {
			return NewBool(false)
		}
		return NewBool(args[0].setState().find(args[1]) >= 0)
	})
	defMethod(SetPrototype, "delete", func(args ...*JSValue) *JSValue {
		if len(args) < 2 || args[0] == nil || args[0].setState() == nil {
			return NewBool(false)
		}
		this := args[0]
		if i := this.setState().find(args[1]); i >= 0 {
			this.setState().items = append(this.setState().items[:i], this.setState().items[i+1:]...)
			return NewBool(true)
		}
		return NewBool(false)
	})
	defMethod(SetPrototype, "clear", func(args ...*JSValue) *JSValue {
		if len(args) >= 1 && args[0] != nil && args[0].setState() != nil {
			args[0].setState().items = nil
		}
		return NewUndefined()
	})
	defMethod(SetPrototype, "values", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil || args[0].setState() == nil {
			return NewArray()
		}
		elems := make([]*JSValue, len(args[0].setState().items))
		copy(elems, args[0].setState().items)
		return NewArray(elems...)
	})
	defMethod(SetPrototype, "keys", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil || args[0].setState() == nil {
			return NewArray()
		}
		elems := make([]*JSValue, len(args[0].setState().items))
		copy(elems, args[0].setState().items)
		return NewArray(elems...)
	})
	defMethod(SetPrototype, "entries", func(args ...*JSValue) *JSValue {
		if len(args) < 1 || args[0] == nil || args[0].setState() == nil {
			return NewArray()
		}
		pairs := make([]*JSValue, len(args[0].setState().items))
		for i, item := range args[0].setState().items {
			pairs[i] = NewArray(item, item)
		}
		return NewArray(pairs...)
	})
	defMethod(SetPrototype, "forEach", func(args ...*JSValue) *JSValue {
		if len(args) < 2 || args[0] == nil || args[0].setState() == nil {
			return NewUndefined()
		}
		fn := args[1]
		if fn == nil || fn.funcVal == nil {
			return NewUndefined()
		}
		for _, item := range args[0].setState().items {
			fn.funcVal(item)
		}
		return NewUndefined()
	})
}
