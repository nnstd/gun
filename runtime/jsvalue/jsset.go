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

// NewSet creates an empty Set JSValue.
func NewSet() *JSValue {
	return &JSValue{typ: TypeSet, setVal: &jsSet{}}
}

// SetAdd adds a value to the set and returns the set.
func SetAdd(s *JSValue, value *JSValue) *JSValue {
	if s == nil || s.setVal == nil {
		return s
	}
	if s.setVal.find(value) < 0 {
		s.setVal.items = append(s.setVal.items, value)
	}
	return s
}

// SetHas returns whether the set contains value.
func SetHas(s *JSValue, value *JSValue) *JSValue {
	if s == nil || s.setVal == nil {
		return NewBool(false)
	}
	return NewBool(s.setVal.find(value) >= 0)
}

// SetDelete removes value from the set.
func SetDelete(s *JSValue, value *JSValue) *JSValue {
	if s == nil || s.setVal == nil {
		return NewBool(false)
	}
	if i := s.setVal.find(value); i >= 0 {
		s.setVal.items = append(s.setVal.items[:i], s.setVal.items[i+1:]...)
		return NewBool(true)
	}
	return NewBool(false)
}

// SetClear removes all entries.
func SetClear(s *JSValue) {
	if s != nil && s.setVal != nil {
		s.setVal.items = nil
	}
}

// SetSize returns the number of elements.
func SetSize(s *JSValue) *JSValue {
	if s == nil || s.setVal == nil {
		return NewNumber(0)
	}
	return NewNumber(float64(len(s.setVal.items)))
}

// SetValues returns an array of values in insertion order.
func SetValues(s *JSValue) *JSValue {
	if s == nil || s.setVal == nil {
		return NewArray()
	}
	elems := make([]*JSValue, len(s.setVal.items))
	copy(elems, s.setVal.items)
	return NewArray(elems...)
}

// SetKeys is an alias for SetValues (JS Set.keys() === Set.values()).
func SetKeys(s *JSValue) *JSValue {
	return SetValues(s)
}

// SetEntries returns an array of [value, value] pairs.
func SetEntries(s *JSValue) *JSValue {
	if s == nil || s.setVal == nil {
		return NewArray()
	}
	pairs := make([]*JSValue, len(s.setVal.items))
	for i, item := range s.setVal.items {
		pairs[i] = NewArray(item, item)
	}
	return NewArray(pairs...)
}

// SetForEach calls fn(value) for each element.
func SetForEach(s *JSValue, fn *JSValue) {
	if s == nil || s.setVal == nil || fn == nil || fn.funcVal == nil {
		return
	}
	for _, item := range s.setVal.items {
		fn.funcVal(item)
	}
}
