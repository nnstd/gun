package jsvalue

import "testing"

func TestFromEntriesBuildsObject(t *testing.T) {
	entries := NewArray(
		NewArray(NewString("path"), NewString("/")),
		NewArray(NewString("method"), NewString("GET")),
	)

	obj := FromEntries(entries)

	if got := obj.Get("path").String(); got != "/" {
		t.Fatalf("FromEntries path = %q, want %q", got, "/")
	}
	if got := obj.Get("method").String(); got != "GET" {
		t.Fatalf("FromEntries method = %q, want %q", got, "GET")
	}
}

func TestDefineAccessorGetterAndSetter(t *testing.T) {
	obj := NewObject()
	var stored *JSValue

	getter := NewFunction(func(args ...*JSValue) *JSValue {
		if stored == nil {
			return NewUndefined()
		}
		return stored
	})
	setter := NewFunction(func(args ...*JSValue) *JSValue {
		if len(args) < 2 {
			return NewUndefined()
		}
		stored = args[1]
		return NewUndefined()
	})

	DefineAccessor(obj, "value", getter, setter)
	obj.Set("value", NewString("Hono!"))

	if got := obj.Get("value").String(); got != "Hono!" {
		t.Fatalf("accessor round-trip = %q, want %q", got, "Hono!")
	}
}

func TestDefineAccessorPreservesExistingGetter(t *testing.T) {
	obj := NewObject()
	var stored = NewString("before")

	getter := NewFunction(func(args ...*JSValue) *JSValue {
		return stored
	})
	setter := NewFunction(func(args ...*JSValue) *JSValue {
		if len(args) >= 2 {
			stored = args[1]
		}
		return NewUndefined()
	})

	DefineAccessor(obj, "value", getter, nil)
	DefineAccessor(obj, "value", nil, setter)

	obj.Set("value", NewString("after"))

	if got := obj.Get("value").String(); got != "after" {
		t.Fatalf("preserved getter after setter install = %q, want %q", got, "after")
	}
}
