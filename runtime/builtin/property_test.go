package jsvalue

import (
	"strconv"
	"testing"
)

func TestParseArrayIndex(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	overflow := strconv.FormatUint(uint64(maxInt)+1, 10)

	tests := []struct {
		name string
		want int
		ok   bool
	}{
		{name: "", want: 0, ok: false},
		{name: "0", want: 0, ok: true},
		{name: "7", want: 7, ok: true},
		{name: "42", want: 42, ok: true},
		{name: "01", want: 0, ok: false},
		{name: "-1", want: 0, ok: false},
		{name: "name", want: 0, ok: false},
		{name: "12px", want: 0, ok: false},
		{name: overflow, want: 0, ok: false},
	}

	for _, tt := range tests {
		got, ok := parseArrayIndex(tt.name)
		if got != tt.want || ok != tt.ok {
			t.Fatalf("parseArrayIndex(%q) = (%d, %v), want (%d, %v)", tt.name, got, ok, tt.want, tt.ok)
		}
	}
}

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

func BenchmarkGetMissCold(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		obj := NewObject()
		_ = obj.Get("nonexistent")
	}
}

func BenchmarkGetStringIndex(b *testing.B) {
	s := NewString("hello")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = s.Get("1")
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
