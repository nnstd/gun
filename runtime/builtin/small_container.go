package jsvalue

const (
	smallPropMapCapacity  = 4
	smallValueListCapacity = 6
)

// ---------------------------------------------------------------------------
// SmallPropMap — inline property storage (≤4 entries), spills to map
// ---------------------------------------------------------------------------

// SmallPropMap stores up to smallPropMapCapacity key-value pairs inline
// without allocating a map. Once the inline capacity is exceeded it spills
// to a heap-allocated map and never goes back (matches V8 behaviour).
//
// The zero-value is ready to use — no initialisation required.
type SmallPropMap struct {
	// Inline storage. len is tracked by count; entries after count are zero-valued.
	keys   [smallPropMapCapacity]string
	vals   [smallPropMapCapacity]*PropertyDescriptor
	count  int
	overflow map[string]*PropertyDescriptor
}

// Get returns the descriptor for key, or (nil, false).
func (m *SmallPropMap) Get(key string) (*PropertyDescriptor, bool) {
	if m.overflow != nil {
		v, ok := m.overflow[key]
		return v, ok
	}
	for i := 0; i < m.count; i++ {
		if m.keys[i] == key {
			return m.vals[i], true
		}
	}
	return nil, false
}

// Set stores a key/descriptor pair, overwriting if the key already exists.
func (m *SmallPropMap) Set(key string, desc *PropertyDescriptor) {
	if m.overflow != nil {
		m.overflow[key] = desc
		return
	}
	// Check for overwrite in inline storage.
	for i := 0; i < m.count; i++ {
		if m.keys[i] == key {
			m.vals[i] = desc
			return
		}
	}
	// Append inline if room.
	if m.count < smallPropMapCapacity {
		m.keys[m.count] = key
		m.vals[m.count] = desc
		m.count++
		return
	}
	// Spill to map.
	m.overflow = make(map[string]*PropertyDescriptor, smallPropMapCapacity+1)
	for i := 0; i < m.count; i++ {
		m.overflow[m.keys[i]] = m.vals[i]
	}
	// Clear inline references so GC can collect.
	for i := 0; i < m.count; i++ {
		m.keys[i] = ""
		m.vals[i] = nil
	}
	m.count = 0
	m.overflow[key] = desc
}

// Delete removes a key. For inline storage, remaining entries are shifted down.
func (m *SmallPropMap) Delete(key string) {
	if m.overflow != nil {
		delete(m.overflow, key)
		return
	}
	for i := 0; i < m.count; i++ {
		if m.keys[i] == key {
			// Shift remaining entries down.
			copy(m.keys[i:], m.keys[i+1:m.count])
			copy(m.vals[i:], m.vals[i+1:m.count])
			m.count--
			m.keys[m.count] = ""
			m.vals[m.count] = nil
			return
		}
	}
}

// Has returns true if the key exists.
func (m *SmallPropMap) Has(key string) bool {
	if m.overflow != nil {
		_, ok := m.overflow[key]
		return ok
	}
	for i := 0; i < m.count; i++ {
		if m.keys[i] == key {
			return true
		}
	}
	return false
}

// Len returns the number of entries.
func (m *SmallPropMap) Len() int {
	if m.overflow != nil {
		return len(m.overflow)
	}
	return m.count
}

// ForEach calls fn for each entry. Iteration order is undefined.
func (m *SmallPropMap) ForEach(fn func(key string, desc *PropertyDescriptor)) {
	if m.overflow != nil {
		for k, v := range m.overflow {
			fn(k, v)
		}
		return
	}
	for i := 0; i < m.count; i++ {
		fn(m.keys[i], m.vals[i])
	}
}

// Keys returns all keys as a freshly allocated slice.
func (m *SmallPropMap) Keys() []string {
	if m.overflow != nil {
		keys := make([]string, 0, len(m.overflow))
		for k := range m.overflow {
			keys = append(keys, k)
		}
		return keys
	}
	keys := make([]string, m.count)
	copy(keys, m.keys[:m.count])
	return keys
}

// ---------------------------------------------------------------------------
// SmallValueList — inline element storage (≤6 entries), spills to slice
// ---------------------------------------------------------------------------

// SmallValueList stores up to smallValueListCapacity *JSValue pointers inline
// without allocating a slice. Once the inline capacity is exceeded it spills
// to a heap-allocated slice and never goes back.
//
// The zero-value is ready to use — no initialisation required.
type SmallValueList struct {
	inline  [smallValueListCapacity]*JSValue
	count   int
	overflow []*JSValue
}

// Len returns the number of elements.
func (l *SmallValueList) Len() int {
	if l.overflow != nil {
		return len(l.overflow)
	}
	return l.count
}

// Get returns the element at index i. Returns nil if out of bounds.
func (l *SmallValueList) Get(i int) *JSValue {
	if l.overflow != nil {
		if i < 0 || i >= len(l.overflow) {
			return nil
		}
		return l.overflow[i]
	}
	if i < 0 || i >= l.count {
		return nil
	}
	return l.inline[i]
}

// Set sets the element at index i. No-op if out of bounds.
func (l *SmallValueList) Set(i int, v *JSValue) {
	if l.overflow != nil {
		if i >= 0 && i < len(l.overflow) {
			l.overflow[i] = v
		}
		return
	}
	if i >= 0 && i < l.count {
		l.inline[i] = v
	}
}

// Push appends an element.
func (l *SmallValueList) Push(v *JSValue) {
	if l.overflow != nil {
		l.overflow = append(l.overflow, v)
		return
	}
	if l.count < smallValueListCapacity {
		l.inline[l.count] = v
		l.count++
		return
	}
	// Spill to slice.
	l.overflow = make([]*JSValue, smallValueListCapacity, smallValueListCapacity+1)
	copy(l.overflow, l.inline[:])
	for i := 0; i < smallValueListCapacity; i++ {
		l.inline[i] = nil
	}
	l.count = 0
	l.overflow = append(l.overflow, v)
}

// ExtendTo grows the list to at least n elements by appending nils.
// If the list already has >= n elements, it is unchanged.
func (l *SmallValueList) ExtendTo(n int) {
	cur := l.Len()
	for i := cur; i < n; i++ {
		l.Push(nil)
	}
}

// Slice returns the elements as a slice. WARNING: for inline storage, the
// returned slice aliases the internal array — callers that need a
// mutation-safe copy must copy it themselves.
func (l *SmallValueList) Slice() []*JSValue {
	if l.overflow != nil {
		return l.overflow
	}
	return l.inline[:l.count]
}

// ReplaceAll replaces the entire content with the provided slice.
func (l *SmallValueList) ReplaceAll(s []*JSValue) {
	if l.overflow != nil {
		l.overflow = s
		return
	}
	// If the new slice fits inline, copy it in.
	if len(s) <= smallValueListCapacity {
		l.count = 0
		for _, v := range s {
			l.inline[l.count] = v
			l.count++
		}
		// Clear remaining slots.
		for i := l.count; i < smallValueListCapacity; i++ {
			l.inline[i] = nil
		}
		return
	}
	// Spill.
	for i := 0; i < smallValueListCapacity; i++ {
		l.inline[i] = nil
	}
	l.count = 0
	l.overflow = s
}

// Truncate removes the last element. No-op if empty.
func (l *SmallValueList) Truncate() {
	if l.overflow != nil {
		if len(l.overflow) > 0 {
			l.overflow = l.overflow[:len(l.overflow)-1]
		}
		return
	}
	if l.count > 0 {
		l.count--
		l.inline[l.count] = nil
	}
}

// RemoveFirst removes the first element and shifts remaining elements left.
// No-op if empty.
func (l *SmallValueList) RemoveFirst() {
	if l.overflow != nil {
		if len(l.overflow) > 0 {
			l.overflow = l.overflow[1:]
		}
		return
	}
	if l.count <= 0 {
		return
	}
	copy(l.inline[:], l.inline[1:l.count])
	l.count--
	l.inline[l.count] = nil
}

// Prepend inserts elements at the beginning of the list.
func (l *SmallValueList) Prepend(elems ...*JSValue) {
	if len(elems) == 0 {
		return
	}
	if l.overflow != nil {
		result := make([]*JSValue, 0, len(elems)+len(l.overflow))
		result = append(result, elems...)
		result = append(result, l.overflow...)
		l.overflow = result
		return
	}
	newLen := l.count + len(elems)
	if newLen <= smallValueListCapacity {
		// Shift existing elements right to make room.
		copy(l.inline[len(elems):l.count+len(elems)], l.inline[:l.count])
		copy(l.inline[:len(elems)], elems)
		l.count = newLen
		return
	}
	// Spill.
	result := make([]*JSValue, 0, newLen)
	result = append(result, elems...)
	result = append(result, l.inline[:l.count]...)
	for i := 0; i < smallValueListCapacity; i++ {
		l.inline[i] = nil
	}
	l.count = 0
	l.overflow = result
}
