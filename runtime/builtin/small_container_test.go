package jsvalue

import (
	"sync"
	"testing"
)

// ---------------------------------------------------------------------------
// SmallPropMap tests
// ---------------------------------------------------------------------------

func TestSmallPropMapInlineGetSet(t *testing.T) {
	var m SmallPropMap
	desc := &PropertyDescriptor{Value: NewNumber(1)}
	m.Set("a", desc)
	got, ok := m.Get("a")
	if !ok {
		t.Fatal("expected key 'a' to exist")
	}
	if got != desc {
		t.Fatal("expected to get back the same descriptor")
	}
}

func TestSmallPropMapInlineOverwrite(t *testing.T) {
	var m SmallPropMap
	d1 := &PropertyDescriptor{Value: NewNumber(1)}
	d2 := &PropertyDescriptor{Value: NewNumber(2)}
	m.Set("a", d1)
	m.Set("a", d2)
	got, ok := m.Get("a")
	if !ok {
		t.Fatal("expected key 'a' to exist")
	}
	if got != d2 {
		t.Fatal("expected overwritten descriptor")
	}
	if m.Len() != 1 {
		t.Fatalf("expected Len()=1, got %d", m.Len())
	}
}

func TestSmallPropMapInlineDelete(t *testing.T) {
	var m SmallPropMap
	m.Set("a", &PropertyDescriptor{Value: NewNumber(1)})
	m.Set("b", &PropertyDescriptor{Value: NewNumber(2)})
	m.Delete("a")
	if m.Has("a") {
		t.Fatal("expected 'a' to be deleted")
	}
	if !m.Has("b") {
		t.Fatal("expected 'b' to still exist")
	}
	if m.Len() != 1 {
		t.Fatalf("expected Len()=1, got %d", m.Len())
	}
}

func TestSmallPropMapInlineDeleteShift(t *testing.T) {
	var m SmallPropMap
	m.Set("a", &PropertyDescriptor{Value: NewNumber(1)})
	m.Set("b", &PropertyDescriptor{Value: NewNumber(2)})
	m.Set("c", &PropertyDescriptor{Value: NewNumber(3)})
	// Delete middle entry
	m.Delete("b")
	keys := m.Keys()
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(keys))
	}
	// Verify remaining keys are correct
	for _, k := range keys {
		if k != "a" && k != "c" {
			t.Fatalf("unexpected key: %s", k)
		}
	}
}

func TestSmallPropMapSpill(t *testing.T) {
	var m SmallPropMap
	// Fill inline capacity
	for i := 0; i < smallPropMapCapacity; i++ {
		m.Set(string(rune('a'+i)), &PropertyDescriptor{Value: NewNumber(float64(i))})
	}
	if m.overflow != nil {
		t.Fatal("should not spill at exactly capacity")
	}
	// One more should spill
	m.Set(string(rune('a'+smallPropMapCapacity)), &PropertyDescriptor{Value: NewNumber(float64(smallPropMapCapacity))})
	if m.overflow == nil {
		t.Fatal("expected to spill after exceeding capacity")
	}
	if m.Len() != smallPropMapCapacity+1 {
		t.Fatalf("expected Len()=%d, got %d", smallPropMapCapacity+1, m.Len())
	}
	// Verify all entries are accessible
	for i := 0; i <= smallPropMapCapacity; i++ {
		key := string(rune('a' + i))
		desc, ok := m.Get(key)
		if !ok {
			t.Fatalf("expected key %q to exist", key)
		}
		if desc.Value.Number() != float64(i) {
			t.Fatalf("expected value %d for key %q", i, key)
		}
	}
}

func TestSmallPropMapSpillOverwrite(t *testing.T) {
	var m SmallPropMap
	d1 := &PropertyDescriptor{Value: NewNumber(1)}
	m.Set("a", d1)
	// Force spill
	for i := 1; i <= smallPropMapCapacity; i++ {
		m.Set(string(rune('a'+i)), &PropertyDescriptor{Value: NewNumber(float64(i))})
	}
	d2 := &PropertyDescriptor{Value: NewNumber(99)}
	m.Set("a", d2)
	got, ok := m.Get("a")
	if !ok {
		t.Fatal("expected key 'a' to exist")
	}
	if got.Value.Number() != 99 {
		t.Fatalf("expected overwritten value 99, got %v", got.Value.Number())
	}
}

func TestSmallPropMapForEach(t *testing.T) {
	var m SmallPropMap
	m.Set("x", &PropertyDescriptor{Value: NewNumber(1)})
	m.Set("y", &PropertyDescriptor{Value: NewNumber(2)})
	seen := map[string]bool{}
	m.ForEach(func(key string, desc *PropertyDescriptor) {
		seen[key] = true
		if desc == nil {
			t.Fatalf("nil descriptor for key %q", key)
		}
	})
	if len(seen) != 2 || !seen["x"] || !seen["y"] {
		t.Fatalf("ForEach missed keys: %v", seen)
	}
}

func TestSmallPropMapKeys(t *testing.T) {
	var m SmallPropMap
	m.Set("a", &PropertyDescriptor{Value: NewNumber(1)})
	m.Set("b", &PropertyDescriptor{Value: NewNumber(2)})
	keys := m.Keys()
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(keys))
	}
}

func TestSmallPropMapDeleteNonExistent(t *testing.T) {
	var m SmallPropMap
	m.Delete("nonexistent") // should not panic
	if m.Len() != 0 {
		t.Fatalf("expected Len()=0, got %d", m.Len())
	}
}

func TestSmallPropMapGetNonExistent(t *testing.T) {
	var m SmallPropMap
	_, ok := m.Get("nonexistent")
	if ok {
		t.Fatal("expected ok=false for missing key")
	}
}

func TestSmallPropMapSpillDelete(t *testing.T) {
	var m SmallPropMap
	for i := 0; i <= smallPropMapCapacity; i++ {
		m.Set(string(rune('a'+i)), &PropertyDescriptor{Value: NewNumber(float64(i))})
	}
	m.Delete("b")
	if m.Has("b") {
		t.Fatal("expected 'b' to be deleted after spill")
	}
	if m.Len() != smallPropMapCapacity {
		t.Fatalf("expected Len()=%d, got %d", smallPropMapCapacity, m.Len())
	}
}

// ---------------------------------------------------------------------------
// SmallValueList tests
// ---------------------------------------------------------------------------

func TestSmallValueListPushGet(t *testing.T) {
	var l SmallValueList
	v := NewNumber(1)
	l.Push(v)
	if l.Len() != 1 {
		t.Fatalf("expected Len()=1, got %d", l.Len())
	}
	got := l.Get(0)
	if got != v {
		t.Fatal("expected to get back same value")
	}
}

func TestSmallValueListSet(t *testing.T) {
	var l SmallValueList
	v1 := NewNumber(1)
	v2 := NewNumber(2)
	l.Push(v1)
	l.Set(0, v2)
	if l.Get(0) != v2 {
		t.Fatal("expected Set to update value")
	}
}

func TestSmallValueListSpill(t *testing.T) {
	var l SmallValueList
	for i := 0; i < smallValueListCapacity; i++ {
		l.Push(NewNumber(float64(i)))
	}
	if l.overflow != nil {
		t.Fatal("should not spill at exactly capacity")
	}
	// One more should spill
	l.Push(NewNumber(float64(smallValueListCapacity)))
	if l.overflow == nil {
		t.Fatal("expected to spill after exceeding capacity")
	}
	if l.Len() != smallValueListCapacity+1 {
		t.Fatalf("expected Len()=%d, got %d", smallValueListCapacity+1, l.Len())
	}
	// Verify all elements accessible
	for i := 0; i <= smallValueListCapacity; i++ {
		got := l.Get(i)
		if got == nil || got.Number() != float64(i) {
			t.Fatalf("expected element %d to be %d", i, i)
		}
	}
}

func TestSmallValueListTruncate(t *testing.T) {
	var l SmallValueList
	l.Push(NewNumber(1))
	l.Push(NewNumber(2))
	l.Truncate()
	if l.Len() != 1 {
		t.Fatalf("expected Len()=1, got %d", l.Len())
	}
	if l.Get(0).Number() != 1 {
		t.Fatal("expected first element to remain")
	}
}

func TestSmallValueListRemoveFirst(t *testing.T) {
	var l SmallValueList
	l.Push(NewNumber(1))
	l.Push(NewNumber(2))
	l.RemoveFirst()
	if l.Len() != 1 {
		t.Fatalf("expected Len()=1, got %d", l.Len())
	}
	if l.Get(0).Number() != 2 {
		t.Fatal("expected second element to become first")
	}
}

func TestSmallValueListPrepend(t *testing.T) {
	var l SmallValueList
	l.Push(NewNumber(2))
	l.Push(NewNumber(3))
	l.Prepend(NewNumber(1))
	if l.Len() != 3 {
		t.Fatalf("expected Len()=3, got %d", l.Len())
	}
	for i := 0; i < 3; i++ {
		if l.Get(i).Number() != float64(i+1) {
			t.Fatalf("expected element %d to be %d", i, i+1)
		}
	}
}

func TestSmallValueListReplaceAll(t *testing.T) {
	var l SmallValueList
	l.Push(NewNumber(1))
	l.Push(NewNumber(2))
	replacement := []*JSValue{NewNumber(10), NewNumber(20), NewNumber(30)}
	l.ReplaceAll(replacement)
	if l.Len() != 3 {
		t.Fatalf("expected Len()=3, got %d", l.Len())
	}
	if l.Get(0).Number() != 10 {
		t.Fatal("expected first element to be 10")
	}
}

func TestSmallValueListReplaceAllSmall(t *testing.T) {
	var l SmallValueList
	// Start with spilled list
	for i := 0; i <= smallValueListCapacity; i++ {
		l.Push(NewNumber(float64(i)))
	}
	// Replace with small list
	small := []*JSValue{NewNumber(42)}
	l.ReplaceAll(small)
	if l.Len() != 1 {
		t.Fatalf("expected Len()=1, got %d", l.Len())
	}
	if l.Get(0).Number() != 42 {
		t.Fatal("expected element to be 42")
	}
}

func TestSmallValueListExtendTo(t *testing.T) {
	var l SmallValueList
	l.ExtendTo(3)
	if l.Len() != 3 {
		t.Fatalf("expected Len()=3, got %d", l.Len())
	}
	for i := 0; i < 3; i++ {
		if l.Get(i) != nil {
			t.Fatalf("expected nil at index %d", i)
		}
	}
}

func TestSmallValueListSlice(t *testing.T) {
	var l SmallValueList
	l.Push(NewNumber(1))
	l.Push(NewNumber(2))
	s := l.Slice()
	if len(s) != 2 {
		t.Fatalf("expected len 2, got %d", len(s))
	}
	if s[0].Number() != 1 || s[1].Number() != 2 {
		t.Fatal("slice contents mismatch")
	}
}

func TestSmallValueListOutOfBounds(t *testing.T) {
	var l SmallValueList
	if got := l.Get(0); got != nil {
		t.Fatal("expected nil for empty list Get")
	}
	if got := l.Get(-1); got != nil {
		t.Fatal("expected nil for negative index")
	}
	l.Push(NewNumber(1))
	if got := l.Get(10); got != nil {
		t.Fatal("expected nil for out-of-bounds Get")
	}
}

func TestSmallValueListPrependSpill(t *testing.T) {
	var l SmallValueList
	// Fill to near capacity
	for i := 0; i < smallValueListCapacity-1; i++ {
		l.Push(NewNumber(float64(i)))
	}
	// Prepend 2 elements should spill
	l.Prepend(NewNumber(100), NewNumber(101))
	if l.overflow == nil {
		t.Fatal("expected to spill after prepend")
	}
	if l.Len() != smallValueListCapacity+1 {
		t.Fatalf("expected Len()=%d, got %d", smallValueListCapacity+1, l.Len())
	}
	if l.Get(0).Number() != 100 || l.Get(1).Number() != 101 {
		t.Fatal("prepended elements not at front")
	}
}

func TestSmallValueListSpillTruncate(t *testing.T) {
	var l SmallValueList
	for i := 0; i <= smallValueListCapacity; i++ {
		l.Push(NewNumber(float64(i)))
	}
	l.Truncate()
	if l.Len() != smallValueListCapacity {
		t.Fatalf("expected Len()=%d, got %d", smallValueListCapacity, l.Len())
	}
}

func TestSmallValueListSpillRemoveFirst(t *testing.T) {
	var l SmallValueList
	for i := 0; i <= smallValueListCapacity; i++ {
		l.Push(NewNumber(float64(i)))
	}
	l.RemoveFirst()
	if l.Len() != smallValueListCapacity {
		t.Fatalf("expected Len()=%d, got %d", smallValueListCapacity, l.Len())
	}
	if l.Get(0).Number() != 1 {
		t.Fatal("expected first element to shift after RemoveFirst")
	}
}

// ---------------------------------------------------------------------------
// Race tests
// ---------------------------------------------------------------------------

func TestSmallPropMapConcurrentReadWrite(t *testing.T) {
	// SmallPropMap itself is not thread-safe; it relies on JSValue's RWMutex.
	// Test that concurrent access through a JSValue with locking is race-free.
	v := NewObject()
	var wg sync.WaitGroup
	desc := &PropertyDescriptor{Value: NewNumber(42)}

	for i := 0; i < 100; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			v.rlock()
			v.properties.Get("shared")
			v.runlock()
		}()
		go func() {
			defer wg.Done()
			v.lock()
			v.properties.Set("shared", desc)
			v.unlock()
		}()
	}
	wg.Wait()
}

func TestSmallValueListConcurrentReadWrite(t *testing.T) {
	// SmallValueList itself is not thread-safe; it relies on JSValue's RWMutex.
	// Test that concurrent access through a JSValue with locking is race-free.
	v := NewArray(NewNumber(1))
	var wg sync.WaitGroup
	elem := NewNumber(42)

	for i := 0; i < 100; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			v.rlock()
			v.arrayVal.Get(0)
			v.runlock()
		}()
		go func() {
			defer wg.Done()
			v.lock()
			v.arrayVal.Push(elem)
			v.unlock()
		}()
	}
	wg.Wait()
}
