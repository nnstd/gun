package jsvalue

import (
	"testing"
)

// BenchmarkGetOwn benchmarks Get() for an own property (no prototype walk).
func BenchmarkGetOwn(b *testing.B) {
	obj := NewObject()
	obj.Set("x", NewNumber(42))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = obj.Get("x")
	}
}

// BenchmarkGetPrototype benchmarks Get() for a prototype-inherited property.
func BenchmarkGetPrototype(b *testing.B) {
	proto := NewObject()
	proto.Set("method", NewFunction(func(args ...*JSValue) *JSValue { return NewUndefined() }))
	obj := NewObject()
	obj.SetPrototype(proto)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = obj.Get("method")
	}
}

// BenchmarkSetOwn benchmarks Set() for an existing own property (fast path).
func BenchmarkSetOwn(b *testing.B) {
	obj := NewObject()
	obj.Set("x", NewNumber(1))
	val := NewNumber(42)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		obj.Set("x", val)
	}
}

// BenchmarkSetNew benchmarks Set() for a new property (map insert).
func BenchmarkSetNew(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		obj := NewObject()
		obj.Set("x", NewNumber(42))
	}
}

// BenchmarkGetMiss benchmarks Get() for a non-existent property.
func BenchmarkGetMiss(b *testing.B) {
	obj := NewObject()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = obj.Get("nonexistent")
	}
}

// BenchmarkGetCached benchmarks Get() after cache is warm (second access).
func BenchmarkGetCached(b *testing.B) {
	proto := NewObject()
	proto.Set("method", NewFunction(func(args ...*JSValue) *JSValue { return NewUndefined() }))
	obj := NewObject()
	obj.SetPrototype(proto)
	// Warm the cache
	_ = obj.Get("method")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = obj.Get("method")
	}
}
