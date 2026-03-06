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
