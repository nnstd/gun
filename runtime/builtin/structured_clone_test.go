package jsvalue

import (
	"testing"
)

func TestStructuredClonePrimitives(t *testing.T) {
	// Frozen singletons (undefined, null, booleans, small ints, empty string) are
	// returned as-is by design; all non-frozen primitives get new allocations.
	cases := []*JSValue{
		NewUndefined(),
		NewNull(),
		NewBool(true),
		NewBool(false),
		NewNumber(42),
		NewNumber(0),
		NewString("hello"),
		NewString(""),
		{typ: TypeBigInt, bigIntVal: 12345},
	}
	for _, v := range cases {
		clone := StructuredClone(v)
		if clone.typ != v.typ {
			t.Errorf("type mismatch: got %v want %v", clone.typ, v.typ)
		}
		if !v.frozen && clone == v {
			t.Errorf("clone should be a new pointer for type %v", v.typ)
		}
	}
}

func TestStructuredCloneObjectDeep(t *testing.T) {
	inner := NewObject()
	inner.Set("c", NewNumber(2))
	outer := NewObject()
	outer.Set("b", inner)

	clone := StructuredClone(outer)

	// different pointers
	if clone == outer {
		t.Fatal("clone should not be same pointer as original")
	}
	cloneInner := clone.Get("b")
	if cloneInner == inner {
		t.Fatal("nested object should be a new pointer")
	}

	// values match
	if got := cloneInner.Get("c").Number(); got != 2 {
		t.Errorf("inner.c: got %v want 2", got)
	}

	// mutation isolation
	cloneInner.Set("c", NewNumber(99))
	if got := inner.Get("c").Number(); got != 2 {
		t.Errorf("original inner.c mutated: got %v want 2", got)
	}
}

func TestStructuredCloneArray(t *testing.T) {
	inner := NewArray(NewNumber(2), NewNumber(3))
	arr := NewArray(NewNumber(1), inner)

	clone := StructuredClone(arr)

	if clone == arr {
		t.Fatal("clone should not be same pointer")
	}
	cloneInner := clone.Index(1)
	if cloneInner == inner {
		t.Fatal("nested array should be a new pointer")
	}
	if got := cloneInner.Index(0).Number(); got != 2 {
		t.Errorf("clone inner[0]: got %v want 2", got)
	}

	// mutation isolation
	cloneInner.arrayVal.Set(0, NewNumber(99))
	if got := inner.Index(0).Number(); got != 2 {
		t.Errorf("original inner[0] mutated: got %v want 2", got)
	}
}

func TestStructuredCloneMap(t *testing.T) {
	m := NewMap()
	val := NewObject()
	val.Set("v", NewNumber(42))
	m.mapVal.entries = append(m.mapVal.entries, &jsMapEntry{NewString("key"), val})

	clone := StructuredClone(m)

	if clone == m {
		t.Fatal("clone should not be same pointer")
	}
	if clone.typ != TypeMap {
		t.Fatalf("clone type: got %v want TypeMap", clone.typ)
	}
	if len(clone.mapVal.entries) != 1 {
		t.Fatalf("clone map entries: got %d want 1", len(clone.mapVal.entries))
	}
	clonedVal := clone.mapVal.entries[0].value
	if clonedVal == val {
		t.Fatal("map value should be a new pointer")
	}
	if got := clonedVal.Get("v").Number(); got != 42 {
		t.Errorf("map value .v: got %v want 42", got)
	}

	// mutation isolation
	clonedVal.Set("v", NewNumber(99))
	if got := val.Get("v").Number(); got != 42 {
		t.Errorf("original map value mutated: got %v want 42", got)
	}
}

func TestStructuredCloneSet(t *testing.T) {
	s := NewSet()
	s.setVal.items = append(s.setVal.items, NewNumber(1), NewNumber(2))

	clone := StructuredClone(s)

	if clone == s {
		t.Fatal("clone should not be same pointer")
	}
	if clone.typ != TypeSet {
		t.Fatalf("clone type: got %v want TypeSet", clone.typ)
	}
	if len(clone.setVal.items) != 2 {
		t.Fatalf("clone set items: got %d want 2", len(clone.setVal.items))
	}

	// clear original — clone should be unaffected
	s.setVal.items = nil
	if len(clone.setVal.items) != 2 {
		t.Error("clone set items changed when original was cleared")
	}
}

func TestStructuredCloneCircularReference(t *testing.T) {
	obj := NewObject()
	obj.Set("self", obj)

	clone := StructuredClone(obj)

	if clone == obj {
		t.Fatal("clone should not be same pointer")
	}
	cloneSelf := clone.Get("self")
	if cloneSelf != clone {
		t.Errorf("circular ref not preserved: clone.self should === clone")
	}
}

func TestStructuredCloneTransfer(t *testing.T) {
	arr := NewArray(NewNumber(1), NewNumber(2), NewNumber(3))
	container := NewObject()
	container.Set("data", arr)

	opts := NewObject()
	transferList := NewArray(arr)
	opts.Set("transfer", transferList)

	clone := StructuredClone(container, opts)

	// clone has the data
	cloneData := clone.Get("data")
	if cloneData.arrayVal.Len() != 3 {
		t.Errorf("clone.data.length: got %d want 3", cloneData.arrayVal.Len())
	}

	// original arr is detached
	if arr.arrayVal.Len() != 0 {
		t.Errorf("original arr.length after transfer: got %d want 0", arr.arrayVal.Len())
	}
}

func TestStructuredCloneTransferMap(t *testing.T) {
	m := NewMap()
	m.mapVal.entries = append(m.mapVal.entries, &jsMapEntry{NewString("k"), NewNumber(1)})

	opts := NewObject()
	opts.Set("transfer", NewArray(m))

	wrapper := NewObject()
	wrapper.Set("m", m)

	clone := StructuredClone(wrapper, opts)

	// clone has the map entry
	clonedMap := clone.Get("m")
	if clonedMap.typ != TypeMap {
		t.Fatalf("clone.m type: got %v want TypeMap", clonedMap.typ)
	}
	if len(clonedMap.mapVal.entries) != 1 {
		t.Errorf("clone.m entries: got %d want 1", len(clonedMap.mapVal.entries))
	}

	// original map is detached
	if len(m.mapVal.entries) != 0 {
		t.Errorf("original map entries after transfer: got %d want 0", len(m.mapVal.entries))
	}
}

func TestStructuredCloneFunctionPanics(t *testing.T) {
	fn := NewFunction(func(args ...*JSValue) *JSValue { return NewUndefined() })
	defer func() {
		r := recover()
		if r == nil {
			t.Error("expected panic for function clone")
		}
	}()
	StructuredClone(fn)
}

func TestStructuredCloneSymbolPanics(t *testing.T) {
	sym := NewSymbol("test")
	defer func() {
		r := recover()
		if r == nil {
			t.Error("expected panic for symbol clone")
		}
	}()
	StructuredClone(sym)
}

func TestStructuredCloneNoOptions(t *testing.T) {
	obj := NewObject()
	obj.Set("x", NewNumber(1))
	clone := StructuredClone(obj)
	if clone.Get("x").Number() != 1 {
		t.Error("clone without options should work")
	}
}
